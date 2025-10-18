// Package formatter provides formatting utilities for module data
package formatter

import (
	"fmt"
	"strings"
	"time"

	"github.com/cloudnationhq/az-cn-go-wammcp/internal/database"
)

func ModuleList(modules []database.Module) string {
	var text strings.Builder
	text.WriteString(fmt.Sprintf("# Azure CloudNation Terraform Modules (%d modules)\n\n", len(modules)))

	for i, module := range modules {
		if i >= 50 {
			text.WriteString(fmt.Sprintf("... and %d more modules\n", len(modules)-50))
			break
		}
		text.WriteString(fmt.Sprintf("**%s**\n", module.Name))
		if module.Description != "" {
			text.WriteString(fmt.Sprintf("  %s\n", module.Description))
		}
		text.WriteString(fmt.Sprintf("  Repo: %s\n", module.RepoURL))
		text.WriteString(fmt.Sprintf("  Last synced: %s\n\n", module.SyncedAt.Format("2006-01-02 15:04:05")))
	}

	return text.String()
}

func SearchResults(query string, modules []database.Module) string {
	var text strings.Builder
	text.WriteString(fmt.Sprintf("# Search Results for '%s' (%d matches)\n\n", query, len(modules)))

	for _, module := range modules {
		text.WriteString(fmt.Sprintf("**%s**\n", module.Name))
		if module.Description != "" {
			text.WriteString(fmt.Sprintf("  %s\n", module.Description))
		}
		text.WriteString(fmt.Sprintf("  Repo: %s\n\n", module.RepoURL))
	}

	if len(modules) == 0 {
		text.WriteString("No modules found matching your query.\n")
	}

	return text.String()
}

func ModuleInfo(module *database.Module, variables []database.ModuleVariable, outputs []database.ModuleOutput, resources []database.ModuleResource, files []database.ModuleFile) string {
	var text strings.Builder
	text.WriteString(fmt.Sprintf("# %s\n\n", module.Name))

	if module.Description != "" {
		text.WriteString(fmt.Sprintf("**Description:** %s\n\n", module.Description))
	}

	text.WriteString(fmt.Sprintf("**Repository:** %s\n", module.RepoURL))
	text.WriteString(fmt.Sprintf("**Last Updated:** %s\n", module.LastUpdated))
	text.WriteString(fmt.Sprintf("**Last Synced:** %s\n\n", module.SyncedAt.Format("2006-01-02 15:04:05")))

	if len(variables) > 0 {
		text.WriteString(VariablesSection(variables))
	}

	if len(outputs) > 0 {
		text.WriteString(OutputsSection(outputs))
	}

	if len(resources) > 0 {
		text.WriteString(ResourcesSection(resources))
	}

	if len(files) > 0 {
		text.WriteString(FilesSection(files))
	}

	if module.ReadmeContent != "" {
		text.WriteString(ReadmeExcerpt(module.ReadmeContent))
	}

	return text.String()
}
func StructuralSummaryValues(resourceCount, lifecycleCount, withIgnore int, topResourceTypes, dynamicLabels []string) string {
	var text strings.Builder
	text.WriteString("## Structural Summary\n\n")
	text.WriteString(fmt.Sprintf("- Resources: %d\n", resourceCount))
	text.WriteString(fmt.Sprintf("- Lifecycle blocks: %d\n", lifecycleCount))
	if withIgnore > 0 {
		text.WriteString(fmt.Sprintf("- Resources with lifecycle.ignore_changes: %d\n", withIgnore))
	}
	if len(topResourceTypes) > 0 {
		text.WriteString("- Top resource types: ")
		text.WriteString(strings.Join(topResourceTypes, ", "))
		text.WriteString("\n")
	}
	if len(dynamicLabels) > 0 {
		text.WriteString("- Dynamic labels: ")
		text.WriteString(strings.Join(dynamicLabels, ", "))
		text.WriteString("\n")
	}
	text.WriteString("\n")
	return text.String()
}

func VariablesSection(variables []database.ModuleVariable) string {
	var text strings.Builder
	text.WriteString("## Variables\n\n")
	for _, v := range variables {
		text.WriteString(fmt.Sprintf("- **%s**", v.Name))
		if v.Type != "" {
			text.WriteString(fmt.Sprintf(" (`%s`)", v.Type))
		}
		if v.Required {
			text.WriteString(" *[required]*")
		}
		if v.Sensitive {
			text.WriteString(" *[sensitive]*")
		}
		if v.DefaultValue != "" {
			text.WriteString(fmt.Sprintf(" - default: `%s`", v.DefaultValue))
		}
		if v.Description != "" {
			text.WriteString(fmt.Sprintf("\n  %s", v.Description))
		}
		text.WriteString("\n")
	}
	text.WriteString("\n")
	return text.String()
}

