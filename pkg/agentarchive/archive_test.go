package agentarchive

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
)

const testKey = "11111111-1111-4111-8111-111111111111/22222222-2222-4222-8222-222222222222.zip"

func zipFixture(t *testing.T, names []string, bodies [][]byte) []byte {
	t.Helper()
	var b bytes.Buffer
	w := zip.NewWriter(&b)
	for i, name := range names {
		f, e := w.Create(name)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = f.Write(bodies[i]); e != nil {
			t.Fatal(e)
		}
	}
	if e := w.Close(); e != nil {
		t.Fatal(e)
	}
	return b.Bytes()
}

func TestArchiveRoundTrip(t *testing.T) {
	names := []string{".agents/pm.md", ".claude/agents/dev.md", "设计/需求.md", "a/config.json", "b/config.json", "empty", "binary"}
	bodies := [][]byte{[]byte("pm\n"), []byte("dev\n"), []byte("需求\r\n"), []byte("{}"), []byte("[]"), {}, {0, 1, 255}}
	input := zipFixture(t, names, bodies)
	hash := sha256.Sum256(input)
	store, e := New(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	a, e := store.SaveZIP(context.Background(), testKey, bytes.NewReader(input))
	if e != nil {
		t.Fatal(e)
	}
	if a.SHA256 != hex.EncodeToString(hash[:]) || len(a.Files) != 7 {
		t.Fatalf("wrong archive: %+v", a)
	}
	f, e := store.Open(testKey)
	if e != nil {
		t.Fatal(e)
	}
	output, e := io.ReadAll(f)
	f.Close()
	if e != nil || !bytes.Equal(output, input) {
		t.Fatal("ZIP bytes changed", e)
	}
	for i, entry := range a.Files {
		h := sha256.Sum256(bodies[i])
		if entry.Path != names[i] || entry.Size != int64(len(bodies[i])) || entry.SHA256 != hex.EncodeToString(h[:]) {
			t.Fatalf("manifest mismatch %+v", entry)
		}
	}
	text, e := store.ReadText(context.Background(), testKey, "设计/需求.md")
	if e != nil || string(text) != "需求\r\n" {
		t.Fatal(string(text), e)
	}
	if _, e = store.ReadText(context.Background(), testKey, "binary"); !errors.Is(e, ErrUnsupportedPreview) {
		t.Fatal(e)
	}
	if _, e = store.SaveZIP(context.Background(), testKey, bytes.NewReader(input)); e == nil {
		t.Fatal("overwrote immutable archive")
	}
}

func TestFilesRoundTrip(t *testing.T) {
	s, _ := New(t.TempDir())
	files := []InputFile{}
	for _, p := range []string{".agents/a.md", "中文/x", "zero"} {
		data := []byte(p)
		if p == "zero" {
			data = nil
		}
		files = append(files, InputFile{Path: p, Open: func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(data)), nil }})
	}
	a, e := s.SaveFiles(context.Background(), testKey, files)
	if e != nil {
		t.Fatal(e)
	}
	if len(a.Files) != 3 {
		t.Fatal(a)
	}
	for _, p := range []string{".agents/a.md", "中文/x", "zero"} {
		got, e := s.ReadText(context.Background(), testKey, p)
		want := p
		if p == "zero" {
			want = ""
		}
		if e != nil || string(got) != want {
			t.Fatal(p, string(got), e)
		}
	}
}

func TestArchiveRejectsUnsafePaths(t *testing.T) {
	for _, name := range []string{"../x", "/x", "C:/x", "a\\b", "a:b", "a//b", "a/./b", "a/../b", ".git/config", "a/.GiT/x", "a\x00b", "a\nb", string([]byte{255}), strings.Repeat("a", 1025)} {
		t.Run(name, func(t *testing.T) {
			s, _ := New(t.TempDir())
			_, e := s.SaveZIP(context.Background(), testKey, bytes.NewReader(zipFixture(t, []string{name}, [][]byte{{1}})))
			if e == nil {
				t.Fatal("accepted unsafe path")
			}
			if _, e := s.Open(testKey); !os.IsNotExist(e) {
				t.Fatal("failed archive left behind", e)
			}
		})
	}
}

