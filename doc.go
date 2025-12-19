// Package psrp provides a pure Go implementation of the PowerShell Remoting Protocol.
//
// This library implements MS-PSRP following the sans-IO pattern - it handles protocol
// logic only, with no transport or I/O code. Consumers provide their own io.ReadWriter
// for the underlying transport (WSMan, SSH, VMBus, etc.).
//
// # Architecture
//
// The library is organized into layers:
//
//   - Client/RunspacePool/PowerShell: High-level API for executing commands
//   - messages: PSRP message type definitions and encoding
//   - fragments: Message fragmentation and reassembly
//   - serialization: CLIXML serialization/deserialization
//   - objects: PowerShell complex objects (PSCredential, SecureString, etc.)
//   - host: Host callback interface for interactive sessions
//
// # Basic Usage
//
//	// Create a client with your transport
//	client := psrp.NewClient(transport)
//
//	// Open a runspace pool
//	pool, err := client.CreateRunspacePool(ctx)
//	if err != nil {
//	    return err
//	}
//	defer pool.Close(ctx)
//
//	// Execute PowerShell
//	ps := pool.CreatePowerShell()
//	ps.AddScript("Get-Process")
//	output, err := ps.Invoke(ctx)
//
// # Transport Agnostic
//
// This library does not include any transport code. You must provide an io.ReadWriter
// that handles the underlying communication. Common transports include:
//
//   - WSMan/HTTP(S): Traditional WinRM remoting
//   - SSH: PowerShell Core cross-platform remoting
//   - AF_HYPERV/VMBus: PowerShell Direct for Hyper-V VMs
//   - Named Pipes: Local PowerShell remoting
//
// # Reference
//
// Protocol specification: https://docs.microsoft.com/en-us/openspecs/windows_protocols/ms-psrp/
package psrp

// Version is the library version.
const Version = "0.1.0-dev"
