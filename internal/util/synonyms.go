package util

import "strings"

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
	spaced := strings.NewReplacer("-", " ", "_", " ", "/", " ").Replace(base)
	add(strings.Join(strings.Fields(spaced), " "))
	add(strings.ReplaceAll(spaced, " ", ""))

	out := make([]string, 0, len(variants))
	for v := range variants {
		out = append(out, strings.ToLower(v))
	}
	return out
}
