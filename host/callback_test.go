package host

import (
	"testing"

	"github.com/smnsjas/go-psrpcore/objects"
)

// mockHost implements Host for testing
type mockHost struct {
	ui *mockHostUI
}

func newMockHost() *mockHost {
	return &mockHost{
		ui: &mockHostUI{
			readLineResult: "test input",
			promptResult:   make(map[string]interface{}),
			choiceResult:   0,
		},
	}
}

func (h *mockHost) GetName() string                  { return "MockHost" }
func (h *mockHost) GetVersion() Version              { return Version{Major: 1, Minor: 0} }
func (h *mockHost) GetInstanceId() string            { return "mock-id" }
func (h *mockHost) GetCurrentCulture() string        { return "en-US" }
func (h *mockHost) GetCurrentUICulture() string      { return "en-US" }
func (h *mockHost) UI() HostUI                       { return h.ui }

// mockHostUI implements HostUI for testing
type mockHostUI struct {
	readLineResult      string
	readLineError       error
	writeCallCount      int
	writeErrorCallCount int
	lastWritten         string
	lastError           string
	promptResult        map[string]interface{}
	promptError         error
	credentialResult    *objects.PSCredential
	credentialError     error
	choiceResult        int
	choiceError         error
}

func (ui *mockHostUI) ReadLine() (string, error) {
	return ui.readLineResult, ui.readLineError
}

func (ui *mockHostUI) ReadLineAsSecureString() (*objects.SecureString, error) {
	return objects.NewSecureString(ui.readLineResult)
}

func (ui *mockHostUI) Write(text string) {
	ui.writeCallCount++
	ui.lastWritten = text
}

func (ui *mockHostUI) WriteLine(text string) {
	ui.writeCallCount++
	ui.lastWritten = text
}

func (ui *mockHostUI) WriteErrorLine(text string) {
	ui.writeErrorCallCount++
	ui.lastError = text
}

func (ui *mockHostUI) WriteDebugLine(text string) {
	ui.writeCallCount++
	ui.lastWritten = text
}

func (ui *mockHostUI) WriteVerboseLine(text string) {
	ui.writeCallCount++
	ui.lastWritten = text
}

func (ui *mockHostUI) WriteWarningLine(text string) {
	ui.writeCallCount++
	ui.lastWritten = text
}

func (ui *mockHostUI) WriteProgress(sourceId int64, record *objects.ProgressRecord) {}

func (ui *mockHostUI) Prompt(caption, message string, descriptions []FieldDescription) (map[string]interface{}, error) {
	return ui.promptResult, ui.promptError
}

func (ui *mockHostUI) PromptForCredential(caption, message, userName, targetName string, allowedCredentialTypes CredentialTypes, options CredentialUIOptions) (*objects.PSCredential, error) {
	return ui.credentialResult, ui.credentialError
}

func (ui *mockHostUI) PromptForChoice(caption, message string, choices []ChoiceDescription, defaultChoice int) (int, error) {
	return ui.choiceResult, ui.choiceError
}

func TestMethodIDString(t *testing.T) {
	tests := []struct {
		id       MethodID
		expected string
	}{
		// PSHost methods (1-8)
		{MethodIDGetName, "GetName"},
		{MethodIDGetVersion, "GetVersion"},
		{MethodIDGetInstanceId, "GetInstanceId"},
		{MethodIDGetCurrentCulture, "GetCurrentCulture"},
		{MethodIDGetCurrentUICulture, "GetCurrentUICulture"},
		{MethodIDSetShouldExit, "SetShouldExit"},
		{MethodIDEnterNestedPrompt, "EnterNestedPrompt"},
		{MethodIDExitNestedPrompt, "ExitNestedPrompt"},

		// Application notification (9-10)
		{MethodIDNotifyBeginApplication, "NotifyBeginApplication"},
		{MethodIDNotifyEndApplication, "NotifyEndApplication"},

		// PSHostUserInterface methods (11-26)
		{MethodIDReadLine, "ReadLine"},
		{MethodIDReadLineAsSecureString, "ReadLineAsSecureString"},
		{MethodIDWrite1, "Write1"},
		{MethodIDWrite2, "Write2"},
		{MethodIDWriteLine1, "WriteLine1"},
		{MethodIDWriteLine2, "WriteLine2"},
		{MethodIDWriteLine3, "WriteLine3"},
		{MethodIDWriteErrorLine, "WriteErrorLine"},
		{MethodIDWriteDebugLine, "WriteDebugLine"},
		{MethodIDWriteProgress, "WriteProgress"},
		{MethodIDWriteVerboseLine, "WriteVerboseLine"},
		{MethodIDWriteWarningLine, "WriteWarningLine"},
		{MethodIDPrompt, "Prompt"},
		{MethodIDPromptForCredential1, "PromptForCredential1"},
		{MethodIDPromptForCredential2, "PromptForCredential2"},
		{MethodIDPromptForChoice, "PromptForChoice"},

		// RawUI methods (27-51)
		{MethodIDGetForegroundColor, "GetForegroundColor"},
		{MethodIDSetWindowTitle, "SetWindowTitle"},

		// Runspace methods (52-55)
		{MethodIDPushRunspace, "PushRunspace"},
		{MethodIDGetRunspace, "GetRunspace"},

		// Additional methods (56)
		{MethodIDPromptForChoiceMultipleSelection, "PromptForChoiceMultipleSelection"},

		// Unknown
		{MethodID(999), "Unknown(999)"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if got := tt.id.String(); got != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, got)
			}
		})
	}
}

