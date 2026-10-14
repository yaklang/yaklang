package ssadb

import (
	"io/fs"
	"os"
	"path"
	"sort"
	"strings"
	"sync"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
)

// overlayTreeSource materializes one overlay layer's filesystem data on
// first touch. layerName comes from the withLayers chain; returning an
// error degrades the layer: the merged view keeps serving the layers
// above it.
type overlayTreeSource func(layerName string) (filesys_interface.FileSystem, error)

// bodyLoader loads one file's body by its storage path — the path inside
// the owning layer's own prefix space (e.g. "/layerName/dir/file").
type bodyLoader func(contentPath string) ([]byte, error)

// overlayOption configures an overlayFS.
type overlayOption func(*overlayFS)

// withLayers sets the layer chain, oldest first, HEAD INCLUDED and LAST
// (e.g. [base, diff1, diff2]: diff2 is the head). The head names the
// exported root and serves the diff-only view.
func withLayers(layers ...string) overlayOption {
	return func(o *overlayFS) { o.layers = append(o.layers, layers...) }
}

// withLayerTrees supplies the layer tree builder. Required.
func withLayerTrees(src overlayTreeSource) overlayOption {
	return func(o *overlayFS) { o.layerTrees = src }
}

// withBodyLoader supplies the loader overlayFS fills bodies through:
// ReadFile resolves the owning layer's storage path and loads the body
// with it on first read, caching the body under the head-rooted view
// path. Without a loader, ReadFile keeps serving whatever the body cache
// holds (empty for a path nobody wrote yet).
func withBodyLoader(load bodyLoader) overlayOption {
	return func(o *overlayFS) { o.bodyLoader = load }
}

// withFileStatus adds one layer's file-status map (path → status) to the
// deletion resolution: the NEWEST layer's entry per path decides — status
// "-1" (statusDeleted) removes the path from the final view; any other
// status keeps it visible, so a newer layer re-owning a path an older
// layer deleted resurrects it.
func withFileStatus(layerName string, fileStatus map[string]string) overlayOption {
	return func(o *overlayFS) {
		// Store oldest→newest; resolution walks backwards, so append keeps
		// the recipe order.
		o.statusLayers = append(o.statusLayers, overlayStatusLayer{
			layer: layerName,
			files: fileStatus,
		})
	}
}

// overlayFS is a filesystem view over a chain of layers: it merges every
// layer's entries under one head root (the chain's last layer, 换头),
// newest layers shadowing older ones on name conflicts, minus files
// deleted in the final view. It implements the complete
// filesys_interface.FileSystem natively — it embeds nothing and is the
// single owner of the overlay semantics:
//
//   - ReadDir serves the fully expanded view: every layer's entries
//     re-rooted under the head prefix, deduplicated by name (newest wins),
//     minus deleted files, sorted by name.
//   - ReadDirDiff lists only the head layer's own entries (the last-diff
//     view). Callers that care about a "show all files" toggle pick
//     ReadDir vs ReadDirDiff; overlayFS itself does not parse query params.
//   - ReadFile fills bodies on demand: the owning layer's storage path
//     resolves, the body loads through the configured bodyLoader, and the
//     filled body caches in a separate lazy VirtualFS keyed by head paths
//     (including base-layer content pulled up on first read) without
//     polluting the head layer used by ReadDirDiff.
type overlayFS struct {
	layers []string // layer chain oldest→newest, head included and last

	statusLayers []overlayStatusLayer // per-layer file-status maps, recipe order

	layerTrees overlayTreeSource                       // materializes one layer's tree on first touch
	bodyLoader bodyLoader                              // loads one body by its owning layer's storage path
	view       *filesys.VirtualFS                      // lazy body cache under head paths
	layerCache map[string]filesys_interface.FileSystem // built layer trees, by layer name

	mu sync.Mutex // guards layerCache and view writes
}

type overlayStatusLayer struct {
	layer string
	files map[string]string
}

// statusDeleted is the file-status value marking a file its layer removed.
const statusDeleted = "-1"

var (
	_ filesys_interface.FileSystem = (*overlayFS)(nil)
)

// newOverlayFS builds an overlay filesystem. layers and a layer tree
// source are required: a layer that fails to build degrades to the layers
// above it, and without a layer tree source every read errors.
func newOverlayFS(opts ...overlayOption) *overlayFS {
	o := &overlayFS{
		layerCache: make(map[string]filesys_interface.FileSystem),
		view:       filesys.NewVirtualFs(),
	}
	for _, opt := range opts {
		opt(o)
	}
	return o
}

// fillBody loads one body through the configured loader and caches it in
// the lazy view under the head-rooted view path. contentPath keeps the
// owning layer's own prefix — head paths never reach the loader as-is.
func (o *overlayFS) fillBody(viewPath, contentPath string) ([]byte, error) {
	if o.bodyLoader == nil {
		return nil, utils.Errorf("file [%v] not exist", viewPath)
	}
	data, err := o.bodyLoader(contentPath)
	if err != nil {
		return nil, err
	}
	o.mu.Lock()
	o.view.RemoveFileOrDir(viewPath)
	o.view.AddFile(viewPath, string(data))
	o.mu.Unlock()
	return data, nil
}

