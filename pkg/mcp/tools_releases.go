package mcp

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
	"regexp"
	"strings"

	"github.com/cloudnationhq/az-cn-go-wammcp/internal/database"
	"github.com/cloudnationhq/az-cn-go-wammcp/internal/formatter"
	"github.com/cloudnationhq/az-cn-go-wammcp/internal/indexer"
)

type releaseSummaryArgs struct {
	ModuleName string `json:"module_name"`
	Version    string `json:"version"`
}

type releaseSnippetArgs struct {
	ModuleName    string `json:"module_name"`
	Version       string `json:"version"`
	Query         string `json:"query"`
	MaxContext    int    `json:"max_context_lines"`
	FallbackMatch string `json:"fallback_match"`
}

type backfillReleaseArgs struct {
	ModuleName string `json:"module_name"`
	Version    string `json:"version"`
}

func (s *Server) handleGetReleaseSummary(args any) map[string]any {
	if err := s.ensureDB(); err != nil {
		return ErrorResponse(fmt.Sprintf("Failed to initialize database: %v", err))
	}

	params, err := UnmarshalArgs[releaseSummaryArgs](args)
	if err != nil || strings.TrimSpace(params.ModuleName) == "" {
		return ErrorResponse("module_name is required")
	}

	module, err := s.resolveModule(params.ModuleName)
	if err != nil {
		return ErrorResponse(fmt.Sprintf("Module '%s' not found", params.ModuleName))
	}

	var (
		release *database.ModuleRelease
		entries []database.ModuleReleaseEntry
	)

	version := strings.TrimSpace(params.Version)
	if version == "" {
		release, entries, err = s.db.GetLatestModuleReleaseWithEntries(module.ID)
	} else {
		versionOnly := strings.TrimPrefix(version, "v")
		release, entries, err = s.db.GetModuleReleaseWithEntriesByVersion(module.ID, versionOnly)
		if err != nil {
			tag := version
			if !strings.HasPrefix(strings.ToLower(tag), "v") {
				tag = "v" + tag
			}
			release, entries, err = s.db.GetModuleReleaseWithEntriesByTag(module.ID, tag)
		}
	}

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			if version == "" {
				return ErrorResponse(fmt.Sprintf("No release metadata available for %s. Run a sync first.", module.Name))
			}
			return ErrorResponse(fmt.Sprintf("No release metadata found for %s %s", module.Name, version))
		}
		return ErrorResponse(fmt.Sprintf("Failed to load release metadata: %v", err))
	}

	name := module.FullName
	if name == "" {
		name = module.Name
	}

	summary := formatter.ReleaseSummary(name, release, entries)
	return SuccessResponse(summary)
}

func (s *Server) handleGetReleaseSnippet(args any) map[string]any {
	if err := s.ensureDB(); err != nil {
		return ErrorResponse(fmt.Sprintf("Failed to initialize database: %v", err))
	}

	params, err := UnmarshalArgs[releaseSnippetArgs](args)
	if err != nil || strings.TrimSpace(params.ModuleName) == "" {
		return ErrorResponse("module_name is required")
	}
	if strings.TrimSpace(params.Version) == "" || strings.TrimSpace(params.Query) == "" {
		return ErrorResponse("version and query are required")
	}

	module, err := s.resolveModule(params.ModuleName)
	if err != nil {
		return ErrorResponse(fmt.Sprintf("Module '%s' not found", params.ModuleName))
	}

	release, entries, err := s.lookupModuleRelease(module.ID, params.Version)
	if err != nil {
		return ErrorResponse(err.Error())
	}

	entry := selectReleaseEntry(entries, params.Query, params.FallbackMatch)
	if entry == nil {
		return ErrorResponse("No matching release entry found for that query")
	}

	if !release.PreviousTag.Valid || release.PreviousTag.String == "" {
		return ErrorResponse("Unable to compute diff for the earliest release (missing previous tag)")
	}

	if s.syncer == nil {
		return ErrorResponse("Syncer is not initialized; run a sync first")
	}

	compare, err := s.syncer.CompareTags(module.FullName, release.PreviousTag.String, release.Tag)
	if err != nil {
		return ErrorResponse(fmt.Sprintf("Failed to fetch GitHub compare diff: %v", err))
	}

	filename, patch := locatePatchForEntry(compare, entry, params.Query)
	if filename == "" || patch == "" {
		return ErrorResponse("Diff data not available for that entry. Try a different query or rerun the incremental sync.")
	}

	maxLines := params.MaxContext
	if maxLines <= 0 {
		maxLines = 24
	}

	trimmed, truncated := trimPatchLines(patch, maxLines)
	moduleName := module.FullName
	if moduleName == "" {
		moduleName = module.Name
	}
	text := formatReleaseSnippetResponse(moduleName, release, entry, filename, trimmed, truncated, maxLines)
	return SuccessResponse(text)
}

