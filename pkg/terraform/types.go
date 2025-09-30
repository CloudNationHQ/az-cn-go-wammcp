// Package terraform defines shared data structures for Terraform metadata.
package terraform

import "time"

type Module struct {
	Name        string         `json:"name"`
	Path        string         `json:"path"`
	Description string         `json:"description"`
	Version     string         `json:"version"`
	Provider    string         `json:"provider"`
	Resources   []Resource     `json:"resources"`
	Variables   []Variable     `json:"variables"`
	Outputs     []Output       `json:"outputs"`
	Examples    []Example      `json:"examples"`
	Tags        []string       `json:"tags"`
	LastUpdated time.Time      `json:"last_updated"`
	Repository  RepositoryInfo `json:"repository"`
}

type Variable struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Description string `json:"description"`
	Default     any    `json:"default,omitempty"`
	Required    bool   `json:"required"`
	Sensitive   bool   `json:"sensitive"`
}

type Output struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Sensitive   bool   `json:"sensitive"`
}

type Resource struct {
	Type     string `json:"type"`
	Name     string `json:"name"`
	Provider string `json:"provider"`
}

type Example struct {
	Name        string `json:"name"`
	Path        string `json:"path"`
	Description string `json:"description"`
	Content     string `json:"content"`
}

type RepositoryInfo struct {
	URL       string    `json:"url"`
	Branch    string    `json:"branch"`
	CommitSHA string    `json:"commit_sha"`
	LastSync  time.Time `json:"last_sync"`
}

type ModuleIndex struct {
	Modules     []Module            `json:"modules"`
	Categories  map[string][]string `json:"categories"`
	LastUpdated time.Time           `json:"last_updated"`
}

type SearchQuery struct {
	Query      string   `json:"query"`
	Categories []string `json:"categories,omitempty"`
	Provider   string   `json:"provider,omitempty"`
	Tags       []string `json:"tags,omitempty"`
	Limit      int      `json:"limit,omitempty"`
}

type SearchResult struct {
	Modules []Module `json:"modules"`
	Total   int      `json:"total"`
}
