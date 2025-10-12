package formatter

import (
	"fmt"
	"strings"

	"github.com/cloudnationhq/az-cn-go-wammcp/internal/indexer"
)

func SyncProgress(progress *indexer.SyncProgress) string {
	if progress == nil {
		return ""
	}

	var text strings.Builder
	text.WriteString("## Summary\n\n")
	succeeded := progress.ProcessedRepos - len(progress.Errors)
	text.WriteString(fmt.Sprintf("Successfully synced %d/%d repositories\n\n", succeeded, progress.TotalRepos))

	if len(progress.UpdatedRepos) > 0 {
		text.WriteString("Updated repositories:\n")
		for _, repo := range progress.UpdatedRepos {
			text.WriteString(fmt.Sprintf("- %s\n", repo))
		}
		text.WriteString("\n")
	}

	if len(progress.Errors) > 0 {
		text.WriteString(fmt.Sprintf("%d errors occurred:\n", len(progress.Errors)))
		for i, err := range progress.Errors {
			if i >= 10 {
				remaining := len(progress.Errors) - 10
				text.WriteString(fmt.Sprintf("... and %d more errors\n", remaining))
				break
			}
			text.WriteString(fmt.Sprintf("- %s\n", err))
		}
	}

	return text.String()
}
