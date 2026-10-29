package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/cinience/skillhub/pkg/agentarchive"
)

type AgentPublishOptions struct{ Path, Slug, Name, Kind, Version, Summary, Tags, Changelog, InstructionsFile string }
type AgentPublishError struct {
	StatusCode    int
	Code, Message string
}

func (e *AgentPublishError) Error() string {
	code := ""
	if e.Code != "" {
		code = "/" + e.Code
	}
	return fmt.Sprintf("Agent API error (%d%s): %s", e.StatusCode, code, e.Message)
}

type agentSource struct {
	Path string
	Size int64
}

func collectAgentRootFiles(root *os.Root) ([]agentSource, error) {
	files := []agentSource{}
	var total int64
	err := fs.WalkDir(root.FS(), ".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == "." {
			return nil
		}
		if err := agentarchive.ValidatePath(path, entry.IsDir()); err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("only ordinary files are supported: %s", path)
		}
		if info.Size() > agentarchive.MaxFile {
			return agentarchive.ErrTooLarge
		}
		total += info.Size()
		if total > agentarchive.MaxContent || len(files) >= agentarchive.MaxFiles {
			return agentarchive.ErrTooLarge
		}
		files = append(files, agentSource{path, info.Size()})
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("agent directory contains no ordinary files")
	}
	return files, nil
}
func collectAgentFiles(dir string) ([]agentSource, error) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	return collectAgentRootFiles(root)
}

type agentUploadWriter struct {
	dst       io.Writer
	remaining int64
}

func (w *agentUploadWriter) Write(p []byte) (int, error) {
	if int64(len(p)) > w.remaining {
		return 0, agentarchive.ErrTooLarge
	}
	n, e := w.dst.Write(p)
	w.remaining -= int64(n)
	return n, e
}
func copyAgentSource(dst io.Writer, src io.Reader, limit int64) (int64, error) {
	n, err := io.Copy(dst, io.LimitReader(src, limit+1))
	if err == nil && n > limit {
		err = agentarchive.ErrTooLarge
	}
	return n, err
}

func readAgentInstructions(path string) (string, error) {
	if path == "" {
		return "", nil
	}
	info, err := os.Lstat(path)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("instructions must be an ordinary UTF-8 text file")
	}
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	opened, err := f.Stat()
	if err != nil {
		return "", err
	}
	if !os.SameFile(info, opened) {
		return "", fmt.Errorf("instructions file changed while opening")
	}
	b, err := io.ReadAll(io.LimitReader(f, (64<<10)+1))
	if err != nil {
		return "", err
	}
	if len(b) > 64<<10 {
		return "", fmt.Errorf("instructions exceed 64 KiB")
	}
	if !utf8.Valid(b) {
		return "", fmt.Errorf("instructions must be valid UTF-8")
	}
	return string(b), nil
}

