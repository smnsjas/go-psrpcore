// Package objects defines PowerShell complex object types.
//
// This package provides Go representations of common PowerShell objects
// that require special handling during serialization, such as PSCredential,
// SecureString, ErrorRecord, and various information records.
//
// # PSCredential
//
// PSCredential wraps a username and SecureString password:
//
//	cred := objects.NewPSCredential("user", securePassword)
//
// # SecureString
//
// SecureString provides encrypted string storage for sensitive data:
//
//	ss, err := objects.NewSecureString("secret")
//	if err != nil {
//		return err
//	}
//	defer ss.Clear() // Clear from memory when done
//
// # ErrorRecord
//
// ErrorRecord represents PowerShell errors with full context:
//
//	err := objects.ErrorRecord{
//	    Exception: "System.Exception",
//	    Message:   "Something went wrong",
//	    // ...
//	}
//
// # Reference
//
// MS-PSRP Section 2.2.5.2: https://docs.microsoft.com/en-us/openspecs/windows_protocols/ms-psrp/
package objects

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"fmt"
	"io"
)

// PSCredential represents a PowerShell PSCredential object.
type PSCredential struct {
	UserName string
	Password *SecureString
}

// NewPSCredential creates a new PSCredential.
func NewPSCredential(userName string, password *SecureString) *PSCredential {
	return &PSCredential{
		UserName: userName,
		Password: password,
	}
}

// Clear securely clears the credential from memory.
func (c *PSCredential) Clear() {
	if c.Password != nil {
		c.Password.Clear()
	}
}

// SecureString represents an encrypted string for sensitive data.
// In a real implementation, this would use DPAPI on Windows.
type SecureString struct {
	encrypted []byte
	key       []byte
}

// NewSecureString creates a SecureString from plaintext.
// Returns an error if cryptographic operations fail.
func NewSecureString(plaintext string) (*SecureString, error) {
	ss := &SecureString{}
	ss.key = make([]byte, 32)

	// Generate random encryption key
	if _, err := io.ReadFull(rand.Reader, ss.key); err != nil {
		return nil, fmt.Errorf("failed to generate encryption key: %w", err)
	}

	// Encrypt the plaintext
	block, err := aes.NewCipher(ss.key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("failed to generate nonce: %w", err)
	}

	ss.encrypted = gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return ss, nil
}

