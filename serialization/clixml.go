// Package serialization implements CLIXML serialization and deserialization.
//
// CLIXML is an XML-based format used by PowerShell to serialize objects for
// transmission over PSRP. This package provides encoding and decoding of
// PowerShell objects to and from CLIXML format.
//
// # Supported Types
//
// The following PowerShell types are supported:
//
//   - Primitives: String, Int32, Int64, Boolean, Double, Decimal, DateTime, etc.
//   - Collections: Array (LST), Hashtable (DCT), Queue, Stack
//   - Complex: PSObject with properties and type names
//   - Special: SecureString, PSCredential, ErrorRecord, ScriptBlock
//
// # CLIXML Structure
//
// CLIXML documents have the following root structure:
//
//	<Objs Version="1.1.0.1" xmlns="http://schemas.microsoft.com/powershell/2004/04">
//	  <!-- Serialized objects here -->
//	</Objs>
//
// # Type Tags
//
// Common type tags:
//
//	<S>     - String
//	<I32>   - Int32
//	<I64>   - Int64
//	<B>     - Boolean
//	<D>     - Decimal
//	<Db>    - Double
//	<DT>    - DateTime
//	<BA>    - Byte Array (base64)
//	<G>     - GUID
//	<URI>   - URI
//	<Nil>   - Null
//	<Obj>   - Complex object
//	<LST>   - List/Array
//	<DCT>   - Dictionary
//	<SS>    - SecureString
//
// # Reference
//
// MS-PSRP Section 2.2.5: https://docs.microsoft.com/en-us/openspecs/windows_protocols/ms-psrp/
package serialization

import (
	"bytes"
	"encoding/base64"
	"encoding/xml"
	"errors"
	"fmt"
	"math"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/smnsjas/go-psrpcore/objects"
)

// PSRPEnum represents a .NET Enum for serialization.
// It allows serializing an Enum as a complex object with TypeNames, String representation, and Integer value.
type PSRPEnum struct {
	Type     string
	Value    int32
	ToString string
}

// CLIXML namespace and version.
const (
	CLIXMLNamespace = "http://schemas.microsoft.com/powershell/2004/04"
	CLIXMLVersion   = "1.1.0.1"
)

var (
	// ErrUnsupportedType is returned for unsupported types.
	ErrUnsupportedType = errors.New("unsupported type")
	// ErrInvalidCLIXML is returned for malformed CLIXML.
	ErrInvalidCLIXML = errors.New("invalid CLIXML")
)

// PSObject represents a PowerShell object with type information and properties.
type PSObject struct {
	TypeNames  []string
	Properties map[string]interface{}
	Members    map[string]interface{} // Extended properties (MS)
	// ToString optionally provides a string representation
	ToString string
}

// TypedList represents a list with specific TypeNames (for List<PSObject> etc.)
type TypedList struct {
	TypeNames []string
	Items     []interface{}
}

// PSObjectWithRef wraps a PSObject with reference tracking.
type PSObjectWithRef struct {
	PSObject
	RefID int
}

// EncryptionProvider defines the interface for encrypting and decrypting sensitive data.
type EncryptionProvider interface {
	Encrypt(data []byte) ([]byte, error)
	Decrypt(data []byte) ([]byte, error)
}

// Serializer encodes Go values to CLIXML.
type Serializer struct {
	buf        bytes.Buffer
	enc        *xml.Encoder
	refCounter int
	objRefs    []*objRef      // Track object references to avoid cycles (slice because maps aren't hashable)
	tnRefs     map[string]int // Track type name references
	encryptor  EncryptionProvider
}

type objRef struct {
	obj   interface{}
	refID int
}

// pool for serializers
var serializerPool = sync.Pool{
	New: func() interface{} {
		s := &Serializer{
			objRefs: make([]*objRef, 0, 64),
			tnRefs:  make(map[string]int),
		}
		s.enc = xml.NewEncoder(&s.buf)
		// No indentation for performance in production, but preserving behavior for now or making it optional
		// s.enc.Indent("", "  ")
		return s
	},
}

// pool for deserializers
var deserializerPool = sync.Pool{
	New: func() interface{} {
		return &Deserializer{
			objRefs: make(map[int]interface{}),
			tnRefs:  make(map[int][]string),
		}
	},
}

// NewSerializer creates a new Serializer.
// It retrieves a serializer from the pool. Release it with Close().
func NewSerializer() *Serializer {
	return NewSerializerWithEncryption(nil)
}

// NewSerializerWithEncryption creates a new Serializer with an encryption provider.
func NewSerializerWithEncryption(encryptor EncryptionProvider) *Serializer {
	s := serializerPool.Get().(*Serializer)
	s.Reset()
	s.encryptor = encryptor
	// NOTE: Do NOT use s.enc.Indent() - PowerShell's OutOfProcess parser
	// rejects XML with whitespace-only text nodes.
	return s
}

// Close returns the Serializer to the pool.
func (s *Serializer) Close() {
	if s == nil {
		return
	}
	s.Reset()
	serializerPool.Put(s)
}

// Reset clears the Serializer state for reuse.
func (s *Serializer) Reset() {
	s.buf.Reset()
	s.refCounter = 0
	// Keep capacity
	s.objRefs = s.objRefs[:0]
	// Clear map
	for k := range s.tnRefs {
		delete(s.tnRefs, k)
	}
	s.encryptor = nil
}

// findObjRef searches for an existing object reference
func (s *Serializer) findObjRef(v interface{}) (int, bool) {
	for _, ref := range s.objRefs {
		// Use pointer equality for PSObject pointers
		if pObj1, ok1 := v.(*PSObject); ok1 {
			if pObj2, ok2 := ref.obj.(*PSObject); ok2 {
				if pObj1 == pObj2 {
					return ref.refID, true
				}
			}
		}
		// For other types, just skip (maps can't be compared)
	}
	return 0, false
}

// addObjRef adds a new object reference
func (s *Serializer) addObjRef(v interface{}, refID int) {
	s.objRefs = append(s.objRefs, &objRef{obj: v, refID: refID})
}

// Serialize converts a Go value to CLIXML bytes.
func (s *Serializer) Serialize(v interface{}) ([]byte, error) {
	s.buf.Reset()

	// Omit XML header for PSRP payloads as they are usually fragments or wrapped
	// s.buf.WriteString(xml.Header)

	// Write root Objs element
	s.buf.WriteString(`<Objs Version="` + CLIXMLVersion + `" xmlns="` + CLIXMLNamespace + `">`)

	if err := s.serializeValue(v); err != nil {
		return nil, fmt.Errorf("serialize value: %w", err)
	}

	s.buf.WriteString("</Objs>")

	// Return a copy of the data to ensure safety when the serializer is returned to the pool
	// The pool reuse clears the buffer, which would invalidate the slice if we returned it directly
	/// guarding against async usage of the returned slice.
	// We use append which allocates a new slice of the exact required size.
	return append([]byte(nil), s.buf.Bytes()...), nil
}

