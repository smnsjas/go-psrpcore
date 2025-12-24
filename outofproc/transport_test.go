package outofproc

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestParseDataPacket(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantType PacketType
		wantGUID string
		wantData string
	}{
		{
			name:     "data packet with content",
			input:    "<Data Stream='Default' PSGuid='00000000-0000-0000-0000-000000000000'>SGVsbG8gV29ybGQ=</Data>",
			wantType: PacketTypeData,
			wantGUID: "00000000-0000-0000-0000-000000000000",
			wantData: "Hello World",
		},
		{
			name:     "data packet with pipeline guid",
			input:    "<Data Stream='Default' PSGuid='12345678-1234-1234-1234-123456789abc'>dGVzdA==</Data>",
			wantType: PacketTypeData,
			wantGUID: "12345678-1234-1234-1234-123456789abc",
			wantData: "test",
		},
		{
			name:     "data packet with prompt response stream",
			input:    "<Data Stream='PromptResponse' PSGuid='00000000-0000-0000-0000-000000000000'>YWJj</Data>",
			wantType: PacketTypeData,
			wantGUID: "00000000-0000-0000-0000-000000000000",
			wantData: "abc",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			packet, err := parsePacket(tt.input)
			if err != nil {
				t.Fatalf("parsePacket() error = %v", err)
			}

			if packet.Type != tt.wantType {
				t.Errorf("Type = %v, want %v", packet.Type, tt.wantType)
			}

			if packet.PSGuid.String() != tt.wantGUID {
				t.Errorf("PSGuid = %v, want %v", packet.PSGuid, tt.wantGUID)
			}

			if string(packet.Data) != tt.wantData {
				t.Errorf("Data = %q, want %q", string(packet.Data), tt.wantData)
			}
		})
	}
}

func TestParseAckPackets(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantType PacketType
		wantGUID string
	}{
		{
			name:     "data ack",
			input:    "<DataAck PSGuid='00000000-0000-0000-0000-000000000000' />",
			wantType: PacketTypeDataAck,
			wantGUID: "00000000-0000-0000-0000-000000000000",
		},
		{
			name:     "command ack",
			input:    "<CommandAck PSGuid='12345678-1234-1234-1234-123456789abc' />",
			wantType: PacketTypeCommandAck,
			wantGUID: "12345678-1234-1234-1234-123456789abc",
		},
		{
			name:     "close ack",
			input:    "<CloseAck PSGuid='00000000-0000-0000-0000-000000000000' />",
			wantType: PacketTypeCloseAck,
			wantGUID: "00000000-0000-0000-0000-000000000000",
		},
		{
			name:     "signal ack",
			input:    "<SignalAck PSGuid='abcdef12-3456-7890-abcd-ef1234567890' />",
			wantType: PacketTypeSignalAck,
			wantGUID: "abcdef12-3456-7890-abcd-ef1234567890",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			packet, err := parsePacket(tt.input)
			if err != nil {
				t.Fatalf("parsePacket() error = %v", err)
			}

			if packet.Type != tt.wantType {
				t.Errorf("Type = %v, want %v", packet.Type, tt.wantType)
			}

			if packet.PSGuid.String() != tt.wantGUID {
				t.Errorf("PSGuid = %v, want %v", packet.PSGuid, tt.wantGUID)
			}
		})
	}
}

