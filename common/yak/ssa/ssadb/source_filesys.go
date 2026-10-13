package ssadb

import (
	"io/fs"
	"net/url"
	"os"
	"path"
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
// owns every overlay decision (full expansion, diff-only listing, layer
// fallback). Trees start structure-only (no QuotedCode pulled); file
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
	// Incremental programs: the per-program fs owns body resolution (head
	// tree first, owning-layer fallback with re-rooting).
	if ofs, ok := pfs.(overlayLister); ok {
		return ofs.ReadFile(path)
	}
	// Plain programs: filled bodies live in the tree; a non-empty read is a
	// pure memory hit. Empty real files degrade to a DB fetch per read
	// (correct, just uncached): ir_sources models empty quoted_code rows as
	// directories, so a file row's quoted source is at least `""` — an
	// empty tree stub always means "not yet filled" except for that
	// empty-file corner.
	if data, err := pfs.ReadFile(path); err == nil && len(data) > 0 {
		return data, nil
	}
	return fs.fillFileContent(pfs, path)
}

func (fs *irSourceFS) Open(path string) (fs.File, error) {
	if path == "/" {
		return nil, utils.Errorf("path [%v] is a program root path, not file.", path)
	}
	pfs, err := fs.treeFor(path)
	if err != nil {
		return nil, err
	}
	if ofs, ok := pfs.(overlayLister); ok {
		return ofs.Open(path)
	}
	if data, err := pfs.ReadFile(path); err == nil && len(data) > 0 {
		return filesys.NewVirtualFile(path, string(data)), nil
	}
	data, err := fs.fillFileContent(pfs, path)
	if err != nil {
		return nil, err
	}
	return filesys.NewVirtualFile(path, string(data)), nil
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
// UNCHECKED): an incremental program then lists its head layer's own
// entries instead of the fully expanded tree. Everything else — including
// all other filesystems — ignores the query and lists normally.
func (isfs *irSourceFS) ReadDirQuery(path string, query url.Values) ([]fs.DirEntry, error) {
	if path == "/" {
		return programRootEntries(), nil
	}
	pfs, err := isfs.treeFor(path)
	if err != nil {
		return nil, err
	}
	if ofs, ok := pfs.(overlayLister); ok && isDiffOnlyQuery(query) {
		return ofs.ReadDirDiff(path)
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

// overlayLister is the capability interface of an incremental program's
// filesystem (incrementalFS): beside the plain FileSystem listing it serves
// the diff-only view — the head layer's own entries ("show all files"
// unchecked) — and owns body resolution through the owning layers.
type overlayLister interface {
	ReadDir(path string) ([]fs.DirEntry, error)
	ReadDirDiff(path string) ([]fs.DirEntry, error)
	Stat(path string) (fs.FileInfo, error)
	ReadFile(path string) ([]byte, error)
	Open(path string) (fs.File, error)
}

var _ overlayLister = (*incrementalFS)(nil)
var _ filesys_interface.FileSystem = (*incrementalFS)(nil)

// incrementalFS is the per-program filesystem of an incremental (overlay)
// program. ir_sources keeps every layer's rows under the layer's own
// program prefix and the head layer records only the last diff, so this fs
// keeps the head's diff tree plus the overlay recipe and builds the rest on
// demand: lower layers' trees are materialized lazily, ReadDir expands the
// whole chain re-rooted under the head prefix (换头), ReadDirDiff serves the
// head's own entries, and file bodies fall back through the owning layer.
type incrementalFS struct {
	*filesys.VirtualFS // head layer's own tree: last diff + ExtraFile stubs; also provides the FileSystem plumbing

	prog    *IrProgram          // overlay recipe: layers oldest→head + FileHashMaps
	deleted map[string]struct{} // files deleted in the final overlay view

	mu         sync.Mutex                    // guards layerTrees and stub fills
	layerTrees map[string]*filesys.VirtualFS // lower layers' raw trees, lazily built
}

func newIncrementalFS(prog *IrProgram, head *filesys.VirtualFS) *incrementalFS {
	return &incrementalFS{
		VirtualFS:  head,
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
	if len(byName) == 0 {
		// No layer has entries here: keep the head tree's error for a path
		// that exists nowhere (a real but empty dir still reads as empty).
		if _, err := ifs.VirtualFS.ReadDir(path); err != nil {
			return nil, err
		}
	}
	entries := make([]fs.DirEntry, 0, len(byName))
	for _, e := range byName {
		entries = append(entries, e)
	}
	return entries, nil
}

// ReadDirDiff lists the diff-only view (diffOnly=true): the head layer's
// own entries. A directory that lives in lower layers only reads as empty
// instead of erroring, so nodes expanded in the full view don't break the
// diff view.
func (ifs *incrementalFS) ReadDirDiff(path string) ([]fs.DirEntry, error) {
	entries, err := ifs.VirtualFS.ReadDir(path)
	if err == nil {
		return entries, nil
	}
	if _, ok := ifs.fallbackPath(path); ok {
		return []fs.DirEntry{}, nil
	}
	return nil, err
}

// fallbackTree resolves /<head>/<rest> against the layer that actually
// stores it: layers newest-first, the first whose tree has the path wins
// (the compiler's aggregation order). The returned path keeps the owning
// layer's own program prefix — re-rooting to the head prefix happens only
// for full listings (ReadDir); bodies resolve through the owner directly.
func (ifs *incrementalFS) fallbackTree(path string) (*filesys.VirtualFS, string, bool) {
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

func (ifs *incrementalFS) fallbackPath(path string) (string, bool) {
	_, p, ok := ifs.fallbackTree(path)
	return p, ok
}

func (ifs *incrementalFS) Stat(path string) (fs.FileInfo, error) {
	if info, err := ifs.VirtualFS.Stat(path); err == nil {
		return info, nil
	}
	if tree, layerPath, ok := ifs.fallbackTree(path); ok {
		return tree.Stat(layerPath)
	}
	// Not found anywhere: keep the head tree's own error.
	return ifs.VirtualFS.Stat(path)
}

func (ifs *incrementalFS) Exists(path string) (bool, error) {
	_, err := ifs.Stat(path)
	return err == nil, err
}

// ReadFile reads through the head prefix: a memory hit on the head tree
// first, otherwise the owning layer's tree. Bodies fill into the tree that
// owns the row (keeping the layer prefix), so repeated reads are memory
// hits and the filled body shows up in that layer's full-view listings.
func (ifs *incrementalFS) ReadFile(path string) ([]byte, error) {
	if data, err := ifs.VirtualFS.ReadFile(path); err == nil && len(data) > 0 {
		return data, nil
	}
	if tree, layerPath, ok := ifs.fallbackTree(path); ok {
		if data, err := tree.ReadFile(layerPath); err == nil && len(data) > 0 {
			return data, nil
		}
		return ifs.fillFileContent(tree, layerPath)
	}
	return ifs.fillFileContent(ifs.VirtualFS, path)
}

func (ifs *incrementalFS) Open(path string) (fs.File, error) {
	data, err := ifs.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return filesys.NewVirtualFile(path, string(data)), nil
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
// the head is flat). The head layer maps to the embedded head tree; lower
// layers are built lazily once and cached in this fs.
func (ifs *incrementalFS) layerTreeFor(layer string) (*filesys.VirtualFS, error) {
	if layer == ifs.prog.ProgramName {
		return ifs.VirtualFS, nil
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
func (fs *irSourceFS) fillFileContent(vf filesys_interface.FileSystem, path string) ([]byte, error) {
	data, err := readIrSourceFileContent(path)
	if err != nil {
		return nil, err
	}
	fs.mu.Lock()
	_ = vf.Delete(path)
	_ = vf.WriteFile(path, data, 0o644)
	fs.mu.Unlock()
	return data, nil
}

// buildProgramTree builds the per-program filesystem from its ir_sources
// rows: dirs as dir nodes, files as empty stubs (bodies are lazily filled).
// A full-compile program keeps the plain VirtualFS; an incremental program
// wraps it in an incrementalFS, which records the overlay recipe and owns
// every overlay decision.
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
