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
	"sync"

	"github.com/google/uuid"
	"github.com/jasonmfehr/go-psrp/fragments"
	"github.com/jasonmfehr/go-psrp/host"
	"github.com/jasonmfehr/go-psrp/messages"
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
)

// State represents the current state of a RunspacePool.
type State int

const (
	// StateBeforeOpen is the initial state before the pool is opened.
	StateBeforeOpen State = iota
	// StateOpening indicates capability exchange and initialization in progress.
	StateOpening
	// StateOpened indicates the pool is ready for use.
	StateOpened
	// StateClosing indicates the pool is being closed.
	StateClosing
	// StateClosed indicates the pool is closed.
	StateClosed
	// StateBroken indicates an error occurred and the pool is in a failed state.
	StateBroken
)

// String returns a string representation of the state.
func (s State) String() string {
	switch s {
	case StateBeforeOpen:
		return "BeforeOpen"
	case StateOpening:
		return "Opening"
	case StateOpened:
		return "Opened"
	case StateClosing:
		return "Closing"
	case StateClosed:
		return "Closed"
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

	// Configuration
	minRunspaces int
	maxRunspaces int

	// Protocol state
	fragmenter *fragments.Fragmenter
	assembler  *fragments.Assembler

	// Host callback handling
	hostCallbackHandler *host.CallbackHandler

	// Channels for message processing
	stateCh chan State
	errCh   chan error
}

// New creates a new RunspacePool with the given transport and ID.
// The pool starts in StateBeforeOpen.
func New(transport io.ReadWriter, id uuid.UUID) *Pool {
	return &Pool{
		id:                  id,
		state:               StateBeforeOpen,
		transport:           transport,
		minRunspaces:        1,
		maxRunspaces:        1,
		fragmenter:          fragments.NewFragmenter(32768), // Default max fragment size
		assembler:           fragments.NewAssembler(),
		hostCallbackHandler: host.NewCallbackHandler(host.NewNullHost()),
		stateCh:             make(chan State, 1),
		errCh:               make(chan error, 1),
	}
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
	p.hostCallbackHandler = host.NewCallbackHandler(h)
	return nil
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

// SetMinRunspaces sets the minimum number of runspaces in the pool.
// Must be called before Open().
func (p *Pool) SetMinRunspaces(min int) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.state != StateBeforeOpen {
		return ErrInvalidState
	}
	if min < 1 {
		return fmt.Errorf("min runspaces must be >= 1, got %d", min)
	}
	p.minRunspaces = min
	return nil
}

// SetMaxRunspaces sets the maximum number of runspaces in the pool.
// Must be called before Open().
func (p *Pool) SetMaxRunspaces(max int) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.state != StateBeforeOpen {
		return ErrInvalidState
	}
	if max < 1 {
		return fmt.Errorf("max runspaces must be >= 1, got %d", max)
	}
	p.maxRunspaces = max
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
	if err := p.sendSessionCapability(ctx); err != nil {
		p.setBroken()
		return fmt.Errorf("send session capability: %w", err)
	}

	// Receive SESSION_CAPABILITY response
	if err := p.receiveSessionCapability(ctx); err != nil {
		p.setBroken()
		return fmt.Errorf("receive session capability: %w", err)
	}

	// Send INIT_RUNSPACEPOOL
	if err := p.sendInitRunspacePool(ctx); err != nil {
		p.setBroken()
		return fmt.Errorf("send init runspace pool: %w", err)
	}

	// Wait for RUNSPACEPOOL_STATE response
	if err := p.waitForOpened(ctx); err != nil {
		p.setBroken()
		return fmt.Errorf("wait for opened: %w", err)
	}

	return nil
}

// Close closes the runspace pool.
// Transitions from Opened → Closing → Closed.
func (p *Pool) Close(ctx context.Context) error {
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

	// TODO: Send close messages to server

	// Transition to Closed
	p.mu.Lock()
	p.setState(StateClosed)
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
// Can be called from any state.
func (p *Pool) setBroken() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.setState(StateBroken)
}

// sendSessionCapability sends the SESSION_CAPABILITY message to the server.
func (p *Pool) sendSessionCapability(ctx context.Context) error {
	// TODO: Build proper capability data with protocol version, etc.
	// For now, send minimal capability
	capabilityData := []byte(`<Obj RefId="0">
  <MS>
    <Version N="protocolversion">2.3</Version>
    <Version N="PSVersion">5.1.0.0</Version>
    <Version N="SerializationVersion">1.1.0.1</Version>
  </MS>
</Obj>`)

	msg := messages.NewSessionCapability(p.id, capabilityData)
	return p.sendMessage(ctx, msg)
}

// receiveSessionCapability receives and validates the server's SESSION_CAPABILITY response.
func (p *Pool) receiveSessionCapability(ctx context.Context) error {
	msg, err := p.receiveMessageWithHostCallbacks(ctx)
	if err != nil {
		return err
	}

	if msg.Type != messages.MessageTypeSessionCapability {
		return fmt.Errorf("expected SESSION_CAPABILITY, got %v", msg.Type)
	}

	// TODO: Parse and validate capability data
	return nil
}

// sendInitRunspacePool sends the INIT_RUNSPACEPOOL message to the server.
func (p *Pool) sendInitRunspacePool(ctx context.Context) error {
	p.mu.RLock()
	minRunspaces := p.minRunspaces
	maxRunspaces := p.maxRunspaces
	p.mu.RUnlock()

	// TODO: Build proper initialization data with min/max runspaces, thread options, etc.
	initData := []byte(fmt.Sprintf(`<Obj RefId="0">
  <MS>
    <I32 N="MinRunspaces">%d</I32>
    <I32 N="MaxRunspaces">%d</I32>
  </MS>
</Obj>`, minRunspaces, maxRunspaces))

	msg := messages.NewInitRunspacePool(p.id, initData)
	return p.sendMessage(ctx, msg)
}

// waitForOpened waits for the RUNSPACEPOOL_STATE message indicating the pool is opened.
func (p *Pool) waitForOpened(ctx context.Context) error {
	msg, err := p.receiveMessageWithHostCallbacks(ctx)
	if err != nil {
		return err
	}

	if msg.Type != messages.MessageTypeRunspacePoolState {
		return fmt.Errorf("expected RUNSPACEPOOL_STATE, got %v", msg.Type)
	}

	// TODO: Parse state from message data
	// For now, assume it's Opened if we got the message
	p.mu.Lock()
	p.setState(StateOpened)
	p.mu.Unlock()

	return nil
}

// sendMessage sends a PSRP message through the transport.
func (p *Pool) sendMessage(ctx context.Context, msg *messages.Message) error {
	// Encode message
	encoded, err := msg.Encode()
	if err != nil {
		return fmt.Errorf("encode message: %w", err)
	}

	// Fragment message
	frags, err := p.fragmenter.Fragment(encoded)
	if err != nil {
		return fmt.Errorf("fragment message: %w", err)
	}

	// Send each fragment
	for _, frag := range frags {
		fragData := frag.Encode()

		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if _, err := p.transport.Write(fragData); err != nil {
			return fmt.Errorf("write fragment: %w", err)
		}
	}

	return nil
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