// SerializeRaw converts a Go value to CLIXML bytes WITHOUT the <Objs> wrapper.
// This is used for PSRP message payloads which expect raw serialized objects.
// Per MS-PSRP, message data contains serialized objects directly, not wrapped in <Objs>.
func (s *Serializer) SerializeRaw(v interface{}) ([]byte, error) {
	s.buf.Reset()

	if err := s.serializeValue(v); err != nil {
		return nil, fmt.Errorf("serialize value: %w", err)
	}

	return append([]byte(nil), s.buf.Bytes()...), nil
}

// SerializeMultipleRaw serializes multiple values WITHOUT the <Objs> wrapper.
// This is used for PSRP messages that contain multiple sequential objects.
func (s *Serializer) SerializeMultipleRaw(values ...interface{}) ([]byte, error) {
	s.buf.Reset()

	for _, v := range values {
		if err := s.serializeValue(v); err != nil {
			return nil, fmt.Errorf("serialize value: %w", err)
		}
	}

	return append([]byte(nil), s.buf.Bytes()...), nil
}

func (s *Serializer) serializeValue(v interface{}) error {
	return s.serializeValueWithName(v, "")
}

func (s *Serializer) serializeValueWithName(v interface{}, name string) error {
	if v == nil {
		if name != "" {
			s.buf.WriteString(fmt.Sprintf("<Nil N=\"%s\"/>", name))
		} else {
			s.buf.WriteString("<Nil/>")
		}
		return nil
	}

	// Check for reference (only for PSObject pointers)
	switch v.(type) {
	case *PSObject:
		if refID, exists := s.findObjRef(v); exists {
			// Object already serialized, emit reference
			if name != "" {
				s.buf.WriteString(`<Ref N="`)
				s.buf.WriteString(name)
				s.buf.WriteString(`" RefId="`)
				s.buf.WriteString(strconv.Itoa(refID))
				s.buf.WriteString(`"/>`)
			} else {
				s.buf.WriteString(`<Ref RefId="`)
				s.buf.WriteString(strconv.Itoa(refID))
				s.buf.WriteString(`"/>`)
			}
			return nil
		}
	}

	nameAttr := ""
	if name != "" {
		// Use manual string concatenation or builder if heavily used, but Sprintf is okay for short strings
		// Optimization: avoid Sprintf for common case
		nameAttr = ` N="` + name + `"`
	}

	switch val := v.(type) {
	case *PSRPEnum:
		// <Obj N="..." RefId="..."><TN RefId="..."><T>Type</T>...</TN><ToString>Str</ToString><I32>Val</I32></Obj>
		// NOTE: Must use same increment pattern as serializePSObject (get, then increment)
		refID := s.refCounter
		s.refCounter++

		s.buf.WriteString("<Obj")
		s.buf.WriteString(nameAttr) // Handles N="..."
		s.buf.WriteString(" RefId=\"")
		s.buf.WriteString(strconv.Itoa(refID))
		s.buf.WriteString("\">")

		// Create TypeNames key for PSRPEnum
		tnKey := val.Type + "|System.Enum|System.ValueType|System.Object"

		if tnRefID, exists := s.tnRefs[tnKey]; exists {
			s.buf.WriteString(`<TNRef RefId="`)
			s.buf.WriteString(strconv.Itoa(tnRefID))
			s.buf.WriteString(`"/>`)
		} else {
			tnRefID := len(s.tnRefs)
			s.tnRefs[tnKey] = tnRefID

			s.buf.WriteString("<TN RefId=\"")
			s.buf.WriteString(strconv.Itoa(tnRefID))
			s.buf.WriteString("\"><T>")
			s.buf.WriteString(val.Type)
			s.buf.WriteString("</T><T>System.Enum</T><T>System.ValueType</T><T>System.Object</T></TN>")
		}

		s.buf.WriteString("<ToString>")
		s.buf.WriteString(val.ToString)
		s.buf.WriteString("</ToString>")

		s.buf.WriteString("<I32>")
		s.buf.WriteString(strconv.Itoa(int(val.Value)))
		s.buf.WriteString("</I32>")

		s.buf.WriteString("</Obj>")

	case string:
		s.buf.WriteString("<S")
		s.buf.WriteString(nameAttr)
		s.buf.WriteString(">")
		if err := xml.EscapeText(&s.buf, []byte(val)); err != nil {
			return fmt.Errorf("escape string: %w", err)
		}
		s.buf.WriteString("</S>")

	case int:
		s.buf.WriteString("<I32")
		s.buf.WriteString(nameAttr)
		s.buf.WriteString(">")
		s.buf.WriteString(strconv.FormatInt(int64(val), 10))
		s.buf.WriteString("</I32>")

	case int32:
		s.buf.WriteString("<I32")
		s.buf.WriteString(nameAttr)
		s.buf.WriteString(">")
		s.buf.WriteString(strconv.FormatInt(int64(val), 10))
		s.buf.WriteString("</I32>")

	case int64:
		s.buf.WriteString("<I64")
		s.buf.WriteString(nameAttr)
		s.buf.WriteString(">")
		s.buf.WriteString(strconv.FormatInt(val, 10))
		s.buf.WriteString("</I64>")

	case bool:
		s.buf.WriteString("<B")
		s.buf.WriteString(nameAttr)
		s.buf.WriteString(">")
		if val {
			s.buf.WriteString("true")
		} else {
			s.buf.WriteString("false")
		}
		s.buf.WriteString("</B>")

	case float64:
		s.buf.WriteString("<Db")
		s.buf.WriteString(nameAttr)
		s.buf.WriteString(">")
		s.buf.WriteString(strconv.FormatFloat(val, 'f', -1, 64))
		s.buf.WriteString("</Db>")

	case []byte:
		s.buf.WriteString("<BA")
		s.buf.WriteString(nameAttr)
		s.buf.WriteString(">")
		s.buf.WriteString(base64.StdEncoding.EncodeToString(val))
		s.buf.WriteString("</BA>")

	case time.Time:
		s.buf.WriteString("<DT")
		s.buf.WriteString(nameAttr)
		s.buf.WriteString(">")
		s.buf.WriteString(val.Format(time.RFC3339Nano))
		s.buf.WriteString("</DT>")

	case uuid.UUID:
		s.buf.WriteString("<G")
		s.buf.WriteString(nameAttr)
		s.buf.WriteString(">")
		s.buf.WriteString(val.String())
		s.buf.WriteString("</G>")

	case []interface{}:
		return s.serializeArray(reflect.ValueOf(val), name)

	case PSObject:
		return s.serializePSObject(&val, name)

	case *PSObject:
		return s.serializePSObject(val, name)

	case map[string]interface{}:
		return s.serializeHashtable(val, name)

	case map[interface{}]interface{}:
		return s.serializeGenericMap(val, name)

	case objects.ErrorRecord:
		return s.serializePSObject(ErrorRecordToPSObject(&val), name)

	case *objects.ErrorRecord:
		return s.serializePSObject(ErrorRecordToPSObject(val), name)

	case *objects.SecureString:
		s.buf.WriteString("<SS")
		s.buf.WriteString(nameAttr)
		s.buf.WriteString(">")
		var data []byte
		if s.encryptor != nil {
			// Encrypt with provider (Session Key)
			var err error
			data, err = s.encryptor.Encrypt(val.EncryptedBytes())
			if err != nil {
				return fmt.Errorf("encrypt secure string: %w", err)
			}
		} else {
			// No provider, use internal encrypted bytes (local protection)
			data = val.EncryptedBytes()
		}
		s.buf.WriteString(base64.StdEncoding.EncodeToString(data))
		s.buf.WriteString("</SS>")

	case *objects.ScriptBlock:
		s.buf.WriteString(fmt.Sprintf("<SB%s>", nameAttr))
		if err := xml.EscapeText(&s.buf, []byte(val.Text)); err != nil {
			return fmt.Errorf("escape script block: %w", err)
		}
		s.buf.WriteString("</SB>")

	case *objects.PowerShell:
		return s.serializePSObject(PowerShellToPSObject(val), name)

	case *objects.Command:
		return s.serializePSObject(CommandToPSObject(val), name)

	case *objects.CommandParameter:
		return s.serializePSObject(CommandParameterToPSObject(val), name)

	case objects.Version:
		s.buf.WriteString("<Version")
		s.buf.WriteString(nameAttr)
		s.buf.WriteString(">")
		s.buf.WriteString(val.String())
		s.buf.WriteString("</Version>")

	case *objects.Version:
		s.buf.WriteString("<Version")
		s.buf.WriteString(nameAttr)
		s.buf.WriteString(">")
		s.buf.WriteString(val.String())
		s.buf.WriteString("</Version>")

	case *TypedList:
		return s.serializeTypedList(val, name)

	default:
		// Handle generic slices/arrays
		rVal := reflect.ValueOf(v)
		switch rVal.Kind() {
		case reflect.Slice, reflect.Array:
			return s.serializeArray(rVal, name)
		}

		return fmt.Errorf("%w: %T", ErrUnsupportedType, v)
	}

	return nil
}

