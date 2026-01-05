package fragments

import (
	"bytes"
	"testing"
)

func TestFragmentEncodeDecode(t *testing.T) {
	tests := []struct {
		name string
		frag *Fragment
	}{
		{
			name: "single fragment",
			frag: &Fragment{
				ObjectID:   1,
				FragmentID: 0,
				Start:      true,
				End:        true,
				Data:       []byte("hello world"),
			},
		},
		{
			name: "start fragment",
			frag: &Fragment{
				ObjectID:   42,
				FragmentID: 0,
				Start:      true,
				End:        false,
				Data:       []byte("part one"),
			},
		},
		{
			name: "end fragment",
			frag: &Fragment{
				ObjectID:   42,
				FragmentID: 2,
				Start:      false,
				End:        true,
				Data:       []byte("part three"),
			},
		},
		{
			name: "empty data",
			frag: &Fragment{
				ObjectID:   1,
				FragmentID: 0,
				Start:      true,
				End:        true,
				Data:       nil,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoded, err := tt.frag.Encode()
			if err != nil {
				t.Fatalf("Encode failed: %v", err)
			}
			decoded, err := Decode(encoded)
			if err != nil {
				t.Fatalf("Decode failed: %v", err)
			}

			if decoded.ObjectID != tt.frag.ObjectID {
				t.Errorf("ObjectID mismatch: got %d, want %d", decoded.ObjectID, tt.frag.ObjectID)
			}
			if decoded.FragmentID != tt.frag.FragmentID {
				t.Errorf("FragmentID mismatch: got %d, want %d", decoded.FragmentID, tt.frag.FragmentID)
			}
			if decoded.Start != tt.frag.Start {
				t.Errorf("Start mismatch: got %v, want %v", decoded.Start, tt.frag.Start)
			}
			if decoded.End != tt.frag.End {
				t.Errorf("End mismatch: got %v, want %v", decoded.End, tt.frag.End)
			}
			if !bytes.Equal(decoded.Data, tt.frag.Data) {
				t.Errorf("Data mismatch: got %v, want %v", decoded.Data, tt.frag.Data)
			}
		})
	}
}

func TestFragmenter(t *testing.T) {
	tests := []struct {
		name      string
		maxSize   int
		data      []byte
		wantCount int
	}{
		{
			name:      "single fragment",
			maxSize:   1000,
			data:      []byte("small message"),
			wantCount: 1,
		},
		{
			name:      "multiple fragments",
			maxSize:   HeaderSize + 10, // 10 bytes payload per fragment
			data:      []byte("this is a longer message that needs splitting"),
			wantCount: 5,
		},
		{
			name:      "empty data",
			maxSize:   100,
			data:      []byte{},
			wantCount: 1,
		},
		{
			name:      "exact fit",
			maxSize:   HeaderSize + 5,
			data:      []byte("12345"),
			wantCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := NewFragmenter(tt.maxSize)
			frags, err := f.Fragment(tt.data)
			if err != nil {
				t.Fatalf("Fragment failed: %v", err)
			}

			if len(frags) != tt.wantCount {
				t.Errorf("fragment count: got %d, want %d", len(frags), tt.wantCount)
			}

			// Verify first fragment has Start flag
			if len(frags) > 0 && !frags[0].Start {
				t.Error("first fragment should have Start flag")
			}

			// Verify last fragment has End flag
			if len(frags) > 0 && !frags[len(frags)-1].End {
				t.Error("last fragment should have End flag")
			}

			// Verify middle fragments have neither flag
			for i := 1; i < len(frags)-1; i++ {
				if frags[i].Start || frags[i].End {
					t.Errorf("middle fragment %d should not have Start or End flags", i)
				}
			}
		})
	}
}

func TestAssembler(t *testing.T) {
	// Create test data
	data := []byte("this is a test message for assembly")

	// Fragment it
	f := NewFragmenter(HeaderSize + 10)
	frags, err := f.Fragment(data)
	if err != nil {
		t.Fatalf("Fragment failed: %v", err)
	}

	// Reassemble
	a := NewAssembler()
	var result []byte
	var complete bool

	for _, frag := range frags {
		complete, result, err = a.Add(frag)
		if err != nil {
			t.Fatalf("Add failed: %v", err)
		}
	}

	if !complete {
		t.Error("expected complete message after all fragments")
	}

	if !bytes.Equal(result, data) {
		t.Errorf("reassembled data mismatch:\ngot:  %s\nwant: %s", result, data)
	}
}

