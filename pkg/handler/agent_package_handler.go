package handler

import (
	"encoding/json"
	"errors"
	"github.com/cinience/skillhub/pkg/agentarchive"
	"github.com/cinience/skillhub/pkg/middleware"
	"github.com/cinience/skillhub/pkg/repository"
	"github.com/cinience/skillhub/pkg/service"
	"github.com/gin-gonic/gin"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"regexp"
	"strconv"
	"unicode/utf8"
)

type AgentPackageHandler struct{ svc *service.AgentPackageService }

func NewAgentPackageHandler(s *service.AgentPackageService) *AgentPackageHandler {
	return &AgentPackageHandler{s}
}
func agentHTTPError(c *gin.Context, err error) {
	var e *service.AgentError
	if !errors.As(err, &e) {
		e = &service.AgentError{Code: "internal_error", Message: "Unable to complete the agent operation"}
	}
	status := map[string]int{"validation": 400, "unauthorized": 401, "forbidden": 403, "not_found": 404, "conflict": 409, "too_large": 413, "unsupported_preview": 415, "unavailable": 503, "publish_outcome_unknown": 503}[e.Code]
	if status == 0 {
		status = 500
	}
	c.JSON(status, gin.H{"error": e.Message, "code": e.Code})
}
func badAgentForm(c *gin.Context, message string) {
	agentHTTPError(c, &service.AgentError{Code: "validation", Message: message})
}

var agentPageDigits = regexp.MustCompile(`^[0-9]+$`)

