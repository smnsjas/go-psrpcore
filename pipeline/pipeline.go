package pipeline

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/smnsjas/go-psrpcore/fragments"
	"github.com/smnsjas/go-psrpcore/host"
	"github.com/smnsjas/go-psrpcore/messages"
	"github.com/smnsjas/go-psrpcore/objects"
	"github.com/smnsjas/go-psrpcore/serialization"
)

var (
	// ErrInvalidState is returned when an operation is attempted in an invalid state.
	ErrInvalidState = errors.New("invalid pipeline state")
	// ErrBufferFull is returned when a channel buffer is full and a message cannot be delivered within the timeout.
	ErrBufferFull = errors.New("channel buffer full, message delivery timed out")
)

const (
	// DefaultChannelTimeout is the default timeout for channel send operations when buffer is full.
	// This provides back-pressure to prevent unbounded memory growth while avoiding deadlocks.
	DefaultChannelTimeout = 5 * time.Second
)

// State represents the current state of a Pipeline.
type State int

const (
	// StateNotStarted indicates the pipeline has not been invoked yet.
	// MS-PSRP Section 2.2.3.9: PSInvocationState value 0.
	StateNotStarted State = iota
	// StateRunning indicates the pipeline is currently executing.
	// MS-PSRP Section 2.2.3.9: PSInvocationState value 1.
	StateRunning
	// StateStopping indicates the pipeline is in the process of stopping.
	// MS-PSRP Section 2.2.3.9: PSInvocationState value 2.
	StateStopping
	// StateStopped indicates the pipeline has been stopped.
	// MS-PSRP Section 2.2.3.9: PSInvocationState value 3.
	StateStopped
	// StateCompleted indicates the pipeline completed successfully.
	// MS-PSRP Section 2.2.3.9: PSInvocationState value 4.
	StateCompleted
	// StateFailed indicates the pipeline failed with an error.
	// MS-PSRP Section 2.2.3.9: PSInvocationState value 5.
	StateFailed
	// StateDisconnected indicates the pipeline is in disconnected state.
	// MS-PSRP Section 2.2.3.9: PSInvocationState value 6.
	StateDisconnected
)

// String returns a string representation of the state.
func (s State) String() string {
	switch s {
	case StateNotStarted:
		return "NotStarted"
	case StateRunning:
		return "Running"
	case StateStopping:
		return "Stopping"
	case StateStopped:
		return "Stopped"
	case StateCompleted:
		return "Completed"
	case StateFailed:
		return "Failed"
	case StateDisconnected:
		return "Disconnected"
	default:
		return fmt.Sprintf("Unknown(%d)", s)
	}
}

// Transport defines the interface for sending messages to the server.
// This is typically implemented by the RunspacePool.
type Transport interface {
	SendMessage(ctx context.Context, msg *messages.Message) error
	Host() host.Host
}

// Pipeline represents a PSRP command execution pipeline.
type Pipeline struct {
	mu sync.RWMutex

	id         uuid.UUID
	runspaceID uuid.UUID
	state      State
	transport  Transport

	// powerShell represents the pipeline definition (commands and parameters)
	powerShell *objects.PowerShell

	// Channels for streams
	outputCh chan *messages.Message
	errorCh  chan *messages.Message

	// Completion
	doneCh chan struct{}
	err    error

	// Lifecycle management
	ctx    context.Context
	cancel context.CancelFunc

	// channelTimeout is the timeout for channel send operations when buffer is full.
	channelTimeout time.Duration

	// skipInvokeSend prevents Invoke from sending the CreatePipeline message.
	// Used when the data was already sent via another mechanism (e.g., WSMan Command Arguments).
	skipInvokeSend bool
}

// New creates a new Pipeline attached to the given transport.
// command can be a raw script, which will be wrapped in a PowerShell object.
func New(transport Transport, runspaceID uuid.UUID, command string) *Pipeline {
	ps := objects.NewPowerShell()
	// Default to true (script) to support arbitrary commands and pipelines
	ps.AddCommand(command, true)

	ctx, cancel := context.WithCancel(context.Background())
	return &Pipeline{
		id:             uuid.New(),
		runspaceID:     runspaceID,
		state:          StateNotStarted,
		transport:      transport,
		powerShell:     ps,
		outputCh:       make(chan *messages.Message, 100), // Buffered to prevent blocking
		errorCh:        make(chan *messages.Message, 100),
		doneCh:         make(chan struct{}),
		ctx:            ctx,
		cancel:         cancel,
		channelTimeout: DefaultChannelTimeout,
	}
}

