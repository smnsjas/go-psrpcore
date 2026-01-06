package messages

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"testing"

	"github.com/google/uuid"
)

func TestMessageEncodeDecodeRoundTrip(t *testing.T) {
	tests := []struct {
		name string
		msg  *Message
	}{
		{
			name: "empty message",
			msg: &Message{
				Destination: DestinationServer,
				Type:        MessageTypeSessionCapability,
				RunspaceID:  uuid.MustParse("12345678-1234-1234-1234-123456789abc"),
				PipelineID:  uuid.MustParse("87654321-4321-4321-4321-cba987654321"),
				Data:        nil,
			},
		},
		{
			name: "message with data",
			msg: &Message{
				Destination: DestinationClient,
				Type:        MessageTypePipelineOutput,
				RunspaceID:  uuid.MustParse("aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"),
				PipelineID:  uuid.MustParse("11111111-2222-3333-4444-555555555555"),
				Data:        []byte("<?xml version=\"1.0\"?><Objs></Objs>"),
			},
		},
		{
			name: "init runspace pool",
			msg: &Message{
				Destination: DestinationServer,
				Type:        MessageTypeInitRunspacePool,
				RunspaceID:  uuid.MustParse("fedcba98-7654-3210-fedc-ba9876543210"),
				PipelineID:  uuid.Nil,
				Data:        []byte("<Obj><Props><I32>1</I32><I32>5</I32></Props></Obj>"),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Encode
			encoded, err := tt.msg.Encode()
			if err != nil {
				t.Fatalf("Encode failed: %v", err)
			}

			// Verify header size
			if len(encoded) < HeaderSize {
				t.Fatalf("encoded message too short: got %d, want at least %d", len(encoded), HeaderSize)
			}

			// Decode
			decoded, err := Decode(encoded)
			if err != nil {
				t.Fatalf("Decode failed: %v", err)
			}

			// Verify fields
			if decoded.Destination != tt.msg.Destination {
				t.Errorf("Destination mismatch: got %d, want %d", decoded.Destination, tt.msg.Destination)
			}
			if decoded.Type != tt.msg.Type {
				t.Errorf("Type mismatch: got 0x%08X, want 0x%08X", decoded.Type, tt.msg.Type)
			}
			if decoded.RunspaceID != tt.msg.RunspaceID {
				t.Errorf("RunspaceID mismatch: got %s, want %s", decoded.RunspaceID, tt.msg.RunspaceID)
			}
			if decoded.PipelineID != tt.msg.PipelineID {
				t.Errorf("PipelineID mismatch: got %s, want %s", decoded.PipelineID, tt.msg.PipelineID)
			}
			if !bytes.Equal(decoded.Data, tt.msg.Data) {
				t.Errorf("Data mismatch: got %v, want %v", decoded.Data, tt.msg.Data)
			}
		})
	}
}

