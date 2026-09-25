package tools

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestImageGenTool_Info(t *testing.T) {
	info := NewImageGenTool(nil).Info()

	if info.Name != ImageGenToolName {
		t.Errorf("expected name %q, got %q", ImageGenToolName, info.Name)
	}
	if info.Description == "" {
		t.Error("description should not be empty")
	}

	props := info.Parameters["properties"].(map[string]any)
	for _, want := range []string{"provider", "prompt", "model", "size", "quality", "style", "n", "seed", "negative_prompt", "output_dir"} {
		if props[want] == nil {
			t.Errorf("missing property: %s", want)
		}
	}
	for _, want := range []string{"provider", "prompt"} {
		if !containsString(info.Parameters["required"].([]string), want) {
			t.Errorf("missing required parameter: %s", want)
		}
	}
	enum := props["provider"].(map[string]any)["enum"].([]string)
	for _, want := range []string{"openai", "stability", "replicate", "huggingface"} {
		if !containsString(enum, want) {
			t.Errorf("missing provider in enum: %s", want)
		}
	}
}

func TestImageGenTool_Run_Validation(t *testing.T) {
	cases := []struct {
		name  string
		input string
	}{
		{"invalid json", `{invalid json`},
		{"missing prompt", `{"provider": "openai"}`},
		{"blank prompt", `{"provider": "openai", "prompt": "  "}`},
	}

	tool := NewImageGenTool(nil)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := tool.Run(context.Background(), ToolCall{
				ID: "test", Name: ImageGenToolName, Input: tc.input,
			})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !res.IsError {
				t.Error("expected an error response")
			}
		})
	}
}

func TestImageGenTool_Run_MissingToken(t *testing.T) {
	// Clearing the environment proves the tool does not silently proceed
	// without a credential.
	t.Setenv("OPENAI_API_KEY", "")

	res, err := NewImageGenTool(nil).Run(context.Background(), ToolCall{
		ID:    "test",
		Name:  ImageGenToolName,
		Input: `{"provider": "openai", "prompt": "a cat"}`,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.IsError {
		t.Error("expected an error response when no API key is configured")
	}
	if !strings.Contains(res.Content, "API key") {
		t.Errorf("the error should mention the missing key, got %q", res.Content)
	}
}

func TestImageGenTool_Run_UnsupportedProvider(t *testing.T) {
	res, err := NewImageGenTool(nil).Run(context.Background(), ToolCall{
		ID:    "test",
		Name:  ImageGenToolName,
		Input: `{"provider": "dalle2", "prompt": "a cat", "token": "t"}`,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.IsError {
		t.Error("expected an error response for an unsupported provider")
	}
}

func TestImageGenTool_WritesBase64Result(t *testing.T) {
	// A one pixel PNG, so the test does not need an image library.
	png := base64.StdEncoding.EncodeToString([]byte{
		0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A,
		0x00, 0x01, 0x02, 0x03,
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Errorf("unexpected auth header %q", got)
		}
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		if body["prompt"] != "a cat" {
			t.Errorf("prompt not forwarded: %v", body)
		}
		if body["model"] != "dall-e-3" {
			t.Errorf("model should default to dall-e-3, got %v", body["model"])
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{{"b64_json": png}},
		})
	}))
	defer server.Close()

	dir := t.TempDir()
	res, err := NewImageGenTool(nil).Run(context.Background(), ToolCall{
		ID:   "test",
		Name: ImageGenToolName,
		Input: `{"provider":"openai","prompt":"a cat","token":"test-token","api_url":` +
			quote(server.URL) + `,"output_dir":` + quote(dir) + `}`,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected error response: %s", res.Content)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading the output directory: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected one image, got %d", len(entries))
	}
	if !strings.HasSuffix(entries[0].Name(), ".png") {
		t.Errorf("unexpected file name %q", entries[0].Name())
	}
	if !strings.Contains(res.Content, filepath.Join(dir, entries[0].Name())) {
		t.Errorf("the response should name the written file, got %q", res.Content)
	}
}

func TestImageGenTool_ReportsProviderError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":{"message":"billing hard limit reached"}}`))
	}))
	defer server.Close()

	res, err := NewImageGenTool(nil).Run(context.Background(), ToolCall{
		ID:   "test",
		Name: ImageGenToolName,
		Input: `{"provider":"openai","prompt":"a cat","token":"t","api_url":` +
			quote(server.URL) + `}`,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.IsError {
		t.Fatal("expected an error response")
	}
	// The provider's own message is far more useful than a bare status code.
	if !strings.Contains(res.Content, "billing hard limit reached") {
		t.Errorf("the provider message should survive, got %q", res.Content)
	}
}

func TestImageGenTool_StabilityPostsTheNegativePrompt(t *testing.T) {
	image := base64.StdEncoding.EncodeToString([]byte("image-bytes"))

	var gotQuery string
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("unexpected content type %q", ct)
		}
		json.NewDecoder(r.Body).Decode(&gotBody)
		json.NewEncoder(w).Encode(map[string]any{"image": image})
	}))
	defer server.Close()

	res, err := NewImageGenTool(nil).Run(context.Background(), ToolCall{
		ID:   "test",
		Name: ImageGenToolName,
		Input: `{"provider":"stability","prompt":"a cat","negative_prompt":"dogs","seed":42,` +
			`"aspect_ratio":"16:9","token":"t","api_url":` + quote(server.URL) + `}`,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected error response: %s", res.Content)
	}

	if gotBody["negative_prompt"] != "dogs" {
		t.Errorf("negative prompt not forwarded: %v", gotBody)
	}
	// Stability takes its knobs as query parameters.
	for _, want := range []string{"output_format=png", "seed=42", "aspect_ratio=16:9"} {
		if !strings.Contains(gotQuery, want) {
			t.Errorf("query missing %q, got %q", want, gotQuery)
		}
	}
}

