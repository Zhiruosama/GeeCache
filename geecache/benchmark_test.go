package geecache

import (
	"strconv"
	"sync/atomic"
	"testing"
)

func BenchmarkGroupGetHotKeyParallel(b *testing.B) {
	var loadCount int64
	group := NewGroup("bench-hot", 1<<20, GetterFunc(func(key string) ([]byte, error) {
		atomic.AddInt64(&loadCount, 1)
		return []byte("value"), nil
	}))

	_, _ = group.Get("Tom")
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _ = group.Get("Tom")
		}
	})
	b.ReportMetric(float64(atomic.LoadInt64(&loadCount)), "loads")
}

func BenchmarkGroupGetMixedKeysParallel(b *testing.B) {
	group := NewGroup("bench-mixed", 8<<20, GetterFunc(func(key string) ([]byte, error) {
		return []byte(key), nil
	}))

	var seq uint64
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			n := atomic.AddUint64(&seq, 1)
			key := strconv.FormatUint(n%2048, 10)
			_, _ = group.Get(key)
		}
	})
}