func TestCallbackHandler_HandleReadLine(t *testing.T) {
	host := newMockHost()
	handler := NewCallbackHandler(host)

	call := &RemoteHostCall{
		CallID:           1,
		MethodID:         MethodIDReadLine,
		MethodParameters: []interface{}{},
	}

	response := handler.HandleCall(call)

	if response.CallID != 1 {
		t.Errorf("expected CallID 1, got %d", response.CallID)
	}
	if response.ExceptionRaised {
		t.Errorf("expected no exception, got: %v", response.ReturnValue)
	}
	if response.ReturnValue != "test input" {
		t.Errorf("expected 'test input', got %v", response.ReturnValue)
	}
}

func TestCallbackHandler_HandleWriteErrorLine(t *testing.T) {
	host := newMockHost()
	handler := NewCallbackHandler(host)

	call := &RemoteHostCall{
		CallID:           2,
		MethodID:         MethodIDWriteErrorLine,
		MethodParameters: []interface{}{"error message"},
	}

	response := handler.HandleCall(call)

	if response.CallID != 2 {
		t.Errorf("expected CallID 2, got %d", response.CallID)
	}
	if response.ExceptionRaised {
		t.Errorf("expected no exception, got: %v", response.ReturnValue)
	}
	if host.ui.writeErrorCallCount != 1 {
		t.Errorf("expected 1 WriteErrorLine call, got %d", host.ui.writeErrorCallCount)
	}
	if host.ui.lastError != "error message" {
		t.Errorf("expected 'error message', got %q", host.ui.lastError)
	}
}

func TestCallbackHandler_HandleWrite(t *testing.T) {
	host := newMockHost()
	handler := NewCallbackHandler(host)

	call := &RemoteHostCall{
		CallID:           3,
		MethodID:         MethodIDWrite1,
		MethodParameters: []interface{}{"output text"},
	}

	response := handler.HandleCall(call)

	if response.ExceptionRaised {
		t.Errorf("expected no exception, got: %v", response.ReturnValue)
	}
	if host.ui.writeCallCount != 1 {
		t.Errorf("expected 1 Write call, got %d", host.ui.writeCallCount)
	}
	if host.ui.lastWritten != "output text" {
		t.Errorf("expected 'output text', got %q", host.ui.lastWritten)
	}
}

func TestCallbackHandler_HandlePromptForChoice(t *testing.T) {
	host := newMockHost()
	host.ui.choiceResult = 1
	handler := NewCallbackHandler(host)

	call := &RemoteHostCall{
		CallID:   4,
		MethodID: MethodIDPromptForChoice,
		MethodParameters: []interface{}{
			"caption",
			"message",
			[]ChoiceDescription{},
			int32(0), // default choice
		},
	}

	response := handler.HandleCall(call)

	if response.ExceptionRaised {
		t.Errorf("expected no exception, got: %v", response.ReturnValue)
	}
	if response.ReturnValue != 1 {
		t.Errorf("expected choice 1, got %v", response.ReturnValue)
	}
}

func TestCallbackHandler_MissingParameters(t *testing.T) {
	host := newMockHost()
	handler := NewCallbackHandler(host)

	call := &RemoteHostCall{
		CallID:           5,
		MethodID:         MethodIDWriteErrorLine,
		MethodParameters: []interface{}{}, // Missing required parameter
	}

	response := handler.HandleCall(call)

	if !response.ExceptionRaised {
		t.Error("expected exception for missing parameters")
	}
}