// NewBuilder creates a new Pipeline with an empty command list.
// Use AddCommand/AddParameter to build the pipeline.
func NewBuilder(transport Transport, runspaceID uuid.UUID) *Pipeline {
	ctx, cancel := context.WithCancel(context.Background())
	return &Pipeline{
		id:             uuid.New(),
		runspaceID:     runspaceID,
		state:          StateNotStarted,
		transport:      transport,
		powerShell:     objects.NewPowerShell(),
		outputCh:       make(chan *messages.Message, 100),
		errorCh:        make(chan *messages.Message, 100),
		doneCh:         make(chan struct{}),
		ctx:            ctx,
		cancel:         cancel,
		channelTimeout: DefaultChannelTimeout,
	}
}

// SkipInvokeSend prevents Invoke from sending the CreatePipeline message.
// Use this when the CreatePipeline data was already sent via another mechanism
// (e.g., embedded in WSMan Command Arguments).
func (p *Pipeline) SkipInvokeSend() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.skipInvokeSend = true
}

// GetCreatePipelineData generates the CreatePipeline PSRP fragment data.
// This is used by WSMan transports that need to embed the fragment in Command Arguments.
// The returned data is the raw PSRP fragment bytes (not base64 encoded).
func (p *Pipeline) GetCreatePipelineData() ([]byte, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Serialize the PowerShell object to CLIXML
	serializer := serialization.NewSerializer()
	defer serializer.Close()
	cmdData, err := serializer.SerializeRaw(p.powerShell)
	if err != nil {
		return nil, fmt.Errorf("serialize command: %w", err)
	}

	// Create CREATE_PIPELINE message
	msg := messages.NewCreatePipeline(p.runspaceID, p.id, cmdData)

	// Encode message to bytes
	encoded, err := msg.Encode()
	if err != nil {
		return nil, fmt.Errorf("encode message: %w", err)
	}

	// Fragment the encoded message using 32KB max size (standard PSRP)
	fragmenter := fragments.NewFragmenter(32768)
	frags, err := fragmenter.Fragment(encoded)
	if err != nil {
		return nil, fmt.Errorf("fragment message: %w", err)
	}
	if len(frags) == 0 {
		return nil, fmt.Errorf("no fragments generated")
	}

	// Encode all fragments into a single byte slice
	var result []byte
	for _, f := range frags {
		result = append(result, f.Encode()...)
	}

	return result, nil
}

// GetCreatePipelineDataWithID generates the CreatePipeline PSRP fragment data with a specific starting object ID.
// This allows synchronizing the object ID with the session's sequence.
// The provided startObjectID will be incremented before use (ID = startObjectID + 1).
// So to use ID 3, pass startObjectID = 2.
func (p *Pipeline) GetCreatePipelineDataWithID(startObjectID uint64) ([]byte, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Serialize the PowerShell object to CLIXML
	serializer := serialization.NewSerializer()
	defer serializer.Close()
	cmdData, err := serializer.SerializeRaw(p.powerShell)
	if err != nil {
		return nil, fmt.Errorf("serialize command: %w", err)
	}

	// Create CREATE_PIPELINE message
	msg := messages.NewCreatePipeline(p.runspaceID, p.id, cmdData)

	// Encode message to bytes
	encoded, err := msg.Encode()
	if err != nil {
		return nil, fmt.Errorf("encode message: %w", err)
	}

	// Fragment the encoded message using 32KB max size (standard PSRP)
	// Use explicit startObjectID.
	// Note: Fragmenter increments ID before usage (f.objectID++), so we must initialize with startObjectID-1
	// to ensure the first fragment actually uses startObjectID.
	fragmenter := fragments.NewFragmenterWithID(32768, startObjectID-1)
	frags, err := fragmenter.Fragment(encoded)
	if err != nil {
		return nil, fmt.Errorf("fragment message: %w", err)
	}
	if len(frags) == 0 {
		return nil, fmt.Errorf("no fragments generated")
	}

	// Encode all fragments into a single byte slice
	var result []byte
	for _, f := range frags {
		result = append(result, f.Encode()...)
	}

	return result, nil
}

