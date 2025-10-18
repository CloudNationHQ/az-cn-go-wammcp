package formatter

import (
	"fmt"
	"sort"
	"strings"

	"github.com/cloudnationhq/az-cn-go-wammcp/internal/database"
)

// ModuleRelationshipView bundles per-module relationship data for multi-module reporting.
type ModuleRelationshipView struct {
	ModuleName    string
	Relationships []database.HCLRelationship
	Files         map[string]database.ModuleFile
}

// RelationshipAnalysis renders a human-readable summary of relationship matches
// for a given module and query term.
func RelationshipAnalysis(moduleName, term string, rels []database.HCLRelationship, files map[string]database.ModuleFile) string {
	return renderRelationshipSection("#", moduleName, term, rels, files)
}

// RelationshipAnalysisAcross renders a grouped summary for multiple modules.
func RelationshipAnalysisAcross(term string, views []ModuleRelationshipView) string {
	var text strings.Builder

	totalMatches := 0
	for _, v := range views {
		totalMatches += len(v.Relationships)
	}

	text.WriteString(fmt.Sprintf("# Relationship Analysis for '%s' across %d module%s\n\n", term, len(views), pluralSuffix(len(views))))
	text.WriteString(fmt.Sprintf("Found %d relationship%s.\n\n", totalMatches, pluralSuffix(totalMatches)))

	sort.SliceStable(views, func(i, j int) bool {
		return views[i].ModuleName < views[j].ModuleName
	})

	for idx, view := range views {
		if idx > 0 {
			text.WriteString("\n")
		}
		text.WriteString(renderRelationshipSection("##", view.ModuleName, term, view.Relationships, view.Files))
	}

	return text.String()
}

func renderRelationshipSection(headingPrefix, moduleName, term string, rels []database.HCLRelationship, files map[string]database.ModuleFile) string {
	var text strings.Builder

	total := len(rels)
	text.WriteString(fmt.Sprintf("%s Relationship Analysis for '%s' in %s\n\n", headingPrefix, term, moduleName))
	text.WriteString(fmt.Sprintf("Found %d relationship%s.\n\n", total, pluralSuffix(total)))

	if total == 0 {
		return text.String()
	}

	// ensure deterministic ordering (e.g., map iteration over files)
	sort.SliceStable(rels, func(i, j int) bool {
		if rels[i].FilePath == rels[j].FilePath {
			if rels[i].BlockType == rels[j].BlockType {
				if rels[i].AttributePath == rels[j].AttributePath {
					return rels[i].ReferenceName < rels[j].ReferenceName
				}
				return rels[i].AttributePath < rels[j].AttributePath
			}
			return rels[i].BlockType < rels[j].BlockType
		}
		return rels[i].FilePath < rels[j].FilePath
	})

	blockHeading := headingPrefix + "#"
	if len(blockHeading) > 6 {
		blockHeading = "######"
	}

	for _, rel := range rels {
		blockDesc := describeBlock(rel.BlockType, rel.BlockLabels)
		text.WriteString(fmt.Sprintf("%s %s — `%s`\n", blockHeading, blockDesc, rel.AttributePath))

		file, ok := files[rel.FilePath]
		var snippet string
		lineNumber := 0

		if ok {
			snippet, lineNumber = snippetForByteRange(file.Content, rel.StartByte)
			text.WriteString(fmt.Sprintf("- **File:** %s:%d\n", rel.FilePath, lineNumber))
		} else {
			text.WriteString(fmt.Sprintf("- **File:** %s\n", rel.FilePath))
		}

		text.WriteString(fmt.Sprintf("- **Reference:** `%s` (%s)\n\n", rel.ReferenceName, rel.ReferenceType))

		if snippet != "" {
			text.WriteString("```hcl\n")
			text.WriteString(snippet)
			text.WriteString("```\n\n")
		} else {
			text.WriteString("_Code snippet unavailable._\n\n")
		}
	}

	return text.String()
}

func describeBlock(blockType, labels string) string {
	if labels == "" {
		return blockType
	}

	parts := strings.Split(labels, ".")
	switch blockType {
	case "resource":
		if len(parts) >= 2 {
			return fmt.Sprintf("resource %s %s", parts[0], parts[1])
		}
	case "data":
		if len(parts) >= 2 {
			return fmt.Sprintf("data %s %s", parts[0], parts[1])
		}
	case "module":
		return fmt.Sprintf("module %s", parts[0])
	}

	return fmt.Sprintf("%s %s", blockType, labels)
}

func snippetForByteRange(content string, startByte int64) (string, int) {
	if startByte < 0 {
		startByte = 0
	}

	lines := strings.Split(content, "\n")
	if len(lines) == 0 {
		return "", 0
	}

	var byteOffset int64
	highlightIdx := len(lines) - 1

	for i, line := range lines {
		lineLen := int64(len(line))
		lineEnd := byteOffset + lineLen
		if startByte <= lineEnd {
			highlightIdx = i
			break
		}
		byteOffset = lineEnd + 1 // account for newline
	}

	start := max(0, highlightIdx-2)
	end := min(len(lines)-1, highlightIdx+2)

	var snippet strings.Builder
	for idx := start; idx <= end; idx++ {
		prefix := "  "
		if idx == highlightIdx {
			prefix = "→ "
		}
		snippet.WriteString(fmt.Sprintf("%s%d: %s\n", prefix, idx+1, lines[idx]))
	}

	return snippet.String(), highlightIdx + 1
}

func pluralSuffix(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
