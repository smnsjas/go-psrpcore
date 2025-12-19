# RunspacePool State Machine Implementation

This package implements the PSRP RunspacePool state machine following the MS-PSRP protocol specification.

## Features

### State Machine
- **BeforeOpen** → **Opening** → **Opened** → **Closing** → **Closed**
- Can transition to **Broken** from any state on error
- Thread-safe state transitions with mutex protection
- State validation on all operations

### Message Exchange
- **SESSION_CAPABILITY**: Protocol version and capability negotiation
- **INIT_RUNSPACEPOOL**: Pool initialization with min/max runspaces
- **RUNSPACEPOOL_STATE**: State change notifications from server

### Configuration
- Configurable minimum and maximum runspaces
- Settings must be configured before opening the pool
- Default: 1 min, 1 max runspaces

### Error Handling
- `ErrInvalidState`: Operation not allowed in current state
- `ErrAlreadyOpen`: Attempting to open already open pool
- `ErrNotOpen`: Operation requires open pool
- `ErrClosed`: Operation on closed pool
- `ErrBroken`: Pool in failed state
- Automatic transition to Broken state on errors

## State Transitions

```
┌─────────────┐
│ BeforeOpen  │  ← Initial state
└──────┬──────┘
       │ Open()
       ▼
┌─────────────┐
│  Opening    │  ← Capability exchange in progress
└──────┬──────┘
       │ Server responds
       ▼
┌─────────────┐
│   Opened    │  ← Ready for pipeline execution
└──────┬──────┘
       │ Close()
       ▼
┌─────────────┐
│  Closing    │  ← Cleanup in progress
└──────┬──────┘
       │ Cleanup complete
       ▼
┌─────────────┐
│   Closed    │  ← Final state
└─────────────┘

Any state can transition to:
┌─────────────┐
│   Broken    │  ← Error occurred
└─────────────┘
```

## Message Flow

### Opening Sequence

```
Client                          Server
  │                               │
  ├─── SESSION_CAPABILITY ───────>│  Protocol version, capabilities
  │<── SESSION_CAPABILITY ────────┤  Server capabilities
  │                               │
  ├─── INIT_RUNSPACEPOOL ────────>│  Min/max runspaces, config
  │<── RUNSPACEPOOL_STATE ────────┤  State = Opened
  │                               │
```

## Usage

```go
// Create pool
transport := createYourTransport() // io.ReadWriter
pool := runspace.New(transport, uuid.New())

// Configure (before opening)
pool.SetMinRunspaces(1)
pool.SetMaxRunspaces(5)

// Open pool
ctx := context.Background()
if err := pool.Open(ctx); err != nil {
    log.Fatal(err)
}
defer pool.Close(ctx)

// Check state
if pool.State() == runspace.StateOpened {
    // Ready to execute pipelines
}
```

## Host Callback Support

The RunspacePool automatically handles host callbacks during initialization and operation. By default, it uses a `NullHost` that provides safe defaults for non-interactive scenarios.

### Interactive Sessions

For interactive sessions that require user input (prompts, credentials, etc.), provide a custom `Host` implementation:

```go
import "github.com/jasonmfehr/go-psrp/host"

// Create pool
pool := runspace.New(transport, uuid.New())

// Set custom host for interactive sessions
pool.SetHost(myCustomHost)

// Open pool - host callbacks will be automatically handled
ctx := context.Background()
if err := pool.Open(ctx); err != nil {
    log.Fatal(err)
}
defer pool.Close(ctx)
```

When the server sends `RUNSPACEPOOL_HOST_CALL` messages (e.g., for `Read-Host` or `Get-Credential` in PowerShell scripts), the pool automatically:

1. Decodes the host call message
2. Dispatches to the appropriate `Host.UI()` method
3. Encodes and sends the response
4. Continues processing the next message

See the [host package README](../host/README.md) for details on implementing custom hosts.

## Test Coverage

Current coverage: **85.5%**

Tests cover:
- State transitions and validation
- Message exchange sequences
- Error conditions
- Thread safety
- Context cancellation
- Idempotent operations

## Implementation Details

### Sans-IO Pattern
The implementation follows the sans-IO pattern:
- No I/O logic in protocol code
- Transport provided by consumer
- Focuses purely on protocol state machine

### Message Fragmentation
- Uses [fragments](../fragments) package for fragmentation/reassembly
- Configurable max fragment size (default: 32KB)
- Automatic message reassembly from fragments

### Thread Safety
- All public methods are thread-safe
- Internal mutex protects state transitions
- Non-blocking state notifications via channels

## Reference

- **Protocol Spec**: [MS-PSRP](https://docs.microsoft.com/en-us/openspecs/windows_protocols/ms-psrp/)
- **Reference Implementation**: [psrpcore](https://github.com/jborean93/psrpcore) (Python)
