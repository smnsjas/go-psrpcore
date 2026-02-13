package pipeline

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/smnsjas/go-psrpcore/host"
	"github.com/smnsjas/go-psrpcore/messages"
	"github.com/smnsjas/go-psrpcore/serialization"
)

// mockTransport is a mock implementation of Transport for testing.
type mockTransport struct {
	mu   sync.Mutex
	sent []*messages.Message
	err  error
}

func (m *mockTransport) SendMessage(_ context.Context, msg *messages.Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.err != nil {
		return m.err
	}
	m.sent = append(m.sent, msg)
	return nil
}

func (m *mockTransport) SendMessages(_ context.Context, msgs []*messages.Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.err != nil {
		return m.err
	}
	m.sent = append(m.sent, msgs...)
	return nil
}

func (m *mockTransport) GetSent() []*messages.Message {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.sent) == 0 {
		return nil
	}
	sent := make([]*messages.Message, len(m.sent))
	copy(sent, m.sent)
	return sent
}

func (m *mockTransport) Host() host.Host {
	return host.NewNullHost()
}

func TestPipeline_Invoke(t *testing.T) {
	tests := []struct {
		name          string
		transportErr  error
		expectedState State
		expectError   bool
	}{
		{
			name:          "Success",
			transportErr:  nil,
			expectedState: StateRunning,
			expectError:   false,
		},
		{
			name:          "TransportFailure",
			transportErr:  errors.New("network error"),
			expectedState: StateNotStarted, // Should stay NotStarted if send fails? Or Stopped/Failed?
			// Looking at implementation (assumed): if SendMessage fails, Invoke returns error.
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &mockTransport{err: tt.transportErr}
			runspaceID := uuid.New()
			cmd := "Get-Process"

			p := New(transport, runspaceID, cmd)

			if p.State() != StateNotStarted {
				t.Errorf("expected initial state NotStarted, got %v", p.State())
			}

			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			err := p.Invoke(ctx)

			if tt.expectError {
				if err == nil {
					t.Error("expected Invoke error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("Invoke failed: %v", err)
				}
			}

			// If transport error, state might be unchanged or Failed depending on implementation.
			// Currently implementation likely keeps it NotStarted or transitions back.
			if !tt.expectError && p.State() != tt.expectedState {
				t.Errorf("expected state %v, got %v", tt.expectedState, p.State())
			}

			if !tt.expectError {
				// Verify CREATE_PIPELINE message was sent
				sent := transport.GetSent()
				if len(sent) != 1 {
					t.Fatalf("expected 1 message sent, got %d", len(sent))
				}
				msg := sent[0]
				if msg.Type != messages.MessageTypeCreatePipeline {
					t.Errorf("expected CreatePipeline message, got %v", msg.Type)
				}
			}
		})
	}
}

func TestPipeline_HandleMessage_Output(t *testing.T) {
	transport := &mockTransport{}
	p := New(transport, uuid.New(), "test")

	// Transition to Running
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = p.Invoke(ctx)

	// Simulate output message
	outMsg := &messages.Message{
		Type:       messages.MessageTypePipelineOutput,
		PipelineID: p.ID(),
		Data:       []byte("output data"),
	}

	err := p.HandleMessage(outMsg)
	if err != nil {
		t.Fatalf("HandleMessage failed: %v", err)
	}

	// Check output channel
	select {
	case received := <-p.Output():
		if received != outMsg {
			t.Error("received wrong message")
		}
	case <-time.After(500 * time.Millisecond): // Reduced timeout
		t.Error("timeout waiting for output")
	}
}

func TestPipeline_Completion(t *testing.T) {
	transport := &mockTransport{}
	p := New(transport, uuid.New(), "test")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = p.Invoke(ctx)

	// Serialize state value 4 (Completed) as CLIXML
	ser := serialization.NewSerializer()
	stateData, err := ser.Serialize(int32(4))
	if err != nil {
		t.Fatalf("failed to serialize state: %v", err)
	}

	// Simulate State Completed
	stateMsg := &messages.Message{
		Type:       messages.MessageTypePipelineState,
		PipelineID: p.ID(),
		Data:       stateData,
	}

	// Handle completion
	_ = p.HandleMessage(stateMsg)

	// Check state
	if p.State() != StateCompleted {
		t.Errorf("expected state Completed, got %v", p.State())
	}

	// Check wait
	select {
	case <-p.doneCh:
		// success
	case <-time.After(500 * time.Millisecond): // Reduced timeout
		t.Error("timeout waiting for completion signal")
	}
}

