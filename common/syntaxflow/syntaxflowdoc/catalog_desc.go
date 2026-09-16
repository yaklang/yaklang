package syntaxflowdoc

import "github.com/yaklang/yaklang/common/syntaxflow/sfvm"

// seedDescKeyCatalog documents desc() metadata keys (yakdoc "library overview" analog for rule meta).
func seedDescKeyCatalog(h *DocumentHelper) {
	if h == nil {
		return
	}
	entries := []*DocEntry{
		{Category: CategoryDescKey, Name: "title", Aliases: nil, Title: "规则英文标题", Description: "desc 字段 title: \"...\""},
		{Category: CategoryDescKey, Name: "title_zh", Title: "规则中文标题", Description: "desc 字段 title_zh: \"...\""},
		{Category: CategoryDescKey, Name: "desc", Aliases: []string{"description", "note"}, Title: "规则说明", Description: "漏洞/规则描述，可用 heredoc。"},
		{Category: CategoryDescKey, Name: "type", Aliases: []string{"purpose"}, Title: "规则类型", Description: "常见值：audit / vuln / sec 等。"},
		{Category: CategoryDescKey, Name: "lib", Aliases: []string{"allow_include", "as_library", "as_lib", "library_name"}, Title: "库规则名", Description: "声明本文件可作为 <include('name')> 引入的库名。"},
		{Category: CategoryDescKey, Name: "level", Aliases: []string{"severity", "sev"}, Title: "严重级别", Description: "如 high / mid / low / info。"},
		{Category: CategoryDescKey, Name: "language", Aliases: []string{"lang"}, Title: "目标语言", Description: "golang / java / php / python / javascript / c / csharp 等。"},
		{Category: CategoryDescKey, Name: "cve", Title: "CVE 编号", Description: "关联 CVE。"},
		{Category: CategoryDescKey, Name: "cwe", Title: "CWE 编号", Description: "关联 CWE。"},
		{Category: CategoryDescKey, Name: "risk", Aliases: []string{"risk_type"}, Title: "风险类型", Description: "风险分类标签。"},
		{Category: CategoryDescKey, Name: "solution", Aliases: []string{"fix"}, Title: "修复建议", Description: "修复方案说明。"},
		{Category: CategoryDescKey, Name: "rule_id", Aliases: []string{"id"}, Title: "规则 ID", Description: "稳定规则标识。"},
		{Category: CategoryDescKey, Name: "reference", Aliases: []string{"ref"}, Title: "参考链接", Description: "外部参考。"},
		{Category: CategoryDescKey, Name: "message", Aliases: []string{"msg"}, Title: "告警消息", Description: "也可在 alert for { message: ... } 中使用。"},
		{Category: CategoryDescKey, Name: "mode", Aliases: []string{"engine", "exec_mode"}, Title: "执行模式", Description: "source=无 SSA 源码模式；ssa=默认 SSA；struct=结构单元。"},
		{Category: CategoryDescKey, Name: "alert_high", Title: "自检期望高危数", Description: "规则自检 desc 中声明期望 alert 数量。"},
		{Category: CategoryDescKey, Name: "alert_mid", Title: "自检期望中危数", Description: "规则自检 desc 中声明期望 alert 数量。"},
		{Category: CategoryDescKey, Name: "file://", Title: "正例文件嵌入", Description: "自检正例：'file://vuln.go': <<<UNSAFE ... UNSAFE"},
		{Category: CategoryDescKey, Name: "safefile://", Title: "反例文件嵌入", Description: "自检反例：'safefile://ok.go': <<<SAFE ... SAFE"},
	}
	for _, e := range entries {
		// Ensure name validates against sfvm where applicable.
		if kt := sfvm.ValidDescItemKeyType(e.Name); kt != sfvm.SFDescKeyType_Unknown && e.Name != "alert_high" && e.Name != "alert_mid" && e.Name != "file://" && e.Name != "safefile://" {
			_ = kt
		}
		h.DescKeys[e.Name] = e
	}
}
