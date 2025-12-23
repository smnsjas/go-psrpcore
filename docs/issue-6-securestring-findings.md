# Issue #6: SecureString Serialization Format - Research Findings

**Date**: 2025-12-19
**Status**: Research Complete - Implementation Verification Needed

## Executive Summary

The current SecureString implementation uses **local AES-GCM encryption** with per-instance random keys. According to MS-PSRP specification, SecureString values transmitted over PSRP should be encrypted using **AES-256-CBC with a session key** established via RSA key exchange.

## MS-PSRP Specification (Section 2.2.5.1.24)

### Wire Format

- **XML Element**: `<SS>`
- **Content**: Base64-encoded encrypted bytes
- **Encryption Algorithm**: AES-256 (FIPS 197)
- **Cipher Mode**: CBC (Cipher Block Chaining, SP800-38A section 6.2)
- **Encryption Key**: Session key (exchanged via PUBLIC_KEY and ENCRYPTED_SESSION_KEY messages)

### Example from Spec
```xml
<SS>np7uo8n2ZhbN5Pp9LMpf03WLccPK1NQWYFQrg1UzyA8=</SS>
```

### Session Key Exchange Process

1. **Client sends PUBLIC_KEY_REQUEST** (MessageType 0x00010007)
2. **Server responds with PUBLIC_KEY** (MessageType 0x00010005)
   - Contains RSA public key for encrypting the session key
3. **Client generates 256-bit AES session key**
4. **Client sends ENCRYPTED_SESSION_KEY** (MessageType 0x00010006)
   - Session key encrypted with server's RSA public key (RSAES-PKCS-v1_5)
   - Base64 encoded
5. **Both parties use session key for SecureString encryption/decryption**

### Protocol Version Consideration

**Important**: PowerShell v2.4+ (recent versions) **skip SecureString encryption** when both client and server support v2.4+, relying on the secure transport layer (TLS/SSH) instead. However, for backward compatibility with older servers, the session key exchange must still be supported.

## Current Implementation Analysis

### Files Reviewed

- [`objects/objects.go`](../objects/objects.go) - SecureString type definition
- [`serialization/clixml.go`](../serialization/clixml.go) - Serialization logic
- [`serialization/clixml_encryption_test.go`](../serialization/clixml_encryption_test.go) - Encryption tests

### Current Approach

#### SecureString Structure (objects/objects.go:69-72)
```go
type SecureString struct {
    encrypted []byte  // AES-GCM encrypted data with nonce
    key       []byte  // 32-byte random key (local to this instance)
}
```

#### Current Encryption Method
- **Algorithm**: AES-GCM (Galois/Counter Mode)
- **Key**: 256-bit random key generated per SecureString instance
- **Nonce**: Random nonce prepended to ciphertext
- **Storage**: `encrypted = nonce || gcm.Seal(plaintext)`

#### Serialization (clixml.go:358-375)
```go
case *objects.SecureString:
    s.buf.WriteString("<SS")
    s.buf.WriteString(nameAttr)
    s.buf.WriteString(">")
    var data []byte
    if s.encryptor != nil {
        // Encrypt with provider (Session Key)
        var err error
        data, err = s.encryptor.Encrypt(val.EncryptedBytes())
        if err != nil {
            return fmt.Errorf("encrypt secure string: %w", err)
        }
    } else {
        // No provider, use internal encrypted bytes (local protection)
        data = val.EncryptedBytes()
    }
    s.buf.WriteString(base64.StdEncoding.EncodeToString(data))
    s.buf.WriteString("</SS>")
```

#### Deserialization (clixml.go:880-908)
```go
case "SS": // SecureString
    var s string
    if err := d.dec.DecodeElement(&s, &se); err != nil {
        return nil, fmt.Errorf("decode secure string: %w", err)
    }
    data, err := base64.StdEncoding.DecodeString(s)
    if err != nil {
        return nil, fmt.Errorf("invalid base64 in SecureString: %w", err)
    }

    if d.decryptor != nil {
        // Decrypt with provider
        decrypted, err := d.decryptor.Decrypt(data)
        if err != nil {
            return nil, fmt.Errorf("failed to decrypt SecureString: %w", err)
        }
        // Re-wrap in SecureString (using local protection)
        return objects.NewSecureString(string(decrypted))
    }

    // No provider, assume data is already locally protected
    return objects.NewSecureStringFromEncrypted(data), nil
```

### EncryptionProvider Interface (clixml.go:93-97)
```go
type EncryptionProvider interface {
    Encrypt(data []byte) ([]byte, error)
    Decrypt(data []byte) ([]byte, error)
}
```

## Gap Analysis

### ✅ What's Correct

1. **XML Tag**: Using `<SS>` tag is correct
2. **Base64 Encoding**: Properly encoding encrypted bytes as base64
3. **EncryptionProvider Interface**: Abstraction exists for external encryption
4. **Two-Layer Design**: Supports both local protection and wire encryption

### ⚠️ Potential Issues

1. **No Session Key Implementation**: The codebase defines message types for key exchange (PUBLIC_KEY, ENCRYPTED_SESSION_KEY) but no implementation exists

2. **Default Encryption Mismatch**: When no EncryptionProvider is set, SecureString uses **local AES-GCM** which won't be decryptable by PowerShell servers expecting **AES-256-CBC with session key**

3. **Encryption Provider Semantics**: The current flow encrypts the **already-encrypted** local bytes:
   ```
   Plaintext → AES-GCM (local) → encrypted bytes → EncryptionProvider.Encrypt() → wire
   ```

   This creates a double-encryption scenario, which may not match PSRP expectations. The spec implies:
   ```
   Plaintext → AES-256-CBC (session key) → wire
   ```

