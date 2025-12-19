// Package host defines host callback handling for PSRP.
package host

import (
	"fmt"

	"github.com/jasonmfehr/go-psrp/objects"
)

// MethodID represents a PSHostUserInterface method identifier.
type MethodID int32

// Host method IDs mapping to PSHostUserInterface methods.
const (
	MethodIDRead                MethodID = 1  // Read user input from console
	MethodIDReadLine            MethodID = 2  // Read a line of user input
	MethodIDWriteErrorLine      MethodID = 3  // Write an error message
	MethodIDWrite               MethodID = 4  // Write output to console
	MethodIDWriteDebugLine      MethodID = 5  // Write debug output
	MethodIDWriteVerboseLine    MethodID = 6  // Write verbose output
	MethodIDWriteWarningLine    MethodID = 7  // Write warning output
	MethodIDWriteInformation    MethodID = 8  // Write information output
	MethodIDPrompt              MethodID = 9  // Prompt user with options
	MethodIDPromptForCredential MethodID = 10 // Prompt for PSCredential
	MethodIDPromptForChoice     MethodID = 11 // Prompt user to choose from options
	MethodIDPromptForPassword   MethodID = 12 // Prompt for password (SecureString)
)

// String returns the string representation of a method ID.
func (m MethodID) String() string {
	switch m {
	case MethodIDRead:
		return "Read"
	case MethodIDReadLine:
		return "ReadLine"
	case MethodIDWriteErrorLine:
		return "WriteErrorLine"
	case MethodIDWrite:
		return "Write"
	case MethodIDWriteDebugLine:
		return "WriteDebugLine"
	case MethodIDWriteVerboseLine:
		return "WriteVerboseLine"
	case MethodIDWriteWarningLine:
		return "WriteWarningLine"
	case MethodIDWriteInformation:
		return "WriteInformation"
	case MethodIDPrompt:
		return "Prompt"
	case MethodIDPromptForCredential:
		return "PromptForCredential"
	case MethodIDPromptForChoice:
		return "PromptForChoice"
	case MethodIDPromptForPassword:
		return "PromptForPassword"
	default:
		return fmt.Sprintf("Unknown(%d)", m)
	}
}

// RemoteHostCall represents a host callback request from the server.
// Corresponds to Microsoft.PowerShell.Remoting.Internal.RemoteHostCall
type RemoteHostCall struct {
	CallID           int64         // ci - Unique identifier to correlate call with response
	MethodID         MethodID      // mi - Host method ID
	MethodParameters []interface{} // mp - Method-specific parameters
}

// RemoteHostResponse represents a host callback response to the server.
// Corresponds to Microsoft.PowerShell.Remoting.Internal.RemoteHostResponse
type RemoteHostResponse struct {
	CallID           int64       // ci - Must match CallID from request
	ExceptionRaised  bool        // er - True if an exception occurred
	ReturnValue      interface{} // rv - Return value from host method, or exception if er=true
}

// CallbackHandler manages host callback execution.
// It dispatches incoming host calls to the appropriate Host methods.
type CallbackHandler struct {
	host Host
}

// NewCallbackHandler creates a new callback handler with the given host.
func NewCallbackHandler(host Host) *CallbackHandler {
	return &CallbackHandler{
		host: host,
	}
}

// HandleCall processes a RemoteHostCall and returns a RemoteHostResponse.
func (h *CallbackHandler) HandleCall(call *RemoteHostCall) *RemoteHostResponse {
	response := &RemoteHostResponse{
		CallID:          call.CallID,
		ExceptionRaised: false,
	}

	// Dispatch to appropriate method
	var err error
	switch call.MethodID {
	case MethodIDReadLine:
		response.ReturnValue, err = h.handleReadLine(call)
	case MethodIDWriteErrorLine:
		err = h.handleWriteErrorLine(call)
	case MethodIDWrite:
		err = h.handleWrite(call)
	case MethodIDWriteDebugLine:
		err = h.handleWriteDebugLine(call)
	case MethodIDWriteVerboseLine:
		err = h.handleWriteVerboseLine(call)
	case MethodIDWriteWarningLine:
		err = h.handleWriteWarningLine(call)
	case MethodIDPrompt:
		response.ReturnValue, err = h.handlePrompt(call)
	case MethodIDPromptForCredential:
		response.ReturnValue, err = h.handlePromptForCredential(call)
	case MethodIDPromptForChoice:
		response.ReturnValue, err = h.handlePromptForChoice(call)
	case MethodIDPromptForPassword:
		response.ReturnValue, err = h.handlePromptForPassword(call)
	default:
		err = fmt.Errorf("unsupported host method ID: %d", call.MethodID)
	}

	if err != nil {
		response.ExceptionRaised = true
		response.ReturnValue = err.Error()
	}

	return response
}

