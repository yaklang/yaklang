package ssadb

import (
	"fmt"
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

// Directory trees stay in memory (small). File bodies use a separate TTL cache:
// last access extends life; 10 minutes without ReadFile/Open drops that file's
// content. The path tree itself is not TTL-evicted — only DropProgramCache /
// Delete removes it.
const (
	irSourceFileContentTTL = 10 * time.Minute
	irSourceFileContentCap = 512
)

// irSourceFS caches one VirtualFS per program that holds the full path tree
// (directories + empty file stubs). File bodies live in fileContent, not in the tree.
type irSourceFS struct {
	mu          sync.Mutex
	virtual     map[string]*filesys.VirtualFS // program -> path tree (long-lived)
	fileContent *utils.CacheExWithKey[string, []byte]
}

var IrSourceFsSeparators = '/'

var _ filesys_interface.ReadOnlyFileSystem = (*irSourceFS)(nil)
var _ filesys_interface.FileSystem = (*irSourceFS)(nil)

func NewIrSourceFs() *irSourceFS {
	content := utils.NewCacheExWithKey[string, []byte](
		utils.WithCacheTTL(irSourceFileContentTTL),
		utils.WithCacheCapacity(irSourceFileContentCap),
	)
	// Touch-on-hit (default): actively viewed files stay; idle 10m → evict.
	return &irSourceFS{
		virtual:     make(map[string]*filesys.VirtualFS),
		fileContent: content,
	}
}

func (fs *irSourceFS) ReadFile(path string) ([]byte, error) {
	if path == "/" {
		return nil, utils.Errorf("path [%v] is a program root path, not file.", path)
	}

	fs.mu.Lock()
	vf, err := fs.programTreeLocked(path)
	fs.mu.Unlock()
	if err != nil {
		return nil, err
	}
	if ok, _ := vf.Exists(path); !ok {
		return nil, utils.Errorf("file [%v] not found", path)
	}
	return fs.getFileContent(path)
}

func (fs *irSourceFS) Open(path string) (fs.File, error) {
	if path == "/" {
		return nil, utils.Errorf("path [%v] is a program root path, not file.", path)
	}
	fs.mu.Lock()
	vf, err := fs.programTreeLocked(path)
	fs.mu.Unlock()
	if err != nil {
		return nil, err
	}
	if ok, _ := vf.Exists(path); !ok {
		return nil, utils.Errorf("file [%v] not found", path)
	}
	content, err := fs.getFileContent(path)
	if err != nil {
		return nil, err
	}
	// Do not write content into the shared tree — keep tree as structure only.
	return filesys.NewVirtualFile(path, string(content)), nil
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
	fs.mu.Lock()
	defer fs.mu.Unlock()
	vf, err := fs.programTreeLocked(path)
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
	isfs.mu.Lock()
	defer isfs.mu.Unlock()
	vf, err := isfs.programTreeLocked(path)
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

// splitProjectPath 传入全路径，会以路径分隔符分割，分割后的第一个元素为项目名，后面的元素为文件路径
func splitProjectPath(p string) (projectPath string, fileName string) {
	paths := strings.Split(p, string(IrSourceFsSeparators))
	paths = utils.StringArrayFilterEmpty(paths)
	if len(paths) == 1 {
		return paths[0], ""
	} else if len(paths) > 1 {
		return paths[0], strings.Join(paths[1:], string(IrSourceFsSeparators))
	}
	return "", ""
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

// DropProgramCache removes the path tree and any cached file bodies for programName.
// Directory trees are otherwise retained; call this on program rewrite / delete.
func (f *irSourceFS) DropProgramCache(programName string) {
	if programName == "" {
		return
	}
	f.mu.Lock()
	delete(f.virtual, programName)
	f.mu.Unlock()
	f.dropFileContentPrefix("/" + programName)
}

func (f *irSourceFS) dropFileContentPrefix(prefix string) {
	if f.fileContent == nil {
		return
	}
	f.fileContent.ForEach(func(key string, _ []byte) {
		if key == prefix || strings.HasPrefix(key, prefix+"/") {
			f.fileContent.Remove(key)
		}
	})
}

func (fs *irSourceFS) getFileContent(filePath string) ([]byte, error) {
	if data, ok := fs.fileContent.Get(filePath); ok {
		return data, nil
	}
	data, err := readIrSourceFileContent(filePath)
	if err != nil {
		return nil, err
	}
	fs.fileContent.Set(filePath, data)
	return data, nil
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

func GetIrSourceFsSeparators() rune {
	return IrSourceFsSeparators
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

func (f *irSourceFS) String() string {
	if f == nil {
		return "<nil>"
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	var builder strings.Builder
	builder.WriteString("irSourceFS{")
	first := true
	for programName, virtualFS := range f.virtual {
		if !first {
			builder.WriteString(", ")
		}
		first = false
		builder.WriteString(fmt.Sprintf("%s: %s", programName, virtualFS.String()))
	}
	builder.WriteString("}")
	return builder.String()
}

// programTreeLocked requires fs.mu held.
// First touch of a program builds the entire path tree once (no QuotedCode).
func (fs *irSourceFS) programTreeLocked(anyPath string) (*filesys.VirtualFS, error) {
	progName, _ := fs.getProgram(anyPath)
	if progName == "" {
		return nil, utils.Errorf("invalid path [%v]: missing program name", anyPath)
	}
	if vf, ok := fs.virtual[progName]; ok {
		return vf, nil
	}
	vf := filesys.NewVirtualFs()
	if err := buildProgramTree(progName, fs, vf); err != nil {
		return nil, err
	}
	fs.virtual[progName] = vf
	return vf, nil
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

func irSourceJoin(element ...string) string {
	return path.Join(element...)
}