// GetCloseInputData generates the PSRP fragments for the END_OF_PIPELINE_INPUT message.
// This allows embedding the EOF message in the WSMan Command arguments.
func (p *Pipeline) GetCloseInputData() ([]byte, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	msg := messages.NewEndOfPipelineInput(p.runspaceID, p.id)

	encoded, err := msg.Encode()
	if err != nil {
		return nil, fmt.Errorf("encode message: %w", err)
	}

	fragmenter := fragments.NewFragmenter(32768)
	frags, err := fragmenter.Fragment(encoded)
	if err != nil {
		return nil, fmt.Errorf("fragment message: %w", err)
	}

	var result []byte
	for _, f := range frags {
		result = append(result, f.Encode()...)
	}

	return result, nil
}

// AddCommand adds a cmdlet or script to the pipeline.
// isScript should be true if name is a script block or raw script code.
func (p *Pipeline) AddCommand(name string, isScript bool) *Pipeline {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.powerShell.AddCommand(name, isScript)
	return p
}

// AddParameter adds a named parameter to the last added command.
func (p *Pipeline) AddParameter(name string, value interface{}) *Pipeline {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.powerShell.AddParameter(name, value)
	return p
}

// AddArgument adds a positional argument (unnamed parameter) to the last added command.
func (p *Pipeline) AddArgument(value interface{}) *Pipeline {
	// Positional arguments are just parameters with empty names in some contexts,
	// but strictly speaking PSRP often treats them as parameters with no name in the list.
	// We'll reuse AddParameter with empty name which is common convention or check implementation details.
	// For now, empty string name implies positional.
	return p.AddParameter("", value)
}

// SetChannelTimeout sets the timeout for channel send operations when buffers are full.
// The default is DefaultChannelTimeout (5 seconds).
// Setting a longer timeout allows for slower consumers, while a shorter timeout provides faster failure detection.
func (p *Pipeline) SetChannelTimeout(timeout time.Duration) *Pipeline {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.channelTimeout = timeout
	return p
}

// ID returns the unique identifier of the pipeline.
func (p *Pipeline) ID() uuid.UUID {
	return p.id
}

// State returns the current state of the pipeline.
func (p *Pipeline) State() State {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.state
}

// Invoke starts the pipeline execution.
func (p *Pipeline) Invoke(ctx context.Context) error {
	p.mu.Lock()
	if p.state != StateNotStarted {
		p.mu.Unlock()
		return ErrInvalidState
	}
	p.state = StateRunning
	skipSend := p.skipInvokeSend
	p.mu.Unlock()

	// If CreatePipeline data was already sent (e.g., via WSMan Command Arguments),
	// skip sending it again
	if skipSend {
		return nil
	}

	// Create CREATE_PIPELINE message
	// Serialize the PowerShell object to CLIXML
	serializer := serialization.NewSerializer()
	defer serializer.Close()
	cmdData, err := serializer.SerializeRaw(p.powerShell)
	if err != nil {
		p.transition(StateFailed, err)
		return fmt.Errorf("serialize command: %w", err)
	}

	msg := messages.NewCreatePipeline(p.runspaceID, p.id, cmdData)
	if err := p.transport.SendMessage(ctx, msg); err != nil {
		p.transition(StateFailed, err)
		return fmt.Errorf("send create pipeline: %w", err)
	}

	return nil
}

// Stop sends a signal to stop the running pipeline.
// It sends a SIGNAL message (MS-PSRP 2.2.2.10) and transitions to StateStopping.
func (p *Pipeline) Stop(ctx context.Context) error {
	p.mu.Lock()
	if p.state != StateRunning {
		p.mu.Unlock()
		return fmt.Errorf("%w: cannot stop pipeline that is not running (state=%s)", ErrInvalidState, p.state)
	}
	p.state = StateStopping
	p.mu.Unlock()

	msg := messages.NewSignal(p.runspaceID, p.id)
	if err := p.transport.SendMessage(ctx, msg); err != nil {
		return fmt.Errorf("send signal: %w", err)
	}

	return nil
}

