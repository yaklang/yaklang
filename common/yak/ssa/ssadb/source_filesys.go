package ssadb

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"strconv"
	"strings"
	"sync"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
)

// GetAggregatedFileSystemFunc 用于获取聚合文件系统的函数类型
// 这个函数变量由 ssaapi 包在初始化时注册，避免循环导入
var GetAggregatedFileSystemFunc func(programName string) filesys_interface.FileSystem

// SetGetAggregatedFileSystemFunc 设置获取聚合文件系统的函数
// 由 ssaapi 包在初始化时调用
func SetGetAggregatedFileSystemFunc(fn func(programName string) filesys_interface.FileSystem) {
	GetAggregatedFileSystemFunc = fn
}

type irSourceFS struct {
	mu            sync.Mutex
	virtual       map[string]*filesys.VirtualFS // program -> virtual fs
	loadedDirs    map[string]struct{}           // directory paths already hydrated from DB
	overlayLoaded map[string]struct{}           // overlay programs already copied into virtual fs
	notOverlay    map[string]struct{}           // programs known not to use overlay
}

var IrSourceFsSeparators = '/'

var _ filesys_interface.ReadOnlyFileSystem = (*irSourceFS)(nil)
var _ filesys_interface.FileSystem = (*irSourceFS)(nil)

func NewIrSourceFs() *irSourceFS {
	return &irSourceFS{
		virtual:       make(map[string]*filesys.VirtualFS),
		loadedDirs:    make(map[string]struct{}),
		overlayLoaded: make(map[string]struct{}),
		notOverlay:    make(map[string]struct{}),
	}
}

func (fs *irSourceFS) ReadFile(path string) ([]byte, error) {
	if path == "/" {
		return nil, utils.Errorf("path [%v] is a program root path, not file.", path)
	}

	vf, err := fs.checkPath(path)
	if err != nil {
		return nil, err
	}
	return vf.ReadFile(path)
}

func (fs *irSourceFS) Open(path string) (fs.File, error) {
	if path == "/" {
		return nil, utils.Errorf("path [%v] is a program root path, not file.", path)
	}
	vf, err := fs.checkPath(path)
	if err != nil {
		return nil, err
	}
	return vf.Open(path)
}

func (fs *irSourceFS) OpenFile(path string, flag int, perm os.FileMode) (fs.File, error) {
	if path == "/" {
		return nil, utils.Errorf("path [%v] is a program root path, not file.", path)
	}
	vf, err := fs.checkPath(path)
	if err != nil {
		return nil, err
	}
	return vf.OpenFile(path, flag, perm)
}

