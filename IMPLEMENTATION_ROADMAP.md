# Performance Optimization Implementation Roadmap

**Status:** Ready for Implementation
**Created:** 2025-12-23
**Target:** 30-50% throughput improvement, 40-60% allocation reduction

---

## Executive Summary

This roadmap combines insights from:

- Comprehensive codebase analysis
- Go performance best practices (goperf.dev, go-perfbook)
- Identified transport layer bottlenecks
- Critical hot path analysis

**Key Decision Points:**

1. ✅ **Start with safe optimizations** (map[*PSObject]int, not unsafe.Pointer)
2. ✅ **Measure everything** (benchstat before/after)
3. ✅ **Phase 1 first** (highest ROI, lowest risk)
4. ⚠️ **Review unsafe usage** only if safe optimizations don't meet targets

---

## Phase 1: Critical Optimizations (Week 1-2)

**Goal:** Achieve 20-30% improvement with low-risk changes
**Priority:** 🔴 CRITICAL
**Risk Level:** LOW

### 1.1 Object Serialization - Reference Lookup (serialization/clixml.go)

**Current:** O(n) linear search through slice
**Target:** O(1) map lookup

**Implementation:**

```go
// serialization/clixml.go

type Serializer struct {
    buf        bytes.Buffer
    refCounter int
    objRefs    map[*PSObject]int  // SAFE: pointer comparison
    tnRefs     map[string]int
    encryptor  EncryptionProvider
}

var serializerPool = sync.Pool{
    New: func() interface{} {
        return &Serializer{
            objRefs: make(map[*PSObject]int, 16),
            tnRefs:  make(map[string]int, 16),
        }
    },
}

func (s *Serializer) findObjRef(v interface{}) (int, bool) {
    if pObj, ok := v.(*PSObject); ok {
        refID, exists := s.objRefs[pObj]
        return refID, exists
    }
    return 0, false
}

func (s *Serializer) addObjRef(v interface{}, refID int) {
    if pObj, ok := v.(*PSObject); ok {
        s.objRefs[pObj] = refID
    }
}

func (s *Serializer) Reset() {
    s.buf.Reset()
    s.refCounter = 0
    // Clear maps efficiently
    for k := range s.objRefs {
        delete(s.objRefs, k)
    }
    for k := range s.tnRefs {
        delete(s.tnRefs, k)
    }
    s.encryptor = nil
}
```

**Files to modify:**

- `serialization/clixml.go` (lines 128-222)

**Expected Impact:** 10-50x faster for large object graphs
**Risk:** NONE - Go's map with pointer keys is standard

---

### 1.2 Cached Sorted Keys (serialization/clixml.go)

**Current:** Allocate + sort on every serialization
**Target:** Reuse slice from pool

**Implementation:**

```go
// serialization/clixml.go

var keySlicePool = sync.Pool{
    New: func() interface{} {
        return make([]string, 0, 32)
    },
}

// In serializePSObject (around line 867):
func (s *Serializer) serializePSObject(obj *PSObject, name string) error {
    // ... existing code ...

    // Serialize Properties
    if len(obj.Properties) > 0 {
        s.buf.WriteString("<Props>")

        // Get pooled slice
        keys := keySlicePool.Get().([]string)
        keys = keys[:0] // Reset length

        for k := range obj.Properties {
            keys = append(keys, k)
        }
        sort.Strings(keys)

        for _, propName := range keys {
            propValue := obj.Properties[propName]
            if err := s.serializeValueWithName(propValue, propName); err != nil {
                keySlicePool.Put(keys) // Return on error
                return fmt.Errorf("serialize property %s: %w", propName, err)
            }
        }

        s.buf.WriteString("</Props>")
        keySlicePool.Put(keys) // Return to pool
    }

    // Same for Members...
}
```

**Files to modify:**

- `serialization/clixml.go` (lines 841-878)

**Expected Impact:** 20-30% reduction in serialization allocations
**Risk:** LOW - standard pooling pattern

---

### 1.3 Serialization Fast Paths (serialization/clixml.go)

**Current:** Type switch then full processing
**Target:** Inline fast paths for common types

**Implementation:**

