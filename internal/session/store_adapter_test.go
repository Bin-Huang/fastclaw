package session

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/fastclaw-ai/fastclaw/internal/provider"
	"github.com/fastclaw-ai/fastclaw/internal/store"
)

func newSQLiteStore(t *testing.T) store.Store {
	t.Helper()
	dsn := filepath.Join(t.TempDir(), "test.db")
	st, err := store.NewDBStore("sqlite", dsn)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := st.Migrate(context.Background()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

// TestStoreAdapter_TimestampRoundTrip is the regression for a pre-PR
// bug surfaced by the trace viewer: SaveSession overwrote each
// message's Timestamp with time.Now() and GetSession dropped it on the
// way back. Result: every store-backed session loaded with timestamp=0
// across the board, which made the trace viewer's relative-time offsets
// and the runs list's duration column read 0 in the default
// (SQLite-backed) deployment.
func TestStoreAdapter_TimestampRoundTrip(t *testing.T) {
	a := NewStoreAdapter(newSQLiteStore(t), "u1")

	ts0 := time.Now().UnixMilli()
	in := []provider.Message{
		{Role: "user", Content: "hi", Timestamp: ts0},
		{Role: "assistant", Content: "hello", Timestamp: ts0 + 1200},
		{Role: "user", Content: "again", Timestamp: ts0 + 5000},
	}

	if err := a.SaveSession(context.Background(), "agt_1", "web_s1", in); err != nil {
		t.Fatalf("save: %v", err)
	}
	out, err := a.GetSession(context.Background(), "agt_1", "web_s1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(out) != len(in) {
		t.Fatalf("got %d messages, want %d", len(out), len(in))
	}
	for i, want := range in {
		if out[i].Timestamp != want.Timestamp {
			t.Errorf("msg[%d].Timestamp = %d, want %d (delta %d ms)",
				i, out[i].Timestamp, want.Timestamp, out[i].Timestamp-want.Timestamp)
		}
	}
}

// TestStoreAdapter_TimestampZeroFallsBackToNow verifies that a message
// without an explicit timestamp (legacy data, programmatic API caller
// that doesn't set it) still gets a non-zero stamp at save time —
// otherwise the trace timeline gets a "#1" anchor with no baseline.
func TestStoreAdapter_TimestampZeroFallsBackToNow(t *testing.T) {
	a := NewStoreAdapter(newSQLiteStore(t), "u1")

	in := []provider.Message{{Role: "user", Content: "hi"}} // Timestamp=0

	before := time.Now().UnixMilli()
	if err := a.SaveSession(context.Background(), "agt_1", "web_s1", in); err != nil {
		t.Fatalf("save: %v", err)
	}
	after := time.Now().UnixMilli()
	out, err := a.GetSession(context.Background(), "agt_1", "web_s1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got := out[0].Timestamp; got < before || got > after {
		t.Errorf("Timestamp = %d, want in [%d, %d] — should fall back to time.Now()", got, before, after)
	}
}
