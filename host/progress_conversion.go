package host

import (
	"fmt"

	"github.com/smnsjas/go-psrpcore/objects"
	"github.com/smnsjas/go-psrpcore/serialization"
)

// convertToProgressRecord converts a PSObject or map to a ProgressRecord.
// This is used to deserialize ProgressRecord parameters in host callbacks.
func convertToProgressRecord(obj interface{}) (*objects.ProgressRecord, error) {
	var props map[string]interface{}

	switch v := obj.(type) {
	case *serialization.PSObject:
		props = v.Properties
	case map[string]interface{}:
		props = v
	default:
		return nil, fmt.Errorf("expected PSObject or map for ProgressRecord, got %T", obj)
	}

	record := &objects.ProgressRecord{}

	// Extract ActivityId (required)
	if activityID, ok := props["ActivityId"].(int32); ok {
		record.ActivityID = int(activityID)
	}

	// Extract ParentActivityId (optional, default -1)
	if parentActivityID, ok := props["ParentActivityId"].(int32); ok {
		record.ParentActivityID = int(parentActivityID)
	} else {
		record.ParentActivityID = -1
	}

	// Extract Activity (required)
	if activity, ok := props["Activity"].(string); ok {
		record.Activity = activity
	}

	// Extract StatusDescription (optional)
	if statusDescription, ok := props["StatusDescription"].(string); ok {
		record.StatusDescription = statusDescription
	}

	// Extract CurrentOperation (optional)
	if currentOperation, ok := props["CurrentOperation"].(string); ok {
		record.CurrentOperation = currentOperation
	}

	// Extract PercentComplete (optional, default -1)
	if percentComplete, ok := props["PercentComplete"].(int32); ok {
		record.PercentComplete = int(percentComplete)
	} else {
		record.PercentComplete = -1
	}

	// Extract SecondsRemaining (optional, default -1)
	if secondsRemaining, ok := props["SecondsRemaining"].(int32); ok {
		record.SecondsRemaining = int(secondsRemaining)
	} else {
		record.SecondsRemaining = -1
	}

	// Extract RecordType (optional, default Processing)
	if recordType, ok := props["RecordType"].(int32); ok {
		record.RecordType = objects.ProgressRecordType(recordType)
	} else {
		record.RecordType = objects.ProgressRecordTypeProcessing
	}

	return record, nil
}
