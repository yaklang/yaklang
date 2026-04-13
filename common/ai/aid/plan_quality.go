package aid

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/yaklang/yaklang/common/log"
)

const (
	planTaskPreferredTargetCount = 3
	planQualityRetryLimit        = 2
)

var (
	planMaterialSectionLabels   = []string{"已知材料：", "已知材料:", "输入材料：", "输入材料:", "available materials:", "materials:"}
	planTargetSectionLabels     = []string{"待测列表：", "待测列表:", "目标列表：", "目标列表:", "scope:", "targets:", "目标：", "目标:"}
	planAcceptanceSectionLabels = []string{"验收标准：", "验收标准:", "acceptance criteria:", "acceptance:", "验证要求：", "验证要求:", "verification requirements:", "verification requirements："}
	planGroupTaskPattern        = regexp.MustCompile(`第\s*\d+\s*组|group\s*\d+|当前分组|本组范围|本组目标`)
	planBulletPrefixPattern     = regexp.MustCompile(`^\s*(?:[-*•]|\d+[.)])\s+`)
)

type planQualityIssue struct {
	TaskName        string
	TaskGoal        string
	Reasons         []string
	TargetItems     []string
	AcceptanceItems []string
	MaterialItems   []string
}

type taskGoalSections struct {
	Materials  string
	Targets    string
	Acceptance string
}

func (i planQualityIssue) String() string {
	if len(i.Reasons) == 0 {
		return fmt.Sprintf("- 任务 %q 需要细化", i.TaskName)
	}
	parts := []string{fmt.Sprintf("- 任务 %q 存在以下问题: %s", i.TaskName, strings.Join(i.Reasons, "；"))}
	if len(i.TargetItems) > 0 {
		parts = append(parts, fmt.Sprintf("  当前待测列表: %s", strings.Join(i.TargetItems, "；")))
	}
	if len(i.AcceptanceItems) > 0 {
		parts = append(parts, fmt.Sprintf("  当前验收标准: %s", strings.Join(i.AcceptanceItems, "；")))
	}
	return strings.Join(parts, "\n")
}

func (pr *planRequest) improvePlanQuality(root *AiTask) *AiTask {
	current := root
	for attempt := 0; attempt < planQualityRetryLimit; attempt++ {
		if current == nil {
			return root
		}

		if splitOversizedLeafTasks(current) {
			current = pr.cod.standardizeTaskTreeAndNotify(current, "plan auto-split oversized leaf tasks")
		}

		issues := collectPlanQualityIssues(current)
		missingInventoryTargets := pr.collectInventoryCoverageGaps(current)
		if len(missingInventoryTargets) > 0 {
			issues = append(issues, planQualityIssue{
				TaskName:    taskOutputInventoryPersistentKey,
				TaskGoal:    "shared task output inventory",
				Reasons:     []string{fmt.Sprintf("共享测试资产库存中仍有 %d 个目标未被任何子任务覆盖，不能直接接受当前计划", len(missingInventoryTargets))},
				TargetItems: missingInventoryTargets,
			})
		}
		if len(issues) == 0 {
			return current
		}

		if attempt == planQualityRetryLimit-1 {
			if len(missingInventoryTargets) > 0 {
				current = appendInventoryCoverageTasks(current, missingInventoryTargets)
				current = pr.cod.standardizeTaskTreeAndNotify(current, "plan auto-appended inventory coverage tasks")
			}
			log.Warnf("plan quality issues remain after retries: %v", formatPlanQualityIssues(issues))
			pr.cod.EmitInfo("plan quality issues remain after retries: %s", formatPlanQualityIssues(issues))
			return current
		}

		extraPrompt := buildPlanQualityExtraPrompt(issues)
		newPlan, err := pr.generateNewPlan("incomplete", extraPrompt, pr.cod.newPlanResponse(current))
		if err != nil {
			log.Warnf("generate plan for quality refinement failed: %v", err)
			pr.cod.EmitInfo("plan quality refinement failed: %v", err)
			return current
		}
		if newPlan == nil || newPlan.RootTask == nil {
			return current
		}
		current = pr.cod.standardizeTaskTreeAndNotify(newPlan.RootTask, fmt.Sprintf("plan quality refinement attempt %d", attempt+1))
	}
	return current
}