// handleReadLine processes ReadLine method calls.
// Parameters: none
// Returns: string
func (h *CallbackHandler) handleReadLine(call *RemoteHostCall) (interface{}, error) {
	if h.host == nil || h.host.UI() == nil {
		return "", nil
	}
	return h.host.UI().ReadLine()
}

// handleWriteErrorLine processes WriteErrorLine method calls.
// Parameters: [0] string (message)
// Returns: none
func (h *CallbackHandler) handleWriteErrorLine(call *RemoteHostCall) error {
	if len(call.MethodParameters) < 1 {
		return fmt.Errorf("WriteErrorLine requires 1 parameter, got %d", len(call.MethodParameters))
	}
	message, ok := call.MethodParameters[0].(string)
	if !ok {
		return fmt.Errorf("WriteErrorLine parameter must be string, got %T", call.MethodParameters[0])
	}
	if h.host != nil && h.host.UI() != nil {
		h.host.UI().WriteErrorLine(message)
	}
	return nil
}

// handleWrite processes Write method calls.
// Parameters: [0] string (text)
// Returns: none
func (h *CallbackHandler) handleWrite(call *RemoteHostCall) error {
	if len(call.MethodParameters) < 1 {
		return fmt.Errorf("Write requires 1 parameter, got %d", len(call.MethodParameters))
	}
	text, ok := call.MethodParameters[0].(string)
	if !ok {
		return fmt.Errorf("Write parameter must be string, got %T", call.MethodParameters[0])
	}
	if h.host != nil && h.host.UI() != nil {
		h.host.UI().Write(text)
	}
	return nil
}

// handleWriteDebugLine processes WriteDebugLine method calls.
// Parameters: [0] string (message)
// Returns: none
func (h *CallbackHandler) handleWriteDebugLine(call *RemoteHostCall) error {
	if len(call.MethodParameters) < 1 {
		return fmt.Errorf("WriteDebugLine requires 1 parameter, got %d", len(call.MethodParameters))
	}
	message, ok := call.MethodParameters[0].(string)
	if !ok {
		return fmt.Errorf("WriteDebugLine parameter must be string, got %T", call.MethodParameters[0])
	}
	if h.host != nil && h.host.UI() != nil {
		h.host.UI().WriteDebugLine(message)
	}
	return nil
}

// handleWriteVerboseLine processes WriteVerboseLine method calls.
// Parameters: [0] string (message)
// Returns: none
func (h *CallbackHandler) handleWriteVerboseLine(call *RemoteHostCall) error {
	if len(call.MethodParameters) < 1 {
		return fmt.Errorf("WriteVerboseLine requires 1 parameter, got %d", len(call.MethodParameters))
	}
	message, ok := call.MethodParameters[0].(string)
	if !ok {
		return fmt.Errorf("WriteVerboseLine parameter must be string, got %T", call.MethodParameters[0])
	}
	if h.host != nil && h.host.UI() != nil {
		h.host.UI().WriteVerboseLine(message)
	}
	return nil
}

// handleWriteWarningLine processes WriteWarningLine method calls.
// Parameters: [0] string (message)
// Returns: none
func (h *CallbackHandler) handleWriteWarningLine(call *RemoteHostCall) error {
	if len(call.MethodParameters) < 1 {
		return fmt.Errorf("WriteWarningLine requires 1 parameter, got %d", len(call.MethodParameters))
	}
	message, ok := call.MethodParameters[0].(string)
	if !ok {
		return fmt.Errorf("WriteWarningLine parameter must be string, got %T", call.MethodParameters[0])
	}
	if h.host != nil && h.host.UI() != nil {
		h.host.UI().WriteWarningLine(message)
	}
	return nil
}

