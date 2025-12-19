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
// # Usage
//
//	pipeline, err := pool.CreatePipeline("Get-Process")
//	if err != nil {
//	    return err
//	}
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
