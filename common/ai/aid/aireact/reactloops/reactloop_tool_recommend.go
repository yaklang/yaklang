package reactloops

import (
	"strings"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

// ToolRecommender 工具和 forge 推荐管理器
type ToolRecommender struct {
	// 缓存的工具和 forge 列表
	cachedToolsList  []*aitool.Tool
	cachedForgesList []*schema.AIForge
	cachedLoopsList  []*LoopMetadata

	// 互斥锁保护缓存
	cachedToolsListMutex  *sync.Mutex
	cachedForgesListMutex *sync.Mutex
	cachedLoopsListMutex  *sync.Mutex

	// 异步任务控制
	recommendTaskSizeWaitGroup *utils.SizedWaitGroup

	// 关联的 AIInvokeRuntime（用于获取配置和任务信息）
	invoker aicommon.AIInvokeRuntime

	// 限制配置
	maxToolsLimit  int
	maxForgesLimit int
	maxLoopsLimit  int
}

// NewToolRecommender 创建新的工具推荐管理器
// 初始化时会使用 limit 查询填充缓存默认值
func NewToolRecommender(invoker aicommon.AIInvokeRuntime) *ToolRecommender {
	tr := &ToolRecommender{
		cachedToolsListMutex:       new(sync.Mutex),
		cachedForgesListMutex:      new(sync.Mutex),
		cachedLoopsListMutex:       new(sync.Mutex),
		recommendTaskSizeWaitGroup: utils.NewSizedWaitGroup(10),
		invoker:                    invoker,
		maxToolsLimit:              30,  // 默认限制工具数量为 30 个
		maxForgesLimit:             200, // 默认限制 forge 数量为 200 个
		maxLoopsLimit:              10,  // 默认限制 loop 数量为 10 个
	}

	// 使用 limit 查询初始化缓存默认值
	if tools := tr.getAllAvailableTools(); len(tools) > 0 {
		tr.cachedToolsList = tr.prioritizeAndLimitTools(tools, tr.maxToolsLimit)
	} else {
		tr.cachedToolsList = make([]*aitool.Tool, 0)
	}

	if forges := tr.getAllAvailableForges(); len(forges) > 0 {
		tr.cachedForgesList = tr.limitForges(forges, tr.maxForgesLimit)
	} else {
		tr.cachedForgesList = make([]*schema.AIForge, 0)
	}

	if loops := tr.getAllAvailableLoops(); len(loops) > 0 {
		tr.cachedLoopsList = loops
	} else {
		tr.cachedLoopsList = make([]*LoopMetadata, 0)
	}

	return tr
}

// GetRecommendedToolsAndForges 根据用户输入通过关键词匹配获取推荐的 tools 和 forges
// 使用默认的限制值
func (tr *ToolRecommender) GetRecommendedToolsAndForges(userInput string, config aicommon.AICallerConfigIf) ([]*aitool.Tool, []*schema.AIForge) {
	return tr.GetRecommendedToolsAndForgesWithLimits(userInput, config, tr.maxToolsLimit, tr.maxForgesLimit)
}

// GetRecommendedToolsAndForgesWithLimits 根据用户输入通过关键词匹配获取推荐的 tools 和 forges
// 支持自定义限制值
func (tr *ToolRecommender) GetRecommendedToolsAndForgesWithLimits(userInput string, config aicommon.AICallerConfigIf, maxToolsLimit, maxForgesLimit int) ([]*aitool.Tool, []*schema.AIForge) {
	tr.cachedToolsListMutex.Lock()
	defer tr.cachedToolsListMutex.Unlock()

	tr.cachedForgesListMutex.Lock()
	defer tr.cachedForgesListMutex.Unlock()

	// 获取所有可用工具
	var allTools []*aitool.Tool
	if mgr := config.GetAiToolManager(); mgr != nil {
		allTools, _ = mgr.GetEnableTools()
	}

	// 获取所有可用 forges
	var allForges []*schema.AIForge
	if cfg, ok := config.(*aicommon.Config); ok {
		allForges = append(allForges, cfg.ExtendedForge...)
		if mgr := cfg.GetAIForgeManager(); mgr != nil {
			if forges, err := mgr.Query(config.GetContext()); err == nil {
				allForges = append(allForges, forges...)
			} else {
				log.Warnf("failed to query forges: %v", err)
			}
		}
	}

	// 如果缓存的工具列表为空，使用全部工具初始化
	if len(tr.cachedToolsList) == 0 {
		tr.cachedToolsList = tr.prioritizeAndLimitTools(allTools, maxToolsLimit)
	}

	// 如果缓存的 forges 列表为空，使用全部 forges 初始化
	if len(tr.cachedForgesList) == 0 {
		tr.cachedForgesList = tr.limitForges(allForges, maxForgesLimit)
	}

	// 如果没有用户输入，返回缓存的列表
	if userInput == "" {
		return tr.prioritizeAndLimitTools(allTools, maxToolsLimit), tr.limitForges(allForges, maxForgesLimit)
	}

	queryLower := strings.ToLower(userInput)

	// ============ 使用关键词匹配工具 ============
	matchedTools := keywordMatchTools(allTools, queryLower, maxToolsLimit)

	var recommendedTools []*aitool.Tool
	if len(matchedTools) > 0 {
		matchedPrioritized := tr.prioritizeAndLimitTools(matchedTools, maxToolsLimit)
		recommendedTools = append(recommendedTools, matchedPrioritized...)

		matchedToolsMap := make(map[string]bool)
		for _, t := range matchedTools {
			matchedToolsMap[t.Name] = true
		}
		for _, cachedTool := range tr.cachedToolsList {
			if len(recommendedTools) >= maxToolsLimit {
				break
			}
			if !matchedToolsMap[cachedTool.Name] {
				recommendedTools = append(recommendedTools, cachedTool)
			}
		}
		tr.cachedToolsList = recommendedTools
	} else {
		recommendedTools = tr.cachedToolsList
	}

	// ============ 使用关键词匹配 forges ============
	matchedForges := keywordMatchForges(allForges, queryLower, maxForgesLimit)

	var recommendedForges []*schema.AIForge
	if len(matchedForges) > 0 {
		recommendedForges = append(recommendedForges, tr.limitForges(matchedForges, maxForgesLimit)...)

		matchedForgesMap := make(map[string]bool)
		for _, f := range matchedForges {
			matchedForgesMap[f.ForgeName] = true
		}
		for _, cachedForge := range tr.cachedForgesList {
			if len(recommendedForges) >= maxForgesLimit {
				break
			}
			if !matchedForgesMap[cachedForge.ForgeName] {
				recommendedForges = append(recommendedForges, cachedForge)
			}
		}
		tr.cachedForgesList = recommendedForges
	} else {
		recommendedForges = tr.cachedForgesList
	}

	return recommendedTools, recommendedForges
}

// GetRecommendedToolsAndForgesAsync 异步搜索推荐的 tools 和 forges
// 使用 AiToolManager 进行异步搜索，搜索成功后更新缓存
// onFinished: 可选的回调函数，在搜索完成后调用
func (tr *ToolRecommender) GetRecommendedToolsAndForgesAsync(userInput string, config aicommon.AICallerConfigIf, onFinished ...func()) {
	tr.recommendTaskSizeWaitGroup.Add(1)
	go func() {
		defer tr.recommendTaskSizeWaitGroup.Done()
		defer func() {
			if err := recover(); err != nil {
				log.Errorf("panic occurred during async tools and forges search: %v", err)
			}
		}()
		defer func() {
			// 调用完成回调
			for _, callback := range onFinished {
				if callback != nil {
					callback()
				}
			}
		}()

		// 如果没有用户输入，不执行搜索
		if userInput == "" {
			return
		}

		// 获取 AiToolManager
		cfg, ok := config.(*aicommon.Config)
		if !ok {
			return
		}
		toolMgr := cfg.GetAiToolManager()
		if toolMgr == nil {
			log.Debug("AiToolManager is not set, cannot perform async search")
			return
		}

		var searchedTools []*aitool.Tool

		// ============ 搜索工具 ============
		tools, err := toolMgr.SearchTools("keyword", userInput)
		if err != nil {
			log.Errorf("failed to search tools asynchronously: %v", err)
		} else if len(tools) > 0 {
			searchedTools = tools
			log.Infof("found %d relevant tools via async search", len(tools))
		}

		// ============ 更新缓存 ============
		if len(searchedTools) > 0 {
			tr.updateCacheWithMatchedItems(searchedTools, nil, nil)
		}
	}()
}

// WaitRecommendTask 等待所有推荐任务完成
func (tr *ToolRecommender) WaitRecommendTask() {
	tr.recommendTaskSizeWaitGroup.Wait()
}

// GetCachedToolsAndForges 获取缓存的工具、forges 和 loops（需要加锁）
func (tr *ToolRecommender) GetCachedToolsAndForges() ([]*aitool.Tool, []*schema.AIForge, []*LoopMetadata) {
	tr.cachedToolsListMutex.Lock()
	tools := tr.cachedToolsList
	tr.cachedToolsListMutex.Unlock()

	tr.cachedForgesListMutex.Lock()
	forges := tr.cachedForgesList
	tr.cachedForgesListMutex.Unlock()

	tr.cachedLoopsListMutex.Lock()
	loops := tr.cachedLoopsList
	tr.cachedLoopsListMutex.Unlock()

	return tools, forges, loops
}

// WaitForAsyncSearchWithTimeout 等待异步搜索完成，最多等待指定时间
// 返回 true 表示在超时前完成，false 表示超时
func (tr *ToolRecommender) WaitForAsyncSearchWithTimeout(userInput string, config aicommon.AICallerConfigIf, timeout time.Duration) bool {
	done := make(chan struct{}, 1)
	tr.GetRecommendedToolsAndForgesAsync(userInput, config, func() {
		select {
		case done <- struct{}{}:
		default:
		}
	})

	select {
	case <-done:
		return true
	case <-time.After(timeout):
		return false
	}
}

// prioritizeAndLimitTools 对工具进行优先级排序并限制数量
func (tr *ToolRecommender) prioritizeAndLimitTools(tools []*aitool.Tool, maxCount int) []*aitool.Tool {
	if len(tools) == 0 {
		return tools
	}

	// 优先工具列表（与 aireact/tools.go 中的保持一致）
	priorityNames := []string{
		"tools_search",
		"aiforge_search",
		"now",
		"bash",
		"read_file",
		"ls",
		"grep",
		"find_file",
		"send_http_request_by_url",
		"whois",
		"dig",
		"scan_tcp_port",
		"encode",
		"decode",
		"auto_decode",
		"current_time",
		"echo",
	}

	// 创建工具映射表
	toolMap := make(map[string]*aitool.Tool)
	for _, tool := range tools {
		toolMap[tool.Name] = tool
	}

	var result []*aitool.Tool
	usedNames := make(map[string]bool)

	// 首先添加优先工具
	for _, name := range priorityNames {
		if len(result) >= maxCount {
			break
		}
		if tool, exists := toolMap[name]; exists {
			result = append(result, tool)
			usedNames[name] = true
		}
	}

	// 然后添加剩余的工具，直到达到限制
	for _, tool := range tools {
		if len(result) >= maxCount {
			break
		}
		if !usedNames[tool.Name] {
			result = append(result, tool)
			usedNames[tool.Name] = true
		}
	}

	return result
}

// limitForges 限制 forges 的数量
func (tr *ToolRecommender) limitForges(forges []*schema.AIForge, maxCount int) []*schema.AIForge {
	if len(forges) <= maxCount {
		return forges
	}
	return forges[:maxCount]
}

// keywordMatchTools 使用关键词匹配工具（name、verboseName、keywords 子串匹配）
func keywordMatchTools(allTools []*aitool.Tool, queryLower string, limit int) []*aitool.Tool {
	var matched []*aitool.Tool
	for _, tool := range allTools {
		if utils.MatchAnyOfSubString(queryLower, strings.ToLower(tool.Name)) {
			matched = append(matched, tool)
		} else if tool.VerboseName != "" && utils.MatchAnyOfSubString(queryLower, strings.ToLower(tool.VerboseName)) {
			matched = append(matched, tool)
		} else {
			for _, keyword := range tool.Keywords {
				if keyword != "" && utils.MatchAnyOfSubString(queryLower, strings.ToLower(keyword)) {
					matched = append(matched, tool)
					break
				}
			}
		}
		if limit > 0 && len(matched) >= limit {
			break
		}
	}
	return matched
}

// keywordMatchForges 使用关键词匹配 forges（forgeName、verboseName、keywords 子串匹配）
func keywordMatchForges(allForges []*schema.AIForge, queryLower string, limit int) []*schema.AIForge {
	var matched []*schema.AIForge
	for _, forge := range allForges {
		if utils.MatchAnyOfSubString(queryLower, strings.ToLower(forge.ForgeName)) {
			matched = append(matched, forge)
		} else if forge.ForgeVerboseName != "" && utils.MatchAnyOfSubString(queryLower, strings.ToLower(forge.ForgeVerboseName)) {
			matched = append(matched, forge)
		} else {
			for _, keyword := range forge.GetKeywords() {
				if keyword != "" && utils.MatchAnyOfSubString(queryLower, strings.ToLower(keyword)) {
					matched = append(matched, forge)
					break
				}
			}
		}
		if limit > 0 && len(matched) >= limit {
			break
		}
	}
	return matched
}

// mergeAndDeduplicateTools 合并并去重工具列表，primary 优先
func mergeAndDeduplicateTools(primary, secondary []*aitool.Tool, limit int) []*aitool.Tool {
	seen := make(map[string]bool)
	var result []*aitool.Tool
	for _, t := range primary {
		if !seen[t.Name] {
			seen[t.Name] = true
			result = append(result, t)
		}
	}
	for _, t := range secondary {
		if limit > 0 && len(result) >= limit {
			break
		}
		if !seen[t.Name] {
			seen[t.Name] = true
			result = append(result, t)
		}
	}
	return result
}

// mergeAndDeduplicateForges 合并并去重 forge 列表，primary 优先
func mergeAndDeduplicateForges(primary, secondary []*schema.AIForge, limit int) []*schema.AIForge {
	seen := make(map[string]bool)
	var result []*schema.AIForge
	for _, f := range primary {
		if !seen[f.ForgeName] {
			seen[f.ForgeName] = true
			result = append(result, f)
		}
	}
	for _, f := range secondary {
		if limit > 0 && len(result) >= limit {
			break
		}
		if !seen[f.ForgeName] {
			seen[f.ForgeName] = true
			result = append(result, f)
		}
	}
	return result
}

// mergeAndDeduplicateLoops 合并并去重 loop 列表，primary 优先
func mergeAndDeduplicateLoops(primary, secondary []*LoopMetadata, limit int) []*LoopMetadata {
	seen := make(map[string]bool)
	var result []*LoopMetadata
	for _, l := range primary {
		if !seen[l.Name] {
			seen[l.Name] = true
			result = append(result, l)
		}
	}
	for _, l := range secondary {
		if limit > 0 && len(result) >= limit {
			break
		}
		if !seen[l.Name] {
			seen[l.Name] = true
			result = append(result, l)
		}
	}
	return result
}

// updateCacheWithMatchedItems 更新缓存，包括优先级处理和限制处理
// 新匹配的结果会排在前面，原缓存中未重复的条目补充到后面，直到达到限制
func (tr *ToolRecommender) updateCacheWithMatchedItems(matchedTools []*aitool.Tool, matchedForges []*schema.AIForge, matchedLoops []*LoopMetadata) {
	maxToolsLimit := tr.maxToolsLimit
	maxForgesLimit := tr.maxForgesLimit
	maxLoopsLimit := tr.maxLoopsLimit

	// ============ 更新工具缓存 ============
	if len(matchedTools) > 0 {
		tr.cachedToolsListMutex.Lock()

		// 先合并 matchedTools（优先）和缓存中的工具，去重
		merged := mergeAndDeduplicateTools(matchedTools, tr.cachedToolsList, 0)
		// 再整体按 priorityNames 排序并限制数量
		tr.cachedToolsList = tr.prioritizeAndLimitTools(merged, maxToolsLimit)

		tr.cachedToolsListMutex.Unlock()
		log.Infof("updated tools cache: matched %d tools, total cache size %d", len(matchedTools), len(tr.cachedToolsList))
	}

	// ============ 更新 forge 缓存 ============
	if len(matchedForges) > 0 {
		tr.cachedForgesListMutex.Lock()
		limitedMatched := tr.limitForges(matchedForges, maxForgesLimit)

		forgeMap := make(map[string]*schema.AIForge)
		var newForgesList []*schema.AIForge
		for _, forge := range limitedMatched {
			forgeMap[forge.ForgeName] = forge
			newForgesList = append(newForgesList, forge)
		}
		for _, cachedForge := range tr.cachedForgesList {
			if len(newForgesList) >= maxForgesLimit {
				break
			}
			if _, exists := forgeMap[cachedForge.ForgeName]; !exists {
				newForgesList = append(newForgesList, cachedForge)
				forgeMap[cachedForge.ForgeName] = cachedForge
			}
		}

		tr.cachedForgesList = newForgesList
		tr.cachedForgesListMutex.Unlock()
		log.Infof("updated forge cache: matched %d forges, total cache size %d", len(matchedForges), len(tr.cachedForgesList))
	}

	// ============ 更新 loops 缓存 ============
	if len(matchedLoops) > 0 {
		tr.cachedLoopsListMutex.Lock()

		loopMap := make(map[string]*LoopMetadata)
		var newLoopsList []*LoopMetadata
		for _, loop := range matchedLoops {
			loopMap[loop.Name] = loop
			newLoopsList = append(newLoopsList, loop)
		}
		for _, cachedLoop := range tr.cachedLoopsList {
			if maxLoopsLimit > 0 && len(newLoopsList) >= maxLoopsLimit {
				break
			}
			if _, exists := loopMap[cachedLoop.Name]; !exists {
				newLoopsList = append(newLoopsList, cachedLoop)
				loopMap[cachedLoop.Name] = cachedLoop
			}
		}

		tr.cachedLoopsList = newLoopsList
		tr.cachedLoopsListMutex.Unlock()
		log.Infof("updated loops cache: matched %d loops, total cache size %d", len(matchedLoops), len(tr.cachedLoopsList))
	}
}

// QuickSearch 执行快速双通道搜索（BM25 + 关键词匹配），适用于 FastIntentMatch 等快速意图识别场景。
// 使用较小的默认限制（各 5 条），搜索后自动更新缓存并返回合并去重后的结果。
func (tr *ToolRecommender) QuickSearch(query string) ([]*aitool.Tool, []*schema.AIForge, []*LoopMetadata, error) {
	return tr.SearchAndUpdateCache(query, 5, 5, 5)
}

// SearchCapabilitiesBM25 使用 BM25 搜索工具、forge 和 loops
// 返回搜索到的工具、forges 和 loops
func (tr *ToolRecommender) SearchCapabilitiesBM25(query string, toolLimit, forgeLimit, loopLimit int) (
	tools []*aitool.Tool,
	forges []*schema.AIForge,
	loops []*LoopMetadata,
	err error,
) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, nil, nil, utils.Error("search query is empty")
	}

	// 1. 使用 BM25 搜索工具
	db := consts.GetGormProfileDatabase()
	if db != nil {
		// 搜索 AIYakTool
		yakTools, err := yakit.SearchAIYakToolBM25(db, &yakit.AIYakToolFilter{
			Keywords: query,
		}, toolLimit, 0)
		if err != nil {
			log.Warnf("BM25 tool search failed: %v", err)
		} else if len(yakTools) > 0 {
			// 转换为 aitool.Tool
			// 从 AiToolManager 中获取实际的工具实例
			if tr.invoker != nil {
				config := tr.invoker.GetConfig()
				if mgr := config.GetAiToolManager(); mgr != nil {
					for _, yakTool := range yakTools {
						if tool, err := mgr.GetToolByName(yakTool.Name); err == nil && tool != nil {
							tools = append(tools, tool)
						}
					}
				}
			}
			log.Infof("BM25 search found %d tools", len(tools))
		}

		// 2. 使用 BM25 搜索 forges
		forges, err = yakit.SearchAIForgeBM25(db, &yakit.AIForgeSearchFilter{
			Keywords: query,
		}, forgeLimit, 0)
		if err != nil {
			log.Warnf("BM25 forge search failed: %v", err)
		} else if len(forges) > 0 {
			log.Infof("BM25 search found %d forges", len(forges))
		}
	}

	// 3. 搜索 loops（使用关键词匹配）
	loops = tr.searchLoopMetadata(query, loopLimit)
	if len(loops) > 0 {
		log.Infof("Found %d matching loops", len(loops))
	}

	return tools, forges, loops, nil
}

