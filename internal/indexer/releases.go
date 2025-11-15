package indexer

import (
	"bufio"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"regexp"
	"strings"
	"time"

	"github.com/cloudnationhq/az-cn-go-wammcp/internal/database"
)

const maxReleaseHistory = 40

var (
	markdownLinkPattern = regexp.MustCompile(`\[([^\]]+)\]\([^)]+\)`)
	resourceNamePattern = regexp.MustCompile(`azurerm_[a-z0-9_]+`)
	moduleNamePattern   = regexp.MustCompile(`terraform-[a-z0-9\-]+`)
)

type parsedRelease struct {
	Version     string
	Tag         string
	ReleaseDate string
	Sections    []*parsedSection
}

type parsedSection struct {
	Name    string
	Entries []string
}

func (s *Syncer) captureModuleReleaseMetadata(moduleID int64, repo GitHubRepo) error {
	file, err := s.findChangelogFile(repo.Name)
	if err != nil {
		return err
	}
	if file == nil || strings.TrimSpace(file.Content) == "" {
		return nil
	}

	releases := parseChangelogReleases(file.Content)
	if len(releases) == 0 {
		return nil
	}

	var tags []GitHubTag
	if s.githubClient != nil {
		if fetched, err := s.githubClient.listTags(repo.FullName, 5); err == nil {
			tags = fetched
		} else {
			log.Printf("Warning: failed to fetch tags for %s: %v", repo.FullName, err)
		}
	}

	tagLookup := make(map[string]GitHubTag)
	for _, tag := range tags {
		normalized := normalizeTagName(tag.Name)
		if normalized != "" {
			tagLookup[normalized] = tag
		}
	}

	for idx, rel := range releases {
		record := &database.ModuleRelease{
			ModuleID:    moduleID,
			Version:     rel.Version,
			Tag:         rel.Tag,
			ReleaseDate: makeNullString(rel.ReleaseDate),
		}

		if tagInfo, ok := tagLookup[strings.ToLower(rel.Tag)]; ok {
			record.CommitSHA = makeNullString(tagInfo.Commit.SHA)
		}

		if idx+1 < len(releases) {
			prev := releases[idx+1]
			record.PreviousVersion = makeNullString(prev.Version)
			record.PreviousTag = makeNullString(prev.Tag)
			if prev.Tag != "" {
				if tagInfo, ok := tagLookup[strings.ToLower(prev.Tag)]; ok {
					record.PreviousCommitSHA = makeNullString(tagInfo.Commit.SHA)
				}
				if rel.Tag != "" {
					record.ComparisonURL = makeNullString(fmt.Sprintf("https://github.com/%s/compare/%s...%s", repo.FullName, prev.Tag, rel.Tag))
				}
			}
		}

		releaseID, err := s.db.UpsertModuleRelease(record)
		if err != nil {
			return fmt.Errorf("failed to persist release %s for %s: %w", rel.Version, repo.Name, err)
		}

		entries := buildModuleReleaseEntries(rel)
		if err := s.db.ReplaceModuleReleaseEntries(releaseID, entries); err != nil {
			return fmt.Errorf("failed to persist release entries for %s %s: %w", repo.Name, rel.Version, err)
		}
	}

	return nil
}

func (s *Syncer) findChangelogFile(moduleName string) (*database.ModuleFile, error) {
	candidates := []string{
		"CHANGELOG.md",
		"changelog.md",
		"docs/CHANGELOG.md",
		"docs/changelog.md",
	}
	for _, path := range candidates {
		if file, err := s.db.GetFile(moduleName, path); err == nil {
			return file, nil
		} else if errors.Is(err, sql.ErrNoRows) {
			continue
		} else if err != nil {
			return nil, err
		}
	}
	return nil, nil
}

func parseChangelogReleases(content string) []parsedRelease {
	scanner := bufio.NewScanner(strings.NewReader(content))
	scanner.Split(bufio.ScanLines)

	releases := []parsedRelease{}
	var current *parsedRelease
	var currentSection *parsedSection

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "## ") {
			if current != nil {
				releases = append(releases, *current)
				if len(releases) >= maxReleaseHistory {
					break
				}
			}

			rel, ok := parseReleaseHeading(line)
			if !ok {
				current = nil
				currentSection = nil
				continue
			}
			current = rel
			currentSection = nil
			continue
		}

		if current == nil {
			continue
		}

		if remainder, ok := strings.CutPrefix(line, "### "); ok {
			sectionName := strings.TrimSpace(remainder)
			currentSection = &parsedSection{Name: sectionName}
			current.Sections = append(current.Sections, currentSection)
			continue
		}

		if isListEntry(line) {
			if currentSection == nil {
				currentSection = &parsedSection{Name: "Other"}
				current.Sections = append(current.Sections, currentSection)
			}
			entry := cleanBulletText(line)
			if entry != "" {
				currentSection.Entries = append(currentSection.Entries, entry)
			}
		}
	}

	if current != nil && len(releases) < maxReleaseHistory {
		releases = append(releases, *current)
	}

	filtered := make([]parsedRelease, 0, len(releases))
	for _, rel := range releases {
		if rel.Version != "" && rel.Tag != "" {
			filtered = append(filtered, rel)
		}
	}

	return filtered
}

