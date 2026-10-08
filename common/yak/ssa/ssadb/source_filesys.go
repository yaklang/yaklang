package ssadb

import (
	"io/fs"
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

// One VirtualFS per program is cached as a whole: the path tree (directories +
// empty file stubs) is built once, file bodies are lazily filled into the tree
// on first ReadFile/Open, and an idle program (tree untouched for 10 minutes)
// is evicted entirely — structure and bodies together. Touch-on-hit: active
// viewing extends life. DropProgramCache / Delete removes a program explicitly.
const (
	irSourceTreeTTL = 10 * time.Minute
	irSourceTreeCap = 64
)

// irSourceFS caches one VirtualFS per program. The tree starts as structure
// only (no QuotedCode pulled); file content is filled in place on demand, so
// listings stay cheap and read bodies become plain memory afterwards.
type irSourceFS struct {
	mu      sync.Mutex // guards the tree cache itself (build / fill)
	virtual *utils.CacheExWithKey[string, *filesys.VirtualFS]
}

var IrSourceFsSeparators = '/'

var _ filesys_interface.ReadOnlyFileSystem = (*irSourceFS)(nil)
var _ filesys_interface.FileSystem = (*irSourceFS)(nil)

func NewIrSourceFs() *irSourceFS {
	// Touch-on-hit (default): active viewing extends life; idle 10m → evict.
	return &irSourceFS{
		virtual: utils.NewCacheExWithKey[string, *filesys.VirtualFS](
			utils.WithCacheTTL(irSourceTreeTTL),
			utils.WithCacheCapacity(irSourceTreeCap),
		),
	}
}

func (fs *irSourceFS) ReadFile(path string) ([]byte, error) {
	if path == "/" {
		return nil, utils.Errorf("path [%v] is a program root path, not file.", path)
	}

	vf, err := fs.treeFor(path)
	if err != nil {
		return nil, err
	}
	if ok, _ := vf.Exists(path); !ok {
		return nil, utils.Errorf("file [%v] not found", path)
	}
	// Filled bodies live in the tree; a non-empty read is a pure memory hit.
	// Empty real files degrade to a DB fetch per read (correct, just uncached):
	// ir_sources models empty quoted_code rows as directories, so a file row's
	// quoted source is at least `""` — an empty tree stub always means "not yet
	// filled" except for that empty-file corner.
	if data, err := vf.ReadFile(path); err == nil && len(data) > 0 {
		return data, nil
	}
	return fs.fillFileContent(vf, path)
}

func (fs *irSourceFS) Open(path string) (fs.File, error) {
	if path == "/" {
		return nil, utils.Errorf("path [%v] is a program root path, not file.", path)
	}
	vf, err := fs.treeFor(path)
	if err != nil {
		return nil, err
	}
	if ok, _ := vf.Exists(path); !ok {
		return nil, utils.Errorf("file [%v] not found", path)
	}
	if data, err := vf.ReadFile(path); err == nil && len(data) > 0 {
		return filesys.NewVirtualFile(path, string(data)), nil
	}
	data, err := fs.fillFileContent(vf, path)
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
	vf, err := fs.treeFor(path)
	if err != nil {
		return nil, err
	}
	return vf.Stat(path)
}

func (isfs *irSourceFS) ReadDir(path string) ([]fs.DirEntry, error) {
	if path == "/" {
		ret := make([]fs.DirEntry, 0)
		for _, porgram := range AllPrograms(GetDB()) {
			ret = append(ret, filesys.NewVirtualFileInfo(porgram.ProgramName, 0, true))
		}
		return ret, nil
	}
	vf, err := isfs.treeFor(path)
	if err != nil {
		return nil, err
	}
	return vf.ReadDir(path)
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

// treeFor resolves (and builds on first touch) the cached tree for the program
// named by anyPath. The DB build runs under fs.mu to avoid duplicate builds.
func (fs *irSourceFS) treeFor(anyPath string) (*filesys.VirtualFS, error) {
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
	vf := filesys.NewVirtualFs()
	if err := buildProgramTree(progName, fs, vf); err != nil {
		return nil, err
	}
	fs.virtual.Set(progName, vf)
	return vf, nil
}

// fillFileContent loads one file's body from the database and writes it back
// into the tree (replacing the empty stub, which also makes Stat report the
// real size). The DB fetch runs outside fs.mu — a slow read never blocks
// listings; only the cheap stub replacement takes the lock.
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

func buildProgramTree(progName string, irfs *irSourceFS, vf *filesys.VirtualFS) error {
	entries, err := GetIrSourceTreeByProgram(progName)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		sourcePath := irfs.Join(entry.FolderPath, entry.FileName)
		if entry.IsDir {
			vf.AddDir(sourcePath)
			continue
		}
		vf.AddFile(sourcePath, "")
	}
	mergeExtraFileStubs(progName, vf)
	return nil
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

// mergeExtraFileStubs adds ExtraFile paths as empty stubs (tree only).
func mergeExtraFileStubs(progName string, vf *filesys.VirtualFS) {
	if progName == "" || vf == nil {
		return
	}
	prog, err := GetApplicationProgram(progName)
	if err != nil || prog == nil || len(prog.ExtraFile) == 0 {
		return
	}
	for fileURL := range prog.ExtraFile {
		vfPath := normalizeExtraFilePath(progName, fileURL)
		if ok, err := vf.Exists(vfPath); err == nil && ok {
			continue
		}
		vf.AddFile(vfPath, "")
	}
}
