package loop_plan

import (
	"fmt"
	"io"
	"strings"
	"sync"
	"unicode"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/aitag"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

const (
	PlanFactsFieldName = "facts"
	PlanFactsAITagName = "FACTS"
	PlanFactsAINodeID  = "plan-facts"

	planFactsGeneralSection = "## 通用事实"
)

type factsSection struct {
	Title string
	Lines []string
	seen  map[string]struct{}
}

func normalizeFactsDocument(content string) string {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	return strings.TrimSpace(content)
}

func extractEvidenceDocument(content string) string {
	content = strings.TrimSpace(content)
	if content == "" {
		return ""
	}
	blocks := discoverEvidenceAITagBlocks(content, "EVIDENCE", "PLAN_EVIDENCE")
	if len(blocks) == 0 {
		return ""
	}

	results := make([]string, len(blocks))
	options := make([]aitag.ParseOption, 0, len(blocks))
	var mu sync.Mutex
	for index, block := range blocks {
		index := index
		block := block
		options = append(options, aitag.WithCallback(block.TagName, block.Nonce, func(reader io.Reader) {
			contentBytes, err := io.ReadAll(reader)
			if err != nil {
				return
			}
			mu.Lock()
			results[index] = strings.TrimSpace(string(contentBytes))
			mu.Unlock()
		}))
	}
	if err := aitag.Parse(strings.NewReader(content), options...); err != nil {
		return ""
	}
	for _, result := range results {
		if result != "" {
			return result
		}
	}
	return ""
}

type discoveredEvidenceAITagBlock struct {
	TagName string
	Nonce   string
}

func discoverEvidenceAITagBlocks(content string, tagNames ...string) []discoveredEvidenceAITagBlock {
	if content == "" || len(tagNames) == 0 {
		return nil
	}
	allowedTags := make(map[string]struct{}, len(tagNames))
	for _, tagName := range tagNames {
		if tagName == "" {
			continue
		}
		allowedTags[tagName] = struct{}{}
	}

	blocks := make([]discoveredEvidenceAITagBlock, 0, 2)
	for offset := 0; offset < len(content); {
		startOffset := strings.Index(content[offset:], "<|")
		if startOffset < 0 {
			break
		}
		start := offset + startOffset
		tagCloseOffset := strings.Index(content[start:], "|>")
		if tagCloseOffset < 0 {
			break
		}
		tagClose := start + tagCloseOffset + 2
		tagName, nonce, ok := parseEvidenceAITagStartToken(content[start+2 : tagClose-2])
		if !ok {
			offset = tagClose
			continue
		}
		if _, exists := allowedTags[tagName]; !exists {
			offset = tagClose
			continue
		}
		endTag := fmt.Sprintf("<|%s_END_%s|>", tagName, nonce)
		if endOffset := strings.Index(content[tagClose:], endTag); endOffset >= 0 {
			blocks = append(blocks, discoveredEvidenceAITagBlock{TagName: tagName, Nonce: nonce})
			offset = tagClose + endOffset + len(endTag)
			continue
		}
		offset = tagClose
	}
	return blocks
}

func parseEvidenceAITagStartToken(token string) (string, string, bool) {
	if token == "" || strings.Contains(token, "_END_") {
		return "", "", false
	}
	underscore := strings.LastIndex(token, "_")
	if underscore <= 0 || underscore >= len(token)-1 {
		return "", "", false
	}
	tagName := token[:underscore]
	nonce := token[underscore+1:]
	for _, ch := range tagName {
		if !unicode.IsLetter(ch) && !unicode.IsDigit(ch) && ch != '_' {
			return "", "", false
		}
	}
	return tagName, nonce, true
}

func getLoopTaskEvidenceDocument(loop *reactloops.ReActLoop) string {
	if loop == nil {
		return ""
	}
	if evidence := strings.TrimSpace(loop.Get(PLAN_EVIDENCE_KEY)); evidence != "" {
		return evidence
	}
	task := loop.GetCurrentTask()
	if task == nil {
		return ""
	}
	return extractEvidenceDocument(task.GetUserInput())
}

func parseFactsSections(content string) []*factsSection {
	content = normalizeFactsDocument(content)
	if content == "" {
		return nil
	}

	sections := make([]*factsSection, 0)
	sectionMap := make(map[string]*factsSection)
	currentTitle := ""

	getSection := func(title string) *factsSection {
		if title == "" {
			title = planFactsGeneralSection
		}
		if sec, ok := sectionMap[title]; ok {
			return sec
		}
		sec := &factsSection{
			Title: title,
			Lines: make([]string, 0),
			seen:  make(map[string]struct{}),
		}
		sectionMap[title] = sec
		sections = append(sections, sec)
		return sec
	}

	for _, rawLine := range strings.Split(content, "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "## ") {
			currentTitle = line
			getSection(currentTitle)
			continue
		}
		sec := getSection(currentTitle)
		if _, exists := sec.seen[line]; exists {
			continue
		}
		sec.seen[line] = struct{}{}
		sec.Lines = append(sec.Lines, line)
	}

	filtered := make([]*factsSection, 0, len(sections))
	for _, sec := range sections {
		if len(sec.Lines) == 0 {
			continue
		}
		filtered = append(filtered, sec)
	}
	return filtered
}

