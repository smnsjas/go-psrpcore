package outofproc

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
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
	// PacketTypeData represents a data packet containing a PSRP fragment.
	PacketTypeData PacketType = "Data"
	// PacketTypeDataAck represents an acknowledgment for a data packet.
	PacketTypeDataAck PacketType = "DataAck"
	// PacketTypeCommand represents a command packet (pipeline creation).
	PacketTypeCommand PacketType = "Command"
	// PacketTypeCommandAck represents an acknowledgment for a command packet.
	PacketTypeCommandAck PacketType = "CommandAck"
	// PacketTypeClose represents a close signal.
	PacketTypeClose PacketType = "Close"
	// PacketTypeCloseAck represents an acknowledgment for a close signal.
	PacketTypeCloseAck PacketType = "CloseAck"
	// PacketTypeSignal represents a signal (e.g., stop).
	PacketTypeSignal PacketType = "Signal"
	// PacketTypeSignalAck represents an acknowledgment for a signal.
	PacketTypeSignalAck PacketType = "SignalAck"
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

// sendBufferPool recycles buffers for sending packets to reduce allocations.
var sendBufferPool = sync.Pool{
	New: func() interface{} {
		return new(bytes.Buffer)
	},
}

// SendData sends fragment data to the remote end.
// The data should be one or more complete PSRP fragments.
func (t *Transport) SendData(psGUID uuid.UUID, data []byte) error {
	return t.SendDataWithStream(psGUID, StreamDefault, data)
}

// SendDataWithStream sends fragment data with a specific stream type.
func (t *Transport) SendDataWithStream(psGUID uuid.UUID, stream Stream, data []byte) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	buf := sendBufferPool.Get().(*bytes.Buffer)
	buf.Reset()
	defer sendBufferPool.Put(buf)

	buf.WriteString("<Data Stream='")
	buf.WriteString(string(stream))
	buf.WriteString("' PSGuid='")
	buf.WriteString(formatGUID(psGUID))
	buf.WriteString("'>")

	encoder := base64.NewEncoder(base64.StdEncoding, buf)
	if _, err := encoder.Write(data); err != nil {
		return fmt.Errorf("base64 encode: %w", err)
	}
	if err := encoder.Close(); err != nil {
		return fmt.Errorf("base64 close: %w", err)
	}

	buf.WriteString("</Data>\n")

	// Debug: log what we're sending
	// fmt.Fprintf(os.Stderr, "DEBUG SEND: %s\n", truncate(buf.String(), 200))

	_, err := t.writer.Write(buf.Bytes())
	return err
}

// SendCommand signals pipeline creation to the server.
// This must be sent before sending pipeline data.
func (t *Transport) SendCommand(pipelineGUID uuid.UUID) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	buf := sendBufferPool.Get().(*bytes.Buffer)
	buf.Reset()
	defer sendBufferPool.Put(buf)

	buf.WriteString("<Command PSGuid='")
	buf.WriteString(formatGUID(pipelineGUID))
	buf.WriteString("' />\n")

	_, err := t.writer.Write(buf.Bytes())
	return err
}

// SendClose sends a close signal for a runspace or pipeline.
// Use NullGUID for runspace-level close.
func (t *Transport) SendClose(psGUID uuid.UUID) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	buf := sendBufferPool.Get().(*bytes.Buffer)
	buf.Reset()
	defer sendBufferPool.Put(buf)

	buf.WriteString("<Close PSGuid='")
	buf.WriteString(formatGUID(psGUID))
	buf.WriteString("' />\n")

	_, err := t.writer.Write(buf.Bytes())
	return err
}

// SendSignal sends a signal to a pipeline (e.g., to stop execution).
func (t *Transport) SendSignal(psGUID uuid.UUID) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	buf := sendBufferPool.Get().(*bytes.Buffer)
	buf.Reset()
	defer sendBufferPool.Put(buf)

	buf.WriteString("<Signal PSGuid='")
	buf.WriteString(formatGUID(psGUID))
	buf.WriteString("' />\n")

	_, err := t.writer.Write(buf.Bytes())
	return err
}

// SendDataAck sends a data acknowledgment.
func (t *Transport) SendDataAck(psGUID uuid.UUID) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	buf := sendBufferPool.Get().(*bytes.Buffer)
	buf.Reset()
	defer sendBufferPool.Put(buf)

	buf.WriteString("<DataAck PSGuid='")
	buf.WriteString(formatGUID(psGUID))
	buf.WriteString("' />\n")

	_, err := t.writer.Write(buf.Bytes())
	return err
}

