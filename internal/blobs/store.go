package blobs

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/JoeyJoHa/OCP4-TLS-Routes-Benchmarks/internal/config"
)

const (
	blobsSubdir     = "blobs"
	dirPerm         = 0o750
	filePerm        = 0o644
	copyBufferBytes = 256 * 1024
)

var (
	ErrInvalidName    = errors.New("invalid blob name")
	ErrTooLarge       = errors.New("blob exceeds MAX_BLOB_BYTES")
	ErrNotFound       = errors.New("blob not found")
	ErrSizeMismatch   = errors.New("wrote fewer bytes than requested")
	ErrLengthMismatch = errors.New("body size does not match Content-Length")
)

var (
	blobNamePattern = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)
	copyBufPool     = sync.Pool{
		New: func() any {
			buf := make([]byte, copyBufferBytes)
			return &buf
		},
	}
)

// Info describes a file stored on the PVC.
type Info struct {
	Name     string    `json:"name"`
	Bytes    int64     `json:"bytes"`
	SHA256   string    `json:"sha256,omitempty"`
	Modified time.Time `json:"modified"`
	WriteMs  float64   `json:"write_ms,omitempty"`
	ReadMs   float64   `json:"read_ms,omitempty"`
}

// Store persists blobs under DATA_DIR/blobs.
type Store struct {
	dir      string
	maxBytes int64
	random   io.Reader
}

// NewStore creates the blobs directory. random is typically /dev/urandom.
func NewStore(dataDir string, maxBytes int64, random io.Reader) (*Store, error) {
	if maxBytes <= 0 {
		return nil, fmt.Errorf("max blob bytes must be greater than 0")
	}
	dir := filepath.Join(dataDir, blobsSubdir)
	if err := os.MkdirAll(dir, dirPerm); err != nil {
		return nil, fmt.Errorf("create blobs directory: %w", err)
	}
	return &Store{dir: dir, maxBytes: maxBytes, random: random}, nil
}

// Generate writes size bytes from the random source to a named blob.
func (s *Store) Generate(name string, size int64) (Info, error) {
	if err := validateName(name); err != nil {
		return Info{}, err
	}
	if size <= 0 {
		return Info{}, fmt.Errorf("size must be greater than 0")
	}
	if size > s.maxBytes {
		return Info{}, ErrTooLarge
	}
	if s.random == nil {
		return Info{}, fmt.Errorf("random source is not configured")
	}
	limited := io.LimitReader(s.random, size)
	return s.writeFrom(name, limited, size, true)
}

// Put writes the request body to a named blob. expected is 0 when Content-Length is unknown.
func (s *Store) Put(name string, body io.Reader, expected int64) (Info, error) {
	if err := validateName(name); err != nil {
		return Info{}, err
	}
	if expected > s.maxBytes {
		return Info{}, ErrTooLarge
	}
	limit := s.maxBytes
	if expected > 0 {
		limit = expected
	}
	limited := io.LimitReader(body, limit+1)
	info, err := s.writeFrom(name, limited, expected, false)
	if err != nil {
		return Info{}, err
	}
	if info.Bytes > s.maxBytes {
		if err := removeBestEffort(s, name); err != nil {
			return Info{}, err
		}
		return Info{}, ErrTooLarge
	}
	if expected > 0 && info.Bytes != expected {
		if err := removeBestEffort(s, name); err != nil {
			return Info{}, err
		}
		return Info{}, ErrLengthMismatch
	}
	return info, nil
}

// Open returns a read-only file handle for download.
func (s *Store) Open(name string) (*os.File, Info, error) {
	if err := validateName(name); err != nil {
		return nil, Info{}, err
	}
	path := s.path(name)
	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, Info{}, ErrNotFound
		}
		return nil, Info{}, fmt.Errorf("open blob %s: %w", name, err)
	}
	stat, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, Info{}, fmt.Errorf("stat blob %s: %w", name, err)
	}
	info := Info{
		Name:     name,
		Bytes:    stat.Size(),
		SHA256:   readHashSidecar(s.hashPath(name)),
		Modified: stat.ModTime().UTC(),
	}
	return file, info, nil
}

