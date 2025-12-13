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

func TestSparseChannelsEqual_IdenticalJSON(t *testing.T) {
	json1 := `[{"offset":0,"value":255},{"offset":1,"value":128}]`
	json2 := `[{"offset":0,"value":255},{"offset":1,"value":128}]`

	if !sparseChannelsEqual(json1, json2) {
		t.Error("sparseChannelsEqual should return true for identical JSON")
	}
}

func TestSparseChannelsEqual_DifferentOrder(t *testing.T) {
	// Same values but different order in the JSON array
	json1 := `[{"offset":0,"value":255},{"offset":1,"value":128}]`
	json2 := `[{"offset":1,"value":128},{"offset":0,"value":255}]`

	if !sparseChannelsEqual(json1, json2) {
		t.Error("sparseChannelsEqual should return true for same values in different order")
	}
}

func TestSparseChannelsEqual_DifferentValues(t *testing.T) {
	json1 := `[{"offset":0,"value":255},{"offset":1,"value":128}]`
	json2 := `[{"offset":0,"value":255},{"offset":1,"value":64}]`

	if sparseChannelsEqual(json1, json2) {
		t.Error("sparseChannelsEqual should return false for different values")
	}
}

func TestSparseChannelsEqual_DifferentOffsets(t *testing.T) {
	json1 := `[{"offset":0,"value":255},{"offset":1,"value":128}]`
	json2 := `[{"offset":0,"value":255},{"offset":2,"value":128}]`

	if sparseChannelsEqual(json1, json2) {
		t.Error("sparseChannelsEqual should return false for different offsets")
	}
}

func TestSparseChannelsEqual_DifferentLength(t *testing.T) {
	json1 := `[{"offset":0,"value":255},{"offset":1,"value":128}]`
	json2 := `[{"offset":0,"value":255}]`

	if sparseChannelsEqual(json1, json2) {
		t.Error("sparseChannelsEqual should return false for different number of channels")
	}
}

func TestSparseChannelsEqual_EmptyArrays(t *testing.T) {
	json1 := `[]`
	json2 := `[]`

	if !sparseChannelsEqual(json1, json2) {
		t.Error("sparseChannelsEqual should return true for two empty arrays")
	}
}

func TestSparseChannelsEqual_EmptyAndNonEmpty(t *testing.T) {
	json1 := `[]`
	json2 := `[{"offset":0,"value":255}]`

	if sparseChannelsEqual(json1, json2) {
		t.Error("sparseChannelsEqual should return false for empty vs non-empty")
	}
}

func TestSparseChannelsEqual_InvalidJSON(t *testing.T) {
	validJSON := `[{"offset":0,"value":255}]`
	invalidJSON := `not valid json`

	// Invalid JSON should be treated as empty array
	if sparseChannelsEqual(validJSON, invalidJSON) {
		t.Error("sparseChannelsEqual should return false for valid vs invalid JSON")
	}

	// Two invalid JSONs should be equal (both treated as empty)
	if !sparseChannelsEqual(invalidJSON, invalidJSON) {
		t.Error("sparseChannelsEqual should return true for two invalid JSONs (both empty)")
	}
}

func TestSparseChannelsEqual_WhitespaceDifference(t *testing.T) {
	// Same values with different whitespace
	json1 := `[{"offset":0,"value":255},{"offset":1,"value":128}]`
	json2 := `[{"offset": 0, "value": 255}, {"offset": 1, "value": 128}]`

	if !sparseChannelsEqual(json1, json2) {
		t.Error("sparseChannelsEqual should return true regardless of whitespace")
	}
}

func TestSparseChannelsEqual_ManyChannels(t *testing.T) {
	// Test with more channels, different order
	json1 := `[{"offset":0,"value":255},{"offset":5,"value":128},{"offset":10,"value":64},{"offset":15,"value":32}]`
	json2 := `[{"offset":15,"value":32},{"offset":0,"value":255},{"offset":10,"value":64},{"offset":5,"value":128}]`

	if !sparseChannelsEqual(json1, json2) {
		t.Error("sparseChannelsEqual should return true for same channels in different order")
	}
}
