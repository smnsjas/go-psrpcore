package serialization

import (
	"strings"
	"testing"
)

func TestDecodeCLIXMLString(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"no escapes", "hello world", "hello world"},
		{"empty string", "", ""},
		{"CR", "_x000D_", "\r"},
		{"LF", "_x000A_", "\n"},
		{"CRLF", "_x000D__x000A_", "\r\n"},
		{"tab", "_x0009_", "\t"},
		{"null", "_x0000_", "\x00"},
		{"mixed text and escapes", "line1_x000D__x000A_line2", "line1\r\nline2"},
		{"escape at start", "_x000A_hello", "\nhello"},
		{"escape at end", "hello_x000A_", "hello\n"},
		{"escaped underscore", "_x005F_x000D_", "_x000D_"},
		{"double escaped underscore", "_x005F_x005F_", "_x005F_"},
		{"escaped underscore then real escape", "_x005F_x000A__x000D_", "_x000A_\r"},
		{"uppercase hex", "_x000D_", "\r"},
		{"lowercase hex", "_x000d_", "\r"},
		{"mixed case hex", "_x000D_", "\r"},
		{"uppercase X", "_X000D_", "\r"},
		{"DEL char", "_x007F_", "\x7f"},
		{"C1 control char", "_x0085_", "\u0085"},
		{"last C1 char", "_x009F_", "\u009F"},
		{"invalid hex not decoded", "_x00GG_", "_x00GG_"},
		{"incomplete pattern", "_x000", "_x000"},
		{"partial pattern no closing", "_x000D", "_x000D"},
		{"just underscore", "_", "_"},
		{"underscore x without hex", "_xZZZZ_", "_xZZZZ_"},
		// Surrogate pairs
		{"surrogate pair treble clef", "_xD834__xDD1E_", "\U0001D11E"},
		{"surrogate pair emoji", "_xD83D__xDE00_", "\U0001F600"},
		// Lone surrogate (edge case — technically invalid but should not crash)
		{"lone high surrogate", "_xD800_", string(rune(0xD800))},
		{"lone low surrogate", "_xDC00_", string(rune(0xDC00))},
		// Multiple escapes in a row
		{"three escapes", "_x0041__x0042__x0043_", "ABC"},
		// Normal underscore not before x
		{"underscore not before x", "a_b_c", "a_b_c"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := decodeCLIXMLString(tt.input)
			if got != tt.expected {
				t.Errorf("decodeCLIXMLString(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestEncodeCLIXMLString(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"no encoding needed", "hello world", "hello world"},
		{"empty string", "", ""},
		{"CR", "\r", "_x000D_"},
		{"LF", "\n", "_x000A_"},
		{"CRLF", "\r\n", "_x000D__x000A_"},
		{"tab", "\t", "_x0009_"},
		{"null", "\x00", "_x0000_"},
		{"mixed text and control chars", "line1\r\nline2", "line1_x000D__x000A_line2"},
		{"DEL", "\x7f", "_x007F_"},
		{"C1 NEL", "\u0085", "_x0085_"},
		{"last C1", "\u009F", "_x009F_"},
		// Escape-escaping: underscore before x/X
		{"underscore x pattern", "_x000D_", "_x005F_x000D_"},
		{"underscore X pattern", "_X000D_", "_x005F_X000D_"},
		{"bare underscore", "a_b", "a_b"},
		{"underscore not before x", "a_1", "a_1"},
		// Supplementary plane characters → surrogate pair encoding
		{"treble clef", "\U0001D11E", "_xD834__xDD1E_"},
		{"emoji grinning face", "\U0001F600", "_xD83D__xDE00_"},
		// Normal printable ASCII and non-ASCII above C1
		{"printable ASCII", "Hello, World! 123", "Hello, World! 123"},
		{"non-ASCII above C1", "caf\u00e9", "caf\u00e9"},
		{"unicode CJK", "\u4e16\u754c", "\u4e16\u754c"},
		// XML-special chars should pass through (XML escaping is separate)
		{"XML special chars", "<>&", "<>&"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := encodeCLIXMLString(tt.input)
			if got != tt.expected {
				t.Errorf("encodeCLIXMLString(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestCLIXMLStringRoundTrip(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
	}{
		{"plain text", "hello world"},
		{"empty", ""},
		{"control chars", "line1\r\nline2\ttab"},
		{"null byte", "before\x00after"},
		{"underscore x literal", "literal _x000D_ text"},
		{"underscore X literal", "literal _X000D_ text"},
		{"double underscore x", "_x005F_x000D_"},
		{"mixed escapes and text", "start_x000A_\r\nmiddle_xABCD_end"},
		{"treble clef emoji", "music: \U0001D11E"},
		{"emoji", "hello \U0001F600 world"},
		{"complex mix", "treble clef\n _x0000_ _X0000_ \U0001D11E caf\u00e9"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoded := encodeCLIXMLString(tt.input)
			decoded := decodeCLIXMLString(encoded)
			if decoded != tt.input {
				t.Errorf("round-trip failed:\n  input:   %q\n  encoded: %q\n  decoded: %q", tt.input, encoded, decoded)
			}
		})
	}
}

// TestCLIXMLStringComplexRoundTrip is modeled after psrpcore's COMPLEX_STRING test.
func TestCLIXMLStringComplexRoundTrip(t *testing.T) {
	t.Parallel()

	// This matches the Python psrpcore test case
	input := "treble clef\n _x0000_ _X0000_ \U0001D11E caf\u00e9 _x001G_"
	expectedEncoded := "treble clef_x000A_ _x005F_x0000_ _x005F_X0000_ _xD834__xDD1E_ caf\u00e9 _x005F_x001G_"

	encoded := encodeCLIXMLString(input)
	if encoded != expectedEncoded {
		t.Errorf("encode mismatch:\n  got:  %q\n  want: %q", encoded, expectedEncoded)
	}

	decoded := decodeCLIXMLString(encoded)
	if decoded != input {
		t.Errorf("round-trip failed:\n  input:   %q\n  decoded: %q", input, decoded)
	}
}

// TestSerializeCLIXMLStringIntegration tests the full serialize→deserialize path.
func TestSerializeCLIXMLStringIntegration(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
	}{
		{"plain text", "hello"},
		{"with CRLF", "line1\r\nline2"},
		{"with tab", "col1\tcol2"},
		{"with null", "before\x00after"},
		{"literal escape pattern", "_x000D_ literal"},
		{"XML special chars", "<tag>&amp;"},
		{"mixed everything", "line1\r\n<b>bold</b>\t_x000A_ end"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewSerializer()
			data, err := s.Serialize(tt.input)
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

			got, ok := results[0].(string)
			if !ok {
				t.Fatalf("expected string, got %T", results[0])
			}

			if got != tt.input {
				t.Errorf("round-trip mismatch:\n  input: %q\n  got:   %q", tt.input, got)
			}
		})
	}
}

// TestSerializeCLIXMLStringOutput verifies the serialized XML content.
func TestSerializeCLIXMLStringOutput(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		input    string
		contains string
	}{
		{"CR encoded as CLIXML", "\r", "_x000D_"},
		{"LF encoded as CLIXML", "\n", "_x000A_"},
		{"tab encoded as CLIXML", "\t", "_x0009_"},
		{"underscore x escaped", "_xABCD_", "_x005F_xABCD_"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewSerializer()
			data, err := s.Serialize(tt.input)
			if err != nil {
				t.Fatalf("Serialize failed: %v", err)
			}

			result := string(data)
			if !strings.Contains(result, tt.contains) {
				t.Errorf("expected serialized output to contain %q, got:\n%s", tt.contains, result)
			}
		})
	}
}

