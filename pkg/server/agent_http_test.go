package server

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/cinience/skillhub/pkg/auth"
	"github.com/cinience/skillhub/pkg/config"
	"github.com/cinience/skillhub/pkg/model"
	"github.com/google/uuid"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
)

func agentTestServer(t *testing.T) (*Server, *http.Cookie, string) {
	t.Helper()
	cfg := config.DefaultConfig()
	root := t.TempDir()
	cfg.Database.URL = filepath.Join(root, "db.sqlite")
	cfg.GitStore.BasePath = filepath.Join(root, "repos")
	cfg.Search.IndexPath = filepath.Join(root, "search")
	cfg.AgentStore.BasePath = filepath.Join(root, "agents")
	cfg.RateLimit.ReadLimit = 10000
	cfg.RateLimit.WriteLimit = 10000
	cfg.RateLimit.DownloadLimit = 10000
	s, e := New(cfg)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { s.Shutdown(context.Background()) })
	password := uuid.NewString()
	hash, e := auth.HashPassword(password)
	if e != nil {
		t.Fatal(e)
	}
	u := model.User{ID: uuid.New(), Handle: "test-author", Role: "user", PasswordHash: &hash}
	if e = s.db.Create(&u).Error; e != nil {
		t.Fatal(e)
	}
	body := url.Values{"handle": {u.Handle}, "password": {password}}.Encode()
	req := httptest.NewRequest("POST", "/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
	cookies := w.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("no login cookie")
	}
	return s, cookies[0], cookies[0].Value
}
func agentTestZIP(t *testing.T) []byte {
	t.Helper()
	var b bytes.Buffer
	w := zip.NewWriter(&b)
	for _, p := range []string{".agents/pm.md", "设计/需求.md", "a/config.json", "b/config.json"} {
		f, e := w.Create(p)
		if e != nil {
			t.Fatal(e)
		}
		f.Write([]byte(p + "\n"))
	}
	if e := w.Close(); e != nil {
		t.Fatal(e)
	}
	return b.Bytes()
}
func agentForm(t *testing.T, version string, zipBytes []byte, extra map[string][]string) (*bytes.Buffer, string) {
	t.Helper()
	var b bytes.Buffer
	w := multipart.NewWriter(&b)
	fields := map[string]string{"slug": "suite", "displayName": "Suite", "kind": "suite", "version": version, "mode": "archive", "tags": "[\"team\"]", "instructions": "<script>window.injected=true</script>"}
	for k, v := range fields {
		w.WriteField(k, v)
	}
	for k, values := range extra {
		for _, v := range values {
			w.WriteField(k, v)
		}
	}
	f, e := w.CreateFormFile("archive", "suite.zip")
	if e != nil {
		t.Fatal(e)
	}
	f.Write(zipBytes)
	w.Close()
	return &b, w.FormDataContentType()
}
func TestAgentHTTPFlow(t *testing.T) {
	s, cookie, token := agentTestServer(t)
	input := agentTestZIP(t)
	publish := func(version string, extra map[string][]string, origin string) *httptest.ResponseRecorder {
		body, ct := agentForm(t, version, input, extra)
		r := httptest.NewRequest("POST", "/api/v1/agents", body)
		r.Header.Set("Content-Type", ct)
		r.AddCookie(cookie)
		if origin != "" {
			r.Header.Set("Origin", origin)
			r.Header.Set("Sec-Fetch-Site", "cross-site")
		}
		w := httptest.NewRecorder()
		s.router.ServeHTTP(w, r)
		return w
	}
	if w := publish("1.0.0", nil, ""); w.Code != 201 {
		t.Fatal(w.Code, w.Body)
	}
	if w := publish("1.0.0", nil, ""); w.Code != 409 {
		t.Fatal(w.Code, w.Body)
	}
	if w := publish("1.1.0", nil, "https://untrusted.example"); w.Code != 403 {
		t.Fatal(w.Code, w.Body)
	}
	for _, extra := range []map[string][]string{{"slug": {"other"}}, {"paths": {"[]"}}, {"other": {"x"}}, {"tags": {"null"}}} {
		if w := publish("1.1.0", extra, ""); w.Code != 400 {
			t.Fatal(w.Code, w.Body)
		}
	}
	if w := publish("1.1.0", nil, ""); w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
	get := func(path string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", path, nil)
		w := httptest.NewRecorder()
		s.router.ServeHTTP(w, r)
		if w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("cache policy", path)
		}
		return w
	}
	for _, v := range []string{"1.0.0", "1.1.0"} {
		w := get("/api/v1/agents/suite/download?version=" + v)
		if w.Code != 200 || !bytes.Equal(w.Body.Bytes(), input) {
			t.Fatal(w.Code, w.Body)
		}
		hash := sha256.Sum256(w.Body.Bytes())
		if w.Header().Get("X-Content-SHA256") != hex.EncodeToString(hash[:]) || !strings.Contains(w.Header().Get("Content-Disposition"), "attachment") {
			t.Fatal(w.Header())
		}
	}
	w := get("/api/v1/agents/suite/file?version=1.0.0&path=" + url.QueryEscape(".agents/pm.md"))
	if w.Code != 200 || w.Body.String() != ".agents/pm.md\n" || !strings.HasPrefix(w.Header().Get("Content-Type"), "text/plain") {
		t.Fatal(w.Code, w.Body)
	}
	if w := get("/api/v1/agents/suite/file?path=../x"); w.Code != 404 {
		t.Fatal(w.Code, w.Body)
	}
	for _, q := range []string{"page=0", "pageSize=101", "page=1.5", "page=9223372036854775807&pageSize=100", "kind=other"} {
		if w := get("/api/v1/agents?" + q); w.Code != 400 {
			t.Fatal(q, w.Code, w.Body)
		}
	}
	w = get("/api/v1/agents")
	if strings.Contains(w.Body.String(), "archiveKey") || strings.Contains(w.Body.String(), "instructions") || strings.Contains(w.Body.String(), "password") {
		t.Fatal("list leaks metadata", w.Body)
	}
	r := httptest.NewRequest("DELETE", "/api/v1/agents/suite", nil)
	r.Header.Set("Authorization", "Bearer "+token)
	w = httptest.NewRecorder()
	s.router.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
	for _, path := range []string{"", "/versions", "/file?path=.agents/pm.md", "/download?version=1.0.0", "?version=1.0.0"} {
		if w := get("/api/v1/agents/suite" + path); w.Code != 404 {
			t.Fatal(path, w.Code, w.Body)
		}
	}
	var list struct {
		Data  []any
		Total int
	}
	w = get("/api/v1/agents")
	json.Unmarshal(w.Body.Bytes(), &list)
	if list.Data == nil || list.Total != 0 || len(list.Data) != 0 {
		t.Fatal(w.Body)
	}
}
func TestAgentHTTPFiles(t *testing.T) {
	s, cookie, _ := agentTestServer(t)
	var b bytes.Buffer
	w := multipart.NewWriter(&b)
	for k, v := range map[string]string{"slug": "files", "displayName": "Files", "kind": "single", "version": "1.0.0", "mode": "files", "paths": "[\".agents/a.md\",\"nested/a.md\"]"} {
		w.WriteField(k, v)
	}
	for _, content := range []string{"first", "second"} {
		p, _ := w.CreateFormFile("files", "a.md")
		io.WriteString(p, content)
	}
	w.Close()
	r := httptest.NewRequest("POST", "/api/v1/agents", &b)
	r.Header.Set("Content-Type", w.FormDataContentType())
	r.AddCookie(cookie)
	response := httptest.NewRecorder()
	s.router.ServeHTTP(response, r)
	if response.Code != 201 {
		t.Fatal(response.Code, response.Body)
	}
	for i, path := range []string{".agents/a.md", "nested/a.md"} {
		r = httptest.NewRequest("GET", "/api/v1/agents/files/file?path="+url.QueryEscape(path), nil)
		response = httptest.NewRecorder()
		s.router.ServeHTTP(response, r)
		want := []string{"first", "second"}[i]
		if response.Code != 200 || response.Body.String() != want {
			t.Fatal(fmt.Sprintf("%s: %d %s", path, response.Code, response.Body))
		}
	}
}
