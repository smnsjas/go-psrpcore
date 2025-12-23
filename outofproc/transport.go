package outofproc

import (
	"bufio"
	"encoding/base64"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	"github.com/google/uuid"
)

// Stream represents the data stream type in OutOfProcess protocol.
type Stream string

const (
	// StreamDefault is the default data stream.
	StreamDefault Stream = "Default"
	// StreamPromptResponse is used for host prompt responses.
	StreamPromptResponse Stream = "PromptResponse"
)

// NullGUID is the zero GUID used for runspace-level operations.
// Pipeline-specific operations use the pipeline's GUID.
var NullGUID = uuid.UUID{}

// PacketType represents the type of OutOfProcess packet.
type PacketType string

const (
	PacketTypeData       PacketType = "Data"
	PacketTypeDataAck    PacketType = "DataAck"
	PacketTypeCommand    PacketType = "Command"
	PacketTypeCommandAck PacketType = "CommandAck"
	PacketTypeClose      PacketType = "Close"
	PacketTypeCloseAck   PacketType = "CloseAck"
	PacketTypeSignal     PacketType = "Signal"
	PacketTypeSignalAck  PacketType = "SignalAck"
)

// Packet represents a received packet from the OutOfProcess protocol.
type Packet struct {
	Type   PacketType
	PSGuid uuid.UUID
	Stream Stream
	Data   []byte // Decoded fragment data (only for Data packets)
}

// Transport implements the OutOfProcess framing protocol.
// It wraps a reader and writer (typically stdin/stdout of a child process).
type Transport struct {
	reader *bufio.Reader
	writer io.Writer
	mu     sync.Mutex // Protects writer
}

// NewTransport creates a new OutOfProcess transport.
// The reader is used for receiving packets, the writer for sending.
// For bidirectional streams, pass the same io.ReadWriter for both.
func NewTransport(reader io.Reader, writer io.Writer) *Transport {
	return &Transport{
		reader: bufio.NewReader(reader),
		writer: writer,
	}
}

// NewTransportFromReadWriter creates a transport from a single io.ReadWriter.
func NewTransportFromReadWriter(rw io.ReadWriter) *Transport {
	return NewTransport(rw, rw)
}

// SendData sends fragment data to the remote end.
// The data should be one or more complete PSRP fragments.
func (t *Transport) SendData(psGuid uuid.UUID, data []byte) error {
	return t.SendDataWithStream(psGuid, StreamDefault, data)
}

// SendDataWithStream sends fragment data with a specific stream type.
func (t *Transport) SendDataWithStream(psGuid uuid.UUID, stream Stream, data []byte) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	encoded := base64.StdEncoding.EncodeToString(data)
	packet := fmt.Sprintf("<Data Stream='%s' PSGuid='%s'>%s</Data>\n",
		stream, formatGUID(psGuid), encoded)

	// Debug: log what we're sending
	fmt.Fprintf(os.Stderr, "DEBUG SEND: %s\n", truncate(packet, 200))

	_, err := t.writer.Write([]byte(packet))
	return err
}

// SendCommand signals pipeline creation to the server.
// This must be sent before sending pipeline data.
func (t *Transport) SendCommand(pipelineGuid uuid.UUID) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	packet := fmt.Sprintf("<Command PSGuid='%s' />\n", formatGUID(pipelineGuid))
	_, err := t.writer.Write([]byte(packet))
	return err
}

// SendClose sends a close signal for a runspace or pipeline.
// Use NullGUID for runspace-level close.
func (t *Transport) SendClose(psGuid uuid.UUID) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	packet := fmt.Sprintf("<Close PSGuid='%s' />\n", formatGUID(psGuid))
	_, err := t.writer.Write([]byte(packet))
	return err
}

// SendSignal sends a signal to a pipeline (e.g., to stop execution).
func (t *Transport) SendSignal(psGuid uuid.UUID) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	packet := fmt.Sprintf("<Signal PSGuid='%s' />\n", formatGUID(psGuid))
	_, err := t.writer.Write([]byte(packet))
	return err
}

// SendDataAck sends a data acknowledgment.
func (t *Transport) SendDataAck(psGuid uuid.UUID) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	packet := fmt.Sprintf("<DataAck PSGuid='%s' />\n", formatGUID(psGuid))
	_, err := t.writer.Write([]byte(packet))
	return err
}

// ReceivePacket reads and parses the next packet from the transport.
// It blocks until a complete packet is received or an error occurs.
func (t *Transport) ReceivePacket() (*Packet, error) {
	for {
		line, err := t.reader.ReadString('\n')
		if err != nil {
			return nil, err
		}

		// Debug: log what we receive
		fmt.Fprintf(os.Stderr, "DEBUG RECV: %s\n", truncate(line, 200))

		line = strings.TrimSpace(line)
		if line == "" {
			continue // Skip empty lines
		}

		// Skip UTF-8 BOM if present (\xEF\xBB\xBF)
		line = strings.TrimPrefix(line, "\xEF\xBB\xBF")

		// Find the start of the XML element
		idx := strings.Index(line, "<")
		if idx == -1 {
			// No XML element found, skip this line
			continue
		}
		if idx > 0 {
			// Strip any leading non-XML content
			line = line[idx:]
		}

		packet, err := parsePacket(line)
		if err != nil {
			return nil, fmt.Errorf("parse packet: %w", err)
		}

		return packet, nil
	}
}

// parsePacket parses a single line of OutOfProcess protocol.
func parsePacket(line string) (*Packet, error) {
	decoder := xml.NewDecoder(strings.NewReader(line))

	token, err := decoder.Token()
	if err != nil {
		return nil, fmt.Errorf("read token: %w (line: %q)", err, truncate(line, 100))
	}

	startElem, ok := token.(xml.StartElement)
	if !ok {
		return nil, fmt.Errorf("expected start element, got %T (line: %q)", token, truncate(line, 100))
	}

	packet := &Packet{
		Type:   PacketType(startElem.Name.Local),
		Stream: StreamDefault,
	}

	// Extract attributes
	for _, attr := range startElem.Attr {
		switch attr.Name.Local {
		case "PSGuid":
			guid, err := uuid.Parse(attr.Value)
			if err != nil {
				return nil, fmt.Errorf("parse PSGuid %q: %w", attr.Value, err)
			}
			packet.PSGuid = guid
		case "Stream":
			packet.Stream = Stream(attr.Value)
		}
	}

	// For Data packets, read the base64 content
	if packet.Type == PacketTypeData {
		token, err := decoder.Token()
		if err != nil {
			if errors.Is(err, io.EOF) {
				// Self-closing tag with no content
				return packet, nil
			}
			return nil, fmt.Errorf("read data content: %w", err)
		}

		switch t := token.(type) {
		case xml.CharData:
			decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(t)))
			if err != nil {
				return nil, fmt.Errorf("decode base64: %w", err)
			}
			packet.Data = decoded
		case xml.EndElement:
			// Empty data element
		default:
			return nil, fmt.Errorf("unexpected token type in Data element: %T", token)
		}
	}

	return packet, nil
}

// formatGUID formats a UUID in the PowerShell-expected format (lowercase with hyphens).
func formatGUID(id uuid.UUID) string {
	return strings.ToLower(id.String())
}

// IsSessionGUID returns true if the GUID is the null GUID used for session/runspace operations.
func IsSessionGUID(id uuid.UUID) bool {
	return id == NullGUID
}

// truncate shortens a string for error messages.
func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
