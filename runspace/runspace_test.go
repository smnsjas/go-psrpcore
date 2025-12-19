package runspace

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jasonmfehr/go-psrp/fragments"
	"github.com/jasonmfehr/go-psrp/host"
	"github.com/jasonmfehr/go-psrp/messages"
)

// mockTransport is a mock transport for testing.
type mockTransport struct {
	readBuf     *bytes.Buffer
	writeBuf    *bytes.Buffer
	fragmenter  *fragments.Fragmenter
	readAssembler *fragments.Assembler
}

func newMockTransport() *mockTransport {
	return &mockTransport{
		readBuf:       &bytes.Buffer{},
		writeBuf:      &bytes.Buffer{},
		fragmenter:    fragments.NewFragmenter(32768),
		readAssembler: fragments.NewAssembler(),
	}
}

func (m *mockTransport) Read(p []byte) (n int, err error) {
	return m.readBuf.Read(p)
}

func (m *mockTransport) Write(p []byte) (n int, err error) {
	return m.writeBuf.Write(p)
}

// queueMessage adds a message to the read buffer as fragments.
func (m *mockTransport) queueMessage(msg *messages.Message) error {
	encoded, err := msg.Encode()
	if err != nil {
		return err
	}

	frags, err := m.fragmenter.Fragment(encoded)
	if err != nil {
		return err
	}

	for _, frag := range frags {
		fragData := frag.Encode()
		m.readBuf.Write(fragData)
	}

	return nil
}

// readMessage reads a message from the write buffer.
func (m *mockTransport) readMessage() (*messages.Message, error) {
	for {
		// Read fragment header
		header := make([]byte, fragments.HeaderSize)
		if _, err := io.ReadFull(m.writeBuf, header); err != nil {
			return nil, err
		}

		// Read blob data (big-endian)
		blobLen := binary.BigEndian.Uint32(header[17:21])
		fragData := make([]byte, fragments.HeaderSize+int(blobLen))
		copy(fragData[:fragments.HeaderSize], header)

		if blobLen > 0 {
			if _, err := io.ReadFull(m.writeBuf, fragData[fragments.HeaderSize:]); err != nil {
				return nil, err
			}
		}

		// Decode fragment
		frag, err := fragments.Decode(fragData)
		if err != nil {
			return nil, err
		}

		// Assemble message
		complete, msgData, err := m.readAssembler.Add(frag)
		if err != nil {
			return nil, err
		}

		if complete {
			return messages.Decode(msgData)
		}
	}
}

func TestNewPool(t *testing.T) {
	transport := newMockTransport()
	poolID := uuid.New()
	pool := New(transport, poolID)

	if pool.ID() != poolID {
		t.Errorf("expected pool ID %v, got %v", poolID, pool.ID())
	}

	if pool.State() != StateBeforeOpen {
		t.Errorf("expected state BeforeOpen, got %v", pool.State())
	}
}

func TestStateString(t *testing.T) {
	tests := []struct {
		state    State
		expected string
	}{
		{StateBeforeOpen, "BeforeOpen"},
		{StateOpening, "Opening"},
		{StateOpened, "Opened"},
		{StateClosing, "Closing"},
		{StateClosed, "Closed"},
		{StateBroken, "Broken"},
		{State(99), "Unknown(99)"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if got := tt.state.String(); got != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, got)
			}
		})
	}
}

func TestSetMinMaxRunspaces(t *testing.T) {
	transport := newMockTransport()
	pool := New(transport, uuid.New())

	// Test setting min runspaces
	if err := pool.SetMinRunspaces(2); err != nil {
		t.Errorf("SetMinRunspaces failed: %v", err)
	}

	// Test setting max runspaces
	if err := pool.SetMaxRunspaces(10); err != nil {
		t.Errorf("SetMaxRunspaces failed: %v", err)
	}

	// Test invalid values
	if err := pool.SetMinRunspaces(0); err == nil {
		t.Error("expected error for min runspaces = 0")
	}

	if err := pool.SetMaxRunspaces(0); err == nil {
		t.Error("expected error for max runspaces = 0")
	}

	// Test setting after state change
	pool.mu.Lock()
	pool.state = StateOpening
	pool.mu.Unlock()

	if err := pool.SetMinRunspaces(3); err != ErrInvalidState {
		t.Errorf("expected ErrInvalidState, got %v", err)
	}

	if err := pool.SetMaxRunspaces(3); err != ErrInvalidState {
		t.Errorf("expected ErrInvalidState, got %v", err)
	}
}

