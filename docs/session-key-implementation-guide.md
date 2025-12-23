# Session Key Implementation Guide

This guide shows how to implement SecureString encryption using Go's standard library.

## Required Packages (All Standard Library)

```go
import (
    "crypto/aes"
    "crypto/cipher"
    "crypto/rand"
    "crypto/rsa"
    "crypto/sha256"
    "encoding/base64"
    "fmt"
    "io"
)
```

**No external dependencies needed!** Go's standard library provides everything:
- `crypto/rsa` - RSA key generation and PKCS#1 v1.5 encryption
- `crypto/aes` - AES encryption (256-bit with 32-byte key)
- `crypto/cipher` - CBC mode implementation
- `crypto/rand` - Cryptographically secure random number generation

## Implementation Components

### 1. Session Key Manager

Handles RSA key pair generation and session key exchange.

```go
package crypto

import (
    "crypto/rand"
    "crypto/rsa"
    "crypto/sha256"
    "encoding/base64"
    "fmt"
)

// SessionKeyManager handles RSA key exchange for PSRP session keys
type SessionKeyManager struct {
    privateKey *rsa.PrivateKey
    publicKey  *rsa.PublicKey
    sessionKey []byte // 32 bytes for AES-256
}

// NewSessionKeyManager creates a new session key manager with RSA key pair
func NewSessionKeyManager(keySize int) (*SessionKeyManager, error) {
    if keySize < 2048 {
        keySize = 2048 // Minimum secure key size
    }

    privateKey, err := rsa.GenerateKey(rand.Reader, keySize)
    if err != nil {
        return nil, fmt.Errorf("generate RSA key: %w", err)
    }

    return &SessionKeyManager{
        privateKey: privateKey,
        publicKey:  &privateKey.PublicKey,
    }, nil
}

// PublicKeyBytes returns the DER-encoded public key for PUBLIC_KEY message
func (m *SessionKeyManager) PublicKeyBytes() ([]byte, error) {
    // For PSRP, typically send modulus and exponent
    // MS-PSRP Section 2.2.5.2 specifies the format
    // This is a simplified example - actual implementation should match spec
    return []byte(m.publicKey.N.Bytes()), nil
}

// GenerateSessionKey creates a new 256-bit AES session key
func (m *SessionKeyManager) GenerateSessionKey() ([]byte, error) {
    sessionKey := make([]byte, 32) // 256 bits
    if _, err := io.ReadFull(rand.Reader, sessionKey); err != nil {
        return nil, fmt.Errorf("generate session key: %w", err)
    }
    m.sessionKey = sessionKey
    return sessionKey, nil
}

// EncryptSessionKey encrypts the session key with peer's public key
// Returns base64-encoded ciphertext for ENCRYPTED_SESSION_KEY message
func (m *SessionKeyManager) EncryptSessionKey(peerPublicKey *rsa.PublicKey, sessionKey []byte) (string, error) {
    // IMPORTANT: MS-PSRP uses PKCS#1 v1.5 for backward compatibility
    // Even though it's deprecated for general use, the spec requires it
    ciphertext, err := rsa.EncryptPKCS1v15(rand.Reader, peerPublicKey, sessionKey)
    if err != nil {
        return "", fmt.Errorf("encrypt session key: %w", err)
    }
    return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// DecryptSessionKey decrypts the session key received from peer
func (m *SessionKeyManager) DecryptSessionKey(encryptedKey string) ([]byte, error) {
    ciphertext, err := base64.StdEncoding.DecodeString(encryptedKey)
    if err != nil {
        return nil, fmt.Errorf("decode encrypted session key: %w", err)
    }

    // Use constant-time session key decryption for security
    sessionKey := make([]byte, 32) // Expected 256-bit key
    err = rsa.DecryptPKCS1v15SessionKey(rand.Reader, m.privateKey, ciphertext, sessionKey)
    if err != nil {
        return nil, fmt.Errorf("decrypt session key: %w", err)
    }

    m.sessionKey = sessionKey
    return sessionKey, nil
}

// GetSessionKey returns the current session key
func (m *SessionKeyManager) GetSessionKey() []byte {
    return m.sessionKey
}
```

### 2. AES-256-CBC Encryption Provider

Implements the EncryptionProvider interface for SecureString encryption.

