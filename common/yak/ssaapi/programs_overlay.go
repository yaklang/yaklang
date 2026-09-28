package ssaapi

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path"
	"sort"
	"strings"

	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

// ProgramLayer represents one diff compile layer (never the base).
// File holds canonical paths this layer finally owns (include filter for Ref/Match).
type ProgramLayer struct {
	Program *Program
	File    []string
}

// Ref looks up a symbol only in this layer's owned files.
func (l *ProgramLayer) Ref(name string) Values {
	if l == nil || l.Program == nil {
		return nil
	}
	if len(l.File) == 0 {
		return nil
	}
	return l.Program.refWithIncludeFiles(name, l.File)
}

// ProgramOverLay is the dual-source incremental view: base Program + ordered diffs.
// Core model per layer:
//   - Diff[i].File = this layer's owned additions/modifications (include for scan)
//   - ExcludeFile  = ∪ all layers' additions ∪ deletions (Base must skip these)
// Ownership of a path is Diff[i].File itself (no separate owner index).
type ProgramOverLay struct {
	Base *Program
	Diff []*ProgramLayer
	// ExcludeFile: paths base Ref/Match must skip (owned ∪ deleted).
	ExcludeFile []string

	AggregatedFS fi.FileSystem
}

// IsIncrementalCompile 判断这个 program 是否是增量编译的
// program overlay 本质上就是增量编译的虚拟视图，所以返回 true
func (o *ProgramOverLay) IsIncrementalCompile() bool {
	return true
}

// topProgram returns the newest diff program, or Base when Diff is empty.
func (p *ProgramOverLay) topProgram() *Program {
	if p == nil {
		return nil
	}
	if n := len(p.Diff); n > 0 {
		if layer := p.Diff[n-1]; layer != nil {
			return layer.Program
		}
	}
	return p.Base
}

// IsBaseProgram reports whether the top program is a base program.
func (p *ProgramOverLay) IsBaseProgram() bool {
	if prog := p.topProgram(); prog != nil {
		return prog.IsBaseProgram()
	}
	return false
}

// GetBaseProgramName returns the base program name recorded on the top program.
func (p *ProgramOverLay) GetBaseProgramName() string {
	if prog := p.topProgram(); prog != nil {
		return prog.GetBaseProgramName()
	}
	return ""
}

// ProgramNames returns [base, diff0, diff1, ...] program names.
func (o *ProgramOverLay) ProgramNames() []string {
	if o == nil {
		return nil
	}
	names := make([]string, 0, 1+len(o.Diff))
	if o.Base != nil {
		names = append(names, o.Base.GetProgramName())
	}
	for _, layer := range o.Diff {
		if layer != nil && layer.Program != nil {
			names = append(names, layer.Program.GetProgramName())
		}
	}
	return names
}

var _ sfvm.ValueOperator = (*ProgramOverLay)(nil)

func (p *ProgramOverLay) GetProgramName() string {
	if p == nil {
		return ""
	}
	if top := p.topProgram(); top != nil {
		return top.GetProgramName()
	}
	return ""
}

func (p *ProgramOverLay) GetProgramKind() ssadb.ProgramKind {
	if p == nil {
		return ""
	}
	if top := p.topProgram(); top != nil {
		return top.GetProgramKind()
	}
	return ssadb.Application
}

func (p *ProgramOverLay) GetLanguage() ssaconfig.Language {
	if p == nil {
		return ""
	}
	if top := p.topProgram(); top != nil {
		return top.GetLanguage()
	}
	return ""
}

func (p *ProgramOverLay) Hash() (string, bool) {
	if p == nil {
		return "", false
	}
	names := p.ProgramNames()
	if len(names) == 0 {
		return "", false
	}
	args := make([]interface{}, len(names))
	for i, name := range names {
		args[i] = name
	}
	hash := utils.CalcSha256(args...)
	return hash, true
}

// ResetInterRuleState clears analysis caches on base and every diff program.
func (p *ProgramOverLay) ResetInterRuleState() {
	if p == nil {
		return
	}
	if p.Base != nil {
		p.Base.ResetInterRuleState()
	}
	for _, layer := range p.Diff {
		if layer != nil && layer.Program != nil {
			layer.Program.ResetInterRuleState()
		}
	}
}

func newEmptyOverlay() *ProgramOverLay {
	return &ProgramOverLay{Diff: make([]*ProgramLayer, 0)}
}

func (p *ProgramOverLay) ensureExcludePath(path string) {
	if p == nil {
		return
	}
	path = ensureOverlayPathSlash(path)
	if path == "" {
		return
	}
	for _, f := range p.ExcludeFile {
		if f == path {
			return
		}
	}
	p.ExcludeFile = append(p.ExcludeFile, path)
}

