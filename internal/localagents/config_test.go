package localagents

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/fastclaw-ai/fastclaw/internal/store"
)

func setupTestHome(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "fastclaw")
	t.Setenv("FASTCLAW_HOME", root)
	return root
}

func TestInitPreservesAgentConfigAndSettings(t *testing.T) {
	setupTestHome(t)
	t.Setenv("OPENAI_API_KEY", "test-key")

	if _, err := Init("scratch", InitOptions{
		Description: "initial",
		Provider:    "openai",
		Model:       "openai/gpt-4.1",
		APIKeyEnv:   "OPENAI_API_KEY",
	}); err != nil {
		t.Fatalf("init: %v", err)
	}
	if err := SetConfig("scratch", "temperature", "0.2"); err != nil {
		t.Fatalf("set temperature: %v", err)
	}

	st, inst, err := storeForName("scratch")
	if err != nil {
		t.Fatalf("storeForName: %v", err)
	}
	rec, err := st.GetAgent(context.Background(), inst.AgentID)
	if err != nil {
		t.Fatalf("get agent: %v", err)
	}
	rec.Config["workspace"] = "/tmp/workspace"
	if err := st.SaveAgent(context.Background(), rec); err != nil {
		t.Fatalf("save agent: %v", err)
	}
	st.Close()

	if _, err := Init("scratch", InitOptions{
		Provider:  "openai",
		Model:     "openai/gpt-4.1-mini",
		APIKeyEnv: "OPENAI_API_KEY",
	}); err != nil {
		t.Fatalf("re-init: %v", err)
	}

	st, inst, err = storeForName("scratch")
	if err != nil {
		t.Fatalf("storeForName after re-init: %v", err)
	}
	defer st.Close()
	rec, err = st.GetAgent(context.Background(), inst.AgentID)
	if err != nil {
		t.Fatalf("get agent after re-init: %v", err)
	}
	if got := rec.Config["description"]; got != "initial" {
		t.Fatalf("description not preserved: got %#v", got)
	}
	if got := rec.Config["workspace"]; got != "/tmp/workspace" {
		t.Fatalf("workspace not preserved: got %#v", got)
	}

	temp, err := GetConfig("scratch", "temperature")
	if err != nil {
		t.Fatalf("get temperature: %v", err)
	}
	if got, ok := temp.(float64); !ok || got != 0.2 {
		t.Fatalf("temperature not preserved: got %#v", temp)
	}
	model, err := GetConfig("scratch", "model")
	if err != nil {
		t.Fatalf("get model: %v", err)
	}
	if model != "openai/gpt-4.1-mini" {
		t.Fatalf("model not updated: got %#v", model)
	}
}

func TestSetProviderFieldUsesPresetForNewProvider(t *testing.T) {
	setupTestHome(t)
	t.Setenv("OPENAI_API_KEY", "test-key")

	if _, err := Init("scratch", InitOptions{}); err != nil {
		t.Fatalf("init: %v", err)
	}
	if err := SetConfig("scratch", "provider.openai.apiKeyEnv", "OPENAI_API_KEY"); err != nil {
		t.Fatalf("set provider apiKeyEnv: %v", err)
	}

	base, err := GetConfig("scratch", "provider.openai.apiBase")
	if err != nil {
		t.Fatalf("get provider apiBase: %v", err)
	}
	if base != "https://api.openai.com/v1" {
		t.Fatalf("apiBase preset missing: got %#v", base)
	}
	apiType, err := GetConfig("scratch", "provider.openai.apiType")
	if err != nil {
		t.Fatalf("get provider apiType: %v", err)
	}
	if apiType != "openai-chat" {
		t.Fatalf("apiType preset missing: got %#v", apiType)
	}
}

func TestInitRejectsMismatchedProviderModel(t *testing.T) {
	setupTestHome(t)
	t.Setenv("OPENAI_API_KEY", "test-key")

	_, err := Init("scratch", InitOptions{
		Provider:  "openai",
		Model:     "anthropic/claude-3-5-sonnet",
		APIKeyEnv: "OPENAI_API_KEY",
	})
	if err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("expected provider/model mismatch error, got %v", err)
	}
}

func TestPutFileRejectsUnsupportedSystemFilename(t *testing.T) {
	setupTestHome(t)
	if _, err := Init("scratch", InitOptions{}); err != nil {
		t.Fatalf("init: %v", err)
	}
	path := filepath.Join(t.TempDir(), "NOTES.md")
	if err := os.WriteFile(path, []byte("notes"), 0o644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	err := PutFile("scratch", "NOTES.md", path)
	if err == nil {
		t.Fatal("expected unsupported filename error")
	}
	if errors.Is(err, store.ErrNotFound) {
		t.Fatalf("expected validation before store lookup, got %v", err)
	}
}

func TestInitRejectsRunningPortChange(t *testing.T) {
	root := setupTestHome(t)
	p, err := instancePaths("scratch")
	if err != nil {
		t.Fatalf("instance paths: %v", err)
	}
	if err := saveInstance(p.metaFile, &Instance{
		Name: "scratch",
		PID:  os.Getpid(),
		Port: 34567,
		Home: filepath.Join(root, "local-agents", "scratch"),
	}); err != nil {
		t.Fatalf("save instance: %v", err)
	}

	_, err = Init("scratch", InitOptions{Port: 34568})
	if err == nil || !strings.Contains(err.Error(), "stop it before changing --port") {
		t.Fatalf("expected running port-change error, got %v", err)
	}
}

func TestStartRejectsUnavailablePort(t *testing.T) {
	setupTestHome(t)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	_, portText, err := net.SplitHostPort(ln.Addr().String())
	if err != nil {
		t.Fatalf("split addr: %v", err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatalf("parse port: %v", err)
	}

	_, err = Start("scratch", StartOptions{Port: port})
	if err == nil || !strings.Contains(err.Error(), "not available") {
		t.Fatalf("expected unavailable port error, got %v", err)
	}
}

func TestCorruptMetadataIsNotOverwritten(t *testing.T) {
	setupTestHome(t)
	p, err := instancePaths("scratch")
	if err != nil {
		t.Fatalf("instance paths: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(p.metaFile), 0o755); err != nil {
		t.Fatalf("create state dir: %v", err)
	}
	if err := os.WriteFile(p.metaFile, []byte("{bad json"), 0o644); err != nil {
		t.Fatalf("write metadata: %v", err)
	}

	if _, err := Init("scratch", InitOptions{}); err == nil {
		t.Fatal("expected init to reject corrupt metadata")
	}
	if _, err := Start("scratch", StartOptions{}); err == nil {
		t.Fatal("expected start to reject corrupt metadata")
	}
}
