package utils

import (
	"sort"

	"github.com/google/go-cmp/cmp"
)

// EqualStrings compares two slices of strings.
func EqualStrings(x1, x2 []string) bool {
	if len(x1) != len(x2) {
		return false
	}
	x1c := make([]string, len(x1))
	x2c := make([]string, len(x2))
	copy(x1c, x1)
	copy(x2c, x2)

	sort.Strings(x1c)
	sort.Strings(x2c)
	return cmp.Equal(x1c, x2c)
}
