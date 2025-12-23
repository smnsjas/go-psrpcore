package outofproc

import (
	"bytes"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestAdapterWrite(t *testing.T) {
	var buf bytes.Buffer
	transport := NewTransport(strings.NewReader(""), &buf)

	runspaceID := uuid.MustParse("12345678-1234-1234-1234-123456789abc")
	adapter := NewAdapter(transport, runspaceID)
	defer adapter.Close()

	testData := []byte("Hello, PSRP!")
	n, err := adapter.Write(testData)
	if err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if n != len(testData) {
		t.Errorf("Write() n = %d, want %d", n, len(testData))
	}

	output := buf.String()
	if !strings.Contains(output, "Stream='Default'") {
		t.Error("missing Stream attribute in output")
	}
	// Write should use NULL GUID for session-level data, not the runspace ID
	if !strings.Contains(output, "PSGuid='00000000-0000-0000-0000-000000000000'") {
		t.Errorf("expected NULL GUID for session-level write, got: %s", output)
	}
	// "Hello, PSRP!" base64 encoded is "SGVsbG8sIFBTUlAh"
	if !strings.Contains(output, "SGVsbG8sIFBTUlAh") {
		t.Errorf("missing base64 data in output, got: %s", output)
	}
}

func TestAdapterRead(t *testing.T) {
	// Simulate server sending data
	// "TestData" base64 encoded is "VGVzdERhdGE="
	input := "<Data Stream='Default' PSGuid='00000000-0000-0000-0000-000000000000'>VGVzdERhdGE=</Data>\n"

	transport := NewTransport(strings.NewReader(input), io.Discard)
	adapter := NewAdapter(transport, NullGUID)
	defer adapter.cancel() // Cancel context to stop read loop

	// Give the read loop time to process the packet
	time.Sleep(50 * time.Millisecond)

	buf := make([]byte, 100)
	n, err := adapter.Read(buf)
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}

	got := string(buf[:n])
	if got != "TestData" {
		t.Errorf("Read() = %q, want %q", got, "TestData")
	}
}

func TestAdapterMultipleReads(t *testing.T) {
	// Simulate multiple data packets
	input := `<Data Stream='Default' PSGuid='00000000-0000-0000-0000-000000000000'>Zmlyc3Q=</Data>
<Data Stream='Default' PSGuid='00000000-0000-0000-0000-000000000000'>c2Vjb25k</Data>
<Data Stream='Default' PSGuid='00000000-0000-0000-0000-000000000000'>dGhpcmQ=</Data>
`

	transport := NewTransport(strings.NewReader(input), io.Discard)
	adapter := NewAdapter(transport, NullGUID)
	defer adapter.cancel()

	// Give the read loop time to process all packets
	time.Sleep(100 * time.Millisecond)

	expected := []string{"first", "second", "third"}
	for i, want := range expected {
		buf := make([]byte, 100)
		n, err := adapter.Read(buf)
		if err != nil {
			t.Fatalf("Read() #%d error = %v", i+1, err)
		}
		got := string(buf[:n])
		if got != want {
			t.Errorf("Read() #%d = %q, want %q", i+1, got, want)
		}
	}
}

func TestAdapterReadAfterEOF(t *testing.T) {
	// Test that we can read all pending data even after transport EOF
	input := "<Data Stream='Default' PSGuid='00000000-0000-0000-0000-000000000000'>dGVzdA==</Data>\n"

	transport := NewTransport(strings.NewReader(input), io.Discard)
	adapter := NewAdapter(transport, NullGUID)
	defer adapter.cancel()

	// Wait for read loop to process and hit EOF
	time.Sleep(100 * time.Millisecond)

	// Should still be able to read the buffered data
	buf := make([]byte, 100)
	n, err := adapter.Read(buf)
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if string(buf[:n]) != "test" {
		t.Errorf("Read() = %q, want %q", string(buf[:n]), "test")
	}

	// Next read should return EOF
	_, err = adapter.Read(buf)
	if err != io.EOF {
		t.Errorf("second Read() error = %v, want EOF", err)
	}
}

