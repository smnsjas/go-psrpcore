package pipeline

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/google/uuid"
	"github.com/jasonmfehr/go-psrp/messages"
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
}

// Pipeline represents a PSRP command execution pipeline.
type Pipeline struct {
	mu sync.RWMutex

	id         uuid.UUID
	runspaceID uuid.UUID
	state      State
	transport  Transport
	command    string

	// Channels for streams
	outputCh chan *messages.Message
	errorCh  chan *messages.Message
	inputCh  chan interface{}

	// Completion
	doneCh chan struct{}
	err    error
}

// New creates a new Pipeline attached to the given transport.
func New(transport Transport, runspaceID uuid.UUID, command string) *Pipeline {
	return &Pipeline{
		id:         uuid.New(),
		runspaceID: runspaceID,
		state:      StateNotStarted,
		transport:  transport,
		command:    command,
		outputCh:   make(chan *messages.Message, 100), // Buffered to prevent blocking
		errorCh:    make(chan *messages.Message, 100),
		doneCh:     make(chan struct{}),
	}
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
	// TODO: Properly serialize command to CLIXML
	// For now, we'll assume the command string is sufficient for a placeholder
	// In reality, this needs to be a PowerShell object
	// We will revisit this when we implement the full serializer integration
	// But for the state machine logic, this is fine.
	cmdData := []byte(p.command)

	msg := messages.NewCreatePipeline(p.runspaceID, p.id, cmdData)
	if err := p.transport.SendMessage(ctx, msg); err != nil {
		p.transition(StateFailed, err)
		return fmt.Errorf("send create pipeline: %w", err)
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
