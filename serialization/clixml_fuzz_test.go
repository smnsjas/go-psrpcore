package serialization

import (
	"testing"
	"unicode/utf8"
)

func FuzzDeserializer(f *testing.F) {
	// Add initial corpus of valid XML
	f.Add([]byte(`<Objs Version="1.1.0.1" xmlns="http://schemas.microsoft.com/powershell/2004/04"><S>test</S></Objs>`))
	f.Add([]byte(`<Objs Version="1.1.0.1" xmlns="http://schemas.microsoft.com/powershell/2004/04"><I32>123</I32></Objs>`))
	f.Add([]byte("garbage data"))

	f.Fuzz(func(_ *testing.T, data []byte) {
		d := NewDeserializer()
		// We expect errors for garbage input, but we must NOT panic.
		// The deserializer should handle invalid XML, deep nesting, etc. safely.
		_, _ = d.Deserialize(data)
	})
}

func FuzzRoundTripString(f *testing.F) {
	f.Add("hello world")
	f.Add("")
	f.Add("<xml>stuff</xml>") // Special chars
	f.Add("!@#$%^&*()")
	f.Add("line1\r\nline2")       // Control chars (now CLIXML-encoded)
	f.Add("\x00null\x01byte")     // C0 control chars
	f.Add("_x000D_ literal")     // Underscore-x escape-escaping
	f.Add("\U0001F600 emoji")     // Supplementary plane character

	f.Fuzz(func(t *testing.T, s string) {
		if !utf8.ValidString(s) {
			return
		}
		// With CLIXML encoding, control characters are encoded as _xHHHH_ before
		// being placed in XML, so most characters now round-trip correctly.
		// Only exclude U+FFFE and U+FFFF which are XML noncharacters that our
		// CLIXML encoder doesn't handle (they're in the BMP, not control chars).
		for _, r := range s {
			if r == 0xFFFE || r == 0xFFFF {
				return
			}
		}

		ser := NewSerializer()
		data, err := ser.Serialize(s)
		if err != nil {
			// Serialization of string should generally not fail
			t.Fatalf("Serialize failed: %v", err)
		}

		des := NewDeserializer()
		results, err := des.Deserialize(data)
		if err != nil {
			t.Fatalf("Deserialize failed: %v", err)
		}

		if len(results) != 1 {
			t.Fatalf("expected 1 result, got %d", len(results))
		}

		got, ok := results[0].(string)
		if !ok {
			t.Fatalf("expected string, got %T", results[0])
		}

		if got != s {
			t.Errorf("RoundTrip mismatch: got %q, want %q", got, s)
		}
	})
}

func FuzzRoundTripInt(f *testing.F) {
	f.Add(int32(0))
	f.Add(int32(12345))
	f.Add(int32(-1))

	f.Fuzz(func(t *testing.T, i int32) {
		ser := NewSerializer()
		data, err := ser.Serialize(i)
		if err != nil {
			t.Fatalf("Serialize failed: %v", err)
		}

		des := NewDeserializer()
		results, err := des.Deserialize(data)
		if err != nil {
			t.Fatalf("Deserialize failed: %v", err)
		}

		if len(results) != 1 {
			t.Fatalf("expected 1 result, got %d", len(results))
		}

		got, ok := results[0].(int32)
		if !ok {
			t.Fatalf("expected int32, got %T", results[0])
		}

		if got != i {
			t.Errorf("RoundTrip mismatch: got %v, want %v", got, i)
		}
	})
}
