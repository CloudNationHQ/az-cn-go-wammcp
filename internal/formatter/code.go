package formatter

import (
	"fmt"
	"strings"

	"github.com/cloudnationhq/az-cn-wam-mcp/internal/database"
)

func CodeSearchResults(query string, files []database.ModuleFile, getModuleName func(int64) string) string {
	var text strings.Builder
	text.WriteString(fmt.Sprintf("# Code Search Results for '%s' (%d matches)\n\n", query, len(files)))

	if len(files) == 0 {
		text.WriteString("No code matches found.\n")
		return text.String()
	}

	for _, file := range files {
		moduleName := getModuleName(file.ModuleID)
		text.WriteString(fmt.Sprintf("## %s / %s\n", moduleName, file.FilePath))
		text.WriteString("```\n")
		text.WriteString(ExtractCodeContext(file.Content, query))
		text.WriteString("```\n\n")
	}

	return text.String()
}

func ExtractCodeContext(content, query string) string {
	var text strings.Builder
	lines := strings.Split(content, "\n")
	queryLower := strings.ToLower(query)

	for i, line := range lines {
		if strings.Contains(strings.ToLower(line), queryLower) {
			start := max(i-2, 0)
			end := min(i+3, len(lines))

			for j := start; j < end; j++ {
				if j == i {
					text.WriteString(fmt.Sprintf("→ %d: %s\n", j+1, lines[j]))
				} else {
					text.WriteString(fmt.Sprintf("  %d: %s\n", j+1, lines[j]))
				}
			}
			text.WriteString("...\n")
			break
		}
	}

	return text.String()
}

func FileContent(moduleName, filePath, fileType string, sizeBytes int64, content string) string {
	var text strings.Builder
	text.WriteString(fmt.Sprintf("# %s / %s\n\n", moduleName, filePath))
	text.WriteString(fmt.Sprintf("**Size:** %d bytes\n", sizeBytes))
	text.WriteString(fmt.Sprintf("**Type:** %s\n\n", fileType))
	text.WriteString("```hcl\n")
	text.WriteString(content)
	text.WriteString("\n```\n")
	return text.String()
}

func VariableDefinition(moduleName, variableName, block string) string {
	var text strings.Builder
	text.WriteString(fmt.Sprintf("# %s / variable \"%s\"\n\n", moduleName, variableName))
	text.WriteString("```hcl\n")
	text.WriteString(block)
	text.WriteString("\n```\n")
	return text.String()
}

func PatternComparison(pattern string, results []PatternMatch, showFullBlocks bool, offset, limit, total int) string {
	var text strings.Builder
	text.WriteString(fmt.Sprintf("# Pattern Comparison: '%s'\n\n", pattern))
	text.WriteString(fmt.Sprintf("Found %d matches across modules", total))

	if limit > 0 || offset > 0 {
		endIdx := offset + len(results)
		text.WriteString(fmt.Sprintf(" (showing %d-%d)\n\n", offset+1, endIdx))
	} else {
		text.WriteString("\n\n")
	}

	if len(results) == 0 {
		if offset >= total && total > 0 {
			text.WriteString(fmt.Sprintf("No results in this range. Total results: %d\n", total))
		} else {
			text.WriteString("No matches found.\n")
		}
		return text.String()
	}

	if showFullBlocks {
		text.WriteString(formatFullBlocks(results))
	} else {
		text.WriteString(formatCompactTable(results))
	}

	if limit > 0 && offset+len(results) < total {
		remaining := total - (offset + len(results))
		text.WriteString(fmt.Sprintf("\n**Pagination:** %d more results available. Use `offset: %d` to see next page.\n", remaining, offset+len(results)))
	}

	return text.String()
}

type PatternMatch struct {
	ModuleName string
	FileName   string
	Match      string
}

func formatFullBlocks(results []PatternMatch) string {
	var text strings.Builder
	for _, result := range results {
		text.WriteString(fmt.Sprintf("## %s (%s)\n\n", result.ModuleName, result.FileName))
		text.WriteString("```hcl\n")
		text.WriteString(result.Match)
		text.WriteString("\n```\n\n")
	}
	return text.String()
}

func formatCompactTable(results []PatternMatch) string {
	var text strings.Builder
	text.WriteString("| Module | File | Preview |\n")
	text.WriteString("|--------|------|---------|\n")
	for _, result := range results {
		firstLine := strings.Split(result.Match, "\n")[0]
		if len(firstLine) > 60 {
			firstLine = firstLine[:60] + "..."
		}
		firstLine = strings.ReplaceAll(firstLine, "|", "\\|")
		text.WriteString(fmt.Sprintf("| %s | %s | %s |\n", result.ModuleName, result.FileName, firstLine))
	}
	text.WriteString("\n**Tip:** Use `show_full_blocks: true` to see complete code blocks\n")
	return text.String()
}

func ExampleList(moduleName string, exampleMap map[string][]string) string {
	var text strings.Builder
	text.WriteString(fmt.Sprintf("# Examples for %s\n\n", moduleName))

	if len(exampleMap) == 0 {
		text.WriteString("No examples found for this module.\n")
		return text.String()
	}

	text.WriteString(fmt.Sprintf("Found %d example(s):\n\n", len(exampleMap)))
	for exampleName, fileList := range exampleMap {
		text.WriteString(fmt.Sprintf("## %s\n", exampleName))
		text.WriteString("Files:\n")
		for _, fileName := range fileList {
			text.WriteString(fmt.Sprintf("- %s\n", fileName))
		}
		text.WriteString("\n")
	}

	return text.String()
}

func ExampleContent(moduleName, exampleName string, files []database.ModuleFile) string {
	var text strings.Builder
	text.WriteString(fmt.Sprintf("# %s / examples/%s\n\n", moduleName, exampleName))
	text.WriteString(fmt.Sprintf("Contains %d file(s)\n\n", len(files)))

	for _, file := range files {
		text.WriteString(fmt.Sprintf("## %s\n\n", file.FileName))
		text.WriteString(formatExampleFile(file))
	}

	return text.String()
}

func formatExampleFile(file database.ModuleFile) string {
	var text strings.Builder

	switch file.FileType {
	case "terraform":
		text.WriteString("```hcl\n")
		text.WriteString(file.Content)
		text.WriteString("\n```\n\n")
	case "yaml":
		text.WriteString("```yaml\n")
		text.WriteString(file.Content)
		text.WriteString("\n```\n\n")
	case "json":
		text.WriteString("```json\n")
		text.WriteString(file.Content)
		text.WriteString("\n```\n\n")
	case "markdown":
		text.WriteString(file.Content)
		if !strings.HasSuffix(file.Content, "\n") {
			text.WriteString("\n")
		}
		text.WriteString("\n")
	default:
		text.WriteString("```\n")
		text.WriteString(file.Content)
		text.WriteString("\n```\n\n")
	}

	return text.String()
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
