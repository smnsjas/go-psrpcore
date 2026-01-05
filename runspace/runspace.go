// Package runspace implements the PSRP RunspacePool state machine.
//
// A RunspacePool represents a pool of PowerShell runspaces on the remote server.
// It manages the lifecycle and state transitions required by the PSRP protocol.
//
// # State Machine
//
// The RunspacePool follows a strict state machine:
//
//	BeforeOpen → Opening → Opened → Closing → Closed
//	             ↓           ↓         ↓
//	             └─────→ Broken ←──────┘
//
// State transitions:
//   - BeforeOpen: Initial state, no connection established
//   - Opening: Capability exchange and initialization in progress
//   - Opened: Pool is ready for pipeline execution
//   - Closing: Close requested, cleanup in progress
//   - Closed: Pool is closed and cannot be reopened
//   - Broken: Error occurred, pool is in failed state
//
// # Message Exchange
//
// Opening sequence:
//  1. Client sends SESSION_CAPABILITY
//  2. Server responds with SESSION_CAPABILITY
//  3. Client sends INIT_RUNSPACEPOOL
//  4. Server responds with RUNSPACEPOOL_STATE (Opened)
//
// # Usage
//
//	pool := runspace.New(client, uuid.New())
//	pool.SetMinRunspaces(1)
//	pool.SetMaxRunspaces(5)
//
//	err := pool.Open(ctx)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer pool.Close(ctx)
//
// # Reference
//
// MS-PSRP Section 3.2.2: https://docs.microsoft.com/en-us/openspecs/windows_protocols/ms-psrp/
package runspace

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/smnsjas/go-psrpcore/fragments"
	"github.com/smnsjas/go-psrpcore/host"
	"github.com/smnsjas/go-psrpcore/messages"
	"github.com/smnsjas/go-psrpcore/objects"
	"github.com/smnsjas/go-psrpcore/pipeline"
	"github.com/smnsjas/go-psrpcore/serialization"
)

var (
	// ErrInvalidState is returned when an operation is attempted in an invalid state.
	ErrInvalidState = errors.New("invalid runspace pool state")
	// ErrAlreadyOpen is returned when Open is called on an already opening/opened pool.
	ErrAlreadyOpen = errors.New("runspace pool already open")
	// ErrNotOpen is returned when an operation requires an open pool.
	ErrNotOpen = errors.New("runspace pool not open")
	// ErrClosed is returned when an operation is attempted on a closed pool.
	ErrClosed = errors.New("runspace pool is closed")
	// ErrBroken is returned when the pool is in a broken state.
	ErrBroken = errors.New("runspace pool is broken")
	// ErrProtocolViolation is returned when the server violates the PSRP protocol.
	ErrProtocolViolation = errors.New("protocol violation")
)

// Logger is an optional interface for debug logging.
// If not set, no logging is performed.
type Logger interface {
	// Printf formats and logs a debug message.
	Printf(format string, v ...interface{})
}

// MultiplexedTransport represents a transport that can send data on different streams/channels.
// This is used by OutOfProcess transport (SSH) to separate session data from command data.
type MultiplexedTransport interface {
	// SendCommand sends a command creation packet.
	SendCommand(id uuid.UUID) error
	// SendPipelineData sends data associated with a specific pipeline.
	SendPipelineData(id uuid.UUID, data []byte) error
}

// State represents the current state of a RunspacePool.
type State int

const (
	// StateBeforeOpen is the initial state before the pool is opened.
	StateBeforeOpen State = iota
	// StateOpening indicates capability exchange and initialization in progress.
	StateOpening
	// StateConnecting indicates reconnection to an existing runspace pool.
	StateConnecting
	// StateOpened indicates the pool is ready for use.
	StateOpened
	// StateClosing indicates the pool is being closed.
	StateClosing
	// StateClosed indicates the pool is closed.
	StateClosed
	// StateDisconnected indicates the pool is disconnected but technically alive on server (persisted).
	StateDisconnected
	// StateBroken indicates an error occurred and the pool is in a failed state.
	StateBroken
)

const (
	// DefaultMaxFragmentSize is the default maximum size for PSRP message fragments.
	// MS-PSRP recommends 32KB (32768 bytes) as a reasonable default.
	DefaultMaxFragmentSize = 32768

	// DefaultMaxConcurrentHostCalls limits the number of concurrent host callback
	// operations that can be processed. This prevents resource exhaustion under
	// heavy callback load.
	DefaultMaxConcurrentHostCalls = 64
)

// String returns a string representation of the state.
func (s State) String() string {
	switch s {
	case StateBeforeOpen:
		return "BeforeOpen"
	case StateOpening:
		return "Opening"
	case StateConnecting:
		return "Connecting"
	case StateOpened:
		return "Opened"
	case StateClosing:
		return "Closing"
	case StateClosed:
		return "Closed"
	case StateDisconnected:
		return "Disconnected"
	case StateBroken:
		return "Broken"
	default:
		return fmt.Sprintf("Unknown(%d)", s)
	}
}

// Pool represents a PSRP runspace pool.
// It manages the state machine and message exchange with the remote server.
type Pool struct {
	mu sync.RWMutex

	// Core fields
	id        uuid.UUID
	state     State
	transport io.ReadWriter

	// Config
	SkipHandshakeSend bool // If true, Open() skips sending handshake messages (for WSMan creationXml)

	// Configuration
	minRunspaces int
	maxRunspaces int

	// Protocol state
	fragmenter *fragments.Fragmenter
	assembler  *fragments.Assembler

	// Negotiated capabilities
	serverProtocolVersion  string
	serverPSVersion        string
	negotiatedMaxRunspaces int
	negotiatedMinRunspaces int

	// Host callback handling
	host                host.Host
	hostCallbackHandler *host.CallbackHandler

	// Debug logging
	logger     Logger
	slogLogger *slog.Logger

	// Metadata
	metadataCh chan *messages.Message

	// Concurrency control
	hostCallLimiter chan struct{}

	// Pipelines
	pipelines sync.Map

	// Channels for message processing
	stateCh    chan State
	outgoingCh chan *messages.Message
	readyCh    chan struct{}
	doneCh     chan struct{}

	// Lifecycle
	ctx                 context.Context
	cancel              context.CancelFunc
	cleanupOnce         sync.Once
	cleanupError        error // Store the error that caused the breakdown
	dispatchLoopStarted bool  // Track if dispatch loop has been started
}

