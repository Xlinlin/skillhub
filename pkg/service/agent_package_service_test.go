package service

import (
	"context"
	"github.com/cinience/skillhub/pkg/agentarchive"
	"github.com/cinience/skillhub/pkg/config"
	"github.com/cinience/skillhub/pkg/model"
	"github.com/cinience/skillhub/pkg/repository"
	"github.com/google/uuid"
	"io"
	"path/filepath"
	"strings"
	"testing"
)

func TestAgentPublishSnapshots(t *testing.T) {
	db, e := repository.NewDB(config.DatabaseConfig{Driver: "sqlite", URL: filepath.Join(t.TempDir(), "db.sqlite"), MaxOpenConns: 4})
	if e != nil {
		t.Fatal(e)
	}
	pool, _ := db.DB()
	defer pool.Close()
	store, e := agentarchive.New(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	s := NewAgentPackageService(repository.NewAgentPackageRepo(db), store)
	user := &model.User{ID: uuid.New(), Handle: "author", Role: "user"}
	db.Create(user)
	other := &model.User{ID: uuid.New(), Handle: "other", Role: "admin"}
	db.Create(other)
	file := func(path, body string) agentarchive.InputFile {
		return agentarchive.InputFile{Path: path, Open: func() (io.ReadCloser, error) { return io.NopCloser(strings.NewReader(body)), nil }}
	}
	req := AgentPublishRequest{Slug: "suite", DisplayName: "Suite", Kind: "suite", Version: "1.0.0", Mode: "files", Files: []agentarchive.InputFile{file(".agents/pm.md", "v1"), file("old.txt", "old")}}
	ctx := context.Background()
	first, e := s.Publish(ctx, user, req)
	if e != nil || !first.Created {
		t.Fatal(first, e)
	}
	req.Version = "1.1.0"
	req.Files = []agentarchive.InputFile{file(".agents/pm.md", "v2")}
	second, e := s.Publish(ctx, user, req)
	if e != nil || second.Created {
		t.Fatal(second, e)
	}
	got, e := s.ReadText(ctx, "suite", "1.0.0", ".agents/pm.md")
	if e != nil || string(got) != "v1" {
		t.Fatal(string(got), e)
	}
	if _, e = s.ReadText(ctx, "suite", "latest", "old.txt"); e == nil {
		t.Fatal("old file leaked into snapshot")
	}
	req.Version = "1.2.0"
	if _, e = s.Publish(ctx, user, req); e != nil {
		t.Fatal("same content version rejected", e)
	}
	if _, e = s.Publish(ctx, other, req); e == nil {
		t.Fatal("nonowner admin appended")
	}
	if e = s.Delete(ctx, other, "suite"); e != nil {
		t.Fatal(e)
	}
	if _, _, e = s.ResolvePackageVersion(ctx, "suite", "1.0.0"); e == nil {
		t.Fatal("deleted asset readable")
	}
}
