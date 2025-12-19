package serialization

import (
	"testing"
)

func BenchmarkSerializePSObject(b *testing.B) {
	obj := &PSObject{
		TypeNames: []string{"System.String", "System.Object"},
		Properties: map[string]interface{}{
			"foo": "bar",
			"baz": 123,
			"qux": true,
		},
		ToString: "test object",
	}

	ser := NewSerializer()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, err := ser.Serialize(obj)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkDeserializePSObject(b *testing.B) {
	obj := &PSObject{
		TypeNames: []string{"System.String", "System.Object"},
		Properties: map[string]interface{}{
			"foo": "bar",
			"baz": int32(123),
			"qux": true,
		},
		ToString: "test object",
	}
	ser := NewSerializer()
	data, err := ser.Serialize(obj)
	if err != nil {
		b.Fatal(err)
	}

	deser := NewDeserializer()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, err := deser.Deserialize(data)
		if err != nil {
			b.Fatal(err)
		}
	}
}
