package pipeline

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/google/uuid"
	"github.com/smnsjas/go-psrpcore/host"
	"github.com/smnsjas/go-psrpcore/messages"
	"github.com/smnsjas/go-psrpcore/objects"
	"github.com/smnsjas/go-psrpcore/serialization"
)

var (
	// ErrInvalidState is returned when an operation is attempted in an invalid state.
	ErrInvalidState = errors.New("invalid pipeline state")
)

// State represents the current state of a Pipeline.
type State int

const (
	// StateNotStarted indicates the pipeline has not been invoked yet.
	StateNotStarted State = iota
	// StateRunning indicates the pipeline is currently executing.
	StateRunning
	// StateStopping indicates the pipeline is in the process of stopping.
	StateStopping
	// StateStopped indicates the pipeline has been stopped.
	StateStopped
	// StateCompleted indicates the pipeline completed successfully.
	StateCompleted
	// StateFailed indicates the pipeline failed with an error.
	StateFailed
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
	inputCh  chan interface{}

	// Completion
	doneCh chan struct{}
	err    error
}

// New creates a new Pipeline attached to the given transport.
// command can be a raw script, which will be wrapped in a PowerShell object.
func New(transport Transport, runspaceID uuid.UUID, command string) *Pipeline {
	ps := objects.NewPowerShell()
	// Default to treating input as a script
	ps.AddCommand(command, true)

	return &Pipeline{
		id:         uuid.New(),
		runspaceID: runspaceID,
		state:      StateNotStarted,
		transport:  transport,
		powerShell: ps,
		outputCh:   make(chan *messages.Message, 100), // Buffered to prevent blocking
		errorCh:    make(chan *messages.Message, 100),
		doneCh:     make(chan struct{}),
	}
}

// NewBuilder creates a new Pipeline with an empty command list.
// Use AddCommand/AddParameter to build the pipeline.
func NewBuilder(transport Transport, runspaceID uuid.UUID) *Pipeline {
	return &Pipeline{
		id:         uuid.New(),
		runspaceID: runspaceID,
		state:      StateNotStarted,
		transport:  transport,
		powerShell: objects.NewPowerShell(),
		outputCh:   make(chan *messages.Message, 100),
		errorCh:    make(chan *messages.Message, 100),
		doneCh:     make(chan struct{}),
	}
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
	p.mu.Unlock()

	// Create CREATE_PIPELINE message
	// Serialize the PowerShell object to CLIXML
	serializer := serialization.NewSerializer()
	cmdData, err := serializer.Serialize(p.powerShell)
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
	xmlData, err := serializer.Serialize(data)
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

// HandleMessage processes an incoming message destined for this pipeline.
func (p *Pipeline) HandleMessage(msg *messages.Message) error {
	switch msg.Type {
	case messages.MessageTypePipelineOutput:
		select {
		case p.outputCh <- msg:
		default:
			// Buffer full, drop or block? For now drop to avoid deadlock if reader is slow
			// In production, this should probably block or have unlimited buffer
			return fmt.Errorf("output buffer full")
		}

	case messages.MessageTypeErrorRecord:
		select {
		case p.errorCh <- msg:
		default:
			return fmt.Errorf("error buffer full")
		}

	case messages.MessageTypePipelineState:
		// TODO: Parse CLIXML state
		// For now, assume Completed if we see this
		// We need to parse the state to know if it's Completed, Failed, or Stopped
		p.transition(StateCompleted, nil)

	case messages.MessageTypePipelineHostCall:
		go func() {
			// Handle host call in background
			if err := p.handleHostCall(context.Background(), msg); err != nil {
				// TODO: Log error or signal failure?
				_ = err
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

	if newState == StateCompleted || newState == StateFailed || newState == StateStopped {
		close(p.doneCh)
		close(p.outputCh)
		close(p.errorCh)
	}
}
