//nolint:lll // Test data XML strings are inherently long
package runspace

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/smnsjas/go-psrpcore/fragments"
	"github.com/smnsjas/go-psrpcore/host"
	"github.com/smnsjas/go-psrpcore/messages"
	"github.com/smnsjas/go-psrpcore/pipeline"
	"github.com/smnsjas/go-psrpcore/serialization"
)

// mockTransport is a mock transport for testing.
type mockTransport struct {
	readBuf       *bytes.Buffer
	writeBuf      *bytes.Buffer
	fragmenter    *fragments.Fragmenter
	readAssembler *fragments.Assembler
}

func newMockTransport() *mockTransport {
	return &mockTransport{
		readBuf:       &bytes.Buffer{},
		writeBuf:      &bytes.Buffer{},
		fragmenter:    fragments.NewFragmenter(DefaultMaxFragmentSize),
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
		fragData, err := frag.Encode()
		if err != nil {
			return err
		}
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
	capabilityData := []byte(`<?xml version="1.0" encoding="utf-8"?>
<Objs Version="1.1.0.1" xmlns="http://schemas.microsoft.com/powershell/2004/04">
  <Obj RefId="0">
    <MS>
      <S N="protocolversion">2.3</S>
    </MS>
  </Obj>
</Objs>`)
	stateData := []byte(`<?xml version="1.0" encoding="utf-8"?>
<Objs Version="1.1.0.1" xmlns="http://schemas.microsoft.com/powershell/2004/04">
  <I32>2</I32>
</Objs>`)

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
	t.Cleanup(cancel)

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
	t.Cleanup(cancel)

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
	t.Cleanup(cancel)

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
	t.Cleanup(cancel)

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

// TestDispatchLoopTransportError tests that transport errors in dispatchLoop
// properly transition the pool to broken state and clean up resources.
func TestDispatchLoopTransportError(t *testing.T) {
	// Use io.Pipe to simulate a closeable connection
	serverReader, clientWriter := io.Pipe()
	clientReader, serverWriter := io.Pipe()

	// Create a wrapper that implements io.ReadWriter
	transport := &struct {
		io.Reader
		io.Writer
	}{
		Reader: clientReader,
		Writer: clientWriter,
	}

	poolID := uuid.New()
	pool := New(transport, poolID)

	// Helper to send messages from "server" side
	sendServerMessage := func(msg *messages.Message) error {
		encoded, err := msg.Encode()
		if err != nil {
			return err
		}
		fragmenter := fragments.NewFragmenter(DefaultMaxFragmentSize)
		frags, err := fragmenter.Fragment(encoded)
		if err != nil {
			return err
		}
		for _, frag := range frags {
			fragData, err := frag.Encode()
			if err != nil {
				return err
			}
			if _, err := serverWriter.Write(fragData); err != nil {
				return err
			}
		}
		return nil
	}

	// Start a goroutine to handle the handshake
	go func() {
		// Read and discard SESSION_CAPABILITY from client
		header := make([]byte, fragments.HeaderSize)
		_, _ = io.ReadFull(serverReader, header)
		blobLen := binary.BigEndian.Uint32(header[17:21])
		_, _ = io.ReadFull(serverReader, make([]byte, blobLen))

		// Send SESSION_CAPABILITY response
		capabilityData := []byte(`<?xml version="1.0" encoding="utf-8"?>
<Objs Version="1.1.0.1" xmlns="http://schemas.microsoft.com/powershell/2004/04">
  <Obj RefId="0">
    <MS>
      <S N="protocolversion">2.3</S>
    </MS>
  </Obj>
</Objs>`)
		capMsg := &messages.Message{
			Destination: messages.DestinationClient,
			Type:        messages.MessageTypeSessionCapability,
			RunspaceID:  poolID,
			Data:        capabilityData,
		}
		_ = sendServerMessage(capMsg)

		// Read and discard INIT_RUNSPACEPOOL from client
		_, _ = io.ReadFull(serverReader, header)
		blobLen = binary.BigEndian.Uint32(header[17:21])
		_, _ = io.ReadFull(serverReader, make([]byte, blobLen))

		// Send RUNSPACEPOOL_STATE response
		stateData := []byte(`<?xml version="1.0" encoding="utf-8"?>
<Objs Version="1.1.0.1" xmlns="http://schemas.microsoft.com/powershell/2004/04">
  <I32>2</I32>
</Objs>`)
		stateMsg := &messages.Message{
			Destination: messages.DestinationClient,
			Type:        messages.MessageTypeRunspacePoolState,
			RunspaceID:  poolID,
			Data:        stateData,
		}
		_ = sendServerMessage(stateMsg)
	}()

	// Open pool
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := pool.Open(ctx); err != nil {
		t.Fatalf("Open failed: %v", err)
	}

	// Verify pool is opened
	if pool.State() != StateOpened {
		t.Fatalf("expected state Opened, got %v", pool.State())
	}

	// Create a pipeline
	pl, err := pool.CreatePipeline("Get-Date")
	if err != nil {
		t.Fatalf("CreatePipeline failed: %v", err)
	}

	// Verify pipeline exists in the map
	// Verify pipeline exists in the map
	if _, ok := pool.pipelines.Load(pl.ID()); !ok {
		t.Fatal("Pipeline not found in pool's pipeline map")
	}

	// Close the server side to simulate a transport error
	// This will cause the next receiveMessage to fail with EOF
	_ = serverWriter.Close()
	_ = clientReader.Close()

	// Wait for pool to detect the error and transition to broken
	select {
	case <-pool.doneCh:
		// Expected - pool detected error and closed doneCh
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for pool to detect transport error")
	}

	// Verify pool is in broken state
	if pool.State() != StateBroken {
		t.Errorf("expected state Broken after transport error, got %v", pool.State())
	}

	// Verify the pipeline was transitioned to failed
	if pl.State() != pipeline.StateFailed {
		t.Errorf("expected pipeline state Failed, got %v", pl.State())
	}

	// Verify pipeline.Wait() returns with an error
	if err := pl.Wait(); err == nil {
		t.Error("expected pipeline.Wait() to return error after pool broke")
	}

	// Verify cleanupError was stored
	if pool.cleanupError == nil {
		t.Error("expected cleanupError to be set")
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
	capabilityData := []byte(`<?xml version="1.0" encoding="utf-8"?>
<Objs Version="1.1.0.1" xmlns="http://schemas.microsoft.com/powershell/2004/04">
  <Obj RefId="0">
    <MS>
      <S N="protocolversion">2.3</S>
    </MS>
  </Obj>
</Objs>`)

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
	stateData := []byte(`<?xml version="1.0" encoding="utf-8"?>
<Objs Version="1.1.0.1" xmlns="http://schemas.microsoft.com/powershell/2004/04">
  <I32>2</I32>
</Objs>`)
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
	t.Cleanup(cancel)

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

func TestParseCapabilityData(t *testing.T) {
	tests := []struct {
		name     string
		data     []byte
		expected *capabilityData
		wantErr  bool
	}{
		{
			name: "valid capability with all fields",
			data: []byte(`<?xml version="1.0" encoding="utf-8"?>
<Objs Version="1.1.0.1" xmlns="http://schemas.microsoft.com/powershell/2004/04">
  <Obj RefId="0">
    <MS>
      <S N="protocolversion">2.3</S>
      <S N="PSVersion">5.1.0.0</S>
      <S N="SerializationVersion">1.1.0.1</S>
    </MS>
  </Obj>
</Objs>`),
			expected: &capabilityData{
				ProtocolVersion:      "2.3",
				PSVersion:            "5.1.0.0",
				SerializationVersion: "1.1.0.1",
			},
			wantErr: false,
		},
		{
			name: "minimal capability with only protocol version",
			data: []byte(`<?xml version="1.0" encoding="utf-8"?>
<Objs Version="1.1.0.1" xmlns="http://schemas.microsoft.com/powershell/2004/04">
  <Obj RefId="0">
    <MS>
      <S N="protocolversion">2.2</S>
    </MS>
  </Obj>
</Objs>`),
			expected: &capabilityData{
				ProtocolVersion: "2.2",
			},
			wantErr: false,
		},
		{
			name:    "empty data",
			data:    []byte(`<?xml version="1.0" encoding="utf-8"?><Objs Version="1.1.0.1" xmlns="http://schemas.microsoft.com/powershell/2004/04"></Objs>`),
			wantErr: true,
		},
		{
			name:    "invalid XML",
			data:    []byte(`not xml`),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseCapabilityData(tt.data)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseCapabilityData() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}
			if result.ProtocolVersion != tt.expected.ProtocolVersion {
				t.Errorf("ProtocolVersion = %v, want %v", result.ProtocolVersion, tt.expected.ProtocolVersion)
			}
			if result.PSVersion != tt.expected.PSVersion {
				t.Errorf("PSVersion = %v, want %v", result.PSVersion, tt.expected.PSVersion)
			}
			if result.SerializationVersion != tt.expected.SerializationVersion {
				t.Errorf("SerializationVersion = %v, want %v", result.SerializationVersion, tt.expected.SerializationVersion)
			}
		})
	}
}

func TestParseRunspacePoolState(t *testing.T) {
	tests := []struct {
		name     string
		data     []byte
		expected *runspacePoolStateInfo
		wantErr  bool
	}{
		{
			name: "state Opened (2)",
			data: []byte(`<?xml version="1.0" encoding="utf-8"?>
<Objs Version="1.1.0.1" xmlns="http://schemas.microsoft.com/powershell/2004/04">
  <I32>2</I32>
</Objs>`),
			expected: &runspacePoolStateInfo{
				State: messages.RunspacePoolStateOpened,
			},
			wantErr: false,
		},
		{
			name: "state with min/max runspaces",
			data: []byte(`<?xml version="1.0" encoding="utf-8"?>
<Objs Version="1.1.0.1" xmlns="http://schemas.microsoft.com/powershell/2004/04">
  <I32>2</I32>
  <Obj RefId="0">
    <MS>
      <I32 N="MinRunspaces">1</I32>
      <I32 N="MaxRunspaces">5</I32>
    </MS>
  </Obj>
</Objs>`),
			expected: &runspacePoolStateInfo{
				State:        messages.RunspacePoolStateOpened,
				MinRunspaces: 1,
				MaxRunspaces: 5,
			},
			wantErr: false,
		},
		{
			name: "state Broken (5)",
			data: []byte(`<?xml version="1.0" encoding="utf-8"?>
<Objs Version="1.1.0.1" xmlns="http://schemas.microsoft.com/powershell/2004/04">
  <I32>5</I32>
</Objs>`),
			expected: &runspacePoolStateInfo{
				State: messages.RunspacePoolStateBroken,
			},
			wantErr: false,
		},
		{
			name:    "empty data",
			data:    []byte(`<?xml version="1.0" encoding="utf-8"?><Objs Version="1.1.0.1" xmlns="http://schemas.microsoft.com/powershell/2004/04"></Objs>`),
			wantErr: true,
		},
		{
			name: "invalid type (string instead of int32)",
			data: []byte(`<?xml version="1.0" encoding="utf-8"?>
<Objs Version="1.1.0.1" xmlns="http://schemas.microsoft.com/powershell/2004/04">
  <S>not a number</S>
</Objs>`),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseRunspacePoolState(tt.data)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseRunspacePoolState() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}
			if result.State != tt.expected.State {
				t.Errorf("State = %v, want %v", result.State, tt.expected.State)
			}
			if result.MinRunspaces != tt.expected.MinRunspaces {
				t.Errorf("MinRunspaces = %v, want %v", result.MinRunspaces, tt.expected.MinRunspaces)
			}
			if result.MaxRunspaces != tt.expected.MaxRunspaces {
				t.Errorf("MaxRunspaces = %v, want %v", result.MaxRunspaces, tt.expected.MaxRunspaces)
			}
		})
	}
}

