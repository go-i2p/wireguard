package testutil

import (
	"strings"
)

// ErrorContains checks if an error contains a specific substring.
// This is a utility function for testing error messages.
func ErrorContains(err error, substring string) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), substring)
}