func TestImageGenTool_StabilityContentFilter(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"image":         "",
			"finish_reason": "CONTENT_FILTERED",
		})
	}))
	defer server.Close()

	res, err := NewImageGenTool(nil).Run(context.Background(), ToolCall{
		ID:    "test",
		Name:  ImageGenToolName,
		Input: `{"provider":"stability","prompt":"x","token":"t","api_url":` + quote(server.URL) + `}`,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.IsError {
		t.Fatal("expected an error response for blocked content")
	}
	if !strings.Contains(res.Content, "content filter") {
		t.Errorf("the error should mention the filter, got %q", res.Content)
	}
}

func TestImageGenTool_ReplicateWaitsForThePrediction(t *testing.T) {
	// One pixel of PNG again.
	url := "data:image/png;base64," + base64.StdEncoding.EncodeToString([]byte{0x89, 0x50})

	var polls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			var body map[string]any
			json.NewDecoder(r.Body).Decode(&body)
			if body["prompt"] != "a cat" {
				t.Errorf("prompt not forwarded: %v", body)
			}
			if body["width"] != float64(512) {
				t.Errorf("size should be split into width/height, got %v", body)
			}
			json.NewEncoder(w).Encode(map[string]any{"id": "abc"})
			return
		}
		polls++
		if polls == 1 {
			// Still running, so the tool has to keep polling.
			json.NewEncoder(w).Encode(map[string]any{"status": "processing"})
			return
		}
		json.NewEncoder(w).Encode(map[string]any{
			"status": "succeeded",
			"output": []string{url},
		})
	}))
	defer server.Close()

	dir := t.TempDir()
	res, err := NewImageGenTool(nil).Run(context.Background(), ToolCall{
		ID:   "test",
		Name: ImageGenToolName,
		Input: `{"provider":"replicate","prompt":"a cat","model":"owner/model:tag","size":"512x512",` +
			`"token":"t","api_url":` + quote(server.URL) + `,"output_dir":` + quote(dir) + `}`,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected error response: %s", res.Content)
	}
	if polls < 2 {
		t.Errorf("expected the tool to poll until the prediction finished, got %d polls", polls)
	}

	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Errorf("expected one written image, got %d", len(entries))
	}
}