func mergeFactsDocuments(existing, incoming string) string {
	sections := parseFactsSections(existing)
	sectionMap := make(map[string]*factsSection, len(sections))
	for _, sec := range sections {
		sectionMap[sec.Title] = sec
	}

	for _, inc := range parseFactsSections(incoming) {
		target, ok := sectionMap[inc.Title]
		if !ok {
			target = &factsSection{
				Title: inc.Title,
				Lines: make([]string, 0, len(inc.Lines)),
				seen:  make(map[string]struct{}, len(inc.Lines)),
			}
			sections = append(sections, target)
			sectionMap[inc.Title] = target
		}
		for _, line := range inc.Lines {
			if _, exists := target.seen[line]; exists {
				continue
			}
			target.seen[line] = struct{}{}
			target.Lines = append(target.Lines, line)
		}
	}

	var blocks []string
	for _, sec := range sections {
		if len(sec.Lines) == 0 {
			continue
		}
		blocks = append(blocks, sec.Title+"\n\n"+strings.Join(sec.Lines, "\n"))
	}
	return strings.TrimSpace(strings.Join(blocks, "\n\n"))
}

func appendPlanFacts(loop *reactloops.ReActLoop, incoming string) (string, bool) {
	incoming = normalizeFactsDocument(incoming)
	if incoming == "" {
		return loop.Get(PLAN_FACTS_KEY), false
	}

	existing := loop.Get(PLAN_FACTS_KEY)
	merged := mergeFactsDocuments(existing, incoming)
	if merged == normalizeFactsDocument(existing) {
		return merged, false
	}
	loop.Set(PLAN_FACTS_KEY, merged)
	return merged, true
}

func emitFactsMarkdown(loop *reactloops.ReActLoop, facts string) {
	facts = normalizeFactsDocument(facts)
	if facts == "" {
		return
	}

	taskIndex := ""
	if task := loop.GetCurrentTask(); task != nil {
		taskIndex = task.GetId()
	}
	if emitter := loop.GetEmitter(); emitter != nil {
		if _, err := emitter.EmitTextMarkdownStreamEvent(PlanFactsAINodeID, strings.NewReader(facts), taskIndex, func() {}); err != nil {
			log.Warnf("plan loop: emit facts markdown failed: %v", err)
		}
	}
}

const contextTokenBudget = 15000

func getLoopTaskContext(loop *reactloops.ReActLoop) string {
	type contextEntry struct {
		Title    string
		Key      string
		Value    string
		Priority int // lower = higher priority
	}

	entries := []contextEntry{
		{Title: "已有事实", Key: "facts", Value: loop.Get(PLAN_FACTS_KEY), Priority: 0},
		{Title: "侦查结果", Key: "recon", Value: loop.Get(PLAN_RECON_RESULTS_KEY), Priority: 1},
		{Title: "文件结果", Key: "file", Value: loop.Get(PLAN_FILE_RESULTS_KEY), Priority: 2},
		{Title: "已有计划", Key: "plan", Value: loop.Get(PLAN_DATA_KEY), Priority: 3},
		{Title: "补充知识", Key: "enhance", Value: loop.Get(PLAN_ENHANCE_KEY), Priority: 4},
		{Title: "互联网结果", Key: "web", Value: loop.Get(PLAN_WEB_RESULTS_KEY), Priority: 5},
	}

	var parts []string
	usedTokens := 0

	for _, entry := range entries {
		value := strings.TrimSpace(entry.Value)
		if value == "" {
			continue
		}

		sectionHeader := fmt.Sprintf("## %s\n", entry.Title)
		headerTokens := aicommon.MeasureTokens(sectionHeader)
		remaining := contextTokenBudget - usedTokens - headerTokens
		if remaining <= 100 {
			break
		}

		valueTokens := aicommon.MeasureTokens(value)
		if valueTokens <= remaining {
			part := sectionHeader + value
			parts = append(parts, part)
			usedTokens += headerTokens + valueTokens
		} else {
			artifactPath := saveContextToArtifact(loop, entry.Key, entry.Title, value)
			summary := aicommon.ShrinkTextBlockByTokens(value, remaining-100)
			var part string
			if artifactPath != "" {
				part = fmt.Sprintf("%s%s\n\n> 以上为摘要，完整内容(%d tokens)已保存至: %s，可通过 read_file 查看", sectionHeader, summary, valueTokens, artifactPath)
			} else {
				part = sectionHeader + summary
			}
			partTokens := aicommon.MeasureTokens(part)
			parts = append(parts, part)
			usedTokens += partTokens
		}
	}
	return strings.Join(parts, "\n\n")
}

