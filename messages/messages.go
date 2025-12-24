// Package messages defines the 41 PSRP message types and their encoding/decoding.
//
// PSRP messages are the core unit of communication in the protocol. Each message
// has a type, destination (client or server), and associated runspace/pipeline IDs.
//
// # Message Structure
//
// All PSRP messages share a common header:
//
//	┌─────────────────────────────────────────────────────────┐
//	│  Destination (4 bytes) - 1=Client, 2=Server            │
//	├─────────────────────────────────────────────────────────┤
//	│  MessageType (4 bytes)                                  │
//	├─────────────────────────────────────────────────────────┤
//	│  RPID (16 bytes) - RunspacePool ID (GUID)              │
//	├─────────────────────────────────────────────────────────┤
//	│  PID (16 bytes) - Pipeline ID (GUID)                   │
//	├─────────────────────────────────────────────────────────┤
//	│  Data (variable) - CLIXML encoded payload              │
//	└─────────────────────────────────────────────────────────┘
//
// # Byte Order (Endianness)
//
// All multi-byte integer fields in PSRP message headers use LITTLE-ENDIAN byte order.
// This follows the .NET GUID/structure serialization convention used by PowerShell.
//
// While MS-PSRP does not explicitly document the byte order for message headers,
// several factors confirm little-endian is correct:
//
//   - MS-PSRP Section 2.2.5.2 explicitly specifies little-endian for PUBLIC_KEY message
//   - .NET's System.Guid serialization uses little-endian (mixed-endian) format
//   - PowerShell is primarily a Windows/.NET technology where little-endian is standard
//   - The reference Python implementation (psrpcore) uses little-endian throughout
//
// GUIDs (RPID/PID) use the .NET GUID serialization format (RFC 4122 mixed-endian):
//   - First 3 components (time-low, time-mid, time-hi) are little-endian
//   - Last 2 components (clock-seq, node) are big-endian
//
// IMPORTANT: This differs from fragment headers, which use big-endian (MS-PSRP 2.2.4).
// The endianness difference exists because:
//   - Fragments are the transport layer (network-facing) - BIG-ENDIAN
//   - Messages are the application layer (.NET/Windows-facing) - LITTLE-ENDIAN
//
// Reference: MS-PSRP Section 2.2.1 (Messages), Section 2.2.5.2 (PUBLIC_KEY)
//
// # Message Categories
//
// Messages are grouped by functionality:
//
//   - Session messages: Capability exchange, key negotiation
//   - Runspace messages: Pool creation, state changes
//   - Pipeline messages: Command execution, output streaming
//   - Host messages: Interactive callbacks to the client
//
// # Reference
//
// MS-PSRP Section 2.2.1: https://docs.microsoft.com/en-us/openspecs/windows_protocols/ms-psrp/
package messages

import (
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/google/uuid"
)

// Destination indicates whether a message is for the client or server.
type Destination uint32

const (
	// DestinationClient indicates the message is for the client.
	DestinationClient Destination = 1
	// DestinationServer indicates the message is for the server.
	DestinationServer Destination = 2
)

// MessageType identifies the type of PSRP message.
type MessageType uint32

// Session and capability message types.
// Reference: MS-PSRP Section 2.2.1
const (
	// Session capability exchange - MS-PSRP 2.2.5.1
	MessageTypeSessionCapability MessageType = 0x00010002
	// Runspace pool initialization - MS-PSRP 2.2.2.1
	MessageTypeInitRunspacePool MessageType = 0x00010004
	// Public key for encryption - MS-PSRP 2.2.5.2
	MessageTypePublicKey MessageType = 0x00010005
	// Encrypted session key - MS-PSRP 2.2.5.3
	MessageTypeEncryptedSessionKey MessageType = 0x00010006
	// Request for public key - MS-PSRP 2.2.5.4
	MessageTypePublicKeyRequest MessageType = 0x00010007
	// Connect to existing runspace pool - MS-PSRP 2.2.2.9
	MessageTypeConnectRunspacePool MessageType = 0x00010008
	// Runspace pool state change - MS-PSRP 2.2.2.2
	MessageTypeRunspacePoolState MessageType = 0x00021005
)