func TestCallbackHandler_InvalidParameterType(t *testing.T) {
	host := newMockHost()
	handler := NewCallbackHandler(host)

	call := &RemoteHostCall{
		CallID:           6,
		MethodID:         MethodIDWriteErrorLine,
		MethodParameters: []interface{}{123}, // Wrong type (int instead of string)
	}

	response := handler.HandleCall(call)

	if !response.ExceptionRaised {
		t.Error("expected exception for invalid parameter type")
	}
}

func TestCallbackHandler_UnsupportedMethod(t *testing.T) {
	host := newMockHost()
	handler := NewCallbackHandler(host)

	call := &RemoteHostCall{
		CallID:           7,
		MethodID:         MethodID(999),
		MethodParameters: []interface{}{},
	}

	response := handler.HandleCall(call)

	if !response.ExceptionRaised {
		t.Error("expected exception for unsupported method")
	}
}

func TestCallbackHandler_WithNullHost(t *testing.T) {
	handler := NewCallbackHandler(NewNullHost())

	call := &RemoteHostCall{
		CallID:           8,
		MethodID:         MethodIDReadLine,
		MethodParameters: []interface{}{},
	}

	response := handler.HandleCall(call)

	// NullHost should not raise exceptions
	if response.ExceptionRaised {
		t.Errorf("expected no exception from NullHost, got: %v", response.ReturnValue)
	}
}

// Test PSHost methods (1-8)
func TestCallbackHandler_HandleGetName(t *testing.T) {
	host := newMockHost()
	handler := NewCallbackHandler(host)

	call := &RemoteHostCall{
		CallID:           10,
		MethodID:         MethodIDGetName,
		MethodParameters: []interface{}{},
	}

	response := handler.HandleCall(call)

	if response.ExceptionRaised {
		t.Errorf("expected no exception, got: %v", response.ReturnValue)
	}
	if response.ReturnValue != "MockHost" {
		t.Errorf("expected 'MockHost', got %v", response.ReturnValue)
	}
}

func TestCallbackHandler_HandleGetVersion(t *testing.T) {
	host := newMockHost()
	handler := NewCallbackHandler(host)

	call := &RemoteHostCall{
		CallID:           11,
		MethodID:         MethodIDGetVersion,
		MethodParameters: []interface{}{},
	}

	response := handler.HandleCall(call)

	if response.ExceptionRaised {
		t.Errorf("expected no exception, got: %v", response.ReturnValue)
	}
	version, ok := response.ReturnValue.(Version)
	if !ok {
		t.Errorf("expected Version type, got %T", response.ReturnValue)
	}
	if version.Major != 1 || version.Minor != 0 {
		t.Errorf("expected version 1.0, got %d.%d", version.Major, version.Minor)
	}
}

func TestCallbackHandler_HandleReadLineAsSecureString(t *testing.T) {
	host := newMockHost()
	handler := NewCallbackHandler(host)

	call := &RemoteHostCall{
		CallID:           12,
		MethodID:         MethodIDReadLineAsSecureString,
		MethodParameters: []interface{}{},
	}

	response := handler.HandleCall(call)

	if response.ExceptionRaised {
		t.Errorf("expected no exception, got: %v", response.ReturnValue)
	}
	if response.ReturnValue == nil {
		t.Error("expected SecureString, got nil")
	}
}

func TestCallbackHandler_HandleWrite2(t *testing.T) {
	host := newMockHost()
	handler := NewCallbackHandler(host)

	call := &RemoteHostCall{
		CallID:           13,
		MethodID:         MethodIDWrite2,
		MethodParameters: []interface{}{int32(15), int32(0), "colored text"},
	}

	response := handler.HandleCall(call)

	if response.ExceptionRaised {
		t.Errorf("expected no exception, got: %v", response.ReturnValue)
	}
	if host.ui.writeCallCount != 1 {
		t.Errorf("expected 1 Write call, got %d", host.ui.writeCallCount)
	}
	if host.ui.lastWritten != "colored text" {
		t.Errorf("expected 'colored text', got %q", host.ui.lastWritten)
	}
}

func TestCallbackHandler_HandleWriteLine2(t *testing.T) {
	host := newMockHost()
	handler := NewCallbackHandler(host)

	call := &RemoteHostCall{
		CallID:           14,
		MethodID:         MethodIDWriteLine2,
		MethodParameters: []interface{}{"line text"},
	}

	response := handler.HandleCall(call)

	if response.ExceptionRaised {
		t.Errorf("expected no exception, got: %v", response.ReturnValue)
	}
	if host.ui.writeCallCount != 1 {
		t.Errorf("expected 1 WriteLine call, got %d", host.ui.writeCallCount)
	}
	if host.ui.lastWritten != "line text" {
		t.Errorf("expected 'line text', got %q", host.ui.lastWritten)
	}
}