func TestStateTransitions(t *testing.T) {
	transport := newMockTransport()
	pool := New(transport, uuid.New())

	// Initial state
	if pool.State() != StateBeforeOpen {
		t.Fatalf("expected BeforeOpen, got %v", pool.State())
	}

	// Test invalid transitions
	tests := []struct {
		name          string
		currentState  State
		operation     func() error
		expectedError error
	}{
		{
			name:         "Open from BeforeOpen",
			currentState: StateBeforeOpen,
			operation: func() error {
				pool.mu.Lock()
				pool.state = StateBeforeOpen
				pool.mu.Unlock()
				return nil // Will test Open separately
			},
			expectedError: nil,
		},
		{
			name:         "Close from Closed",
			currentState: StateClosed,
			operation: func() error {
				pool.mu.Lock()
				pool.state = StateClosed
				pool.mu.Unlock()
				ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
				defer cancel()
				return pool.Close(ctx)
			},
			expectedError: nil, // Close is idempotent
		},
		{
			name:         "Close from BeforeOpen",
			currentState: StateBeforeOpen,
			operation: func() error {
				pool.mu.Lock()
				pool.state = StateBeforeOpen
				pool.mu.Unlock()
				ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
				defer cancel()
				return pool.Close(ctx)
			},
			expectedError: ErrInvalidState,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.operation()
			if tt.expectedError == nil {
				if err != nil && err != tt.expectedError {
					t.Errorf("expected no error, got %v", err)
				}
			} else {
				if err == nil {
					t.Errorf("expected error %v, got nil", tt.expectedError)
				}
			}
		})
	}
}

func TestOpenSuccess(t *testing.T) {
	transport := newMockTransport()
	poolID := uuid.New()
	pool := New(transport, poolID)

	// Queue server responses
	capabilityData := []byte(`<Obj RefId="0"><MS><Version N="protocolversion">2.3</Version></MS></Obj>`)
	stateData := []byte(`<Obj RefId="0"><MS><I32 N="RunspaceState">2</I32></MS></Obj>`)

	// Queue SESSION_CAPABILITY response
	capMsg := &messages.Message{
		Destination: messages.DestinationClient,
		Type:        messages.MessageTypeSessionCapability,
		RunspaceID:  poolID,
		PipelineID:  uuid.Nil,
		Data:        capabilityData,
	}
	if err := transport.queueMessage(capMsg); err != nil {
		t.Fatalf("failed to queue capability message: %v", err)
	}

	// Queue RUNSPACEPOOL_STATE response
	stateMsg := &messages.Message{
		Destination: messages.DestinationClient,
		Type:        messages.MessageTypeRunspacePoolState,
		RunspaceID:  poolID,
		PipelineID:  uuid.Nil,
		Data:        stateData,
	}
	if err := transport.queueMessage(stateMsg); err != nil {
		t.Fatalf("failed to queue state message: %v", err)
	}

	// Open pool
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := pool.Open(ctx); err != nil {
		t.Fatalf("Open failed: %v", err)
	}

	// Verify final state
	if pool.State() != StateOpened {
		t.Errorf("expected state Opened, got %v", pool.State())
	}

	// Verify messages sent
	// Should have sent SESSION_CAPABILITY and INIT_RUNSPACEPOOL
	msg1, err := transport.readMessage()
	if err != nil {
		t.Fatalf("failed to read first message: %v", err)
	}
	if msg1.Type != messages.MessageTypeSessionCapability {
		t.Errorf("expected SESSION_CAPABILITY, got %v", msg1.Type)
	}

	msg2, err := transport.readMessage()
	if err != nil {
		t.Fatalf("failed to read second message: %v", err)
	}
	if msg2.Type != messages.MessageTypeInitRunspacePool {
		t.Errorf("expected INIT_RUNSPACEPOOL, got %v", msg2.Type)
	}
}

