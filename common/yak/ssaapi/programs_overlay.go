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
	// 如果设置了 programName，返回它；否则返回最上层 layer 的 program name
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
	// 如果设置了 programKind，返回它；否则返回最上层 layer 的 program kind
	if p.programKind != "" {
		return p.programKind
	}
	if len(p.Layers) > 0 {
		topLayer := p.Layers[len(p.Layers)-1]
		if topLayer != nil && topLayer.Program != nil {
			return topLayer.Program.GetProgramKind()
		}
	}
	return ssadb.Application // 默认返回 Application
}

func (p *ProgramOverLay) GetLanguage() ssaconfig.Language {
	if p == nil {
		return ""
	}
	// 如果设置了 language，返回它；否则返回最上层 layer 的 language
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
	// 使用所有 layer 的 program names 生成 hash
	layerNames := p.GetLayerProgramNames()
	if len(layerNames) == 0 {
		return "", false
	}
	// 将 []string 转换为 []interface{} 用于 CalcSha256
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
		normalizedPath := normalizeFilePath(filePath)
		if normalizedPath == "" {
			return true
		}
		// Layer1 的所有文件都标记为 1（新增）
		layer.FileSet.Set(normalizedPath, struct{}{})
		layer.FileHashMap.Set(normalizedPath, 1)
		return true
	})

	return layer
}

func findFileInProgram(prog *Program, normalizedPath string) (found bool, content string) {
	if prog == nil {
		return false, ""
	}

	prog.ForEachAllFile(func(filePath string, editor *memedit.MemEditor) bool {
		path := normalizeFilePath(filePath)
		if path == normalizedPath {
			content = editor.GetSourceCode()
			found = true
			return false // 找到文件，停止遍历
		}
		return true
	})

	return found, content
}

