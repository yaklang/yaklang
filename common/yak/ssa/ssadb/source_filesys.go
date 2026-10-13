package ssadb

import (
	"io/fs"
	"net/url"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
)

// One per-program filesystem is cached as a whole: the path tree
// (directories + empty file stubs) is built once, file bodies are lazily
// filled into the tree on first ReadFile/Open, and an idle program (tree
// untouched for 10 minutes) is evicted entirely — structure and bodies
// together. Touch-on-hit: active viewing extends life. DropProgramCache /
// Delete removes a program explicitly.
const (
	irSourceTreeTTL = 10 * time.Minute
	irSourceTreeCap = 64
)

// irSourceFS caches one per-program filesystem behind an interface, not a
// concrete tree: a full-compile program keeps the VirtualFS built from its
// own ir_sources rows; an incremental program gets an incrementalFS that
// records the head layer's last-diff tree plus the whole overlay recipe and
// owns every overlay decision (full expansion, diff-only listing,
// owning-layer resolution). Trees start structure-only (no QuotedCode
// pulled); file
// content is filled in place on demand, so listings stay cheap and read
// bodies become plain memory afterwards.
type irSourceFS struct {
	mu      sync.Mutex // guards the tree cache itself (build / fill)
	virtual *utils.CacheExWithKey[string, filesys_interface.FileSystem]
}

var IrSourceFsSeparators = '/'

var _ filesys_interface.ReadOnlyFileSystem = (*irSourceFS)(nil)
var _ filesys_interface.FileSystem = (*irSourceFS)(nil)

func NewIrSourceFs() *irSourceFS {
	// Touch-on-hit (default): active viewing extends life; idle 10m → evict.
	return &irSourceFS{
		virtual: utils.NewCacheExWithKey[string, filesys_interface.FileSystem](
			utils.WithCacheTTL(irSourceTreeTTL),
			utils.WithCacheCapacity(irSourceTreeCap),
		),
	}
}

func (fs *irSourceFS) ReadFile(path string) ([]byte, error) {
	if path == "/" {
		return nil, utils.Errorf("path [%v] is a program root path, not file.", path)
	}
	pfs, err := fs.treeFor(path)
	if err != nil {
		return nil, err
	}
	// A bare VirtualFS (full-compile program) only holds empty stubs: serve
	// the filled memory hit, else fill from the database here. Any other
	// per-program fs — the incremental one — owns its body resolution: its
	// ReadFile resolves the owning layer internally, so its result stands.
	if vf, plainTree := pfs.(*filesys.VirtualFS); plainTree {
		if data, err := vf.ReadFile(path); err == nil && len(data) > 0 {
			return data, nil
		}
		return fs.fillFileContent(vf, path)
	}
	return pfs.ReadFile(path)
}

func (fs *irSourceFS) Open(path string) (fs.File, error) {
	if path == "/" {
		return nil, utils.Errorf("path [%v] is a program root path, not file.", path)
	}
	pfs, err := fs.treeFor(path)
	if err != nil {
		return nil, err
	}
	// Same dispatch as ReadFile: a plain VirtualFS gets its stub filled
	// here; the incremental fs opens through its own resolution.
	if vf, plainTree := pfs.(*filesys.VirtualFS); plainTree {
		if data, err := vf.ReadFile(path); err == nil && len(data) > 0 {
			return filesys.NewVirtualFile(path, string(data)), nil
		}
		data, err := fs.fillFileContent(vf, path)
		if err != nil {
			return nil, err
		}
		return filesys.NewVirtualFile(path, string(data)), nil
	}
	return pfs.Open(path)
}

func (fs *irSourceFS) OpenFile(path string, flag int, perm os.FileMode) (fs.File, error) {
	if path == "/" {
		return nil, utils.Errorf("path [%v] is a program root path, not file.", path)
	}
	return fs.Open(path)
}

