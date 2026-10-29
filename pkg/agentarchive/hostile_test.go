package agentarchive

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
)

func TestArchiveCentralDirectoryAndCRC(t *testing.T) {
	for _, test := range []string{"false-count", "directory-payload", "encrypted", "crc", "entries", "directory-size"} {
		t.Run(test, func(t *testing.T) {
			var b bytes.Buffer
			writer := zip.NewWriter(&b)
			h := &zip.FileHeader{Name: "aX", Method: zip.Store}
			f, _ := writer.CreateHeader(h)
			f.Write([]byte("body"))
			writer.Close()
			data := b.Bytes()
			want := ErrInvalid
			switch test {
			case "false-count":
				binary.LittleEndian.PutUint16(data[len(data)-14:], 0)
				binary.LittleEndian.PutUint16(data[len(data)-12:], 0)
			case "directory-payload":
				data = bytes.ReplaceAll(data, []byte("aX"), []byte("a/"))
			case "encrypted":
				at := bytes.Index(data, []byte{'P', 'K', 1, 2})
				data[at+8] |= 1
			case "crc":
				r, e := zip.NewReader(bytes.NewReader(data), int64(len(data)))
				if e != nil {
					t.Fatal(e)
				}
				offset, e := r.File[0].DataOffset()
				if e != nil {
					t.Fatal(e)
				}
				data[offset] ^= 1
			case "entries":
				names := make([]string, 1001)
				bodies := make([][]byte, 1001)
				for i := range names {
					names[i] = strings.Repeat("a", i+1) + "/"
				}
				data = zipFixture(t, names, bodies)
				want = ErrTooLarge
			case "directory-size":
				b.Reset()
				writer = zip.NewWriter(&b)
				for range 33 {
					h := &zip.FileHeader{Name: strings.Repeat("a", 1+b.Len()%100), Method: zip.Store, Comment: strings.Repeat("x", 65000)}
					writer.CreateHeader(h)
				}
				writer.Close()
				data = b.Bytes()
				want = ErrTooLarge
			}
			store, _ := New(t.TempDir())
			if _, e := store.SaveZIP(context.Background(), testKey, bytes.NewReader(data)); !errors.Is(e, want) {
				t.Fatalf("want %v got %v", want, e)
			}
		})
	}
}

func TestRejectSlashSuffixedSpecialEntries(t *testing.T) {
	for _, mode := range []os.FileMode{os.ModeSymlink | 0777, os.ModeNamedPipe | 0600, os.ModeDevice | 0600, os.ModeSocket | 0600, 0644} {
		t.Run(mode.String(), func(t *testing.T) {
			var b bytes.Buffer
			w := zip.NewWriter(&b)
			h := &zip.FileHeader{Name: "entry/", Method: zip.Store}
			h.SetMode(mode)
			if _, e := w.CreateHeader(h); e != nil {
				t.Fatal(e)
			}
			f, _ := w.Create("normal.txt")
			f.Write([]byte("content"))
			w.Close()
			s, _ := New(t.TempDir())
			if _, e := s.SaveZIP(context.Background(), testKey, bytes.NewReader(b.Bytes())); !errors.Is(e, ErrInvalid) {
				t.Fatalf("accepted explicit special or contradictory type %v: %v", mode, e)
			}
		})
	}
}

