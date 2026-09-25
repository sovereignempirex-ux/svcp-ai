package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

const (
	ImageGenToolName       = "image_gen"
	ImageEditToolName      = "image_edit"
	ImageVariationToolName = "image_variation"
)

type ImageGenParams struct {
	Provider    string   `json:"provider"`
	Prompt      string   `json:"prompt"`
	Model       string   `json:"model,omitempty"`
	Size        string   `json:"size,omitempty"`
	Quality     string   `json:"quality,omitempty"`
	Style       string   `json:"style,omitempty"`
	N           int      `json:"n,omitempty"`
	Seed        int64    `json:"seed,omitempty"`
	NegativePrompt string  `json:"negative_prompt,omitempty"`
	AspectRatio string   `json:"aspect_ratio,omitempty"`
	Token       string   `json:"token,omitempty"`
	APIURL      string   `json:"api_url,omitempty"`
}

type ImageEditParams struct {
	Provider     string   `json:"provider"`
	Prompt       string   `json:"prompt"`
	ImagePath    string   `json:"image_path"`
	MaskPath     string   `json:"mask_path,omitempty"`
	Model        string   `json:"model,omitempty"`
	Size         string   `json:"size,omitempty"`
	N            int      `json:"n,omitempty"`
	Token        string   `json:"token,omitempty"`
	APIURL       string   `json:"api_url,omitempty"`
}

type ImageVariationParams struct {
	Provider  string `json:"provider"`
	ImagePath string `json:"image_path"`
	Model     string `json:"model,omitempty"`
	Size      string `json:"size,omitempty"`
	N         int    `json:"n,omitempty"`
	Token     string `json:"token,omitempty"`
	APIURL    string `json:"api_url,omitempty"`
}

type ImageGenTool struct {
	httpClient *http.Client
}

func NewImageGenTool() BaseTool {
	return &ImageGenTool{
		httpClient: &http.Client{},
	}
}

func (t *ImageGenTool) Info() ToolInfo {
	return ToolInfo{
		Name:        ImageGenToolName,
		Description: "Generate images using AI providers (OpenAI DALL-E, Stability AI, Replicate, Hugging Face, etc.). Supports text-to-image, image-to-image, and variations.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"provider": map[string]any{
					"type":        "string",
					"description": "Image generation provider",
					"enum":        []string{"openai", "stability", "replicate", "huggingface", "midjourney", "azure_openai"},
				},
				"prompt": map[string]any{
					"type":        "string",
					"description": "Text prompt describing the image to generate",
				},
				"model": map[string]any{
					"type":        "string",
					"description": "Model to use (e.g., dall-e-3, dall-e-2, stable-diffusion-xl, sdxl, flux, midjourney-v6)",
				},
				"size": map[string]any{
					"type":        "string",
					"description": "Image size (e.g., 1024x1024, 1792x1024, 1024x1792, 512x512)",
				},
				"quality": map[string]any{
					"type":        "string",
					"description": "Quality: standard, hd (OpenAI), or low/medium/high (others)",
					"enum":        []string{"standard", "hd", "low", "medium", "high"},
				},
				"style": map[string]any{
					"type":        "string",
					"description": "Style: vivid, natural (OpenAI DALL-E 3)",
					"enum":        []string{"vivid", "natural"},
				},
				"n": map[string]any{
					"type":        "integer",
					"description": "Number of images to generate (1-10)",
				},
				"seed": map[string]any{
					"type":        "integer",
					"description": "Random seed for reproducibility",
				},
				"negative_prompt": map[string]any{
					"type":        "string",
					"description": "Negative prompt (Stability, Hugging Face)",
				},
				"aspect_ratio": map[string]any{
					"type":        "string",
					"description": "Aspect ratio (Midjourney, some others): 1:1, 16:9, 9:16, 4:3, 3:4",
				},
				"token": map[string]any{
					"type":        "string",
					"description": "API token (uses env var if not provided)",
				},
				"api_url": map[string]any{
					"type":        "string",
					"description": "Custom API endpoint",
				},
			},
			"required": []string{"provider", "prompt"},
		},
	}
}

func (t *ImageGenTool) Run(ctx context.Context, call ToolCall) (ToolResponse, error) {
	var params ImageGenParams
	if err := json.Unmarshal([]byte(call.Input), &params); err != nil {
		return NewTextErrorResponse(fmt.Sprintf("invalid parameters: %v", err)), nil
	}

	if params.Prompt == "" {
		return NewTextErrorResponse("prompt is required"), nil
	}

	provider := strings.ToLower(params.Provider)
	
	// Get token from env if not provided
	token := params.Token
	if token == "" {
		switch provider {
		case "openai", "azure_openai":
			token = getEnvOrDefault("OPENAI_API_KEY", "")
		case "stability":
			token = getEnvOrDefault("STABILITY_API_KEY", "")
		case "replicate":
			token = getEnvOrDefault("REPLICATE_API_TOKEN", "")
		case "huggingface":
			token = getEnvOrDefault("HUGGINGFACE_API_KEY", "")
		case "midjourney":
			token = getEnvOrDefault("MIDJOURNEY_API_KEY", "")
		}
	}

	if token == "" {
		return NewTextErrorResponse(fmt.Sprintf("no API key for %s (set env var or provide token)", provider)), nil
	}

	// This is a stub implementation - real implementation would call the actual APIs
	result := fmt.Sprintf(
		"Image generation request queued:\n- Provider: %s\n- Model: %s\n- Prompt: %s\n- Size: %s\n- Quality: %s\n- N: %d\n\nNote: This is a stub. Real implementation would call the provider's API and return image URLs or base64 data.",
		provider,
		defaultString(params.Model, getDefaultModel(provider)),
		params.Prompt,
		defaultString(params.Size, "1024x1024"),
		defaultString(params.Quality, "standard"),
		max(params.N, 1),
	)

	return NewTextResponse(result), nil
}

