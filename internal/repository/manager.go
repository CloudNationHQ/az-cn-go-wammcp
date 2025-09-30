// Package repository manages filesystem access to Terraform module sources.
package repository

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Manager manages repository operations
type Manager struct {
	basePath string
}

// NewManager creates a new repository manager
func NewManager(basePath string) *Manager {
	return &Manager{
		basePath: basePath,
	}
}

// ScanLocalModules scans for local terraform modules
func (m *Manager) ScanLocalModules(ctx context.Context) ([]string, error) {
	var modulePaths []string

	// Check if base path exists
	if _, err := os.Stat(m.basePath); os.IsNotExist(err) {
		return nil, fmt.Errorf("base path does not exist: %s", m.basePath)
	}

	// Walk through directory looking for terraform modules
	entries, err := os.ReadDir(m.basePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory %s: %w", m.basePath, err)
	}

	for _, entry := range entries {
		if entry.IsDir() && strings.HasPrefix(entry.Name(), "terraform-") {
			modulePath := filepath.Join(m.basePath, entry.Name())

			// Check if it contains terraform files
			if m.containsTerraformFiles(modulePath) {
				modulePaths = append(modulePaths, modulePath)
			}
		}
	}

	return modulePaths, nil
}

// containsTerraformFiles checks if directory contains .tf files
func (m *Manager) containsTerraformFiles(path string) bool {
	entries, err := os.ReadDir(path)
	if err != nil {
		return false
	}

	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".tf") {
			return true
		}
	}

	return false
}

// GetModuleInfo gets basic information about a module directory
func (m *Manager) GetModuleInfo(modulePath string) (*ModuleInfo, error) {
	info := &ModuleInfo{
		Path: modulePath,
		Name: filepath.Base(modulePath),
	}

	// Check if README exists
	readmePath := filepath.Join(modulePath, "README.md")
	if _, err := os.Stat(readmePath); err == nil {
		info.HasReadme = true
	}

	// Check if examples exist
	examplesPath := filepath.Join(modulePath, "examples")
	if stat, err := os.Stat(examplesPath); err == nil && stat.IsDir() {
		info.HasExamples = true
	}

	// Count terraform files
	entries, err := os.ReadDir(modulePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read module directory: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".tf") {
			info.TerraformFiles = append(info.TerraformFiles, entry.Name())
		}
	}

	return info, nil
}

// ModuleInfo contains basic information about a module
type ModuleInfo struct {
	Path           string   `json:"path"`
	Name           string   `json:"name"`
	HasReadme      bool     `json:"has_readme"`
	HasExamples    bool     `json:"has_examples"`
	TerraformFiles []string `json:"terraform_files"`
}