```go
// serialization/clixml.go (line 275)

func (s *Serializer) serializeValue(v interface{}) error {
    return s.serializeValueWithName(v, "")
}

func (s *Serializer) serializeValueWithName(v interface{}, name string) error {
    if v == nil {
        s.writeNil(name)
        return nil
    }

    // Fast paths - avoid reflection
    switch val := v.(type) {
    case string:
        return s.serializeStringFast(val, name)
    case int32:
        return s.serializeInt32Fast(val, name)
    case int:
        return s.serializeInt32Fast(int32(val), name)
    case bool:
        return s.serializeBoolFast(val, name)
    case []byte:
        return s.serializeByteArrayFast(val, name)
    case *PSObject:
        return s.serializePSObject(val, name)
    case map[string]interface{}:
        return s.serializeHashtable(val, name)
    case []interface{}:
        return s.serializeArray(val, name)
    default:
        // Slow path: existing reflection-based logic
        return s.serializeValueSlow(v, name)
    }
}

// Inline fast path implementations
func (s *Serializer) serializeStringFast(val string, name string) error {
    if name != "" {
        s.buf.WriteString(`<S N="`)
        s.buf.WriteString(name)
        s.buf.WriteString(`">`)
    } else {
        s.buf.WriteString("<S>")
    }
    if err := xml.EscapeText(&s.buf, []byte(val)); err != nil {
        return err
    }
    s.buf.WriteString("</S>")
    return nil
}

func (s *Serializer) serializeInt32Fast(val int32, name string) error {
    if name != "" {
        s.buf.WriteString(`<I32 N="`)
        s.buf.WriteString(name)
        s.buf.WriteString(`">`)
    } else {
        s.buf.WriteString("<I32>")
    }
    s.buf.WriteString(strconv.FormatInt(int64(val), 10))
    s.buf.WriteString("</I32>")
    return nil
}

func (s *Serializer) serializeBoolFast(val bool, name string) error {
    if name != "" {
        s.buf.WriteString(`<B N="`)
        s.buf.WriteString(name)
        s.buf.WriteString(`">`)
    } else {
        s.buf.WriteString("<B>")
    }
    if val {
        s.buf.WriteString("true")
    } else {
        s.buf.WriteString("false")
    }
    s.buf.WriteString("</B>")
    return nil
}

func (s *Serializer) serializeByteArrayFast(val []byte, name string) error {
    if name != "" {
        s.buf.WriteString(`<BA N="`)
        s.buf.WriteString(name)
        s.buf.WriteString(`">`)
    } else {
        s.buf.WriteString("<BA>")
    }
    s.buf.WriteString(base64.StdEncoding.EncodeToString(val))
    s.buf.WriteString("</BA>")
    return nil
}
```

**Files to modify:**

- `serialization/clixml.go` (refactor around lines 275-600)

**Expected Impact:** 15-25% faster for common types
**Risk:** LOW - existing logic preserved in slow path

---

### 1.4 Transport Manual Parser (outofproc/transport.go)

**Current:** xml.Decoder on every packet
**Target:** String slicing for simple format

**Implementation:**

```go
// outofproc/transport.go (replace parsePacket)

func parsePacket(line string) (*Packet, error) {
    // Trim whitespace and BOM
    line = strings.TrimSpace(line)

    if len(line) < 5 {
        return nil, fmt.Errorf("line too short: %q", line)
    }

    packet := &Packet{Stream: StreamDefault}

    // Fast path: extract element name
    // Format: <ElementName Space='...' PSGuid='...'> or <ElementName ... />
    start := strings.IndexByte(line, '<')
    if start == -1 {
        return nil, fmt.Errorf("no opening < found")
    }
    line = line[start+1:] // Skip '<'

    // Find end of element name
    spaceIdx := strings.IndexByte(line, ' ')
    closeIdx := strings.IndexByte(line, '>')
    slashIdx := strings.IndexByte(line, '/')

    var elemName string
    var attrStart int

    if spaceIdx != -1 && (closeIdx == -1 || spaceIdx < closeIdx) {
        elemName = line[:spaceIdx]
        attrStart = spaceIdx + 1
    } else if slashIdx != -1 && (closeIdx == -1 || slashIdx < closeIdx) {
        elemName = line[:slashIdx]
        attrStart = slashIdx
    } else if closeIdx != -1 {
        elemName = line[:closeIdx]
        attrStart = closeIdx
    } else {
        return nil, fmt.Errorf("malformed element")
    }

    packet.Type = PacketType(elemName)

    // Extract PSGuid attribute
    if idx := strings.Index(line, "PSGuid='"); idx != -1 {
        guidStart := idx + 8 // len("PSGuid='")
        guidEnd := strings.IndexByte(line[guidStart:], '\'')
        if guidEnd == -1 {
            return nil, fmt.Errorf("unterminated PSGuid")
        }
        guid, err := uuid.Parse(line[guidStart : guidStart+guidEnd])
        if err != nil {
            return nil, fmt.Errorf("parse PSGuid: %w", err)
        }
        packet.PSGuid = guid
    }

    // Extract Stream attribute (optional)
    if idx := strings.Index(line, "Stream='"); idx != -1 {
        streamStart := idx + 8 // len("Stream='")
        streamEnd := strings.IndexByte(line[streamStart:], '\'')
        if streamEnd != -1 {
            packet.Stream = Stream(line[streamStart : streamStart+streamEnd])
        }
    }

    // For Data packets, extract base64 content
    if packet.Type == PacketTypeData {
        contentStart := strings.IndexByte(line, '>')
        if contentStart == -1 {
            return packet, nil // Self-closing
        }
        contentStart++

        contentEnd := strings.Index(line[contentStart:], "</")
        if contentEnd == -1 {
            return packet, nil // No content
        }

        base64Data := strings.TrimSpace(line[contentStart : contentStart+contentEnd])
        if base64Data != "" {
            decoded, err := base64.StdEncoding.DecodeString(base64Data)
            if err != nil {
                return nil, fmt.Errorf("decode base64: %w", err)
            }
            packet.Data = decoded
        }
    }

    return packet, nil
}
```

**Files to modify:**

- `outofproc/transport.go` (lines 180-238)

**Expected Impact:** 5-10x faster parsing, 90% fewer allocations
**Risk:** LOW - format is strict and testable

---

### 1.5 Transport Buffer Pooling (outofproc/transport.go)

**Current:** Allocate string on every send
**Target:** Pooled bytes.Buffer

**Implementation:**

```go
// outofproc/transport.go