// handlePrompt processes Prompt method calls.
// Parameters: [0] string (caption), [1] string (message), [2] []FieldDescription
// Returns: map[string]interface{} (field name -> value)
func (h *CallbackHandler) handlePrompt(call *RemoteHostCall) (interface{}, error) {
	if len(call.MethodParameters) < 3 {
		return nil, fmt.Errorf("Prompt requires 3 parameters, got %d", len(call.MethodParameters))
	}

	caption, ok := call.MethodParameters[0].(string)
	if !ok {
		return nil, fmt.Errorf("Prompt caption must be string, got %T", call.MethodParameters[0])
	}

	message, ok := call.MethodParameters[1].(string)
	if !ok {
		return nil, fmt.Errorf("Prompt message must be string, got %T", call.MethodParameters[1])
	}

	// Convert parameter 2 to []FieldDescription
	// TODO: Implement proper deserialization from CLIXML
	descriptions := []FieldDescription{}

	if h.host == nil || h.host.UI() == nil {
		return make(map[string]interface{}), nil
	}
	return h.host.UI().Prompt(caption, message, descriptions)
}

// handlePromptForCredential processes PromptForCredential method calls.
// Parameters: [0] string (caption), [1] string (message), [2] string (userName), [3] string (targetName)
// Returns: *objects.PSCredential
func (h *CallbackHandler) handlePromptForCredential(call *RemoteHostCall) (interface{}, error) {
	if len(call.MethodParameters) < 4 {
		return nil, fmt.Errorf("PromptForCredential requires 4 parameters, got %d", len(call.MethodParameters))
	}

	caption, ok := call.MethodParameters[0].(string)
	if !ok {
		return nil, fmt.Errorf("PromptForCredential caption must be string, got %T", call.MethodParameters[0])
	}

	message, ok := call.MethodParameters[1].(string)
	if !ok {
		return nil, fmt.Errorf("PromptForCredential message must be string, got %T", call.MethodParameters[1])
	}

	userName, ok := call.MethodParameters[2].(string)
	if !ok {
		return nil, fmt.Errorf("PromptForCredential userName must be string, got %T", call.MethodParameters[2])
	}

	targetName, ok := call.MethodParameters[3].(string)
	if !ok {
		return nil, fmt.Errorf("PromptForCredential targetName must be string, got %T", call.MethodParameters[3])
	}

	if h.host == nil || h.host.UI() == nil {
		return nil, nil
	}
	return h.host.UI().PromptForCredential(caption, message, userName, targetName, CredentialTypeDefault, CredentialUIOptionNone)
}

// handlePromptForChoice processes PromptForChoice method calls.
// Parameters: [0] string (caption), [1] string (message), [2] []ChoiceDescription, [3] int (defaultChoice)
// Returns: int (selected choice index)
func (h *CallbackHandler) handlePromptForChoice(call *RemoteHostCall) (interface{}, error) {
	if len(call.MethodParameters) < 4 {
		return nil, fmt.Errorf("PromptForChoice requires 4 parameters, got %d", len(call.MethodParameters))
	}

	caption, ok := call.MethodParameters[0].(string)
	if !ok {
		return nil, fmt.Errorf("PromptForChoice caption must be string, got %T", call.MethodParameters[0])
	}

	message, ok := call.MethodParameters[1].(string)
	if !ok {
		return nil, fmt.Errorf("PromptForChoice message must be string, got %T", call.MethodParameters[1])
	}

	// Convert parameter 2 to []ChoiceDescription
	// TODO: Implement proper deserialization from CLIXML
	choices := []ChoiceDescription{}

	var defaultChoice int
	switch v := call.MethodParameters[3].(type) {
	case int:
		defaultChoice = v
	case int32:
		defaultChoice = int(v)
	case int64:
		defaultChoice = int(v)
	default:
		return nil, fmt.Errorf("PromptForChoice defaultChoice must be int, got %T", call.MethodParameters[3])
	}

	if h.host == nil || h.host.UI() == nil {
		return defaultChoice, nil
	}
	return h.host.UI().PromptForChoice(caption, message, choices, defaultChoice)
}

// handlePromptForPassword processes PromptForPassword method calls.
// Parameters: [0] string (caption), [1] string (message)
// Returns: *objects.SecureString
func (h *CallbackHandler) handlePromptForPassword(call *RemoteHostCall) (interface{}, error) {
	// PromptForPassword is typically implemented as ReadLineAsSecureString
	if h.host == nil || h.host.UI() == nil {
		return objects.NewSecureString(""), nil
	}
	return h.host.UI().ReadLineAsSecureString()
}