// serializeArray serializes a slice or array as a LST element
func (s *Serializer) serializeArray(v reflect.Value, name string) error {
	// Allocate RefID (arrays are reference types)
	refID := s.refCounter
	s.refCounter++

	nameAttr := ""
	if name != "" {
		nameAttr = fmt.Sprintf(" N=\"%s\"", name)
	}

	s.buf.WriteString("<Obj")
	s.buf.WriteString(nameAttr)
	s.buf.WriteString(` RefId="`)
	s.buf.WriteString(strconv.Itoa(refID))
	s.buf.WriteString(`">`)

	// TypeNames for Array
	typeNames := []string{"System.Object[]", "System.Array", "System.Object"}
	tnKey := strings.Join(typeNames, "|")

	if tnRefID, exists := s.tnRefs[tnKey]; exists {
		s.buf.WriteString(`<TNRef RefId="`)
		s.buf.WriteString(strconv.Itoa(tnRefID))
		s.buf.WriteString(`"/>`)
	} else {
		tnRefID := len(s.tnRefs)
		s.tnRefs[tnKey] = tnRefID
		s.buf.WriteString(`<TN RefId="`)
		s.buf.WriteString(strconv.Itoa(tnRefID))
		s.buf.WriteString(`">`)
		for _, tn := range typeNames {
			s.buf.WriteString("<T>")
			s.buf.WriteString(tn)
			s.buf.WriteString("</T>")
		}
		s.buf.WriteString("</TN>")
	}

	s.buf.WriteString("<LST>")

	for i := 0; i < v.Len(); i++ {
		val := v.Index(i).Interface()
		if err := s.serializeValueWithName(val, ""); err != nil {
			return fmt.Errorf("serialize array element %d: %w", i, err)
		}
	}

	s.buf.WriteString("</LST>")
	s.buf.WriteString("</Obj>")
	return nil
}

// serializeTypedList serializes a TypedList with custom TypeNames
func (s *Serializer) serializeTypedList(list *TypedList, name string) error {
	// Allocate RefID
	refID := s.refCounter
	s.refCounter++

	nameAttr := ""
	if name != "" {
		nameAttr = fmt.Sprintf(" N=\"%s\"", name)
	}

	s.buf.WriteString("<Obj")
	s.buf.WriteString(nameAttr)
	s.buf.WriteString(` RefId="`)
	s.buf.WriteString(strconv.Itoa(refID))
	s.buf.WriteString(`">`)

	// Write TypeNames using the custom TypeNames from the struct
	if len(list.TypeNames) > 0 {
		tnKey := strings.Join(list.TypeNames, "|")

		if tnRefID, exists := s.tnRefs[tnKey]; exists {
			s.buf.WriteString(`<TNRef RefId="`)
			s.buf.WriteString(strconv.Itoa(tnRefID))
			s.buf.WriteString(`"/>`)
		} else {
			// Allocate new TN RefID
			tnRefID := len(s.tnRefs)
			s.tnRefs[tnKey] = tnRefID

			s.buf.WriteString(`<TN RefId="`)
			s.buf.WriteString(strconv.Itoa(tnRefID))
			s.buf.WriteString(`">`)
			for _, tn := range list.TypeNames {
				s.buf.WriteString("<T>")
				s.buf.WriteString(tn)
				s.buf.WriteString("</T>")
			}
			s.buf.WriteString("</TN>")
		}
	}

	s.buf.WriteString("<LST>")
	for i, item := range list.Items {
		if err := s.serializeValueWithName(item, ""); err != nil {
			return fmt.Errorf("serialize typed list element %d: %w", i, err)
		}
	}
	s.buf.WriteString("</LST>")
	s.buf.WriteString("</Obj>")
	return nil
}

