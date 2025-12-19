// Package fragments handles PSRP message fragmentation and reassembly.
//
// PSRP messages can be larger than the transport's maximum message size,
// so they must be fragmented for transmission and reassembled on receipt.
//
// # Fragment Structure
//
// Each fragment has the following structure:
//
//	┌─────────────────────────────────────────────────────────┐
//	│  ObjectId (8 bytes) - Identifies the original message  │
//	├─────────────────────────────────────────────────────────┤
//	│  FragmentId (8 bytes) - Sequence number                │
//	├─────────────────────────────────────────────────────────┤
//	│  Flags (1 byte)                                        │
//	│    Bit 0: Start fragment                               │
//	│    Bit 1: End fragment                                 │
//	├─────────────────────────────────────────────────────────┤
//	│  BlobLength (4 bytes) - Length of blob data            │
//	├─────────────────────────────────────────────────────────┤
//	│  Blob (variable) - Fragment payload                    │
//	└─────────────────────────────────────────────────────────┘
//
// # Usage
//
// To fragment a message:
//
//	fragmenter := fragments.NewFragmenter(maxSize)
//	frags, err := fragmenter.Fragment(message)
//
// To reassemble fragments:
//
//	assembler := fragments.NewAssembler()
//	for _, frag := range receivedFragments {
//	    complete, message, err := assembler.Add(frag)
//	    if complete {
//	        // message is ready
//	    }
//	}
//
// # Reference
//
// MS-PSRP Section 2.2.4: https://docs.microsoft.com/en-us/openspecs/windows_protocols/ms-psrp/
package fragments

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
)

// Fragment header size in bytes.
const HeaderSize = 21

// Flag bits for fragment headers.
const (
	FlagStart = 1 << 0
	FlagEnd   = 1 << 1
)

var (
	// ErrInvalidFragment is returned when a fragment is malformed.
	ErrInvalidFragment = errors.New("invalid fragment")
	// ErrIncompleteMessage is returned when reassembly is incomplete.
	ErrIncompleteMessage = errors.New("incomplete message")
	// ErrDuplicateFragment is returned when a duplicate fragment is received.
	ErrDuplicateFragment = errors.New("duplicate fragment")
)

// Fragment represents a single PSRP message fragment.
type Fragment struct {
	ObjectID   uint64
	FragmentID uint64
	Start      bool
	End        bool
	Data       []byte
}

// Encode serializes the fragment to bytes.
func (f *Fragment) Encode() []byte {
	buf := make([]byte, HeaderSize+len(f.Data))

	binary.BigEndian.PutUint64(buf[0:8], f.ObjectID)
	binary.BigEndian.PutUint64(buf[8:16], f.FragmentID)

	var flags byte
	if f.Start {
		flags |= FlagStart
	}
	if f.End {
		flags |= FlagEnd
	}
	buf[16] = flags

	if len(f.Data) > math.MaxUint32 {
		// Should never happen with reasonable fragment sizes
		panic("fragment data too large")
	}
	binary.BigEndian.PutUint32(buf[17:21], uint32(len(f.Data))) // #nosec G115 -- length checked against MaxUint32 above
	copy(buf[21:], f.Data)

	return buf
}

// Decode deserializes a fragment from bytes.
func Decode(data []byte) (*Fragment, error) {
	if len(data) < HeaderSize {
		return nil, ErrInvalidFragment
	}

	f := &Fragment{
		ObjectID:   binary.BigEndian.Uint64(data[0:8]),
		FragmentID: binary.BigEndian.Uint64(data[8:16]),
		Start:      data[16]&FlagStart != 0,
		End:        data[16]&FlagEnd != 0,
	}

	blobLen := binary.BigEndian.Uint32(data[17:21])
	if len(data) < HeaderSize+int(blobLen) {
		return nil, ErrInvalidFragment
	}

	f.Data = make([]byte, blobLen)
	copy(f.Data, data[21:21+blobLen])

	return f, nil
}

// Fragmenter splits messages into fragments.
type Fragmenter struct {
	maxSize  int
	objectID uint64
}

// NewFragmenter creates a new Fragmenter with the given maximum fragment size.
func NewFragmenter(maxSize int) *Fragmenter {
	return &Fragmenter{
		maxSize: maxSize,
	}
}

