package ssaapi

import (
	"context"
	"fmt"
	"regexp"

	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/memedit"
)

// ProgramOverLay 实现增量编译的虚拟视图
// effective program = diff-program ∪ (base-program - shadow-program)
// 核心思想：
//   - Diff: 差量文件系统编译的 SSA IR (所有修改过的文件)
//   - Shadow Set: 在 Diff 中出现的文件路径列表 (被修改的文件名)
//   - Base: 基础仓库的 SSA IR
//
// 虚拟视图逻辑: 凡是在 Diff 中的文件，Base 中对应文件的 IR 节点逻辑上视为"已删除"
type ProgramOverLay struct {
	// ShadowFile 存储被修改的文件路径 (Shadow Set)
	// key: 文件的唯一标识 (programName/folderPath/fileName)
	ShadowFile *utils.SafeMap[struct{}]

	// Diff 差量 Program (修改后的文件)
	Diff *Program

	// Base 基础 Program
	Base []*Program

	// signatureCache 缓存 Value 的签名，用于重定位
	signatureCache *utils.CacheWithKey[string, *Value]
}

var _ sfvm.ValueOperator = (*ProgramOverLay)(nil)

// NewProgramOverLay 创建一个新的 ProgramOverLay
func NewProgramOverLay(diff *Program, bases ...*Program) *ProgramOverLay {
	overlay := &ProgramOverLay{
		ShadowFile:     utils.NewSafeMap[struct{}](),
		Diff:           diff,
		Base:           bases,
		signatureCache: utils.NewTTLCacheWithKey[string, *Value](0), // 永久缓存
	}

	// 构建 Shadow Set: 收集 Diff Program 中所有文件路径
	overlay.buildShadowSet()

	return overlay
}

// GetShadowFileCount 获取 Shadow Set 中的文件数量（用于测试）
func (p *ProgramOverLay) GetShadowFileCount() int {
	if p == nil {
		return 0
	}
	return p.ShadowFile.Count()
}

// GetShadowFiles 获取所有被修改的文件路径（用于测试）
func (p *ProgramOverLay) GetShadowFiles() []string {
	if p == nil {
		return nil
	}
	return p.ShadowFile.Keys()
}

// buildShadowSet 构建 Shadow Set，遍历 Diff Program 获取所有文件路径
func (p *ProgramOverLay) buildShadowSet() {
	if p.Diff == nil || p.Diff.Program == nil {
		return
	}

	// lazy load shadow files

	p.Diff.ForEachAllFile(func(s string, me *memedit.MemEditor) bool {
		// TODO: bug is here
		p.ShadowFile.Set(me.GetFilePath(), struct{}{})
		return true
	})

	log.Infof("ProgramOverLay: Shadow Set built with %d files", p.ShadowFile.Count())
}

// IsShadow 判断一个 Value 是否属于被修改的文件 (在 Shadow Set 中)
func (p *ProgramOverLay) IsShadow(v *Value) bool {
	if v == nil || p == nil {
		return false
	}

	if v.GetProgramName() == p.Diff.GetProgramName() {
		// 来自 Diff Program 的 Value 肯定不是 Shadow 直接用
		return false
	}

	// 获取 Value 的文件路径
	rng := v.GetRange()
	if rng == nil {
		return false
	}

	editor := rng.GetEditor()
	if editor == nil {
		return false
	}

	filePath := editor.GetFilePath()

	return p.ShadowFile.Have(filePath)
}

// Query 实现覆盖优先的查询策略
// 1. 先在 Diff 中查询
// 2. 如果 Diff 中没找到，在 Base 中查询
// 3. 过滤掉 Base 中属于 Shadow Set 的结果
func (p *ProgramOverLay) Ref(name string) Values {
	var result Values

	// Step 1: 在 Diff 中搜索
	if p.Diff != nil {
		diffValues := p.Diff.Ref(name)
		result = append(result, diffValues...)
	}

	// Step 2 & 3: 在 Base 中搜索，并过滤 Shadow Set
	for _, baseProg := range p.Base {
		baseValues := baseProg.Ref(name)
		for _, v := range baseValues {
			// 检查是否在 Shadow Set 中
			if !p.IsShadow(v) {
				result = append(result, v)
			}
		}
	}

	return result
}

// MergeResults 合并两个搜索结果，应用"同文件用最新"原则
func (p *ProgramOverLay) MergeResults(baseResults, diffResults Values) Values {
	var finalResults Values

	// 1. 优先添加 Diff 结果
	finalResults = append(finalResults, diffResults...)

	// 2. 过滤添加 Base 结果 (排除已在 Shadow Set 中的文件)
	for _, v := range baseResults {
		if !p.IsShadow(v) {
			finalResults = append(finalResults, v)
		}
	}

	return finalResults
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

	log.Errorf("syntaxflow rule: \n%s", rule)
	return rule
}