// NewWithContext creates a new RunspacePool with the given context, transport, and ID.
// The context is used for cancellation propagation - when the parent context is cancelled,
// the pool's internal operations will also be cancelled.
// The pool starts in StateBeforeOpen.
func NewWithContext(ctx context.Context, transport io.ReadWriter, id uuid.UUID) *Pool {
	defaultHost := host.NewNullHost()
	poolCtx, cancel := context.WithCancel(ctx)
	return &Pool{
		id:                  id,
		state:               StateBeforeOpen,
		transport:           transport,
		minRunspaces:        1,
		maxRunspaces:        1,
		host:                defaultHost,
		hostCallbackHandler: host.NewCallbackHandler(defaultHost),
		metadataCh:          make(chan *messages.Message, 1),
		hostCallLimiter:     make(chan struct{}, DefaultMaxConcurrentHostCalls),
		fragmenter:          fragments.NewFragmenter(DefaultMaxFragmentSize),
		assembler:           fragments.NewAssembler(),
		stateCh:             make(chan State, 1),
		outgoingCh:          make(chan *messages.Message, 100),
		readyCh:             make(chan struct{}),
		doneCh:              make(chan struct{}),
		ctx:                 poolCtx,
		cancel:              cancel,
	}
}

// New creates a new RunspacePool with the given transport and ID.
// The pool starts in StateBeforeOpen.
// For better lifecycle control, consider using NewWithContext instead.
func New(transport io.ReadWriter, id uuid.UUID) *Pool {
	return NewWithContext(context.Background(), transport, id)
}

// SetHost sets the host implementation for handling host callbacks.
// This allows the caller to provide a custom Host implementation for interactive sessions.
// Must be called before Open().
func (p *Pool) SetHost(h host.Host) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.state != StateBeforeOpen {
		return ErrInvalidState
	}
	p.host = h
	p.hostCallbackHandler = host.NewCallbackHandler(h)
	return nil
}

// SetLogger sets the logger for debug logging.
// This is optional - if not set, no logging is performed.
// Must be called before Open().
//
// Deprecated: Use SetSlogLogger instead.
func (p *Pool) SetLogger(logger Logger) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.state != StateBeforeOpen {
		return ErrInvalidState
	}
	p.logger = logger
	return nil
}

// SetSlogLogger sets the structured logger for debug logging.
// This uses the log/slog package for structured, context-aware logging.
// Must be called before Open().
func (p *Pool) SetSlogLogger(logger *slog.Logger) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.state != StateBeforeOpen {
		return ErrInvalidState
	}
	p.slogLogger = logger.With("runspace_id", p.id)
	return nil
}

// EnableDebugLogging enables debug logging to stderr using the standard log package.
func (p *Pool) EnableDebugLogging() {
	p.mu.Lock()
	defer p.mu.Unlock()
	// Configure legacy logger
	p.logger = log.New(os.Stderr, "[psrp] ", log.LstdFlags)

	// Determine log level from environment variable
	level := slog.LevelDebug
	if env := os.Getenv("PSRP_LOG_LEVEL"); env != "" {
		switch strings.ToLower(env) {
		case "info":
			level = slog.LevelInfo
		case "warn":
			level = slog.LevelWarn
		case "error":
			level = slog.LevelError
		}
	}

	// Also configure slog to write to stderr in text format for backward compatibility
	p.slogLogger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: level,
	})).With("runspace_id", p.id)
}

// Host returns the host implementation associated with the runspace pool.
func (p *Pool) Host() host.Host {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.host
}

// ID returns the unique identifier of the runspace pool.
func (p *Pool) ID() uuid.UUID {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.id
}

// State returns the current state of the runspace pool.
func (p *Pool) State() State {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.state
}

// SetMessageID sets the current message ID sequence number.
// This is useful when handshake messages were sent via an alternate path
// (e.g., WSMan creationXml sends SESSION_CAPABILITY and INIT_RUNSPACEPOOL).
// The next message sent will use id + 1.
func (p *Pool) SetMessageID(id uint64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.fragmenter.SetObjectID(id)
}

// SetMinRunspaces sets the minimum number of runspaces in the pool.
// Must be called before Open().
func (p *Pool) SetMinRunspaces(minVal int) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.state != StateBeforeOpen {
		return ErrInvalidState
	}
	if minVal < 1 {
		return fmt.Errorf("min runspaces must be >= 1, got %d", minVal)
	}
	p.minRunspaces = minVal
	return nil
}

// SetMaxRunspaces sets the maximum number of runspaces in the pool.
// Must be called before Open().
func (p *Pool) SetMaxRunspaces(maxVal int) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.state != StateBeforeOpen {
		return ErrInvalidState
	}
	if maxVal < 1 {
		return fmt.Errorf("max runspaces must be >= 1, got %d", maxVal)
	}
	p.maxRunspaces = maxVal
	return nil
}

// Open opens the runspace pool by performing the capability exchange and initialization.
//
// The opening sequence:
//  1. Transition to StateOpening
//  2. Send SESSION_CAPABILITY message
//  3. Receive SESSION_CAPABILITY response
//  4. Send INIT_RUNSPACEPOOL message
//  5. Receive RUNSPACEPOOL_STATE message (Opened)
//  6. Transition to StateOpened
func (p *Pool) Open(ctx context.Context) error {
	p.logf("[pool] Open called (state=%s)", p.state)
	p.mu.Lock()
	if p.state != StateBeforeOpen {
		p.mu.Unlock()
		if p.state == StateOpened || p.state == StateOpening {
			return ErrAlreadyOpen
		}
		if p.state == StateClosed {
			return ErrClosed
		}
		if p.state == StateBroken {
			return ErrBroken
		}
		return fmt.Errorf("%w: cannot open from state %s", ErrInvalidState, p.state)
	}

	// Transition to Opening
	p.setState(StateOpening)
	p.mu.Unlock()

	// Send SESSION_CAPABILITY
	if !p.SkipHandshakeSend {
		if err := p.sendSessionCapability(ctx); err != nil {
			p.setBroken()
			return fmt.Errorf("send session capability: %w", err)
		}

		// Receive SESSION_CAPABILITY response
		if err := p.receiveSessionCapability(ctx); err != nil {
			p.setBroken()
			return fmt.Errorf("receive session capability: %w", err)
		}
	}

	// Send INIT_RUNSPACEPOOL
	if !p.SkipHandshakeSend {
		if err := p.sendInitRunspacePool(ctx); err != nil {
			p.setBroken()
			return fmt.Errorf("send init runspace pool: %w", err)
		}

		// Wait for RUNSPACEPOOL_STATE response
		if err := p.waitForOpened(ctx); err != nil {
			p.setBroken()
			return fmt.Errorf("wait for opened: %w", err)
		}
	} else {
		// When using creationXml, the server processes handshake synchronously
		// and doesn't queue responses for Receive operations.
		// Don't start dispatchLoop here - it will be started when CreatePipeline
		// is called and the transport is properly configured.
		p.mu.Lock()
		p.setState(StateOpened)
		p.mu.Unlock()
		return nil
	}

	// Start message dispatch loop (only for normal handshake flow)
	p.mu.Lock()
	p.dispatchLoopStarted = true
	p.mu.Unlock()
	go p.dispatchLoop(p.ctx)

	return nil
}

