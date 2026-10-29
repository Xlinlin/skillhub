package repository

import (
	"encoding/json"
	"errors"
	"github.com/cinience/skillhub/pkg/config"
	"github.com/cinience/skillhub/pkg/model"
	"github.com/google/uuid"
	"path/filepath"
	"testing"
)

func TestAgentMigrationPreservesSkillsAndEnforcesIndexes(t *testing.T) {
	cfg := config.DatabaseConfig{Driver: "sqlite", URL: filepath.Join(t.TempDir(), "db.sqlite") + "?cache=private", MaxOpenConns: 4}
	db, e := NewDB(cfg)
	if e != nil {
		t.Fatal(e)
	}
	pool, _ := db.DB()
	t.Cleanup(func() { pool.Close() })
	u := model.User{ID: uuid.New(), Handle: "existing-owner", Role: "user"}
	if e = db.Create(&u).Error; e != nil {
		t.Fatal(e)
	}
	skill := model.Skill{ID: uuid.New(), Slug: "existing-skill", OwnerID: u.ID, Tags: model.StringArray{"preserve"}}
	if e = db.Create(&skill).Error; e != nil {
		t.Fatal(e)
	}
	again, e := NewDB(cfg)
	if e != nil {
		t.Fatal(e)
	}
	otherPool, _ := again.DB()
	t.Cleanup(func() { otherPool.Close() })
	var got model.Skill
	if e = again.Where("id = ?", skill.ID).Take(&got).Error; e != nil || got.OwnerID != u.ID || got.Slug != "existing-skill" || len(got.Tags) != 1 || got.Tags[0] != "preserve" {
		t.Fatal(got, e)
	}
	a := model.AgentPackage{ID: uuid.New(), Slug: "unique", DisplayName: "Unique", Kind: "suite", OwnerID: u.ID}
	if e = again.Create(&a).Error; e != nil {
		t.Fatal(e)
	}
	duplicate := a
	duplicate.ID = uuid.New()
	if e = again.Create(&duplicate).Error; !errors.Is(agentDBError(e), ErrAgentConflict) {
		t.Fatal("slug unique index not enforced", e)
	}
	v := model.AgentPackageVersion{ID: uuid.New(), AgentPackageID: a.ID, Version: "1.0.0", Files: json.RawMessage("[]"), ArchiveKey: "key", ArchiveSHA256: "hash", CreatedBy: u.ID}
	if e = again.Create(&v).Error; e != nil {
		t.Fatal(e)
	}
	v.ID = uuid.New()
	if e = again.Create(&v).Error; !errors.Is(agentDBError(e), ErrAgentConflict) {
		t.Fatal("version unique index not enforced", e)
	}
}
