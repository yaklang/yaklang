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

// overlayLayer is one link in the chain: the layer (program) name plus its
// file-status map (IrProgram.FileHashMap), nil when the layer recorded
// none. The chain's order carries everything: newer layers shadow older
// ones and win the deletion resolution.
type overlayLayer struct {
	name  string
	files map[string]string
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
//     resolves, the body loads from the database through
//     readIrSourceFileContent, and the filled body replaces the owning
//     layer tree's stub in place — so later Stats and listings report the
//     real size. The diff view stays clean: a lower layer's body fills at
//     its own layer-prefixed path and never enters the head layer's tree.
//
// Structure comes from buildPlainProgramTree, one tree per layer, built
// lazily on first touch and cached for the overlay's lifetime (which is
// bounded by irSourceFS's program cache).
type overlayFS struct {
	layers     []overlayLayer                          // chain oldest→newest, head included and last
	layerCache map[string]filesys_interface.FileSystem // built layer trees, by layer name

	mu sync.Mutex // serializes every tree build, read, and body fill
}

// statusDeleted is the file-status value marking a file its layer removed.
const statusDeleted = "-1"

var (
	_ filesys_interface.FileSystem = (*overlayFS)(nil)
)

// head returns the chain's last layer name ("" when the chain is empty).
func (o *overlayFS) head() string {
	if len(o.layers) == 0 {
		return ""
	}
	return o.layers[len(o.layers)-1].name
}

// headTreeLocked returns the head layer's tree, or nil when the chain is
// empty or the head layer failed to build.
func (o *overlayFS) headTreeLocked() filesys_interface.FileSystem {
	return o.treeOfLocked(o.head())
}

// deletedInFinalView reports whether the head-rooted path is a file the
// final overlay view removed: it exists in no view this fs serves.
func (o *overlayFS) deletedInFinalView(headPath string) bool {
	return o.deletedInFinalViewByRel(o.relToHead(headPath))
}

// deletedInFinalViewByRel is deletedInFinalView for a path already relative
// to the head root (ReadDir composes one while merging entries). The chain
// resolves newest-first — the first layer holding the path decides:
// its status is statusDeleted → removed, anything else → alive (a newer
// layer re-owning a path an older layer deleted resurrects it).
func (o *overlayFS) deletedInFinalViewByRel(rel string) bool {
	for i := len(o.layers) - 1; i >= 0; i-- {
		if o.layers[i].files == nil {
			continue
		}
		if status, ok := o.layers[i].files[rel]; ok {
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

// treeOfLocked returns one layer's tree, building it on first touch. A
// layer that fails to build yields nil and is remembered as broken: the
// merged view degrades to the layers above it, bodies fall through to the
// next owner.
func (o *overlayFS) treeOfLocked(layer string) filesys_interface.FileSystem {
	if layer == "" {
		return nil
	}
	if tree, ok := o.layerCache[layer]; ok {
		return tree
	}
	tree, err := buildPlainProgramTree(layer)
	if err != nil || tree == nil {
		o.layerCache[layer] = nil // remember the broken layer
		return nil
	}
	o.layerCache[layer] = tree
	return tree
}

// lowerLayerTreeLocked resolves a head-rooted path against the newest
// LOWER layer that stores it (head excluded — callers own head lookups).
// The returned path keeps the owning layer's own prefix.
func (o *overlayFS) lowerLayerTreeLocked(headPath string) (filesys_interface.FileSystem, string, bool) {
	// Stat, not Exists: some filesystems' Exists goes through Open, which
	// fails on directories ("is a dir"), so base-only directories would
	// never match.
	for i := len(o.layers) - 2; i >= 0; i-- {
		layer := o.layers[i].name
		if layer == "" || layer == o.head() {
			continue
		}
		tree := o.treeOfLocked(layer)
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
	o.mu.Lock()
	defer o.mu.Unlock()
	byName := make(map[string]fs.DirEntry)
	exists := false
	for i := range o.layers {
		tree := o.treeOfLocked(o.layers[i].name)
		if tree == nil {
			continue // a damaged layer degrades to the layers above it
		}
		list, err := tree.ReadDir(o.layerPath(o.layers[i].name, headPath))
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
		if tree := o.headTreeLocked(); tree != nil {
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
	o.mu.Lock()
	defer o.mu.Unlock()
	tree := o.headTreeLocked()
	if tree == nil {
		return nil, utils.Errorf("directory [%v] not exists", headPath)
	}
	entries, err := tree.ReadDir(headPath)
	if err == nil {
		return entries, nil
	}
	if _, _, ok := o.lowerLayerTreeLocked(headPath); ok {
		return []fs.DirEntry{}, nil
	}
	return nil, err
}

// Stat resolves through the owning layer; a filled body replaces its
// tree stub in place, so the size turns real after the first read. Files
// removed in the final overlay view read as missing everywhere.
func (o *overlayFS) Stat(headPath string) (fs.FileInfo, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if !o.deletedInFinalView(headPath) {
		if tree := o.headTreeLocked(); tree != nil {
			if info, err := tree.Stat(headPath); err == nil {
				return info, nil
			}
		}
		if tree, lp, ok := o.lowerLayerTreeLocked(headPath); ok {
			return tree.Stat(lp)
		}
	}
	// Not found anywhere (or removed in the final view): a not-found error
	// naming the caller's path.
	return nil, utils.Errorf("file [%v] not exist", headPath)
}

// resolveOwnerLocked finds the owning layer's tree and storage path for a
// head-rooted path. Ownership is head-first: any hit on the head tree —
// including an unfilled stub — owns the path and must never fall through
// to an older layer. Deleted-in-final-view paths error.
func (o *overlayFS) resolveOwnerLocked(headPath string) (filesys_interface.FileSystem, string, error) {
	if o.deletedInFinalView(headPath) {
		return nil, "", utils.Errorf("file [%v] not exist", headPath)
	}
	if tree := o.headTreeLocked(); tree != nil {
		if _, statErr := tree.Stat(headPath); statErr == nil {
			return tree, headPath, nil
		}
	}
	if tree, lp, ok := o.lowerLayerTreeLocked(headPath); ok {
		return tree, lp, nil
	}
	return nil, "", utils.Errorf("file [%v] not exist", headPath)
}

// ReadFile serves the owning layer tree's body when already filled;
// otherwise resolves the owning layer, loads the body from the database
// through readIrSourceFileContent, and replaces the tree's stub in place.
// The DB fetch runs outside the lock — a slow query never blocks
// listings; only the cheap stub replacement takes it.
func (o *overlayFS) ReadFile(headPath string) ([]byte, error) {
	o.mu.Lock()
	tree, contentPath, err := o.resolveOwnerLocked(headPath)
	if err != nil {
		o.mu.Unlock()
		return nil, err
	}
	// A structure-only stub reads as empty: an unfilled body.
	data, readErr := tree.ReadFile(contentPath)
	o.mu.Unlock()

	if readErr == nil && len(data) > 0 {
		return data, nil
	}
	data, err = readIrSourceFileContent(contentPath)
	if err != nil {
		return nil, err
	}
	o.mu.Lock()
	tree.Delete(contentPath)
	tree.WriteFile(contentPath, data, 0o644)
	o.mu.Unlock()
	return data, nil
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