func buildPlanQualityExtraPrompt(issues []planQualityIssue) string {
	var builder strings.Builder
	builder.WriteString("请重新规划当前任务树，并严格修复以下问题。核心要求不是靠猜，而是把每个叶子任务写成可直接执行、可直接验证的 goal。\n")
	builder.WriteString("\n强约束：\n")
	builder.WriteString("1. 每个叶子任务的 subtask_goal 都必须至少包含两个分段：待测列表： 和 验收标准：。\n")
	builder.WriteString("2. 待测列表必须是具体清单，逐条列出本任务真正要处理的对象；如果没有足够材料，请先新增一个‘收集材料/补齐材料’子任务，而不是直接编造执行任务。\n")
	builder.WriteString("3. 验收标准必须逐条说明如何判断完成，不允许写‘进一步分析’‘检查是否有问题’‘输出结果即可’这类空泛描述。\n")
	builder.WriteString("4. 一个叶子任务应当保持很短的执行范围。若待测列表超过 3 项，必须拆成多个更小的子任务，每个子任务通常只保留 3 项左右。\n")
	builder.WriteString("5. 禁止使用 等、等等、相关、若干、多个、一些、其它、相关模块 等模糊词替代具体目标。\n")
	builder.WriteString("6. 不得凭空发明新的目标。只能基于用户输入、已有材料、上游任务产出进行规划。材料不足时，先规划材料收集任务。\n")
	builder.WriteString("\nFew-shot 参考：\n")
	builder.WriteString("坏例子：\n")
	builder.WriteString("subtask_goal: 测试用户相关接口，检查是否存在问题等。\n\n")
	builder.WriteString("好例子：\n")
	builder.WriteString("subtask_goal:\n")
	builder.WriteString("待测列表：\n")
	builder.WriteString("- GET /user/id?id=1\n")
	builder.WriteString("- GET /user/name?name=admin\n")
	builder.WriteString("- GET /user/limit/int?limit=1\n")
	builder.WriteString("验收标准：\n")
	builder.WriteString("- 每个目标都至少执行一次真实请求并记录结果\n")
	builder.WriteString("- 对每个目标明确标注已完成、未完成或阻断原因\n")
	builder.WriteString("- 产出本子任务的结论摘要或证据文件\n\n")
	builder.WriteString("材料不足时的正确做法：\n")
	builder.WriteString("坏例子：\n")
	builder.WriteString("subtask_goal: 测试登录相关接口安全性。\n\n")
	builder.WriteString("好例子：\n")
	builder.WriteString("先新增一个材料收集子任务，其 subtask_goal 应写成：\n")
	builder.WriteString("待测列表：\n")
	builder.WriteString("- 已知抓包记录中的登录请求\n")
	builder.WriteString("- API 文档中提到的登录入口\n")
	builder.WriteString("- 页面或 JS 中暴露的认证路径\n")
	builder.WriteString("验收标准：\n")
	builder.WriteString("- 输出可执行的登录接口清单\n")
	builder.WriteString("- 为每个接口记录方法、路径、关键参数\n")
	builder.WriteString("- 明确哪些接口可以进入下一步执行任务\n\n")
	builder.WriteString("请重点修复以下任务：\n")
	for _, issue := range issues {
		builder.WriteString(issue.String())
		builder.WriteString("\n")
	}
	return builder.String()
}

func formatPlanQualityIssues(issues []planQualityIssue) string {
	parts := make([]string, 0, len(issues))
	for _, issue := range issues {
		parts = append(parts, issue.String())
	}
	return strings.Join(parts, " | ")
}

func collectPlanQualityIssues(root *AiTask) []planQualityIssue {
	if root == nil {
		return nil
	}
	issues := make([]planQualityIssue, 0)
	order := DFSOrderAiTask(root)
	for index := 0; index < order.Len(); index++ {
		task, ok := order.Get(index)
		if !ok {
			continue
		}
		if task == nil || len(task.Subtasks) > 0 {
			continue
		}
		issue, ok := validateSingleTaskQuality(task)
		if !ok {
			continue
		}
		issues = append(issues, issue)
	}
	return issues
}

