package objects

import (
	"testing"
)

func TestSecureString(t *testing.T) {
	t.Parallel()

	// given: a plaintext password
	plaintext := "MySecretPassword"

	// when: creating a SecureString
	ss, err := NewSecureString(plaintext)

	// then: creation succeeds and encrypted bytes are available
	if err != nil {
		t.Fatalf("NewSecureString failed: %v", err)
	}
	if ss.EncryptedBytes() == nil {
		t.Error("EncryptedBytes returned nil")
	}

	// when: decrypting the SecureString
	decrypted, err := ss.Decrypt()

	// then: decrypted value matches original plaintext
	if err != nil {
		t.Fatalf("Decrypt failed: %v", err)
	}
	if string(decrypted) != plaintext {
		t.Errorf("Decrypt mismatch: got %q, want %q", string(decrypted), plaintext)
	}

	// when: clearing the SecureString
	ss.Clear()

	// then: decryption should fail (key zeroed, GCM tag validation fails)
	_, err = ss.Decrypt()
	if err == nil {
		t.Error("Decrypt should fail after Clear")
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
