package gitstore

import (
	"archive/zip"
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestGetFileSkillMarkdown(t *testing.T) {
	tests := []struct {
		name      string
		entries   []string
		content   string
		direct    bool
		wantError string
	}{
		{name: "root markdown in ZIP", entries: []string{"SKILL.md"}, content: "# Root skill"},
		{name: "wrapped markdown in ZIP", entries: []string{"demo/references/notes.md", "demo/SKILL.md"}, content: "# Wrapped skill"},
		{name: "direct markdown takes priority", entries: []string{"demo/SKILL.md"}, content: "# Archived skill", direct: true},
		{name: "missing markdown", entries: []string{"demo/README.md"}, content: "# Readme", wantError: "file not found"},
		{name: "invalid wrapper", entries: []string{"../SKILL.md"}, content: "# Invalid", wantError: "file not found"},
		{name: "ambiguous wrappers", entries: []string{"one/SKILL.md", "two/SKILL.md"}, content: "# Ambiguous", wantError: "multiple SKILL.md"},
		{name: "expanded markdown size limit", entries: []string{"demo/SKILL.md"}, content: strings.Repeat("x", 200*1024+1), wantError: "file too large"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var archive bytes.Buffer
			writer := zip.NewWriter(&archive)
			for _, name := range tt.entries {
				entry, err := writer.Create(name)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := entry.Write([]byte(tt.content)); err != nil {
					t.Fatal(err)
				}
			}
			if err := writer.Close(); err != nil {
				t.Fatal(err)
			}
			store, err := New(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			files := map[string][]byte{"demo.zip": archive.Bytes()}
			want := tt.content
			if tt.direct {
				files["SKILL.md"] = []byte("# Direct skill")
				want = "# Direct skill"
			}
			_, err = store.Publish(context.Background(), PublishOpts{
				Owner: "tester", Slug: "demo", Version: "1.0.0", Files: files,
				Author: "tester", Email: "tester@example.test",
			})
			if err != nil {
				t.Fatal(err)
			}
			got, err := store.GetFile("tester", "demo", "1.0.0", "SKILL.md")
			if tt.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantError) {
					t.Fatalf("GetFile error = %v, want %q", err, tt.wantError)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != want {
				t.Fatalf("GetFile = %q, want %q", got, want)
			}
		})
	}
}
