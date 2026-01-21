package resolvers

import (
	"testing"
)

func TestExtractFixtureIDsFromRelatedIDs_ValidJSON(t *testing.T) {
	tests := []struct {
		name       string
		relatedIDs *string
		expected   []string
	}{
		{
			name:       "single fixture ID",
			relatedIDs: toStringPtr(`["fixture-1"]`),
			expected:   []string{"fixture-1"},
		},
		{
			name:       "multiple fixture IDs",
			relatedIDs: toStringPtr(`["fixture-1", "fixture-2", "fixture-3"]`),
			expected:   []string{"fixture-1", "fixture-2", "fixture-3"},
		},
		{
			name:       "empty array",
			relatedIDs: toStringPtr(`[]`),
			expected:   []string{},
		},
		{
			name:       "UUIDs",
			relatedIDs: toStringPtr(`["550e8400-e29b-41d4-a716-446655440000", "6ba7b810-9dad-11d1-80b4-00c04fd430c8"]`),
			expected:   []string{"550e8400-e29b-41d4-a716-446655440000", "6ba7b810-9dad-11d1-80b4-00c04fd430c8"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractFixtureIDsFromRelatedIDs(tt.relatedIDs)

			if len(result) != len(tt.expected) {
				t.Errorf("extractFixtureIDsFromRelatedIDs() returned %d items, expected %d", len(result), len(tt.expected))
				return
			}

			for i, id := range result {
				if id != tt.expected[i] {
					t.Errorf("extractFixtureIDsFromRelatedIDs()[%d] = %q, expected %q", i, id, tt.expected[i])
				}
			}
		})
	}
}

func TestExtractFixtureIDsFromRelatedIDs_NilAndEmpty(t *testing.T) {
	tests := []struct {
		name       string
		relatedIDs *string
	}{
		{
			name:       "nil pointer",
			relatedIDs: nil,
		},
		{
			name:       "empty string",
			relatedIDs: toStringPtr(""),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractFixtureIDsFromRelatedIDs(tt.relatedIDs)

			if result != nil {
				t.Errorf("extractFixtureIDsFromRelatedIDs() returned %v, expected nil", result)
			}
		})
	}
}

func TestExtractFixtureIDsFromRelatedIDs_InvalidJSON(t *testing.T) {
	tests := []struct {
		name       string
		relatedIDs *string
	}{
		{
			name:       "invalid JSON - not an array",
			relatedIDs: toStringPtr(`"fixture-1"`),
		},
		{
			name:       "invalid JSON - object",
			relatedIDs: toStringPtr(`{"id": "fixture-1"}`),
		},
		{
			name:       "invalid JSON - malformed",
			relatedIDs: toStringPtr(`["fixture-1", "fixture-2"`),
		},
		{
			name:       "invalid JSON - random text",
			relatedIDs: toStringPtr(`not json at all`),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractFixtureIDsFromRelatedIDs(tt.relatedIDs)

			if result != nil {
				t.Errorf("extractFixtureIDsFromRelatedIDs() returned %v for invalid JSON, expected nil", result)
			}
		})
	}
}

// toStringPtr creates a string pointer for test inputs
func toStringPtr(s string) *string {
	return &s
}