func OutputsSection(outputs []database.ModuleOutput) string {
	var text strings.Builder
	text.WriteString("## Outputs\n\n")
	for _, o := range outputs {
		text.WriteString(fmt.Sprintf("- **%s**", o.Name))
		if o.Sensitive {
			text.WriteString(" *[sensitive]*")
		}
		if o.Description != "" {
			text.WriteString(fmt.Sprintf("\n  %s", o.Description))
		}
		text.WriteString("\n")
	}
	text.WriteString("\n")
	return text.String()
}

func ResourcesSection(resources []database.ModuleResource) string {
	var text strings.Builder
	text.WriteString(fmt.Sprintf("## Resources (%d)\n\n", len(resources)))
	for i, r := range resources {
		if i >= 20 {
			text.WriteString(fmt.Sprintf("... and %d more resources\n", len(resources)-20))
			break
		}
		text.WriteString(fmt.Sprintf("- `%s.%s`", r.ResourceType, r.ResourceName))
		if r.SourceFile != "" {
			text.WriteString(fmt.Sprintf(" (in %s)", r.SourceFile))
		}
		text.WriteString("\n")
	}
	text.WriteString("\n")
	return text.String()
}

func FilesSection(files []database.ModuleFile) string {
	var text strings.Builder
	text.WriteString(fmt.Sprintf("## Files (%d)\n\n", len(files)))
	for i, f := range files {
		if i >= 30 {
			text.WriteString(fmt.Sprintf("... and %d more files\n", len(files)-30))
			break
		}
		text.WriteString(fmt.Sprintf("- %s", f.FilePath))
		if f.SizeBytes > 0 {
			text.WriteString(fmt.Sprintf(" (%d bytes)", f.SizeBytes))
		}
		text.WriteString("\n")
	}
	text.WriteString("\n")
	return text.String()
}

func ReadmeExcerpt(readme string) string {
	var text strings.Builder
	text.WriteString("## README (excerpt)\n\n")
	lines := strings.Split(readme, "\n")
	lineCount := 0
	for _, line := range lines {
		if lineCount >= 30 {
			text.WriteString("\n... (truncated, see full README at repository)\n")
			break
		}
		text.WriteString(line + "\n")
		lineCount++
	}
	return text.String()
}

func IncrementalSyncProgress(totalRepos, synced, skipped int, updatedRepos, errors []string) string {
	var text strings.Builder
	text.WriteString("# Incremental Sync Completed\n\n")

	text.WriteString(fmt.Sprintf("Checked %d repositories\n", totalRepos))
	text.WriteString(fmt.Sprintf("Updated modules: %d\n", synced))
	text.WriteString(fmt.Sprintf("Skipped (up-to-date): %d\n\n", skipped))

	if synced > 0 {
		text.WriteString("Updated repositories:\n")
		for _, repo := range updatedRepos {
			text.WriteString(fmt.Sprintf("- %s\n", repo))
		}
		text.WriteString("\n")
	}

	if len(errors) > 0 {
		text.WriteString(fmt.Sprintf("%d errors occurred:\n", len(errors)))
		for i, err := range errors {
			if i >= 10 {
				text.WriteString(fmt.Sprintf("... and %d more errors\n", len(errors)-10))
				break
			}
			text.WriteString(fmt.Sprintf("- %s\n", err))
		}
	}

	return text.String()
}

func JobDetails(jobID, jobType, status string, startedAt time.Time, completedAt *time.Time, errorMsg string, progressText string) string {
	var text strings.Builder
	text.WriteString(fmt.Sprintf("# Sync Job %s (%s)\n\n", jobID, jobType))
	text.WriteString(fmt.Sprintf("Status: %s\n", strings.ToUpper(status)))
	text.WriteString(fmt.Sprintf("Started: %s\n", startedAt.Format(time.RFC3339)))
	if completedAt != nil {
		duration := completedAt.Sub(startedAt)
		text.WriteString(fmt.Sprintf("Completed: %s (duration %s)\n", completedAt.Format(time.RFC3339), duration.Round(time.Second)))
	} else {
		text.WriteString(fmt.Sprintf("Elapsed: %s\n", time.Since(startedAt).Round(time.Second)))
	}

	if errorMsg != "" {
		text.WriteString(fmt.Sprintf("\nError: %s\n", errorMsg))
	}

	if progressText != "" {
		text.WriteString("\n")
		text.WriteString(progressText)
	}

	return text.String()
}

func JobList(jobs []JobInfo) string {
	if len(jobs) == 0 {
		return "No sync jobs have been scheduled yet."
	}

	var text strings.Builder
	text.WriteString("# Sync Jobs\n\n")
	for _, job := range jobs {
		text.WriteString(fmt.Sprintf("- %s (%s) — %s", job.ID, job.Type, strings.ToUpper(job.Status)))
		if job.CompletedAt != nil {
			duration := job.CompletedAt.Sub(job.StartedAt)
			text.WriteString(fmt.Sprintf(" in %s", duration.Round(time.Second)))
		}
		text.WriteString("\n")
	}

	text.WriteString("\nUse `sync_status` with a job_id for detailed information.\n")
	return text.String()
}

type JobInfo struct {
	ID          string
	Type        string
	Status      string
	StartedAt   time.Time
	CompletedAt *time.Time
}