// ErrorRecordToPSObject converts an ErrorRecord to a PSObject for serialization
func ErrorRecordToPSObject(err *objects.ErrorRecord) *PSObject {
	props := make(map[string]interface{})

	// Exception
	if err.Exception.Type != "" || err.Exception.Message != "" {
		exProps := make(map[string]interface{})
		if err.Exception.Type != "" {
			exProps["Type"] = err.Exception.Type
		}
		if err.Exception.Message != "" {
			exProps["Message"] = err.Exception.Message
		}
		if err.Exception.StackTrace != "" {
			exProps["StackTrace"] = err.Exception.StackTrace
		}
		props["Exception"] = &PSObject{
			TypeNames:  []string{"System.Management.Automation.RuntimeException"},
			Properties: exProps,
		}
	}

	// TargetObject
	if err.TargetObject != nil {
		props["TargetObject"] = err.TargetObject
	}

	// FullyQualifiedErrorId
	if err.FullyQualifiedErrorId != "" {
		props["FullyQualifiedErrorId"] = err.FullyQualifiedErrorId
	}

	// InvocationInfo
	if err.InvocationInfo != nil {
		invProps := make(map[string]interface{})
		if err.InvocationInfo.MyCommand != "" {
			invProps["MyCommand"] = err.InvocationInfo.MyCommand
		}
		if err.InvocationInfo.ScriptLineNumber > 0 {
			if err.InvocationInfo.ScriptLineNumber > math.MaxInt32 {
				// Cap at MaxInt32 if somehow larger
				invProps["ScriptLineNumber"] = int32(math.MaxInt32)
			} else {
				invProps["ScriptLineNumber"] = int32(err.InvocationInfo.ScriptLineNumber) // #nosec G115 -- bounds checked via if/else cap
			}
		}
		if err.InvocationInfo.ScriptName != "" {
			invProps["ScriptName"] = err.InvocationInfo.ScriptName
		}
		props["InvocationInfo"] = &PSObject{
			TypeNames:  []string{"System.Management.Automation.InvocationInfo"},
			Properties: invProps,
		}
	}

	// CategoryInfo
	catProps := make(map[string]interface{})
	cat := int(err.CategoryInfo.Category)
	if cat < math.MinInt32 || cat > math.MaxInt32 {
		cat = int(objects.ErrorCategoryNotSpecified)
	}
	catProps["Category"] = int32(cat) // #nosec G115 -- bounds checked above
	if err.CategoryInfo.Activity != "" {
		catProps["Activity"] = err.CategoryInfo.Activity
	}
	if err.CategoryInfo.Reason != "" {
		catProps["Reason"] = err.CategoryInfo.Reason
	}
	props["CategoryInfo"] = &PSObject{
		TypeNames:  []string{"System.Management.Automation.ErrorCategoryInfo"},
		Properties: catProps,
	}

	return &PSObject{
		TypeNames:  []string{"System.Management.Automation.ErrorRecord", "System.Object"},
		Properties: props,
		ToString:   err.Exception.Message,
	}
}

// serializeGenericMap serializes a generic map as a PowerShell Hashtable (DCT)
func (s *Serializer) serializeGenericMap(m map[interface{}]interface{}, name string) error {
	// Allocate RefID for this object
	refID := s.refCounter
	s.refCounter++
	s.addObjRef(m, refID)

	nameAttr := ""
	if name != "" {
		nameAttr = fmt.Sprintf(" N=\"%s\"", name)
	}

	s.buf.WriteString("<Obj")
	s.buf.WriteString(nameAttr)
	s.buf.WriteString(` RefId="`)
	s.buf.WriteString(strconv.Itoa(refID))
	s.buf.WriteString(`">`)

	// TypeNames for Hashtable
	typeNames := []string{"System.Collections.Hashtable", "System.Object"}
	tnKey := strings.Join(typeNames, "|")

	if tnRefID, exists := s.tnRefs[tnKey]; exists {
		s.buf.WriteString(`<TNRef RefId="`)
		s.buf.WriteString(strconv.Itoa(tnRefID))
		s.buf.WriteString(`"/>`)
	} else {
		tnRefID := len(s.tnRefs)
		s.tnRefs[tnKey] = tnRefID
		s.buf.WriteString(`<TN RefId="`)
		s.buf.WriteString(strconv.Itoa(tnRefID))
		s.buf.WriteString(`">`)
		for _, tn := range typeNames {
			s.buf.WriteString("<T>")
			s.buf.WriteString(tn)
			s.buf.WriteString("</T>")
		}
		s.buf.WriteString("</TN>")
	}

	s.buf.WriteString("<DCT>")

	for k, v := range m {
		s.buf.WriteString("<En>")
		// Key (Name is empty string because it's a dictionary entry key, not a property)
		if err := s.serializeValueWithName(k, "Key"); err != nil {
			return fmt.Errorf("serialize map key: %w", err)
		}
		// Value
		if err := s.serializeValueWithName(v, "Value"); err != nil {
			return fmt.Errorf("serialize map value: %w", err)
		}
		s.buf.WriteString("</En>")
	}

	s.buf.WriteString("</DCT>")
	s.buf.WriteString("</Obj>")
	return nil
}

