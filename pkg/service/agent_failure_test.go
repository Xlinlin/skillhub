package service

import (
	"context"
	"database/sql"
	"errors"
	"github.com/cinience/skillhub/pkg/agentarchive"
	"github.com/cinience/skillhub/pkg/config"
	"github.com/cinience/skillhub/pkg/model"
	"github.com/cinience/skillhub/pkg/repository"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

type ackLossPool struct {
	gorm.ConnPool
	commit      bool
	failedReads *atomic.Bool
	afterCommit func()
}

func (p *ackLossPool) BeginTx(ctx context.Context, opts *sql.TxOptions) (gorm.ConnPool, error) {
	tx, e := p.ConnPool.(*sql.DB).BeginTx(ctx, opts)
	if e != nil {
		return nil, e
	}
	return &ackLossTx{Tx: tx, commit: p.commit, afterCommit: p.afterCommit}, nil
}

func (p *ackLossPool) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	if p.failedReads != nil && p.failedReads.Load() {
		return nil, errors.New("injected database outage")
	}
	return p.ConnPool.QueryContext(ctx, query, args...)
}

type ackLossTx struct {
	*sql.Tx
	commit      bool
	afterCommit func()
}

func (t *ackLossTx) Commit() error {
	if t.commit {
		if e := t.Tx.Commit(); e != nil {
			return e
		}
	} else {
		t.Tx.Rollback()
	}
	if t.afterCommit != nil {
		t.afterCommit()
	}
	return errors.New("simulated lost commit acknowledgement")
}

func agentFailureFixture(t *testing.T) (*gorm.DB, *agentarchive.Store, *model.User, string) {
	t.Helper()
	root := t.TempDir()
	db, e := repository.NewDB(config.DatabaseConfig{Driver: "sqlite", URL: filepath.Join(root, "db.sqlite"), MaxOpenConns: 4})
	if e != nil {
		t.Fatal(e)
	}
	pool, _ := db.DB()
	t.Cleanup(func() { pool.Close() })
	storage := filepath.Join(root, "agents")
	store, e := agentarchive.New(storage)
	if e != nil {
		t.Fatal(e)
	}
	u := &model.User{ID: uuid.New(), Handle: "owner", Role: "user"}
	if e = db.Create(u).Error; e != nil {
		t.Fatal(e)
	}
	return db, store, u, storage
}
func failureRequest() AgentPublishRequest {
	return AgentPublishRequest{Slug: "sample", DisplayName: "Sample", Kind: "suite", Version: "1.0.0", Mode: "files", Files: []agentarchive.InputFile{{Path: "agent.md", Open: func() (io.ReadCloser, error) { return io.NopCloser(strings.NewReader("content")), nil }}}}
}
func TestAgentCommitAcknowledgementLoss(t *testing.T) {
	for _, committed := range []bool{true, false} {
		t.Run(map[bool]string{true: "committed", false: "not-found"}[committed], func(t *testing.T) {
			db, store, u, root := agentFailureFixture(t)
			pool, _ := db.DB()
			isolated := db.Session(&gorm.Session{NewDB: true, Context: context.Background()})
			isolated.Statement.ConnPool = &ackLossPool{ConnPool: pool, commit: committed}
			svc := NewAgentPackageService(repository.NewAgentPackageRepo(isolated), store)
			out, e := svc.Publish(context.Background(), u, failureRequest())
			if committed {
				if e != nil || out == nil {
					t.Fatal(out, e)
				}
			} else {
				var ae *AgentError
				if !errors.As(e, &ae) || ae.Code != "publish_outcome_unknown" {
					t.Fatal(out, e)
				}
			}
			matches, e := filepath.Glob(filepath.Join(root, "*", "*.zip"))
			if e != nil || len(matches) != 1 {
				t.Fatal("uncertain archive was deleted", matches, e)
			}
		})
	}
}