func (fs *irSourceFS) Stat(path string) (fs.FileInfo, error) {
	if path == "/" {
		return filesys.NewVirtualFileInfo("/", 0, true), nil
	}
	pfs, err := fs.treeFor(path)
	if err != nil {
		return nil, err
	}
	return pfs.Stat(path)
}

// programRootEntries lists every program name as a directory entry for "/".
func programRootEntries() []fs.DirEntry {
	ret := make([]fs.DirEntry, 0)
	for _, porgram := range AllPrograms(GetDB()) {
		ret = append(ret, filesys.NewVirtualFileInfo(porgram.ProgramName, 0, true))
	}
	return ret
}

// ReadDir lists a directory through the program's own filesystem: plain
// programs list their single tree; incremental programs get the fully
// expanded overlay tree (incrementalFS owns the expansion). The diff-only
// view is ReadDirQuery with diffOnly=true.
func (isfs *irSourceFS) ReadDir(path string) ([]fs.DirEntry, error) {
	if path == "/" {
		return programRootEntries(), nil
	}
	pfs, err := isfs.treeFor(path)
	if err != nil {
		return nil, err
	}
	return pfs.ReadDir(path)
}

// ReadDirQuery lists with the caller's query options. ssadb's only option
// today is diffOnly (the audit frontend sends it with "show all files"
// UNCHECKED). The per-program fs that understands options judges them
// itself — incrementalFS.ReadDirQuery collapses to its head diff — and
// everything else ignores the query and lists normally.
func (isfs *irSourceFS) ReadDirQuery(path string, query url.Values) ([]fs.DirEntry, error) {
	if path == "/" {
		return programRootEntries(), nil
	}
	pfs, err := isfs.treeFor(path)
	if err != nil {
		return nil, err
	}
	if ql, ok := pfs.(queryDirLister); ok {
		return ql.ReadDirQuery(path, query)
	}
	return pfs.ReadDir(path)
}

func isDiffOnlyQuery(query url.Values) bool {
	switch strings.ToLower(strings.TrimSpace(query.Get("diffOnly"))) {
	case "1", "true", "yes":
		return true
	}
	return false
}

// overlayDeletedFiles returns the program-relative paths that are deleted in
// the final overlay view: for each path the NEWEST layer's FileHashMap entry
// decides ("-1" = deleted, "0"/"1" = alive), mirroring how the compiler
// applies layer FileHashMaps (a newer layer can re-own a path an older layer
// deleted). All layer rows are fetched in one query.
func overlayDeletedFiles(prog *IrProgram) map[string]struct{} {
	rows := GetApplicationPrograms(prog.OverlayLayers)
	deleted := make(map[string]struct{})
	seen := make(map[string]struct{})
	// Newest layer first: the first entry seen for a path decides its fate.
	for i := len(prog.OverlayLayers) - 1; i >= 0; i-- {
		lp, ok := rows[prog.OverlayLayers[i]]
		if !ok {
			continue
		}
		for file, status := range lp.FileHashMap {
			file = strings.Trim(file, "/")
			if _, done := seen[file]; done {
				continue
			}
			seen[file] = struct{}{}
			if status == "-1" {
				deleted[file] = struct{}{}
			}
		}
	}
	return deleted
}

// ------- incremental program fs -------

// queryDirLister marks a per-program fs that takes listing query options
// into account: the incremental fs judges the frontend's "show all files"
// checkbox (diffOnly) inside its own listing path.
type queryDirLister interface {
	ReadDirQuery(path string, query url.Values) ([]fs.DirEntry, error)
}

var _ filesys_interface.FileSystem = (*incrementalFS)(nil)
var _ queryDirLister = (*incrementalFS)(nil)

