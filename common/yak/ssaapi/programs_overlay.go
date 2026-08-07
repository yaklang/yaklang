package ssaapi

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/utils/memedit"
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

func deleteFileFromAggregatedFS(vfs *filesys.VirtualFS, canonicalPath string) {
	if vfs == nil || canonicalPath == "" {
		return
	}
	vfsPath := overlayAggregatedFSPath(canonicalPath)
	if vfsPath == "" {
		return
	}
	if exists, _ := vfs.Exists(vfsPath); exists {
		_ = vfs.Delete(vfsPath)
	}
}

// cloneAndPatchAggregatedFS copies prev FS then applies newLayer FileHashMap (-1 delete, else upsert).
func cloneAndPatchAggregatedFS(prev fi.FileSystem, newLayer *Program) (*filesys.VirtualFS, error) {
	out := filesys.NewVirtualFs()
	if prev != nil {
		err := filesys.Recursive(".", filesys.WithFileSystem(prev), filesys.WithFileStat(func(path string, info os.FileInfo) error {
			content, err := prev.ReadFile(path)
			if err != nil {
				return nil
			}
			addFileToAggregatedFS(out, overlayPathFromAggregatedFS(path), string(content))
			return nil
		}))
		if err != nil {
			return nil, err
		}
	}
	if newLayer == nil || newLayer.Program == nil {
		return out, nil
	}
	fileHashMap := newLayer.Program.FileHashMap
	progName := newLayer.GetProgramName()
	for filePath, hash := range fileHashMap {
		path := normalizeOverlayFilePath(filePath, progName)
		if hash == -1 {
			deleteFileFromAggregatedFS(out, path)
			continue
		}
		if content, ok := readProgramFileContent(newLayer, path); ok {
			addFileToAggregatedFS(out, path, content)
		}
	}
	return out, nil
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
	if baseOverlay.AggregatedFS != nil {
		patched, err := cloneAndPatchAggregatedFS(baseOverlay.AggregatedFS, newLayerProgram)
		if err != nil {
			log.Warnf("patch AggregatedFS failed, falling back to full rebuild: %v", err)
			overlay.rebuildAggregatedFS()
		} else {
			overlay.AggregatedFS = patched
		}
	} else {
		overlay.rebuildAggregatedFS()
	}

	log.Infof("ProgramOverLay: Extended base+%d diffs, exclude=%d files",
		len(overlay.Diff), len(overlay.ExcludeFile))
	return overlay
}

// finishBuild wires programs and builds AggregatedFS from Diff.File + Base − ExcludeFile.
func (p *ProgramOverLay) finishBuild() {
	if p == nil {
		return
	}
	wireOverlayPrograms(p)
	p.rebuildAggregatedFS()
}

func (p *ProgramOverLay) rebuildAggregatedFS() {
	if p == nil {
		return
	}
	aggregatedFS, err := p.aggregateFileSystems()
	if err != nil {
		log.Errorf("failed to aggregate file systems: %v", err)
		return
	}
	p.AggregatedFS = aggregatedFS
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

// aggregateFileSystems builds the effective FS from ownership:
// Diff[i].File → that layer; base FileList − ExcludeFile → Base.
func (p *ProgramOverLay) aggregateFileSystems() (fi.FileSystem, error) {
	if p == nil || p.Base == nil {
		return nil, utils.Errorf("aggregateFileSystems requires Base program")
	}
	if len(p.Diff) == 0 {
		return nil, utils.Errorf("aggregateFileSystems requires at least one Diff layer")
	}

	aggregated := filesys.NewVirtualFs()
	for _, layer := range p.Diff {
		if layer == nil || layer.Program == nil {
			continue
		}
		for _, filePath := range layer.File {
			path := ensureOverlayPathSlash(filePath)
			if path == "" {
				continue
			}
			if content, ok := readProgramFileContent(layer.Program, path); ok {
				addFileToAggregatedFS(aggregated, path, content)
			}
		}
	}
	if p.Base.Program != nil {
		progName := p.Base.GetProgramName()
		exclude := overlayPathSet(p.ExcludeFile)
		for filePath, hash := range p.Base.Program.FileList {
			normalized := normalizeOverlayFilePath(filePath, progName)
			if normalized == "" {
				continue
			}
			if _, skip := exclude[normalized]; skip {
				continue
			}
			ed, err := p.Base.getEditor(filePath, hash)
			if err != nil || ed == nil {
				continue
			}
			addFileToAggregatedFS(aggregated, normalized, ed.GetSourceCode())
		}
	}
	return aggregated, nil
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

// Relocate maps a value from Base/older Diff onto the Diff layer that owns its file.
// Lookup is layer.Ref only (include File); no SyntaxFlow.
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
	if v.ParentProgram != nil {
		progName = v.ParentProgram.GetProgramName()
	}
	ownerDi, owned := p.ownerDiffIndex(normalizeOverlayFilePath(filePath, progName))
	if !owned || ownerDi < 0 || ownerDi >= len(p.Diff) {
		return v
	}
	if !fromBase && di >= ownerDi {
		return v
	}
	layer := p.Diff[ownerDi]
	if layer == nil {
		return v
	}

	wantOpcode := v.GetOpcode()
	seen := make(map[string]struct{})
	for _, name := range getValueNames(v) {
		if name == "" || strings.ContainsAny(name, "=-") {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		for _, cand := range layer.Ref(name) {
			if cand == nil {
				continue
			}
			if wantOpcode != "" && cand.GetOpcode() != wantOpcode {
				continue
			}
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