func saveContextToArtifact(loop *reactloops.ReActLoop, key string, title string, content string) string {
	invoker := loop.GetInvoker()
	if invoker == nil {
		return ""
	}
	filename := fmt.Sprintf("plan_context_%s", key)
	fullContent := fmt.Sprintf("# %s\n\n%s", title, content)
	path := invoker.EmitFileArtifactWithExt(filename, ".md", fullContent)
	if path != "" {
		log.Infof("plan loop: context section %q saved to artifact: %s", title, path)
	}
	return path
}

func autoGenerateFacts(loop *reactloops.ReActLoop, task aicommon.AIStatefulTask, mode string, lastAction *reactloops.ActionRecord) string {
	invoker := loop.GetInvoker()
	ctx := invoker.GetConfig().GetContext()
	if task != nil && !utils.IsNil(task.GetContext()) {
		ctx = task.GetContext()
	}

	userInput := ""
	if task != nil {
		userInput = task.GetUserInput()
	}

	lastActionText := ""
	if lastAction != nil {
		lastActionText = fmt.Sprintf("ActionType: %s\nActionName: %s\nActionParams: %s", lastAction.ActionType, lastAction.ActionName, utils.InterfaceToString(lastAction.ActionParams))
	}

	instruction := "基于最新执行结果，只输出本轮新增且可验证的事实，不能重复已有事实。"
	if mode == "bootstrap" {
		instruction = "当前还没有任何已记录 facts。请基于用户原始诉求、已有 plan 和已收集结果，整理出一份初始事实文档。只能写明确存在于上下文里的事实，禁止编造。"
	}

	prompt := fmt.Sprintf(`你正在为任务规划循环生成 FACTS 文档。

要求：
- 输出 Markdown
- 使用 ## 标题组织内容
- 每条事实单独一行，优先使用 bullet point
- 只允许具体、可验证的信息
- **语言跟随用户输入**：section 标题、说明语、bullet 描述的语言必须与下文"用户输入"保持一致。用户用中文就全部写中文，用户用英文就全部写英文。严禁把中文场景的事实写成 "URL discovered: ..."、"Open port discovered: ..."、"Server response header ..."、"Static file referenced: ..." 之类的英文动词短语开头的 bullet——这既是语言错位，也是下面要禁止的"重复前缀"
- **信息密度（去重复前缀）**：同类事实集中到同一个 ## section，由 section 标题承担分类语义；bullet 里只写差异值本身，禁止在每条 bullet 里复述相同的动词短语 / 名词前缀。公共前缀（同一域名、同一 base URL、同一根目录）抽到 section 标题括号里或 section 开头一行引导语里，bullet 只写相对部分
  - 反例（密度差，禁止）：
    - URL discovered: http://127.0.0.1:8787/_/
    - URL discovered: http://127.0.0.1:8787/_/submit-ai-practice
    - URL discovered: http://127.0.0.1:8787/api/
    - Server response header Content-Type: text/html
    - Static CSS file referenced: /static/js/bootstrap.min.css
  - 正例（密度高，必须）：
    ## 已发现 URL (base: http://127.0.0.1:8787)
    - /_/
    - /_/submit-ai-practice
    - /api/
    ## HTTP 响应头
    - Content-Type: text/html
    ## 静态资源引用
    - /static/js/bootstrap.min.css
- **N 对 N 硬量化**：信息源里出现了 N 条同类条目（目录、URL、文件、端口、接口、参数等），FACTS 中就必须写出 N 行 bullet，一条都不能合并、一条都不能省略、一条都不能概括；来源有 10 个 URL 就写 10 行，有 155 个 URL 就写 155 行。信息密度规则指的是"bullet 不重复前缀"，不是"压缩 bullet 数量"
- **严禁使用任何概括词**：不允许出现 "等"、"其他"、"相关"、"类似"、"若干"、"一些"、"多个"、"若干个"、"数个"、"部分"、"主要"，以及英文等价表达（etc. / and more / and so on / various / several / others / similar / including…）；一旦出现任一词汇本次输出视为不合格
- 反例（严禁）：已确认存在 /_/, /api, /bruteplayground, /crypto 等目录 ； /fastjson/json-in-cookie 等 FastJSON 端点
- 正例（必须）：为每个目录 / URL / 端点各写一行独立 bullet，把本应被"等"吞掉的条目全部展开
- 如果担心单次输出过长，优先选择"只在本轮新增事实"这一点让输出缩短，**禁止用概括词压缩条目数**
- 如果没有新增事实，返回空字符串

本次模式：%s
%s

## 用户输入
%s

## 最近 action
%s

## 当前上下文
%s
`, mode, instruction, userInput, lastActionText, getLoopTaskContext(loop))

	taskIndex := ""
	if task != nil {
		taskIndex = task.GetId()
	}

	action, err := invoker.InvokeSpeedPriorityLiteForge(
		ctx,
		"plan_facts_hook",
		prompt,
		[]aitool.ToolOption{
			aitool.WithStringParam(PlanFactsFieldName, aitool.WithParam_Description("增量 facts markdown；没有新增事实时返回空字符串")),
		},
		aicommon.WithGeneralConfigStreamableFieldEmitterCallback(
			[]string{PlanFactsFieldName},
			func(key string, r io.Reader, emitter *aicommon.Emitter) {
				r = utils.JSONStringReader(r)
				if emitter == nil {
					io.Copy(io.Discard, r)
					return
				}
				emitter.EmitTextMarkdownStreamEvent(PlanFactsAINodeID, r, taskIndex)
			},
		),
	)
	if err != nil {
		log.Warnf("plan loop: auto generate facts failed: %v", err)
		return ""
	}
	if action == nil {
		return ""
	}
	return normalizeFactsDocument(action.GetString(PlanFactsFieldName))
}