func parseReleaseHeading(line string) (*parsedRelease, bool) {
	trimmed := strings.TrimSpace(strings.TrimPrefix(line, "##"))
	if trimmed == "" {
		return nil, false
	}

	versionPart := trimmed
	if strings.HasPrefix(versionPart, "[") {
		if closing := strings.Index(versionPart, "]"); closing > 0 {
			versionPart = versionPart[1:closing]
		}
	} else {
		if idx := strings.Index(versionPart, " "); idx > 0 {
			versionPart = versionPart[:idx]
		}
	}

	versionPart = strings.TrimSpace(versionPart)
	if strings.EqualFold(versionPart, "unreleased") {
		return nil, false
	}

	version := normalizeVersion(versionPart)
	if version == "" {
		return nil, false
	}

	date := extractDate(trimmed)
	tag := ensureTagPrefix(versionPart)

	return &parsedRelease{
		Version:     version,
		Tag:         tag,
		ReleaseDate: date,
		Sections:    []*parsedSection{},
	}, true
}

func extractDate(line string) string {
	openIdx := strings.LastIndex(line, "(")
	closeIdx := strings.LastIndex(line, ")")
	if openIdx >= 0 && closeIdx > openIdx {
		candidate := strings.TrimSpace(line[openIdx+1 : closeIdx])
		if candidate != "" && !strings.EqualFold(candidate, "unreleased") {
			if normalized := normalizeDate(candidate); normalized != "" {
				return normalized
			}
		}
	}
	return ""
}

func normalizeDate(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}

	layouts := []string{
		"2006-01-02",
		"January 2, 2006",
		"Jan 2, 2006",
		"02 January 2006",
	}

	for _, layout := range layouts {
		if t, err := time.Parse(layout, raw); err == nil {
			return t.Format("2006-01-02")
		}
	}

	return raw
}

func normalizeVersion(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if raw[0] == 'v' || raw[0] == 'V' {
		raw = raw[1:]
	}
	if strings.EqualFold(raw, "unreleased") {
		return ""
	}
	return raw
}

func ensureTagPrefix(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if raw[0] == 'v' || raw[0] == 'V' {
		return "v" + raw[1:]
	}
	return "v" + raw
}

func normalizeTagName(tag string) string {
	tag = strings.TrimSpace(tag)
	if tag == "" {
		return ""
	}
	if tag[0] != 'v' && tag[0] != 'V' {
		tag = "v" + tag
	}
	return strings.ToLower(tag)
}

func isListEntry(line string) bool {
	trimmed := strings.TrimSpace(line)
	return strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ") || strings.HasPrefix(trimmed, "-") || strings.HasPrefix(trimmed, "*")
}

func cleanBulletText(line string) string {
	trimmed := strings.TrimSpace(line)
	if strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ") {
		trimmed = strings.TrimSpace(trimmed[2:])
	} else if strings.HasPrefix(trimmed, "-") || strings.HasPrefix(trimmed, "*") {
		trimmed = strings.TrimSpace(trimmed[1:])
	}
	if trimmed == "" {
		return ""
	}
	cleaned := markdownLinkPattern.ReplaceAllString(trimmed, "$1")
	cleaned = strings.ReplaceAll(cleaned, "`", "")
	cleaned = strings.TrimSpace(cleaned)
	return cleaned
}

func buildModuleReleaseEntries(rel parsedRelease) []database.ModuleReleaseEntry {
	entries := []database.ModuleReleaseEntry{}
	order := 0
	for _, section := range rel.Sections {
		if section == nil || len(section.Entries) == 0 {
			continue
		}
		sectionName := section.Name
		if sectionName == "" {
			sectionName = "Other"
		}
		for _, raw := range section.Entries {
			text := strings.TrimSpace(raw)
			if text == "" {
				continue
			}
			identifier := detectIdentifier(text)
			changeType := changeTypeForSection(sectionName, text)
			entryKey := fmt.Sprintf("%s-%s-%03d", slugify(sectionName), slugify(rel.Version), order)
			entries = append(entries, database.ModuleReleaseEntry{
				Section:    sectionName,
				EntryKey:   entryKey,
				Title:      text,
				Identifier: makeNullString(identifier),
				ChangeType: makeNullString(changeType),
				OrderIndex: order,
			})
			order++
		}
	}
	return entries
}

func detectIdentifier(text string) string {
	lower := strings.ToLower(text)
	if module := moduleNamePattern.FindString(lower); module != "" {
		return module
	}
	if resource := resourceNamePattern.FindString(lower); resource != "" {
		return resource
	}
	return slugify(text)
}

func changeTypeForSection(section, text string) string {
	lower := strings.ToLower(section)
	switch lower {
	case "features":
		return "feature"
	case "enhancements", "improvements":
		return "enhancement"
	case "bug fixes", "bugfixes", "bugs":
		return "bugfix"
	case "breaking changes":
		return "breaking_change"
	case "security":
		return "security"
	}
	if strings.Contains(strings.ToLower(text), "new module") {
		return "new_module"
	}
	return ""
}

func slugify(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return "item"
	}
	var builder strings.Builder
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			builder.WriteRune(r)
			continue
		}
		builder.WriteByte('-')
	}
	slug := strings.Trim(builder.String(), "-")
	if slug == "" {
		return "item"
	}
	return slug
}

func makeNullString(value string) sql.NullString {
	value = strings.TrimSpace(value)
	if value == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: value, Valid: true}
}
