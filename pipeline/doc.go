// Package pipeline implements the PSRP Pipeline state machine.
//
// A Pipeline represents a command or script to be executed on a remote RunspacePool.
// It manages the lifecycle of the execution, including input/output streams and
// state transitions.
//
// # State Machine
//
// The Pipeline follows this state transition:
//
//	NotStarted → Running → Completed
//	             ↓         ↓
//	             Stopped   Failed
//
// # Channel Buffer Management
//
// Output and error channels use buffered channels (default 100 messages) with
// timeout-based back-pressure to prevent unbounded memory growth:
//
//   - Messages are delivered immediately if the buffer has space (fast path)
//   - If the buffer is full, HandleMessage blocks for up to channelTimeout (default 5s)
//   - After timeout, HandleMessage returns ErrBufferFull
//   - Consumers should read from channels continuously to avoid buffer saturation
//   - The timeout is configurable via SetChannelTimeout()
//
// This approach provides:
//   - Back-pressure: Slow consumers naturally slow down message processing
//   - Deadlock prevention: Timeout ensures HandleMessage doesn't block indefinitely
//   - Memory bounds: Buffer size + timeout limit maximum memory usage
//
// # Usage
//
//	pipeline, err := pool.CreatePipeline("Get-Process")
//	if err != nil {
//	    return err
//	}
//
//	// Optional: Configure channel timeout
//	pipeline.SetChannelTimeout(10 * time.Second)
//
//	// Start execution (async)
//	if err := pipeline.Invoke(); err != nil {
//	    return err
//	}
//
//	// Read output
//	for output := range pipeline.Output() {
//	    fmt.Println(output)
//	}
//
//	// Wait for completion
//	pipeline.Wait()
package pipeline
