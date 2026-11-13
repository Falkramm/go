package swissmaplifecycle

import (
	"encoding/binary"
	"fmt"
	"maps"
	"math"
	"math/rand"
	"runtime"
	"strings"
	"testing"
)

type entry[K comparable, V comparable] struct {
	key   K
	value V
}

// The reference representation uses a linear association list. Its operations
// and duplicate checks never use a Go map.
type oracle[K comparable, V comparable] []entry[K, V]

func (o oracle[K, V]) index(key K) int {
	for i := range o {
		if o[i].key == key {
			return i
		}
	}
	return -1
}

func (o *oracle[K, V]) set(key K, value V) {
	if i := (*o).index(key); i >= 0 {
		(*o)[i].value = value
		return
	}
	*o = append(*o, entry[K, V]{key, value})
}

func (o *oracle[K, V]) remove(key K) {
	if i := (*o).index(key); i >= 0 {
		copy((*o)[i:], (*o)[i+1:])
		*o = (*o)[:len(*o)-1]
	}
}

func checkMap[K comparable, V comparable](t *testing.T, m map[K]V, want oracle[K, V], iterate bool) {
	t.Helper()
	if len(m) != len(want) {
		t.Fatalf("length got %d, want %d", len(m), len(want))
	}
	for _, item := range want {
		got, ok := m[item.key]
		if !ok || got != item.value {
			t.Fatalf("lookup key %v got (%v, %v), want %v", item.key, got, ok, item.value)
		}
	}
	if !iterate {
		return
	}
	seen := make([]bool, len(want))
	count := 0
	for key, value := range m {
		i := want.index(key)
		if i < 0 {
			t.Fatalf("iteration returned unexpected key %v", key)
		}
		if seen[i] || value != want[i].value {
			t.Fatalf("iteration key %v repeated=%v, value got %v want %v", key, seen[i], value, want[i].value)
		}
		seen[i] = true
		count++
	}
	if count != len(want) {
		t.Fatalf("iteration returned %d entries, want %d", count, len(want))
	}
}

func exerciseLifecycle[K comparable, V comparable](t *testing.T, key func(int) K, value func(int, int) V) {
	const initial = 1536 // Crosses the single-table split boundary.
	m := make(map[K]V)
	var want oracle[K, V]
	for i := range initial {
		v := value(i, 0)
		m[key(i)] = v
		want.set(key(i), v)
	}
	checkMap(t, m, want, true)

	// Delete densely populated groups, then refill the same keys and new keys.
	for i := 0; i < initial; i += 2 {
		delete(m, key(i))
		want.remove(key(i))
	}
	for i := 0; i < 2048; i += 2 {
		v := value(i, 1)
		m[key(i)] = v
		want.set(key(i), v)
	}
	checkMap(t, m, want, true)

	random := rand.New(rand.NewSource(70966))
	for step := range 4096 {
		index := random.Intn(2048)
		k := key(index)
		switch random.Intn(16) {
		case 0:
			clear(m)
			want = nil
		case 1:
			clone := maps.Clone(m)
			checkMap(t, clone, want, true)
			clone[key(4096)] = value(4096, step+2)
			if _, ok := m[key(4096)]; ok {
				t.Fatal("insertion into clone appeared in original map")
			}
			if len(want) > 0 {
				clone[want[0].key] = value(8192, step+2)
			}
			checkMap(t, m, want, false)
			m[key(4097)] = value(4097, step+2)
			if _, ok := clone[key(4097)]; ok {
				t.Fatal("insertion into original map appeared in clone")
			}
			delete(m, key(4097))
			delete(clone, key(4096))
			clear(clone)
			checkMap(t, m, want, false)
		case 2, 3, 4:
			delete(m, k)
			want.remove(k)
		default:
			v := value(index, step+2)
			m[k] = v
			want.set(k, v)
		}
		if step%127 == 0 {
			checkMap(t, m, want, false)
		}
	}
	checkMap(t, m, want, true)
	clear(m)
	checkMap(t, m, nil, true)
}

type largeKey struct {
	bytes [160]byte
}

func makeLargeKey(index int) largeKey {
	var key largeKey
	binary.LittleEndian.PutUint64(key.bytes[:], uint64(index))
	key.bytes[159] = byte(index >> 8)
	return key
}

func makeLargeValue(index, generation int) [256]byte {
	var value [256]byte
	binary.LittleEndian.PutUint64(value[:], uint64(index))
	binary.LittleEndian.PutUint64(value[8:], uint64(generation))
	value[255] = byte(generation)
	return value
}

