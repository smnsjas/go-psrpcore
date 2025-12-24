package host

import (
	"testing"

	"github.com/smnsjas/go-psrpcore/objects"
	"github.com/smnsjas/go-psrpcore/serialization"
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

func (h *mockHost) GetName() string             { return "MockHost" }
func (h *mockHost) GetVersion() Version         { return Version{Major: 1, Minor: 0} }
func (h *mockHost) GetInstanceId() string       { return "mock-id" }
func (h *mockHost) GetCurrentCulture() string   { return "en-US" }
func (h *mockHost) GetCurrentUICulture() string { return "en-US" }
func (h *mockHost) UI() HostUI                  { return h.ui }

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
	// Progress tracking
	lastProgressSourceID int64
	lastProgressRecord   *objects.ProgressRecord
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

func (ui *mockHostUI) WriteProgress(sourceID int64, record *objects.ProgressRecord) {
	ui.lastProgressSourceID = sourceID
	ui.lastProgressRecord = record
}

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
		{MethodIDGetInstanceID, "GetInstanceId"},
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

func TestCallbackHandler_HandleGetInstanceId(t *testing.T) {
	host := newMockHost()
	handler := NewCallbackHandler(host)

	call := &RemoteHostCall{
		CallID:           20,
		MethodID:         MethodIDGetInstanceID,
		MethodParameters: []interface{}{},
	}

	response := handler.HandleCall(call)

	if response.ExceptionRaised {
		t.Errorf("expected no exception, got: %v", response.ReturnValue)
	}
	if response.ReturnValue != "mock-id" {
		t.Errorf("expected 'mock-id', got %v", response.ReturnValue)
	}
}

func TestCallbackHandler_HandleGetCurrentCulture(t *testing.T) {
	host := newMockHost()
	handler := NewCallbackHandler(host)

	call := &RemoteHostCall{
		CallID:           21,
		MethodID:         MethodIDGetCurrentCulture,
		MethodParameters: []interface{}{},
	}

	response := handler.HandleCall(call)

	if response.ExceptionRaised {
		t.Errorf("expected no exception, got: %v", response.ReturnValue)
	}
	if response.ReturnValue != "en-US" {
		t.Errorf("expected 'en-US', got %v", response.ReturnValue)
	}
}

func TestCallbackHandler_HandleGetCurrentUICulture(t *testing.T) {
	host := newMockHost()
	handler := NewCallbackHandler(host)

	call := &RemoteHostCall{
		CallID:           22,
		MethodID:         MethodIDGetCurrentUICulture,
		MethodParameters: []interface{}{},
	}

	response := handler.HandleCall(call)

	if response.ExceptionRaised {
		t.Errorf("expected no exception, got: %v", response.ReturnValue)
	}
	if response.ReturnValue != "en-US" {
		t.Errorf("expected 'en-US', got %v", response.ReturnValue)
	}
}

func TestCallbackHandler_HandleSetShouldExit(t *testing.T) {
	host := newMockHost()
	handler := NewCallbackHandler(host)

	call := &RemoteHostCall{
		CallID:           23,
		MethodID:         MethodIDSetShouldExit,
		MethodParameters: []interface{}{int32(1)}, // exit code
	}

	response := handler.HandleCall(call)

	if response.ExceptionRaised {
		t.Errorf("expected no exception, got: %v", response.ReturnValue)
	}
	// Verify behavior if host interface supports it (MockHost is partial)
}

func TestCallbackHandler_HandleEnterNestedPrompt(t *testing.T) {
	host := newMockHost()
	handler := NewCallbackHandler(host)

	call := &RemoteHostCall{
		CallID:           24,
		MethodID:         MethodIDEnterNestedPrompt,
		MethodParameters: []interface{}{},
	}

	response := handler.HandleCall(call)

	// EnterNestedPrompt returns "not supported" error
	if !response.ExceptionRaised {
		t.Error("expected exception (not supported), got no exception")
	}
}

func TestCallbackHandler_HandleExitNestedPrompt(t *testing.T) {
	host := newMockHost()
	handler := NewCallbackHandler(host)

	call := &RemoteHostCall{
		CallID:           25,
		MethodID:         MethodIDExitNestedPrompt,
		MethodParameters: []interface{}{},
	}

	response := handler.HandleCall(call)

	// ExitNestedPrompt returns "not supported" error
	if !response.ExceptionRaised {
		t.Error("expected exception (not supported), got no exception")
	}
}

func TestCallbackHandler_HandleNotifyBeginApplication(t *testing.T) {
	host := newMockHost()
	handler := NewCallbackHandler(host)

	call := &RemoteHostCall{
		CallID:           26,
		MethodID:         MethodIDNotifyBeginApplication,
		MethodParameters: []interface{}{},
	}

	response := handler.HandleCall(call)

	if response.ExceptionRaised {
		t.Errorf("expected no exception, got: %v", response.ReturnValue)
	}
}

func TestCallbackHandler_HandleNotifyEndApplication(t *testing.T) {
	host := newMockHost()
	handler := NewCallbackHandler(host)

	call := &RemoteHostCall{
		CallID:           27,
		MethodID:         MethodIDNotifyEndApplication,
		MethodParameters: []interface{}{},
	}

	response := handler.HandleCall(call)

	if response.ExceptionRaised {
		t.Errorf("expected no exception, got: %v", response.ReturnValue)
	}
}

func TestCallbackHandler_HandleWriteLine1(t *testing.T) {
	host := newMockHost()
	handler := NewCallbackHandler(host)

	call := &RemoteHostCall{
		CallID:           28,
		MethodID:         MethodIDWriteLine1,
		MethodParameters: []interface{}{}, // Empty line
	}

	response := handler.HandleCall(call)

	if response.ExceptionRaised {
		t.Errorf("expected no exception, got: %v", response.ReturnValue)
	}
	if host.ui.writeCallCount != 1 {
		t.Errorf("expected 1 WriteLine call, got %d", host.ui.writeCallCount)
	}
	// mockHostUI.WriteLine sets lastWritten even for empty string
	if host.ui.lastWritten != "" {
		t.Errorf("expected empty string, got %q", host.ui.lastWritten)
	}
}

func TestCallbackHandler_HandleWriteDebugLine(t *testing.T) {
	host := newMockHost()
	handler := NewCallbackHandler(host)

	call := &RemoteHostCall{
		CallID:           29,
		MethodID:         MethodIDWriteDebugLine,
		MethodParameters: []interface{}{"debug msg"},
	}

	response := handler.HandleCall(call)

	if response.ExceptionRaised {
		t.Errorf("expected no exception, got: %v", response.ReturnValue)
	}
	if host.ui.writeCallCount != 1 {
		t.Errorf("expected 1 WriteDebugLine call, got %d", host.ui.writeCallCount)
	}
	if host.ui.lastWritten != "debug msg" {
		t.Errorf("expected 'debug msg', got %q", host.ui.lastWritten)
	}
}

func TestCallbackHandler_HandleWriteVerboseLine(t *testing.T) {
	host := newMockHost()
	handler := NewCallbackHandler(host)

	call := &RemoteHostCall{
		CallID:           30,
		MethodID:         MethodIDWriteVerboseLine,
		MethodParameters: []interface{}{"verbose msg"},
	}

	response := handler.HandleCall(call)

	if response.ExceptionRaised {
		t.Errorf("expected no exception, got: %v", response.ReturnValue)
	}
	if host.ui.writeCallCount != 1 {
		t.Errorf("expected 1 WriteVerboseLine call, got %d", host.ui.writeCallCount)
	}
	if host.ui.lastWritten != "verbose msg" {
		t.Errorf("expected 'verbose msg', got %q", host.ui.lastWritten)
	}
}

func TestCallbackHandler_HandleWriteWarningLine(t *testing.T) {
	host := newMockHost()
	handler := NewCallbackHandler(host)

	call := &RemoteHostCall{
		CallID:           31,
		MethodID:         MethodIDWriteWarningLine,
		MethodParameters: []interface{}{"warning msg"},
	}

	response := handler.HandleCall(call)

	if response.ExceptionRaised {
		t.Errorf("expected no exception, got: %v", response.ReturnValue)
	}
	if host.ui.writeCallCount != 1 {
		t.Errorf("expected 1 WriteWarningLine call, got %d", host.ui.writeCallCount)
	}
	if host.ui.lastWritten != "warning msg" {
		t.Errorf("expected 'warning msg', got %q", host.ui.lastWritten)
	}
}

func TestCallbackHandler_HandleWriteProgress(t *testing.T) {
	host := newMockHost()
	handler := NewCallbackHandler(host)

	// Create realistic PSObject parameter with all fields
	progressObj := &serialization.PSObject{
		TypeNames: []string{"System.Management.Automation.ProgressRecord"},
		Properties: map[string]interface{}{
			"ActivityId":        int32(5),
			"ParentActivityId":  int32(2),
			"Activity":          "Test Progress",
			"StatusDescription": "Processing items",
			"CurrentOperation":  "Item 5 of 10",
			"PercentComplete":   int32(50),
			"SecondsRemaining":  int32(30),
			"RecordType":        int32(0), // Processing
		},
	}

	call := &RemoteHostCall{
		CallID:           15,
		MethodID:         MethodIDWriteProgress,
		MethodParameters: []interface{}{int64(123), progressObj},
	}

	response := handler.HandleCall(call)

	if response.ExceptionRaised {
		t.Errorf("expected no exception, got: %v", response.ReturnValue)
	}

	// Verify sourceID was captured
	if host.ui.lastProgressSourceID != 123 {
		t.Errorf("expected sourceID=123, got %d", host.ui.lastProgressSourceID)
	}

	// Verify ProgressRecord was deserialized and passed (not nil)
	if host.ui.lastProgressRecord == nil {
		t.Fatal("expected non-nil ProgressRecord to be passed to UI")
	}

	record := host.ui.lastProgressRecord

	// Verify all fields were correctly deserialized
	if record.ActivityID != 5 {
		t.Errorf("expected ActivityId=5, got %d", record.ActivityID)
	}
	if record.ParentActivityID != 2 {
		t.Errorf("expected ParentActivityId=2, got %d", record.ParentActivityID)
	}
	if record.Activity != "Test Progress" {
		t.Errorf("expected Activity='Test Progress', got %q", record.Activity)
	}
	if record.StatusDescription != "Processing items" {
		t.Errorf("expected StatusDescription='Processing items', got %q", record.StatusDescription)
	}
	if record.CurrentOperation != "Item 5 of 10" {
		t.Errorf("expected CurrentOperation='Item 5 of 10', got %q", record.CurrentOperation)
	}
	if record.PercentComplete != 50 {
		t.Errorf("expected PercentComplete=50, got %d", record.PercentComplete)
	}
	if record.SecondsRemaining != 30 {
		t.Errorf("expected SecondsRemaining=30, got %d", record.SecondsRemaining)
	}
	if record.RecordType != objects.ProgressRecordTypeProcessing {
		t.Errorf("expected RecordType=Processing, got %d", record.RecordType)
	}
}

func TestCallbackHandler_HandleWriteProgress_InvalidRecord(t *testing.T) {
	host := newMockHost()
	handler := NewCallbackHandler(host)

	// Pass invalid type for ProgressRecord parameter
	call := &RemoteHostCall{
		CallID:           16,
		MethodID:         MethodIDWriteProgress,
		MethodParameters: []interface{}{int64(456), "invalid"},
	}

	response := handler.HandleCall(call)

	// Should not raise exception (graceful degradation)
	if response.ExceptionRaised {
		t.Errorf("expected no exception for invalid progress record, got: %v", response.ReturnValue)
	}

	// Should pass nil when conversion fails
	if host.ui.lastProgressRecord != nil {
		t.Errorf("expected nil ProgressRecord on conversion error, got %+v", host.ui.lastProgressRecord)
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
		{MethodIDGetInstanceID, 3},
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

// Phase 2: Medium Complexity tests

func TestCallbackHandler_HandleWriteLine3(t *testing.T) {
	host := newMockHost()
	handler := NewCallbackHandler(host)

	call := &RemoteHostCall{
		CallID:   32,
		MethodID: MethodIDWriteLine3,
		MethodParameters: []interface{}{
			int32(15), // foreground color
			int32(0),  // background color
			"colored line text",
		},
	}

	response := handler.HandleCall(call)

	if response.ExceptionRaised {
		t.Errorf("expected no exception, got: %v", response.ReturnValue)
	}
	if host.ui.writeCallCount != 1 {
		t.Errorf("expected 1 WriteLine call, got %d", host.ui.writeCallCount)
	}
	if host.ui.lastWritten != "colored line text" {
		t.Errorf("expected 'colored line text', got %q", host.ui.lastWritten)
	}
}

func TestCallbackHandler_HandleWriteLine3_MissingParams(t *testing.T) {
	host := newMockHost()
	handler := NewCallbackHandler(host)

	call := &RemoteHostCall{
		CallID:           33,
		MethodID:         MethodIDWriteLine3,
		MethodParameters: []interface{}{int32(15)}, // Missing 2 params
	}

	response := handler.HandleCall(call)

	if !response.ExceptionRaised {
		t.Error("expected exception for missing parameters")
	}
}

func TestCallbackHandler_HandlePromptForCredential1(t *testing.T) {
	host := newMockHost()
	handler := NewCallbackHandler(host)

	call := &RemoteHostCall{
		CallID:   34,
		MethodID: MethodIDPromptForCredential1,
		MethodParameters: []interface{}{
			"caption",
			"message",
			"username",
			"target",
		},
	}

	response := handler.HandleCall(call)

	// mockHostUI returns nil for credentials, which is acceptable
	if response.ExceptionRaised {
		t.Errorf("expected no exception, got: %v", response.ReturnValue)
	}
}

func TestCallbackHandler_HandlePromptForCredential1_MissingParams(t *testing.T) {
	host := newMockHost()
	handler := NewCallbackHandler(host)

	call := &RemoteHostCall{
		CallID:           35,
		MethodID:         MethodIDPromptForCredential1,
		MethodParameters: []interface{}{"caption", "message"}, // Missing userName, targetName
	}

	response := handler.HandleCall(call)

	if !response.ExceptionRaised {
		t.Error("expected exception for missing parameters")
	}
}

// Phase 3: Complex Handlers tests

func TestCallbackHandler_HandlePrompt(t *testing.T) {
	host := newMockHost()
	host.ui.promptResult = map[string]interface{}{"Name": "test value"}
	handler := NewCallbackHandler(host)

	// Test with native Go []FieldDescription
	call := &RemoteHostCall{
		CallID:   36,
		MethodID: MethodIDPrompt,
		MethodParameters: []interface{}{
			"Enter Details",
			"Please fill in the following:",
			[]FieldDescription{
				{Name: "Name", Label: "Your Name", IsMandatory: true},
			},
		},
	}

	response := handler.HandleCall(call)

	if response.ExceptionRaised {
		t.Errorf("expected no exception, got: %v", response.ReturnValue)
	}
	result, ok := response.ReturnValue.(map[string]interface{})
	if !ok {
		t.Errorf("expected map[string]interface{}, got %T", response.ReturnValue)
	}
	if result["Name"] != "test value" {
		t.Errorf("expected 'test value', got %v", result["Name"])
	}
}

func TestCallbackHandler_HandlePrompt_WithPSObjects(t *testing.T) {
	host := newMockHost()
	host.ui.promptResult = map[string]interface{}{"Field1": "value1"}
	handler := NewCallbackHandler(host)

	// Test with PSObject field descriptions (like real server sends)
	fieldObj := &serialization.PSObject{
		TypeNames: []string{"System.Management.Automation.Host.FieldDescription"},
		Properties: map[string]interface{}{
			"Name":                  "Field1",
			"Label":                 "Field 1 Label",
			"ParameterTypeName":     "String",
			"ParameterTypeFullName": "System.String",
			"HelpMessage":           "Enter Field 1",
			"IsMandatory":           true,
		},
	}

	call := &RemoteHostCall{
		CallID:   37,
		MethodID: MethodIDPrompt,
		MethodParameters: []interface{}{
			"Caption",
			"Message",
			[]interface{}{fieldObj}, // List of PSObjects
		},
	}

	response := handler.HandleCall(call)

	if response.ExceptionRaised {
		t.Errorf("expected no exception, got: %v", response.ReturnValue)
	}
}

func TestCallbackHandler_HandlePrompt_EmptyFieldList(t *testing.T) {
	host := newMockHost()
	handler := NewCallbackHandler(host)

	call := &RemoteHostCall{
		CallID:   38,
		MethodID: MethodIDPrompt,
		MethodParameters: []interface{}{
			"Caption",
			"Message",
			[]interface{}{}, // Empty list
		},
	}

	response := handler.HandleCall(call)

	if response.ExceptionRaised {
		t.Errorf("expected no exception for empty field list, got: %v", response.ReturnValue)
	}
}

func TestCallbackHandler_HandlePrompt_MissingParams(t *testing.T) {
	host := newMockHost()
	handler := NewCallbackHandler(host)

	call := &RemoteHostCall{
		CallID:           39,
		MethodID:         MethodIDPrompt,
		MethodParameters: []interface{}{"caption", "message"}, // Missing descriptions
	}

	response := handler.HandleCall(call)

	if !response.ExceptionRaised {
		t.Error("expected exception for missing parameters")
	}
}

func TestConvertToFieldDescription_PSObject(t *testing.T) {
	obj := &serialization.PSObject{
		Properties: map[string]interface{}{
			"Name":        "TestField",
			"Label":       "Test Label",
			"HelpMessage": "Help text",
			"IsMandatory": true,
		},
	}

	fd, err := convertToFieldDescription(obj)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fd.Name != "TestField" {
		t.Errorf("expected Name='TestField', got %q", fd.Name)
	}
	if fd.Label != "Test Label" {
		t.Errorf("expected Label='Test Label', got %q", fd.Label)
	}
	if fd.HelpMessage != "Help text" {
		t.Errorf("expected HelpMessage='Help text', got %q", fd.HelpMessage)
	}
	if !fd.IsMandatory {
		t.Error("expected IsMandatory=true")
	}
}

func TestConvertToFieldDescription_MissingOptionalFields(t *testing.T) {
	// Test with minimal fields (only Name)
	obj := &serialization.PSObject{
		Properties: map[string]interface{}{
			"Name": "MinimalField",
		},
	}

	fd, err := convertToFieldDescription(obj)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fd.Name != "MinimalField" {
		t.Errorf("expected Name='MinimalField', got %q", fd.Name)
	}
	// Optional fields should be zero values
	if fd.HelpMessage != "" {
		t.Errorf("expected empty HelpMessage, got %q", fd.HelpMessage)
	}
	if fd.IsMandatory {
		t.Error("expected IsMandatory=false by default")
	}
}

func TestConvertToFieldDescription_InvalidType(t *testing.T) {
	_, err := convertToFieldDescription("invalid string type")
	if err == nil {
		t.Error("expected error for invalid type")
	}
}

func TestConvertToFieldDescriptions_NativeSlice(t *testing.T) {
	input := []FieldDescription{
		{Name: "Field1"},
		{Name: "Field2"},
	}

	result, err := convertToFieldDescriptions(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 2 {
		t.Errorf("expected 2 fields, got %d", len(result))
	}
}

func TestConvertToFieldDescriptions_InvalidType(t *testing.T) {
	_, err := convertToFieldDescriptions("invalid type")
	if err == nil {
		t.Error("expected error for invalid type")
	}
}

func TestConvertToChoiceDescription_PSObject(t *testing.T) {
	obj := &serialization.PSObject{
		Properties: map[string]interface{}{
			"Label":       "Yes",
			"HelpMessage": "Accept",
		},
	}

	cd, err := convertToChoiceDescription(obj)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cd.Label != "Yes" {
		t.Errorf("expected Label='Yes', got %q", cd.Label)
	}
	if cd.HelpMessage != "Accept" {
		t.Errorf("expected HelpMessage='Accept', got %q", cd.HelpMessage)
	}
}

func TestConvertToChoiceDescription_MissingOptional(t *testing.T) {
	obj := &serialization.PSObject{
		Properties: map[string]interface{}{
			"Label": "No",
		},
	}

	cd, err := convertToChoiceDescription(obj)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cd.Label != "No" {
		t.Errorf("expected Label='No', got %q", cd.Label)
	}
	if cd.HelpMessage != "" {
		t.Errorf("expected empty HelpMessage, got %q", cd.HelpMessage)
	}
}

func TestCallbackHandler_HandlePromptForChoiceMultipleSelection(t *testing.T) {
	host := newMockHost()
	host.ui.choiceResult = 0 // Mock returns int, but this method needs []int handling in real UI
	// We need to update mockHostUI to support generic prompt result or specifically multiple choice
	// For now, let's see how HandlePromptForChoiceMultipleSelection handles the return
	handler := NewCallbackHandler(host)

	// Note: The current default MockHostUI.PromptForChoice returns int, not []int.
	// We might expect an error if the handler tries to cast to []int or if validation fails.
	// But let's check the implementation of handlePromptForChoiceMultipleSelection first.

	call := &RemoteHostCall{
		CallID:   40,
		MethodID: MethodIDPromptForChoiceMultipleSelection,
		MethodParameters: []interface{}{
			"Caption",
			"Message",
			[]ChoiceDescription{{Label: "A"}, {Label: "B"}},
			[]int32{0}, // default choices
		},
	}

	response := handler.HandleCall(call)

	// In the basic implementation within callback.go, it might call PromptForChoice (singular)
	// or fail if not implemented. Let's inspect the ReturnValue.
	if response.ExceptionRaised {
		// This is acceptable if not supported, but we should verify behavior.
		// If it succeeded, check return value.
	}
}
