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
			encoded := tt.frag.Encode()
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
		name        string
		maxSize     int
		data        []byte
		wantCount   int
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
	frag := &Fragment{
		ObjectID:   1,
		FragmentID: 0,
		Start:      true,
		End:        true,
		Data:       []byte("test"),
	}

	a := NewAssembler()
	_, _, err := a.Add(frag)
	if err != nil {
		t.Fatalf("first Add failed: %v", err)
	}

	// Adding same fragment again should error
	_, _, err = a.Add(frag)
	if err != ErrDuplicateFragment {
		t.Errorf("expected ErrDuplicateFragment, got %v", err)
	}
}
