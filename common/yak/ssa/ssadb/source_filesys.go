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

// One per-program filesystem is cached as a whole: the tree is built once,
// and an idle program (untouched for 10 minutes) is evicted entirely.
// Touch-on-hit: active viewing extends life. DropProgramCache / Delete
// removes a program explicitly.
const (
	irSourceTreeTTL = 10 * time.Minute
	irSourceTreeCap = 64
)

// irSourceFS is the ssadb:// filesystem: one root listing every compiled
// program ("/"), each program served by its own cached filesystem. Bodies
// are filled only here on ReadFile/Open into the program's LazyBodyTree
// under the caller's path (a lazy view that can grow as more files are
// read). Optional ContentPathResolver remaps that path for DB loads.
type irSourceFS struct {
	mu      sync.Mutex // guards the tree cache itself (build / plain fill)
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
	bt, ok := pfs.(filesys.LazyBodyTree)
	if !ok {
		return nil, utils.Errorf("program filesystem has no body tree: %T", pfs)
	}
	vf := bt.BodyTree()
	if data, err := vf.ReadFile(path); err == nil && len(data) > 0 {
		return data, nil
	}
	contentPath := path
	if r, ok := pfs.(filesys.ContentPathResolver); ok {
		contentPath, err = r.ResolveContentPath(path)
		if err != nil {
			return nil, err
		}
	}
	return fs.fillFileContent(pfs, vf, path, contentPath)
}

func (fs *irSourceFS) Open(path string) (fs.File, error) {
	if path == "/" {
		return nil, utils.Errorf("path [%v] is a program root path, not file.", path)
	}
	data, err := fs.ReadFile(path)
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
// expanded overlay (OverlayFS.ReadDir).
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

// ReadDirQuery is the yakurl entry for listing with query options. Only an
// incremental program's OverlayFS has a second view: diffOnly collapses to
// the head layer's own entries. Plain VirtualFS ignores the query.
func (isfs *irSourceFS) ReadDirQuery(path string, query url.Values) ([]fs.DirEntry, error) {
	if path == "/" {
		return programRootEntries(), nil
	}
	pfs, err := isfs.treeFor(path)
	if err != nil {
		return nil, err
	}
	if of, ok := pfs.(*filesys.OverlayFS); ok && isDiffOnlyQuery(query) {
		return of.ReadDirDiff(path)
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

// ------- per-program tree construction -------

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

// buildProgramTree builds the per-program filesystem from its database
// rows: a plain program keeps its structure-only VirtualFS (ir_sources +
// ExtraFile stubs); an incremental program gets a filesys.OverlayFS that
// assembles the overlay chain. Bodies are never stored at build time —
// irSourceFS.fill them on ReadFile/Open into LazyBodyTree under the caller path.
func buildProgramTree(progName string) (filesys_interface.FileSystem, error) {
	prog, err := GetApplicationProgram(progName)
	if err != nil {
		return nil, err
	}
	if prog != nil && prog.HasSavedOverlayLayers() {
		return buildOverlayProgramTree(prog), nil
	}

	// Plain program: structure-only tree; irSourceFS fills bodies on read.
	vf, err := buildPlainProgramTree(progName)
	if err != nil {
		return nil, err
	}
	if prog != nil {
		mergeExtraFileStubs(prog, vf)
	}
	return vf, nil
}

// buildOverlayProgramTree wires an incremental program's database rows
// into a filesys.OverlayFS: every layer's tree (structure + ExtraFile
// stubs) builds lazily on first touch; each layer's FileHashMap contributes
// to deletion resolution. Bodies stay empty until irSourceFS fills them
// into OverlayFS's lazy head-path BodyTree (base content is cached there
// on first read without writing into layer trees).
func buildOverlayProgramTree(prog *IrProgram) filesys_interface.FileSystem {
	layers := prog.OverlayLayers
	layerProgs := GetApplicationPrograms(layers)

	buildLayer := func(layer string) (filesys_interface.FileSystem, error) {
		vf, err := buildPlainProgramTree(layer)
		if err != nil {
			return nil, err
		}
		if lp, ok := layerProgs[layer]; ok && lp != nil {
			mergeExtraFileStubs(lp, vf)
		}
		return vf, nil
	}

	opts := []filesys.OverlayOption{
		filesys.WithLayers(layers...),
		filesys.WithLayerTrees(buildLayer),
	}
	for _, layer := range layers {
		if lp, ok := layerProgs[layer]; ok && lp != nil && len(lp.FileHashMap) > 0 {
			opts = append(opts, filesys.WithFileStatus(layer, lp.FileHashMap))
		}
	}
	return filesys.NewOverlayFS(opts...)
}

// buildPlainProgramTree builds one program's structure-only VirtualFS from
// its ir_sources rows: dirs as dir nodes, files as empty stubs.
func buildPlainProgramTree(progName string) (*filesys.VirtualFS, error) {
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
	return vf, nil
}

// fillFileContent loads one file's body from the database at contentPath
// and caches it into the lazy body tree at viewPath (the caller's path).
// The DB fetch runs outside any lock. Write-back uses BodyWriter when the
// tree provides one, otherwise fs.mu for a plain VirtualFS.
func (fs *irSourceFS) fillFileContent(pfs filesys_interface.FileSystem, vf *filesys.VirtualFS, viewPath, contentPath string) ([]byte, error) {
	data, err := readIrSourceFileContent(contentPath)
	if err != nil {
		return nil, err
	}
	if w, ok := pfs.(filesys.BodyWriter); ok {
		w.WriteBody(viewPath, data)
		return data, nil
	}
	fs.mu.Lock()
	vf.RemoveFileOrDir(viewPath)
	vf.AddFile(viewPath, string(data))
	fs.mu.Unlock()
	return data, nil
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