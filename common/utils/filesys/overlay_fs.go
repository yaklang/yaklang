package filesys

import (
	"io/fs"
	"os"
	"path"
	"sort"
	"strings"
	"sync"

	"github.com/yaklang/yaklang/common/utils"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
)

// OverlayTreeSource materializes one overlay layer's filesystem data on
// first touch. layerName comes from the recipe (OverlayOption.Layers);
// returning an error degrades the layer: the merged view keeps serving the
// layers above it.
type OverlayTreeSource func(layerName string) (fi.FileSystem, error)

// ContentSource loads one file's body by its path in the owning layer's
// tree. OverlayFS uses it to fill empty stubs on first read and write the
// body back into that layer's tree (not a wrapper filesystem).
type ContentSource func(path string) ([]byte, error)

// OverlayOption configures an OverlayFS.
type OverlayOption func(*OverlayFS)

// WithLayers sets the layer chain, oldest first, HEAD INCLUDED and LAST
// (e.g. [base, diff1, diff2]: diff2 is the head). The head names the
// exported root and serves the diff-only view.
func WithLayers(layers ...string) OverlayOption {
	return func(o *OverlayFS) { o.layers = append(o.layers, layers...) }
}

// WithLayerTrees supplies the layer tree builder. Required.
func WithLayerTrees(src OverlayTreeSource) OverlayOption {
	return func(o *OverlayFS) { o.layerTrees = src }
}

// WithDeletedPaths sets the paths deleted in the final overlay view
// (leading "/" trimmed, same normalization as OverlayOption's file-status
// maps): the merged view hides them and bodies resolve them as missing.
func WithDeletedPaths(paths []string) OverlayOption {
	return func(o *OverlayFS) {
		for _, p := range paths {
			o.deleted[strings.Trim(p, "/")] = struct{}{}
		}
	}
}

// WithFileStatus adds one layer's file-status map (path → status) to the
// deletion resolution: the NEWEST layer's entry per path decides — any
// status other than the delete one keeps the file visible, so a newer
// layer re-owning a path an older layer deleted resurrects it.
func WithFileStatus(layerName string, fileStatus map[string]string) OverlayOption {
	return func(o *OverlayFS) {
		// Store oldest→newest; resolution walks backwards, so append keeps
		// the recipe order.
		o.statusLayers = append(o.statusLayers, overlayStatusLayer{
			layer: layerName,
			files: fileStatus,
		})
	}
}

// WithDeleteStatus sets which status value marks a deleted file. Default
// "-1".
func WithDeleteStatus(status string) OverlayOption {
	return func(o *OverlayFS) { o.deleteStatus = status }
}

// WithHeadSeparator sets the rune separating head-root segments in caller
// paths. Default '/'.
func WithHeadSeparator(sep rune) OverlayOption {
	return func(o *OverlayFS) { o.sep = sep }
}

// WithContentSource supplies the body loader for empty stubs: on first
// ReadFile of a stub, OverlayFS loads the body and writes it back into the
// owning layer's tree so repeated reads are memory hits and Stat sizes
// update. Without a source, stubs stay empty (structure-only trees).
func WithContentSource(src ContentSource) OverlayOption {
	return func(o *OverlayFS) { o.readContent = src }
}

// OverlayFS is a filesystem view over a chain of layers: it merges every
// layer's entries under one head root (the chain's last layer, 换头),
// newest layers shadowing older ones on name conflicts, minus files
// deleted in the final view. It implements the complete fi.FileSystem
// natively — it embeds nothing and is the single owner of the overlay
// semantics:
//
//   - ReadDir serves the fully expanded view: every layer's entries
//     re-rooted under the head prefix, deduplicated by name (newest wins),
//     minus deleted files, sorted by name.
//   - ReadDirDiff lists only the head layer's own entries (the last-diff
//     view). Callers that care about a "show all files" toggle pick
//     ReadDir vs ReadDirDiff; OverlayFS itself does not parse query params.
//   - ReadFile/Open/Stat resolve each path through its OWNING layer, head
//     first: any hit on the head tree — including an unfilled stub — means
//     the head owns the path and must never fall through to an older
//     layer's copy of the same path.
type OverlayFS struct {
	sep     rune           // head-root separator, exported paths use it
	layers  []string       // layer chain oldest→newest, head included and last
	deleted map[string]struct{} // files deleted in the final overlay view

	statusLayers []overlayStatusLayer // per-layer file-status maps, recipe order
	deleteStatus string               // the status value marking a deleted file

	layerTrees  OverlayTreeSource // materializes one layer's tree on first touch
	readContent ContentSource     // optional: fills empty stubs on first read

	mu         sync.Mutex               // guards layerTreeCache and stub fills
	layerCache map[string]fi.FileSystem // built layer trees, by layer name
}