// Runspace pool management message types.
// Reference: MS-PSRP Section 2.2.2
const (
	// Set maximum runspaces - MS-PSRP 2.2.2.3
	MessageTypeSetMaxRunspaces MessageType = 0x00021002
	// Set minimum runspaces - MS-PSRP 2.2.2.4
	MessageTypeSetMinRunspaces MessageType = 0x00021003
	// Runspace availability notification - MS-PSRP 2.2.2.5
	MessageTypeRunspaceAvailability MessageType = 0x00021004
	// Get available runspaces - MS-PSRP 2.2.2.6
	MessageTypeGetAvailableRunspaces MessageType = 0x00021007
	// User event - MS-PSRP 2.2.2.7
	MessageTypeUserEvent MessageType = 0x00021008
	// Application private data - MS-PSRP 2.2.2.8
	MessageTypeApplicationPrivate MessageType = 0x00021009
	// Get command metadata - MS-PSRP 2.2.3.1
	MessageTypeGetCommandMetadata MessageType = 0x0002100A
	// Runspace pool initialization data - MS-PSRP 2.2.2.10
	MessageTypeRunspacePoolInitData MessageType = 0x0002100B
	// Reset runspace state - MS-PSRP 2.2.2.11
	MessageTypeResetRunspaceState MessageType = 0x0002100C
)

// Host callback message types.
// Reference: MS-PSRP Section 2.2.4
const (
	// Runspace pool host call - MS-PSRP 2.2.4.1
	MessageTypeRunspaceHostCall MessageType = 0x00021100
	// Runspace pool host response - MS-PSRP 2.2.4.2
	MessageTypeRunspaceHostResponse MessageType = 0x00021101
)

// Pipeline message types.
// Reference: MS-PSRP Section 2.2.3
const (
	// Create pipeline - MS-PSRP 2.2.3.2
	MessageTypeCreatePipeline MessageType = 0x00021006
	// Signal pipeline (stop/interrupt) - MS-PSRP 2.2.3.13
	MessageTypeSignal MessageType = 0x00041001
	// Pipeline input data - MS-PSRP 2.2.3.3
	MessageTypePipelineInput MessageType = 0x00041002
	// End of pipeline input - MS-PSRP 2.2.3.4
	MessageTypeEndOfPipelineInput MessageType = 0x00041003
	// Pipeline output data - MS-PSRP 2.2.3.5
	MessageTypePipelineOutput MessageType = 0x00041004
	// Error record - MS-PSRP 2.2.3.6
	MessageTypeErrorRecord MessageType = 0x00041005
	// Pipeline state - MS-PSRP 2.2.3.7
	MessageTypePipelineState MessageType = 0x00041006
	// Debug record - MS-PSRP 2.2.3.8
	MessageTypeDebugRecord MessageType = 0x00041007
	// Verbose record - MS-PSRP 2.2.3.9
	MessageTypeVerboseRecord MessageType = 0x00041008
	// Warning record - MS-PSRP 2.2.3.10
	MessageTypeWarningRecord MessageType = 0x00041009
	// Progress record - MS-PSRP 2.2.3.11
	MessageTypeProgressRecord MessageType = 0x00041010
	// Information record - MS-PSRP 2.2.3.12
	MessageTypeInformationRecord MessageType = 0x00041011
	// Pipeline host call - MS-PSRP 2.2.4.3
	MessageTypePipelineHostCall MessageType = 0x00041100
	// Pipeline host response - MS-PSRP 2.2.4.4
	MessageTypePipelineHostResponse MessageType = 0x00041101
)

// HeaderSize is the message header size in bytes.
const HeaderSize = 40 // 4 (Destination) + 4 (MessageType) + 16 (RPID) + 16 (PID)

var (
	// ErrInvalidMessage is returned when a message cannot be decoded.
	ErrInvalidMessage = errors.New("invalid PSRP message")
	// ErrMessageTooShort is returned when message is smaller than header size.
	ErrMessageTooShort = errors.New("message too short")
)

// Message represents a PSRP message.
type Message struct {
	Destination Destination
	Type        MessageType
	RunspaceID  uuid.UUID
	PipelineID  uuid.UUID
	Data        []byte // CLIXML encoded
}