// stripOwnedPath removes path from the Diff layer that currently owns it (if any).
func (p *ProgramOverLay) stripOwnedPath(path string) {
	di, ok := p.ownerDiffIndex(path)
	if !ok || di < 0 || di >= len(p.Diff) || p.Diff[di] == nil {
		return
	}
	layer := p.Diff[di]
	path = ensureOverlayPathSlash(path)
	dst := layer.File[:0]
	for _, f := range layer.File {
		if f != path {
			dst = append(dst, f)
		}
	}
	layer.File = dst
}

// IsTopLayerProgram reports whether prog is the newest diff (or Base if no diffs).
func (p *ProgramOverLay) IsTopLayerProgram(prog *Program) bool {
	if p == nil || prog == nil {
		return false
	}
	top := p.topProgram()
	return top != nil && top.GetProgramName() == prog.GetProgramName()
}

// IsExcludedPath reports whether path is in ExcludeFile (owned or deleted).
func (p *ProgramOverLay) IsExcludedPath(path string) bool {
	if p == nil || path == "" {
		return false
	}
	path = ensureOverlayPathSlash(path)
	for _, f := range p.ExcludeFile {
		if f == path {
			return true
		}
	}
	return false
}

// ownerDiffIndex returns which Diff layer finally owns path (from Diff[i].File).
func (p *ProgramOverLay) ownerDiffIndex(path string) (int, bool) {
	if p == nil || path == "" {
		return -1, false
	}
	path = ensureOverlayPathSlash(path)
	for i := len(p.Diff) - 1; i >= 0; i-- {
		layer := p.Diff[i]
		if layer == nil {
			continue
		}
		for _, f := range layer.File {
			if f == path {
				return i, true
			}
		}
	}
	return -1, false
}

// valueSource locates which overlay source produced v: Base or Diff[di].
func (p *ProgramOverLay) valueSource(v *Value) (fromBase bool, di int, ok bool) {
	if v == nil || p == nil {
		return false, -1, false
	}
	programName := v.GetProgramName()
	if programName == "" {
		return false, -1, false
	}
	if p.Base != nil && p.Base.GetProgramName() == programName {
		return true, -1, true
	}
	for i, layer := range p.Diff {
		if layer != nil && layer.Program != nil && layer.Program.GetProgramName() == programName {
			return false, i, true
		}
	}
	return false, -1, false
}

// readProgramFileContent looks up source by FileList/GetEditor (O(1) candidates, no full scan).
func readProgramFileContent(prog *Program, filePath string) (string, bool) {
	if prog == nil || prog.Program == nil || filePath == "" {
		return "", false
	}
	progName := prog.GetProgramName()
	rel := strings.TrimPrefix(ensureOverlayPathSlash(filePath), "/")
	candidates := []string{ensureOverlayPathSlash(filePath), rel}
	if progName != "" {
		candidates = append([]string{"/" + progName + "/" + rel, progName + "/" + rel}, candidates...)
	}
	for _, candidate := range candidates {
		if hash, ok := prog.Program.FileList[candidate]; ok {
			if ed, err := prog.getEditor(candidate, hash); err == nil && ed != nil {
				return ed.GetSourceCode(), true
			}
		}
		if ed, ok := prog.Program.GetEditor(candidate); ok && ed != nil {
			return ed.GetSourceCode(), true
		}
	}
	want := ensureOverlayPathSlash(filePath)
	for path, hash := range prog.Program.FileList {
		if normalizeOverlayFilePath(path, progName) != want {
			continue
		}
		if ed, err := prog.getEditor(path, hash); err == nil && ed != nil {
			return ed.GetSourceCode(), true
		}
	}
	return "", false
}

func addFileToAggregatedFS(vfs *filesys.VirtualFS, canonicalPath, content string) {
	if vfs == nil || canonicalPath == "" {
		return
	}
	vfsPath := overlayAggregatedFSPath(canonicalPath)
	if vfsPath == "" {
		return
	}
	vfs.AddFile(vfsPath, content)
}

// applyLayerFileHashMap appends a Diff layer and applies its FileHashMap directly:
//   - add/mod  → strip from older Diff.File, own on new layer.File, add to ExcludeFile
//   - delete   → strip from older Diff.File, add to ExcludeFile (not owned)
func applyLayerFileHashMap(overlay *ProgramOverLay, diffProg *Program) error {
	if overlay == nil {
		return utils.Errorf("overlay is nil")
	}
	if diffProg == nil || diffProg.Program == nil {
		return utils.Errorf("diff program is nil")
	}
	fileHashMap := diffProg.Program.FileHashMap
	if len(fileHashMap) == 0 {
		return utils.Errorf("FileHashMap is required for diff program %s, but it is empty", diffProg.GetProgramName())
	}

	layer := &ProgramLayer{Program: diffProg}
	for filePath, hash := range fileHashMap {
		if filePath == "" {
			continue
		}
		path := normalizeOverlayFilePath(filePath, diffProg.GetProgramName())
		overlay.stripOwnedPath(path)
		overlay.ensureExcludePath(path)
		if hash != -1 {
			layer.File = append(layer.File, path)
		}
	}
	overlay.Diff = append(overlay.Diff, layer)
	return nil
}

