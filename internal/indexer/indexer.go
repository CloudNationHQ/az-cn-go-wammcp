// Package indexer builds searchable indexes for Terraform modules.
package indexer

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/cloudnationhq/az-cn-wam-mcp/internal/parser"
	"github.com/cloudnationhq/az-cn-wam-mcp/pkg/terraform"
)

// Indexer manages the module index and provides search capabilities
type Indexer struct {
	modules    map[string]*terraform.Module
	index      *terraform.ModuleIndex
	parser     *parser.TerraformParser
	mutex      sync.RWMutex
	basePath   string
	lastUpdate time.Time
}

// NewIndexer creates a new module indexer
func NewIndexer(basePath string) *Indexer {
	return &Indexer{
		modules:  make(map[string]*terraform.Module),
		parser:   parser.NewTerraformParser(),
		basePath: basePath,
	}
}

// Initialize initializes the indexer by scanning all modules
func (i *Indexer) Initialize(ctx context.Context) error {
	i.mutex.Lock()
	defer i.mutex.Unlock()

	fmt.Fprintf(os.Stderr, "Initializing indexer, scanning modules in: %s\n", i.basePath)

	// Find all Terraform module directories
	moduleDirs, err := i.findModuleDirectories()
	if err != nil {
		return fmt.Errorf("failed to find module directories: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Found %d module directories\n", len(moduleDirs))

	// Parse each module
	for _, moduleDir := range moduleDirs {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			if err := i.parseAndIndexModule(moduleDir); err != nil {
				fmt.Printf("Warning: failed to parse module %s: %v\n", moduleDir, err)
				continue
			}
		}
	}

	// Train the category learner with all modules
	i.trainCategoryLearner()

	// Build the search index
	i.buildIndex()

	i.lastUpdate = time.Now()
	fmt.Fprintf(os.Stderr, "Indexer initialized with %d modules\n", len(i.modules))

	return nil
}

// findModuleDirectories finds all terraform module directories
func (i *Indexer) findModuleDirectories() ([]string, error) {
	var moduleDirs []string

	entries, err := filepath.Glob(filepath.Join(i.basePath, "terraform-*"))
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		// Check if it contains .tf files
		tfFiles, err := filepath.Glob(filepath.Join(entry, "*.tf"))
		if err != nil {
			continue
		}
		if len(tfFiles) > 0 {
			moduleDirs = append(moduleDirs, entry)
		}
	}

	return moduleDirs, nil
}

// parseAndIndexModule parses and indexes a single module
func (i *Indexer) parseAndIndexModule(moduleDir string) error {
	module, err := i.parser.ParseModule(moduleDir)
	if err != nil {
		return err
	}

	i.modules[module.Name] = module
	return nil
}

// trainCategoryLearner trains the category learner with all modules
func (i *Indexer) trainCategoryLearner() {
	learner := parser.NewCategoryLearner()

	// Train on all modules
	for _, module := range i.modules {
		learner.LearnFromModule(module)
	}

	// Set the trained learner back to the parser
	i.parser.SetLearner(learner)

	// Re-categorize all modules with the learned patterns
	for _, module := range i.modules {
		module.Tags = i.categorizeWithLearner(module, learner)
	}
}

// categorizeWithLearner categorizes a module using the trained learner
func (i *Indexer) categorizeWithLearner(module *terraform.Module, learner *parser.CategoryLearner) []string {
	categories := []string{}
	categoryMap := make(map[string]bool)

	// Get categories from learned patterns
	for _, resource := range module.Resources {
		cats := learner.GetLearnedCategories(resource.Type)
		for _, cat := range cats {
			if !categoryMap[cat] {
				categories = append(categories, cat)
				categoryMap[cat] = true
			}
		}
	}

	// Add categories from learned text patterns
	text := module.Name + " " + module.Description
	textCats := learner.GetLearnedTextCategories(text)
	for _, cat := range textCats {
		if !categoryMap[cat] {
			categories = append(categories, cat)
			categoryMap[cat] = true
		}
	}

	// Add provider-specific category
	if module.Provider != "" && !categoryMap[module.Provider] {
		categories = append(categories, module.Provider)
	}

	return categories
}

