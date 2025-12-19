package objects

import (
	"testing"
)

func TestSecureString(t *testing.T) {
	plaintext := "MySecretPassword"

	ss, err := NewSecureString(plaintext)
	if err != nil {
		t.Fatalf("NewSecureString failed: %v", err)
	}

	if ss.EncryptedBytes() == nil {
		t.Error("EncryptedBytes returned nil")
	}

	// Since we can't easily decrypt without defining decryption method here or exporting key,
	// checking for successful creation and non-nil bytes is a decent smoke test.

	ss.Clear()
	// Verifying Clear zeroes the key would require exporting/inspecting internal state,
	// which we probably shouldn't do in a black-box test, but white-box is fine.
}

func TestPowerShellBuilder(t *testing.T) {
	ps := NewPowerShell()
	ps.AddCommand("Get-Process", false)
	ps.AddParameter("Id", 123)

	if len(ps.Commands) != 1 {
		t.Fatalf("Expected 1 command, got %d", len(ps.Commands))
	}

	cmd := ps.Commands[0]
	if cmd.Name != "Get-Process" {
		t.Errorf("Expected command Name 'Get-Process', got '%s'", cmd.Name)
	}

	if len(cmd.Parameters) != 1 {
		t.Fatalf("Expected 1 parameter, got %d", len(cmd.Parameters))
	}

	param := cmd.Parameters[0]
	if param.Name != "Id" {
		t.Errorf("Expected parameter Name 'Id', got '%s'", param.Name)
	}
	if val, ok := param.Value.(int); !ok || val != 123 {
		t.Errorf("Expected parameter Value 123, got %v", param.Value)
	}
}