func createOverlayFromLayers(programs ...*Program) *ProgramOverLay {
	if len(programs) < 2 {
		log.Errorf("createOverlayFromLayers requires at least 2 programs, got %d", len(programs))
		return nil
	}

	overlay := newEmptyOverlay()
	overlay.Base = programs[0]
	overlay.Diff = make([]*ProgramLayer, 0, len(programs)-1)

	for i := 1; i < len(programs); i++ {
		if programs[i] == nil {
			continue
		}
		if err := applyLayerFileHashMap(overlay, programs[i]); err != nil {
			log.Errorf("createOverlayFromLayers: %v", err)
			return nil
		}
	}

	overlay.finishBuild()
	log.Infof("ProgramOverLay: Built base+%d diffs, exclude=%d files",
		len(overlay.Diff), len(overlay.ExcludeFile))
	return overlay
}

func wireOverlayPrograms(overlay *ProgramOverLay) {
	if overlay == nil {
		return
	}
	if overlay.Base != nil {
		if overlay.Base.overlay == nil {
			overlay.Base.overlay = overlay
		}
		overlay.ensureProgramLoaded(overlay.Base)
	}
	for _, layer := range overlay.Diff {
		if layer == nil || layer.Program == nil {
			continue
		}
		if layer.Program.overlay == nil {
			layer.Program.overlay = overlay
		}
		overlay.ensureProgramLoaded(layer.Program)
	}
}

// extendOverlayWithNewLayer reuses Base and prior Diff programs, appends a new diff.
// Avoids re-touching Base so its updated_at stays stable.
func extendOverlayWithNewLayer(baseOverlay *ProgramOverLay, newLayerProgram *Program) *ProgramOverLay {
	if baseOverlay == nil || baseOverlay.Base == nil {
		return nil
	}

	overlay := newEmptyOverlay()
	overlay.Base = baseOverlay.Base
	overlay.ExcludeFile = append([]string(nil), baseOverlay.ExcludeFile...)
	overlay.Diff = make([]*ProgramLayer, 0, len(baseOverlay.Diff)+1)
	for _, layer := range baseOverlay.Diff {
		if layer == nil {
			continue
		}
		// Copy File slice so ownership mutations don't mutate the previous overlay.
		copied := append([]string(nil), layer.File...)
		overlay.Diff = append(overlay.Diff, &ProgramLayer{Program: layer.Program, File: copied})
	}

	if err := applyLayerFileHashMap(overlay, newLayerProgram); err != nil {
		log.Errorf("extendOverlayWithNewLayer: %v", err)
		return nil
	}

	wireOverlayPrograms(overlay)
	// AggregatedFS is lazy (built from ownership metadata on demand), so no
	// clone/patch of the previous overlay's FS is needed here: the new overlay
	// simply owns Base + copied layers + the new diff.

	log.Infof("ProgramOverLay: Extended base+%d diffs, exclude=%d files",
		len(overlay.Diff), len(overlay.ExcludeFile))
	return overlay
}

// finishBuild wires programs; AggregatedFS stays lazy (metadata only).
func (p *ProgramOverLay) finishBuild() {
	if p == nil {
		return
	}
	wireOverlayPrograms(p)
}

func NewProgramOverLay(layers ...*Program) *ProgramOverLay {
	valid := make([]*Program, 0, len(layers))
	for _, layer := range layers {
		if layer != nil {
			valid = append(valid, layer)
		}
	}
	if len(valid) == 0 {
		return newEmptyOverlay()
	}
	if len(valid) < 2 {
		log.Errorf("NewProgramOverLay requires at least 2 layers, got %d", len(valid))
		return nil
	}
	return createOverlayFromLayers(valid...)
}

// lazyOverlayFS is the aggregated view of a ProgramOverLay without materializing
// file bodies: ownership (Diff[].File + Base.FileList − ExcludeFile) is plain
// metadata, and ReadFile resolves content on demand through each layer's
// editors (O(1) lookups). Building the overlay therefore never walks/copies
// file contents; the first ReadFile/Stat pays the per-file lookup instead.
type lazyOverlayFS struct {
	overlay *ProgramOverLay
	// entries: canonical path ("/"-prefixed, no program name) -> owning layer index.
	// index -1 means owned by Base.
	entries map[string]int
	// dirs caches the derived parent-dir set for ReadDir/Stat of directories.
	dirs map[string]bool
}

var (
	_ fi.FileSystem         = (*lazyOverlayFS)(nil)
	_ fi.ReadOnlyFileSystem = (*lazyOverlayFS)(nil)
)

