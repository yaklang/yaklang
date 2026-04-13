package aid

import (
	"encoding/json"
	"fmt"
	"net/url"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/utils"
)

const taskOutputInventoryPersistentKey = "test_asset_inventory"

var taskOutputInventoryURLPattern = regexp.MustCompile(`https?://[^\s'"<>]+`)

type taskOutputInventory struct {
	Entries []*taskOutputInventoryEntry `json:"entries"`
}

type taskOutputInventoryEntry struct {
	TaskIndex         string   `json:"task_index"`
	TaskName          string   `json:"task_name"`
	TaskGoal          string   `json:"task_goal,omitempty"`
	TaskStatus        string   `json:"task_status,omitempty"`
	TaskSummary       string   `json:"task_summary,omitempty"`
	TaskDir           string   `json:"task_dir,omitempty"`
	ResultSummaryPath string   `json:"result_summary_path,omitempty"`
	ToolNames         []string `json:"tool_names,omitempty"`
	Targets           []string `json:"targets,omitempty"`
	DiscoveredURLs    []string `json:"discovered_urls,omitempty"`
	UpdatedAt         string   `json:"updated_at,omitempty"`
}

func (m *PromptContextProvider) loadTaskOutputInventory() *taskOutputInventory {
	if m == nil {
		return &taskOutputInventory{}
	}
	raw, ok := m.GetPersistentData(taskOutputInventoryPersistentKey)
	if !ok || strings.TrimSpace(raw) == "" {
		return &taskOutputInventory{}
	}
	var inventory taskOutputInventory
	if err := json.Unmarshal([]byte(raw), &inventory); err != nil {
		return &taskOutputInventory{}
	}
	if inventory.Entries == nil {
		inventory.Entries = []*taskOutputInventoryEntry{}
	}
	return &inventory
}

func (m *PromptContextProvider) saveTaskOutputInventory(inventory *taskOutputInventory) {
	if m == nil || inventory == nil {
		return
	}
	if inventory.Entries == nil {
		inventory.Entries = []*taskOutputInventoryEntry{}
	}
	raw, err := json.MarshalIndent(inventory, "", "  ")
	if err != nil {
		return
	}
	m.SetPersistentData(taskOutputInventoryPersistentKey, string(raw))
}

func (m *PromptContextProvider) UpsertTaskOutputInventoryEntry(entry *taskOutputInventoryEntry) {
	if m == nil || entry == nil {
		return
	}
	entry.ToolNames = dedupeNonEmptyStrings(entry.ToolNames)
	entry.Targets = dedupeNonEmptyStrings(entry.Targets)
	entry.DiscoveredURLs = dedupeNonEmptyStrings(entry.DiscoveredURLs)
	entry.UpdatedAt = strings.TrimSpace(entry.UpdatedAt)

	inventory := m.loadTaskOutputInventory()
	for index, existing := range inventory.Entries {
		if existing == nil {
			continue
		}
		if existing.TaskIndex == entry.TaskIndex && entry.TaskIndex != "" {
			inventory.Entries[index] = entry
			m.saveTaskOutputInventory(inventory)
			return
		}
	}
	inventory.Entries = append(inventory.Entries, entry)
	m.saveTaskOutputInventory(inventory)
}

func (m *PromptContextProvider) RegisterTaskOutputSnapshot(task *AiTask, taskDir, resultSummaryPath string) {
	if m == nil || task == nil {
		return
	}
	m.UpsertTaskOutputInventoryEntry(buildTaskOutputInventoryEntry(task, taskDir, resultSummaryPath))
}

func (m *PromptContextProvider) TaskOutputInventoryTargets() []string {
	if m == nil {
		return nil
	}
	inventory := m.loadTaskOutputInventory()
	targets := make([]string, 0)
	for _, entry := range inventory.Entries {
		if entry == nil {
			continue
		}
		targets = append(targets, entry.Targets...)
		targets = append(targets, entry.DiscoveredURLs...)
	}
	return dedupeNonEmptyStrings(targets)
}

func (m *PromptContextProvider) TaskOutputInventoryContext() string {
	if m == nil {
		return ""
	}
	inventory := m.loadTaskOutputInventory()
	if len(inventory.Entries) == 0 {
		return ""
	}

	var builder strings.Builder
	builder.WriteString("以下是当前会话中已登记的共享测试资产库存。后续 plan 和 task 必须优先基于这些真实产物做分配，不允许只依据前序摘要挑少量目标。\n")
	builder.WriteString(fmt.Sprintf("库存常量 Key: %s\n\n", taskOutputInventoryPersistentKey))

	entries := make([]*taskOutputInventoryEntry, 0, len(inventory.Entries))
	for _, entry := range inventory.Entries {
		if entry != nil {
			entries = append(entries, entry)
		}
	}
	sort.SliceStable(entries, func(i, j int) bool {
		return entries[i].TaskIndex < entries[j].TaskIndex
	})

	for _, entry := range entries {
		builder.WriteString(fmt.Sprintf("- 任务 [%s] %s\n", entry.TaskIndex, entry.TaskName))
		if entry.TaskStatus != "" {
			builder.WriteString(fmt.Sprintf("  状态: %s\n", entry.TaskStatus))
		}
		if entry.ResultSummaryPath != "" {
			builder.WriteString(fmt.Sprintf("  结果摘要文件: %s\n", entry.ResultSummaryPath))
		}
		if entry.TaskDir != "" {
			builder.WriteString(fmt.Sprintf("  任务目录: %s\n", entry.TaskDir))
		}
		if entry.TaskSummary != "" {
			builder.WriteString(fmt.Sprintf("  摘要: %s\n", utils.ShrinkString(strings.TrimSpace(entry.TaskSummary), 280)))
		}
		if len(entry.ToolNames) > 0 {
			builder.WriteString(fmt.Sprintf("  工具: %s\n", strings.Join(entry.ToolNames, ", ")))
		}
		if len(entry.Targets) > 0 {
			builder.WriteString("  待处理目标:\n")
			for _, target := range entry.Targets {
				builder.WriteString(fmt.Sprintf("    - %s\n", target))
			}
		}
		if len(entry.DiscoveredURLs) > 0 {
			builder.WriteString(fmt.Sprintf("  发现 URL(%d):\n", len(entry.DiscoveredURLs)))
			for _, discoveredURL := range entry.DiscoveredURLs {
				builder.WriteString(fmt.Sprintf("    - %s\n", discoveredURL))
			}
		}
		if entry.UpdatedAt != "" {
			builder.WriteString(fmt.Sprintf("  更新时间: %s\n", entry.UpdatedAt))
		}
		builder.WriteString("\n")
	}

	return strings.TrimSpace(builder.String())
}

