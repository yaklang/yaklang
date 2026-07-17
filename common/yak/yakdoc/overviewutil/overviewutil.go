// Package overviewutil 提供库级 overview markdown 的统一读取与"首段摘要"派生。
//
// 它是 overviews/<lib>.md 这份"唯一作者源"的共享消费方:
//   - web 文档生成器读取全文注入站点;
//   - doc 生成器(generate_doc)取首段写入 ScriptLib.OverviewShort, 烤进 doc.gob.zst,
//     供 AI loop 在运行时零成本拼"库选择索引"。
//
// 本包仅在文档/数据生成阶段被 CLI 工具使用, 不会被引擎运行时导入, 因此不引入任何 embed,
// 也不增加引擎二进制体积。
//
// 关键词: overviewutil, overviews 单一数据源, 首段摘要, 库选择索引, 零 embed
package overviewutil

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/yaklang/yaklang/common/log"
)

// LoadAll 读取 dir 下所有 <lib>.md, 返回 库名 -> 总览全文(已 TrimSpace) 的映射。
// 目录不存在或为空时返回空映射(模块总览为可选增强)。
func LoadAll(dir string) map[string]string {
	res := map[string]string{}
	if strings.TrimSpace(dir) == "" {
		return res
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		log.Warnf("read overviews dir %s failed: %v", dir, err)
		return res
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".md")
		content, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			log.Warnf("read overview %s failed: %v", e.Name(), err)
			continue
		}
		res[name] = strings.TrimSpace(string(content))
	}
	if len(res) > 0 {
		log.Infof("loaded %d yakdoc module overview(s) from %s", len(res), dir)
	}
	return res
}

// maxShortRunes 控制库选择索引里单库摘要的最大长度, 保证整体 prompt 紧凑。
const maxShortRunes = 200

// FirstParagraph 从 overview 全文派生"一句话库定位": 跳过前导空行/标题/代码围栏,
// 取第一个非空正文段落(到下一个空行为止), 归一化空白并按 maxShortRunes 截断。
// 用于生成紧凑的库选择索引, 避免把全文塞进 prompt。
func FirstParagraph(md string) string {
	lines := strings.Split(strings.ReplaceAll(md, "\r", ""), "\n")
	var paragraph []string
	inFence := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			// 第一段之前的代码围栏直接跳过; 若已开始收集正文则在围栏处停止
			if len(paragraph) > 0 {
				break
			}
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		if trimmed == "" {
			if len(paragraph) > 0 {
				break // 段落结束
			}
			continue // 前导空行
		}
		// 跳过前导 markdown 标题行(如 "# file")
		if len(paragraph) == 0 && strings.HasPrefix(trimmed, "#") {
			continue
		}
		paragraph = append(paragraph, trimmed)
	}

	summary := strings.Join(strings.Fields(strings.Join(paragraph, " ")), " ")
	runes := []rune(summary)
	if len(runes) > maxShortRunes {
		summary = string(runes[:maxShortRunes]) + "..."
	}
	return summary
}
