package blobs

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestGenerateUploadDownload(t *testing.T) {
	dir := t.TempDir()
	payload := bytes.Repeat([]byte("n"), 4096)
	store, err := NewStore(dir, 1024*1024, bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	generated, err := store.Generate("blob-4k.bin", 4096)
	if err != nil {
		t.Fatal(err)
	}
	if generated.Bytes != 4096 || generated.SHA256 == "" || generated.WriteMs < 0 {
		t.Fatalf("unexpected generate result: %+v", generated)
	}

	file, info, err := store.Open("blob-4k.bin")
	if err != nil {
		t.Fatal(err)
	}
	read, err := io.ReadAll(file)
	_ = file.Close()
	if err != nil {
		t.Fatal(err)
	}
	if int64(len(read)) != info.Bytes {
		t.Fatalf("read %d want %d", len(read), info.Bytes)
	}

	uploaded, err := store.Put("upload.bin", bytes.NewReader(payload), int64(len(payload)))
	if err != nil {
		t.Fatal(err)
	}
	if uploaded.Name != "upload.bin" {
		t.Fatalf("name=%s", uploaded.Name)
	}
	list, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("list=%d", len(list))
	}
	if err := store.Delete("upload.bin"); err != nil {
		t.Fatal(err)
	}
	if err := store.Delete("upload.bin"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected not found, got %v", err)
	}
}

func TestRejectsInvalidNameAndOversize(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir, 16, bytes.NewReader(bytes.Repeat([]byte("x"), 64)))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Generate("../etc/passwd", 8); !errors.Is(err, ErrInvalidName) {
		t.Fatalf("path traversal: %v", err)
	}
	if _, err := store.Generate("ok.bin", 64); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("oversize generate: %v", err)
	}
	if _, err := store.Put("ok.bin", strings.NewReader(strings.Repeat("y", 32)), 32); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("oversize put: %v", err)
	}
}
