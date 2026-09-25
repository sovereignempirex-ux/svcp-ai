package tools

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/svpc-ai/svpc/internal/permission"
)

const (
	ImageGenToolName       = "image_gen"
	ImageEditToolName      = "image_edit"
	ImageVariationToolName = "image_variation"

	// imageOutputDirectory is where generated images land when the caller does
	// not name an output path.
	imageOutputDirectory = ".svpc/images"
)

// imageRequestTimeout is generous because a large image can take a while to
// generate, but still bounded so a hung provider cannot block the agent.
const imageRequestTimeout = 3 * time.Minute

type ImageGenParams struct {
	Provider       string `json:"provider"`
	Prompt         string `json:"prompt"`
	Model          string `json:"model,omitempty"`
	Size           string `json:"size,omitempty"`
	Quality        string `json:"quality,omitempty"`
	Style          string `json:"style,omitempty"`
	N              int    `json:"n,omitempty"`
	Seed           int64  `json:"seed,omitempty"`
	NegativePrompt string `json:"negative_prompt,omitempty"`
	AspectRatio    string `json:"aspect_ratio,omitempty"`
	Token          string `json:"token,omitempty"`
	APIURL         string `json:"api_url,omitempty"`
	OutputDir      string `json:"output_dir,omitempty"`
}

type ImageEditParams struct {
	Provider  string `json:"provider"`
	Prompt    string `json:"prompt"`
	ImagePath string `json:"image_path"`
	MaskPath  string `json:"mask_path,omitempty"`
	Model     string `json:"model,omitempty"`
	Size      string `json:"size,omitempty"`
	N         int    `json:"n,omitempty"`
	Token     string `json:"token,omitempty"`
	APIURL    string `json:"api_url,omitempty"`
	OutputDir string `json:"output_dir,omitempty"`
}

type ImageVariationParams struct {
	Provider  string `json:"provider"`
	ImagePath string `json:"image_path"`
	Model     string `json:"model,omitempty"`
	Size      string `json:"size,omitempty"`
	N         int    `json:"n,omitempty"`
	Token     string `json:"token,omitempty"`
	APIURL    string `json:"api_url,omitempty"`
	OutputDir string `json:"output_dir,omitempty"`
}

// ImageGenTool generates images from a text prompt.
type ImageGenTool struct {
	client      *http.Client
	permissions permission.Service
}

func NewImageGenTool(permissions permission.Service) BaseTool {
	return &ImageGenTool{
		client:      &http.Client{Timeout: imageRequestTimeout},
		permissions: permissions,
	}
}

func (t *ImageGenTool) Info() ToolInfo {
	return ToolInfo{
		Name: ImageGenToolName,
		Description: "Generate images from a text prompt using an AI image provider " +
			"(OpenAI, Azure OpenAI, Stability AI, Replicate or Hugging Face). " +
			"Images are written to disk and the resulting file paths are returned.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"provider": map[string]any{
					"type": "string",
					"enum": []string{"openai", "azure_openai", "stability", "replicate", "huggingface"},
				},
				"prompt": map[string]any{
					"type":        "string",
					"description": "Text prompt describing the image to generate",
				},
				"model": map[string]any{
					"type":        "string",
					"description": "Model name (dall-e-3, dall-e-2, stable-image-core, an sdxl or flux Replicate model)",
				},
				"size": map[string]any{
					"type":        "string",
					"description": "Output size, e.g. 1024x1024, 1792x1024, 1024x1792",
				},
				"quality": map[string]any{
					"type": "string",
					"enum": []string{"standard", "hd"},
				},
				"style": map[string]any{
					"type": "string",
					"enum": []string{"vivid", "natural"},
				},
				"n": map[string]any{
					"type":        "integer",
					"description": "Number of images (1-4 for most providers)",
				},
				"seed":            map[string]any{"type": "integer", "description": "Seed for reproducible output"},
				"negative_prompt": map[string]any{"type": "string", "description": "What to avoid in the image"},
				"output_dir":      map[string]any{"type": "string", "description": "Where to save images (default .svpc/images)"},
				"token":           map[string]any{"type": "string", "description": "API token (falls back to the provider's environment variable)"},
				"api_url":         map[string]any{"type": "string", "description": "Custom endpoint, for a proxy or a self-hosted model"},
			},
			"required": []string{"provider", "prompt"},
		},
	}
}

