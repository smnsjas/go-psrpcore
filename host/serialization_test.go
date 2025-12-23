package host

import (
	"strings"
	"testing"
)

func TestEncodeDecodeRemoteHostCall(t *testing.T) {
	tests := []struct {
		name string
		call *RemoteHostCall
	}{
		{
			name: "ReadLine with no parameters",
			call: &RemoteHostCall{
				CallID:           1,
				MethodID:         MethodIDReadLine,
				MethodParameters: []interface{}{},
			},
		},
		{
			name: "WriteErrorLine with string parameter",
			call: &RemoteHostCall{
				CallID:           2,
				MethodID:         MethodIDWriteErrorLine,
				MethodParameters: []interface{}{"error message"},
			},
		},
		{
			name: "PromptForChoice with multiple parameters",
			call: &RemoteHostCall{
				CallID:   3,
				MethodID: MethodIDPromptForChoice,
				MethodParameters: []interface{}{
					"Choose an option",
					"Please select",
					[]interface{}{"Yes", "No"},
					int32(0),
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Encode
			encoded, err := EncodeRemoteHostCall(tt.call)
			if err != nil {
				t.Fatalf("EncodeRemoteHostCall failed: %v", err)
			}

			// Verify it's valid CLIXML
			if !strings.Contains(string(encoded), "<Objs") {
				t.Error("encoded data should contain <Objs> root element")
			}
			if !strings.Contains(string(encoded), "Microsoft.PowerShell.Remoting.Internal.RemoteHostCall") {
				t.Error("encoded data should contain RemoteHostCall type name")
			}

			// Decode
			decoded, err := DecodeRemoteHostCall(encoded)
			if err != nil {
				t.Fatalf("DecodeRemoteHostCall failed: %v", err)
			}

			// Verify CallID
			if decoded.CallID != tt.call.CallID {
				t.Errorf("expected CallID %d, got %d", tt.call.CallID, decoded.CallID)
			}

			// Verify MethodID
			if decoded.MethodID != tt.call.MethodID {
				t.Errorf("expected MethodID %d, got %d", tt.call.MethodID, decoded.MethodID)
			}

			// Verify parameter count
			if len(decoded.MethodParameters) != len(tt.call.MethodParameters) {
				t.Errorf("expected %d parameters, got %d", len(tt.call.MethodParameters), len(decoded.MethodParameters))
			}
		})
	}
}

func TestEncodeDecodeRemoteHostResponse(t *testing.T) {
	tests := []struct {
		name     string
		response *RemoteHostResponse
	}{
		{
			name: "Success response with string return value",
			response: &RemoteHostResponse{
				CallID:          1,
				ExceptionRaised: false,
				ReturnValue:     "user input",
			},
		},
		{
			name: "Success response with int return value",
			response: &RemoteHostResponse{
				CallID:          2,
				ExceptionRaised: false,
				ReturnValue:     int32(42),
			},
		},
		{
			name: "Exception response",
			response: &RemoteHostResponse{
				CallID:          3,
				ExceptionRaised: true,
				ReturnValue:     "Operation failed",
			},
		},
		{
			name: "Response with nil return value",
			response: &RemoteHostResponse{
				CallID:          4,
				ExceptionRaised: false,
				ReturnValue:     nil,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Encode
			encoded, err := EncodeRemoteHostResponse(tt.response)
			if err != nil {
				t.Fatalf("EncodeRemoteHostResponse failed: %v", err)
			}

			// Verify it's valid CLIXML
			if !strings.Contains(string(encoded), "<Objs") {
				t.Error("encoded data should contain <Objs> root element")
			}
			if !strings.Contains(string(encoded), "Microsoft.PowerShell.Remoting.Internal.RemoteHostResponse") {
				t.Error("encoded data should contain RemoteHostResponse type name")
			}

			// Decode
			decoded, err := DecodeRemoteHostResponse(encoded)
			if err != nil {
				t.Fatalf("DecodeRemoteHostResponse failed: %v", err)
			}

			// Verify CallID
			if decoded.CallID != tt.response.CallID {
				t.Errorf("expected CallID %d, got %d", tt.response.CallID, decoded.CallID)
			}

			// Verify ExceptionRaised
			if decoded.ExceptionRaised != tt.response.ExceptionRaised {
				t.Errorf("expected ExceptionRaised %v, got %v", tt.response.ExceptionRaised, decoded.ExceptionRaised)
			}

			// Verify ReturnValue (basic check - exact value depends on serialization round-trip)
			if tt.response.ReturnValue == nil {
				if decoded.ReturnValue != nil {
					t.Errorf("expected nil ReturnValue, got %v", decoded.ReturnValue)
				}
			} else {
				if decoded.ReturnValue == nil {
					t.Error("expected non-nil ReturnValue")
				}
			}
		})
	}
}

func TestDecodeRemoteHostCall_InvalidData(t *testing.T) {
	tests := []struct {
		name string
		data []byte
	}{
		{
			name: "Empty data",
			data: []byte{},
		},
		{
			name: "Invalid XML",
			data: []byte("not xml"),
		},
		{
			name: "Valid XML but missing CallID",
			data: []byte(`<?xml version="1.0"?><Objs Version="1.1.0.1" xmlns="http://schemas.microsoft.com/powershell/2004/04"><Obj RefId="0"><MS><I32 N="mi">2</I32></MS></Obj></Objs>`),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := DecodeRemoteHostCall(tt.data)
			if err == nil {
				t.Error("expected error decoding invalid data")
			}
		})
	}
}

