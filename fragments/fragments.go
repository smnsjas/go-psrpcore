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
// # Byte Order (Endianness)
//
// ALL multi-byte integer fields in fragment headers use BIG-ENDIAN (network byte order).
// This is explicitly specified in MS-PSRP Section 2.2.4:
//
//	"ObjectId, FragmentId, and BlobLength are in network-byte order (big-endian)"
//
// IMPORTANT: This differs from PSRP message headers, which use little-endian.
// The endianness difference exists because:
//   - Fragments are the transport layer (network-facing)
//   - Messages are the application layer (.NET/Windows-facing)
//
// Reference: MS-PSRP Section 2.2.4 - "Fragementation Protocol"
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
	"sync"
)

// HeaderSize is the fragment header size in bytes.
const HeaderSize = 21

// Flag bits for fragment headers.
// MS-PSRP Section 2.2.4 defines only two flags: Start (S) and End (E).
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
// All multi-byte fields use big-endian per MS-PSRP Section 2.2.4.
func (f *Fragment) Encode() []byte {
	buf := make([]byte, HeaderSize+len(f.Data))

	// ObjectID (8 bytes, big-endian) - MS-PSRP 2.2.4
	binary.BigEndian.PutUint64(buf[0:8], f.ObjectID)
	// FragmentID (8 bytes, big-endian) - MS-PSRP 2.2.4
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
	// BlobLength (4 bytes, big-endian) - MS-PSRP 2.2.4
	binary.BigEndian.PutUint32(buf[17:21], uint32(len(f.Data))) // #nosec G115 -- length checked against MaxUint32 above
	copy(buf[21:], f.Data)

	return buf
}

var fragmentDataPool = sync.Pool{
	New: func() interface{} {
		return make([]byte, 0, 4096) // Common size
	},
}

// Decode deserializes a fragment from bytes.
// All multi-byte fields use big-endian per MS-PSRP Section 2.2.4.
func Decode(data []byte) (*Fragment, error) {
	if len(data) < HeaderSize {
		return nil, ErrInvalidFragment
	}

	f := &Fragment{
		// ObjectID (8 bytes, big-endian) - MS-PSRP 2.2.4
		ObjectID: binary.BigEndian.Uint64(data[0:8]),
	}
	f.FragmentID = binary.BigEndian.Uint64(data[8:16])

	flags := data[16]
	f.Start = (flags & FlagStart) != 0
	f.End = (flags & FlagEnd) != 0

	// BlobLength (4 bytes, big-endian) - MS-PSRP 2.2.4
	blobLen := binary.BigEndian.Uint32(data[17:21])
	if len(data) < HeaderSize+int(blobLen) {
		return nil, ErrInvalidFragment
	}

	// Get pooled buffer
	buf := fragmentDataPool.Get().([]byte)
	if cap(buf) < int(blobLen) {
		// Need larger buffer if pool buffer is too small
		buf = make([]byte, blobLen)
	} else {
		buf = buf[:blobLen]
	}

	copy(buf, data[21:21+blobLen])
	f.Data = buf

	return f, nil
}

// Release returns the fragment's data buffer to the pool.
// Use this when the fragment is no longer needed.
func (f *Fragment) Release() {
	if f.Data != nil {
		// Only pool buffers that aren't too massive to avoid hoarding memory
		// and ensure they are somewhat standard size.
		// For now, we pool everything that fits the initial capacity or larger,
		// but typically we might want to discard very large ones.
		// The roadmap suggested simple pooling.
		//nolint:staticcheck // SA6002: overhead of pointer pool is higher than value pool for small slices here
		fragmentDataPool.Put(f.Data)
		f.Data = nil
	}
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

// NewFragmenterWithID creates a new Fragmenter with the given maximum fragment size and starting object ID.
// The object ID will be incremented before use (so pass startID = desiredID - 1).
// Alternatively, we can adjust the logic. The current Fragment method does `f.objectID++` first.
// So if we want the first message to be X, we should initialize with X-1.
// However, to make it more intuitive, let's allow setting the current value and adjust usage or internal logic?
// No, stick to the pattern: Fragment() increments then uses.
// So NewFragmenterWithID(size, 2) means next Fragment() call uses 3.
func NewFragmenterWithID(maxSize int, currentObjectID uint64) *Fragmenter {
	return &Fragmenter{
		maxSize:  maxSize,
		objectID: currentObjectID,
	}
}

// SetObjectID sets the current object ID counter.
// The next fragment generated will use objectID + 1.
// This is useful for synchronizing sequence numbers when handshake messages
// are sent via an alternate path (e.g., WSMan creationXml).
func (f *Fragmenter) SetObjectID(id uint64) {
	f.objectID = id
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
		return false, nil, fmt.Errorf(
			"too many fragments for message %d: %d >= %d", f.ObjectID, pm.received, a.maxFragmentsPerMsg)
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
		total := uint64(pm.total) // #nosec G115 -- pm.total derived safely from positive FragmentID

		// Calculate total size to pre-allocate buffer
		var totalSize int
		for i := uint64(0); i < total; i++ {
			totalSize += len(pm.fragments[i])
		}

		result := make([]byte, 0, totalSize)
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