// incrementalFS is the per-program filesystem of an incremental (overlay)
// program — a standalone FileSystem implementation, not a wrapper around
// one tree. ir_sources keeps every layer's rows under the layer's own
// program prefix and the head layer records only the last diff, so this fs
// records the head's diff tree plus the whole overlay recipe (layer chain,
// deletion map) and decides, inside its own ReadFile/Open/ReadDir methods,
// whether and how far the incremental project actually expands:
//
//   - ReadDir serves the fully expanded view: every layer's entries
//     re-rooted under the head prefix (换头), deduplicated by name with
//     newer layers shadowing older ones, minus files deleted in the final
//     view.
//   - ReadDirQuery judges diffOnly itself: set ("show all files" unchecked)
//     → ReadDirDiff serves only the head layer's own entries.
//   - ReadFile/Open/Stat resolve each path through its OWNING layer — head
//     first (a head stub means the head modified the file: never fall
//     through to an older layer's copy of the same path), then lower layers
//     newest-first.
//
// Lower layers' trees are materialized lazily and cached inside this fs.
type incrementalFS struct {
	head *filesys.VirtualFS // the head layer's own tree: last diff + ExtraFile stubs

	prog    *IrProgram          // overlay recipe: layers oldest→head + FileHashMaps
	deleted map[string]struct{} // files deleted in the final overlay view

	mu         sync.Mutex                    // guards layerTrees and stub fills
	layerTrees map[string]*filesys.VirtualFS // lower layers' raw trees, lazily built
}

func newIncrementalFS(prog *IrProgram, head *filesys.VirtualFS) *incrementalFS {
	return &incrementalFS{
		head:       head,
		prog:       prog,
		deleted:    overlayDeletedFiles(prog),
		layerTrees: make(map[string]*filesys.VirtualFS),
	}
}