// Connect connects to an existing runspace pool (Reconnection).
// It assumes the session capability exchange has happened or will happen similarly to Open,
// but sends CONNECT_RUNSPACEPOOL instead of INIT_RUNSPACEPOOL.
func (p *Pool) Connect(ctx context.Context) error {
	p.logf("[pool] Connect called (state=%s)", p.state)
	p.mu.Lock()
	if p.state != StateBeforeOpen {
		p.mu.Unlock()
		return ErrAlreadyOpen
	}
	p.state = StateConnecting
	p.ctx = ctx
	p.mu.Unlock()

	// 1. Start message dispatch loop
	// For reconnection, we typically expect the receive loop to handle incoming messages
	p.StartDispatchLoop()

	// 2. Send SESSION_CAPABILITY
	if !p.SkipHandshakeSend {
		if err := p.sendSessionCapability(ctx); err != nil {
			return fmt.Errorf("send session capability: %w", err)
		}
	}

	// 3. Send CONNECT_RUNSPACEPOOL
	// Payload is typically same as Init, or minimal.
	// For now, we use the same Min/Max runspaces payload.
	if !p.SkipHandshakeSend {
		if err := p.sendConnectRunspacePool(ctx); err != nil {
			return fmt.Errorf("send connect runspace pool: %w", err)
		}

		// 4. Wait for state to be Opened
		// Since dispatchLoop is running, we wait for state transition via channel
		// We add a timeout because some transports (HvSocket) might just hang if session doesn't exist.
		timeout := time.After(30 * time.Second)

		for {
			p.mu.RLock()
			state := p.state
			p.mu.RUnlock()

			if state == StateOpened {
				return nil
			}
			if state == StateBroken || state == StateClosed {
				return ErrBroken
			}

			select {
			case newState := <-p.stateCh:
				if newState == StateOpened {
					return nil
				}
				if newState == StateBroken || newState == StateClosed {
					return ErrBroken
				}
			case <-timeout:
				return fmt.Errorf("connection timed out waiting for RUNSPACEPOOL_STATE (server might not support session persistence)")
			case <-ctx.Done():
				return ctx.Err()
			case <-p.doneCh:
				return ErrClosed
			}
		}
	}

	// For reconnection (SkipHandshakeSend=true), the pool is already opened on the server.
	// We just need to transition our local state and start receiving.
	p.mu.Lock()
	p.setState(StateOpened)
	p.mu.Unlock()
	return nil
}

// StartDispatchLoop starts the message dispatch loop.
// This should be called after configuring the transport when using creationXml flow
// (SkipHandshakeSend = true). For normal handshake flow, the loop is started automatically.
func (p *Pool) StartDispatchLoop() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.dispatchLoopStarted {
		return // Already started
	}

	p.dispatchLoopStarted = true
	go p.dispatchLoop(p.ctx)
}

// ResumeOpened sets the pool state to Opened without sending any PSRP messages.
// This is used when reconnecting to an already-opened RunspacePool.
func (p *Pool) ResumeOpened() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.state = StateOpened
}

// AdoptPipeline registers an existing pipeline with the runspace pool.
// This is used for recovering disconnected pipelines where the ID is known.
// The pipeline must have been created with NewWithID.
func (p *Pool) AdoptPipeline(pl *pipeline.Pipeline) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.state != StateOpened && p.state != StateConnecting {
		return ErrNotOpen
	}

	if _, exists := p.pipelines.Load(pl.ID()); exists {
		return fmt.Errorf("pipeline %s already exists", pl.ID())
	}

	p.pipelines.Store(pl.ID(), pl)
	return nil
}

// GetActivePipelineIDs returns the list of IDs for all currently tracked pipelines.
func (p *Pool) GetActivePipelineIDs() []uuid.UUID {
	var ids []uuid.UUID
	p.pipelines.Range(func(key, value interface{}) bool {
		if id, ok := key.(uuid.UUID); ok {
			ids = append(ids, id)
		}
		return true
	})
	return ids
}

// ProcessConnectResponse processes the PSRP response data from WSManConnectShellEx.
// This data contains the server's response to our SESSION_CAPABILITY and CONNECT_RUNSPACEPOOL
// messages that were piggybacked in the Connect request.
func (p *Pool) ProcessConnectResponse(data []byte) error {
	if len(data) == 0 {
		return nil
	}

	p.logf("[connect] Processing %d bytes of connect response data", len(data))

	// The response data contains PSRP fragments. We need to decode and process them.
	// This typically includes:
	// 1. SESSION_CAPABILITY response
	// 2. CONNECT_RUNSPACEPOOL response (or RUNSPACEPOOL_STATE indicating Opened)

	// Feed the data through the fragment assembler
	offset := 0
	for offset < len(data) {
		// Check if we have enough data for a fragment header
		if offset+fragments.HeaderSize > len(data) {
			break
		}

		// Get blob length from header
		blobLen := binary.BigEndian.Uint32(data[offset+17 : offset+21])
		fragLen := fragments.HeaderSize + int(blobLen)

		if offset+fragLen > len(data) {
			return fmt.Errorf("incomplete fragment at offset %d", offset)
		}

		// Decode fragment
		frag, err := fragments.Decode(data[offset : offset+fragLen])
		if err != nil {
			return fmt.Errorf("decode fragment at offset %d: %w", offset, err)
		}

		// Assemble message
		complete, msgData, err := p.assembler.Add(frag)
		if err != nil {
			return fmt.Errorf("assemble fragment: %w", err)
		}

		if complete {
			// Decode and process message
			msg, err := messages.Decode(msgData)
			if err != nil {
				p.logWarn("[connect] Warning: failed to decode message: %v", err)
			} else {
				p.logf("[connect] Received message type=0x%08X", uint32(msg.Type))
				// We can optionally process these messages, but for connect
				// we mainly care that they don't error - the state is set
				// to Opened by ResumeOpened() regardless.
			}
		}

		offset += fragLen
	}

	return nil
}

// Disconnect disconnects the transport without closing the runspace pool on the server.
// This allows the session to remain alive for later reconnection.
func (p *Pool) Disconnect() error {
	p.mu.Lock()
	if p.state != StateOpened {
		p.mu.Unlock()
		return fmt.Errorf("cannot disconnect from state %s", p.state)
	}
	// Transition to Disconnected
	p.setState(StateDisconnected)
	p.mu.Unlock()

	// Close the transport explicitly. This stops the dispatch loop (read error).
	// We do NOT send a CLOSE message.
	if closer, ok := p.transport.(io.Closer); ok {
		return closer.Close()
	}
	return nil
}

