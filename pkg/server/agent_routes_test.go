package server

import (
	"context"
	"encoding/json"
	"github.com/cinience/skillhub/pkg/config"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func TestAgentRoutes(t *testing.T) {
	cfg := config.DefaultConfig()
	root := t.TempDir()
	cfg.Database.URL = filepath.Join(root, "db.sqlite")
	cfg.GitStore.BasePath = filepath.Join(root, "repos")
	cfg.Search.IndexPath = filepath.Join(root, "missing-parent", "missing", "index")
	cfg.AgentStore.BasePath = filepath.Join(root, "agents")
	s, e := New(cfg)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Shutdown(context.Background())
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/agents", nil))
	var page struct {
		Data  []any `json:"data"`
		Total int   `json:"total"`
	}
	if e = json.Unmarshal(w.Body.Bytes(), &page); e != nil || w.Code != 200 || page.Data == nil {
		t.Fatalf("status=%d body=%s error=%v", w.Code, w.Body, e)
	}
	w = httptest.NewRecorder()
	s.router.ServeHTTP(w, httptest.NewRequest("POST", "/api/v1/agents", nil))
	if w.Code != 401 {
		t.Fatalf("want401 got %d %s", w.Code, w.Body)
	}
}