// Decrypt returns the plaintext value.
// The caller should clear the returned slice when done.
func (s *SecureString) Decrypt() ([]byte, error) {
	if len(s.encrypted) == 0 {
		return nil, nil
	}

	block, err := aes.NewCipher(s.key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonceSize := gcm.NonceSize()
	if len(s.encrypted) < nonceSize {
		return nil, io.ErrUnexpectedEOF
	}

	nonce, ciphertext := s.encrypted[:nonceSize], s.encrypted[nonceSize:]
	return gcm.Open(nil, nonce, ciphertext, nil)
}

// NewSecureStringFromEncrypted creates a SecureString from already encrypted bytes.
// This is used during deserialization when an EncryptionProvider has handled the decryption
// (or preservation) of the data.
func NewSecureStringFromEncrypted(encrypted []byte) *SecureString {
	// Copy buffer to avoid retaining reference
	buf := make([]byte, len(encrypted))
	copy(buf, encrypted)
	return &SecureString{
		encrypted: buf,
	}
}

// EncryptedBytes returns the raw encrypted bytes.
// This is used during serialization to pass data to the EncryptionProvider.
func (s *SecureString) EncryptedBytes() []byte {
	// Return a copy to ensure safety
	buf := make([]byte, len(s.encrypted))
	copy(buf, s.encrypted)
	return buf
}

// Clear securely clears the SecureString from memory.
func (s *SecureString) Clear() {
	for i := range s.encrypted {
		s.encrypted[i] = 0
	}
	for i := range s.key {
		s.key[i] = 0
	}
}

// ErrorRecord represents a PowerShell ErrorRecord.
type ErrorRecord struct {
	Exception             ExceptionInfo
	TargetObject          interface{}
	FullyQualifiedErrorId string
	InvocationInfo        *InvocationInfo
	CategoryInfo          CategoryInfo
	ErrorDetails          *ErrorDetails
	PipelineIterationInfo []int
	ScriptStackTrace      string
}

// ExceptionInfo contains exception details.
type ExceptionInfo struct {
	Type           string
	Message        string
	StackTrace     string
	InnerException *ExceptionInfo
}

// InvocationInfo contains command invocation details.
type InvocationInfo struct {
	MyCommand        string
	BoundParameters  map[string]interface{}
	UnboundArguments []interface{}
	ScriptLineNumber int
	OffsetInLine     int
	HistoryId        int64
	ScriptName       string
	Line             string
	PositionMessage  string
	PSScriptRoot     string
	PSCommandPath    string
	InvocationName   string
}

// CategoryInfo contains error category information.
type CategoryInfo struct {
	Category   ErrorCategory
	Activity   string
	Reason     string
	TargetName string
	TargetType string
}

// ErrorCategory represents PowerShell error categories.
type ErrorCategory int

const (
	ErrorCategoryNotSpecified ErrorCategory = iota
	ErrorCategoryOpenError
	ErrorCategoryCloseError
	ErrorCategoryDeviceError
	ErrorCategoryDeadlockDetected
	ErrorCategoryInvalidArgument
	ErrorCategoryInvalidData
	ErrorCategoryInvalidOperation
	ErrorCategoryInvalidResult
	ErrorCategoryInvalidType
	ErrorCategoryMetadataError
	ErrorCategoryNotImplemented
	ErrorCategoryNotInstalled
	ErrorCategoryObjectNotFound
	ErrorCategoryOperationStopped
	ErrorCategoryOperationTimeout
	ErrorCategorySyntaxError
	ErrorCategoryParserError
	ErrorCategoryPermissionDenied
	ErrorCategoryResourceBusy
	ErrorCategoryResourceExists
	ErrorCategoryResourceUnavailable
	ErrorCategoryReadError
	ErrorCategoryWriteError
	ErrorCategoryFromStdErr
	ErrorCategorySecurityError
	ErrorCategoryProtocolError
	ErrorCategoryConnectionError
	ErrorCategoryAuthenticationError
	ErrorCategoryLimitsExceeded
	ErrorCategoryQuotaExceeded
	ErrorCategoryNotEnabled
)

// ErrorDetails contains additional error details.
type ErrorDetails struct {
	Message           string
	RecommendedAction string
}

// ProgressRecord represents a PowerShell progress update.
type ProgressRecord struct {
	ActivityId        int
	ParentActivityId  int
	Activity          string
	StatusDescription string
	CurrentOperation  string
	PercentComplete   int
	SecondsRemaining  int
	RecordType        ProgressRecordType
}

// ProgressRecordType indicates the type of progress record.
type ProgressRecordType int

const (
	ProgressRecordTypeProcessing ProgressRecordType = iota
	ProgressRecordTypeCompleted
)

// InformationRecord represents a PowerShell information record.
type InformationRecord struct {
	MessageData     interface{}
	Source          string
	TimeGenerated   string
	Tags            []string
	User            string
	Computer        string
	ProcessId       uint32
	NativeThreadId  uint32
	ManagedThreadId int
}

// DebugRecord represents a PowerShell debug message.
type DebugRecord struct {
	Message        string
	InvocationInfo *InvocationInfo
}

// VerboseRecord represents a PowerShell verbose message.
type VerboseRecord struct {
	Message        string
	InvocationInfo *InvocationInfo
}

// ScriptBlock represents a PowerShell ScriptBlock.
// Serialization: <SB>text</SB>
type ScriptBlock struct {
	Text string
}

// String returns the script block text.
func (s ScriptBlock) String() string {
	return s.Text
}

// CommandParameter represents a parameter for a PowerShell command.
type CommandParameter struct {
	Name  string
	Value interface{}
}

// Command represents a single PowerShell command (cmdlet or script).
type Command struct {
	Name          string
	IsScript      bool
	UseLocalScope bool
	MergeMyResult string // "None", "Error", "All"
	Parameters    []CommandParameter
}

// PowerShell represents a pipeline of commands to be executed.
// This structure corresponds to the serialized object graph sent in CREATE_PIPELINE.
type PowerShell struct {
	Commands []Command
	IsNested bool
	History  string // "Add", "Clear", "None"
}

// NewPowerShell creates a new PowerShell pipeline object.
func NewPowerShell() *PowerShell {
	return &PowerShell{
		Commands: make([]Command, 0),
		History:  "None",
	}
}

// AddCommand adds a command to the pipeline.
func (p *PowerShell) AddCommand(name string, isScript bool) {
	p.Commands = append(p.Commands, Command{
		Name:          name,
		IsScript:      isScript,
		MergeMyResult: "None",
		Parameters:    make([]CommandParameter, 0),
	})
}

// AddParameter adds a parameter to the last command in the pipeline.
func (p *PowerShell) AddParameter(name string, value interface{}) {
	if len(p.Commands) == 0 {
		return
	}
	idx := len(p.Commands) - 1
	p.Commands[idx].Parameters = append(p.Commands[idx].Parameters, CommandParameter{
		Name:  name,
		Value: value,
	})
}

// ParameterMetadata represents metadata for a command parameter.
type ParameterMetadata struct {
	Name string
	Type string // e.g., "System.String"
}

// CommandMetadata represents metadata for a discovered command.
type CommandMetadata struct {
	Name        string
	CommandType int // 1=Alias, 2=Function, 4=Filter, 8=Cmdlet
	Parameters  map[string]ParameterMetadata
}
