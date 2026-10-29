package cli

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPublishOptions(t *testing.T) {
	for _, args := range [][]string{{"./suite", "--type", "unknown"}, {"./suite", "--type"}, {"./suite", "--typo", "agent"}, {"./suite", "--kind", "suite"}, {"./suite", "extra"}} {
		if _, e := parsePublishOptions(args); e == nil {
			t.Fatalf("accepted invalid flags: %v", args)
		}
	}
	old, e := parsePublishOptions([]string{"./skill", "--slug", "legacy", "--version", "1.0.0", "--summary", "text"})
	if e != nil || old.Type != "skill" || old.Slug != "legacy" || old.Summary != "text" {
		t.Fatal(old, e)
	}
	agent, e := parsePublishOptions([]string{"./suite.zip", "--type=agent", "--slug", "suite", "--version", "1.0.0", "--kind", "suite", "--name", "Suite"})
	if e != nil || agent.Type != "agent" || agent.Name != "Suite" {
		t.Fatal(agent, e)
	}
}
func agentOptions(path string) AgentPublishOptions {
	return AgentPublishOptions{Path: path, Slug: "suite", Name: "Suite", Kind: "suite", Version: "1.0.0", Tags: "one,two"}
}
func writeCLIFile(t *testing.T, root, path string, data []byte) {
	t.Helper()
	target := filepath.Join(root, filepath.FromSlash(path))
	if e := os.MkdirAll(filepath.Dir(target), 0700); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(target, data, 0600); e != nil {
		t.Fatal(e)
	}
}
func TestPublishAgentDirectory(t *testing.T) {
	root := t.TempDir()
	writeCLIFile(t, root, ".agents/same.md", []byte("产品\n"))
	writeCLIFile(t, root, ".claude/same.md", []byte{0, 1, 255})
	instructions := filepath.Join(t.TempDir(), "USAGE.md")
	os.WriteFile(instructions, []byte("Run locally"), 0600)
	for _, status := range []int{200, 201} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/api/v1/agents" || r.Header.Get("Authorization") != "Bearer unit-test-token" {
					t.Error("wrong endpoint/auth")
				}
				if e := r.ParseMultipartForm(1 << 20); e != nil {
					t.Error(e)
					w.WriteHeader(400)
					return
				}
				defer r.MultipartForm.RemoveAll()
				if r.FormValue("mode") != "files" || r.FormValue("paths") != "[\".agents/same.md\",\".claude/same.md\"]" || r.FormValue("tags") != "[\"one\",\"two\"]" || r.FormValue("instructions") != "Run locally" || r.FormValue("displayName") != "Suite" {
					t.Error("incorrect metadata", r.MultipartForm.Value)
				}
				parts := r.MultipartForm.File["files"]
				if len(parts) != 2 {
					t.Error("hidden files lost")
					w.WriteHeader(400)
					return
				}
				for i, want := range [][]byte{[]byte("产品\n"), {0, 1, 255}} {
					f, e := parts[i].Open()
					if e != nil {
						t.Error(e)
						return
					}
					body, e := io.ReadAll(f)
					f.Close()
					if e != nil || !bytes.Equal(body, want) {
						t.Error("file bytes changed", e)
					}
				}
				w.WriteHeader(status)
				io.WriteString(w, `{"agent":{"slug":"suite"},"version":{"version":"1.0.0"}}`)
			}))
			defer server.Close()
			client := NewClient(&CLIConfig{Registry: server.URL, Token: "unit-test-token"})
			opts := agentOptions(root)
			opts.InstructionsFile = instructions
			result, e := client.PublishAgent(opts)
			if e != nil || result["agent"] == nil {
				t.Fatal(result, e)
			}
		})
	}
}
func TestPublishAgentZIPAndErrors(t *testing.T) {
	root := t.TempDir()
	data := []byte("opaque ZIP bytes: server validates the ZIP")
	path := filepath.Join(root, "suite.zip")
	os.WriteFile(path, data, 0600)
	calls := 0
	status := 201
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if status != 201 {
			w.WriteHeader(status)
			io.WriteString(w, `{"error":"check version before retry","code":"publish_outcome_unknown"}`)
			return
		}
		if e := r.ParseMultipartForm(1 << 20); e != nil {
			t.Error(e)
			return
		}
		defer r.MultipartForm.RemoveAll()
		parts := r.MultipartForm.File["archive"]
		if r.FormValue("mode") != "archive" || len(parts) != 1 || len(r.MultipartForm.File["files"]) != 0 {
			t.Error("wrong ZIP mode")
			return
		}
		f, _ := parts[0].Open()
		body, _ := io.ReadAll(f)
		f.Close()
		if !bytes.Equal(body, data) {
			t.Error("ZIP bytes changed")
		}
		w.WriteHeader(status)
		io.WriteString(w, `{"agent":{"slug":"suite"},"version":{"version":"1.0.0"}}`)
	}))
	defer server.Close()
	client := NewClient(&CLIConfig{Registry: server.URL, Token: "unit-test-token"})
	if _, e := client.PublishAgent(agentOptions(path)); e != nil {
		t.Fatal(e)
	}
	for _, code := range []int{401, 403, 409, 413, 503} {
		status = code
		before := calls
		_, e := client.PublishAgent(agentOptions(path))
		var api *AgentPublishError
		if !errors.As(e, &api) || api.StatusCode != code || api.Code != "publish_outcome_unknown" || calls != before+1 {
			t.Fatal("error/automatic retry", e, calls)
		}
	}
}
func TestPublishAgentRejectsUnsafeInputBeforeHTTP(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(500) }))
	defer server.Close()
	client := NewClient(&CLIConfig{Registry: server.URL, Token: "unit-test-token"})
	for _, path := range []string{".git/config", "nested/.GiT/file"} {
		root := t.TempDir()
		writeCLIFile(t, root, path, []byte("secret"))
		if _, e := client.PublishAgent(agentOptions(root)); e == nil {
			t.Fatal("accepted .git")
		}
	}
	root := t.TempDir()
	writeCLIFile(t, root, "large", nil)
	f, _ := os.OpenFile(filepath.Join(root, "large"), os.O_WRONLY, 0600)
	f.Truncate((10 << 20) + 1)
	f.Close()
	if _, e := client.PublishAgent(agentOptions(root)); e == nil {
		t.Fatal("accepted oversized file")
	}
	empty := t.TempDir()
	if _, e := client.PublishAgent(agentOptions(empty)); e == nil {
		t.Fatal("accepted empty directory")
	}
	if calls != 0 {
		t.Fatal("invalid inputs reached server", calls)
	}
}
func TestPublishAgentRejectsSymlinks(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(t.TempDir(), "outside")
	os.WriteFile(target, []byte("outside"), 0600)
	if e := os.Symlink(target, filepath.Join(root, "link")); e != nil {
		t.Skip("symlink creation unavailable", e)
	}
	if _, e := collectAgentFiles(root); e == nil {
		t.Fatal("accepted symlink")
	}
}
func TestPublishAgentDoesNotFollowRedirect(t *testing.T) {
	root := t.TempDir()
	writeCLIFile(t, root, "agent.md", []byte("agent"))
	forwarded := 0
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { forwarded++; io.Copy(io.Discard, r.Body) }))
	defer target.Close()
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, 307) }))
	defer origin.Close()
	client := NewClient(&CLIConfig{Registry: origin.URL, Token: "unit-test-token"})
	if _, e := client.PublishAgent(agentOptions(root)); e == nil || forwarded != 0 {
		t.Fatal("forwarded upload", e, forwarded)
	}
}
func TestPublishAgentMetadataAndCleanup(t *testing.T) {
	root := t.TempDir()
	writeCLIFile(t, root, "agent.md", []byte("agent"))
	tmp := t.TempDir()
	t.Setenv("TMP", tmp)
	t.Setenv("TEMP", tmp)
	t.Setenv("TMPDIR", tmp)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		io.WriteString(w, `{"error":"denied"}`)
	}))
	defer server.Close()
	client := NewClient(&CLIConfig{Registry: server.URL, Token: "unit-test-token"})
	for range 3 {
		if _, e := client.PublishAgent(agentOptions(root)); e == nil {
			t.Fatal("early rejection accepted")
		}
	}
	files, _ := os.ReadDir(tmp)
	if len(files) != 0 {
		t.Fatal("multipart spool leaked", files)
	}
	opts := agentOptions(root)
	opts.InstructionsFile = filepath.Join(t.TempDir(), "large.md")
	os.WriteFile(opts.InstructionsFile, []byte(strings.Repeat("x", (64<<10)+1)), 0600)
	if _, e := client.PublishAgent(opts); e == nil {
		t.Fatal("oversized instructions accepted")
	}
}
