package pipeline

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

// TestSkipInvokeSend verifies the boolean accessor
func TestSkipInvokeSend(t *testing.T) {
	p := NewBuilder(&mockTransport{}, uuid.New())

	// Default is false
	if p.skipInvokeSend {
		t.Error("Expected skipInvokeSend to be false by default")
	}

	// Can set it via method
	p.SkipInvokeSend()
	if !p.skipInvokeSend {
		t.Error("Expected skipInvokeSend to be true")
	}
}

// TestStateAccessors verifies read-only accessors
func TestStateAccessors(t *testing.T) {
	transport := &mockTransport{}
	runspaceID := uuid.New()
	p := New(transport, runspaceID, "Get-Process")

	if p.ID() == uuid.Nil {
		t.Error("ID() returned Nil UUID")
	}

	if p.State() != StateNotStarted {
		t.Errorf("Expected NotStarted, got %v", p.State())
	}
}

// TestChannelAccessors verifies channel accessors
func TestChannelAccessors(t *testing.T) {
	transport := &mockTransport{}
	p := New(transport, uuid.New(), "Get-Process")

	if p.Output() == nil {
		t.Error("Output() returned nil")
	}
	if p.Error() == nil {
		t.Error("Error() returned nil")
	}
	if p.Warning() == nil {
		t.Error("Warning() returned nil")
	}
	if p.Verbose() == nil {
		t.Error("Verbose() returned nil")
	}
	if p.Debug() == nil {
		t.Error("Debug() returned nil")
	}
	if p.Progress() == nil {
		t.Error("Progress() returned nil")
	}
	if p.Information() == nil {
		t.Error("Information() returned nil")
	}
	if p.Done() == nil {
		t.Error("Done() returned nil")
	}
}

// TestFragmentGenerators verifies internal fragment generation helpers
// Note: These are verified indirectly via Invoke/Close/Input, but calling them directly
// ensures coverage even if logic changes.
func TestFragmentGenerators(t *testing.T) {
	transport := &mockTransport{}
	p := New(transport, uuid.New(), "Get-Process")

	// CreatePipelineData
	frags, err := p.GetCreatePipelineData()
	if err != nil {
		t.Fatalf("GetCreatePipelineData failed: %v", err)
	}
	if len(frags) == 0 {
		t.Error("Expected create pipeline fragments")
	}

	// CreatePipelineDataWithID
	// pipelineID := uuid.New() // removed, expected uint64
	startObjectID := uint64(100)
	fragsID, err := p.GetCreatePipelineDataWithID(startObjectID)
	if err != nil {
		t.Fatalf("GetCreatePipelineDataWithID failed: %v", err)
	}
	if len(fragsID) == 0 {
		t.Error("Expected create pipeline with ID fragments")
	}

	// CloseInputData
	closeFrags, err := p.GetCloseInputData()
	if err != nil {
		t.Fatalf("GetCloseInputData failed: %v", err)
	}
	if len(closeFrags) == 0 {
		t.Error("Expected close input fragments")
	}
}

// TestSetChannelTimeout verifies timeout setter
func TestSetChannelTimeout(t *testing.T) {
	transport := &mockTransport{}
	p := New(transport, uuid.New(), "Get-Process")

	timeout := 5 * time.Second
	p.SetChannelTimeout(timeout)

	if time.Duration(p.channelTimeout.Load()) != timeout {
		t.Errorf("Expected timeout %v, got %v", timeout, time.Duration(p.channelTimeout.Load()))
	}
}
