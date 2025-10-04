// Package util provides common utility functions
package util

import "strings"

func ExtractProvider(resourceType string) string {
	parts := strings.Split(resourceType, "_")
	if len(parts) > 0 {
		return parts[0]
	}
	return "unknown"
}