// buildIndex builds the search index
func (i *Indexer) buildIndex() {
	modules := make([]terraform.Module, 0, len(i.modules))
	categories := make(map[string][]string)

	for _, module := range i.modules {
		modules = append(modules, *module)

		// Build category index
		for _, tag := range module.Tags {
			categories[tag] = append(categories[tag], module.Name)
		}
	}

	i.index = &terraform.ModuleIndex{
		Modules:     modules,
		Categories:  categories,
		LastUpdated: time.Now(),
	}
}

// GetModules returns all modules with optional filtering
func (i *Indexer) GetModules(ctx context.Context) ([]terraform.Module, error) {
	i.mutex.RLock()
	defer i.mutex.RUnlock()

	modules := make([]terraform.Module, 0, len(i.modules))
	for _, module := range i.modules {
		modules = append(modules, *module)
	}

	return modules, nil
}

// GetModule returns a specific module by name
func (i *Indexer) GetModule(ctx context.Context, name string) (*terraform.Module, error) {
	i.mutex.RLock()
	defer i.mutex.RUnlock()

	module, exists := i.modules[name]
	if !exists {
		return nil, fmt.Errorf("module %s not found", name)
	}

	return module, nil
}

// SearchModules searches modules based on query
func (i *Indexer) SearchModules(ctx context.Context, query terraform.SearchQuery) (*terraform.SearchResult, error) {
	i.mutex.RLock()
	defer i.mutex.RUnlock()

	var results []terraform.Module
	queryLower := strings.ToLower(query.Query)

	for _, module := range i.modules {
		score := i.calculateSearchScore(module, queryLower)
		if score > 0 {
			results = append(results, *module)
		}
	}

	// Limit results
	if query.Limit > 0 && len(results) > query.Limit {
		results = results[:query.Limit]
	}

	return &terraform.SearchResult{
		Modules: results,
		Total:   len(results),
	}, nil
}

// calculateSearchScore calculates relevance score for search
func (i *Indexer) calculateSearchScore(module *terraform.Module, query string) int {
	score := 0

	// Check module name
	if strings.Contains(strings.ToLower(module.Name), query) {
		score += 10
	}

	// Check description
	if strings.Contains(strings.ToLower(module.Description), query) {
		score += 5
	}

	// Check tags
	for _, tag := range module.Tags {
		if strings.Contains(strings.ToLower(tag), query) {
			score += 3
		}
	}

	// Check resource types
	for _, resource := range module.Resources {
		if strings.Contains(strings.ToLower(resource.Type), query) {
			score += 2
		}
	}

	return score
}

// FindDependencies finds modules that commonly work together
func (i *Indexer) FindDependencies(ctx context.Context, moduleName string) ([]string, error) {
	i.mutex.RLock()
	defer i.mutex.RUnlock()

	module, exists := i.modules[moduleName]
	if !exists {
		return nil, fmt.Errorf("module %s not found", moduleName)
	}

	var dependencies []string

	// Find modules with similar tags
	for _, otherModule := range i.modules {
		if otherModule.Name == moduleName {
			continue
		}

		commonTags := i.countCommonTags(module.Tags, otherModule.Tags)
		if commonTags >= 2 { // At least 2 common tags
			dependencies = append(dependencies, otherModule.Name)
		}
	}

	return dependencies, nil
}

// countCommonTags counts common tags between two modules
func (i *Indexer) countCommonTags(tags1, tags2 []string) int {
	tagMap := make(map[string]bool)
	for _, tag := range tags1 {
		tagMap[tag] = true
	}

	count := 0
	for _, tag := range tags2 {
		if tagMap[tag] {
			count++
		}
	}

	return count
}

// GetIndex returns the current module index
func (i *Indexer) GetIndex() *terraform.ModuleIndex {
	i.mutex.RLock()
	defer i.mutex.RUnlock()
	return i.index
}

// Refresh refreshes the index by re-scanning modules
func (i *Indexer) Refresh(ctx context.Context) error {
	return i.Initialize(ctx)
}