func TestImageGenTool_ReplicateNeedsAFullModelReference(t *testing.T) {
	res, err := NewImageGenTool(nil).Run(context.Background(), ToolCall{
		ID:    "test",
		Name:  ImageGenToolName,
		Input: `{"provider":"replicate","prompt":"x","model":"flux","token":"t"}`,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.IsError {
		t.Fatal("expected an error response for an ambiguous model name")
	}
	if !strings.Contains(res.Content, "owner/") && !strings.Contains(res.Content, "/") {
		t.Errorf("the error should show the expected shape, got %q", res.Content)
	}
}

func TestReplicateOutputShapes(t *testing.T) {
	single, err := replicateOutput(json.RawMessage(`"https://example.com/a.png"`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(single) != 1 || single[0].URL != "https://example.com/a.png" {
		t.Errorf("unexpected single output: %v", single)
	}

	many, err := replicateOutput(json.RawMessage(`["https://a","https://b"]`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(many) != 2 {
		t.Errorf("expected two outputs, got %d", len(many))
	}

	if _, err := replicateOutput(nil); err == nil {
		t.Error("an empty output should be an error")
	}
	if _, err := replicateOutput(json.RawMessage(`{"unexpected":true}`)); err == nil {
		t.Error("an unexpected shape should be an error")
	}
}

func TestImageEditTool_Info(t *testing.T) {
	info := NewImageEditTool(nil).Info()

	if info.Name != ImageEditToolName {
		t.Errorf("expected name %q, got %q", ImageEditToolName, info.Name)
	}
	props := info.Parameters["properties"].(map[string]any)
	for _, want := range []string{"provider", "prompt", "image_path", "mask_path", "model", "size", "n"} {
		if props[want] == nil {
			t.Errorf("missing property: %s", want)
		}
	}
}

func TestImageEditTool_Run_Validation(t *testing.T) {
	cases := []struct {
		name  string
		input string
	}{
		{"invalid json", `{invalid json`},
		{"missing prompt", `{"provider": "openai"}`},
		{"missing image", `{"provider": "openai", "prompt": "x"}`},
	}

	tool := NewImageEditTool(nil)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := tool.Run(context.Background(), ToolCall{
				ID: "test", Name: ImageEditToolName, Input: tc.input,
			})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !res.IsError {
				t.Error("expected an error response")
			}
		})
	}
}

func TestImageEditTool_ReportsAnUnreadableSource(t *testing.T) {
	res, err := NewImageEditTool(nil).Run(context.Background(), ToolCall{
		ID:   "test",
		Name: ImageEditToolName,
		Input: `{"provider":"openai","prompt":"x","image_path":` +
			quote(filepath.Join(t.TempDir(), "missing.png")) + `,"token":"t"}`,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.IsError {
		t.Fatal("expected an error response for a missing source image")
	}
	if !strings.Contains(res.Content, "cannot read image") {
		t.Errorf("the error should name the problem, got %q", res.Content)
	}
}

func TestImageEditTool_OpenAIUsesMultipart(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.png")
	writeFile(t, src, "source-bytes")

	var gotContentType, gotFilename string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotContentType = r.Header.Get("Content-Type")
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Errorf("expected a multipart body: %v", err)
		}
		file, header, err := r.FormFile("image")
		if err != nil {
			t.Errorf("the image part is missing: %v", err)
		} else {
			defer file.Close()
			gotFilename = header.Filename
		}
		json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{{"b64_json": base64.StdEncoding.EncodeToString([]byte("edited"))}},
		})
	}))
	defer server.Close()

	out := t.TempDir()
	res, err := NewImageEditTool(nil).Run(context.Background(), ToolCall{
		ID:   "test",
		Name: ImageEditToolName,
		Input: `{"provider":"openai","prompt":"add a hat","image_path":` + quote(src) +
			`,"token":"t","api_url":` + quote(server.URL) + `,"output_dir":` + quote(out) + `}`,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected error response: %s", res.Content)
	}
	if !strings.HasPrefix(gotContentType, "multipart/form-data") {
		t.Errorf("expected a multipart content type, got %q", gotContentType)
	}
	if gotFilename != "src.png" {
		t.Errorf("the upload should keep the file name, got %q", gotFilename)
	}
}

func TestImageVariationTool_Info(t *testing.T) {
	info := NewImageVariationTool(nil).Info()

	if info.Name != ImageVariationToolName {
		t.Errorf("expected name %q, got %q", ImageVariationToolName, info.Name)
	}
	props := info.Parameters["properties"].(map[string]any)
	for _, want := range []string{"provider", "image_path", "model", "size", "n"} {
		if props[want] == nil {
			t.Errorf("missing property: %s", want)
		}
	}
}

func TestImageVariationTool_Run_Validation(t *testing.T) {
	cases := []struct {
		name  string
		input string
	}{
		{"invalid json", `{invalid json`},
		{"missing image", `{"provider": "openai"}`},
	}

	tool := NewImageVariationTool(nil)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := tool.Run(context.Background(), ToolCall{
				ID: "test", Name: ImageVariationToolName, Input: tc.input,
			})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !res.IsError {
				t.Error("expected an error response")
			}
		})
	}
}

func TestImageTokenPrefersExplicitValue(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "from-env")
	if got := imageToken("openai", "explicit"); got != "explicit" {
		t.Errorf("an explicit token should win, got %q", got)
	}
	if got := imageToken("openai", ""); got != "from-env" {
		t.Errorf("the environment should be used as a fallback, got %q", got)
	}
}

