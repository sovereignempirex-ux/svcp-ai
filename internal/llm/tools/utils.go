package tools

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// orDefault returns v unless it is blank, in which case fallback wins.
func orDefault(v, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return strings.TrimSpace(v)
}

// orString is a readability alias for orDefault.
func orString(v, fallback string) string { return orDefault(v, fallback) }

// sortedKeys gives map iteration a stable order, which matters because the
// result is turned into a command line the user may read.
func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// baseOf returns the final element of a path.
func baseOf(p string) string { return filepath.Base(p) }

// parentOf returns the directory containing a path.
func parentOf(p string) string { return filepath.Dir(p) }

// defaultString returns s if non-empty, otherwise fallback
func defaultString(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}

// max returns the larger of a and b
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// getEnvOrDefault returns the environment variable value or default
func getEnvOrDefault(key, defaultValue string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultValue
}

// getDefaultModel returns the default model for a provider
func getDefaultModel(provider string) string {
	switch strings.ToLower(provider) {
	case "openai":
		return "dall-e-3"
	case "azure_openai":
		return "dall-e-3"
	case "stability":
		return "stable-diffusion-xl-1024-v1-0"
	case "replicate":
		return "stability-ai/sdxl:latest"
	case "huggingface":
		return "stabilityai/stable-diffusion-xl-base-1.0"
	case "midjourney":
		return "midjourney-v6"
	default:
		return "dall-e-3"
	}
}
