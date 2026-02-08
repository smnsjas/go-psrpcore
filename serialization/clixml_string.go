package serialization

import (
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

const hexDigits = "0123456789ABCDEF"

// decodeCLIXMLString decodes PowerShell's _xHHHH_ escape sequences in a CLIXML
// string value. PowerShell uses this proprietary encoding for control characters
// and characters above U+FFFF (encoded as UTF-16 surrogate pairs).
//
// Examples:
//
//	"_x000D_"          → "\r"
//	"_x000A_"          → "\n"
//	"_x0000_"          → "\x00"
//	"_xD834__xDD1E_"   → "𝄞" (U+1D11E, surrogate pair)
//	"_x005F_x000D_"    → "_x000D_" (escaped underscore, literal text)
func decodeCLIXMLString(s string) string {
	// Fast path: if there's no _x or _X pattern, nothing to decode.
	if !containsCLIXMLEscape(s) {
		return s
	}

	var b strings.Builder
	b.Grow(len(s))

	i := 0
	for i < len(s) {
		// Check for _xHHHH_ pattern (need at least 7 chars remaining).
		if i+7 <= len(s) && s[i] == '_' && (s[i+1] == 'x' || s[i+1] == 'X') &&
			isHexDigit(s[i+2]) && isHexDigit(s[i+3]) &&
			isHexDigit(s[i+4]) && isHexDigit(s[i+5]) && s[i+6] == '_' {

			codeUnit := parseHex4(s[i+2 : i+6])

			// Check for UTF-16 surrogate pair: high surrogate followed by low surrogate.
			if codeUnit >= 0xD800 && codeUnit <= 0xDBFF && i+14 <= len(s) {
				if s[i+7] == '_' && (s[i+8] == 'x' || s[i+8] == 'X') &&
					isHexDigit(s[i+9]) && isHexDigit(s[i+10]) &&
					isHexDigit(s[i+11]) && isHexDigit(s[i+12]) && s[i+13] == '_' {

					low := parseHex4(s[i+9 : i+13])
					if low >= 0xDC00 && low <= 0xDFFF {
						r := utf16.DecodeRune(rune(codeUnit), rune(low))
						b.WriteRune(r)
						i += 14
						continue
					}
				}
			}

			b.WriteRune(rune(codeUnit))
			i += 7
			continue
		}

		b.WriteByte(s[i])
		i++
	}

	return b.String()
}

// encodeCLIXMLString encodes a Go string into PowerShell's CLIXML string format.
// Control characters (U+0000–U+001F, U+007F–U+009F) are encoded as _xHHHH_.
// Characters above U+FFFF are encoded as UTF-16 surrogate pairs (_xHHHH__xHHHH_).
// Underscores followed by 'x' or 'X' are escaped as _x005F_ to prevent ambiguity.
//
// This encoding is applied BEFORE XML escaping. The resulting string will only
// contain printable characters (plus standard XML-special chars like <, >, &).
func encodeCLIXMLString(s string) string {
	if !needsCLIXMLEncoding(s) {
		return s
	}

	var b strings.Builder
	b.Grow(len(s) + len(s)/4)

	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])

		// Escape underscore before 'x'/'X' to prevent ambiguity with _xHHHH_ sequences.
		// Must happen before control char encoding (conceptually "step 1").
		if r == '_' && i+size < len(s) && (s[i+size] == 'x' || s[i+size] == 'X') {
			b.WriteString("_x005F_")
			i += size
			continue
		}

		if needsCLIXMLCharEscape(r) {
			if r > 0xFFFF {
				// Supplementary plane character: encode as UTF-16 surrogate pair.
				high, low := utf16.EncodeRune(r)
				writeHex4(&b, uint16(high))
				writeHex4(&b, uint16(low))
			} else {
				writeHex4(&b, uint16(r))
			}
		} else {
			b.WriteRune(r)
		}
		i += size
	}

	return b.String()
}

// needsCLIXMLEncoding reports whether the string contains any characters that
// require CLIXML _xHHHH_ encoding: control characters, supplementary plane
// characters, or underscore-x patterns that need escape-escaping.
func needsCLIXMLEncoding(s string) bool {
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		if needsCLIXMLCharEscape(r) {
			return true
		}
		// Check for _x or _X that needs escape-escaping.
		if r == '_' && i+size < len(s) && (s[i+size] == 'x' || s[i+size] == 'X') {
			return true
		}
		i += size
	}
	return false
}

// needsCLIXMLCharEscape reports whether a rune requires _xHHHH_ encoding.
func needsCLIXMLCharEscape(r rune) bool {
	return r <= 0x1F || // C0 control characters (null, tab, CR, LF, etc.)
		(r >= 0x7F && r <= 0x9F) || // DEL + C1 control characters
		(r >= 0xD800 && r <= 0xDFFF) || // Surrogates (shouldn't appear in valid Go strings)
		r > 0xFFFF // Supplementary plane (emoji, musical symbols, etc.)
}

// containsCLIXMLEscape reports whether the string might contain _xHHHH_ sequences.
func containsCLIXMLEscape(s string) bool {
	for i := 0; i < len(s)-1; i++ {
		if s[i] == '_' && (s[i+1] == 'x' || s[i+1] == 'X') {
			return true
		}
	}
	return false
}

// parseHex4 parses a 4-character hex string into a uint16.
// Caller must ensure s is exactly 4 valid hex digits.
func parseHex4(s string) uint16 {
	var val uint16
	for _, c := range s {
		val <<= 4
		switch {
		case c >= '0' && c <= '9':
			val |= uint16(c - '0')
		case c >= 'a' && c <= 'f':
			val |= uint16(c - 'a' + 10)
		case c >= 'A' && c <= 'F':
			val |= uint16(c - 'A' + 10)
		}
	}
	return val
}

// writeHex4 writes a _xHHHH_ escape sequence for a 16-bit code unit directly
// to the builder without any heap allocations (unlike fmt.Fprintf).
func writeHex4(b *strings.Builder, v uint16) {
	var buf [7]byte
	buf[0] = '_'
	buf[1] = 'x'
	buf[2] = hexDigits[v>>12&0xF]
	buf[3] = hexDigits[v>>8&0xF]
	buf[4] = hexDigits[v>>4&0xF]
	buf[5] = hexDigits[v&0xF]
	buf[6] = '_'
	b.Write(buf[:])
}

// isHexDigit reports whether a byte is a valid hexadecimal digit.
func isHexDigit(b byte) bool {
	return (b >= '0' && b <= '9') || (b >= 'a' && b <= 'f') || (b >= 'A' && b <= 'F')
}
