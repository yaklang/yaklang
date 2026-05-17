package aicommon

import (
	"fmt"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/ytoken"
)

// ToolInventoryTokenBudget 控制 Tool Inventory 段渲染时单次 prompt 中工具
// 列表 (含 name + description 行) 允许占用的 token 上限. 主路径
// (prompt_loop_materials.go) 与老路径 (prompts.go GetBasicPromptInfo) 都按
// 这个预算挑选可展示工具数量, 让"Top N tools"的 N 是 token 实测结果而不是
// 拍脑袋写死的常量.
//
// 预算定为 3000 token 是经验值: 大致够展示 20~40 个工具描述, 同时给
// frozen-block 段下方的 Forge Inventory / Timeline Frozen 留出空间.
// 关键词: ToolInventoryTokenBudget, 3K token 预算, Tool Inventory 裁剪
const ToolInventoryTokenBudget = 3000

// ToolInventoryMinCount 是 token 预算耗尽时仍要保留的最少工具数量. 即便
// 前几个工具的描述非常长导致 3K 预算瞬间打满, 也至少展示这么多个工具,
// 让 LLM 能稳定看到"可见工具池"的最小集.
// 关键词: ToolInventoryMinCount, 保底 20
const ToolInventoryMinCount = 20

// SelectToolsByTokenBudget 从 prioritized 工具列表里, 按 token 预算挑出
// 可展示工具子集, 同时受最小数量保底约束.
//
// 选择策略:
//  1. 当 prioritized 长度 <= minCount 时, 直接全量返回 (不再省略).
//  2. 当 prioritized 长度 >  minCount 时, 从头按工具描述行的 token 数
//     累加, 超过 budget 即截断, 但截断点最少保留 minCount 个工具.
//
// 每个工具的 token 估算与模板真实渲染行 "* `<name>`: <description>" 形态
// 保持一致, 计入名字、描述与列表项框架开销, 让结果与 LLM 真正消费的
// token 数尽量接近 (会略偏保守, 即估出来 >= 实际渲染).
//
// 关键词: SelectToolsByTokenBudget, Tool Inventory token 预算选择,
//
//	保底 minCount, 累加截断
func SelectToolsByTokenBudget(prioritized []*aitool.Tool, budget int, minCount int) []*aitool.Tool {
	if len(prioritized) == 0 {
		return prioritized
	}
	if minCount < 0 {
		minCount = 0
	}
	if minCount > len(prioritized) {
		minCount = len(prioritized)
	}
	if budget <= 0 {
		if minCount == 0 {
			return prioritized
		}
		return prioritized[:minCount]
	}

	used := 0
	idx := 0
	for ; idx < len(prioritized); idx++ {
		tool := prioritized[idx]
		if tool == nil {
			continue
		}
		line := fmt.Sprintf("* `%s`: %s\n", tool.Name, tool.Description)
		cost := ytoken.CalcTokenCount(line)
		if idx < minCount {
			used += cost
			continue
		}
		if used+cost > budget {
			break
		}
		used += cost
	}
	if idx > len(prioritized) {
		idx = len(prioritized)
	}
	if idx < minCount {
		idx = minCount
	}
	return prioritized[:idx]
}