// Encode serializes the message to bytes.
// Format: Destination (4) + MessageType (4) + RPID (16) + PID (16) + Data
// All fields are little-endian per .NET serialization conventions.
// Reference: MS-PSRP Section 2.2.1, Section 2.2.5.2
func (m *Message) Encode() ([]byte, error) {
	buf := make([]byte, HeaderSize+len(m.Data))

	// Destination (4 bytes, little-endian) - MS-PSRP 2.2.1
	binary.LittleEndian.PutUint32(buf[0:4], uint32(m.Destination))

	// MessageType (4 bytes, little-endian) - MS-PSRP 2.2.1
	binary.LittleEndian.PutUint32(buf[4:8], uint32(m.Type))

	// RunspacePool ID (16 bytes, .NET GUID format) - MS-PSRP 2.2.1
	rpidBytes := uuidToLittleEndianBytes(m.RunspaceID)
	copy(buf[8:24], rpidBytes)

	// Pipeline ID (16 bytes, .NET GUID format) - MS-PSRP 2.2.1
	pidBytes := uuidToLittleEndianBytes(m.PipelineID)
	copy(buf[24:40], pidBytes)

	// Data (variable length, CLIXML encoded)
	copy(buf[40:], m.Data)

	return buf, nil
}

// Decode deserializes a message from bytes.
// All fields are little-endian per .NET serialization conventions.
// Reference: MS-PSRP Section 2.2.1, Section 2.2.5.2
func Decode(data []byte) (*Message, error) {
	if len(data) < HeaderSize {
		return nil, fmt.Errorf("%w: got %d bytes, need at least %d", ErrMessageTooShort, len(data), HeaderSize)
	}

	m := &Message{}

	// Destination (4 bytes, little-endian) - MS-PSRP 2.2.1
	m.Destination = Destination(binary.LittleEndian.Uint32(data[0:4]))

	// MessageType (4 bytes, little-endian) - MS-PSRP 2.2.1
	m.Type = MessageType(binary.LittleEndian.Uint32(data[4:8]))

	// RunspacePool ID (16 bytes, .NET GUID format) - MS-PSRP 2.2.1
	rpid, err := uuidFromLittleEndianBytes(data[8:24])
	if err != nil {
		return nil, fmt.Errorf("decode RPID: %w", err)
	}
	m.RunspaceID = rpid

	// Pipeline ID (16 bytes, .NET GUID format) - MS-PSRP 2.2.1
	pid, err := uuidFromLittleEndianBytes(data[24:40])
	if err != nil {
		return nil, fmt.Errorf("decode PID: %w", err)
	}
	m.PipelineID = pid

	// Data (remaining bytes)
	if len(data) > HeaderSize {
		m.Data = make([]byte, len(data)-HeaderSize)
		copy(m.Data, data[HeaderSize:])
	}

	return m, nil
}

// uuidToLittleEndianBytes converts a UUID to little-endian byte representation.
// .NET stores GUIDs in little-endian format, so we need to swap the byte order
// for the first three components while keeping the last two as-is.
func uuidToLittleEndianBytes(u uuid.UUID) []byte {
	b := make([]byte, 16)
	ub := u[:]

	// Time-low (4 bytes) - reverse for little-endian
	b[0], b[1], b[2], b[3] = ub[3], ub[2], ub[1], ub[0]

	// Time-mid (2 bytes) - reverse for little-endian
	b[4], b[5] = ub[5], ub[4]

	// Time-hi-and-version (2 bytes) - reverse for little-endian
	b[6], b[7] = ub[7], ub[6]

	// Clock-seq and node (8 bytes) - keep as-is (already in correct order)
	copy(b[8:], ub[8:])

	return b
}

// uuidFromLittleEndianBytes converts little-endian bytes to a UUID.
func uuidFromLittleEndianBytes(b []byte) (uuid.UUID, error) {
	if len(b) != 16 {
		return uuid.Nil, fmt.Errorf("invalid UUID bytes: expected 16, got %d", len(b))
	}

	var u uuid.UUID

	// Time-low (4 bytes) - reverse from little-endian
	u[0], u[1], u[2], u[3] = b[3], b[2], b[1], b[0]

	// Time-mid (2 bytes) - reverse from little-endian
	u[4], u[5] = b[5], b[4]

	// Time-hi-and-version (2 bytes) - reverse from little-endian
	u[6], u[7] = b[7], b[6]

	// Clock-seq and node (8 bytes) - keep as-is
	copy(u[8:], b[8:])

	return u, nil
}

