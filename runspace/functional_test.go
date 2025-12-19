package runspace

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/smnsjas/go-psrpcore/fragments"
	"github.com/smnsjas/go-psrpcore/messages"
)

// blockingTransport uses io.Pipe to provide a thread-safe, blocking transport.
type blockingTransport struct {
	clientReader *io.PipeReader
	clientWriter *io.PipeWriter
	serverReader *io.PipeReader
	serverWriter *io.PipeWriter

	fragmenter    *fragments.Fragmenter
	readAssembler *fragments.Assembler
}

func newBlockingTransport() *blockingTransport {
	cr, sw := io.Pipe()
	sr, cw := io.Pipe()
	return &blockingTransport{
		clientReader:  cr,
		clientWriter:  cw,
		serverReader:  sr,
		serverWriter:  sw,
		fragmenter:    fragments.NewFragmenter(32768),
		readAssembler: fragments.NewAssembler(),
	}
}

func (b *blockingTransport) Read(p []byte) (n int, err error) {
	return b.clientReader.Read(p)
}

func (b *blockingTransport) Write(p []byte) (n int, err error) {
	return b.clientWriter.Write(p)
}

func (b *blockingTransport) Close() error {
	b.clientReader.Close()
	b.clientWriter.Close()
	b.serverReader.Close()
	b.serverWriter.Close()
	return nil
}

// queueMessage adds a message to the server-to-client pipe.
func (b *blockingTransport) queueMessage(msg *messages.Message) error {
	encoded, err := msg.Encode()
	if err != nil {
		return err
	}

	frags, err := b.fragmenter.Fragment(encoded)
	if err != nil {
		return err
	}

	for _, frag := range frags {
		fragData := frag.Encode()
		if _, err := b.serverWriter.Write(fragData); err != nil {
			return err
		}
	}

	return nil
}

// readMessage reads a message from the client-to-server pipe.
func (b *blockingTransport) readMessage() (*messages.Message, error) {
	for {
		// Read fragment header
		header := make([]byte, fragments.HeaderSize)
		if _, err := io.ReadFull(b.serverReader, header); err != nil {
			return nil, err
		}

		// Read blob data (big-endian)
		blobLen := binary.BigEndian.Uint32(header[17:21])
		fragData := make([]byte, fragments.HeaderSize+int(blobLen))
		copy(fragData[:fragments.HeaderSize], header)

		if blobLen > 0 {
			if _, err := io.ReadFull(b.serverReader, fragData[fragments.HeaderSize:]); err != nil {
				return nil, err
			}
		}

		// Decode fragment
		frag, err := fragments.Decode(fragData)
		if err != nil {
			return nil, err
		}

		// Assemble message
		complete, msgData, err := b.readAssembler.Add(frag)
		if err != nil {
			return nil, err
		}

		if complete {
			return messages.Decode(msgData)
		}
	}
}

