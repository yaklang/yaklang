package loop_yaklangcode

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/log"
)

// ModifyRecord 记录单次 modify_code 操作
type ModifyRecord struct {
	StartLine int
	EndLine   int
	Content   string
}

// abs 返回整数的绝对值
func absInt(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// isInSameRegion 判断两次修改是否在相同区域（±5行内）
func isInSameRegion(r1, r2 ModifyRecord) bool {
	// 检查起始行和结束行是否都在相近范围内（±5行）
	startDistance := absInt(r1.StartLine - r2.StartLine)
	endDistance := absInt(r1.EndLine - r2.EndLine)

	// 两个条件都满足才算在同一区域
	return startDistance <= 5 && endDistance <= 5
}

// isSmallEdit 判断是否是小幅修改（≤3行）
func isSmallEdit(record ModifyRecord) bool {
	lineCount := record.EndLine - record.StartLine + 1
	return lineCount <= 3
}

// detectSpinning 检测是否在打转
// 返回: isSpinning bool, reason string
func detectSpinning(loop interface {
	Get(string) string
	Set(string, any)
}, currentRecord ModifyRecord) (bool, string) {

	// 获取历史记录
	historyStr := loop.Get("modify_history")
	spinCountStr := loop.Get("modify_spin_count")

	spinCount := 0
	if spinCountStr != "" {
		fmt.Sscanf(spinCountStr, "%d", &spinCount)
	}

	// 解析历史修改记录
	var historyRecords []ModifyRecord
	if historyStr != "" {
		lines := strings.Split(historyStr, ";")
		for _, line := range lines {
			var record ModifyRecord
			if n, _ := fmt.Sscanf(line, "%d-%d", &record.StartLine, &record.EndLine); n == 2 {
				historyRecords = append(historyRecords, record)
			}
		}
	}

	// 检查是否在相同区域重复修改
	isSameRegion := false
	isSmallChange := isSmallEdit(currentRecord)

	if len(historyRecords) > 0 {
		lastRecord := historyRecords[len(historyRecords)-1]
		if isInSameRegion(currentRecord, lastRecord) {
			isSameRegion = true
		}
	} else {
		// 第一次记录，初始化计数为1
		spinCount = 1
		log.Infof("first modify_code record, initialize spin count to 1, lines: %d-%d",
			currentRecord.StartLine, currentRecord.EndLine)
		// 保存记录并返回
		historyRecords = append(historyRecords, currentRecord)
		var historyParts []string
		for _, record := range historyRecords {
			historyParts = append(historyParts, fmt.Sprintf("%d-%d", record.StartLine, record.EndLine))
		}
		loop.Set("modify_history", strings.Join(historyParts, ";"))
		loop.Set("modify_spin_count", fmt.Sprintf("%d", spinCount))
		return false, ""
	}

	// 判断是否打转
	if isSameRegion && isSmallChange {
		spinCount++
		log.Infof("detected same region modification, spin count: %d, lines: %d-%d",
			spinCount, currentRecord.StartLine, currentRecord.EndLine)
	} else {
		// 修改区域变化明显，重置计数
		if !isSameRegion {
			log.Infof("modification region changed significantly, reset spin count")
			spinCount = 0 // 完全重置
		} else {
			// 区域相同但改动较大，不算打转但重置计数为1
			spinCount = 1
		}
	}

	// 保存当前记录到历史
	historyRecords = append(historyRecords, currentRecord)

	// 只保留最近3条记录
	if len(historyRecords) > 3 {
		historyRecords = historyRecords[len(historyRecords)-3:]
	}

	var historyParts []string
	for _, record := range historyRecords {
		historyParts = append(historyParts, fmt.Sprintf("%d-%d", record.StartLine, record.EndLine))
	}
	loop.Set("modify_history", strings.Join(historyParts, ";"))
	loop.Set("modify_spin_count", fmt.Sprintf("%d", spinCount))

	// 连续3次在相同区域小幅修改，判定为打转
	if spinCount >= 3 {
		reason := fmt.Sprintf("检测到在第 %d-%d 行附近连续 %d 次小幅修改代码，可能陷入了修改循环",
			currentRecord.StartLine, currentRecord.EndLine, spinCount)

		log.Warnf("spinning detected: %s", reason)

		// 重置计数，避免重复触发
		loop.Set("modify_spin_count", "0")
		loop.Set("spinning_triggered", "true")

		return true, reason
	}

	return false, ""
}

// generateReflectionPrompt 生成反思提示
func generateReflectionPrompt(record ModifyRecord, reason string) string {
	return fmt.Sprintf(`【代码修改空转警告】

%s

请停下来进行反思，回答以下问题：

【问题1：改动价值】
本次修改第 %d-%d 行的目标是什么？与上几次修改相比，有什么新的价值或进展？

【问题2：备选路径】
如果不继续修改这几行代码，还有哪些其他解决方案？
请至少列出 3 个不同层面的策略：
- 数据/变量层面的调整
- 算法/逻辑层面的重构
- 接口/API 调用方式的改变
- 使用不同的库或工具函数

【问题3：搜索建议】
强烈建议在继续修改前，先执行以下搜索以寻找正确的代码模式：

1. 使用 grep_yaklang_samples 搜索相关函数的用法示例
   - 搜索你正在使用的关键函数或API
   - 搜索相关的错误处理模式

2. 使用 semantic_search_yaklang_samples 进行语义搜索
   - 提出完整的问题："Yaklang中如何...?"
   - 从功能角度描述你要实现的目标

【行动建议】
请选择收益最高、风险最低的一个策略，并说明理由。
如果需要搜索代码示例，请先搜索再修改。

不要再继续在同一位置反复尝试小幅修改！`,
		reason,
		record.StartLine,
		record.EndLine)
}