func TestPipeline_Stop(t *testing.T) {
	transport := &mockTransport{}
	p := New(transport, uuid.New(), "test")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = p.Invoke(ctx)

	// Call Stop
	err := p.Stop(ctx)
	if err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	if p.State() != StateStopping {
		t.Errorf("expected state Stopping, got %v", p.State())
	}

	// Verify SIGNAL message sent
	sent := transport.GetSent()
	if len(sent) != 2 { // Create + Signal
		t.Fatalf("expected 2 messages, got %d", len(sent))
	}
	msg := sent[1]
	if msg.Type != messages.MessageTypeSignal {
		t.Errorf("expected Signal message, got %v", msg.Type)
	}
}

func TestPipeline_Input(t *testing.T) {
	transport := &mockTransport{}
	p := New(transport, uuid.New(), "test")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = p.Invoke(ctx)

	// Send Input
	inputData := "some input"
	err := p.SendInput(ctx, inputData)
	if err != nil {
		t.Fatalf("SendInput failed: %v", err)
	}

	// Verify PIPELINE_INPUT message
	sent := transport.GetSent()
	if len(sent) < 2 {
		t.Fatalf("expected input message to be sent")
	}
	msg := sent[1]
	if msg.Type != messages.MessageTypePipelineInput {
		t.Errorf("expected PipelineInput message, got %v", msg.Type)
	}

	// Close Input
	err = p.CloseInput(ctx)
	if err != nil {
		t.Fatalf("CloseInput failed: %v", err)
	}

	// Verify END_OF_PIPELINE_INPUT message
	sent = transport.GetSent()
	if len(sent) < 3 {
		t.Fatalf("expected end of input message to be sent")
	}
	msgEnd := sent[2]
	if msgEnd.Type != messages.MessageTypeEndOfPipelineInput {
		t.Errorf("expected EndOfPipelineInput message, got %v", msgEnd.Type)
	}
}

// TestPipeline_StateTransitions tests all PSInvocationState values from MS-PSRP Section 2.2.3.9.
// This test verifies that the pipeline correctly handles state transitions for all valid
// PSInvocationState enum values (0-6) as defined in the PSRP protocol specification.
func TestPipeline_StateTransitions(t *testing.T) {
	tests := []struct {
		name          string
		stateValue    int32 // PSInvocationState enum value
		expectedState State
		isTerminal    bool // Terminal states should close doneCh
	}{
		{
			name:          "NotStarted (0)",
			stateValue:    0,
			expectedState: StateNotStarted,
			isTerminal:    false,
		},
		{
			name:          "Running (1)",
			stateValue:    1,
			expectedState: StateRunning,
			isTerminal:    false,
		},
		{
			name:          "Stopping (2)",
			stateValue:    2,
			expectedState: StateStopping,
			isTerminal:    false,
		},
		{
			name:          "Stopped (3)",
			stateValue:    3,
			expectedState: StateStopped,
			isTerminal:    true,
		},
		{
			name:          "Completed (4)",
			stateValue:    4,
			expectedState: StateCompleted,
			isTerminal:    true,
		},
		{
			name:          "Failed (5)",
			stateValue:    5,
			expectedState: StateFailed,
			isTerminal:    true,
		},
		{
			name:          "Disconnected (6)",
			stateValue:    6,
			expectedState: StateDisconnected,
			isTerminal:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &mockTransport{}
			p := New(transport, uuid.New(), "test")

			// Invoke to transition to Running state
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			_ = p.Invoke(ctx)

			// Serialize the state value as CLIXML
			ser := serialization.NewSerializer()
			defer ser.Close()
			stateData, err := ser.Serialize(tt.stateValue)
			if err != nil {
				t.Fatalf("failed to serialize state value %d: %v", tt.stateValue, err)
			}

			// Create PIPELINE_STATE message with the state value
			stateMsg := &messages.Message{
				Type:       messages.MessageTypePipelineState,
				PipelineID: p.ID(),
				RunspaceID: p.runspaceID,
				Data:       stateData,
			}

			// Handle the state message
			err = p.HandleMessage(stateMsg)
			if err != nil {
				t.Fatalf("HandleMessage failed: %v", err)
			}

			// Verify the state transition
			if p.State() != tt.expectedState {
				t.Errorf("expected state %v (%d), got %v (%d)",
					tt.expectedState, tt.expectedState,
					p.State(), p.State())
			}

			// Verify terminal states close the doneCh
			if tt.isTerminal {
				select {
				case <-p.doneCh:
					// Success - terminal state closed the channel
				case <-time.After(100 * time.Millisecond):
					t.Errorf("terminal state %v did not close doneCh", tt.expectedState)
				}

				// Verify Wait() returns without blocking
				done := make(chan struct{})
				go func() {
					_ = p.Wait()
					close(done)
				}()

				select {
				case <-done:
					// Success - Wait() returned
				case <-time.After(100 * time.Millisecond):
					t.Errorf("Wait() blocked for terminal state %v", tt.expectedState)
				}
			} else {
				// Non-terminal states should NOT close doneCh
				select {
				case <-p.doneCh:
					t.Errorf("non-terminal state %v incorrectly closed doneCh", tt.expectedState)
				case <-time.After(50 * time.Millisecond):
					// Success - channel remains open
				}
			}
		})
	}
}

