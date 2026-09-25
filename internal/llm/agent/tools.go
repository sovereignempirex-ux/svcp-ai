package agent

import (
	"context"

	"github.com/svpc-ai/svpc/internal/history"
	"github.com/svpc-ai/svpc/internal/llm/tools"
	"github.com/svpc-ai/svpc/internal/lsp"
	"github.com/svpc-ai/svpc/internal/message"
	"github.com/svpc-ai/svpc/internal/permission"
	"github.com/svpc-ai/svpc/internal/session"
)

func CoderAgentTools(
	permissions permission.Service,
	sessions session.Service,
	messages message.Service,
	history history.Service,
	lspClients map[string]*lsp.Client,
) []tools.BaseTool {
	ctx := context.Background()
	otherTools := GetMcpTools(ctx, permissions)
	if len(lspClients) > 0 {
		otherTools = append(otherTools, tools.NewDiagnosticsTool(lspClients))
	}
	return append(
		[]tools.BaseTool{
			tools.NewBashTool(permissions),
			tools.NewBuildTool(permissions),
			tools.NewCICDTool(permissions),
			tools.NewCloudTool(permissions),
			tools.NewDockerTool(permissions),
			tools.NewEditTool(lspClients, permissions, history),
			tools.NewFetchTool(permissions),
			tools.NewGitHubCLITool(permissions),
			tools.NewGitHubWorkflowTool(permissions),
			tools.NewGitLabCLITool(permissions),
			tools.NewGlobTool(),
			tools.NewGrepTool(),
			tools.NewHostingTool(),
			tools.NewImageEditTool(permissions),
			tools.NewImageGenTool(permissions),
			tools.NewImageVariationTool(permissions),
			tools.NewKubernetesTool(permissions),
			tools.NewLsTool(),
			tools.NewNotarizeTool(permissions),
			tools.NewOpenTool(),
			tools.NewPackageTool(permissions),
			tools.NewSignTool(permissions),
			tools.NewSourcegraphTool(),
			tools.NewViewTool(lspClients),
			tools.NewPatchTool(lspClients, permissions, history),
			tools.NewWebhookTool(permissions),
			tools.NewWriteTool(lspClients, permissions, history),
			NewAgentTool(sessions, messages, lspClients),
		}, otherTools...,
	)
}

func TaskAgentTools(lspClients map[string]*lsp.Client) []tools.BaseTool {
	return []tools.BaseTool{
		tools.NewGlobTool(),
		tools.NewGrepTool(),
		tools.NewLsTool(),
		tools.NewSourcegraphTool(),
		tools.NewViewTool(lspClients),
	}
}
