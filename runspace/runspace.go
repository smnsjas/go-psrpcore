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

	// Negotiated capabilities
	serverProtocolVersion  string
	serverPSVersion        string
	negotiatedMaxRunspaces int
	negotiatedMinRunspaces int

	// Host callback handling
	// Host callback handling
	host                host.Host
	hostCallbackHandler *host.CallbackHandler

	// Metadata
	metadataCh chan *messages.Message

	// Pipelines
	pipelines map[uuid.UUID]*pipeline.Pipeline

	// Channels for message processing
	stateCh    chan State
	outgoingCh chan *messages.Message
	readyCh    chan struct{}
	doneCh     chan struct{}

	// Lifecycle
	ctx    context.Context
	cancel context.CancelFunc
}

// New creates a new RunspacePool with the given transport and ID.
// The pool starts in StateBeforeOpen.
func New(transport io.ReadWriter, id uuid.UUID) *Pool {
	defaultHost := host.NewNullHost()
	ctx, cancel := context.WithCancel(context.Background())
	return &Pool{
		id:                  id,
		state:               StateBeforeOpen,
		transport:           transport,
		minRunspaces:        1,
		maxRunspaces:        1,
		host:                defaultHost,
		hostCallbackHandler: host.NewCallbackHandler(defaultHost),
		metadataCh:          make(chan *messages.Message, 1),
		fragmenter:          fragments.NewFragmenter(32768), // Default max fragment size
		assembler:           fragments.NewAssembler(),
		pipelines:           make(map[uuid.UUID]*pipeline.Pipeline),
		stateCh:             make(chan State, 1),
		outgoingCh:          make(chan *messages.Message, 100),
		readyCh:             make(chan struct{}),
		doneCh:              make(chan struct{}),
		ctx:                 ctx,
		cancel:              cancel,
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
	p.host = h
	p.hostCallbackHandler = host.NewCallbackHandler(h)
	return nil
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

	// Start message dispatch loop
	go p.dispatchLoop(p.ctx)

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
		return fmt.Errorf("%w: incompatible protocol version: server=%s, client=2.3", ErrProtocolViolation, caps.ProtocolVersion)
	}

	// Store negotiated capabilities
	p.mu.Lock()
	p.serverProtocolVersion = caps.ProtocolVersion
	p.serverPSVersion = caps.PSVersion
	p.mu.Unlock()

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
	data, err := serializer.Serialize(names)
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
func (p *Pool) waitForOpened(ctx context.Context) error {
	msg, err := p.receiveMessageWithHostCallbacks(ctx)
	if err != nil {
		return err
	}

	if msg.Type != messages.MessageTypeRunspacePoolState {
		return fmt.Errorf("%w: expected RUNSPACEPOOL_STATE, got %v", ErrProtocolViolation, msg.Type)
	}

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
}

// SendMessage sends a PSRP message to the server (implements pipeline.Transport).
func (p *Pool) SendMessage(ctx context.Context, msg *messages.Message) error {
	return p.sendMessage(ctx, msg)
}

// CreatePipeline creates a new pipeline in the runspace pool.
func (p *Pool) CreatePipeline(command string) (*pipeline.Pipeline, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.state != StateOpened {
		return nil, ErrNotOpen
	}

	pl := pipeline.New(p, p.id, command)
	p.pipelines[pl.ID()] = pl

	// Start a monitoring goroutine to remove the pipeline when it's done
	go func() {
		_ = pl.Wait() // Wait for completion
		p.removePipeline(pl.ID())
	}()

	return pl, nil
}

// removePipeline removes a pipeline from the pool's map.
func (p *Pool) removePipeline(id uuid.UUID) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.pipelines, id)
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
			// TODO: Handle disconnection/error
			// For now, simple exit (or maybe set Broken)
			return
		}

		// Dispatch based on PipelineID
		if msg.PipelineID != uuid.Nil {
			p.mu.RLock()
			pl, exists := p.pipelines[msg.PipelineID]
			p.mu.RUnlock()

			if exists {
				// Non-blocking handle?
				// HandleMessage in pipeline should be fast (just channel send)
				_ = pl.HandleMessage(msg)
			}
			continue
		}

		// Runspace-level messages
		switch msg.Type {
		case messages.MessageTypeRunspaceHostCall:
			// Dispatch host call in background to not block loop
			go func() {
				if err := p.handleHostCall(ctx, msg); err != nil {
					// Handling host call failed (likely transport error), pool is likely broken
					p.setBroken()
				}
			}()
			// Handle metadata replies
		case messages.MessageTypeGetCommandMetadataReply:
			select {
			case p.metadataCh <- msg:
			default:
				// Dropping metadata reply if channel is full or no one is listening
				// In a real implementation we might want to log this
			}

		case messages.MessageTypeRunspacePoolState:
			// Handle state changes if any (e.g. Broken/Closed from server)
		}
	}
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

	// The state should be an Int32
	if state, ok := objs[0].(int32); ok {
		info.State = messages.RunspacePoolState(state)
	} else {
		return nil, fmt.Errorf("state is not int32, got %T", objs[0])
	}

	// Additional objects may contain min/max runspaces
	if len(objs) > 1 {
		if psObj, ok := objs[1].(*serialization.PSObject); ok {
			if min, ok := psObj.Properties["MinRunspaces"].(int32); ok {
				info.MinRunspaces = int(min)
			}
			if max, ok := psObj.Properties["MaxRunspaces"].(int32); ok {
				info.MaxRunspaces = int(max)
			}
		}
	}

	return info, nil
}
