package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAgentStoreConfig(t *testing.T) {
	t.Setenv("SKILLHUB_AGENT_PATH", "")
	if cfg := DefaultConfig(); cfg.AgentStore.BasePath != "./data/agents" {
		t.Fatal(cfg.AgentStore)
	}
	p := filepath.Join(t.TempDir(), "config.yaml")
	if e := os.WriteFile(p, []byte("agentstore:\n  base_path: ./custom-agents\n"), 0600); e != nil {
		t.Fatal(e)
	}
	cfg, e := Load(p)
	if e != nil || cfg.AgentStore.BasePath != "./custom-agents" {
		t.Fatal(cfg, e)
	}
	t.Setenv("SKILLHUB_AGENT_PATH", "./env-agents")
	cfg, e = Load(p)
	if e != nil || cfg.AgentStore.BasePath != "./env-agents" {
		t.Fatal(cfg, e)
	}
}
