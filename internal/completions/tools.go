package completions

import (
	"github.com/svpc-ai/svpc/internal/tui/components/dialog"
)

type toolCompletionItem struct {
	name        string
	description string
}

type ToolsContextGroup struct {
	prefix string
	tools  []toolCompletionItem
}

func (cg *ToolsContextGroup) GetId() string {
	return cg.prefix
}

func (cg *ToolsContextGroup) GetEntry() dialog.CompletionItemI {
	return dialog.NewCompletionItem(dialog.CompletionItem{
		Title: "Tools",
		Value: "tools",
	})
}

func (cg *ToolsContextGroup) GetChildEntries(query string) ([]dialog.CompletionItemI, error) {
	items := make([]dialog.CompletionItemI, 0, len(cg.tools))
	for _, tool := range cg.tools {
		if query == "" || fuzzyMatch(query, tool.name) || fuzzyMatch(query, tool.description) {
			item := dialog.NewCompletionItem(dialog.CompletionItem{
				Title: tool.name + " - " + tool.description,
				Value: tool.name,
			})
			items = append(items, item)
		}
	}
	return items, nil
}

// fuzzyMatch does a simple substring match for tool filtering
func fuzzyMatch(query, target string) bool {
	queryLen := len(query)
	if queryLen == 0 {
		return true
	}
	if queryLen > len(target) {
		return false
	}
	for i := 0; i <= len(target)-queryLen; i++ {
		if target[i:i+queryLen] == query {
			return true
		}
	}
	return false
}

func NewToolsContextGroup() dialog.CompletionProvider {
	return &ToolsContextGroup{
		prefix: "tool",
		tools: []toolCompletionItem{
			{"bash", "Run shell commands"},
			{"view", "View file contents"},
			{"edit", "Edit files"},
			{"write", "Write files"},
			{"glob", "Find files by pattern"},
			{"grep", "Search file contents"},
			{"ls", "List directory contents"},
			{"fetch", "Fetch web content"},
			{"patch", "Apply patches"},
			{"open", "Open URLs or local files/applications"},
			{"hosting", "GitHub/GitLab/Bitbucket integration (PRs, issues, etc.)"},
			{"github_workflow", "Create GitHub Actions workflows"},
			{"gh", "Run GitHub CLI commands"},
			{"glab", "Run GitLab CLI commands"},
			{"webhook", "Manage webhooks on hosting platforms"},
			{"image_gen", "Generate images (DALL-E, Stable Diffusion, Midjourney, etc.)"},
			{"image_edit", "Edit images (inpainting, outpainting)"},
			{"image_variation", "Create image variations"},
			{"cloud", "Cloud providers (AWS, GCP, Azure, Cloudflare, Vercel, Netlify, Heroku)"},
			{"docker", "Docker operations (build, run, push, compose)"},
			{"k8s", "Kubernetes operations (apply, logs, scale, helm)"},
			{"cicd", "CI/CD platforms (GitHub Actions, GitLab CI, CircleCI, etc.)"},
			{"build", "Cross-platform build (Android APK/AAB, Windows EXE, Linux AppImage/deb/rpm, macOS DMG/pkg, iOS IPA)"},
			{"sign", "Code signing (Windows, macOS, iOS, Android)"},
			{"notarize", "Apple notarization for macOS/iOS"},
			{"package", "Create distributable packages (APK, EXE, AppImage, DMG, IPA)"},
			{"task", "Launch a sub-agent"},
		},
	}
}
