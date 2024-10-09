package cli

import (
	"testing"
)

func TestEscapeIfNeeded(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "AlreadyEscaped",
			input:    "hello%20world",
			expected: "hello%20world",
		},
		{
			name:     "Unescaped",
			input:    "hello world",
			expected: "hello+world",
		},
		{
			name:     "PartiallyEscaped",
			input:    "hello%20world!",
			expected: "hello%20world!",
		},
		{
			name:     "EmptyString",
			input:    "",
			expected: "",
		},
		{
			name:     "SpecialCharacters",
			input:    "hello@world.com",
			expected: "hello%40world.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := EscapeIfNeeded(tt.input)
			if result != tt.expected {
				t.Errorf("EscapeIfNeeded(%q) actual = %q; want = %q", tt.input, result, tt.expected)
			}
		})
	}
}