func (s *Server) handleBackfillRelease(args any) map[string]any {
	if err := s.ensureDB(); err != nil {
		return ErrorResponse(fmt.Sprintf("Failed to initialize database: %v", err))
	}

	params, err := UnmarshalArgs[backfillReleaseArgs](args)
	if err != nil || strings.TrimSpace(params.ModuleName) == "" || strings.TrimSpace(params.Version) == "" {
		return ErrorResponse("module_name and version are required")
	}

	module, err := s.resolveModule(params.ModuleName)
	if err != nil {
		return ErrorResponse(fmt.Sprintf("Module '%s' not found", params.ModuleName))
	}

	file, err := s.getModuleChangelog(module)
	if err != nil {
		return ErrorResponse("CHANGELOG.md not found in local index; run a full sync first")
	}

	raw := strings.TrimSpace(file.Content)
	if raw == "" {
		return ErrorResponse("CHANGELOG.md is empty")
	}

	ver := strings.TrimSpace(params.Version)
	normalized := strings.TrimPrefix(strings.ToLower(ver), "v")
	tag := ver
	if !strings.HasPrefix(strings.ToLower(tag), "v") {
		tag = "v" + tag
	}

	block, date, ok := extractReleaseBlock(raw, normalized)
	if !ok {
		return ErrorResponse(fmt.Sprintf("Version %s not found in changelog", ver))
	}

	entries := parseReleaseEntriesFromBlock(block)
	rel := &database.ModuleRelease{
		ModuleID:    module.ID,
		Version:     normalized,
		Tag:         tag,
		ReleaseDate: sql.NullString{String: date, Valid: date != ""},
	}

	releaseID, err := s.db.UpsertModuleRelease(rel)
	if err != nil {
		return ErrorResponse(fmt.Sprintf("Failed to store release: %v", err))
	}

	if err := s.db.ReplaceModuleReleaseEntries(releaseID, entries); err != nil {
		return ErrorResponse(fmt.Sprintf("Failed to store release entries: %v", err))
	}

	return SuccessResponse(fmt.Sprintf("Backfilled release %s for %s with %d entries", tag, module.Name, len(entries)))
}

func (s *Server) lookupModuleRelease(moduleID int64, versionInput string) (*database.ModuleRelease, []database.ModuleReleaseEntry, error) {
	version := strings.TrimSpace(versionInput)
	if version == "" {
		return nil, nil, fmt.Errorf("version is required")
	}

	relVersion := strings.TrimPrefix(version, "v")
	release, entries, err := s.db.GetModuleReleaseWithEntriesByVersion(moduleID, relVersion)
	if err == nil {
		return release, entries, nil
	}
	tag := version
	if !strings.HasPrefix(strings.ToLower(tag), "v") {
		tag = "v" + tag
	}
	release, entries, err = s.db.GetModuleReleaseWithEntriesByTag(moduleID, tag)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil, fmt.Errorf("no release metadata found for version %s", version)
		}
		return nil, nil, fmt.Errorf("failed to load release metadata: %w", err)
	}
	return release, entries, nil
}

func (s *Server) getModuleChangelog(module *database.Module) (*database.ModuleFile, error) {
	candidates := []string{"CHANGELOG.md", "changelog.md", "docs/CHANGELOG.md", "docs/changelog.md"}
	for _, candidate := range candidates {
		file, err := s.db.GetFile(module.Name, candidate)
		if err == nil {
			return file, nil
		}
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		return nil, err
	}
	return nil, sql.ErrNoRows
}