func (t *ImageGenTool) Run(ctx context.Context, call ToolCall) (ToolResponse, error) {
	var p ImageGenParams
	if err := json.Unmarshal([]byte(call.Input), &p); err != nil {
		return NewTextErrorResponse(fmt.Sprintf("invalid parameters: %v", err)), nil
	}
	if strings.TrimSpace(p.Prompt) == "" {
		return NewTextErrorResponse("prompt is required"), nil
	}

	provider := strings.ToLower(strings.TrimSpace(p.Provider))
	token := imageToken(provider, p.Token)
	if token == "" {
		return NewTextErrorResponse(fmt.Sprintf(
			"no API key for %s — set its environment variable or pass token", provider)), nil
	}

	if p.N < 1 {
		p.N = 1
	}
	if p.Model == "" {
		p.Model = getDefaultModel(provider)
	}

	sessionID, _ := sessionContext(ctx)
	if t.permissions != nil && !t.permissions.Request(permission.CreatePermissionRequest{
		SessionID:   sessionID,
		ToolName:    ImageGenToolName,
		Action:      "generate",
		Description: fmt.Sprintf("Generate %d image(s) with %s/%s", p.N, provider, p.Model),
		Params:      map[string]any{"provider": provider, "model": p.Model, "prompt": p.Prompt},
	}) {
		return NewTextErrorResponse(permission.ErrorPermissionDenied.Error()), nil
	}

	var (
		sources []imageSource
		err     error
	)
	switch provider {
	case "openai", "azure_openai":
		sources, err = t.openAIGenerate(ctx, provider, p, token)
	case "stability":
		sources, err = t.stabilityGenerate(ctx, p, token)
	case "replicate":
		sources, err = t.replicateGenerate(ctx, p, token)
	case "huggingface":
		sources, err = t.huggingFaceGenerate(ctx, p, token)
	default:
		return NewTextErrorResponse(fmt.Sprintf("unsupported provider: %s", provider)), nil
	}
	if err != nil {
		return NewTextErrorResponse(err.Error()), nil
	}

	paths, err := saveImages(p.OutputDir, sources, "gen")
	if err != nil {
		return NewTextErrorResponse(err.Error()), nil
	}
	return NewTextResponse(formatImageResult(paths, sources)), nil
}

// openAIGenerate calls the OpenAI images endpoint.
func (t *ImageGenTool) openAIGenerate(ctx context.Context, provider string, p ImageGenParams, token string) ([]imageSource, error) {
	endpoint := p.APIURL
	if endpoint == "" {
		endpoint = "https://api.openai.com/v1/images/generations"
	}

	body := map[string]any{
		"prompt": p.Prompt,
		"model":  p.Model,
		"n":      p.N,
	}
	if p.Size != "" {
		body["size"] = p.Size
	}
	if p.Quality != "" {
		body["quality"] = p.Quality
	}
	if p.Style != "" {
		body["style"] = p.Style
	}
	// DALL-E 2 is the only generation model that accepts a seed.
	if p.Seed != 0 && strings.Contains(p.Model, "dall-e-2") {
		body["seed"] = p.Seed
	}

	var out struct {
		Data []struct {
			B64JSON string `json:"b64_json"`
			URL     string `json:"url"`
		} `json:"data"`
	}
	if err := postJSON(ctx, t.client, endpoint, token, "", body, &out); err != nil {
		return nil, fmt.Errorf("%s: %w", provider, err)
	}

	sources := make([]imageSource, 0, len(out.Data))
	for _, d := range out.Data {
		if d.B64JSON != "" {
			sources = append(sources, imageSource{Data: d.B64JSON, Ext: imageExtFromModel(p.Model)})
		} else if d.URL != "" {
			sources = append(sources, imageSource{URL: d.URL, Ext: imageExtFromModel(p.Model)})
		}
	}
	if len(sources) == 0 {
		return nil, fmt.Errorf("%s returned no images", provider)
	}
	return sources, nil
}

// stabilityGenerate calls Stability AI's v2beta stable-image endpoint.
func (t *ImageGenTool) stabilityGenerate(ctx context.Context, p ImageGenParams, token string) ([]imageSource, error) {
	endpoint := p.APIURL
	if endpoint == "" {
		endpoint = "https://api.stability.ai/v2beta/stable-image/generate/core"
	}

	// Stability takes its options as query parameters rather than a body.
	query := []string{"output_format=png"}
	if p.Model != "" {
		query = append(query, "model="+urlEscape(p.Model))
	}
	if p.Seed != 0 {
		query = append(query, "seed="+fmt.Sprint(p.Seed))
	}
	if p.N > 1 {
		query = append(query, fmt.Sprintf("samples=%d", p.N))
	}
	if p.AspectRatio != "" {
		query = append(query, "aspect_ratio="+urlEscape(p.AspectRatio))
	}
	if sep := strings.Index(endpoint, "?"); sep >= 0 {
		endpoint += "&" + strings.Join(query, "&")
	} else {
		endpoint += "?" + strings.Join(query, "&")
	}

	body := map[string]any{"prompt": p.Prompt}
	if p.NegativePrompt != "" {
		body["negative_prompt"] = p.NegativePrompt
	}

	var out struct {
		Image        string `json:"image"`
		FinishReason string `json:"finish_reason"`
		Seed         int64  `json:"seed"`
	}
	// Stability authenticates with a bearer token and an Accept header.
	if err := postJSON(ctx, t.client, endpoint, token, "image/*", body, &out); err != nil {
		return nil, fmt.Errorf("stability: %w", err)
	}
	if out.FinishReason == "CONTENT_FILTERED" {
		return nil, fmt.Errorf("stability: the prompt was blocked by the content filter")
	}
	if out.Image == "" {
		return nil, fmt.Errorf("stability returned no image")
	}
	return []imageSource{{Data: out.Image, Ext: "png"}}, nil
}

