package server

import (
	"bytes"
	"fmt"
	"github.com/cinience/skillhub/pkg/agentarchive"
	"io"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

type agentZeros struct{}

func (agentZeros) Read(p []byte) (int, error) { clear(p); return len(p), nil }
func TestAgentRequestLimitAndCleanup(t *testing.T) {
	s, cookie, _ := agentTestServer(t)
	tmp := t.TempDir()
	t.Setenv("TMP", tmp)
	t.Setenv("TEMP", tmp)
	boundary := "agent-boundary"
	head := fmt.Sprintf("--%s\r\nContent-Disposition: form-data; name=\"archive\"; filename=\"large.zip\"\r\nContent-Type: application/zip\r\n\r\n", boundary)
	tail := "\r\n--" + boundary + "--\r\n"
	body := io.MultiReader(bytes.NewBufferString(head), io.LimitReader(agentZeros{}, agentarchive.MaxRequest+1), bytes.NewBufferString(tail))
	request := httptest.NewRequest("POST", "/api/v1/agents", body)
	request.Header.Set("Content-Type", "multipart/form-data; boundary="+boundary)
	request.AddCookie(cookie)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, request)
	if w.Code != 413 {
		t.Fatal(w.Code, w.Body)
	}
	files, e := os.ReadDir(tmp)
	if e != nil || len(files) != 0 {
		t.Fatal("multipart temporary files leaked", files, e)
	}
	archives, e := filepath.Glob(filepath.Join(s.cfg.AgentStore.BasePath, "*", "*.zip"))
	if e != nil || len(archives) != 0 {
		t.Fatal(archives, e)
	}
}