func TestArchiveConflictsAndDirectories(t *testing.T) {
	for _, names := range [][]string{{"a", "a"}, {"a", "a/b"}, {"a/b", "a"}, {"a/", "a"}, {"a/", "a/"}} {
		t.Run(strings.Join(names, ","), func(t *testing.T) {
			s, _ := New(t.TempDir())
			bodies := make([][]byte, len(names))
			_, e := s.SaveZIP(context.Background(), testKey, bytes.NewReader(zipFixture(t, names, bodies)))
			if !errors.Is(e, ErrInvalid) {
				t.Fatalf("want invalid got %v", e)
			}
		})
	}
	s, _ := New(t.TempDir())
	input := zipFixture(t, []string{"a/b", "a/", "empty/", ".gitignore"}, [][]byte{{1}, {}, {}, {2}})
	if a, e := s.SaveZIP(context.Background(), testKey, bytes.NewReader(input)); e != nil || len(a.Files) != 2 {
		t.Fatal(a, e)
	}
	for _, names := range [][]string{nil, {"empty/"}} {
		s, _ := New(t.TempDir())
		if _, e := s.SaveZIP(context.Background(), testKey, bytes.NewReader(zipFixture(t, names, make([][]byte, len(names))))); !errors.Is(e, ErrInvalid) {
			t.Fatal(e)
		}
	}
}

func TestArchiveLimitsAndCorruption(t *testing.T) {
	cases := []struct {
		name     string
		names    []string
		bodies   [][]byte
		tooLarge bool
	}{
		{"file", []string{"x"}, [][]byte{bytes.Repeat([]byte{'x'}, (10<<20)+1)}, true},
		{"count", make([]string, 501), make([][]byte, 501), true},
	}
	for i := range cases[1].names {
		cases[1].names[i] = strings.Repeat("a", i+1)
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s, _ := New(t.TempDir())
			_, e := s.SaveZIP(context.Background(), testKey, bytes.NewReader(zipFixture(t, c.names, c.bodies)))
			if !errors.Is(e, ErrTooLarge) {
				t.Fatal(e)
			}
		})
	}
	s, _ := New(t.TempDir())
	if _, e := s.SaveZIP(context.Background(), testKey, bytes.NewReader([]byte("broken"))); !errors.Is(e, ErrInvalid) {
		t.Fatal(e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e := s.SaveFiles(ctx, testKey, []InputFile{{Path: "x", Open: func() (io.ReadCloser, error) { return io.NopCloser(strings.NewReader("x")), nil }}}); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
}

func TestArchiveSpecialEntries(t *testing.T) {
	for _, mode := range []os.FileMode{os.ModeSymlink | 0777, os.ModeNamedPipe | 0600, os.ModeDevice | 0600} {
		var b bytes.Buffer
		w := zip.NewWriter(&b)
		h := &zip.FileHeader{Name: "entry", Method: zip.Store}
		h.SetMode(mode)
		f, e := w.CreateHeader(h)
		if e != nil {
			t.Fatal(e)
		}
		f.Write([]byte("target"))
		w.Close()
		s, _ := New(t.TempDir())
		if _, e := s.SaveZIP(context.Background(), testKey, bytes.NewReader(b.Bytes())); !errors.Is(e, ErrInvalid) {
			t.Fatal(e)
		}
	}
}

func TestPreviewBoundaries(t *testing.T) {
	for _, size := range []int{0, 256 << 10, (256 << 10) + 1} {
		s, _ := New(t.TempDir())
		data := bytes.Repeat([]byte{'x'}, size)
		_, e := s.SaveZIP(context.Background(), testKey, bytes.NewReader(zipFixture(t, []string{"x"}, [][]byte{data})))
		if e != nil {
			t.Fatal(e)
		}
		got, e := s.ReadText(context.Background(), testKey, "x")
		if size > 256<<10 {
			if !errors.Is(e, ErrUnsupportedPreview) {
				t.Fatal(e)
			}
		} else if e != nil || len(got) != size {
			t.Fatal(e)
		}
	}
}