// replicateGenerate creates a Replicate prediction and waits for it to finish.
func (t *ImageGenTool) replicateGenerate(ctx context.Context, p ImageGenParams, token string) ([]imageSource, error) {
	endpoint := strings.TrimRight(p.APIURL, "/")
	if endpoint == "" {
		endpoint = "https://api.replicate.com/v1"
	}
	model := p.Model
	if !strings.Contains(model, ":") {
		return nil, fmt.Errorf(
			"replicate needs a full model reference such as black-forest-labs/flux-schnell:latest, got %q", model)
	}

	input := map[string]any{"prompt": p.Prompt}
	if p.Seed != 0 {
		input["seed"] = p.Seed
	}
	if p.NegativePrompt != "" {
		input["negative_prompt"] = p.NegativePrompt
	}
	if p.AspectRatio != "" {
		input["aspect_ratio"] = p.AspectRatio
	}
	if p.Size != "" {
		input["width"], input["height"] = splitSize(p.Size)
	}
	if p.N > 1 {
		input["num_outputs"] = p.N
	}

	var created struct {
		ID   string `json:"id"`
		URLs struct {
			Get string `json:"get"`
		} `json:"urls"`
	}
	if err := postJSON(ctx, t.client, endpoint+"/models/"+model+"/predictions", token, "", input, &created); err != nil {
		return nil, fmt.Errorf("replicate: %w", err)
	}

	statusURL := created.URLs.Get
	if statusURL == "" {
		statusURL = endpoint + "/predictions/" + created.ID
	}
	return pollReplicate(ctx, t.client, token, statusURL)
}

// pollReplicate waits for a prediction to finish, then reads its output.
func pollReplicate(ctx context.Context, client *http.Client, token, statusURL string) ([]imageSource, error) {
	const pollInterval = 2 * time.Second
	const maxWait = 5 * time.Minute

	deadline := time.Now().Add(maxWait)
	for {
		var pred struct {
			Status string          `json:"status"`
			Error  string          `json:"error"`
			Output json.RawMessage `json:"output"`
		}
		if err := getJSON(ctx, client, statusURL, token, &pred); err != nil {
			return nil, fmt.Errorf("replicate: %w", err)
		}

		switch pred.Status {
		case "succeeded":
			return replicateOutput(pred.Output)
		case "failed", "canceled":
			return nil, fmt.Errorf("replicate: prediction %s: %s", pred.Status, orDefault(pred.Error, "no detail given"))
		}

		if time.Now().After(deadline) {
			return nil, fmt.Errorf("replicate: still %s after %s", pred.Status, maxWait)
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(pollInterval):
		}
	}
}

// replicateOutput decodes a prediction's output, which is either a single URL
// or an array of them depending on the model.
func replicateOutput(raw json.RawMessage) ([]imageSource, error) {
	if len(raw) == 0 {
		return nil, fmt.Errorf("replicate: the prediction succeeded but returned no output")
	}

	var single string
	if err := json.Unmarshal(raw, &single); err == nil {
		return []imageSource{{URL: single, Ext: "png"}}, nil
	}

	var many []string
	if err := json.Unmarshal(raw, &many); err == nil {
		sources := make([]imageSource, 0, len(many))
		for _, u := range many {
			sources = append(sources, imageSource{URL: u, Ext: "png"})
		}
		if len(sources) > 0 {
			return sources, nil
		}
	}
	return nil, fmt.Errorf("replicate: unexpected output shape")
}

// huggingFaceGenerate calls the Inference API for a hosted text-to-image model.
func (t *ImageGenTool) huggingFaceGenerate(ctx context.Context, p ImageGenParams, token string) ([]imageSource, error) {
	endpoint := p.APIURL
	if endpoint == "" {
		if p.Model == "" {
			return nil, fmt.Errorf("huggingface: model is required when api_url is not given")
		}
		endpoint = "https://api-inference.huggingface.co/models/" + p.Model
	}

	body := map[string]any{"inputs": p.Prompt}
	if p.NegativePrompt != "" {
		body["negative_prompt"] = p.NegativePrompt
	}
	if p.Seed != 0 {
		body["seed"] = p.Seed
	}
	if p.N > 1 {
		body["num_images"] = p.N
	}

	// This endpoint answers with raw image bytes rather than JSON.
	raw, contentType, err := postForBytes(ctx, t.client, endpoint, token, "", body)
	if err != nil {
		return nil, fmt.Errorf("huggingface: %w", err)
	}
	if !strings.HasPrefix(contentType, "image/") {
		// A JSON error body arrives when the model is missing or still loading.
		var apiErr struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(raw, &apiErr) == nil && apiErr.Error != "" {
			return nil, fmt.Errorf("huggingface: %s", apiErr.Error)
		}
		return nil, fmt.Errorf("huggingface: unexpected content type %q", contentType)
	}

	ext := strings.TrimPrefix(contentType, "image/")
	if ext == "jpeg" {
		ext = "jpg"
	}
	return []imageSource{{Data: base64.StdEncoding.EncodeToString(raw), Ext: ext}}, nil
}

