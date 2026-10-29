package gitstore

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"os/exec"
	"testing"
)

func TestArchivePreservesContentWithAutoCRLF(t *testing.T) {
	gs, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	want := []byte("# Skill\nKeep original line endings.\n")
	_, err = gs.Publish(context.Background(), PublishOpts{
		Owner: "test", Slug: "archive", Version: "1.0.0",
		Author: "test", Email: "test@example.com",
		Files: map[string][]byte{"SKILL.md": want},
	})
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("git", "-C", gs.RepoPath("test", "archive"), "config", "core.autocrlf", "true")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("set repository autocrlf: %v: %s", err, out)
	}
	archive, err := gs.Archive("test", "archive", "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(archive)
	closeErr := archive.Close()
	if err != nil {
		t.Fatal(err)
	}
	if closeErr != nil {
		t.Fatal(closeErr)
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	f, err := zr.Open("archive-1.0.0/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	got, err := io.ReadAll(f)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("archive changed content: got %q, want %q", got, want)
	}
}
