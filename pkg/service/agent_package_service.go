package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/cinience/skillhub/pkg/agentarchive"
	"github.com/cinience/skillhub/pkg/model"
	"github.com/cinience/skillhub/pkg/repository"
	"github.com/google/uuid"
	"io"
	"log"
	"math"
	"os"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

type AgentError struct {
	Code    string
	Message string
}

func (e *AgentError) Error() string         { return e.Message }
func agentError(code, message string) error { return &AgentError{code, message} }
func agentFailure(err error) error {
	if err == nil {
		return nil
	}
	var ae *AgentError
	if errors.As(err, &ae) {
		return err
	}
	switch {
	case errors.Is(err, repository.ErrAgentForbidden):
		return agentError("forbidden", "Only the owner can publish; the owner or an administrator can delete")
	case errors.Is(err, repository.ErrAgentConflict):
		return agentError("conflict", "Package changed, was deleted, or version is not newer; refresh before retrying")
	case errors.Is(err, repository.ErrAgentNotFound):
		return agentError("not_found", "Agent or version not found")
	case errors.Is(err, repository.ErrAgentUnavailable):
		return agentError("unavailable", "Database temporarily unavailable")
	case errors.Is(err, agentarchive.ErrTooLarge):
		return agentError("too_large", "Upload exceeds the size or file count limits")
	case errors.Is(err, agentarchive.ErrInvalid):
		return agentError("validation", err.Error())
	case errors.Is(err, agentarchive.ErrUnsupportedPreview):
		return agentError("unsupported_preview", "This file cannot be previewed as UTF-8 text")
	}
	log.Printf("agent operation failed: %v", err)
	return agentError("internal_error", "Unable to complete the agent operation")
}

type AgentPublishRequest struct {
	Slug, DisplayName, Kind, Version, Summary, Instructions, Changelog string
	Tags                                                               []string
	Mode                                                               string
	Files                                                              []agentarchive.InputFile
	Archive                                                            io.Reader
}
type AgentDetail struct {
	Agent   model.AgentPackageSummary  `json:"agent"`
	Version *model.AgentPackageVersion `json:"version"`
}
type AgentPublishResult struct {
	AgentDetail
	Created bool `json:"-"`
}
type AgentPage[T any] struct {
	Data     []T   `json:"data"`
	Page     int   `json:"page"`
	PageSize int   `json:"pageSize"`
	Total    int64 `json:"total"`
}
type AgentPackageService struct {
	repo  *repository.AgentPackageRepo
	store *agentarchive.Store
}

func NewAgentPackageService(repo *repository.AgentPackageRepo, store *agentarchive.Store) *AgentPackageService {
	return &AgentPackageService{repo, store}
}

var agentSlugPattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,126}[a-z0-9])?$`)

func validateAgentRequest(r AgentPublishRequest) error {
	if !agentSlugPattern.MatchString(r.Slug) || r.Slug == "publish" {
		return agentError("validation", "Slug must contain 1–128 lowercase letters, digits or hyphens, with letters/digits at both ends; publish is reserved")
	}
	if _, e := repository.ParseAgentVersion(r.Version); e != nil {
		return agentError("validation", e.Error())
	}
	if r.Kind != "single" && r.Kind != "suite" {
		return agentError("validation", "Kind must be single or suite")
	}
	if strings.TrimSpace(r.DisplayName) == "" || utf8.RuneCountInString(r.DisplayName) > 256 || utf8.RuneCountInString(r.Summary) > 2000 || len(r.Instructions) > 64<<10 || len(r.Changelog) > 16<<10 || len(r.Tags) > 20 {
		return agentError("validation", "Metadata exceeds the allowed limits")
	}
	for _, s := range append([]string{r.DisplayName, r.Summary, r.Instructions, r.Changelog}, r.Tags...) {
		if !utf8.ValidString(s) {
			return agentError("validation", "Metadata must be valid UTF-8")
		}
	}
	for _, tag := range r.Tags {
		if tag == "" || utf8.RuneCountInString(tag) > 64 {
			return agentError("validation", "Tags must contain 1–64 characters")
		}
	}
	if r.Mode == "archive" {
		if r.Archive == nil || len(r.Files) != 0 {
			return agentError("validation", "Archive mode requires one ZIP and no files")
		}
	} else if r.Mode == "files" {
		if len(r.Files) == 0 || r.Archive != nil {
			return agentError("validation", "Files mode requires files and no ZIP")
		}
	} else {
		return agentError("validation", "Upload mode must be files or archive")
	}
	return nil
}
func (s *AgentPackageService) Publish(ctx context.Context, user *model.User, req AgentPublishRequest) (*AgentPublishResult, error) {
	if user == nil {
		return nil, agentError("unauthorized", "Authentication required")
	}
	if e := validateAgentRequest(req); e != nil {
		return nil, e
	}
	existing, e := s.repo.GetBySlugIncludingDeleted(ctx, req.Slug)
	if e != nil {
		return nil, agentFailure(e)
	}
	id := uuid.New()
	if existing != nil {
		if existing.SoftDeletedAt != nil {
			return nil, agentFailure(repository.ErrAgentConflict)
		}
		if existing.OwnerID != user.ID {
			return nil, agentFailure(repository.ErrAgentForbidden)
		}
		if existing.Kind != req.Kind {
			return nil, agentFailure(repository.ErrAgentConflict)
		}
		id = existing.ID
	}
	versionID := uuid.New()
	key := id.String() + "/" + versionID.String() + ".zip"
	var archive *agentarchive.Archive
	if req.Mode == "archive" {
		archive, e = s.store.SaveZIP(ctx, key, req.Archive)
	} else {
		archive, e = s.store.SaveFiles(ctx, key, req.Files)
	}
	if e != nil {
		if errors.Is(e, agentarchive.ErrInvalid) || errors.Is(e, agentarchive.ErrTooLarge) {
			return nil, agentFailure(e)
		}
		log.Printf("agent archive save: %v", e)
		return nil, agentError("unavailable", "Archive storage unavailable")
	}
	manifest, e := json.Marshal(archive.Files)
	if e != nil {
		s.cleanup(key)
		return nil, agentFailure(e)
	}
	tags := model.StringArray(req.Tags)
	if tags == nil {
		tags = model.StringArray{}
	}
	a := &model.AgentPackage{ID: id, Slug: req.Slug, DisplayName: req.DisplayName, Kind: req.Kind, Summary: req.Summary, Tags: tags, OwnerID: user.ID}
	v := &model.AgentPackageVersion{ID: versionID, AgentPackageID: id, Version: req.Version, Instructions: req.Instructions, Changelog: req.Changelog, ArchiveKey: key, ArchiveSHA256: archive.SHA256, ArchiveSize: archive.Size, Files: manifest, CreatedBy: user.ID}
	created, uncertain, e := s.repo.CommitPublish(ctx, user, a, v)
	if e != nil {
		if !uncertain {
			s.cleanup(key)
			return nil, agentFailure(e)
		}
		checkCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		found, checkErr := s.repo.GetVersionByID(checkCtx, versionID)
		if checkErr != nil || found == nil {
			log.Printf("agent publish outcome unknown version=%s", versionID)
			return nil, agentError("publish_outcome_unknown", "Publish result is unknown. Check this version before retrying; the archive has been retained")
		}
		v = found
		created = existing == nil
	}
	// A response read failure after commit must never clean the committed archive.
	detail, e := s.Get(ctx, req.Slug, v.Version)
	if e != nil {
		return nil, e
	}
	return &AgentPublishResult{*detail, created}, nil
}
func (s *AgentPackageService) cleanup(key string) {
	if e := s.store.Remove(key); e != nil && !os.IsNotExist(e) {
		log.Printf("agent orphan cleanup key=%s: %v", key, e)
	}
}
func (s *AgentPackageService) ResolvePackageVersion(ctx context.Context, slug, version string) (*model.AgentPackageWithOwner, *model.AgentPackageVersion, error) {
	a, e := s.repo.GetBySlug(ctx, slug)
	if e != nil {
		return nil, nil, agentFailure(e)
	}
	if a == nil {
		return nil, nil, agentFailure(repository.ErrAgentNotFound)
	}
	var v *model.AgentPackageVersion
	if version == "" || version == "latest" {
		if a.LatestVersionID == nil {
			return nil, nil, agentFailure(repository.ErrAgentNotFound)
		}
		v, e = s.repo.GetVersionByID(ctx, *a.LatestVersionID)
	} else {
		if _, err := repository.ParseAgentVersion(version); err != nil {
			return nil, nil, agentError("validation", err.Error())
		}
		v, e = s.repo.GetVersion(ctx, a.ID, version)
	}
	if e != nil {
		return nil, nil, agentFailure(e)
	}
	if v == nil || v.AgentPackageID != a.ID {
		return nil, nil, agentFailure(repository.ErrAgentNotFound)
	}
	return a, v, nil
}
func (s *AgentPackageService) Get(ctx context.Context, slug, version string) (*AgentDetail, error) {
	a, v, e := s.ResolvePackageVersion(ctx, slug, version)
	if e != nil {
		return nil, e
	}
	latest := v.Version
	if a.LatestVersionID != nil && *a.LatestVersionID != v.ID {
		lv, e := s.repo.GetVersionByID(ctx, *a.LatestVersionID)
		if e != nil {
			return nil, agentFailure(e)
		}
		if lv == nil || lv.AgentPackageID != a.ID {
			return nil, agentFailure(fmt.Errorf("invalid latest version"))
		}
		latest = lv.Version
	}
	return &AgentDetail{a.ToSummary(latest), v}, nil
}
func ValidateAgentPage(page, size int) error {
	if page < 1 || size < 1 || size > 100 || int64(page-1) > math.MaxInt64/int64(size) {
		return agentError("validation", "Invalid page or pageSize")
	}
	return nil
}
func (s *AgentPackageService) List(ctx context.Context, o repository.AgentListOptions) (*AgentPage[model.AgentPackageSummary], error) {
	if e := ValidateAgentPage(o.Page, o.PageSize); e != nil {
		return nil, e
	}
	if !utf8.ValidString(o.Q) || utf8.RuneCountInString(o.Q) > 128 || (o.Kind != "" && o.Kind != "single" && o.Kind != "suite") {
		return nil, agentError("validation", "Invalid query or kind")
	}
	data, total, e := s.repo.List(ctx, o)
	if e != nil {
		return nil, agentFailure(e)
	}
	return &AgentPage[model.AgentPackageSummary]{data, o.Page, o.PageSize, total}, nil
}
func (s *AgentPackageService) Versions(ctx context.Context, slug string, page, size int) (*AgentPage[model.AgentVersionSummary], error) {
	if e := ValidateAgentPage(page, size); e != nil {
		return nil, e
	}
	a, _, e := s.ResolvePackageVersion(ctx, slug, "latest")
	if e != nil {
		return nil, e
	}
	data, total, e := s.repo.ListVersions(ctx, a.ID, page, size)
	if e != nil {
		return nil, agentFailure(e)
	}
	return &AgentPage[model.AgentVersionSummary]{data, page, size, total}, nil
}
func (s *AgentPackageService) ReadText(ctx context.Context, slug, version, path string) ([]byte, error) {
	_, v, e := s.ResolvePackageVersion(ctx, slug, version)
	if e != nil {
		return nil, e
	}
	var files []agentarchive.Entry
	if e = json.Unmarshal(v.Files, &files); e != nil {
		return nil, agentFailure(e)
	}
	member := false
	for _, f := range files {
		if f.Path == path {
			member = true
			break
		}
	}
	if !member {
		return nil, agentFailure(repository.ErrAgentNotFound)
	}
	b, e := s.store.ReadText(ctx, v.ArchiveKey, path)
	if errors.Is(e, agentarchive.ErrUnsupportedPreview) {
		return nil, agentFailure(e)
	}
	if e != nil {
		log.Printf("agent preview read: %v", e)
		return nil, agentError("unavailable", "Archive storage unavailable")
	}
	return b, nil
}
func (s *AgentPackageService) Download(ctx context.Context, slug, version string) (*os.File, *model.AgentPackageVersion, error) {
	_, v, e := s.ResolvePackageVersion(ctx, slug, version)
	if e != nil {
		return nil, nil, e
	}
	f, e := s.store.Open(v.ArchiveKey)
	if e != nil {
		log.Printf("agent download open: %v", e)
		return nil, nil, agentError("unavailable", "Archive storage unavailable")
	}
	info, e := f.Stat()
	if e != nil || info.Size() != v.ArchiveSize {
		f.Close()
		return nil, nil, agentError("unavailable", "Archive storage unavailable")
	}
	hash := sha256.New()
	if _, e = io.Copy(hash, f); e != nil || hex.EncodeToString(hash.Sum(nil)) != v.ArchiveSHA256 {
		f.Close()
		return nil, nil, agentError("unavailable", "Archive storage unavailable")
	}
	if _, e = f.Seek(0, io.SeekStart); e != nil {
		f.Close()
		return nil, nil, agentError("unavailable", "Archive storage unavailable")
	}
	return f, v, nil
}
func (s *AgentPackageService) Delete(ctx context.Context, user *model.User, slug string) error {
	return agentFailure(s.repo.SoftDelete(ctx, user, slug))
}