func agentPagination(c *gin.Context) (int, int, error) {
	p, s := c.DefaultQuery("page", "1"), c.DefaultQuery("pageSize", "20")
	if !agentPageDigits.MatchString(p) || !agentPageDigits.MatchString(s) {
		return 0, 0, &service.AgentError{Code: "validation", Message: "Invalid pagination"}
	}
	page, e := strconv.Atoi(p)
	if e != nil {
		return 0, 0, &service.AgentError{Code: "validation", Message: "Invalid page"}
	}
	size, e := strconv.Atoi(s)
	if e != nil {
		return 0, 0, &service.AgentError{Code: "validation", Message: "Invalid pageSize"}
	}
	return page, size, service.ValidateAgentPage(page, size)
}
func (h *AgentPackageHandler) List(c *gin.Context) {
	p, s, e := agentPagination(c)
	if e != nil {
		agentHTTPError(c, e)
		return
	}
	out, e := h.svc.List(c.Request.Context(), repository.AgentListOptions{Q: c.Query("q"), Kind: c.Query("kind"), Page: p, PageSize: s})
	if e != nil {
		agentHTTPError(c, e)
		return
	}
	c.JSON(200, out)
}
func (h *AgentPackageHandler) Get(c *gin.Context) {
	out, e := h.svc.Get(c.Request.Context(), c.Param("slug"), c.DefaultQuery("version", "latest"))
	if e != nil {
		agentHTTPError(c, e)
		return
	}
	c.JSON(200, out)
}
func (h *AgentPackageHandler) Versions(c *gin.Context) {
	p, s, e := agentPagination(c)
	if e != nil {
		agentHTTPError(c, e)
		return
	}
	out, e := h.svc.Versions(c.Request.Context(), c.Param("slug"), p, s)
	if e != nil {
		agentHTTPError(c, e)
		return
	}
	c.JSON(200, out)
}
func (h *AgentPackageHandler) GetFile(c *gin.Context) {
	out, e := h.svc.ReadText(c.Request.Context(), c.Param("slug"), c.DefaultQuery("version", "latest"), c.Query("path"))
	if e != nil {
		agentHTTPError(c, e)
		return
	}
	c.Data(200, "text/plain; charset=utf-8", out)
}
func (h *AgentPackageHandler) Delete(c *gin.Context) {
	if e := h.svc.Delete(c.Request.Context(), middleware.GetUser(c), c.Param("slug")); e != nil {
		agentHTTPError(c, e)
		return
	}
	c.JSON(200, gin.H{"message": "Agent deleted"})
}
func (h *AgentPackageHandler) Download(c *gin.Context) {
	f, v, e := h.svc.Download(c.Request.Context(), c.Param("slug"), c.DefaultQuery("version", "latest"))
	if e != nil {
		agentHTTPError(c, e)
		return
	}
	defer f.Close()
	c.Header("Content-Type", "application/zip")
	c.Header("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": c.Param("slug") + "-" + v.Version + ".zip"}))
	c.Header("X-Content-SHA256", v.ArchiveSHA256)
	http.ServeContent(c.Writer, c.Request, "", v.CreatedAt, f)
}
func (h *AgentPackageHandler) Publish(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, agentarchive.MaxRequest)
	defer func() {
		if c.Request.MultipartForm != nil {
			c.Request.MultipartForm.RemoveAll()
		}
	}()
	if e := c.Request.ParseMultipartForm(8 << 20); e != nil {
		var max *http.MaxBytesError
		if errors.As(e, &max) || errors.Is(e, multipart.ErrMessageTooLarge) {
			agentHTTPError(c, &service.AgentError{Code: "too_large", Message: "Upload exceeds request limits"})
		} else {
			badAgentForm(c, "Invalid multipart upload")
		}
		return
	}
	form := c.Request.MultipartForm
	if form == nil {
		badAgentForm(c, "Multipart upload required")
		return
	}
	allowed := map[string]bool{"slug": true, "displayName": true, "kind": true, "version": true, "summary": true, "instructions": true, "changelog": true, "tags": true, "mode": true, "paths": true}
	metadataBytes := 0
	for key, values := range form.Value {
		if !allowed[key] || len(values) != 1 {
			badAgentForm(c, "Unknown or repeated metadata field")
			return
		}
		if !utf8.ValidString(values[0]) {
			badAgentForm(c, "Metadata must be valid UTF-8")
			return
		}
		metadataBytes += len(values[0])
	}
	if metadataBytes > 1<<20 {
		agentHTTPError(c, &service.AgentError{Code: "too_large", Message: "Metadata exceeds 1 MiB"})
		return
	}
	for key := range form.File {
		if key != "files" && key != "archive" {
			badAgentForm(c, "Unknown upload file field")
			return
		}
	}
	val := func(key string) string {
		if v := form.Value[key]; len(v) == 1 {
			return v[0]
		}
		return ""
	}
	req := service.AgentPublishRequest{Slug: val("slug"), DisplayName: val("displayName"), Kind: val("kind"), Version: val("version"), Summary: val("summary"), Instructions: val("instructions"), Changelog: val("changelog"), Mode: val("mode")}
	if tags, ok := form.Value["tags"]; ok {
		if json.Unmarshal([]byte(tags[0]), &req.Tags) != nil || req.Tags == nil {
			badAgentForm(c, "Tags must be a JSON string array")
			return
		}
	}
	switch req.Mode {
	case "files":
		headers := form.File["files"]
		if len(form.File["archive"]) != 0 || len(headers) == 0 {
			badAgentForm(c, "Files mode requires files and no archive")
			return
		}
		if len(headers) > agentarchive.MaxFiles {
			agentHTTPError(c, &service.AgentError{Code: "too_large", Message: "At most 500 files are allowed"})
			return
		}
		var paths []string
		if json.Unmarshal([]byte(val("paths")), &paths) != nil || len(paths) != len(headers) {
			badAgentForm(c, "Paths must be a JSON string array matching files in order")
			return
		}
		for i, header := range headers {
			if header.Size > agentarchive.MaxFile {
				agentHTTPError(c, &service.AgentError{Code: "too_large", Message: "Each file must be at most 10 MiB"})
				return
			}
			req.Files = append(req.Files, agentarchive.InputFile{Path: paths[i], Open: func() (io.ReadCloser, error) { return header.Open() }})
		}
	case "archive":
		_, hasPaths := form.Value["paths"]
		if len(form.File["archive"]) != 1 || len(form.File["files"]) != 0 || hasPaths {
			badAgentForm(c, "Archive mode requires one ZIP and no files or paths")
			return
		}
		header := form.File["archive"][0]
		if header.Size > agentarchive.MaxZIP {
			agentHTTPError(c, &service.AgentError{Code: "too_large", Message: "ZIP must be at most 50 MiB"})
			return
		}
		src, e := header.Open()
		if e != nil {
			agentHTTPError(c, &service.AgentError{Code: "unavailable", Message: "Upload temporarily unavailable"})
			return
		}
		defer src.Close()
		req.Archive = src
	default:
		badAgentForm(c, "Mode must be files or archive")
		return
	}
	result, e := h.svc.Publish(c.Request.Context(), middleware.GetUser(c), req)
	if e != nil {
		agentHTTPError(c, e)
		return
	}
	status := 200
	if result.Created {
		status = 201
	}
	c.JSON(status, result)
}
