package pipeline

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jasonmfehr/go-psrp/messages"
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

func TestPipeline_Invoke(t *testing.T) {
	transport := &mockTransport{}
	runspaceID := uuid.New()
	cmd := "Get-Process"

	p := New(transport, runspaceID, cmd)

	if p.State() != StateNotStarted {
		t.Errorf("expected state NotStarted, got %v", p.State())
	}

	ctx := context.Background()
	err := p.Invoke(ctx)
	if err != nil {
		t.Fatalf("Invoke failed: %v", err)
	}

	if p.State() != StateRunning {
		t.Errorf("expected state Running, got %v", p.State())
	}

	// Verify CREATE_PIPELINE message was sent
	if len(transport.sent) != 1 {
		t.Fatalf("expected 1 message sent, got %d", len(transport.sent))
	}
	msg := transport.sent[0]
	if msg.Type != messages.MessageTypeCreatePipeline {
		t.Errorf("expected CreatePipeline message, got %v", msg.Type)
	}
	if msg.RunspaceID != runspaceID {
		t.Errorf("expected RunspaceID %v, got %v", runspaceID, msg.RunspaceID)
	}
	if msg.PipelineID != p.ID() {
		t.Errorf("expected PipelineID %v, got %v", p.ID(), msg.PipelineID)
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
	case <-time.After(time.Second):
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
	case <-time.After(time.Second):
		t.Error("timeout waiting for completion signal")
	}
}