// findValueBySignature 在 Diff Program 中根据签名查找对应的 Value
func (p *ProgramOverLay) findValueBySignature(rule string) *Value {
	if p.Diff.IsEmpty() {
		return nil
	}

	// 从签名中提取名称 (简化处理，实际可能需要更复杂的解析)
	// 签名格式: path/file.ext:res 或 path/file.ext:class.method(...)
	res, err := p.Diff.SyntaxFlowWithError(fmt.Sprintf("%s", rule), QueryWithEnableDebug())
	if err != nil {
		log.Errorf("search value by Rule failed in Diff program: %v", err)
		return nil
	}
	res.Show()

	// 使用 Ref 搜索
	values := res.GetAllValuesChain()

	// 如果只有一个匹配，直接返回
	if len(values) <= 0 {
		log.Errorf("findValueBySignature not found ")
		return nil
	}

	// 如果有多个匹配，尝试根据签名进一步过滤
	log.Errorf("multiple value : %s", values)
	return values[0]
}

// Relocate 实现基于签名的动态重定位
// 如果 Value 来自 Base 且其文件已被修改，则尝试在 Diff 中找到对应的 Value
func (p *ProgramOverLay) Relocate(v *Value) *Value {
	if v == nil || p == nil {
		return v
	}

	// 如果不在 Shadow Set 中，说明文件未被修改，直接返回 Base 中的 Value
	if !p.IsShadow(v) {
		return v
	}

	// // 文件已被修改，尝试在 Diff 中重定位
	relocateRule := p.generateRelocateRule(v)
	if relocateRule == "" {
		return v // 无法生成签名，返回原值
	}

	// // 尝试从缓存中获取
	if cached, ok := p.signatureCache.Get(relocateRule); ok {
		return cached
	}

	// // 在 Diff Program 中查找对应的 Value
	diffValue := p.findValueBySignature(relocateRule)
	if diffValue != nil {
		p.signatureCache.Set(relocateRule, diffValue)
		return diffValue
	}

	// 未找到对应值，返回原值 (可能该符号在新版本中被删除)
	log.Debugf("ProgramOverLay: Relocate failed for %s, using original value", relocateRule)
	return v
}

// Implement sfvm.ValueOperator interface

func (p *ProgramOverLay) String() string {
	return fmt.Sprintf("ProgramOverLay(diff=%v, bases=%d, shadows=%d)", p.Diff, len(p.Base), p.ShadowFile.Count())
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
	if p.Diff != nil && !p.Diff.IsEmpty() {
		return false
	}
	for _, base := range p.Base {
		if base != nil && !base.IsEmpty() {
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
	if p.Diff != nil {
		if err := f(p.Diff); err != nil {
			return err
		}
	}
	for _, base := range p.Base {
		if base != nil {
			if err := f(base); err != nil {
				return err
			}
		}
	}
	return nil
}

func (p *ProgramOverLay) ExactMatch(ctx context.Context, mod int, want string) (bool, sfvm.ValueOperator, error) {
	// Search in diff first
	var results Values
	if p.Diff != nil {
		matched, vals, err := p.Diff.ExactMatch(ctx, mod, want)
		if err == nil && matched {
			if values, ok := vals.(Values); ok {
				results = append(results, values...)
			}
		}
	}

	// Search in base programs, filter shadow values
	for _, base := range p.Base {
		if base != nil {
			matched, vals, err := base.ExactMatch(ctx, mod, want)
			if err == nil && matched {
				if values, ok := vals.(Values); ok {
					for _, v := range values {
						if !p.IsShadow(v) {
							results = append(results, v)
						}
					}
				}
			}
		}
	}

	return len(results) > 0, results, nil
}

func (p *ProgramOverLay) GlobMatch(ctx context.Context, mod int, g string) (bool, sfvm.ValueOperator, error) {
	// Search in diff first
	var results Values
	if p.Diff != nil {
		matched, vals, err := p.Diff.GlobMatch(ctx, mod, g)
		if err == nil && matched {
			if values, ok := vals.(Values); ok {
				results = append(results, values...)
			}
		}
	}

	// Search in base programs, filter shadow values
	for _, base := range p.Base {
		if base != nil {
			matched, vals, err := base.GlobMatch(ctx, mod, g)
			if err == nil && matched {
				if values, ok := vals.(Values); ok {
					for _, v := range values {
						if !p.IsShadow(v) {
							results = append(results, v)
						}
					}
				}
			}
		}
	}

	return len(results) > 0, results, nil
}

func (p *ProgramOverLay) RegexpMatch(ctx context.Context, mod int, re string) (bool, sfvm.ValueOperator, error) {
	// Search in diff first
	var results Values
	if p.Diff != nil {
		matched, vals, err := p.Diff.RegexpMatch(ctx, mod, re)
		if err == nil && matched {
			if values, ok := vals.(Values); ok {
				results = append(results, values...)
			}
		}
	}

	// Search in base programs, filter shadow values
	for _, base := range p.Base {
		if base != nil {
			matched, vals, err := base.RegexpMatch(ctx, mod, re)
			if err == nil && matched {
				if values, ok := vals.(Values); ok {
					for _, v := range values {
						if !p.IsShadow(v) {
							results = append(results, v)
						}
					}
				}
			}
		}
	}

	return len(results) > 0, results, nil
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
