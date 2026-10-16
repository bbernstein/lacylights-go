package gdtf

import (
	"fmt"
	"strconv"
	"strings"
)

// maxValueBytes bounds DMX value resolution (GDTF uses up to 4 bytes:
// coarse, fine, ultra, uber).
const maxValueBytes = 4

// dmxValue is a GDTF DMXValue: "v/n" (byte mirroring) or "v/ns" (byte
// shifting), where n is the byte count v is expressed in.
type dmxValue struct {
	value uint64
	bytes int
	shift bool
}

// parseDMXValue parses a DMXValue. ok is false for an empty value or the
// special value "None". A bare integer is read as a 1-byte value.
func parseDMXValue(s string) (v dmxValue, ok bool, err error) {
	s = strings.TrimSpace(s)
	if s == "" || strings.EqualFold(s, "none") {
		return dmxValue{}, false, nil
	}
	num, count, hasCount := strings.Cut(s, "/")
	v.bytes = 1
	if hasCount {
		if strings.HasSuffix(count, "s") {
			v.shift = true
			count = strings.TrimSuffix(count, "s")
		}
		n, err := strconv.Atoi(count)
		if err != nil || n < 1 || n > maxValueBytes {
			return dmxValue{}, false, fmt.Errorf("invalid DMX value %q", s)
		}
		v.bytes = n
	}
	val, err := strconv.ParseUint(strings.TrimSpace(num), 10, 64)
	if err != nil {
		return dmxValue{}, false, fmt.Errorf("invalid DMX value %q", s)
	}
	if limit := maxFor(v.bytes); val > limit {
		val = limit
	}
	v.value = val
	return v, true, nil
}

// maxFor is the largest value of an n-byte DMX value.
func maxFor(n int) uint64 { return (uint64(1) << (8 * uint(n))) - 1 }

// to converts the value to an n-byte channel resolution. Widening uses byte
// mirroring by default (128/1 in 16 bit is 0x8080, 255/1 is 0xFFFF) or byte
// shifting for "/ns" values (255/1s in 16 bit is 0xFF00). Narrowing keeps
// the most significant bytes.
func (v dmxValue) to(n int) uint64 {
	if n < 1 {
		n = 1
	}
	switch {
	case n == v.bytes:
		return v.value
	case n < v.bytes:
		return v.value >> (8 * uint(v.bytes-n))
	case v.shift:
		return v.value << (8 * uint(n-v.bytes))
	}
	// Mirroring: repeat the source bytes, most significant first.
	src := splitBytes(v.value, v.bytes)
	var out uint64
	for i := 0; i < n; i++ {
		out = out<<8 | uint64(src[i%v.bytes])
	}
	return out
}

// splitBytes returns the n bytes of value, most significant first.
func splitBytes(value uint64, n int) []int {
	out := make([]int, n)
	for i := n - 1; i >= 0; i-- {
		out[i] = int(value & 0xFF)
		value >>= 8
	}
	return out
}