// SendCloseAck sends a close acknowledgment (response to server Close).
func (t *Transport) SendCloseAck(psGUID uuid.UUID) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	buf := sendBufferPool.Get().(*bytes.Buffer)
	buf.Reset()
	defer sendBufferPool.Put(buf)

	buf.WriteString("<CloseAck PSGuid='")
	buf.WriteString(formatGUID(psGUID))
	buf.WriteString("' />\n")

	_, err := t.writer.Write(buf.Bytes())
	return err
}

// SendSignalAck sends a signal acknowledgment (response to server Signal).
func (t *Transport) SendSignalAck(psGUID uuid.UUID) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	buf := sendBufferPool.Get().(*bytes.Buffer)
	buf.Reset()
	defer sendBufferPool.Put(buf)

	buf.WriteString("<SignalAck PSGuid='")
	buf.WriteString(formatGUID(psGUID))
	buf.WriteString("' />\n")

	_, err := t.writer.Write(buf.Bytes())
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

		line = strings.TrimSpace(line)
		if line == "" {
			continue // Skip empty lines
		}

		// Strip UTF-8 BOM if present
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

// parsePacket parses a single line of OutOfProcess protocol manually for performance.
// XML Decoder is too slow and allocates too much for this high-throughput path.
func parsePacket(line string) (*Packet, error) {
	// Trim whitespace and BOM (rare but possible)
	line = strings.TrimSpace(line)
	line = strings.TrimPrefix(line, "\xEF\xBB\xBF")

	if len(line) < 5 { // Minimal packet: <A/>
		return nil, fmt.Errorf("line too short: %q", line)
	}

	packet := &Packet{Stream: StreamDefault}

	// Fast path: find element name
	// Format: <ElementName Space='...' PSGuid='...'> or <ElementName ... />
	if line[0] != '<' {
		return nil, fmt.Errorf("no opening < found")
	}
	line = line[1:] // Skip '<'

	// Find end of element name
	spaceIdx := strings.IndexByte(line, ' ')
	closeIdx := strings.IndexByte(line, '>')
	slashIdx := strings.IndexByte(line, '/')

	var elemName string

	// Logic to find the earliest delimiter
	minIdx := -1
	if spaceIdx != -1 {
		minIdx = spaceIdx
	}
	if closeIdx != -1 {
		if minIdx == -1 || closeIdx < minIdx {
			minIdx = closeIdx
		}
	}
	if slashIdx != -1 {
		if minIdx == -1 || slashIdx < minIdx {
			minIdx = slashIdx
		}
	}

	if minIdx == -1 {
		return nil, fmt.Errorf("malformed element name")
	}

	elemName = line[:minIdx]
	packet.Type = PacketType(elemName)

	// Extract attributes manually
	// Looking for PSGuid='...' and Stream='...'
	// We scan the string for these patterns. Since attributes are distinct, we can use strings.Index.

	// PSGuid
	if idx := strings.Index(line, "PSGuid='"); idx != -1 {
		guidStart := idx + 8 // len("PSGuid='")
		if guidStart < len(line) {
			guidEnd := strings.IndexByte(line[guidStart:], '\'')
			if guidEnd != -1 {
				guidStr := line[guidStart : guidStart+guidEnd]
				guid, err := uuid.Parse(guidStr)
				if err != nil {
					return nil, fmt.Errorf("parse PSGuid %q: %w", guidStr, err)
				}
				packet.PSGuid = guid
			}
		}
	}

	// Stream
	if idx := strings.Index(line, "Stream='"); idx != -1 {
		streamStart := idx + 8 // len("Stream='")
		if streamStart < len(line) {
			streamEnd := strings.IndexByte(line[streamStart:], '\'')
			if streamEnd != -1 {
				packet.Stream = Stream(line[streamStart : streamStart+streamEnd])
			}
		}
	}

	// Data Content
	if packet.Type == PacketTypeData {
		// Find Content Start: First '>'
		contentStart := strings.IndexByte(line, '>')
		if contentStart == -1 {
			return nil, fmt.Errorf("malformed data packet start")
		}
		contentStart++ // Skip '>'

		// Check for self-closing <Data ... /> which means empty data
		// We handle this case by checking for the endTag below

		// Robust way: Find </Data> (or </ElementName>)
		endTag := "</" + elemName + ">"
		contentEnd := strings.LastIndex(line, endTag)

		if contentEnd != -1 && contentEnd > contentStart {
			base64Data := line[contentStart:contentEnd]
			if len(base64Data) > 0 {
				decoded, err := base64.StdEncoding.DecodeString(base64Data)
				if err != nil {
					return nil, fmt.Errorf("decode base64: %w", err)
				}
				packet.Data = decoded
			}
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
