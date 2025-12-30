package outofproc

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Adapter bridges the OutOfProcess transport with the runspace package.
// It implements io.ReadWriter to provide raw fragment I/O while handling
// the OutOfProcess framing protocol internally.
type Adapter struct {
	transport    *Transport
	runspaceGUID uuid.UUID

	// Read state
	readMu   sync.Mutex
	notifyCh chan struct{}

	// Background reader state
	pending [][]byte
	closed  bool
	readErr error

	// Context for cancellation
	ctx    context.Context
	cancel context.CancelFunc

	// Callbacks for non-data packets (protected by handlerMu)
	handlerMu    sync.RWMutex
	onCommandAck func(pipelineGUID uuid.UUID)
	onCloseAck   func(psGuid uuid.UUID)
	onSignalAck  func(psGuid uuid.UUID)
}

// NewAdapter creates an adapter for a specific runspace.
// The adapter handles the OutOfProcess framing and provides raw fragment I/O.
func NewAdapter(transport *Transport, runspaceGUID uuid.UUID) *Adapter {
	ctx, cancel := context.WithCancel(context.Background())

	a := &Adapter{
		transport:    transport,
		runspaceGUID: runspaceGUID,
		pending:      make([][]byte, 0, 16),
		notifyCh:     make(chan struct{}, 1),
		ctx:          ctx,
		cancel:       cancel,
	}

	// Start background reader
	go a.readLoop()

	return a
}

// SetCommandAckHandler sets a callback for CommandAck packets.
func (a *Adapter) SetCommandAckHandler(handler func(pipelineGUID uuid.UUID)) {
	a.handlerMu.Lock()
	defer a.handlerMu.Unlock()
	a.onCommandAck = handler
}

// SetCloseAckHandler sets a callback for CloseAck packets.
func (a *Adapter) SetCloseAckHandler(handler func(psGuid uuid.UUID)) {
	a.handlerMu.Lock()
	defer a.handlerMu.Unlock()
	a.onCloseAck = handler
}

// SetSignalAckHandler sets a callback for SignalAck packets.
func (a *Adapter) SetSignalAckHandler(handler func(psGuid uuid.UUID)) {
	a.handlerMu.Lock()
	defer a.handlerMu.Unlock()
	a.onSignalAck = handler
}

// readLoop reads packets from the transport and dispatches them.
func (a *Adapter) readLoop() {
	defer func() {
		a.readMu.Lock()
		a.closed = true

		a.readMu.Unlock()
		// Signaling cleanup
		select {
		case a.notifyCh <- struct{}{}:
		default:
		}
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
			a.readMu.Unlock()
			// Signal error
			select {
			case a.notifyCh <- struct{}{}:
			default:
			}
			return
		}

		switch packet.Type {
		case PacketTypeData:
			// MS-PSRP OutOfProcess: The receiver MUST accept the message and respond with a <DataAck> message.
			if err := a.transport.SendDataAck(packet.PSGuid); err != nil {
				_ = err // Best-effort Ack, continue on error
			}

			a.readMu.Lock()
			a.pending = append(a.pending, packet.Data)

			a.readMu.Unlock()
			// Signal data availability
			select {
			case a.notifyCh <- struct{}{}:
			default:
			}

		case PacketTypeDataAck:
			// No-op

		case PacketTypeCommandAck:
			a.handlerMu.RLock()
			handler := a.onCommandAck
			a.handlerMu.RUnlock()
			if handler != nil {
				handler(packet.PSGuid)
			}

		case PacketTypeCloseAck:
			a.handlerMu.RLock()
			handler := a.onCloseAck
			a.handlerMu.RUnlock()
			if handler != nil {
				handler(packet.PSGuid)
			}

		case PacketTypeSignalAck:
			a.handlerMu.RLock()
			handler := a.onSignalAck
			a.handlerMu.RUnlock()
			if handler != nil {
				handler(packet.PSGuid)
			}

		case PacketTypeClose:
			if err := a.transport.SendCloseAck(packet.PSGuid); err != nil {
				_ = err // Best-effort Ack, continue on error
			}

		case PacketTypeSignal:
			if err := a.transport.SendSignalAck(packet.PSGuid); err != nil {
				_ = err // Best-effort Ack, continue on error
			}
		}
	}
}

