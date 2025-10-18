package util

import "strings"

// NormalizeQuery lowercases and trims lightweight punctuation/spacing.
func NormalizeQuery(q string) string {
	s := strings.ToLower(q)
	s = strings.TrimSpace(s)
	// Normalize common separators to single spaces
	replacers := []string{"-", "_", "/", "."}
	for _, r := range replacers {
		s = strings.ReplaceAll(s, r, " ")
	}
	s = strings.Join(strings.Fields(s), " ")
	return s
}

// ExpandQueryVariants generates simple, non-hardcoded variants to improve recall
// without maintaining a manual synonyms list. Examples:
//   - "key-vault" -> ["key vault", "keyvault"]
//   - "private endpoint" -> ["private endpoint", "privateendpoint"]
func ExpandQueryVariants(q string) []string {
	base := strings.TrimSpace(q)
	if base == "" {
		return []string{""}
	}
	variants := map[string]struct{}{}
	add := func(s string) {
		if s != "" {
			variants[s] = struct{}{}
		}
	}

	add(base)
	// De-hyphenate/underscore to spaces
	spaced := strings.NewReplacer("-", " ", "_", " ", "/", " ").Replace(base)
	add(strings.Join(strings.Fields(spaced), " "))
	// Remove spaces entirely
	add(strings.ReplaceAll(spaced, " ", ""))

	// If already single token, add a split-at-caps variant (best-effort)
	// Keep minimal to avoid over-expansion (not implementing full camelCase here)

	out := make([]string, 0, len(variants))
	for v := range variants {
		out = append(out, strings.ToLower(v))
	}
	return out
}
