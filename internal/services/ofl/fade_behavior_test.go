package ofl

import "testing"

func TestMapFadeBehavior_Exported(t *testing.T) {
	tests := []struct {
		channelType string
		discrete    bool
		want        string
	}{
		{"INTENSITY", false, "FADE"},
		{"INTENSITY", true, "SNAP"},
		{"GOBO", false, "SNAP"},
		{"PAN", false, "FADE"},
	}
	for _, tt := range tests {
		if got := MapFadeBehavior(tt.channelType, tt.discrete); got != tt.want {
			t.Errorf("MapFadeBehavior(%q, %v) = %q, want %q", tt.channelType, tt.discrete, got, tt.want)
		}
	}
}