func (s *Server) releaseSummaryIfUpdated(updated []string) string {
	if len(updated) != 1 {
		return ""
	}
	moduleName := updated[0]
	module, err := s.db.GetModule(moduleName)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			log.Printf("Warning: unable to load module metadata for release summary: %v", err)
		}
		return ""
	}
	release, entries, err := s.db.GetLatestModuleReleaseWithEntries(module.ID)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			log.Printf("Warning: failed to load latest release summary: %v", err)
		}
		return ""
	}
	name := module.FullName
	if name == "" {
		name = module.Name
	}
	return formatter.ReleaseSummary(name, release, entries)
}

func selectReleaseEntry(entries []database.ModuleReleaseEntry, query string, fallback string) *database.ModuleReleaseEntry {
	normalized := strings.ToLower(strings.TrimSpace(query))
	slugged := slugifyToken(normalized)
	if normalized == "" {
		return nil
	}

	for idx := range entries {
		entry := &entries[idx]
		if entry.Identifier.Valid {
			if strings.EqualFold(entry.Identifier.String, normalized) || slugifyToken(entry.Identifier.String) == slugged {
				return entry
			}
		}
	}

	if fallback != "" {
		fallbackLower := strings.ToLower(fallback)
		for idx := range entries {
			entry := &entries[idx]
			if strings.Contains(strings.ToLower(entry.Title), fallbackLower) {
				return entry
			}
		}
	}

	for idx := range entries {
		entry := &entries[idx]
		if strings.Contains(strings.ToLower(entry.Title), normalized) {
			return entry
		}
	}

	return nil
}

type releaseEntryTargets struct {
	filenameTokens       []string
	contentTokens        []string
	fallbackContentToken string
}

func buildReleaseEntryTargets(entry *database.ModuleReleaseEntry, query string) releaseEntryTargets {
	targets := releaseEntryTargets{}

	if entry.Identifier.Valid {
		id := strings.ToLower(entry.Identifier.String)
		targets.filenameTokens = append(targets.filenameTokens, id)
		targets.contentTokens = append(targets.contentTokens, id)
		for _, part := range tokenizeIdentifier(id) {
			targets.filenameTokens = append(targets.filenameTokens, part)
			targets.contentTokens = append(targets.contentTokens, part)
		}
	}

	if query != "" {
		for _, token := range tokenizeIdentifier(strings.ToLower(query)) {
			targets.filenameTokens = append(targets.filenameTokens, token)
			targets.contentTokens = append(targets.contentTokens, token)
		}
		targets.fallbackContentToken = strings.ToLower(query)
	} else if entry.Title != "" {
		targets.fallbackContentToken = strings.ToLower(entry.Title)
	}

	targets.filenameTokens = uniqueStrings(targets.filenameTokens)
	targets.contentTokens = uniqueStrings(targets.contentTokens)
	return targets
}

func locatePatchForEntry(compare *indexer.GitHubCompareResult, entry *database.ModuleReleaseEntry, query string) (string, string) {
	if compare == nil {
		return "", ""
	}

	targets := buildReleaseEntryTargets(entry, query)
	bestScore := -1 << 30
	bestFile := ""
	bestPatch := ""

	for _, file := range compare.Files {
		if file.Patch == "" {
			continue
		}
		score := scorePatchCandidate(file.Filename, file.Patch, targets)
		if score > bestScore {
			bestScore = score
			bestFile = file.Filename
			bestPatch = file.Patch
		}
	}

	return bestFile, bestPatch
}

func scorePatchCandidate(filename string, patch string, targets releaseEntryTargets) int {
	lowerPath := strings.ToLower(strings.ReplaceAll(filename, "\\", "/"))
	lowerPatch := strings.ToLower(patch)
	score := 0

	if strings.HasSuffix(lowerPath, ".tf") {
		score += 150
	}
	if strings.Contains(lowerPath, "/modules/") {
		score += 20
	}
	if strings.Contains(lowerPath, "/examples/") {
		score -= 60
	}
	if strings.HasSuffix(lowerPath, ".md") || strings.Contains(lowerPath, "changelog") {
		score -= 180
	}
	if strings.Contains(lowerPath, "/test") {
		score -= 40
	}

	for _, token := range targets.filenameTokens {
		if token != "" && strings.Contains(lowerPath, token) {
			score += 35
		}
	}

	for _, token := range targets.contentTokens {
		if token != "" && strings.Contains(lowerPatch, token) {
			score += 20
		}
	}

	if targets.fallbackContentToken != "" && strings.Contains(lowerPatch, targets.fallbackContentToken) {
		score += 10
	}

	score += len(patch) / 400
	return score
}