func TestAssemblerOutOfOrder(t *testing.T) {
	// Create test data
	data := []byte("out of order test message here")

	// Fragment it
	f := NewFragmenter(HeaderSize + 8)
	frags, err := f.Fragment(data)
	if err != nil {
		t.Fatalf("Fragment failed: %v", err)
	}

	// Shuffle fragments (reverse order)
	shuffled := make([]*Fragment, len(frags))
	for i, frag := range frags {
		shuffled[len(frags)-1-i] = frag
	}

	// Reassemble
	a := NewAssembler()
	var result []byte
	var complete bool

	for _, frag := range shuffled {
		complete, result, err = a.Add(frag)
		if err != nil {
			t.Fatalf("Add failed: %v", err)
		}
	}

	if !complete {
		t.Error("expected complete message after all fragments")
	}

	if !bytes.Equal(result, data) {
		t.Errorf("reassembled data mismatch:\ngot:  %s\nwant: %s", result, data)
	}
}

func TestAssemblerDuplicate(t *testing.T) {
	// Create a multi-fragment message to test duplicate detection
	frag1 := &Fragment{
		ObjectID:   1,
		FragmentID: 0,
		Start:      true,
		End:        false,
		Data:       []byte("test"),
	}
	frag2 := &Fragment{
		ObjectID:   1,
		FragmentID: 1,
		Start:      false,
		End:        true,
		Data:       []byte("data"),
	}

	a := NewAssembler()

	// Add first fragment
	complete, _, err := a.Add(frag1)
	if err != nil {
		t.Fatalf("first Add failed: %v", err)
	}
	if complete {
		t.Fatal("message should not be complete yet")
	}

	// Adding same fragment again should error (message not yet complete)
	_, _, err = a.Add(frag1)
	if err != ErrDuplicateFragment {
		t.Errorf("expected ErrDuplicateFragment, got %v", err)
	}

	// Complete the message
	complete, result, err := a.Add(frag2)
	if err != nil {
		t.Fatalf("second Add failed: %v", err)
	}
	if !complete {
		t.Fatal("message should be complete")
	}
	expected := []byte("testdata")
	if !bytes.Equal(result, expected) {
		t.Errorf("got %v, want %v", result, expected)
	}

	// After completion, the message is removed from pending (prevents memory leak)
	// Adding fragments for the same ObjectID now starts a new message
	complete, _, err = a.Add(frag1)
	if err != nil {
		t.Fatalf("adding fragment after completion should succeed: %v", err)
	}
	if complete {
		t.Fatal("new message should not be complete with just first fragment")
	}
}

func TestAssemblerDoSProtection(t *testing.T) {
	t.Run("max pending messages", func(t *testing.T) {
		a := NewAssemblerWithLimits(2, 100) // Only allow 2 pending messages

		// Add first message (incomplete)
		frag1 := &Fragment{
			ObjectID:   1,
			FragmentID: 0,
			Start:      true,
			End:        false,
			Data:       []byte("test1"),
		}
		_, _, err := a.Add(frag1)
		if err != nil {
			t.Fatalf("first message failed: %v", err)
		}

		// Add second message (incomplete)
		frag2 := &Fragment{
			ObjectID:   2,
			FragmentID: 0,
			Start:      true,
			End:        false,
			Data:       []byte("test2"),
		}
		_, _, err = a.Add(frag2)
		if err != nil {
			t.Fatalf("second message failed: %v", err)
		}

		// Third message should be rejected
		frag3 := &Fragment{
			ObjectID:   3,
			FragmentID: 0,
			Start:      true,
			End:        false,
			Data:       []byte("test3"),
		}
		_, _, err = a.Add(frag3)
		if err == nil {
			t.Fatal("expected error for exceeding max pending messages")
		}
		if !bytes.Contains([]byte(err.Error()), []byte("too many pending messages")) {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("max fragments per message", func(t *testing.T) {
		a := NewAssemblerWithLimits(100, 2) // Only allow 2 fragments per message

		// Add first fragment
		frag1 := &Fragment{
			ObjectID:   1,
			FragmentID: 0,
			Start:      true,
			End:        false,
			Data:       []byte("test1"),
		}
		_, _, err := a.Add(frag1)
		if err != nil {
			t.Fatalf("first fragment failed: %v", err)
		}

		// Add second fragment
		frag2 := &Fragment{
			ObjectID:   1,
			FragmentID: 1,
			Start:      false,
			End:        false,
			Data:       []byte("test2"),
		}
		_, _, err = a.Add(frag2)
		if err != nil {
			t.Fatalf("second fragment failed: %v", err)
		}

		// Third fragment should be rejected
		frag3 := &Fragment{
			ObjectID:   1,
			FragmentID: 2,
			Start:      false,
			End:        true,
			Data:       []byte("test3"),
		}
		_, _, err = a.Add(frag3)
		if err == nil {
			t.Fatal("expected error for exceeding max fragments")
		}
		if !bytes.Contains([]byte(err.Error()), []byte("too many fragments")) {
			t.Errorf("unexpected error: %v", err)
		}
	})
}
