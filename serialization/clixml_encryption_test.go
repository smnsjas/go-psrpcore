package serialization

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/smnsjas/go-psrpcore/objects"
)

// mockEncryptor implements EncryptionProvider for testing.
type mockEncryptor struct{}

func (m *mockEncryptor) Encrypt(data []byte) ([]byte, error) {
	// Simple reverse for testing
	res := make([]byte, len(data))
	for i, b := range data {
		res[len(data)-1-i] = b
	}
	return res, nil
}

func (m *mockEncryptor) Decrypt(data []byte) ([]byte, error) {
	// Simple reverse back
	res := make([]byte, len(data))
	for i, b := range data {
		res[len(data)-1-i] = b
	}
	return res, nil
}

func TestSecureString_Encryption(t *testing.T) {
	// 1. Create mock provider
	provider := &mockEncryptor{}

	// 2. Create Serializer with provider
	ser := NewSerializerWithEncryption(provider)

	// 3. Create SecureString
	raw := []byte("secret")
	ss := objects.NewSecureStringFromEncrypted(raw)

	// 4. Serialize
	encoded, err := ser.Serialize(ss)
	if err != nil {
		t.Fatalf("Serialize failed: %v", err)
	}

	// Verify output contains SS tag
	if !strings.Contains(string(encoded), "<SS>") {
		t.Errorf("expected <SS> tag, got %s", encoded)
	}

	// Verify encryption was used (mock reverses "secret" -> "terces")
	// "terces" in base64 is "dGVyY2Vz"
	expectedB64 := base64.StdEncoding.EncodeToString([]byte("terces"))
	if !strings.Contains(string(encoded), expectedB64) {
		t.Errorf("expected encrypted content %s, got %s", expectedB64, encoded)
	}

	// 5. Deserializer with provider
	deser := NewDeserializerWithEncryption(provider)

	// 6. Deserialize
	items, err := deser.Deserialize([]byte(encoded))
	if err != nil {
		t.Fatalf("Deserialize failed: %v", err)
	}

	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}

	// 7. Verify result
	outSS, ok := items[0].(*objects.SecureString)
	if !ok {
		t.Fatalf("expected *SecureString, got %T", items[0])
	}

	// Verify decrypted content matches original "secret" which is "c2VjcmV0" in base64 or just bytes
	// Wait, NewSecureStringFromEncrypted takes bytes.
	// Our mock Decrypt returns "secret".
	// The implementation calls NewSecureString(string(decrypted)).
	// So implementation treats decrypted data as plaintext string.
	// Let's check objects.go again to see what NewSecureString does.
	// It generates a random key and encrypts it locally.
	// So we can't compare bytes directly. We need to Decrypt() the result.

	decryptedBytes, err := outSS.Decrypt()
	if err != nil {
		t.Fatalf("outSS.Decrypt failed: %v", err)
	}
	if string(decryptedBytes) != "secret" {
		t.Errorf("expected 'secret', got '%s'", string(decryptedBytes))
	}
}

func TestScriptBlock_Serialization(t *testing.T) {
	ser := NewSerializer()
	sb := &objects.ScriptBlock{Text: "Get-Process | Where-Object { $_.Id -eq 1 }"}

	// Serialize
	encoded, err := ser.Serialize(sb)
	if err != nil {
		t.Fatalf("Serialize failed: %v", err)
	}

	// Verify <SB> tag
	if !strings.Contains(string(encoded), "<SB>") {
		t.Errorf("expected <SB> tag, got %s", encoded)
	}
	if !strings.Contains(string(encoded), "Get-Process") {
		t.Errorf("expected script text, got %s", encoded)
	}

	// Deserialize
	deser := NewDeserializer()
	items, err := deser.Deserialize([]byte(encoded))
	if err != nil {
		t.Fatalf("Deserialize failed: %v", err)
	}

	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}

	outSB, ok := items[0].(*objects.ScriptBlock)
	if !ok {
		t.Fatalf("expected *ScriptBlock, got %T", items[0])
	}

	if outSB.Text != sb.Text {
		t.Errorf("expected '%s', got '%s'", sb.Text, outSB.Text)
	}
}