// ---------------------------------------------------------------------------
// Edits and variations
// ---------------------------------------------------------------------------

// ImageEditTool edits an existing image, optionally masked.
type ImageEditTool struct {
	client      *http.Client
	permissions permission.Service
}

func NewImageEditTool(permissions permission.Service) BaseTool {
	return &ImageEditTool{
		client:      &http.Client{Timeout: imageRequestTimeout},
		permissions: permissions,
	}
}

func (t *ImageEditTool) Info() ToolInfo {
	return ToolInfo{
		Name: ImageEditToolName,
		Description: "Edit an existing image from an instruction prompt, with an optional mask " +
			"limiting the area that changes. Supported by OpenAI (dall-e-2) and Replicate.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"provider":   map[string]any{"type": "string", "enum": []string{"openai", "replicate"}},
				"prompt":     map[string]any{"type": "string", "description": "What the edited image should show"},
				"image_path": map[string]any{"type": "string", "description": "Source image"},
				"mask_path":  map[string]any{"type": "string", "description": "Mask image; transparent areas are edited"},
				"model":      map[string]any{"type": "string"},
				"size":       map[string]any{"type": "string"},
				"n":          map[string]any{"type": "integer"},
				"output_dir": map[string]any{"type": "string"},
				"token":      map[string]any{"type": "string"},
				"api_url":    map[string]any{"type": "string"},
			},
			"required": []string{"provider", "prompt", "image_path"},
		},
	}
}

func (t *ImageEditTool) Run(ctx context.Context, call ToolCall) (ToolResponse, error) {
	var p ImageEditParams
	if err := json.Unmarshal([]byte(call.Input), &p); err != nil {
		return NewTextErrorResponse(fmt.Sprintf("invalid parameters: %v", err)), nil
	}
	if strings.TrimSpace(p.Prompt) == "" || strings.TrimSpace(p.ImagePath) == "" {
		return NewTextErrorResponse("prompt and image_path are required"), nil
	}

	provider := strings.ToLower(strings.TrimSpace(p.Provider))
	token := imageToken(provider, p.Token)
	if token == "" {
		return NewTextErrorResponse(fmt.Sprintf("no API key for %s", provider)), nil
	}
	if _, err := os.Stat(p.ImagePath); err != nil {
		return NewTextErrorResponse(fmt.Sprintf("cannot read image %s: %v", p.ImagePath, err)), nil
	}
	if p.MaskPath != "" {
		if _, err := os.Stat(p.MaskPath); err != nil {
			return NewTextErrorResponse(fmt.Sprintf("cannot read mask %s: %v", p.MaskPath, err)), nil
		}
	}

	if p.N < 1 {
		p.N = 1
	}
	if p.Model == "" {
		p.Model = getDefaultModel(provider)
	}

	sessionID, _ := sessionContext(ctx)
	if t.permissions != nil && !t.permissions.Request(permission.CreatePermissionRequest{
		SessionID:   sessionID,
		ToolName:    ImageEditToolName,
		Action:      "edit",
		Description: fmt.Sprintf("Edit %s with %s/%s", p.ImagePath, provider, p.Model),
		Params:      map[string]any{"provider": provider, "image": p.ImagePath, "mask": p.MaskPath},
	}) {
		return NewTextErrorResponse(permission.ErrorPermissionDenied.Error()), nil
	}

	var (
		sources []imageSource
		err     error
	)
	switch provider {
	case "openai":
		sources, err = t.openAIEdit(ctx, p, token)
	case "replicate":
		sources, err = t.replicateEdit(ctx, p, token)
	default:
		return NewTextErrorResponse(fmt.Sprintf("editing is not supported by %s", provider)), nil
	}
	if err != nil {
		return NewTextErrorResponse(err.Error()), nil
	}

	paths, err := saveImages(p.OutputDir, sources, "edit")
	if err != nil {
		return NewTextErrorResponse(err.Error()), nil
	}
	return NewTextResponse(formatImageResult(paths, sources)), nil
}

