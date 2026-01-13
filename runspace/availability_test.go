package runspace

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestWaitForAvailability_BlocksWhenZero(t *testing.T) {
	transport := &mockTransport{}
	pool := New(transport, uuid.New())

	// Set pool to Opened state (prerequisite for WaitForAvailability)
	pool.mu.Lock()
	pool.state = StateOpened
	pool.mu.Unlock()

	// Availability is 0 by default
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	start := time.Now()
	err := pool.WaitForAvailability(ctx, 1)
	elapsed := time.Since(start)

	// Should timeout
	if err != context.DeadlineExceeded {
		t.Errorf("Expected context.DeadlineExceeded, got %v", err)
	}

	// Should have waited close to timeout duration
	if elapsed < 90*time.Millisecond {
		t.Errorf("Wait returned too quickly: %v", elapsed)
	}
}

func TestWaitForAvailability_ReturnsWhenAvailable(t *testing.T) {
	transport := &mockTransport{}
	pool := New(transport, uuid.New())

	// Set pool to Opened state
	pool.mu.Lock()
	pool.state = StateOpened
	pool.mu.Unlock()

	// Set availability to non-zero
	pool.availabilityMu.Lock()
	pool.availableRunspaces = 3
	pool.availabilityMu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	start := time.Now()
	err := pool.WaitForAvailability(ctx, 1)
	elapsed := time.Since(start)

	// Should succeed immediately
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	// Should return quickly (fast path)
	if elapsed > 50*time.Millisecond {
		t.Errorf("Wait took too long: %v", elapsed)
	}
}

func TestWaitForAvailability_FailsWhenNotOpened(t *testing.T) {
	transport := &mockTransport{}
	pool := New(transport, uuid.New())

	// Pool is in StateBeforeOpen by default

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	err := pool.WaitForAvailability(ctx, 1)

	// Should fail with state error
	if err == nil {
		t.Error("Expected error when pool not opened, got nil")
	}

	// Error message should mention state
	if err != nil && err.Error() == "" {
		t.Error("Expected non-empty error message")
	}
}

func TestWaitForAvailability_ChannelNotification(t *testing.T) {
	transport := &mockTransport{}
	pool := New(transport, uuid.New())

	// Set pool to Opened state
	pool.mu.Lock()
	pool.state = StateOpened
	pool.mu.Unlock()

	// Availability is 0 initially
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Start wait in background
	errCh := make(chan error, 1)
	go func() {
		errCh <- pool.WaitForAvailability(ctx, 1)
	}()

	// Give it time to start waiting
	time.Sleep(50 * time.Millisecond)

	// Simulate receiving RUNSPACE_AVAILABILITY message
	pool.availabilityMu.Lock()
	pool.availableRunspaces = 2
	if pool.availabilityCh == nil {
		pool.availabilityCh = make(chan int, 1)
	}
	pool.availabilityCh <- 2
	pool.availabilityMu.Unlock()

	// Wait should complete
	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("Expected no error after availability update, got %v", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Error("WaitForAvailability did not return after channel notification")
	}
}

func TestRunspaceUtilization(t *testing.T) {
	transport := &mockTransport{}
	pool := New(transport, uuid.New())

	// Set max runspaces
	pool.mu.Lock()
	pool.maxRunspaces = 5
	pool.negotiatedMaxRunspaces = 5
	pool.mu.Unlock()

	// Set availability
	pool.availabilityMu.Lock()
	pool.availableRunspaces = 3
	pool.availabilityMu.Unlock()

	available, total := pool.RunspaceUtilization()

	if available != 3 {
		t.Errorf("Expected available=3, got %d", available)
	}
	if total != 5 {
		t.Errorf("Expected total=5, got %d", total)
	}
}

func TestRunspaceUtilization_BeforeNegotiation(t *testing.T) {
	transport := &mockTransport{}
	pool := New(transport, uuid.New())

	// Set only maxRunspaces (not negotiated yet)
	pool.mu.Lock()
	pool.maxRunspaces = 3
	pool.negotiatedMaxRunspaces = 0 // Not negotiated
	pool.mu.Unlock()

	available, total := pool.RunspaceUtilization()

	// Should fall back to configured max
	if total != 3 {
		t.Errorf("Expected total=3 (fallback to maxRunspaces), got %d", total)
	}
	if available != 0 {
		t.Errorf("Expected available=0, got %d", available)
	}
}