```go
package crypto

import (
    "crypto/aes"
    "crypto/cipher"
    "crypto/rand"
    "fmt"
    "io"
)

// SessionKeyEncryptionProvider implements AES-256-CBC encryption for SecureStrings
type SessionKeyEncryptionProvider struct {
    sessionKey []byte // 32 bytes for AES-256
}

// NewSessionKeyEncryptionProvider creates an encryption provider with the session key
func NewSessionKeyEncryptionProvider(sessionKey []byte) (*SessionKeyEncryptionProvider, error) {
    if len(sessionKey) != 32 {
        return nil, fmt.Errorf("session key must be 32 bytes for AES-256, got %d", len(sessionKey))
    }

    return &SessionKeyEncryptionProvider{
        sessionKey: sessionKey,
    }, nil
}

// Encrypt encrypts plaintext using AES-256-CBC
// Returns IV prepended to ciphertext (as commonly done for CBC mode)
func (p *SessionKeyEncryptionProvider) Encrypt(plaintext []byte) ([]byte, error) {
    // Create AES cipher block
    block, err := aes.NewCipher(p.sessionKey)
    if err != nil {
        return nil, fmt.Errorf("create AES cipher: %w", err)
    }

    // Apply PKCS#7 padding
    paddedPlaintext := pkcs7Pad(plaintext, aes.BlockSize)

    // Generate random IV
    iv := make([]byte, aes.BlockSize) // 16 bytes for AES
    if _, err := io.ReadFull(rand.Reader, iv); err != nil {
        return nil, fmt.Errorf("generate IV: %w", err)
    }

    // Encrypt using CBC mode
    mode := cipher.NewCBCEncrypter(block, iv)
    ciphertext := make([]byte, len(paddedPlaintext))
    mode.CryptBlocks(ciphertext, paddedPlaintext)

    // Prepend IV to ciphertext (standard practice for CBC)
    // Format: [IV (16 bytes)][Ciphertext (variable)]
    result := make([]byte, len(iv)+len(ciphertext))
    copy(result[:aes.BlockSize], iv)
    copy(result[aes.BlockSize:], ciphertext)

    return result, nil
}

// Decrypt decrypts ciphertext using AES-256-CBC
// Expects IV prepended to ciphertext
func (p *SessionKeyEncryptionProvider) Decrypt(data []byte) ([]byte, error) {
    if len(data) < aes.BlockSize {
        return nil, fmt.Errorf("ciphertext too short: %d bytes", len(data))
    }

    // Create AES cipher block
    block, err := aes.NewCipher(p.sessionKey)
    if err != nil {
        return nil, fmt.Errorf("create AES cipher: %w", err)
    }

    // Extract IV and ciphertext
    iv := data[:aes.BlockSize]
    ciphertext := data[aes.BlockSize:]

    // Ciphertext must be multiple of block size
    if len(ciphertext)%aes.BlockSize != 0 {
        return nil, fmt.Errorf("ciphertext is not a multiple of block size")
    }

    // Decrypt using CBC mode
    mode := cipher.NewCBCDecrypter(block, iv)
    plaintext := make([]byte, len(ciphertext))
    mode.CryptBlocks(plaintext, ciphertext)

    // Remove PKCS#7 padding
    unpaddedPlaintext, err := pkcs7Unpad(plaintext, aes.BlockSize)
    if err != nil {
        return nil, fmt.Errorf("remove padding: %w", err)
    }

    return unpaddedPlaintext, nil
}

// pkcs7Pad applies PKCS#7 padding to data
func pkcs7Pad(data []byte, blockSize int) []byte {
    padding := blockSize - (len(data) % blockSize)
    padText := make([]byte, padding)
    for i := range padText {
        padText[i] = byte(padding)
    }
    return append(data, padText...)
}

// pkcs7Unpad removes PKCS#7 padding from data
func pkcs7Unpad(data []byte, blockSize int) ([]byte, error) {
    if len(data) == 0 {
        return nil, fmt.Errorf("empty data")
    }

    padding := int(data[len(data)-1])
    if padding > blockSize || padding == 0 {
        return nil, fmt.Errorf("invalid padding")
    }

    // Verify all padding bytes
    for i := len(data) - padding; i < len(data); i++ {
        if data[i] != byte(padding) {
            return nil, fmt.Errorf("invalid padding bytes")
        }
    }

    return data[:len(data)-padding], nil
}
```

### 3. Integration with Serialization

Update the serializer to use the session key encryption provider:

```go
package serialization

import (
    "github.com/smnsjas/go-psrpcore/crypto"
    "github.com/smnsjas/go-psrpcore/objects"
)

// Example usage:
func ExampleSecureStringWithSessionKey() {
    // 1. Create session key manager
    keyManager, _ := crypto.NewSessionKeyManager(2048)

    // 2. Generate session key (or decrypt received key)
    sessionKey, _ := keyManager.GenerateSessionKey()

    // 3. Create encryption provider
    encProvider, _ := crypto.NewSessionKeyEncryptionProvider(sessionKey)

    // 4. Create serializer with encryption
    serializer := NewSerializerWithEncryption(encProvider)

    // 5. Create and serialize SecureString
    secureStr, _ := objects.NewSecureString("MyPassword123")
    clixml, _ := serializer.Serialize(secureStr)

    // Output: <Objs>...<SS>base64(IV||ciphertext)</SS>...</Objs>
    _ = clixml
}
```

## Message Flow Example

### Client-Side (Initiating Connection)

