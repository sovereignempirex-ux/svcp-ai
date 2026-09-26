# SVPC AI

<p align="center"><img src="https://github.com/user-attachments/assets/9ae61ef6-70e5-4876-bc45-5bcb4e52c714" width="800"></p>

> **⚠️ Early Development Notice:** This project is in early development and is not yet ready for production use. Features may change, break, or be incomplete. Use at your own risk.

A powerful terminal-based AI assistant for developers, providing intelligent coding assistance directly in your terminal.

## Overview

SVPC AI is a Go-based CLI application that brings AI assistance to your terminal. It provides a TUI (Terminal User Interface) for interacting with various AI models to help with coding tasks, debugging, and more.

## Features

- **Interactive TUI**: Built with [Bubble Tea](https://github.com/charmbracelet/bubbletea) for a smooth terminal experience
- **Multiple AI Providers**: Support for OpenAI, Anthropic Claude, Google Gemini, AWS Bedrock, Groq, Azure OpenAI, and OpenRouter
- **Session Management**: Save and manage multiple conversation sessions
- **Tool Integration**: AI can execute commands, search files, and modify code
- **Vim-like Editor**: Integrated editor with text input capabilities
- **Persistent Storage**: SQLite database for storing conversations and sessions
- **LSP Integration**: Language Server Protocol support for code intelligence
- **File Change Tracking**: Track and visualize file changes during sessions
- **External Editor Support**: Open your preferred editor for composing messages
- **Named Arguments for Custom Commands**: Create powerful custom commands with multiple named placeholders
- **Cross-platform Build**: Build for Android (APK/AAB), Windows (EXE/MSI), Linux (AppImage/deb/rpm), macOS (DMG/pkg), iOS (IPA)
- **Cloud Integration**: AWS, GCP, Azure, Cloudflare, Vercel, Netlify, Heroku
- **Container & Orchestration**: Docker, Kubernetes, Helm
- **CI/CD Integration**: GitHub Actions, GitLab CI, CircleCI, Buildkite, Jenkins
- **Image Generation**: DALL-E, Stable Diffusion, Midjourney, Flux

## Installation

### Windows

Download `SVPC.AI.exe` from the [latest release](https://github.com/sovereignempirex-ux/svcp-ai/releases/latest) and run it.

### Linux and macOS

```bash
# Install the latest release
curl -fsSL https://raw.githubusercontent.com/sovereignempirex-ux/svcp-ai/refs/heads/main/install | bash

# Install a specific version
curl -fsSL https://raw.githubusercontent.com/sovereignempirex-ux/svcp-ai/refs/heads/main/install | VERSION=1.0.0 bash
```

The script installs to `~/.svpc/bin` and needs no root. It prints the one line to add
to your shell, or adds it for you.

<details>
<summary>Building from the release archives by hand</summary>

```bash
VERSION=1.0.0
ARCH=arm64        # or x86_64
OS=linux          # or mac

curl -fLO "https://github.com/sovereignempirex-ux/svcp-ai/releases/download/v${VERSION}/svpc-${OS}-${ARCH}.tar.gz"
tar -xzf "svpc-${OS}-${ARCH}.tar.gz"
install svpc ~/.svpc/bin/
```

Each archive holds one binary, `LICENSE` and `README.md`, and the archive names are
the ones the script asks for. `checksums.txt` in the same release lists a SHA-256 for
every asset; `sha256sum -c checksums.txt` verifies them.

</details>

<details>
<summary>Package managers</summary>

There is no Homebrew tap or AUR package for this project. The release archives and
the install script above are the supported routes; if you maintain a tap and would
like it listed here, open an issue.

</details>

### Using Go

```bash
go install github.com/sovereignempirex-ux/svcp/cmd@latest
```

### Android

`svpc-ai.apk` from the same release is a client: it drives the agent running on your
own machine over the network, rather than running a model on the phone. Start the
agent with `svpc --serve 0.0.0.0` and enter the address and token it prints.

### Using SVPC AI as a library

The agent is a Go package, and it is a client over an HTTP contract. Whichever you
want, the working directory, the provider and the sessions are the same files the
command line tool uses.

| | |
|---|---|
| [`pkg/svpc`](pkg/svpc) | Go. Embed the agent directly. `go get github.com/sovereignempirex-ux/svcp/pkg/svpc` |
| [`sdk/python`](sdk/python) | Python, standard library only. `pip install svpc-ai` |
| [`sdk/js`](sdk/js) | JavaScript, no dependencies. Works in a browser, Deno, Bun and Node |
| [`docs/api.md`](docs/api.md) | The HTTP contract, for a language nobody has written a client for yet |

```go
agent, _ := svpc.New(ctx, svpc.Options{WorkingDir: "."})
defer agent.Close()

answer, session, _ := agent.Answer(ctx, "", "what does this repository do?")
fmt.Println(answer)
```

```python
from svpc import Client

with Client("192.168.1.20", 8080, token) as agent:
    agent.connect()                                   # check the API version
    print(agent.ask_once("what changed here?")["text"])
```

```js
import { Client } from 'svpc-ai-sdk';

const agent = new Client({ host: '192.168.1.20', port: 8080, token });
await agent.version();
const { text } = await agent.askOnce('what changed here?');
```

The Go library runs the agent in your process. The two SDKs run it somewhere else
and speak to it, which is what makes a phone, a browser or a service possible: start
the agent with `svpc --serve 0.0.0.0` and give the client the address and token it
prints. A turn asks before it runs a command, and the client answers that from its
own code — a browser can put up a dialog, and a program that nobody is watching
denies by default.


SVPC AI looks for configuration in the following locations:

- `$HOME/.svpc.json`
- `$XDG_CONFIG_HOME/svpc/.svpc.json`
- `./.svpc.json` (local directory)

### Auto Compact Feature

SVPC AI includes an auto compact feature that automatically summarizes your conversation when it approaches the model's context window limit. When enabled (default setting), this feature:

- Monitors token usage during your conversation
- Automatically triggers summarization when usage reaches 95% of the model's context window
- Creates a new session with the summary, allowing you to continue your work without losing context
- Helps prevent "out of context" errors that can occur with long conversations

You can enable or disable this feature in your configuration file:

```json
{
  "autoCompact": true // default is true
}
```

### Environment Variables

You can configure SVPC AI using environment variables:

| Environment Variable       | Purpose                                                                          |
| -------------------------- | -------------------------------------------------------------------------------- |
| `ANTHROPIC_API_KEY`        | For Claude models                                                                |
| `OPENAI_API_KEY`           | For OpenAI models                                                                |
| `GEMINI_API_KEY`           | For Google Gemini models                                                         |
| `GITHUB_TOKEN`             | For Github Copilot models                                                        |
| `VERTEXAI_PROJECT`         | For Google Cloud VertexAI (Gemini)                                               |
| `VERTEXAI_LOCATION`        | For Google Cloud VertexAI (Gemini)                                               |
| `GROQ_API_KEY`             | For Groq models                                                                  |
| `AWS_ACCESS_KEY_ID`        | For AWS Bedrock (Claude)                                                         |
| `AWS_SECRET_ACCESS_KEY`    | For AWS Bedrock (Claude)                                                         |
| `AWS_REGION`               | For AWS Bedrock (Claude)                                                         |
| `AZURE_OPENAI_ENDPOINT`    | For Azure OpenAI models                                                          |
| `AZURE_OPENAI_API_KEY`     | For Azure OpenAI models (optional when using Entra ID)                           |
| `AZURE_OPENAI_API_VERSION` | For Azure OpenAI models                                                          |
| `LOCAL_ENDPOINT`           | For self-hosted models                                                           |
| `SHELL`                    | Default shell to use (if not specified in config)                                |

### Shell Configuration

SVPC AI allows you to configure the shell used by the bash tool. By default, it uses the shell specified in the `SHELL` environment variable, or falls back to `/bin/bash` if not set.

You can override this in your configuration file:

```json
{
  "shell": {
    "path": "/bin/zsh",
    "args": ["-l"]
  }
}
```

This is useful if you want to use a different shell than your default system shell, or if you need to pass specific arguments to the shell.

### Configuration File Structure

```json
{
  "data": {
    "directory": ".svpc"
  },
  "providers": {
    "openai": {
      "apiKey": "your-api-key",
      "disabled": false,
      "baseUrl": "https://your-gateway.example/v1"
    },
    "anthropic": {
      "apiKey": "your-api-key",
      "disabled": false
    },
    "copilot": {
      "disabled": false
    },
    "groq": {
      "apiKey": "your-api-key",
      "disabled": false
    },
    "openrouter": {
      "apiKey": "your-api-key",
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
    "example": {
      "type": "stdio",
      "command": "path/to/mcp-server",
      "env": [],
      "args": []
    }
  },
  "lsp": {
    "go": {
      "disabled": false,
      "command": "gopls"
    }
  },
  "debug": false,
  "debugLSP": false,
  "autoCompact": true
}
```

## Supported AI Models

SVPC AI supports a variety of AI models from different providers:

### OpenAI
- GPT-4.1 family (gpt-4.1, gpt-4.1-mini, gpt-4.1-nano)
- GPT-4.5 Preview
- GPT-4o family (gpt-4o, gpt-4o-mini)
- O1 family (o1, o1-pro, o1-mini)
- O3 family (o3, o3-mini)
- O4 Mini

### Anthropic
- Claude 4 Sonnet
- Claude 4 Opus
- Claude 3.5 Sonnet
- Claude 3.5 Haiku
- Claude 3.7 Sonnet
- Claude 3 Haiku
- Claude 3 Opus

### GitHub Copilot
- GPT-3.5 Turbo
- GPT-4
- GPT-4o
- GPT-4o Mini
- GPT-4.1
- Claude 3.5 Sonnet
- Claude 3.7 Sonnet
- Claude 3.7 Sonnet Thinking
- Claude Sonnet 4
- O1
- O3 Mini
- O4 Mini
- Gemini 2.0 Flash
- Gemini 2.5 Pro

### Google
- Gemini 2.5
- Gemini 2.5 Flash
- Gemini 2.0 Flash
- Gemini 2.0 Flash Lite

### AWS Bedrock
- Claude 3.7 Sonnet

### Groq
- Llama 4 Maverick (17b-128e-instruct)
- Llama 4 Scout (17b-16e-instruct)
- QWEN QWQ-32b
- Deepseek R1 distill Llama 70b
- Llama 3.3 70b Versatile

### Azure OpenAI
- GPT-4.1 family (gpt-4.1, gpt-4.1-mini, gpt-4.1-nano)
- GPT-4.5 Preview
- GPT-4o family (gpt-4o, gpt-4o-mini)
- O1 family (o1, o1-mini)
- O3 family (o3, o3-mini)
- O4 Mini

### Google Cloud VertexAI
- Gemini 2.5
- Gemini 2.5 Flash

## Usage

```bash
# Start SVPC AI
svpc

# Start with debug logging
svpc -d

# Start with a specific working directory
svpc -c /path/to/project
```

## Non-interactive Prompt Mode

You can run SVPC AI in non-interactive mode by passing a prompt directly as a command-line argument. This is useful for scripting, automation, or when you want a quick answer without launching the full TUI.

```bash
# Run a single prompt and print the AI's response to the terminal
svpc -p "Explain the use of context in Go"

# Get response in JSON format
svpc -p "Explain the use of context in Go" -f json

# Run without showing the spinner (useful for scripts)
svpc -p "Explain the use of context in Go" -q
```

In this mode, SVPC AI will process your prompt, print the result to standard output, and then exit. All permissions are auto-approved for the session.

By default, a spinner animation is displayed while the model is processing your query. You can disable this spinner with the `-q` or `--quiet` flag, which is particularly useful when running SVPC AI from scripts or automated workflows.

### Output Formats

SVPC AI supports the following output formats in non-interactive mode:

| Format | Description                     |
| ------ | ------------------------------- |
| `text` | Plain text output (default)     |
| `json` | Output wrapped in a JSON object |

The output format is implemented as a strongly-typed `OutputFormat` in the codebase, ensuring type safety and validation when processing outputs.

## Command-line Flags

| Flag              | Short | Description                                         |
| ----------------- | ----- | --------------------------------------------------- |
| `--help`          | `-h`  | Display help information                            |
| `--debug`         | `-d`  | Enable debug mode                                   |
| `--cwd`           | `-c`  | Set current working directory                       |
| `--prompt`        | `-p`  | Run a single prompt in non-interactive mode         |
| `--output-format` | `-f`  | Output format for non-interactive mode (text, json) |
| `--quiet`         | `-q`  | Hide spinner in non-interactive mode                |

## Keyboard Shortcuts

### Global Shortcuts

| Shortcut | Action                                                  |
| -------- | ------------------------------------------------------- |
| `Ctrl+C` | Quit application                                        |
| `Ctrl+?` | Toggle help dialog                                      |
| `?`      | Toggle help dialog (when not in editing mode)           |
| `Ctrl+L` | View logs                                               |
| `Ctrl+A` | Switch session                                          |
| `Ctrl+K` | Command dialog                                          |
| `Ctrl+O` | Toggle model selection dialog                           |
| `Esc`    | Close current overlay/dialog or return to previous mode |

### Chat Page Shortcuts

| Shortcut | Action                                  |
| -------- | --------------------------------------- |
| `Ctrl+N` | Create new session                      |
| `Ctrl+X` | Cancel current operation/generation     |
| `i`      | Focus editor (when not in writing mode) |
| `Esc`    | Exit writing mode and focus messages    |

### Editor Shortcuts

| Shortcut            | Action                                    |
| ------------------- | ----------------------------------------- |
| `Ctrl+S`            | Send message (when editor is focused)     |
| `Enter` or `Ctrl+S` | Send message (when editor is not focused) |
| `Ctrl+E`            | Open external editor                      |
| `Esc`               | Blur editor and focus messages            |

### Session Dialog Shortcuts

| Shortcut   | Action           |
| ---------- | ---------------- |
| `↑` or `k` | Previous session |
| `↓` or `j` | Next session     |
| `Enter`    | Select session   |
| `Esc`      | Close dialog     |

### Model Dialog Shortcuts

| Shortcut   | Action            |
| ---------- | ----------------- |
| `↑` or `k` | Move up           |
| `↓` or `j` | Move down         |
| `←` or `h` | Previous provider |
| `→` or `l` | Next provider     |
| `Esc`      | Close dialog      |

### Permission Dialog Shortcuts

| Shortcut                | Action                       |
| ----------------------- | ---------------------------- |
| `←` or `left`           | Switch options left          |
| `→` or `right` or `tab` | Switch options right         |
| `Enter` or `space`      | Confirm selection            |
| `a`                     | Allow permission             |
| `A`                     | Allow permission for session |
| `d`                     | Deny permission              |

### Logs Page Shortcuts

| Shortcut           | Action              |
| ------------------ | ------------------- |
| `Backspace` or `q` | Return to chat page |

## AI Assistant Tools

SVPC AI's AI assistant has access to various tools to help with coding tasks:

### File and Code Tools

| Tool          | Description                 | Parameters                                                                               |
| ------------- | --------------------------- | ---------------------------------------------------------------------------------------- |
| `glob`        | Find files by pattern       | `pattern` (required), `path` (optional)                                                  |
| `grep`        | Search file contents        | `pattern` (required), `path` (optional), `include` (optional), `literal_text` (optional) |
| `ls`          | List directory contents     | `path` (optional), `ignore` (optional array of patterns)                                 |
| `view`        | View file contents          | `file_path` (required), `offset` (optional), `limit` (optional)                          |
| `write`       | Write to files              | `file_path` (required), `content` (required)                                             |
| `edit`        | Edit files                  | Various parameters for file editing                                                      |
| `patch`       | Apply patches to files      | `file_path` (required), `diff` (required)                                                |
| `diagnostics` | Get diagnostics information | `file_path` (optional)                                                                   |

### Build & Deploy Tools

| Tool          | Description                            | Parameters                                                                                |
| ------------- | -------------------------------------- | ----------------------------------------------------------------------------------------- |
| `build`       | Cross-platform build (Android, Windows, Linux, macOS, iOS) | `platform`, `action`, `config`, `version`, `arch`, `target` |
| `sign`        | Code signing (Windows, macOS, iOS, Android) | `platform`, `file`, `certificate`, `password`, `provisioning_profile`, `keystore` |
| `notarize`    | Apple notarization for macOS/iOS       | `file`, `apple_id`, `password`, `team_id`, `bundle_id` |
| `package`     | Create distributable packages          | `platform`, `format`, `input`, `output`, `config` |

### Cloud & Infrastructure Tools

| Tool          | Description                            | Parameters                                                                                |
| ------------- | -------------------------------------- | ----------------------------------------------------------------------------------------- |
| `cloud`       | Cloud providers (AWS, GCP, Azure, Cloudflare, Vercel, Netlify, Heroku) | `provider`, `action`, `region`, `params` |
| `docker`      | Docker operations (build, run, push, compose) | `action`, `image`, `dockerfile`, `context`, `tag`, `ports`, `env`, `volumes` |
| `k8s`         | Kubernetes operations (apply, logs, scale, helm) | `action`, `resource`, `name`, `namespace`, `file`, `replicas`, `chart`, `release` |
| `cicd`        | CI/CD platforms (GitHub Actions, GitLab CI, CircleCI, etc.) | `provider`, `action`, `project`, `params` |

### Image Generation Tools

| Tool                | Description                                    | Parameters                                         |
| ------------------- | ---------------------------------------------- | -------------------------------------------------- |
| `image_gen`         | Generate images (DALL-E, Stable Diffusion, Midjourney, Flux) | `provider`, `prompt`, `model`, `size`, `quality`, `style`, `n`, `seed`, `negative_prompt`, `aspect_ratio` |
| `image_edit`        | Edit images (inpainting, outpainting)          | `provider`, `prompt`, `image_path`, `mask_path`, `model`, `size`, `n` |
| `image_variation`   | Create image variations                        | `provider`, `image_path`, `model`, `size`, `n` |

### Hosting & Repository Tools

| Tool          | Description                            | Parameters                                                                                |
| ------------- | -------------------------------------- | ----------------------------------------------------------------------------------------- |
| `hosting`     | GitHub/GitLab/Bitbucket integration (PRs, issues, etc.) | `provider`, `action`, `owner`, `repo`, `title`, `body`, `head`, `base`, `number`, `state`, `labels`, `assignees` |
| `github_workflow` | Create GitHub Actions workflows       | `owner`, `repo`, `name`, `on`, `jobs` |
| `gh`          | Run GitHub CLI commands                | `command`, `args`, `workdir` |
| `glab`        | Run GitLab CLI commands                | `command`, `args`, `workdir` |
| `webhook`     | Manage webhooks on hosting platforms   | `provider`, `action`, `owner`, `repo`, `url`, `events`, `secret`, `id` |

### Other Tools

| Tool          | Description                            | Parameters                                                                                |
| ------------- | -------------------------------------- | ----------------------------------------------------------------------------------------- |
| `bash`        | Execute shell commands                 | `command` (required), `timeout` (optional)                                                |
| `fetch`       | Fetch data from URLs                   | `url` (required), `format` (required), `timeout` (optional)                               |
| `sourcegraph` | Search code across public repositories | `query` (required), `count` (optional), `context_window` (optional), `timeout` (optional) |
| `agent`       | Run sub-tasks with the AI agent        | `prompt` (required)                                                                       |

## Architecture

SVPC AI is built with a modular architecture:

- **cmd**: Command-line interface using Cobra
- **internal/app**: Core application services
- **internal/config**: Configuration management
- **internal/db**: Database operations and migrations
- **internal/llm**: LLM providers and tools integration
- **internal/tui**: Terminal UI components and layouts
- **internal/logging**: Logging infrastructure
- **internal/message**: Message handling
- **internal/session**: Session management
- **internal/lsp**: Language Server Protocol integration

## Custom Commands

SVPC AI supports custom commands that can be created by users to quickly send predefined prompts to the AI assistant.

### Creating Custom Commands

Custom commands are predefined prompts stored as Markdown files in one of three locations:

1. **User Commands** (prefixed with `user:`):

    ```
    $XDG_CONFIG_HOME/svpc/commands/
    ```

    (typically `~/.config/svpc/commands/` on Linux/macOS)

    or

    ```
    $HOME/.svpc/commands/
    ```

2. **Project Commands** (prefixed with `project:`):

    ```
    <PROJECT DIR>/.svpc/commands/
    ```

Each `.md` file in these directories becomes a custom command. The file name (without extension) becomes the command ID.

For example, creating a file at `~/.config/svpc/commands/prime-context.md` with content:

```markdown
RUN git ls-files
READ README.md
```

This creates a command called `user:prime-context`.

### Command Arguments

SVPC AI supports named arguments in custom commands using placeholders in the format `$NAME` (where NAME consists of uppercase letters, numbers, and underscores, and must start with a letter).

For example:

```markdown
# Fetch Context for Issue $ISSUE_NUMBER

RUN gh issue view $ISSUE_NUMBER --json title,body,comments
RUN git grep --author="$AUTHOR_NAME" -n .
RUN grep -R "$SEARCH_PATTERN" $DIRECTORY
```

When you run a command with arguments, SVPC AI will prompt you to enter values for each unique placeholder. Named arguments provide several benefits:

- Clear identification of what each argument represents
- Ability to use the same argument multiple times
- Better organization for commands with multiple inputs

### Organizing Commands

You can organize commands in subdirectories:

```
~/.config/svpc/commands/git/commit.md
```

This creates a command with ID `user:git:commit`.

### Using Custom Commands

1. Press `Ctrl+K` to open the command dialog
2. Select your custom command (prefixed with either `user:` or `project:`)
3. Press Enter to execute the command

The content of the command file will be sent as a message to the AI assistant.

### Built-in Commands

SVPC AI includes several built-in commands:

| Command            | Description                                                                                         |
| ------------------ | --------------------------------------------------------------------------------------------------- |
| Initialize Project | Creates or updates the SVPC.md memory file with project-specific information                        |
| Compact Session    | Manually triggers the summarization of the current session, creating a new session with the summary |

## MCP (Model Context Protocol)

SVPC AI implements the Model Context Protocol (MCP) to extend its capabilities through external tools. MCP provides a standardized way for the AI assistant to interact with external services and tools.

### MCP Features

- **External Tool Integration**: Connect to external tools and services via a standardized protocol
- **Tool Discovery**: Automatically discover available tools from MCP servers
- **Multiple Connection Types**:
  - **Stdio**: Communicate with tools via standard input/output
  - **SSE**: Communicate with tools via Server-Sent Events
- **Security**: Permission system for controlling access to MCP tools

### Configuring MCP Servers

MCP servers are defined in the configuration file under the `mcpServers` section:

```json
{
  "mcpServers": {
    "example": {
      "type": "stdio",
      "command": "path/to/mcp-server",
      "env": [],
      "args": []
    },
    "web-example": {
      "type": "sse",
      "url": "https://example.com/mcp",
      "headers": {
        "Authorization": "Bearer token"
      }
    }
  }
}
```

### MCP Tool Usage

Once configured, MCP tools are automatically available to the AI assistant alongside built-in tools. They follow the same permission model as other tools, requiring user approval before execution.

## LSP (Language Server Protocol)

SVPC AI integrates with Language Server Protocol to provide code intelligence features across multiple programming languages.

### LSP Features

- **Multi-language Support**: Connect to language servers for different programming languages
- **Diagnostics**: Receive error checking and linting information
- **File Watching**: Automatically notify language servers of file changes

### Configuring LSP

Language servers are configured in the configuration file under the `lsp` section:

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

### LSP Integration with AI

The AI assistant can access LSP features through the `diagnostics` tool, allowing it to:

- Check for errors in your code
- Suggest fixes based on diagnostics

While the LSP client implementation supports the full LSP protocol (including completions, hover, definition, etc.), currently only diagnostics are exposed to the AI assistant.

## Using Github Copilot

_Copilot support is currently experimental._

### Requirements

- [Copilot chat in the IDE](https://github.com/settings/copilot) enabled in GitHub settings
- One of:
  - VSCode Github Copilot chat extension
  - Github `gh` CLI
  - Neovim Github Copilot plugin (`copilot.vim` or `copilot.lua`)
  - Github token with copilot permissions

If using one of the above plugins or cli tools, make sure you use the authenticate
the tool with your github account. This should create a github token at one of the following locations:

- `~/.config/github-copilot/[hosts,apps].json`
- `$XDG_CONFIG_HOME/github-copilot/[hosts,apps].json`

If using an explicit github token, you may either set the `$GITHUB_TOKEN` environment variable or add it to the svpc.json config file at `providers.copilot.apiKey`.

## Using a self-hosted model provider

SVPC AI can also load and use models from a self-hosted (OpenAI-like) provider.
This is useful for developers who want to experiment with custom models.

### Configuring a self-hosted provider

You can use a self-hosted model by setting the `LOCAL_ENDPOINT` environment variable.
This will cause SVPC AI to load and use the models from the specified endpoint.

```bash
LOCAL_ENDPOINT=http://localhost:1235/v1
```

### Configuring a self-hosted model

You can also configure a self-hosted model in the configuration file under the `agents` section:

```json
{
  "agents": {
    "coder": {
      "model": "local.granite-3.3-2b-instruct@q8_0",
      "reasoningEffort": "high"
    }
  }
}
```

## Development

### Prerequisites

- Go 1.24.0 or higher

### Building from Source

```bash
# Clone the repository
git clone https://github.com/sovereignempirex-ux/svcp-ai.git
cd svcp-ai

# Build
go build -o svpc

# Run
./svpc
```

## License

SVPC AI is licensed under the MIT License. See the [LICENSE](LICENSE) file for details.

## Contributing

Contributions are welcome! Here's how you can contribute:

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add some amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

Please make sure to update tests as appropriate and follow the existing code style.