// buildLazyOverlayFS derives the path→owner index from the overlay's
// ownership structure. Paths are canonical overlay paths ("/"-prefixed,
// program-name prefix stripped), matching the legacy materialized layout.
func buildLazyOverlayFS(o *ProgramOverLay) *lazyOverlayFS {
	fs := &lazyOverlayFS{
		overlay: o,
		entries: make(map[string]int),
		dirs:    map[string]bool{"/": true},
	}
	if o == nil {
		return fs
	}
	for i, layer := range o.Diff {
		if layer == nil || layer.Program == nil {
			continue
		}
		for _, filePath := range layer.File {
			path := ensureOverlayPathSlash(filePath)
			if path == "" || path == "/" {
				continue
			}
			fs.entries[path] = i
		}
	}
	if o.Base != nil && o.Base.Program != nil {
		exclude := overlayPathSet(o.ExcludeFile)
		progName := o.Base.GetProgramName()
		for filePath := range o.Base.Program.FileList {
			normalized := normalizeOverlayFilePath(filePath, progName)
			if normalized == "" || normalized == "/" {
				continue
			}
			if _, skip := exclude[normalized]; skip {
				continue
			}
			if _, owned := fs.entries[normalized]; owned {
				continue
			}
			fs.entries[normalized] = -1
		}
	}
	for p := range fs.entries {
		for dir := path.Dir(p); dir != "/" && dir != "."; dir = path.Dir(dir) {
			fs.dirs[dir] = true
		}
	}
	return fs
}

// owner resolves which layer owns canonical path.
func (f *lazyOverlayFS) owner(name string) (int, bool) {
	owner, ok := f.entries[name]
	return owner, ok
}

// contentAt reads the file content for canonical path from its owning layer.
func (f *lazyOverlayFS) contentAt(name string, owner int) ([]byte, error) {
	if f.overlay == nil {
		return nil, utils.Errorf("overlay file system: overlay is nil")
	}
	// Diff layers (and Base) resolve content through their own editors;
	// later layers never own a path owned earlier (applyLayerFileHashMap strips).
	var prog *Program
	if owner >= 0 {
		if layer := f.overlay.Diff[owner]; layer != nil {
			prog = layer.Program
		}
	} else {
		prog = f.overlay.Base
	}
	if prog == nil {
		return nil, utils.Errorf("overlay file system: no owner program for [%v]", name)
	}
	if content, ok := readProgramFileContent(prog, name); ok {
		return []byte(content), nil
	}
	return nil, utils.Errorf("overlay file system: file [%v] not found", name)
}

func (f *lazyOverlayFS) clean(name string) string {
	name = path.Clean("/" + strings.TrimPrefix(name, "/"))
	if name == "" || name == "." {
		return "/"
	}
	return name
}

func (f *lazyOverlayFS) ReadFile(name string) ([]byte, error) {
	name = f.clean(name)
	owner, ok := f.owner(name)
	if !ok {
		return nil, utils.Errorf("file [%v] not exist", name)
	}
	return f.contentAt(name, owner)
}

func (f *lazyOverlayFS) Stat(name string) (os.FileInfo, error) {
	name = f.clean(name)
	if owner, ok := f.owner(name); ok {
		data, err := f.contentAt(name, owner)
		if err != nil {
			return nil, err
		}
		return filesys.NewVirtualFileInfo(path.Base(name), int64(len(data)), false), nil
	}
	if f.dirs[name] {
		return filesys.NewVirtualFileInfo(path.Base(name), 0, true), nil
	}
	return nil, utils.Errorf("path [%v] not exist", name)
}

func (f *lazyOverlayFS) ReadDir(name string) ([]fs.DirEntry, error) {
	name = f.clean(name)
	if !f.dirs[name] {
		if _, ok := f.entries[name]; !ok {
			return nil, utils.Errorf("directory [%v] not exist", name)
		}
		return nil, utils.Errorf("path [%v] is a file, not a directory", name)
	}
	// Listing "/" must strip the leading "/" so first-segment names don't
	// trip the "contains /" filter below.
	prefix := "/"
	if name != "/" {
		prefix = name + "/"
	}
	seen := make(map[string]bool)
	var out []fs.DirEntry
	for p := range f.entries {
		if !strings.HasPrefix(p, prefix) {
			continue
		}
		base := strings.TrimSuffix(strings.TrimPrefix(p, prefix), "/")
		if base == "" || strings.Contains(base, "/") {
			continue
		}
		if seen[base] {
			continue
		}
		seen[base] = true
		out = append(out, filesys.NewVirtualFileInfo(base, int64(len(p)), false))
	}
	for dir := range f.dirs {
		if dir == "/" || !strings.HasPrefix(dir, prefix) {
			continue
		}
		base := strings.TrimSuffix(strings.TrimPrefix(dir, prefix), "/")
		if base == "" || strings.Contains(base, "/") || seen[base] {
			continue
		}
		seen[base] = true
		out = append(out, filesys.NewVirtualFileInfo(base, 0, true))
	}
	// Deterministic order keeps tree listings stable across rebuilds.
	sort.Slice(out, func(i, j int) bool { return out[i].Name() < out[j].Name() })
	return out, nil
}