// Close closes the runspace pool.
// Transitions from Opened → Closing → (wait) → Closed.
func (p *Pool) Close(ctx context.Context) error {
	p.logf("[pool] Close called")
	p.mu.Lock()
	currentState := p.state

	// Already closed or closing
	if currentState == StateClosed || currentState == StateClosing {
		p.mu.Unlock()
		return nil
	}

	// Can't close if not opened
	if currentState != StateOpened {
		p.mu.Unlock()
		return fmt.Errorf("%w: cannot close from state %s", ErrInvalidState, currentState)
	}

	// Transition to Closing
	p.setState(StateClosing)
	p.mu.Unlock()

	// Send RUNSPACEPOOL_STATE (Closed) message
	// MS-PSRP 3.1.5.3.14: Client MUST send RUNSPACEPOOL_STATE with State=Closed(4)
	closeData := []byte(fmt.Sprintf(
		`<Objs Version="1.1.0.1" xmlns="http://schemas.microsoft.com/powershell/2004/04"><I32>%d</I32></Objs>`,
		messages.RunspacePoolStateClosed))

	// We ignore errors here because we are closing anyway, and the underlying transport
	// might already be closed or broken.
	// NewRunspacePoolStateMessage helper creates the message.
	// Note: We ignore the second argument of NewRunspacePoolStateMessage as it's not used in header.
	closeMsg := messages.NewRunspacePoolStateMessage(p.id, messages.RunspacePoolStateClosed, closeData)
	_ = p.sendMessage(ctx, closeMsg)

	// Wait for server to acknowledge and close (via dispatchLoop -> cleanup)
	// Or timeout if server is unresponsive
	// We use a very short timeout (100ms) because for OutOfProcess transports,
	// the server often doesn't ack cleanly, and we don't want to make the user wait.
	// The backend.Close() will handle the actual resource cleanup (socket close/process kill).
	p.logf("[pool] Waiting for server Clean close (timeout 100ms)...")
	select {
	case <-p.doneCh:
		// Clean exit triggered by server response
		p.logInfo("[pool] Server acknowledged close (Clean)")
	case <-time.After(100 * time.Millisecond):
		// Timeout - force clean up locally
		p.logWarn("[pool] Timeout waiting for server close")
		p.cleanup(StateClosed, nil)
	case <-ctx.Done():
		// Context cancelled - force cleanup might be needed or just exit
		p.logf("[pool] Context cancelled during close")
		p.cleanup(StateClosed, ctx.Err())
	}

	// cleanup() handles state transition and doneCh closing.
	// We also need to cancel the context to stop any background workers.
	p.mu.Lock()
	p.cancel()
	p.mu.Unlock()

	return nil
}

// setState transitions to a new state (caller must hold lock).
func (p *Pool) setState(newState State) {
	oldState := p.state
	p.state = newState

	// Non-blocking notification
	select {
	case p.stateCh <- newState:
	default:
	}

	_ = oldState // For future logging/debugging
}

// setBroken transitions the pool to the Broken state.
// Can be called from any state. This is used during the Open sequence
// when an error occurs before dispatchLoop is running.
func (p *Pool) setBroken() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.setState(StateBroken)
	// Note: doneCh is not closed here because it's only closed by handleTransportError
	// when dispatchLoop is running. During Open(), the caller handles cleanup.
}

// sendSessionCapability sends the SESSION_CAPABILITY message to the server.
func (p *Pool) sendSessionCapability(ctx context.Context) error {
	msg := p.createSessionCapabilityMessage()
	return p.sendMessage(ctx, msg)
}

func (p *Pool) createSessionCapabilityMessage() *messages.Message {
	// SESSION_CAPABILITY uses raw <Obj> without <Objs> wrapper per MS-PSRP
	// XML must be compact (no whitespace) for OutOfProcess transport
	capabilityData := []byte(`<Obj RefId="0"><MS><Version N="protocolversion">2.3</Version>` +
		`<Version N="PSVersion">2.0</Version><Version N="SerializationVersion">1.1.0.1</Version></MS></Obj>`)

	return messages.NewSessionCapability(p.id, capabilityData)
}

// receiveSessionCapability receives and validates the server's SESSION_CAPABILITY response.
func (p *Pool) receiveSessionCapability(ctx context.Context) error {
	msg, err := p.receiveMessageWithHostCallbacks(ctx)
	if err != nil {
		return err
	}

	if msg.Type != messages.MessageTypeSessionCapability {
		return fmt.Errorf("%w: expected SESSION_CAPABILITY, got %v", ErrProtocolViolation, msg.Type)
	}

	// Parse the capability data
	caps, err := parseCapabilityData(msg.Data)
	if err != nil {
		return fmt.Errorf("parse capability data: %w", err)
	}

	// Validate protocol version
	if caps.ProtocolVersion == "" {
		return fmt.Errorf("%w: server did not provide protocol version", ErrProtocolViolation)
	}

	// Check if protocol version is compatible (we support 2.x)
	if len(caps.ProtocolVersion) < 2 || caps.ProtocolVersion[0] != '2' {
		return fmt.Errorf(
			"%w: incompatible protocol version: server=%s, client=2.3", ErrProtocolViolation, caps.ProtocolVersion)
	}

	// Store negotiated capabilities
	p.mu.Lock()
	p.serverProtocolVersion = caps.ProtocolVersion
	p.serverPSVersion = caps.PSVersion
	p.mu.Unlock()

	return nil
}

// sendConnectRunspacePool sends the CONNECT_RUNSPACEPOOL message (blocking).
func (p *Pool) sendConnectRunspacePool(ctx context.Context) error {
	p.mu.RLock()
	minRunspaces := p.minRunspaces
	maxRunspaces := p.maxRunspaces
	p.mu.RUnlock()

	// Reuse the same payload structure as INIT_RUNSPACEPOOL
	// MS-PSRP 3.1.5.3.3 says we should validate RunspacePoolID.
	// We send the same Min/Max params.
	xmlData := p.createRunspacePoolInitXML(minRunspaces, maxRunspaces)

	// Create CONNECT_RUNSPACEPOOL message using the builder from messages package
	// Note: We need to pass the payload as []byte
	msg := messages.NewConnectRunspacePool(p.id, []byte(xmlData))

	// We rely on sendMessage to handle ID generation
	return p.sendMessage(ctx, msg)
}

// sendInitRunspacePool sends the INIT_RUNSPACEPOOL message to the server.
func (p *Pool) sendInitRunspacePool(ctx context.Context) error {
	p.mu.RLock()
	minRunspaces := p.minRunspaces
	maxRunspaces := p.maxRunspaces
	p.mu.RUnlock()

	msg := p.createInitRunspacePoolMessage(minRunspaces, maxRunspaces)
	return p.sendMessage(ctx, msg)
}

func (p *Pool) createInitRunspacePoolMessage(minRunspaces, maxRunspaces int) *messages.Message {
	initData := p.createRunspacePoolInitXML(minRunspaces, maxRunspaces)
	return messages.NewInitRunspacePool(p.id, []byte(initData))
}

