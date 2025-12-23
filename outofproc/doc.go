// Package outofproc implements the OutOfProcess transport framing for PSRP.
//
// This transport is used by:
//   - pwsh -ServerMode PSRP (stdin/stdout)
//   - SSH-based PowerShell remoting (pwsh subsystem)
//   - Named pipe connections
//
// # Protocol Overview
//
// The OutOfProcess protocol wraps PSRP fragments in XML elements and base64-encodes
// the binary data. Each packet is a single line of XML terminated by a newline.
//
// # Packet Types
//
// The protocol defines the following packet types:
//
//	<Data Stream='Default' PSGuid='guid'>base64</Data>     - Fragment data
//	<DataAck PSGuid='guid' />                              - Data acknowledgment
//	<Command PSGuid='pipeline-guid' />                     - Create pipeline
//	<CommandAck PSGuid='pipeline-guid' />                  - Pipeline created
//	<Close PSGuid='guid' />                                - Close request
//	<CloseAck PSGuid='guid' />                             - Close acknowledgment
//	<Signal PSGuid='guid' />                               - Signal (e.g., stop)
//	<SignalAck PSGuid='guid' />                            - Signal acknowledgment
//
// # Usage
//
// The Transport type wraps an io.ReadWriter (typically stdin/stdout of a child
// process) and provides methods for sending and receiving OutOfProcess packets.
//
// Example with pwsh -ServerMode:
//
//	cmd := exec.Command("pwsh", "-ServerMode", "PSRP", "-NoLogo", "-NoProfile")
//	stdin, _ := cmd.StdinPipe()
//	stdout, _ := cmd.StdoutPipe()
//	cmd.Start()
//
//	transport := outofproc.NewTransport(stdout, stdin)
//	runspaceID := uuid.New()
//
//	// Send initial session capability and init runspacepool fragments
//	transport.SendData(outofproc.NullGUID, fragmentData)
//
//	// Receive response
//	packet, _ := transport.ReceivePacket()
//
// # Reference
//
// The OutOfProcess protocol is implemented in PowerShell's OutOfProcTransportManager.cs
// and OutOfProcessUtils.cs. It is not formally documented in MS-PSRP.
package outofproc