func (f *lazyOverlayFS) Open(name string) (fs.File, error) {
	data, err := f.ReadFile(name)
	if err != nil {
		if f.dirs[f.clean(name)] {
			return &lazyOverlayDir{fs: f, name: f.clean(name)}, nil
		}
		return nil, err
	}
	vf := filesys.NewVirtualFs()
	vf.AddFile(f.clean(name), string(data))
	return vf.Open(f.clean(name))
}

func (f *lazyOverlayFS) OpenFile(name string, _ int, _ os.FileMode) (fs.File, error) {
	return f.Open(name)
}

func (f *lazyOverlayFS) ExtraInfo(string) map[string]any { return nil }
func (f *lazyOverlayFS) Delete(string) error {
	return utils.Error("overlay aggregated file system is read-only")
}
func (f *lazyOverlayFS) GetSeparators() rune { return '/' }
func (f *lazyOverlayFS) Join(elem ...string) string {
	return path.Join(elem...)
}
func (f *lazyOverlayFS) Base(name string) string { return path.Base(name) }
func (f *lazyOverlayFS) PathSplit(name string) (string, string) {
	dir, file := path.Split(name)
	if len(dir) > 1 && strings.HasSuffix(dir, "/") {
		dir = dir[:len(dir)-1]
	}
	return dir, file
}
func (f *lazyOverlayFS) Ext(name string) string  { return path.Ext(name) }
func (f *lazyOverlayFS) IsAbs(name string) bool  { return len(name) > 0 && name[0] == '/' }
func (f *lazyOverlayFS) Getwd() (string, error)  { return "", nil }
func (f *lazyOverlayFS) Exists(name string) (bool, error) {
	name = f.clean(name)
	if _, ok := f.entries[name]; ok {
		return true, nil
	}
	return f.dirs[name], nil
}
func (f *lazyOverlayFS) Rel(from, to string) (string, error) {
	if from == "" || to == "" {
		return "", utils.Error("Rel requires non-empty paths")
	}
	// Best-effort relative resolution over the canonical tree.
	from, to = f.clean(from), f.clean(to)
	if from == to {
		return ".", nil
	}
	if strings.HasPrefix(to, from+"/") {
		return strings.TrimPrefix(to, from+"/"), nil
	}
	if strings.HasPrefix(from, to+"/") {
		depth := strings.Count(strings.TrimPrefix(from, to+"/"), "/") + 1
		return strings.Repeat("../", depth), nil
	}
	return "", utils.Errorf("cannot make [%v] relative to [%v]", to, from)
}
func (f *lazyOverlayFS) Rename(string, string) error {
	return utils.Error("overlay aggregated file system is read-only")
}
func (f *lazyOverlayFS) WriteFile(string, []byte, os.FileMode) error {
	return utils.Error("overlay aggregated file system is read-only")
}
func (f *lazyOverlayFS) MkdirAll(string, os.FileMode) error {
	return utils.Error("overlay aggregated file system is read-only")
}

// lazyOverlayDir adapts a lazyOverlayFS directory to fs.File (Open on a dir).
type lazyOverlayDir struct {
	fs   *lazyOverlayFS
	name string
}

func (d *lazyOverlayDir) Stat() (fs.FileInfo, error) {
	return d.fs.Stat(d.name)
}
func (d *lazyOverlayDir) Read([]byte) (int, error) {
	return 0, utils.Error("directory is not readable")
}
func (d *lazyOverlayDir) Close() error { return nil }

// aggregateFileSystems builds the effective FS from ownership:
// Diff[i].File → that layer; base FileList − ExcludeFile → Base.
func (p *ProgramOverLay) aggregateFileSystems() (fi.FileSystem, error) {
	if p == nil || p.Base == nil {
		return nil, utils.Errorf("aggregateFileSystems requires Base program")
	}
	if len(p.Diff) == 0 {
		return nil, utils.Errorf("aggregateFileSystems requires at least one Diff layer")
	}
	return buildLazyOverlayFS(p), nil
}

// ProgramCount returns the program-stack size: 1(base)+len(Diff).
func (p *ProgramOverLay) ProgramCount() int {
	if p == nil {
		return 0
	}
	n := len(p.Diff)
	if p.Base != nil {
		n++
	}
	return n
}

func (p *ProgramOverLay) GetFileCount() int {
	if p == nil {
		return 0
	}
	// Prefer already-built AggregatedFS (exact visible set).
	if p.AggregatedFS != nil {
		n := 0
		_ = filesys.Recursive(".", filesys.WithFileSystem(p.AggregatedFS), filesys.WithFileStat(func(_ string, _ os.FileInfo) error {
			n++
			return nil
		}))
		return n
	}
	// Diff[i].File paths are exclusive after applyLayerFileHashMap; ExcludeFile ⊇ owned.
	n := 0
	for _, layer := range p.Diff {
		if layer != nil {
			n += len(layer.File)
		}
	}
	if p.Base != nil && p.Base.Program != nil {
		exclude := overlayPathSet(p.ExcludeFile)
		for filePath := range p.Base.Program.FileList {
			normalized := normalizeOverlayFilePath(filePath, p.Base.GetProgramName())
			if normalized == "" {
				continue
			}
			if _, skip := exclude[normalized]; skip {
				continue
			}
			n++
		}
	}
	return n
}