// getAllAvailableTools 从 invoker 配置中获取所有可用工具
func (tr *ToolRecommender) getAllAvailableTools() []*aitool.Tool {
	if tr.invoker == nil {
		return nil
	}
	config := tr.invoker.GetConfig()
	if config == nil {
		return nil
	}
	if mgr := config.GetAiToolManager(); mgr != nil {
		tools, _ := mgr.GetEnableTools()
		return tools
	}
	return nil
}

// getAllAvailableForges 从 invoker 配置中获取所有可用 forges
func (tr *ToolRecommender) getAllAvailableForges() []*schema.AIForge {
	if tr.invoker == nil {
		return nil
	}
	config := tr.invoker.GetConfig()
	if config == nil {
		return nil
	}
	cfg, ok := config.(*aicommon.Config)
	if !ok {
		return nil
	}
	var allForges []*schema.AIForge
	allForges = append(allForges, cfg.ExtendedForge...)
	if mgr := cfg.GetAIForgeManager(); mgr != nil {
		if forges, err := mgr.Query(config.GetContext()); err == nil {
			allForges = append(allForges, forges...)
		} else {
			log.Warnf("failed to query forges: %v", err)
		}
	}
	return allForges
}

// getAllAvailableLoops 获取所有非隐藏的 loop metadata，按 maxLoopsLimit 限制数量
func (tr *ToolRecommender) getAllAvailableLoops() []*LoopMetadata {
	allMeta := GetAllLoopMetadata()
	var visible []*LoopMetadata
	for _, meta := range allMeta {
		if meta.IsHidden {
			continue
		}
		visible = append(visible, meta)
		if tr.maxLoopsLimit > 0 && len(visible) >= tr.maxLoopsLimit {
			break
		}
	}
	return visible
}