// head returns the chain's last layer name ("" when the chain is empty).
func (o *overlayFS) head() string {
	if len(o.layers) == 0 {
		return ""
	}
	return o.layers[len(o.layers)-1]
}

// headTree returns the head layer's tree, or nil when the chain is empty or
// the head layer failed to build.
func (o *overlayFS) headTree() filesys_interface.FileSystem {
	return o.treeOf(o.head())
}

// deletedInFinalView reports whether the head-rooted path is a file the
// final overlay view removed: it exists in no view this fs serves.
func (o *overlayFS) deletedInFinalView(headPath string) bool {
	return o.deletedInFinalViewByRel(o.relToHead(headPath))
}

// deletedInFinalViewByRel is deletedInFinalView for a path already relative
// to the head root (ReadDir composes one while merging entries). The status
// maps resolve newest-first — the first layer holding the path decides:
// its status is statusDeleted → removed, anything else → alive (a newer
// layer re-owning a path an older layer deleted resurrects it).
func (o *overlayFS) deletedInFinalViewByRel(rel string) bool {
	for i := len(o.statusLayers) - 1; i >= 0; i-- {
		if status, ok := o.statusLayers[i].files[rel]; ok {
			return status == statusDeleted
		}
	}
	return false
}

// relToHead trims the head prefix and the leading separator from a
// head-rooted path: "" names the head root itself.
func (o *overlayFS) relToHead(headPath string) string {
	trimmed := strings.TrimPrefix(strings.Trim(headPath, "/"), o.head())
	return strings.Trim(trimmed, "/")
}

// layerPath maps a head-rooted path into a layer's own prefix: "/" +
// layerName + rest. Caller paths stay within the head root; each layer's
// tree keeps its own layer-prefixed paths, so bodies resolve through the
// owning layer directly (re-rooting to the head prefix happens only in
// full listings).
func (o *overlayFS) layerPath(layer, headPath string) string {
	rest := strings.TrimPrefix(headPath, "/"+o.head())
	layerPrefix := "/" + layer
	if rest == "" || rest == "/" {
		return layerPrefix
	}
	return layerPrefix + rest
}

