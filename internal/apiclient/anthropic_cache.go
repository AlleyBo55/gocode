package apiclient

import (
	"github.com/AlleyBo55/gocode/internal/apitypes"
)

// Prompt caching on Anthropic.
//
// Every turn of an agent loop resends the same tools, the same system prompt
// and the whole conversation so far. Anthropic bills a cached prefix read at
// a tenth of the normal input price, so on a 30-turn tool loop caching is the
// difference between paying for the prefix once and paying for it thirty
// times. Two breakpoints do it:
//
//   1. On the system prompt. The cache prefix is ordered tools, then system,
//      then messages, so a breakpoint here covers the tool schemas as well.
//   2. On the last block of the last message. Everything before the newest
//      user turn was already sent last turn and is a cache hit.
//
// Breakpoints are set on copies: MessageRequest.Messages aliases the
// runtime's session, and a marker left on a stored block would still be there
// next turn, adding one breakpoint per turn until the API rejects the request
// at five. Prefixes shorter than the model's minimum (1024 tokens on Sonnet
// and Opus) are simply not cached; the marker is harmless.

// promptCacheDisableEnv turns caching off for Anthropic-compatible endpoints
// that reject cache_control.
const promptCacheDisableEnv = "GOCODE_DISABLE_PROMPT_CACHE"

func promptCacheEnabled() bool {
	return !envNonEmpty(promptCacheDisableEnv)
}

// anthropicRequest is the wire shape. It differs from MessageRequest only in
// System, which becomes a list of blocks so one of them can carry the marker.
type anthropicRequest struct {
	Model      string                       `json:"model"`
	MaxTokens  int                          `json:"max_tokens"`
	Messages   []apitypes.InputMessage      `json:"messages"`
	System     []apitypes.InputContentBlock `json:"system,omitempty"`
	Tools      []apitypes.ToolDef           `json:"tools,omitempty"`
	ToolChoice *apitypes.ToolChoice         `json:"tool_choice,omitempty"`
	Stream     bool                         `json:"stream,omitempty"`
}

// anthropicWireRequest converts a MessageRequest for the Anthropic API,
// adding cache breakpoints when enabled.
func anthropicWireRequest(req apitypes.MessageRequest, cache bool) anthropicRequest {
	out := anthropicRequest{
		Model:      req.Model,
		MaxTokens:  req.MaxTokens,
		Messages:   req.Messages,
		Tools:      req.Tools,
		ToolChoice: req.ToolChoice,
		Stream:     req.Stream,
	}
	if req.System != "" {
		block := apitypes.InputContentBlock{Kind: "text", Text: req.System}
		if cache {
			block.CacheControl = apitypes.EphemeralCache()
		}
		out.System = []apitypes.InputContentBlock{block}
	}
	if cache && len(req.Messages) > 0 {
		out.Messages = withTrailingBreakpoint(req.Messages)
	}
	return out
}

// withTrailingBreakpoint returns messages with a cache marker on the final
// block of the final message, copying only what it changes. Any markers that
// somehow reached the stored session are stripped so the count stays at one.
func withTrailingBreakpoint(msgs []apitypes.InputMessage) []apitypes.InputMessage {
	out := make([]apitypes.InputMessage, len(msgs))
	copy(out, msgs)
	for i := range out {
		if !hasCacheMarker(out[i].Content) {
			continue
		}
		blocks := make([]apitypes.InputContentBlock, len(out[i].Content))
		copy(blocks, out[i].Content)
		for j := range blocks {
			blocks[j].CacheControl = nil
		}
		out[i].Content = blocks
	}
	last := len(out) - 1
	if len(out[last].Content) == 0 {
		return out
	}
	blocks := make([]apitypes.InputContentBlock, len(out[last].Content))
	copy(blocks, out[last].Content)
	blocks[len(blocks)-1].CacheControl = apitypes.EphemeralCache()
	out[last].Content = blocks
	return out
}

func hasCacheMarker(blocks []apitypes.InputContentBlock) bool {
	for _, b := range blocks {
		if b.CacheControl != nil {
			return true
		}
	}
	return false
}