func TestDecodeRemoteHostResponse_InvalidData(t *testing.T) {
	tests := []struct {
		name string
		data []byte
	}{
		{
			name: "Empty data",
			data: []byte{},
		},
		{
			name: "Invalid XML",
			data: []byte("not xml"),
		},
		{
			name: "Valid XML but missing CallID",
			data: []byte(`<?xml version="1.0"?><Objs Version="1.1.0.1" xmlns="http://schemas.microsoft.com/powershell/2004/04"><Obj RefId="0"><MS><B N="er">false</B></MS></Obj></Objs>`),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := DecodeRemoteHostResponse(tt.data)
			if err == nil {
				t.Error("expected error decoding invalid data")
			}
		})
	}
}

func TestRoundTripComplexCall(t *testing.T) {
	// Test with a complex PromptForCredential call
	call := &RemoteHostCall{
		CallID:   100,
		MethodID: MethodIDPromptForCredential1,
		MethodParameters: []interface{}{
			"Enter Credentials",
			"Please provide your credentials",
			"username",
			"target.example.com",
		},
	}

	// Encode
	encoded, err := EncodeRemoteHostCall(call)
	if err != nil {
		t.Fatalf("encode failed: %v", err)
	}

	// Decode
	decoded, err := DecodeRemoteHostCall(encoded)
	if err != nil {
		t.Fatalf("decode failed: %v", err)
	}

	// Verify
	if decoded.CallID != call.CallID {
		t.Errorf("CallID mismatch: expected %d, got %d", call.CallID, decoded.CallID)
	}
	if decoded.MethodID != call.MethodID {
		t.Errorf("MethodID mismatch: expected %d, got %d", call.MethodID, decoded.MethodID)
	}
	if len(decoded.MethodParameters) != 4 {
		t.Errorf("expected 4 parameters, got %d", len(decoded.MethodParameters))
	}
}

// Phase 4: Additional error path tests

func TestDecodeRemoteHostCall_MissingMethodID(t *testing.T) {
	// Valid XML with CallID but missing MethodID (mi)
	data := []byte(`<?xml version="1.0"?><Objs Version="1.1.0.1" xmlns="http://schemas.microsoft.com/powershell/2004/04"><Obj RefId="0"><MS><I64 N="ci">1</I64></MS></Obj></Objs>`)
	_, err := DecodeRemoteHostCall(data)
	if err == nil {
		t.Error("expected error for missing MethodID")
	}
	if !strings.Contains(err.Error(), "MethodID") {
		t.Errorf("error should mention MethodID, got: %v", err)
	}
}

func TestDecodeRemoteHostCall_InvalidCallIDType(t *testing.T) {
	// CallID as string instead of integer
	data := []byte(`<?xml version="1.0"?><Objs Version="1.1.0.1" xmlns="http://schemas.microsoft.com/powershell/2004/04"><Obj RefId="0"><MS><S N="ci">invalid</S><I32 N="mi">11</I32></MS></Obj></Objs>`)
	_, err := DecodeRemoteHostCall(data)
	if err == nil {
		t.Error("expected error for invalid CallID type")
	}
}

func TestDecodeRemoteHostCall_InvalidMethodIDType(t *testing.T) {
	// MethodID as string instead of integer
	data := []byte(`<?xml version="1.0"?><Objs Version="1.1.0.1" xmlns="http://schemas.microsoft.com/powershell/2004/04"><Obj RefId="0"><MS><I64 N="ci">1</I64><S N="mi">invalid</S></MS></Obj></Objs>`)
	_, err := DecodeRemoteHostCall(data)
	if err == nil {
		t.Error("expected error for invalid MethodID type")
	}
}

func TestDecodeRemoteHostCall_InvalidMethodParametersType(t *testing.T) {
	// MethodParameters as string instead of list
	data := []byte(`<?xml version="1.0"?><Objs Version="1.1.0.1" xmlns="http://schemas.microsoft.com/powershell/2004/04"><Obj RefId="0"><MS><I64 N="ci">1</I64><I32 N="mi">11</I32><S N="mp">invalid</S></MS></Obj></Objs>`)
	_, err := DecodeRemoteHostCall(data)
	if err == nil {
		t.Error("expected error for invalid MethodParameters type")
	}
}

func TestDecodeRemoteHostResponse_InvalidExceptionRaisedType(t *testing.T) {
	// ExceptionRaised as string instead of bool
	data := []byte(`<?xml version="1.0"?><Objs Version="1.1.0.1" xmlns="http://schemas.microsoft.com/powershell/2004/04"><Obj RefId="0"><MS><I64 N="ci">1</I64><S N="er">not_bool</S></MS></Obj></Objs>`)
	_, err := DecodeRemoteHostResponse(data)
	if err == nil {
		t.Error("expected error for invalid ExceptionRaised type")
	}
}

func TestDecodeRemoteHostResponse_InvalidCallIDType(t *testing.T) {
	// CallID as string instead of integer
	data := []byte(`<?xml version="1.0"?><Objs Version="1.1.0.1" xmlns="http://schemas.microsoft.com/powershell/2004/04"><Obj RefId="0"><MS><S N="ci">invalid</S><B N="er">false</B></MS></Obj></Objs>`)
	_, err := DecodeRemoteHostResponse(data)
	if err == nil {
		t.Error("expected error for invalid CallID type")
	}
}

func TestDecodeRemoteHostResponse_MissingExceptionRaised(t *testing.T) {
	// Missing ExceptionRaised (er) should default to false without error
	data := []byte(`<?xml version="1.0"?><Objs Version="1.1.0.1" xmlns="http://schemas.microsoft.com/powershell/2004/04"><Obj RefId="0"><MS><I64 N="ci">1</I64></MS></Obj></Objs>`)
	resp, err := DecodeRemoteHostResponse(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.ExceptionRaised {
		t.Error("ExceptionRaised should default to false")
	}
}