func (p *Pool) createRunspacePoolInitXML(minRunspaces, maxRunspaces int) string {
	// Build INIT_RUNSPACEPOOL XML manually to match exact PowerShell format
	// Per MS-PSRP, this uses <MS> (MemberSet) format, not <Props>
	// ApplicationArguments is required per server error "Remoting data is missing ApplicationArguments property"
	return fmt.Sprintf(`<Obj RefId="0"><MS>`+
		`<I32 N="MinRunspaces">%d</I32>`+
		`<I32 N="MaxRunspaces">%d</I32>`+
		`<Obj N="PSThreadOptions" RefId="1">`+
		`<TN RefId="0"><T>System.Management.Automation.Runspaces.PSThreadOptions</T>`+
		`<T>System.Enum</T><T>System.ValueType</T><T>System.Object</T></TN>`+
		`<ToString>Default</ToString><I32>0</I32></Obj>`+
		`<Obj N="ApartmentState" RefId="2">`+
		`<TN RefId="1"><T>System.Threading.ApartmentState</T>`+
		`<T>System.Enum</T><T>System.ValueType</T><T>System.Object</T></TN>`+
		`<ToString>Unknown</ToString><I32>2</I32></Obj>`+
		`<Obj N="HostInfo" RefId="3"><MS>`+
		`<B N="_isHostNull">true</B>`+
		`<B N="_isHostUINull">true</B>`+
		`<B N="_isHostRawUINull">true</B>`+
		`<B N="_useRunspaceHost">true</B>`+
		`</MS></Obj>`+
		`<Nil N="ApplicationArguments"/>`+
		`<Obj N="ApplicationPrivateData" RefId="4"><MS>`+
		`<B N="PSVersionTable">true</B>`+
		`</MS></Obj>`+
		`</MS></Obj>`, minRunspaces, maxRunspaces)
}

// GetCommandMetadata queries the server for information about available commands.
// names allows filtering by wildcards (e.g., "*Process", "Get-*"). If empty, returns all commands.
func (p *Pool) GetCommandMetadata(ctx context.Context, names []string) ([]*objects.CommandMetadata, error) {
	if p.state != StateOpened {
		return nil, ErrNotOpen
	}

	// Default to * if no names provided
	if len(names) == 0 {
		names = []string{"*"}
	}

	// Serialize the request (list of strings)
	serializer := serialization.NewSerializer()
	data, err := serializer.SerializeRaw(names)
	if err != nil {
		return nil, fmt.Errorf("serialize metadata request: %w", err)
	}

	// Send GET_COMMAND_METADATA message
	msg := messages.NewGetCommandMetadata(p.id, data)
	if err := p.sendMessage(ctx, msg); err != nil {
		return nil, fmt.Errorf("send metadata request: %w", err)
	}

	// Wait for reply
	select {
	case reply := <-p.metadataCh:
		return parseCommandMetadata(reply.Data)
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-p.doneCh:
		return nil, ErrClosed
	}
}

// parseCommandMetadata parses the CLIXML command metadata from a GET_COMMAND_METADATA_REPLY message.
func parseCommandMetadata(data []byte) ([]*objects.CommandMetadata, error) {
	deser := serialization.NewDeserializer()
	objs, err := deser.Deserialize(data)
	if err != nil {
		return nil, fmt.Errorf("deserialize metadata: %w", err)
	}

	var results []*objects.CommandMetadata

	for _, obj := range objs {
		// Expecting PSObjects representing CommandMetadata
		psObj, ok := obj.(*serialization.PSObject)
		if !ok {
			// Might be a primitive if the server returns weird data, but usually objects
			continue
		}

		meta := &objects.CommandMetadata{}
		if name, ok := psObj.Properties["Name"].(string); ok {
			meta.Name = name
		}

		// CommandType is often int or enum
		if ct, ok := psObj.Properties["CommandType"].(int32); ok {
			meta.CommandType = int(ct)
		} else if ct, ok := psObj.Properties["CommandType"].(int); ok {
			meta.CommandType = ct
		}

		// Parameters (Dictionary)
		if params, ok := psObj.Properties["Parameters"].(map[string]interface{}); ok {
			meta.Parameters = make(map[string]objects.ParameterMetadata)
			for pname, pval := range params {
				// Each value in the dictionary is usually a ParameterMetadata object
				pm := objects.ParameterMetadata{Name: pname}
				if innerPSObj, ok := pval.(*serialization.PSObject); ok {
					if t, ok := innerPSObj.Properties["ParameterType"].(string); ok {
						pm.Type = t
					}
				}
				meta.Parameters[pname] = pm
			}
		}

		results = append(results, meta)
	}

	return results, nil
}

// waitForOpened waits for the RUNSPACEPOOL_STATE message indicating the pool is opened.
// The server may send RUNSPACEPOOL_INIT_DATA (ApplicationPrivate) before RUNSPACEPOOL_STATE.
func (p *Pool) waitForOpened(ctx context.Context) error {
	for {
		msg, err := p.receiveMessageWithHostCallbacks(ctx)
		if err != nil {
			return err
		}

		switch msg.Type {
		case messages.MessageTypeApplicationPrivate, messages.MessageTypeRunspacePoolInitData:
			// Server sent initialization data - store or log it, then continue waiting
			// This contains application private data from the server
			p.logf("Received RUNSPACEPOOL_INIT_DATA")
			continue

		case messages.MessageTypeRunspacePoolState:
			// Parse the state message
			stateInfo, err := parseRunspacePoolState(msg.Data)
			if err != nil {
				return fmt.Errorf("parse runspace pool state: %w", err)
			}

			// Verify the state is Opened
			if stateInfo.State != messages.RunspacePoolStateOpened {
				return fmt.Errorf("%w: expected state Opened, got %d", ErrInvalidState, stateInfo.State)
			}

			// Store negotiated runspace counts if provided
			p.mu.Lock()
			if stateInfo.MinRunspaces > 0 {
				p.negotiatedMinRunspaces = stateInfo.MinRunspaces
			} else {
				p.negotiatedMinRunspaces = p.minRunspaces
			}
			if stateInfo.MaxRunspaces > 0 {
				p.negotiatedMaxRunspaces = stateInfo.MaxRunspaces
			} else {
				p.negotiatedMaxRunspaces = p.maxRunspaces
			}
			p.setState(StateOpened)
			p.mu.Unlock()

			return nil

		default:
			return fmt.Errorf("%w: expected RUNSPACEPOOL_STATE, got %v", ErrProtocolViolation, msg.Type)
		}
	}
}

// SendMessage sends a PSRP message to the server (implements pipeline.Transport).
func (p *Pool) SendMessage(ctx context.Context, msg *messages.Message) error {
	return p.sendMessage(ctx, msg)
}

