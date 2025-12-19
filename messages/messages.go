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
const (
	MessageTypeSessionCapability     MessageType = 0x00010002
	MessageTypeInitRunspacePool      MessageType = 0x00010004
	MessageTypePublicKey             MessageType = 0x00010005
	MessageTypeEncryptedSessionKey   MessageType = 0x00010006
	MessageTypePublicKeyRequest      MessageType = 0x00010007
	MessageTypeSetMaxRunspaces       MessageType = 0x00010008
	MessageTypeSetMinRunspaces       MessageType = 0x00010009
	MessageTypeGetAvailableRunspaces MessageType = 0x0001000A
	MessageTypeUserEvent             MessageType = 0x0001000B
	MessageTypeConnectRunspacePool   MessageType = 0x0001000C
	MessageTypeRunspacePoolInitData  MessageType = 0x0001000D
	MessageTypeRunspaceAvailability  MessageType = 0x00010010
	MessageTypeRunspacePoolState     MessageType = 0x00010011
	MessageTypeApplicationPrivate    MessageType = 0x00010021
	MessageTypeGetCommandMetadata    MessageType = 0x00010022
)

// Pipeline message types.
const (
	MessageTypeCreatePipeline          MessageType = 0x00021002
	MessageTypeGetCommandMetadataReply MessageType = 0x00021003
	MessageTypeRunspaceHostCall        MessageType = 0x00021004
	MessageTypeRunspaceHostResponse    MessageType = 0x00021005
	MessageTypePipelineInput           MessageType = 0x00021006
	MessageTypeEndOfPipelineInput      MessageType = 0x00021007
	MessageTypePipelineOutput          MessageType = 0x00021008
	MessageTypeErrorRecord             MessageType = 0x00021009
	MessageTypePipelineState           MessageType = 0x0002100A
	MessageTypeDebugRecord             MessageType = 0x00021010
	MessageTypeVerboseRecord           MessageType = 0x00021011
	MessageTypeWarningRecord           MessageType = 0x00021012
	MessageTypeProgressRecord          MessageType = 0x00021013
	MessageTypeInformationRecord       MessageType = 0x00021014
	MessageTypePipelineHostCall        MessageType = 0x00021100
	MessageTypePipelineHostResponse    MessageType = 0x00021101
)

// Message header size in bytes.
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
// All fields are little-endian. UUIDs are stored in little-endian byte order.
func (m *Message) Encode() ([]byte, error) {
	buf := make([]byte, HeaderSize+len(m.Data))

	// Destination (4 bytes, little-endian)
	binary.LittleEndian.PutUint32(buf[0:4], uint32(m.Destination))

	// MessageType (4 bytes, little-endian)
	binary.LittleEndian.PutUint32(buf[4:8], uint32(m.Type))

	// RunspacePool ID (16 bytes, little-endian UUID)
	rpidBytes, err := uuidToLittleEndianBytes(m.RunspaceID)
	if err != nil {
		return nil, fmt.Errorf("encode RPID: %w", err)
	}
	copy(buf[8:24], rpidBytes)

	// Pipeline ID (16 bytes, little-endian UUID)
	pidBytes, err := uuidToLittleEndianBytes(m.PipelineID)
	if err != nil {
		return nil, fmt.Errorf("encode PID: %w", err)
	}
	copy(buf[24:40], pidBytes)

	// Data (variable length)
	copy(buf[40:], m.Data)

	return buf, nil
}

// Decode deserializes a message from bytes.
func Decode(data []byte) (*Message, error) {
	if len(data) < HeaderSize {
		return nil, fmt.Errorf("%w: got %d bytes, need at least %d", ErrMessageTooShort, len(data), HeaderSize)
	}

	m := &Message{}

	// Destination (4 bytes, little-endian)
	m.Destination = Destination(binary.LittleEndian.Uint32(data[0:4]))

	// MessageType (4 bytes, little-endian)
	m.Type = MessageType(binary.LittleEndian.Uint32(data[4:8]))

	// RunspacePool ID (16 bytes, little-endian UUID)
	rpid, err := uuidFromLittleEndianBytes(data[8:24])
	if err != nil {
		return nil, fmt.Errorf("decode RPID: %w", err)
	}
	m.RunspaceID = rpid

	// Pipeline ID (16 bytes, little-endian UUID)
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
func uuidToLittleEndianBytes(u uuid.UUID) ([]byte, error) {
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

	return b, nil
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
func NewRunspacePoolStateMessage(runspaceID uuid.UUID, state RunspacePoolState, data []byte) *Message {
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

// PipelineState represents the state of a pipeline.
type PipelineState int32

const (
	PipelineStateNotStarted   PipelineState = 0
	PipelineStateRunning      PipelineState = 1
	PipelineStateStopping     PipelineState = 2
	PipelineStateStopped      PipelineState = 3
	PipelineStateCompleted    PipelineState = 4
	PipelineStateFailed       PipelineState = 5
	PipelineStateDisconnected PipelineState = 6
)

// NewPipelineState creates a PIPELINE_STATE message.
func NewPipelineState(runspaceID, pipelineID uuid.UUID, state PipelineState, data []byte) *Message {
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
