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

// ProgramLayer 表示一个编译层
type ProgramLayer struct {
	LayerIndex int                      // 层索引，从1开始（1=最底层）
	Program    *Program                 // 该层的 Program
	FileSet    *utils.SafeMap[struct{}] // 该层包含的文件路径集合
	// FileHashMap: 文件路径 -> hash状态（相对于前一层）
	// -1: 删除（在前一层存在，但在本层不存在）
	// 0: 修改（在前一层和本层都存在，但内容不同）
	// 1: 新增（只在本层存在，或对于 Layer1，所有文件都是新增）
	FileHashMap *utils.SafeMap[int] // key: 文件路径, value: hash状态
}

// ProgramOverLay 实现多层 Layer 增量编译的虚拟视图
// 核心思想：
//   - 多层 Layer 概念（Layer1, Layer2, Layer3...），没有 Base/Diff 的区别
//   - 文件系统聚合：所有 Layer 文件系统的 differ 聚合
//   - 查找策略：上层覆盖下层，从最上层开始查找
//   - 删除语义：如果文件在底层存在但在上层不存在，该文件应该被删除
//
// 虚拟视图逻辑: 上层 Layer 中的文件会覆盖下层 Layer 中同名文件的 IR 节点
type ProgramOverLay struct {
	// Layers: 按顺序存储所有层（从底层到上层）
	// Layers[0] = Layer1 (最底层)
	// Layers[1] = Layer2 (中间层)
	// Layers[2] = Layer3 (最上层)
	Layers []*ProgramLayer

	// FileToLayerMap: 文件路径 -> 最上层包含该文件的 Layer 索引
	// 用于快速判断文件在哪个 Layer 中，以及上层是否覆盖下层
	FileToLayerMap *utils.SafeMap[int] // key: 文件路径, value: Layer 索引

	// 聚合后的文件系统（所有 Layer 文件系统的 differ 聚合）
	AggregatedFS fi.FileSystem

	// signatureCache 缓存 Value 的签名，用于重定位
	signatureCache *utils.CacheWithKey[string, *Value]

	// overlay 的元数据（用于实现 Program interface）
	programName string
	programKind ssadb.ProgramKind
	language    ssaconfig.Language
}

