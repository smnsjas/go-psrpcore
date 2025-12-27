//nolint:lll // Test data XML strings are inherently long
package serialization

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/smnsjas/go-psrpcore/objects"
)

func TestSerializePrimitives(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		value    interface{}
		contains string
	}{
		{"string", "hello", "<S>hello</S>"},
		{"int", 42, "<I32>42</I32>"},
		{"int32", int32(42), "<I32>42</I32>"},
		{"int64", int64(9999999999), "<I64>9999999999</I64>"},
		{"bool true", true, "<B>true</B>"},
		{"bool false", false, "<B>false</B>"},
		{"float64", 3.14, "<Db>3.14</Db>"},
		{"nil", nil, "<Nil/>"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewSerializer()
			data, err := s.Serialize(tt.value)
			if err != nil {
				t.Fatalf("Serialize failed: %v", err)
			}

			result := string(data)
			if !strings.Contains(result, tt.contains) {
				t.Errorf("expected %q in output, got:\n%s", tt.contains, result)
			}

			// Verify root element
			if !strings.Contains(result, "<Objs") {
				t.Error("missing <Objs> root element")
			}
			if !strings.Contains(result, CLIXMLNamespace) {
				t.Error("missing CLIXML namespace")
			}
		})
	}
}

func TestSerializeSpecialCharacters(t *testing.T) {
	t.Parallel()
	s := NewSerializer()
	data, err := s.Serialize("<script>alert('xss')</script>")
	if err != nil {
		t.Fatalf("Serialize failed: %v", err)
	}

	result := string(data)
	// Should be XML escaped
	if strings.Contains(result, "<script>") {
		t.Error("special characters should be escaped")
	}
	if !strings.Contains(result, "&lt;script&gt;") {
		t.Error("expected escaped content")
	}
}

func TestSerializeList(t *testing.T) {
	t.Parallel()
	s := NewSerializer()
	data, err := s.Serialize([]interface{}{"a", "b", "c"})
	if err != nil {
		t.Fatalf("Serialize failed: %v", err)
	}

	result := string(data)
	if !strings.Contains(result, "<LST>") {
		t.Error("expected <LST> element")
	}
	if !strings.Contains(result, "</LST>") {
		t.Error("expected </LST> element")
	}
	if !strings.Contains(result, "<S>a</S>") {
		t.Error("expected list items")
	}
}

func TestSerializeByteArray(t *testing.T) {
	t.Parallel()
	s := NewSerializer()
	data, err := s.Serialize([]byte{0x01, 0x02, 0x03, 0x04})
	if err != nil {
		t.Fatalf("Serialize failed: %v", err)
	}

	result := string(data)
	if !strings.Contains(result, "<BA>") {
		t.Error("expected <BA> element")
	}
	// Base64 of 0x01020304 is "AQIDBA=="
	if !strings.Contains(result, "AQIDBA==") {
		t.Errorf("expected base64 encoded bytes, got:\n%s", result)
	}
}

func TestDeserializePrimitives(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		clixml   string
		expected interface{}
	}{
		{
			name:     "string",
			clixml:   `<Objs Version="1.1.0.1" xmlns="http://schemas.microsoft.com/powershell/2004/04"><S>hello</S></Objs>`,
			expected: "hello",
		},
		{
			name:     "int32",
			clixml:   `<Objs Version="1.1.0.1" xmlns="http://schemas.microsoft.com/powershell/2004/04"><I32>42</I32></Objs>`,
			expected: int32(42),
		},
		{
			name:     "int64",
			clixml:   `<Objs Version="1.1.0.1" xmlns="http://schemas.microsoft.com/powershell/2004/04"><I64>9999999999</I64></Objs>`,
			expected: int64(9999999999),
		},
		{
			name:     "bool true",
			clixml:   `<Objs Version="1.1.0.1" xmlns="http://schemas.microsoft.com/powershell/2004/04"><B>true</B></Objs>`,
			expected: true,
		},
		{
			name:     "bool false",
			clixml:   `<Objs Version="1.1.0.1" xmlns="http://schemas.microsoft.com/powershell/2004/04"><B>false</B></Objs>`,
			expected: false,
		},
		{
			name:     "nil",
			clixml:   `<Objs Version="1.1.0.1" xmlns="http://schemas.microsoft.com/powershell/2004/04"><Nil/></Objs>`,
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := NewDeserializer()
			results, err := d.Deserialize([]byte(tt.clixml))
			if err != nil {
				t.Fatalf("Deserialize failed: %v", err)
			}

			if len(results) != 1 {
				t.Fatalf("expected 1 result, got %d", len(results))
			}

			if results[0] != tt.expected {
				t.Errorf("expected %v (%T), got %v (%T)", tt.expected, tt.expected, results[0], results[0])
			}
		})
	}
}

