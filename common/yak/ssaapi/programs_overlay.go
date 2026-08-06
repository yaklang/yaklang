package ssaapi

import (
	"context"
	"fmt"
	"io"
	"os"
	"regexp"
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
// Base is NOT a ProgramLayer. Ownership is ownerByPath; deleted-only paths live in
// deletedFiles. Base queries exclude ExcludeFile (ownerByPath ∪ deletedFiles).
type ProgramOverLay struct {
	Base *Program
	Diff []*ProgramLayer
	// ExcludeFile: paths base Ref/Match must skip (owned ∪ deleted), rebuilt with ownership maps.
	ExcludeFile []string

	// ownerByPath: canonical path -> Diff index that finally owns it.
	ownerByPath map[string]int
	// deletedFiles: paths removed in some diff and not re-owned.
	deletedFiles *utils.SafeMap[struct{}]

	AggregatedFS   fi.FileSystem
	signatureCache *utils.CacheWithKey[string, *Value]
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
	return &ProgramOverLay{
		Diff:           make([]*ProgramLayer, 0),
		ownerByPath:    make(map[string]int),
		deletedFiles:   utils.NewSafeMap[struct{}](),
		signatureCache: utils.NewTTLCacheWithKey[string, *Value](0),
	}
}

func (p *ProgramOverLay) setOwnerByPath(owner map[string]int) {
	if p == nil {
		return
	}
	p.ownerByPath = make(map[string]int, len(owner))
	for path, di := range owner {
		p.ownerByPath[ensureOverlayPathSlash(path)] = di
	}
	p.rebuildExcludeFile()
}

func (p *ProgramOverLay) setDeletedFiles(deleted map[string]struct{}) {
	if p == nil {
		return
	}
	p.deletedFiles = utils.NewSafeMap[struct{}]()
	for path := range deleted {
		p.deletedFiles.Set(ensureOverlayPathSlash(path), struct{}{})
	}
	p.rebuildExcludeFile()
}

// rebuildExcludeFile refreshes ExcludeFile from ownerByPath ∪ deletedFiles.
func (p *ProgramOverLay) rebuildExcludeFile() {
	if p == nil {
		return
	}
	n := len(p.ownerByPath)
	if p.deletedFiles != nil {
		n += p.deletedFiles.Count()
	}
	out := make([]string, 0, n)
	seen := make(map[string]struct{}, n)
	add := func(path string) {
		path = ensureOverlayPathSlash(path)
		if path == "" {
			return
		}
		if _, ok := seen[path]; ok {
			return
		}
		seen[path] = struct{}{}
		out = append(out, path)
	}
	for path := range p.ownerByPath {
		add(path)
	}
	if p.deletedFiles != nil {
		p.deletedFiles.ForEach(func(path string, _ struct{}) bool {
			add(path)
			return true
		})
	}
	p.ExcludeFile = out
}

// excludeFiles returns paths base queries must skip: owned ∪ deleted.
func (p *ProgramOverLay) excludeFiles() []string {
	if p == nil {
		return nil
	}
	return p.ExcludeFile
}

func (p *ProgramOverLay) excludeCount() int {
	if p == nil {
		return 0
	}
	return len(p.ExcludeFile)
}

// IsTopLayerProgram reports whether prog is the newest diff (or Base if no diffs).
func (p *ProgramOverLay) IsTopLayerProgram(prog *Program) bool {
	if p == nil || prog == nil {
		return false
	}
	top := p.topProgram()
	return top != nil && top.GetProgramName() == prog.GetProgramName()
}

// IsExcludedPath reports whether path is owned by a Diff or deleted.
func (p *ProgramOverLay) IsExcludedPath(path string) bool {
	if p == nil || path == "" {
		return false
	}
	path = ensureOverlayPathSlash(path)
	if _, ok := p.ownerDiffIndex(path); ok {
		return true
	}
	return p.isFileDeleted(path)
}

func (p *ProgramOverLay) ownerDiffIndex(path string) (int, bool) {
	if p == nil || path == "" || p.ownerByPath == nil {
		return -1, false
	}
	di, ok := p.ownerByPath[ensureOverlayPathSlash(path)]
	if !ok {
		return -1, false
	}
	return di, true
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

func findFileInProgram(prog *Program, filePath string) (found bool, content string) {
	if prog == nil {
		return false, ""
	}

	prog.ForEachAllFile(func(path string, editor *memedit.MemEditor) bool {
		if path == filePath {
			content = editor.GetSourceCode()
			found = true
			return false
		}
		return true
	})

	return found, content
}

// findFileInProgramWithPrefix 查找文件，自动尝试带前缀和不带前缀的路径
func findFileInProgramWithPrefix(prog *Program, filePath string, programName string) (found bool, content string) {
	if prog == nil {
		return false, ""
	}
	if programName != "" {
		pathWithPrefix := "/" + programName + "/" + strings.TrimPrefix(filePath, "/")
		found, content = findFileInProgram(prog, pathWithPrefix)
		if found {
			return found, content
		}
	}
	return findFileInProgram(prog, filePath)
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

// applyDiffFileHashMap updates owner/deleted maps from a program's FileHashMap.
// owner maps path -> diff index (last write wins). hash==-1 marks deleted.
func applyDiffFileHashMap(diffProg *Program, di int, owner map[string]int, deleted map[string]struct{}) error {
	if diffProg == nil || diffProg.Program == nil {
		return utils.Errorf("diff program is nil")
	}
	fileHashMap := diffProg.Program.FileHashMap
	if len(fileHashMap) == 0 {
		return utils.Errorf("FileHashMap is required for diff program %s, but it is empty", diffProg.GetProgramName())
	}
	for filePath, hash := range fileHashMap {
		if filePath == "" {
			continue
		}
		normalizedPath := normalizeOverlayFilePath(filePath, diffProg.GetProgramName())
		if hash == -1 {
			delete(owner, normalizedPath)
			deleted[normalizedPath] = struct{}{}
			continue
		}
		owner[normalizedPath] = di
		delete(deleted, normalizedPath)
	}
	return nil
}

func materializeDiffFiles(diff []*ProgramLayer, owner map[string]int) {
	for _, layer := range diff {
		if layer != nil {
			layer.File = nil
		}
	}
	for path, di := range owner {
		if di < 0 || di >= len(diff) || diff[di] == nil {
			continue
		}
		diff[di].File = append(diff[di].File, path)
	}
}

func createOverlayFromLayers(programs ...*Program) *ProgramOverLay {
	if len(programs) < 2 {
		log.Errorf("createOverlayFromLayers requires at least 2 programs, got %d", len(programs))
		return nil
	}

	overlay := newEmptyOverlay()
	overlay.Base = programs[0]
	overlay.Diff = make([]*ProgramLayer, 0, len(programs)-1)

	owner := make(map[string]int)
	deleted := make(map[string]struct{})

	for i := 1; i < len(programs); i++ {
		diffProgram := programs[i]
		if diffProgram == nil {
			continue
		}
		di := len(overlay.Diff)
		if err := applyDiffFileHashMap(diffProgram, di, owner, deleted); err != nil {
			log.Errorf("createOverlayFromLayers: %v", err)
			return nil
		}
		overlay.Diff = append(overlay.Diff, &ProgramLayer{Program: diffProgram})
	}

	materializeDiffFiles(overlay.Diff, owner)
	overlay.setOwnerByPath(owner)
	overlay.setDeletedFiles(deleted)

	aggregatedFS, err := overlay.aggregateFileSystems()
	if err != nil {
		log.Errorf("failed to aggregate file systems: %v", err)
	} else {
		overlay.AggregatedFS = aggregatedFS
	}

	wireOverlayPrograms(overlay)

	log.Infof("ProgramOverLay: Built base+%d diffs, exclude=%d files",
		len(overlay.Diff), overlay.excludeCount())

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
	if newLayerProgram == nil || newLayerProgram.Program == nil {
		return nil
	}
	fileHashMap := newLayerProgram.Program.FileHashMap
	if len(fileHashMap) == 0 {
		log.Errorf("FileHashMap is required for diff program %s, but it is empty", newLayerProgram.GetProgramName())
		return nil
	}

	overlay := newEmptyOverlay()
	overlay.Base = baseOverlay.Base
	overlay.Diff = make([]*ProgramLayer, 0, len(baseOverlay.Diff)+1)
	for _, layer := range baseOverlay.Diff {
		if layer == nil {
			continue
		}
		// Copy File slice so ownership mutations don't mutate the previous overlay.
		copied := append([]string(nil), layer.File...)
		overlay.Diff = append(overlay.Diff, &ProgramLayer{Program: layer.Program, File: copied})
	}

	owner := make(map[string]int)
	deleted := make(map[string]struct{})
	for di, layer := range overlay.Diff {
		for _, path := range layer.File {
			owner[ensureOverlayPathSlash(path)] = di
		}
	}
	if baseOverlay.deletedFiles != nil {
		baseOverlay.deletedFiles.ForEach(func(path string, _ struct{}) bool {
			deleted[ensureOverlayPathSlash(path)] = struct{}{}
			return true
		})
	}

	di := len(overlay.Diff)
	if err := applyDiffFileHashMap(newLayerProgram, di, owner, deleted); err != nil {
		log.Errorf("extendOverlayWithNewLayer: %v", err)
		return nil
	}
	overlay.Diff = append(overlay.Diff, &ProgramLayer{Program: newLayerProgram})
	materializeDiffFiles(overlay.Diff, owner)
	overlay.setOwnerByPath(owner)
	overlay.setDeletedFiles(deleted)

	if baseOverlay.AggregatedFS != nil {
		newAggregatedFS := filesys.NewVirtualFs()
		err := filesys.Recursive(".", filesys.WithFileSystem(baseOverlay.AggregatedFS), filesys.WithFileStat(func(path string, info os.FileInfo) error {
			if info.IsDir() {
				return nil
			}
			content, err := baseOverlay.AggregatedFS.ReadFile(path)
			if err != nil {
				log.Warnf("failed to read file %s from baseOverlay AggregatedFS: %v", path, err)
				return nil
			}
			addFileToAggregatedFS(newAggregatedFS, overlayPathFromAggregatedFS(path), string(content))
			return nil
		}))
		if err != nil {
			log.Warnf("failed to copy files from baseOverlay AggregatedFS: %v", err)
		}
		for filePath, hash := range fileHashMap {
			normalizedPath := normalizeOverlayFilePath(filePath, newLayerProgram.GetProgramName())
			if hash == -1 {
				deleteFileFromAggregatedFS(newAggregatedFS, normalizedPath)
			}
		}
		newLayerProgram.ForEachAllFile(func(filePath string, me *memedit.MemEditor) bool {
			if filePath == "" || me == nil {
				return true
			}
			if hash, exists := fileHashMap[filePath]; exists && hash == -1 {
				return true
			}
			normalizedPath := normalizeOverlayFilePath(filePath, newLayerProgram.GetProgramName())
			addFileToAggregatedFS(newAggregatedFS, normalizedPath, me.GetSourceCode())
			return true
		})
		overlay.AggregatedFS = newAggregatedFS
	}

	wireOverlayPrograms(overlay)

	log.Infof("ProgramOverLay: Extended base+%d diffs, exclude=%d files",
		len(overlay.Diff), overlay.excludeCount())

	return overlay
}

func NewProgramOverLay(layers ...*Program) *ProgramOverLay {
	validLayers := make([]*Program, 0, len(layers))
	for _, layer := range layers {
		if layer != nil {
			validLayers = append(validLayers, layer)
		}
	}

	if len(validLayers) == 0 {
		return newEmptyOverlay()
	}

	if len(validLayers) == 1 {
		log.Errorf("NewProgramOverLay requires at least 2 layers, got 1")
		return nil
	}

	overlay := createOverlayFromLayers(validLayers...)

	return overlay
}

// aggregateFileSystems builds the effective FS from Diff (top-down) then Base,
// skipping deleted paths.
func (p *ProgramOverLay) aggregateFileSystems() (fi.FileSystem, error) {
	if p == nil || p.Base == nil {
		return nil, utils.Errorf("aggregateFileSystems requires Base program")
	}
	if len(p.Diff) == 0 {
		return nil, utils.Errorf("aggregateFileSystems requires at least one Diff layer")
	}

	aggregated := filesys.NewVirtualFs()
	allFiles := p.getAggregatedFilesSet()

	allFiles.ForEach(func(filePath string, _ struct{}) bool {
		for i := len(p.Diff) - 1; i >= 0; i-- {
			layer := p.Diff[i]
			if layer == nil || layer.Program == nil {
				continue
			}
			foundInLayer, content := findFileInProgramWithPrefix(layer.Program, filePath, layer.Program.GetProgramName())
			if foundInLayer {
				addFileToAggregatedFS(aggregated, filePath, content)
				return true
			}
		}
		if p.Base != nil {
			found, content := findFileInProgramWithPrefix(p.Base, filePath, p.Base.GetProgramName())
			if found {
				addFileToAggregatedFS(aggregated, filePath, content)
			}
		}
		return true
	})

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

// getAggregatedFilesSet returns visible paths: ∪Diff.File ∪ (base files − deleted).
func (p *ProgramOverLay) getAggregatedFilesSet() *utils.SafeMap[struct{}] {
	fileSet := utils.NewSafeMap[struct{}]()
	if p == nil {
		return fileSet
	}
	for _, layer := range p.Diff {
		if layer == nil {
			continue
		}
		for _, path := range layer.File {
			fileSet.Set(ensureOverlayPathSlash(path), struct{}{})
		}
	}
	if p.Base != nil && p.Base.Program != nil {
		for filePath := range p.Base.Program.FileList {
			if filePath == "" {
				continue
			}
			normalized := normalizeOverlayFilePath(filePath, p.Base.GetProgramName())
			if p.deletedFiles != nil && p.deletedFiles.Have(normalized) {
				continue
			}
			if _, owned := p.ownerDiffIndex(normalized); owned {
				continue
			}
			fileSet.Set(normalized, struct{}{})
		}
	}
	return fileSet
}

func (p *ProgramOverLay) GetFileCount() int {
	if p == nil {
		return 0
	}
	return p.getAggregatedFilesSet().Count()
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

// calculateFileSystemDiff 计算两个文件系统的差异，返回差量文件系统和 hash 映射
// hash状态: -1=删除, 0=修改, 1=新增
func calculateFileSystemDiff(baseFS, newFS fi.FileSystem) (diffFS *filesys.VirtualFS, fileHashMap map[string]int, err error) {
	diffFS = filesys.NewVirtualFs()
	fileHashMap = make(map[string]int)

	baseFiles := make(map[string][]byte)
	err = filesys.Recursive(".", filesys.WithFileSystem(baseFS), filesys.WithStat(func(isDir bool, pathname string, info os.FileInfo) error {
		if isDir {
			return nil
		}
		if pathname == "" {
			return nil
		}
		file, err := baseFS.Open(pathname)
		if err != nil {
			return nil
		}
		defer file.Close()
		content, err := io.ReadAll(file)
		if err != nil {
			return nil
		}
		baseFiles[pathname] = content
		return nil
	}))
	if err != nil {
		return nil, nil, utils.Wrap(err, "failed to collect baseFS files")
	}

	newFiles := make(map[string][]byte)
	err = filesys.Recursive(".", filesys.WithFileSystem(newFS), filesys.WithStat(func(isDir bool, pathname string, info os.FileInfo) error {
		if isDir {
			return nil
		}
		if pathname == "" {
			return nil
		}
		file, err := newFS.Open(pathname)
		if err != nil {
			return nil
		}
		defer file.Close()
		content, err := io.ReadAll(file)
		if err != nil {
			return nil
		}
		newFiles[pathname] = content
		return nil
	}))
	if err != nil {
		return nil, nil, utils.Wrap(err, "failed to collect newFS files")
	}

	deletedFiles := make([]string, 0)
	modifiedFiles := make([]string, 0)
	addedFiles := make([]string, 0)
	unchangedFiles := make([]string, 0)

	for filePath, baseContent := range baseFiles {
		newContent, existsInNew := newFiles[filePath]
		if !existsInNew {
			fileHashMap[filePath] = -1
			deletedFiles = append(deletedFiles, filePath)
		} else if !equalContent(baseContent, newContent) {
			fileHashMap[filePath] = 0
			diffFS.AddFile(filePath, string(newContent))
			modifiedFiles = append(modifiedFiles, filePath)
		} else {
			unchangedFiles = append(unchangedFiles, filePath)
		}
	}

	for filePath, newContent := range newFiles {
		if _, existsInBase := baseFiles[filePath]; !existsInBase {
			fileHashMap[filePath] = 1
			diffFS.AddFile(filePath, string(newContent))
			addedFiles = append(addedFiles, filePath)
		}
	}

	return diffFS, fileHashMap, nil
}

// equalContent 比较两个字节切片是否相等
func equalContent(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func (p *ProgramOverLay) getValueFilePath(v *Value) string {
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

	// Prefer path without program-name prefix so it matches ownerByPath /
	// Diff.File canonical form (normalizeOverlayFilePath).
	filePath := editor.GetFilePath()
	if filePath == "" {
		filePath = editor.GetUrl()
	}

	return filePath
}

func (p *ProgramOverLay) isFileDeleted(filePath string) bool {
	if p == nil || filePath == "" || p.deletedFiles == nil {
		return false
	}
	return p.deletedFiles.Have(ensureOverlayPathSlash(filePath))
}

// visibleDiff reports whether v belongs to Diff[di] (ownership + not deleted).
func (p *ProgramOverLay) visibleDiff(v *Value, di int) bool {
	if v == nil || p == nil || di < 0 || di >= len(p.Diff) || p.Diff[di] == nil || p.Diff[di].Program == nil {
		return false
	}
	filePath := p.getValueFilePath(v)
	if filePath == "" {
		return true
	}
	normalizedPath := normalizeOverlayFilePath(filePath, p.Diff[di].Program.GetProgramName())
	if p.isFileDeleted(normalizedPath) {
		return false
	}
	ownerDi, ok := p.ownerDiffIndex(normalizedPath)
	return ok && ownerDi == di
}

// visibleBase reports whether v is owned by Base (not overridden/deleted).
func (p *ProgramOverLay) visibleBase(v *Value) bool {
	if v == nil || p == nil || p.Base == nil {
		return false
	}
	filePath := p.getValueFilePath(v)
	if filePath == "" {
		return true
	}
	normalizedPath := normalizeOverlayFilePath(filePath, p.Base.GetProgramName())
	return !p.IsExcludedPath(normalizedPath)
}

func mergeOverlayLayerFoundFiles(layerFound, globalFound *utils.SafeMap[struct{}]) {
	if layerFound == nil || globalFound == nil {
		return
	}
	layerFound.ForEach(func(path string, _ struct{}) bool {
		globalFound.Set(path, struct{}{})
		return true
	})
}

func (p *ProgramOverLay) collectVisibleDiffOps(
	di int,
	vals sfvm.Values,
	foundFiles *utils.SafeMap[struct{}],
	results *[]sfvm.ValueOperator,
) {
	if vals == nil || results == nil {
		return
	}
	layerFoundFiles := utils.NewSafeMap[struct{}]()
	_ = vals.Recursive(func(op sfvm.ValueOperator) error {
		v, ok := op.(*Value)
		if !ok {
			*results = append(*results, op)
			return nil
		}
		if !p.visibleDiff(v, di) {
			return nil
		}
		filePath := p.getValueFilePath(v)
		if filePath == "" {
			*results = append(*results, v)
			return nil
		}
		normalizedPath := normalizeOverlayFilePath(filePath, p.Diff[di].Program.GetProgramName())
		if foundFiles.Have(normalizedPath) {
			return nil
		}
		layerFoundFiles.Set(normalizedPath, struct{}{})
		*results = append(*results, v)
		return nil
	})
	mergeOverlayLayerFoundFiles(layerFoundFiles, foundFiles)
}

func (p *ProgramOverLay) collectVisibleBaseOps(
	vals sfvm.Values,
	foundFiles *utils.SafeMap[struct{}],
	results *[]sfvm.ValueOperator,
) {
	if vals == nil || results == nil {
		return
	}
	layerFoundFiles := utils.NewSafeMap[struct{}]()
	_ = vals.Recursive(func(op sfvm.ValueOperator) error {
		v, ok := op.(*Value)
		if !ok {
			*results = append(*results, op)
			return nil
		}
		if !p.visibleBase(v) {
			return nil
		}
		filePath := p.getValueFilePath(v)
		if filePath == "" {
			*results = append(*results, v)
			return nil
		}
		normalizedPath := normalizeOverlayFilePath(filePath, p.Base.GetProgramName())
		if foundFiles.Have(normalizedPath) {
			return nil
		}
		layerFoundFiles.Set(normalizedPath, struct{}{})
		*results = append(*results, v)
		return nil
	})
	mergeOverlayLayerFoundFiles(layerFoundFiles, foundFiles)
}

// Ref dual-source: Diff include(owned) then Base exclude(owned∪deleted).
func (p *ProgramOverLay) Ref(name string) Values {
	var result Values
	if p == nil || p.Base == nil {
		return result
	}

	foundFiles := utils.NewSafeMap[struct{}]()
	for i := len(p.Diff) - 1; i >= 0; i-- {
		layer := p.Diff[i]
		if layer == nil {
			continue
		}
		layerFoundFiles := utils.NewSafeMap[struct{}]()
		for _, v := range layer.Ref(name) {
			if !p.visibleDiff(v, i) {
				continue
			}
			filePath := p.getValueFilePath(v)
			if filePath == "" {
				result = append(result, v)
				continue
			}
			normalizedPath := normalizeOverlayFilePath(filePath, layer.Program.GetProgramName())
			if foundFiles.Have(normalizedPath) {
				continue
			}
			layerFoundFiles.Set(normalizedPath, struct{}{})
			result = append(result, v)
		}
		mergeOverlayLayerFoundFiles(layerFoundFiles, foundFiles)
	}

	exclude := p.excludeFiles()
	var layerValues Values
	if len(exclude) > 0 {
		layerValues = p.Base.refWithExcludeFiles(name, exclude)
	} else {
		layerValues = p.Base.Ref(name)
	}
	layerFoundFiles := utils.NewSafeMap[struct{}]()
	for _, v := range layerValues {
		if !p.visibleBase(v) {
			continue
		}
		filePath := p.getValueFilePath(v)
		if filePath == "" {
			result = append(result, v)
			continue
		}
		normalizedPath := normalizeOverlayFilePath(filePath, p.Base.GetProgramName())
		if foundFiles.Have(normalizedPath) {
			continue
		}
		layerFoundFiles.Set(normalizedPath, struct{}{})
		result = append(result, v)
	}
	mergeOverlayLayerFoundFiles(layerFoundFiles, foundFiles)
	return result
}

func (p *ProgramOverLay) generateRelocateRule(v *Value) string {
	if v == nil {
		return ""
	}
	op := v.GetOpcode()

	filter := func(name string) bool {
		if name == "" {
			return true
		}
		banList := `.*(=|-).*`
		if match, err := regexp.Match(banList, []byte(name)); err == nil && match {
			return true
		}
		return false
	}

	rule := ""
	for _, name := range getValueNames(v) {
		if filter(name) {
			continue
		}
		rule += fmt.Sprintf("%s?{opcode: %s} as $res_op\n", name, op)
	}

	log.Debugf("syntaxflow rule: \n%s", rule)
	return rule
}

// relocateNameCandidates returns filtered names suitable for direct SSA lookup.
func relocateNameCandidates(v *Value) []string {
	if v == nil {
		return nil
	}
	seen := make(map[string]struct{})
	var out []string
	for _, name := range getValueNames(v) {
		if name == "" {
			continue
		}
		if match, err := regexp.Match(`.*(=|-).*`, []byte(name)); err == nil && match {
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

func (p *ProgramOverLay) findValueInLayer(layer *ProgramLayer, v *Value) *Value {
	if layer == nil || layer.Program == nil || v == nil {
		return nil
	}

	cacheKey := fmt.Sprintf("%d:%s", v.GetId(), layer.Program.GetProgramName())
	if cached, ok := p.signatureCache.Get(cacheKey); ok {
		return cached
	}

	wantOpcode := v.GetOpcode()
	names := relocateNameCandidates(v)

	// Fast path: direct SSA variable lookup (no nested SyntaxFlow VM).
	for _, name := range names {
		candidates := layer.Program.Ref(name)
		for _, cand := range candidates {
			if cand == nil {
				continue
			}
			if wantOpcode != "" && cand.GetOpcode() != wantOpcode {
				continue
			}
			p.signatureCache.Set(cacheKey, cand)
			return cand
		}
	}

	// Fallback: preserve previous correctness for consts / oddly-named values
	// via a targeted SyntaxFlow rule (still cached; debug disabled for hot path).
	rule := p.generateRelocateRule(v)
	if rule != "" {
		res, err := layer.Program.SyntaxFlowWithError(rule)
		if err == nil {
			values := res.GetAllValuesChain()
			if len(values) > 0 {
				p.signatureCache.Set(cacheKey, values[0])
				return values[0]
			}
		}
	}

	return nil
}

func (p *ProgramOverLay) Relocate(v *Value) *Value {
	if v == nil || p == nil {
		return v
	}

	filePath := p.getValueFilePath(v)
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
	// Relocate when value comes from Base or an older Diff than the owner.
	if fromBase || di < ownerDi {
		if relocated := p.findValueInLayer(p.Diff[ownerDi], v); relocated != nil {
			return relocated
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
	return fmt.Sprintf("ProgramOverLay(base+diffs=%d, exclude=%d)", p.ProgramCount(), p.excludeCount())
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
	foundFiles := utils.NewSafeMap[struct{}]()

	for i := len(p.Diff) - 1; i >= 0; i-- {
		layer := p.Diff[i]
		if layer == nil || layer.Program == nil || len(layer.File) == 0 {
			continue
		}
		matched, vals, err := layer.Program.matchVariableWithIncludeFiles(ctx, compareMode, mod, query, layer.File)
		if err != nil || !matched {
			continue
		}
		p.collectVisibleDiffOps(i, vals, foundFiles, &results)
	}

	matched, vals, err := p.Base.matchVariableWithExcludeFiles(ctx, compareMode, mod, query, p.excludeFiles())
	if err == nil && matched {
		p.collectVisibleBaseOps(vals, foundFiles, &results)
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

func (p *ProgramOverLay) compareAcrossLayers(compare func(*Program) (sfvm.Values, []bool)) sfvm.Values {
	if p == nil || p.Base == nil {
		return sfvm.NewEmptyValues()
	}

	results := make([]sfvm.ValueOperator, 0)
	foundFiles := utils.NewSafeMap[struct{}]()

	for i := len(p.Diff) - 1; i >= 0; i-- {
		layer := p.Diff[i]
		if layer == nil || layer.Program == nil {
			continue
		}
		values, _ := compare(layer.Program)
		if values.IsEmpty() {
			continue
		}
		p.collectVisibleDiffOps(i, values, foundFiles, &results)
	}

	values, _ := compare(p.Base)
	if !values.IsEmpty() {
		p.collectVisibleBaseOps(values, foundFiles, &results)
	}
	return sfvm.NewValues(results)
}

func (p *ProgramOverLay) CompareString(comparator *sfvm.StringComparator) (sfvm.Values, []bool) {
	return p.compareAcrossLayers(func(prog *Program) (sfvm.Values, []bool) {
		return prog.CompareString(comparator)
	}), nil
}

func (p *ProgramOverLay) CompareOpcode(comparator *sfvm.OpcodeComparator) (sfvm.Values, []bool) {
	return p.compareAcrossLayers(func(prog *Program) (sfvm.Values, []bool) {
		return prog.CompareOpcode(comparator)
	}), nil
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
