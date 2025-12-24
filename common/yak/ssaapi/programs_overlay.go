package ssaapi

import (
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
	ShadowFile *utils.SafeMap[*memedit.MemEditor]

	// Diff 差量 Program (修改后的文件)
	Diff *Program

	// Base 基础 Program
	Base []*Program

	// signatureCache 缓存 Value 的签名，用于重定位
	signatureCache *utils.CacheWithKey[string, *Value]
}

// NewProgramOverLay 创建一个新的 ProgramOverLay
func NewProgramOverLay(diff *Program, bases ...*Program) *ProgramOverLay {
	overlay := &ProgramOverLay{
		ShadowFile:     utils.NewSafeMap[*memedit.MemEditor](),
		Diff:           diff,
		Base:           bases,
		signatureCache: utils.NewTTLCacheWithKey[string, *Value](0), // 永久缓存
	}

	// 构建 Shadow Set: 收集 Diff Program 中所有文件路径
	overlay.buildShadowSet()

	return overlay
}

// buildShadowSet 构建 Shadow Set，遍历 Diff Program 获取所有文件路径
func (p *ProgramOverLay) buildShadowSet() {
	if p.Diff == nil || p.Diff.Program == nil {
		return
	}

	// lazy load shadow files

	p.Diff.ForEachAllFile(func(s string, me *memedit.MemEditor) bool {
		// TODO: bug is here
		p.ShadowFile.Set(s, me)
		return true
	})

	log.Infof("ProgramOverLay: Shadow Set built with %d files", p.ShadowFile.Count())
}

// getFileIdentifier 生成文件的唯一标识符
// 格式: programName/folderPath/fileName
func (p *ProgramOverLay) getFileIdentifier(programName, folderPath, fileName string) string {
	// 确保路径格式统一
	if folderPath != "" && folderPath[0] != '/' {
		folderPath = "/" + folderPath
	}
	return programName + folderPath + fileName
}

// IsShadow 判断一个 Value 是否属于被修改的文件 (在 Shadow Set 中)
func (p *ProgramOverLay) IsShadow(v *Value) bool {
	if v == nil || p == nil {
		return false
	}

	if v.GetProgramName() == p.Diff.GetProgramName() {
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

	filePath := editor.GetUrl()

	return p.ShadowFile.Have(filePath)
}

// Relocate 实现基于签名的动态重定位
// 如果 Value 来自 Base 且其文件已被修改，则尝试在 Diff 中找到对应的 Value
func (p *ProgramOverLay) Relocate(v *Value) *Value {
	if v == nil || p == nil {
		return v
	}

	// 如果不是是 Shadow ，直接返回
	if !p.IsShadow(v) {
		return v
	}

	// 如果不在 Shadow Set 中，说明文件未被修改，直接返回 Base 中的 Value
	if !p.isValueFromShadowFile(v) {
		return v
	}

	// // 文件已被修改，尝试在 Diff 中重定位
	// signature := p.generateSignature(v)
	// if signature == "" {
	// 	return v // 无法生成签名，返回原值
	// }

	// // 尝试从缓存中获取
	// if cached, ok := p.signatureCache.Get(signature); ok {
	// 	return cached
	// }

	// // 在 Diff Program 中查找对应的 Value
	// diffValue := p.findValueBySignature(signature)
	// if diffValue != nil {
	// 	p.signatureCache.Set(signature, diffValue)
	// 	return diffValue
	// }

	// 未找到对应值，返回原值 (可能该符号在新版本中被删除)
	// log.Debugf("ProgramOverLay: Relocate failed for %s, using original value", signature)
	return v
}

// isValueFromShadowFile 检查 Value 的文件是否在 Shadow Set 中
func (p *ProgramOverLay) isValueFromShadowFile(v *Value) bool {
	if v == nil {
		return false
	}

	rng := v.GetRange()
	if rng == nil {
		return false
	}

	editor := rng.GetEditor()
	if editor == nil {
		return false
	}

	filePath := p.getFileIdentifier(editor.GetProgramName(),
		editor.GetFolderPath(), editor.GetFilename())

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
			if !p.isValueFromShadowFile(v) {
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
		if !p.isValueFromShadowFile(v) {
			finalResults = append(finalResults, v)
		}
	}

	return finalResults
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
