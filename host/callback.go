// Package host defines host callback handling for PSRP.
package host

import (
	"fmt"

	"github.com/smnsjas/go-psrpcore/objects"
	"github.com/smnsjas/go-psrpcore/serialization"
)

// MethodID represents a PSHostUserInterface method identifier.
// MS-PSRP Section 2.2.3.17 defines all 56 method identifiers.
type MethodID int32

// PSHost methods (1-8)
const (
	MethodIDGetName             MethodID = 1 // Get host name
	MethodIDGetVersion          MethodID = 2 // Get host version
	MethodIDGetInstanceID       MethodID = 3 // Get host instance ID (GUID)
	MethodIDGetCurrentCulture   MethodID = 4 // Get current culture (e.g., "en-US")
	MethodIDGetCurrentUICulture MethodID = 5 // Get current UI culture
	MethodIDSetShouldExit       MethodID = 6 // Set exit code and exit
	MethodIDEnterNestedPrompt   MethodID = 7 // Enter nested prompt
	MethodIDExitNestedPrompt    MethodID = 8 // Exit nested prompt
)

// PSHost application notification methods (9-10)
const (
	MethodIDNotifyBeginApplication MethodID = 9  // Notify application started
	MethodIDNotifyEndApplication   MethodID = 10 // Notify application ended
)

// PSHostUserInterface methods (11-26)
const (
	MethodIDReadLine               MethodID = 11 // Read a line of user input
	MethodIDReadLineAsSecureString MethodID = 12 // Read sensitive input
	MethodIDWrite1                 MethodID = 13 // Write(string value)
	MethodIDWrite2                 MethodID = 14 // Write(ConsoleColor foreground, ConsoleColor background, string value)
	MethodIDWriteLine1             MethodID = 15 // WriteLine()
	MethodIDWriteLine2             MethodID = 16 // WriteLine(string value)
	MethodIDWriteLine3             MethodID = 17 // WriteLine(ConsoleColor fg, bg, string)
	MethodIDWriteErrorLine         MethodID = 18 // Write an error message
	MethodIDWriteDebugLine         MethodID = 19 // Write debug output
	MethodIDWriteProgress          MethodID = 20 // Write progress record
	MethodIDWriteVerboseLine       MethodID = 21 // Write verbose output
	MethodIDWriteWarningLine       MethodID = 22 // Write warning output
	MethodIDPrompt                 MethodID = 23 // Prompt user with fields
	MethodIDPromptForCredential1   MethodID = 24 // PromptForCredential(caption, message, userName, targetName)
	// PromptForCredential2 has all params
	MethodIDPromptForCredential2 MethodID = 25
	MethodIDPromptForChoice      MethodID = 26 // Prompt user to choose from options
)

// PSHostRawUserInterface methods (27-51)
const (
	MethodIDGetForegroundColor       MethodID = 27 // Get console foreground color
	MethodIDSetForegroundColor       MethodID = 28 // Set console foreground color
	MethodIDGetBackgroundColor       MethodID = 29 // Get console background color
	MethodIDSetBackgroundColor       MethodID = 30 // Set console background color
	MethodIDGetCursorPosition        MethodID = 31 // Get cursor position
	MethodIDSetCursorPosition        MethodID = 32 // Set cursor position
	MethodIDGetWindowPosition        MethodID = 33 // Get window position
	MethodIDSetWindowPosition        MethodID = 34 // Set window position
	MethodIDGetCursorSize            MethodID = 35 // Get cursor size
	MethodIDSetCursorSize            MethodID = 36 // Set cursor size
	MethodIDGetBufferSize            MethodID = 37 // Get buffer size
	MethodIDSetBufferSize            MethodID = 38 // Set buffer size
	MethodIDGetWindowSize            MethodID = 39 // Get window size
	MethodIDSetWindowSize            MethodID = 40 // Set window size
	MethodIDGetWindowTitle           MethodID = 41 // Get window title
	MethodIDSetWindowTitle           MethodID = 42 // Set window title
	MethodIDGetMaxWindowSize         MethodID = 43 // Get maximum window size
	MethodIDGetMaxPhysicalWindowSize MethodID = 44 // Get maximum physical window size
	MethodIDGetKeyAvailable          MethodID = 45 // Check if key is available
	MethodIDReadKey                  MethodID = 46 // Read a key press
	MethodIDFlushInputBuffer         MethodID = 47 // Flush input buffer
	MethodIDSetBufferContents1       MethodID = 48 // Set buffer contents (single rectangle)
	MethodIDSetBufferContents2       MethodID = 49 // Set buffer contents (with coordinates)
	MethodIDGetBufferContents        MethodID = 50 // Get buffer contents
	MethodIDScrollBufferContents     MethodID = 51 // Scroll buffer contents
)

