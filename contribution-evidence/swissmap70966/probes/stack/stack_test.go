package swissmapstack

import "testing"

var stackMapSink uint64

const longPrefix = "012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789"

//go:noinline
func stackUint64One(key uint64) uint64 {
	m := map[uint64]uint64{1000: 11}
	return m[key]
}

//go:noinline
func stackUint64Six(key uint64) uint64 {
	m := map[uint64]uint64{1000: 11, 1001: 12, 1002: 13, 1003: 14, 1004: 15, 1005: 16}
	return m[key]
}

//go:noinline
func stackUint64Eight(key uint64) uint64 {
	m := map[uint64]uint64{1000: 11, 1001: 12, 1002: 13, 1003: 14, 1004: 15, 1005: 16, 1006: 17, 1007: 18}
	return m[key]
}

//go:noinline
func stackStringShort(key string) uint64 {
	m := map[string]uint64{"a": 11, "b": 12, "c": 13, "d": 14, "e": 15, "f": 16}
	return m[key]
}

//go:noinline
func stackStringLong(key string) uint64 {
	m := map[string]uint64{
		longPrefix + "a": 11,
		longPrefix + "b": 12,
		longPrefix + "c": 13,
		longPrefix + "d": 14,
		longPrefix + "e": 15,
		longPrefix + "f": 16,
	}
	return m[key]
}

func TestStackMapAllocations(t *testing.T) {
	integerCases := []struct {
		name   string
		lookup func(uint64) uint64
	}{
		{"Uint64One", stackUint64One},
		{"Uint64Six", stackUint64Six},
		{"Uint64Eight", stackUint64Eight},
	}
	for _, test := range integerCases {
		t.Run(test.name, func(t *testing.T) {
			if test.lookup(1000) != 11 || test.lookup(2000) != 0 {
				t.Fatal("present or absent lookup result changed")
			}
			allocations := testing.AllocsPerRun(1000, func() {
				stackMapSink = test.lookup(1000)
			})
			t.Logf("allocations/run=%v", allocations)
			if allocations != 0 {
				t.Fatalf("got %v allocations per call, want zero", allocations)
			}
		})
	}
	stringCases := []struct {
		name   string
		lookup func(string) uint64
		key    string
	}{
		{"StringShort", stackStringShort, "a"},
		{"StringLong", stackStringLong, longPrefix + "a"},
	}
	for _, test := range stringCases {
		t.Run(test.name, func(t *testing.T) {
			if test.lookup(test.key) != 11 || test.lookup("absent") != 0 {
				t.Fatal("present or absent lookup result changed")
			}
			allocations := testing.AllocsPerRun(1000, func() {
				stackMapSink = test.lookup(test.key)
			})
			t.Logf("allocations/run=%v", allocations)
			if allocations != 0 {
				t.Fatalf("got %v allocations per call, want zero", allocations)
			}
		})
	}
}

// The function boundary and rotating lookup inputs preserve the same call and
// observable-result contract for baseline and patched toolchains.
func BenchmarkStackMap(b *testing.B) {
	integerCases := []struct {
		name   string
		lookup func(uint64) uint64
		keys   [8]uint64
	}{
		{"Uint64One", stackUint64One, [8]uint64{1000, 2000, 1000, 2000, 1000, 2000, 1000, 2000}},
		{"Uint64Six", stackUint64Six, [8]uint64{1000, 1001, 1002, 1003, 1004, 1005, 2000, 2001}},
		{"Uint64Eight", stackUint64Eight, [8]uint64{1000, 1001, 1002, 1003, 1004, 1005, 1006, 1007}},
	}
	for _, test := range integerCases {
		b.Run(test.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				stackMapSink = test.lookup(test.keys[i&7])
			}
		})
	}
	stringCases := []struct {
		name   string
		lookup func(string) uint64
		prefix string
	}{
		{"StringShort", stackStringShort, ""},
		{"StringLong", stackStringLong, longPrefix},
	}
	for _, test := range stringCases {
		b.Run(test.name, func(b *testing.B) {
			keys := [8]string{
				test.prefix + "a", test.prefix + "b", test.prefix + "c", test.prefix + "d",
				test.prefix + "e", test.prefix + "f", test.prefix + "absent0", test.prefix + "absent1",
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				stackMapSink = test.lookup(keys[i&7])
			}
		})
	}
}