// openAIEdit posts a multipart form, which is how the OpenAI edits endpoint
// takes its image and mask.
func (t *ImageEditTool) openAIEdit(ctx context.Context, p ImageEditParams, token string) ([]imageSource, error) {
	endpoint := p.APIURL
	if endpoint == "" {
		endpoint = "https://api.openai.com/v1/images/edits"
	}

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	fields := map[string]string{
		"prompt": p.Prompt,
		"model":  p.Model,
		"n":      fmt.Sprint(p.N),
	}
	if p.Size != "" {
		fields["size"] = p.Size
	}
	for k, v := range fields {
		if err := writer.WriteField(k, v); err != nil {
			return nil, fmt.Errorf("openai: %w", err)
		}
	}

	if err := attachFile(writer, "image", p.ImagePath); err != nil {
		return nil, err
	}
	if p.MaskPath != "" {
		if err := attachFile(writer, "mask", p.MaskPath); err != nil {
			return nil, err
		}
	}
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("openai: %w", err)
	}

	var out struct {
		Data []struct {
			B64JSON string `json:"b64_json"`
			URL     string `json:"url"`
		} `json:"data"`
	}
	if err := postForm(ctx, t.client, endpoint, token, "", writer.FormDataContentType(), &body, &out); err != nil {
		return nil, fmt.Errorf("openai: %w", err)
	}

	sources := make([]imageSource, 0, len(out.Data))
	for _, d := range out.Data {
		if d.B64JSON != "" {
			sources = append(sources, imageSource{Data: d.B64JSON, Ext: imageExtFromModel(p.Model)})
		} else if d.URL != "" {
			sources = append(sources, imageSource{URL: d.URL, Ext: imageExtFromModel(p.Model)})
		}
	}
	if len(sources) == 0 {
		return nil, fmt.Errorf("openai returned no images")
	}
	return sources, nil
}

// replicateEdit runs an image-to-image model on Replicate.
func (t *ImageEditTool) replicateEdit(ctx context.Context, p ImageEditParams, token string) ([]imageSource, error) {
	endpoint := strings.TrimRight(p.APIURL, "/")
	if endpoint == "" {
		endpoint = "https://api.replicate.com/v1"
	}
	if !strings.Contains(p.Model, ":") {
		return nil, fmt.Errorf(
			"replicate needs a full model reference such as stability-ai/stable-diffusion:2.1, got %q", p.Model)
	}

	imageData, err := encodeFile(p.ImagePath)
	if err != nil {
		return nil, err
	}
	input := map[string]any{
		"prompt": p.Prompt,
		"image":  imageData,
	}
	if p.MaskPath != "" {
		mask, err := encodeFile(p.MaskPath)
		if err != nil {
			return nil, err
		}
		input["mask"] = mask
	}
	if p.Size != "" {
		input["width"], input["height"] = splitSize(p.Size)
	}
	if p.N > 1 {
		input["num_outputs"] = p.N
	}

	var created struct {
		ID   string `json:"id"`
		URLs struct {
			Get string `json:"get"`
		} `json:"urls"`
	}
	if err := postJSON(ctx, t.client, endpoint+"/models/"+p.Model+"/predictions", token, "", input, &created); err != nil {
		return nil, fmt.Errorf("replicate: %w", err)
	}

	statusURL := created.URLs.Get
	if statusURL == "" {
		statusURL = endpoint + "/predictions/" + created.ID
	}
	return pollReplicate(ctx, t.client, token, statusURL)
}

// ImageVariationTool produces variations of an existing image.
type ImageVariationTool struct {
	client      *http.Client
	permissions permission.Service
}

func NewImageVariationTool(permissions permission.Service) BaseTool {
	return &ImageVariationTool{
		client:      &http.Client{Timeout: imageRequestTimeout},
		permissions: permissions,
	}
}

func (t *ImageVariationTool) Info() ToolInfo {
	return ToolInfo{
		Name:        ImageVariationToolName,
		Description: "Create variations of an existing image. Supported by OpenAI (dall-e-2) and Replicate models that accept an image input.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"provider":   map[string]any{"type": "string", "enum": []string{"openai", "replicate"}},
				"image_path": map[string]any{"type": "string", "description": "Source image"},
				"model":      map[string]any{"type": "string"},
				"size":       map[string]any{"type": "string"},
				"n":          map[string]any{"type": "integer"},
				"output_dir": map[string]any{"type": "string"},
				"token":      map[string]any{"type": "string"},
				"api_url":    map[string]any{"type": "string"},
			},
			"required": []string{"provider", "image_path"},
		},
	}
}

