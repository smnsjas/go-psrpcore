package serialization

import (
	"testing"

	"github.com/smnsjas/go-psrpcore/objects"
)

func TestProgressRecordRoundTrip(t *testing.T) {
	original := &objects.ProgressRecord{
		ActivityID:        10,
		ParentActivityID:  5,
		Activity:          "Deploying application",
		StatusDescription: "Step 2/5",
		CurrentOperation:  "Uploading files",
		PercentComplete:   40,
		SecondsRemaining:  180,
		RecordType:        objects.ProgressRecordTypeProcessing,
	}

	serializer := NewSerializer()
	data, err := serializer.Serialize(original)
	if err != nil {
		t.Fatalf("serialization failed: %v", err)
	}

	deserializer := NewDeserializer()
	results, err := deserializer.Deserialize(data)
	if err != nil {
		t.Fatalf("deserialization failed: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	psObj, ok := results[0].(*PSObject)
	if !ok {
		t.Fatalf("expected PSObject, got %T", results[0])
	}

	// Verify TypeNames
	if len(psObj.TypeNames) < 1 || psObj.TypeNames[0] != "System.Management.Automation.ProgressRecord" {
		t.Errorf("expected TypeName 'System.Management.Automation.ProgressRecord', got %v", psObj.TypeNames)
	}

	// Verify all properties were serialized correctly
	tests := []struct {
		field    string
		expected interface{}
	}{
		{"ActivityId", int32(10)},
		{"ParentActivityId", int32(5)},
		{"Activity", "Deploying application"},
		{"StatusDescription", "Step 2/5"},
		{"CurrentOperation", "Uploading files"},
		{"PercentComplete", int32(40)},
		{"SecondsRemaining", int32(180)},
		{"RecordType", int32(0)}, // Processing
	}

	for _, tt := range tests {
		t.Run(tt.field, func(t *testing.T) {
			got, ok := psObj.Properties[tt.field]
			if !ok {
				t.Fatalf("property %q not found in serialized object", tt.field)
			}
			if got != tt.expected {
				t.Errorf("property %q = %v (%T), want %v (%T)", tt.field, got, got, tt.expected, tt.expected)
			}
		})
	}
}

func TestProgressRecordToPSObject(t *testing.T) {
	record := &objects.ProgressRecord{
		ActivityID:        1,
		ParentActivityID:  0,
		Activity:          "Test",
		StatusDescription: "Testing",
		CurrentOperation:  "Running tests",
		PercentComplete:   75,
		SecondsRemaining:  10,
		RecordType:        objects.ProgressRecordTypeCompleted,
	}

	psObj := ProgressRecordToPSObject(record)

	if psObj == nil {
		t.Fatal("ProgressRecordToPSObject returned nil")
	}

	// Verify TypeNames
	if len(psObj.TypeNames) != 2 {
		t.Errorf("expected 2 TypeNames, got %d", len(psObj.TypeNames))
	}
	if psObj.TypeNames[0] != "System.Management.Automation.ProgressRecord" {
		t.Errorf("expected first TypeName to be ProgressRecord, got %q", psObj.TypeNames[0])
	}

	// Verify properties
	if psObj.Properties["ActivityId"] != int32(1) {
		t.Errorf("ActivityId = %v, want 1", psObj.Properties["ActivityId"])
	}
	if psObj.Properties["Activity"] != "Test" {
		t.Errorf("Activity = %v, want 'Test'", psObj.Properties["Activity"])
	}
	if psObj.Properties["RecordType"] != int32(1) { // Completed
		t.Errorf("RecordType = %v, want 1 (Completed)", psObj.Properties["RecordType"])
	}
}