func TestLifecycleOracle(t *testing.T) {
	t.Run("uint32", func(t *testing.T) {
		exerciseLifecycle(t, func(i int) uint32 { return uint32(i) }, func(i, generation int) uint64 {
			return uint64(i) | uint64(generation)<<32
		})
	})
	t.Run("uint64", func(t *testing.T) {
		exerciseLifecycle(t, func(i int) uint64 { return uint64(i) << 32 }, func(i, generation int) uint64 {
			return uint64(i) | uint64(generation)<<32
		})
	})
	t.Run("short-string", func(t *testing.T) {
		exerciseLifecycle(t, func(i int) string { return fmt.Sprintf("key-%d", i) }, func(i, generation int) string {
			return fmt.Sprintf("value-%d-%d", i, generation)
		})
	})
	t.Run("long-string", func(t *testing.T) {
		prefix := strings.Repeat("prefix/", 20)
		exerciseLifecycle(t, func(i int) string { return prefix + fmt.Sprint(i) }, func(i, generation int) [64]byte {
			var value [64]byte
			binary.LittleEndian.PutUint64(value[:], uint64(i))
			binary.LittleEndian.PutUint64(value[8:], uint64(generation))
			return value
		})
	})
	t.Run("indirect-key-and-value", func(t *testing.T) {
		exerciseLifecycle(t, makeLargeKey, makeLargeValue)
	})
}

// Exercise every deletion pattern before a uint64 map grows beyond one group.
// Distinct replacement keys require the first-free-slot path to reuse holes.
func TestSmallUint64HolesAndGrowth(t *testing.T) {
	key := func(index, generation int) uint64 {
		return uint64(generation)<<60 | uint64(index)<<32 | 1000
	}
	for size := 0; size <= 8; size++ {
		for deleted := 0; deleted < 1<<size; deleted++ {
			t.Run(fmt.Sprintf("size=%d/deleted=%02x", size, deleted), func(t *testing.T) {
				m := make(map[uint64]uint64)
				var want oracle[uint64, uint64]
				for index := range size {
					m[key(index, 0)] = uint64(index + 11)
					want.set(key(index, 0), uint64(index+11))
				}
				// Existing-key updates must work even when all eight slots are full.
				for index := range size {
					m[key(index, 0)] = uint64(index + 111)
					want.set(key(index, 0), uint64(index+111))
				}
				for index := range size {
					if deleted&(1<<index) != 0 {
						delete(m, key(index, 0))
						want.remove(key(index, 0))
					}
				}
				checkMap(t, m, want, true)
				for index := range size {
					if deleted&(1<<index) != 0 {
						m[key(index, 1)] = uint64(index + 211)
						want.set(key(index, 1), uint64(index+211))
					}
				}
				for index := size; index < 8; index++ {
					m[key(index, 2)] = uint64(index + 311)
					want.set(key(index, 2), uint64(index+311))
				}
				checkMap(t, m, want, true)

				clone := maps.Clone(m)
				checkMap(t, clone, want, true)
				delete(clone, want[0].key)
				checkMap(t, m, want, true)
				clear(clone)
				checkMap(t, clone, nil, true)
				clone[key(0, 3)] = 411
				checkMap(t, clone, oracle[uint64, uint64]{{key(0, 3), 411}}, true)

				m[key(8, 2)] = 319
				want.set(key(8, 2), 319)
				checkMap(t, m, want, true)
				clear(m)
				checkMap(t, m, nil, true)
			})
		}
	}
}

func TestNilAndInterfaceKeys(t *testing.T) {
	var empty map[any]any
	if value, ok := empty[nil]; value != nil || ok || len(empty) != 0 || maps.Clone(empty) != nil {
		t.Fatal("nil-map lookup, length, or clone changed")
	}
	delete(empty, nil)
	clear(empty)
	expectPanic(t, func() { empty[nil] = 1 })

	var pointer *int
	m := map[any]any{nil: "nil", pointer: "typed nil", "nil value": pointer, "zero value": nil}
	if len(m) != 4 || m[nil] != "nil" || m[pointer] != "typed nil" {
		t.Fatal("nil and typed-nil keys lost their distinct identities")
	}
	if value, ok := m["zero value"]; !ok || value != nil {
		t.Fatal("present nil value confused with absent key")
	}
	if value, ok := m["nil value"].(*int); !ok || value != nil {
		t.Fatal("typed-nil value lost its dynamic type")
	}
	for _, source := range []map[any]any{empty, m} {
		expectPanic(t, func() { _ = source[[]int{1}] })
		expectPanic(t, func() { delete(source, []int{1}) })
		expectPanic(t, func() { source[[]int{1}] = 1 })
	}
}

func expectPanic(t *testing.T, operation func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Fatal("operation completed, expected panic")
		}
	}()
	operation()
}

type payload struct {
	index int
	data  [256]byte
	next  *payload
}

func TestMapOnlyGCRoots(t *testing.T) {
	m := make(map[largeKey]*payload)
	for i := range 1536 {
		m[makeLargeKey(i)] = &payload{index: i, data: makeLargeValue(i, 7), next: &payload{index: ^i}}
	}
	clone := maps.Clone(m)
	clear(m)
	for range 3 {
		runtime.GC()
		// Reuse the key/value size classes so a missed GC root cannot rely on
		// its previous contents surviving until the next read.
		keyGarbage := make([]*largeKey, 2048)
		valueGarbage := make([]*payload, 4096)
		for i := range keyGarbage {
			key := makeLargeKey(i + 1<<20)
			keyGarbage[i] = &key
		}
		for i := range valueGarbage {
			valueGarbage[i] = &payload{index: -1}
		}
		for i := range 1536 {
			p, ok := clone[makeLargeKey(i)]
			if !ok || p == nil || p.index != i || p.data != makeLargeValue(i, 7) || p.next == nil || p.next.index != ^i {
				t.Fatalf("GC-rooted value %d lost or corrupted", i)
			}
		}
		runtime.KeepAlive(keyGarbage)
		runtime.KeepAlive(valueGarbage)
	}
	runtime.KeepAlive(clone)
}