func TestReceiveSessionCapability_VersionValidation(t *testing.T) {
	tests := []struct {
		name          string
		protocolVer   string
		wantErr       bool
		expectedError string
	}{
		{
			name:        "compatible version 2.3",
			protocolVer: "2.3",
			wantErr:     false,
		},
		{
			name:        "compatible version 2.2",
			protocolVer: "2.2",
			wantErr:     false,
		},
		{
			name:        "compatible version 2.0",
			protocolVer: "2.0",
			wantErr:     false,
		},
		{
			name:          "incompatible version 1.0",
			protocolVer:   "1.0",
			wantErr:       true,
			expectedError: "incompatible protocol version",
		},
		{
			name:          "incompatible version 3.0",
			protocolVer:   "3.0",
			wantErr:       true,
			expectedError: "incompatible protocol version",
		},
		{
			name:          "no version provided",
			protocolVer:   "",
			wantErr:       true,
			expectedError: "server did not provide protocol version",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := newMockTransport()
			poolID := uuid.New()
			pool := New(transport, poolID)

			// Create capability data
			var capabilityData []byte
			if tt.protocolVer != "" {
				capabilityData = []byte(`<?xml version="1.0" encoding="utf-8"?>
<Objs Version="1.1.0.1" xmlns="http://schemas.microsoft.com/powershell/2004/04">
  <Obj RefId="0">
    <MS>
      <S N="protocolversion">` + tt.protocolVer + `</S>
    </MS>
  </Obj>
</Objs>`)
			} else {
				capabilityData = []byte(`<?xml version="1.0" encoding="utf-8"?>
<Objs Version="1.1.0.1" xmlns="http://schemas.microsoft.com/powershell/2004/04">
  <Obj RefId="0">
    <MS>
    </MS>
  </Obj>
</Objs>`)
			}

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

			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()

			err := pool.receiveSessionCapability(ctx)
			if (err != nil) != tt.wantErr {
				t.Errorf("receiveSessionCapability() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr && tt.expectedError != "" {
				if err == nil || !bytes.Contains([]byte(err.Error()), []byte(tt.expectedError)) {
					t.Errorf("expected error containing %q, got %v", tt.expectedError, err)
				}
			}

			if !tt.wantErr {
				// Verify the protocol version was stored
				pool.mu.RLock()
				if pool.serverProtocolVersion != tt.protocolVer {
					t.Errorf("serverProtocolVersion = %v, want %v", pool.serverProtocolVersion, tt.protocolVer)
				}
				pool.mu.RUnlock()
			}
		})
	}
}

