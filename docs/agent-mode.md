# Agent Mode Guide

[← Back to README](../README.md)

gocode's agent mode lets you use any supported LLM as a standalone coding agent directly from your terminal. No IDE required.

---

## Setting Up Your API Key

Set one environment variable for your preferred provider:

```bash
# Anthropic Claude
export ANTHROPIC_API_KEY=sk-ant-api03-...

# OpenAI GPT
export OPENAI_API_KEY=sk-proj-...

# Google Gemini
export GEMINI_API_KEY=AIza...
# or
export GOOGLE_API_KEY=AIza...

# xAI Grok
export XAI_API_KEY=xai-...
```

Add it to your `~/.zshrc`, `~/.bashrc`, or `~/.bash_profile` to persist across sessions:

```bash
echo 'export ANTHROPIC_API_KEY=sk-ant-...' >> ~/.zshrc
source ~/.zshrc
```

You can also pass the key inline with `--api-key`:

```bash
gocode chat --api-key sk-ant-api03-...
```

### Anthropic Advanced Auth

Anthropic supports two auth methods simultaneously:

| Env Var | Purpose |
|---------|---------|
| `ANTHROPIC_API_KEY` | Standard API key (sent as `x-api-key` header) |
| `ANTHROPIC_AUTH_TOKEN` | Bearer token for OAuth/proxy setups |

If both are set, gocode sends both headers on every request.

---

## Interactive Chat (`gocode chat`)

Start a conversation:

```bash
gocode chat
```

You'll see a prompt:

```
gocode agent — type /exit to quit, /clear to reset, /cost for usage

you> help me refactor the auth module
```

The agent will read your code, suggest changes, run tools, and iterate until the task is done.

### Chat Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--model` | `sonnet` | Model name or alias |
| `--max-turns` | `30` | Max agent loop iterations per message |
| `--max-tokens` | `8192` | Max output tokens per API request |
| `--api-key` | (env var) | Override API key |
| `--resume` | — | Resume a saved session by ID |

### Slash Commands

| Command | What It Does |
|---------|-------------|
| `/exit` | Quit the session (also: Ctrl+D) |
| `/clear` | Reset conversation history, keep connection |
| `/cost` | Show cumulative token usage and cost |

### Multi-line Input

End a line with `\` to continue on the next line:

```
you> write a function that\
takes a list of integers\
and returns the sum
```

---

## One-Shot Mode (`gocode prompt`)

Run a single prompt and exit:

```bash
gocode prompt "find all files with TODO comments and list them"
```

### Prompt Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--model` | `sonnet` | Model name or alias |
| `--max-turns` | `30` | Max agent loop iterations |
| `--max-tokens` | `8192` | Max output tokens per request |
| `--api-key` | (env var) | Override API key |
| `--no-stream` | `false` | Wait for full response before printing |

### Examples

```bash
# Use GPT-4o
gocode prompt --model gpt4o "explain this codebase structure"

# Use Gemini
gocode prompt --model gemini "write tests for the auth module"

# Use Grok
gocode prompt --model grok "find performance bottlenecks in main.go"

# Disable streaming for piping
gocode prompt --no-stream "list all exported functions" > functions.txt
```

---

## Changing Models

Use the `--model` flag with either a short alias or a full model ID:

```bash
# Short aliases
gocode chat --model opus          # Claude Opus 4
gocode chat --model sonnet        # Claude Sonnet 4
gocode chat --model gpt4o         # GPT-4o
gocode chat --model gpt4-mini     # GPT-4o-mini
gocode chat --model o3             # OpenAI o3
gocode chat --model gemini        # Gemini 2.5 Pro
gocode chat --model gemini-flash  # Gemini 2.5 Flash
gocode chat --model grok          # Grok 3
gocode chat --model codex         # Codex Mini

# Full model IDs
gocode chat --model claude-opus-4-6
gocode chat --model gpt-4o-2024-08-06
gocode chat --model gemini-2.5-pro
gocode chat --model grok-3
```

### Model Alias Table

| Alias | Resolves To | Provider |
|-------|------------|----------|
| `opus` | `claude-opus-4-6` | Anthropic |
| `sonnet` | `claude-sonnet-4-6` | Anthropic |
| `haiku` | `claude-haiku-4-5-20251213` | Anthropic |
| `gpt4o` / `gpt4` / `gpt` | `gpt-4o` | OpenAI |
| `gpt4-mini` | `gpt-4o-mini` | OpenAI |
| `o1` | `o1` | OpenAI |
| `o1-mini` | `o1-mini` | OpenAI |
| `o3` | `o3` | OpenAI |
| `o3-mini` | `o3-mini` | OpenAI |
| `o4-mini` | `o4-mini` | OpenAI |
| `codex` | `codex-mini-latest` | OpenAI |
| `gemini` / `gemini-pro` | `gemini-2.5-pro` | Google |
| `gemini-flash` | `gemini-2.5-flash` | Google |
| `grok` | `grok-3` | xAI |
| `grok-mini` | `grok-3-mini` | xAI |
| `grok-2` | `grok-2` | xAI |

### Auto-Detection

If you don't specify `--model`, gocode defaults to `sonnet` (Claude Sonnet 4). The provider is auto-detected from the model name:

- `claude*` → Anthropic
- `gpt*`, `o1*`, `o3*`, `o4*`, `codex*` → OpenAI
- `gemini*` → Google
- `grok*` → xAI

If the model name doesn't match any prefix, gocode checks which API key env vars are set and picks the first available.

---

## Custom Base URLs

Override the API endpoint for any provider:

```bash
# Anthropic proxy
export ANTHROPIC_BASE_URL=https://my-proxy.example.com

# OpenAI-compatible endpoint (e.g., Azure, local)
export OPENAI_BASE_URL=https://my-openai-proxy.example.com/v1

# Gemini proxy
export GEMINI_BASE_URL=https://my-gemini-proxy.example.com/v1beta/openai

# xAI proxy
export XAI_BASE_URL=https://my-xai-proxy.example.com/v1
```

This works with any OpenAI-compatible API (LiteLLM, Ollama, vLLM, etc.):

```bash
export OPENAI_BASE_URL=http://localhost:11434/v1
export OPENAI_API_KEY=ollama
gocode chat --model llama3.1
```

---

## How the Agent Loop Works

1. You send a message
2. gocode sends it to the LLM with your conversation history + available tools
3. The LLM responds with text and/or tool calls
4. gocode executes each tool (file read, shell command, etc.)
5. Tool results are sent back to the LLM
6. Repeat until the LLM responds with just text (no more tool calls)

The loop has safety limits:
- `--max-turns` caps the number of iterations (default: 30)
- Permission prompts for dangerous operations (in `chat` mode)
- Token usage tracking via `/cost`

---

## Available Tools

When running as an agent, the LLM can use these tools:

| Tool | What It Does |
|------|-------------|
| `BashTool` | Run shell commands |
| `FileReadTool` | Read file contents |
| `FileEditTool` | Edit files via search/replace |
| `FileWriteTool` | Create or overwrite files |
| `GlobTool` | Find files by pattern |
| `GrepTool` | Search file contents |
| `ListDirectoryTool` | List directory contents |

---

[← Back to README](../README.md)