// GetLayerProgramNames 获取所有 layer 的 program names（按顺序，从底层到上层）
func (o *ProgramOverLay) GetLayerProgramNames() []string {
	if o == nil || len(o.Layers) == 0 {
		return nil
	}
	names := make([]string, 0, len(o.Layers))
	for _, layer := range o.Layers {
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
	if p.programName != "" {
		return p.programName
	}
	if len(p.Layers) > 0 {
		topLayer := p.Layers[len(p.Layers)-1]
		if topLayer != nil && topLayer.Program != nil {
			return topLayer.Program.GetProgramName()
		}
	}
	return ""
}

func (p *ProgramOverLay) GetProgramKind() ssadb.ProgramKind {
	if p == nil {
		return ""
	}
	if p.programKind != "" {
		return p.programKind
	}
	if len(p.Layers) > 0 {
		topLayer := p.Layers[len(p.Layers)-1]
		if topLayer != nil && topLayer.Program != nil {
			return topLayer.Program.GetProgramKind()
		}
	}
	return ssadb.Application
}

func (p *ProgramOverLay) GetLanguage() ssaconfig.Language {
	if p == nil {
		return ""
	}
	if p.language != "" {
		return p.language
	}
	if len(p.Layers) > 0 {
		topLayer := p.Layers[len(p.Layers)-1]
		if topLayer != nil && topLayer.Program != nil {
			return topLayer.Program.GetLanguage()
		}
	}
	return ""
}

func (p *ProgramOverLay) Hash() (string, bool) {
	if p == nil {
		return "", false
	}
	layerNames := p.GetLayerProgramNames()
	if len(layerNames) == 0 {
		return "", false
	}
	args := make([]interface{}, len(layerNames))
	for i, name := range layerNames {
		args[i] = name
	}
	hash := utils.CalcSha256(args...)
	return hash, true
}

func (p *ProgramOverLay) GetOverlay() *ProgramOverLay {
	return p
}

// newEmptyOverlay 创建一个空的 ProgramOverLay
func newEmptyOverlay() *ProgramOverLay {
	return &ProgramOverLay{
		Layers:         make([]*ProgramLayer, 0),
		FileToLayerMap: utils.NewSafeMap[int](),
		signatureCache: utils.NewTTLCacheWithKey[string, *Value](0),
	}
}

func createLayer1FromProgram(prog *Program, layerIndex int) *ProgramLayer {
	layer := &ProgramLayer{
		LayerIndex:  layerIndex,
		Program:     prog,
		FileSet:     utils.NewSafeMap[struct{}](),
		FileHashMap: utils.NewSafeMap[int](),
	}

	prog.ForEachAllFile(func(filePath string, me *memedit.MemEditor) bool {
		if filePath == "" {
			return true
		}
		normalizedPath := removeProgramNamePrefix(filePath, prog.GetProgramName())
		layer.FileSet.Set(normalizedPath, struct{}{})
		layer.FileHashMap.Set(normalizedPath, 1)
		return true
	})

	return layer
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

func createOverlayFromLayers(layers ...*Program) *ProgramOverLay {
	if len(layers) < 2 {
		log.Errorf("createOverlayFromLayers requires at least 2 layers, got %d", len(layers))
		return nil
	}

	overlay := newEmptyOverlay()
	overlay.Layers = make([]*ProgramLayer, 0, len(layers))

	layer1 := createLayer1FromProgram(layers[0], 1)
	overlay.Layers = append(overlay.Layers, layer1)

	for i := 1; i < len(layers); i++ {
		diffProgram := layers[i]
		if diffProgram == nil {
			continue
		}

		layerIndex := i + 1
		layer := &ProgramLayer{
			LayerIndex:  layerIndex,
			Program:     diffProgram,
			FileSet:     utils.NewSafeMap[struct{}](),
			FileHashMap: utils.NewSafeMap[int](),
		}

		var fileHashMap map[string]int
		if diffProgram.Program == nil {
			log.Errorf("diffProgram.Program is nil for layer %d", layerIndex)
			return nil
		}

		if len(diffProgram.Program.FileHashMap) == 0 {
			log.Errorf("FileHashMap is required for diff program in layer %d, but it is empty", layerIndex)
			return nil
		}

		fileHashMap = diffProgram.Program.FileHashMap

		for filePath, hash := range fileHashMap {
			if filePath != "" {
				layer.FileHashMap.Set(removeProgramNamePrefix(filePath, diffProgram.GetProgramName()), hash)
			}
		}

		diffProgram.ForEachAllFile(func(filePath string, me *memedit.MemEditor) bool {
			if filePath == "" {
				return true
			}

			hash := 1
			if fileHashMap != nil {
				if h, exists := fileHashMap[filePath]; exists {
					hash = h
				}
			}

			if hash != -1 {
				layer.FileSet.Set(removeProgramNamePrefix(filePath, diffProgram.GetProgramName()), struct{}{})
				overlay.FileToLayerMap.Set(removeProgramNamePrefix(filePath, diffProgram.GetProgramName()), layerIndex)
			}
			return true
		})

		overlay.Layers = append(overlay.Layers, layer)
	}

	layer1ProgramName := layers[0].GetProgramName()
	layers[0].ForEachAllFile(func(filePath string, me *memedit.MemEditor) bool {
		if filePath == "" {
			return true
		}
		normalizedPath := removeProgramNamePrefix(filePath, layer1ProgramName)
		if !overlay.FileToLayerMap.Have(normalizedPath) {
			isDeleted := false
			for i := 1; i < len(overlay.Layers); i++ {
				if hash, exists := overlay.Layers[i].FileHashMap.Get(normalizedPath); exists && hash == -1 {
					isDeleted = true
					break
				}
			}
			if !isDeleted {
				overlay.FileToLayerMap.Set(normalizedPath, 1)
			}
		}
		return true
	})

	overlay.programName = layers[len(layers)-1].GetProgramName()
	aggregatedFS, err := overlay.aggregateFileSystems()
	if err != nil {
		log.Errorf("failed to aggregate file systems: %v", err)
	} else {
		overlay.AggregatedFS = aggregatedFS
	}

	for _, layer := range overlay.Layers {
		if layer != nil && layer.Program != nil {
			// 设置 overlay，这样 Value.ParentProgram.GetOverlay() 可以返回 overlay
			if layer.Program.overlay == nil {
				layer.Program.overlay = overlay
			}
			overlay.ensureLayerProgramLoaded(layer)
		}
	}

	log.Infof("ProgramOverLay: Built with %d layers (from pre-compiled programs), %d unique files",
		len(overlay.Layers), overlay.FileToLayerMap.Count())

	return overlay
}

// extendOverlayWithNewLayer 扩展一个已存在的 overlay，添加新的 layer
// 复用 baseOverlay 的所有 layer，避免重新创建 layer1（防止更新 layer1 的 updated_at）
func extendOverlayWithNewLayer(baseOverlay *ProgramOverLay, newLayerProgram *Program) *ProgramOverLay {
	if baseOverlay == nil || len(baseOverlay.Layers) == 0 {
		return nil
	}
	if newLayerProgram == nil {
		return nil
	}

	// 创建新的 overlay，复用 baseOverlay 的所有 layer
	overlay := newEmptyOverlay()
	overlay.Layers = make([]*ProgramLayer, 0, len(baseOverlay.Layers)+1)

	// 复用 baseOverlay 的所有 layer（包括 layer1），避免重新创建
	for _, layer := range baseOverlay.Layers {
		if layer != nil {
			overlay.Layers = append(overlay.Layers, layer)
		}
	}

	// 创建新的 layer（layer3）的 ProgramLayer
	layerIndex := len(overlay.Layers) + 1
	newLayer := &ProgramLayer{
		LayerIndex:  layerIndex,
		Program:     newLayerProgram,
		FileSet:     utils.NewSafeMap[struct{}](),
		FileHashMap: utils.NewSafeMap[int](),
	}

	var fileHashMap map[string]int
	if newLayerProgram.Program == nil {
		log.Errorf("newLayerProgram.Program is nil for layer %d", layerIndex)
		return nil
	}

	if len(newLayerProgram.Program.FileHashMap) == 0 {
		log.Errorf("FileHashMap is required for diff program in layer %d, but it is empty", layerIndex)
		return nil
	}

	fileHashMap = newLayerProgram.Program.FileHashMap

	for filePath, hash := range fileHashMap {
		if filePath != "" {
			newLayer.FileHashMap.Set(removeProgramNamePrefix(filePath, newLayerProgram.GetProgramName()), hash)
		}
	}

	newLayerProgram.ForEachAllFile(func(filePath string, me *memedit.MemEditor) bool {
		if filePath == "" {
			return true
		}

		hash := 1
		if fileHashMap != nil {
			if h, exists := fileHashMap[filePath]; exists {
				hash = h
			}
		}

		if hash != -1 {
			normalizedPath := removeProgramNamePrefix(filePath, newLayerProgram.GetProgramName())
			newLayer.FileSet.Set(normalizedPath, struct{}{})
			overlay.FileToLayerMap.Set(normalizedPath, layerIndex)
		}
		return true
	})

	overlay.Layers = append(overlay.Layers, newLayer)

	// 复用 baseOverlay 的 FileToLayerMap，然后添加新的 layer 的文件映射
	baseOverlay.FileToLayerMap.ForEach(func(normalizedPath string, layerIdx int) bool {
		overlay.FileToLayerMap.Set(normalizedPath, layerIdx)
		return true
	})

	// 复用 baseOverlay 的 AggregatedFS，避免重新调用 aggregateFileSystems()
	// 这样可以避免访问 layer1 的 program，防止更新 layer1 的 updated_at
	// 创建一个新的 VirtualFS，从 baseOverlay 的 AggregatedFS 复制所有文件，然后添加 layer3 的新文件
	if baseOverlay.AggregatedFS != nil {
		// 从 baseOverlay 的 AggregatedFS 复制所有文件到新的 VirtualFS
		newAggregatedFS := filesys.NewVirtualFs()
		err := filesys.Recursive(".", filesys.WithFileSystem(baseOverlay.AggregatedFS), filesys.WithFileStat(func(path string, info os.FileInfo) error {
			if info.IsDir() {
				return nil
			}
			// 复制文件内容
			content, err := baseOverlay.AggregatedFS.ReadFile(path)
			if err != nil {
				log.Warnf("failed to read file %s from baseOverlay AggregatedFS: %v", path, err)
				return nil
			}
			newAggregatedFS.AddFile(path, string(content))
			return nil
		}))
		if err != nil {
			log.Warnf("failed to copy files from baseOverlay AggregatedFS: %v", err)
		}

		// 删除 layer3 中被删除的文件（hash == -1）
		// 这些文件在 baseOverlay 的 AggregatedFS 中存在，但在 layer3 中被删除了
		for filePath, hash := range fileHashMap {
			if hash == -1 {
				normalizedPath := removeProgramNamePrefix(filePath, newLayerProgram.GetProgramName())
				// 删除文件（如果存在）
				if exists, _ := newAggregatedFS.Exists(normalizedPath); exists {
					newAggregatedFS.Delete(normalizedPath)
				}
			}
		}

		// 添加 layer3 的新文件或修改的文件
		// 只添加 layer3 中 hash != -1 的文件（新增或修改的文件）
		newLayerProgram.ForEachAllFile(func(filePath string, me *memedit.MemEditor) bool {
			if filePath == "" || me == nil {
				return true
			}
			normalizedPath := removeProgramNamePrefix(filePath, newLayerProgram.GetProgramName())
			// 检查这个文件是否在 layer3 中被删除
			if hash, exists := fileHashMap[filePath]; exists && hash == -1 {
				// 文件被删除，不需要添加到 AggregatedFS
				return true
			}
			// 添加或更新文件（layer3 的文件会覆盖 baseOverlay 的文件）
			newAggregatedFS.AddFile(normalizedPath, me.GetSourceCode())
			return true
		})

		overlay.AggregatedFS = newAggregatedFS
	}
	// 如果 baseOverlay 没有 AggregatedFS，延迟到 GetAggregatedFileSystem() 时再聚合

	// 确保所有 layer 的 program 都已加载
	for _, layer := range overlay.Layers {
		if layer != nil {
			overlay.ensureLayerProgramLoaded(layer)
		}
	}

	// 设置 overlay 的元数据
	if newLayerProgram != nil {
		overlay.programName = newLayerProgram.GetProgramName()
		overlay.programKind = newLayerProgram.GetProgramKind()
		overlay.language = newLayerProgram.GetLanguage()
	}

	log.Infof("ProgramOverLay: Extended with %d layers (reused base overlay), %d unique files",
		len(overlay.Layers), overlay.FileToLayerMap.Count())

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

	if overlay != nil && len(validLayers) > 0 {
		topLayer := validLayers[len(validLayers)-1]
		if topLayer != nil {
			overlay.programName = topLayer.GetProgramName()
			overlay.programKind = topLayer.GetProgramKind()
			overlay.language = topLayer.GetLanguage()
		}
	}

	return overlay
}

// aggregateFileSystems 聚合所有 Layer 的文件系统
func (p *ProgramOverLay) aggregateFileSystems() (fi.FileSystem, error) {
	aggregated := filesys.NewVirtualFs()
	aggregatedFilesSet := p.getAggregatedFilesSet()

	hasFileHashMap := false
	for _, layer := range p.Layers {
		if layer != nil && layer.FileHashMap != nil && layer.FileHashMap.Count() > 0 {
			hasFileHashMap = true
			break
		}
	}

	if !hasFileHashMap {
		return nil, utils.Errorf("FileHashMap is required for aggregateFileSystems, but no layer has FileHashMap")
	}

	allFilesSet := utils.NewSafeMap[struct{}]()

	aggregatedFilesSet.ForEach(func(normalizedPath string, _ struct{}) bool {
		allFilesSet.Set(normalizedPath, struct{}{})
		return true
	})

	// 使用 layer1 的 FileSet 和 FileHashMap，而不是访问 layer1 的 program
	// 这样可以避免访问 layer1 的 program，防止更新 layer1 的 updated_at
	if len(p.Layers) > 0 && p.Layers[0] != nil && p.Layers[0].FileSet != nil {
		p.Layers[0].FileSet.ForEach(func(normalizedPath string, _ struct{}) bool {
			// 检查这个文件是否在后续 layer 中被删除
			isDeleted := false
			for i := 1; i < len(p.Layers); i++ {
				if p.Layers[i] != nil && p.Layers[i].FileHashMap != nil {
					if hash, exists := p.Layers[i].FileHashMap.Get(normalizedPath); exists && hash == -1 {
						isDeleted = true
						break
					}
				}
			}
			if !isDeleted {
				allFilesSet.Set(normalizedPath, struct{}{})
			}
			return true
		})
	}

	allFilesSet.ForEach(func(filePath string, _ struct{}) bool {
		for i := len(p.Layers) - 1; i >= 0; i-- {
			layer := p.Layers[i]
			if layer == nil || layer.Program == nil {
				continue
			}

			layerProgramName := layer.Program.GetProgramName()
			foundInLayer, content := findFileInProgramWithPrefix(layer.Program, filePath, layerProgramName)

			if foundInLayer {
				aggregated.AddFile(filePath, content)
				break
			}
		}
		return true
	})

	return aggregated, nil
}

// GetLayerCount 获取 Layer 数量
func (p *ProgramOverLay) GetLayerCount() int {
	if p == nil {
		return 0
	}
	return len(p.Layers)
}

// getAggregatedFilesSet 获取聚合后的文件集合，hash 值求和为 1 的文件才会被包含
func (p *ProgramOverLay) getAggregatedFilesSet() *utils.SafeMap[struct{}] {
	fileSet := utils.NewSafeMap[struct{}]()

	if p == nil {
		return fileSet
	}

	fileHashSum := make(map[string]int)
	layerHashDetails := make(map[int]map[string]int)

	for layerIdx, layer := range p.Layers {
		if layer == nil || layer.FileHashMap == nil {
			continue
		}

		layerHashDetails[layerIdx+1] = make(map[string]int)
		layer.FileHashMap.ForEach(func(normalizedPath string, hash int) bool {
			fileHashSum[normalizedPath] += hash
			layerHashDetails[layerIdx+1][normalizedPath] = hash
			return true
		})
	}

	hashSumDetails := make(map[string]string)
	for filePath, sum := range fileHashSum {
		details := make([]string, 0)
		for layerIdx := 1; layerIdx <= len(p.Layers); layerIdx++ {
			if layerHashes, ok := layerHashDetails[layerIdx]; ok {
				if hash, exists := layerHashes[filePath]; exists {
					details = append(details, fmt.Sprintf("L%d:%d", layerIdx, hash))
				}
			}
		}
		hashSumDetails[filePath] = fmt.Sprintf("sum=%d [%s]", sum, strings.Join(details, ", "))
	}

	for filePath, sum := range fileHashSum {
		if sum == 1 {
			fileSet.Set(filePath, struct{}{})
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

// GetFilesInLayer 获取指定层中的所有文件路径
func (p *ProgramOverLay) GetFilesInLayer(layerIndex int) []string {
	if p == nil || layerIndex < 1 || layerIndex > len(p.Layers) {
		return nil
	}
	layer := p.Layers[layerIndex-1]
	if layer == nil {
		return nil
	}
	return layer.FileSet.Keys()
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

	filePath := editor.GetFilePath()
	if filePath == "" {
		filePath = editor.GetUrl()
	}

	return filePath
}

func (p *ProgramOverLay) isFileDeleted(filePath string, currentLayerIndex int) bool {
	if p == nil || filePath == "" {
		return false
	}

	normalizedPath := filePath
	if currentLayerIndex >= 0 && currentLayerIndex < len(p.Layers) {
		layer := p.Layers[currentLayerIndex]
		if layer != nil && layer.Program != nil {
			layerProgramName := layer.Program.GetProgramName()
			normalizedPath = removeProgramNamePrefix(filePath, layerProgramName)
			normalizedPath = strings.TrimPrefix(normalizedPath, "/")
		}
	}

	aggregatedFS := p.GetAggregatedFileSystem()
	if aggregatedFS != nil {
		exists, _ := aggregatedFS.Exists(normalizedPath)
		if !exists {
			return true
		}
		return false
	}

	for j := currentLayerIndex + 1; j < len(p.Layers); j++ {
		upperLayer := p.Layers[j]
		if upperLayer != nil && upperLayer.FileHashMap != nil {
			hash, exists := upperLayer.FileHashMap.Get(normalizedPath)
			if exists && hash == -1 {
				return true
			}
		}
	}

	return false
}

func (p *ProgramOverLay) getValueLayerIndex(v *Value) int {
	if v == nil || p == nil {
		return 0
	}

	programName := v.GetProgramName()
	if programName == "" {
		return 0
	}

	for i, layer := range p.Layers {
		if layer != nil && layer.Program != nil {
			if layer.Program.GetProgramName() == programName {
				return i + 1
			}
		}
	}

	return 0
}

func (p *ProgramOverLay) IsOverridden(v *Value) bool {
	if v == nil || p == nil {
		return false
	}

	filePath := p.getValueFilePath(v)
	if filePath == "" {
		return false
	}

	valueLayerIndex := p.getValueLayerIndex(v)
	if valueLayerIndex == 0 {
		return false
	}

	normalizedPath := filePath
	if valueLayerIndex > 0 && valueLayerIndex <= len(p.Layers) {
		layer := p.Layers[valueLayerIndex-1]
		if layer != nil && layer.Program != nil {
			layerProgramName := layer.Program.GetProgramName()
			normalizedPath = removeProgramNamePrefix(filePath, layerProgramName)
			normalizedPath = strings.TrimPrefix(normalizedPath, "/")
		}
	}

	topLayerIndex, exists := p.FileToLayerMap.Get(normalizedPath)
	if !exists {
		return false
	}

	return topLayerIndex > valueLayerIndex
}

// Ref 实现基于层的查找策略：从上层到下层，上层覆盖下层
func (p *ProgramOverLay) Ref(name string) Values {
	var result Values
	if p == nil {
		return result
	}

	foundFiles := utils.NewSafeMap[struct{}]()
	excludeFiles := make([]string, 0)

	for i := len(p.Layers) - 1; i >= 0; i-- {
		layer := p.Layers[i]
		if layer == nil || layer.Program == nil {
			continue
		}

		layerProgramName := layer.Program.GetProgramName()

		var layerValues Values
		if len(excludeFiles) > 0 {
			layerValues = layer.Program.refWithExcludeFiles(name, excludeFiles)
		} else {
			layerValues = layer.Program.Ref(name)
		}

		currentLayerFoundFiles := make([]string, 0)

		for _, v := range layerValues {
			filePath := p.getValueFilePath(v)
			if filePath == "" {
				result = append(result, v)
				continue
			}

			normalizedPath := removeProgramNamePrefix(filePath, layerProgramName)
			normalizedPath = strings.TrimPrefix(normalizedPath, "/")

			if p.isFileDeleted(normalizedPath, i) {
				continue
			}

			if foundFiles.Have(normalizedPath) {
				continue
			}

			if layer.FileSet.Have(normalizedPath) {
				foundFiles.Set(normalizedPath, struct{}{})
				currentLayerFoundFiles = append(currentLayerFoundFiles, normalizedPath)
				result = append(result, v)
			} else {
				actualLayerIndex, exists := p.FileToLayerMap.Get(normalizedPath)
				if exists && actualLayerIndex > layer.LayerIndex {
					continue
				}
				foundFiles.Set(normalizedPath, struct{}{})
				currentLayerFoundFiles = append(currentLayerFoundFiles, normalizedPath)
				result = append(result, v)
			}
		}

		excludeFiles = append(excludeFiles, currentLayerFoundFiles...)
	}

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

func (p *ProgramOverLay) findValueInLayer(layer *ProgramLayer, v *Value) *Value {
	if layer == nil || layer.Program == nil {
		return nil
	}

	rule := p.generateRelocateRule(v)
	if rule == "" {
		return nil
	}

	cacheKey := fmt.Sprintf("%s:%s", layer.Program.GetProgramName(), rule)
	if cached, ok := p.signatureCache.Get(cacheKey); ok {
		return cached
	}

	res, err := layer.Program.SyntaxFlowWithError(rule, QueryWithEnableDebug())
	if err != nil {
		log.Debugf("search value by Rule failed in Layer %d: %v", layer.LayerIndex, err)
		return nil
	}

	values := res.GetAllValuesChain()
	if len(values) > 0 {
		p.signatureCache.Set(cacheKey, values[0])
		return values[0]
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

	valueLayerIndex := p.getValueLayerIndex(v)
	if valueLayerIndex == 0 {
		return v
	}

	normalizedPath := filePath
	if valueLayerIndex > 0 && valueLayerIndex <= len(p.Layers) {
		layer := p.Layers[valueLayerIndex-1]
		if layer != nil && layer.Program != nil {
			layerProgramName := layer.Program.GetProgramName()
			normalizedPath = removeProgramNamePrefix(filePath, layerProgramName)
			normalizedPath = strings.TrimPrefix(normalizedPath, "/")
		}
	}

	fileLayerIndex, exists := p.FileToLayerMap.Get(normalizedPath)
	if !exists {
		return v
	}

	if valueLayerIndex < fileLayerIndex {
		upperLayer := p.Layers[fileLayerIndex-1]
		if upperLayer != nil {
			relocated := p.findValueInLayer(upperLayer, v)
			if relocated != nil {
				return relocated
			}
		}
	}

	return v
}

func (p *ProgramOverLay) ensureLayerProgramLoaded(layer *ProgramLayer) {
	if layer == nil || layer.Program == nil || layer.Program.Program == nil {
		return
	}
	func() {
		defer func() {
			if r := recover(); r != nil {
				log.Debugf("LazyBuild panic for layer %d: %v", layer.LayerIndex, r)
			}
		}()
		layer.Program.Program.LazyBuild()
	}()
}

func (p *ProgramOverLay) Show() *ProgramOverLay {
	if p == nil {
		return p
	}
	for i, layer := range p.Layers {
		if layer != nil && layer.Program != nil {
			fmt.Printf("=== Layer %d (Index: %d) ===\n", i+1, layer.LayerIndex)
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
	return fmt.Sprintf("ProgramOverLay(layers=%d, files=%d)", len(p.Layers), p.FileToLayerMap.Count())
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
	for _, layer := range p.Layers {
		if layer != nil && layer.Program != nil && !layer.Program.IsEmpty() {
			return false
		}
	}
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
	for i := len(p.Layers) - 1; i >= 0; i-- {
		layer := p.Layers[i]
		if layer != nil && layer.Program != nil {
			if err := f(layer.Program); err != nil {
				return err
			}
		}
	}
	return nil
}

// queryMatch 通用的查询匹配方法，应用上层优先策略
func (p *ProgramOverLay) queryMatch(
	ctx context.Context,
	mod int,
	queryFunc func(*Program, context.Context, int, string, []string) (bool, sfvm.ValueOperator, error),
	query string,
) (bool, sfvm.ValueOperator, error) {
	if p == nil {
		return false, nil, nil
	}

	var results Values
	foundFiles := utils.NewSafeMap[struct{}]()
	excludeFiles := make([]string, 0)

	for i := len(p.Layers) - 1; i >= 0; i-- {
		layer := p.Layers[i]
		if layer == nil || layer.Program == nil {
			continue
		}

		layerProgramName := layer.Program.GetProgramName()

		matched, vals, err := queryFunc(layer.Program, ctx, mod, query, excludeFiles)
		if err != nil {
			continue
		}

		currentLayerFoundFiles := make([]string, 0)

		if matched {
			vals.Recursive(func(op sfvm.ValueOperator) error {
				if v, ok := op.(*Value); ok {
					filePath := p.getValueFilePath(v)
					if filePath == "" {
						results = append(results, v)
						return nil
					}

					normalizedPath := removeProgramNamePrefix(filePath, layerProgramName)
					normalizedPath = strings.TrimPrefix(normalizedPath, "/")
					if p.isFileDeleted(normalizedPath, i) {
						return nil
					}

					if foundFiles.Have(normalizedPath) {
						return nil
					}

					if layer.FileSet.Have(normalizedPath) {
						foundFiles.Set(normalizedPath, struct{}{})
						currentLayerFoundFiles = append(currentLayerFoundFiles, normalizedPath)
						results = append(results, v)
					} else {
						actualLayerIndex, exists := p.FileToLayerMap.Get(normalizedPath)
						if exists && actualLayerIndex > layer.LayerIndex {
							return nil
						}
						foundFiles.Set(normalizedPath, struct{}{})
						currentLayerFoundFiles = append(currentLayerFoundFiles, normalizedPath)
						results = append(results, v)
					}
				}
				return nil
			})
		}

		excludeFiles = append(excludeFiles, currentLayerFoundFiles...)
	}

	return len(results) > 0, ValuesToSFValueList(results), nil
}

func (p *ProgramOverLay) ExactMatch(ctx context.Context, mod int, want string) (bool, sfvm.ValueOperator, error) {
	return p.queryMatch(ctx, mod, func(prog *Program, ctx context.Context, mod int, query string, excludeFiles []string) (bool, sfvm.ValueOperator, error) {
		return prog.matchVariableWithExcludeFiles(ctx, ssadb.ExactCompare, mod, query, excludeFiles)
	}, want)
}

func (p *ProgramOverLay) GlobMatch(ctx context.Context, mod int, g string) (bool, sfvm.ValueOperator, error) {
	return p.queryMatch(ctx, mod, func(prog *Program, ctx context.Context, mod int, query string, excludeFiles []string) (bool, sfvm.ValueOperator, error) {
		return prog.matchVariableWithExcludeFiles(ctx, ssadb.GlobCompare, mod, query, excludeFiles)
	}, g)
}

func (p *ProgramOverLay) RegexpMatch(ctx context.Context, mod int, re string) (bool, sfvm.ValueOperator, error) {
	return p.queryMatch(ctx, mod, func(prog *Program, ctx context.Context, mod int, query string, excludeFiles []string) (bool, sfvm.ValueOperator, error) {
		return prog.matchVariableWithExcludeFiles(ctx, ssadb.RegexpCompare, mod, query, excludeFiles)
	}, re)
}

func (p *ProgramOverLay) GetCalled() (sfvm.ValueOperator, error) {
	return nil, utils.Error("ProgramOverLay does not support GetCalled")
}

func (p *ProgramOverLay) GetCallActualParams(index int, contain bool) (sfvm.ValueOperator, error) {
	return nil, utils.Error("ProgramOverLay does not support GetCallActualParams")
}

func (p *ProgramOverLay) GetFields() (sfvm.ValueOperator, error) {
	return sfvm.NewEmptyValues(), nil
}

func (p *ProgramOverLay) GetSyntaxFlowUse() (sfvm.ValueOperator, error) {
	return nil, utils.Error("ProgramOverLay does not support GetSyntaxFlowUse")
}

func (p *ProgramOverLay) GetSyntaxFlowDef() (sfvm.ValueOperator, error) {
	return nil, utils.Error("ProgramOverLay does not support GetSyntaxFlowDef")
}

func (p *ProgramOverLay) GetSyntaxFlowTopDef(sfResult *sfvm.SFFrameResult, sfConfig *sfvm.Config, config ...*sfvm.RecursiveConfigItem) (sfvm.ValueOperator, error) {
	return nil, utils.Error("ProgramOverLay does not support GetSyntaxFlowTopDef")
}

func (p *ProgramOverLay) GetSyntaxFlowBottomUse(sfResult *sfvm.SFFrameResult, sfConfig *sfvm.Config, config ...*sfvm.RecursiveConfigItem) (sfvm.ValueOperator, error) {
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
	return utils.Error("ProgramOverLay does not support AppendPredecessor")
}

func (p *ProgramOverLay) FileFilter(path string, match string, rule map[string]string, rule2 []string) (sfvm.ValueOperator, error) {
	return nil, utils.Error("ProgramOverLay does not support FileFilter")
}

func (p *ProgramOverLay) CompareString(comparator *sfvm.StringComparator) (sfvm.ValueOperator, []bool) {
	return p, nil
}

func (p *ProgramOverLay) CompareOpcode(comparator *sfvm.OpcodeComparator) (sfvm.ValueOperator, []bool) {
	return p, nil
}

func (p *ProgramOverLay) CompareConst(comparator *sfvm.ConstComparator) []bool {
	return nil
}

func (p *ProgramOverLay) NewConst(i any, rng ...*memedit.Range) sfvm.ValueOperator {
	return nil
}
