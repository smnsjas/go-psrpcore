package host

import (
	"testing"

	"github.com/smnsjas/go-psrpcore/objects"
	"github.com/smnsjas/go-psrpcore/serialization"
)

func TestConvertToProgressRecord(t *testing.T) {
	tests := []struct {
		name    string
		input   interface{}
		want    *objects.ProgressRecord
		wantErr bool
	}{
		{
			name: "PSObject with all fields",
			input: &serialization.PSObject{
				TypeNames: []string{"System.Management.Automation.ProgressRecord"},
				Properties: map[string]interface{}{
					"ActivityId":        int32(1),
					"ParentActivityId":  int32(0),
					"Activity":          "Installing modules",
					"StatusDescription": "Module 3 of 10",
					"CurrentOperation":  "Downloading Az.Accounts",
					"PercentComplete":   int32(30),
					"SecondsRemaining":  int32(120),
					"RecordType":        int32(0), // Processing
				},
			},
			want: &objects.ProgressRecord{
				ActivityId:        1,
				ParentActivityId:  0,
				Activity:          "Installing modules",
				StatusDescription: "Module 3 of 10",
				CurrentOperation:  "Downloading Az.Accounts",
				PercentComplete:   30,
				SecondsRemaining:  120,
				RecordType:        objects.ProgressRecordTypeProcessing,
			},
			wantErr: false,
		},
		{
			name: "Map with minimal fields",
			input: map[string]interface{}{
				"ActivityId": int32(5),
				"Activity":   "Test Progress",
			},
			want: &objects.ProgressRecord{
				ActivityId:       5,
				ParentActivityId: -1,
				Activity:         "Test Progress",
				PercentComplete:  -1,
				SecondsRemaining: -1,
				RecordType:       objects.ProgressRecordTypeProcessing,
			},
			wantErr: false,
		},
		{
			name: "Completed progress record",
			input: &serialization.PSObject{
				Properties: map[string]interface{}{
					"ActivityId":      int32(10),
					"Activity":        "Deployment",
					"RecordType":      int32(1), // Completed
					"PercentComplete": int32(100),
				},
			},
			want: &objects.ProgressRecord{
				ActivityId:       10,
				ParentActivityId: -1,
				Activity:         "Deployment",
				PercentComplete:  100,
				SecondsRemaining: -1,
				RecordType:       objects.ProgressRecordTypeCompleted,
			},
			wantErr: false,
		},
		{
			name:    "Invalid type",
			input:   "not a PSObject",
			want:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := convertToProgressRecord(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("convertToProgressRecord() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}

			if got.ActivityId != tt.want.ActivityId {
				t.Errorf("ActivityId = %d, want %d", got.ActivityId, tt.want.ActivityId)
			}
			if got.ParentActivityId != tt.want.ParentActivityId {
				t.Errorf("ParentActivityId = %d, want %d", got.ParentActivityId, tt.want.ParentActivityId)
			}
			if got.Activity != tt.want.Activity {
				t.Errorf("Activity = %q, want %q", got.Activity, tt.want.Activity)
			}
			if got.StatusDescription != tt.want.StatusDescription {
				t.Errorf("StatusDescription = %q, want %q", got.StatusDescription, tt.want.StatusDescription)
			}
			if got.CurrentOperation != tt.want.CurrentOperation {
				t.Errorf("CurrentOperation = %q, want %q", got.CurrentOperation, tt.want.CurrentOperation)
			}
			if got.PercentComplete != tt.want.PercentComplete {
				t.Errorf("PercentComplete = %d, want %d", got.PercentComplete, tt.want.PercentComplete)
			}
			if got.SecondsRemaining != tt.want.SecondsRemaining {
				t.Errorf("SecondsRemaining = %d, want %d", got.SecondsRemaining, tt.want.SecondsRemaining)
			}
			if got.RecordType != tt.want.RecordType {
				t.Errorf("RecordType = %d, want %d", got.RecordType, tt.want.RecordType)
			}
		})
	}
}