func hasValidPlan(loop *reactloops.ReActLoop) bool {
	planData := strings.TrimSpace(loop.Get(PLAN_DATA_KEY))
	if planData == "" {
		return false
	}
	action, err := aicommon.ExtractAction(planData, "plan", "plan")
	if err != nil {
		return false
	}
	return action.GetString("main_task") != "" && action.GetString("main_task_goal") != "" && len(action.GetInvokeParamsArray("tasks")) > 0
}

func shouldAutoFactsForAction(actionType string) bool {
	switch actionType {
	case "", "output_facts":
		return false
	default:
		return true
	}
}

func isMaxIterationReason(reason any) bool {
	if reason == nil {
		return false
	}
	if err, ok := reason.(error); ok {
		return strings.Contains(err.Error(), "max iterations")
	}
	return strings.Contains(utils.InterfaceToString(reason), "max iterations")
}

func buildPlanPostIterationHook(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	_ = r
	return reactloops.WithOnPostIteraction(func(loop *reactloops.ReActLoop, iteration int, task aicommon.AIStatefulTask, isDone bool, reason any, operator *reactloops.OnPostIterationOperator) {
		lastAction := loop.GetLastAction()
		if lastAction != nil && shouldAutoFactsForAction(lastAction.ActionType) {
			incoming := ""
			if lastAction.ActionParams != nil {
				incoming = normalizeFactsDocument(utils.InterfaceToString(lastAction.ActionParams[PlanFactsFieldName]))
			}
			if incoming == "" {
				incoming = autoGenerateFacts(loop, task, "incremental", lastAction)
			}
			if _, changed := appendPlanFacts(loop, incoming); changed {
				log.Infof("plan loop: post-action facts hook merged facts after action=%s iteration=%d", lastAction.ActionType, iteration)
			}
		}

		if !isDone {
			return
		}

		if strings.TrimSpace(loop.Get(PLAN_FACTS_KEY)) == "" {
			bootstrapFacts := autoGenerateFacts(loop, task, "bootstrap", lastAction)
			if _, changed := appendPlanFacts(loop, bootstrapFacts); changed {
				log.Infof("plan loop: generated bootstrap facts at finalization")
			}
		}

		document := generateGuidanceDocument(loop, task)
		if document != "" {
			loop.Set(PLAN_DOCUMENT_KEY, document)
			log.Infof("plan loop: generated guidance document at finalization")
		}

		if !hasValidPlan(loop) {
			planData := generatePlanFromDocument(loop, task)
			if planData != "" {
				loop.Set(PLAN_DATA_KEY, planData)
				log.Infof("plan loop: generated plan from document at finalization")
			}
		}

		if isMaxIterationReason(reason) && hasValidPlan(loop) {
			operator.IgnoreError()
		}
	})
}
