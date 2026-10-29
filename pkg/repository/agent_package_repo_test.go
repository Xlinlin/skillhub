package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/cinience/skillhub/pkg/config"
	"github.com/cinience/skillhub/pkg/model"
	"github.com/google/uuid"
	"path/filepath"
	"testing"
)

func TestSQLiteConnectionPragmas(t *testing.T) {
	db, e := NewDB(config.DatabaseConfig{Driver: "sqlite", URL: filepath.Join(t.TempDir(), "db.sqlite"), MaxOpenConns: 4, MaxIdleConns: 2})
	if e != nil {
		t.Fatal(e)
	}
	pool, e := db.DB()
	if e != nil {
		t.Fatal(e)
	}
	defer pool.Close()
	ctx := context.Background()
	c1, e := pool.Conn(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer c1.Close()
	c2, e := pool.Conn(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer c2.Close()
	for _, c := range []interface {
		QueryRowContext(context.Context, string, ...any) *sql.Row
	}{c1, c2} {
		var mode string
		var timeout int
		if e = c.QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&mode); e != nil {
			t.Fatal(e)
		}
		if e = c.QueryRowContext(ctx, "PRAGMA busy_timeout").Scan(&timeout); e != nil {
			t.Fatal(e)
		}
		if mode != "wal" || timeout != 5000 {
			t.Fatalf("journal=%s timeout=%d", mode, timeout)
		}
	}
}

func TestAgentPublishConstraints(t *testing.T) {
	db, e := NewDB(config.DatabaseConfig{Driver: "sqlite", URL: filepath.Join(t.TempDir(), "db.sqlite"), MaxOpenConns: 4})
	if e != nil {
		t.Fatal(e)
	}
	pool, _ := db.DB()
	defer pool.Close()
	ctx := context.Background()
	r := NewAgentPackageRepo(db)
	owner := &model.User{ID: uuid.New(), Handle: "author", Role: "user"}
	db.Create(owner)
	a := &model.AgentPackage{ID: uuid.New(), Slug: "sample", DisplayName: "Sample", Kind: "suite", OwnerID: owner.ID, Tags: model.StringArray{}}
	v := &model.AgentPackageVersion{ID: uuid.New(), AgentPackageID: a.ID, Version: "1.0.0", Files: json.RawMessage("[]"), CreatedBy: owner.ID, ArchiveKey: "key", ArchiveSHA256: "hash"}
	created, uncertain, e := r.CommitPublish(ctx, owner, a, v)
	if e != nil || !created || uncertain {
		t.Fatal(created, uncertain, e)
	}
	got, e := r.GetBySlug(ctx, "sample")
	if e != nil || got.VersionsCount != 1 || *got.LatestVersionID != v.ID {
		t.Fatal(got, e)
	}
	v2 := *v
	v2.ID = uuid.New()
	if _, _, e = r.CommitPublish(ctx, owner, a, &v2); !errors.Is(e, ErrAgentConflict) {
		t.Fatal("duplicate accepted", e)
	}
	v2.Version = "1.1.0"
	other := &model.User{ID: uuid.New(), Role: "admin"}
	if _, _, e = r.CommitPublish(ctx, other, a, &v2); !errors.Is(e, ErrAgentForbidden) {
		t.Fatal(e)
	}
	v2.Version = "1.10.0"
	if _, _, e = r.CommitPublish(ctx, owner, a, &v2); e != nil {
		t.Fatal(e)
	}
	list, total, e := r.List(ctx, AgentListOptions{Page: 1, PageSize: 20})
	if e != nil || total != 1 || len(list) != 1 || list[0].LatestVersion != "1.10.0" {
		t.Fatal(list, total, e)
	}
	if e = r.SoftDelete(ctx, owner, "sample"); e != nil {
		t.Fatal(e)
	}
	got, e = r.GetBySlug(ctx, "sample")
	if e != nil || got != nil {
		t.Fatal(got, e)
	}
	v2.ID = uuid.New()
	v2.Version = "2.0.0"
	if _, _, e = r.CommitPublish(ctx, owner, a, &v2); !errors.Is(e, ErrAgentConflict) {
		t.Fatal(e)
	}
}

func TestAgentVersion(t *testing.T) {
	for _, v := range []string{"01.0.0", "1.0", "1.0.0-beta", "1.0.0+build", "2147483648.0.0", "-1.0.0"} {
		if _, e := ParseAgentVersion(v); e == nil {
			t.Fatal(v)
		}
	}
	if v, e := ParseAgentVersion("2147483647.10.0"); e != nil || v[1] != 10 {
		t.Fatal(v, e)
	}
}