func trimPatchLines(patch string, maxLines int) (string, bool) {
	if maxLines <= 0 {
		return patch, false
	}
	lines := strings.Split(patch, "\n")
	truncated := false
	if len(lines) > maxLines {
		lines = lines[:maxLines]
		truncated = true
	}
	return strings.Join(lines, "\n"), truncated
}

func formatReleaseSnippetResponse(moduleName string, release *database.ModuleRelease, entry *database.ModuleReleaseEntry, filename, patch string, truncated bool, maxLines int) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Release %s – %s\n", release.Version, entry.Title))
	b.WriteString(fmt.Sprintf("Module: %s\n", moduleName))
	b.WriteString(fmt.Sprintf("File: %s\n", filename))
	b.WriteString("```diff\n")
	b.WriteString(patch)
	b.WriteString("\n```")
	if truncated {
		b.WriteString(fmt.Sprintf("\n… showing first %d diff lines", maxLines))
	}
	if release.ComparisonURL.Valid && release.ComparisonURL.String != "" {
		b.WriteString(fmt.Sprintf("\nCompare: %s", release.ComparisonURL.String))
	}
	return b.String()
}

func extractReleaseBlock(changelog string, version string) (string, string, bool) {
	esc := regexp.QuoteMeta(version)
	heading := regexp.MustCompile(`(?m)^##\s*(?:\[` + esc + `\]|v?` + esc + `)\s*(?:\(([^)]+)\))?\s*$`)
	loc := heading.FindStringSubmatchIndex(changelog)
	if loc == nil {
		return "", "", false
	}
	start := loc[0]
	date := ""
	if len(loc) >= 4 && loc[2] != -1 && loc[3] != -1 {
		date = strings.TrimSpace(changelog[loc[2]:loc[3]])
	}
	next := regexp.MustCompile(`(?m)^##\s+`).FindStringIndex(changelog[start+2:])
	var end int
	if next == nil {
		end = len(changelog)
	} else {
		end = start + 2 + next[0]
	}
	block := strings.TrimSpace(changelog[start:end])
	return block, date, true
}

func parseReleaseEntriesFromBlock(block string) []database.ModuleReleaseEntry {
	lines := strings.Split(block, "\n")
	section := ""
	order := 0
	var entries []database.ModuleReleaseEntry
	for _, line := range lines {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "## ") {
			continue
		}
		if s, ok := strings.CutPrefix(t, "### "); ok {
			section = strings.TrimSpace(s)
			continue
		}
		if strings.HasPrefix(t, "-") || strings.HasPrefix(t, "*") {
			title := strings.TrimSpace(strings.TrimLeft(t, "-* "))
			if title == "" {
				continue
			}
			entries = append(entries, database.ModuleReleaseEntry{
				Section:    ifEmpty(section, "Other"),
				EntryKey:   fmt.Sprintf("%s-%04d", safeSlug(section), order),
				Title:      title,
				OrderIndex: order,
				Identifier: sql.NullString{String: slugifyToken(title), Valid: title != ""},
			})
			order++
		}
	}
	return entries
}

func ifEmpty(val, fallback string) string {
	if strings.TrimSpace(val) == "" {
		return fallback
	}
	return val
}

func safeSlug(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return "section"
	}
	b := strings.Builder{}
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else {
			b.WriteByte('-')
		}
	}
	return strings.Trim(b.String(), "-")
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{})
	var out []string
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

func tokenizeIdentifier(value string) []string {
	value = strings.ReplaceAll(value, "_", "-")
	value = strings.ReplaceAll(value, ".", "-")
	parts := strings.FieldsFunc(value, func(r rune) bool {
		return r == '-' || r == ' ' || r == ':'
	})
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			result = append(result, part)
		}
	}
	return result
}

func slugifyToken(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return ""
	}
	b := strings.Builder{}
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else {
			b.WriteByte('-')
		}
	}
	return strings.Trim(b.String(), "-")
}