// CreatePipeline creates a new pipeline in the runspace pool.
// Note: For creationXml flow, call StartDispatchLoop() after configuring the transport.
func (p *Pool) CreatePipeline(command string) (*pipeline.Pipeline, error) {
	p.mu.RLock()
	if p.state != StateOpened {
		p.mu.RUnlock()
		return nil, ErrNotOpen
	}

	p.mu.RUnlock()

	pl := pipeline.New(p, p.id, command)
	p.pipelines.Store(pl.ID(), pl)

	// If the transport supports MultiplexedTransport (e.g. OutOfProcess),
	// we must send a Command creation packet before any pipeline data.
	if mux, ok := p.transport.(MultiplexedTransport); ok {
		if err := mux.SendCommand(pl.ID()); err != nil {
			p.pipelines.Delete(pl.ID())
			return nil, fmt.Errorf("send command creation: %w", err)
		}
	}

	// Start a monitoring goroutine to remove the pipeline when it's done
	// Also listens to pool's done channel to avoid goroutine leak if pool is closed
	go func() {
		defer p.pipelines.Delete(pl.ID())
		select {
		case <-pl.Done():
		case <-p.doneCh:
			// If pool is closed, cancel the pipeline
			pl.Cancel()
		case <-p.ctx.Done(): // Also respect the pool's context cancellation
			pl.Cancel()
		}
	}()

	return pl, nil
}

// CreatePipelineBuilder creates a new pipeline builder in the runspace pool.
// This allows constructing a pipeline with multiple commands or specific options (e.g. IsScript=false).
func (p *Pool) CreatePipelineBuilder() (*pipeline.Pipeline, error) {
	p.mu.Lock()

	if p.state != StateOpened {
		p.mu.Unlock()
		return nil, ErrNotOpen
	}

	p.mu.Unlock()

	pl := pipeline.NewBuilder(p, p.id)
	p.pipelines.Store(pl.ID(), pl)

	// If the transport supports MultiplexedTransport (e.g. OutOfProcess),
	// we must send a Command creation packet before any pipeline data.
	if mux, ok := p.transport.(MultiplexedTransport); ok {
		if err := mux.SendCommand(pl.ID()); err != nil {
			p.pipelines.Delete(pl.ID())
			return nil, fmt.Errorf("send command creation: %w", err)
		}
	}

	// Start a monitoring goroutine to remove the pipeline when it's done
	// Also listens to pool's done channel to avoid goroutine leak if pool is closed
	go func() {
		defer p.pipelines.Delete(pl.ID())
		select {
		case <-pl.Done():
		case <-p.doneCh:
			// If pool is closed, cancel the pipeline
			pl.Cancel()
		case <-p.ctx.Done(): // Also respect the pool's context cancellation
			pl.Cancel()
		}
	}()

	return pl, nil
}

// dispatchLoop processes incoming messages and routes them to pipelines.
func (p *Pool) dispatchLoop(ctx context.Context) {
	for {
		// Check if closed
		p.mu.RLock()
		state := p.state
		p.mu.RUnlock()
		if state == StateClosed || state == StateBroken {
			return
		}

		msg, err := p.receiveMessage(ctx)
		if err != nil {
			// Check for context cancellation (normal shutdown)
			if ctx.Err() != nil {
				p.logf("[dispatch] exiting: context cancelled")
				return
			}

			// Check for transient errors that shouldn't break the pool
			// EOF can happen when there's no more data but pool is still valid
			errStr := err.Error()
			if strings.Contains(errStr, "EOF") ||
				strings.Contains(errStr, "timeout") ||
				strings.Contains(errStr, "context canceled") {
				p.logWarn("[dispatch] transient error: %v", err)
				// Check if we should continue or exit
				p.mu.RLock()
				// If we are closed, broken, OR disconnected, we stop.
				// Disconnected state implies we expect the transport to die.
				shouldExit := p.state == StateClosed || p.state == StateBroken || p.state == StateDisconnected
				p.mu.RUnlock()
				if shouldExit {
					p.logf("[dispatch] exiting: pool state suggests stop (state=%v)", p.state)
					return
				}
				// Transient error, continue polling?
				// If "context canceled" loop continues forever if context is not p.ctx?
				// p.ctx drives the loop? No, p.ctx is cancelling on Close.
				// But Disconnect() does NOT cancel p.ctx.
				// So we rely on "state" check.

				// However, "read fragment header: context canceled" likely comes from p.ctx cancellation?
				// If p.ctx is canceled, we should return.
				if ctx.Err() != nil {
					return
				}

				// If error is "closed networking", preventing loop.
				if strings.Contains(err.Error(), "closed network connection") {
					return
				}

				continue
			}

			// Fatal transport error - transition to broken state
			p.logError("[dispatch] fatal error: %v", err)
			p.handleTransportError(err)
			return
		}

		p.logf("[dispatch] received message type=0x%08X pipeline=%s", uint32(msg.Type), msg.PipelineID)

		// Dispatch based on PipelineID
		if msg.PipelineID != uuid.Nil {
			val, exists := p.pipelines.Load(msg.PipelineID)

			if exists {
				pl := val.(*pipeline.Pipeline)
				p.logf("[dispatch] routing to pipeline %s", msg.PipelineID)
				// HandleMessage can block with timeout if pipeline's channel buffers are full.
				// This provides back-pressure to prevent unbounded memory growth.
				if err := pl.HandleMessage(msg); err != nil {
					// If HandleMessage fails (buffer timeout or context cancelled),
					// mark the pipeline as failed to signal the error to the consumer.
					p.logError("[dispatch] HandleMessage failed: %v", err)
					pl.Fail(err)
				}
			} else {
				p.logf("[dispatch] pipeline %s not found!", msg.PipelineID)
			}
			continue
		}

		// Runspace-level messages
		switch msg.Type {
		case messages.MessageTypeRunspaceHostCall:
			// Dispatch host call in background to not block loop
			// Use limiter to prevent unbounded goroutine growth
			select {
			case p.hostCallLimiter <- struct{}{}:
				go func() {
					defer func() { <-p.hostCallLimiter }()
					if err := p.handleHostCall(ctx, msg); err != nil {
						// Handling host call failed (likely transport error), pool is likely broken
						p.handleTransportError(fmt.Errorf("host call failed: %w", err))
					}
				}()
			case <-ctx.Done():
				return
			}

		case messages.MessageTypeRunspacePoolState:
			// Handle state changes if any (e.g. Broken/Closed from server)
			stateInfo, err := parseRunspacePoolState(msg.Data)
			if err != nil {
				// Failed to parse state... ignore or log?
				continue
			}

			switch stateInfo.State {
			case messages.RunspacePoolStateClosed:
				// Clean shutdown confirmed by server
				p.cleanup(StateClosed, nil)
				return // Exit loop

			case messages.RunspacePoolStateBroken:
				// Server reported broken state
				p.cleanup(StateBroken, fmt.Errorf("server reported broken state"))
				return // Exit loop

			case messages.RunspacePoolStateOpened:
				p.mu.Lock()
				p.setState(StateOpened)
				p.mu.Unlock()
			}
		}
	}
}

// handleTransportError handles a transport error by transitioning to broken state
// and cleaning up all resources. It uses sync.Once to ensure cleanup only happens once.
// handleTransportError handles a transport error by transitioning to broken state
// and cleaning up all resources.
func (p *Pool) handleTransportError(err error) {
	p.cleanup(StateBroken, err)
}