func TestCallbackHandler_HandleWriteProgress(t *testing.T) {
	host := newMockHost()
	handler := NewCallbackHandler(host)

	call := &RemoteHostCall{
		CallID:           15,
		MethodID:         MethodIDWriteProgress,
		MethodParameters: []interface{}{int64(123), nil},
	}

	response := handler.HandleCall(call)

	if response.ExceptionRaised {
		t.Errorf("expected no exception, got: %v", response.ReturnValue)
	}
}

func TestCallbackHandler_HandlePromptForCredential2(t *testing.T) {
	host := newMockHost()
	handler := NewCallbackHandler(host)

	call := &RemoteHostCall{
		CallID:   16,
		MethodID: MethodIDPromptForCredential2,
		MethodParameters: []interface{}{
			"caption",
			"message",
			"username",
			"target",
			int32(3), // CredentialTypeDefault
			int32(0), // CredentialUIOptionNone
		},
	}

	response := handler.HandleCall(call)

	// NullHost returns nil for credentials, which is acceptable
	if response.ExceptionRaised {
		t.Errorf("expected no exception, got: %v", response.ReturnValue)
	}
}

func TestCallbackHandler_RawUIMethodsNotImplemented(t *testing.T) {
	host := newMockHost()
	handler := NewCallbackHandler(host)

	call := &RemoteHostCall{
		CallID:           17,
		MethodID:         MethodIDGetForegroundColor,
		MethodParameters: []interface{}{},
	}

	response := handler.HandleCall(call)

	if !response.ExceptionRaised {
		t.Error("expected exception for RawUI method")
	}
}

func TestCallbackHandler_RunspaceMethodsNotImplemented(t *testing.T) {
	host := newMockHost()
	handler := NewCallbackHandler(host)

	call := &RemoteHostCall{
		CallID:           18,
		MethodID:         MethodIDPushRunspace,
		MethodParameters: []interface{}{},
	}

	response := handler.HandleCall(call)

	if !response.ExceptionRaised {
		t.Error("expected exception for Runspace method")
	}
}

// Test that method ID values are correct per MS-PSRP spec
func TestMethodIDValues(t *testing.T) {
	tests := []struct {
		id       MethodID
		expected int32
	}{
		{MethodIDGetName, 1},
		{MethodIDGetVersion, 2},
		{MethodIDGetInstanceId, 3},
		{MethodIDGetCurrentCulture, 4},
		{MethodIDGetCurrentUICulture, 5},
		{MethodIDReadLine, 11},
		{MethodIDReadLineAsSecureString, 12},
		{MethodIDWrite1, 13},
		{MethodIDWriteErrorLine, 18},
		{MethodIDWriteDebugLine, 19},
		{MethodIDWriteProgress, 20},
		{MethodIDWriteVerboseLine, 21},
		{MethodIDWriteWarningLine, 22},
		{MethodIDPrompt, 23},
		{MethodIDPromptForCredential1, 24},
		{MethodIDPromptForCredential2, 25},
		{MethodIDPromptForChoice, 26},
		{MethodIDGetForegroundColor, 27},
		{MethodIDGetWindowTitle, 41},
		{MethodIDSetWindowTitle, 42},
		{MethodIDGetMaxWindowSize, 43},
		{MethodIDGetMaxPhysicalWindowSize, 44},
		{MethodIDGetKeyAvailable, 45},
		{MethodIDReadKey, 46},
		{MethodIDFlushInputBuffer, 47},
		{MethodIDSetBufferContents1, 48},
		{MethodIDSetBufferContents2, 49},
		{MethodIDGetBufferContents, 50},
		{MethodIDScrollBufferContents, 51},
		{MethodIDPushRunspace, 52},
		{MethodIDPopRunspace, 53},
		{MethodIDGetIsRunspacePushed, 54},
		{MethodIDGetRunspace, 55},
		{MethodIDPromptForChoiceMultipleSelection, 56},
	}

	for _, tt := range tests {
		t.Run(tt.id.String(), func(t *testing.T) {
			if int32(tt.id) != tt.expected {
				t.Errorf("expected method ID %d to have value %d, got %d", tt.id, tt.expected, int32(tt.id))
			}
		})
	}
}