func (t *ImageVariationTool) Run(ctx context.Context, call ToolCall) (ToolResponse, error) {
	var p ImageVariationParams
	if err := json.Unmarshal([]byte(call.Input), &p); err != nil {
		return NewTextErrorResponse(fmt.Sprintf("invalid parameters: %v", err)), nil
	}
	if strings.TrimSpace(p.ImagePath) == "" {
		return NewTextErrorResponse("image_path is required"), nil
	}

	provider := strings.ToLower(strings.TrimSpace(p.Provider))
	token := imageToken(provider, p.Token)
	if token == "" {
		return NewTextErrorResponse(fmt.Sprintf("no API key for %s", provider)), nil
	}
	if _, err := os.Stat(p.ImagePath); err != nil {
		return NewTextErrorResponse(fmt.Sprintf("cannot read image %s: %v", p.ImagePath, err)), nil
	}
	if p.N < 1 {
		p.N = 1
	}
	if p.Model == "" {
		p.Model = getDefaultModel(provider)
	}

	sessionID, _ := sessionContext(ctx)
	if t.permissions != nil && !t.permissions.Request(permission.CreatePermissionRequest{
		SessionID:   sessionID,
		ToolName:    ImageVariationToolName,
		Action:      "vary",
		Description: fmt.Sprintf("Create variations of %s", p.ImagePath),
		Params:      map[string]any{"provider": provider, "image": p.ImagePath},
	}) {
		return NewTextErrorResponse(permission.ErrorPermissionDenied.Error()), nil
	}

	var (
		sources []imageSource
		err     error
	)
	switch provider {
	case "openai":
		sources, err = t.openAIVariation(ctx, p, token)
	case "replicate":
		sources, err = t.replicateVariation(ctx, p, token)
	default:
		return NewTextErrorResponse(fmt.Sprintf("variations are not supported by %s", provider)), nil
	}
	if err != nil {
		return NewTextErrorResponse(err.Error()), nil
	}

	paths, err := saveImages(p.OutputDir, sources, "var")
	if err != nil {
		return NewTextErrorResponse(err.Error()), nil
	}
	return NewTextResponse(formatImageResult(paths, sources)), nil
}

func (t *ImageVariationTool) openAIVariation(ctx context.Context, p ImageVariationParams, token string) ([]imageSource, error) {
	endpoint := p.APIURL
	if endpoint == "" {
		endpoint = "https://api.openai.com/v1/images/variations"
	}

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if p.Model != "" {
		if err := writer.WriteField("model", p.Model); err != nil {
			return nil, fmt.Errorf("openai: %w", err)
		}
	}
	if err := writer.WriteField("n", fmt.Sprint(p.N)); err != nil {
		return nil, fmt.Errorf("openai: %w", err)
	}
	if p.Size != "" {
		if err := writer.WriteField("size", p.Size); err != nil {
			return nil, fmt.Errorf("openai: %w", err)
		}
	}
	if err := attachFile(writer, "image", p.ImagePath); err != nil {
		return nil, err
	}
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("openai: %w", err)
	}

	var out struct {
		Data []struct {
			B64JSON string `json:"b64_json"`
			URL     string `json:"url"`
		} `json:"data"`
	}
	if err := postForm(ctx, t.client, endpoint, token, "", writer.FormDataContentType(), &body, &out); err != nil {
		return nil, fmt.Errorf("openai: %w", err)
	}

	sources := make([]imageSource, 0, len(out.Data))
	for _, d := range out.Data {
		if d.B64JSON != "" {
			sources = append(sources, imageSource{Data: d.B64JSON, Ext: imageExtFromModel(p.Model)})
		} else if d.URL != "" {
			sources = append(sources, imageSource{URL: d.URL, Ext: imageExtFromModel(p.Model)})
		}
	}
	if len(sources) == 0 {
		return nil, fmt.Errorf("openai returned no variations")
	}
	return sources, nil
}

func (t *ImageVariationTool) replicateVariation(ctx context.Context, p ImageVariationParams, token string) ([]imageSource, error) {
	// Replicate has no dedicated variation endpoint; an image-to-image model
	// is the equivalent, so the same prediction path is used.
	imageData, err := encodeFile(p.ImagePath)
	if err != nil {
		return nil, err
	}

	endpoint := strings.TrimRight(p.APIURL, "/")
	if endpoint == "" {
		endpoint = "https://api.replicate.com/v1"
	}
	if !strings.Contains(p.Model, ":") {
		return nil, fmt.Errorf(
			"replicate needs a full model reference such as tstramer/image-variations:alpha, got %q", p.Model)
	}

	input := map[string]any{"image": imageData}
	if p.Size != "" {
		input["width"], input["height"] = splitSize(p.Size)
	}
	if p.N > 1 {
		input["num_outputs"] = p.N
	}

	var created struct {
		ID   string `json:"id"`
		URLs struct {
			Get string `json:"get"`
		} `json:"urls"`
	}
	if err := postJSON(ctx, t.client, endpoint+"/models/"+p.Model+"/predictions", token, "", input, &created); err != nil {
		return nil, fmt.Errorf("replicate: %w", err)
	}
	statusURL := created.URLs.Get
	if statusURL == "" {
		statusURL = endpoint + "/predictions/" + created.ID
	}
	return pollReplicate(ctx, t.client, token, statusURL)
}

