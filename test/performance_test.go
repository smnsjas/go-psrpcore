package psrp_test

import (
	"bytes"
	"fmt"
	"io"
	"testing"

	"github.com/google/uuid"
	"github.com/smnsjas/go-psrpcore/fragments"
	"github.com/smnsjas/go-psrpcore/messages"
	"github.com/smnsjas/go-psrpcore/outofproc"
	"github.com/smnsjas/go-psrpcore/serialization"
)

// Baseline Benchmark Suite
// Run with: go test -bench=. -benchmem -count=5 -run=^$ > baseline.txt
// Compare: benchstat baseline.txt optimized.txt

// =============================================================================
// Serialization Benchmarks
// =============================================================================

func BenchmarkSerializeSmallObject(b *testing.B) {
	obj := &serialization.PSObject{
		TypeNames: []string{"System.String"},
		Properties: map[string]interface{}{
			"Value": "test",
		},
	}

	ser := serialization.NewSerializer()
	defer ser.Close()

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, err := ser.Serialize(obj)
		if err != nil {
			b.Fatal(err)
		}
		ser.Reset()
	}
}

func BenchmarkSerializeMediumObject(b *testing.B) {
	obj := &serialization.PSObject{
		TypeNames:  []string{"System.Management.Automation.PSCustomObject"},
		Properties: make(map[string]interface{}),
	}
	for i := 0; i < 10; i++ {
		obj.Properties[fmt.Sprintf("Property%d", i)] = fmt.Sprintf("Value%d", i)
	}

	ser := serialization.NewSerializer()
	defer ser.Close()

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, err := ser.Serialize(obj)
		if err != nil {
			b.Fatal(err)
		}
		ser.Reset()
	}
}

func BenchmarkSerializeLargeObject(b *testing.B) {
	obj := &serialization.PSObject{
		TypeNames:  []string{"System.Management.Automation.PSCustomObject"},
		Properties: make(map[string]interface{}),
	}
	for i := 0; i < 50; i++ {
		obj.Properties[fmt.Sprintf("Property%d", i)] = fmt.Sprintf("Value%d", i)
	}

	ser := serialization.NewSerializer()
	defer ser.Close()

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, err := ser.Serialize(obj)
		if err != nil {
			b.Fatal(err)
		}
		ser.Reset()
	}
}

func BenchmarkSerializeWithReferences(b *testing.B) {
	// Create object graph with references
	inner := &serialization.PSObject{
		TypeNames:  []string{"InnerObject"},
		Properties: map[string]interface{}{"Inner": "value"},
	}
	outer := &serialization.PSObject{
		TypeNames: []string{"OuterObject"},
		Properties: map[string]interface{}{
			"Ref1": inner,
			"Ref2": inner, // Same reference
			"Ref3": inner,
		},
	}

	ser := serialization.NewSerializer()
	defer ser.Close()

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, err := ser.Serialize(outer)
		if err != nil {
			b.Fatal(err)
		}
		ser.Reset()
	}
}

func BenchmarkSerializeString(b *testing.B) {
	ser := serialization.NewSerializer()
	defer ser.Close()

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, err := ser.SerializeRaw("test string value")
		if err != nil {
			b.Fatal(err)
		}
		ser.Reset()
	}
}

func BenchmarkSerializeInt32(b *testing.B) {
	ser := serialization.NewSerializer()
	defer ser.Close()

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, err := ser.SerializeRaw(int32(12345))
		if err != nil {
			b.Fatal(err)
		}
		ser.Reset()
	}
}

func BenchmarkSerializeHashtable(b *testing.B) {
	hashtable := map[string]interface{}{
		"Key1": "Value1",
		"Key2": int32(123),
		"Key3": true,
		"Key4": []byte("data"),
	}

	ser := serialization.NewSerializer()
	defer ser.Close()

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, err := ser.SerializeRaw(hashtable)
		if err != nil {
			b.Fatal(err)
		}
		ser.Reset()
	}
}