func createOverlayFromLayers(layers ...*Program) *ProgramOverLay {
	if len(layers) < 2 {
		log.Errorf("createOverlayFromLayers requires at least 2 layers, got %d", len(layers))
		return nil
	}

	overlay := newEmptyOverlay()
	overlay.Layers = make([]*ProgramLayer, 0, len(layers))

	// Step 1: 创建 Layer1（基础层，全量编译）
	layer1 := createLayer1FromProgram(layers[0], 1)
	overlay.Layers = append(overlay.Layers, layer1)

	// Step 2: 处理后续的 layers（差量编译层）
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

		// 从 diffProgram 的 Program 结构体中获取 FileHashMap
		// FileHashMap 必须存在，否则返回错误
		var fileHashMap map[string]int
		if diffProgram.Program == nil {
			log.Errorf("diffProgram.Program is nil for layer %d", layerIndex)
			return nil
		}

		// 必须从 Program.FileHashMap 获取
		if len(diffProgram.Program.FileHashMap) == 0 {
			log.Errorf("FileHashMap is required for diff program in layer %d, but it is empty", layerIndex)
			return nil
		}

		fileHashMap = diffProgram.Program.FileHashMap

		// 设置 Layer 的 FileHashMap
		for filePath, hash := range fileHashMap {
			layer.FileHashMap.Set(filePath, hash)
		}

		// 填充 Layer 的文件集合和 FileToLayerMap
		diffProgram.ForEachAllFile(func(filePath string, me *memedit.MemEditor) bool {
			normalizedPath := normalizeFilePath(filePath)
			if normalizedPath == "" {
				return true
			}

			// 检查文件在 fileHashMap 中的状态
			hash := 1 // 默认新增
			if fileHashMap != nil {
				if h, exists := fileHashMap[normalizedPath]; exists {
					hash = h
				}
			}

			// 只有新增（hash=1）或修改（hash=0）的文件才应该出现在 Layer 的 FileSet 中
			// 删除的文件（hash=-1）不应该出现在 diffProgram 中，但如果出现了，也不应该添加到 FileSet
			if hash != -1 {
				layer.FileSet.Set(normalizedPath, struct{}{})
				overlay.FileToLayerMap.Set(normalizedPath, layerIndex)
			}
			return true
		})

		overlay.Layers = append(overlay.Layers, layer)
	}

	// Step 3: 构建 FileToLayerMap（对于 Layer1 中的文件，如果不在后续层，则记录为 Layer1）
	layers[0].ForEachAllFile(func(filePath string, me *memedit.MemEditor) bool {
		normalizedPath := normalizeFilePath(filePath)
		if normalizedPath == "" {
			return true
		}
		// 如果文件不在 FileToLayerMap 中（即不在后续层），检查是否应该保留在 Layer1
		if !overlay.FileToLayerMap.Have(normalizedPath) {
			// 检查文件是否被删除（在所有后续层中查找）
			isDeleted := false
			for i := 1; i < len(overlay.Layers); i++ {
				if hash, exists := overlay.Layers[i].FileHashMap.Get(normalizedPath); exists && hash == -1 {
					isDeleted = true
					break
				}
			}
			if !isDeleted {
				// 文件没有被删除，保留在 Layer1
				overlay.FileToLayerMap.Set(normalizedPath, 1)
			}
		}
		return true
	})

	// Step 4: 构建聚合文件系统
	aggregatedFS, err := overlay.aggregateFileSystems()
	if err != nil {
		log.Errorf("failed to aggregate file systems: %v", err)
	} else {
		overlay.AggregatedFS = aggregatedFS
	}

	log.Infof("ProgramOverLay: Built with %d layers (from pre-compiled programs), %d unique files",
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

	// 如果只有一个 layer，返回 nil（错误）
	if len(validLayers) == 1 {
		log.Errorf("NewProgramOverLay requires at least 2 layers, got 1")
		return nil
	}

	// 直接使用传入的 layers 创建 overlay（不再进行内部 diff 计算和编译）
	overlay := createOverlayFromLayers(validLayers...)

	// 设置 overlay 的元数据（从最上层 layer 获取）
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
// 基于 FileHashMap 聚合文件系统：
//   - 将所有 layer 的 FileHashMap 中每个文件的 hash 值相加
//   - 只有最终结果为 1 的文件才会被包含在聚合文件系统中
//   - 从最上层开始查找文件内容（上层覆盖下层）
func (p *ProgramOverLay) aggregateFileSystems() (fi.FileSystem, error) {
	// 使用 VirtualFS 作为基础
	aggregated := filesys.NewVirtualFs()

	// 获取聚合后的文件集合（hash 值相加为 1 的文件）
	aggregatedFilesSet := p.getAggregatedFilesSet()

	// 检查是否有 layer 包含 FileHashMap（增量编译模式）
	hasFileHashMap := false
	for _, layer := range p.Layers {
		if layer != nil && layer.FileHashMap != nil && layer.FileHashMap.Count() > 0 {
			hasFileHashMap = true
			break
		}
	}

	if !hasFileHashMap {
		// 如果没有 FileHashMap，直接返回错误
		return nil, utils.Errorf("FileHashMap is required for aggregateFileSystems, but no layer has FileHashMap")
	}

	// 增量编译模式：只包含 hash 值相加为 1 的文件
	// 从最上层开始查找文件内容（上层覆盖下层）
	aggregatedFilesSet.ForEach(func(normalizedPath string, _ struct{}) bool {
		// 从最上层开始查找文件
		for i := len(p.Layers) - 1; i >= 0; i-- {
			layer := p.Layers[i]
			if layer == nil {
				continue
			}

			found, content := findFileInProgram(layer.Program, normalizedPath)
			if found {
				aggregated.AddFile(normalizedPath, content)
				return true // 文件已找到，停止查找
			}
		}

		// 如果文件在所有层中都没有找到，记录警告
		log.Warnf("file %s should be in aggregated file system but not found in any layer", normalizedPath)
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

// getAggregatedFilesSet 获取聚合后的文件集合
// 通过将所有 ProgramLayer 的 FileHashMap 相加，只有最终结果为 1 的文件才能被算做是聚合后的文件系统
// 逻辑：
//   - 遍历所有 layer 的 FileHashMap，对每个文件路径的 hash 值求和
//   - 如果最终结果为 1，说明该文件是新增的（应该包含在聚合文件系统中）
//   - 如果最终结果为 0 或 -1，说明文件被修改或删除（不应该包含）
func (p *ProgramOverLay) getAggregatedFilesSet() *utils.SafeMap[struct{}] {
	fileSet := utils.NewSafeMap[struct{}]()

	if p == nil {
		return fileSet
	}

	// 收集所有文件路径及其 hash 值的总和
	fileHashSum := make(map[string]int)

	// 遍历所有 layer，累加每个文件的 hash 值
	for _, layer := range p.Layers {
		if layer == nil || layer.FileHashMap == nil {
			continue
		}

		layer.FileHashMap.ForEach(func(normalizedPath string, hash int) bool {
			fileHashSum[normalizedPath] += hash
			return true
		})
	}

	// 只有最终结果为 1 的文件才应该被包含
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
	return p.AggregatedFS
}

func programToFileSystem(prog *Program) fi.FileSystem {
	if prog == nil {
		return filesys.NewVirtualFs()
	}

	vfs := filesys.NewVirtualFs()
	prog.ForEachAllFile(func(filePath string, me *memedit.MemEditor) bool {
		if me == nil {
			return true
		}
		normalizedPath := normalizeFilePath(filePath)
		if normalizedPath == "" {
			return true
		}
		content := me.GetSourceCode()
		vfs.AddFile(normalizedPath, content)
		return true
	})
	return vfs
}

// calculateFileSystemDiff 计算两个文件系统的差异，返回差量文件系统和 hash 映射
// hash状态: -1=删除, 0=修改, 1=新增
func calculateFileSystemDiff(ctx context.Context, baseFS, newFS fi.FileSystem) (diffFS *filesys.VirtualFS, fileHashMap map[string]int, err error) {
	diffFS = filesys.NewVirtualFs()
	fileHashMap = make(map[string]int)

	// 收集 baseFS 的所有文件（路径已规范化，因为 programToFileSystem 使用了 normalizeFilePath）
	baseFiles := make(map[string][]byte)
	err = filesys.Recursive(".", filesys.WithFileSystem(baseFS), filesys.WithStat(func(isDir bool, pathname string, info os.FileInfo) error {
		if isDir {
			return nil
		}
		// 确保路径规范化（虽然 programToFileSystem 已经规范化了）
		normalizedPath := normalizeFilePath(pathname)
		if normalizedPath == "" {
			return nil // 跳过无效路径
		}
		file, err := baseFS.Open(pathname)
		if err != nil {
			return nil // 忽略无法打开的文件
		}
		defer file.Close()
		content, err := io.ReadAll(file)
		if err != nil {
			return nil
		}
		baseFiles[normalizedPath] = content
		return nil
	}))
	if err != nil {
		return nil, nil, utils.Wrap(err, "failed to collect baseFS files")
	}

	// 收集 newFS 的所有文件（路径已规范化）
	newFiles := make(map[string][]byte)
	err = filesys.Recursive(".", filesys.WithFileSystem(newFS), filesys.WithStat(func(isDir bool, pathname string, info os.FileInfo) error {
		if isDir {
			return nil
		}
		// 确保路径规范化
		normalizedPath := normalizeFilePath(pathname)
		if normalizedPath == "" {
			return nil // 跳过无效路径
		}
		file, err := newFS.Open(pathname)
		if err != nil {
			return nil // 忽略无法打开的文件
		}
		defer file.Close()
		content, err := io.ReadAll(file)
		if err != nil {
			return nil
		}
		newFiles[normalizedPath] = content
		return nil
	}))
	if err != nil {
		return nil, nil, utils.Wrap(err, "failed to collect newFS files")
	}

	// 计算差异
	// baseFiles 和 newFiles 中的路径已经是规范化后的路径
	// 1. 检查 baseFS 中的文件
	for filePath, baseContent := range baseFiles {
		newContent, existsInNew := newFiles[filePath]
		if !existsInNew {
			// 文件在 baseFS 存在但 newFS 不存在：删除
			fileHashMap[filePath] = -1
		} else if !equalContent(baseContent, newContent) {
			// 文件在两个文件系统都存在但内容不同：修改
			fileHashMap[filePath] = 0
			diffFS.AddFile(filePath, string(newContent))
		}
		// 如果文件存在且内容相同，不包含在差量中
	}

	// 2. 检查 newFS 中的新文件
	for filePath, newContent := range newFiles {
		if _, existsInBase := baseFiles[filePath]; !existsInBase {
			// 文件在 newFS 存在但 baseFS 不存在：新增
			fileHashMap[filePath] = 1
			diffFS.AddFile(filePath, string(newContent))
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

// normalizeFilePath 规范化文件路径，去掉 UUID 前缀，只保留相对路径部分
// 输入可能是 GetUrl() 格式: /programName/folderPath/fileName
// 或 GetFilePath() 格式: /folderPath/fileName
// 输出: /folderPath/fileName (去掉 programName/UUID 前缀)
func normalizeFilePath(filePath string) string {
	if filePath == "" {
		return ""
	}
	// 去掉开头的 /
	path := strings.TrimPrefix(filePath, "/")
	// 分割路径
	parts := strings.Split(path, "/")
	if len(parts) == 0 {
		return ""
	}
	// 检查第一部分是否是 UUID (格式: xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx)
	// 或者 UUID_diff (格式: xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx_diff)
	firstPart := parts[0]

	// 检查是否是 UUID 格式（36个字符，4个连字符）
	if len(firstPart) >= 36 && strings.Count(firstPart[:36], "-") == 4 {
		// 检查前36个字符是否是 UUID
		uuidPart := firstPart[:36]
		if len(uuidPart) == 36 && strings.Count(uuidPart, "-") == 4 {
			// 第一部分是 UUID 或 UUID_suffix，去掉它
			if len(parts) > 1 {
				return "/" + strings.Join(parts[1:], "/")
			}
			return ""
		}
	}

	// 不是 UUID，返回原路径
	return "/" + path
}

// getValueFilePath 获取 Value 的文件路径（规范化后的）
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

	// 获取文件路径，可能是 GetUrl() 或 GetFilePath() 格式
	filePath := editor.GetFilePath()
	if filePath == "" {
		// 如果 GetFilePath() 为空，尝试 GetUrl()
		filePath = editor.GetUrl()
	}

	// 规范化路径，去掉 UUID 前缀
	return normalizeFilePath(filePath)
}

// getValueLayerIndex 获取 Value 所在的层索引
func (p *ProgramOverLay) getValueLayerIndex(v *Value) int {
	if v == nil || p == nil {
		return 0
	}

	programName := v.GetProgramName()
	if programName == "" {
		return 0
	}

	// 通过 Program 名称找到对应的层
	for i, layer := range p.Layers {
		if layer != nil && layer.Program != nil {
			if layer.Program.GetProgramName() == programName {
				return i + 1 // 返回层索引（从1开始）
			}
		}
	}

	return 0
}

// IsOverridden 判断一个 Value 是否被上层 Layer 覆盖
func (p *ProgramOverLay) IsOverridden(v *Value) bool {
	if v == nil || p == nil {
		return false
	}

	filePath := p.getValueFilePath(v)
	if filePath == "" {
		return false
	}

	// 获取该文件在最上层出现的 Layer 索引
	topLayerIndex, exists := p.FileToLayerMap.Get(filePath)
	if !exists {
		return false
	}

	// 获取 Value 所在的层索引
	valueLayerIndex := p.getValueLayerIndex(v)
	if valueLayerIndex == 0 {
		return false
	}

	// 如果文件在更高层也存在，说明被覆盖
	return topLayerIndex > valueLayerIndex
}

// Ref 实现基于层的查找策略：从上层到下层，上层覆盖下层
func (p *ProgramOverLay) Ref(name string) Values {
	var result Values
	if p == nil {
		return result
	}

	// 用于去重：记录已经找到的文件路径，避免同一文件在不同层重复返回
	foundFiles := utils.NewSafeMap[struct{}]()

	// 从最上层开始查找（倒序遍历）
	for i := len(p.Layers) - 1; i >= 0; i-- {
		layer := p.Layers[i]
		if layer == nil || layer.Program == nil {
			continue
		}

		// 在该层查找
		layerValues := layer.Program.Ref(name)

		for _, v := range layerValues {
			// 获取 Value 的文件路径
			filePath := p.getValueFilePath(v)
			if filePath == "" {
				// 无法确定文件路径的值，直接添加（可能是全局值）
				result = append(result, v)
				continue
			}

			normalizedPath := normalizeFilePath(filePath)

			// 检查该文件是否在更高层（上层）被标记为删除
			// 如果文件在更高层的 FileHashMap 中被标记为 -1（删除），则跳过
			isDeleted := false
			for j := i + 1; j < len(p.Layers); j++ {
				upperLayer := p.Layers[j]
				if upperLayer != nil && upperLayer.FileHashMap != nil {
					if hash, exists := upperLayer.FileHashMap.Get(normalizedPath); exists && hash == -1 {
						isDeleted = true
						break
					}
				}
			}
			if isDeleted {
				// 文件在更高层被删除，跳过
				continue
			}

			// 检查该文件是否在更高层（上层）也存在
			// 如果存在，说明上层覆盖了下层，跳过这个值
			if foundFiles.Have(normalizedPath) {
				// 该文件已经在更高层找到，跳过
				continue
			}

			// 检查该文件是否在当前层
			if layer.FileSet.Have(normalizedPath) {
				// 文件在当前层，标记为已找到
				foundFiles.Set(normalizedPath, struct{}{})
				result = append(result, v)
			} else {
				// 文件不在当前层，可能是从其他层引用过来的
				// 检查该文件实际在哪个层
				actualLayerIndex, exists := p.FileToLayerMap.Get(normalizedPath)
				if exists && actualLayerIndex > layer.LayerIndex {
					// 文件在更高层，跳过（会被更高层处理）
					continue
				}
				// 文件在当前层或更低层，添加
				foundFiles.Set(normalizedPath, struct{}{})
				result = append(result, v)
			}
		}
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

// findValueInLayer 在指定层中查找对应的 Value（通过签名匹配）
func (p *ProgramOverLay) findValueInLayer(layer *ProgramLayer, v *Value) *Value {
	if layer == nil || layer.Program == nil {
		return nil
	}

	// 生成签名规则
	rule := p.generateRelocateRule(v)
	if rule == "" {
		return nil
	}

	// 尝试从缓存获取
	cacheKey := fmt.Sprintf("%s:%s", layer.Program.GetProgramName(), rule)
	if cached, ok := p.signatureCache.Get(cacheKey); ok {
		return cached
	}

	// 在该层中查找
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

// Relocate 实现基于多层架构的跨层 Value 重定位
// 如果 Value 在下层，且文件在上层也存在，尝试在上层找到对应的值
func (p *ProgramOverLay) Relocate(v *Value) *Value {
	if v == nil || p == nil {
		return v
	}

	filePath := p.getValueFilePath(v)
	if filePath == "" {
		return v // 无法确定文件路径，直接返回
	}

	// 找到该文件所在的层
	fileLayerIndex, exists := p.FileToLayerMap.Get(filePath)
	if !exists {
		return v // 文件不在任何层中
	}

	// 获取 Value 所在的层
	valueLayerIndex := p.getValueLayerIndex(v)
	if valueLayerIndex == 0 {
		return v // 无法确定 Value 所在的层
	}

	// 如果 Value 在下层，且文件在上层也存在，尝试在上层找到对应的值
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

// Implement sfvm.ValueOperator interface

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
	// 从最上层到最下层遍历
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
	queryFunc func(*Program, context.Context, int, string) (bool, sfvm.ValueOperator, error),
	query string,
) (bool, sfvm.ValueOperator, error) {
	if p == nil {
		return false, nil, nil
	}

	var results Values
	foundFiles := utils.NewSafeMap[struct{}]() // 去重：已找到的文件

	// 从最上层开始查找（倒序遍历）
	for i := len(p.Layers) - 1; i >= 0; i-- {
		layer := p.Layers[i]
		if layer == nil || layer.Program == nil {
			continue
		}

		matched, vals, err := queryFunc(layer.Program, ctx, mod, query)
		if err != nil {
			continue
		}

		if matched {
			vals.Recursive(func(op sfvm.ValueOperator) error {
				if v, ok := op.(*Value); ok {
					filePath := p.getValueFilePath(v)
					if filePath == "" {
						// 全局值，直接添加
						results = append(results, v)
						return nil
					}

					// 如果文件已在更高层找到，跳过（被覆盖）
					if foundFiles.Have(filePath) {
						return nil
					}

					// 检查文件是否在当前层
					if layer.FileSet.Have(filePath) {
						foundFiles.Set(filePath, struct{}{})
						results = append(results, v)
					} else {
						// 检查文件实际在哪个层
						actualLayerIndex, exists := p.FileToLayerMap.Get(filePath)
						if exists && actualLayerIndex > layer.LayerIndex {
							return nil
						}
						foundFiles.Set(filePath, struct{}{})
						results = append(results, v)
					}
				}
				return nil
			})
		}
	}

	return len(results) > 0, ValuesToSFValueList(results), nil
}

func (p *ProgramOverLay) ExactMatch(ctx context.Context, mod int, want string) (bool, sfvm.ValueOperator, error) {
	return p.queryMatch(ctx, mod, func(prog *Program, ctx context.Context, mod int, query string) (bool, sfvm.ValueOperator, error) {
		return prog.ExactMatch(ctx, mod, query)
	}, want)
}

func (p *ProgramOverLay) GlobMatch(ctx context.Context, mod int, g string) (bool, sfvm.ValueOperator, error) {
	return p.queryMatch(ctx, mod, func(prog *Program, ctx context.Context, mod int, query string) (bool, sfvm.ValueOperator, error) {
		return prog.GlobMatch(ctx, mod, query)
	}, g)
}

func (p *ProgramOverLay) RegexpMatch(ctx context.Context, mod int, re string) (bool, sfvm.ValueOperator, error) {
	return p.queryMatch(ctx, mod, func(prog *Program, ctx context.Context, mod int, query string) (bool, sfvm.ValueOperator, error) {
		return prog.RegexpMatch(ctx, mod, query)
	}, re)
}

func (p *ProgramOverLay) GetCalled() (sfvm.ValueOperator, error) {
	return nil, utils.Error("ProgramOverLay does not support GetCalled")
}

func (p *ProgramOverLay) GetCallActualParams(index int, contain bool) (sfvm.ValueOperator, error) {
	return nil, utils.Error("ProgramOverLay does not support GetCallActualParams")
}

func (p *ProgramOverLay) GetFields() (sfvm.ValueOperator, error) {
	return nil, utils.Error("ProgramOverLay does not support GetFields")
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
