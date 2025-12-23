package host

import (
	"testing"
)

func TestNewNullHost(t *testing.T) {
	t.Parallel()
	h := NewNullHost()
	if h == nil {
		t.Fatal("NewNullHost returned nil")
	}

	if h.GetName() != "go-psrp" {
		t.Errorf("expected default name 'go-psrp', got %q", h.GetName())
	}
	if h.GetVersion().Major != 1 {
		t.Errorf("expected default version major 1, got %d", h.GetVersion().Major)
	}
	if h.GetInstanceId() == "" {
		t.Error("expected non-empty instance id")
	}
	if h.GetCurrentCulture() != "en-US" {
		t.Errorf("expected culture 'en-US', got %q", h.GetCurrentCulture())
	}
	if h.GetCurrentUICulture() != "en-US" {
		t.Errorf("expected UI culture 'en-US', got %q", h.GetCurrentUICulture())
	}
}

func TestNullHostUI_Methods(t *testing.T) {
	t.Parallel()
	h := NewNullHost()
	ui := h.UI()
	if ui == nil {
		t.Fatal("UI() returned nil")
	}

	t.Run("ReadLine", func(t *testing.T) {
		s, err := ui.ReadLine()
		if err != nil {
			t.Errorf("ReadLine unexpected error: %v", err)
		}
		if s != "" {
			t.Errorf("ReadLine expected empty string, got %q", s)
		}
	})

	t.Run("ReadLineAsSecureString", func(t *testing.T) {
		ss, err := ui.ReadLineAsSecureString()
		if err != nil {
			t.Errorf("ReadLineAsSecureString unexpected error: %v", err)
		}
		if ss == nil {
			t.Error("ReadLineAsSecureString expected non-nil result")
		}
	})

	t.Run("WriteMethods", func(t *testing.T) {
		// Verify these do not panic
		ui.Write("test")
		ui.WriteLine("test")
		ui.WriteErrorLine("test")
		ui.WriteDebugLine("test")
		ui.WriteVerboseLine("test")
		ui.WriteWarningLine("test")
		ui.WriteProgress(1, nil)
	})

	t.Run("Prompt", func(t *testing.T) {
		res, err := ui.Prompt("cap", "msg", nil)
		if err != nil {
			t.Errorf("Prompt unexpected error: %v", err)
		}
		if len(res) != 0 {
			t.Error("Prompt expected empty map")
		}
	})

	t.Run("PromptForCredential", func(t *testing.T) {
		res, err := ui.PromptForCredential("cap", "msg", "user", "target", CredentialTypeDefault, CredentialUIOptionNone)
		if err != nil {
			t.Errorf("PromptForCredential unexpected error: %v", err)
		}
		if res != nil {
			t.Errorf("PromptForCredential expected nil result, got %v", res)
		}
	})

	t.Run("PromptForChoice", func(t *testing.T) {
		res, err := ui.PromptForChoice("cap", "msg", nil, 1)
		if err != nil {
			t.Errorf("PromptForChoice unexpected error: %v", err)
		}
		if res != 1 {
			t.Errorf("PromptForChoice expected default choice 1, got %d", res)
		}
	})
}

func TestCredentialTypes(t *testing.T) {
	t.Parallel()
	// Simple smoke test for constants
	if CredentialTypeGeneric == 0 {
		t.Error("CredentialTypeGeneric should not be 0")
	}
	if CredentialTypeDomain == 0 {
		t.Error("CredentialTypeDomain should not be 0")
	}
}

func TestMockHostImplementation(t *testing.T) {
	t.Parallel()
	// Verify that MockHost implements Host interface (compile-time check)
	var _ Host = (*NullHost)(nil)
	var _ HostUI = (*NullHostUI)(nil)
}

func TestFieldDescription(t *testing.T) {
	t.Parallel()
	fd := FieldDescription{
		Name:        "test",
		IsMandatory: true,
	}
	if fd.Name != "test" {
		t.Error("FieldDescription struct issue")
	}
}

func TestChoiceDescription(t *testing.T) {
	t.Parallel()
	cd := ChoiceDescription{
		Label:       "Yes",
		HelpMessage: "Accept",
	}
	if cd.Label != "Yes" {
		t.Error("ChoiceDescription struct issue")
	}
}
