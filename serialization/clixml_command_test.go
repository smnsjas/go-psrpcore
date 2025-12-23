package serialization

import (
	"strings"
	"testing"

	"github.com/smnsjas/go-psrpcore/objects"
)

func TestSerializePowerShell(t *testing.T) {
	ps := objects.NewPowerShell()
	ps.AddCommand("Get-Process", false)
	ps.AddParameter("Id", 123)
	ps.AddCommand("Select-Object", false)
	ps.AddParameter("Property", "Name")

	serializer := NewSerializer()
	data, err := serializer.Serialize(ps)
	if err != nil {
		t.Fatalf("Serialize failed: %v", err)
	}

	xmlStr := string(data)

	// Check for key elements in the XML
	expectedSubstrings := []string{
		`<S N="Cmd">Get-Process</S>`,
		`<S N="Cmd">Select-Object</S>`,
		// Parameters are now Args
		`<Obj N="Args"`,
	}

	for _, expected := range expectedSubstrings {
		if !strings.Contains(xmlStr, expected) {
			t.Errorf("Expected XML to contain %q, but it didn't.\nXML: %s", expected, xmlStr)
		}
	}
}