// Runspace push/pop methods (52-55)
const (
	MethodIDPushRunspace        MethodID = 52 // Push runspace onto stack
	MethodIDPopRunspace         MethodID = 53 // Pop runspace from stack
	MethodIDGetIsRunspacePushed MethodID = 54 // Check if runspace is pushed
	MethodIDGetRunspace         MethodID = 55 // Get current runspace
)

// Additional PSHostUserInterface methods (56)
const (
	MethodIDPromptForChoiceMultipleSelection MethodID = 56 // Prompt for multiple choices
)

// Default values for host responses
const (
	// DefaultCulture is the default culture string returned when no host is configured.
	DefaultCulture = "en-US"
)

// methodNames maps MethodID values to their string representations.
var methodNames = map[MethodID]string{
	// PSHost methods (1-8)
	MethodIDGetName:             "GetName",
	MethodIDGetVersion:          "GetVersion",
	MethodIDGetInstanceID:       "GetInstanceId",
	MethodIDGetCurrentCulture:   "GetCurrentCulture",
	MethodIDGetCurrentUICulture: "GetCurrentUICulture",
	MethodIDSetShouldExit:       "SetShouldExit",
	MethodIDEnterNestedPrompt:   "EnterNestedPrompt",
	MethodIDExitNestedPrompt:    "ExitNestedPrompt",
	// Application notification methods (9-10)
	MethodIDNotifyBeginApplication: "NotifyBeginApplication",
	MethodIDNotifyEndApplication:   "NotifyEndApplication",
	// PSHostUserInterface methods (11-26)
	MethodIDReadLine:               "ReadLine",
	MethodIDReadLineAsSecureString: "ReadLineAsSecureString",
	MethodIDWrite1:                 "Write1",
	MethodIDWrite2:                 "Write2",
	MethodIDWriteLine1:             "WriteLine1",
	MethodIDWriteLine2:             "WriteLine2",
	MethodIDWriteLine3:             "WriteLine3",
	MethodIDWriteErrorLine:         "WriteErrorLine",
	MethodIDWriteDebugLine:         "WriteDebugLine",
	MethodIDWriteProgress:          "WriteProgress",
	MethodIDWriteVerboseLine:       "WriteVerboseLine",
	MethodIDWriteWarningLine:       "WriteWarningLine",
	MethodIDPrompt:                 "Prompt",
	MethodIDPromptForCredential1:   "PromptForCredential1",
	MethodIDPromptForCredential2:   "PromptForCredential2",
	MethodIDPromptForChoice:        "PromptForChoice",
	// RawUI methods (27-51)
	MethodIDGetForegroundColor:       "GetForegroundColor",
	MethodIDSetForegroundColor:       "SetForegroundColor",
	MethodIDGetBackgroundColor:       "GetBackgroundColor",
	MethodIDSetBackgroundColor:       "SetBackgroundColor",
	MethodIDGetCursorPosition:        "GetCursorPosition",
	MethodIDSetCursorPosition:        "SetCursorPosition",
	MethodIDGetWindowPosition:        "GetWindowPosition",
	MethodIDSetWindowPosition:        "SetWindowPosition",
	MethodIDGetCursorSize:            "GetCursorSize",
	MethodIDSetCursorSize:            "SetCursorSize",
	MethodIDGetBufferSize:            "GetBufferSize",
	MethodIDSetBufferSize:            "SetBufferSize",
	MethodIDGetWindowSize:            "GetWindowSize",
	MethodIDSetWindowSize:            "SetWindowSize",
	MethodIDGetWindowTitle:           "GetWindowTitle",
	MethodIDSetWindowTitle:           "SetWindowTitle",
	MethodIDGetMaxWindowSize:         "GetMaxWindowSize",
	MethodIDGetMaxPhysicalWindowSize: "GetMaxPhysicalWindowSize",
	MethodIDGetKeyAvailable:          "GetKeyAvailable",
	MethodIDReadKey:                  "ReadKey",
	MethodIDFlushInputBuffer:         "FlushInputBuffer",
	MethodIDSetBufferContents1:       "SetBufferContents1",
	MethodIDSetBufferContents2:       "SetBufferContents2",
	MethodIDGetBufferContents:        "GetBufferContents",
	MethodIDScrollBufferContents:     "ScrollBufferContents",
	// Runspace methods (52-55)
	MethodIDPushRunspace:        "PushRunspace",
	MethodIDPopRunspace:         "PopRunspace",
	MethodIDGetIsRunspacePushed: "GetIsRunspacePushed",
	MethodIDGetRunspace:         "GetRunspace",
	// Additional methods (56)
	MethodIDPromptForChoiceMultipleSelection: "PromptForChoiceMultipleSelection",
}