func TestSendData(t *testing.T) {
	var buf bytes.Buffer
	transport := NewTransport(strings.NewReader(""), &buf)

	testGUID := uuid.MustParse("12345678-1234-1234-1234-123456789abc")
	err := transport.SendData(testGUID, []byte("Hello World"))
	if err != nil {
		t.Fatalf("SendData() error = %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Stream='Default'") {
		t.Error("missing Stream attribute")
	}
	if !strings.Contains(output, "PSGuid='12345678-1234-1234-1234-123456789abc'") {
		t.Error("missing PSGuid attribute")
	}
	if !strings.Contains(output, "SGVsbG8gV29ybGQ=") {
		t.Error("missing base64 encoded data")
	}
	if !strings.HasSuffix(output, "\n") {
		t.Error("missing trailing newline")
	}
}

func TestSendCommand(t *testing.T) {
	var buf bytes.Buffer
	transport := NewTransport(strings.NewReader(""), &buf)

	testGUID := uuid.MustParse("abcdef12-3456-7890-abcd-ef1234567890")
	err := transport.SendCommand(testGUID)
	if err != nil {
		t.Fatalf("SendCommand() error = %v", err)
	}

	output := buf.String()
	expected := "<Command PSGuid='abcdef12-3456-7890-abcd-ef1234567890' />\n"
	if output != expected {
		t.Errorf("got %q, want %q", output, expected)
	}
}

func TestSendClose(t *testing.T) {
	var buf bytes.Buffer
	transport := NewTransport(strings.NewReader(""), &buf)

	err := transport.SendClose(NullGUID)
	if err != nil {
		t.Fatalf("SendClose() error = %v", err)
	}

	output := buf.String()
	expected := "<Close PSGuid='00000000-0000-0000-0000-000000000000' />\n"
	if output != expected {
		t.Errorf("got %q, want %q", output, expected)
	}
}

func TestSendSignal(t *testing.T) {
	var buf bytes.Buffer
	transport := NewTransport(strings.NewReader(""), &buf)

	testGUID := uuid.MustParse("11111111-2222-3333-4444-555555555555")
	err := transport.SendSignal(testGUID)
	if err != nil {
		t.Fatalf("SendSignal() error = %v", err)
	}

	output := buf.String()
	expected := "<Signal PSGuid='11111111-2222-3333-4444-555555555555' />\n"
	if output != expected {
		t.Errorf("got %q, want %q", output, expected)
	}
}

func TestReceivePacket(t *testing.T) {
	input := `<Data Stream='Default' PSGuid='00000000-0000-0000-0000-000000000000'>dGVzdA==</Data>
<DataAck PSGuid='00000000-0000-0000-0000-000000000000' />
<CommandAck PSGuid='12345678-1234-1234-1234-123456789abc' />
`

	transport := NewTransport(strings.NewReader(input), io.Discard)

	// First packet: Data
	p1, err := transport.ReceivePacket()
	if err != nil {
		t.Fatalf("ReceivePacket() #1 error = %v", err)
	}
	if p1.Type != PacketTypeData {
		t.Errorf("Packet #1 Type = %v, want %v", p1.Type, PacketTypeData)
	}
	if string(p1.Data) != "test" {
		t.Errorf("Packet #1 Data = %q, want %q", string(p1.Data), "test")
	}

	// Second packet: DataAck
	p2, err := transport.ReceivePacket()
	if err != nil {
		t.Fatalf("ReceivePacket() #2 error = %v", err)
	}
	if p2.Type != PacketTypeDataAck {
		t.Errorf("Packet #2 Type = %v, want %v", p2.Type, PacketTypeDataAck)
	}

	// Third packet: CommandAck
	p3, err := transport.ReceivePacket()
	if err != nil {
		t.Fatalf("ReceivePacket() #3 error = %v", err)
	}
	if p3.Type != PacketTypeCommandAck {
		t.Errorf("Packet #3 Type = %v, want %v", p3.Type, PacketTypeCommandAck)
	}
	if p3.PSGuid.String() != "12345678-1234-1234-1234-123456789abc" {
		t.Errorf("Packet #3 PSGuid = %v, want 12345678-1234-1234-1234-123456789abc", p3.PSGuid)
	}
}

func TestReceivePacketSkipsEmptyLines(t *testing.T) {
	input := `

<DataAck PSGuid='00000000-0000-0000-0000-000000000000' />

`

	transport := NewTransport(strings.NewReader(input), io.Discard)

	packet, err := transport.ReceivePacket()
	if err != nil {
		t.Fatalf("ReceivePacket() error = %v", err)
	}

	if packet.Type != PacketTypeDataAck {
		t.Errorf("Type = %v, want %v", packet.Type, PacketTypeDataAck)
	}
}

func TestRoundTrip(t *testing.T) {
	// Create a pipe to simulate bidirectional communication
	pr, pw := io.Pipe()

	// Server side writes, client side reads
	serverTransport := NewTransport(strings.NewReader(""), pw)
	clientTransport := NewTransport(pr, io.Discard)

	testGUID := uuid.MustParse("12345678-1234-1234-1234-123456789abc")
	testData := []byte("Hello, PSRP!")

	// Send in a goroutine
	go func() {
		if err := serverTransport.SendData(testGUID, testData); err != nil {
			t.Errorf("SendData() error = %v", err)
		}
		_ = pw.Close()
	}()

	// Receive
	packet, err := clientTransport.ReceivePacket()
	if err != nil {
		t.Fatalf("ReceivePacket() error = %v", err)
	}

	if packet.Type != PacketTypeData {
		t.Errorf("Type = %v, want %v", packet.Type, PacketTypeData)
	}
	if packet.PSGuid != testGUID {
		t.Errorf("PSGuid = %v, want %v", packet.PSGuid, testGUID)
	}
	if !bytes.Equal(packet.Data, testData) {
		t.Errorf("Data = %q, want %q", packet.Data, testData)
	}
}

func TestIsSessionGUID(t *testing.T) {
	if !IsSessionGUID(NullGUID) {
		t.Error("IsSessionGUID(NullGUID) = false, want true")
	}

	randomGUID := uuid.New()
	if IsSessionGUID(randomGUID) {
		t.Error("IsSessionGUID(randomGUID) = true, want false")
	}
}

func TestFormatGUID(t *testing.T) {
	guid := uuid.MustParse("ABCDEF12-3456-7890-ABCD-EF1234567890")
	formatted := formatGUID(guid)

	// Should be lowercase
	if formatted != "abcdef12-3456-7890-abcd-ef1234567890" {
		t.Errorf("formatGUID() = %q, want lowercase", formatted)
	}
}