// ReadDir lists the fully expanded view of the overlay chain: every layer's
// entries re-rooted under the head prefix, deduplicated by name with newer
// layers shadowing older ones, minus files deleted in the final overlay
// view. Layer trees are cached inside this fs, so a listing only walks
// in-memory directory entries.
func (ifs *incrementalFS) ReadDir(path string) ([]fs.DirEntry, error) {
	rel := strings.Trim(strings.TrimPrefix(path, "/"+ifs.prog.ProgramName), "/")
	byName := make(map[string]fs.DirEntry)
	exists := false
	for _, layer := range ifs.prog.OverlayLayers {
		tree, err := ifs.layerTreeFor(layer)
		if err != nil {
			continue // a damaged layer degrades to the layers above it
		}
		layerPath := "/" + layer
		if rel != "" {
			layerPath += "/" + rel
		}
		list, err := tree.ReadDir(layerPath)
		if err != nil {
			continue // this layer doesn't have the directory
		}
		exists = true
		for _, e := range list {
			fullRel := e.Name()
			if rel != "" {
				fullRel = rel + "/" + e.Name()
			}
			if _, ok := ifs.deleted[fullRel]; ok && !e.IsDir() {
				continue // deleted in the final overlay view
			}
			byName[e.Name()] = e // layers oldest→head: head entries overwrite
		}
	}
	if !exists {
		// No layer has this directory: the head tree's error says the path
		// exists nowhere (a real but empty dir still reads as empty above).
		return ifs.head.ReadDir(path)
	}
	entries := make([]fs.DirEntry, 0, len(byName))
	for _, e := range byName {
		entries = append(entries, e)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	return entries, nil
}

// ReadDirQuery judges the listing options inside this fs: diffOnly (the
// audit frontend sends it with "show all files" UNCHECKED) collapses the
// view to the head layer's own entries; anything else expands the whole
// overlay chain (ReadDir).
func (ifs *incrementalFS) ReadDirQuery(path string, query url.Values) ([]fs.DirEntry, error) {
	if isDiffOnlyQuery(query) {
		return ifs.ReadDirDiff(path)
	}
	return ifs.ReadDir(path)
}

// ReadDirDiff lists the diff-only view: the head layer's own entries. A
// directory that lives in lower layers only reads as empty instead of
// erroring, so nodes visible in the full view don't break the diff view.
func (ifs *incrementalFS) ReadDirDiff(path string) ([]fs.DirEntry, error) {
	entries, err := ifs.head.ReadDir(path)
	if err == nil {
		return entries, nil
	}
	if _, _, ok := ifs.lowerLayerTree(path); ok {
		return []fs.DirEntry{}, nil
	}
	return nil, err
}

// lowerLayerTree resolves /<head>/<rest> against the newest LOWER layer
// that stores it (head excluded — callers own head lookups). The returned
// path keeps the owning layer's own program prefix: bodies resolve through
// the owner directly, re-rooting to the head prefix happens only in full
// listings (ReadDir).
func (ifs *incrementalFS) lowerLayerTree(path string) (*filesys.VirtualFS, string, bool) {
	rest := strings.TrimPrefix(path, "/"+ifs.prog.ProgramName)
	for i := len(ifs.prog.OverlayLayers) - 1; i >= 0; i-- {
		layer := ifs.prog.OverlayLayers[i]
		if layer == "" || layer == ifs.prog.ProgramName {
			continue
		}
		layerPath := "/" + layer + rest
		tree, err := ifs.layerTreeFor(layer)
		if err != nil {
			continue
		}
		// Stat, not Exists: VirtualFS.Exists goes through Open, which fails
		// on directories ("is a dir"), so base-only directories would never
		// match.
		if _, statErr := tree.Stat(layerPath); statErr == nil {
			return tree, layerPath, true
		}
	}
	return nil, "", false
}

func (ifs *incrementalFS) Stat(path string) (fs.FileInfo, error) {
	if info, err := ifs.head.Stat(path); err == nil {
		return info, nil
	}
	if !ifs.deletedInFinalView(path) {
		if tree, layerPath, ok := ifs.lowerLayerTree(path); ok {
			return tree.Stat(layerPath)
		}
	}
	// Not found anywhere (or removed in the final view): the head tree's
	// own not-found error.
	return ifs.head.Stat(path)
}

func (ifs *incrementalFS) Exists(path string) (bool, error) {
	_, err := ifs.Stat(path)
	return err == nil, err
}

// deletedInFinalView reports whether the head-prefixed path is a file the
// final overlay view removed (FileHashMap "-1" from its newest owning
// layer): it exists in no view this fs serves.
func (ifs *incrementalFS) deletedInFinalView(path string) bool {
	rel := strings.Trim(strings.TrimPrefix(path, "/"+ifs.prog.ProgramName), "/")
	_, ok := ifs.deleted[rel]
	return ok
}

// ReadFile resolves the path through its OWNING layer, head first: any hit
// on the head tree — including an unfilled stub, which means the head diff
// touched the file — owns the path and must never fall through to an older
// layer's copy of the same path. Bodies fill into the tree that owns the
// row (keeping the layer prefix), so repeated reads are memory hits and the
// filled body shows up in that layer's full-view listings.
func (ifs *incrementalFS) ReadFile(path string) ([]byte, error) {
	if ifs.deletedInFinalView(path) {
		return nil, utils.Errorf("file [%v] not exist", path)
	}
	if _, err := ifs.head.Stat(path); err == nil {
		return ifs.readTreeFile(ifs.head, path)
	}
	if tree, layerPath, ok := ifs.lowerLayerTree(path); ok {
		return ifs.readTreeFile(tree, layerPath)
	}
	return nil, utils.Errorf("file [%v] not exist", path)
}

// readTreeFile serves one body out of an owning tree: the filled memory
// hit, else a fill from the database into that tree.
func (ifs *incrementalFS) readTreeFile(vf *filesys.VirtualFS, path string) ([]byte, error) {
	if data, err := vf.ReadFile(path); err == nil && len(data) > 0 {
		return data, nil
	}
	return ifs.fillFileContent(vf, path)
}

func (ifs *incrementalFS) Open(path string) (fs.File, error) {
	data, err := ifs.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return filesys.NewVirtualFile(path, string(data)), nil
}

func (ifs *incrementalFS) OpenFile(path string, flag int, perm os.FileMode) (fs.File, error) {
	return ifs.Open(path)
}

// fillFileContent loads one body and writes it back into the owning tree
// (replacing the empty stub, which also makes Stat report the real size).
// The DB fetch runs outside ifs.mu — a slow read never blocks listings;
// only the cheap stub replacement takes the lock.
func (ifs *incrementalFS) fillFileContent(vf *filesys.VirtualFS, path string) ([]byte, error) {
	data, err := readIrSourceFileContent(path)
	if err != nil {
		return nil, err
	}
	ifs.mu.Lock()
	vf.RemoveFileOrDir(path)
	vf.AddFile(path, string(data))
	ifs.mu.Unlock()
	return data, nil
}

// layerTreeFor returns the raw tree of one overlay layer (its own ir_sources
// rows + ExtraFile stubs only — no recursive chain expansion: the recipe on
// the head is flat). The head layer maps to this fs's head tree; lower
// layers are built lazily once and cached in this fs.
func (ifs *incrementalFS) layerTreeFor(layer string) (*filesys.VirtualFS, error) {
	if layer == ifs.prog.ProgramName {
		return ifs.head, nil
	}
	ifs.mu.Lock()
	defer ifs.mu.Unlock()
	if tree, ok := ifs.layerTrees[layer]; ok {
		return tree, nil
	}
	entries, err := GetIrSourceTreeByProgram(layer)
	if err != nil {
		return nil, err
	}
	tree := filesys.NewVirtualFs()
	for _, entry := range entries {
		sourcePath := path.Join(entry.FolderPath, entry.FileName)
		if entry.IsDir {
			tree.AddDir(sourcePath)
		} else {
			tree.AddFile(sourcePath, "")
		}
	}
	if lp, err := GetApplicationProgram(layer); err == nil && lp != nil {
		mergeExtraFileStubs(lp, tree)
	}
	ifs.layerTrees[layer] = tree
	return tree, nil
}

// Native path plumbing: this fs serves VirtualFS trees, so the path rules
// match irSourceFS (separator '/', path.Join semantics, VirtualFS's
// extension rule). Writes are unsupported — program writes go through
// irSourceFS, which deletes whole programs.
func (ifs *incrementalFS) GetSeparators() rune         { return IrSourceFsSeparators }
func (ifs *incrementalFS) Join(paths ...string) string { return path.Join(paths...) }
func (ifs *incrementalFS) Base(p string) string        { return path.Base(p) }
func (ifs *incrementalFS) PathSplit(p string) (string, string) {
	return pathSplit(p)
}
func (ifs *incrementalFS) Ext(name string) string {
	if idx := strings.LastIndex(name, "."); idx != -1 {
		return name[idx:]
	}
	return ""
}
func (ifs *incrementalFS) IsAbs(name string) bool {
	return len(name) > 0 && name[0] == byte(ifs.GetSeparators())
}
func (ifs *incrementalFS) Getwd() (string, error)          { return "", nil }
func (ifs *incrementalFS) ExtraInfo(string) map[string]any { return nil }
func (ifs *incrementalFS) Rename(string, string) error     { return utils.Error("implement me") }
func (ifs *incrementalFS) Rel(string, string) (string, error) {
	return "", utils.Error("implement me")
}
func (ifs *incrementalFS) WriteFile(string, []byte, os.FileMode) error {
	return utils.Error("implement me")
}
func (ifs *incrementalFS) Delete(string) error { return utils.Error("implement me") }
func (ifs *incrementalFS) MkdirAll(string, os.FileMode) error {
	return utils.Error("implement me")
}

func (fs *irSourceFS) PathSplit(p string) (string, string) {
	return pathSplit(p)
}

func pathSplit(p string) (string, string) {
	if p == "" {
		return "", ""
	}
	dir, name := path.Split(p)
	if len(dir) != 1 && dir[len(dir)-1] == '/' {
		dir = dir[:len(dir)-1]
	}
	return dir, name
}

func (f *irSourceFS) ExtraInfo(path string) map[string]any {
	m := make(map[string]any)
	programName, isProgram := f.getProgram(path)
	if !isProgram {
		return m
	}
	if prog, err := GetProgram(programName, Application); prog != nil && err == nil {
		m["programName"] = programName
		m["CreateAt"] = prog.CreatedAt.Unix()
		m["Language"] = prog.Language
		m["Description"] = prog.Description
		// Incremental markers for the audit frontend (program badges).
		m["IsIncremental"] = prog.IsIncrementalKind()
		m["IsOverlayProgram"] = prog.HasSavedOverlayLayers()
	}
	return m
}

func (f *irSourceFS) Delete(path string) error {
	if path == "/" {
		return utils.Errorf("path [%v] is a program root path, can't delete", path)
	}
	programName, isProgram := f.getProgram(path)
	if !isProgram {
		return utils.Errorf("path [%v] is not a program root path, can't delete", path)
	}
	f.DropProgramCache(programName)
	DeleteProgram(GetDB(), programName)
	return nil
}

// DropProgramCache removes the cached tree (structure + filled bodies) for
// programName. Call this on program rewrite / delete.
func (f *irSourceFS) DropProgramCache(programName string) {
	if programName == "" {
		return
	}
	f.virtual.Remove(programName)
}

func (fs *irSourceFS) Ext(string) string {
	return ""
}

func (fs *irSourceFS) getProgram(path string) (string, bool) {
	if path == "" {
		return "", false
	}
	dir := strings.Split(path, string(fs.GetSeparators()))
	if len(dir) < 2 {
		return "", false
	} else {
		return dir[1], len(dir) == 2
	}
}

func (f *irSourceFS) GetSeparators() rune         { return IrSourceFsSeparators }
func (f *irSourceFS) Join(paths ...string) string { return path.Join(paths...) }
func (f *irSourceFS) IsAbs(name string) bool {
	return len(name) > 0 && name[0] == byte(f.GetSeparators())
}
func (f *irSourceFS) Getwd() (string, error) { return "", nil }
func (f *irSourceFS) Exists(path string) (bool, error) {
	_, err := f.Stat(path)
	return err == nil, err
}
func (f *irSourceFS) Rename(string, string) error                 { return utils.Error("implement me") }
func (f *irSourceFS) Rel(string, string) (string, error)          { return "", utils.Error("implement me") }
func (f *irSourceFS) WriteFile(string, []byte, os.FileMode) error { return utils.Error("implement me") }
func (f *irSourceFS) MkdirAll(string, os.FileMode) error          { return utils.Error("implement me") }
func (f *irSourceFS) Base(p string) string                        { return path.Base(p) }

// treeFor resolves (and builds on first touch) the cached per-program
// filesystem named by anyPath. The DB build runs under fs.mu to avoid
// duplicate builds.
func (fs *irSourceFS) treeFor(anyPath string) (filesys_interface.FileSystem, error) {
	progName, _ := fs.getProgram(anyPath)
	if progName == "" {
		return nil, utils.Errorf("invalid path [%v]: missing program name", anyPath)
	}
	if vf, ok := fs.virtual.Get(progName); ok && vf != nil {
		return vf, nil
	}
	fs.mu.Lock()
	defer fs.mu.Unlock()
	if vf, ok := fs.virtual.Get(progName); ok && vf != nil {
		return vf, nil
	}
	vf, err := buildProgramTree(progName)
	if err != nil {
		return nil, err
	}
	fs.virtual.Set(progName, vf)
	return vf, nil
}

// fillFileContent loads one plain-program file's body from the database and
// writes it back into its tree (replacing the empty stub, which also makes
// Stat report the real size). The DB fetch runs outside fs.mu — a slow read
// never blocks listings; only the cheap stub replacement takes the lock.
func (fs *irSourceFS) fillFileContent(vf *filesys.VirtualFS, path string) ([]byte, error) {
	data, err := readIrSourceFileContent(path)
	if err != nil {
		return nil, err
	}
	fs.mu.Lock()
	vf.RemoveFileOrDir(path)
	vf.AddFile(path, string(data))
	fs.mu.Unlock()
	return data, nil
}

// buildProgramTree builds the per-program filesystem from its ir_sources
// rows: dirs as dir nodes, files as empty stubs (bodies are lazily filled).
// A full-compile program keeps the plain VirtualFS; an incremental program
// gets an incrementalFS that keeps the head's last-diff tree plus the
// overlay recipe and owns every overlay decision.
func buildProgramTree(progName string) (filesys_interface.FileSystem, error) {
	vf := filesys.NewVirtualFs()
	entries, err := GetIrSourceTreeByProgram(progName)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		sourcePath := path.Join(entry.FolderPath, entry.FileName)
		if entry.IsDir {
			vf.AddDir(sourcePath)
			continue
		}
		vf.AddFile(sourcePath, "")
	}
	prog, err := GetApplicationProgram(progName)
	if err != nil || prog == nil {
		// No Application row (yet): plain tree, no extra files, no overlay.
		return vf, nil
	}
	mergeExtraFileStubs(prog, vf)
	if prog.HasSavedOverlayLayers() {
		return newIncrementalFS(prog, vf), nil
	}
	return vf, nil
}