// SearchCapabilitiesKeyword 使用关键词匹配搜索工具、forge 和 loops
// 与 SearchCapabilitiesBM25 签名一致，但使用 name/verboseName/keywords 子串匹配
func (tr *ToolRecommender) SearchCapabilitiesKeyword(query string, toolLimit, forgeLimit, loopLimit int) (
	tools []*aitool.Tool,
	forges []*schema.AIForge,
	loops []*LoopMetadata,
	err error,
) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, nil, nil, utils.Error("search query is empty")
	}

	queryLower := strings.ToLower(query)

	// 1. 关键词匹配工具
	allTools := tr.getAllAvailableTools()
	tools = keywordMatchTools(allTools, queryLower, toolLimit)
	if len(tools) > 0 {
		log.Infof("keyword search found %d tools", len(tools))
	}

	// 2. 关键词匹配 forges
	allForges := tr.getAllAvailableForges()
	forges = keywordMatchForges(allForges, queryLower, forgeLimit)
	if len(forges) > 0 {
		log.Infof("keyword search found %d forges", len(forges))
	}

	// 3. 搜索 loops（使用关键词匹配）
	loops = tr.searchLoopMetadata(query, loopLimit)
	if len(loops) > 0 {
		log.Infof("keyword search found %d matching loops", len(loops))
	}

	return tools, forges, loops, nil
}

