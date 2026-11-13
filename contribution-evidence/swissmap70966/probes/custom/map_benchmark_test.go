package swissmap70966

import (
	"encoding/binary"
	"fmt"
	"strconv"
	"testing"
)

var sinkMap any
var sinkFound bool

// These workloads follow runtime/map_benchmark_test.go's separation of map
// creation, fill, access, existing-key assignment, and deletion. Key and value
// generation stays outside the timed sections.
func BenchmarkSwissMap(b *testing.B) {
	const keyCount = 2 * 4096

	b.Run("U64U64", func(b *testing.B) {
		keys := make([]uint64, keyCount)
		values := make([]uint64, keyCount/2)
		for i := range keys {
			keys[i] = uint64(i)
		}
		for i := range values {
			values[i] = uint64(i + 1)
		}
		benchmarkLayout(b, keys, values)
	})

	b.Run("StringValue64", func(b *testing.B) {
		keys := make([]string, keyCount)
		values := make([][64]byte, keyCount/2)
		for i := range keys {
			keys[i] = fmt.Sprintf("key-%012d", i)
		}
		for i := range values {
			binary.LittleEndian.PutUint64(values[i][:], uint64(i+1))
		}
		benchmarkLayout(b, keys, values)
	})

	b.Run("Key32Value256", func(b *testing.B) {
		keys := make([][32]byte, keyCount)
		values := make([][256]byte, keyCount/2)
		for i := range keys {
			binary.LittleEndian.PutUint64(keys[i][:], uint64(i))
		}
		for i := range values {
			binary.LittleEndian.PutUint64(values[i][:], uint64(i+1))
		}
		benchmarkLayout(b, keys, values)
	})
}

func benchmarkLayout[K comparable, V any](b *testing.B, keys []K, values []V) {
	for _, size := range []int{0, 65536} {
		b.Run("Size="+strconv.Itoa(size)+"/Make", func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				sinkMap = make(map[K]V, size)
			}
		})
	}

	for _, size := range []int{8, 64, 4096} {
		b.Run("Size="+strconv.Itoa(size), func(b *testing.B) {
			b.Run("Make", func(b *testing.B) {
				b.ReportAllocs()
				for i := 0; i < b.N; i++ {
					sinkMap = make(map[K]V, size)
				}
			})

			b.Run("FillHint", func(b *testing.B) {
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					m := make(map[K]V, size)
					for j := range size {
						m[keys[j]] = values[j]
					}
					sinkMap = m
				}
			})

			b.Run("Grow", func(b *testing.B) {
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					m := make(map[K]V)
					for j := range size {
						m[keys[j]] = values[j]
					}
					sinkMap = m
				}
			})

			for _, hitPercent := range []int{0, 50, 100} {
				b.Run("Lookup/Hit="+strconv.Itoa(hitPercent), func(b *testing.B) {
					m := filledMap(keys[:size], values[:size])
					probes := make([]K, size)
					for i := range probes {
						probes[i] = keys[size+i]
						if i%2 == 0 && hitPercent == 50 || hitPercent == 100 {
							probes[i] = keys[i]
						}
					}
					b.ReportAllocs()
					b.ResetTimer()
					for i := 0; i < b.N; i++ {
						_, sinkFound = m[probes[i&(size-1)]]
					}
				})
			}

			b.Run("Update", func(b *testing.B) {
				m := filledMap(keys[:size], values[:size])
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					index := i & (size - 1)
					m[keys[index]] = values[index]
				}
			})

			// Each iteration performs one successful deletion and one insertion.
			// Keeping the population stable avoids timing mostly missing deletes
			// or unbounded map growth. Large indirect values allocate on insertion.
			b.Run("DeleteInsert", func(b *testing.B) {
				m := filledMap(keys[:size], values[:size])
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					index := i & (size - 1)
					delete(m, keys[index])
					m[keys[index]] = values[index]
				}
			})
		})
	}
}

func filledMap[K comparable, V any](keys []K, values []V) map[K]V {
	m := make(map[K]V, len(keys))
	for i, key := range keys {
		m[key] = values[i]
	}
	return m
}
