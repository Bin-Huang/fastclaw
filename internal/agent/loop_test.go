package agent

import (
	"testing"
	"time"

	"github.com/fastclaw-ai/fastclaw/internal/provider"
)

func TestMetadataInt(t *testing.T) {
	tests := []struct {
		name string
		md   map[string]any
		key  string
		want int
	}{
		{"nil map", nil, "tokens_in", 0},
		{"missing key", map[string]any{"other": 1}, "tokens_in", 0},
		{"int", map[string]any{"tokens_in": 1234}, "tokens_in", 1234},
		{"int64", map[string]any{"tokens_in": int64(9001)}, "tokens_in", 9001},
		// JSON round-trip path: encoding/json decodes numbers into
		// float64 when the destination is map[string]any. Without the
		// float branch the runs aggregator would silently drop every
		// persisted assistant turn.
		{"float64 from JSON", map[string]any{"tokens_in": float64(2048)}, "tokens_in", 2048},
		// Unsupported types (e.g. strings) must not panic — the
		// metadata map is loose-typed so a malformed write must
		// degrade to "unknown" rather than crash the trace render.
		{"wrong type degrades to 0", map[string]any{"tokens_in": "1234"}, "tokens_in", 0},
		{"zero is zero", map[string]any{"tokens_in": 0}, "tokens_in", 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := metadataInt(tc.md, tc.key); got != tc.want {
				t.Errorf("metadataInt(%v, %q) = %d, want %d", tc.md, tc.key, got, tc.want)
			}
		})
	}
}

func TestTurnMetadata_OmitsZeroUsageFields(t *testing.T) {
	// Zero-valued Usage fields must NOT appear in the metadata map —
	// downstream renderers gate on the field being present to decide
	// whether to display it. A literal 0 ("zero tokens used") is
	// indistinguishable from "API didn't tell us", so we never emit
	// the field when we can't tell.
	start := time.Now().Add(-time.Second)
	md := turnMetadata("claude-sonnet-4-6", start, provider.Usage{})
	if _, ok := md["tokens_in"]; ok {
		t.Errorf("tokens_in should be omitted when InputTokens=0; got %v", md)
	}
	if _, ok := md["tokens_out"]; ok {
		t.Errorf("tokens_out should be omitted when OutputTokens=0; got %v", md)
	}
	if _, ok := md["cache_read"]; ok {
		t.Errorf("cache_read should be omitted when CacheReadTokens=0; got %v", md)
	}
	if md["model"] != "claude-sonnet-4-6" {
		t.Errorf("model not propagated: %v", md["model"])
	}
	if d, ok := md["duration_ms"].(int64); !ok || d <= 0 {
		t.Errorf("duration_ms missing or non-positive: %v", md["duration_ms"])
	}
}

func TestTurnMetadata_IncludesNonZeroFields(t *testing.T) {
	md := turnMetadata("gpt-4o", time.Now(), provider.Usage{
		InputTokens:         1234,
		OutputTokens:        567,
		CacheReadTokens:     100,
		CacheCreationTokens: 50,
	})
	for k, want := range map[string]int{
		"tokens_in":      1234,
		"tokens_out":     567,
		"cache_read":     100,
		"cache_creation": 50,
	} {
		if got, ok := md[k].(int); !ok || got != want {
			t.Errorf("%s = %v, want %d", k, md[k], want)
		}
	}
}
