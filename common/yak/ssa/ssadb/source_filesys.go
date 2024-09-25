package ssadb

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"strconv"
	"strings"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
)

type irSourceFS struct {
	virtual *filesys.VirtualFS
}

func NewIrSourceFs() *irSourceFS {
	virtual := filesys.NewVirtualFs()
	virtual.AddDir("/")
	return &irSourceFS{
		virtual: virtual,
	}
}

var _ filesys_interface.FileSystem = (*irSourceFS)(nil)

func (fs *irSourceFS) addFile(source *IrSource) {
	path := fs.Join(source.FolderPath, source.FileName)
	if source.QuotedCode == "" {
		// fs.virtual.add dir
		fs.virtual.AddDir(path)
	} else {
		code, _ := strconv.Unquote(source.QuotedCode)
		if code == "" {
			code = source.QuotedCode
		}

		// fs.virtual.add file
		fs.virtual.AddFile(path, code)
	}
}
func (fs *irSourceFS) loadStat(fullpath string) error {
	if fullpath == "/" {
		for _, program := range AllSSAPrograms() {
			fs.virtual.AddDir(fmt.Sprintf("/%s", program.Name))
		}
		return nil
	}
	return fs.loadFile(fullpath)
}

func (fs *irSourceFS) loadFile(fullPath string) error {
	programName, _ := fs.getProgram(fullPath)
	CheckAndSwitchDB(programName)

	path, name := fs.PathSplit(fullPath)
	if name == "" {
		fs.loadFolder(path)
	} else {
		// just file
		source, err := GetIrSourceByPathAndName(path, name)
		if err != nil {
			return err
		}
		fs.addFile(source)
	}
	return nil
}

func (fs *irSourceFS) loadFolder(path string) error {
	if path == "/" {
		for _, program := range AllSSAPrograms() {
			fs.virtual.AddDir(fmt.Sprintf("/%s", program.Name))
		}
		return nil
	}
	programName, _ := fs.getProgram(path)
	CheckAndSwitchDB(programName)

	// just folder
	sources, err := GetIrSourceByPath(path)
	if err != nil {
		return err
	}
	for _, source := range sources {
		fs.addFile(source)
	}
	return nil
}

func (fs *irSourceFS) ReadFile(path string) ([]byte, error) {
	if data, err := fs.virtual.ReadFile(path); err == nil {
		return data, nil
	}
	if err := fs.loadFile(path); err != nil {
		return nil, err
	}

	return fs.virtual.ReadFile(path)
}

func (fs *irSourceFS) Open(path string) (fs.File, error) {
	if file, err := fs.virtual.Open(path); err == nil {
		return file, nil
	}
	if err := fs.loadFile(path); err != nil {
		return nil, err
	}
	return fs.virtual.Open(path)
}

func (fs *irSourceFS) OpenFile(path string, flag int, perm os.FileMode) (fs.File, error) {
	if file, err := fs.virtual.OpenFile(path, flag, perm); err == nil {
		return file, nil
	}
	if err := fs.loadFile(path); err != nil {
		return nil, err
	}
	return fs.virtual.OpenFile(path, flag, perm)
}

func (fs *irSourceFS) Stat(path string) (fs.FileInfo, error) {
	if info, err := fs.virtual.Stat(path); err == nil {
		return info, nil
	}
	if err := fs.loadStat(path); err != nil {
		return nil, err
	}
	return fs.virtual.Stat(path)
}

func (fs *irSourceFS) ReadDir(path string) ([]fs.DirEntry, error) {
	if entry, err := fs.virtual.ReadDir(path); err == nil && entry != nil {
		return entry, nil
	}
	if err := fs.loadFolder(path); err != nil {
		return nil, err
	}
	return fs.virtual.ReadDir(path)
}

func (fs *irSourceFS) PathSplit(p string) (string, string) {
	return pathSplit(p)
}

func pathSplit(p string) (string, string) {
	dir, name := path.Split(p)
	if len(dir) != 1 && dir[len(dir)-1] == '/' {
		dir = dir[:len(dir)-1]
	}
	return dir, name
}

func (fs *irSourceFS) getProgram(path string) (string, bool) {
	dir := strings.Split(path, string(fs.GetSeparators()))
	return dir[1], len(dir) == 2
}

func (f *irSourceFS) ExtraInfo(path string) map[string]any {
	m := make(map[string]any)
	// if root path ? get program Name
	programName, programRoot := f.getProgram(path)
	if !programRoot {
		return m
	}

	if prog := GetSSAProgram(programName); prog != nil {
		m["programName"] = programName
		m["CreateAt"] = prog.CreatedAt.Unix()
		m["Language"] = prog.Language
		m["Path"] = prog.DBPath
		m["Description"] = prog.Description
	}
	return m
}

func (f *irSourceFS) Delete(path string) error {
	f.virtual.Delete(path)
	// if root path ? get program Name
	programName, programRoot := f.getProgram(path)
	if !programRoot {
		return utils.Errorf("path [%v] is not a program root path, can't delete", path)
	}
	prog := GetSSAProgram(programName)
	if prog == nil {
		return utils.Errorf("program [%v] not exist", programName)
	}
	// switch db path
	origin := consts.GetSSADataBasePath()
	if origin != prog.DBPath {
		consts.SetSSADataBasePath(prog.DBPath)
	}
	// delete program
	DeleteProgram(GetDB(), programName)
	DeleteSSAProgram(programName)
	return nil
}

func (fs *irSourceFS) Ext(string) string {
	return ""
}

func (f *irSourceFS) GetSeparators() rune         { return '/' }
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
