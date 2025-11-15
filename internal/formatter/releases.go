package formatter

import (
	"fmt"
	"strings"
	"time"

	"github.com/cloudnationhq/az-cn-go-wammcp/internal/database"
)

func ReleaseSummary(moduleName string, release *database.ModuleRelease, entries []database.ModuleReleaseEntry) string {
	if release == nil {
		return "Module Release Summary\n- No release metadata available"
	}

	name := moduleName
	if name == "" {
		name = "cloudnationhq modules"
	}

	var b strings.Builder
	b.WriteString("Module Release Summary\n")
	b.WriteString(fmt.Sprintf("- Module: %s\n", name))
	b.WriteString(fmt.Sprintf("- Range: %s\n", renderRange(release)))
	b.WriteString(fmt.Sprintf("- Date: %s\n", releaseDateOrFallback(release)))

	sections := groupEntriesBySection(entries)
	if len(sections.order) == 0 {
		b.WriteString("- No categorized entries found\n")
		return b.String()
	}

	for _, section := range sections.order {
		b.WriteString(fmt.Sprintf("- %s\n", section))
		for _, title := range sections.entries[section] {
			b.WriteString(fmt.Sprintf("    - %s\n", title))
		}
	}

	return b.String()
}

type sectionGrouping struct {
	order   []string
	entries map[string][]string
}

func groupEntriesBySection(entries []database.ModuleReleaseEntry) sectionGrouping {
	grouping := sectionGrouping{
		order:   []string{},
		entries: make(map[string][]string),
	}

	fallbackOrder := []string{"Features", "Enhancements", "Bug Fixes", "Breaking Changes", "Security"}
	seenFallback := make(map[string]bool)
	appearanceOrder := []string{}
	appearanceTracker := make(map[string]bool)

	for _, entry := range entries {
		section := strings.TrimSpace(entry.Section)
		if section == "" {
			section = "Other"
		}
		grouping.entries[section] = append(grouping.entries[section], entry.Title)
		if !appearanceTracker[section] {
			appearanceTracker[section] = true
			appearanceOrder = append(appearanceOrder, section)
		}
	}

	for _, preferred := range fallbackOrder {
		if titles, ok := grouping.entries[preferred]; ok && len(titles) > 0 {
			grouping.order = append(grouping.order, preferred)
			seenFallback[preferred] = true
		}
	}

	for _, section := range appearanceOrder {
		if seenFallback[section] {
			continue
		}
		grouping.order = append(grouping.order, section)
	}

	return grouping
}

func renderRange(release *database.ModuleRelease) string {
	head := formatTag(release.Tag, release.CommitSHA.String)
	if release.PreviousTag.Valid {
		prev := formatTag(release.PreviousTag.String, release.PreviousCommitSHA.String)
		if prev != "" {
			return fmt.Sprintf("%s → %s", prev, head)
		}
	}
	return head
}

func formatTag(tag, sha string) string {
	tag = strings.TrimSpace(tag)
	if tag == "" {
		return ""
	}
	if sha != "" {
		return fmt.Sprintf("%s (%s)", tag, shortSHA(sha))
	}
	return tag
}

func releaseDateOrFallback(release *database.ModuleRelease) string {
	if release.ReleaseDate.Valid && release.ReleaseDate.String != "" {
		if t, err := time.Parse("2006-01-02", release.ReleaseDate.String); err == nil {
			return t.Format("January 2, 2006")
		}
		return release.ReleaseDate.String
	}
	return "unknown"
}

func shortSHA(sha string) string {
	sha = strings.TrimSpace(sha)
	if len(sha) >= 7 {
		return sha[:7]
	}
	return sha
}
