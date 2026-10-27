package gitstore

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"strings"

	"github.com/go-git/go-git/v5/plumbing/object"
)

// Older uploads can store an entire skill ZIP instead of its individual files.
func readArchivedSkillMarkdown(tree *object.Tree) ([]byte, error) {
	files := tree.Files()
	defer files.Close()
	archive, err := files.Next()
	if err != nil {
		return nil, err
	}
	if !strings.HasSuffix(strings.ToLower(archive.Name), ".zip") {
		return nil, object.ErrFileNotFound
	}
	if _, err := files.Next(); err != io.EOF {
		if err != nil {
			return nil, err
		}
		return nil, object.ErrFileNotFound
	}
	if archive.Size > 50*1024*1024 {
		return nil, fmt.Errorf("ZIP too large (max 50MB)")
	}
	content, err := archive.Contents()
	if err != nil {
		return nil, err
	}
	reader, err := zip.NewReader(bytes.NewReader([]byte(content)), int64(len(content)))
	if err != nil {
		return nil, fmt.Errorf("read skill ZIP: %w", err)
	}
	var markdown *zip.File
	var candidates int
	for _, entry := range reader.File {
		if entry.Name == "SKILL.md" {
			markdown, candidates = entry, 1
			break
		}
		if fs.ValidPath(entry.Name) && strings.Count(entry.Name, "/") == 1 && strings.HasSuffix(entry.Name, "/SKILL.md") {
			markdown = entry
			candidates++
		}
	}
	if markdown == nil {
		return nil, object.ErrFileNotFound
	}
	if candidates > 1 {
		return nil, fmt.Errorf("multiple SKILL.md files in ZIP")
	}
	const limit = 200 * 1024
	if markdown.UncompressedSize64 > limit {
		return nil, fmt.Errorf("file too large (max 200KB)")
	}
	file, err := markdown.Open()
	if err != nil {
		return nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return nil, err
	}
	if len(data) > limit {
		return nil, fmt.Errorf("file too large (max 200KB)")
	}
	return data, nil
}