func TestOpenAlreadyOpen(t *testing.T) {
	transport := newMockTransport()
	pool := New(transport, uuid.New())

	// Set state to Opened
	pool.mu.Lock()
	pool.state = StateOpened
	pool.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := pool.Open(ctx)
	if err != ErrAlreadyOpen {
		t.Errorf("expected ErrAlreadyOpen, got %v", err)
	}
}

func TestOpenFromInvalidState(t *testing.T) {
	tests := []struct {
		name          string
		state         State
		expectedError error
	}{
		{"Closed", StateClosed, ErrClosed},
		{"Broken", StateBroken, ErrBroken},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := newMockTransport()
			pool := New(transport, uuid.New())

			pool.mu.Lock()
			pool.state = tt.state
			pool.mu.Unlock()

			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()

			err := pool.Open(ctx)
			if err != tt.expectedError {
				t.Errorf("expected %v, got %v", tt.expectedError, err)
			}
		})
	}
}

func TestCloseSuccess(t *testing.T) {
	transport := newMockTransport()
	pool := New(transport, uuid.New())

	// Set state to Opened
	pool.mu.Lock()
	pool.state = StateOpened
	pool.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	if err := pool.Close(ctx); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// Verify final state
	if pool.State() != StateClosed {
		t.Errorf("expected state Closed, got %v", pool.State())
	}
}

func TestCloseIdempotent(t *testing.T) {
	transport := newMockTransport()
	pool := New(transport, uuid.New())

	pool.mu.Lock()
	pool.state = StateOpened
	pool.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// First close
	if err := pool.Close(ctx); err != nil {
		t.Fatalf("first Close failed: %v", err)
	}

	// Second close should succeed (idempotent)
	if err := pool.Close(ctx); err != nil {
		t.Errorf("second Close failed: %v", err)
	}
}

func TestSetBroken(t *testing.T) {
	transport := newMockTransport()
	pool := New(transport, uuid.New())

	// Can transition to Broken from any state
	states := []State{StateBeforeOpen, StateOpening, StateOpened, StateClosing, StateClosed}

	for _, state := range states {
		pool.mu.Lock()
		pool.state = state
		pool.mu.Unlock()

		pool.setBroken()

		if pool.State() != StateBroken {
			t.Errorf("expected state Broken after setBroken from %v, got %v", state, pool.State())
		}

		// Reset for next test
		pool.mu.Lock()
		pool.state = StateBeforeOpen
		pool.mu.Unlock()
	}
}

func TestContextCancellation(t *testing.T) {
	transport := newMockTransport()
	pool := New(transport, uuid.New())

	// Create a context that's already cancelled
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := pool.Open(ctx)
	if err == nil {
		t.Error("expected error from cancelled context")
	}

	// Pool should be in Broken state after failed open
	if pool.State() != StateBroken {
		t.Errorf("expected state Broken after failed open, got %v", pool.State())
	}
}

func TestSetHost(t *testing.T) {
	transport := newMockTransport()
	pool := New(transport, uuid.New())

	// Create a custom host
	customHost := host.NewNullHost()

	// Set host before opening
	if err := pool.SetHost(customHost); err != nil {
		t.Errorf("SetHost failed: %v", err)
	}

	// Try setting host after state change (should fail)
	pool.mu.Lock()
	pool.state = StateOpening
	pool.mu.Unlock()

	if err := pool.SetHost(customHost); err != ErrInvalidState {
		t.Errorf("expected ErrInvalidState, got %v", err)
	}
}

