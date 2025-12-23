package outofproc

import (
	"encoding/base64"
	"fmt"
	"testing"

	"github.com/google/uuid"
)

func BenchmarkParsePacket(b *testing.B) {
	// Setup a sample packet line
	guid := uuid.New()
	data := []byte("some test data payload")
	encoded := base64.StdEncoding.EncodeToString(data)
	line := fmt.Sprintf("<Data Stream='Default' PSGuid='%s'>%s</Data>", guid, encoded)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := parsePacket(line)
		if err != nil {
			b.Fatalf("parsePacket failed: %v", err)
		}
	}
}

func BenchmarkSendData(b *testing.B) {
	// benchmark the payload construction part primarily
	// We use a discard writer to avoid IO noise
	t := &Transport{
		writer: &discardWriter{},
	}
	guid := uuid.New()
	data := make([]byte, 1024) // 1KB payload

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := t.SendData(guid, data); err != nil {
			b.Fatalf("SendData failed: %v", err)
		}
	}
}

type discardWriter struct{}

func (w *discardWriter) Write(p []byte) (int, error) {
	return len(p), nil
}