// TestDeserializeCLIXMLFromPowerShell tests decoding strings as PowerShell would send them.
func TestDeserializeCLIXMLFromPowerShell(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		clixml   string
		expected string
	}{
		{
			name:     "CRLF in string",
			clixml:   `<Objs Version="1.1.0.1" xmlns="http://schemas.microsoft.com/powershell/2004/04"><S>line1_x000D__x000A_line2</S></Objs>`,
			expected: "line1\r\nline2",
		},
		{
			name:     "tab in string",
			clixml:   `<Objs Version="1.1.0.1" xmlns="http://schemas.microsoft.com/powershell/2004/04"><S>col1_x0009_col2</S></Objs>`,
			expected: "col1\tcol2",
		},
		{
			name:     "escaped underscore preserves literal",
			clixml:   `<Objs Version="1.1.0.1" xmlns="http://schemas.microsoft.com/powershell/2004/04"><S>_x005F_x000D_</S></Objs>`,
			expected: "_x000D_",
		},
		{
			name:     "surrogate pair",
			clixml:   `<Objs Version="1.1.0.1" xmlns="http://schemas.microsoft.com/powershell/2004/04"><S>_xD834__xDD1E_</S></Objs>`,
			expected: "\U0001D11E",
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

			got, ok := results[0].(string)
			if !ok {
				t.Fatalf("expected string, got %T", results[0])
			}

			if got != tt.expected {
				t.Errorf("got %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestNeedsCLIXMLCharEscape(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		r      rune
		expect bool
	}{
		{"null", 0x00, true},
		{"tab", 0x09, true},
		{"LF", 0x0A, true},
		{"CR", 0x0D, true},
		{"last C0", 0x1F, true},
		{"space", 0x20, false},
		{"printable A", 'A', false},
		{"tilde", '~', false},
		{"DEL", 0x7F, true},
		{"first C1", 0x80, true},
		{"NEL", 0x85, true},
		{"last C1", 0x9F, true},
		{"non-break space", 0xA0, false},
		{"BMP CJK", 0x4E16, false},
		{"last BMP", 0xFFFF, false},
		{"first supplementary", 0x10000, true},
		{"emoji", 0x1F600, true},
		{"treble clef", 0x1D11E, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := needsCLIXMLCharEscape(tt.r)
			if got != tt.expect {
				t.Errorf("needsCLIXMLCharEscape(%U) = %v, want %v", tt.r, got, tt.expect)
			}
		})
	}
}

func BenchmarkDecodeCLIXMLString(b *testing.B) {
	b.Run("no escapes", func(b *testing.B) {
		s := "hello world this is a normal string with no escapes"
		for b.Loop() {
			decodeCLIXMLString(s)
		}
	})
	b.Run("with escapes", func(b *testing.B) {
		s := "line1_x000D__x000A_line2_x0009_col2_x000D__x000A_line3"
		for b.Loop() {
			decodeCLIXMLString(s)
		}
	})
	b.Run("surrogate pair", func(b *testing.B) {
		s := "music _xD834__xDD1E_ and _xD83D__xDE00_ emoji"
		for b.Loop() {
			decodeCLIXMLString(s)
		}
	})
}

func BenchmarkEncodeCLIXMLString(b *testing.B) {
	b.Run("no encoding needed", func(b *testing.B) {
		s := "hello world this is a normal string with no encoding needed"
		for b.Loop() {
			encodeCLIXMLString(s)
		}
	})
	b.Run("with control chars", func(b *testing.B) {
		s := "line1\r\nline2\tcol2\r\nline3"
		for b.Loop() {
			encodeCLIXMLString(s)
		}
	})
	b.Run("escape escape pattern", func(b *testing.B) {
		s := "literal _x000D_ and _X000A_ patterns"
		for b.Loop() {
			encodeCLIXMLString(s)
		}
	})
}
