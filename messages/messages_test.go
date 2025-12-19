package messages

import (
	"bytes"
	"encoding/binary"
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
	leBytes, err := uuidToLittleEndianBytes(testUUID)
	if err != nil {
		t.Fatalf("uuidToLittleEndianBytes failed: %v", err)
	}

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
	// Verify key message type constants have correct values
	tests := []struct {
		name  string
		value MessageType
		want  uint32
	}{
		{"SESSION_CAPABILITY", MessageTypeSessionCapability, 0x00010002},
		{"INIT_RUNSPACEPOOL", MessageTypeInitRunspacePool, 0x00010004},
		{"RUNSPACEPOOL_STATE", MessageTypeRunspacePoolState, 0x00010011},
		{"CREATE_PIPELINE", MessageTypeCreatePipeline, 0x00021002},
		{"PIPELINE_OUTPUT", MessageTypePipelineOutput, 0x00021008},
		{"PIPELINE_STATE", MessageTypePipelineState, 0x0002100A},
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
		name        string
		createFunc  func() *Message
		wantType    MessageType
		wantDest    Destination
		wantRPID    uuid.UUID
		wantPID     uuid.UUID
	}{
		{
			name:        "NewSessionCapability",
			createFunc:  func() *Message { return NewSessionCapability(rpID, data) },
			wantType:    MessageTypeSessionCapability,
			wantDest:    DestinationServer,
			wantRPID:    rpID,
			wantPID:     uuid.Nil,
		},
		{
			name:        "NewInitRunspacePool",
			createFunc:  func() *Message { return NewInitRunspacePool(rpID, data) },
			wantType:    MessageTypeInitRunspacePool,
			wantDest:    DestinationServer,
			wantRPID:    rpID,
			wantPID:     uuid.Nil,
		},
		{
			name:        "NewRunspacePoolStateMessage",
			createFunc:  func() *Message { return NewRunspacePoolStateMessage(rpID, RunspacePoolStateOpened, data) },
			wantType:    MessageTypeRunspacePoolState,
			wantDest:    DestinationClient,
			wantRPID:    rpID,
			wantPID:     uuid.Nil,
		},
		{
			name:        "NewCreatePipeline",
			createFunc:  func() *Message { return NewCreatePipeline(rpID, pID, data) },
			wantType:    MessageTypeCreatePipeline,
			wantDest:    DestinationServer,
			wantRPID:    rpID,
			wantPID:     pID,
		},
		{
			name:        "NewPipelineOutput",
			createFunc:  func() *Message { return NewPipelineOutput(rpID, pID, data) },
			wantType:    MessageTypePipelineOutput,
			wantDest:    DestinationClient,
			wantRPID:    rpID,
			wantPID:     pID,
		},
		{
			name:        "NewPipelineState",
			createFunc:  func() *Message { return NewPipelineState(rpID, pID, PipelineStateCompleted, data) },
			wantType:    MessageTypePipelineState,
			wantDest:    DestinationClient,
			wantRPID:    rpID,
			wantPID:     pID,
		},
		{
			name:        "NewErrorRecord",
			createFunc:  func() *Message { return NewErrorRecord(rpID, pID, data) },
			wantType:    MessageTypeErrorRecord,
			wantDest:    DestinationClient,
			wantRPID:    rpID,
			wantPID:     pID,
		},
		{
			name:        "NewPipelineInput",
			createFunc:  func() *Message { return NewPipelineInput(rpID, pID, data) },
			wantType:    MessageTypePipelineInput,
			wantDest:    DestinationServer,
			wantRPID:    rpID,
			wantPID:     pID,
		},
		{
			name:        "NewEndOfPipelineInput",
			createFunc:  func() *Message { return NewEndOfPipelineInput(rpID, pID) },
			wantType:    MessageTypeEndOfPipelineInput,
			wantDest:    DestinationServer,
			wantRPID:    rpID,
			wantPID:     pID,
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
