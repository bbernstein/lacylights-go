package importeos

import "testing"

func TestDecodeUText_RoundTripASCII(t *testing.T) {
	got, ok := decodeUText([]string{"4800", "6900"})
	if !ok || got != "Hi" {
		t.Errorf("got %q ok=%v", got, ok)
	}
}

func TestDecodeUText_NonASCII(t *testing.T) {
	got, ok := decodeUText([]string{"E900"})
	if !ok || got != "é" {
		t.Errorf("got %q ok=%v", got, ok)
	}
}

func TestDecodeUText_BadHex(t *testing.T) {
	_, ok := decodeUText([]string{"zz"})
	if ok {
		t.Error("expected failure on non-hex token")
	}
}