func validateSingleTaskQuality(task *AiTask) (planQualityIssue, bool) {
	issue := planQualityIssue{}
	if task == nil {
		return issue, false
	}
	issue.TaskName = task.Name
	issue.TaskGoal = task.Goal
	goal := strings.TrimSpace(task.Goal)
	if goal == "" {
		issue.Reasons = []string{"任务目标为空"}
		return issue, true
	}

	sections := extractGoalSections(goal)
	issue.MaterialItems = extractSectionItems(sections.Materials)
	issue.TargetItems = extractSectionItems(sections.Targets)
	issue.AcceptanceItems = extractSectionItems(sections.Acceptance)

	if len(issue.TargetItems) == 0 {
		issue.Reasons = append(issue.Reasons, "缺少具体待测列表，当前任务没有可直接执行的对象")
	}
	if len(issue.AcceptanceItems) == 0 {
		issue.Reasons = append(issue.Reasons, "缺少具体验收标准，无法判断任务何时完成")
	}
	if len(issue.TargetItems) == 0 && len(issue.MaterialItems) == 0 {
		issue.Reasons = append(issue.Reasons, "缺少可规划材料，应先新增材料收集子任务")
	}
	if containsPlanVaguePhrases(goal) {
		issue.Reasons = append(issue.Reasons, "存在模糊措辞，未写清具体目标或完成条件")
	}
	if hasVagueItems(issue.TargetItems) {
		issue.Reasons = append(issue.Reasons, "待测列表中存在泛化项，未写成具体清单")
	}
	if hasVagueItems(issue.AcceptanceItems) {
		issue.Reasons = append(issue.Reasons, "验收标准中存在空泛项，未写清完成判据")
	}
	if len(issue.TargetItems) > planTaskPreferredTargetCount && !isGroupedTask(task) {
		issue.Reasons = append(issue.Reasons, fmt.Sprintf("待测列表过长(%d项)，应拆成更短的叶子任务", len(issue.TargetItems)))
	}

	return issue, len(issue.Reasons) > 0
}

func splitOversizedLeafTasks(root *AiTask) bool {
	if root == nil {
		return false
	}
	changed := false
	var walk func(task *AiTask)
	walk = func(task *AiTask) {
		if task == nil {
			return
		}
		for index := 0; index < len(task.Subtasks); index++ {
			child := task.Subtasks[index]
			walk(child)
			groupedTasks, ok := buildGroupedTasks(child)
			if !ok {
				continue
			}
			updated := make([]*AiTask, 0, len(task.Subtasks)-1+len(groupedTasks))
			updated = append(updated, task.Subtasks[:index]...)
			updated = append(updated, groupedTasks...)
			updated = append(updated, task.Subtasks[index+1:]...)
			task.Subtasks = updated
			changed = true
			index += len(groupedTasks) - 1
		}
	}
	walk(root)
	return changed
}

func splitLargeInterfaceTasks(root *AiTask) bool {
	return splitOversizedLeafTasks(root)
}

func buildGroupedTasks(task *AiTask) ([]*AiTask, bool) {
	if task == nil || task.ParentTask == nil || task.Coordinator == nil || len(task.Subtasks) > 0 || isGroupedTask(task) {
		return nil, false
	}
	sections := extractGoalSections(task.Goal)
	targetItems := extractSectionItems(sections.Targets)
	if len(targetItems) <= planTaskPreferredTargetCount {
		return nil, false
	}
	acceptanceItems := extractSectionItems(sections.Acceptance)
	if len(acceptanceItems) == 0 {
		acceptanceItems = []string{"本组待测列表中的每个目标都必须有明确执行结果", "若无法完成，记录阻断原因和下一步所需材料", "产出本组任务结论摘要或证据文件"}
	}
	materialItems := extractSectionItems(sections.Materials)

	groupCount := (len(targetItems) + planTaskPreferredTargetCount - 1) / planTaskPreferredTargetCount
	groupedTasks := make([]*AiTask, 0, groupCount)
	for groupIndex := 0; groupIndex < groupCount; groupIndex++ {
		start := groupIndex * planTaskPreferredTargetCount
		end := start + planTaskPreferredTargetCount
		if end > len(targetItems) {
			end = len(targetItems)
		}
		groupTargets := targetItems[start:end]
		name := fmt.Sprintf("%s 第%d组", strings.TrimSpace(task.Name), groupIndex+1)
		goal := buildGroupedTaskGoal(task, materialItems, groupTargets, acceptanceItems, groupIndex+1, groupCount)
		groupTask := task.Coordinator.generateAITaskWithName(name, goal)
		groupTask.ParentTask = task.ParentTask
		if groupIndex == 0 {
			groupTask.DependsOn = append([]string(nil), task.DependsOn...)
		} else {
			groupTask.DependsOn = []string{groupedTasks[groupIndex-1].Name}
		}
		groupedTasks = append(groupedTasks, groupTask)
	}
	return groupedTasks, true
}