var sendBufferPool = sync.Pool{
    New: func() interface{} {
        return new(bytes.Buffer)
    },
}

func (t *Transport) SendDataWithStream(psGuid uuid.UUID, stream Stream, data []byte) error {
    t.mu.Lock()
    defer t.mu.Unlock()

    buf := sendBufferPool.Get().(*bytes.Buffer)
    buf.Reset()
    defer sendBufferPool.Put(buf)

    // Write framing
    buf.WriteString("<Data Stream='")
    buf.WriteString(string(stream))
    buf.WriteString("' PSGuid='")
    buf.WriteString(formatGUID(psGuid))
    buf.WriteString("'>")

    // Streaming base64 encode
    encoder := base64.NewEncoder(base64.StdEncoding, buf)
    encoder.Write(data)
    encoder.Close()

    buf.WriteString("</Data>\n")

    // Single write
    _, err := t.writer.Write(buf.Bytes())
    return err
}

// Apply same pattern to SendCommand, SendClose, SendSignal, SendDataAck
```

**Files to modify:**

- `outofproc/transport.go` (lines 76-138)

**Expected Impact:** 80% reduction in send allocations
**Risk:** LOW - standard pooling

---

### 1.6 Fragment Buffer Pool (fragments/fragments.go)

**Current:** Allocate on every decode
**Target:** Reuse buffers

**Implementation:**

```go
// fragments/fragments.go

var fragmentDataPool = sync.Pool{
    New: func() interface{} {
        return make([]byte, 0, 4096) // Common size
    },
}

// Update Decode function
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

    // Get pooled buffer
    buf := fragmentDataPool.Get().([]byte)
    if cap(buf) < int(blobLen) {
        // Need larger buffer
        buf = make([]byte, blobLen)
    } else {
        buf = buf[:blobLen]
    }
    copy(buf, data[21:21+blobLen])
    f.Data = buf

    return f, nil
}

// IMPORTANT: Caller must return buffer to pool
// Add a method to Fragment:
func (f *Fragment) Release() {
    if f.Data != nil {
        fragmentDataPool.Put(f.Data[:0])
        f.Data = nil
    }
}
```

**Files to modify:**

- `fragments/fragments.go` (lines 124-150)
- Call sites must call `Release()` when done

**Expected Impact:** 40-60% reduction in fragment allocations
**Risk:** MEDIUM - requires careful lifecycle management

---

## Phase 1 Verification Checklist

### Before Starting

- [ ] Establish baseline benchmarks (see below)
- [ ] Run all tests: `go test ./...`
- [ ] Run race detector: `go test -race ./...`
- [ ] Profile current state: CPU + memory

### After Each Change

- [ ] Run specific benchmarks for changed package
- [ ] Verify no test regressions
- [ ] Run race detector
- [ ] Compare benchstat results

### Final Phase 1 Validation

- [ ] All tests pass
- [ ] No new race conditions
- [ ] Allocation reduction ≥ 30%
- [ ] Throughput improvement ≥ 20%
- [ ] Code review (readability maintained)

---

## Baseline Benchmark Suite

**Create: `performance_test.go` (root level)**

```go
package psrpcore_test

import (
    "bytes"
    "testing"
    "github.com/google/uuid"
    "github.com/smnsjas/go-psrpcore/serialization"
    "github.com/smnsjas/go-psrpcore/fragments"
    "github.com/smnsjas/go-psrpcore/outofproc"
)

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