type overlayStatusLayer struct {
	layer string
	files map[string]string
}

var _ fi.FileSystem = (*OverlayFS)(nil)

// NewOverlayFS builds an overlay filesystem. layers and a layer tree source
// are required; without a layer tree source the overlay serves nothing
// (every read errors, listings degrade to the layers that did build).
func NewOverlayFS(opts ...OverlayOption) *OverlayFS {
	o := &OverlayFS{
		sep:         '/',
		deleted:     make(map[string]struct{}),
		deleteStatus: "-1",
		layerCache:  make(map[string]fi.FileSystem),
	}
	for _, opt := range opts {
		opt(o)
	}
	return o
}

// head returns the chain's last layer name ("" when the chain is empty).
func (o *OverlayFS) head() string {
	if len(o.layers) == 0 {
		return ""
	}
	return o.layers[len(o.layers)-1]
}

// headTree returns the head layer's tree, or nil when the chain is empty or
// the head layer failed to build.
func (o *OverlayFS) headTree() fi.FileSystem {
	return o.treeOf(o.head())
}

// deletedInFinalView reports whether the head-rooted path is a file the
// final overlay view removed: it exists in no view this fs serves.
func (o *OverlayFS) deletedInFinalView(headPath string) bool {
	return o.deletedInFinalViewByRel(o.relToHead(headPath))
}

// deletedInFinalViewByRel is deletedInFinalView for a path already relative
// to the head root (ReadDir composes one while merging entries). Explicit
// deletions always win; otherwise the status maps resolve newest-first —
// the first layer holding the path decides: its status is the delete one →
// removed, anything else → alive (a newer layer re-owning a path an older
// layer deleted resurrects it).
func (o *OverlayFS) deletedInFinalViewByRel(rel string) bool {
	if _, ok := o.deleted[rel]; ok {
		return true
	}
	for i := len(o.statusLayers) - 1; i >= 0; i-- {
		if status, ok := o.statusLayers[i].files[rel]; ok {
			return status == o.deleteStatus
		}
	}
	return false
}

// relToHead trims the head prefix and the leading separator from a
// head-rooted path: "" names the head root itself.
func (o *OverlayFS) relToHead(headPath string) string {
	trimmed := strings.TrimPrefix(strings.Trim(headPath, string(o.sep)), o.head())
	return strings.Trim(trimmed, string(o.sep))
}

// layerPath maps a head-rooted path into a layer's own prefix: "/" +
// layerName + rest. Caller paths stay within the head root; each layer's
// tree keeps its own layer-prefixed paths, so bodies resolve through the
// owning layer directly (re-rooting to the head prefix happens only in
// full listings).
func (o *OverlayFS) layerPath(layer, headPath string) string {
	rest := strings.TrimPrefix(headPath, string(o.sep)+o.head())
	layerPrefix := string(o.sep) + layer
	if rest == "" || rest == string(o.sep) {
		return layerPrefix
	}
	return layerPrefix + rest
}