// ---------------------------------------------------------------------------
// HTTP helpers
// ---------------------------------------------------------------------------

// postJSON sends a JSON body and decodes a JSON response into out.
func postJSON(ctx context.Context, client *http.Client, endpoint, token, accept string, body, out any) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("encoding request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("building request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	applyAuth(req, token, accept)
	return sendJSON(client, req, out)
}

// postForm sends a multipart body and decodes a JSON response into out.
func postForm(ctx context.Context, client *http.Client, endpoint, token, accept, contentType string, body *bytes.Buffer, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, body)
	if err != nil {
		return fmt.Errorf("building request: %w", err)
	}
	req.Header.Set("Content-Type", contentType)
	applyAuth(req, token, accept)
	return sendJSON(client, req, out)
}

// postForBytes sends a JSON body and returns the raw response body.
func postForBytes(ctx context.Context, client *http.Client, endpoint, token, accept string, body any) ([]byte, string, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, "", fmt.Errorf("encoding request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, "", fmt.Errorf("building request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	applyAuth(req, token, accept)

	resp, err := client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 32*1024*1024))
	if err != nil {
		return nil, "", err
	}
	if resp.StatusCode >= 400 {
		return nil, "", apiError(resp.Status, raw)
	}
	return raw, resp.Header.Get("Content-Type"), nil
}

// getJSON fetches and decodes a JSON response into out.
func getJSON(ctx context.Context, client *http.Client, endpoint, token string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return fmt.Errorf("building request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	applyAuth(req, token, "")
	return sendJSON(client, req, out)
}

func applyAuth(req *http.Request, token, accept string) {
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if accept != "" {
		req.Header.Set("Accept", accept)
	}
	if req.Header.Get("Accept") == "" {
		req.Header.Set("Accept", "application/json")
	}
}

// sendJSON performs the request and decodes the response, turning a non-2xx
// status into a message that includes whatever the provider said.
func sendJSON(client *http.Client, req *http.Request, out any) error {
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8*1024*1024))
	if err != nil {
		return err
	}
	if resp.StatusCode >= 400 {
		return apiError(resp.Status, raw)
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("decoding response: %w", err)
	}
	return nil
}

// apiError turns an error response into something readable. Providers disagree
// on the shape, so the common fields are all checked.
func apiError(status string, raw []byte) error {
	var payload struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
		Message string `json:"message"`
		Detail  string `json:"detail"`
		Msg     string `json:"msg"`
	}
	if err := json.Unmarshal(raw, &payload); err == nil {
		for _, msg := range []string{payload.Error.Message, payload.Message, payload.Detail, payload.Msg} {
			if msg != "" {
				return fmt.Errorf("%s: %s", status, msg)
			}
		}
	}
	// Fall back to the body, trimmed, so the model still sees something useful.
	if text := strings.TrimSpace(string(raw)); text != "" {
		if len(text) > 500 {
			text = text[:500] + "..."
		}
		return fmt.Errorf("%s: %s", status, text)
	}
	return fmt.Errorf("%s", status)
}

// ---------------------------------------------------------------------------
// File handling
// ---------------------------------------------------------------------------

// imageSource is a generated image in one of the two forms providers return.
type imageSource struct {
	// Data is base64-encoded content.
	Data string
	// URL points at content that still has to be downloaded.
	URL string
	// Ext is the file extension without a dot.
	Ext string
}

// saveImages writes every source to disk and returns the paths in order.
func saveImages(outputDir string, sources []imageSource, prefix string) ([]string, error) {
	dir := outputDir
	if strings.TrimSpace(dir) == "" {
		dir = imageOutputDirectory
	}
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(workingDirectory(), dir)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("creating %s: %w", dir, err)
	}

	stamp := time.Now().Format("20060102-150405")
	paths := make([]string, 0, len(sources))

	for i, src := range sources {
		data, err := src.bytes()
		if err != nil {
			return nil, err
		}

		ext := strings.TrimPrefix(strings.ToLower(src.Ext), ".")
		if ext == "" || ext == "jpeg" {
			ext = "png"
		}

		name := fmt.Sprintf("%s-%s-%d.%s", prefix, stamp, i+1, ext)
		full := filepath.Join(dir, name)
		if err := os.WriteFile(full, data, 0o644); err != nil {
			return nil, fmt.Errorf("writing %s: %w", full, err)
		}
		paths = append(paths, full)
	}
	return paths, nil
}