func buildGroupedTaskGoal(task *AiTask, materialItems []string, groupTargets []string, acceptanceItems []string, groupIndex int, groupCount int) string {
	var builder strings.Builder
	if len(materialItems) > 0 {
		builder.WriteString("已知材料：\n")
		for _, item := range materialItems {
			builder.WriteString("- ")
			builder.WriteString(item)
			builder.WriteString("\n")
		}
	}
	builder.WriteString(fmt.Sprintf("- 当前分组：第 %d 组 / 共 %d 组\n", groupIndex, groupCount))
	builder.WriteString(fmt.Sprintf("- 原任务名称：%s\n\n", strings.TrimSpace(task.Name)))
	builder.WriteString("待测列表：\n")
	for _, target := range groupTargets {
		builder.WriteString("- ")
		builder.WriteString(target)
		builder.WriteString("\n")
	}
	builder.WriteString("\n验收标准：\n")
	for _, item := range acceptanceItems {
		builder.WriteString("- ")
		builder.WriteString(item)
		builder.WriteString("\n")
	}
	builder.WriteString("- 只允许基于本组待测列表判断当前任务是否完成\n")
	return strings.TrimSpace(builder.String())
}

func isGroupedTask(task *AiTask) bool {
	if task == nil {
		return false
	}
	combined := strings.TrimSpace(task.Name + "\n" + task.Goal)
	return planGroupTaskPattern.MatchString(strings.ToLower(combined))
}

func containsPlanVaguePhrases(goal string) bool {
	vaguePhrases := []string{"等等", "相关接口", "若干接口", "多个参数", "等已知", "等接口", "以及相关", "以及其它", "等目标", "相关模块", "若干目标", "一些接口", "部分接口", "检查是否有问题", "进一步分析", "继续研究", "输出结果即可"}
	for _, phrase := range vaguePhrases {
		if strings.Contains(goal, phrase) {
			return true
		}
	}
	if strings.Contains(goal, "等") {
		if strings.Contains(goal, "接口等") || strings.Contains(goal, "参数等") || strings.Contains(goal, "目标等") || strings.Contains(goal, "模块等") || strings.Contains(goal, "文件等") {
			return true
		}
	}
	return false
}

func hasVagueItems(items []string) bool {
	for _, item := range items {
		if containsPlanVaguePhrases(item) {
			return true
		}
	}
	return false
}

func extractGoalSections(goal string) taskGoalSections {
	normalized := strings.ReplaceAll(goal, "\r\n", "\n")
	sections := []struct {
		name   string
		labels []string
	}{
		{name: "materials", labels: planMaterialSectionLabels},
		{name: "targets", labels: planTargetSectionLabels},
		{name: "acceptance", labels: planAcceptanceSectionLabels},
	}
	type foundSection struct {
		name  string
		index int
		label string
	}
	found := make([]foundSection, 0, len(sections))
	for _, section := range sections {
		index, label := findFirstSectionLabel(normalized, section.labels)
		if index >= 0 {
			found = append(found, foundSection{name: section.name, index: index, label: label})
		}
	}
	sort.Slice(found, func(i, j int) bool {
		return found[i].index < found[j].index
	})
	result := taskGoalSections{}
	for index, section := range found {
		start := section.index + len(section.label)
		end := len(normalized)
		if index+1 < len(found) {
			end = found[index+1].index
		}
		content := strings.TrimSpace(normalized[start:end])
		switch section.name {
		case "materials":
			result.Materials = content
		case "targets":
			result.Targets = content
		case "acceptance":
			result.Acceptance = content
		}
	}
	return result
}

func findFirstSectionLabel(goal string, labels []string) (int, string) {
	matchedIndex := -1
	matchedLabel := ""
	for _, label := range labels {
		index := strings.Index(strings.ToLower(goal), strings.ToLower(label))
		if index < 0 {
			continue
		}
		if matchedIndex == -1 || index < matchedIndex {
			matchedIndex = index
			matchedLabel = goal[index : index+len(label)]
		}
	}
	return matchedIndex, matchedLabel
}