func TestDeserializeList(t *testing.T) {
	t.Parallel()
	clixml := `<Objs Version="1.1.0.1" xmlns="http://schemas.microsoft.com/powershell/2004/04">
		<LST>
			<S>a</S>
			<S>b</S>
			<S>c</S>
		</LST>
	</Objs>`

	d := NewDeserializer()
	results, err := d.Deserialize([]byte(clixml))
	if err != nil {
		t.Fatalf("Deserialize failed: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	list, ok := results[0].([]interface{})
	if !ok {
		t.Fatalf("expected []interface{}, got %T", results[0])
	}

	if len(list) != 3 {
		t.Fatalf("expected 3 items, got %d", len(list))
	}

	expected := []string{"a", "b", "c"}
	for i, exp := range expected {
		if list[i] != exp {
			t.Errorf("item %d: expected %q, got %v", i, exp, list[i])
		}
	}
}

func TestRoundTrip(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		value interface{}
	}{
		{"string", "hello world"},
		{"int", int32(12345)},
		{"bool", true},
		{"list", []interface{}{"x", "y", "z"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewSerializer()
			data, err := s.Serialize(tt.value)
			if err != nil {
				t.Fatalf("Serialize failed: %v", err)
			}

			d := NewDeserializer()
			results, err := d.Deserialize(data)
			if err != nil {
				t.Fatalf("Deserialize failed: %v", err)
			}

			if len(results) != 1 {
				t.Fatalf("expected 1 result, got %d", len(results))
			}

			// Type-specific comparison
			switch expected := tt.value.(type) {
			case int:
				// int serializes as I32
				if results[0] != safeInt32(expected) {
					t.Errorf("expected %v, got %v", expected, results[0])
				}
			case []interface{}:
				got, ok := results[0].([]interface{})
				if !ok {
					t.Fatalf("expected []interface{}, got %T", results[0])
				}
				if len(got) != len(expected) {
					t.Errorf("expected %d items, got %d", len(expected), len(got))
				}
			default:
				if results[0] != tt.value {
					t.Errorf("expected %v, got %v", tt.value, results[0])
				}
			}
		})
	}
}

func TestPSObjectRoundTrip(t *testing.T) {
	t.Parallel()
	obj := &PSObject{
		TypeNames: []string{"System.Management.Automation.PSCustomObject", "System.Object"},
		Properties: map[string]interface{}{
			"Name":   "John Doe",
			"Age":    int32(30),
			"Active": true,
			"Score":  3.14,
		},
		ToString: "John Doe (30)",
	}

	s := NewSerializer()
	data, err := s.Serialize(obj)
	if err != nil {
		t.Fatalf("Serialize failed: %v", err)
	}

	t.Logf("Serialized CLIXML:\n%s", string(data))

	d := NewDeserializer()
	results, err := d.Deserialize(data)
	if err != nil {
		t.Fatalf("Deserialize failed: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	got, ok := results[0].(*PSObject)
	if !ok {
		t.Fatalf("expected *PSObject, got %T", results[0])
	}

	// Verify TypeNames
	if len(got.TypeNames) != len(obj.TypeNames) {
		t.Errorf("TypeNames length mismatch: got %d, want %d", len(got.TypeNames), len(obj.TypeNames))
	}
	for i, tn := range obj.TypeNames {
		if i < len(got.TypeNames) && got.TypeNames[i] != tn {
			t.Errorf("TypeName[%d]: got %q, want %q", i, got.TypeNames[i], tn)
		}
	}

	// Verify Properties
	if got.Properties["Name"] != "John Doe" {
		t.Errorf("Name: got %v, want %v", got.Properties["Name"], "John Doe")
	}
	if got.Properties["Age"] != int32(30) {
		t.Errorf("Age: got %v, want %v", got.Properties["Age"], int32(30))
	}
	if got.Properties["Active"] != true {
		t.Errorf("Active: got %v, want %v", got.Properties["Active"], true)
	}

	// Verify ToString
	if got.ToString != obj.ToString {
		t.Errorf("ToString: got %q, want %q", got.ToString, obj.ToString)
	}
}

func TestHashtableRoundTrip(t *testing.T) {
	t.Parallel()
	hashtable := map[string]interface{}{
		"key1": "value1",
		"key2": int32(42),
		"key3": true,
	}

	s := NewSerializer()
	data, err := s.Serialize(hashtable)
	if err != nil {
		t.Fatalf("Serialize failed: %v", err)
	}

	t.Logf("Serialized Hashtable CLIXML:\n%s", string(data))

	// Verify it contains proper hashtable structure
	if !strings.Contains(string(data), "System.Collections.Hashtable") {
		t.Error("expected Hashtable type in serialized data")
	}
	if !strings.Contains(string(data), "<DCT>") {
		t.Error("expected <DCT> element in serialized data")
	}
	if !strings.Contains(string(data), "<En>") {
		t.Error("expected <En> entries in serialized data")
	}

	d := NewDeserializer()
	results, err := d.Deserialize(data)
	if err != nil {
		t.Fatalf("Deserialize failed: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	got, ok := results[0].(map[string]interface{})
	if !ok {
		t.Fatalf("expected map[string]interface{}, got %T", results[0])
	}

	// Verify all keys are present
	if got["key1"] != "value1" {
		t.Errorf("key1: got %v, want %v", got["key1"], "value1")
	}
	if got["key2"] != int32(42) {
		t.Errorf("key2: got %v, want %v", got["key2"], int32(42))
	}
	if got["key3"] != true {
		t.Errorf("key3: got %v, want %v", got["key3"], true)
	}
}

func TestReferenceTracking(t *testing.T) {
	t.Parallel()
	// Create a PSObject
	shared := &PSObject{
		TypeNames: []string{"Shared.Object"},
		Properties: map[string]interface{}{
			"Value": "shared data",
		},
	}

	// Create a container with the same object referenced twice
	container := &PSObject{
		TypeNames: []string{"Container"},
		Properties: map[string]interface{}{
			"First":  shared,
			"Second": shared,
		},
	}

	s := NewSerializer()
	data, err := s.Serialize(container)
	if err != nil {
		t.Fatalf("Serialize failed: %v", err)
	}

	t.Logf("Serialized with references:\n%s", string(data))

	// Verify that a Ref element is present
	if !strings.Contains(string(data), "<Ref") {
		t.Error("expected <Ref> element for shared reference")
	}

	d := NewDeserializer()
	results, err := d.Deserialize(data)
	if err != nil {
		t.Fatalf("Deserialize failed: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	got, ok := results[0].(*PSObject)
	if !ok {
		t.Fatalf("expected *PSObject, got %T", results[0])
	}

	// Both properties should reference the same object
	first, ok1 := got.Properties["First"].(*PSObject)
	second, ok2 := got.Properties["Second"].(*PSObject)

	if !ok1 || !ok2 {
		t.Fatal("expected both properties to be *PSObject")
	}

	// Verify they're the same object (same pointer)
	if first != second {
		t.Error("expected First and Second to reference the same object")
	}
}

func TestTypeNameReuse(t *testing.T) {
	t.Parallel()
	// Create two objects with the same TypeNames
	obj1 := &PSObject{
		TypeNames: []string{"Custom.Type", "System.Object"},
		Properties: map[string]interface{}{
			"Value": "first",
		},
	}

	obj2 := &PSObject{
		TypeNames: []string{"Custom.Type", "System.Object"},
		Properties: map[string]interface{}{
			"Value": "second",
		},
	}

	// Serialize both in a list
	list := []interface{}{obj1, obj2}

	s := NewSerializer()
	data, err := s.Serialize(list)
	if err != nil {
		t.Fatalf("Serialize failed: %v", err)
	}

	t.Logf("Serialized with TypeName reuse:\n%s", string(data))

	// Verify TNRef is present (TypeNames reference)
	if !strings.Contains(string(data), "TNRef") {
		t.Error("expected TNRef element for reused TypeNames")
	}

	d := NewDeserializer()
	results, err := d.Deserialize(data)
	if err != nil {
		t.Fatalf("Deserialize failed: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	gotList, ok := results[0].([]interface{})
	if !ok {
		t.Fatalf("expected []interface{}, got %T", results[0])
	}

	if len(gotList) != 2 {
		t.Fatalf("expected 2 items, got %d", len(gotList))
	}

	// Verify both objects have the correct TypeNames
	for i, item := range gotList {
		obj, ok := item.(*PSObject)
		if !ok {
			t.Fatalf("item %d: expected *PSObject, got %T", i, item)
		}
		if len(obj.TypeNames) != 2 {
			t.Errorf("item %d: expected 2 TypeNames, got %d", i, len(obj.TypeNames))
		}
		if obj.TypeNames[0] != "Custom.Type" {
			t.Errorf("item %d: TypeNames[0] = %q, want %q", i, obj.TypeNames[0], "Custom.Type")
		}
	}
}

func TestNestedObjects(t *testing.T) {
	t.Parallel()
	nested := &PSObject{
		TypeNames: []string{"Nested.Object"},
		Properties: map[string]interface{}{
			"InnerValue": "nested data",
		},
	}

	parent := &PSObject{
		TypeNames: []string{"Parent.Object"},
		Properties: map[string]interface{}{
			"Child": nested,
			"Name":  "parent",
			"Count": int32(1),
		},
	}

	s := NewSerializer()
	data, err := s.Serialize(parent)
	if err != nil {
		t.Fatalf("Serialize failed: %v", err)
	}

	t.Logf("Serialized nested objects:\n%s", string(data))

	d := NewDeserializer()
	results, err := d.Deserialize(data)
	if err != nil {
		t.Fatalf("Deserialize failed: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	got, ok := results[0].(*PSObject)
	if !ok {
		t.Fatalf("expected *PSObject, got %T", results[0])
	}

	// Verify parent properties
	if got.Properties["Name"] != "parent" {
		t.Errorf("Name: got %v, want %v", got.Properties["Name"], "parent")
	}

	// Verify nested child
	child, ok := got.Properties["Child"].(*PSObject)
	if !ok {
		t.Fatalf("expected Child to be *PSObject, got %T", got.Properties["Child"])
	}

	if child.Properties["InnerValue"] != "nested data" {
		t.Errorf("InnerValue: got %v, want %v", child.Properties["InnerValue"], "nested data")
	}
}

func TestErrorRecordSerialization(t *testing.T) {
	t.Parallel()
	errRec := &objects.ErrorRecord{
		Exception: objects.ExceptionInfo{
			Type:       "System.InvalidOperationException",
			Message:    "Something went wrong",
			StackTrace: "at MyFunction() line 42",
		},
		FullyQualifiedErrorID: "ErrorId123",
		CategoryInfo: objects.CategoryInfo{
			Category: objects.ErrorCategoryInvalidOperation,
			Activity: "Test-Command",
			Reason:   "InvalidOperation",
		},
		InvocationInfo: &objects.InvocationInfo{
			MyCommand:        "Test-Command",
			ScriptName:       "test.ps1",
			ScriptLineNumber: 42,
		},
	}

	s := NewSerializer()
	data, err := s.Serialize(errRec)
	if err != nil {
		t.Fatalf("Serialize failed: %v", err)
	}

	t.Logf("Serialized ErrorRecord:\n%s", string(data))

	// Verify it contains ErrorRecord type
	if !strings.Contains(string(data), "System.Management.Automation.ErrorRecord") {
		t.Error("expected ErrorRecord type in serialized data")
	}

	// Verify key fields are present
	if !strings.Contains(string(data), "Something went wrong") {
		t.Error("expected exception message in serialized data")
	}
	if !strings.Contains(string(data), "ErrorId123") {
		t.Error("expected FullyQualifiedErrorId in serialized data")
	}

	// Deserialize and verify
	d := NewDeserializer()
	results, err := d.Deserialize(data)
	if err != nil {
		t.Fatalf("Deserialize failed: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	obj, ok := results[0].(*PSObject)
	if !ok {
		t.Fatalf("expected *PSObject, got %T", results[0])
	}

	// Verify TypeNames
	if len(obj.TypeNames) == 0 || obj.TypeNames[0] != "System.Management.Automation.ErrorRecord" {
		t.Errorf("expected ErrorRecord TypeName, got %v", obj.TypeNames)
	}

	// Verify FullyQualifiedErrorId
	if obj.Properties["FullyQualifiedErrorId"] != "ErrorId123" {
		t.Errorf("FullyQualifiedErrorID: got %v, want %v", obj.Properties["FullyQualifiedErrorId"], "ErrorId123")
	}

	// Verify Exception
	exception, ok := obj.Properties["Exception"].(*PSObject)
	if !ok {
		t.Fatalf("expected Exception to be *PSObject, got %T", obj.Properties["Exception"])
	}
	if exception.Properties["Message"] != "Something went wrong" {
		t.Errorf("Exception.Message: got %v, want %v", exception.Properties["Message"], "Something went wrong")
	}
}

func TestDeserializerRecursionDepthProtection(t *testing.T) {
	t.Parallel()
	// Create deeply nested XML that would cause stack overflow without protection
	buildNestedXML := func(depth int) string {
		xml := `<?xml version="1.0" encoding="UTF-8"?>
<Objs Version="1.1.0.1" xmlns="http://schemas.microsoft.com/powershell/2004/04">`

		// Start the first object
		xml += `<Obj RefId="0"><TN RefId="0"><T>Level0</T></TN><Props>`

		// Build nested structure (skip i=0 since we already started it)
		for i := 1; i < depth; i++ {
			xml += `<Obj N="Child" RefId="` + fmt.Sprintf("%d", i) + `"><TN RefId="` + fmt.Sprintf("%d", i) + `"><T>Level` + fmt.Sprintf("%d", i) + `</T></TN><Props>`
		}

		// Add the deepest value
		xml += `<S N="Value">deepest</S>`

		// Close all tags
		for i := 0; i < depth; i++ {
			xml += `</Props></Obj>`
		}
		xml += `</Objs>`
		return xml
	}

	t.Run("within limit", func(t *testing.T) {
		// Create nested structure within default limit (100)
		xml := buildNestedXML(50)

		d := NewDeserializer()
		_, err := d.Deserialize([]byte(xml))
		if err != nil {
			t.Fatalf("deserialization within limit failed: %v", err)
		}
	})

	t.Run("exceeds limit", func(t *testing.T) {
		// Create nested structure that exceeds default limit
		xml := buildNestedXML(110)

		d := NewDeserializer()
		_, err := d.Deserialize([]byte(xml))
		if err == nil {
			t.Fatal("expected error for exceeding recursion depth")
		}
		if !errors.Is(err, ErrMaxRecursionDepth) {
			t.Errorf("expected ErrMaxRecursionDepth, got: %v", err)
		}
	})

	t.Run("custom limit", func(t *testing.T) {
		// Test with custom low limit
		xml := buildNestedXML(15)

		d := NewDeserializerWithMaxDepth(10)
		_, err := d.Deserialize([]byte(xml))
		if err == nil {
			t.Fatal("expected error for exceeding custom recursion depth")
		}
		if !errors.Is(err, ErrMaxRecursionDepth) {
			t.Errorf("expected ErrMaxRecursionDepth, got: %v", err)
		}
	})
}

func TestSerializePowerShell(t *testing.T) {
	t.Parallel()
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
