package ssadb

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"strconv"
	"strings"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
)

type irSourceFS struct {
	virtual map[string]*filesys.VirtualFS // echo program -> virtual fs
}

var IrSourceFsSeparators = '/'

var _ filesys_interface.ReadOnlyFileSystem = (*irSourceFS)(nil)
var _ filesys_interface.FileSystem = (*irSourceFS)(nil)

func NewIrSourceFs() *irSourceFS {
	ret := &irSourceFS{}
	ret.virtual = make(map[string]*filesys.VirtualFS)
	return ret
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
	delete(f.virtual, programName)
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
	return dir[1], len(dir) == 2
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

func (fs *irSourceFS) checkPath(path string, isDirs ...bool) (*filesys.VirtualFS, error) {
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

func loadIrSourceFS(path, progName string, isDir bool, fs *irSourceFS, vf *filesys.VirtualFS) {
	add2FS := func(source *IrSource) {
		path := fs.Join(source.FolderPath, source.FileName)
		if source.QuotedCode == "" {
			// fs.virtual.add dir
			vf.AddDir(path)
		} else {
			code, _ := strconv.Unquote(source.QuotedCode)
			if code == "" {
				code = source.QuotedCode
			}

			// fs.virtual.add file
			vf.AddFile(path, code)
		}
	}

	addDir := func(path string) {
		sources, err := GetIrSourceByPath(path)
		if err != nil {
			return
		}
		for _, source := range sources {
			add2FS(source)
		}
	}

	// if _, err := vf.Stat(path); err == nil {
	// 	return
	// }

	if isDir {
		addDir(path)
		return
	}

	// other
	path, name := fs.PathSplit(path)
	// if is program, this is root path
	if name == "" {
		// directory
		sources, err := GetIrSourceByPath(path)
		if err != nil {
			return
		}
		for _, source := range sources {
			add2FS(source)
		}
	} else {
		// file
		source, err := GetIrSourceByPathAndName(path, name)
		if err != nil {
			return
		}
		add2FS(source)
	}
}

func irSourceJoin(element ...string) string {
	return path.Join(element...)
}