func TestAgentCommitOutageAndCanceledCaller(t *testing.T) {
	for _, outage := range []bool{true, false} {
		t.Run(map[bool]string{true: "outage", false: "caller-canceled"}[outage], func(t *testing.T) {
			db, store, u, root := agentFailureFixture(t)
			pool, _ := db.DB()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var failedReads atomic.Bool
			wrapped := &ackLossPool{ConnPool: pool, commit: true, failedReads: &failedReads}
			if outage {
				wrapped.afterCommit = func() { failedReads.Store(true) }
			} else {
				wrapped.afterCommit = cancel
			}
			isolated := db.Session(&gorm.Session{NewDB: true, Context: context.Background()})
			isolated.Statement.ConnPool = wrapped
			svc := NewAgentPackageService(repository.NewAgentPackageRepo(isolated), store)
			_, e := svc.Publish(ctx, u, failureRequest())
			if e == nil {
				t.Fatal("expected response failure")
			}
			if outage {
				var ae *AgentError
				if !errors.As(e, &ae) || ae.Code != "publish_outcome_unknown" {
					t.Fatal(e)
				}
			}
			matches, _ := filepath.Glob(filepath.Join(root, "*", "*.zip"))
			var count int64
			if e = db.Model(&model.AgentPackageVersion{}).Count(&count).Error; e != nil || count != 1 || len(matches) != 1 {
				t.Fatal("committed data was removed", count, matches, e)
			}
		})
	}
}
func TestAgentCorruptArchiveUnavailable(t *testing.T) {
	db, store, u, _ := agentFailureFixture(t)
	svc := NewAgentPackageService(repository.NewAgentPackageRepo(db), store)
	out, e := svc.Publish(context.Background(), u, failureRequest())
	if e != nil {
		t.Fatal(e)
	}
	f, e := store.Open(out.Version.ArchiveKey)
	if e != nil {
		t.Fatal(e)
	}
	path := f.Name()
	f.Close()
	b, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	b[35] ^= 1
	if e = os.WriteFile(path, b, 0600); e != nil {
		t.Fatal(e)
	}
	download, _, e := svc.Download(context.Background(), "sample", "1.0.0")
	if download != nil {
		download.Close()
	}
	var ae *AgentError
	if !errors.As(e, &ae) || ae.Code != "unavailable" {
		t.Fatal("corruption was downloadable", e)
	}
}

type agentCloseFailure struct{ io.Reader }

func (agentCloseFailure) Close() error { return errors.New("injected reader close failure") }
func TestAgentFailedPublishLeavesNoAsset(t *testing.T) {
	for _, failure := range []string{"open", "close", "database-rollback", "storage"} {
		t.Run(failure, func(t *testing.T) {
			db, store, u, storage := agentFailureFixture(t)
			svc := NewAgentPackageService(repository.NewAgentPackageRepo(db), store)
			req := failureRequest()
			switch failure {
			case "open":
				req.Files[0].Open = func() (io.ReadCloser, error) { return nil, errors.New("injected open failure") }
			case "close":
				req.Files[0].Open = func() (io.ReadCloser, error) { return agentCloseFailure{strings.NewReader("content")}, nil }
			case "database-rollback":
				if e := db.Callback().Create().Before("gorm:create").Register("agent_test_reject_version", func(tx *gorm.DB) {
					if tx.Statement.Table == "agent_package_versions" {
						tx.AddError(errors.New("injected insert failure"))
					}
				}); e != nil {
					t.Fatal(e)
				}
			case "storage":
				if e := os.Remove(storage); e != nil {
					t.Fatal(e)
				}
				if e := os.WriteFile(storage, []byte("not a directory"), 0600); e != nil {
					t.Fatal(e)
				}
			}
			if _, e := svc.Publish(context.Background(), u, req); e == nil {
				t.Fatal("failure was accepted")
			}
			for _, m := range []any{&model.AgentPackage{}, &model.AgentPackageVersion{}} {
				var count int64
				if e := db.Model(m).Count(&count).Error; e != nil || count != 0 {
					t.Fatal("failed publication left visible rows", count, e)
				}
			}
			files, _ := filepath.Glob(filepath.Join(storage, "*", "*.zip"))
			if len(files) != 0 {
				t.Fatal("failed publication left candidates", files)
			}
		})
	}
}
