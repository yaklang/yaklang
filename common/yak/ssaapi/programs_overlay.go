package ssaapi

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/utils/memedit"
)

// ProgramLayer 表示一个编译层
type ProgramLayer struct {
	LayerIndex int                      // 层索引，从1开始（1=最底层）
	Program    *Program                 // 该层的 Program
	FileSet    *utils.SafeMap[struct{}] // 该层包含的文件路径集合
}

// ProgramOverLay 实现多层 Layer 增量编译的虚拟视图
// 核心思想：
//   - 多层 Layer 概念（Layer1, Layer2, Layer3...），没有 Base/Diff 的区别
//   - 文件系统聚合：所有 Layer 文件系统的 differ 聚合
//   - 查找策略：上层覆盖下层，从最上层开始查找
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
}

var _ sfvm.ValueOperator = (*ProgramOverLay)(nil)

// NewProgramOverLay 创建一个新的 ProgramOverLay
// layers 按顺序传入：layers[0] = Layer1（最底层），layers[1] = Layer2，以此类推
func NewProgramOverLay(layers ...*Program) *ProgramOverLay {
	overlay := &ProgramOverLay{
		Layers:         make([]*ProgramLayer, 0, len(layers)),
		FileToLayerMap: utils.NewSafeMap[int](),
		signatureCache: utils.NewTTLCacheWithKey[string, *Value](0), // 永久缓存
	}

	// 构建每层的 FileSet 和 FileToLayerMap
	for i, prog := range layers {
		if prog == nil {
			continue
		}

		layer := &ProgramLayer{
			LayerIndex: i + 1, // 从1开始
			Program:    prog,
			FileSet:    utils.NewSafeMap[struct{}](),
		}

		// 收集该层的所有文件
		prog.ForEachAllFile(func(filePath string, me *memedit.MemEditor) bool {
			// 规范化文件路径，去掉 UUID 前缀
			normalizedPath := normalizeFilePath(filePath)
			if normalizedPath == "" {
				return true
			}

			layer.FileSet.Set(normalizedPath, struct{}{})
			// 如果文件已经在更高层出现，不更新映射（上层覆盖下层）
			// 否则记录该文件在最上层出现的 Layer 索引
			if !overlay.FileToLayerMap.Have(normalizedPath) {
				overlay.FileToLayerMap.Set(normalizedPath, i+1)
			} else {
				// 更新为更上层的索引
				currentLayer, _ := overlay.FileToLayerMap.Get(normalizedPath)
				if i+1 > currentLayer {
					overlay.FileToLayerMap.Set(normalizedPath, i+1)
				}
			}
			return true
		})

		overlay.Layers = append(overlay.Layers, layer)
	}

	// 构建聚合文件系统
	if len(layers) > 0 {
		aggregatedFS, err := aggregateFileSystems(layers...)
		if err != nil {
			log.Errorf("failed to aggregate file systems: %v", err)
		} else {
			overlay.AggregatedFS = aggregatedFS
		}
	}

	log.Infof("ProgramOverLay: Built with %d layers, %d unique files", len(overlay.Layers), overlay.FileToLayerMap.Count())

	return overlay
}

// aggregateFileSystems 聚合所有 Layer 的文件系统
// 从最底层到最上层遍历，上层文件覆盖下层同名文件
func aggregateFileSystems(layers ...*Program) (fi.FileSystem, error) {
	// 使用 VirtualFS 作为基础
	aggregated := filesys.NewVirtualFs()

	// 从最底层到最上层遍历
	for _, layer := range layers {
		if layer == nil {
			continue
		}

		layer.ForEachAllFile(func(filePath string, editor *memedit.MemEditor) bool {
			content := editor.GetSourceCode()
			// 规范化文件路径，去掉 UUID 前缀
			normalizedPath := normalizeFilePath(filePath)
			if normalizedPath == "" {
				return true
			}
			// 上层文件会自动覆盖下层同名文件
			aggregated.AddFile(normalizedPath, content)
			return true
		})
	}

	return aggregated, nil
}

// GetLayerCount 获取 Layer 数量
func (p *ProgramOverLay) GetLayerCount() int {
	if p == nil {
		return 0
	}
	return len(p.Layers)
}

// GetFileCount 获取唯一文件数量
func (p *ProgramOverLay) GetFileCount() int {
	if p == nil {
		return 0
	}
	return p.FileToLayerMap.Count()
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
	firstPart := parts[0]
	if len(firstPart) == 36 && strings.Count(firstPart, "-") == 4 {
		// 第一部分是 UUID，去掉它
		if len(parts) > 1 {
			return "/" + strings.Join(parts[1:], "/")
		}
		return ""
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

			// 检查该文件是否在更高层（上层）也存在
			// 如果存在，说明上层覆盖了下层，跳过这个值
			if foundFiles.Have(filePath) {
				// 该文件已经在更高层找到，跳过
				continue
			}

			// 检查该文件是否在当前层
			if layer.FileSet.Have(filePath) {
				// 文件在当前层，标记为已找到
				foundFiles.Set(filePath, struct{}{})
				result = append(result, v)
			} else {
				// 文件不在当前层，可能是从其他层引用过来的
				// 检查该文件实际在哪个层
				actualLayerIndex, exists := p.FileToLayerMap.Get(filePath)
				if exists && actualLayerIndex > layer.LayerIndex {
					// 文件在更高层，跳过（会被更高层处理）
					continue
				}
				// 文件在当前层或更低层，添加
				foundFiles.Set(filePath, struct{}{})
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
			if values, ok := vals.(Values); ok {
				for _, v := range values {
					filePath := p.getValueFilePath(v)
					if filePath == "" {
						// 全局值，直接添加
						results = append(results, v)
						continue
					}

					// 如果文件已在更高层找到，跳过（被覆盖）
					if foundFiles.Have(filePath) {
						continue
					}

					// 检查文件是否在当前层
					if layer.FileSet.Have(filePath) {
						foundFiles.Set(filePath, struct{}{})
						results = append(results, v)
					} else {
						// 检查文件实际在哪个层
						actualLayerIndex, exists := p.FileToLayerMap.Get(filePath)
						if exists && actualLayerIndex > layer.LayerIndex {
							continue
						}
						foundFiles.Set(filePath, struct{}{})
						results = append(results, v)
					}
				}
			}
		}
	}

	return len(results) > 0, results, nil
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