func TestDecodeErrors(t *testing.T) {
	tests := []struct {
		name    string
		data    []byte
		wantErr error
	}{
		{
			name:    "empty data",
			data:    []byte{},
			wantErr: ErrMessageTooShort,
		},
		{
			name:    "too short",
			data:    make([]byte, 10),
			wantErr: ErrMessageTooShort,
		},
		{
			name:    "exactly header size",
			data:    make([]byte, HeaderSize),
			wantErr: nil, // Should succeed with empty data
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Decode(tt.data)
			if tt.wantErr != nil {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if !bytes.Contains([]byte(err.Error()), []byte(tt.wantErr.Error())) {
					t.Errorf("expected error containing %q, got %q", tt.wantErr, err)
				}
			} else if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestHeaderSize(t *testing.T) {
	msg := &Message{
		Destination: DestinationServer,
		Type:        MessageTypeSessionCapability,
		RunspaceID:  uuid.New(),
		PipelineID:  uuid.New(),
		Data:        []byte("test data"),
	}

	encoded, err := msg.Encode()
	if err != nil {
		t.Fatalf("Encode failed: %v", err)
	}

	expectedSize := HeaderSize + len(msg.Data)
	if len(encoded) != expectedSize {
		t.Errorf("encoded size mismatch: got %d, want %d", len(encoded), expectedSize)
	}

	// Verify header fields are in the right positions
	if binary.LittleEndian.Uint32(encoded[0:4]) != uint32(DestinationServer) {
		t.Error("Destination not at correct position")
	}
	if binary.LittleEndian.Uint32(encoded[4:8]) != uint32(MessageTypeSessionCapability) {
		t.Error("MessageType not at correct position")
	}
}

func TestUUIDLittleEndianConversion(t *testing.T) {
	// Test UUID: 12345678-1234-1234-1234-123456789abc
	testUUID := uuid.MustParse("12345678-1234-1234-1234-123456789abc")

	// Convert to little-endian
	leBytes := uuidToLittleEndianBytes(testUUID)

	if len(leBytes) != 16 {
		t.Fatalf("expected 16 bytes, got %d", len(leBytes))
	}

	// Convert back
	converted, err := uuidFromLittleEndianBytes(leBytes)
	if err != nil {
		t.Fatalf("uuidFromLittleEndianBytes failed: %v", err)
	}

	if converted != testUUID {
		t.Errorf("UUID round-trip failed: got %s, want %s", converted, testUUID)
	}

	// Verify byte order conversion
	// For UUID 12345678-1234-1234-1234-123456789abc in big-endian:
	// Bytes: 12 34 56 78 12 34 12 34 12 34 12 34 56 78 9a bc
	// Little-endian should reverse first 3 components:
	// Expected: 78 56 34 12 34 12 34 12 12 34 12 34 56 78 9a bc
	expected := []byte{0x78, 0x56, 0x34, 0x12, 0x34, 0x12, 0x34, 0x12, 0x12, 0x34, 0x12, 0x34, 0x56, 0x78, 0x9a, 0xbc}
	if !bytes.Equal(leBytes, expected) {
		t.Errorf("UUID little-endian bytes mismatch:\ngot:  %x\nwant: %x", leBytes, expected)
	}
}

func TestMessageTypes(t *testing.T) {
	// Verify key message type constants have correct values per MS-PSRP Section 2.2.1
	tests := []struct {
		name  string
		value MessageType
		want  uint32
	}{
		// Session and initialization messages
		{"SESSION_CAPABILITY", MessageTypeSessionCapability, 0x00010002},
		{"INIT_RUNSPACEPOOL", MessageTypeInitRunspacePool, 0x00010004},
		{"PUBLIC_KEY", MessageTypePublicKey, 0x00010005},
		{"ENCRYPTED_SESSION_KEY", MessageTypeEncryptedSessionKey, 0x00010006},
		{"PUBLIC_KEY_REQUEST", MessageTypePublicKeyRequest, 0x00010007},
		{"CONNECT_RUNSPACEPOOL", MessageTypeConnectRunspacePool, 0x00010008},

		// Runspace pool management messages
		{"RUNSPACEPOOL_STATE", MessageTypeRunspacePoolState, 0x00021005},
		{"SET_MAX_RUNSPACES", MessageTypeSetMaxRunspaces, 0x00021002},
		{"SET_MIN_RUNSPACES", MessageTypeSetMinRunspaces, 0x00021003},
		{"RUNSPACE_AVAILABILITY", MessageTypeRunspaceAvailability, 0x00021004},
		{"GET_AVAILABLE_RUNSPACES", MessageTypeGetAvailableRunspaces, 0x00021007},
		{"USER_EVENT", MessageTypeUserEvent, 0x00021008},
		{"APPLICATION_PRIVATE", MessageTypeApplicationPrivate, 0x00021009},
		{"GET_COMMAND_METADATA", MessageTypeGetCommandMetadata, 0x0002100A},
		{"RUNSPACEPOOL_INIT_DATA", MessageTypeRunspacePoolInitData, 0x0002100B},
		{"RESET_RUNSPACE_STATE", MessageTypeResetRunspaceState, 0x0002100C},

		// Host callback messages
		{"RUNSPACE_HOST_CALL", MessageTypeRunspaceHostCall, 0x00021100},
		{"RUNSPACE_HOST_RESPONSE", MessageTypeRunspaceHostResponse, 0x00021101},

		// Pipeline messages
		{"CREATE_PIPELINE", MessageTypeCreatePipeline, 0x00021006},
		{"SIGNAL", MessageTypeSignal, 0x00041001},
		{"PIPELINE_INPUT", MessageTypePipelineInput, 0x00041002},
		{"END_OF_PIPELINE_INPUT", MessageTypeEndOfPipelineInput, 0x00041003},
		{"PIPELINE_OUTPUT", MessageTypePipelineOutput, 0x00041004},
		{"ERROR_RECORD", MessageTypeErrorRecord, 0x00041005},
		{"PIPELINE_STATE", MessageTypePipelineState, 0x00041006},
		{"DEBUG_RECORD", MessageTypeDebugRecord, 0x00041007},
		{"VERBOSE_RECORD", MessageTypeVerboseRecord, 0x00041008},
		{"WARNING_RECORD", MessageTypeWarningRecord, 0x00041009},
		{"PROGRESS_RECORD", MessageTypeProgressRecord, 0x00041010},
		{"INFORMATION_RECORD", MessageTypeInformationRecord, 0x00041011},
		{"PIPELINE_HOST_CALL", MessageTypePipelineHostCall, 0x00041100},
		{"PIPELINE_HOST_RESPONSE", MessageTypePipelineHostResponse, 0x00041101},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if uint32(tt.value) != tt.want {
				t.Errorf("MessageType %s: got 0x%08X, want 0x%08X", tt.name, tt.value, tt.want)
			}
		})
	}
}

func TestDestinationValues(t *testing.T) {
	if uint32(DestinationClient) != 1 {
		t.Errorf("DestinationClient: got %d, want 1", DestinationClient)
	}
	if uint32(DestinationServer) != 2 {
		t.Errorf("DestinationServer: got %d, want 2", DestinationServer)
	}
}

func BenchmarkMessageEncode(b *testing.B) {
	msg := &Message{
		Destination: DestinationServer,
		Type:        MessageTypePipelineOutput,
		RunspaceID:  uuid.New(),
		PipelineID:  uuid.New(),
		Data:        make([]byte, 1024),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := msg.Encode()
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkMessageDecode(b *testing.B) {
	msg := &Message{
		Destination: DestinationServer,
		Type:        MessageTypePipelineOutput,
		RunspaceID:  uuid.New(),
		PipelineID:  uuid.New(),
		Data:        make([]byte, 1024),
	}

	encoded, err := msg.Encode()
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := Decode(encoded)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func TestMessageHelpers(t *testing.T) {
	rpID := uuid.New()
	pID := uuid.New()
	data := []byte("test data")

	tests := []struct {
		name       string
		createFunc func() *Message
		wantType   MessageType
		wantDest   Destination
		wantRPID   uuid.UUID
		wantPID    uuid.UUID
	}{
		{
			name:       "NewSessionCapability",
			createFunc: func() *Message { return NewSessionCapability(rpID, data) },
			wantType:   MessageTypeSessionCapability,
			wantDest:   DestinationServer,
			wantRPID:   rpID,
			wantPID:    uuid.Nil,
		},
		{
			name:       "NewInitRunspacePool",
			createFunc: func() *Message { return NewInitRunspacePool(rpID, data) },
			wantType:   MessageTypeInitRunspacePool,
			wantDest:   DestinationServer,
			wantRPID:   rpID,
			wantPID:    uuid.Nil,
		},
		{
			name:       "NewRunspacePoolStateMessage",
			createFunc: func() *Message { return NewRunspacePoolStateMessage(rpID, RunspacePoolStateOpened, data) },
			wantType:   MessageTypeRunspacePoolState,
			wantDest:   DestinationClient,
			wantRPID:   rpID,
			wantPID:    uuid.Nil,
		},
		{
			name:       "NewCreatePipeline",
			createFunc: func() *Message { return NewCreatePipeline(rpID, pID, data) },
			wantType:   MessageTypeCreatePipeline,
			wantDest:   DestinationServer,
			wantRPID:   rpID,
			wantPID:    pID,
		},
		{
			name:       "NewPipelineOutput",
			createFunc: func() *Message { return NewPipelineOutput(rpID, pID, data) },
			wantType:   MessageTypePipelineOutput,
			wantDest:   DestinationClient,
			wantRPID:   rpID,
			wantPID:    pID,
		},
		{
			name:       "NewPipelineState",
			createFunc: func() *Message { return NewPipelineState(rpID, pID, PipelineStateCompleted, data) },
			wantType:   MessageTypePipelineState,
			wantDest:   DestinationClient,
			wantRPID:   rpID,
			wantPID:    pID,
		},
		{
			name:       "NewErrorRecord",
			createFunc: func() *Message { return NewErrorRecord(rpID, pID, data) },
			wantType:   MessageTypeErrorRecord,
			wantDest:   DestinationClient,
			wantRPID:   rpID,
			wantPID:    pID,
		},
		{
			name:       "NewPipelineInput",
			createFunc: func() *Message { return NewPipelineInput(rpID, pID, data) },
			wantType:   MessageTypePipelineInput,
			wantDest:   DestinationServer,
			wantRPID:   rpID,
			wantPID:    pID,
		},
		{
			name:       "NewEndOfPipelineInput",
			createFunc: func() *Message { return NewEndOfPipelineInput(rpID, pID) },
			wantType:   MessageTypeEndOfPipelineInput,
			wantDest:   DestinationServer,
			wantRPID:   rpID,
			wantPID:    pID,
		},
		{
			name: "NewGetAvailableRunspaces",
			createFunc: func() *Message {
				msg := NewGetAvailableRunspaces(rpID, 12345)
				// Verify it's a Complex Object with ci property per MS-PSRP 2.2.2.6
				expected := `<Obj RefId="0"><MS><I64 N="ci">12345</I64></MS></Obj>`
				if string(msg.Data) != expected {
					panic(fmt.Sprintf("unexpected data: %s", msg.Data))
				}
				return msg
			},
			wantType: MessageTypeGetAvailableRunspaces,
			wantDest: DestinationServer,
			wantRPID: rpID,
			wantPID:  uuid.Nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := tt.createFunc()

			if msg.Type != tt.wantType {
				t.Errorf("Type: got 0x%08X, want 0x%08X", msg.Type, tt.wantType)
			}
			if msg.Destination != tt.wantDest {
				t.Errorf("Destination: got %d, want %d", msg.Destination, tt.wantDest)
			}
			if msg.RunspaceID != tt.wantRPID {
				t.Errorf("RunspaceID: got %s, want %s", msg.RunspaceID, tt.wantRPID)
			}
			if msg.PipelineID != tt.wantPID {
				t.Errorf("PipelineID: got %s, want %s", msg.PipelineID, tt.wantPID)
			}
		})
	}
}

func TestRunspacePoolStates(t *testing.T) {
	states := []struct {
		name  string
		value RunspacePoolState
		want  int32
	}{
		{"BeforeOpen", RunspacePoolStateBeforeOpen, 0},
		{"Opening", RunspacePoolStateOpening, 1},
		{"Opened", RunspacePoolStateOpened, 2},
		{"Closing", RunspacePoolStateClosing, 3},
		{"Closed", RunspacePoolStateClosed, 4},
		{"Broken", RunspacePoolStateBroken, 5},
		{"Disconnected", RunspacePoolStateDisconnected, 6},
		{"Connecting", RunspacePoolStateConnecting, 7},
	}

	for _, s := range states {
		t.Run(s.name, func(t *testing.T) {
			if int32(s.value) != s.want {
				t.Errorf("got %d, want %d", s.value, s.want)
			}
		})
	}
}

func TestPipelineStates(t *testing.T) {
	states := []struct {
		name  string
		value PipelineState
		want  int32
	}{
		{"NotStarted", PipelineStateNotStarted, 0},
		{"Running", PipelineStateRunning, 1},
		{"Stopping", PipelineStateStopping, 2},
		{"Stopped", PipelineStateStopped, 3},
		{"Completed", PipelineStateCompleted, 4},
		{"Failed", PipelineStateFailed, 5},
		{"Disconnected", PipelineStateDisconnected, 6},
	}

	for _, s := range states {
		t.Run(s.name, func(t *testing.T) {
			if int32(s.value) != s.want {
				t.Errorf("got %d, want %d", s.value, s.want)
			}
		})
	}
}

// TestMessageEndiannessKnownBytes verifies that message encoding uses correct
// little-endian byte order by testing against a manually crafted message.
//
// This test ensures:
//   - Destination field uses little-endian (4 bytes)
//   - MessageType field uses little-endian (4 bytes)
//   - RPID/PID GUIDs use .NET GUID format (mixed-endian per RFC 4122)
//
// The expected byte layout validates our implementation against the PSRP spec
// and .NET conventions, even though MS-PSRP doesn't explicitly document endianness.
func TestMessageEndiannessKnownBytes(t *testing.T) {
	// Create a message with predictable values
	// UUID: 00010203-0405-0607-0809-0a0b0c0d0e0f (sequential bytes for easy verification)
	rpID := uuid.UUID{0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f}
	// UUID: 10111213-1415-1617-1819-1a1b1c1d1e1f
	pID := uuid.UUID{0x10, 0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17, 0x18, 0x19, 0x1a, 0x1b, 0x1c, 0x1d, 0x1e, 0x1f}

	msg := &Message{
		Destination: DestinationServer,            // 2 (0x00000002)
		Type:        MessageTypeSessionCapability, // 0x00010002
		RunspaceID:  rpID,
		PipelineID:  pID,
		Data:        []byte{0xAA, 0xBB}, // Simple test payload
	}

	encoded, err := msg.Encode()
	if err != nil {
		t.Fatalf("Encode failed: %v", err)
	}

	// Expected byte layout (little-endian):
	expected := []byte{
		// Destination: 2 (little-endian uint32)
		0x02, 0x00, 0x00, 0x00,
		// MessageType: 0x00010002 (little-endian uint32)
		0x02, 0x00, 0x01, 0x00,
		// RPID: 00010203-0405-0607-0809-0a0b0c0d0e0f (.NET GUID format)
		// Time-low (4 bytes, little-endian): 0x00010203 -> 03 02 01 00
		0x03, 0x02, 0x01, 0x00,
		// Time-mid (2 bytes, little-endian): 0x0405 -> 05 04
		0x05, 0x04,
		// Time-hi (2 bytes, little-endian): 0x0607 -> 07 06
		0x07, 0x06,
		// Clock-seq and node (8 bytes, big-endian - no swap)
		0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f,
		// PID: 10111213-1415-1617-1819-1a1b1c1d1e1f (.NET GUID format)
		// Time-low (4 bytes, little-endian): 0x10111213 -> 13 12 11 10
		0x13, 0x12, 0x11, 0x10,
		// Time-mid (2 bytes, little-endian): 0x1415 -> 15 14
		0x15, 0x14,
		// Time-hi (2 bytes, little-endian): 0x1617 -> 17 16
		0x17, 0x16,
		// Clock-seq and node (8 bytes, big-endian - no swap)
		0x18, 0x19, 0x1a, 0x1b, 0x1c, 0x1d, 0x1e, 0x1f,
		// Data payload
		0xAA, 0xBB,
	}

	if !bytes.Equal(encoded, expected) {
		t.Errorf("Encoded message bytes don't match expected little-endian layout")
		t.Logf("Expected (%d bytes):\n%s", len(expected), formatHexDump(expected))
		t.Logf("Got (%d bytes):\n%s", len(encoded), formatHexDump(encoded))

		// Detailed breakdown for debugging
		if len(encoded) >= 4 {
			t.Logf("Destination bytes: %02x %02x %02x %02x (should be 02 00 00 00)",
				encoded[0], encoded[1], encoded[2], encoded[3])
		}
		if len(encoded) >= 8 {
			t.Logf("MessageType bytes: %02x %02x %02x %02x (should be 02 00 01 00)",
				encoded[4], encoded[5], encoded[6], encoded[7])
		}
		if len(encoded) >= 24 {
			t.Logf("RPID bytes: %02x %02x %02x %02x %02x %02x %02x %02x %02x %02x %02x %02x %02x %02x %02x %02x",
				encoded[8], encoded[9], encoded[10], encoded[11], encoded[12], encoded[13], encoded[14], encoded[15],
				encoded[16], encoded[17], encoded[18], encoded[19], encoded[20], encoded[21], encoded[22], encoded[23])
		}
	}

	// Verify round-trip: decode and check all fields match
	decoded, err := Decode(encoded)
	if err != nil {
		t.Fatalf("Decode failed: %v", err)
	}

	if decoded.Destination != msg.Destination {
		t.Errorf("Destination mismatch after round-trip: got %d, want %d", decoded.Destination, msg.Destination)
	}
	if decoded.Type != msg.Type {
		t.Errorf("MessageType mismatch after round-trip: got 0x%08X, want 0x%08X", decoded.Type, msg.Type)
	}
	if decoded.RunspaceID != msg.RunspaceID {
		t.Errorf("RunspaceID mismatch after round-trip: got %s, want %s", decoded.RunspaceID, msg.RunspaceID)
	}
	if decoded.PipelineID != msg.PipelineID {
		t.Errorf("PipelineID mismatch after round-trip: got %s, want %s", decoded.PipelineID, msg.PipelineID)
	}
	if !bytes.Equal(decoded.Data, msg.Data) {
		t.Errorf("Data mismatch after round-trip: got %v, want %v", decoded.Data, msg.Data)
	}
}

// TestMessageDecodeKnownGoodCapture tests decoding against a known-good PSRP message.
// This validates interoperability with real PowerShell/PSRP implementations.
//
// The test message is a SESSION_CAPABILITY message with:
//   - Destination: Server (2)
//   - MessageType: SESSION_CAPABILITY (0x00010002)
//   - RunspacePool ID: 12345678-1234-5678-9abc-def012345678
//   - Pipeline ID: 00000000-0000-0000-0000-000000000000 (nil)
//   - Data: Empty (real SESSION_CAPABILITY would have CLIXML data)
func TestMessageDecodeKnownGoodCapture(t *testing.T) {
	// Hand-crafted PSRP SESSION_CAPABILITY message header (40 bytes)
	// This represents the binary format that would be sent over the wire
	knownGoodMessage := []byte{
		// Destination: 2 (Server) - little-endian uint32
		0x02, 0x00, 0x00, 0x00,
		// MessageType: 0x00010002 (SESSION_CAPABILITY) - little-endian uint32
		0x02, 0x00, 0x01, 0x00,
		// RunspacePool ID: 12345678-1234-5678-9abc-def012345678
		// In .NET GUID format (mixed-endian):
		// Time-low: 0x12345678 -> 78 56 34 12 (little-endian)
		0x78, 0x56, 0x34, 0x12,
		// Time-mid: 0x1234 -> 34 12 (little-endian)
		0x34, 0x12,
		// Time-hi: 0x5678 -> 78 56 (little-endian)
		0x78, 0x56,
		// Clock-seq-hi, clock-seq-low: 0x9abc (big-endian, no swap)
		0x9a, 0xbc,
		// Node: 0xdef012345678 (big-endian, no swap)
		0xde, 0xf0, 0x12, 0x34, 0x56, 0x78,
		// Pipeline ID: 00000000-0000-0000-0000-000000000000 (nil UUID)
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		// Data: (none in this test case)
	}

	// Decode the message
	msg, err := Decode(knownGoodMessage)
	if err != nil {
		t.Fatalf("Failed to decode known-good message: %v", err)
	}

	// Verify all fields
	if msg.Destination != DestinationServer {
		t.Errorf("Destination: got %d, want %d", msg.Destination, DestinationServer)
	}

	if msg.Type != MessageTypeSessionCapability {
		t.Errorf("MessageType: got 0x%08X, want 0x%08X", msg.Type, MessageTypeSessionCapability)
	}

	expectedRPID := uuid.MustParse("12345678-1234-5678-9abc-def012345678")
	if msg.RunspaceID != expectedRPID {
		t.Errorf("RunspaceID: got %s, want %s", msg.RunspaceID, expectedRPID)
	}

	if msg.PipelineID != uuid.Nil {
		t.Errorf("PipelineID: got %s, want %s", msg.PipelineID, uuid.Nil)
	}

	if len(msg.Data) != 0 {
		t.Errorf("Data length: got %d, want 0", len(msg.Data))
	}

	// Verify round-trip encoding produces identical bytes
	reencoded, err := msg.Encode()
	if err != nil {
		t.Fatalf("Failed to re-encode message: %v", err)
	}

	if !bytes.Equal(reencoded, knownGoodMessage) {
		t.Errorf("Re-encoded message doesn't match original")
		t.Logf("Original:\n%s", formatHexDump(knownGoodMessage))
		t.Logf("Re-encoded:\n%s", formatHexDump(reencoded))
	}
}

// formatHexDump formats bytes as a readable hex dump for test output.
func formatHexDump(data []byte) string {
	var buf bytes.Buffer
	for i := 0; i < len(data); i += 16 {
		end := i + 16
		if end > len(data) {
			end = len(data)
		}
		buf.WriteString("  ")
		for j := i; j < end; j++ {
			buf.WriteString(fmt.Sprintf("%02x ", data[j]))
		}
		buf.WriteString("\n")
	}
	return buf.String()
}