func (fs *irSourceFS) Stat(path string) (fs.FileInfo, error) {
	if path == "/" {
		return filesys.NewVirtualFileInfo("/", 0, true), nil
	}
	// handler path
	vf, err := fs.checkPath(path, false)
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
	vf, err := isfs.checkPath(path, true)
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

// splitProjectPath传入全路径，会以路径分隔符分割，分割后的第一个元素为项目名，后面的元素为文件路径
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
	// switch db path
	// if prog := CheckAndSwitchDB(programName); prog == nil {
	// 	return utils.Errorf("program [%v] not exist", programName)
	// }
	f.mu.Lock()
	delete(f.virtual, programName)
	delete(f.overlayLoaded, programName)
	delete(f.notOverlay, programName)
	prefix := "/" + programName
	for p := range f.loadedDirs {
		if p == prefix || strings.HasPrefix(p, prefix+"/") {
			delete(f.loadedDirs, p)
		}
	}
	f.mu.Unlock()
	// delete program
	DeleteProgram(GetDB(), programName)
	return nil
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

// mergeExtraFileEntriesIntoVF ensures paths recorded only in IrProgram.extra_file appear in the
// audit / ssadb virtual tree (e.g. duplicate content hash kept a single IrSource row, or legacy rows).
func mergeExtraFileEntriesIntoVF(progName string, vf *filesys.VirtualFS) {
	if progName == "" || vf == nil {
		return
	}
	prog, err := GetApplicationProgram(progName)
	if err != nil || prog == nil || len(prog.ExtraFile) == 0 {
		return
	}
	db := GetDB()
	for fileURL, hash := range prog.ExtraFile {
		if hash == "" || fileURL == "" {
			continue
		}
		vfPath := strings.ReplaceAll(strings.TrimSpace(fileURL), "\\", "/")
		if !strings.HasPrefix(vfPath, "/") {
			vfPath = "/" + vfPath
		}
		vfPath = path.Clean(vfPath)
		segs := strings.Split(strings.Trim(vfPath, "/"), "/")
		if len(segs) == 0 || segs[0] != progName {
			vfPath = path.Join("/", progName, strings.Trim(strings.TrimPrefix(vfPath, "/"), "/"))
		}
		if ok, err := vf.Exists(vfPath); err == nil && ok {
			continue
		}
		var source IrSource
		if err := db.Where("program_name = ? AND source_code_hash = ?", progName, hash).First(&source).Error; err != nil {
			continue
		}
		if source.QuotedCode == "" {
			continue
		}
		code, e := strconv.Unquote(source.QuotedCode)
		if e != nil || code == "" {
			code = source.QuotedCode
		}
		if code == "" {
			continue
		}
		vf.AddFile(vfPath, code)
	}
}

func (fs *irSourceFS) checkPath(path string, isDirs ...bool) (*filesys.VirtualFS, error) {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	progName, isProgram := fs.getProgram(path)
	vf, ok := fs.virtual[progName]
	if !ok {
		vf = filesys.NewVirtualFs()
		fs.virtual[progName] = vf
	}
	// is directory parameter
	isDir := false
	if len(isDirs) > 0 {
		isDir = isDirs[0]
	}
	// if "/programName" this is a program root path, is directory
	if isProgram {
		isDir = true
	}
	loadIrSourceFS(path, progName, isDir, fs, vf)
	return vf, nil
}

func loadIrSourceFS(path, progName string, isDir bool, irfs *irSourceFS, vf *filesys.VirtualFS) {
	if progName != "" {
		if _, ok := irfs.overlayLoaded[progName]; ok {
			return
		}
		if _, skip := irfs.notOverlay[progName]; !skip {
			prog, err := GetApplicationProgram(progName)
			if err == nil && prog != nil && prog.IsOverlay && len(prog.OverlayLayers) > 0 {
				if GetAggregatedFileSystemFunc != nil {
					log.Debugf("loading aggregated file system for overlay program: %s", progName)
					aggregatedFS := GetAggregatedFileSystemFunc(progName)
					if aggregatedFS != nil {
						err := filesys.Recursive(".",
							filesys.WithFileSystem(aggregatedFS),
							filesys.WithFileStat(func(filePath string, info fs.FileInfo) error {
								if info.IsDir() {
									return nil
								}
								content, err := aggregatedFS.ReadFile(filePath)
								if err != nil {
									log.Warnf("failed to read file %s from aggregatedFS: %v", filePath, err)
									return nil
								}
								normalizedPath := strings.TrimPrefix(filePath, "/")
								vf.AddFile("/"+progName+"/"+normalizedPath, string(content))
								return nil
							}))
						if err == nil {
							mergeExtraFileEntriesIntoVF(progName, vf)
							irfs.overlayLoaded[progName] = struct{}{}
							return
						}
						log.Warnf("failed to copy files from aggregatedFS: %v, fallback to single program", err)
					}
				}
			} else {
				irfs.notOverlay[progName] = struct{}{}
			}
		}
	}

	add2FS := func(source *IrSource) {
		sourcePath := irfs.Join(source.FolderPath, source.FileName)
		if source.QuotedCode == "" {
			vf.AddDir(sourcePath)
		} else {
			code, _ := strconv.Unquote(source.QuotedCode)
			if code == "" {
				code = source.QuotedCode
			}
			vf.AddFile(sourcePath, code)
		}
	}

	addDir := func(dirPath string) {
		if _, ok := irfs.loadedDirs[dirPath]; ok {
			return
		}
		sources, err := GetIrSourceByPath(dirPath)
		if err != nil {
			return
		}
		for _, source := range sources {
			add2FS(source)
		}
		irfs.loadedDirs[dirPath] = struct{}{}
	}

	if isDir {
		if _, ok := irfs.loadedDirs[path]; ok {
			return
		}
		addDir(path)
		mergeExtraFileEntriesIntoVF(progName, vf)
		return
	}

	if _, err := vf.Stat(path); err == nil {
		return
	}

	dir, name := irfs.PathSplit(path)
	if name == "" {
		addDir(dir)
	} else {
		source, err := GetIrSourceByPathAndName(dir, name)
		if err != nil {
			mergeExtraFileEntriesIntoVF(progName, vf)
			return
		}
		add2FS(source)
	}
	mergeExtraFileEntriesIntoVF(progName, vf)
}

func irSourceJoin(element ...string) string {
	return path.Join(element...)
}