// Fragment splits data into one or more fragments.
func (f *Fragmenter) Fragment(data []byte) ([]*Fragment, error) {
	f.objectID++
	objectID := f.objectID

	maxPayload := f.maxSize - HeaderSize
	if maxPayload <= 0 {
		maxPayload = len(data)
	}

	var fragments []*Fragment
	var fragmentID uint64

	for offset := 0; offset < len(data); {
		end := offset + maxPayload
		if end > len(data) {
			end = len(data)
		}

		frag := &Fragment{
			ObjectID:   objectID,
			FragmentID: fragmentID,
			Start:      offset == 0,
			End:        end == len(data),
			Data:       data[offset:end],
		}

		fragments = append(fragments, frag)
		offset = end
		fragmentID++
	}

	// Handle empty data
	if len(fragments) == 0 {
		fragments = append(fragments, &Fragment{
			ObjectID:   objectID,
			FragmentID: 0,
			Start:      true,
			End:        true,
			Data:       nil,
		})
	}

	return fragments, nil
}

// Assembler reassembles fragments into complete messages.
type Assembler struct {
	pending            map[uint64]*pendingMessage
	maxPendingMessages int // Maximum number of pending messages (DoS protection)
	maxFragmentsPerMsg int // Maximum fragments per message (DoS protection)
}

type pendingMessage struct {
	fragments map[uint64][]byte
	total     int
	received  int
}

const (
	// DefaultMaxPendingMessages is the default limit for concurrent pending messages
	DefaultMaxPendingMessages = 1000
	// DefaultMaxFragmentsPerMsg is the default limit for fragments per message
	DefaultMaxFragmentsPerMsg = 10000
)

// NewAssembler creates a new Assembler with default limits.
func NewAssembler() *Assembler {
	return &Assembler{
		pending:            make(map[uint64]*pendingMessage),
		maxPendingMessages: DefaultMaxPendingMessages,
		maxFragmentsPerMsg: DefaultMaxFragmentsPerMsg,
	}
}

// NewAssemblerWithLimits creates a new Assembler with custom limits.
func NewAssemblerWithLimits(maxPending, maxFragments int) *Assembler {
	return &Assembler{
		pending:            make(map[uint64]*pendingMessage),
		maxPendingMessages: maxPending,
		maxFragmentsPerMsg: maxFragments,
	}
}

// Add adds a fragment and returns true if the message is complete.
// If complete, the assembled message data is returned and the message is removed from pending.
func (a *Assembler) Add(f *Fragment) (complete bool, data []byte, err error) {
	pm, exists := a.pending[f.ObjectID]
	if !exists {
		// DoS protection: limit concurrent pending messages
		if len(a.pending) >= a.maxPendingMessages {
			return false, nil, fmt.Errorf("too many pending messages: %d >= %d", len(a.pending), a.maxPendingMessages)
		}

		pm = &pendingMessage{
			fragments: make(map[uint64][]byte),
			total:     -1,
		}
		a.pending[f.ObjectID] = pm
	}

	// DoS protection: limit fragments per message
	if pm.received >= a.maxFragmentsPerMsg {
		delete(a.pending, f.ObjectID) // Clean up to prevent leak
		return false, nil, fmt.Errorf("too many fragments for message %d: %d >= %d", f.ObjectID, pm.received, a.maxFragmentsPerMsg)
	}

	if _, dup := pm.fragments[f.FragmentID]; dup {
		return false, nil, ErrDuplicateFragment
	}

	pm.fragments[f.FragmentID] = f.Data
	pm.received++

	if f.End {
		if f.FragmentID > uint64(math.MaxInt-1) {
			return false, nil, fmt.Errorf("fragment ID too large: %d", f.FragmentID)
		}
		pm.total = int(f.FragmentID) + 1
	}

	// Check if complete
	if pm.total > 0 && pm.received == pm.total {
		// Reassemble in order
		var result []byte
		total := uint64(pm.total) // #nosec G115 -- pm.total derived safely from positive FragmentID
		for i := uint64(0); i < total; i++ {
			result = append(result, pm.fragments[i]...)
		}

		// CRITICAL FIX: Delete completed message to prevent memory leak
		// Duplicate detection is handled by TCP layer in most transports
		delete(a.pending, f.ObjectID)

		return true, result, nil
	}

	return false, nil, nil
}