// serializePSObject serializes a PSObject with TypeNames and Properties
func (s *Serializer) serializePSObject(obj *PSObject, name string) error {
	// Allocate RefID for this object
	refID := s.refCounter
	s.refCounter++
	s.addObjRef(obj, refID)

	nameAttr := ""
	if name != "" {
		nameAttr = fmt.Sprintf(" N=\"%s\"", name)
	}

	s.buf.WriteString("<Obj")
	s.buf.WriteString(nameAttr)
	s.buf.WriteString(` RefId="`)
	s.buf.WriteString(strconv.Itoa(refID))
	s.buf.WriteString(`">`)

	// Serialize TypeNames
	if len(obj.TypeNames) > 0 {
		tnKey := strings.Join(obj.TypeNames, "|")
		if tnRefID, exists := s.tnRefs[tnKey]; exists {
			// Reference existing TypeNames
			s.buf.WriteString(`<TNRef RefId="`)
			s.buf.WriteString(strconv.Itoa(tnRefID))
			s.buf.WriteString(`"/>`)
		} else {
			// New TypeNames
			tnRefID := len(s.tnRefs)
			s.tnRefs[tnKey] = tnRefID
			s.buf.WriteString(`<TN RefId="`)
			s.buf.WriteString(strconv.Itoa(tnRefID))
			s.buf.WriteString(`">`)
			for _, tn := range obj.TypeNames {
				s.buf.WriteString("<T>")
				if err := xml.EscapeText(&s.buf, []byte(tn)); err != nil {
					return fmt.Errorf("escape type name: %w", err)
				}
				s.buf.WriteString("</T>")
			}
			s.buf.WriteString("</TN>")
		}
	}

	// Serialize ToString if present
	if obj.ToString != "" {
		s.buf.WriteString("<ToString>")
		if err := xml.EscapeText(&s.buf, []byte(obj.ToString)); err != nil {
			return fmt.Errorf("escape tostring: %w", err)
		}
		s.buf.WriteString("</ToString>")
	}

	// Serialize Members (MS)
	if len(obj.Members) > 0 {
		s.buf.WriteString("<MS>")
		// Sort keys for deterministic output
		keys := make([]string, 0, len(obj.Members))
		for k := range obj.Members {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, propName := range keys {
			propValue := obj.Members[propName]
			if err := s.serializeValueWithName(propValue, propName); err != nil {
				return fmt.Errorf("serialize member %s: %w", propName, err)
			}
		}
		s.buf.WriteString("</MS>")
	}

	// Serialize Properties
	if len(obj.Properties) > 0 {
		s.buf.WriteString("<Props>")
		// Sort keys for deterministic output
		keys := make([]string, 0, len(obj.Properties))
		for k := range obj.Properties {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, propName := range keys {
			propValue := obj.Properties[propName]
			if err := s.serializeValueWithName(propValue, propName); err != nil {
				return fmt.Errorf("serialize property %s: %w", propName, err)
			}
		}
		s.buf.WriteString("</Props>")
	}

	s.buf.WriteString("</Obj>")
	return nil
}

// serializeHashtable serializes a map as a PowerShell Hashtable (DCT)
func (s *Serializer) serializeHashtable(m map[string]interface{}, name string) error {
	// Allocate RefID for this hashtable
	refID := s.refCounter
	s.refCounter++
	// Don't track map references since maps aren't hashable in Go

	nameAttr := ""
	if name != "" {
		nameAttr = fmt.Sprintf(" N=\"%s\"", name)
	}

	s.buf.WriteString("<Obj")
	s.buf.WriteString(nameAttr)
	s.buf.WriteString(` RefId="`)
	s.buf.WriteString(strconv.Itoa(refID))
	s.buf.WriteString(`">`)

	// TypeNames for Hashtable
	tnKey := "System.Collections.Hashtable"
	if tnRefID, exists := s.tnRefs[tnKey]; exists {
		s.buf.WriteString(`<TNRef RefId="`)
		s.buf.WriteString(strconv.Itoa(tnRefID))
		s.buf.WriteString(`"/>`)
	} else {
		tnRefID := len(s.tnRefs)
		s.tnRefs[tnKey] = tnRefID
		s.buf.WriteString(`<TN RefId="`)
		s.buf.WriteString(strconv.Itoa(tnRefID))
		s.buf.WriteString(`">`)
		s.buf.WriteString("<T>System.Collections.Hashtable</T>")
		s.buf.WriteString("<T>System.Object</T>")
		s.buf.WriteString("</TN>")
	}

	// Serialize dictionary entries
	s.buf.WriteString("<DCT>")
	for k, v := range m {
		s.buf.WriteString("<En>")
		s.buf.WriteString("<S N=\"Key\">")
		if err := xml.EscapeText(&s.buf, []byte(k)); err != nil {
			return fmt.Errorf("escape dict key: %w", err)
		}
		s.buf.WriteString("</S>")
		if err := s.serializeValueWithName(v, "Value"); err != nil {
			return fmt.Errorf("serialize dict value for key %s: %w", k, err)
		}
		s.buf.WriteString("</En>")
	}
	s.buf.WriteString("</DCT>")

	s.buf.WriteString("</Obj>")
	return nil
}

// Deserializer decodes CLIXML to Go values.
type Deserializer struct {
	dec       *xml.Decoder
	objRefs   map[int]interface{} // Track deserialized objects by RefId
	tnRefs    map[int][]string    // Track TypeNames by RefId
	depth     int                 // Current recursion depth
	maxDepth  int                 // Maximum allowed recursion depth
	decryptor EncryptionProvider
}

const (
	// DefaultMaxRecursionDepth is the default limit for CLIXML nesting depth
	DefaultMaxRecursionDepth = 100
)

// NewDeserializer creates a new Deserializer with default recursion limit.
func NewDeserializer() *Deserializer {
	return NewDeserializerWithEncryption(nil)
}

// NewDeserializerWithEncryption creates a new Deserializer with an encryption provider.
func NewDeserializerWithEncryption(decryptor EncryptionProvider) *Deserializer {
	return NewDeserializerWithMaxDepthAndEncryption(DefaultMaxRecursionDepth, decryptor)
}

// NewDeserializerWithMaxDepth creates a new Deserializer with custom recursion limit.
func NewDeserializerWithMaxDepth(maxDepth int) *Deserializer {
	return NewDeserializerWithMaxDepthAndEncryption(maxDepth, nil)
}

// NewDeserializerWithMaxDepthAndEncryption creates a new Deserializer with custom settings.
func NewDeserializerWithMaxDepthAndEncryption(maxDepth int, decryptor EncryptionProvider) *Deserializer {
	d := deserializerPool.Get().(*Deserializer)
	d.Reset()
	d.maxDepth = maxDepth
	d.decryptor = decryptor
	return d
}

// Close returns the Deserializer to the pool.
func (d *Deserializer) Close() {
	if d == nil {
		return
	}
	d.Reset()
	deserializerPool.Put(d)
}

// Reset clears the Deserializer state.
func (d *Deserializer) Reset() {
	d.dec = nil
	// Clear map
	for k := range d.objRefs {
		delete(d.objRefs, k)
	}
	// Clear map
	for k := range d.tnRefs {
		delete(d.tnRefs, k)
	}
	d.depth = 0
	d.maxDepth = 0
	d.decryptor = nil
}

// Deserialize converts CLIXML bytes to Go values.
func (d *Deserializer) Deserialize(data []byte) ([]interface{}, error) {
	// Strip UTF-8 BOM if present
	if len(data) >= 3 && data[0] == 0xef && data[1] == 0xbb && data[2] == 0xbf {
		data = data[3:]
	}

	d.dec = xml.NewDecoder(bytes.NewReader(data))

	// Find root element (Objs or single Obj)
	var root xml.StartElement
	for {
		tok, err := d.dec.Token()
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrInvalidCLIXML, err)
		}

		if se, ok := tok.(xml.StartElement); ok {
			root = se
			break
		}
	}

	var results []interface{}

	if root.Name.Local == "Objs" {
		// Standard list wrapper <Objs>...</Objs>
		for {
			val, done, err := d.deserializeNext()
			if err != nil {
				return nil, err
			}
			if done {
				break
			}
			results = append(results, val)
		}
	} else {
		// Single root object, e.g. <Obj>...</Obj>
		// We already consumed the StartElement, so decode it directly.
		val, err := d.deserializeElement(root)
		if err != nil {
			return nil, err
		}
		results = append(results, val)
	}

	return results, nil
}