func TestImageTokenHuggingFaceAlias(t *testing.T) {
	// HF_TOKEN is the newer name; HF_API_KEY is the older one.
	t.Setenv("HUGGINGFACE_API_KEY", "old")
	t.Setenv("HF_TOKEN", "new")
	if got := imageToken("huggingface", ""); got != "new" {
		t.Errorf("HF_TOKEN should be preferred, got %q", got)
	}
}

func TestSaveImagesWritesEverySource(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "out", "nested")

	paths, err := saveImages(dir, []imageSource{
		{Data: base64.StdEncoding.EncodeToString([]byte("one")), Ext: "png"},
		{Data: base64.StdEncoding.EncodeToString([]byte("two")), Ext: "webp"},
	}, "gen")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(paths) != 2 {
		t.Fatalf("expected two files, got %d", len(paths))
	}
	for i, want := range []string{".png", ".webp"} {
		if !strings.HasSuffix(paths[i], want) {
			t.Errorf("file %d = %q, want a %s suffix", i, paths[i], want)
		}
	}
	// The directories are created on demand.
	if _, err := os.Stat(dir); err != nil {
		t.Errorf("the output directory was not created: %v", err)
	}
}

func TestSaveImagesNormalisesExtensions(t *testing.T) {
	dir := t.TempDir()
	paths, err := saveImages(dir, []imageSource{
		{Data: base64.StdEncoding.EncodeToString([]byte("x")), Ext: "jpeg"},
		{Data: base64.StdEncoding.EncodeToString([]byte("y")), Ext: ""},
	}, "gen")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.HasSuffix(paths[0], ".png") {
		t.Errorf("jpeg should be normalised to png, got %q", paths[0])
	}
	if !strings.HasSuffix(paths[1], ".png") {
		t.Errorf("a missing extension should default to png, got %q", paths[1])
	}
}

func TestSaveImagesRejectsCorruptBase64(t *testing.T) {
	_, err := saveImages(t.TempDir(), []imageSource{{Data: "!!! not base64 !!!"}}, "gen")
	if err == nil {
		t.Error("expected an error for undecodable image data")
	}
}

func TestImageExtFromModel(t *testing.T) {
	cases := map[string]string{
		"dall-e-3":                 "png",
		"dall-e-2":                 "png",
		"flux-schnell":             "webp",
		"stability-ai/sdxl:latest": "webp",
	}
	for model, want := range cases {
		if got := imageExtFromModel(model); got != want {
			t.Errorf("imageExtFromModel(%q) = %q, want %q", model, got, want)
		}
	}
}

func TestSplitSize(t *testing.T) {
	w, h := splitSize("1024x768")
	if w != 1024 || h != 768 {
		t.Errorf("splitSize = %v x %v, want 1024 x 768", w, h)
	}
	if w, h := splitSize("nonsense"); w != nil || h != nil {
		t.Errorf("an unparsable size should yield nils, got %v x %v", w, h)
	}
}

func TestAPIErrorUnpacksProviderMessages(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{
		{"openai", `{"error":{"message":"nope"}}`, "nope"},
		{"flat message", `{"message":"flat"}`, "flat"},
		{"gitlab detail", `{"detail":"bad token"}`, "bad token"},
		{"huggingface", `{"error":"model is loading"}`, "model is loading"},
		{"plain text", `upstream exploded`, "upstream exploded"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := apiError("400 Bad Request", []byte(tc.body))
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %q, want it to contain %q", err, tc.want)
			}
			if !strings.Contains(err.Error(), "400") {
				t.Errorf("error should carry the status: %q", err)
			}
		})
	}
}

func TestEncodeFile(t *testing.T) {
	dir := t.TempDir()

	jpg := filepath.Join(dir, "a.jpg")
	writeFile(t, jpg, "bytes")
	got, err := encodeFile(jpg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.HasPrefix(got, "data:image/jpeg;base64,") {
		t.Errorf("unexpected data url %q", got)
	}

	if _, err := encodeFile(filepath.Join(dir, "missing.png")); err == nil {
		t.Error("expected an error for a missing file")
	}
}

func TestCountRemote(t *testing.T) {
	got := countRemote([]imageSource{
		{URL: "https://a"},
		{Data: "abc"},
		{URL: "https://b"},
	})
	if got != 2 {
		t.Errorf("countRemote = %d, want 2", got)
	}
}