func BenchmarkDeserializeSmallObject(b *testing.B) {
	obj := &serialization.PSObject{
		TypeNames: []string{"System.String"},
		Properties: map[string]interface{}{
			"Value": "test",
		},
	}
	ser := serialization.NewSerializer()
	data, err := ser.Serialize(obj)
	ser.Close()
	if err != nil {
		b.Fatal(err)
	}

	deser := serialization.NewDeserializer()
	defer deser.Close()

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, err := deser.Deserialize(data)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// =============================================================================
// Fragment Benchmarks
// =============================================================================

func BenchmarkFragmentEncode(b *testing.B) {
	frag := &fragments.Fragment{
		ObjectID:   1,
		FragmentID: 0,
		Start:      true,
		End:        true,
		Data:       make([]byte, 1024),
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = frag.Encode()
	}
}

func BenchmarkFragmentDecode(b *testing.B) {
	frag := &fragments.Fragment{
		ObjectID:   1,
		FragmentID: 0,
		Start:      true,
		End:        true,
		Data:       make([]byte, 1024),
	}
	encoded := frag.Encode()

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		f, err := fragments.Decode(encoded)
		if err != nil {
			b.Fatal(err)
		}
		f.Release()
	}
}

func BenchmarkFragmentEncodeLarge(b *testing.B) {
	frag := &fragments.Fragment{
		ObjectID:   1,
		FragmentID: 0,
		Start:      true,
		End:        true,
		Data:       make([]byte, 32768), // 32KB
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = frag.Encode()
	}
}

func BenchmarkFragmentAssemble(b *testing.B) {
	// Create multiple fragments
	fragmenter := fragments.NewFragmenter(1024)
	data := make([]byte, 4096) // 4KB data = 4 fragments
	frags, err := fragmenter.Fragment(data)
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		assembler := fragments.NewAssembler()
		for _, frag := range frags {
			complete, _, err := assembler.Add(frag)
			if err != nil {
				b.Fatal(err)
			}
			if complete && frag != frags[len(frags)-1] {
				b.Fatal("completed too early")
			}
		}
	}
}

// =============================================================================
// Message Benchmarks
// =============================================================================

