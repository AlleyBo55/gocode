# Supported Models — The Complete List

gocode speaks to 200+ models across 11 providers. Four native. Seven via the OpenAI-compatible shim. Plus any local model you can run.

Set one env var. Pick a model. Go.

---

## Native Providers

These are first-class integrations with full streaming, tool use, and thinking block support.

### Anthropic (Claude)

| Model | Alias | Full ID | Max Output |
|-------|-------|---------|------------|
| Claude Opus 4.6 | `opus` | `claude-opus-4-6` | 32K |
| Claude Sonnet 4.6 | `sonnet` | `claude-sonnet-4-6` | 64K |
| Claude Haiku 4.5 | `haiku` | `claude-haiku-4-5-20251213` | 8K |

```bash
export ANTHROPIC_API_KEY=sk-ant-...
gocode chat --model sonnet
```

### OpenAI (GPT)

| Model | Alias | Full ID | Max Output |
|-------|-------|---------|------------|
| GPT-5.4 | `gpt5`, `gpt54`, `gpt` | `gpt-5.4` | 128K |
| GPT-5.4 Mini | `gpt5-mini`, `gpt54-mini` | `gpt-5.4-mini` | 128K |
| GPT-5.4 Nano | `gpt54-nano` | `gpt-5.4-nano` | 128K |
| GPT-4o | `gpt4`, `gpt4o` | `gpt-4o` | 16K |
| GPT-4o Mini | `gpt4-mini` | `gpt-4o-mini` | 16K |
| o1 | `o1` | `o1` | 100K |
| o1 Mini | `o1-mini` | `o1-mini` | 100K |
| o3 | `o3` | `o3` | 100K |
| o3 Mini | `o3-mini` | `o3-mini` | 100K |
| o4 Mini | `o4-mini` | `o4-mini` | 100K |
| Codex Mini | `codex` | `codex-mini-latest` | 16K |

```bash
export OPENAI_API_KEY=sk-...
gocode chat --model gpt5
```

### Google (Gemini)

| Model | Alias | Full ID | Max Output |
|-------|-------|---------|------------|
| Gemini 3.1 Pro | `gemini`, `gemini-pro` | `gemini-3.1-pro-preview` | 65K |
| Gemini 3 Flash | `gemini-flash` | `gemini-3-flash` | 65K |
| Gemini 2.5 Pro | `gemini-2.5` | `gemini-2.5-pro` | 65K |

```bash
export GEMINI_API_KEY=AI...
gocode chat --model gemini
```

### xAI (Grok)

| Model | Alias | Full ID | Max Output |
|-------|-------|---------|------------|
| Grok 4.20 Beta | `grok` | `grok-4.20-beta` | 131K |
| Grok 3 | `grok-3` | `grok-3` | 131K |
| Grok 3 Mini | `grok-mini` | `grok-3-mini` | 131K |
| Grok 2 | `grok-2` | `grok-2` | 131K |

```bash
export XAI_API_KEY=xai-...
gocode chat --model grok
```

---

## Proxy Providers (OpenAI-Compatible Shim)

Any service that speaks the OpenAI chat completions API works out of the box. Set the API key and gocode auto-detects the provider.

### DeepSeek

| Model | Alias | Full ID |
|-------|-------|---------|
| DeepSeek Chat | `deepseek` | `deepseek-chat` |
| DeepSeek Reasoner (R1) | `deepseek-r1` | `deepseek-reasoner` |
| DeepSeek Coder | `deepseek-code` | `deepseek-coder` |

```bash
export DEEPSEEK_API_KEY=sk-...
gocode chat --model deepseek
```

### Mistral

| Model | Alias | Full ID |
|-------|-------|---------|
| Mistral Large | `mistral` | `mistral-large-latest` |
| Mistral Small | `mistral-small` | `mistral-small-latest` |
| Mistral Nemo | `mistral-nemo` | `open-mistral-nemo` |
| Codestral | `codestral` | `codestral-latest` |
| Pixtral Large | `pixtral` | `pixtral-large-latest` |

```bash
export MISTRAL_API_KEY=...
gocode chat --model mistral
```

### Groq (Ultra-Fast Inference)

| Model | Alias | Full ID |
|-------|-------|---------|
| Llama 3.3 70B | `groq-llama` | `llama-3.3-70b-versatile` |
| Mixtral 8x7B | `groq-mixtral` | `mixtral-8x7b-32768` |
| Gemma 2 9B | `groq-gemma` | `gemma2-9b-it` |

```bash
export GROQ_API_KEY=gsk_...
gocode chat --model groq-llama
```

### Together AI

| Model | Alias | Full ID |
|-------|-------|---------|
| Llama 3.1 70B Turbo | `together-llama` | `meta-llama/Meta-Llama-3.1-70B-Instruct-Turbo` |
| Llama 3.1 405B | `llama-405` | `meta-llama/Meta-Llama-3.1-405B-Instruct-Turbo` |
| Qwen 2.5 72B Turbo | `together-qwen` | `Qwen/Qwen2.5-72B-Instruct-Turbo` |

