<div align="center">

<img src="https://github.com/user-attachments/assets/9ae61ef6-70e5-4876-bc45-5bcb4e52c714" width="720" alt="SVPC AI">

# SVPC AI

**A Go-based AI agent that runs directly in your terminal.**  
Inspect files. Edit code. Execute tools. Maintain sessions. Drive it remotely.

[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.24%2B-00ADD8?logo=go)](https://go.dev)
[![Release](https://img.shields.io/github/v/release/sovereignempirex-ux/svcp-ai?label=release)](https://github.com/sovereignempirex-ux/svcp-ai/releases/latest)
[![Early Development](https://img.shields.io/badge/status-early%20development-orange)](https://github.com/sovereignempirex-ux/svcp-ai)

[Installation](#installation) · [Quick Start](#quick-start) · [Configuration](#configuration) · [Releases](https://github.com/sovereignempirex-ux/svcp-ai/releases)

</div>

---

> **⚠️ Early Development**
>
> SVPC AI is under active development and is **not yet production-ready**. APIs, configuration structures, and CLI flags may change between releases without notice. Some features listed in this document are experimental or partially implemented — they are clearly marked. Use with that understanding.

---

## Table of Contents

- [Overview](#overview)
- [Why SVPC AI](#why-svpc-ai)
- [Feature Highlights](#feature-highlights)
- [Capability Matrix](#capability-matrix)
- [Architecture](#architecture)
- [Project Structure](#project-structure)
- [Installation](#installation)
- [Quick Start](#quick-start)
- [Configuration](#configuration)
- [Providers & Models](#providers--models)
- [Agents](#agents)
- [Tools](#tools)
- [Permissions](#permissions)
- [Sessions & Context](#sessions--context)
- [MCP](#mcp-model-context-protocol)
- [LSP](#lsp-language-server-protocol)
- [Remote Mode](#remote-mode)
- [SDKs](#sdks)
- [HTTP API](#http-api)
- [Custom Commands](#custom-commands)
- [Non-interactive Mode](#non-interactive-mode)
- [CLI Reference](#cli-reference)
- [Keyboard Shortcuts](#keyboard-shortcuts)
- [Development](#development)
- [Build & Release](#build--release)
- [Security](#security)
- [Contributing](#contributing)
- [License](#license)

---

## Overview

SVPC AI is a terminal AI agent written in Go. It runs a TUI built on [Bubble Tea](https://github.com/charmbracelet/bubbletea) and connects to multiple LLM providers — OpenAI, Anthropic, Google, Groq, GitHub Copilot, AWS Bedrock, Azure OpenAI, VertexAI, and self-hosted endpoints.

Unlike a simple chat wrapper, SVPC AI exposes a set of developer tools to the model: the agent can search and read files, write and patch code, run shell commands, query LSP diagnostics, fetch URLs, and call MCP servers — all with an explicit permission system that keeps you in control.

Conversation state is stored in SQLite. Sessions persist across restarts. Long conversations are automatically compacted when they approach the model's context limit.

The agent can also run headlessly as an HTTP server, allowing Python, JavaScript, Android, or any HTTP client to drive it remotely.

---

## Why SVPC AI

Most terminal AI tools are thin API wrappers: you type, the model replies, nothing else happens. SVPC AI is structured differently.

```
  Your Terminal
       │
       ▼
  ┌─────────────────────────────┐
  │         SVPC AI TUI         │
  │   (Bubble Tea / Cobra CLI)  │
  └──────────────┬──────────────┘
                 │
                 ▼
  ┌─────────────────────────────┐
  │          Agent Core         │
  │                             │
  │  Context  Sessions  Memory  │
  │  Tools    Policies  Model   │
  └──────┬──────────┬───────────┘
         │          │
    ┌────┘          └────────────────┐
    ▼                                ▼
┌──────────┐   ┌──────────┐   ┌──────────┐
│   LLM    │   │   LSP    │   │   MCP    │
│Providers │   │ Servers  │   │ Servers  │
└──────────┘   └──────────┘   └──────────┘
 OpenAI         gopls          stdio/SSE
 Anthropic       tsserver       external
 Gemini         rust-analyzer   tools
 Groq           …              …
 Bedrock
 Copilot
 Azure / Vertex
 Local
```

The agent receives your prompt, chooses tools, executes them, feeds results back into context, and continues until it has an answer or needs your input. The permission layer intercepts every destructive action before it runs.

---

## Feature Highlights

**🤖 AI & Models**
- Connect to OpenAI, Anthropic, Google Gemini, Groq, GitHub Copilot, AWS Bedrock, Azure OpenAI, VertexAI, and OpenAI-compatible local endpoints
- Switch models mid-session from the model dialog
- Configure separate models per agent role (coder, task, title)
- GitHub Copilot support (experimental — see [Using GitHub Copilot](#using-github-copilot))

**🛠️ Developer Tools**
- Search files with glob and grep
- Read, write, edit, and patch files
- Execute shell commands with configurable shell
- Query LSP diagnostics
- Fetch URLs
- Spawn sub-agents for parallel tasks

**🧠 Context & Sessions**
- Persistent sessions stored in SQLite
- Auto-compact: summarize and continue when the context window approaches its limit
- Switch between sessions with `Ctrl+A`
- Project memory via `SVPC.md`

**🔌 Extensibility**
- MCP: connect external tool servers over stdio or SSE
- Go library (`pkg/svpc`): embed the agent in your own program
- Python SDK: `pip install svpc-ai`
- JavaScript SDK: available as a release tarball
- HTTP API: documented in [`docs/api.md`](docs/api.md)

**🌐 Remote Access**
- Run `svpc --serve` to expose the agent as an HTTP server
- Connect from the Android client, Python, JavaScript, or any HTTP client
- Token-based authentication

**💻 Terminal UX**
- Vim-like keyboard navigation
- External editor support (`Ctrl+E`)
- Named custom commands with argument placeholders
- Non-interactive mode for scripting (`-p`)

---

## Capability Matrix

| Capability | Status | Notes |
|---|---|---|
| Interactive TUI | ✅ | Bubble Tea, keyboard-driven |
| OpenAI provider | ✅ | GPT-4.1, GPT-4o, O-series |
| Anthropic provider | ✅ | Claude 3–4 families |
| Google Gemini provider | ✅ | 2.0 / 2.5 families |
| Groq provider | ✅ | Llama 4, Deepseek, QWEN |
| GitHub Copilot provider | 🧪 | Experimental |
| AWS Bedrock provider | ✅ | Claude 3.7 Sonnet |
| Azure OpenAI provider | ✅ | GPT-4.1 / O-series |
| Google VertexAI provider | ✅ | Gemini 2.5 / 2.5 Flash |
| Self-hosted (OpenAI-compatible) | ✅ | Via `LOCAL_ENDPOINT` |
| File tools (glob/grep/ls/view/write/edit/patch) | ✅ | Core toolset |
| Shell execution (bash) | ✅ | Configurable shell |
| URL fetch | ✅ | |
| LSP diagnostics | ✅ | Exposed to agent via `diagnostics` tool |
| LSP completions / hover / definition | ⚠️ | Protocol supported; not exposed to agent |
| MCP (stdio) | ✅ | |
| MCP (SSE) | ✅ | |
| Sessions & persistence (SQLite) | ✅ | |
| Auto-compact | ✅ | Default enabled |
| Custom commands | ✅ | Named argument placeholders |
| Non-interactive mode | ✅ | `-p` flag |
| Remote HTTP server | ✅ | `--serve` flag |
| Android client | 🧪 | Remote client only, not a local agent |
| Go SDK (`pkg/svpc`) | ✅ | In-process embedding |
| Python SDK | ✅ | `pip install svpc-ai`; HTTP client |
| JavaScript SDK | ⚠️ | Not on npm; install from release tarball |
| Build tools (Android/Windows/macOS/iOS) | 🧪 | Experimental |
| Cloud tools (AWS/GCP/Azure/etc.) | 🧪 | Experimental |
| Image generation tools | 🧪 | Experimental |
| CI/CD tools | 🧪 | Experimental |
| Sourcegraph tool | 🧪 | Experimental |

> **Key:** ✅ Implemented · 🧪 Experimental · ⚠️ Partial · 🚧 Planned

---

## Project Structure

```
svcp-ai/
├── cmd/                    # CLI entry point (Cobra)
├── internal/
│   ├── app/                # Core application services
│   ├── config/             # Configuration loading & validation
│   ├── db/                 # SQLite storage & migrations
│   ├── llm/                # LLM provider adapters & tool definitions
│   ├── lsp/                # Language Server Protocol client
│   ├── logging/            # Logging infrastructure
│   ├── message/            # Message handling & formatting
│   ├── session/            # Session lifecycle management
│   └── tui/                # Terminal UI components & layouts
├── pkg/
│   └── svpc/               # Go library for embedding the agent
├── sdk/
│   ├── python/             # Python HTTP client SDK
│   └── js/                 # JavaScript HTTP client SDK
├── docs/
│   └── api.md              # HTTP API reference
├── install                 # Shell install script
├── go.mod
└── LICENSE
```

---

## Installation

### Linux & macOS

The install script places the binary at `~/.svpc/bin/svpc` and requires no root.

```bash
# Latest release
curl -fsSL https://raw.githubusercontent.com/sovereignempirex-ux/svcp-ai/refs/heads/main/install | bash

# Specific version
curl -fsSL https://raw.githubusercontent.com/sovereignempirex-ux/svcp-ai/refs/heads/main/install | VERSION=1.0.0 bash
```

After installation, add `~/.svpc/bin` to your `PATH` as the script instructs.

<details>
<summary>Manual install from release archive</summary>

```bash
VERSION=1.0.0
ARCH=arm64        # or x86_64
OS=linux          # or mac

curl -fLO "https://github.com/sovereignempirex-ux/svcp-ai/releases/download/v${VERSION}/svpc-${OS}-${ARCH}.tar.gz"
tar -xzf "svpc-${OS}-${ARCH}.tar.gz"
install svpc ~/.svpc/bin/
```

Verify the download against `checksums.txt` in the same release:

```bash
sha256sum -c checksums.txt
```

</details>

### Windows

Download `SVPC.AI.exe` from the [latest release](https://github.com/sovereignempirex-ux/svcp-ai/releases/latest) and run it.

### Using Go

```bash
go install github.com/sovereignempirex-ux/svcp/cmd@latest
```

Requires Go 1.24 or higher.

### Build from Source

```bash
git clone https://github.com/sovereignempirex-ux/svcp-ai.git
cd svcp-ai
go build -o svpc ./cmd
./svpc
```

### Android Client

The Android APK (`svpc-ai.apk`) in each release is a **remote client** — it does not run a model on the device. It connects to an agent running on your machine. See [Remote Mode](#remote-mode) for setup.

---

## Quick Start

```bash
# Launch SVPC AI
svpc
```

On first run:

1. Open the model dialog with `Ctrl+O`
2. Set your API key for the provider you want (see [Environment Variables](#environment-variables))
3. Select a model and press `Enter`
4. Type your first message and press `Ctrl+S` or `Enter` to send
5. When the agent requests permission to run a tool, press `a` to allow or `d` to deny

For a one-shot answer without the TUI:

```bash
export ANTHROPIC_API_KEY=sk-ant-...
svpc -p "What does this repository do?"
```

---

## Configuration

SVPC AI loads configuration from the first file it finds in this order:

1. `./.svpc.json` (project-local)
2. `$XDG_CONFIG_HOME/svpc/.svpc.json`
3. `$HOME/.svpc.json`

### Full Configuration Reference

```json
{
  "data": {
    "directory": ".svpc"
  },
  "providers": {
    "openai": {
      "apiKey": "",
      "disabled": false,
      "baseUrl": "https://api.openai.com/v1"
    },
    "anthropic": {
      "apiKey": "",
      "disabled": false
    },
    "copilot": {
      "disabled": false
    },
    "groq": {
      "apiKey": "",
      "disabled": false
    },
    "openrouter": {
      "apiKey": "",
      "disabled": false
    }
  },
  "agents": {
    "coder": {
      "model": "claude-3.7-sonnet",
      "maxTokens": 5000
    },
    "task": {
      "model": "claude-3.7-sonnet",
      "maxTokens": 5000
    },
    "title": {
      "model": "claude-3.7-sonnet",
      "maxTokens": 80
    }
  },
  "shell": {
    "path": "/bin/bash",
    "args": ["-l"]
  },
  "mcpServers": {
    "my-server": {
      "type": "stdio",
      "command": "path/to/mcp-server",
      "args": [],
      "env": []
    }
  },
  "lsp": {
    "go": {
      "disabled": false,
      "command": "gopls"
    }
  },
  "autoCompact": true,
  "debug": false,
  "debugLSP": false
}
```

### Environment Variables

API keys can be set via environment variable instead of the config file. Environment variables take precedence.

| Variable | Provider |
|---|---|
| `ANTHROPIC_API_KEY` | Anthropic Claude |
| `OPENAI_API_KEY` | OpenAI |
| `GEMINI_API_KEY` | Google Gemini |
| `GITHUB_TOKEN` | GitHub Copilot |
| `GROQ_API_KEY` | Groq |
| `AWS_ACCESS_KEY_ID` | AWS Bedrock |
| `AWS_SECRET_ACCESS_KEY` | AWS Bedrock |
| `AWS_REGION` | AWS Bedrock |
| `AZURE_OPENAI_ENDPOINT` | Azure OpenAI |
| `AZURE_OPENAI_API_KEY` | Azure OpenAI |
| `AZURE_OPENAI_API_VERSION` | Azure OpenAI |
| `VERTEXAI_PROJECT` | Google VertexAI |
| `VERTEXAI_LOCATION` | Google VertexAI |
| `LOCAL_ENDPOINT` | Self-hosted (OpenAI-compatible) |
| `SHELL` | Shell used by the bash tool |

### Shell Configuration

By default the bash tool uses the shell in `$SHELL`, falling back to `/bin/bash`. Override it:

```json
{
  "shell": {
    "path": "/bin/zsh",
    "args": ["-l"]
  }
}
```

### Auto Compact

When `autoCompact` is `true` (the default), SVPC AI monitors token usage and automatically summarizes the conversation at 95% of the model's context window. The summary becomes the start of a new session, so work continues without losing important context.

Disable it if you prefer to manage context manually:

```json
{
  "autoCompact": false
}
```

---

## Providers & Models

### OpenAI

| Model family | Examples |
|---|---|
| GPT-4.1 | `gpt-4.1`, `gpt-4.1-mini`, `gpt-4.1-nano` |
| GPT-4.5 | `gpt-4.5-preview` |
| GPT-4o | `gpt-4o`, `gpt-4o-mini` |
| O1 | `o1`, `o1-pro`, `o1-mini` |
| O3 | `o3`, `o3-mini` |
| O4 | `o4-mini` |

### Anthropic

| Model |
|---|
| `claude-4-sonnet` |
| `claude-4-opus` |
| `claude-3-7-sonnet` |
| `claude-3-5-sonnet` |
| `claude-3-5-haiku` |
| `claude-3-haiku` |
| `claude-3-opus` |

### Google Gemini

| Model |
|---|
| `gemini-2.5`, `gemini-2.5-flash` |
| `gemini-2.0-flash`, `gemini-2.0-flash-lite` |

### GitHub Copilot _(experimental)_

Copilot exposes a mix of OpenAI and Anthropic models including `gpt-4.1`, `claude-3.7-sonnet`, `claude-sonnet-4`, `gemini-2.5-pro`, `o4-mini`, and others. See [Using GitHub Copilot](#using-github-copilot).

### AWS Bedrock

| Model |
|---|
| `claude-3-7-sonnet` (via Bedrock) |

### Groq

| Model |
|---|
| `llama-4-maverick-17b-128e-instruct` |
| `llama-4-scout-17b-16e-instruct` |
| `qwen-qwq-32b` |
| `deepseek-r1-distill-llama-70b` |
| `llama-3.3-70b-versatile` |

### Azure OpenAI

Mirrors the OpenAI model list. Requires `AZURE_OPENAI_ENDPOINT` and `AZURE_OPENAI_API_VERSION`.

### Google VertexAI

| Model |
|---|
| `gemini-2.5`, `gemini-2.5-flash` |

### Self-hosted (OpenAI-compatible)

```bash
export LOCAL_ENDPOINT=http://localhost:1235/v1
```

Reference a local model in agent config using the `local.` prefix:

```json
{
  "agents": {
    "coder": {
      "model": "local.granite-3.3-2b-instruct@q8_0"
    }
  }
}
```

### Using GitHub Copilot

> **Experimental** — token acquisition depends on a third-party tool.

Requirements: Copilot Chat enabled in [GitHub settings](https://github.com/settings/copilot), plus one of:

- VSCode GitHub Copilot Chat extension
- GitHub `gh` CLI
- Neovim `copilot.vim` / `copilot.lua`
- A GitHub token with Copilot permissions

SVPC AI reads the token from:

- `~/.config/github-copilot/hosts.json` or `apps.json`
- `$XDG_CONFIG_HOME/github-copilot/hosts.json` or `apps.json`
- `$GITHUB_TOKEN` environment variable
- `providers.copilot.apiKey` in the config file

---

## Agents

SVPC AI uses three named agent roles internally, each independently configurable:

| Role | Purpose | Config key |
|---|---|---|
| `coder` | Handles the main conversation and tool use | `agents.coder` |
| `task` | Runs sub-tasks and parallel work | `agents.task` |
| `title` | Generates session titles | `agents.title` |

Each role accepts:

```json
{
  "model": "claude-3.7-sonnet",
  "maxTokens": 5000,
  "reasoningEffort": "high"
}
```

A request flows as follows:

1. Your message enters the `coder` agent
2. The agent builds a context from session history, project memory (`SVPC.md`), and any active LSP diagnostics
3. The model responds — either with a final answer, or with one or more tool calls
4. Each tool call goes through the permission system before execution
5. Tool results feed back into context
6. The loop continues until the model produces a final response

---

## Tools

Tools are the actions the agent can take. Every tool call is intercepted by the permission system before execution.

### File & Code

| Tool | Description | Key parameters |
|---|---|---|
| `glob` | Find files by pattern | `pattern` (required), `path` |
| `grep` | Search file contents | `pattern` (required), `path`, `include`, `literal_text` |
| `ls` | List directory contents | `path`, `ignore` |
| `view` | Read file contents | `file_path` (required), `offset`, `limit` |
| `write` | Write a file | `file_path` (required), `content` (required) |
| `edit` | Edit a file in place | file path and edit parameters |
| `patch` | Apply a unified diff | `file_path` (required), `diff` (required) |

### Shell & Network

| Tool | Description | Key parameters |
|---|---|---|
| `bash` | Execute a shell command | `command` (required), `timeout` |
| `fetch` | Fetch a URL | `url` (required), `format` (required), `timeout` |

### Intelligence

| Tool | Description | Key parameters |
|---|---|---|
| `diagnostics` | Get LSP diagnostics for a file | `file_path` |
| `agent` | Spawn a sub-agent for a parallel task | `prompt` (required) |

### Experimental Tools

The following tools are defined but are **experimental** — their stability and completeness should not be assumed:

| Category | Tool | Notes |
|---|---|---|
| Build | `build`, `sign`, `notarize`, `package` | Cross-platform build targets |
| Cloud | `cloud`, `docker`, `k8s`, `cicd` | Cloud/container/CI operations |
| Image | `image_gen`, `image_edit`, `image_variation` | Requires provider API key |
| Hosting | `hosting`, `github_workflow`, `gh`, `glab`, `webhook` | Git platform integrations |
| Search | `sourcegraph` | Public code search |

---

## Permissions

Before the agent executes any tool, it presents a permission prompt. This applies to every tool — shell commands, file writes, network requests, and MCP calls.

### Options

| Choice | Keyboard | Meaning |
|---|---|---|
| Allow | `a` | Allow this specific invocation |
| Allow for session | `A` | Allow this tool for the rest of the session without further prompts |
| Deny | `d` | Deny this invocation; the agent receives an error |

### Non-interactive mode

When running with `-p`, all permissions are **auto-approved**. Do not run untrusted prompts in non-interactive mode against a sensitive codebase.

### MCP tools

MCP tools follow the same permission model as built-in tools. A tool discovered from an MCP server requires approval before its first use.

---

## Sessions & Context

Sessions are the primary unit of conversation state. Each session stores:

- Full message history
- Tool call history
- File change tracking
- Auto-generated title

### Managing sessions

| Action | How |
|---|---|
| New session | `Ctrl+N` in the TUI |
| Switch session | `Ctrl+A` → select from list → `Enter` |
| Session title | Auto-generated by the `title` agent after the first exchange |

Sessions persist in `.svpc/` (or the configured `data.directory`) using SQLite.

### Auto Compact

When `autoCompact: true`, the agent monitors token usage. At 95% of the model's context limit, it:

1. Asks the model to summarize the conversation
2. Creates a new session containing the summary
3. Continues from the new session transparently

The original session is preserved. You can switch back to it at any time.

### Project Memory

Create an `SVPC.md` file in your project root. SVPC AI reads it at the start of each session and uses it as persistent project context — architecture notes, conventions, important file paths, and anything else you want the agent to always know.

Generate or update it with the built-in **Initialize Project** command (`Ctrl+K` → `Initialize Project`).

---

## MCP (Model Context Protocol)

MCP lets you connect external tool servers to SVPC AI. The agent discovers tools from connected servers and treats them identically to built-in tools, including the same permission prompts.

### Connection types

| Type | Use case |
|---|---|
| `stdio` | Local process communicating over stdin/stdout |
| `sse` | Remote server communicating over Server-Sent Events |

### Configuration

```json
{
  "mcpServers": {
    "my-local-tool": {
      "type": "stdio",
      "command": "/usr/local/bin/my-mcp-server",
      "args": ["--flag"],
      "env": ["MY_VAR=value"]
    },
    "my-remote-tool": {
      "type": "sse",
      "url": "https://mcp.example.com/sse",
      "headers": {
        "Authorization": "Bearer your-token"
      }
    }
  }
}
```

SVPC AI starts each stdio server as a subprocess and tears it down when the session ends.

---

## LSP (Language Server Protocol)

SVPC AI maintains connections to language servers and exposes their diagnostics to the agent via the `diagnostics` tool.

### What is exposed to the agent

| LSP capability | Exposed to agent |
|---|---|
| Diagnostics (errors, warnings) | ✅ via `diagnostics` tool |
| Completions | ❌ |
| Hover | ❌ |
| Go-to-definition | ❌ |

The LSP client implementation supports the full protocol, but only diagnostics are currently wired into the agent's tool system.

### Configuration

```json
{
  "lsp": {
    "go": {
      "disabled": false,
      "command": "gopls"
    },
    "typescript": {
      "disabled": false,
      "command": "typescript-language-server",
      "args": ["--stdio"]
    }
  }
}
```

The key (`"go"`, `"typescript"`) is a label; the `command` is the executable the server runs. The language server must be installed separately.

Enable LSP debug logging:

```json
{
  "debugLSP": true
}
```

---

## Remote Mode

```bash
svpc --serve 0.0.0.0
```

This starts the agent as an HTTP server. On startup it prints the address and an authentication token:

```
Listening on 0.0.0.0:PORT
Token: <token>
```

Remote clients connect using that address and token. The agent process handles all tool execution — the client sends prompts and handles permission prompts from its own code.

```
Your Machine (agent)
       │
  HTTP API
       │
  ┌────┴──────────────┐
  │  Python client    │
  │  JS client        │
  │  Android client   │
  │  Any HTTP client  │
  └───────────────────┘
```

### Security

- Only expose the agent on a trusted network. `0.0.0.0` binds to all interfaces.
- The token is required for every request — do not share it.
- The agent can execute shell commands with the permissions of the user it runs as.
- Consider running behind a reverse proxy with TLS for any non-local use.

---

## SDKs

### Go (`pkg/svpc`)

The Go package embeds the agent directly in your process. It does not require a running server.

```bash
go get github.com/sovereignempirex-ux/svcp/pkg/svpc
```

```go
import "github.com/sovereignempirex-ux/svcp/pkg/svpc"

agent, err := svpc.New(ctx, svpc.Options{WorkingDir: "."})
if err != nil {
    log.Fatal(err)
}
defer agent.Close()

answer, session, err := agent.Answer(ctx, "", "what does this repository do?")
if err != nil {
    log.Fatal(err)
}
fmt.Println(answer)
```

### Python (`sdk/python`)

An HTTP client. The agent must be running with `--serve`.

```bash
pip install svpc-ai
```

```python
from svpc import Client

with Client("192.168.1.20", 8080, token) as agent:
    agent.connect()
    result = agent.ask_once("summarize the recent changes")
    print(result["text"])
```

### JavaScript (`sdk/js`)

An HTTP client. Not yet on npm — install from the release tarball.

```bash
VERSION=1.0.0
curl -fLO "https://github.com/sovereignempirex-ux/svcp-ai/releases/download/v${VERSION}/svpc-ai-sdk-${VERSION}.tgz"
npm install "./svpc-ai-sdk-${VERSION}.tgz"
```

```js
import { Client } from 'svpc-ai-sdk';

const agent = new Client({ host: '192.168.1.20', port: 8080, token });
await agent.version();
const { text } = await agent.askOnce('what changed here?');
console.log(text);
```

Works in Node.js, Deno, Bun, and the browser. No external dependencies.

---

## HTTP API

The HTTP contract is documented in [`docs/api.md`](docs/api.md).

A client authenticates with the token printed at server startup. The API covers:

- Starting and resuming sessions
- Sending messages and receiving responses
- Responding to permission prompts
- Querying version information

The Go library, Python SDK, and JavaScript SDK are all clients over this contract.

---

## Custom Commands

Custom commands are Markdown files that define reusable prompts with optional named placeholders.

### Locations

| Scope | Path | Prefix |
|---|---|---|
| User | `$XDG_CONFIG_HOME/svpc/commands/` or `~/.svpc/commands/` | `user:` |
| Project | `<project>/.svpc/commands/` | `project:` |

### Creating a command

Create `~/.config/svpc/commands/prime-context.md`:

```markdown
RUN git ls-files
READ README.md
```

This registers as `user:prime-context`.

### Named arguments

Use `$NAME` placeholders (uppercase letters, numbers, underscores; must start with a letter):

```markdown
# Review Issue $ISSUE_NUMBER

RUN gh issue view $ISSUE_NUMBER --json title,body,comments
RUN git log --oneline --author="$AUTHOR" -20
```

When you run this command, SVPC AI prompts for `ISSUE_NUMBER` and `AUTHOR` before sending.

### Subdirectories

```
~/.config/svpc/commands/git/review.md  →  user:git:review
```

### Using commands

1. Press `Ctrl+K` to open the command dialog
2. Select your command
3. Fill in any argument prompts
4. Press `Enter`

### Built-in commands

| Command | Description |
|---|---|
| `Initialize Project` | Creates or updates `SVPC.md` with project context |
| `Compact Session` | Manually triggers session summarization |

---

## Non-interactive Mode

Run a single prompt without the TUI:

```bash
# Plain text output
svpc -p "Explain the use of context in Go"

# JSON output
svpc -p "Explain the use of context in Go" -f json

# No spinner (good for scripts)
svpc -p "Explain the use of context in Go" -q
```

SVPC AI processes the prompt, prints the result to stdout, and exits. All tool permissions are **auto-approved** in this mode.

### Output formats

| Format | Description |
|---|---|
| `text` | Plain text (default) |
| `json` | Result wrapped in a JSON object |

---

## CLI Reference

```bash
svpc [flags]
```

| Flag | Short | Description |
|---|---|---|
| `--help` | `-h` | Print help |
| `--debug` | `-d` | Enable debug logging |
| `--cwd` | `-c` | Set the working directory |
| `--prompt` | `-p` | Run a prompt in non-interactive mode |
| `--output-format` | `-f` | Output format: `text` (default) or `json` |
| `--quiet` | `-q` | Suppress the spinner in non-interactive mode |
| `--serve` | | Start the HTTP server (e.g. `--serve 0.0.0.0`) |

---

## Keyboard Shortcuts

### Global

| Shortcut | Action |
|---|---|
| `Ctrl+C` | Quit |
| `Ctrl+?` or `?` | Toggle help |
| `Ctrl+L` | View logs |
| `Ctrl+A` | Switch session |
| `Ctrl+K` | Command dialog |
| `Ctrl+O` | Model selection dialog |
| `Esc` | Close overlay / return to previous mode |

### Chat

| Shortcut | Action |
|---|---|
| `Ctrl+N` | New session |
| `Ctrl+X` | Cancel current operation |
| `i` | Focus editor (when not already editing) |
| `Esc` | Exit editing, focus message list |

### Editor

| Shortcut | Action |
|---|---|
| `Ctrl+S` or `Enter` | Send message |
| `Ctrl+E` | Open external editor |
| `Esc` | Blur editor, focus messages |

### Session dialog

| Shortcut | Action |
|---|---|
| `↑` / `k` | Previous session |
| `↓` / `j` | Next session |
| `Enter` | Select session |
| `Esc` | Close |

### Model dialog

| Shortcut | Action |
|---|---|
| `↑` / `k` | Move up |
| `↓` / `j` | Move down |
| `←` / `h` | Previous provider |
| `→` / `l` | Next provider |
| `Enter` | Select model |
| `Esc` | Close |

### Permission dialog

| Shortcut | Action |
|---|---|
| `a` | Allow |
| `A` | Allow for session |
| `d` | Deny |
| `←` / `→` / `Tab` | Move between options |
| `Enter` / `Space` | Confirm selection |

### Logs page

| Shortcut | Action |
|---|---|
| `Backspace` / `q` | Return to chat |

---

## Development

### Prerequisites

- Go 1.24.0 or higher

```bash
go version  # must be 1.24+
```

### Build

```bash
git clone https://github.com/sovereignempirex-ux/svcp-ai.git
cd svcp-ai
go build -o svpc ./cmd
./svpc
```

### Run tests

```bash
go test ./...
```

### Debug mode

```bash
./svpc -d
```

Logs appear in the Logs page (`Ctrl+L`) and on stderr.

---

## Build & Release

Releases are published to [GitHub Releases](https://github.com/sovereignempirex-ux/svcp-ai/releases).

### Release assets per version

| Asset | Target |
|---|---|
| `svpc-linux-x86_64.tar.gz` | Linux (amd64) |
| `svpc-linux-arm64.tar.gz` | Linux (arm64) |
| `svpc-mac-x86_64.tar.gz` | macOS (Intel) |
| `svpc-mac-arm64.tar.gz` | macOS (Apple Silicon) |
| `SVPC.AI.exe` | Windows |
| `svpc-ai.apk` | Android remote client |
| `svpc-ai-sdk-<version>.tgz` | JavaScript SDK |
| `checksums.txt` | SHA-256 checksums |

Verify any downloaded asset:

```bash
sha256sum -c checksums.txt
```

Cross-platform build targets (Windows MSI, macOS DMG, Linux AppImage/deb/rpm, iOS IPA) are listed as experimental in the current codebase.

---

## Security

**API keys**
Store API keys in environment variables, not in committed config files. If you use `~/.svpc.json`, ensure its permissions are `600`.

**Shell execution**
The `bash` tool executes commands as the user running SVPC AI. Review every tool call before approving it. In untrusted repositories, be especially cautious about what the agent is asked to run.

**File modification**
The `write`, `edit`, and `patch` tools modify files on disk. These changes are not automatically reversible. Use version control.

**Non-interactive mode**
`-p` auto-approves all permissions. Do not pipe untrusted input into SVPC AI in `-p` mode.

**Remote mode**
The `--serve` flag opens an HTTP port. Protect it:
- Use a firewall to restrict which hosts can connect
- Never expose it to the public internet without a reverse proxy and TLS
- The printed token is the only authentication mechanism — treat it like a password

**MCP servers**
MCP servers run as subprocesses with your user's permissions. Only connect to MCP servers you trust.

**Project memory (`SVPC.md`)**
SVPC AI reads `SVPC.md` automatically. Do not check in SVPC.md files from repositories you do not control.

---

## Contributing

1. Fork the repository
2. Create a feature branch: `git checkout -b feature/your-feature`
3. Make your changes
4. Run tests: `go test ./...`
5. Commit with a clear message: `git commit -m 'feat: describe your change'`
6. Push: `git push origin feature/your-feature`
7. Open a Pull Request against `main`

Please:
- Keep PRs focused — one concern per PR
- Update or add tests where applicable
- Follow the existing code style (`gofmt`, standard Go conventions)
- Describe what the PR does and why in the PR description

For significant changes, open an issue first to discuss the approach.

---

## License

SVPC AI is licensed under the **MIT License**.  
See [LICENSE](LICENSE) for the full text.

---

<div align="center">

Built by [sovereignempirex-ux](https://github.com/sovereignempirex-ux)

</div>
