package outofproc

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"sync"

	"github.com/google/uuid"
)

// Adapter bridges the OutOfProcess transport with the runspace package.
// It implements io.ReadWriter to provide raw fragment I/O while handling
// the OutOfProcess framing protocol internally.
type Adapter struct {
	transport    *Transport
	runspaceGuid uuid.UUID

	// Read state
	readBuf  bytes.Buffer
	readMu   sync.Mutex
	readCond *sync.Cond

	// Background reader state
	pending [][]byte
	closed  bool
	readErr error

	// Context for cancellation
	ctx    context.Context
	cancel context.CancelFunc

	// Callbacks for non-data packets
	onCommandAck func(pipelineGuid uuid.UUID)
	onCloseAck   func(psGuid uuid.UUID)
	onSignalAck  func(psGuid uuid.UUID)
}

// NewAdapter creates an adapter for a specific runspace.
// The adapter handles the OutOfProcess framing and provides raw fragment I/O.
func NewAdapter(transport *Transport, runspaceGuid uuid.UUID) *Adapter {
	ctx, cancel := context.WithCancel(context.Background())

	a := &Adapter{
		transport:    transport,
		runspaceGuid: runspaceGuid,
		pending:      make([][]byte, 0, 16),
		ctx:          ctx,
		cancel:       cancel,
	}
	a.readCond = sync.NewCond(&a.readMu)

	// Start background reader
	go a.readLoop()

	return a
}

// SetCommandAckHandler sets a callback for CommandAck packets.
func (a *Adapter) SetCommandAckHandler(handler func(pipelineGuid uuid.UUID)) {
	a.onCommandAck = handler
}

// SetCloseAckHandler sets a callback for CloseAck packets.
func (a *Adapter) SetCloseAckHandler(handler func(psGuid uuid.UUID)) {
	a.onCloseAck = handler
}

// SetSignalAckHandler sets a callback for SignalAck packets.
func (a *Adapter) SetSignalAckHandler(handler func(psGuid uuid.UUID)) {
	a.onSignalAck = handler
}

// readLoop reads packets from the transport and dispatches them.
func (a *Adapter) readLoop() {
	defer func() {
		a.readMu.Lock()
		a.closed = true
		a.readCond.Broadcast()
		a.readMu.Unlock()
	}()

	for {
		select {
		case <-a.ctx.Done():
			return
		default:
		}

		packet, err := a.transport.ReceivePacket()
		if err != nil {
			a.readMu.Lock()
			a.readErr = err
			a.readCond.Broadcast() // Wake up any waiting readers
			a.readMu.Unlock()
			return
		}

		switch packet.Type {
		case PacketTypeData:
			a.readMu.Lock()
			a.pending = append(a.pending, packet.Data)
			a.readCond.Signal()
			a.readMu.Unlock()

		case PacketTypeDataAck:
			// Acknowledgment - could be used for flow control

		case PacketTypeCommandAck:
			if a.onCommandAck != nil {
				a.onCommandAck(packet.PSGuid)
			}

		case PacketTypeCloseAck:
			if a.onCloseAck != nil {
				a.onCloseAck(packet.PSGuid)
			}
			// If it's the session close, we're done
			if IsSessionGUID(packet.PSGuid) {
				return
			}

		case PacketTypeSignalAck:
			if a.onSignalAck != nil {
				a.onSignalAck(packet.PSGuid)
			}
		}
	}
}

// Read implements io.Reader, returning fragment data.
// This blocks until data is available or the adapter is closed.
func (a *Adapter) Read(p []byte) (int, error) {
	a.readMu.Lock()
	defer a.readMu.Unlock()

	// First drain any buffered data
	if a.readBuf.Len() > 0 {
		return a.readBuf.Read(p)
	}

	// Wait for data
	for len(a.pending) == 0 && !a.closed && a.readErr == nil {
		a.readCond.Wait()
	}

	// Check for pending data FIRST (before errors)
	// This ensures we return all buffered data even after EOF/error
	if len(a.pending) > 0 {
		data := a.pending[0]
		a.pending = a.pending[1:]

		// If it fits in p, return directly
		if len(data) <= len(p) {
			copy(p, data)
			return len(data), nil
		}

		// Otherwise buffer the excess
		copy(p, data[:len(p)])
		a.readBuf.Write(data[len(p):])
		return len(p), nil
	}

	// No data available - check error conditions
	if a.readErr != nil {
		return 0, a.readErr
	}

	if a.closed {
		return 0, io.EOF
	}

	return 0, io.EOF
}

// Write implements io.Writer, sending fragment data.
// For session-level operations (runspace pool), data is sent with the NULL GUID.
// This is correct for SESSION_CAPABILITY, INIT_RUNSPACEPOOL, and other runspace messages.
func (a *Adapter) Write(p []byte) (int, error) {
	// Session-level data always uses NULL GUID in OutOfProcess protocol
	// The runspace.Pool only sends session-level data through this interface.
	// Pipeline-specific data is handled via SendPipelineData().
	err := a.transport.SendData(NullGUID, p)
	if err != nil {
		return 0, err
	}
	return len(p), nil
}

// SendCommand sends a Command packet for pipeline creation.
func (a *Adapter) SendCommand(pipelineGuid uuid.UUID) error {
	return a.transport.SendCommand(pipelineGuid)
}

// SendPipelineData sends fragment data for a specific pipeline.
func (a *Adapter) SendPipelineData(pipelineGuid uuid.UUID, data []byte) error {
	return a.transport.SendData(pipelineGuid, data)
}

// SendSignal sends a signal to a pipeline.
func (a *Adapter) SendSignal(pipelineGuid uuid.UUID) error {
	return a.transport.SendSignal(pipelineGuid)
}

// Close shuts down the adapter.
func (a *Adapter) Close() error {
	a.cancel()

	// Send close for the session
	if err := a.transport.SendClose(NullGUID); err != nil {
		return fmt.Errorf("send close: %w", err)
	}

	return nil
}

// Transport returns the underlying transport.
func (a *Adapter) Transport() *Transport {
	return a.transport
}