// cleanup handles resource cleanup and state transition.
// It uses cleanupOnce to ensure it only runs once (whether for error or clean close).
func (p *Pool) cleanup(endState State, err error) {
	p.cleanupOnce.Do(func() {
		p.mu.Lock()
		defer p.mu.Unlock()

		// Store the error if any
		if err != nil {
			p.cleanupError = err
		}

		// Transition to end state
		p.setState(endState)

		// Close all pipeline channels by transitioning them to failed state
		// (For clean close, they should already be done, but safety check)
		p.pipelines.Range(func(_, value interface{}) bool {
			pl := value.(*pipeline.Pipeline)
			if err != nil {
				pl.Fail(fmt.Errorf("runspace pool broken: %w", err))
			} else {
				pl.Cancel()
			}
			return true
		})

		// Notify any waiting goroutines
		close(p.doneCh)
	})
}

// sendMessage sends a PSRP message through the transport.
func (p *Pool) sendMessage(ctx context.Context, msg *messages.Message) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	frags, err := p.prepareMessage(msg)
	if err != nil {
		return err
	}

	// Send each fragment
	for _, frag := range frags {
		fragData, err := frag.Encode()
		if err != nil {
			return fmt.Errorf("encode fragment: %w", err)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// If the transport supports MultiplexedTransport (e.g. OutOfProcess),
		// use SendPipelineData for pipeline messages to route them to correct channel.
		if msg.PipelineID != uuid.Nil {
			if mux, ok := p.transport.(MultiplexedTransport); ok {
				if err := mux.SendPipelineData(msg.PipelineID, fragData); err != nil {
					return fmt.Errorf("write fragment (mux): %w", err)
				}
				continue
			}
		}

		// Fallback to standard Write (Session Data)
		if _, err := p.transport.Write(fragData); err != nil {
			return fmt.Errorf("write fragment: %w", err)
		}
	}

	return nil
}

// prepareMessage encodes and fragments a message.
func (p *Pool) prepareMessage(msg *messages.Message) ([]*fragments.Fragment, error) {
	// Encode message
	encoded, err := msg.Encode()
	if err != nil {
		return nil, fmt.Errorf("encode message: %w", err)
	}

	// Fragment message
	frags, err := p.fragmenter.Fragment(encoded)
	if err != nil {
		return nil, fmt.Errorf("fragment message: %w", err)
	}

	return frags, nil
}

// GetHandshakeFragments generates the base64-encoded PSRP fragments matching
// the session initialization messages (SESSION_CAPABILITY and INIT_RUNSPACEPOOL).
// This is used for creating the creationXml for the WSMan shell creation.
func (p *Pool) GetHandshakeFragments() ([]byte, error) {
	p.mu.RLock()
	minRunspaces := p.minRunspaces
	maxRunspaces := p.maxRunspaces
	p.mu.RUnlock()

	// 1. Session Capability
	capMsg := p.createSessionCapabilityMessage()
	capFrags, err := p.prepareMessage(capMsg)
	if err != nil {
		return nil, fmt.Errorf("prepare capability fragments: %w", err)
	}

	// 2. Init RunspacePool
	initMsg := p.createInitRunspacePoolMessage(minRunspaces, maxRunspaces)
	initFrags, err := p.prepareMessage(initMsg)
	if err != nil {
		return nil, fmt.Errorf("prepare init fragments: %w", err)
	}

	// Flatten all fragments into a single byte slice by encoding each
	var result []byte
	for _, f := range capFrags {
		encoded, err := f.Encode()
		if err != nil {
			return nil, fmt.Errorf("encode capability fragment: %w", err)
		}
		result = append(result, encoded...)
	}
	for _, f := range initFrags {
		encoded, err := f.Encode()
		if err != nil {
			return nil, fmt.Errorf("encode init fragment: %w", err)
		}
		result = append(result, encoded...)
	}

	return result, nil
}

// GetConnectHandshakeFragments generates the PSRP fragments for connecting to
// an existing disconnected RunspacePool (SESSION_CAPABILITY + CONNECT_RUNSPACEPOOL).
// This is used for the connectXml in WSManConnectShellEx when a NEW client
// connects to a session that was disconnected by a different client.
func (p *Pool) GetConnectHandshakeFragments() ([]byte, error) {
	// 1. Session Capability
	capMsg := p.createSessionCapabilityMessage()
	capFrags, err := p.prepareMessage(capMsg)
	if err != nil {
		return nil, fmt.Errorf("prepare capability fragments: %w", err)
	}

	// 2. Connect RunspacePool - EMPTY per pypsrp implementation
	// Unlike INIT_RUNSPACEPOOL, CONNECT_RUNSPACEPOOL uses an empty string payload <S></S>.
	// Reference: pypsrp/messages.py message_type == CONNECT_RUNSPACEPOOL check.
	connectParams := []byte(`<S></S>`)
	connectMsg := messages.NewConnectRunspacePool(p.id, connectParams)
	connectFrags, err := p.prepareMessage(connectMsg)
	if err != nil {
		return nil, fmt.Errorf("prepare connect fragments: %w", err)
	}

	// Flatten all fragments into a single byte slice
	var result []byte
	for _, f := range capFrags {
		encoded, err := f.Encode()
		if err != nil {
			return nil, fmt.Errorf("encode capability fragment: %w", err)
		}
		result = append(result, encoded...)
	}
	for _, f := range connectFrags {
		encoded, err := f.Encode()
		if err != nil {
			return nil, fmt.Errorf("encode connect fragment: %w", err)
		}
		result = append(result, encoded...)
	}

	return result, nil
}

// receiveMessage receives and reassembles a PSRP message from the transport.
func (p *Pool) receiveMessage(ctx context.Context) (*messages.Message, error) {
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		// Read fragment header to determine size
		header := make([]byte, fragments.HeaderSize)
		if _, err := io.ReadFull(p.transport, header); err != nil {
			return nil, fmt.Errorf("read fragment header: %w", err)
		}

		// Read blob data
		// The blob length is at offset 17 in the header (big-endian)
		blobLen := binary.BigEndian.Uint32(header[17:21])
		fragData := make([]byte, fragments.HeaderSize+int(blobLen))
		copy(fragData[:fragments.HeaderSize], header)

		if blobLen > 0 {
			if _, err := io.ReadFull(p.transport, fragData[fragments.HeaderSize:]); err != nil {
				return nil, fmt.Errorf("read fragment data: %w", err)
			}
		}

		// Decode fragment
		frag, err := fragments.Decode(fragData)
		if err != nil {
			return nil, fmt.Errorf("decode fragment: %w", err)
		}

		// Assemble message
		complete, msgData, err := p.assembler.Add(frag)
		if err != nil {
			return nil, fmt.Errorf("assemble fragment: %w", err)
		}

		if complete {
			// Decode message
			msg, err := messages.Decode(msgData)
			if err != nil {
				return nil, fmt.Errorf("decode message: %w", err)
			}
			return msg, nil
		}

		// Need more fragments, continue loop
	}
}