func BenchmarkMessageEncode(b *testing.B) {
	msg := &messages.Message{
		Destination: messages.DestinationServer,
		Type:        messages.MessageTypeCreatePipeline,
		RunspaceID:  uuid.New(),
		PipelineID:  uuid.New(),
		Data:        make([]byte, 1024),
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, err := msg.Encode()
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkMessageDecode(b *testing.B) {
	msg := &messages.Message{
		Destination: messages.DestinationServer,
		Type:        messages.MessageTypeCreatePipeline,
		RunspaceID:  uuid.New(),
		PipelineID:  uuid.New(),
		Data:        make([]byte, 1024),
	}
	encoded, err := msg.Encode()
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, err := messages.Decode(encoded)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// =============================================================================
// Transport Benchmarks
// =============================================================================

func BenchmarkTransportSendData(b *testing.B) {
	var buf bytes.Buffer
	transport := outofproc.NewTransport(&buf, &buf)
	data := make([]byte, 4096)
	guid := uuid.New()

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		buf.Reset()
		err := transport.SendData(guid, data)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkTransportSendDataSmall(b *testing.B) {
	var buf bytes.Buffer
	transport := outofproc.NewTransport(&buf, &buf)
	data := make([]byte, 256)
	guid := uuid.New()

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		buf.Reset()
		err := transport.SendData(guid, data)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkTransportSendCommand(b *testing.B) {
	var buf bytes.Buffer
	transport := outofproc.NewTransport(&buf, &buf)
	guid := uuid.New()

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		buf.Reset()
		err := transport.SendCommand(guid)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkTransportReceivePacket(b *testing.B) {
	// Pre-generate packet lines
	lines := make([]string, b.N)
	for i := 0; i < b.N; i++ {
		lines[i] = "<Data Stream='Default' PSGuid='00000000-0000-0000-0000-000000000000'>dGVzdGRhdGE=</Data>\n"
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		buf := bytes.NewBufferString(lines[i])
		transport := outofproc.NewTransport(buf, io.Discard)
		_, err := transport.ReceivePacket()
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkTransportReceiveCommand(b *testing.B) {
	lines := make([]string, b.N)
	for i := 0; i < b.N; i++ {
		lines[i] = "<Command PSGuid='12345678-1234-1234-1234-123456789012' />\n"
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		buf := bytes.NewBufferString(lines[i])
		transport := outofproc.NewTransport(buf, io.Discard)
		_, err := transport.ReceivePacket()
		if err != nil {
			b.Fatal(err)
		}
	}
}

// =============================================================================
// End-to-End Benchmarks
// =============================================================================

func BenchmarkRoundTripSmall(b *testing.B) {
	// Small object: serialize -> fragment -> message -> decode
	obj := &serialization.PSObject{
		TypeNames:  []string{"System.String"},
		Properties: map[string]interface{}{"Value": "test"},
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		// Serialize
		ser := serialization.NewSerializer()
		data, err := ser.SerializeRaw(obj)
		if err != nil {
			b.Fatal(err)
		}
		ser.Close()

		// Create message
		msg := messages.NewCreatePipeline(uuid.New(), uuid.New(), data)
		msgData, err := msg.Encode()
		if err != nil {
			b.Fatal(err)
		}

		// Fragment
		fragmenter := fragments.NewFragmenter(4096)
		frags, err := fragmenter.Fragment(msgData)
		if err != nil {
			b.Fatal(err)
		}

		// Encode fragments
		for _, frag := range frags {
			_ = frag.Encode()
		}

		// Decode fragments
		assembler := fragments.NewAssembler()
		for _, frag := range frags {
			_, _, err := assembler.Add(frag)
			if err != nil {
				b.Fatal(err)
			}
		}

		// Decode message
		_, err = messages.Decode(msgData)
		if err != nil {
			b.Fatal(err)
		}

		// Deserialize
		deser := serialization.NewDeserializer()
		_, err = deser.Deserialize(data)
		if err != nil {
			b.Fatal(err)
		}
		deser.Close()
	}
}

func BenchmarkRoundTripMedium(b *testing.B) {
	// Medium object with multiple properties
	obj := &serialization.PSObject{
		TypeNames:  []string{"System.Management.Automation.PSCustomObject"},
		Properties: make(map[string]interface{}),
	}
	for i := 0; i < 20; i++ {
		obj.Properties[fmt.Sprintf("Prop%d", i)] = fmt.Sprintf("Value%d", i)
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		ser := serialization.NewSerializer()
		data, err := ser.SerializeRaw(obj)
		if err != nil {
			b.Fatal(err)
		}
		ser.Close()

		msg := messages.NewCreatePipeline(uuid.New(), uuid.New(), data)
		msgData, err := msg.Encode()
		if err != nil {
			b.Fatal(err)
		}

		fragmenter := fragments.NewFragmenter(4096)
		frags, err := fragmenter.Fragment(msgData)
		if err != nil {
			b.Fatal(err)
		}

		for _, frag := range frags {
			_ = frag.Encode()
		}

		assembler := fragments.NewAssembler()
		for _, frag := range frags {
			_, _, err := assembler.Add(frag)
			if err != nil {
				b.Fatal(err)
			}
		}

		_, err = messages.Decode(msgData)
		if err != nil {
			b.Fatal(err)
		}

		deser := serialization.NewDeserializer()
		_, err = deser.Deserialize(data)
		if err != nil {
			b.Fatal(err)
		}
		deser.Close()
	}
}

func BenchmarkTransportRoundTrip(b *testing.B) {
	// Test outofproc send -> receive cycle
	guid := uuid.New()
	data := make([]byte, 1024)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var buf bytes.Buffer
		transport := outofproc.NewTransport(&buf, &buf)

		// Send
		err := transport.SendData(guid, data)
		if err != nil {
			b.Fatal(err)
		}

		// Receive
		_, err = transport.ReceivePacket()
		if err != nil {
			b.Fatal(err)
		}
	}
}