// Helper functions for creating specific message types

// NewSessionCapability creates a SESSION_CAPABILITY message.
// capabilities should be a serialized PSObject with protocol version and other capabilities.
func NewSessionCapability(runspaceID uuid.UUID, capabilities []byte) *Message {
	return &Message{
		Destination: DestinationServer,
		Type:        MessageTypeSessionCapability,
		RunspaceID:  runspaceID,
		PipelineID:  uuid.Nil,
		Data:        capabilities,
	}
}

// NewInitRunspacePool creates an INIT_RUNSPACEPOOL message.
// data should contain minRunspaces and maxRunspaces as CLIXML.
func NewInitRunspacePool(runspaceID uuid.UUID, data []byte) *Message {
	return &Message{
		Destination: DestinationServer,
		Type:        MessageTypeInitRunspacePool,
		RunspaceID:  runspaceID,
		PipelineID:  uuid.Nil,
		Data:        data,
	}
}

// RunspacePoolState represents the state of a runspace pool.
type RunspacePoolState int32

// RunspacePoolState constants represent the state of a runspace pool.
const (
	RunspacePoolStateBeforeOpen   RunspacePoolState = 0
	RunspacePoolStateOpening      RunspacePoolState = 1
	RunspacePoolStateOpened       RunspacePoolState = 2
	RunspacePoolStateClosing      RunspacePoolState = 3
	RunspacePoolStateClosed       RunspacePoolState = 4
	RunspacePoolStateBroken       RunspacePoolState = 5
	RunspacePoolStateDisconnected RunspacePoolState = 6
	RunspacePoolStateConnecting   RunspacePoolState = 7
)

// NewRunspacePoolStateMessage creates a RUNSPACEPOOL_STATE message.
func NewRunspacePoolStateMessage(runspaceID uuid.UUID, _ RunspacePoolState, data []byte) *Message {
	return &Message{
		Destination: DestinationClient,
		Type:        MessageTypeRunspacePoolState,
		RunspaceID:  runspaceID,
		PipelineID:  uuid.Nil,
		Data:        data,
	}
}

// NewCreatePipeline creates a CREATE_PIPELINE message.
// data should contain the command/script to execute as CLIXML.
func NewCreatePipeline(runspaceID, pipelineID uuid.UUID, data []byte) *Message {
	return &Message{
		Destination: DestinationServer,
		Type:        MessageTypeCreatePipeline,
		RunspaceID:  runspaceID,
		PipelineID:  pipelineID,
		Data:        data,
	}
}

// NewPipelineOutput creates a PIPELINE_OUTPUT message.
func NewPipelineOutput(runspaceID, pipelineID uuid.UUID, data []byte) *Message {
	return &Message{
		Destination: DestinationClient,
		Type:        MessageTypePipelineOutput,
		RunspaceID:  runspaceID,
		PipelineID:  pipelineID,
		Data:        data,
	}
}

// NewGetCommandMetadata creates a GET_COMMAND_METADATA message.
func NewGetCommandMetadata(runspaceID uuid.UUID, data []byte) *Message {
	return &Message{
		Destination: DestinationServer,
		Type:        MessageTypeGetCommandMetadata,
		RunspaceID:  runspaceID,
		PipelineID:  uuid.Nil,
		Data:        data,
	}
}

// PipelineState represents the state of a pipeline.
// These values correspond to the PSInvocationState enum defined in MS-PSRP Section 2.2.3.9.
type PipelineState int32

