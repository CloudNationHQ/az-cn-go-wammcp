// Package terraform defines shared data structures for Terraform metadata.
package terraform

import "time"

// Module represents a Terraform module with its metadata
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

// Variable represents a Terraform variable
type Variable struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Description string `json:"description"`
	Default     any    `json:"default,omitempty"`
	Required    bool   `json:"required"`
	Sensitive   bool   `json:"sensitive"`
}

// Output represents a Terraform output
type Output struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Sensitive   bool   `json:"sensitive"`
}

// Resource represents a Terraform resource
type Resource struct {
	Type     string `json:"type"`
	Name     string `json:"name"`
	Provider string `json:"provider"`
}

// Example represents a usage example
type Example struct {
	Name        string `json:"name"`
	Path        string `json:"path"`
	Description string `json:"description"`
	Content     string `json:"content"`
}

// RepositoryInfo contains repository metadata
type RepositoryInfo struct {
	URL       string    `json:"url"`
	Branch    string    `json:"branch"`
	CommitSHA string    `json:"commit_sha"`
	LastSync  time.Time `json:"last_sync"`
}

// ModuleIndex represents the searchable index of all modules
type ModuleIndex struct {
	Modules     []Module            `json:"modules"`
	Categories  map[string][]string `json:"categories"`
	LastUpdated time.Time           `json:"last_updated"`
}

// SearchQuery represents a search request
type SearchQuery struct {
	Query      string   `json:"query"`
	Categories []string `json:"categories,omitempty"`
	Provider   string   `json:"provider,omitempty"`
	Tags       []string `json:"tags,omitempty"`
	Limit      int      `json:"limit,omitempty"`
}

// SearchResult represents search results
type SearchResult struct {
	Modules []Module `json:"modules"`
	Total   int      `json:"total"`
}
