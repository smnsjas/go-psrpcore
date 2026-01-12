package psrp_test

import (
	"bytes"
	"fmt"
	"sync"
	"testing"

	"github.com/smnsjas/go-psrpcore/serialization"
)

// TestSerializeUnsafeConcurrency verifies that SerializeUnsafe is safe to use
// concurrently (when each goroutine has its own serializer from the pool)
// and that the data remains valid long enough to be copied.
func TestSerializeUnsafeConcurrency(t *testing.T) {
	// High concurrency to trigger any potential race conditions in the pool
	concurrency := 100
	iterations := 1000

	var wg sync.WaitGroup
	wg.Add(concurrency)

	errCh := make(chan error, concurrency)

	for i := 0; i < concurrency; i++ {
		go func(id int) {
			defer wg.Done()

			// Each goroutine mimics the behavior in pipeline.go:
			// 1. Get serializer
			// 2. SerializeUnsafe
			// 3. Copy data (consume it)
			// 4. Close serializer

			for j := 0; j < iterations; j++ {
				ser := serialization.NewSerializer()

				// Create a unique object for this iteration
				expected := fmt.Sprintf("Data-%d-%d", id, j)
				obj := &serialization.PSObject{
					TypeNames: []string{"System.String"},
					Properties: map[string]interface{}{
						"Value": expected,
					},
				}

				// Unsafe serialization
				data, err := ser.SerializeUnsafe(obj)
				if err != nil {
					errCh <- fmt.Errorf("serialize err: %v", err)
					ser.Close()
					return
				}

				// Immediate consumption (Copy) - this mimics the Message.Encode behavior
				// If we don't copy, we can't expect it to persist after Close()
				// But here we want to verify that *during* this window, the data is correct
				copiedData := make([]byte, len(data))
				copy(copiedData, data)

				// Close immediately after copy (returning to pool)
				ser.Close()

				// Verify integrity of the COPIED data
				// If SerializeUnsafe had returned a pointer to a buffer that was being written to by another
				// goroutine (bad pool usage), this would likely fail or show corruption.
				if !bytes.Contains(copiedData, []byte(expected)) {
					errCh <- fmt.Errorf("concurrency corruption! expected %s in output, got: %q", expected, copiedData)
					return
				}
			}
		}(i)
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Fatal(err)
	}
}

// TestSerializeUnsafeDataValidity verifies that the buffer returned by SerializeUnsafe
// is valid untainted data, specifically checking that the 'reset' logic works correctly
// when reusing serializers from the pool.
func TestSerializeUnsafeDataValidity(t *testing.T) {
	// Mimic pool reuse
	ser := serialization.NewSerializer()
	defer ser.Close()

	// 1. Serialize a LARGE object to expand the buffer
	largeVal := make([]byte, 1024*10) // 10KB
	for i := range largeVal {
		largeVal[i] = 'A'
	}

	_, err := ser.SerializeUnsafe(map[string]interface{}{"Large": largeVal})
	if err != nil {
		t.Fatal(err)
	}

	// Internally, ser.buf is now at least 10KB + overhead

	// 2. We pretend to return it to pool and get it back (Reset called)
	ser.Reset()

	// 3. Serialize a SMALL object
	// If Reset() didn't work right, or if Unsafe returns the whole capacity,
	// we might see old 'A's if we aren't careful (though slice length handles this).
	smallObj := "Small"
	data, err := ser.SerializeUnsafe(smallObj)
	if err != nil {
		t.Fatal(err)
	}

	// Verify the data contains ONLY the small object XML and not the previous garbage
	// The CLIXML for string "Small" is: <S>Small</S> wrapped in Objs
	expected := "<S>Small</S>"
	if !bytes.Contains(data, []byte(expected)) {
		t.Fatalf("Expected %s, got %s", expected, string(data))
	}

	// Verify length is reasonable (not 10KB)
	if len(data) > 500 {
		t.Errorf("Buffer was not sliced correctly! Len is %d, expected < 500", len(data))
	}
}