// GetAggregatedFileSystem 获取聚合后的文件系统
func (p *ProgramOverLay) GetAggregatedFileSystem() fi.FileSystem {
	if p == nil {
		return nil
	}
	if p.AggregatedFS == nil {
		aggregatedFS, err := p.aggregateFileSystems()
		if err != nil {
			log.Warnf("failed to rebuild aggregated file system: %v", err)
			return nil
		}
		p.AggregatedFS = aggregatedFS
	}
	return p.AggregatedFS
}

func getValueFilePath(v *Value) string {
	if v == nil {
		return ""
	}
	rng := v.GetRange()
	if rng == nil {
		return ""
	}
	editor := rng.GetEditor()
	if editor == nil {
		return ""
	}
	filePath := editor.GetFilePath()
	if filePath == "" {
		filePath = editor.GetUrl()
	}
	return filePath
}

func overlayPathSet(paths []string) map[string]struct{} {
	m := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		path = ensureOverlayPathSlash(path)
		if path != "" {
			m[path] = struct{}{}
		}
	}
	return m
}

// Ref dual-source: Diff include(owned File) then Base exclude(ExcludeFile).
func (p *ProgramOverLay) Ref(name string) Values {
	var result Values
	if p == nil || p.Base == nil {
		return result
	}

	for i := len(p.Diff) - 1; i >= 0; i-- {
		layer := p.Diff[i]
		if layer == nil {
			continue
		}
		result = append(result, layer.Ref(name)...)
	}

	if len(p.ExcludeFile) > 0 {
		result = append(result, p.Base.refWithExcludeFiles(name, p.ExcludeFile)...)
	} else {
		result = append(result, p.Base.Ref(name)...)
	}
	return result
}