```go
// 1. Create session key manager
keyManager, err := crypto.NewSessionKeyManager(2048)
if err != nil {
    return err
}

// 2. Send PUBLIC_KEY_REQUEST message
// (implementation depends on transport layer)

// 3. Receive PUBLIC_KEY message from server
// Parse server's public key from message data

// 4. Generate session key
sessionKey, err := keyManager.GenerateSessionKey()
if err != nil {
    return err
}

// 5. Encrypt session key with server's public key
encryptedKey, err := keyManager.EncryptSessionKey(serverPublicKey, sessionKey)
if err != nil {
    return err
}

// 6. Send ENCRYPTED_SESSION_KEY message with encryptedKey

// 7. Create encryption provider for future SecureString operations
encProvider, err := crypto.NewSessionKeyEncryptionProvider(sessionKey)
if err != nil {
    return err
}

// 8. Use encProvider with serializers for PSCredential/SecureString
```

### Server-Side (Receiving Connection)

```go
// 1. Receive PUBLIC_KEY_REQUEST from client

// 2. Create session key manager
keyManager, err := crypto.NewSessionKeyManager(2048)
if err != nil {
    return err
}

// 3. Send PUBLIC_KEY message with public key bytes

// 4. Receive ENCRYPTED_SESSION_KEY from client

// 5. Decrypt session key
sessionKey, err := keyManager.DecryptSessionKey(encryptedKeyFromClient)
if err != nil {
    return err
}

// 6. Create encryption provider
encProvider, err := crypto.NewSessionKeyEncryptionProvider(sessionKey)
if err != nil {
    return err
}

// 7. Use encProvider with serializers
```

## Security Notes

### ⚠️ PKCS#1 v1.5 Deprecation

The MS-PSRP spec requires PKCS#1 v1.5 for backward compatibility, even though it's being deprecated:
- **Only use for session keys** (not arbitrary plaintexts)
- Use `DecryptPKCS1v15SessionKey` (constant-time) not `DecryptPKCS1v15`
- Modern protocols should use RSA-OAEP, but PSRP predates this recommendation

### 🔒 IV Handling

- **Always use a random IV** for each encryption operation
- Never reuse an IV with the same key
- IV can be sent in the clear (prepended to ciphertext)
- IV must be exactly 16 bytes (AES block size)

### 📋 PKCS#7 Padding

- Required for CBC mode when plaintext isn't block-aligned
- .NET uses PKCS#7 padding by default for AES-CBC
- Padding oracle attacks are possible if errors leak timing information
- Consider adding HMAC for authentication (defense-in-depth)

### 🛡️ Protocol Version Handling

For PowerShell v2.4+:
```go
// Check negotiated protocol version
if clientVersion >= "2.4" && serverVersion >= "2.4" {
    // Skip session key exchange
    // Rely on transport security (TLS/SSH)
    encProvider = nil // No encryption needed
}
```

## Testing Strategy

### Unit Tests

```go
func TestSessionKeyEncryption(t *testing.T) {
    // Generate random session key
    sessionKey := make([]byte, 32)
    rand.Read(sessionKey)

    // Create provider
    provider, err := NewSessionKeyEncryptionProvider(sessionKey)
    if err != nil {
        t.Fatal(err)
    }

    // Test round-trip
    plaintext := []byte("Test SecureString Password")
    ciphertext, err := provider.Encrypt(plaintext)
    if err != nil {
        t.Fatal(err)
    }

    decrypted, err := provider.Decrypt(ciphertext)
    if err != nil {
        t.Fatal(err)
    }

    if !bytes.Equal(plaintext, decrypted) {
        t.Errorf("round-trip failed: got %q, want %q", decrypted, plaintext)
    }
}
```

### Integration Tests

Test against real PowerShell:
```bash
# Start PowerShell in server mode (if supported)
pwsh -ServerMode -NoProfile

# Or use SSH remoting
# Your Go client connects and exchanges keys
```

## References

- [crypto/rsa Package](https://pkg.go.dev/crypto/rsa)
- [crypto/aes Package](https://pkg.go.dev/crypto/aes)
- [crypto/cipher Package](https://pkg.go.dev/crypto/cipher)
- [MS-PSRP PUBLIC_KEY Message](https://learn.microsoft.com/en-us/openspecs/windows_protocols/ms-psrp/0d7e1800-598b-4056-8d4c-8cadc61f0163)
- [NIST SP800-38A - CBC Mode](https://csrc.nist.gov/publications/detail/sp/800-38a/final)
- [RFC 8017 - PKCS#1 v2.2](https://www.rfc-editor.org/rfc/rfc8017)

## Next Steps

1. Create `crypto/` package in go-psrpcore
2. Implement SessionKeyManager
3. Implement SessionKeyEncryptionProvider
4. Add message handlers for PUBLIC_KEY, ENCRYPTED_SESSION_KEY, PUBLIC_KEY_REQUEST
5. Integrate with RunspacePool initialization
6. Test against real PowerShell server
7. Add protocol version detection for v2.4+ optimization
