# Baseline Performance Metrics

**Date:** 2025-12-23
**Platform:** darwin/arm64 (Apple M1 Pro)
**Go Version:** 1.25
**Benchmark Runs:** 5 iterations per test

---

## Summary

Comprehensive baseline benchmarks have been established for the go-psrpcore library. These metrics will be used to measure the impact of Phase 1 optimizations.

**Total Benchmarks:** 28 performance tests covering serialization, fragments, messages, transport, and end-to-end workflows.

---

## Key Performance Metrics

### Serialization Performance

| Benchmark | Time/op | Bytes/op | Allocs/op |
|-----------|---------|----------|-----------|
| Small Object (1 property) | 327 ns | 256 B | 5 |
| Medium Object (10 properties) | 1,475 ns | 952 B | 24 |
| Large Object (50 properties) | 7,269 ns | 3,960 B | 104 |
| With References (3 refs) | 753 ns | 520 B | 11 |
| String | 114 ns | 48 B | 2 |
| Int32 | 56 ns | 21 B | 2 |
| Hashtable | 606 ns | 433 B | 12 |

**Deserialization:**
| Benchmark | Time/op | Bytes/op | Allocs/op |
|-----------|---------|----------|-----------|
| Small Object | 4,044 ns | 3,256 B | 69 |

### Fragment Processing

| Benchmark | Time/op | Bytes/op | Allocs/op |
|-----------|---------|----------|-----------|
| Encode (1KB) | 161 ns | 1,152 B | 1 |
| Decode (1KB) | 179 ns | 1,072 B | 2 |
| Encode Large (32KB) | 3,688 ns | 40,960 B | 1 |
| Assemble (4 fragments) | 713 ns | 4,456 B | 4 |

### Message Processing

| Benchmark | Time/op | Bytes/op | Allocs/op |
|-----------|---------|----------|-----------|
| Message Encode | 154 ns | 1,152 B | 1 |
| Message Decode | 156 ns | 1,088 B | 2 |

### Transport (OutOfProcess)

| Benchmark | Time/op | Bytes/op | Allocs/op |
|-----------|---------|----------|-----------|
| SendData (4KB) | ~11,000 ns | ~7,300 B | ~4 |
| SendData Small (256B) | ~2,700 ns | ~500 B | ~4 |
| SendCommand | ~500 ns | ~150 B | ~4 |
| ReceivePacket (Data) | ~6,900 ns | ~1,600 B | ~21 |
| ReceivePacket (Command) | ~2,300 ns | ~600 B | ~9 |

### End-to-End Performance

| Benchmark | Time/op | Bytes/op | Allocs/op |
|-----------|---------|----------|-----------|
| Round Trip Small | ~5,500 ns | ~6,800 B | ~22 |
| Round Trip Medium | ~8,900 ns | ~10,000 B | ~33 |
| Transport Round Trip | ~13,000 ns | ~8,900 B | ~25 |

---

## Performance Hotspots Identified

### 🔴 Critical (High Impact)

1. **Object Reference Lookup (serialization)**
   - Current: O(n) linear search through slice
   - Impact: Becomes expensive with large object graphs
   - Target: 10-50x improvement with map lookup

2. **Transport XML Parsing**
   - Current: ~6,900 ns for Data packet receive
   - Uses xml.Decoder with reflection
   - Target: 5-10x improvement with string parsing

3. **Transport String Allocation**
   - Current: ~11,000 ns for 4KB send
   - Multiple string allocations (base64 + fmt.Sprintf)
   - Target: 3-5x fewer allocations with buffer pooling

4. **Fragment Data Allocation**
   - Current: 2 allocs per fragment decode
   - Every fragment allocates new byte slice
   - Target: 40-60% reduction with buffer pooling

### 🟡 Medium (Moderate Impact)

5. **Sorted Keys Allocation**
   - Allocates + sorts on every serialization
   - Target: 20-30% reduction with pooling

6. **Large Object Serialization**
   - 7,269 ns / 104 allocs for 50-property object
   - Target: 15-25% improvement with fast paths

---

## Optimization Targets (Phase 1)

Based on the baseline, Phase 1 optimizations target:

### Serialization
- **Current:** 327-7,269 ns depending on object size
- **Target:** 20-30% reduction in time
- **Target:** 30-40% reduction in allocations

### Fragments
- **Current:** 161-179 ns per fragment
- **Target:** 30-50% reduction in allocations

### Transport
- **Current:** 11,000 ns send, 6,900 ns receive (4KB)
- **Target:** 3-5x improvement in send
- **Target:** 5-10x improvement in receive

### Overall Throughput
- **Target:** 20-30% improvement in end-to-end performance
- **Target:** 30-40% reduction in memory allocations

---

## Benchmark Methodology

### Running Benchmarks
```bash
# Run all benchmarks
make bench

# Save baseline
make bench-baseline

# Save with timestamp
make bench-save

# Compare with baseline
make bench-compare
```

### Statistical Analysis
```bash
# View with benchstat
benchstat benchmarks/baseline.txt

# Compare two runs
benchstat before.txt after.txt
```

### Profiling
```bash
# CPU profiling
make profile-cpu

# Memory profiling
make profile-mem

# Both
make profile-all
```

---

## Next Steps

1. ✅ **Baseline established** - Metrics captured in `benchmarks/baseline.txt`
2. ⏭️ **Phase 1.1** - Implement object reference map optimization
3. ⏭️ **Benchmark & Compare** - Validate improvement with benchstat
4. ⏭️ **Phase 1.2-1.6** - Continue through Phase 1 optimizations
5. ⏭️ **Final Validation** - Verify 20-30% overall improvement

---

## Files Created

- ✅ `performance_test.go` - 28 comprehensive benchmarks
- ✅ `Makefile` - Automated benchmark/profiling commands
- ✅ `benchmarks/baseline.txt` - Baseline metrics (5 runs each)
- ✅ `IMPLEMENTATION_ROADMAP.md` - Detailed optimization plan
- ✅ `BASELINE_PERFORMANCE.md` - This summary

---

## Appendix: Full Benchmark Results

See `benchmarks/baseline.txt` for complete results with all 5 runs per benchmark.

**Key Observations:**
- Fragment operations are very fast (150-200ns) - optimizations must be careful
- Transport layer shows most opportunity (11µs send, 7µs receive)
- Serialization scales linearly with object size (as expected)
- Deserialize is 10-12x slower than serialize (XML parsing overhead)

**Performance is already quite good** - optimizations should focus on:
1. Reducing allocations (GC pressure)
2. Eliminating O(n) operations
3. Transport layer improvements (biggest wins available)
