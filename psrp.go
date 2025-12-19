package psrp

import (
	"context"
	"io"

	"github.com/google/uuid"
)

// Client manages PSRP communication over a provided transport.
// It handles the protocol state machine and message exchange.
type Client struct {
	transport io.ReadWriter
	// TODO: Add protocol state, fragment buffers, etc.
}

// NewClient creates a new PSRP client using the provided transport.
// The transport must be a bidirectional byte stream (e.g., WSMan connection,
// SSH channel, VMBus socket).
func NewClient(transport io.ReadWriter) *Client {
	return &Client{
		transport: transport,
	}
}

// CreateRunspacePool creates a new runspace pool on the remote server.
// The pool manages one or more PowerShell runspaces for executing commands.
func (c *Client) CreateRunspacePool(ctx context.Context, opts ...RunspacePoolOption) (*RunspacePool, error) {
	pool := &RunspacePool{
		client: c,
		id:     uuid.New(),
		state:  RunspacePoolStateBeforeOpen,
	}

	for _, opt := range opts {
		opt(pool)
	}

	// TODO: Implement SESSION_CAPABILITY exchange
	// TODO: Implement INIT_RUNSPACEPOOL message
	// TODO: Handle response and state transitions

	return pool, nil
}

// Close closes the client and releases resources.
func (c *Client) Close() error {
	// TODO: Send proper close messages
	if closer, ok := c.transport.(io.Closer); ok {
		return closer.Close()
	}
	return nil
}

// RunspacePoolState represents the state of a RunspacePool.
type RunspacePoolState int

const (
	RunspacePoolStateBeforeOpen RunspacePoolState = iota
	RunspacePoolStateOpening
	RunspacePoolStateOpened
	RunspacePoolStateClosing
	RunspacePoolStateClosed
	RunspacePoolStateBroken
)

// RunspacePool represents a pool of PowerShell runspaces on the remote server.
type RunspacePool struct {
	client *Client
	id     uuid.UUID
	state  RunspacePoolState

	minRunspaces int
	maxRunspaces int
}

// RunspacePoolOption configures a RunspacePool.
type RunspacePoolOption func(*RunspacePool)

// WithMinRunspaces sets the minimum number of runspaces in the pool.
func WithMinRunspaces(min int) RunspacePoolOption {
	return func(rp *RunspacePool) {
		rp.minRunspaces = min
	}
}

// WithMaxRunspaces sets the maximum number of runspaces in the pool.
func WithMaxRunspaces(max int) RunspacePoolOption {
	return func(rp *RunspacePool) {
		rp.maxRunspaces = max
	}
}

// ID returns the unique identifier of the runspace pool.
func (rp *RunspacePool) ID() uuid.UUID {
	return rp.id
}

// State returns the current state of the runspace pool.
func (rp *RunspacePool) State() RunspacePoolState {
	return rp.state
}

// CreatePowerShell creates a new PowerShell pipeline in this runspace pool.
func (rp *RunspacePool) CreatePowerShell() *PowerShell {
	return &PowerShell{
		pool: rp,
		id:   uuid.New(),
	}
}

// Close closes the runspace pool.
func (rp *RunspacePool) Close(ctx context.Context) error {
	// TODO: Implement proper close sequence
	rp.state = RunspacePoolStateClosed
	return nil
}

// PowerShell represents a PowerShell command pipeline.
type PowerShell struct {
	pool     *RunspacePool
	id       uuid.UUID
	commands []command
}

type command struct {
	name       string
	parameters map[string]interface{}
	isScript   bool
}

// ID returns the unique identifier of this pipeline.
func (ps *PowerShell) ID() uuid.UUID {
	return ps.id
}

// AddCommand adds a cmdlet or function to the pipeline.
func (ps *PowerShell) AddCommand(name string) *PowerShell {
	ps.commands = append(ps.commands, command{
		name:       name,
		parameters: make(map[string]interface{}),
		isScript:   false,
	})
	return ps
}

// AddScript adds a script block to the pipeline.
func (ps *PowerShell) AddScript(script string) *PowerShell {
	ps.commands = append(ps.commands, command{
		name:       script,
		parameters: make(map[string]interface{}),
		isScript:   true,
	})
	return ps
}

// AddParameter adds a parameter to the last command in the pipeline.
func (ps *PowerShell) AddParameter(name string, value interface{}) *PowerShell {
	if len(ps.commands) > 0 {
		ps.commands[len(ps.commands)-1].parameters[name] = value
	}
	return ps
}

// AddArgument adds a positional argument to the last command.
func (ps *PowerShell) AddArgument(value interface{}) *PowerShell {
	// Positional arguments use empty string key
	return ps.AddParameter("", value)
}

// Invoke executes the pipeline and returns the output objects.
func (ps *PowerShell) Invoke(ctx context.Context) ([]PSObject, error) {
	// TODO: Implement CREATE_PIPELINE message
	// TODO: Handle PIPELINE_OUTPUT messages
	// TODO: Handle PIPELINE_STATE messages
	// TODO: Return deserialized objects

	return nil, nil
}

// InvokeAsync executes the pipeline asynchronously, returning channels for output.
func (ps *PowerShell) InvokeAsync(ctx context.Context) (<-chan PSObject, <-chan error) {
	outputCh := make(chan PSObject)
	errCh := make(chan error, 1)

	go func() {
		defer close(outputCh)
		defer close(errCh)

		output, err := ps.Invoke(ctx)
		if err != nil {
			errCh <- err
			return
		}

		for _, obj := range output {
			select {
			case outputCh <- obj:
			case <-ctx.Done():
				errCh <- ctx.Err()
				return
			}
		}
	}()

	return outputCh, errCh
}

// PSObject represents a deserialized PowerShell object.
type PSObject struct {
	TypeNames  []string
	Properties map[string]interface{}
	BaseObject interface{}
}

// String returns a string representation of the PSObject.
func (o PSObject) String() string {
	if s, ok := o.BaseObject.(string); ok {
		return s
	}
	// TODO: Better string formatting
	return ""
}
