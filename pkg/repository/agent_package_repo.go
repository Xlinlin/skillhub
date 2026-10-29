package repository

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"github.com/cinience/skillhub/pkg/model"
	sqliteerr "github.com/glebarez/go-sqlite"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"net"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var (
	ErrAgentConflict    = errors.New("agent version conflict")
	ErrAgentForbidden   = errors.New("only the owner can publish; owner or admin can delete")
	ErrAgentNotFound    = errors.New("agent or version not found")
	ErrAgentUnavailable = errors.New("database temporarily unavailable")
	agentVersionPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)
)

func ParseAgentVersion(v string) ([3]int64, error) {
	var out [3]int64
	if !agentVersionPattern.MatchString(v) {
		return out, fmt.Errorf("version must be major.minor.patch without leading zeros")
	}
	for i, s := range strings.Split(v, ".") {
		n, e := strconv.ParseInt(s, 10, 32)
		if e != nil {
			return out, fmt.Errorf("version component exceeds 2147483647")
		}
		out[i] = n
	}
	return out, nil
}
func newerAgentVersion(a, b string) bool {
	x, e := ParseAgentVersion(a)
	if e != nil {
		return false
	}
	y, e := ParseAgentVersion(b)
	if e != nil {
		return false
	}
	for i := range x {
		if x[i] != y[i] {
			return x[i] > y[i]
		}
	}
	return false
}
func agentDBError(err error) error {
	if err == nil {
		return nil
	}
	var sq *sqliteerr.Error
	var pg *pgconn.PgError
	var networkError net.Error
	if errors.As(err, &networkError) || errors.Is(err, driver.ErrBadConn) || errors.Is(err, sql.ErrConnDone) {
		return ErrAgentUnavailable
	}
	if errors.As(err, &sq) {
		switch sq.Code() {
		case 1555, 2067:
			return ErrAgentConflict
		}
		switch sq.Code() & 255 {
		case 5, 6:
			return ErrAgentConflict
		case 10, 13, 14:
			return ErrAgentUnavailable
		}
	}
	if errors.As(err, &pg) {
		switch pg.Code {
		case "23505", "40001", "40P01":
			return ErrAgentConflict
		case "08000", "08003", "08006", "57P01", "53300":
			return ErrAgentUnavailable
		}
	}
	return err
}

type AgentPackageRepo struct{ db *gorm.DB }

func lockAgentWrite(tx *gorm.DB, slug string) error {
	if tx.Dialector.Name() == "sqlite" {
		// Acquire the SQLite writer lock before reading; auth usage writes can otherwise invalidate a WAL snapshot.
		return tx.Exec("UPDATE agent_packages SET versions_count = versions_count WHERE slug = ?", slug).Error
	}
	return nil
}

func agentActor(tx *gorm.DB, supplied *model.User) (*model.User, error) {
	if supplied == nil {
		return nil, ErrAgentForbidden
	}
	var actor model.User
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", supplied.ID).Take(&actor).Error
	if errors.Is(err, gorm.ErrRecordNotFound) || (err == nil && actor.IsBanned) {
		return nil, ErrAgentForbidden
	}
	return &actor, err
}

func NewAgentPackageRepo(db *gorm.DB) *AgentPackageRepo { return &AgentPackageRepo{db} }

type AgentListOptions struct {
	Q        string
	Kind     string
	Page     int
	PageSize int
}

func (r *AgentPackageRepo) get(ctx context.Context, slug string, deleted bool) (*model.AgentPackageWithOwner, error) {
	q := r.db.WithContext(ctx).Table("agent_packages a").Select("a.*, u.handle AS owner_handle").Joins("JOIN users u ON u.id = a.owner_id").Where("a.slug = ?", slug)
	if !deleted {
		q = q.Where("a.soft_deleted_at IS NULL")
	}
	var a model.AgentPackageWithOwner
	e := q.Take(&a).Error
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &a, agentDBError(e)
}
func (r *AgentPackageRepo) GetBySlug(ctx context.Context, slug string) (*model.AgentPackageWithOwner, error) {
	return r.get(ctx, slug, false)
}
func (r *AgentPackageRepo) GetBySlugIncludingDeleted(ctx context.Context, slug string) (*model.AgentPackageWithOwner, error) {
	return r.get(ctx, slug, true)
}
func (r *AgentPackageRepo) GetVersion(ctx context.Context, id uuid.UUID, version string) (*model.AgentPackageVersion, error) {
	var v model.AgentPackageVersion
	e := r.db.WithContext(ctx).Where("agent_package_id = ? AND version = ?", id, version).Take(&v).Error
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &v, agentDBError(e)
}
func (r *AgentPackageRepo) GetVersionByID(ctx context.Context, id uuid.UUID) (*model.AgentPackageVersion, error) {
	var v model.AgentPackageVersion
	e := r.db.WithContext(ctx).Where("id = ?", id).Take(&v).Error
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &v, agentDBError(e)
}
func (r *AgentPackageRepo) List(ctx context.Context, o AgentListOptions) ([]model.AgentPackageSummary, int64, error) {
	q := r.db.WithContext(ctx).Table("agent_packages a").Joins("JOIN users u ON u.id = a.owner_id").Joins("JOIN agent_package_versions v ON v.id = a.latest_version_id AND v.agent_package_id = a.id").Where("a.soft_deleted_at IS NULL")
	if o.Kind != "" {
		q = q.Where("a.kind = ?", o.Kind)
	}
	if o.Q != "" {
		s := strings.NewReplacer("!", "!!", "%", "!%", "_", "!_").Replace(strings.ToLower(o.Q))
		like := "%" + s + "%"
		q = q.Where("(LOWER(a.slug) LIKE ? ESCAPE '!' OR LOWER(a.display_name) LIKE ? ESCAPE '!' OR LOWER(a.summary) LIKE ? ESCAPE '!')", like, like, like)
	}
	var total int64
	if e := q.Count(&total).Error; e != nil {
		return nil, 0, agentDBError(e)
	}
	out := []model.AgentPackageSummary{}
	e := q.Select("a.id,a.slug,a.display_name,a.kind,a.summary,a.tags,a.owner_id,u.handle AS owner_handle,v.version AS latest_version,a.versions_count,a.created_at,a.updated_at").Order("a.updated_at DESC, a.id DESC").Offset((o.Page - 1) * o.PageSize).Limit(o.PageSize).Scan(&out).Error
	return out, total, agentDBError(e)
}
func (r *AgentPackageRepo) ListVersions(ctx context.Context, id uuid.UUID, page, pageSize int) ([]model.AgentVersionSummary, int64, error) {
	q := r.db.WithContext(ctx).Model(&model.AgentPackageVersion{}).Where("agent_package_id = ?", id)
	var total int64
	if e := q.Count(&total).Error; e != nil {
		return nil, 0, agentDBError(e)
	}
	out := []model.AgentVersionSummary{}
	e := q.Select("id,version,changelog,created_at").Order("created_at DESC, id DESC").Offset((page - 1) * pageSize).Limit(pageSize).Scan(&out).Error
	return out, total, agentDBError(e)
}

