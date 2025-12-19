package host

import (
	"fmt"
	"math"

	"github.com/smnsjas/go-psrpcore/serialization"
)

// EncodeRemoteHostCall serializes a RemoteHostCall to CLIXML bytes.
//
// The CLIXML structure matches Microsoft.PowerShell.Remoting.Internal.RemoteHostCall:
//
//	<Obj RefId="0">
//	  <TN RefId="0">
//	    <T>Microsoft.PowerShell.Remoting.Internal.RemoteHostCall</T>
//	  </TN>
//	  <MS>
//	    <I64 N="ci">1</I64>                   <!-- Call ID -->
//	    <I32 N="mi">3</I32>                   <!-- Method ID -->
//	    <LST N="mp">                          <!-- Method Parameters -->
//	      <S>parameter value</S>
//	    </LST>
//	  </MS>
//	</Obj>
func EncodeRemoteHostCall(call *RemoteHostCall) ([]byte, error) {
	// Build PSObject representation
	props := map[string]interface{}{
		"ci": call.CallID,
		"mi": int32(call.MethodID),
		"mp": call.MethodParameters,
	}

	obj := &serialization.PSObject{
		TypeNames:  []string{"Microsoft.PowerShell.Remoting.Internal.RemoteHostCall"},
		Properties: props,
	}

	// Serialize to CLIXML
	ser := serialization.NewSerializer()
	return ser.Serialize(obj)
}

// DecodeRemoteHostCall deserializes CLIXML bytes to a RemoteHostCall.
func DecodeRemoteHostCall(data []byte) (*RemoteHostCall, error) {
	deser := serialization.NewDeserializer()
	results, err := deser.Deserialize(data)
	if err != nil {
		return nil, fmt.Errorf("deserialize host call: %w", err)
	}

	if len(results) == 0 {
		return nil, fmt.Errorf("no objects in CLIXML")
	}

	// Extract PSObject
	psObj, ok := results[0].(*serialization.PSObject)
	if !ok {
		return nil, fmt.Errorf("expected PSObject, got %T", results[0])
	}

	call := &RemoteHostCall{}

	// Extract CallID (ci)
	if ci, ok := psObj.Properties["ci"]; ok {
		switch v := ci.(type) {
		case int64:
			call.CallID = v
		case int32:
			call.CallID = int64(v)
		case int:
			call.CallID = int64(v)
		default:
			return nil, fmt.Errorf("invalid CallID type: %T", ci)
		}
	} else {
		return nil, fmt.Errorf("missing CallID (ci) property")
	}

	// Extract MethodID (mi)
	if mi, ok := psObj.Properties["mi"]; ok {
		switch v := mi.(type) {
		case int32:
			call.MethodID = MethodID(v)
		case int:
			if v < math.MinInt32 || v > math.MaxInt32 {
				return nil, fmt.Errorf("MethodID out of range: %d", v)
			}
			call.MethodID = MethodID(v)
		case int64:
			if v < math.MinInt32 || v > math.MaxInt32 {
				return nil, fmt.Errorf("MethodID out of range: %d", v)
			}
			call.MethodID = MethodID(v)
		default:
			return nil, fmt.Errorf("invalid MethodID type: %T", mi)
		}
	} else {
		return nil, fmt.Errorf("missing MethodID (mi) property")
	}

	// Extract MethodParameters (mp)
	if mp, ok := psObj.Properties["mp"]; ok {
		if params, ok := mp.([]interface{}); ok {
			call.MethodParameters = params
		} else {
			return nil, fmt.Errorf("invalid MethodParameters type: %T", mp)
		}
	} else {
		// Parameters are optional (some methods take no args)
		call.MethodParameters = []interface{}{}
	}

	return call, nil
}

// EncodeRemoteHostResponse serializes a RemoteHostResponse to CLIXML bytes.
//
// The CLIXML structure matches Microsoft.PowerShell.Remoting.Internal.RemoteHostResponse:
//
//	<Obj RefId="0">
//	  <TN RefId="0">
//	    <T>Microsoft.PowerShell.Remoting.Internal.RemoteHostResponse</T>
//	  </TN>
//	  <MS>
//	    <I64 N="ci">1</I64>                   <!-- Call ID -->
//	    <B N="er">false</B>                   <!-- Exception Raised -->
//	    <Obj N="rv" RefId="1">                <!-- Return Value -->
//	      <S>return value</S>
//	    </Obj>
//	  </MS>
//	</Obj>
func EncodeRemoteHostResponse(response *RemoteHostResponse) ([]byte, error) {
	// Build PSObject representation
	props := map[string]interface{}{
		"ci": response.CallID,
		"er": response.ExceptionRaised,
		"rv": response.ReturnValue,
	}

	obj := &serialization.PSObject{
		TypeNames:  []string{"Microsoft.PowerShell.Remoting.Internal.RemoteHostResponse"},
		Properties: props,
	}

	// Serialize to CLIXML
	ser := serialization.NewSerializer()
	return ser.Serialize(obj)
}

// DecodeRemoteHostResponse deserializes CLIXML bytes to a RemoteHostResponse.
func DecodeRemoteHostResponse(data []byte) (*RemoteHostResponse, error) {
	deser := serialization.NewDeserializer()
	results, err := deser.Deserialize(data)
	if err != nil {
		return nil, fmt.Errorf("deserialize host response: %w", err)
	}

	if len(results) == 0 {
		return nil, fmt.Errorf("no objects in CLIXML")
	}

	// Extract PSObject
	psObj, ok := results[0].(*serialization.PSObject)
	if !ok {
		return nil, fmt.Errorf("expected PSObject, got %T", results[0])
	}

	response := &RemoteHostResponse{}

	// Extract CallID (ci)
	if ci, ok := psObj.Properties["ci"]; ok {
		switch v := ci.(type) {
		case int64:
			response.CallID = v
		case int32:
			response.CallID = int64(v)
		case int:
			response.CallID = int64(v)
		default:
			return nil, fmt.Errorf("invalid CallID type: %T", ci)
		}
	} else {
		return nil, fmt.Errorf("missing CallID (ci) property")
	}

	// Extract ExceptionRaised (er)
	if er, ok := psObj.Properties["er"]; ok {
		if b, ok := er.(bool); ok {
			response.ExceptionRaised = b
		} else {
			return nil, fmt.Errorf("invalid ExceptionRaised type: %T", er)
		}
	} else {
		// Default to false if not present
		response.ExceptionRaised = false
	}

	// Extract ReturnValue (rv)
	if rv, ok := psObj.Properties["rv"]; ok {
		response.ReturnValue = rv
	}
	// ReturnValue can be nil

	return response, nil
}
