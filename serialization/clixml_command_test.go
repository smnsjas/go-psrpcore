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
	expectedStrings := []string{
		"Microsoft.PowerShell.Commands.PowerShellCmdlet+PowerShell",
		"<S N=\"Cmdlet\">Get-Process</S>",
		"<S N=\"Name\">Id</S>",
		"<I32 N=\"Value\">123</I32>",
		"<S N=\"Cmdlet\">Select-Object</S>",
		"<S N=\"Name\">Property</S>",
		"<S N=\"Value\">Name</S>",
	}

	for _, expected := range expectedStrings {
		if !strings.Contains(xmlStr, expected) {
			t.Errorf("Expected XML to contain %q, but it didn't.\nXML: %s", expected, xmlStr)
		}
	}
}
