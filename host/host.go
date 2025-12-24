// Package host defines the host callback interface for interactive PSRP sessions.
//
// When PowerShell needs to interact with the user (prompts, ReadLine, etc.),
// it sends host call messages to the client. The client must implement the
// Host interface to handle these callbacks.
//
// # Host Methods
//
// The Host interface maps to PowerShell's PSHost and PSHostUserInterface:
//
//   - ReadLine: Read a line of input from the user
//   - ReadLineAsSecureString: Read sensitive input
//   - Write/WriteLine: Output text
//   - WriteError/Warning/Debug/Verbose: Stream-specific output
//   - Prompt: Display a prompt and get responses
//   - PromptForCredential: Get username/password
//   - PromptForChoice: Display choices and get selection
//
// # Default Implementation
//
// A default no-op implementation is provided for non-interactive scenarios:
//
//	host := host.NewNullHost()
//
// # Reference
//
// MS-PSRP Section 2.2.3.17: https://docs.microsoft.com/en-us/openspecs/windows_protocols/ms-psrp/
package host

import "github.com/smnsjas/go-psrpcore/objects"

// Host defines the interface for handling PowerShell host callbacks.
type Host interface {
	// GetName returns the host name.
	GetName() string

	// GetVersion returns the host version.
	GetVersion() Version

	// GetInstanceId returns a unique identifier for this host instance.
	GetInstanceId() string

	// GetCurrentCulture returns the current culture (e.g., "en-US").
	GetCurrentCulture() string

	// GetCurrentUICulture returns the current UI culture.
	GetCurrentUICulture() string

	// UI returns the user interface implementation.
	UI() HostUI
}

// HostUI defines the user interface callbacks.
type HostUI interface {
	// ReadLine reads a line of text from the user.
	ReadLine() (string, error)

	// ReadLineAsSecureString reads sensitive input.
	ReadLineAsSecureString() (*objects.SecureString, error)

	// Write outputs text without a newline.
	Write(text string)

	// WriteLine outputs text with a newline.
	WriteLine(text string)

	// WriteErrorLine outputs error text.
	WriteErrorLine(text string)

	// WriteDebugLine outputs debug text.
	WriteDebugLine(text string)

	// WriteVerboseLine outputs verbose text.
	WriteVerboseLine(text string)

	// WriteWarningLine outputs warning text.
	WriteWarningLine(text string)

	// WriteProgress outputs a progress record.
	WriteProgress(sourceID int64, record *objects.ProgressRecord)

	// Prompt displays prompts and returns responses.
	Prompt(caption, message string, descriptions []FieldDescription) (map[string]interface{}, error)

	// PromptForCredential prompts for credentials.
	PromptForCredential(caption, message, userName, targetName string, allowedCredentialTypes CredentialTypes, options CredentialUIOptions) (*objects.PSCredential, error)

	// PromptForChoice displays choices and returns the selection.
	PromptForChoice(caption, message string, choices []ChoiceDescription, defaultChoice int) (int, error)
}

// Version represents a host version.
type Version struct {
	Major    int
	Minor    int
	Build    int
	Revision int
}

// FieldDescription describes a prompt field.
type FieldDescription struct {
	Name                  string
	Label                 string
	ParameterTypeName     string
	ParameterTypeFullName string
	HelpMessage           string
	IsMandatory           bool
}

// ChoiceDescription describes a choice option.
type ChoiceDescription struct {
	Label      string
	HelpMessage string
}

// CredentialTypes specifies allowed credential types.
type CredentialTypes int

const (
	CredentialTypeGeneric CredentialTypes = 1 << iota
	CredentialTypeDomain
	CredentialTypeDefault = CredentialTypeGeneric | CredentialTypeDomain
)

// CredentialUIOptions specifies credential UI options.
type CredentialUIOptions int

const (
	CredentialUIOptionNone CredentialUIOptions = iota
	CredentialUIOptionValidateUserNameSyntax
	CredentialUIOptionAlwaysPrompt
	CredentialUIOptionReadOnlyUserName
)

// NullHost provides a no-op host implementation for non-interactive scenarios.
type NullHost struct {
	name    string
	version Version
}

// NewNullHost creates a new NullHost.
func NewNullHost() *NullHost {
	return &NullHost{
		name: "go-psrp",
		version: Version{
			Major: 1,
			Minor: 0,
		},
	}
}

func (h *NullHost) GetName() string           { return h.name }
func (h *NullHost) GetVersion() Version       { return h.version }
func (h *NullHost) GetInstanceId() string     { return "00000000-0000-0000-0000-000000000000" }
func (h *NullHost) GetCurrentCulture() string { return "en-US" }
func (h *NullHost) GetCurrentUICulture() string { return "en-US" }
func (h *NullHost) UI() HostUI                { return &NullHostUI{} }

// NullHostUI provides a no-op HostUI implementation.
type NullHostUI struct{}

func (ui *NullHostUI) ReadLine() (string, error) { return "", nil }
func (ui *NullHostUI) ReadLineAsSecureString() (*objects.SecureString, error) {
	return objects.NewSecureString("")
}
func (ui *NullHostUI) Write(text string)                                    {}
func (ui *NullHostUI) WriteLine(text string)                                {}
func (ui *NullHostUI) WriteErrorLine(text string)                           {}
func (ui *NullHostUI) WriteDebugLine(text string)                           {}
func (ui *NullHostUI) WriteVerboseLine(text string)                         {}
func (ui *NullHostUI) WriteWarningLine(text string)                         {}
func (ui *NullHostUI) WriteProgress(sourceID int64, record *objects.ProgressRecord) {}

func (ui *NullHostUI) Prompt(caption, message string, descriptions []FieldDescription) (map[string]interface{}, error) {
	return make(map[string]interface{}), nil
}

func (ui *NullHostUI) PromptForCredential(caption, message, userName, targetName string, allowedCredentialTypes CredentialTypes, options CredentialUIOptions) (*objects.PSCredential, error) {
	return nil, nil
}

func (ui *NullHostUI) PromptForChoice(caption, message string, choices []ChoiceDescription, defaultChoice int) (int, error) {
	return defaultChoice, nil
}