// searchLoopMetadata 搜索注册的 loop metadata
// 参考 action_search_capabilities.go 中的实现
func (tr *ToolRecommender) searchLoopMetadata(query string, limit int) []*LoopMetadata {
	allMeta := GetAllLoopMetadata()
	queryLower := strings.ToLower(query)
	queryTokens := strings.Fields(queryLower)
	var matched []*LoopMetadata

	for _, meta := range allMeta {
		if meta.IsHidden {
			continue
		}
		searchText := strings.ToLower(meta.Name + " " + meta.Description + " " + meta.UsagePrompt)

		// 完整查询匹配
		if strings.Contains(searchText, queryLower) {
			matched = append(matched, meta)
			if limit > 0 && len(matched) >= limit {
				break
			}
			continue
		}

		// Token 级别匹配：要求至少一半有意义的 token 匹配
		if len(queryTokens) > 1 {
			meaningfulTokens := 0
			matchCount := 0
			for _, token := range queryTokens {
				if len(token) < 2 {
					continue
				}
				meaningfulTokens++
				if strings.Contains(searchText, token) {
					matchCount++
				}
			}
			if meaningfulTokens > 0 && matchCount > 0 && matchCount >= (meaningfulTokens+1)/2 {
				matched = append(matched, meta)
				if limit > 0 && len(matched) >= limit {
					break
				}
			}
		}
	}
	return matched
}

