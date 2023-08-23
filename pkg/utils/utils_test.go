package utils

import (
	"testing"
)

func Test_EqualStrings(t *testing.T) {
	testCases := []struct {
		name     string
		input1   []string
		input2   []string
		expected bool
	}{
		{
			name:     "equal slices",
			input1:   []string{"apple", "banana", "cherry"},
			input2:   []string{"apple", "banana", "cherry"},
			expected: true,
		},
		{
			name:     "different order",
			input1:   []string{"apple", "banana", "cherry"},
			input2:   []string{"cherry", "banana", "apple"},
			expected: true,
		},
		{
			name:     "different lengths",
			input1:   []string{"apple", "banana", "cherry"},
			input2:   []string{"apple", "banana"},
			expected: false,
		},
		{
			name:     "different contents",
			input1:   []string{"apple", "banana", "cherry"},
			input2:   []string{"apple", "orange", "cherry"},
			expected: false,
		},
		{
			name:     "empty slices",
			input1:   []string{},
			input2:   []string{},
			expected: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := EqualStrings(tc.input1, tc.input2)
			if result != tc.expected {
				t.Errorf("expected result to be %v, but got %v", tc.expected, result)
			}
		})
	}
}