// Read reads up to len(p) bytes from the pending buffers.
// It blocks until data is available, an error occurs, or the context is cancelled.
func (a *Adapter) Read(p []byte) (n int, err error) {
	a.readMu.Lock()
	defer a.readMu.Unlock()

	// Wait for data with periodic wakeup to check for closure
	// This prevents hanging if a signal is missed or connection drops silently
	deadline := time.Now().Add(30 * time.Second)

	for len(a.pending) == 0 && !a.closed && a.readErr == nil {
		// Release lock while waiting
		a.readMu.Unlock()

		// Use notifyCh for instant wake-up, fallback to timer for periodic checks
		timer := time.NewTimer(1 * time.Second)
		select {
		case <-a.notifyCh:
			// Data arrived or state changed
			if !timer.Stop() {
				<-timer.C
			}
		case <-timer.C:
			// Timer fired, loop around
		case <-a.ctx.Done():
			timer.Stop()
			a.readMu.Lock()
			return 0, a.ctx.Err()
		}

		a.readMu.Lock()

		if time.Now().After(deadline) {
			// One last check if data arrived
			if len(a.pending) > 0 || a.closed || a.readErr != nil {
				break
			}
			return 0, fmt.Errorf("read timeout: no data received in 30s")
		}
	}

	if len(a.pending) > 0 {
		// Copy data from the first pending buffer
		n = copy(p, a.pending[0])
		if n == len(a.pending[0]) {
			// Consumed the entire buffer, remove it
			a.pending = a.pending[1:]
		} else {
			// Partial read, update the pending buffer
			a.pending[0] = a.pending[0][n:]
		}
		return n, nil
	}

	if a.readErr != nil {
		return 0, a.readErr
	}

	if a.closed {
		return 0, io.EOF
	}

	return 0, nil
}

// Write implements io.Writer, sending fragment data.
// For session-level operations (runspace pool), data is sent with the NULL GUID.
func (a *Adapter) Write(p []byte) (int, error) {
	// Throttle writes slightly to avoid overwhelming the vmicvmsession service,
	// which is known to be sensitive to packet flooding (causing potential hangs).
	time.Sleep(2 * time.Millisecond)

	err := a.transport.SendData(NullGUID, p)
	if err != nil {
		return 0, err
	}
	return len(p), nil
}

// SendCommand sends a Command packet for pipeline creation.
func (a *Adapter) SendCommand(pipelineGUID uuid.UUID) error {
	return a.transport.SendCommand(pipelineGUID)
}

// SendPipelineData sends fragment data for a specific pipeline.
func (a *Adapter) SendPipelineData(pipelineGUID uuid.UUID, data []byte) error {
	// Throttle writes slightly to avoid overwhelming the VM service
	time.Sleep(2 * time.Millisecond)
	return a.transport.SendData(pipelineGUID, data)
}

// SendSignal sends a signal to a pipeline.
func (a *Adapter) SendSignal(pipelineGUID uuid.UUID) error {
	return a.transport.SendSignal(pipelineGUID)
}

// Close shuts down the adapter.
//
// IMPORTANT: We intentionally do NOT send OutOfProc Close packets here.
// The PSRP-level RUNSPACEPOOL_STATE(Closed) message (sent by Pool.Close())
// is sufficient for the server to clean up. Sending additional OutOfProc
// Close packets after the PSRP close can confuse the vmicvmsession service
// and cause subsequent connections to hang.
//
// This matches the behavior of "killing" the client, which we observed
// successfully clears the server state.
func (a *Adapter) Close() error {
	// 1. Give server a moment to process the PSRP close (which happened in Pool.Close)
	time.Sleep(200 * time.Millisecond)

	// 2. Just cancel the context to stop readLoop and release resources.
	// We rely on the subsequent socket close (in backend) to signal EOF to server.
	a.cancel()

	// 3. Brief wait for readLoop to exit
	time.Sleep(100 * time.Millisecond)

	return nil
}

// ClosePipeline sends a Close message for a specific pipeline and waits for CloseAck.
// NOTE: This is typically not needed - pipelines complete naturally and the server
// cleans them up. Only use this if you need to forcibly terminate a running pipeline.
func (a *Adapter) ClosePipeline(pipelineID uuid.UUID) error {
	closeAckCh := make(chan struct{}, 1)

	originalHandler := a.onCloseAck
	a.SetCloseAckHandler(func(psGuid uuid.UUID) {
		if psGuid == pipelineID {
			select {
			case closeAckCh <- struct{}{}:
			default:
			}
		}
		if originalHandler != nil {
			originalHandler(psGuid)
		}
	})
	defer a.SetCloseAckHandler(originalHandler)

	if err := a.transport.SendClose(pipelineID); err != nil {
		return fmt.Errorf("send pipeline close: %w", err)
	}

	select {
	case <-closeAckCh:
	case <-time.After(2 * time.Second):
	case <-a.ctx.Done():
	}

	return nil
}

// Transport returns the underlying transport.
func (a *Adapter) Transport() *Transport {
	return a.transport
}
