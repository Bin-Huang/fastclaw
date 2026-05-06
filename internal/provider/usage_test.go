package provider

import (
	"strings"
	"testing"
)

// TestAnthropicParseSSE_Usage verifies the SSE parser pulls usage from
// the two events Anthropic emits it on: message_start carries input
// tokens (plus cache breakdowns) and an initial output_tokens; the
// FINAL message_delta carries the cumulative output_tokens. Anything
// less and the trace UI's "1240 → 85t" line goes blank for every turn.
func TestAnthropicParseSSE_Usage(t *testing.T) {
	body := strings.Join([]string{
		`data: {"type":"message_start","message":{"id":"msg_1","usage":{"input_tokens":1240,"output_tokens":1,"cache_read_input_tokens":320,"cache_creation_input_tokens":80}}}`,
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hello"}}`,
		`data: {"type":"content_block_stop","index":0}`,
		// Intermediate delta — must NOT clobber the final value below.
		`data: {"type":"message_delta","delta":{"stop_reason":null},"usage":{"output_tokens":12}}`,
		`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":85}}`,
		`data: {"type":"message_stop"}`,
		"",
	}, "\n")

	p := &AnthropicProvider{}
	resp, err := p.parseSSE(strings.NewReader(body))
	if err != nil {
		t.Fatalf("parseSSE error: %v", err)
	}
	if resp.Content != "hello" {
		t.Errorf("content = %q, want %q", resp.Content, "hello")
	}
	if resp.Usage.InputTokens != 1240 {
		t.Errorf("InputTokens = %d, want 1240", resp.Usage.InputTokens)
	}
	if resp.Usage.OutputTokens != 85 {
		t.Errorf("OutputTokens = %d, want 85 (the FINAL message_delta)", resp.Usage.OutputTokens)
	}
	if resp.Usage.CacheReadTokens != 320 {
		t.Errorf("CacheReadTokens = %d, want 320", resp.Usage.CacheReadTokens)
	}
	if resp.Usage.CacheCreationTokens != 80 {
		t.Errorf("CacheCreationTokens = %d, want 80", resp.Usage.CacheCreationTokens)
	}
}

// TestAnthropicParseSSE_NoUsage covers the older endpoints / proxy
// servers that strip usage entirely. Empty Usage is the expected
// fallback — the trace UI then just doesn't render that row.
func TestAnthropicParseSSE_NoUsage(t *testing.T) {
	body := strings.Join([]string{
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"ok"}}`,
		`data: {"type":"message_stop"}`,
		"",
	}, "\n")
	p := &AnthropicProvider{}
	resp, err := p.parseSSE(strings.NewReader(body))
	if err != nil {
		t.Fatalf("parseSSE error: %v", err)
	}
	if resp.Usage.InputTokens != 0 || resp.Usage.OutputTokens != 0 {
		t.Errorf("expected zero Usage, got %+v", resp.Usage)
	}
}

// TestOpenAIParseSSE_Usage verifies the trailing usage chunk (sent only
// when stream_options.include_usage=true) populates Response.Usage.
// OpenAI emits the chunk with empty choices, before [DONE].
func TestOpenAIParseSSE_Usage(t *testing.T) {
	body := strings.Join([]string{
		`data: {"choices":[{"delta":{"role":"assistant","content":"hi"},"finish_reason":null}]}`,
		`data: {"choices":[{"delta":{"content":""},"finish_reason":"stop"}]}`,
		`data: {"choices":[],"usage":{"prompt_tokens":1500,"completion_tokens":42,"total_tokens":1542,"prompt_tokens_details":{"cached_tokens":100}}}`,
		`data: [DONE]`,
		"",
	}, "\n")
	p := &OpenAIProvider{}
	resp, err := p.parseSSE(strings.NewReader(body))
	if err != nil {
		t.Fatalf("parseSSE error: %v", err)
	}
	if resp.Content != "hi" {
		t.Errorf("content = %q, want %q", resp.Content, "hi")
	}
	if resp.Usage.InputTokens != 1500 {
		t.Errorf("InputTokens = %d, want 1500", resp.Usage.InputTokens)
	}
	if resp.Usage.OutputTokens != 42 {
		t.Errorf("OutputTokens = %d, want 42", resp.Usage.OutputTokens)
	}
	if resp.Usage.CacheReadTokens != 100 {
		t.Errorf("CacheReadTokens = %d, want 100 (from prompt_tokens_details.cached_tokens)", resp.Usage.CacheReadTokens)
	}
}

// TestOpenAIParseSSE_NoUsageChunk handles servers that don't honor
// stream_options.include_usage (some OpenAI-compat proxies). Should
// return zero Usage without erroring out the entire response.
func TestOpenAIParseSSE_NoUsageChunk(t *testing.T) {
	body := strings.Join([]string{
		`data: {"choices":[{"delta":{"role":"assistant","content":"hi"},"finish_reason":null}]}`,
		`data: {"choices":[{"delta":{},"finish_reason":"stop"}]}`,
		`data: [DONE]`,
		"",
	}, "\n")
	p := &OpenAIProvider{}
	resp, err := p.parseSSE(strings.NewReader(body))
	if err != nil {
		t.Fatalf("parseSSE error: %v", err)
	}
	if resp.Content != "hi" {
		t.Errorf("content = %q, want %q", resp.Content, "hi")
	}
	if resp.Usage.InputTokens != 0 || resp.Usage.OutputTokens != 0 {
		t.Errorf("expected zero Usage, got %+v", resp.Usage)
	}
}