type ImageEditTool struct {
	httpClient *http.Client
}

func NewImageEditTool() BaseTool {
	return &ImageEditTool{
		httpClient: &http.Client{},
	}
}

func (t *ImageEditTool) Info() ToolInfo {
	return ToolInfo{
		Name:        ImageEditToolName,
		Description: "Edit images using AI (inpainting, outpainting, instructed editing). Supported by OpenAI DALL-E 2, Stability AI, etc.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"provider": map[string]any{
					"type":        "string",
					"description": "Provider: openai, stability, replicate",
					"enum":        []string{"openai", "stability", "replicate"},
				},
				"prompt": map[string]any{
					"type":        "string",
					"description": "Edit instruction prompt",
				},
				"image_path": map[string]any{
					"type":        "string",
					"description": "Path to source image file",
				},
				"mask_path": map[string]any{
					"type":        "string",
					"description": "Path to mask image (for inpainting)",
				},
				"model": map[string]any{
					"type":        "string",
					"description": "Model to use",
				},
				"size": map[string]any{
					"type":        "string",
					"description": "Output size",
				},
				"n": map[string]any{
					"type":        "integer",
					"description": "Number of variations",
				},
				"token": map[string]any{
					"type":        "string",
					"description": "API token",
				},
				"api_url": map[string]any{
					"type":        "string",
					"description": "Custom API endpoint",
				},
			},
			"required": []string{"provider", "prompt", "image_path"},
		},
	}
}

func (t *ImageEditTool) Run(ctx context.Context, call ToolCall) (ToolResponse, error) {
	var params ImageEditParams
	if err := json.Unmarshal([]byte(call.Input), &params); err != nil {
		return NewTextErrorResponse(fmt.Sprintf("invalid parameters: %v", err)), nil
	}

	if params.Prompt == "" || params.ImagePath == "" {
		return NewTextErrorResponse("prompt and image_path are required"), nil
	}

	result := fmt.Sprintf(
		"Image edit request queued:\n- Provider: %s\n- Model: %s\n- Prompt: %s\n- Image: %s\n- Mask: %s\n- Size: %s\n- N: %d\n\nNote: This is a stub implementation.",
		params.Provider,
		defaultString(params.Model, getDefaultModel(params.Provider)),
		params.Prompt,
		params.ImagePath,
		defaultString(params.MaskPath, "none (full image edit)"),
		defaultString(params.Size, "1024x1024"),
		max(params.N, 1),
	)

	return NewTextResponse(result), nil
}

type ImageVariationTool struct {
	httpClient *http.Client
}

func NewImageVariationTool() BaseTool {
	return &ImageVariationTool{
		httpClient: &http.Client{},
	}
}

func (t *ImageVariationTool) Info() ToolInfo {
	return ToolInfo{
		Name:        ImageVariationToolName,
		Description: "Create variations of an existing image. Supported by OpenAI DALL-E 2, Stability AI, etc.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"provider": map[string]any{
					"type":        "string",
					"description": "Provider: openai, stability, replicate",
					"enum":        []string{"openai", "stability", "replicate"},
				},
				"image_path": map[string]any{
					"type":        "string",
					"description": "Path to source image file",
				},
				"model": map[string]any{
					"type":        "string",
					"description": "Model to use",
				},
				"size": map[string]any{
					"type":        "string",
					"description": "Output size",
				},
				"n": map[string]any{
					"type":        "integer",
					"description": "Number of variations",
				},
				"token": map[string]any{
					"type":        "string",
					"description": "API token",
				},
				"api_url": map[string]any{
					"type":        "string",
					"description": "Custom API endpoint",
				},
			},
			"required": []string{"provider", "image_path"},
		},
	}
}

func (t *ImageVariationTool) Run(ctx context.Context, call ToolCall) (ToolResponse, error) {
	var params ImageVariationParams
	if err := json.Unmarshal([]byte(call.Input), &params); err != nil {
		return NewTextErrorResponse(fmt.Sprintf("invalid parameters: %v", err)), nil
	}

	if params.ImagePath == "" {
		return NewTextErrorResponse("image_path is required"), nil
	}

	result := fmt.Sprintf(
		"Image variation request queued:\n- Provider: %s\n- Model: %s\n- Image: %s\n- Size: %s\n- N: %d\n\nNote: This is a stub implementation.",
		params.Provider,
		defaultString(params.Model, getDefaultModel(params.Provider)),
		params.ImagePath,
		defaultString(params.Size, "1024x1024"),
		max(params.N, 1),
	)

	return NewTextResponse(result), nil
}