```bash
export TOGETHER_API_KEY=...
gocode chat --model together-llama
```

### OpenRouter (200+ Models)

OpenRouter gives you access to every model from every provider through a single API key. Pass any model ID from [openrouter.ai/models](https://openrouter.ai/models).

gocode auto-detects OpenRouter when the model name contains a `/` and `OPENROUTER_API_KEY` is set. No extra config needed.

```bash
export OPENROUTER_API_KEY=sk-or-...
gocode chat --model anthropic/claude-sonnet-4-20250514    # Claude
gocode chat --model openai/gpt-4o                         # GPT-4o
gocode chat --model google/gemini-2.5-pro-preview         # Gemini
gocode chat --model x-ai/grok-3                           # Grok
gocode chat --model moonshotai/kimi-k2                    # Kimi K2
gocode chat --model minimax/minimax-01                    # MiniMax
gocode chat --model qwen/qwen-2.5-72b-instruct           # Qwen
gocode chat --model meta-llama/llama-3.3-70b-instruct     # Llama
gocode chat --model mistralai/mistral-large-latest        # Mistral
gocode chat --model deepseek/deepseek-chat                # DeepSeek
# ... any model on openrouter.ai/models
```

### Azure OpenAI

```bash
export AZURE_OPENAI_ENDPOINT=https://your-resource.openai.azure.com
export AZURE_OPENAI_API_KEY=...
gocode chat --model gpt-4o  # uses your Azure deployment
```

### Codex Backend

```bash
# Auth from ~/.codex/auth.json (auto-detected)
gocode chat --model codex
```

---

## Local Models (Ollama, LM Studio, vLLM)

Set `OPENAI_BASE_URL` to point at any local inference server. No API key needed.

### Ollama

| Model | Alias | Full ID |
|-------|-------|---------|
| Llama 3.3 70B | `llama` | `llama3.3:70b` |
| Llama 3.1 8B | `llama-8b` | `llama3.1:8b` |
| Qwen 2.5 72B | `qwen` | `qwen2.5:72b` |
| Qwen 2.5 Coder 32B | `qwen-coder` | `qwen2.5-coder:32b` |
| DeepSeek Chat | `deepseek` | `deepseek-chat` |
| Mistral Large | `mistral` | `mistral-large-latest` |

```bash
# Start Ollama
ollama serve

# Point gocode at it
export OPENAI_BASE_URL=http://localhost:11434/v1
gocode chat --model llama
```

### LM Studio

```bash
# Start LM Studio server
export OPENAI_BASE_URL=http://localhost:1234/v1
gocode chat --model qwen-coder
```

### vLLM / Text Generation Inference / Any OpenAI-Compatible Server

```bash
export OPENAI_BASE_URL=http://localhost:8000/v1
gocode chat --model your-model-name
```

---

## Provider Launch Profiles

Auto-detect the best provider and model for your workflow:

```bash
gocode profile init                    # create default profile
gocode profile auto --goal coding      # auto-detect from env vars
gocode profile auto --goal latency     # optimize for speed
gocode profile auto --goal balanced    # best of both worlds
gocode profile recommend --goal coding # preview without saving
gocode profile show                    # show current profile
```

Or use `--goal` directly:

```bash
gocode chat --goal coding     # picks the best coding model from your available providers
gocode chat --goal latency    # picks the fastest model
gocode chat --goal balanced   # picks a balanced option
```

---

## Goal-Based Model Selection

| Goal | Anthropic | OpenAI | Gemini | xAI |
|------|-----------|--------|--------|-----|
| `coding` | Sonnet 4.6 | GPT-5.4 | Gemini 3.1 Pro | Grok 4.20 |
| `latency` | Haiku 4.5 | GPT-4o | Gemini 3 Flash | Grok 3 Mini |
| `balanced` | GPT-5.4 | GPT-5.4 | Gemini 3.1 Pro | Grok 4.20 |

---

## Environment Variables Reference

| Variable | Provider | Required |
|----------|----------|----------|
| `ANTHROPIC_API_KEY` | Anthropic (Claude) | For Claude models |
| `OPENAI_API_KEY` | OpenAI (GPT) | For GPT/o-series models |
| `GEMINI_API_KEY` | Google (Gemini) | For Gemini models |
| `XAI_API_KEY` | xAI (Grok) | For Grok models |
| `DEEPSEEK_API_KEY` | DeepSeek | For DeepSeek models |
| `MISTRAL_API_KEY` | Mistral | For Mistral/Codestral |
| `GROQ_API_KEY` | Groq | For Groq-hosted models |
| `TOGETHER_API_KEY` | Together AI | For Together-hosted models |
| `OPENROUTER_API_KEY` | OpenRouter | For any OpenRouter model |
| `AZURE_OPENAI_API_KEY` | Azure OpenAI | For Azure deployments |
| `AZURE_OPENAI_ENDPOINT` | Azure OpenAI | Azure resource URL |
| `OPENAI_BASE_URL` | Local / Custom | For Ollama, LM Studio, vLLM |

Set multiple keys to enable automatic fallback across providers.