func readIrSourceFileContent(filePath string) ([]byte, error) {
	dir, name := pathSplit(filePath)
	if name == "" {
		return nil, utils.Errorf("path [%v] is a directory, not file", filePath)
	}
	source, err := GetIrSourceByPathAndName(dir, name)
	if err != nil {
		source, err = lookupExtraFileSource(progNameFromPath(filePath), filePath)
		if err != nil {
			return nil, err
		}
	}
	if source.QuotedCode == "" {
		return nil, utils.Errorf("path [%v] is a directory, not file", filePath)
	}
	code, e := strconv.Unquote(source.QuotedCode)
	if e != nil || code == "" {
		code = source.QuotedCode
	}
	return []byte(code), nil
}

func progNameFromPath(filePath string) string {
	parts := strings.Split(strings.Trim(filePath, "/"), "/")
	if len(parts) == 0 {
		return ""
	}
	return parts[0]
}

func lookupExtraFileSource(progName, filePath string) (*IrSource, error) {
	if progName == "" {
		return nil, utils.Errorf("extra file lookup: empty program")
	}
	prog, err := GetApplicationProgram(progName)
	if err != nil || prog == nil || len(prog.ExtraFile) == 0 {
		return nil, utils.Errorf("extra file not found: %v", filePath)
	}
	hash := ""
	for fileURL, h := range prog.ExtraFile {
		if normalizeExtraFilePath(progName, fileURL) == filePath {
			hash = h
			break
		}
	}
	if hash == "" {
		return nil, utils.Errorf("extra file not found: %v", filePath)
	}
	db := GetDB()
	var source IrSource
	if err := db.Where("program_name = ? AND source_code_hash = ?", progName, hash).First(&source).Error; err != nil {
		return nil, err
	}
	return &source, nil
}

func normalizeExtraFilePath(progName, fileURL string) string {
	vfPath := strings.ReplaceAll(strings.TrimSpace(fileURL), "\\", "/")
	if !strings.HasPrefix(vfPath, "/") {
		vfPath = "/" + vfPath
	}
	vfPath = path.Clean(vfPath)
	segs := strings.Split(strings.Trim(vfPath, "/"), "/")
	if len(segs) == 0 || segs[0] != progName {
		vfPath = path.Join("/", progName, strings.Trim(strings.TrimPrefix(vfPath, "/"), "/"))
	}
	return vfPath
}

// mergeExtraFileStubs adds the program's own ExtraFile paths as empty stubs
// (tree only).
func mergeExtraFileStubs(prog *IrProgram, vf *filesys.VirtualFS) {
	if prog == nil || vf == nil || len(prog.ExtraFile) == 0 {
		return
	}
	for fileURL := range prog.ExtraFile {
		vfPath := normalizeExtraFilePath(prog.ProgramName, fileURL)
		if ok, err := vf.Exists(vfPath); err == nil && ok {
			continue
		}
		vf.AddFile(vfPath, "")
	}
}