func TestPreflightRejectsAlternateEOCD(t *testing.T) {
	header := make([]byte, 47)
	binary.LittleEndian.PutUint32(header, 0x02014b50)
	binary.LittleEndian.PutUint16(header[4:], 20)
	binary.LittleEndian.PutUint16(header[6:], 20)
	binary.LittleEndian.PutUint16(header[28:], 1)
	header[46] = 'x'
	var b bytes.Buffer
	w := zip.NewWriter(&b)
	f, e := w.CreateHeader(&zip.FileHeader{Name: "payload.bin", Method: zip.Store})
	if e != nil {
		t.Fatal(e)
	}
	for range 1001 {
		if _, e = f.Write(header); e != nil {
			t.Fatal(e)
		}
	}
	w.Close()
	original := b.Bytes()
	r, e := zip.NewReader(bytes.NewReader(original), int64(len(original)))
	if e != nil {
		t.Fatal(e)
	}
	offset, e := r.File[0].DataOffset()
	if e != nil {
		t.Fatal(e)
	}
	fake := make([]byte, 22)
	binary.LittleEndian.PutUint32(fake, 0x06054b50)
	binary.LittleEndian.PutUint16(fake[8:], 1001)
	binary.LittleEndian.PutUint16(fake[10:], 1001)
	binary.LittleEndian.PutUint32(fake[12:], uint32(int64(len(original))-offset))
	binary.LittleEndian.PutUint32(fake[16:], uint32(offset))
	binary.LittleEndian.PutUint16(original[len(original)-2:], 23)
	data := append(append(append([]byte(nil), original...), fake...), 0)
	parsed, e := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if e != nil || len(parsed.File) != 1001 {
		t.Fatalf("fixture must reproduce the stdlib alternate directory: %v", e)
	}
	if e = preflight(bytes.NewReader(data), int64(len(data))); e == nil {
		t.Fatal("preflight accepted a different EOCD than archive/zip")
	}
}
func TestArchiveContentAndFileBoundaries(t *testing.T) {
	for _, count := range []int{5, 6} {
		t.Run(strings.Repeat("x", count), func(t *testing.T) {
			s, _ := New(t.TempDir())
			files := []InputFile{}
			data := bytes.Repeat([]byte{'a'}, MaxFile)
			for i := 0; i < count; i++ {
				files = append(files, InputFile{Path: strings.Repeat("a", i+1), Open: func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(data)), nil }})
			}
			_, e := s.SaveFiles(context.Background(), testKey, files)
			if count == 5 && e != nil {
				t.Fatal(e)
			}
			if count == 6 && !errors.Is(e, ErrTooLarge) {
				t.Fatal(e)
			}
		})
	}
}

func TestZIP64AndPathBoundary(t *testing.T) {
	data := zipFixture(t, []string{"a"}, [][]byte{[]byte("ok")})
	end := len(data) - 22
	record := make([]byte, 56)
	binary.LittleEndian.PutUint32(record, 0x06064b50)
	binary.LittleEndian.PutUint64(record[4:], 44)
	binary.LittleEndian.PutUint16(record[12:], 45)
	binary.LittleEndian.PutUint16(record[14:], 45)
	binary.LittleEndian.PutUint64(record[24:], 1)
	binary.LittleEndian.PutUint64(record[32:], 1)
	binary.LittleEndian.PutUint64(record[40:], uint64(binary.LittleEndian.Uint32(data[end+12:])))
	binary.LittleEndian.PutUint64(record[48:], uint64(binary.LittleEndian.Uint32(data[end+16:])))
	locator := make([]byte, 20)
	binary.LittleEndian.PutUint32(locator, 0x07064b50)
	binary.LittleEndian.PutUint64(locator[8:], uint64(end))
	binary.LittleEndian.PutUint32(locator[16:], 1)
	ending := append([]byte(nil), data[end:]...)
	binary.LittleEndian.PutUint16(ending[8:], 65535)
	binary.LittleEndian.PutUint16(ending[10:], 65535)
	binary.LittleEndian.PutUint32(ending[12:], 0xffffffff)
	binary.LittleEndian.PutUint32(ending[16:], 0xffffffff)
	input := append(append(append(append([]byte(nil), data[:end]...), record...), locator...), ending...)
	s, _ := New(t.TempDir())
	if _, e := s.SaveZIP(context.Background(), testKey, bytes.NewReader(input)); e != nil {
		t.Fatal(e)
	}
	if e := ValidatePath(strings.Repeat("a", 1024), false); e != nil {
		t.Fatal(e)
	}
	if e := ValidatePath(strings.Repeat("a", 1025), false); !errors.Is(e, ErrTooLarge) {
		t.Fatal("path limit must be 413 category", e)
	}
}
