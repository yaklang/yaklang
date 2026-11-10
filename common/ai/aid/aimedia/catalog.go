package aimedia

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const MaxResources = 4096

// Resource is an immutable image reference and its origin. It has no prompt
// visibility, pinning, analysis cache or model accounting state. Timeline owns
// what the agent remembers; a Catalog only makes retained bytes addressable.
type Resource struct {
	ID    string `json:"id"`
	Image *Image `json:"image"`
}

type Catalog struct {
	mu    sync.Mutex
	root  string
	items []Resource
	owner string
}

func OpenCatalog(root string) (*Catalog, error) {
	if strings.TrimSpace(root) == "" {
		return nil, fmt.Errorf("image resource directory is empty")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	c := &Catalog{root: abs}
	f, err := os.Open(filepath.Join(abs, "index.json"))
	if os.IsNotExist(err) {
		return c, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, 32*1024*1024+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > 32*1024*1024 {
		return nil, fmt.Errorf("image resource index exceeds limit")
	}
	// Older experimental journals contained visibility and summaries. Ignore
	// those fields: reading a resource index must never restore prompt history.
	if len(raw) > 0 && raw[0] == '[' {
		err = json.Unmarshal(raw, &c.items)
	} else {
		var saved catalogIndex
		err = json.Unmarshal(raw, &saved)
		c.items, c.owner = saved.Items, saved.Owner
	}
	if err != nil {
		return nil, fmt.Errorf("invalid image resource index: %w", err)
	}
	if len(c.items) > MaxResources {
		return nil, fmt.Errorf("too many image resources")
	}
	for _, r := range c.items {
		if r.ID == "" || r.Image == nil {
			return nil, fmt.Errorf("invalid image resource")
		}
		rel, err := filepath.Rel(abs, r.Image.Path)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return nil, fmt.Errorf("image reference is outside the session store")
		}
	}
	return c, nil
}

type catalogIndex struct {
	Version int        `json:"version"`
	Items   []Resource `json:"items"`
	Owner   string     `json:"owner_session,omitempty"`
}

func cloneResource(r Resource) Resource {
	if r.Image != nil {
		img := *r.Image
		r.Image = &img
	}
	return r
}

func (c *Catalog) Root() string { return c.root }

func (c *Catalog) Owner() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.owner
}

func (c *Catalog) BindOwner(session string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.owner != "" && c.owner != session {
		return fmt.Errorf("image resources belong to a different session")
	}
	if c.owner == session {
		return nil
	}
	c.owner = session
	if err := c.saveLocked(); err != nil {
		c.owner = ""
		return err
	}
	return nil
}

func (c *Catalog) Snapshot() []Resource {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]Resource, len(c.items))
	for i, r := range c.items {
		out[i] = cloneResource(r)
	}
	return out
}

func (c *Catalog) Find(id string) (Resource, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for i := len(c.items) - 1; i >= 0; i-- {
		r := c.items[i]
		if r.ID == id || r.Image.ID == id {
			return cloneResource(r), nil
		}
	}
	return Resource{}, fmt.Errorf("image %s is not available in this session", id)
}

func (c *Catalog) Register(ctx context.Context, event string, input *Image) (Resource, error) {
	if input == nil {
		return Resource{}, fmt.Errorf("image is nil")
	}
	if err := ctx.Err(); err != nil {
		return Resource{}, err
	}
	hash := sha256.Sum256([]byte(event + "\x00" + input.ID))
	id := "obs-" + hex.EncodeToString(hash[:16])
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, r := range c.items {
		if r.ID == id {
			return cloneResource(r), nil
		}
	}
	if len(c.items) >= MaxResources {
		return Resource{}, fmt.Errorf("session image resource limit reached")
	}
	store, err := NewStore(filepath.Join(c.root, "originals"))
	if err != nil {
		return Resource{}, err
	}
	img, err := store.Save(ctx, input)
	if err != nil {
		return Resource{}, err
	}
	r := Resource{ID: id, Image: img}
	c.items = append(c.items, r)
	if err := c.saveLocked(); err != nil {
		c.items = c.items[:len(c.items)-1]
		return Resource{}, err
	}
	return cloneResource(r), nil
}

func (c *Catalog) saveLocked() error {
	if err := os.MkdirAll(c.root, 0o700); err != nil {
		return err
	}
	raw, err := json.Marshal(catalogIndex{Version: 2, Items: c.items, Owner: c.owner})
	if err != nil {
		return err
	}
	if len(raw) > 32*1024*1024 {
		return fmt.Errorf("image resource index exceeds limit")
	}
	f, err := os.CreateTemp(c.root, ".index-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	_, writeErr := f.Write(raw)
	closeErr := f.Close()
	if writeErr != nil {
		return writeErr
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(f.Name(), filepath.Join(c.root, "index.json"))
}

// Fork copies resource visibility, never prompt selections or observations.
// Immutable bytes are linked into the new owner's store when possible.
func (c *Catalog) Fork(ctx context.Context, root string) (*Catalog, error) {
	next, err := OpenCatalog(root)
	if err != nil {
		return nil, err
	}
	if len(next.Snapshot()) > 0 {
		return next, nil
	}
	store, err := NewStore(filepath.Join(root, "originals"))
	if err != nil {
		return nil, err
	}
	for _, r := range c.Snapshot() {
		r.Image, err = forkImage(ctx, store, r.Image)
		if err != nil {
			return nil, err
		}
		next.items = append(next.items, r)
	}
	return next, next.saveLocked()
}

func forkImage(ctx context.Context, store *Store, img *Image) (*Image, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	info, err := os.Lstat(img.Path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("inherited image is not a regular file")
	}
	if err := os.MkdirAll(store.root, 0o700); err != nil {
		return nil, err
	}
	ref := *img
	ref.Path = filepath.Join(store.root, img.SHA256+filepath.Ext(img.Path))
	ref.data = nil
	if err := os.Link(img.Path, ref.Path); err != nil && !os.IsExist(err) {
		return store.Save(ctx, img)
	}
	return &ref, nil
}