4. **Missing IV/Nonce for CBC**: AES-CBC requires an Initialization Vector (IV). The spec doesn't explicitly document IV handling, but standard practice is to prepend it to ciphertext or derive it deterministically.

5. **No Protocol Version Negotiation**: No logic to skip encryption for v2.4+ peers

## Recommendations

### High Priority

1. **Implement Session Key Exchange**
   - Add `SessionKeyManager` type to handle PUBLIC_KEY and ENCRYPTED_SESSION_KEY messages
   - Integrate with `RunspacePool` initialization flow
   - Support RSA key generation and PKCS-v1_5 encryption

2. **Implement AES-256-CBC EncryptionProvider**
   - Create `SessionKeyEncryptionProvider` implementing EncryptionProvider
   - Use AES-256-CBC mode (not GCM)
   - Handle IV generation/extraction (prepend IV to ciphertext)
   - Example:
     ```go
     type SessionKeyEncryptionProvider struct {
         sessionKey []byte // 32 bytes
     }

     func (p *SessionKeyEncryptionProvider) Encrypt(plaintext []byte) ([]byte, error) {
         // Generate random IV
         // AES-256-CBC encrypt
         // Return IV || ciphertext
     }
     ```

3. **Refactor SecureString Storage**
   - **Option A**: Store plaintext internally, encrypt only on wire
     ```go
     type SecureString struct {
         plaintext []byte  // Encrypted with OS protection (DPAPI on Windows)
     }
     ```
   - **Option B**: Keep current design but clarify that EncryptionProvider receives **plaintext**, not encrypted bytes
     ```go
     // In Serialize:
     plaintext := val.Decrypt() // Get plaintext
     data := s.encryptor.Encrypt(plaintext) // Encrypt for wire
     ```

### Medium Priority

4. **Test Against Real PowerShell Server**
   - Use `pwsh -ServerMode` or SSH to create a test server
   - Capture wire format of PSCredential with SecureString password
   - Verify IV handling, padding, and base64 format

5. **Add Protocol Version Detection**
   - Track negotiated protocol version in RunspacePool
   - Skip session key exchange for v2.4+ peers
   - Rely on transport security (TLS/SSH)

6. **Improve Documentation**
   - Add session key exchange sequence diagram to docs
   - Document when encryption is/isn't used
   - Add examples for PSCredential serialization

### Low Priority

7. **Integration Tests**
   - Test full key exchange flow
   - Test PSCredential round-trip with real PowerShell
   - Test backward compatibility with older servers

## Open Questions

1. **IV Handling**: How is the IV transmitted for AES-CBC? Prepended to ciphertext, separate field, or deterministic?
   - **Answer needed**: Capture real PSRP traffic or inspect Python implementation

2. **PKCS#7 Padding**: AES-CBC requires padding - is PKCS#7 used?
   - **Likely yes**: Standard for .NET AES-CBC

3. **Key Size Verification**: Spec says 256-bit, but is 128-bit or 192-bit ever used?
   - **Answer**: Test with different PowerShell versions

4. **Local Protection**: Should SecureString use DPAPI on Windows for local storage?
   - **Current**: Uses AES-GCM with random key (cleared on Clear())
   - **Consideration**: DPAPI would provide better local protection but is Windows-only

## Next Steps

1. ✅ **Research complete** - This document
2. ⬜ **Implement SessionKeyManager** - Handle PUBLIC_KEY/ENCRYPTED_SESSION_KEY messages
3. ⬜ **Implement SessionKeyEncryptionProvider** - AES-256-CBC with session key
4. ⬜ **Create integration test** - Test against `pwsh -ServerMode`
5. ⬜ **Refactor SecureString** - Decide on storage strategy (Option A or B)
6. ⬜ **Protocol version support** - Add v2.4+ detection and skip logic

## References

- [MS-PSRP Section 2.2.5.1.24 (Secure String)](https://learn.microsoft.com/en-us/openspecs/windows_protocols/ms-psrp/69b9dc01-a843-4f91-89f8-0205f021a7dd)
- [MS-PSRP Section 2.2.5.2 (PUBLIC_KEY Message)](https://learn.microsoft.com/en-us/openspecs/windows_protocols/ms-psrp/0d7e1800-598b-4056-8d4c-8cadc61f0163)
- [MS-PSRP Section 2.2.5.3 (ENCRYPTED_SESSION_KEY Message)](https://learn.microsoft.com/en-us/openspecs/windows_protocols/ms-psrp/0d7e1800-598b-4056-8d4c-8cadc61f0163)
- [psrpcore Documentation](https://psrpcore.readthedocs.io/en/latest/psrpcore.types.html)
- [PowerShell/PowerShell PR #25774 - Deprecate session key exchange](https://github.com/PowerShell/PowerShell/pull/25774)
- [SP800-38A: Recommendation for Block Cipher Modes](https://csrc.nist.gov/publications/detail/sp/800-38a/final)

## Appendix: Related Message Types

From `messages/messages.go`:

```go
// Session key exchange message types
MessageTypePublicKey           MessageType = 0x00010005  // MS-PSRP 2.2.5.2
MessageTypeEncryptedSessionKey MessageType = 0x00010006  // MS-PSRP 2.2.5.3
MessageTypePublicKeyRequest    MessageType = 0x00010007  // MS-PSRP 2.2.5.4
```

These message types are defined but not yet implemented in the codebase.