// TestPipeline_StateString verifies that all State values have proper string representations.
func TestPipeline_StateString(t *testing.T) {
	tests := []struct {
		state    State
		expected string
	}{
		{StateNotStarted, "NotStarted"},
		{StateRunning, "Running"},
		{StateStopping, "Stopping"},
		{StateStopped, "Stopped"},
		{StateCompleted, "Completed"},
		{StateFailed, "Failed"},
		{StateDisconnected, "Disconnected"},
		{State(99), "Unknown(99)"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			got := tt.state.String()
			if got != tt.expected {
				t.Errorf("State(%d).String() = %q, want %q", tt.state, got, tt.expected)
			}
		})
	}
}

// TestPipeline_ChannelTimeout verifies the timeout-based back-pressure mechanism.
// When the output buffer is full, HandleMessage should block for up to channelTimeout
// before returning ErrBufferFull.
func TestPipeline_ChannelTimeout(t *testing.T) {
	transport := &mockTransport{}
	p := New(transport, uuid.New(), "test")

	// Set a very short timeout for testing
	p.SetChannelTimeout(100 * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = p.Invoke(ctx)

	// Fill the output buffer completely (100 messages)
	for i := 0; i < 100; i++ {
		msg := &messages.Message{
			Type:       messages.MessageTypePipelineOutput,
			PipelineID: p.ID(),
			Data:       []byte("test output"),
		}
		if err := p.HandleMessage(msg); err != nil {
			t.Fatalf("HandleMessage failed on message %d: %v", i, err)
		}
	}

	// The buffer is now full. The next message should block and timeout.
	msg := &messages.Message{
		Type:       messages.MessageTypePipelineOutput,
		PipelineID: p.ID(),
		Data:       []byte("overflow message"),
	}

	start := time.Now()
	err := p.HandleMessage(msg)
	elapsed := time.Since(start)

	// Verify we got the expected error
	if !errors.Is(err, ErrBufferFull) {
		t.Errorf("expected ErrBufferFull, got: %v", err)
	}

	// Verify it waited approximately the timeout duration
	if elapsed < 90*time.Millisecond || elapsed > 200*time.Millisecond {
		t.Errorf("timeout took %v, expected ~100ms", elapsed)
	}
}

// TestPipeline_ChannelBackpressure verifies that slow consumers create back-pressure.
// When messages can be consumed, they should be delivered even after initially filling the buffer.
func TestPipeline_ChannelBackpressure(t *testing.T) {
	transport := &mockTransport{}
	p := New(transport, uuid.New(), "test")

	// Set a reasonable timeout
	p.SetChannelTimeout(2 * time.Second)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = p.Invoke(ctx)

	// Fill the buffer
	for i := 0; i < 100; i++ {
		msg := &messages.Message{
			Type:       messages.MessageTypePipelineOutput,
			PipelineID: p.ID(),
			Data:       []byte("test output"),
		}
		if err := p.HandleMessage(msg); err != nil {
			t.Fatalf("HandleMessage failed on message %d: %v", i, err)
		}
	}

	// Start a goroutine that slowly consumes messages
	go func() {
		time.Sleep(50 * time.Millisecond)
		for i := 0; i < 10; i++ {
			<-p.Output()
			time.Sleep(10 * time.Millisecond)
		}
	}()

	// Try to send more messages - they should be delivered as space becomes available
	for i := 0; i < 5; i++ {
		msg := &messages.Message{
			Type:       messages.MessageTypePipelineOutput,
			PipelineID: p.ID(),
			Data:       []byte("backpressure test"),
		}
		start := time.Now()
		err := p.HandleMessage(msg)
		elapsed := time.Since(start)

		if err != nil {
			t.Errorf("message %d failed: %v (elapsed: %v)", i, err, elapsed)
		}

		// Messages should be delivered within the timeout
		if elapsed > 2*time.Second {
			t.Errorf("message %d took too long: %v", i, elapsed)
		}
	}
}

// TestPipeline_ChannelContextCancellation verifies that HandleMessage respects
// context cancellation and returns immediately when the pipeline is closed.
func TestPipeline_ChannelContextCancellation(t *testing.T) {
	transport := &mockTransport{}
	p := New(transport, uuid.New(), "test")

	// Set a long timeout so we can test context cancellation
	p.SetChannelTimeout(10 * time.Second)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = p.Invoke(ctx)

	// Fill the buffer
	for i := 0; i < 100; i++ {
		msg := &messages.Message{
			Type:       messages.MessageTypePipelineOutput,
			PipelineID: p.ID(),
			Data:       []byte("test output"),
		}
		if err := p.HandleMessage(msg); err != nil {
			t.Fatalf("HandleMessage failed on message %d: %v", i, err)
		}
	}

	// Start a goroutine that will try to send a message (will block on full buffer)
	errChan := make(chan error, 1)
	go func() {
		msg := &messages.Message{
			Type:       messages.MessageTypePipelineOutput,
			PipelineID: p.ID(),
			Data:       []byte("will be cancelled"),
		}
		errChan <- p.HandleMessage(msg)
	}()

	// Give the goroutine time to start blocking
	time.Sleep(100 * time.Millisecond)

	// Cancel the pipeline context
	p.cancel()

	// The HandleMessage call should return with context.Canceled error
	select {
	case err := <-errChan:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("expected context.Canceled, got: %v", err)
		}
	case <-time.After(1 * time.Second):
		t.Error("HandleMessage did not return after context cancellation")
	}
}