func (d *Deserializer) deserializeNext() (interface{}, bool, error) {
	for {
		tok, err := d.dec.Token()
		if err != nil {
			return nil, false, fmt.Errorf("%w: read token: %v", ErrInvalidCLIXML, err)
		}

		switch t := tok.(type) {
		case xml.StartElement:
			val, err := d.deserializeElement(t)
			return val, false, err

		case xml.EndElement:
			if t.Name.Local == "Objs" {
				return nil, true, nil
			}
		}
	}
}

func (d *Deserializer) deserializeElement(se xml.StartElement) (interface{}, error) {
	// Check recursion depth before processing complex types
	if d.depth >= d.maxDepth {
		return nil, fmt.Errorf("maximum recursion depth exceeded: %d", d.maxDepth)
	}

	switch se.Name.Local {
	case "Nil":
		if err := d.dec.Skip(); err != nil {
			return nil, fmt.Errorf("skip nil: %w", err)
		}
		return nil, nil

	case "S": // String
		var s string
		if err := d.dec.DecodeElement(&s, &se); err != nil {
			return nil, fmt.Errorf("decode string: %w", err)
		}
		return s, nil

	case "I32": // Int32
		var s string
		if err := d.dec.DecodeElement(&s, &se); err != nil {
			return nil, fmt.Errorf("decode int32: %w", err)
		}
		v, err := strconv.ParseInt(s, 10, 32)
		return int32(v), err

	case "I64": // Int64
		var s string
		if err := d.dec.DecodeElement(&s, &se); err != nil {
			return nil, fmt.Errorf("decode int64: %w", err)
		}
		return strconv.ParseInt(s, 10, 64)

	case "B": // Boolean
		var s string
		if err := d.dec.DecodeElement(&s, &se); err != nil {
			return nil, fmt.Errorf("decode bool: %w", err)
		}
		return s == "true", nil

	case "Db": // Double
		var s string
		if err := d.dec.DecodeElement(&s, &se); err != nil {
			return nil, fmt.Errorf("decode double: %w", err)
		}
		return strconv.ParseFloat(s, 64)

	case "BA": // Byte Array
		var s string
		if err := d.dec.DecodeElement(&s, &se); err != nil {
			return nil, fmt.Errorf("decode byte array: %w", err)
		}
		return base64.StdEncoding.DecodeString(s)

	case "G": // GUID
		var s string
		if err := d.dec.DecodeElement(&s, &se); err != nil {
			return nil, fmt.Errorf("decode guid: %w", err)
		}
		return uuid.Parse(s)

	case "DT": // DateTime
		var s string
		if err := d.dec.DecodeElement(&s, &se); err != nil {
			return nil, fmt.Errorf("decode datetime: %w", err)
		}
		return time.Parse(time.RFC3339Nano, s)

	case "LST": // List
		d.depth++
		result, err := d.deserializeList()
		d.depth--
		return result, err

	case "DCT": // Dictionary (raw DCT without Obj wrapper)
		d.depth++
		result, err := d.deserializeDict()
		d.depth--
		return result, err

	case "Obj": // Complex object
		d.depth++
		result, err := d.deserializeObject(se)
		d.depth--
		return result, err

	case "Ref": // Reference to existing object
		return d.deserializeRef(se)

	case "SS": // SecureString
		var s string
		if err := d.dec.DecodeElement(&s, &se); err != nil {
			return nil, fmt.Errorf("decode secure string: %w", err)
		}
		data, err := base64.StdEncoding.DecodeString(s)
		if err != nil {
			return nil, fmt.Errorf("invalid base64 in SecureString: %w", err)
		}

		if d.decryptor != nil {
			// Decrypt with provider
			decrypted, err := d.decryptor.Decrypt(data)
			if err != nil {
				return nil, fmt.Errorf("failed to decrypt SecureString: %w", err)
			}
			// Re-wrap in SecureString (using local protection if needed, or keeping raw if desired)
			// For now, assume d.decryptor returns raw bytes that we want to protect locally?
			// OR assume d.decryptor returns the bytes that should be stored inside SecureString.
			// Actually, objects.NewSecureStringFromEncrypted expects bytes that *are* the internal representation.
			// If d.decryptor returns Plaintext, we should use objects.NewSecureString(string(decrypted)).
			// But for PSRP, the "encrypted" data is usually DPAPI or Session Key encrypted.
			// If we decrypt it, we have plaintext. We should re-protect it locally.
			return objects.NewSecureString(string(decrypted))
		}

		// No provider, assume data is already locally protected or we can't do anything with it
		// Just store it as is
		return objects.NewSecureStringFromEncrypted(data), nil

	case "SB": // ScriptBlock
		var s string
		if err := d.dec.DecodeElement(&s, &se); err != nil {
			return nil, fmt.Errorf("decode script block: %w", err)
		}
		return &objects.ScriptBlock{Text: s}, nil

	case "Version": // Version (e.g. used in capabilities)
		var s string
		if err := d.dec.DecodeElement(&s, &se); err != nil {
			return nil, fmt.Errorf("decode version: %w", err)
		}
		return s, nil

	default:
		// Skip unknown elements
		if err := d.dec.Skip(); err != nil {
			return nil, fmt.Errorf("skip unknown: %w", err)
		}
		return nil, nil
	}
}

func (d *Deserializer) deserializeList() ([]interface{}, error) {
	var result []interface{}

	for {
		tok, err := d.dec.Token()
		if err != nil {
			return nil, err
		}

		switch t := tok.(type) {
		case xml.StartElement:
			val, err := d.deserializeElement(t)
			if err != nil {
				return nil, err
			}
			result = append(result, val)

		case xml.EndElement:
			if t.Name.Local == "LST" {
				return result, nil
			}
		}
	}
}

func (d *Deserializer) deserializeDict() (map[string]interface{}, error) {
	result := make(map[string]interface{})

	for {
		tok, err := d.dec.Token()
		if err != nil {
			return nil, err
		}

		switch t := tok.(type) {
		case xml.StartElement:
			if t.Name.Local == "En" { // Entry
				// Parse key and value from entry
				key, value, err := d.deserializeDictEntry()
				if err != nil {
					return nil, err
				}
				result[key] = value
			} else {
				if err := d.dec.Skip(); err != nil {
					return nil, err
				}
			}

		case xml.EndElement:
			if t.Name.Local == "DCT" {
				return result, nil
			}
		}
	}
}

