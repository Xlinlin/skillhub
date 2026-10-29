package agentarchive

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	MaxRequest = 55 << 20
	MaxZIP     = 50 << 20
	MaxContent = 50 << 20
	MaxFile    = 10 << 20
	MaxFiles   = 500
	MaxEntries = 1000
	MaxPreview = 256 << 10
)

var (
	ErrInvalid            = errors.New("invalid agent archive")
	ErrTooLarge           = errors.New("agent archive exceeds limits")
	ErrUnsupportedPreview = errors.New("file cannot be previewed as text")
	keyPattern            = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}/[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}\.zip$`)
)

type Entry struct {
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}
type Archive struct {
	Key    string
	SHA256 string
	Size   int64
	Files  []Entry
}
type InputFile struct {
	Path string
	Open func() (io.ReadCloser, error)
}
type Store struct{ basePath string }

func New(basePath string) (*Store, error) {
	if err := os.MkdirAll(basePath, 0700); err != nil {
		return nil, err
	}
	probe, err := os.CreateTemp(basePath, ".probe-")
	if err != nil {
		return nil, err
	}
	name := probe.Name()
	err = probe.Close()
	removeErr := os.Remove(name)
	if err != nil {
		return nil, err
	}
	if removeErr != nil {
		return nil, removeErr
	}
	return &Store{basePath: basePath}, nil
}
func (s *Store) path(key string) (string, error) {
	if !keyPattern.MatchString(key) {
		return "", ErrInvalid
	}
	return filepath.Join(s.basePath, filepath.FromSlash(key)), nil
}
func (s *Store) Open(key string) (*os.File, error) {
	p, e := s.path(key)
	if e != nil {
		return nil, e
	}
	return os.Open(p)
}
func (s *Store) Remove(key string) error {
	p, e := s.path(key)
	if e != nil {
		return e
	}
	return os.Remove(p)
}
func (s *Store) create(key string) (*os.File, error) {
	p, e := s.path(key)
	if e != nil {
		return nil, e
	}
	if e = os.MkdirAll(filepath.Dir(p), 0700); e != nil {
		return nil, e
	}
	return os.OpenFile(p, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0600)
}

type contextReader struct {
	ctx context.Context
	src io.Reader
}

func (r contextReader) Read(b []byte) (int, error) {
	if e := r.ctx.Err(); e != nil {
		return 0, e
	}
	return r.src.Read(b)
}

type limitedWriter struct {
	dst       io.Writer
	remaining int64
}

func (w *limitedWriter) Write(p []byte) (int, error) {
	if int64(len(p)) > w.remaining {
		return 0, ErrTooLarge
	}
	n, e := w.dst.Write(p)
	w.remaining -= int64(n)
	return n, e
}
func copyBounded(ctx context.Context, dst io.Writer, src io.Reader, limit int64) (int64, error) {
	n, e := io.Copy(dst, io.LimitReader(contextReader{ctx, src}, limit+1))
	if e != nil {
		return n, e
	}
	if n > limit {
		return n, ErrTooLarge
	}
	return n, nil
}
func invalid(reason string) error { return fmt.Errorf("%w: %s", ErrInvalid, reason) }

// ValidatePath never rewrites an author's submitted path.
func ValidatePath(name string, dir bool) error {
	if len(name) > 1024 {
		return ErrTooLarge
	}
	if !utf8.ValidString(name) || strings.ContainsAny(name, "\\:") {
		return invalid("unsafe path")
	}
	if dir {
		name = strings.TrimSuffix(name, "/")
	}
	if name == "" {
		return invalid("empty path")
	}
	for _, r := range name {
		if unicode.IsControl(r) {
			return invalid("control character in path")
		}
	}
	for _, part := range strings.Split(name, "/") {
		if part == "" || part == "." || part == ".." || strings.EqualFold(part, ".git") {
			return invalid("unsafe path component")
		}
	}
	return nil
}

type pathSet struct {
	explicit map[string]bool
	dirs     map[string]bool
	files    map[string]bool
}

func newPathSet() *pathSet { return &pathSet{map[string]bool{}, map[string]bool{}, map[string]bool{}} }
func (p *pathSet) add(name string, dir bool) error {
	if e := ValidatePath(name, dir); e != nil {
		return e
	}
	name = strings.TrimSuffix(name, "/")
	if p.explicit[name] || p.files[name] || (!dir && p.dirs[name]) {
		return invalid("duplicate or conflicting paths")
	}
	p.explicit[name] = true
	parts := strings.Split(name, "/")
	for i := 1; i < len(parts); i++ {
		parent := strings.Join(parts[:i], "/")
		if p.files[parent] {
			return invalid("file used as directory")
		}
		p.dirs[parent] = true
	}
	if dir {
		p.dirs[name] = true
	} else {
		p.files[name] = true
	}
	return nil
}

func (s *Store) SaveZIP(ctx context.Context, key string, src io.Reader) (a *Archive, err error) {
	f, err := s.create(key)
	if err != nil {
		return nil, err
	}
	defer func() {
		f.Close()
		if err != nil {
			s.Remove(key)
		}
	}()
	hash := sha256.New()
	size, err := copyBounded(ctx, io.MultiWriter(f, hash), src, MaxZIP)
	if err != nil {
		return nil, err
	}
	if err = preflight(f, size); err != nil {
		return nil, err
	}
	reader, e := zip.NewReader(f, size)
	if e != nil {
		return nil, invalid("invalid ZIP directory")
	}
	entries, e := validateZIP(ctx, reader)
	if e != nil {
		return nil, e
	}
	if err = f.Sync(); err != nil {
		return nil, err
	}
	if err = f.Close(); err != nil {
		return nil, err
	}
	return &Archive{Key: key, SHA256: hex.EncodeToString(hash.Sum(nil)), Size: size, Files: entries}, nil
}

func (s *Store) SaveFiles(ctx context.Context, key string, files []InputFile) (a *Archive, err error) {
	if len(files) == 0 {
		return nil, invalid("at least one file required")
	}
	if len(files) > MaxFiles {
		return nil, ErrTooLarge
	}
	paths := newPathSet()
	for _, file := range files {
		if e := paths.add(file.Path, false); e != nil {
			return nil, e
		}
	}
	f, err := s.create(key)
	if err != nil {
		return nil, err
	}
	defer func() {
		f.Close()
		if err != nil {
			s.Remove(key)
		}
	}()
	hash := sha256.New()
	output := &limitedWriter{io.MultiWriter(f, hash), MaxRequest}
	w := zip.NewWriter(output)
	entries := make([]Entry, 0, len(files))
	var total int64
	for _, file := range files {
		if err = ctx.Err(); err != nil {
			return nil, err
		}
		if file.Open == nil {
			return nil, invalid("missing file reader")
		}
		src, e := file.Open()
		if e != nil {
			return nil, e
		}
		dst, e := w.Create(file.Path)
		if e != nil {
			src.Close()
			return nil, e
		}
		h := sha256.New()
		n, e := copyBounded(ctx, io.MultiWriter(dst, h), src, min(int64(MaxFile), int64(MaxContent)-total))
		closeErr := src.Close()
		if e != nil {
			return nil, e
		}
		if closeErr != nil {
			return nil, closeErr
		}
		total += n
		if total > MaxContent {
			return nil, ErrTooLarge
		}
		entries = append(entries, Entry{file.Path, n, hex.EncodeToString(h.Sum(nil))})
	}
	if err = w.Close(); err != nil {
		return nil, err
	}
	if err = f.Sync(); err != nil {
		return nil, err
	}
	if err = f.Close(); err != nil {
		return nil, err
	}
	return &Archive{key, hex.EncodeToString(hash.Sum(nil)), MaxRequest - output.remaining, entries}, nil
}

func validateZIP(ctx context.Context, r *zip.Reader) ([]Entry, error) {
	if len(r.File) > MaxEntries {
		return nil, ErrTooLarge
	}
	paths := newPathSet()
	entries := []Entry{}
	var total int64
	for _, file := range r.File {
		if e := ctx.Err(); e != nil {
			return nil, e
		}
		mode := file.Mode()
		dir := strings.HasSuffix(file.Name, "/")
		creator := file.CreatorVersion >> 8
		declaredType := (file.ExternalAttrs >> 16) & 0170000
		unixTypeConflict := (creator == 3 || creator == 19) && declaredType != 0 && ((dir && declaredType != 0040000) || (!dir && declaredType != 0100000))
		// Mode adds ModeDir for a trailing slash, even when the explicit type is a symlink/device.
		if mode&(os.ModeType&^os.ModeDir) != 0 || unixTypeConflict || mode.IsDir() != dir || (!mode.IsRegular() && !mode.IsDir()) || file.Flags&1 != 0 || (file.Method != zip.Store && file.Method != zip.Deflate) {
			return nil, invalid("unsupported ZIP entry")
		}
		if e := paths.add(file.Name, dir); e != nil {
			return nil, e
		}
		if file.UncompressedSize64 > MaxFile {
			return nil, ErrTooLarge
		}
		src, e := file.Open()
		if e != nil {
			return nil, invalid("unreadable ZIP entry")
		}
		h := sha256.New()
		limit := min(int64(MaxFile), int64(MaxContent)-total)
		if dir {
			limit = 0
		}
		n, e := copyBounded(ctx, h, src, limit)
		closeErr := src.Close()
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if errors.Is(e, ErrTooLarge) && !dir {
			return nil, e
		}
		if e != nil || closeErr != nil {
			return nil, invalid("corrupt ZIP entry")
		}
		if dir {
			continue
		}
		total += n
		if total > MaxContent {
			return nil, ErrTooLarge
		}
		entries = append(entries, Entry{file.Name, n, hex.EncodeToString(h.Sum(nil))})
		if len(entries) > MaxFiles {
			return nil, ErrTooLarge
		}
	}
	if len(entries) == 0 {
		return nil, invalid("at least one ordinary file required")
	}
	return entries, nil
}

func (s *Store) ReadText(ctx context.Context, key, path string) ([]byte, error) {
	if e := ValidatePath(path, false); e != nil {
		return nil, e
	}
	f, e := s.Open(key)
	if e != nil {
		return nil, e
	}
	defer f.Close()
	info, e := f.Stat()
	if e != nil {
		return nil, e
	}
	r, e := zip.NewReader(f, info.Size())
	if e != nil {
		return nil, e
	}
	for _, entry := range r.File {
		if entry.Name != path {
			continue
		}
		if entry.Mode().IsDir() {
			return nil, ErrInvalid
		}
		if entry.UncompressedSize64 > MaxPreview {
			return nil, ErrUnsupportedPreview
		}
		src, e := entry.Open()
		if e != nil {
			return nil, e
		}
		defer src.Close()
		b, e := io.ReadAll(io.LimitReader(contextReader{ctx, src}, MaxPreview+1))
		if e != nil {
			return nil, e
		}
		if len(b) > MaxPreview || !utf8.Valid(b) {
			return nil, ErrUnsupportedPreview
		}
		for _, ch := range string(b) {
			if unicode.IsControl(ch) && ch != '\t' && ch != '\r' && ch != '\n' {
				return nil, ErrUnsupportedPreview
			}
		}
		return b, nil
	}
	return nil, os.ErrNotExist
}