// SendInput sends data to the running pipeline's input stream.
// It serializes the data to CLIXML and sends a PIPELINE_INPUT message (MS-PSRP 2.2.2.13).
func (p *Pipeline) SendInput(ctx context.Context, data interface{}) error {
	p.mu.Lock()
	if p.state != StateRunning {
		p.mu.Unlock()
		return fmt.Errorf("%w: cannot send input to pipeline that is not running (state=%s)", ErrInvalidState, p.state)
	}
	p.mu.Unlock()

	serializer := serialization.NewSerializer()
	defer serializer.Close()
	xmlData, err := serializer.SerializeRaw(data)
	if err != nil {
		return fmt.Errorf("serialize input: %w", err)
	}

	msg := messages.NewPipelineInput(p.runspaceID, p.id, xmlData)
	if err := p.transport.SendMessage(ctx, msg); err != nil {
		return fmt.Errorf("send pipeline input: %w", err)
	}

	return nil
}

// CloseInput closes the pipeline's input stream.
// It sends an END_OF_PIPELINE_INPUT message (MS-PSRP 2.2.2.13).
func (p *Pipeline) CloseInput(ctx context.Context) error {
	p.mu.Lock()
	if p.state != StateRunning {
		p.mu.Unlock()
		return fmt.Errorf("%w: cannot close input of pipeline that is not running (state=%s)", ErrInvalidState, p.state)
	}
	p.mu.Unlock()

	// Send END_OF_PIPELINE_INPUT message (MS-PSRP 2.2.2.18)
	msg := messages.NewEndOfPipelineInput(p.runspaceID, p.id)
	if err := p.transport.SendMessage(ctx, msg); err != nil {
		return fmt.Errorf("send end of pipeline input: %w", err)
	}

	return nil
}

// Output returns a channel that emits output messages.
func (p *Pipeline) Output() <-chan *messages.Message {
	return p.outputCh
}

// Error returns a channel that emits error messages.
func (p *Pipeline) Error() <-chan *messages.Message {
	return p.errorCh
}

// Wait waits for the pipeline to complete and returns any error.
func (p *Pipeline) Wait() error {
	<-p.doneCh
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.err
}

// Done returns a channel that is closed when the pipeline completes.
// This can be used with select for non-blocking completion checks.
func (p *Pipeline) Done() <-chan struct{} {
	return p.doneCh
}

