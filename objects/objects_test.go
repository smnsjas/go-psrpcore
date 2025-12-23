package objects

import (
	"testing"
)

func TestSecureString(t *testing.T) {
	t.Parallel()
	plaintext := "MySecretPassword"

	// Test creation
	ss, err := NewSecureString(plaintext)
	if err != nil {
		t.Fatalf("NewSecureString failed: %v", err)
	}

	if ss.EncryptedBytes() == nil {
		t.Error("EncryptedBytes returned nil")
	}

	// Test Decrypt
	decrypted, err := ss.Decrypt()
	if err != nil {
		t.Fatalf("Decrypt failed: %v", err)
	}
	if string(decrypted) != plaintext {
		t.Errorf("Decrypt mismatch: got %q, want %q", string(decrypted), plaintext)
	}

	// Test Clear
	ss.Clear()
	// After clear, decrypt might fail or return garbage/zeros, or internal state is zeroed.
	// Since we are white-box testing to some extent, we can verify internals if we want,
	// but behaviorally, Decrypt might now fail or return empty/different.
	// In the current implementation, key is zeroed, so Aes creation will likely fail or produce garbage if key is all zeros (valid key but wrong one).
	// Actually, 32 bytes of zeros IS a valid AES-256 key, but the GCM tag validation will fail because key changed.
	_, err = ss.Decrypt()
	if err == nil {
		t.Error("Decrypt should fail after Clear (GCM tag validation failure expected due to key change)")
	}
}

func TestNewSecureStringFromEncrypted(t *testing.T) {
	t.Parallel()
	originalData := []byte{0xDE, 0xAD, 0xBE, 0xEF}
	ss := NewSecureStringFromEncrypted(originalData)

	encrypted := ss.EncryptedBytes()
	if string(encrypted) != string(originalData) {
		t.Error("NewSecureStringFromEncrypted data mismatch")
	}

	// Ensure modifying returned bytes doesn't affect internal state
	encrypted[0] = 0xAA
	if ss.EncryptedBytes()[0] == 0xAA {
		t.Error("EncryptedBytes should return a copy")
	}
}

func TestPSCredential(t *testing.T) {
	t.Parallel()
	user := "domain\\user"
	pass := "secret"
	ss, _ := NewSecureString(pass)

	cred := NewPSCredential(user, ss)
	if cred.UserName != user {
		t.Errorf("UserName mismatch: got %q, want %q", cred.UserName, user)
	}
	if cred.Password != ss {
		t.Error("Password pointer mismatch")
	}

	// Test Clear
	cred.Clear()
	// Should clear the underlying SecureString
	_, err := cred.Password.Decrypt()
	if err == nil {
		t.Error("Credential.Clear() did not clear the password (decryption succeeded)")
	}
}

func TestPowerShellBuilder(t *testing.T) {
	t.Parallel()
	ps := NewPowerShell()

	// Test AddCommand
	ps.AddCommand("Get-Process", false)
	if len(ps.Commands) != 1 {
		t.Fatalf("Expected 1 command, got %d", len(ps.Commands))
	}
	cmd := ps.Commands[0]
	if cmd.Name != "Get-Process" || cmd.IsScript {
		t.Error("Command fields mismatch")
	}

	// Test AddParameter
	ps.AddParameter("Id", 123)
	if len(cmd.Parameters) != 1 { // Warning: we are checking a COPY if we accessed ps.Commands[0] before. Re-fetch.
		// Wait, ps.Commands[0] returns a struct copy, so 'cmd' above is a copy.
		// logic: AddParameter modifies slice in 'ps', not 'cmd'.
	}

	// Re-fetch to check params
	cmd = ps.Commands[0]
	if len(cmd.Parameters) != 1 {
		t.Fatalf("Expected 1 parameter, got %d", len(cmd.Parameters))
	}
	param := cmd.Parameters[0]
	if param.Name != "Id" || param.Value != 123 {
		t.Error("Parameter fields mismatch")
	}

	// Test calling AddParameter on empty command list (should be safe no-op)
	psEmpty := NewPowerShell()
	psEmpty.AddParameter("Foo", "Bar")
	if len(psEmpty.Commands) != 0 {
		t.Error("AddParameter on empty shell verified failed")
	}
}

func TestScriptBlock(t *testing.T) {
	t.Parallel()
	sb := ScriptBlock{Text: "Write-Host 'Hello'"}
	if sb.String() != "Write-Host 'Hello'" {
		t.Errorf("ScriptBlock.String() mismatch: %q", sb.String())
	}
}

func TestCommandMetdata(t *testing.T) {
	t.Parallel()
	// Simple struct test
	cm := CommandMetadata{
		Name:        "Get-Test",
		CommandType: 8,
		Parameters: map[string]ParameterMetadata{
			"Name": {Name: "Name", Type: "System.String"},
		},
	}
	if cm.Name != "Get-Test" {
		t.Error("CommandMetadata struct check failed")
	}
}
