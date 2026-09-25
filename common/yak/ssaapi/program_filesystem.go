package ssaapi

import (
	"io/fs"
	"os"
	"path"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
)

// Overlay AggregatedFS is expensive to rebuild. Keep it while the user is
// looking at that program (touch-on-hit); drop after 10 minutes idle so file
// bodies do not stay forever. Directory trees for full programs live in
// IrSourceFS and are retained separately (small).
const (
	programFSOverlayTTL = 10 * time.Minute
	programFSOverlayCap = 64
)

// programFSGeneration bumps when an overlay program is rewritten / deleted so
// in-flight TTL entries are dropped on the next resolve even before expiry.
var programFSGeneration sync.Map // programName -> *atomic.Int64

func programFSGen(progName string) *atomic.Int64 {
	if progName == "" {
		return nil
	}
	v, _ := programFSGeneration.LoadOrStore(progName, &atomic.Int64{})
	return v.(*atomic.Int64)
}

// InvalidateProgramFileSystemCache drops cached overlay backends for progName
// across all ProgramFileSystem instances (generation bump + optional local remove).
func InvalidateProgramFileSystemCache(progName string) {
	if progName == "" {
		return
	}
	if g := programFSGen(progName); g != nil {
		g.Add(1)
	}
}

type overlayCacheEntry struct {
	fs  fi.FileSystem
	gen int64
}

// ProgramFileSystem is the final source tree for IRify / ssadb://.
// Full programs are served from ssadb; incremental programs use
// ProgramOverLay.GetAggregatedFileSystem under /programName.
//
// Lifecycle of overlay backends:
//   - Only multi-layer AggregatedFS wrappers are cached (never the dbFS fallback).
//   - TTL 10m with touch-on-hit: the project the user is viewing stays cached;
//     10 minutes without access drops that overlay (file-heavy) entry.
//   - Cap 64 programs.
//   - InvalidateProgramFileSystemCache / Delete bump generation so a compile
//     that finishes mid-TTL is visible on the next resolve.
type ProgramFileSystem struct {
	dbFS         fi.FileSystem
	overlayCache *utils.CacheExWithKey[string, *overlayCacheEntry]
}

var (
	_ fi.ReadOnlyFileSystem = (*ProgramFileSystem)(nil)
	_ fi.FileSystem         = (*ProgramFileSystem)(nil)
)

func NewProgramFileSystem() *ProgramFileSystem {
	cache := utils.NewCacheExWithKey[string, *overlayCacheEntry](
		utils.WithCacheTTL(programFSOverlayTTL),
		utils.WithCacheCapacity(programFSOverlayCap),
	)
	// Touch-on-hit (default): active viewing extends life; idle 10m → evict.
	return &ProgramFileSystem{
		dbFS:         ssadb.NewIrSourceFs(),
		overlayCache: cache,
	}
}

func (p *ProgramFileSystem) resolve(progName string) fi.FileSystem {
	if progName == "" {
		return p.dbFS
	}
	gen := int64(0)
	if g := programFSGen(progName); g != nil {
		gen = g.Load()
	}
	if entry, ok := p.overlayCache.Get(progName); ok && entry != nil && entry.fs != nil && entry.gen == gen {
		return entry.fs
	}

	prog, err := FromDatabase(progName)
	if err == nil && prog != nil {
		if overlay := prog.GetOverlay(); overlay != nil {
			if agg := overlay.GetAggregatedFileSystem(); agg != nil {
				wrapped := newPrefixedAggregatedFS(progName, agg, prog)
				p.overlayCache.Set(progName, &overlayCacheEntry{fs: wrapped, gen: gen})
				return wrapped
			}
		}
	}
	// Do NOT cache dbFS / negative results: a program may become a multi-layer
	// overlay moments later; caching the fallback would pin a wrong tree.
	return p.dbFS
}

func (p *ProgramFileSystem) Invalidate(progName string) {
	if progName == "" {
		return
	}
	InvalidateProgramFileSystemCache(progName)
	p.overlayCache.Remove(progName)
	if dropper, ok := p.dbFS.(interface{ DropProgramCache(string) }); ok {
		dropper.DropProgramCache(progName)
	}
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
		p.Invalidate(progName)
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
			// Last layer wins: Base is collected first, Diff layers overwrite.
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
