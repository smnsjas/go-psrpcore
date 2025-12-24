package fragments

import (
	"bytes"
	"testing"
)

// FuzzDecode tests the fragment decoder with random input.
// The decoder should handle malformed data gracefully without panicking.
func FuzzDecode(f *testing.F) {
	// Add seed corpus with valid fragments
	validFrag := &Fragment{
		ObjectID:   1,
		FragmentID: 0,
		Start:      true,
		End:        true,
		Data:       []byte("test data"),
	}
	f.Add(validFrag.Encode())

	// Add edge cases
	f.Add(make([]byte, HeaderSize))   // Minimal valid header size
	f.Add(make([]byte, HeaderSize-1)) // Too short
	f.Add([]byte{})                   // Empty
	f.Add([]byte{0xFF, 0xFF, 0xFF})   // Random garbage

	f.Fuzz(func(_ *testing.T, data []byte) {
		// The decoder must not panic on any input
		_, _ = Decode(data)
	})
}

// FuzzFragmentRoundTrip tests that fragmentation and reassembly
// produces the original data for any valid input.
func FuzzFragmentRoundTrip(f *testing.F) {
	// Add seed corpus
	f.Add([]byte("hello world"))
	f.Add([]byte(""))
	f.Add([]byte("a"))
	f.Add(make([]byte, 1000)) // Larger data

	f.Fuzz(func(t *testing.T, data []byte) {
		// Fragment the data
		fragmenter := NewFragmenter(HeaderSize + 50) // Small fragments
		frags, err := fragmenter.Fragment(data)
		if err != nil {
			t.Fatalf("Fragment failed: %v", err)
		}

		// Verify fragment structure
		if len(frags) == 0 {
			t.Fatal("expected at least one fragment")
		}
		if !frags[0].Start {
			t.Error("first fragment should have Start flag")
		}
		if !frags[len(frags)-1].End {
			t.Error("last fragment should have End flag")
		}

		// Reassemble
		assembler := NewAssembler()
		var complete bool
		var result []byte
		for _, frag := range frags {
			var err error
			complete, result, err = assembler.Add(frag)
			if err != nil {
				t.Fatalf("Assembler.Add failed: %v", err)
			}
		}

		if !complete {
			t.Fatal("expected complete message after all fragments")
		}

		// Verify round-trip
		if !bytes.Equal(result, data) {
			t.Errorf("round-trip mismatch:\ngot:  %v\nwant: %v", result, data)
		}
	})
}

// FuzzAssemblerAdd tests the assembler with random fragment data.
// The assembler should handle malformed fragments gracefully.
func FuzzAssemblerAdd(f *testing.F) {
	// Add seed with valid encoded fragment
	validFrag := &Fragment{
		ObjectID:   1,
		FragmentID: 0,
		Start:      true,
		End:        true,
		Data:       []byte("test"),
	}
	f.Add(validFrag.Encode())

	f.Fuzz(func(_ *testing.T, data []byte) {
		// Decode the fragment (may fail - that's OK)
		frag, err := Decode(data)
		if err != nil {
			return // Invalid fragment, skip
		}

		// The assembler must not panic
		assembler := NewAssembler()
		_, _, _ = assembler.Add(frag)
	})
}
