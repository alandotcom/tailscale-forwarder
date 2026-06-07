package util

import (
	"strings"
	"unicode"
)

// SanitizeHostname converts an arbitrary string into a hostname-safe label:
// letters and digits are lowercased, every other character becomes a hyphen,
// runs of hyphens are collapsed to one, and leading/trailing hyphens are
// trimmed. The result may be empty (e.g. for input with no letters or digits),
// which callers must treat as invalid.
func SanitizeHostname(input string) string {
	var result strings.Builder

	for _, char := range input {
		if unicode.IsLetter(char) || unicode.IsDigit(char) {
			result.WriteRune(unicode.ToLower(char))
		} else {
			result.WriteRune('-')
		}
	}

	sanitized := result.String()
	for strings.Contains(sanitized, "--") {
		sanitized = strings.ReplaceAll(sanitized, "--", "-")
	}

	sanitized = strings.Trim(sanitized, "-")

	return sanitized
}
