package runspace

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/smnsjas/go-psrpcore/messages"
)

func TestParseRunspaceAvailability(t *testing.T) {
	tests := []struct {
		name     string
		data     []byte
		expected int
		wantErr  bool
	}{
		{
			name:     "Int64",
			data:     []byte(`<I64>5</I64>`),
			expected: 5,
			wantErr:  false,
		},
		{
			name:     "Int32",
			data:     []byte(`<I32>3</I32>`),
			expected: 3,
			wantErr:  false,
		},
		{
			name:     "PSObject Int64",
			data:     []byte(`<Obj RefId="0"><I64>10</I64></Obj>`),
			expected: 10,
			wantErr:  false,
		},
		{
			name:     "PSObject Int32",
			data:     []byte(`<Obj RefId="0"><I32>8</I32></Obj>`),
			expected: 8,
			wantErr:  false,
		},
		{
			name:    "Invalid XML",
			data:    []byte(`not xml`),
			wantErr: true,
		},
		{
			name:    "Empty",
			data:    []byte(``),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Determine if input is raw XML fragment or full CLIXML
			// Using helper assuming full CLIXML structure if Obj or I64/I32 tag
			// The serializer wraps in Objs automatically if not present?
			// No, parseRunspaceAvailability uses serialization.NewDeserializer().Deserialize(data)
			// which expects valid CLIXML stream (usually starts with <Objs> or just <Obj>/<I64> if fragment).

			// Let's create proper CLIXML payload using serializer for reliable test data
			var payload []byte
			if tt.name == "Invalid XML" || tt.name == "Empty" {
				payload = tt.data
			} else {
				// Construct minimal valid CLIXML
				payload = []byte(`<?xml version="1.0" encoding="utf-8"?><Objs Version="1.1.0.1" xmlns="http://schemas.microsoft.com/powershell/2004/04">` + string(tt.data) + `</Objs>`)
			}

			got, err := parseRunspaceAvailability(payload)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseRunspaceAvailability() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.expected {
				t.Errorf("parseRunspaceAvailability() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestRunspaceAvailabilityMessage(t *testing.T) {
	transport := newMockTransport()
	poolID := uuid.New()
	pool := New(transport, poolID)

	// Set state to Opened so dispatch loop processes messages
	// We need to simulate Open or just set state manifest
	// But dispatchLoop needs to be running.
	// Best way is to fake-Open it.

	pool.mu.Lock()
	pool.state = StateOpened
	pool.mu.Unlock()

	// Start dispatch loop
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go pool.dispatchLoop(ctx)

	// Queue RUNSPACE_AVAILABILITY message
	availData := []byte(`<?xml version="1.0" encoding="utf-8"?>
<Objs Version="1.1.0.1" xmlns="http://schemas.microsoft.com/powershell/2004/04">
  <I64>42</I64>
</Objs>`)

	msg := &messages.Message{
		Destination: messages.DestinationClient,
		Type:        messages.MessageTypeRunspaceAvailability,
		RunspaceID:  poolID,
		PipelineID:  uuid.Nil,
		Data:        availData,
	}

	if err := transport.queueMessage(msg); err != nil {
		t.Fatalf("queue message failed: %v", err)
	}

	// wait for update
	// We can check AvailableRunspaces() periodically
	success := false
	for i := 0; i < 20; i++ {
		if pool.AvailableRunspaces() == 42 {
			success = true
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	if !success {
		t.Errorf("Expected AvailableRunspaces to update to 42, got %d", pool.AvailableRunspaces())
	}
}

func TestSendGetAvailableRunspaces(t *testing.T) {
	transport := newMockTransport()
	poolID := uuid.New()
	pool := New(transport, poolID)

	// Must be opened
	pool.mu.Lock()
	pool.state = StateOpened
	pool.mu.Unlock()

	ctx := context.Background()
	if err := pool.SendGetAvailableRunspaces(ctx); err != nil {
		t.Fatalf("SendGetAvailableRunspaces failed: %v", err)
	}

	// Verify message in transport
	msg, err := transport.readMessage()
	if err != nil {
		t.Fatalf("read message failed: %v", err)
	}

	if msg.Type != messages.MessageTypeGetAvailableRunspaces {
		t.Errorf("expected GET_AVAILABLE_RUNSPACES, got %v", msg.Type)
	}

	if msg.RunspaceID != poolID {
		t.Errorf("expected RunspaceID %v, got %v", poolID, msg.RunspaceID)
	}
}
