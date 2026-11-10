package aimedia

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Store saves immutable originals under a runtime-owned directory. Its owner
// controls retention together with other task artifacts; there is no global
// cache that can leak resources between unrelated sessions.
type Store struct {
	root string
	mu   sync.Mutex
}

const MaxStoreBytes = int64(1024 * 1024 * 1024)

func NewStore(root string) (*Store, error) {
	if strings.TrimSpace(root) == "" {
		return nil, fmt.Errorf("image store directory is empty")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	return &Store{root: abs}, nil
}

// Save copies validated bytes into the store and returns a serializable
// reference. Identical content shares a file; observation metadata stays local
// to each returned reference. Caller-owned Image values are never mutated.
func (s *Store) Save(ctx context.Context, input *Image) (*Image, error) {
	if s == nil {
		return nil, fmt.Errorf("image store is unavailable")
	}
	raw, err := input.Bytes(ctx)
	if err != nil {
		return nil, err
	}
	validated, err := FromBytes(ctx, raw, input.MIMEType)
	if err != nil {
		return nil, err
	}
	validated.Source = input.Source
	validated.Path = filepath.Join(s.root, validated.SHA256+"."+strings.TrimPrefix(validated.MIMEType, "image/"))
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(s.root, 0o700); err != nil {
		return nil, fmt.Errorf("create image store: %w", err)
	}
	if _, err := os.Lstat(validated.Path); err == nil {
		info, err := os.Lstat(validated.Path)
		if err != nil || !info.Mode().IsRegular() {
			return nil, fmt.Errorf("existing image resource is not a regular file")
		}
		ref := *validated
		ref.data = nil
		if _, err := ref.Bytes(ctx); err != nil {
			return nil, fmt.Errorf("existing image resource is damaged: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	} else {
		entries, err := os.ReadDir(s.root)
		if err != nil {
			return nil, err
		}
		used := int64(0)
		for _, entry := range entries {
			if strings.HasPrefix(entry.Name(), ".image-") {
				continue
			}
			info, err := entry.Info()
			if err != nil {
				return nil, err
			}
			if info.Mode().IsRegular() {
				used += info.Size()
			}
		}
		if used+int64(len(raw)) > MaxStoreBytes {
			return nil, fmt.Errorf("image store reached its %d-byte retention limit; remove old session artifacts or start a new session", MaxStoreBytes)
		}
		f, err := os.CreateTemp(s.root, ".image-*")
		if err != nil {
			return nil, err
		}
		defer os.Remove(f.Name())
		_, writeErr := f.Write(raw)
		closeErr := f.Close()
		if writeErr != nil {
			return nil, writeErr
		}
		if closeErr != nil {
			return nil, closeErr
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		// Link installs a fully written file without overwriting an existing
		// resource, even when two Store instances use the same artifact root.
		if err := os.Link(f.Name(), validated.Path); err != nil {
			if !os.IsExist(err) {
				return nil, err
			}
			ref := *validated
			ref.data = nil
			if _, err := ref.Bytes(ctx); err != nil {
				return nil, err
			}
		}
	}
	validated.data = nil
	return validated, nil
}
