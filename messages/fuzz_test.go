package messages

import (
	"bytes"
	"testing"

	"github.com/google/uuid"
)

// FuzzDecode tests the message decoder with random input.
// The decoder should handle malformed data gracefully without panicking.
func FuzzDecode(f *testing.F) {
	// Add seed corpus with valid messages
	validMsg := &Message{
		Destination: DestinationClient,
		Type:        MessageTypeSessionCapability,
		RunspaceID:  uuid.New(),
		PipelineID:  uuid.Nil,
		Data:        []byte("test data"),
	}
	encoded, _ := validMsg.Encode()
	f.Add(encoded)

	// Add edge cases
	f.Add(make([]byte, HeaderSize))   // Minimal valid header size
	f.Add(make([]byte, HeaderSize-1)) // Too short
	f.Add([]byte{})                   // Empty
	f.Add([]byte{0xFF, 0xFF, 0xFF})   // Random garbage

	f.Fuzz(func(t *testing.T, data []byte) {
		// The decoder must not panic on any input
		_, _ = Decode(data)
	})
}

// FuzzMessageRoundTrip tests that message encode/decode produces the original message.
func FuzzMessageRoundTrip(f *testing.F) {
	// Add seed corpus with various message types
	f.Add(uint32(DestinationClient), uint32(MessageTypeSessionCapability), []byte("test"))
	f.Add(uint32(DestinationServer), uint32(MessageTypeCreatePipeline), []byte(""))
	f.Add(uint32(DestinationClient), uint32(MessageTypePipelineOutput), []byte("output data here"))

	f.Fuzz(func(t *testing.T, destInt uint32, typeInt uint32, data []byte) {
		// Create message with fuzzed values
		msg := &Message{
			Destination: Destination(destInt),
			Type:        MessageType(typeInt),
			RunspaceID:  uuid.New(),
			PipelineID:  uuid.New(),
			Data:        data,
		}

		// Encode
		encoded, err := msg.Encode()
		if err != nil {
			t.Fatalf("Encode failed: %v", err)
		}

		// Decode
		decoded, err := Decode(encoded)
		if err != nil {
			t.Fatalf("Decode failed: %v", err)
		}

		// Verify round-trip
		if decoded.Destination != msg.Destination {
			t.Errorf("Destination mismatch: got %v, want %v", decoded.Destination, msg.Destination)
		}
		if decoded.Type != msg.Type {
			t.Errorf("Type mismatch: got %v, want %v", decoded.Type, msg.Type)
		}
		if decoded.RunspaceID != msg.RunspaceID {
			t.Errorf("RunspaceID mismatch: got %v, want %v", decoded.RunspaceID, msg.RunspaceID)
		}
		if decoded.PipelineID != msg.PipelineID {
			t.Errorf("PipelineID mismatch: got %v, want %v", decoded.PipelineID, msg.PipelineID)
		}
		if !bytes.Equal(decoded.Data, msg.Data) {
			t.Errorf("Data mismatch: got %v, want %v", decoded.Data, msg.Data)
		}
	})
}

// FuzzUUIDConversion tests the UUID little-endian conversion functions.
func FuzzUUIDConversion(f *testing.F) {
	// Add seed with known UUIDs
	nilUUID := uuid.Nil
	f.Add(nilUUID[:])
	knownUUID := uuid.MustParse("12345678-1234-5678-9abc-def012345678")
	f.Add(knownUUID[:])

	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) != 16 {
			return // Skip invalid UUID length
		}

		// Create UUID from bytes
		original, err := uuid.FromBytes(data)
		if err != nil {
			return // Skip invalid UUID bytes
		}

		// Convert to little-endian and back
		leBytes := uuidToLittleEndianBytes(original)

		recovered, err := uuidFromLittleEndianBytes(leBytes)
		if err != nil {
			t.Fatalf("uuidFromLittleEndianBytes failed: %v", err)
		}

		// Verify round-trip
		if recovered != original {
			t.Errorf("UUID round-trip failed: got %v, want %v", recovered, original)
		}
	})
}
