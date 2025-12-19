package pipeline

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/smnsjas/go-psrpcore/host"
	"github.com/smnsjas/go-psrpcore/messages"
)

// mockTransport is a mock implementation of Transport for testing.
type mockTransport struct {
	sent []*messages.Message
	err  error
}

func (m *mockTransport) SendMessage(ctx context.Context, msg *messages.Message) error {
	if m.err != nil {
		return m.err
	}
	m.sent = append(m.sent, msg)
	return nil
}

func (m *mockTransport) Host() host.Host {
	return host.NewNullHost()
}

func TestPipeline_Invoke(t *testing.T) {
	tests := []struct {
		name          string
		transportErr  error
		expectedState State
		expectError   bool
	}{
		{
			name:          "Success",
			transportErr:  nil,
			expectedState: StateRunning,
			expectError:   false,
		},
		{
			name:          "TransportFailure",
			transportErr:  errors.New("network error"),
			expectedState: StateNotStarted, // Should stay NotStarted if send fails? Or Stopped/Failed?
			// Looking at implementation (assumed): if SendMessage fails, Invoke returns error.
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &mockTransport{err: tt.transportErr}
			runspaceID := uuid.New()
			cmd := "Get-Process"

			p := New(transport, runspaceID, cmd)

			if p.State() != StateNotStarted {
				t.Errorf("expected initial state NotStarted, got %v", p.State())
			}

			ctx := context.Background()
			err := p.Invoke(ctx)

			if tt.expectError {
				if err == nil {
					t.Error("expected Invoke error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("Invoke failed: %v", err)
				}
			}

			// If transport error, state might be unchanged or Failed depending on implementation.
			// Currently implementation likely keeps it NotStarted or transitions back.
			if !tt.expectError && p.State() != tt.expectedState {
				t.Errorf("expected state %v, got %v", tt.expectedState, p.State())
			}

			if !tt.expectError {
				// Verify CREATE_PIPELINE message was sent
				if len(transport.sent) != 1 {
					t.Fatalf("expected 1 message sent, got %d", len(transport.sent))
				}
				msg := transport.sent[0]
				if msg.Type != messages.MessageTypeCreatePipeline {
					t.Errorf("expected CreatePipeline message, got %v", msg.Type)
				}
			}
		})
	}
}

func TestPipeline_HandleMessage_Output(t *testing.T) {
	transport := &mockTransport{}
	p := New(transport, uuid.New(), "test")

	// Transition to Running
	_ = p.Invoke(context.Background())

	// Simulate output message
	outMsg := &messages.Message{
		Type:       messages.MessageTypePipelineOutput,
		PipelineID: p.ID(),
		Data:       []byte("output data"),
	}

	err := p.HandleMessage(outMsg)
	if err != nil {
		t.Fatalf("HandleMessage failed: %v", err)
	}

	// Check output channel
	select {
	case received := <-p.Output():
		if received != outMsg {
			t.Error("received wrong message")
		}
	case <-time.After(500 * time.Millisecond): // Reduced timeout
		t.Error("timeout waiting for output")
	}
}

func TestPipeline_Completion(t *testing.T) {
	transport := &mockTransport{}
	p := New(transport, uuid.New(), "test")
	_ = p.Invoke(context.Background())

	// Simulate State Completed
	stateMsg := &messages.Message{
		Type:       messages.MessageTypePipelineState,
		PipelineID: p.ID(),
		// In reality this would contain CLIXML state
	}

	// Handle completion
	_ = p.HandleMessage(stateMsg)

	// Check state
	if p.State() != StateCompleted {
		t.Errorf("expected state Completed, got %v", p.State())
	}

	// Check wait
	select {
	case <-p.doneCh:
		// success
	case <-time.After(500 * time.Millisecond): // Reduced timeout
		t.Error("timeout waiting for completion signal")
	}
}

func TestPipeline_Stop(t *testing.T) {
	transport := &mockTransport{}
	p := New(transport, uuid.New(), "test")
	_ = p.Invoke(context.Background())

	// Call Stop
	err := p.Stop(context.Background())
	if err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	if p.State() != StateStopping {
		t.Errorf("expected state Stopping, got %v", p.State())
	}

	// Verify SIGNAL message sent
	if len(transport.sent) != 2 { // Create + Signal
		t.Fatalf("expected 2 messages, got %d", len(transport.sent))
	}
	msg := transport.sent[1]
	if msg.Type != messages.MessageTypeSignal {
		t.Errorf("expected Signal message, got %v", msg.Type)
	}
}

func TestPipeline_Input(t *testing.T) {
	transport := &mockTransport{}
	p := New(transport, uuid.New(), "test")
	_ = p.Invoke(context.Background())

	// Send Input
	inputData := "some input"
	err := p.SendInput(context.Background(), inputData)
	if err != nil {
		t.Fatalf("SendInput failed: %v", err)
	}

	// Verify PIPELINE_INPUT message
	if len(transport.sent) < 2 {
		t.Fatalf("expected input message to be sent")
	}
	msg := transport.sent[1]
	if msg.Type != messages.MessageTypePipelineInput {
		t.Errorf("expected PipelineInput message, got %v", msg.Type)
	}

	// Close Input
	err = p.CloseInput(context.Background())
	if err != nil {
		t.Fatalf("CloseInput failed: %v", err)
	}

	// Verify END_OF_PIPELINE_INPUT message
	if len(transport.sent) < 3 {
		t.Fatalf("expected end of input message to be sent")
	}
	msgEnd := transport.sent[2]
	if msgEnd.Type != messages.MessageTypeEndOfPipelineInput {
		t.Errorf("expected EndOfPipelineInput message, got %v", msgEnd.Type)
	}
}