// treeOf returns one layer's tree, building it on first touch. A layer
// that fails to build yields nil and is remembered as broken: the merged
// view degrades to the layers above it, bodies fall through to the next
// owner.
func (o *overlayFS) treeOf(layer string) filesys_interface.FileSystem {
	if layer == "" || o.layerTrees == nil {
		return nil
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if tree, ok := o.layerCache[layer]; ok {
		return tree
	}
	tree, err := o.layerTrees(layer)
	if err != nil || tree == nil {
		o.layerCache[layer] = nil // remember the broken layer
		return nil
	}
	o.layerCache[layer] = tree
	return tree
}

// lowerLayerTree resolves a head-rooted path against the newest LOWER layer
// that stores it (head excluded — callers own head lookups). The returned
// path keeps the owning layer's own prefix.
func (o *overlayFS) lowerLayerTree(headPath string) (filesys_interface.FileSystem, string, bool) {
	// Stat, not Exists: some filesystems' Exists goes through Open, which
	// fails on directories ("is a dir"), so base-only directories would
	// never match.
	for i := len(o.layers) - 2; i >= 0; i-- {
		layer := o.layers[i]
		if layer == "" || layer == o.head() {
			continue
		}
		tree := o.treeOf(layer)
		if tree == nil {
			continue
		}
		lp := o.layerPath(layer, headPath)
		if _, err := tree.Stat(lp); err == nil {
			return tree, lp, true
		}
	}
	return nil, "", false
}

// ReadDir lists the fully expanded view of the overlay chain: every layer's
// entries re-rooted under the head prefix, deduplicated by name with newer
// layers shadowing older ones, minus files deleted in the final view.
func (o *overlayFS) ReadDir(headPath string) ([]fs.DirEntry, error) {
	rel := o.relToHead(headPath)
	byName := make(map[string]fs.DirEntry)
	exists := false
	for _, layer := range o.layers {
		tree := o.treeOf(layer)
		if tree == nil {
			continue // a damaged layer degrades to the layers above it
		}
		list, err := tree.ReadDir(o.layerPath(layer, headPath))
		if err != nil {
			continue // this layer doesn't have the directory
		}
		exists = true
		for _, e := range list {
			fullRel := e.Name()
			if rel != "" {
				fullRel = rel + "/" + e.Name()
			}
			if o.deletedInFinalViewByRel(fullRel) && !e.IsDir() {
				continue // deleted in the final overlay view
			}
			byName[e.Name()] = e // layers oldest→head: head entries overwrite
		}
	}
	if !exists {
		// No layer has this directory: the head tree's error says the path
		// exists nowhere (a real but empty dir still reads as empty above).
		if tree := o.headTree(); tree != nil {
			return tree.ReadDir(headPath)
		}
		return nil, utils.Errorf("directory [%v] not exists", headPath)
	}
	entries := make([]fs.DirEntry, 0, len(byName))
	for _, e := range byName {
		entries = append(entries, e)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	return entries, nil
}

// ReadDirDiff lists the diff-only view: the head layer's own entries. A
// directory that lives in lower layers only reads as empty instead of
// erroring, so nodes expanded in the full view don't break the diff view.
func (o *overlayFS) ReadDirDiff(headPath string) ([]fs.DirEntry, error) {
	tree := o.headTree()
	if tree == nil {
		return nil, utils.Errorf("directory [%v] not exists", headPath)
	}
	entries, err := tree.ReadDir(headPath)
	if err == nil {
		return entries, nil
	}
	if _, _, ok := o.lowerLayerTree(headPath); ok {
		return []fs.DirEntry{}, nil
	}
	return nil, err
}

// Stat prefers a cached body in the lazy view (so sizes update after fill);
// otherwise resolves through the owning layer. Files removed in the final
// overlay view read as missing everywhere.
func (o *overlayFS) Stat(headPath string) (fs.FileInfo, error) {
	if info, err := o.view.Stat(headPath); err == nil {
		return info, nil
	}
	if !o.deletedInFinalView(headPath) {
		if tree := o.headTree(); tree != nil {
			if info, err := tree.Stat(headPath); err == nil {
				return info, nil
			}
		}
		if tree, lp, ok := o.lowerLayerTree(headPath); ok {
			return tree.Stat(lp)
		}
	}
	// Not found anywhere (or removed in the final view): a not-found error
	// naming the caller's path.
	return nil, utils.Errorf("file [%v] not exist", headPath)
}

// resolveContentPath finds the owning layer's storage path for a
// head-rooted path. Ownership is head-first: any hit on the head tree —
// including an unfilled stub — owns the path and must never fall through
// to an older layer. Deleted-in-final-view paths error.
func (o *overlayFS) resolveContentPath(headPath string) (string, error) {
	if o.deletedInFinalView(headPath) {
		return "", utils.Errorf("file [%v] not exist", headPath)
	}
	if tree := o.headTree(); tree != nil {
		if _, statErr := tree.Stat(headPath); statErr == nil {
			return headPath, nil
		}
	}
	if _, lp, ok := o.lowerLayerTree(headPath); ok {
		return lp, nil
	}
	return "", utils.Errorf("file [%v] not exist", headPath)
}

// ReadFile serves a cached body from the lazy view when present; otherwise
// resolves the owning layer's storage path, loads the body through the
// configured bodyLoader, and caches it under the head-rooted view path.
func (o *overlayFS) ReadFile(headPath string) ([]byte, error) {
	if data, err := o.view.ReadFile(headPath); err == nil && len(data) > 0 {
		return data, nil
	}
	contentPath, err := o.resolveContentPath(headPath)
	if err != nil {
		return nil, err
	}
	return o.fillBody(headPath, contentPath)
}

// Open reads through the owning-layer resolution and serves the body as a
// read-only file at the caller's (head-rooted) path.
func (o *overlayFS) Open(headPath string) (fs.File, error) {
	data, err := o.ReadFile(headPath)
	if err != nil {
		return nil, err
	}
	return filesys.NewVirtualFile(path.Base(headPath), string(data)), nil
}

func (o *overlayFS) OpenFile(headPath string, flag int, perm os.FileMode) (fs.File, error) {
	return o.Open(headPath)
}

func (o *overlayFS) Exists(headPath string) (bool, error) {
	_, err := o.Stat(headPath)
	return err == nil, err
}

// extOf mirrors VirtualFS's extension rule: the suffix from the last '.'
// in the path, "" when there is none.
func extOf(p string) string {
	if idx := strings.LastIndex(p, "."); idx != -1 {
		return p[idx:]
	}
	return ""
}

// Native path plumbing: the merged view serves every layer's trees, so the
// path rules follow VirtualFS (separator '/', path.Join semantics,
// extOf's extension rule). Writes are unsupported — an overlay view
// is read-only by design; callers mutate the layers' own filesystems.
func (o *overlayFS) GetSeparators() rune         { return '/' }
func (o *overlayFS) Join(paths ...string) string { return path.Join(paths...) }
func (o *overlayFS) Base(p string) string        { return path.Base(p) }
func (o *overlayFS) PathSplit(p string) (string, string) {
	return filesys.SplitWithSeparator(p, '/')
}
func (o *overlayFS) Ext(name string) string { return extOf(name) }
func (o *overlayFS) IsAbs(name string) bool {
	return len(name) > 0 && name[0] == '/'
}
func (o *overlayFS) Getwd() (string, error)          { return "", nil }
func (o *overlayFS) ExtraInfo(string) map[string]any { return nil }
func (o *overlayFS) Rename(string, string) error     { return utils.Error("implement me") }
func (o *overlayFS) Rel(string, string) (string, error) {
	return "", utils.Error("implement me")
}
func (o *overlayFS) WriteFile(string, []byte, os.FileMode) error {
	return utils.Error("implement me")
}
func (o *overlayFS) Delete(string) error { return utils.Error("implement me") }
func (o *overlayFS) MkdirAll(string, os.FileMode) error {
	return utils.Error("implement me")
}