// --- Merged from metadata_test.go ---

// simpleMockTransport is a simple mock that enables testing of GetCommandMetadata
type simpleMockTransport struct {
	sentData []byte
}

func (m *simpleMockTransport) Write(p []byte) (n int, err error) {
	m.sentData = append(m.sentData, p...)
	return len(p), nil
}

func (m *simpleMockTransport) Read(_ []byte) (n int, err error) {
	return 0, nil
}

func TestGetCommandMetadata(t *testing.T) {
	transport := &simpleMockTransport{}

	pool := New(transport, uuid.New())
	pool.state = StateOpened

	meta := &serialization.PSObject{
		Properties: map[string]interface{}{
			"Name":        "Get-Process",
			"CommandType": int32(8), // Cmdlet
			"Parameters": map[string]interface{}{
				"Id": &serialization.PSObject{
					Properties: map[string]interface{}{
						"ParameterType": "System.Int32",
					},
				},
			},
		},
	}

	serializer := serialization.NewSerializer()
	replyData, _ := serializer.Serialize(meta)

	go func() {
		time.Sleep(50 * time.Millisecond)

		// Note: Per MS-PSRP spec (section 3.1.4.5), GET_COMMAND_METADATA responses
		// come via PIPELINE_OUTPUT messages containing CommandMetadata objects.
		// This test simulates the channel-based response mechanism used by the
		// current implementation. A full implementation would use a pipeline
		// to receive PIPELINE_OUTPUT messages.
		replyMsg := &messages.Message{
			Type: messages.MessageTypePipelineOutput,
			Data: replyData,
		}
		pool.metadataCh <- replyMsg
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	results, err := pool.GetCommandMetadata(ctx, []string{"Get-Process"})
	if err != nil {
		t.Fatalf("GetCommandMetadata failed: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("Expected 1 result, got %d", len(results))
	}

	cmd := results[0]
	if cmd.Name != "Get-Process" {
		t.Errorf("Expected Name 'Get-Process', got '%s'", cmd.Name)
	}
	if cmd.CommandType != 8 {
		t.Errorf("Expected CommandType 8, got %d", cmd.CommandType)
	}

	param, ok := cmd.Parameters["Id"]
	if !ok {
		t.Error("Expected parameter 'Id' not found")
	}
	if param.Type != "System.Int32" {
		t.Errorf("Expected Id type 'System.Int32', got '%s'", param.Type)
	}
}

// --- Merged from memory_verification_test.go ---

// blockingMockReadWriter is a mock that blocks on Read to simulate idle connection
type blockingMockReadWriter struct{}

func (m *blockingMockReadWriter) Read(_ []byte) (n int, err error) {
	select {} // Block indefinitely
}

func (m *blockingMockReadWriter) Write(p []byte) (n int, err error) {
	return len(p), nil
}

func TestRunspacePool_PipelineCleanup(t *testing.T) {
	pool := New(&blockingMockReadWriter{}, uuid.New())
	pool.state = StateOpened

	pl, err := pool.CreatePipeline("Get-Date")
	if err != nil {
		t.Fatalf("CreatePipeline failed: %v", err)
	}

	if _, ok := pool.pipelines.Load(pl.ID()); !ok {
		t.Fatal("Pipeline not found in map immediately after creation")
	}

	msg := &messages.Message{
		PipelineID: pl.ID(),
		Type:       messages.MessageTypePipelineState,
		Data:       []byte{},
	}

	err = pl.HandleMessage(msg)
	if err != nil {
		t.Fatalf("HandleMessage failed: %v", err)
	}

	time.Sleep(200 * time.Millisecond)

	if _, ok := pool.pipelines.Load(pl.ID()); ok {
		t.Error("Pipeline was NOT removed from map after completion")
	}
}

func TestRunspacePool_ContextLifecycle(t *testing.T) {
	pool := New(&blockingMockReadWriter{}, uuid.New())
	pool.state = StateOpened

	go pool.dispatchLoop(pool.ctx)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = pool.Close(ctx)

	select {
	case <-pool.ctx.Done():
		// Success
	case <-time.After(1 * time.Second):
		t.Error("Pool context was not cancelled after Close()")
	}
}