func TestAdapterSendCommand(t *testing.T) {
	var buf bytes.Buffer
	transport := NewTransport(strings.NewReader(""), &buf)

	adapter := NewAdapter(transport, NullGUID)
	defer adapter.Close()

	pipelineID := uuid.MustParse("abcdef12-3456-7890-abcd-ef1234567890")
	err := adapter.SendCommand(pipelineID)
	if err != nil {
		t.Fatalf("SendCommand() error = %v", err)
	}

	output := buf.String()
	expected := "<Command PSGuid='abcdef12-3456-7890-abcd-ef1234567890' />\n"
	if output != expected {
		t.Errorf("SendCommand() output = %q, want %q", output, expected)
	}
}

func TestAdapterSendPipelineData(t *testing.T) {
	var buf bytes.Buffer
	transport := NewTransport(strings.NewReader(""), &buf)

	adapter := NewAdapter(transport, NullGUID)
	defer adapter.Close()

	pipelineID := uuid.MustParse("abcdef12-3456-7890-abcd-ef1234567890")
	err := adapter.SendPipelineData(pipelineID, []byte("test"))
	if err != nil {
		t.Fatalf("SendPipelineData() error = %v", err)
	}

	output := buf.String()
	// Should use the pipeline GUID, not NULL GUID
	if !strings.Contains(output, "PSGuid='abcdef12-3456-7890-abcd-ef1234567890'") {
		t.Errorf("SendPipelineData() should use pipeline GUID, got: %s", output)
	}
}

func TestAdapterSendSignal(t *testing.T) {
	var buf bytes.Buffer
	transport := NewTransport(strings.NewReader(""), &buf)

	adapter := NewAdapter(transport, NullGUID)
	defer adapter.Close()

	pipelineID := uuid.MustParse("11111111-2222-3333-4444-555555555555")
	err := adapter.SendSignal(pipelineID)
	if err != nil {
		t.Fatalf("SendSignal() error = %v", err)
	}

	output := buf.String()
	expected := "<Signal PSGuid='11111111-2222-3333-4444-555555555555' />\n"
	if output != expected {
		t.Errorf("SendSignal() output = %q, want %q", output, expected)
	}
}

func TestAdapterCommandAckCallback(t *testing.T) {
	// Simulate server sending CommandAck
	input := "<CommandAck PSGuid='abcdef12-3456-7890-abcd-ef1234567890' />\n"

	transport := NewTransport(strings.NewReader(input), io.Discard)
	adapter := NewAdapter(transport, NullGUID)
	defer adapter.cancel()

	var receivedGUID uuid.UUID
	var callbackCalled bool
	var mu sync.Mutex
	adapter.SetCommandAckHandler(func(guid uuid.UUID) {
		mu.Lock()
		defer mu.Unlock()
		callbackCalled = true
		receivedGUID = guid
	})

	// Give the read loop time to process
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if !callbackCalled {
		t.Error("CommandAck callback was not called")
	}

	expectedGUID := uuid.MustParse("abcdef12-3456-7890-abcd-ef1234567890")
	if receivedGUID != expectedGUID {
		t.Errorf("CommandAck GUID = %v, want %v", receivedGUID, expectedGUID)
	}
}

func TestAdapterCloseAckCallback(t *testing.T) {
	// Simulate server sending CloseAck for a pipeline (not session)
	input := "<CloseAck PSGuid='12345678-1234-1234-1234-123456789abc' />\n"

	transport := NewTransport(strings.NewReader(input), io.Discard)
	adapter := NewAdapter(transport, NullGUID)
	defer adapter.cancel()

	var callbackCalled bool
	var mu sync.Mutex
	adapter.SetCloseAckHandler(func(guid uuid.UUID) {
		mu.Lock()
		defer mu.Unlock()
		callbackCalled = true
	})

	// Give the read loop time to process
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if !callbackCalled {
		t.Error("CloseAck callback was not called")
	}
}

func TestAdapterTransport(t *testing.T) {
	var buf bytes.Buffer
	transport := NewTransport(strings.NewReader(""), &buf)
	adapter := NewAdapter(transport, NullGUID)
	defer adapter.cancel()

	if adapter.Transport() != transport {
		t.Error("Transport() did not return the expected transport")
	}
}

func TestAdapterClose(t *testing.T) {
	var buf bytes.Buffer
	transport := NewTransport(strings.NewReader(""), &buf)
	adapter := NewAdapter(transport, NullGUID)

	err := adapter.Close()
	if err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "<Close PSGuid='00000000-0000-0000-0000-000000000000' />") {
		t.Errorf("Close() did not send close packet, got: %s", output)
	}
}