func BenchmarkSerializeLargeObject(b *testing.B) {
    obj := &serialization.PSObject{
        TypeNames: []string{"System.Management.Automation.PSCustomObject"},
        Properties: make(map[string]interface{}),
    }
    for i := 0; i < 50; i++ {
        obj.Properties[fmt.Sprintf("Prop%d", i)] = "value"
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
        _, err := fragments.Decode(encoded)
        if err != nil {
            b.Fatal(err)
        }
    }
}

func BenchmarkTransportParsePacket(b *testing.B) {
    line := "<Data Stream='Default' PSGuid='00000000-0000-0000-0000-000000000000'>dGVzdGRhdGE=</Data>"

    b.ResetTimer()
    b.ReportAllocs()
    for i := 0; i < b.N; i++ {
        _, err := outofproc.ParsePacket(line) // Need to export this
        if err != nil {
            b.Fatal(err)
        }
    }
}

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
```

**Establish baseline:**

```bash
# Run benchmarks and save
go test -bench=. -benchmem -count=5 ./... > baseline.txt

# Statistical summary
benchstat baseline.txt
```

---

## Phase 2: Concurrency & Scale (Week 3-4)

**Goal:** Further improve under high concurrency
**Priority:** 🟡 MEDIUM

### 2.1 sync.Map for Pipelines (runspace/runspace.go)

Replace `map[uuid.UUID]*pipeline.Pipeline` with `sync.Map`

**When to do this:** Only if profiling shows RWMutex contention

### 2.2 Worker Pool for Host Callbacks (runspace/runspace.go)

Fixed worker pool instead of unbounded goroutine creation

**When to do this:** Only if profiling shows goroutine overhead

### 2.3 Configurable Channel Sizes

Make channel buffer sizes tunable

**When to do this:** After Phase 1 benchmarks show bottlenecks

---

## Decision Points

### When to use unsafe.Pointer for objRefs?

**Criteria:**

- Phase 1 safe optimization doesn't meet 30% allocation reduction target
- Profiling shows map lookup is still a hotspot
- Team accepts the maintenance cost

**How to implement:**

```go
type Serializer struct {
    buf        bytes.Buffer
    refCounter int
    objRefs    map[uintptr]int  // unsafe.Pointer as uintptr
    tnRefs     map[string]int
    encryptor  EncryptionProvider
}

func (s *Serializer) findObjRef(v interface{}) (int, bool) {
    if pObj, ok := v.(*PSObject); ok {
        addr := uintptr(unsafe.Pointer(pObj))
        refID, exists := s.objRefs[addr]
        return refID, exists
    }
    return 0, false
}
```

**Risk:** Higher - must ensure pointers don't move (they won't in Go, but less obvious)

---

## Success Metrics

### Phase 1 Targets

- ✅ Serialization: 25-35% faster
- ✅ Transport parsing: 5-10x faster
- ✅ Transport sending: 3-5x fewer allocations
- ✅ Fragment decode: 40-60% fewer allocations
- ✅ Overall: 20-30% throughput improvement

### Phase 2 Targets

- ✅ Concurrent pipeline creation: 2x faster
- ✅ Host callback overhead: 50% reduction
- ✅ Overall: 30-50% throughput improvement (cumulative)

---

## Rollback Strategy

If any optimization causes issues:

1. **Identify via benchmarks:** `benchstat before.txt after.txt`
2. **Verify with tests:** `go test -race ./...`
3. **Revert specific change:** Git revert individual commits
4. **Re-establish baseline:** Run benchmarks again

Each optimization should be in a separate commit for easy rollback.

---

## Next Steps

1. **Create baseline benchmarks** (today)

   ```bash
   # In root directory
   # Create performance_test.go with benchmarks above
   go test -bench=. -benchmem -count=5 -run=^$ > baseline.txt
   ```

2. **Implement Phase 1.1** (object ref map) - 2 hours
3. **Benchmark and validate** - 30 minutes
4. **Implement Phase 1.2** (cached keys) - 1 hour
5. **Benchmark and validate** - 30 minutes
6. **Continue through Phase 1** - 1-2 days total
7. **Review results** - decide on Phase 2

---

## Questions for User

1. **Unsafe usage:** Are you comfortable with `map[*PSObject]int` for Phase 1, with option to switch to `map[uintptr]int` later if needed?

2. **Fragment buffer lifecycle:** Fragment.Release() requires callers to explicitly release buffers. Is this acceptable, or should we use finalizers (slower)?

3. **Benchmarking timeline:** Should we establish baselines now, or would you prefer to start implementing immediately?

---

**Document Status:** Ready for Implementation
**Approval Required:** Unsafe decision, Fragment lifecycle approach
