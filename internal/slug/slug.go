// Package slug provides URL-safe slug generation.
package slug

import (
	"regexp"
	"strings"
)

var nonSlugChars = regexp.MustCompile(`[^a-z0-9가-힣]+`)

// Slugify converts a string to a URL-safe slug.
// Supports Korean characters (가-힣), collapses non-alphanumeric runs to dashes,
// trims leading/trailing dashes, and defaults to "task" for empty results.
func Slugify(s string) string {
	s = strings.ToLower(s)
	s = nonSlugChars.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if s == "" {
		s = "task"
	}
	return s
}
