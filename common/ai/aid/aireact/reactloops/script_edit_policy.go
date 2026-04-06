package reactloops

import "strings"

const (
	LoopStateRequireEditBeforeExecution   = "require_edit_before_execution"
	LoopStateEditBeforeExecutionCompleted = "edit_before_execution_completed"
)

var existingScriptEditTerms = []string{
	"修改", "编辑", "更新", "改一下", "改这个", "增加注释", "添加注释", "注释",
	"modify", "edit", "update", "insert", "replace", "prepend", "append", "comment",
}

var existingScriptTargetTerms = []string{
	"这个脚本", "该脚本", "已有脚本", "现有脚本", "当前脚本", "脚本",
	"this script", "existing script", "current script", "script",
	".py", ".js", ".ts", ".sh", ".go", ".java",
}

var existingFileTargetTerms = []string{
	"这个文件", "该文件", "已有文件", "现有文件", "当前文件", "文件",
	"this file", "existing file", "current file", "file",
}

var editLocationTerms = []string{
	"开头", "头部", "前面", "上方", "函数上方", "增加一段", "插入一段",
	"at the top", "top of", "before function", "header", "comment block",
}

var rerunTerms = []string{
	"重新执行", "再执行", "重新运行", "再次运行", "重跑", "重新再执行", "执行一下", "然后执行",
	"rerun", "run again", "execute again", "re-execute", "run it again",
}

func DetectExistingScriptEditIntent(userQuery string) bool {
	query := strings.TrimSpace(strings.ToLower(userQuery))
	if query == "" {
		return false
	}

	hasEdit := containsAnyFold(query, existingScriptEditTerms...) ||
		containsAnyFold(query, editLocationTerms...)
	if !hasEdit {
		return false
	}

	hasScriptTarget := containsAnyFold(query, existingScriptTargetTerms...)
	hasFileTarget := containsAnyFold(query, existingFileTargetTerms...)
	if !hasScriptTarget && !hasFileTarget {
		return false
	}

	return true
}

func DetectEditThenExecuteIntent(userQuery string) bool {
	return DetectExistingScriptEditIntent(userQuery) && containsAnyFold(userQuery, rerunTerms...)
}

func ApplyScriptEditExecutionPolicy(loop *ReActLoop, recommendedCaps []string) []string {
	normalized := dedupeCapabilityNames(recommendedCaps)
	if loop == nil {
		return normalized
	}

	query := loop.Get("user_query")
	if !DetectExistingScriptEditIntent(query) {
		return normalized
	}

	loop.Set(LoopStateRequireEditBeforeExecution, "true")
	loop.Delete(LoopStateEditBeforeExecutionCompleted)

	normalized = moveCapabilityToFront(normalized, "modify_file")
	if DetectEditThenExecuteIntent(query) {
		normalized = moveCapabilityToPosition(normalized, "bash", 1)
	}
	return normalized
}

func IsEditToolName(toolName string) bool {
	switch strings.TrimSpace(toolName) {
	case "modify_file", "write_file":
		return true
	default:
		return false
	}
}

func MarkEditBeforeExecutionCompleted(loop *ReActLoop, toolName string) {
	if loop == nil || !IsEditToolName(toolName) {
		return
	}
	if strings.TrimSpace(loop.Get(LoopStateRequireEditBeforeExecution)) != "true" {
		return
	}
	loop.Set(LoopStateEditBeforeExecutionCompleted, "true")
}

func ShouldBlockBashUntilEdit(loop *ReActLoop, toolName string) bool {
	if loop == nil || strings.TrimSpace(toolName) != "bash" {
		return false
	}
	return strings.TrimSpace(loop.Get(LoopStateRequireEditBeforeExecution)) == "true" &&
		strings.TrimSpace(loop.Get(LoopStateEditBeforeExecutionCompleted)) != "true"
}

func BuildEditBeforeExecutionFeedback(loop *ReActLoop) string {
	query := ""
	if loop != nil {
		query = strings.TrimSpace(loop.Get("user_query"))
	}
	if query == "" {
		query = "当前任务要求先修改已有脚本，再执行。"
	}

	return "当前任务命中了“先编辑后执行”的强制路由策略，暂时禁止直接使用 bash 覆盖或重写脚本。\n" +
		"先使用 modify_file（优先）或 write_file 完成文件编辑，确认编辑步骤成功后，才能使用 bash 执行脚本。\n" +
		"用户请求: " + query + "\n\n" +
		"正确顺序示例:\n" +
		"1. {\"@action\":\"require_tool\",\"tool_require_payload\":\"modify_file\"}\n" +
		"2. 编辑完成后再调用 {\"@action\":\"require_tool\",\"tool_require_payload\":\"bash\"} 执行脚本"
}

func containsAnyFold(text string, terms ...string) bool {
	lower := strings.ToLower(text)
	for _, term := range terms {
		if term == "" {
			continue
		}
		if strings.Contains(lower, strings.ToLower(term)) {
			return true
		}
	}
	return false
}

func dedupeCapabilityNames(items []string) []string {
	seen := make(map[string]bool, len(items))
	result := make([]string, 0, len(items))
	for _, item := range items {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" || seen[trimmed] {
			continue
		}
		seen[trimmed] = true
		result = append(result, trimmed)
	}
	return result
}

func moveCapabilityToFront(items []string, target string) []string {
	return moveCapabilityToPosition(items, target, 0)
}

func moveCapabilityToPosition(items []string, target string, index int) []string {
	items = dedupeCapabilityNames(items)
	target = strings.TrimSpace(target)
	if target == "" {
		return items
	}

	filtered := make([]string, 0, len(items)+1)
	for _, item := range items {
		if item != target {
			filtered = append(filtered, item)
		}
	}

	if index < 0 {
		index = 0
	}
	if index > len(filtered) {
		index = len(filtered)
	}

	result := make([]string, 0, len(filtered)+1)
	result = append(result, filtered[:index]...)
	result = append(result, target)
	result = append(result, filtered[index:]...)
	return result
}