func (d *Deserializer) deserializeDictEntry() (string, interface{}, error) {
	var key string
	var value interface{}

	for {
		tok, err := d.dec.Token()
		if err != nil {
			return "", nil, err
		}

		switch t := tok.(type) {
		case xml.StartElement:
			// Check for N attribute to determine if this is Key or Value
			var isKey, isValue bool
			for _, attr := range t.Attr {
				if attr.Name.Local == "N" {
					if attr.Value == "Key" {
						isKey = true
					} else if attr.Value == "Value" {
						isValue = true
					}
				}
			}

			val, err := d.deserializeElement(t)
			if err != nil {
				return "", nil, err
			}

			if isKey {
				if s, ok := val.(string); ok {
					key = s
				}
			} else if isValue {
				value = val
			}

		case xml.EndElement:
			if t.Name.Local == "En" {
				return key, value, nil
			}
		}
	}
}

func (d *Deserializer) deserializeObject(se xml.StartElement) (interface{}, error) {
	obj := &PSObject{
		Properties: make(map[string]interface{}),
	}

	// Check for RefId attribute
	var refID int
	hasRefID := false
	for _, attr := range se.Attr {
		if attr.Name.Local == "RefId" {
			id, err := strconv.Atoi(attr.Value)
			if err == nil {
				refID = id
				hasRefID = true
			}
		}
	}

	var typeNames []string
	var hasDict bool
	depth := 0

	for {
		tok, err := d.dec.Token()
		if err != nil {
			return nil, err
		}

		switch t := tok.(type) {
		case xml.StartElement:
			depth++
			switch t.Name.Local {
			case "TN": // TypeNames
				typeNames, err = d.deserializeTypeNames()
				if err != nil {
					return nil, err
				}
				obj.TypeNames = typeNames
				// Store TypeNames in reference map
				if hasRefID {
					for _, attr := range t.Attr {
						if attr.Name.Local == "RefId" {
							tnRefID, err := strconv.Atoi(attr.Value)
							if err == nil {
								d.tnRefs[tnRefID] = typeNames
							}
						}
					}
				}

			case "TNRef": // TypeNames reference
				for _, attr := range t.Attr {
					if attr.Name.Local == "RefId" {
						tnRefID, err := strconv.Atoi(attr.Value)
						if err == nil {
							if tn, exists := d.tnRefs[tnRefID]; exists {
								obj.TypeNames = tn
								typeNames = tn
							}
						}
					}
				}
				if err := d.dec.Skip(); err != nil {
					return nil, err
				}

			case "ToString":
				var s string
				if err := d.dec.DecodeElement(&s, &t); err != nil {
					return nil, err
				}
				obj.ToString = s

			case "Props", "MS": // Properties or Member Set
				props, err := d.deserializeProperties()
				if err != nil {
					return nil, err
				}
				obj.Properties = props

			case "DCT": // Dictionary
				hasDict = true
				dict, err := d.deserializeDict()
				if err != nil {
					return nil, err
				}
				// If this is a Hashtable, return the map directly
				if len(typeNames) > 0 && typeNames[0] == "System.Collections.Hashtable" {
					if hasRefID {
						d.objRefs[refID] = dict
					}
					return dict, nil
				}
				// Store in object
				for k, v := range dict {
					obj.Properties[k] = v
				}

			case "LST": // List
				list, err := d.deserializeList()
				if err != nil {
					return nil, err
				}
				// Return the list directly
				if hasRefID {
					d.objRefs[refID] = list
				}
				return list, nil

			default:
				if err := d.dec.Skip(); err != nil {
					return nil, err
				}
			}

		case xml.EndElement:
			if t.Name.Local == "Obj" {
				// Store object in reference map
				if hasRefID {
					if hasDict && len(typeNames) > 0 && typeNames[0] == "System.Collections.Hashtable" {
						// Already handled above
					} else {
						d.objRefs[refID] = obj
					}
				}
				return obj, nil
			}
			depth--
		}
	}
}

func (d *Deserializer) deserializeTypeNames() ([]string, error) {
	var typeNames []string

	for {
		tok, err := d.dec.Token()
		if err != nil {
			return nil, err
		}

		switch t := tok.(type) {
		case xml.StartElement:
			if t.Name.Local == "T" {
				var tn string
				if err := d.dec.DecodeElement(&tn, &t); err != nil {
					return nil, err
				}
				typeNames = append(typeNames, tn)
			}

		case xml.EndElement:
			if t.Name.Local == "TN" {
				return typeNames, nil
			}
		}
	}
}

func (d *Deserializer) deserializeProperties() (map[string]interface{}, error) {
	props := make(map[string]interface{})

	for {
		tok, err := d.dec.Token()
		if err != nil {
			return nil, err
		}

		switch t := tok.(type) {
		case xml.StartElement:
			// Get property name from N attribute
			var propName string
			for _, attr := range t.Attr {
				if attr.Name.Local == "N" {
					propName = attr.Value
					break
				}
			}

			val, err := d.deserializeElement(t)
			if err != nil {
				return nil, err
			}

			if propName != "" {
				props[propName] = val
			}

		case xml.EndElement:
			if t.Name.Local == "Props" || t.Name.Local == "MS" {
				return props, nil
			}
		}
	}
}

func (d *Deserializer) deserializeRef(se xml.StartElement) (interface{}, error) {
	// Get RefId from attributes
	for _, attr := range se.Attr {
		if attr.Name.Local == "RefId" {
			refID, err := strconv.Atoi(attr.Value)
			if err != nil {
				return nil, err
			}
			if obj, exists := d.objRefs[refID]; exists {
				if err := d.dec.Skip(); err != nil {
					return nil, err
				}
				return obj, nil
			}
			if err := d.dec.Skip(); err != nil {
				return nil, err
			}
			return nil, fmt.Errorf("reference to unknown object: RefId=%d", refID)
		}
	}
	if err := d.dec.Skip(); err != nil {
		return nil, err
	}
	return nil, fmt.Errorf("ref element missing RefId attribute")
}