func TestEndToEndFunctional(t *testing.T) {
	transport := newBlockingTransport()
	defer transport.Close()

	poolID := uuid.New()
	pool := New(transport, poolID)

	// Context for the test
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Start a goroutine to act as the "PowerShell Server"
	errChan := make(chan error, 1)
	go func() {
		// 1. Expected Handshake: SessionCapability
		msg, err := transport.readMessage()
		if err != nil {
			errChan <- fmt.Errorf("server read error (cap): %w", err)
			return
		}
		if msg.Type != messages.MessageTypeSessionCapability {
			errChan <- fmt.Errorf("server expected SessionCapability, got %v", msg.Type)
			return
		}

		// Respond with Server Capability
		// Note: Minimal CLIXML for capability
		capResp := &messages.Message{
			Destination: messages.DestinationClient,
			Type:        messages.MessageTypeSessionCapability,
			RunspaceID:  poolID,
			Data:        []byte(`<Obj RefId="0"><MS><S N="protocolversion">2.3</S><S N="PSVersion">5.1.0.0</S><S N="SerializationVersion">1.1.0.1</S></MS></Obj>`),
		}
		if err := transport.queueMessage(capResp); err != nil {
			errChan <- fmt.Errorf("server queue error (cap): %w", err)
			return
		}

		// 2. Expected Handshake: InitRunspacePool
		msg, err = transport.readMessage()
		if err != nil {
			errChan <- fmt.Errorf("server read error (init): %w", err)
			return
		}
		if msg.Type != messages.MessageTypeInitRunspacePool {
			errChan <- fmt.Errorf("server expected InitRunspacePool, got %v", msg.Type)
			return
		}

		// Respond with RunspacePoolState (Opened=2)
		stateResp := &messages.Message{
			Destination: messages.DestinationClient,
			Type:        messages.MessageTypeRunspacePoolState,
			RunspaceID:  poolID,
			Data:        []byte(`<Objs Version="1.1.0.1" xmlns="http://schemas.microsoft.com/powershell/2004/04"><I32>2</I32></Objs>`),
		}
		if err := transport.queueMessage(stateResp); err != nil {
			errChan <- fmt.Errorf("server queue error (state): %w", err)
			return
		}

		// 3. Expected Pipeline: CreatePipeline
		msg, err = transport.readMessage()
		if err != nil {
			errChan <- fmt.Errorf("server read error (pipeline): %w", err)
			return
		}
		if msg.Type != messages.MessageTypeCreatePipeline {
			errChan <- fmt.Errorf("server expected CreatePipeline, got %v", msg.Type)
			return
		}

		pipelineID := msg.PipelineID

		// Respond with PipelineState (Running=2)
		plStateRunning := &messages.Message{
			Destination: messages.DestinationClient,
			Type:        messages.MessageTypePipelineState,
			RunspaceID:  poolID,
			PipelineID:  pipelineID,
			Data:        []byte(`<Objs Version="1.1.0.1" xmlns="http://schemas.microsoft.com/powershell/2004/04"><I32>2</I32></Objs>`),
		}
		transport.queueMessage(plStateRunning)

		// Send some Output
		outputMsg := &messages.Message{
			Destination: messages.DestinationClient,
			Type:        messages.MessageTypePipelineOutput,
			RunspaceID:  poolID,
			PipelineID:  pipelineID,
			Data:        []byte(`<Objs Version="1.1.0.1" xmlns="http://schemas.microsoft.com/powershell/2004/04"><S>Hello from Mock Server!</S></Objs>`),
		}
		transport.queueMessage(outputMsg)

		// Respond with PipelineState (Completed=4)
		plStateCompleted := &messages.Message{
			Destination: messages.DestinationClient,
			Type:        messages.MessageTypePipelineState,
			RunspaceID:  poolID,
			PipelineID:  pipelineID,
			Data:        []byte(`<Objs Version="1.1.0.1" xmlns="http://schemas.microsoft.com/powershell/2004/04"><I32>4</I32></Objs>`),
		}
		transport.queueMessage(plStateCompleted)

		errChan <- nil
	}()

	// CLIENT SIDE
	t.Log("Opening RunspacePool...")
	if err := pool.Open(ctx); err != nil {
		t.Fatalf("Client Open failed: %v", err)
	}

	t.Log("Creating Pipeline...")
	pl, err := pool.CreatePipeline("Get-Date")
	if err != nil {
		t.Fatalf("CreatePipeline failed: %v", err)
	}

	t.Log("Invoking Pipeline...")
	if err := pl.Invoke(ctx); err != nil {
		t.Fatalf("Invoke failed: %v", err)
	}

	// Capture output
	select {
	case out, ok := <-pl.Output():
		if !ok {
			t.Fatal("Output channel closed before receiving data")
		}
		if out == nil {
			t.Fatal("Received nil output message")
		}
		t.Logf("Received Output Payload: %s", string(out.Data))
	case <-ctx.Done():
		t.Fatal("Timed out waiting for output")
	}

	t.Log("Waiting for pipeline completion...")
	if err := pl.Wait(); err != nil {
		t.Fatalf("Pipeline Wait failed: %v", err)
	}

	// Check for server-side errors
	select {
	case err := <-errChan:
		if err != nil {
			t.Errorf("Mock Server encountered error: %v", err)
		}
	case <-time.After(1 * time.Second):
		t.Error("Timed out waiting for server completion")
	}

	t.Log("Test Passed!")
}