// HandleMessage processes an incoming message destined for this pipeline.
// Messages are delivered to buffered channels with timeout-based back-pressure.
// If a channel buffer is full, this method will block for up to channelTimeout
// before returning ErrBufferFull. This prevents unbounded memory growth while
// avoiding deadlocks with slow consumers.
func (p *Pipeline) HandleMessage(msg *messages.Message) error {
	switch msg.Type {
	case messages.MessageTypePipelineOutput:
		// Try immediate send first (fast path)
		select {
		case p.outputCh <- msg:
			return nil
		default:
		}

		// Buffer full - block with timeout for back-pressure
		p.mu.RLock()
		timeout := p.channelTimeout
		p.mu.RUnlock()

		select {
		case p.outputCh <- msg:
			return nil
		case <-time.After(timeout):
			return fmt.Errorf("%w: output channel timeout after %v", ErrBufferFull, timeout)
		case <-p.ctx.Done():
			return p.ctx.Err()
		}

	case messages.MessageTypeErrorRecord:
		// Try immediate send first (fast path)
		select {
		case p.errorCh <- msg:
			return nil
		default:
		}

		// Buffer full - block with timeout for back-pressure
		p.mu.RLock()
		timeout := p.channelTimeout
		p.mu.RUnlock()

		select {
		case p.errorCh <- msg:
			return nil
		case <-time.After(timeout):
			return fmt.Errorf("%w: error channel timeout after %v", ErrBufferFull, timeout)
		case <-p.ctx.Done():
			return p.ctx.Err()
		}

	case messages.MessageTypePipelineState:
		deser := serialization.NewDeserializer()
		objs, err := deser.Deserialize(msg.Data)
		if err != nil || len(objs) == 0 {
			p.transition(StateFailed, fmt.Errorf("parse pipeline state: %w", err))
			return nil
		}

		// Try to extract PipelineState value - it may be a simple int32 or a complex PSObject
		var stateVal int32
		var exception interface{}

		switch v := objs[0].(type) {
		case int32:
			// Simple int32 value (used in unit tests)
			stateVal = v
		case *serialization.PSObject:
			// Complex PSObject (real server response)
			// Look for PipelineState in Members first, then Properties
			var found bool
			if ps, ok := v.Members["PipelineState"]; ok {
				if psInt, ok := ps.(int32); ok {
					stateVal = psInt
					found = true
				}
			}
			if !found {
				if ps, ok := v.Properties["PipelineState"]; ok {
					if psInt, ok := ps.(int32); ok {
						stateVal = psInt
						found = true
					}
				}
			}
			if !found {
				// Fallback to Completed
				p.transition(StateCompleted, nil)
				return nil
			}
			// Extract exception info if present
			if exc, ok := v.Members["ExceptionAsErrorRecord"]; ok {
				exception = exc
			} else if exc, ok := v.Properties["ExceptionAsErrorRecord"]; ok {
				exception = exc
			}
		default:
			// Unknown type, fallback to Completed
			p.transition(StateCompleted, nil)
			return nil
		}

		// MS-PSRP Section 2.2.3.9 - PSInvocationState enum values
		switch stateVal {
		case 0: // NotStarted
			p.mu.Lock()
			p.state = StateNotStarted
			p.mu.Unlock()
		case 1: // Running
			p.mu.Lock()
			p.state = StateRunning
			p.mu.Unlock()
		case 2: // Stopping
			p.mu.Lock()
			p.state = StateStopping
			p.mu.Unlock()
		case 3: // Stopped
			p.transition(StateStopped, nil)
		case 4: // Completed
			p.transition(StateCompleted, nil)
		case 5: // Failed
			errMsg := "pipeline failed on server"
			if exception != nil {
				errMsg = fmt.Sprintf("pipeline failed: %v", exception)
			}
			p.transition(StateFailed, errors.New(errMsg))
		case 6: // Disconnected
			p.transition(StateDisconnected, nil)
		}

	case messages.MessageTypePipelineHostCall:
		go func() {
			// Handle host call in background
			if err := p.handleHostCall(p.ctx, msg); err != nil {
				// Signal failure if host call handling failed (likely transport or protocol error)
				p.transition(StateFailed, fmt.Errorf("handle host call: %w", err))
			}
		}()
	}

	return nil
}

// handleHostCall processes a PIPELINE_HOST_CALL message and sends a response.
func (p *Pipeline) handleHostCall(ctx context.Context, msg *messages.Message) error {
	// Decode the RemoteHostCall from the message data
	call, err := host.DecodeRemoteHostCall(msg.Data)
	if err != nil {
		return fmt.Errorf("decode host call: %w", err)
	}

	// Execute the host callback
	h := p.transport.Host()
	response := host.NewCallbackHandler(h).HandleCall(call)

	// Encode the response
	responseData, err := host.EncodeRemoteHostResponse(response)
	if err != nil {
		return fmt.Errorf("encode host response: %w", err)
	}

	// Send PIPELINE_HOST_RESPONSE message
	responseMsg := messages.NewPipelineHostResponse(p.runspaceID, p.id, responseData)
	if err := p.transport.SendMessage(ctx, responseMsg); err != nil {
		return fmt.Errorf("send host response: %w", err)
	}

	return nil
}

// transition updates the state and signals completion if needed.
func (p *Pipeline) transition(newState State, err error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.state == newState {
		return
	}

	p.state = newState
	p.err = err

	// Terminal states: close channels and cancel context
	if newState == StateCompleted || newState == StateFailed || newState == StateStopped || newState == StateDisconnected {
		close(p.doneCh)
		close(p.outputCh)
		close(p.errorCh)
		p.cancel()
	}
}

// Fail transitions the pipeline to StateFailed with the given error.
// This is used by the runspace pool to signal a fatal transport error.
func (p *Pipeline) Fail(err error) {
	p.transition(StateFailed, err)
}