func TestLongStringSmallMap(t *testing.T) {
	prefix, suffix := strings.Repeat("a", 96), strings.Repeat("z", 96)
	key := func(i int) string { return prefix + fmt.Sprint(i) + suffix }
	m := make(map[string]int)
	for i := range 8 {
		m[key(i)] = i + 1
	}
	// Identical lengths, prefixes, and suffixes force multiple quick-match
	// candidates before the full hashing path in a small string-key map.
	for i := range 8 {
		if value, ok := m[key(i)]; !ok || value != i+1 {
			t.Fatalf("long-string lookup %d got (%d, %v)", i, value, ok)
		}
	}
	delete(m, key(3))
	for _, i := range []int{3, 8} {
		if value, ok := m[key(i)]; ok || value != 0 {
			t.Fatalf("long-string missing lookup %d got (%d, %v)", i, value, ok)
		}
	}
	clear(m)
	m[key(4)] = 5
	if value, ok := m[key(4)]; !ok || value != 5 {
		t.Fatal("single long-string quick-match lookup failed after clear")
	}
}

func TestIterationMutation(t *testing.T) {
	for round := range 8 {
		m := make(map[int]int)
		for i := range 1536 {
			m[i] = i + 1000
		}
		seen := make([]bool, 4096)
		count, retained := 0, -1
		for key, value := range m {
			if key < 0 || key >= len(seen) || seen[key] || value != key+1000 {
				t.Fatalf("round %d unexpected iteration entry (%d, %d)", round, key, value)
			}
			if count > 0 && key < 1536 {
				t.Fatalf("deleted original key %d returned after mutation", key)
			}
			seen[key] = true
			if count == 0 {
				retained = key
				for i := range 1536 {
					if i != key {
						delete(m, i)
					}
				}
				for i := 1536; i < 4096; i++ {
					m[i] = i + 1000
				}
			}
			count++
		}
		if retained < 0 || len(m) != 2561 || m[retained] != retained+1000 {
			t.Fatalf("round %d final contents length %d retained %d", round, len(m), retained)
		}
		for i := 1536; i < 4096; i++ {
			if value, ok := m[i]; !ok || value != i+1000 {
				t.Fatalf("new key %d lost after iteration growth", i)
			}
		}
		count = 0
		for range m {
			clear(m)
			count++
		}
		if count != 1 || len(m) != 0 {
			t.Fatalf("clear during range yielded %d entries, final length %d", count, len(m))
		}
	}
}

func TestNaNAndSignedZero(t *testing.T) {
	m := make(map[float64]int)
	nan := math.Float64frombits(0x7ff8000000000001)
	for i := range 1024 {
		m[nan] = i
		m[float64(i)] = i + 1024
	}
	if _, ok := m[nan]; ok {
		t.Fatal("NaN unexpectedly found by lookup")
	}
	delete(m, nan)
	if len(m) != 2048 {
		t.Fatalf("NaN insertion/delete changed length to %d", len(m))
	}
	clone := maps.Clone(m)
	clear(m)
	for i := range 1024 {
		if value, ok := clone[float64(i)]; !ok || value != i+1024 {
			t.Fatalf("normal floating key %d lost after clone", i)
		}
	}
	seen := make([]bool, 2048)
	count := 0
	for key, value := range clone {
		if value < 0 {
			t.Fatalf("unexpected negative entry value %d", value)
		}
		if value < len(seen) {
			if seen[value] || math.IsNaN(key) != (value < 1024) {
				t.Fatalf("original entry %d repeated or changed class", value)
			}
			if value >= 1024 && key != float64(value-1024) {
				t.Fatalf("original value %d moved to key %v", value, key)
			}
			seen[value] = true
		} else if math.IsNaN(key) || key < 1024 || key >= 4096 || value != int(key)+2048 {
			t.Fatalf("unexpected added entry (%v, %d)", key, value)
		}
		if count == 0 {
			for i := 1024; i < 4096; i++ {
				clone[float64(i)] = i + 2048
			}
		}
		count++
	}
	for i, present := range seen {
		if !present {
			t.Fatalf("original entry %d lost during clone/growth iteration", i)
		}
	}
	zero := map[float64]int{0: 1}
	zero[math.Copysign(0, -1)] = 2
	for key, value := range zero {
		if len(zero) != 1 || !math.Signbit(key) || value != 2 {
			t.Fatal("signed-zero update failed to retain the replacement key and value")
		}
	}
}
