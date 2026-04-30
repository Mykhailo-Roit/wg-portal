package domain

import (
	"strings"
	"unicode/utf8"
)

// SanitizeString trims leading and trailing whitespace, strips all control characters
// (runes with value < 0x20), and truncates the result to maxLen runes (rune-aware, not byte-aware).
// If maxLen <= 0, returns "".
func SanitizeString(s string, maxLen int) string {
	if maxLen <= 0 {
		return ""
	}

	// Trim leading/trailing whitespace first
	s = strings.TrimSpace(s)

	// Strip all runes with value < 0x20 (control characters)
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if r >= 0x20 {
			b.WriteRune(r)
		}
	}
	s = b.String()

	// Truncate to maxLen runes (rune-aware)
	if utf8.RuneCountInString(s) > maxLen {
		runes := []rune(s)
		s = string(runes[:maxLen])
	}

	// Re-trim after truncation: truncation can expose a trailing space that was
	// previously in the middle of the string (e.g. "A A" truncated to 2 → "A ").
	s = strings.TrimSpace(s)

	return s
}

// SanitizeEmail applies SanitizeString first, then returns "" if the original s contains
// \r (U+000D) or \n (U+000A), or if the sanitized result contains no '@' character.
func SanitizeEmail(s string, maxLen int) string {
	// Check the original string for CR or LF before sanitization
	if strings.ContainsRune(s, '\r') || strings.ContainsRune(s, '\n') {
		return ""
	}

	sanitized := SanitizeString(s, maxLen)

	// Return "" if the sanitized result contains no '@'
	if !strings.ContainsRune(sanitized, '@') {
		return ""
	}

	return sanitized
}

// SanitizePhone applies SanitizeString first, then removes all characters not in the
// set [0-9+\-() .]. Returns "" if the result after filtering is empty.
func SanitizePhone(s string, maxLen int) string {
	sanitized := SanitizeString(s, maxLen)

	// Remove all characters not in [0-9+\-() .]
	var b strings.Builder
	b.Grow(len(sanitized))
	for _, r := range sanitized {
		if isAllowedPhoneRune(r) {
			b.WriteRune(r)
		}
	}
	result := strings.TrimSpace(b.String())

	if result == "" {
		return ""
	}

	return result
}

// isAllowedPhoneRune reports whether r is in the allowed phone character set [0-9+\-() .].
func isAllowedPhoneRune(r rune) bool {
	switch {
	case r >= '0' && r <= '9':
		return true
	case r == '+', r == '-', r == '(', r == ')', r == ' ', r == '.':
		return true
	default:
		return false
	}
}

// SanitizeIdentifier applies SanitizeString first, then returns "" if the result equals
// the reserved value "all" (case-sensitive, exact match).
func SanitizeIdentifier(s string, maxLen int) string {
	sanitized := SanitizeString(s, maxLen)

	if sanitized == "all" {
		return ""
	}

	return sanitized
}
