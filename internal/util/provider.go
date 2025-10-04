// Package util provides common utility functions
package util

import "strings"

// ExtractProvider extracts the provider name from a resource or data source type
func ExtractProvider(resourceType string) string {
	parts := strings.Split(resourceType, "_")
	if len(parts) > 0 {
		return parts[0]
	}
	return "unknown"
}