// treeOf returns one layer's tree, building it on first touch. A layer
// that fails to build yields nil and is remembered as broken: the merged
// view degrades to the layers above it, bodies fall through to the next
// owner.
func (o *OverlayFS) treeOf(layer string) fi.FileSystem {
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
func (o *OverlayFS) lowerLayerTree(headPath string) (fi.FileSystem, string, bool) {
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
func (o *OverlayFS) ReadDir(headPath string) ([]fs.DirEntry, error) {
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
				fullRel = rel + string(o.sep) + e.Name()
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
func (o *OverlayFS) ReadDirDiff(headPath string) ([]fs.DirEntry, error) {
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

// Stat resolves the path through its owning layer, head first; files
// removed in the final overlay view read as missing everywhere.
func (o *OverlayFS) Stat(headPath string) (fs.FileInfo, error) {
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

// ReadFile resolves the path through its OWNING layer, head first: any hit
// on the head tree — including an unfilled stub — owns the path and must
// never fall through to an older layer's copy of the same path. Ownership is
// decided by Stat, so a stub whose content source fails still reports the
// load error instead of silently serving an older layer's stale copy.
// Bodies fill into the owning layer's tree when a ContentSource is set.
func (o *OverlayFS) ReadFile(headPath string) ([]byte, error) {
	if o.deletedInFinalView(headPath) {
		return nil, utils.Errorf("file [%v] not exist", headPath)
	}
	if tree := o.headTree(); tree != nil {
		if _, statErr := tree.Stat(headPath); statErr == nil {
			return o.readTreeFile(tree, headPath)
		}
	}
	if tree, lp, ok := o.lowerLayerTree(headPath); ok {
		return o.readTreeFile(tree, lp)
	}
	return nil, utils.Errorf("file [%v] not exist", headPath)
}

// readTreeFile serves one body out of an owning tree: the filled memory
// hit, else a fill through the content source into that tree (replacing
// the empty stub). The content fetch runs outside o.mu — a slow load never
// blocks listings; only the cheap stub replacement takes the lock.
func (o *OverlayFS) readTreeFile(tree fi.FileSystem, path string) ([]byte, error) {
	if data, err := tree.ReadFile(path); err == nil && len(data) > 0 {
		return data, nil
	}
	if o.readContent == nil {
		return tree.ReadFile(path)
	}
	data, err := o.readContent(path)
	if err != nil {
		return nil, err
	}
	o.mu.Lock()
	_ = tree.Delete(path)
	_ = tree.WriteFile(path, data, 0o644)
	o.mu.Unlock()
	return data, nil
}

// Open reads the path through the owning-layer resolution and serves the
// body as a read-only file at the caller's (head-rooted) path.
func (o *OverlayFS) Open(headPath string) (fs.File, error) {
	data, err := o.ReadFile(headPath)
	if err != nil {
		return nil, err
	}
	return NewVirtualFile(path.Base(headPath), string(data)), nil
}

func (o *OverlayFS) OpenFile(headPath string, flag int, perm os.FileMode) (fs.File, error) {
	return o.Open(headPath)
}

func (o *OverlayFS) Exists(headPath string) (bool, error) {
	_, err := o.Stat(headPath)
	return err == nil, err
}

// Native path plumbing: the merged view serves every layer's trees, so the
// path rules follow VirtualFS (separator '/', path.Join semantics,
// getExtension's extension rule). Writes are unsupported — an overlay view
// is read-only by design; callers mutate the layers' own filesystems.
func (o *OverlayFS) GetSeparators() rune         { return o.sep }
func (o *OverlayFS) Join(paths ...string) string { return path.Join(paths...) }
func (o *OverlayFS) Base(p string) string        { return path.Base(p) }
func (o *OverlayFS) PathSplit(p string) (string, string) {
	return SplitWithSeparator(p, o.sep)
}
func (o *OverlayFS) Ext(name string) string { return getExtension(name) }
func (o *OverlayFS) IsAbs(name string) bool {
	return len(name) > 0 && name[0] == byte(o.sep)
}
func (o *OverlayFS) Getwd() (string, error)          { return "", nil }
func (o *OverlayFS) ExtraInfo(string) map[string]any { return nil }
func (o *OverlayFS) Rename(string, string) error     { return utils.Error("implement me") }
func (o *OverlayFS) Rel(string, string) (string, error) {
	return "", utils.Error("implement me")
}
func (o *OverlayFS) WriteFile(string, []byte, os.FileMode) error {
	return utils.Error("implement me")
}
func (o *OverlayFS) Delete(string) error { return utils.Error("implement me") }
func (o *OverlayFS) MkdirAll(string, os.FileMode) error {
	return utils.Error("implement me")
}