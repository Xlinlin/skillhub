package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/cinience/skillhub/pkg/config"
	"github.com/cinience/skillhub/pkg/model"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func testAgentQueries(t *testing.T, db *gorm.DB) {
	t.Helper()
	ctx := context.Background()
	repo := NewAgentPackageRepo(db)
	owner := model.User{ID: uuid.New(), Handle: "query-" + uuid.NewString(), Role: "user"}
	if e := db.Create(&owner).Error; e != nil {
		t.Fatal(e)
	}
	prefix := "query-" + uuid.NewString()
	ids := []uuid.UUID{}
	t.Cleanup(func() {
		for _, id := range ids {
			db.Where("agent_package_id = ?", id).Delete(&model.AgentPackageVersion{})
			db.Where("id = ?", id).Delete(&model.AgentPackage{})
		}
		db.Where("id = ?", owner.ID).Delete(&owner)
	})
	for i, name := range []string{"Alpha_100% 中文", "alpha!z", "' OR 1=1 --"} {
		a := model.AgentPackage{ID: uuid.New(), Slug: fmt.Sprintf("%s-%d", prefix, i), DisplayName: name, Kind: "suite", OwnerID: owner.ID}
		v := model.AgentPackageVersion{ID: uuid.New(), AgentPackageID: a.ID, Version: "1.0.0", CreatedBy: owner.ID, Files: json.RawMessage("[]"), ArchiveKey: "key", ArchiveSHA256: "hash"}
		if _, _, e := repo.CommitPublish(ctx, &owner, &a, &v); e != nil {
			t.Fatal(e)
		}
		ids = append(ids, a.ID)
	}
	for _, q := range []struct {
		value string
		count int64
	}{{"100%", 1}, {"_", 1}, {"!z", 1}, {"中文", 1}, {"ALPHA", 2}, {"' OR 1=1 --", 1}, {"zzzxqnomatch", 0}} {
		out, total, e := repo.List(ctx, AgentListOptions{Q: q.value, Page: 1, PageSize: 20})
		if e != nil || total != q.count || int64(len(out)) != total {
			t.Fatalf("%s => %d %d %v", q.value, total, len(out), e)
		}
	}
	stamp := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	db.Model(&model.AgentPackage{}).Where("id IN ?", ids).Update("updated_at", stamp)
	seen := map[uuid.UUID]bool{}
	for p := 1; p <= 3; p++ {
		out, total, e := repo.List(ctx, AgentListOptions{Q: prefix, Page: p, PageSize: 1})
		if e != nil || total != 3 || len(out) != 1 || seen[out[0].ID] {
			t.Fatal(out, total, e)
		}
		seen[out[0].ID] = true
	}
	out, total, e := repo.List(ctx, AgentListOptions{Q: prefix, Page: 20, PageSize: 1})
	if e != nil || total != 3 || out == nil || len(out) != 0 {
		t.Fatal(out, total, e)
	}
}
func TestAgentQuerySemantics(t *testing.T) {
	db, e := NewDB(config.DatabaseConfig{Driver: "sqlite", URL: filepath.Join(t.TempDir(), "db.sqlite"), MaxOpenConns: 4})
	if e != nil {
		t.Fatal(e)
	}
	pool, _ := db.DB()
	t.Cleanup(func() { pool.Close() })
	testAgentQueries(t, db)
}
func TestAgentPostgres(t *testing.T) {
	dsn := os.Getenv("SKILLHUB_TEST_POSTGRES_URL")
	if dsn == "" {
		t.Skip("SKILLHUB_TEST_POSTGRES_URL is not set; PostgreSQL runtime is unverified")
	}
	db, e := NewDB(config.DatabaseConfig{Driver: "postgres", URL: dsn, MaxOpenConns: 4, MaxIdleConns: 2})
	if e != nil {
		t.Fatal(e)
	}
	pool, _ := db.DB()
	t.Cleanup(func() { pool.Close() })
	testAgentQueries(t, db)
}