func writeAgentPayload(w *multipart.Writer, o AgentPublishOptions, instructions string) error {
	info, err := os.Lstat(o.Path)
	if err != nil {
		return err
	}
	if !info.IsDir() && (!info.Mode().IsRegular() || !strings.EqualFold(filepath.Ext(o.Path), ".zip")) {
		return fmt.Errorf("Agent source must be a directory or an ordinary ZIP file")
	}
	mode := "archive"
	if info.IsDir() {
		mode = "files"
	}
	tags := []string{}
	for _, tag := range strings.Split(o.Tags, ",") {
		if tag = strings.TrimSpace(tag); tag != "" {
			tags = append(tags, tag)
		}
	}
	encodedTags, err := json.Marshal(tags)
	if err != nil {
		return err
	}
	fields := map[string]string{"mode": mode, "slug": o.Slug, "displayName": o.Name, "kind": o.Kind, "version": o.Version, "summary": o.Summary, "tags": string(encodedTags), "instructions": instructions, "changelog": o.Changelog}
	for key, value := range fields {
		if err = w.WriteField(key, value); err != nil {
			return err
		}
	}
	if mode == "archive" {
		if info.Size() > agentarchive.MaxZIP {
			return agentarchive.ErrTooLarge
		}
		src, err := os.Open(o.Path)
		if err != nil {
			return err
		}
		defer src.Close()
		opened, err := src.Stat()
		if err != nil {
			return err
		}
		if !os.SameFile(info, opened) || !opened.Mode().IsRegular() {
			return fmt.Errorf("ZIP changed while opening")
		}
		part, err := w.CreateFormFile("archive", filepath.Base(o.Path))
		if err != nil {
			return err
		}
		_, err = copyAgentSource(part, src, agentarchive.MaxZIP)
		return err
	}
	root, err := os.OpenRoot(o.Path)
	if err != nil {
		return err
	}
	defer root.Close()
	opened, err := root.Stat(".")
	if err != nil {
		return err
	}
	if !os.SameFile(info, opened) {
		return fmt.Errorf("directory changed while opening")
	}
	sources, err := collectAgentRootFiles(root)
	if err != nil {
		return err
	}
	paths := make([]string, len(sources))
	for i, file := range sources {
		paths[i] = file.Path
	}
	pathJSON, err := json.Marshal(paths)
	if err != nil {
		return err
	}
	if err = w.WriteField("paths", string(pathJSON)); err != nil {
		return err
	}
	var total int64
	for _, source := range sources {
		before, err := root.Lstat(filepath.FromSlash(source.Path))
		if err != nil {
			return err
		}
		if !before.Mode().IsRegular() {
			return fmt.Errorf("source is no longer an ordinary file: %s", source.Path)
		}
		src, err := root.Open(filepath.FromSlash(source.Path))
		if err != nil {
			return err
		}
		opened, err := src.Stat()
		if err != nil {
			src.Close()
			return err
		}
		if !os.SameFile(before, opened) || !opened.Mode().IsRegular() {
			src.Close()
			return fmt.Errorf("source changed while opening: %s", source.Path)
		}
		part, err := w.CreateFormFile("files", filepath.Base(source.Path))
		if err != nil {
			src.Close()
			return err
		}
		n, err := copyAgentSource(part, src, min(int64(agentarchive.MaxFile), int64(agentarchive.MaxContent)-total))
		closeErr := src.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
		total += n
	}
	return nil
}

func (c *Client) PublishAgent(o AgentPublishOptions) (map[string]interface{}, error) {
	if c.Token == "" {
		return nil, fmt.Errorf("run skillhub login first with your Registry URL and API Token")
	}
	if o.Slug == "" || strings.TrimSpace(o.Name) == "" || o.Version == "" || (o.Kind != "single" && o.Kind != "suite") {
		return nil, fmt.Errorf("Agent publishing requires --slug, --name, --version and --kind single|suite")
	}
	instructions, err := readAgentInstructions(o.InstructionsFile)
	if err != nil {
		return nil, err
	}
	body, err := os.CreateTemp("", "skillhub-agent-upload-*.multipart")
	if err != nil {
		return nil, err
	}
	defer func() { body.Close(); os.Remove(body.Name()) }()
	limited := &agentUploadWriter{dst: body, remaining: agentarchive.MaxRequest}
	writer := multipart.NewWriter(limited)
	if err = writeAgentPayload(writer, o, instructions); err != nil {
		return nil, err
	}
	if err = writer.Close(); err != nil {
		return nil, err
	}
	if _, err = body.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	// A registry redirect must never replay the archive or credentials to another endpoint.
	uploadClient := *c
	transport := *c.httpClient
	transport.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	uploadClient.httpClient = &transport
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	resp, err := uploadClient.doContext(ctx, "POST", "/api/v1/agents", body, writer.FormDataContentType())
	if err != nil {
		return nil, fmt.Errorf("publish request failed (check the requested version before retrying; its result may be unknown): %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
		var response struct {
			Error string `json:"error"`
			Code  string `json:"code"`
		}
		_ = json.Unmarshal(b, &response)
		if response.Error == "" {
			response.Error = http.StatusText(resp.StatusCode)
		}
		return nil, &AgentPublishError{resp.StatusCode, response.Code, response.Error}
	}
	var result map[string]interface{}
	if err = json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&result); err != nil {
		return nil, fmt.Errorf("publish response could not be decoded; check the requested version before retrying: %w", err)
	}
	return result, nil
}