// relocateNameCandidates returns stable names for cross-layer SSA lookup.
// Drops empty / operator-like noise; keeps method names (e.g. getValue) that
// NameMatch may not index (Java methods are often stored as Class_method_<hash>).
func relocateNameCandidates(v *Value) []string {
	if v == nil {
		return nil
	}
	seen := make(map[string]struct{})
	var out []string
	for _, name := range getValueNames(v) {
		if name == "" || strings.ContainsAny(name, "=-") {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	return out
}

func relocateNamesIntersect(cand *Value, names map[string]struct{}) bool {
	if cand == nil || len(names) == 0 {
		return false
	}
	for _, name := range getValueNames(cand) {
		if name == "" {
			continue
		}
		if _, ok := names[name]; ok {
			return true
		}
	}
	return false
}

// Relocate maps a value from Base/older Diff onto the Diff layer that owns its file.
// Direct SSA only (no nested SyntaxFlow):
//  1. Program.Ref by name + opcode (variables / NameMatch hits)
//  2. MatchInstructionByOpcodes + stable name intersection (Functions keyed by
//     method name, etc., which Ref/NameMatch alone often misses)
func (p *ProgramOverLay) Relocate(v *Value) *Value {
	if v == nil || p == nil {
		return v
	}
	filePath := getValueFilePath(v)
	if filePath == "" {
		return v
	}
	fromBase, di, ok := p.valueSource(v)
	if !ok {
		return v
	}
	progName := ""
	if fromBase && p.Base != nil {
		progName = p.Base.GetProgramName()
	} else if di >= 0 && di < len(p.Diff) && p.Diff[di] != nil && p.Diff[di].Program != nil {
		progName = p.Diff[di].Program.GetProgramName()
	}
	normalizedPath := normalizeOverlayFilePath(filePath, progName)
	ownerDi, owned := p.ownerDiffIndex(normalizedPath)
	if !owned || ownerDi < 0 || ownerDi >= len(p.Diff) {
		return v
	}
	// Only relocate when value comes from Base or an older Diff than the owner.
	if !fromBase && di >= ownerDi {
		return v
	}
	layer := p.Diff[ownerDi]
	if layer == nil || layer.Program == nil || layer.Program.Program == nil {
		return v
	}

	names := relocateNameCandidates(v)
	if len(names) == 0 {
		return v
	}
	nameSet := make(map[string]struct{}, len(names))
	for _, name := range names {
		nameSet[name] = struct{}{}
	}
	wantOpcode := v.getOpcode()

	// Fast path: variable NameMatch index.
	for _, name := range names {
		for _, cand := range layer.Program.Ref(name) {
			if cand == nil {
				continue
			}
			if wantOpcode != ssa.SSAOpcodeUnKnow && cand.getOpcode() != wantOpcode {
				continue
			}
			return cand
		}
	}

	// Opcode scan: match Functions/others by stable names (method name, verbose…).
	if wantOpcode == ssa.SSAOpcodeUnKnow {
		return v
	}
	for _, inst := range ssa.MatchInstructionByOpcodes(context.Background(), layer.Program.Program, wantOpcode) {
		cand, err := layer.Program.NewValue(inst)
		if err != nil || cand == nil {
			continue
		}
		if relocateNamesIntersect(cand, nameSet) {
			return cand
		}
	}
	return v
}

func (p *ProgramOverLay) ensureProgramLoaded(prog *Program) {
	if prog == nil || prog.Program == nil {
		return
	}
	func() {
		defer func() {
			if r := recover(); r != nil {
				log.Debugf("LazyBuild panic for program %s: %v", prog.GetProgramName(), r)
			}
		}()
		prog.Program.LazyBuild()
	}()
}

func (p *ProgramOverLay) Show() *ProgramOverLay {
	if p == nil {
		return p
	}
	if p.Base != nil {
		fmt.Printf("=== Base %s ===\n", p.Base.GetProgramName())
		p.Base.Show()
		fmt.Println()
	}
	for i, layer := range p.Diff {
		if layer != nil && layer.Program != nil {
			fmt.Printf("=== Diff %d (%s) files=%d ===\n", i, layer.Program.GetProgramName(), len(layer.File))
			layer.Program.Show()
			fmt.Println()
		}
	}
	return p
}

func (p *ProgramOverLay) String() string {
	if p == nil {
		return "ProgramOverLay(nil)"
	}
	return fmt.Sprintf("ProgramOverLay(base+diffs=%d, exclude=%d)", p.ProgramCount(), len(p.ExcludeFile))
}

func (p *ProgramOverLay) IsMap() bool {
	return false
}

func (p *ProgramOverLay) IsList() bool {
	return false
}

func (p *ProgramOverLay) IsEmpty() bool {
	if p == nil {
		return true
	}
	if p.Base != nil && !p.Base.IsEmpty() {
		return false
	}
	for _, layer := range p.Diff {
		if layer != nil && layer.Program != nil && !layer.Program.IsEmpty() {
			return false
		}
	}
	return true
}

func (p *ProgramOverLay) GetAnchorBitVector() *utils.BitVector {
	return nil
}

func (p *ProgramOverLay) SetAnchorBitVector(*utils.BitVector) {}

func (p *ProgramOverLay) ShouldUseConditionCandidate() bool {
	return true
}

func (p *ProgramOverLay) GetOpcode() string {
	return ""
}

func (p *ProgramOverLay) GetBinaryOperator() string {
	return ""
}

func (p *ProgramOverLay) GetUnaryOperator() string {
	return ""
}

func (p *ProgramOverLay) Recursive(f func(sfvm.ValueOperator) error) error {
	if p == nil {
		return nil
	}
	for i := len(p.Diff) - 1; i >= 0; i-- {
		layer := p.Diff[i]
		if layer != nil && layer.Program != nil {
			if err := f(layer.Program); err != nil {
				return err
			}
		}
	}
	if p.Base != nil {
		if err := f(p.Base); err != nil {
			return err
		}
	}
	return nil
}

// queryMatch uses dual-source routing: Diff include, Base exclude.
// Include/exclude filters already enforce visibility.
func (p *ProgramOverLay) queryMatch(
	ctx context.Context,
	mod ssadb.MatchMode,
	compareMode ssadb.CompareMode,
	query string,
) (bool, sfvm.Values, error) {
	if p == nil || p.Base == nil {
		return false, nil, nil
	}

	results := make([]sfvm.ValueOperator, 0)
	appendMatch := func(vals sfvm.Values) {
		if vals == nil {
			return
		}
		_ = vals.Recursive(func(op sfvm.ValueOperator) error {
			results = append(results, op)
			return nil
		})
	}

	for i := len(p.Diff) - 1; i >= 0; i-- {
		layer := p.Diff[i]
		if layer == nil || layer.Program == nil || len(layer.File) == 0 {
			continue
		}
		matched, vals, err := layer.Program.matchVariableWithIncludeFiles(ctx, compareMode, mod, query, layer.File)
		if err != nil || !matched {
			continue
		}
		appendMatch(vals)
	}

	matched, vals, err := p.Base.matchVariableWithExcludeFiles(ctx, compareMode, mod, query, p.ExcludeFile)
	if err == nil && matched {
		appendMatch(vals)
	}

	return len(results) > 0, sfvm.NewValues(results), nil
}

func (p *ProgramOverLay) ExactMatch(ctx context.Context, mod ssadb.MatchMode, want string) (bool, sfvm.Values, error) {
	return p.queryMatch(ctx, mod, ssadb.ExactCompare, want)
}

func (p *ProgramOverLay) GlobMatch(ctx context.Context, mod ssadb.MatchMode, g string) (bool, sfvm.Values, error) {
	return p.queryMatch(ctx, mod, ssadb.GlobCompare, g)
}

func (p *ProgramOverLay) RegexpMatch(ctx context.Context, mod ssadb.MatchMode, re string) (bool, sfvm.Values, error) {
	return p.queryMatch(ctx, mod, ssadb.RegexpCompare, re)
}

func (p *ProgramOverLay) GetCalled() (sfvm.Values, error) {
	return nil, utils.Error("ProgramOverLay does not support GetCalled")
}

func (p *ProgramOverLay) GetCallActualParams(index int, contain bool) (sfvm.Values, error) {
	return nil, utils.Error("ProgramOverLay does not support GetCallActualParams")
}

func (p *ProgramOverLay) GetFields() (sfvm.Values, error) {
	return sfvm.NewEmptyValues(), nil
}

func (p *ProgramOverLay) GetSyntaxFlowUse() (sfvm.Values, error) {
	return nil, utils.Error("ProgramOverLay does not support GetSyntaxFlowUse")
}

func (p *ProgramOverLay) GetSyntaxFlowDef() (sfvm.Values, error) {
	return nil, utils.Error("ProgramOverLay does not support GetSyntaxFlowDef")
}

func (p *ProgramOverLay) GetSyntaxFlowTopDef(sfResult *sfvm.SFFrameResult, sfConfig *sfvm.Config, config ...*sfvm.RecursiveConfigItem) (sfvm.Values, error) {
	return nil, utils.Error("ProgramOverLay does not support GetSyntaxFlowTopDef")
}

func (p *ProgramOverLay) GetSyntaxFlowBottomUse(sfResult *sfvm.SFFrameResult, sfConfig *sfvm.Config, config ...*sfvm.RecursiveConfigItem) (sfvm.Values, error) {
	return nil, utils.Error("ProgramOverLay does not support GetSyntaxFlowBottomUse")
}

func (p *ProgramOverLay) ListIndex(i int) (sfvm.ValueOperator, error) {
	return nil, utils.Error("ProgramOverLay does not support ListIndex")
}

func (p *ProgramOverLay) Merge(values ...sfvm.ValueOperator) (sfvm.ValueOperator, error) {
	return nil, utils.Error("ProgramOverLay does not support Merge")
}

func (p *ProgramOverLay) Remove(values ...sfvm.ValueOperator) (sfvm.ValueOperator, error) {
	return nil, utils.Error("ProgramOverLay does not support Remove")
}

func (p *ProgramOverLay) AppendPredecessor(operator sfvm.ValueOperator, opts ...sfvm.AnalysisContextOption) error {
	return nil
}

func (p *ProgramOverLay) FileFilter(path string, match string, rule map[string]string, rule2 []string) (sfvm.Values, error) {
	return nil, utils.Error("ProgramOverLay does not support FileFilter")
}

// compareAcrossLayers: Diff include(File), Base exclude(ExcludeFile) — same dual-source as Ref/Match.
func (p *ProgramOverLay) compareAcrossLayers(
	compareInclude func(*Program, []string) (sfvm.Values, []bool),
	compareExclude func(*Program, []string) (sfvm.Values, []bool),
) sfvm.Values {
	if p == nil || p.Base == nil {
		return sfvm.NewEmptyValues()
	}

	results := make([]sfvm.ValueOperator, 0)
	appendVals := func(vals sfvm.Values) {
		if vals == nil || vals.IsEmpty() {
			return
		}
		_ = vals.Recursive(func(op sfvm.ValueOperator) error {
			results = append(results, op)
			return nil
		})
	}

	for i := len(p.Diff) - 1; i >= 0; i-- {
		layer := p.Diff[i]
		if layer == nil || layer.Program == nil || len(layer.File) == 0 {
			continue
		}
		values, _ := compareInclude(layer.Program, layer.File)
		appendVals(values)
	}

	values, _ := compareExclude(p.Base, p.ExcludeFile)
	appendVals(values)
	return sfvm.NewValues(results)
}

func (p *ProgramOverLay) CompareString(comparator *sfvm.StringComparator) (sfvm.Values, []bool) {
	return p.compareAcrossLayers(
		func(prog *Program, include []string) (sfvm.Values, []bool) {
			return prog.compareStringWithFileFilter(comparator, include, nil)
		},
		func(prog *Program, exclude []string) (sfvm.Values, []bool) {
			return prog.compareStringWithFileFilter(comparator, nil, exclude)
		},
	), nil
}

func (p *ProgramOverLay) CompareOpcode(comparator *sfvm.OpcodeComparator) (sfvm.Values, []bool) {
	return p.compareAcrossLayers(
		func(prog *Program, include []string) (sfvm.Values, []bool) {
			return prog.compareOpcodeWithFileFilter(comparator, include, nil)
		},
		func(prog *Program, exclude []string) (sfvm.Values, []bool) {
			return prog.compareOpcodeWithFileFilter(comparator, nil, exclude)
		},
	), nil
}

func (p *ProgramOverLay) CompareConst(comparator *sfvm.ConstComparator) bool {
	return false
}

func (p *ProgramOverLay) NewConst(i any, rng ...*memedit.Range) sfvm.ValueOperator {
	if p == nil {
		return nil
	}
	top := p.topProgram()
	if top == nil {
		return nil
	}
	return top.NewConst(i, rng...)
}