func extractSectionItems(section string) []string {
	trimmed := strings.TrimSpace(section)
	if trimmed == "" {
		return nil
	}
	seen := make(map[string]struct{})
	items := make([]string, 0)
	for _, line := range strings.Split(trimmed, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		line = planBulletPrefixPattern.ReplaceAllString(line, "")
		line = strings.TrimSpace(strings.TrimRight(line, ".,，;；"))
		if line == "" {
			continue
		}
		if _, ok := seen[line]; ok {
			continue
		}
		seen[line] = struct{}{}
		items = append(items, line)
	}
	if len(items) > 0 {
		return items
	}
	return []string{trimmed}
}

func extractInterfaceTargets(text string) []string {
	sections := extractGoalSections(text)
	return extractSectionItems(sections.Targets)
}

func (pr *planRequest) collectInventoryCoverageGaps(root *AiTask) []string {
	if pr == nil || pr.cod == nil || pr.cod.ContextProvider == nil || root == nil {
		return nil
	}
	targets := pr.cod.ContextProvider.TaskOutputInventoryTargets()
	if len(targets) == 0 {
		return nil
	}
	corpus := collectTaskCoverageCorpus(root)
	missing := make([]string, 0)
	for _, target := range targets {
		if inventoryTargetCovered(target, corpus) {
			continue
		}
		missing = append(missing, target)
	}
	return dedupeNonEmptyStrings(missing)
}

func collectTaskCoverageCorpus(root *AiTask) []string {
	if root == nil {
		return nil
	}
	order := DFSOrderAiTask(root)
	corpus := make([]string, 0, order.Len())
	for index := 0; index < order.Len(); index++ {
		task, ok := order.Get(index)
		if !ok || task == nil {
			continue
		}
		combined := strings.ToLower(strings.TrimSpace(task.Name + "\n" + task.Goal))
		if combined != "" {
			corpus = append(corpus, combined)
		}
	}
	return corpus
}

func inventoryTargetCovered(target string, corpus []string) bool {
	aliases := inventoryTargetAliases(target)
	if len(aliases) == 0 {
		return true
	}
	for _, text := range corpus {
		for _, alias := range aliases {
			if alias != "" && strings.Contains(text, alias) {
				return true
			}
		}
	}
	return false
}

func appendInventoryCoverageTasks(root *AiTask, missingTargets []string) *AiTask {
	if root == nil || root.Coordinator == nil {
		return root
	}
	missingTargets = dedupeNonEmptyStrings(missingTargets)
	if len(missingTargets) == 0 {
		return root
	}
	groupCount := (len(missingTargets) + planTaskPreferredTargetCount - 1) / planTaskPreferredTargetCount
	var previousName string
	if len(root.Subtasks) > 0 {
		previousName = root.Subtasks[len(root.Subtasks)-1].Name
	}
	for groupIndex := 0; groupIndex < groupCount; groupIndex++ {
		start := groupIndex * planTaskPreferredTargetCount
		end := start + planTaskPreferredTargetCount
		if end > len(missingTargets) {
			end = len(missingTargets)
		}
		groupTargets := missingTargets[start:end]
		name := fmt.Sprintf("基于库存补齐剩余目标 第%d组", groupIndex+1)
		goal := buildInventoryCoverageGoal(groupTargets, groupIndex+1, groupCount)
		task := root.Coordinator.generateAITaskWithName(name, goal)
		task.ParentTask = root
		if previousName != "" {
			task.DependsOn = []string{previousName}
		}
		root.Subtasks = append(root.Subtasks, task)
		previousName = task.Name
	}
	return root
}

func buildInventoryCoverageGoal(targets []string, groupIndex int, groupCount int) string {
	var builder strings.Builder
	builder.WriteString("已知材料：\n")
	builder.WriteString(fmt.Sprintf("- 来源常量：%s\n", taskOutputInventoryPersistentKey))
	builder.WriteString("- 这些目标来自前序真实任务产物，不允许在规划时丢弃\n")
	builder.WriteString(fmt.Sprintf("- 当前分组：第 %d 组 / 共 %d 组\n\n", groupIndex, groupCount))
	builder.WriteString("待测列表：\n")
	for _, target := range targets {
		builder.WriteString("- ")
		builder.WriteString(target)
		builder.WriteString("\n")
	}
	builder.WriteString("\n验收标准：\n")
	builder.WriteString("- 为本组每个目标明确安排后续验证路径，或记录无法继续的阻断原因\n")
	builder.WriteString("- 不允许遗漏本组任何目标，不允许用等或剩余接口代替具体清单\n")
	builder.WriteString("- 为每个目标产出至少一条可追溯结论或证据引用\n")
	return strings.TrimSpace(builder.String())
}