// PowerShellToPSObject converts a PowerShell object to a PSObject for serialization
// Structure:
// Root Object (Generic wrapper)
//   - Extended Properties (ApartmentState, HostInfo, etc.)
//   - Property "PowerShell" -> Inner Object (Cmds, History, etc.)
func PowerShellToPSObject(p *objects.PowerShell) *PSObject {
	// 1. Create the Inner "PowerShell" object
	cmds := make([]interface{}, len(p.Commands))
	for i, c := range p.Commands {
		cmds[i] = CommandToPSObject(&c)
	}

	// Wrap cmds in TypedList with correct List<PSObject> TypeNames (per pypsrp/PowerShell reference)
	// pypsrp lines 440-447: System.Collections.Generic.List`1[[System.Management.Automation.PSObject, ...]]
	cmdsListTypeName := "System.Collections.Generic.List`1[[System.Management.Automation.PSObject, System.Management.Automation, Version=1.0.0.0, Culture=neutral, PublicKeyToken=31bf3856ad364e35]]"
	cmdsWrapper := &TypedList{
		TypeNames: []string{cmdsListTypeName, "System.Object"},
		Items:     cmds,
	}

	innerPS := &PSObject{
		// PowerShell's CreateEmptyPSObject() omits TypeNames for remoting (EncodeAndDecode.cs:401-403)
		// PowerShell uses Members (MS), not Properties (Props) for remoting objects
		Members: make(map[string]interface{}),
	}
	// MS-PSRP spec section 2.2.2.10 example shows Cmds and IsNested
	// But PS 7.5 also REQUIRES History and RedirectShellErrorOutputPipe!
	// pypsrp sends BOTH as Nil (not empty string or boolean)
	innerPS.Members["Cmds"] = cmdsWrapper
	innerPS.Members["IsNested"] = p.IsNested
	innerPS.Members["History"] = nil                        // Nil per pypsrp
	innerPS.Members["RedirectShellErrorOutputPipe"] = false // PS 7.5 requires Boolean, not Nil
	// ExtraCmds removed (caused Network Error when nil).

	// 2. Create the Root Wrapper object
	// pypsrp uses _extended_properties which serialize to MS (Members), not Props
	// See pypsrp/messages.py:478-486: CreatePipeline uses _extended_properties
	root := &PSObject{
		Members: make(map[string]interface{}), // Members (MS), not Properties (Props)!
	}

	// 3. Add Extended Properties to Root (as Members, per pypsrp)
	root.Members["NoInput"] = true        // pypsrp uses true (not accepting input)
	root.Members["AddToHistory"] = false  // PowerShell default (EncodeAndDecode.cs:896)
	root.Members["IsNested"] = p.IsNested // Only on root wrapper (pypsrp messages.py:485)

	// ApartmentState: MTA (1) - per MS-PSRP official spec section 2.2.2.10 example
	// Type = System.Threading.ApartmentState (NOT Runspaces!)
	root.Members["ApartmentState"] = &PSRPEnum{
		Type:     "System.Threading.ApartmentState", // Per MS-PSRP spec
		Value:    1,                                 // MTA per spec example
		ToString: "Unknown",
	}

	// RemoteStreamOptions: AddInvocationInfo (15) - per MS-PSRP spec
	// Type = System.Management.Automation.RemoteStreamOptions (NO 'Runspaces'!)
	root.Members["RemoteStreamOptions"] = &PSRPEnum{
		Type:     "System.Management.Automation.RemoteStreamOptions", // Per MS-PSRP spec
		Value:    15,
		ToString: "AddInvocationInfo",
	}

	// 4. HostInfo - per pypsrp: when no host, _useRunspaceHost=true and _isHostNull=true
	hostInfoProps := make(map[string]interface{})
	hostInfoProps["_isHostNull"] = true // No host implementation
	hostInfoProps["_isHostUINull"] = true
	hostInfoProps["_isHostRawUINull"] = true
	hostInfoProps["_useRunspaceHost"] = true // Use runspace's host

	// MS-PSRP spec CREATE_PIPELINE example shows NO _hostDefaultData in HostInfo
	// Only the four boolean flags

	// HostInfo also uses Members (MS) per pypsrp Gold Standard - no TypeNames
	root.Members["HostInfo"] = &PSObject{
		Members: hostInfoProps, // Members (MS), not Properties (Props)!
	}

	// 5. Nest the Inner Object
	root.Members["PowerShell"] = innerPS

	return root
}

// CommandToPSObject converts a Command object to a PSObject for serialization
func CommandToPSObject(c *objects.Command) *PSObject {
	params := make([]interface{}, len(c.Parameters))
	for i, p := range c.Parameters {
		params[i] = CommandParameterToPSObject(&p)
	}

	// Command.ToPSObjectForRemoting() uses CreateEmptyPSObject() - no TypeNames
	// Commands use Members (MS), not Properties (Props)
	obj := &PSObject{
		Members: make(map[string]interface{}),
	}

	obj.Members["Cmd"] = c.Name
	// Revert IsScript to boolean false (server requires System.Boolean)
	obj.Members["IsScript"] = c.IsScript
	obj.Members["UseLocalScope"] = nil // Nil, not false (per PowerShell reference)
	// Merge flags - Use PSRPEnum for ALL versions to satisfy strict type requirements
	noneEnum := &PSRPEnum{
		Type:     "System.Management.Automation.Runspaces.PipelineResultTypes",
		Value:    0, // None
		ToString: "None",
	}

	// V2 backwards compatibility properties
	// Spec EXAMPLE (2.2.2.10) uses SINGULAR. Server error "missing MergeMyResult" confirms SINGULAR is required.
	obj.Members["MergeMyResult"] = noneEnum
	obj.Members["MergeToResult"] = noneEnum
	obj.Members["MergePreviousResults"] = noneEnum // Maps to MergeUnclaimedPreviousCommandResults

	// V3+ merge instructions (protocol 2.2+)
	obj.Members["MergeError"] = noneEnum
	obj.Members["MergeWarning"] = noneEnum
	// MergeInformation is NOT in the MS-PSRP spec (2.2.3.12 or example 2.2.2.10)
	// Removing it to match spec strictly. It caused Network Error when present.

	// Wrap Args in TypedList with correct List<PSObject> TypeNames
	argsListTypeName := "System.Collections.Generic.List`1[[System.Management.Automation.PSObject, System.Management.Automation, Version=1.0.0.0, Culture=neutral, PublicKeyToken=31bf3856ad364e35]]"
	argsWrapper := &TypedList{
		TypeNames: []string{argsListTypeName, "System.Object"},
		Items:     params,
	}
	obj.Members["Args"] = argsWrapper

	return obj
}

// CommandParameterToPSObject converts a CommandParameter to a PSObject for serialization
func CommandParameterToPSObject(p *objects.CommandParameter) *PSObject {
	// CommandParameter also uses CreateEmptyPSObject() - no TypeNames
	obj := &PSObject{
		Members: make(map[string]interface{}), // Members (MS), not Properties (Props)!
	}

	// Per pypsrp lines 650-652: use "N" and "V" not "Name" and "Value"
	obj.Members["N"] = p.Name
	obj.Members["V"] = p.Value

	return obj
}