func TestHostCallbackDuringOpen(t *testing.T) {
	transport := newMockTransport()
	poolID := uuid.New()
	pool := New(transport, poolID)

	// Queue server responses including a host callback
	capabilityData := []byte(`<Obj RefId="0"><MS><Version N="protocolversion">2.3</Version></MS></Obj>`)

	// Queue SESSION_CAPABILITY response
	capMsg := &messages.Message{
		Destination: messages.DestinationClient,
		Type:        messages.MessageTypeSessionCapability,
		RunspaceID:  poolID,
		PipelineID:  uuid.Nil,
		Data:        capabilityData,
	}
	if err := transport.queueMessage(capMsg); err != nil {
		t.Fatalf("failed to queue capability message: %v", err)
	}

	// Queue a RUNSPACEPOOL_HOST_CALL (WriteErrorLine) before RUNSPACEPOOL_STATE
	hostCall := &host.RemoteHostCall{
		CallID:           1,
		MethodID:         host.MethodIDWriteErrorLine,
		MethodParameters: []interface{}{"test error"},
	}
	hostCallData, err := host.EncodeRemoteHostCall(hostCall)
	if err != nil {
		t.Fatalf("failed to encode host call: %v", err)
	}
	hostCallMsg := &messages.Message{
		Destination: messages.DestinationClient,
		Type:        messages.MessageTypeRunspaceHostCall,
		RunspaceID:  poolID,
		PipelineID:  uuid.Nil,
		Data:        hostCallData,
	}
	if err := transport.queueMessage(hostCallMsg); err != nil {
		t.Fatalf("failed to queue host call message: %v", err)
	}

	// Queue RUNSPACEPOOL_STATE response
	stateData := []byte(`<Obj RefId="0"><MS><I32 N="RunspaceState">2</I32></MS></Obj>`)
	stateMsg := &messages.Message{
		Destination: messages.DestinationClient,
		Type:        messages.MessageTypeRunspacePoolState,
		RunspaceID:  poolID,
		PipelineID:  uuid.Nil,
		Data:        stateData,
	}
	if err := transport.queueMessage(stateMsg); err != nil {
		t.Fatalf("failed to queue state message: %v", err)
	}

	// Open pool (should handle host callback automatically)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := pool.Open(ctx); err != nil {
		t.Fatalf("Open failed: %v", err)
	}

	// Verify pool is opened
	if pool.State() != StateOpened {
		t.Errorf("expected state Opened, got %v", pool.State())
	}

	// Verify host callback response was sent
	// Should have: SESSION_CAPABILITY, INIT_RUNSPACEPOOL, RUNSPACEPOOL_HOST_RESPONSE
	msg1, err := transport.readMessage()
	if err != nil {
		t.Fatalf("failed to read first message: %v", err)
	}
	if msg1.Type != messages.MessageTypeSessionCapability {
		t.Errorf("expected SESSION_CAPABILITY, got %v", msg1.Type)
	}

	msg2, err := transport.readMessage()
	if err != nil {
		t.Fatalf("failed to read second message: %v", err)
	}
	if msg2.Type != messages.MessageTypeInitRunspacePool {
		t.Errorf("expected INIT_RUNSPACEPOOL, got %v", msg2.Type)
	}

	// Verify RUNSPACEPOOL_HOST_RESPONSE was sent
	msg3, err := transport.readMessage()
	if err != nil {
		t.Fatalf("failed to read third message: %v", err)
	}
	if msg3.Type != messages.MessageTypeRunspaceHostResponse {
		t.Errorf("expected RUNSPACEPOOL_HOST_RESPONSE, got %v", msg3.Type)
	}

	// Decode and verify the response
	response, err := host.DecodeRemoteHostResponse(msg3.Data)
	if err != nil {
		t.Fatalf("failed to decode host response: %v", err)
	}
	if response.CallID != 1 {
		t.Errorf("expected CallID 1, got %d", response.CallID)
	}
	if response.ExceptionRaised {
		t.Errorf("expected no exception, got: %v", response.ReturnValue)
	}
}