// List returns blob metadata without reading file contents.
func (s *Store) List() ([]Info, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, fmt.Errorf("list blobs: %w", err)
	}
	out := make([]Info, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || strings.HasSuffix(entry.Name(), ".sha256") {
			continue
		}
		if err := validateName(entry.Name()); err != nil {
			continue
		}
		stat, err := entry.Info()
		if err != nil {
			continue
		}
		out = append(out, Info{
			Name:     entry.Name(),
			Bytes:    stat.Size(),
			SHA256:   readHashSidecar(s.hashPath(entry.Name())),
			Modified: stat.ModTime().UTC(),
		})
	}
	return out, nil
}

// Delete removes a blob and its hash sidecar.
func (s *Store) Delete(name string) error {
	if err := validateName(name); err != nil {
		return err
	}
	err := os.Remove(s.path(name))
	_ = os.Remove(s.hashPath(name))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ErrNotFound
		}
		return fmt.Errorf("delete blob %s: %w", name, err)
	}
	return nil
}

func removeBestEffort(s *Store, name string) error {
	err := s.Delete(name)
	if err == nil || errors.Is(err, ErrNotFound) {
		return nil
	}
	return fmt.Errorf("remove rejected blob %s: %w", name, err)
}

func (s *Store) writeFrom(name string, src io.Reader, expected int64, exact bool) (Info, error) {
	finalPath := s.path(name)
	tmp, err := os.CreateTemp(s.dir, "."+name+".tmp-*")
	if err != nil {
		return Info{}, fmt.Errorf("create temp blob: %w", err)
	}
	tmpName := tmp.Name()
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
	}()

	hash := sha256.New()
	bufPtr := copyBufPool.Get().(*[]byte)
	defer copyBufPool.Put(bufPtr)
	buf := *bufPtr
	started := time.Now()
	written, err := io.CopyBuffer(io.MultiWriter(tmp, hash), src, buf)
	writeMs := msSince(started)
	if err != nil {
		return Info{}, fmt.Errorf("write blob %s: %w", name, err)
	}
	if written > s.maxBytes {
		return Info{}, ErrTooLarge
	}
	if exact && written != expected {
		return Info{}, ErrSizeMismatch
	}
	if err := tmp.Chmod(filePerm); err != nil {
		return Info{}, fmt.Errorf("chmod blob temp: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		return Info{}, fmt.Errorf("sync blob temp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return Info{}, fmt.Errorf("close blob temp: %w", err)
	}
	if err := os.Rename(tmpName, finalPath); err != nil {
		return Info{}, fmt.Errorf("rename blob %s: %w", name, err)
	}

	sum := hex.EncodeToString(hash.Sum(nil))
	if err := os.WriteFile(s.hashPath(name), []byte(sum+"\n"), filePerm); err != nil {
		return Info{}, fmt.Errorf("write blob hash %s: %w", name, err)
	}
	stat, err := os.Stat(finalPath)
	modified := time.Now().UTC()
	if err == nil {
		modified = stat.ModTime().UTC()
	}
	return Info{
		Name:     name,
		Bytes:    written,
		SHA256:   sum,
		Modified: modified,
		WriteMs:  writeMs,
	}, nil
}

func (s *Store) path(name string) string {
	return filepath.Join(s.dir, name)
}

func (s *Store) hashPath(name string) string {
	return filepath.Join(s.dir, name+".sha256")
}

func validateName(name string) error {
	if name == "" || len(name) > config.MaxBlobNameLength {
		return ErrInvalidName
	}
	if strings.Contains(name, "/") || strings.Contains(name, "\\") || strings.Contains(name, "..") {
		return ErrInvalidName
	}
	if !blobNamePattern.MatchString(name) {
		return ErrInvalidName
	}
	return nil
}

func readHashSidecar(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func msSince(started time.Time) float64 {
	return float64(time.Since(started).Microseconds()) / 1000.0
}
