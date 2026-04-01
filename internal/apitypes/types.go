package apitypes

import "encoding/json"

// --- Request Types ---

// MessageRequest  MessageRequest.
type MessageRequest struct {
	Model      string         `json:"model"`
	MaxTokens  int            `json:"max_tokens"`
	Messages   []InputMessage `json:"messages"`
	System     string         `json:"system,omitempty"`
	Tools      []ToolDef      `json:"tools,omitempty"`
	ToolChoice *ToolChoice    `json:"tool_choice,omitempty"`
	Stream     bool           `json:"stream,omitempty"`
}

// WithStreaming returns a copy with Stream set to true.
func (r MessageRequest) WithStreaming() MessageRequest {
	r.Stream = true
	return r
}

// InputMessage is a message with role and content blocks.
type InputMessage struct {
	Role    string              `json:"role"`
	Content []InputContentBlock `json:"content"`
}

// UserText creates a user message with a single text block.
func UserText(text string) InputMessage {
	return InputMessage{
		Role:    "user",
		Content: []InputContentBlock{{Kind: "text", Text: text}},
	}
}

// UserToolResult creates a user message with a single tool_result block.
func UserToolResult(toolUseID, content string, isError bool) InputMessage {
	return InputMessage{
		Role: "user",
		Content: []InputContentBlock{{
			Kind:      "tool_result",
			ToolUseID: toolUseID,
			Content:   content,
			IsError:   isError,
		}},
	}
}

// InputContentBlock is a union type with Kind discriminator.
// Kind: "text", "tool_use", "tool_result"
type InputContentBlock struct {
	Kind      string          `json:"type"`
	Text      string          `json:"text,omitempty"`
	ID        string          `json:"id,omitempty"`
	Name      string          `json:"name,omitempty"`
	Input     json.RawMessage `json:"input,omitempty"`
	ToolUseID string          `json:"tool_use_id,omitempty"`
	Content   string          `json:"content,omitempty"`
	IsError   bool            `json:"is_error,omitempty"`
}

// ToolDef is an LLM tool definition.
type ToolDef struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"input_schema"`
}

// ToolChoice controls tool selection behavior.
type ToolChoice struct {
	Kind string `json:"type"`
	Name string `json:"name,omitempty"`
}

// --- Response Types ---

// MessageResponse  MessageResponse.
type MessageResponse struct {
	ID           string               `json:"id"`
	Type         string               `json:"type"`
	Role         string               `json:"role"`
	Content      []OutputContentBlock `json:"content"`
	Model        string               `json:"model"`
	StopReason   string               `json:"stop_reason,omitempty"`
	StopSequence string               `json:"stop_sequence,omitempty"`
	Usage        Usage                `json:"usage"`
	RequestID    string               `json:"request_id,omitempty"`
}

// TotalTokens returns the sum of input and output tokens.
func (r *MessageResponse) TotalTokens() int {
	return r.Usage.TotalTokens()
}

// OutputContentBlock is a union type with Kind discriminator.
// Kind: "text", "tool_use", "thinking", "redacted_thinking"
type OutputContentBlock struct {
	Kind      string          `json:"type"`
	Text      string          `json:"text,omitempty"`
	ID        string          `json:"id,omitempty"`
	Name      string          `json:"name,omitempty"`
	Input     json.RawMessage `json:"input,omitempty"`
	Thinking  string          `json:"thinking,omitempty"`
	Signature string          `json:"signature,omitempty"`
	Data      json.RawMessage `json:"data,omitempty"`
}

// Usage tracks token consumption.
type Usage struct {
	InputTokens              int `json:"input_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens,omitempty"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens,omitempty"`
	OutputTokens             int `json:"output_tokens"`
}

// TotalTokens returns input + output tokens.
func (u Usage) TotalTokens() int {
	return u.InputTokens + u.OutputTokens
}

// --- Streaming Types ---

// StreamEvent  StreamEvent enum.
type StreamEvent struct {
	Kind  string `json:"type"`
	Index int    `json:"index,omitempty"`

	// MessageStart
	Message *MessageResponse `json:"message,omitempty"`

	// MessageDelta
	Delta      *DeltaPayload `json:"delta,omitempty"`
	DeltaUsage *Usage        `json:"usage,omitempty"`

	// ContentBlockStart
	ContentBlock *OutputContentBlock `json:"content_block,omitempty"`

	// ContentBlockDelta
	BlockDelta *ContentBlockDelta `json:"block_delta,omitempty"`
}

// DeltaPayload carries stop_reason in message_delta events.
type DeltaPayload struct {
	StopReason   string `json:"stop_reason,omitempty"`
	StopSequence string `json:"stop_sequence,omitempty"`
}

// ContentBlockDelta is a union for incremental content updates.
// Kind: "text_delta", "input_json_delta", "thinking_delta", "signature_delta"
type ContentBlockDelta struct {
	Kind        string `json:"type"`
	Text        string `json:"text,omitempty"`
	PartialJSON string `json:"partial_json,omitempty"`
	Thinking    string `json:"thinking,omitempty"`
	Signature   string `json:"signature,omitempty"`
}

// --- Auth ---

// AuthSource represents credential types,  AuthSource enum.
type AuthSource struct {
	Kind        string // "none", "api_key", "bearer_token", "api_key_and_bearer"
	ApiKey      string
	BearerToken string
}

// AuthNone returns an empty AuthSource.
func AuthNone() AuthSource { return AuthSource{Kind: "none"} }

// AuthApiKey returns an AuthSource with an API key.
func AuthApiKey(key string) AuthSource { return AuthSource{Kind: "api_key", ApiKey: key} }

// AuthBearer returns an AuthSource with a bearer token.
func AuthBearer(token string) AuthSource { return AuthSource{Kind: "bearer_token", BearerToken: token} }

// AuthApiKeyAndBearer returns an AuthSource with both.
func AuthApiKeyAndBearer(key, token string) AuthSource {
	return AuthSource{Kind: "api_key_and_bearer", ApiKey: key, BearerToken: token}
}

// ToolResult is the outcome of a tool execution in the agent loop.
type ToolResult struct {
	ToolUseID string
	Output    string
	IsError   bool
}