func buildTaskOutputInventoryEntry(task *AiTask, taskDir, resultSummaryPath string) *taskOutputInventoryEntry {
	entry := &taskOutputInventoryEntry{
		TaskIndex:         strings.TrimSpace(task.Index),
		TaskName:          strings.TrimSpace(task.Name),
		TaskGoal:          strings.TrimSpace(task.Goal),
		TaskStatus:        strings.TrimSpace(string(task.GetStatus())),
		TaskSummary:       strings.TrimSpace(task.GetSummary()),
		TaskDir:           strings.TrimSpace(taskDir),
		ResultSummaryPath: strings.TrimSpace(resultSummaryPath),
		UpdatedAt:         time.Now().Format("2006-01-02 15:04:05"),
	}

	entry.Targets = append(entry.Targets, extractInterfaceTargets(task.Goal)...)
	for _, result := range task.GetAllToolCallResults() {
		if result == nil {
			continue
		}
		entry.ToolNames = append(entry.ToolNames, strings.TrimSpace(result.Name))
		entry.DiscoveredURLs = append(entry.DiscoveredURLs, extractURLsFromText(extractToolResultStdout(result))...)
		entry.Targets = append(entry.Targets, extractTargetsFromToolResult(result)...)
	}

	entry.ToolNames = dedupeNonEmptyStrings(entry.ToolNames)
	entry.Targets = dedupeNonEmptyStrings(entry.Targets)
	entry.DiscoveredURLs = dedupeNonEmptyStrings(entry.DiscoveredURLs)
	return entry
}

func extractToolResultStdout(result *aitool.ToolResult) string {
	if result == nil || result.Data == nil {
		return ""
	}
	switch data := result.Data.(type) {
	case *aitool.ToolExecutionResult:
		return data.Stdout
	default:
		rawMap := utils.InterfaceToGeneralMap(result.Data)
		if len(rawMap) == 0 {
			return ""
		}
		return utils.MapGetString(rawMap, "stdout")
	}
}

func extractTargetsFromToolResult(result *aitool.ToolResult) []string {
	if result == nil {
		return nil
	}
	var targets []string
	paramMap := utils.InterfaceToGeneralMap(result.Param)
	for _, key := range []string{"url", "urls", "target", "targets", "request"} {
		value := strings.TrimSpace(utils.MapGetString(paramMap, key))
		if value == "" {
			continue
		}
		targets = append(targets, splitTargets(value)...)
	}
	return dedupeNonEmptyStrings(targets)
}

func splitTargets(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	if strings.Contains(raw, "\n") {
		return dedupeNonEmptyStrings(strings.Split(raw, "\n"))
	}
	if strings.Contains(raw, ",") {
		return dedupeNonEmptyStrings(strings.Split(raw, ","))
	}
	return []string{raw}
}

func dedupeNonEmptyStrings(items []string) []string {
	seen := make(map[string]struct{}, len(items))
	result := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		item = strings.TrimRight(item, ".,;，；")
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		result = append(result, item)
	}
	return result
}

func extractURLsFromText(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	matches := taskOutputInventoryURLPattern.FindAllString(raw, -1)
	for index, match := range matches {
		matches[index] = strings.TrimRight(match, ".,;)]")
	}
	return dedupeNonEmptyStrings(matches)
}

func inventoryTargetAliases(target string) []string {
	target = strings.TrimSpace(target)
	if target == "" {
		return nil
	}
	aliases := []string{target}
	if parsed, err := url.Parse(target); err == nil && parsed.Scheme != "" && parsed.Host != "" {
		pathWithQuery := parsed.EscapedPath()
		if parsed.RawQuery != "" {
			pathWithQuery += "?" + parsed.RawQuery
		}
		aliases = append(aliases, pathWithQuery)
		if unescaped, err := url.QueryUnescape(pathWithQuery); err == nil && unescaped != pathWithQuery {
			aliases = append(aliases, unescaped)
		}
		if parsed.Path != "" {
			aliases = append(aliases, parsed.Path)
		}
	}
	for index, alias := range aliases {
		aliases[index] = strings.ToLower(strings.TrimSpace(alias))
	}
	return dedupeNonEmptyStrings(aliases)
}

func buildTaskResultSummaryPath(taskDir string, task *AiTask) string {
	if task == nil {
		return ""
	}
	taskIndex := task.Index
	if taskIndex == "" {
		taskIndex = "0"
	}
	return filepath.Join(taskDir, aicommon.BuildTaskResultSummaryFilename(taskIndex, task.GetSemanticIdentifier()))
}
