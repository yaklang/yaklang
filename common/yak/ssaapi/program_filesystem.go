package ssaapi

import (
	"io/fs"
	"os"
	"path"
	"strconv"
	"strings"
	"sync"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
)

// ProgramFileSystem is the final source tree for IRify / ssadb://.
// Full programs are served from ssadb; incremental programs use
// ProgramOverLay.GetAggregatedFileSystem under /programName.
type ProgramFileSystem struct {
	mu       sync.Mutex
	dbFS     fi.FileSystem
	resolved map[string]fi.FileSystem // programName -> backend
}

var (
	_ fi.ReadOnlyFileSystem = (*ProgramFileSystem)(nil)
	_ fi.FileSystem         = (*ProgramFileSystem)(nil)
)

func NewProgramFileSystem() *ProgramFileSystem {
	return &ProgramFileSystem{
		dbFS:     ssadb.NewIrSourceFs(),
		resolved: make(map[string]fi.FileSystem),
	}
}

func (p *ProgramFileSystem) resolve(progName string) fi.FileSystem {
	if progName == "" {
		return p.dbFS
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if fs, ok := p.resolved[progName]; ok {
		return fs
	}
	prog, err := FromDatabase(progName)
	if err == nil && prog != nil {
		if overlay := prog.GetOverlay(); overlay != nil {
			if agg := overlay.GetAggregatedFileSystem(); agg != nil {
				wrapped := newPrefixedAggregatedFS(progName, agg, prog)
				p.resolved[progName] = wrapped
				return wrapped
			}
		}
	}
	p.resolved[progName] = p.dbFS
	return p.dbFS
}

func (p *ProgramFileSystem) programName(name string) string {
	if name == "" || name == "/" {
		return ""
	}
	parts := strings.Split(strings.Trim(name, "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		return ""
	}
	return parts[0]
}

func (p *ProgramFileSystem) backend(name string) fi.FileSystem {
	return p.resolve(p.programName(name))
}

func (p *ProgramFileSystem) ReadFile(name string) ([]byte, error) {
	if name == "/" {
		return nil, utils.Errorf("path [%v] is a program root path, not file.", name)
	}
	return p.backend(name).ReadFile(name)
}

func (p *ProgramFileSystem) Open(name string) (fs.File, error) {
	if name == "/" {
		return nil, utils.Errorf("path [%v] is a program root path, not file.", name)
	}
	return p.backend(name).Open(name)
}

func (p *ProgramFileSystem) OpenFile(name string, flag int, perm os.FileMode) (fs.File, error) {
	if name == "/" {
		return nil, utils.Errorf("path [%v] is a program root path, not file.", name)
	}
	return p.backend(name).OpenFile(name, flag, perm)
}

func (p *ProgramFileSystem) Stat(name string) (fs.FileInfo, error) {
	if name == "/" {
		return filesys.NewVirtualFileInfo("/", 0, true), nil
	}
	return p.backend(name).Stat(name)
}

func (p *ProgramFileSystem) ReadDir(name string) ([]fs.DirEntry, error) {
	if name == "/" {
		return p.dbFS.ReadDir("/")
	}
	return p.backend(name).ReadDir(name)
}

func (p *ProgramFileSystem) ExtraInfo(name string) map[string]any {
	return p.dbFS.ExtraInfo(name)
}

func (p *ProgramFileSystem) Delete(name string) error {
	progName := p.programName(name)
	if progName != "" {
		p.mu.Lock()
		delete(p.resolved, progName)
		p.mu.Unlock()
	}
	return p.dbFS.Delete(name)
}

func (p *ProgramFileSystem) GetSeparators() rune        { return p.dbFS.GetSeparators() }
func (p *ProgramFileSystem) Join(elem ...string) string { return p.dbFS.Join(elem...) }
func (p *ProgramFileSystem) Base(name string) string    { return p.dbFS.Base(name) }
func (p *ProgramFileSystem) PathSplit(name string) (string, string) {
	return p.dbFS.PathSplit(name)
}
func (p *ProgramFileSystem) Ext(name string) string { return p.dbFS.Ext(name) }
func (p *ProgramFileSystem) IsAbs(name string) bool {
	return len(name) > 0 && name[0] == byte(p.GetSeparators())
}
func (p *ProgramFileSystem) Getwd() (string, error) { return "", nil }
func (p *ProgramFileSystem) Exists(name string) (bool, error) {
	_, err := p.Stat(name)
	return err == nil, err
}
func (p *ProgramFileSystem) Rel(string, string) (string, error) {
	return "", utils.Error("implement me")
}
func (p *ProgramFileSystem) Rename(string, string) error { return utils.Error("implement me") }
func (p *ProgramFileSystem) WriteFile(string, []byte, os.FileMode) error {
	return utils.Error("implement me")
}
func (p *ProgramFileSystem) MkdirAll(string, os.FileMode) error {
	return utils.Error("implement me")
}

// prefixedAggregatedFS exposes an overlay AggregatedFS under /programName/...
type prefixedAggregatedFS struct {
	progName    string
	prefix      string
	inner       fi.FileSystem
	prog        *Program
	extraMu     sync.Mutex
	extraLoaded bool
	extra       map[string]string // full path -> content
}

func newPrefixedAggregatedFS(progName string, inner fi.FileSystem, prog *Program) *prefixedAggregatedFS {
	return &prefixedAggregatedFS{
		progName: progName,
		prefix:   "/" + progName,
		inner:    inner,
		prog:     prog,
		extra:    make(map[string]string),
	}
}

func (f *prefixedAggregatedFS) toInner(name string) (string, error) {
	name = path.Clean("/" + strings.TrimPrefix(name, "/"))
	if name == f.prefix {
		return ".", nil
	}
	if !strings.HasPrefix(name, f.prefix+"/") {
		return "", utils.Errorf("path [%v] is outside program [%v]", name, f.progName)
	}
	rel := strings.TrimPrefix(name, f.prefix+"/")
	if rel == "" {
		return ".", nil
	}
	return rel, nil
}

func (f *prefixedAggregatedFS) ensureExtraLoaded() {
	f.extraMu.Lock()
	defer f.extraMu.Unlock()
	if f.extraLoaded || f.prog == nil {
		return
	}
	f.extraLoaded = true

	type extraSource struct {
		progName string
		files    map[string]string
	}
	var sources []extraSource
	collect := func(prog *Program) {
		if prog == nil {
			return
		}
		name := prog.GetProgramName()
		if prog.Program != nil && len(prog.Program.ExtraFile) > 0 {
			sources = append(sources, extraSource{progName: name, files: prog.Program.ExtraFile})
			return
		}
		if prog.irProgram != nil && len(prog.irProgram.ExtraFile) > 0 {
			sources = append(sources, extraSource{progName: name, files: map[string]string(prog.irProgram.ExtraFile)})
		}
	}
	if overlay := f.prog.GetOverlay(); overlay != nil {
		collect(overlay.Base)
		for _, layer := range overlay.Diff {
			if layer != nil {
				collect(layer.Program)
			}
		}
	} else {
		collect(f.prog)
	}
	if len(sources) == 0 {
		return
	}

	db := ssadb.GetDB()
	for _, src := range sources {
		for fileURL, hash := range src.files {
			if hash == "" || fileURL == "" {
				continue
			}
			vfPath := strings.ReplaceAll(strings.TrimSpace(fileURL), "\\", "/")
			if !strings.HasPrefix(vfPath, "/") {
				vfPath = "/" + vfPath
			}
			vfPath = path.Clean(vfPath)
			segs := strings.Split(strings.Trim(vfPath, "/"), "/")
			if len(segs) == 0 || segs[0] != f.progName {
				vfPath = path.Join("/", f.progName, strings.Trim(strings.TrimPrefix(vfPath, "/"), "/"))
			}
			if _, exists := f.extra[vfPath]; exists {
				continue
			}
			var source ssadb.IrSource
			if err := db.Where("program_name = ? AND source_code_hash = ?", src.progName, hash).First(&source).Error; err != nil {
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
			f.extra[vfPath] = code
		}
	}
}

func (f *prefixedAggregatedFS) ReadFile(name string) ([]byte, error) {
	inner, err := f.toInner(name)
	if err != nil {
		return nil, err
	}
	data, err := f.inner.ReadFile(inner)
	if err == nil {
		return data, nil
	}
	f.ensureExtraLoaded()
	if code, ok := f.extra[path.Clean(name)]; ok {
		return []byte(code), nil
	}
	return nil, err
}

func (f *prefixedAggregatedFS) Open(name string) (fs.File, error) {
	inner, err := f.toInner(name)
	if err != nil {
		return nil, err
	}
	file, err := f.inner.Open(inner)
	if err == nil {
		return file, nil
	}
	f.ensureExtraLoaded()
	if code, ok := f.extra[path.Clean(name)]; ok {
		vf := filesys.NewVirtualFs()
		vf.AddFile(name, code)
		return vf.Open(name)
	}
	return nil, err
}

func (f *prefixedAggregatedFS) OpenFile(name string, flag int, perm os.FileMode) (fs.File, error) {
	inner, err := f.toInner(name)
	if err != nil {
		return nil, err
	}
	file, err := f.inner.OpenFile(inner, flag, perm)
	if err == nil {
		return file, nil
	}
	f.ensureExtraLoaded()
	if code, ok := f.extra[path.Clean(name)]; ok {
		vf := filesys.NewVirtualFs()
		vf.AddFile(name, code)
		return vf.OpenFile(name, flag, perm)
	}
	return nil, err
}

func (f *prefixedAggregatedFS) Stat(name string) (fs.FileInfo, error) {
	inner, err := f.toInner(name)
	if err != nil {
		return nil, err
	}
	info, err := f.inner.Stat(inner)
	if err == nil {
		return info, nil
	}
	f.ensureExtraLoaded()
	if code, ok := f.extra[path.Clean(name)]; ok {
		return filesys.NewVirtualFileInfo(path.Base(name), int64(len(code)), false), nil
	}
	return nil, err
}

func (f *prefixedAggregatedFS) ReadDir(name string) ([]fs.DirEntry, error) {
	inner, err := f.toInner(name)
	if err != nil {
		return nil, err
	}
	entries, err := f.inner.ReadDir(inner)
	if err != nil {
		entries = nil
	}
	f.ensureExtraLoaded()
	dir := path.Clean(name)
	seen := make(map[string]struct{}, len(entries))
	for _, e := range entries {
		seen[e.Name()] = struct{}{}
	}
	for full := range f.extra {
		parent := path.Dir(full)
		if parent != dir {
			continue
		}
		base := path.Base(full)
		if _, ok := seen[base]; ok {
			continue
		}
		seen[base] = struct{}{}
		entries = append(entries, filesys.NewVirtualFileInfo(base, int64(len(f.extra[full])), false))
	}
	if len(entries) == 0 && err != nil {
		return nil, err
	}
	return entries, nil
}

func (f *prefixedAggregatedFS) ExtraInfo(string) map[string]any { return nil }
func (f *prefixedAggregatedFS) Delete(string) error {
	return utils.Error("implement me")
}
func (f *prefixedAggregatedFS) GetSeparators() rune { return '/' }
func (f *prefixedAggregatedFS) Join(elem ...string) string {
	return path.Join(elem...)
}
func (f *prefixedAggregatedFS) Base(name string) string { return path.Base(name) }
func (f *prefixedAggregatedFS) PathSplit(name string) (string, string) {
	dir, file := path.Split(name)
	if len(dir) > 1 && strings.HasSuffix(dir, "/") {
		dir = dir[:len(dir)-1]
	}
	return dir, file
}
func (f *prefixedAggregatedFS) Ext(name string) string { return path.Ext(name) }
func (f *prefixedAggregatedFS) IsAbs(name string) bool {
	return len(name) > 0 && name[0] == '/'
}
func (f *prefixedAggregatedFS) Getwd() (string, error) { return "", nil }
func (f *prefixedAggregatedFS) Exists(name string) (bool, error) {
	_, err := f.Stat(name)
	return err == nil, err
}
func (f *prefixedAggregatedFS) Rel(string, string) (string, error) {
	return "", utils.Error("implement me")
}
func (f *prefixedAggregatedFS) Rename(string, string) error { return utils.Error("implement me") }
func (f *prefixedAggregatedFS) WriteFile(string, []byte, os.FileMode) error {
	return utils.Error("implement me")
}
func (f *prefixedAggregatedFS) MkdirAll(string, os.FileMode) error {
	return utils.Error("implement me")
}
