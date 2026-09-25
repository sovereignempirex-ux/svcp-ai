package tools

import (
	"os"
	"strings"
)

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