const (
	// PipelineStateNotStarted indicates the pipeline has not been invoked yet.
	// MS-PSRP Section 2.2.3.9: PSInvocationState value 0.
	PipelineStateNotStarted PipelineState = 0
	// PipelineStateRunning indicates the pipeline is currently executing.
	// MS-PSRP Section 2.2.3.9: PSInvocationState value 1.
	PipelineStateRunning PipelineState = 1
	// PipelineStateStopping indicates the pipeline is in the process of stopping.
	// MS-PSRP Section 2.2.3.9: PSInvocationState value 2.
	PipelineStateStopping PipelineState = 2
	// PipelineStateStopped indicates the pipeline has been stopped.
	// MS-PSRP Section 2.2.3.9: PSInvocationState value 3.
	PipelineStateStopped PipelineState = 3
	// PipelineStateCompleted indicates the pipeline completed successfully.
	// MS-PSRP Section 2.2.3.9: PSInvocationState value 4.
	PipelineStateCompleted PipelineState = 4
	// PipelineStateFailed indicates the pipeline failed with an error.
	// MS-PSRP Section 2.2.3.9: PSInvocationState value 5.
	PipelineStateFailed PipelineState = 5
	// PipelineStateDisconnected indicates the pipeline is in disconnected state.
	// MS-PSRP Section 2.2.3.9: PSInvocationState value 6.
	PipelineStateDisconnected PipelineState = 6
)

// NewPipelineState creates a PIPELINE_STATE message.
func NewPipelineState(runspaceID, pipelineID uuid.UUID, _ PipelineState, data []byte) *Message {
	return &Message{
		Destination: DestinationClient,
		Type:        MessageTypePipelineState,
		RunspaceID:  runspaceID,
		PipelineID:  pipelineID,
		Data:        data,
	}
}

// NewErrorRecord creates an ERROR_RECORD message.
func NewErrorRecord(runspaceID, pipelineID uuid.UUID, errorData []byte) *Message {
	return &Message{
		Destination: DestinationClient,
		Type:        MessageTypeErrorRecord,
		RunspaceID:  runspaceID,
		PipelineID:  pipelineID,
		Data:        errorData,
	}
}

// NewPipelineInput creates a PIPELINE_INPUT message.
func NewPipelineInput(runspaceID, pipelineID uuid.UUID, inputData []byte) *Message {
	return &Message{
		Destination: DestinationServer,
		Type:        MessageTypePipelineInput,
		RunspaceID:  runspaceID,
		PipelineID:  pipelineID,
		Data:        inputData,
	}
}

// NewEndOfPipelineInput creates an END_OF_PIPELINE_INPUT message.
func NewEndOfPipelineInput(runspaceID, pipelineID uuid.UUID) *Message {
	return &Message{
		Destination: DestinationServer,
		Type:        MessageTypeEndOfPipelineInput,
		RunspaceID:  runspaceID,
		PipelineID:  pipelineID,
		Data:        nil,
	}
}

// NewRunspaceHostCall creates a RUNSPACEPOOL_HOST_CALL message.
// data should contain the serialized RemoteHostCall.
func NewRunspaceHostCall(runspaceID uuid.UUID, data []byte) *Message {
	return &Message{
		Destination: DestinationClient,
		Type:        MessageTypeRunspaceHostCall,
		RunspaceID:  runspaceID,
		PipelineID:  uuid.Nil,
		Data:        data,
	}
}

// NewRunspaceHostResponse creates a RUNSPACEPOOL_HOST_RESPONSE message.
// data should contain the serialized RemoteHostResponse.
func NewRunspaceHostResponse(runspaceID uuid.UUID, data []byte) *Message {
	return &Message{
		Destination: DestinationServer,
		Type:        MessageTypeRunspaceHostResponse,
		RunspaceID:  runspaceID,
		PipelineID:  uuid.Nil,
		Data:        data,
	}
}

// NewPipelineHostCall creates a PIPELINE_HOST_CALL message.
// data should contain the serialized RemoteHostCall.
func NewPipelineHostCall(runspaceID, pipelineID uuid.UUID, data []byte) *Message {
	return &Message{
		Destination: DestinationClient,
		Type:        MessageTypePipelineHostCall,
		RunspaceID:  runspaceID,
		PipelineID:  pipelineID,
		Data:        data,
	}
}

// NewPipelineHostResponse creates a PIPELINE_HOST_RESPONSE message.
// data should contain the serialized RemoteHostResponse.
func NewPipelineHostResponse(runspaceID, pipelineID uuid.UUID, data []byte) *Message {
	return &Message{
		Destination: DestinationServer,
		Type:        MessageTypePipelineHostResponse,
		RunspaceID:  runspaceID,
		PipelineID:  pipelineID,
		Data:        data,
	}
}