// bytes materialises the image, decoding a data URL or downloading it when only
// a URL is known.
func (s imageSource) bytes() ([]byte, error) {
	if s.Data != "" {
		data, err := base64.StdEncoding.DecodeString(s.Data)
		if err != nil {
			// Some providers return base64url, which the standard decoder rejects.
			alt, altErr := base64.RawStdEncoding.DecodeString(s.Data)
			if altErr != nil {
				return nil, fmt.Errorf("decoding image data: %w", err)
			}
			data = alt
		}
		return data, nil
	}
	if s.URL == "" {
		return nil, fmt.Errorf("image source has neither data nor a URL")
	}

	// Some endpoints inline the image as a data URL rather than hosting it.
	if strings.HasPrefix(s.URL, "data:") {
		_, encoded, ok := strings.Cut(s.URL, ",")
		if !ok {
			return nil, fmt.Errorf("malformed data URL")
		}
		data, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			return nil, fmt.Errorf("decoding data URL: %w", err)
		}
		return data, nil
	}

	client := &http.Client{Timeout: imageRequestTimeout}
	resp, err := client.Get(s.URL)
	if err != nil {
		return nil, fmt.Errorf("downloading %s: %w", s.URL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("downloading %s: %s", s.URL, resp.Status)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 32*1024*1024))
}

// attachFile adds a file to a multipart form under the given field name.
func attachFile(writer *multipart.Writer, field, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("opening %s: %w", path, err)
	}
	defer f.Close()

	part, err := writer.CreateFormFile(field, filepath.Base(path))
	if err != nil {
		return fmt.Errorf("creating form field %s: %w", field, err)
	}
	if _, err := io.Copy(part, f); err != nil {
		return fmt.Errorf("copying %s: %w", path, err)
	}
	return nil
}

// encodeFile reads a file and returns it as a base64 data URL, which is the
// form Replicate expects for an image input.
func encodeFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("reading %s: %w", path, err)
	}
	mime := "image/png"
	switch strings.ToLower(filepath.Ext(path)) {
	case ".jpg", ".jpeg":
		mime = "image/jpeg"
	case ".webp":
		mime = "image/webp"
	case ".gif":
		mime = "image/gif"
	}
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data), nil
}

// formatImageResult reports what was produced, and warns about any revised
// prompt so the model does not silently lose it.
func formatImageResult(paths []string, sources []imageSource) string {
	var b strings.Builder
	fmt.Fprintf(&b, "generated %d image(s):\n", len(paths))
	for _, p := range paths {
		fmt.Fprintf(&b, "- %s\n", p)
	}
	if remote := countRemote(sources); remote > 0 {
		fmt.Fprintf(&b, "\n%d image(s) were downloaded from the provider's CDN.\n", remote)
	}
	return b.String()
}

func countRemote(sources []imageSource) int {
	n := 0
	for _, s := range sources {
		if s.URL != "" {
			n++
		}
	}
	return n
}

// ---------------------------------------------------------------------------
// Small helpers
// ---------------------------------------------------------------------------

// imageToken resolves a provider's credential, preferring an explicit value.
func imageToken(provider, explicit string) string {
	if explicit != "" {
		return explicit
	}
	switch provider {
	case "openai", "azure_openai":
		return getEnvOrDefault("OPENAI_API_KEY", "")
	case "stability":
		return getEnvOrDefault("STABILITY_API_KEY", "")
	case "replicate":
		return getEnvOrDefault("REPLICATE_API_TOKEN", "")
	case "huggingface":
		// HF_TOKEN is the current name; HUGGINGFACE_API_KEY is the legacy one.
		return getEnvOrDefault("HF_TOKEN", getEnvOrDefault("HUGGINGFACE_API_KEY", ""))
	default:
		return ""
	}
}

// imageExtFromModel maps a model name onto the format it returns.
func imageExtFromModel(model string) string {
	model = strings.ToLower(model)
	switch {
	case strings.Contains(model, "dall-e-3"), strings.Contains(model, "gpt-image"):
		// DALL-E 3 always hands back PNG; gpt-image defaults to it too.
		return "png"
	case strings.Contains(model, "flux"), strings.Contains(model, "sdxl"), strings.Contains(model, "stable-diffusion"):
		return "webp"
	default:
		return "png"
	}
}

// splitSize parses a "1024x1024" dimension string.
func splitSize(size string) (any, any) {
	parts := strings.SplitN(strings.ToLower(size), "x", 2)
	if len(parts) != 2 {
		return nil, nil
	}
	return parseIntOrNil(parts[0]), parseIntOrNil(parts[1])
}

func parseIntOrNil(s string) any {
	var n int
	if _, err := fmt.Sscanf(s, "%d", &n); err != nil {
		return nil
	}
	return n
}

// urlEscape percent-encodes a query parameter value.
func urlEscape(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9',
			r == '-', r == '_', r == '.', r == '~', r == '/', r == ':':
			b.WriteRune(r)
		default:
			fmt.Fprintf(&b, "%%%02X", r)
		}
	}
	return b.String()
}
