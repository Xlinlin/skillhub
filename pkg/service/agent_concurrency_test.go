package service

import (
	"context"
	"errors"
	"github.com/cinience/skillhub/pkg/model"
	"github.com/cinience/skillhub/pkg/repository"
	"github.com/google/uuid"
	"io"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestAgentConcurrentSameVersion(t *testing.T) {
	db, store, u, root := agentFailureFixture(t)
	svc := NewAgentPackageService(repository.NewAgentPackageRepo(db), store)
	if _, e := svc.Publish(context.Background(), u, failureRequest()); e != nil {
		t.Fatal(e)
	}
	var successes atomic.Int32
	var wg sync.WaitGroup
	start := make(chan struct{})
	errs := make(chan error, 8)
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			req := failureRequest()
			req.Version = "1.1.0"
			_, e := svc.Publish(context.Background(), u, req)
			if e == nil {
				successes.Add(1)
			} else {
				var ae *AgentError
				if !errors.As(e, &ae) || ae.Code != "conflict" {
					errs <- e
				}
			}
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	for e := range errs {
		t.Error(e)
	}
	if successes.Load() != 1 {
		t.Fatal(successes.Load())
	}
	a, e := svc.Get(context.Background(), "sample", "latest")
	if e != nil || a.Agent.VersionsCount != 2 || a.Version.Version != "1.1.0" {
		t.Fatal(a, e)
	}
	files, e := filepath.Glob(filepath.Join(root, "*", "*.zip"))
	if e != nil || len(files) != 2 {
		t.Fatal("conflicting archives leaked", files, e)
	}
}

func holdAgentPublish(t *testing.T, svc *AgentPackageService, u *model.User, version string) (func(), <-chan error) {
	t.Helper()
	ready := make(chan struct{})
	resume := make(chan struct{})
	result := make(chan error, 1)
	var once sync.Once
	release := func() { once.Do(func() { close(resume) }) }
	t.Cleanup(release)
	req := failureRequest()
	req.Version = version
	open := req.Files[0].Open
	req.Files[0].Open = func() (io.ReadCloser, error) { close(ready); <-resume; return open() }
	go func() { _, e := svc.Publish(context.Background(), u, req); result <- e }()
	select {
	case <-ready:
	case e := <-result:
		t.Fatalf("publish failed before barrier: %v", e)
	case <-time.After(5 * time.Second):
		t.Fatal("archive barrier timed out")
	}
	return release, result
}
func TestAgentDifferentVersionCommitOrder(t *testing.T) {
	for _, highFirst := range []bool{true, false} {
		t.Run(map[bool]string{true: "higher-first", false: "lower-first"}[highFirst], func(t *testing.T) {
			db, store, u, root := agentFailureFixture(t)
			svc := NewAgentPackageService(repository.NewAgentPackageRepo(db), store)
			if _, e := svc.Publish(context.Background(), u, failureRequest()); e != nil {
				t.Fatal(e)
			}
			held, next := "1.1.0", "1.2.0"
			if !highFirst {
				held, next = next, held
			}
			release, result := holdAgentPublish(t, svc, u, held)
			req := failureRequest()
			req.Version = next
			if _, e := svc.Publish(context.Background(), u, req); e != nil {
				t.Fatal(e)
			}
			release()
			e := <-result
			count := 3
			if highFirst {
				var ae *AgentError
				if !errors.As(e, &ae) || ae.Code != "conflict" {
					t.Fatal(e)
				}
				count = 2
			} else if e != nil {
				t.Fatal(e)
			}
			detail, e := svc.Get(context.Background(), "sample", "latest")
			if e != nil || detail.Version.Version != "1.2.0" || detail.Agent.VersionsCount != count {
				t.Fatal(detail, e)
			}
			files, _ := filepath.Glob(filepath.Join(root, "*", "*.zip"))
			if len(files) != count {
				t.Fatal(files)
			}
		})
	}
}
func TestAgentDeleteDuringUpload(t *testing.T) {
	db, store, u, root := agentFailureFixture(t)
	svc := NewAgentPackageService(repository.NewAgentPackageRepo(db), store)
	if _, e := svc.Publish(context.Background(), u, failureRequest()); e != nil {
		t.Fatal(e)
	}
	release, result := holdAgentPublish(t, svc, u, "1.1.0")
	if e := svc.Delete(context.Background(), u, "sample"); e != nil {
		t.Fatal(e)
	}
	release()
	var ae *AgentError
	if e := <-result; !errors.As(e, &ae) || ae.Code != "conflict" {
		t.Fatal(e)
	}
	if _, _, e := svc.ResolvePackageVersion(context.Background(), "sample", "1.0.0"); e == nil {
		t.Fatal("deleted version became visible")
	}
	files, _ := filepath.Glob(filepath.Join(root, "*", "*.zip"))
	if len(files) != 1 {
		t.Fatal(files)
	}
}
func TestAgentRevalidatesIdentity(t *testing.T) {
	db, store, u, root := agentFailureFixture(t)
	svc := NewAgentPackageService(repository.NewAgentPackageRepo(db), store)
	if _, e := svc.Publish(context.Background(), u, failureRequest()); e != nil {
		t.Fatal(e)
	}
	forged := &model.User{ID: uuid.New(), Handle: "other", Role: "user"}
	db.Create(forged)
	forged.Role = "admin"
	if e := svc.Delete(context.Background(), forged, "sample"); e == nil {
		t.Fatal("trusted stale admin role")
	}
	if e := db.Model(&model.User{}).Where("id = ?", u.ID).Update("is_banned", true).Error; e != nil {
		t.Fatal(e)
	}
	req := failureRequest()
	req.Version = "1.1.0"
	if _, e := svc.Publish(context.Background(), u, req); e == nil {
		t.Fatal("banned owner published")
	}
	files, _ := filepath.Glob(filepath.Join(root, "*", "*.zip"))
	if len(files) != 1 {
		t.Fatal(files)
	}
}