// GetCachedLoops 获取缓存的 loops
func (tr *ToolRecommender) GetCachedLoops() []*LoopMetadata {
	tr.cachedLoopsListMutex.Lock()
	defer tr.cachedLoopsListMutex.Unlock()
	return tr.cachedLoopsList
}

// UpdateCachedLoops 更新缓存的 loops
func (tr *ToolRecommender) UpdateCachedLoops(loops []*LoopMetadata) {
	tr.cachedLoopsListMutex.Lock()
	defer tr.cachedLoopsListMutex.Unlock()
	tr.cachedLoopsList = loops
}

// SearchAndUpdateCache 统一的搜索方法，同时使用 BM25 和关键词双通道搜索工具、forges 和 loops，
// 搜索完成后去重合并并更新缓存，返回合并后的结果。
func (tr *ToolRecommender) SearchAndUpdateCache(query string, toolLimit, forgeLimit, loopLimit int) (
	[]*aitool.Tool, []*schema.AIForge, []*LoopMetadata, error,
) {
	// 执行 BM25 搜索
	bm25Tools, bm25Forges, bm25Loops, err := tr.SearchCapabilitiesBM25(query, toolLimit, forgeLimit, loopLimit)
	if err != nil {
		return nil, nil, nil, err
	}

	// 执行关键词搜索
	kwTools, kwForges, kwLoops, _ := tr.SearchCapabilitiesKeyword(query, toolLimit, forgeLimit, loopLimit)

	// 合并去重（BM25 结果优先）
	tools := mergeAndDeduplicateTools(bm25Tools, kwTools, toolLimit)
	forges := mergeAndDeduplicateForges(bm25Forges, kwForges, forgeLimit)
	loops := mergeAndDeduplicateLoops(bm25Loops, kwLoops, loopLimit)

	// 使用统一的缓存更新方法
	tr.updateCacheWithMatchedItems(tools, forges, loops)

	return tools, forges, loops, nil
}

// SearchAndUpdateCacheAsync 异步执行搜索并更新缓存
func (tr *ToolRecommender) SearchAndUpdateCacheAsync(query string, toolLimit, forgeLimit, loopLimit int, onFinished ...func()) {
	tr.recommendTaskSizeWaitGroup.Add(1)
	go func() {
		defer tr.recommendTaskSizeWaitGroup.Done()
		defer func() {
			if err := recover(); err != nil {
				log.Errorf("panic occurred during async capability search: %v", err)
			}
		}()
		defer func() {
			// 调用完成回调
			for _, callback := range onFinished {
				if callback != nil {
					callback()
				}
			}
		}()

		if _, _, _, err := tr.SearchAndUpdateCache(query, toolLimit, forgeLimit, loopLimit); err != nil {
			log.Errorf("async capability search failed: %v", err)
		}
	}()
}