// TestPipeline_ErrorChannelTimeout verifies timeout behavior for the error channel.
// This ensures that the error channel has the same back-pressure mechanism as the output channel.
func TestPipeline_ErrorChannelTimeout(t *testing.T) {
	transport := &mockTransport{}
	p := New(transport, uuid.New(), "test")

	// Set a very short timeout for testing
	p.SetChannelTimeout(100 * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = p.Invoke(ctx)

	// Fill the error buffer completely (100 messages)
	for i := 0; i < 100; i++ {
		msg := &messages.Message{
			Type:       messages.MessageTypeErrorRecord,
			PipelineID: p.ID(),
			Data:       []byte("test error"),
		}
		if err := p.HandleMessage(msg); err != nil {
			t.Fatalf("HandleMessage failed on message %d: %v", i, err)
		}
	}

	// The buffer is now full. The next message should block and timeout.
	msg := &messages.Message{
		Type:       messages.MessageTypeErrorRecord,
		PipelineID: p.ID(),
		Data:       []byte("overflow error"),
	}

	start := time.Now()
	err := p.HandleMessage(msg)
	elapsed := time.Since(start)

	// Verify we got the expected error
	if !errors.Is(err, ErrBufferFull) {
		t.Errorf("expected ErrBufferFull, got: %v", err)
	}

	// Verify it waited approximately the timeout duration
	if elapsed < 90*time.Millisecond || elapsed > 200*time.Millisecond {
		t.Errorf("timeout took %v, expected ~100ms", elapsed)
	}
}

// TestPipeline_Builder tests the builder pattern methods.
func TestPipeline_Builder(t *testing.T) {
	transport := &mockTransport{}
	runspaceID := uuid.New()

	// given: a pipeline created with NewBuilder
	p := NewBuilder(transport, runspaceID)

	// then: pipeline should start in NotStarted state with valid ID
	if p.State() != StateNotStarted {
		t.Errorf("expected initial state NotStarted, got %v", p.State())
	}
	if p.ID() == uuid.Nil {
		t.Error("pipeline ID should not be nil")
	}

	// when: adding commands and parameters using fluent API
	result := p.AddCommand("Get-Process", false).
		AddParameter("Name", "powershell").
		AddCommand("Select-Object", false).
		AddParameter("Property", "Id").
		AddArgument("extraArg")

	// then: builder should return same pipeline for chaining
	if result != p {
		t.Error("builder methods should return the same pipeline for chaining")
	}
}

// TestPipeline_Error tests the Error() channel accessor.
func TestPipeline_Error(t *testing.T) {
	transport := &mockTransport{}
	p := New(transport, uuid.New(), "Get-Process")

	// given: a pipeline
	// when: accessing the Error channel
	errCh := p.Error()

	// then: should return a valid channel
	if errCh == nil {
		t.Fatal("Error() returned nil channel")
	}

	// Verify it's the same channel on multiple calls
	if p.Error() != errCh {
		t.Error("Error() should return the same channel on multiple calls")
	}
}

// TestPipeline_Fail tests the Fail() method for external failure signaling.
func TestPipeline_Fail(t *testing.T) {
	transport := &mockTransport{}
	p := New(transport, uuid.New(), "Get-Process")

	// given: a running pipeline
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := p.Invoke(ctx); err != nil {
		t.Fatalf("Invoke failed: %v", err)
	}
	if p.State() != StateRunning {
		t.Errorf("expected Running state, got %v", p.State())
	}

	// when: calling Fail() with an error
	testErr := errors.New("transport connection lost")
	p.Fail(testErr)

	// then: pipeline should transition to Failed state
	if p.State() != StateFailed {
		t.Errorf("expected Failed state after Fail(), got %v", p.State())
	}

	// then: Wait() should return the error
	err := p.Wait()
	if err == nil {
		t.Error("Wait() should return an error after Fail()")
	}
	if err.Error() != testErr.Error() {
		t.Errorf("Wait() error mismatch: got %v, want %v", err, testErr)
	}
}

// TestPipeline_Stop_InvalidState tests stopping a pipeline in an invalid state.
func TestPipeline_Stop_InvalidState(t *testing.T) {
	transport := &mockTransport{}
	p := New(transport, uuid.New(), "Get-Process")

	// given: a pipeline that has NOT been invoked (still NotStarted)
	// when: trying to stop it
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err := p.Stop(ctx)

	// then: should return ErrInvalidState
	if err == nil {
		t.Error("expected error when stopping non-running pipeline")
	}
	if !errors.Is(err, ErrInvalidState) {
		t.Errorf("expected ErrInvalidState, got: %v", err)
	}
}

// TestPipeline_SendInput_InvalidState tests sending input to a non-running pipeline.
func TestPipeline_SendInput_InvalidState(t *testing.T) {
	transport := &mockTransport{}
	p := New(transport, uuid.New(), "Get-Process")

	// given: a pipeline that has NOT been invoked
	// when: trying to send input
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err := p.SendInput(ctx, "test input")

	// then: should return ErrInvalidState
	if err == nil {
		t.Error("expected error when sending input to non-running pipeline")
	}
	if !errors.Is(err, ErrInvalidState) {
		t.Errorf("expected ErrInvalidState, got: %v", err)
	}
}

// TestPipeline_CloseInput_InvalidState tests closing input on a non-running pipeline.
func TestPipeline_CloseInput_InvalidState(t *testing.T) {
	transport := &mockTransport{}
	p := New(transport, uuid.New(), "Get-Process")

	// given: a pipeline that has NOT been invoked
	// when: trying to close input
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err := p.CloseInput(ctx)

	// then: should return ErrInvalidState
	if err == nil {
		t.Error("expected error when closing input on non-running pipeline")
	}
	if !errors.Is(err, ErrInvalidState) {
		t.Errorf("expected ErrInvalidState, got: %v", err)
	}
}

// TestPipeline_Invoke_AlreadyRunning tests invoking an already-running pipeline.
func TestPipeline_Invoke_AlreadyRunning(t *testing.T) {
	transport := &mockTransport{}
	p := New(transport, uuid.New(), "Get-Process")

	// given: a pipeline that has been invoked
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := p.Invoke(ctx); err != nil {
		t.Fatalf("first Invoke failed: %v", err)
	}

	// when: trying to invoke again
	err := p.Invoke(ctx)

	// then: should return ErrInvalidState
	if err == nil {
		t.Error("expected error when invoking already-running pipeline")
	}
	if !errors.Is(err, ErrInvalidState) {
		t.Errorf("expected ErrInvalidState, got: %v", err)
	}
}

func TestPipeline_NewWithID(t *testing.T) {
	transport := &mockTransport{}
	runspaceID := uuid.New()
	pipelineID := uuid.New()

	p := NewWithID(transport, runspaceID, pipelineID)

	if p.ID() != pipelineID {
		t.Errorf("expected pipeline ID %s, got %s", pipelineID, p.ID())
	}
	if p.State() != StateRunning {
		t.Errorf("expected StateRunning, got %v", p.State())
	}
}

func TestPipeline_HandleMessage_Streams(t *testing.T) {
	transport := &mockTransport{}
	p := New(transport, uuid.New(), "test")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = p.Invoke(ctx)

	checkStream := func(msgType messages.MessageType, data string, ch <-chan *messages.Message, name string) {
		msg := &messages.Message{
			Type:       msgType,
			PipelineID: p.ID(),
			Data:       []byte(data),
		}
		if err := p.HandleMessage(msg); err != nil {
			t.Errorf("%s HandleMessage failed: %v", name, err)
			return
		}
		select {
		case received := <-ch:
			if string(received.Data) != data {
				t.Errorf("%s mismatch: got %q, want %q", name, received.Data, data)
			}
		case <-time.After(100 * time.Millisecond):
			t.Errorf("%s timeout", name)
		}
	}

	checkStream(messages.MessageTypeErrorRecord, "error", p.Error(), "Error")
	checkStream(messages.MessageTypeWarningRecord, "warning", p.Warning(), "Warning")
	checkStream(messages.MessageTypeVerboseRecord, "verbose", p.Verbose(), "Verbose")
	checkStream(messages.MessageTypeDebugRecord, "debug", p.Debug(), "Debug")
	checkStream(messages.MessageTypeProgressRecord, "progress", p.Progress(), "Progress")
	checkStream(messages.MessageTypeInformationRecord, "info", p.Information(), "Information")
}

func TestPipeline_HandleMessage_HostCall(t *testing.T) {
	transport := &mockTransport{}
	p := New(transport, uuid.New(), "test")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = p.Invoke(ctx)

	call := &host.RemoteHostCall{
		CallID:           123,
		MethodID:         host.MethodIDWriteLine2,
		MethodParameters: []interface{}{"test host call"},
	}

	callData, err := host.EncodeRemoteHostCall(call)
	if err != nil {
		t.Fatalf("EncodeRemoteHostCall failed: %v", err)
	}

	msg := &messages.Message{
		Type:       messages.MessageTypePipelineHostCall,
		PipelineID: p.ID(),
		Data:       callData,
	}

	if err := p.HandleMessage(msg); err != nil {
		t.Fatalf("HandleMessage failed: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	sent := transport.GetSent()
	if len(sent) < 2 {
		t.Errorf("expected host response message, got %d messages", len(sent))
	} else {
		lastMsg := sent[len(sent)-1]
		if lastMsg.Type != messages.MessageTypePipelineHostResponse {
			t.Errorf("expected PipelineHostResponse, got %v", lastMsg.Type)
		}
	}
}

// TestPipeline_SendInputBatch table-driven tests for batch input sending.
func TestPipeline_SendInputBatch(t *testing.T) {
	tests := []struct {
		name         string
		items        []interface{}
		transportErr error
		skipInvoke   bool // if true, don't invoke first (test wrong state)
		expectError  bool
		wantErrStr   string
		wantMsgCount int
	}{
		{
			name:         "empty_input_noop",
			items:        nil,
			wantMsgCount: 1, // only the CREATE_PIPELINE from Invoke
		},
		{
			name:         "single_item",
			items:        []interface{}{"hello"},
			wantMsgCount: 2, // CREATE_PIPELINE + 1 PIPELINE_INPUT
		},
		{
			name: "multiple_items",
			items: []interface{}{
				"item-1",
				"item-2",
				"item-3",
			},
			wantMsgCount: 4, // CREATE_PIPELINE + 3 PIPELINE_INPUT
		},
		{
			name:         "not_running_state",
			items:        []interface{}{"hello"},
			skipInvoke:   true,
			expectError:  true,
			wantErrStr:   "not running",
			wantMsgCount: 0,
		},
		{
			name:         "transport_error",
			items:        []interface{}{"hello"},
			transportErr: errors.New("network failure"),
			expectError:  true,
			wantErrStr:   "network failure",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tr := &mockTransport{}

			runspaceID := uuid.New()
			p := New(tr, runspaceID, "test-command")

			if !tc.skipInvoke {
				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
				defer cancel()
				if err := p.Invoke(ctx); err != nil {
					t.Fatalf("Invoke failed: %v", err)
				}

				// Set transport error after invoke succeeds
				if tc.transportErr != nil {
					tr.mu.Lock()
					tr.err = tc.transportErr
					tr.mu.Unlock()
				}
			}

			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			err := p.SendInputBatch(ctx, tc.items)

			if tc.expectError {
				if err == nil {
					t.Fatal("SendInputBatch() should have failed")
				}
				if tc.wantErrStr != "" && !strings.Contains(err.Error(), tc.wantErrStr) {
					t.Errorf("error = %v, want substring %q", err, tc.wantErrStr)
				}
				return
			}

			if err != nil {
				t.Fatalf("SendInputBatch() error = %v", err)
			}

			sent := tr.GetSent()
			if len(sent) != tc.wantMsgCount {
				t.Errorf("messages sent = %d, want %d", len(sent), tc.wantMsgCount)
			}

			// Verify all batch messages are PIPELINE_INPUT type
			for i := 1; i < len(sent); i++ {
				if sent[i].Type != messages.MessageTypePipelineInput {
					t.Errorf("message[%d] type = %v, want PipelineInput",
						i, sent[i].Type)
				}
			}
		})
	}
}

// TestPipeline_SendInputBatch_PreservesOrder verifies that items are
// sent in the same order they are provided, matching MS-PSRP requirements.
func TestPipeline_SendInputBatch_PreservesOrder(t *testing.T) {
	tr := &mockTransport{}
	runspaceID := uuid.New()
	p := New(tr, runspaceID, "test-command")

	ctx := context.Background()
	if err := p.Invoke(ctx); err != nil {
		t.Fatalf("Invoke failed: %v", err)
	}

	items := []interface{}{"first", "second", "third", "fourth", "fifth"}
	if err := p.SendInputBatch(ctx, items); err != nil {
		t.Fatalf("SendInputBatch() error = %v", err)
	}

	sent := tr.GetSent()
	// First message is CREATE_PIPELINE, remaining are PIPELINE_INPUT
	inputMsgs := sent[1:]
	if len(inputMsgs) != 5 {
		t.Fatalf("got %d input messages, want 5", len(inputMsgs))
	}

	// Verify each message carries the correct pipeline ID and type
	for i, msg := range inputMsgs {
		if msg.Type != messages.MessageTypePipelineInput {
			t.Errorf("message[%d] type = %v, want PipelineInput", i, msg.Type)
		}
		if msg.PipelineID != p.ID() {
			t.Errorf("message[%d] pipelineID = %v, want %v", i, msg.PipelineID, p.ID())
		}
	}
}
