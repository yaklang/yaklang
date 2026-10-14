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

	"github.com/yaklang/gorm"
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
// program ("/"), each program served by its own cached filesystem. Plain
// programs get their bodies filled here on ReadFile/Open into their
// structure-only tree; the overlay filesystem owns its own body loading
// end to end.
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
	// An incremental program's filesystem owns its body loading end to
	// end: its ReadFile resolves the owning layer internally and never
	// returns an empty body, so its result (data or error) stands as is.
	// A plain program's tree only holds empty stubs: serve the filled
	// memory hit, else fill from the database here.
	data, err := pfs.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(data) > 0 {
		return data, nil
	}
	return fs.fillFileContent(pfs, path)
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

// incrementalFS is the per-program filesystem of an incremental-compile
// program: its ReadFile resolves and loads bodies through the owning
// overlay layer itself, and it serves a diff-only listing view (the head
// layer's own entries) on top of the fully expanded one.
type incrementalFS interface {
	ReadDirDiff(path string) ([]fs.DirEntry, error)
}

// ReadDirQuery is the yakurl entry for listing with query options. Only an
// incremental program's filesystem has a second view: diffOnly collapses to
// the head layer's own entries. A plain program's tree ignores the query.
func (isfs *irSourceFS) ReadDirQuery(path string, query url.Values) ([]fs.DirEntry, error) {
	if path == "/" {
		return programRootEntries(), nil
	}
	pfs, err := isfs.treeFor(path)
	if err != nil {
		return nil, err
	}
	if isDiffOnlyQuery(query) {
		if inc, ok := pfs.(incrementalFS); ok {
			return inc.ReadDirDiff(path)
		}
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
// rows. A program name with ir_sources rows but no ir_program row still
// serves its plain tree (folder rows can outlive or precede the program
// row). A row carrying a layer chain — the head of an incremental series
// (>= 2 layers), or a chain-head base recording just itself (1 layer) —
// assembles a filesys.OverlayFS from it: a chain is a chain, no flavor
// check. The overlay loads every layer's tree lazily through the same
// buildPlainProgramTree, resolves deletions through the layers' file
// status maps, and fills bodies itself through the DB loader. A program
// without a chain keeps its structure-only tree; irSourceFS fills its
// bodies on read.
func buildProgramTree(progName string) (filesys_interface.FileSystem, error) {
	prog, err := GetApplicationProgram(progName)
	if err != nil && !gorm.IsRecordNotFoundError(err) {
		return nil, err
	}
	if prog == nil || len(prog.OverlayLayers) == 0 {
		// No chain: structure-only tree; irSourceFS fills bodies on read.
		return buildPlainProgramTree(progName)
	}

	layers := prog.OverlayLayers
	layerProgs := GetApplicationPrograms(layers)
	opts := []filesys.OverlayOption{
		filesys.WithLayers(layers...),
		filesys.WithLayerTrees(buildPlainProgramTree),
		filesys.WithBodyLoader(readIrSourceFileContent),
	}
	// Keep the recipe order: status layers resolve newest-first.
	for _, layer := range layers {
		if lp := layerProgs[layer]; lp != nil && len(lp.FileHashMap) > 0 {
			opts = append(opts, filesys.WithFileStatus(layer, lp.FileHashMap))
		}
	}
	return filesys.NewOverlayFS(opts...), nil
}

// buildPlainProgramTree builds one program's structure-only tree from its
// ir_sources rows (dirs as dir nodes, files as empty stubs) plus its
// ExtraFile paths as stubs. Shared by plain programs and overlay layers
// alike: the overlay pulls each layer's tree through it on first touch.
func buildPlainProgramTree(progName string) (filesys_interface.FileSystem, error) {
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
	if err != nil && !gorm.IsRecordNotFoundError(err) {
		return nil, err
	}
	if prog != nil {
		mergeExtraFileStubs(prog, vf)
	}
	return vf, nil
}

// fillFileContent loads one plain-program file's body from the database
// and caches it into its tree (replacing the empty stub, which also makes
// Stat report the real size). The DB fetch runs outside fs.mu — a slow
// query never blocks listings; only the cheap stub replacement takes it.
func (fs *irSourceFS) fillFileContent(vf filesys_interface.FileSystem, path string) ([]byte, error) {
	data, err := readIrSourceFileContent(path)
	if err != nil {
		return nil, err
	}
	fs.mu.Lock()
	vf.Delete(path)
	vf.WriteFile(path, data, 0o644)
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
func mergeExtraFileStubs(prog *IrProgram, vf filesys_interface.FileSystem) {
	if prog == nil || vf == nil || len(prog.ExtraFile) == 0 {
		return
	}
	for fileURL := range prog.ExtraFile {
		vfPath := normalizeExtraFilePath(prog.ProgramName, fileURL)
		if ok, err := vf.Exists(vfPath); err == nil && ok {
			continue
		}
		vf.WriteFile(vfPath, nil, 0o644)
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