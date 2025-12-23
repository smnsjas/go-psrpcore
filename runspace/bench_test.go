package runspace

import (
	"sync"
	"testing"

	"github.com/google/uuid"
)

// BenchmarkMap_RWMutex benchmarks the current implementation pattern (RWMutex + map).
// It simulates 100% read workload (dispatch loop) with occasional contention if we added writes.
func BenchmarkMap_RWMutex(b *testing.B) {
	m := make(map[uuid.UUID]int)
	var mu sync.RWMutex

	// Populate map
	ids := make([]uuid.UUID, 100)
	for i := 0; i < 100; i++ {
		id := uuid.New()
		ids[i] = id
		m[id] = i
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			id := ids[i%100]
			mu.RLock()
			_, _ = m[id]
			mu.RUnlock()
			i++
		}
	})
}

// BenchmarkMap_SyncMap benchmarks the proposed implementation pattern (sync.Map).
func BenchmarkMap_SyncMap(b *testing.B) {
	var m sync.Map

	// Populate map
	ids := make([]uuid.UUID, 100)
	for i := 0; i < 100; i++ {
		id := uuid.New()
		ids[i] = id
		m.Store(id, i)
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			id := ids[i%100]
			_, _ = m.Load(id)
			i++
		}
	})
}
