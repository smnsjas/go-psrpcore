package runspace

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/smnsjas/go-psrpcore/messages"
	"github.com/smnsjas/go-psrpcore/serialization"
)

// MockTransport is a simple mock that enables testing of Runspace interactions
type MockTransport struct {
	sentData []byte
	inbox    []byte // bytes allowed to be read
}

func (m *MockTransport) Write(p []byte) (n int, err error) {
	m.sentData = append(m.sentData, p...)
	return len(p), nil
}

func (m *MockTransport) Read(p []byte) (n int, err error) {
	if len(m.inbox) == 0 {
		// return 0, io.EOF // Or block?
		// For testing RunspacePool, it usually spins in a loop reading.
		// We'll return 0, nil to simulate waiting, or block.
		// This mock is too simple for the complex RunspacePool handshake/loop.
		// However, for TestGetCommandMetadata, we bypassed the loop by forcing state=Opened.
		// But GetCommandMetadata calls sendMessage -> Encode -> transport.Write.
		// It does NOT read from transport (it reads from metadataCh).
		// So Read implementation doesn't matter much if we don't start the reader loop.
		return 0, nil
	}
	n = copy(p, m.inbox)
	m.inbox = m.inbox[n:]
	return n, nil
}

func (m *MockTransport) Close() error {
	return nil
}

func TestGetCommandMetadata(t *testing.T) {
	transport := &MockTransport{}

	// 1. Create Pool (initially Closed)
	pool := New(transport, uuid.New())
	// Force state to Opened specifically for this test, bypassing the Open() handshake
	// In reality we should mock the whole handshake or use a test helper to get an opened pool.
	// Since `state` is private, we can't easily set it from outside the package unless we use reflection
	// or add a backdoor.
	// However, since this test is IN the `runspace` package (same package test), we CAN access private fields!
	pool.state = StateOpened

	// 2. Prepare mock reply data
	// Construct a CommandMetadata object graph manually
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

	// 3. Run GetCommandMetadata in a goroutine (it blocks sending request)
	go func() {
		// Wait a bit for request to be sent
		time.Sleep(50 * time.Millisecond)

		// Send mock reply
		replyMsg := &messages.Message{
			Type: messages.MessageTypeGetCommandMetadataReply,
			Data: replyData,
		}
		// We need to inject this into the pool's processing loop.
		// BUT: processing loop isn't running because we didn't call Open().
		// We need to manually inject into metadataCh since we are bypassing the loop.
		pool.metadataCh <- replyMsg
	}()

	results, err := pool.GetCommandMetadata(context.Background(), []string{"Get-Process"})
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
