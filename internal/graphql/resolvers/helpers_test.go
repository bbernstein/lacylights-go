package resolvers

import "testing"

func TestIntPtr(t *testing.T) {
	tests := []struct {
		input    int
		expected int
	}{
		{0, 0},
		{1, 1},
		{-1, -1},
		{100, 100},
		{255, 255},
		{-100, -100},
	}

	for _, tt := range tests {
		result := intPtr(tt.input)
		if result == nil {
			t.Errorf("intPtr(%d) returned nil", tt.input)
			continue
		}
		if *result != tt.expected {
			t.Errorf("intPtr(%d) = %d, want %d", tt.input, *result, tt.expected)
		}
	}
}

func TestIntPtr_IndependentPointers(t *testing.T) {
	// Test that each call returns an independent pointer
	ptr1 := intPtr(10)
	ptr2 := intPtr(10)

	if ptr1 == ptr2 {
		t.Error("intPtr should return independent pointers")
	}

	*ptr1 = 20
	if *ptr2 != 10 {
		t.Error("Modifying one pointer should not affect the other")
	}
}

func TestIntPtr_ZeroValue(t *testing.T) {
	ptr := intPtr(0)
	if ptr == nil {
		t.Fatal("intPtr(0) should not return nil")
	}
	if *ptr != 0 {
		t.Errorf("intPtr(0) = %d, want 0", *ptr)
	}
}

func TestIntPtr_MaxInt(t *testing.T) {
	maxInt := int(^uint(0) >> 1)
	ptr := intPtr(maxInt)
	if ptr == nil {
		t.Fatal("intPtr(maxInt) should not return nil")
	}
	if *ptr != maxInt {
		t.Errorf("intPtr(maxInt) = %d, want %d", *ptr, maxInt)
	}
}

func TestIntPtr_MinInt(t *testing.T) {
	minInt := -int(^uint(0)>>1) - 1
	ptr := intPtr(minInt)
	if ptr == nil {
		t.Fatal("intPtr(minInt) should not return nil")
	}
	if *ptr != minInt {
		t.Errorf("intPtr(minInt) = %d, want %d", *ptr, minInt)
	}
}

func TestIntPtr_ConsecutiveCalls(t *testing.T) {
	// Test rapid consecutive calls
	results := make([]*int, 100)
	for i := 0; i < 100; i++ {
		results[i] = intPtr(i)
	}

	// Verify all values are correct
	for i := 0; i < 100; i++ {
		if results[i] == nil {
			t.Fatalf("intPtr(%d) returned nil", i)
		}
		if *results[i] != i {
			t.Errorf("intPtr(%d) = %d, want %d", i, *results[i], i)
		}
	}

	// Verify all pointers are unique
	ptrSet := make(map[*int]bool)
	for _, ptr := range results {
		if ptrSet[ptr] {
			t.Error("intPtr returned duplicate pointer")
		}
		ptrSet[ptr] = true
	}
}

func TestIntPtr_NegativeValues(t *testing.T) {
	tests := []int{-1, -10, -100, -1000, -32768, -65536}

	for _, val := range tests {
		ptr := intPtr(val)
		if ptr == nil {
			t.Fatalf("intPtr(%d) returned nil", val)
		}
		if *ptr != val {
			t.Errorf("intPtr(%d) = %d, want %d", val, *ptr, val)
		}
	}
}

func TestIntPtr_CommonDMXValues(t *testing.T) {
	// Test common DMX channel values
	dmxValues := []int{0, 1, 127, 128, 254, 255}

	for _, val := range dmxValues {
		ptr := intPtr(val)
		if ptr == nil {
			t.Fatalf("intPtr(%d) returned nil", val)
		}
		if *ptr != val {
			t.Errorf("intPtr(%d) = %d, want %d", val, *ptr, val)
		}
	}
}

func TestIntPtr_UniverseNumbers(t *testing.T) {
	// Test common universe numbers
	universes := []int{0, 1, 2, 3, 4, 15, 16, 31, 32, 63}

	for _, val := range universes {
		ptr := intPtr(val)
		if ptr == nil {
			t.Fatalf("intPtr(%d) returned nil", val)
		}
		if *ptr != val {
			t.Errorf("intPtr(%d) = %d, want %d", val, *ptr, val)
		}
	}
}

func TestIntPtr_ChannelOffsets(t *testing.T) {
	// Test channel offsets (0-511 for DMX)
	offsets := []int{0, 1, 255, 256, 510, 511}

	for _, val := range offsets {
		ptr := intPtr(val)
		if ptr == nil {
			t.Fatalf("intPtr(%d) returned nil", val)
		}
		if *ptr != val {
			t.Errorf("intPtr(%d) = %d, want %d", val, *ptr, val)
		}
	}
}