func (r *AgentPackageRepo) CommitPublish(ctx context.Context, actor *model.User, a *model.AgentPackage, v *model.AgentPackageVersion) (created bool, uncertain bool, err error) {
	if actor == nil {
		return false, false, ErrAgentForbidden
	}
	if _, e := ParseAgentVersion(v.Version); e != nil {
		return false, false, e
	}
	tx := r.db.WithContext(ctx).Begin()
	if tx.Error != nil {
		return false, false, agentDBError(tx.Error)
	}
	defer tx.Rollback()
	if e := lockAgentWrite(tx, a.Slug); e != nil {
		return false, false, agentDBError(e)
	}
	actor, e := agentActor(tx, actor)
	if e != nil {
		return false, false, agentDBError(e)
	}
	var current model.AgentPackage
	e = tx.Where("slug = ?", a.Slug).Take(&current).Error
	if errors.Is(e, gorm.ErrRecordNotFound) {
		if a.OwnerID != actor.ID {
			return false, false, ErrAgentForbidden
		}
		a.LatestVersionID = nil
		a.VersionsCount = 0
		if e = tx.Create(a).Error; e != nil {
			return false, false, agentDBError(e)
		}
		current = *a
		created = true
	} else if e != nil {
		return false, false, agentDBError(e)
	} else {
		if current.SoftDeletedAt != nil {
			return false, false, ErrAgentConflict
		}
		if current.OwnerID != actor.ID {
			return false, false, ErrAgentForbidden
		}
		if current.ID != a.ID || current.Kind != a.Kind {
			return false, false, ErrAgentConflict
		}
	}
	if v.AgentPackageID != current.ID || v.CreatedBy != actor.ID {
		return false, false, ErrAgentConflict
	}
	if current.LatestVersionID != nil {
		var previous model.AgentPackageVersion
		if e = tx.Where("id = ? AND agent_package_id = ?", *current.LatestVersionID, current.ID).Take(&previous).Error; e != nil {
			return false, false, agentDBError(e)
		}
		if !newerAgentVersion(v.Version, previous.Version) {
			return false, false, ErrAgentConflict
		}
	}
	v.CreatedAt = time.Now().UTC()
	if e = tx.Create(v).Error; e != nil {
		return false, false, agentDBError(e)
	}
	update := tx.Model(&model.AgentPackage{}).Where("id = ? AND soft_deleted_at IS NULL", current.ID)
	if current.LatestVersionID == nil {
		update = update.Where("latest_version_id IS NULL")
	} else {
		update = update.Where("latest_version_id = ?", *current.LatestVersionID)
	}
	result := update.Updates(map[string]any{"latest_version_id": v.ID, "versions_count": gorm.Expr("versions_count + 1"), "display_name": a.DisplayName, "summary": a.Summary, "tags": a.Tags, "updated_at": v.CreatedAt})
	if result.Error != nil {
		return false, false, agentDBError(result.Error)
	}
	if result.RowsAffected != 1 {
		return false, false, ErrAgentConflict
	}
	if e = tx.Commit().Error; e != nil {
		mapped := agentDBError(e)
		return false, !errors.Is(mapped, ErrAgentConflict), mapped
	}
	return created, false, nil
}
func (r *AgentPackageRepo) SoftDelete(ctx context.Context, actor *model.User, slug string) error {
	if actor == nil {
		return ErrAgentForbidden
	}
	return agentDBError(r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := lockAgentWrite(tx, slug); e != nil {
			return e
		}
		actor, e := agentActor(tx, actor)
		if e != nil {
			return e
		}
		var a model.AgentPackage
		e = tx.Where("slug = ? AND soft_deleted_at IS NULL", slug).Take(&a).Error
		if errors.Is(e, gorm.ErrRecordNotFound) {
			return ErrAgentNotFound
		}
		if e != nil {
			return e
		}
		if actor.ID != a.OwnerID && !actor.IsAdmin() {
			return ErrAgentForbidden
		}
		now := time.Now().UTC()
		result := tx.Model(&model.AgentPackage{}).Where("id = ? AND soft_deleted_at IS NULL", a.ID).Updates(map[string]any{"soft_deleted_at": now, "updated_at": now})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrAgentConflict
		}
		return nil
	}))
}