// handleHostCall processes a RUNSPACEPOOL_HOST_CALL message and sends a response.
// This method is called when the server needs to interact with the client's host
// (e.g., prompts, credential requests, progress reporting).
func (p *Pool) handleHostCall(ctx context.Context, msg *messages.Message) error {
	// Decode the RemoteHostCall from the message data
	call, err := host.DecodeRemoteHostCall(msg.Data)
	if err != nil {
		return fmt.Errorf("decode host call: %w", err)
	}

	// Execute the host callback
	response := p.hostCallbackHandler.HandleCall(call)

	// Encode the response
	responseData, err := host.EncodeRemoteHostResponse(response)
	if err != nil {
		return fmt.Errorf("encode host response: %w", err)
	}

	// Send RUNSPACEPOOL_HOST_RESPONSE message
	responseMsg := messages.NewRunspaceHostResponse(p.id, responseData)
	if err := p.sendMessage(ctx, responseMsg); err != nil {
		return fmt.Errorf("send host response: %w", err)
	}

	return nil
}

// ReceiveMessageWithHostCallbacks receives a message and automatically handles host callbacks.
// If a RUNSPACEPOOL_HOST_CALL message is received, it processes the callback and continues
// receiving until a non-host-call message is received.
//
// This is useful during Open() and other operations where host callbacks may occur.
func (p *Pool) receiveMessageWithHostCallbacks(ctx context.Context) (*messages.Message, error) {
	for {
		msg, err := p.receiveMessage(ctx)
		if err != nil {
			return nil, err
		}

		// Check if this is a host callback request
		if msg.Type == messages.MessageTypeRunspaceHostCall {
			// Handle the callback and continue receiving
			if err := p.handleHostCall(ctx, msg); err != nil {
				return nil, fmt.Errorf("handle host call: %w", err)
			}
			// Continue loop to receive next message
			continue
		}

		// Not a host call, return the message
		return msg, nil
	}
}

// capabilityData represents parsed SESSION_CAPABILITY data.
type capabilityData struct {
	ProtocolVersion      string
	PSVersion            string
	SerializationVersion string
}

// parseCapabilityData parses the CLIXML capability data from a SESSION_CAPABILITY message.
func parseCapabilityData(data []byte) (*capabilityData, error) {
	deser := serialization.NewDeserializer()
	objs, err := deser.Deserialize(data)
	if err != nil {
		return nil, fmt.Errorf("deserialize capability data: %w", err)
	}

	if len(objs) == 0 {
		return nil, fmt.Errorf("no capability object in message")
	}

	// The capability should be a PSObject with properties
	psObj, ok := objs[0].(*serialization.PSObject)
	if !ok {
		return nil, fmt.Errorf("capability is not a PSObject, got %T", objs[0])
	}

	caps := &capabilityData{}

	// Extract protocol version
	if pv, ok := psObj.Properties["protocolversion"].(string); ok {
		caps.ProtocolVersion = pv
	}

	// Extract PS version
	if psv, ok := psObj.Properties["PSVersion"].(string); ok {
		caps.PSVersion = psv
	}

	// Extract serialization version
	if sv, ok := psObj.Properties["SerializationVersion"].(string); ok {
		caps.SerializationVersion = sv
	}

	return caps, nil
}

// runspacePoolStateInfo represents parsed RUNSPACEPOOL_STATE data.
type runspacePoolStateInfo struct {
	State        messages.RunspacePoolState
	MinRunspaces int
	MaxRunspaces int
}

// parseRunspacePoolState parses the CLIXML state data from a RUNSPACEPOOL_STATE message.
func parseRunspacePoolState(data []byte) (*runspacePoolStateInfo, error) {
	deser := serialization.NewDeserializer()
	objs, err := deser.Deserialize(data)
	if err != nil {
		return nil, fmt.Errorf("deserialize state data: %w", err)
	}

	if len(objs) == 0 {
		return nil, fmt.Errorf("no state object in message")
	}

	info := &runspacePoolStateInfo{}

	// The state can be either a raw Int32 or a PSObject with RunspaceState property
	switch v := objs[0].(type) {
	case int32:
		info.State = messages.RunspacePoolState(v)
	case *serialization.PSObject:
		// Look for RunspaceState or RunspacePoolState property
		if state, ok := v.Properties["RunspaceState"].(int32); ok {
			info.State = messages.RunspacePoolState(state)
		} else if state, ok := v.Properties["RunspacePoolState"].(int32); ok {
			info.State = messages.RunspacePoolState(state)
		} else {
			// Try to find any int32 property that might be the state
			for key, val := range v.Properties {
				if state, ok := val.(int32); ok {
					// Likely the state value
					info.State = messages.RunspacePoolState(state)
					_ = key // suppress unused warning
					break
				}
			}
		}
		// Also check for min/max runspaces in the same object
		if minRS, ok := v.Properties["MinRunspaces"].(int32); ok {
			info.MinRunspaces = int(minRS)
		}
		if maxRS, ok := v.Properties["MaxRunspaces"].(int32); ok {
			info.MaxRunspaces = int(maxRS)
		}
	default:
		return nil, fmt.Errorf("state is not int32 or PSObject, got %T", objs[0])
	}

	// Additional objects may contain min/max runspaces
	if len(objs) > 1 {
		if psObj, ok := objs[1].(*serialization.PSObject); ok {
			if minRS, ok := psObj.Properties["MinRunspaces"].(int32); ok {
				info.MinRunspaces = int(minRS)
			}
			if maxRS, ok := psObj.Properties["MaxRunspaces"].(int32); ok {
				info.MaxRunspaces = int(maxRS)
			}
		}
	}

	return info, nil
}

// logf logs a debug message if a logger is configured.
//
//nolint:unparam // v is intentionally variadic for future use
func (p *Pool) logf(format string, v ...interface{}) {
	p.mu.RLock()
	logger := p.logger
	slogger := p.slogLogger
	p.mu.RUnlock()

	if slogger != nil {
		slogger.Debug(fmt.Sprintf(format, v...))
	} else if logger != nil {
		logger.Printf(format, v...)
	}
}

// logInfo logs an informational message (normal operations).
func (p *Pool) logInfo(format string, v ...interface{}) {
	p.mu.RLock()
	slogger := p.slogLogger
	p.mu.RUnlock()

	if slogger != nil {
		slogger.Info(fmt.Sprintf(format, v...))
	}
}

// logWarn logs a warning message (potential issues, recoverable).
func (p *Pool) logWarn(format string, v ...interface{}) {
	p.mu.RLock()
	slogger := p.slogLogger
	p.mu.RUnlock()

	if slogger != nil {
		slogger.Warn(fmt.Sprintf(format, v...))
	}
}

// logError logs an error message (failures that affect function).
func (p *Pool) logError(format string, v ...interface{}) {
	p.mu.RLock()
	slogger := p.slogLogger
	p.mu.RUnlock()

	if slogger != nil {
		slogger.Error(fmt.Sprintf(format, v...))
	}
}
