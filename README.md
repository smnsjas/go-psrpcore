# go-psrpcore

<!-- markdownlint-disable MD013 -->
[![Go Reference](https://pkg.go.dev/badge/github.com/smnsjas/go-psrpcore.svg)](https://pkg.go.dev/github.com/smnsjas/go-psrpcore)
[![Go Report Card](https://goreportcard.com/badge/github.com/smnsjas/go-psrpcore)](https://goreportcard.com/report/github.com/smnsjas/go-psrpcore)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
<!-- markdownlint-enable MD013 -->

Pure Go implementation of the PowerShell Remoting Protocol (PSRP).

## Overview

<!-- markdownlint-disable MD013 -->
This library implements the [MS-PSRP](https://docs.microsoft.com/en-us/openspecs/windows_protocols/ms-psrp/)
protocol following the **sans-IO** pattern. It handles PSRP protocol logic
only—consumers provide their own transport layer.
<!-- markdownlint-enable MD013 -->

```text
Your Application
       │
       ▼
┌─────────────┐
│   go-psrp   │  ◄── Protocol logic (this library)
└──────┬──────┘
       │ io.ReadWriter
       ▼
┌─────────────┐
│  Transport  │  ◄── You provide: WSMan, SSH, VMBus, etc.
└─────────────┘
```

## Features

- **Transport-agnostic** - Works over any bidirectional byte stream
- **Full PSRP support** - RunspacePools, Pipelines, Host callbacks
- **Object serialization** - CLIXML encode/decode for PowerShell objects
- **SecureString support** - Encrypted credential handling
- **Streaming output** - Progress, Debug, Verbose, Warning, Error,
  Information records
- **Availability Monitoring** - Tracks available runspaces on the server via
  `RUNSPACE_AVAILABILITY` messages
- **Dynamic Transport Swap** - `SetTransport()` allows replacing the underlying
  transport for reconnection scenarios

## Installation

```bash
go get github.com/smnsjas/go-psrpcore
```

## Quick Start

```go
package main

import (
    "context"
    "fmt"
    "io"

    "github.com/smnsjas/go-psrpcore"
)

func main() {
    // You provide the transport (this example assumes you have one)
    var transport io.ReadWriter = getYourTransport()

    // Create a client
    client := psrp.NewClient(transport)

    // Open a runspace pool
    ctx := context.Background()
    pool, err := client.CreateRunspacePool(ctx)
    if err != nil {
        panic(err)
    }
    defer pool.Close(ctx)

    // Create and run a PowerShell pipeline
    ps := pool.CreatePowerShell()
    ps.AddScript("Get-Process | Select-Object -First 5 Name, Id")

    output, err := ps.Invoke(ctx)
    if err != nil {
        panic(err)
    }

    for _, obj := range output {
        fmt.Printf("%+v\n", obj)
    }
}
```

## Use Cases

This library is designed to be composed with transport implementations:

| Transport | Use Case | Example Project |
|-----------|----------|-----------------|
| **AF_HYPERV / VMBus** | PowerShell Direct to Hyper-V VMs | [go-psrp](https://github.com/smnsjas/go-psrp) |
| **WSMan / HTTP(S)** | Traditional WinRM remoting | [go-psrp](https://github.com/smnsjas/go-psrp) |
| **SSH** | PowerShell Core remoting | Use `golang.org/x/crypto/ssh` |
| **Named Pipes** | Local PowerShell remoting | Use OS-specific pipe APIs |

## Package Structure

```
go-psrpcore/
├── runspace/            # RunspacePool management
├── pipeline/            # Pipeline execution
├── messages/            # PSRP message type definitions
├── fragments/           # Message fragmentation/reassembly
├── serialization/       # CLIXML serialization
├── objects/             # PowerShell complex objects
├── host/                # Host callback interface
└── outofproc/           # OutOfProcess transport adapter (for HVSocket/SSH)
```

## Architecture

### Sans-IO Design

This library follows the [sans-IO](https://sans-io.readthedocs.io/) pattern:

- **No network code** - Protocol logic is completely separate from I/O
- **No goroutines** - Caller controls concurrency
- **Testable** - Easy to test with mock transports
- **Composable** - Use with any transport layer

### PSRP Protocol Layers

<!-- markdownlint-disable MD013 -->
```text
┌─────────────────────────────────────────┐
│           PowerShell API                │  High-level commands
├─────────────────────────────────────────┤
│           Pipeline Layer                │  Command execution
├─────────────────────────────────────────┤
│         RunspacePool Layer              │  Session management
├─────────────────────────────────────────┤
│           Message Layer                 │  41 message types
├─────────────────────────────────────────┤
│          Fragment Layer                 │  Chunking large messages
├─────────────────────────────────────────┤
│        Serialization Layer              │  CLIXML encode/decode
└─────────────────────────────────────────┘
```
<!-- markdownlint-enable MD013 -->

## Design & Performance

We maintain detailed design documentation and performance analysis in the
`docs/` directory:

<!-- markdownlint-disable MD013 -->
- [Development Journey](docs/development-journey.md) - Lessons learned building the protocol
- [Security Implementation Guide](docs/session-key-implementation-guide.md) - SecureString and encryption implementation details
<!-- markdownlint-enable MD013 -->
- [Performance Baselines](docs/BASELINE_PERFORMANCE.md) - Benchmark results

See [docs/README.md](docs/README.md) for the full index.

## Logging

 This library supports structured logging via Go's `log/slog` package (Go 1.21+).

### Environment Variables

 You can enable logging globally by setting the `PSRP_LOG_LEVEL` environment variable:

 ```bash
 export PSRP_LOG_LEVEL=info  # options: debug, info, warn, error
 ```

### Custom Logger

 You can also inject a custom `slog.Logger` into the `RunspacePool`:

 ```go
 logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
     Level: slog.LevelDebug,
 }))
 
 pool.SetSlogLogger(logger)
 ```

## Testing

This library uses a **Sans-IO** design, making it easy to test without a live
PowerShell server.

### Functional Tests

We provide a comprehensive functional test suite that simulates a PSRP server
over a mock transport. This verifies the complete handshake and execution flow.

```bash
go test -v ./runspace -run TestEndToEndFunctional
```

### Unit Tests

Run the full test suite (including sub-packages):

```bash
go test ./...
```

## Related Projects

<!-- markdownlint-disable MD013 -->
- [psrpcore](https://github.com/jborean93/psrpcore) - Python PSRP implementation (reference)
- [pypsrp](https://github.com/jborean93/pypsrp) - Python PSRP client with WSMan transport
- [go-psdirect](https://github.com/jasonmfehr/go-psdirect) - PowerShell Direct for Hyper-V (uses this library)
<!-- markdownlint-enable MD013 -->

## Contributing

Contributions are welcome! We follow standard Go project guidelines.

### Reporting Bugs

Please open an issue on GitHub with:

1. A clear description of the bug
2. Minimal reproduction steps
3. Full stack trace (if applicable)
4. Environment details (OS, Go version, PowerShell version)

### Pull Requests

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

### Coding Standards

- **Formatting**: run `go fmt ./...`
- **Linting**: We use `golangci-lint`. Ensure your code passes all linters.
- **Tests**: Add unit tests for new features. Ensure all tests pass.

### Running Tests

Run the full test suite:

```bash
go test -v ./...
```

## License

MIT License - see [LICENSE](LICENSE) for details.

## Acknowledgements

This library references the [pypsrp](https://github.com/jborean93/pypsrp) and
[psrpcore](https://github.com/jborean93/psrpcore) projects by
[Jordan Borean](https://github.com/jborean93). These implementations served as
the primary reference for the protocol logic.
