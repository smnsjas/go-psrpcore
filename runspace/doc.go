// Package runspace implements the PSRP RunspacePool state machine and lifecycle management.
//
// # Overview
//
// A RunspacePool represents a pool of PowerShell runspaces on a remote server. It manages
// the protocol state machine, message exchange, and provides a high-level API for creating
// and managing PowerShell execution environments.
//
// # State Machine
//
// The RunspacePool follows a strict state machine with the following states:
//
//	BeforeOpen: Initial state, pool not yet connected
//	  │
//	  ├─→ Opening: Capability exchange and initialization in progress
//	  │     │
//	  │     ├─→ Opened: Pool is ready, can execute pipelines
//	  │     │     │
//	  │     │     ├─→ Closing: Close requested, cleanup in progress
//	  │     │     │     │
//	  │     │     │     └─→ Closed: Pool closed, cannot be reopened
//	  │     │     │
//	  │     │     └─→ Broken: Error occurred during operation
//	  │     │
//	  │     └─→ Broken: Error during opening
//	  │
//	  └─→ Broken: Can transition to Broken from any state
//
// # Opening Sequence
//
// When Open() is called, the following message exchange occurs:
//
//  1. Client → Server: SESSION_CAPABILITY (protocol version, capabilities)
//  2. Server → Client: SESSION_CAPABILITY (server capabilities)
//  3. Client → Server: INIT_RUNSPACEPOOL (min/max runspaces, configuration)
//  4. Server → Client: RUNSPACEPOOL_STATE (Opened status)
//
// After successful completion, the state transitions from BeforeOpen → Opening → Opened.
//
// # Usage Example
//
//	// Create transport (e.g., WSMan connection, SSH channel, etc.)
//	transport := createTransport() // Your transport implementation
//
//	// Create a new runspace pool
//	poolID := uuid.New()
//	pool := runspace.New(transport, poolID)
//
//	// Configure pool settings (must be done before Open)
//	if err := pool.SetMinRunspaces(1); err != nil {
//	    log.Fatal(err)
//	}
//	if err := pool.SetMaxRunspaces(5); err != nil {
//	    log.Fatal(err)
//	}
//
//	// Open the pool (performs capability exchange and initialization)
//	ctx := context.Background()
//	if err := pool.Open(ctx); err != nil {
//	    log.Fatalf("Failed to open pool: %v", err)
//	}
//	defer pool.Close(ctx)
//
//	// Pool is now in Opened state and ready for use
//	if pool.State() != runspace.StateOpened {
//	    log.Fatal("Pool not opened")
//	}
//
// # Error Handling
//
// The package defines several error types for common failure scenarios:
//
//   - ErrInvalidState: Operation attempted in invalid state
//   - ErrAlreadyOpen: Open called on already opening/opened pool
//   - ErrNotOpen: Operation requires open pool
//   - ErrClosed: Operation attempted on closed pool
//   - ErrBroken: Pool is in broken state due to error
//
// When an error occurs during Open(), the pool automatically transitions to the Broken state.
//
// # Host Callbacks
//
// The pool automatically handles host callbacks (RUNSPACEPOOL_HOST_CALL messages) from the server.
// These occur when PowerShell scripts need user interaction (e.g., Read-Host, Get-Credential).
//
// By default, the pool uses a NullHost that provides safe defaults for non-interactive scenarios.
// For interactive sessions, set a custom Host implementation before opening:
//
//	pool.SetHost(myInteractiveHost)
//
// The pool automatically dispatches host calls to the appropriate Host.UI() methods and sends
// responses back to the server. This happens transparently during Open() and other operations.
//
// See the host package documentation for details on implementing custom hosts.
//
// # Thread Safety
//
// Pool methods are thread-safe and can be called concurrently. The state machine transitions
// are protected by internal locking.
//
// # Reference
//
// MS-PSRP Protocol: https://docs.microsoft.com/en-us/openspecs/windows_protocols/ms-psrp/
package runspace
