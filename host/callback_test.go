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
		{MethodIDRead, "Read"},
		{MethodIDReadLine, "ReadLine"},
		{MethodIDWriteErrorLine, "WriteErrorLine"},
		{MethodIDWrite, "Write"},
		{MethodIDWriteDebugLine, "WriteDebugLine"},
		{MethodIDWriteVerboseLine, "WriteVerboseLine"},
		{MethodIDWriteWarningLine, "WriteWarningLine"},
		{MethodIDPrompt, "Prompt"},
		{MethodIDPromptForCredential, "PromptForCredential"},
		{MethodIDPromptForChoice, "PromptForChoice"},
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
		MethodID:         MethodIDWrite,
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
