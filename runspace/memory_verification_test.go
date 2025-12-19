package runspace

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/smnsjas/go-psrpcore/messages"
)

// MockTransport implements pipeline.Transport but also runspace.Transport (io.ReadWriter) partially
// Since RunspacePool uses io.ReadWriter for transport, we need to mock that too if we want to run full Open()
// But for memory leak test we can just check pipelines map behavior if we bypass Open or mock enough.
// Actually, CreatePipeline checks for StateOpened. So we must Open() or manually set state.
// Since `state` is private, we are testing from within the `runspace` package so we can set it.

type MockReadWriter struct{}

func (m *MockReadWriter) Read(p []byte) (n int, err error) {
	// Block indefinitely to simulate idle connection
	select {}
}

func (m *MockReadWriter) Write(p []byte) (n int, err error) {
	return len(p), nil
}

func TestRunspacePool_PipelineCleanup(t *testing.T) {
	// 1. Create Pool
	pool := New(&MockReadWriter{}, uuid.New())
	// Manually set state to Opened to bypass handshake
	pool.state = StateOpened

	// 2. Create a Pipeline
	pl, err := pool.CreatePipeline("Get-Date")
	if err != nil {
		t.Fatalf("CreatePipeline failed: %v", err)
	}

	// Verify pipeline is in the map
	pool.mu.RLock()
	if _, ok := pool.pipelines[pl.ID()]; !ok {
		pool.mu.RUnlock()
		t.Fatal("Pipeline not found in map immediately after creation")
	}
	pool.mu.RUnlock()

	// 3. Manually simulate pipeline completion logic
	// Since we are mocking everything, the pipeline won't actually run or complete on its own unless we trigger it.
	// But `pl.Wait()` waits for `pl.doneCh`.
	// If we transition the pipeline to Completed/Stopped, `doneCh` will be closed.
	// We can't access `pl.transition` directly as it's private in `pipeline` package and we are in `runspace` package.
	// However, `pipeline.Pipeline` has a `Stop` method, but that sends a message and waits for transport.
	// We might need to rely on the fact that `New` in `runspace` uses the pool as transport.
	// And `fake` transport might need to handle messages.

	// Let's force close the pipeline's doneCh indirectly?
	// Or we can just cancel the pipeline's context if we exposed it? No.

	// Actually, `pl.Stop(ctx)` sends a Signal.
	// But `pl.transition(StateCompleted)` is what closes doneCh.
	// That happens when `HandleMessage` receives a State message.

	// So let's inject a pipeline state message into the pipeline!
	msg := &messages.Message{
		PipelineID: pl.ID(),
		Type:       messages.MessageTypePipelineState,
		Data:       []byte{}, // Empty data for now, pipeline handleMessage just transitions to Completed
	}

	// We can call HandleMessage on the pipeline directly?
	// Wait, `Pipeline.HandleMessage` is public? Yes.

	err = pl.HandleMessage(msg)
	if err != nil {
		t.Fatalf("HandleMessage failed: %v", err)
	}

	// 4. Wait for cleanup
	// The cleanup happens in a goroutine: `go func() { pl.Wait(); p.removePipeline() }`
	// We need to wait a bit for scheduler.
	time.Sleep(200 * time.Millisecond)

	// 5. Verify removal from map
	pool.mu.RLock()
	defer pool.mu.RUnlock()
	if _, ok := pool.pipelines[pl.ID()]; ok {
		t.Error("Pipeline was NOT removed from map after completion")
	}
}

func TestRunspacePool_ContextLifecycle(t *testing.T) {
	// Verify that the dispatch loop context is tied to pool.Close() and not Open(ctx)

	pool := New(&MockReadWriter{}, uuid.New())
	// Manually set to Opened
	pool.state = StateOpened

	// Start dispatch loop manually
	// dispatchLoop uses p.ctx which is created in New()
	go pool.dispatchLoop(pool.ctx)

	// Verify it's running? Hard to see from outside without side effects.
	// But we can check if Close() cancels the context.

	pool.Close(context.Background())

	// Wait for context cancellation
	select {
	case <-pool.ctx.Done():
		// Success
	case <-time.After(1 * time.Second):
		t.Error("Pool context was not cancelled after Close()")
	}
}
