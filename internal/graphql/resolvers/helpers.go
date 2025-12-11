package resolvers

// Helper function to convert int to *int
func intPtr(i int) *int {
	return &i
}

// Helper function to convert string to *string
func stringPtr(s string) *string {
	return &s
}

// Helper function to convert string to *string, returning nil for empty strings
func stringToPointer(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