// String returns the string representation of a method ID.
func (m MethodID) String() string {
	if name, ok := methodNames[m]; ok {
		return name
	}
	return fmt.Sprintf("Unknown(%d)", m)
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
	CallID          int64       // ci - Must match CallID from request
	ExceptionRaised bool        // er - True if an exception occurred
	ReturnValue     interface{} // rv - Return value from host method, or exception if er=true
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
//
//nolint:gocyclo // Method dispatch - inherent complexity from MS-PSRP spec
func (h *CallbackHandler) HandleCall(call *RemoteHostCall) *RemoteHostResponse {
	response := &RemoteHostResponse{
		CallID:          call.CallID,
		ExceptionRaised: false,
	}

	// Dispatch to appropriate method
	var err error
	switch call.MethodID {
	// PSHost methods (1-8)
	case MethodIDGetName:
		response.ReturnValue, err = h.handleGetName(call)
	case MethodIDGetVersion:
		response.ReturnValue, err = h.handleGetVersion(call)
	case MethodIDGetInstanceID:
		response.ReturnValue, err = h.handleGetInstanceID(call)
	case MethodIDGetCurrentCulture:
		response.ReturnValue, err = h.handleGetCurrentCulture(call)
	case MethodIDGetCurrentUICulture:
		response.ReturnValue, err = h.handleGetCurrentUICulture(call)
	case MethodIDSetShouldExit:
		err = h.handleSetShouldExit(call)
	case MethodIDEnterNestedPrompt:
		err = h.handleEnterNestedPrompt(call)
	case MethodIDExitNestedPrompt:
		err = h.handleExitNestedPrompt(call)

	// Application notification methods (9-10)
	case MethodIDNotifyBeginApplication:
		err = h.handleNotifyBeginApplication(call)
	case MethodIDNotifyEndApplication:
		err = h.handleNotifyEndApplication(call)

	// PSHostUserInterface methods (11-26)
	case MethodIDReadLine:
		response.ReturnValue, err = h.handleReadLine(call)
	case MethodIDReadLineAsSecureString:
		response.ReturnValue, err = h.handleReadLineAsSecureString(call)
	case MethodIDWrite1:
		err = h.handleWrite1(call)
	case MethodIDWrite2:
		err = h.handleWrite2(call)
	case MethodIDWriteLine1:
		err = h.handleWriteLine1(call)
	case MethodIDWriteLine2:
		err = h.handleWriteLine2(call)
	case MethodIDWriteLine3:
		err = h.handleWriteLine3(call)
	case MethodIDWriteErrorLine:
		err = h.handleWriteErrorLine(call)
	case MethodIDWriteDebugLine:
		err = h.handleWriteDebugLine(call)
	case MethodIDWriteProgress:
		err = h.handleWriteProgress(call)
	case MethodIDWriteVerboseLine:
		err = h.handleWriteVerboseLine(call)
	case MethodIDWriteWarningLine:
		err = h.handleWriteWarningLine(call)
	case MethodIDPrompt:
		response.ReturnValue, err = h.handlePrompt(call)
	case MethodIDPromptForCredential1:
		response.ReturnValue, err = h.handlePromptForCredential1(call)
	case MethodIDPromptForCredential2:
		response.ReturnValue, err = h.handlePromptForCredential2(call)
	case MethodIDPromptForChoice:
		response.ReturnValue, err = h.handlePromptForChoice(call)

	// RawUI methods (27-51) - Return unsupported for now
	case MethodIDGetForegroundColor,
		MethodIDSetForegroundColor,
		MethodIDGetBackgroundColor,
		MethodIDSetBackgroundColor,
		MethodIDGetCursorPosition,
		MethodIDSetCursorPosition,
		MethodIDGetWindowPosition,
		MethodIDSetWindowPosition,
		MethodIDGetCursorSize,
		MethodIDSetCursorSize,
		MethodIDGetBufferSize,
		MethodIDSetBufferSize,
		MethodIDGetWindowSize,
		MethodIDSetWindowSize,
		MethodIDGetWindowTitle,
		MethodIDSetWindowTitle,
		MethodIDGetMaxWindowSize,
		MethodIDGetMaxPhysicalWindowSize,
		MethodIDGetKeyAvailable,
		MethodIDReadKey,
		MethodIDFlushInputBuffer,
		MethodIDSetBufferContents1,
		MethodIDSetBufferContents2,
		MethodIDGetBufferContents,
		MethodIDScrollBufferContents:
		err = fmt.Errorf("RawUI method %s not implemented", call.MethodID)

	// Runspace methods (52-55) - Return unsupported for now
	case MethodIDPushRunspace,
		MethodIDPopRunspace,
		MethodIDGetIsRunspacePushed,
		MethodIDGetRunspace:
		err = fmt.Errorf("runspace method %s not implemented", call.MethodID)

	// Additional methods (56)
	case MethodIDPromptForChoiceMultipleSelection:
		response.ReturnValue, err = h.handlePromptForChoiceMultipleSelection(call)

	default:
		err = fmt.Errorf("unsupported host method ID: %d", call.MethodID)
	}

	if err != nil {
		response.ExceptionRaised = true
		response.ReturnValue = err.Error()
	}

	return response
}

// convertToFieldDescriptions converts a parameter to []FieldDescription.
// Handles lists of PSObjects, individual PSObjects, and native Go []FieldDescription.
func convertToFieldDescriptions(param interface{}) ([]FieldDescription, error) {
	var result []FieldDescription

	switch v := param.(type) {
	case []FieldDescription:
		// Already the correct type (for tests and direct usage)
		return v, nil
	case []interface{}:
		// List of field descriptions from CLIXML
		for _, item := range v {
			fd, err := convertToFieldDescription(item)
			if err != nil {
				return nil, err
			}
			result = append(result, fd)
		}
	case *serialization.PSObject:
		// Single field description
		fd, err := convertToFieldDescription(v)
		if err != nil {
			return nil, err
		}
		result = append(result, fd)
	default:
		return nil, fmt.Errorf("expected list or PSObject for field descriptions, got %T", param)
	}

	return result, nil
}

// convertToFieldDescription converts a PSObject or map to FieldDescription.
func convertToFieldDescription(obj interface{}) (FieldDescription, error) {
	fd := FieldDescription{}

	var props map[string]interface{}
	switch v := obj.(type) {
	case *serialization.PSObject:
		props = v.Properties
	case map[string]interface{}:
		props = v
	default:
		return fd, fmt.Errorf("expected PSObject or map for field description, got %T", obj)
	}

	if name, ok := props["Name"].(string); ok {
		fd.Name = name
	}
	if label, ok := props["Label"].(string); ok {
		fd.Label = label
	}
	if paramType, ok := props["ParameterTypeName"].(string); ok {
		fd.ParameterTypeName = paramType
	}
	if paramFullType, ok := props["ParameterTypeFullName"].(string); ok {
		fd.ParameterTypeFullName = paramFullType
	}
	if help, ok := props["HelpMessage"].(string); ok {
		fd.HelpMessage = help
	}
	if mandatory, ok := props["IsMandatory"].(bool); ok {
		fd.IsMandatory = mandatory
	}

	return fd, nil
}

// convertToChoiceDescriptions converts a parameter to []ChoiceDescription.
// Handles lists of PSObjects, individual PSObjects, and native Go []ChoiceDescription.
func convertToChoiceDescriptions(param interface{}) ([]ChoiceDescription, error) {
	var result []ChoiceDescription

	switch v := param.(type) {
	case []ChoiceDescription:
		// Already the correct type (for tests and direct usage)
		return v, nil
	case []interface{}:
		// List of choice descriptions from CLIXML
		for _, item := range v {
			cd, err := convertToChoiceDescription(item)
			if err != nil {
				return nil, err
			}
			result = append(result, cd)
		}
	case *serialization.PSObject:
		// Single choice description
		cd, err := convertToChoiceDescription(v)
		if err != nil {
			return nil, err
		}
		result = append(result, cd)
	default:
		return nil, fmt.Errorf("expected list or PSObject for choice descriptions, got %T", param)
	}

	return result, nil
}

// convertToChoiceDescription converts a PSObject or map to ChoiceDescription.
func convertToChoiceDescription(obj interface{}) (ChoiceDescription, error) {
	cd := ChoiceDescription{}

	var props map[string]interface{}
	switch v := obj.(type) {
	case *serialization.PSObject:
		props = v.Properties
	case map[string]interface{}:
		props = v
	default:
		return cd, fmt.Errorf("expected PSObject or map for choice description, got %T", obj)
	}

	if label, ok := props["Label"].(string); ok {
		cd.Label = label
	}
	if help, ok := props["HelpMessage"].(string); ok {
		cd.HelpMessage = help
	}

	return cd, nil
}

// ========== PSHost Method Handlers (1-8) ==========

// handleGetName processes GetName method calls.
// Parameters: none
// Returns: string
//
//nolint:unparam // check is required by interface
func (h *CallbackHandler) handleGetName(_ *RemoteHostCall) (interface{}, error) {
	if h.host == nil {
		return "", nil
	}
	return h.host.GetName(), nil
}

// handleGetVersion processes GetVersion method calls.
// Parameters: none
// Returns: Version object
//
//nolint:unparam // check is required by interface
func (h *CallbackHandler) handleGetVersion(_ *RemoteHostCall) (interface{}, error) {
	if h.host == nil {
		return Version{}, nil
	}
	return h.host.GetVersion(), nil
}

// handleGetInstanceID processes GetInstanceId method calls.
// Parameters: none
// Returns: string (GUID)
//
//nolint:unparam // check is required by interface
func (h *CallbackHandler) handleGetInstanceID(_ *RemoteHostCall) (interface{}, error) {
	if h.host == nil {
		return "00000000-0000-0000-0000-000000000000", nil
	}
	return h.host.GetInstanceID(), nil
}

// handleGetCurrentCulture processes GetCurrentCulture method calls.
// Parameters: none
// Returns: string (e.g., "en-US")
//
//nolint:unparam // check is required by interface
func (h *CallbackHandler) handleGetCurrentCulture(_ *RemoteHostCall) (interface{}, error) {
	if h.host == nil {
		return DefaultCulture, nil
	}
	return h.host.GetCurrentCulture(), nil
}

// handleGetCurrentUICulture processes GetCurrentUICulture method calls.
// Parameters: none
// Returns: string (e.g., "en-US")
//
//nolint:unparam // check is required by interface
func (h *CallbackHandler) handleGetCurrentUICulture(_ *RemoteHostCall) (interface{}, error) {
	if h.host == nil {
		return DefaultCulture, nil
	}
	return h.host.GetCurrentUICulture(), nil
}

// handleSetShouldExit processes SetShouldExit method calls.
// Parameters: [0] int32 (exit code)
// Returns: none
func (h *CallbackHandler) handleSetShouldExit(_ *RemoteHostCall) error {
	// This is a signal to the host that PowerShell wants to exit
	// We don't implement this in the basic host, but advanced hosts could handle it
	return nil
}

// handleEnterNestedPrompt processes EnterNestedPrompt method calls.
// Parameters: none
// Returns: none
func (h *CallbackHandler) handleEnterNestedPrompt(_ *RemoteHostCall) error {
	// Nested prompts are for interactive debugging scenarios
	// Not implemented in basic host
	return fmt.Errorf("EnterNestedPrompt not supported")
}

// handleExitNestedPrompt processes ExitNestedPrompt method calls.
// Parameters: none
// Returns: none
func (h *CallbackHandler) handleExitNestedPrompt(_ *RemoteHostCall) error {
	// Nested prompts are for interactive debugging scenarios
	// Not implemented in basic host
	return fmt.Errorf("ExitNestedPrompt not supported")
}

// ========== Application Notification Handlers (9-10) ==========

// handleNotifyBeginApplication processes NotifyBeginApplication method calls.
// Parameters: none
// Returns: none
func (h *CallbackHandler) handleNotifyBeginApplication(_ *RemoteHostCall) error {
	// Notification that an external application is starting
	// Most hosts can ignore this
	return nil
}

// handleNotifyEndApplication processes NotifyEndApplication method calls.
// Parameters: none
// Returns: none
func (h *CallbackHandler) handleNotifyEndApplication(_ *RemoteHostCall) error {
	// Notification that an external application has ended
	// Most hosts can ignore this
	return nil
}

// ========== PSHostUserInterface Method Handlers (11-26) ==========

// handleReadLine processes ReadLine method calls.
// Parameters: none
// Returns: string
func (h *CallbackHandler) handleReadLine(_ *RemoteHostCall) (interface{}, error) {
	if h.host == nil || h.host.UI() == nil {
		return "", nil
	}
	return h.host.UI().ReadLine()
}

// handleReadLineAsSecureString processes ReadLineAsSecureString method calls.
// Parameters: none
// Returns: *objects.SecureString
func (h *CallbackHandler) handleReadLineAsSecureString(_ *RemoteHostCall) (interface{}, error) {
	if h.host == nil || h.host.UI() == nil {
		return objects.NewSecureString("")
	}
	return h.host.UI().ReadLineAsSecureString()
}

// handleWrite1 processes Write1 method calls.
// Parameters: [0] string (value)
// Returns: none
func (h *CallbackHandler) handleWrite1(call *RemoteHostCall) error {
	if len(call.MethodParameters) < 1 {
		return fmt.Errorf("Write1 requires 1 parameter, got %d", len(call.MethodParameters))
	}
	text, ok := call.MethodParameters[0].(string)
	if !ok {
		return fmt.Errorf("Write1 parameter must be string, got %T", call.MethodParameters[0])
	}
	if h.host != nil && h.host.UI() != nil {
		h.host.UI().Write(text)
	}
	return nil
}

// handleWrite2 processes Write2 method calls.
// Parameters: [0] ConsoleColor (foreground), [1] ConsoleColor (background), [2] string (value)
// Returns: none
func (h *CallbackHandler) handleWrite2(call *RemoteHostCall) error {
	if len(call.MethodParameters) < 3 {
		return fmt.Errorf("Write2 requires 3 parameters, got %d", len(call.MethodParameters))
	}
	// For now, we ignore the color parameters and just write the text
	// A more advanced implementation could handle console colors
	text, ok := call.MethodParameters[2].(string)
	if !ok {
		return fmt.Errorf("Write2 text parameter must be string, got %T", call.MethodParameters[2])
	}
	if h.host != nil && h.host.UI() != nil {
		h.host.UI().Write(text)
	}
	return nil
}

// handleWriteLine1 processes WriteLine1 method calls.
// Parameters: none
// Returns: none
//
//nolint:unparam // strict signature required
func (h *CallbackHandler) handleWriteLine1(_ *RemoteHostCall) error {
	if h.host != nil && h.host.UI() != nil {
		h.host.UI().WriteLine("")
	}
	return nil
}

// handleWriteLine2 processes WriteLine2 method calls.
// Parameters: [0] string (value)
// Returns: none
func (h *CallbackHandler) handleWriteLine2(call *RemoteHostCall) error {
	if len(call.MethodParameters) < 1 {
		return fmt.Errorf("WriteLine2 requires 1 parameter, got %d", len(call.MethodParameters))
	}
	text, ok := call.MethodParameters[0].(string)
	if !ok {
		return fmt.Errorf("WriteLine2 parameter must be string, got %T", call.MethodParameters[0])
	}
	if h.host != nil && h.host.UI() != nil {
		h.host.UI().WriteLine(text)
	}
	return nil
}

// handleWriteLine3 processes WriteLine3 method calls.
// Parameters: [0] ConsoleColor (foreground), [1] ConsoleColor (background), [2] string (value)
// Returns: none
func (h *CallbackHandler) handleWriteLine3(call *RemoteHostCall) error {
	if len(call.MethodParameters) < 3 {
		return fmt.Errorf("WriteLine3 requires 3 parameters, got %d", len(call.MethodParameters))
	}
	// For now, we ignore the color parameters and just write the text
	text, ok := call.MethodParameters[2].(string)
	if !ok {
		return fmt.Errorf("WriteLine3 text parameter must be string, got %T", call.MethodParameters[2])
	}
	if h.host != nil && h.host.UI() != nil {
		h.host.UI().WriteLine(text)
	}
	return nil
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

// handleWriteProgress processes WriteProgress method calls.
// Parameters: [0] int64 (sourceID), [1] ProgressRecord
// Returns: none
func (h *CallbackHandler) handleWriteProgress(call *RemoteHostCall) error {
	if len(call.MethodParameters) < 2 {
		return fmt.Errorf("WriteProgress requires 2 parameters, got %d", len(call.MethodParameters))
	}

	var sourceID int64
	switch v := call.MethodParameters[0].(type) {
	case int:
		sourceID = int64(v)
	case int32:
		sourceID = int64(v)
	case int64:
		sourceID = v
	default:
		return fmt.Errorf("WriteProgress sourceID must be int, got %T", call.MethodParameters[0])
	}

	// Convert PSObject parameter to ProgressRecord
	record, err := convertToProgressRecord(call.MethodParameters[1])
	if err != nil {
		// Progress is informational - don't fail the callback on conversion errors.
		// Pass nil to maintain backwards compatibility with hosts that may not handle it.
		if h.host != nil && h.host.UI() != nil {
			h.host.UI().WriteProgress(sourceID, nil)
		}
		return nil
	}

	if h.host != nil && h.host.UI() != nil {
		h.host.UI().WriteProgress(sourceID, record)
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
	descriptions, err := convertToFieldDescriptions(call.MethodParameters[2])
	if err != nil {
		return nil, fmt.Errorf("failed to convert field descriptions: %w", err)
	}

	if h.host == nil || h.host.UI() == nil {
		return make(map[string]interface{}), nil
	}
	return h.host.UI().Prompt(caption, message, descriptions)
}

// handlePromptForCredential1 processes PromptForCredential1 method calls.
// Parameters: [0] string (caption), [1] string (message), [2] string (userName), [3] string (targetName)
// Returns: *objects.PSCredential
func (h *CallbackHandler) handlePromptForCredential1(call *RemoteHostCall) (interface{}, error) {
	if len(call.MethodParameters) < 4 {
		return nil, fmt.Errorf("PromptForCredential1 requires 4 parameters, got %d", len(call.MethodParameters))
	}

	caption, ok := call.MethodParameters[0].(string)
	if !ok {
		return nil, fmt.Errorf("PromptForCredential1 caption must be string, got %T", call.MethodParameters[0])
	}

	message, ok := call.MethodParameters[1].(string)
	if !ok {
		return nil, fmt.Errorf("PromptForCredential1 message must be string, got %T", call.MethodParameters[1])
	}

	userName, ok := call.MethodParameters[2].(string)
	if !ok {
		return nil, fmt.Errorf("PromptForCredential1 userName must be string, got %T", call.MethodParameters[2])
	}

	targetName, ok := call.MethodParameters[3].(string)
	if !ok {
		return nil, fmt.Errorf("PromptForCredential1 targetName must be string, got %T", call.MethodParameters[3])
	}

	if h.host == nil || h.host.UI() == nil {
		return nil, nil
	}
	return h.host.UI().PromptForCredential(
		caption, message, userName, targetName, CredentialTypeDefault, CredentialUIOptionNone)
}

// handlePromptForCredential2 processes PromptForCredential2 method calls.
// Parameters: [0] string (caption), [1] string (message), [2] string (userName), [3] string (targetName),
//
//	[4] CredentialTypes (allowedCredentialTypes), [5] CredentialUIOptions (options)
//
// Returns: *objects.PSCredential
func (h *CallbackHandler) handlePromptForCredential2(call *RemoteHostCall) (interface{}, error) {
	if len(call.MethodParameters) < 6 {
		return nil, fmt.Errorf("PromptForCredential2 requires 6 parameters, got %d", len(call.MethodParameters))
	}

	caption, ok := call.MethodParameters[0].(string)
	if !ok {
		return nil, fmt.Errorf("PromptForCredential2 caption must be string, got %T", call.MethodParameters[0])
	}

	message, ok := call.MethodParameters[1].(string)
	if !ok {
		return nil, fmt.Errorf("PromptForCredential2 message must be string, got %T", call.MethodParameters[1])
	}

	userName, ok := call.MethodParameters[2].(string)
	if !ok {
		return nil, fmt.Errorf("PromptForCredential2 userName must be string, got %T", call.MethodParameters[2])
	}

	targetName, ok := call.MethodParameters[3].(string)
	if !ok {
		return nil, fmt.Errorf("PromptForCredential2 targetName must be string, got %T", call.MethodParameters[3])
	}

	var allowedCredentialTypes CredentialTypes
	switch v := call.MethodParameters[4].(type) {
	case int:
		allowedCredentialTypes = CredentialTypes(v)
	case int32:
		allowedCredentialTypes = CredentialTypes(v)
	case int64:
		allowedCredentialTypes = CredentialTypes(v)
	default:
		allowedCredentialTypes = CredentialTypeDefault
	}

	var options CredentialUIOptions
	switch v := call.MethodParameters[5].(type) {
	case int:
		options = CredentialUIOptions(v)
	case int32:
		options = CredentialUIOptions(v)
	case int64:
		options = CredentialUIOptions(v)
	default:
		options = CredentialUIOptionNone
	}

	if h.host == nil || h.host.UI() == nil {
		return nil, nil
	}
	return h.host.UI().PromptForCredential(caption, message, userName, targetName, allowedCredentialTypes, options)
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
	choices, err := convertToChoiceDescriptions(call.MethodParameters[2])
	if err != nil {
		return nil, fmt.Errorf("failed to convert choice descriptions: %w", err)
	}

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

// handlePromptForChoiceMultipleSelection processes PromptForChoiceMultipleSelection method calls.
// Parameters: [0] string (caption), [1] string (message), [2] []ChoiceDescription, [3] []int (defaultChoices)
// Returns: []int (selected choice indices)
func (h *CallbackHandler) handlePromptForChoiceMultipleSelection(call *RemoteHostCall) (interface{}, error) {
	if len(call.MethodParameters) < 4 {
		return nil, fmt.Errorf("PromptForChoiceMultipleSelection requires 4 parameters, got %d", len(call.MethodParameters))
	}

	_, ok := call.MethodParameters[0].(string)
	if !ok {
		return nil, fmt.Errorf("PromptForChoiceMultipleSelection caption must be string, got %T", call.MethodParameters[0])
	}

	_, ok = call.MethodParameters[1].(string)
	if !ok {
		return nil, fmt.Errorf("PromptForChoiceMultipleSelection message must be string, got %T", call.MethodParameters[1])
	}

	// Convert parameter 2 to []ChoiceDescription
	_, err := convertToChoiceDescriptions(call.MethodParameters[2])
	if err != nil {
		return nil, fmt.Errorf("failed to convert choice descriptions: %w", err)
	}

	// For now, return empty array - implementing multi-select is complex
	// and not commonly used in practice. Parameters are validated but not used.
	if h.host == nil || h.host.UI() == nil {
		return []int{}, nil
	}

	// Default behavior: return first choice from default choices if available
	// A more complete implementation would support multi-select
	if defaultChoices, ok := call.MethodParameters[3].([]interface{}); ok && len(defaultChoices) > 0 {
		if firstChoice, ok := defaultChoices[0].(int); ok {
			return []int{firstChoice}, nil
		}
	}

	return []int{}, nil
}
