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
func NewToolRecommender(invoker aicommon.AIInvokeRuntime) *ToolRecommender {
	return &ToolRecommender{
		cachedToolsList:            make([]*aitool.Tool, 0),
		cachedForgesList:           make([]*schema.AIForge, 0),
		cachedLoopsList:            make([]*LoopMetadata, 0),
		cachedToolsListMutex:       new(sync.Mutex),
		cachedForgesListMutex:      new(sync.Mutex),
		cachedLoopsListMutex:       new(sync.Mutex),
		recommendTaskSizeWaitGroup: utils.NewSizedWaitGroup(10),
		invoker:                    invoker,
		maxToolsLimit:              30,  // 默认限制工具数量为 30 个
		maxForgesLimit:             200, // 默认限制 forge 数量为 200 个
		maxLoopsLimit:              10,  // 默认限制 loop 数量为 10 个
	}
}

// GetRecommendedToolsAndForges 根据用户输入通过关键词匹配获取推荐的 tools 和 forges
// 使用默认的限制值
func (tr *ToolRecommender) GetRecommendedToolsAndForges(userInput string, config aicommon.AICallerConfigIf) ([]*aitool.Tool, []*schema.AIForge) {
	return tr.GetRecommendedToolsAndForgesWithLimits(userInput, config, tr.maxToolsLimit, tr.maxForgesLimit)
}

// GetRecommendedToolsAndForgesWithLimits 根据用户输入通过关键词匹配获取推荐的 tools 和 forges
// 支持自定义限制值
func (tr *ToolRecommender) GetRecommendedToolsAndForgesWithLimits(userInput string, config aicommon.AICallerConfigIf, maxToolsLimit, maxForgesLimit int) ([]*aitool.Tool, []*schema.AIForge) {
	// 使用传入的限制值

	tr.cachedToolsListMutex.Lock()
	defer tr.cachedToolsListMutex.Unlock()

	tr.cachedForgesListMutex.Lock()
	defer tr.cachedForgesListMutex.Unlock()

	// 获取所有可用工具
	var allTools []*aitool.Tool
	toolMgr := config.GetAiToolManager()
	if toolMgr != nil {
		allTools, _ = toolMgr.GetEnableTools()
	}

	// 获取所有可用 forges
	var allForges []*schema.AIForge
	if cfg, ok := config.(*aicommon.Config); ok {
		// 首先添加扩展 forges（通过 WithForges 添加的）
		allForges = append(allForges, cfg.ExtendedForge...)

		// 然后查询数据库中的 forges
		mgr := cfg.GetAIForgeManager()
		if mgr != nil {
			forges, err := mgr.Query(config.GetContext())
			if err == nil {
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

	// 如果缓存的 forges 列表为空，使用全部 forges 初始化（限制数量）
	if len(tr.cachedForgesList) == 0 {
		tr.cachedForgesList = tr.limitForges(allForges, maxForgesLimit)
	}

	// 如果没有用户输入，返回缓存的列表
	if userInput == "" {
		return tr.prioritizeAndLimitTools(allTools, maxToolsLimit), tr.limitForges(allForges, maxForgesLimit)
	}

	// 将用户输入转为小写用于匹配
	userInputLower := strings.ToLower(userInput)

	// ============ 处理工具匹配 ============
	var matchedTools []*aitool.Tool
	matchedToolsMap := make(map[string]bool)

	for _, tool := range allTools {
		// 检查工具名称是否匹配
		if utils.MatchAnyOfSubString(userInputLower, strings.ToLower(tool.Name)) {
			matchedTools = append(matchedTools, tool)
			matchedToolsMap[tool.Name] = true
			continue
		}

		// 检查 VerboseName 是否匹配
		if tool.VerboseName != "" && utils.MatchAnyOfSubString(userInputLower, strings.ToLower(tool.VerboseName)) {
			matchedTools = append(matchedTools, tool)
			matchedToolsMap[tool.Name] = true
			continue
		}

		// 检查关键词是否匹配
		for _, keyword := range tool.Keywords {
			if keyword != "" && utils.MatchAnyOfSubString(userInputLower, strings.ToLower(keyword)) {
				matchedTools = append(matchedTools, tool)
				matchedToolsMap[tool.Name] = true
				break
			}
		}
	}

	// 如果有匹配成功的工具，把匹配的放在前面
	var recommendedTools []*aitool.Tool
	if len(matchedTools) > 0 {
		// 首先添加匹配的工具（应用优先级排序）
		matchedPrioritized := tr.prioritizeAndLimitTools(matchedTools, maxToolsLimit)
		recommendedTools = append(recommendedTools, matchedPrioritized...)

		// 然后从缓存列表中添加未匹配的工具，直到达到限制
		for _, cachedTool := range tr.cachedToolsList {
			if len(recommendedTools) >= maxToolsLimit {
				break
			}
			// 如果这个工具不在匹配列表中，添加它
			if !matchedToolsMap[cachedTool.Name] {
				recommendedTools = append(recommendedTools, cachedTool)
			}
		}

		// 更新缓存列表
		tr.cachedToolsList = recommendedTools
	} else {
		// 没有匹配到任何工具，返回缓存的工具列表
		recommendedTools = tr.cachedToolsList
	}

	// ============ 处理 forges 匹配 ============
	var matchedForges []*schema.AIForge
	matchedForgesMap := make(map[string]bool)

	for _, forge := range allForges {
		// 检查 forge 名称是否匹配
		if utils.MatchAnyOfSubString(userInputLower, strings.ToLower(forge.ForgeName)) {
			matchedForges = append(matchedForges, forge)
			matchedForgesMap[forge.ForgeName] = true
			continue
		}

		// 检查 VerboseName 是否匹配
		if forge.ForgeVerboseName != "" && utils.MatchAnyOfSubString(userInputLower, strings.ToLower(forge.ForgeVerboseName)) {
			matchedForges = append(matchedForges, forge)
			matchedForgesMap[forge.ForgeName] = true
			continue
		}

		// 检查标签是否匹配
		keywords := forge.GetKeywords()
		for _, keyword := range keywords {
			if keyword != "" && utils.MatchAnyOfSubString(userInputLower, strings.ToLower(keyword)) {
				matchedForges = append(matchedForges, forge)
				matchedForgesMap[forge.ForgeName] = true
				break
			}
		}
	}

	// 如果有匹配成功的 forges，把匹配的放在前面
	var recommendedForges []*schema.AIForge
	if len(matchedForges) > 0 {
		// 首先添加匹配的 forges（限制数量）
		recommendedForges = append(recommendedForges, tr.limitForges(matchedForges, maxForgesLimit)...)

		// 然后从缓存列表中添加未匹配的 forges，直到达到限制
		for _, cachedForge := range tr.cachedForgesList {
			if len(recommendedForges) >= maxForgesLimit {
				break
			}
			// 如果这个 forge 不在匹配列表中，添加它
			if !matchedForgesMap[cachedForge.ForgeName] {
				recommendedForges = append(recommendedForges, cachedForge)
			}
		}

		// 更新缓存列表
		tr.cachedForgesList = recommendedForges
	} else {
		// 没有匹配到任何 forge，返回缓存的 forges 列表
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
			tr.updateCacheWithMatchedItems(searchedTools, nil)
		}
	}()
}

// WaitRecommendTask 等待所有推荐任务完成
func (tr *ToolRecommender) WaitRecommendTask() {
	tr.recommendTaskSizeWaitGroup.Wait()
}

// GetCachedToolsAndForges 获取缓存的工具和 forges（需要加锁）
func (tr *ToolRecommender) GetCachedToolsAndForges() ([]*aitool.Tool, []*schema.AIForge) {
	tr.cachedToolsListMutex.Lock()
	tools := tr.cachedToolsList
	tr.cachedToolsListMutex.Unlock()

	tr.cachedForgesListMutex.Lock()
	forges := tr.cachedForgesList
	tr.cachedForgesListMutex.Unlock()

	return tools, forges
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

// updateCacheWithMatchedItems 更新缓存，包括优先级处理和限制处理
// matchedTools: 新匹配到的工具列表
// matchedForges: 新匹配到的 forge 列表
func (tr *ToolRecommender) updateCacheWithMatchedItems(matchedTools []*aitool.Tool, matchedForges []*schema.AIForge) {
	// 使用配置的限制值
	maxToolsLimit := tr.maxToolsLimit
	maxForgesLimit := tr.maxForgesLimit

	tr.cachedToolsListMutex.Lock()
	defer tr.cachedToolsListMutex.Unlock()

	tr.cachedForgesListMutex.Lock()
	defer tr.cachedForgesListMutex.Unlock()

	// ============ 更新工具缓存 ============
	if len(matchedTools) > 0 {
		// 创建工具名称映射，用于去重
		toolMap := make(map[string]*aitool.Tool)
		var newToolsList []*aitool.Tool

		// 首先添加新匹配的工具（应用优先级排序）
		prioritizedMatched := tr.prioritizeAndLimitTools(matchedTools, maxToolsLimit)
		for _, tool := range prioritizedMatched {
			toolMap[tool.Name] = tool
			newToolsList = append(newToolsList, tool)
		}

		// 然后从原缓存中添加未匹配的工具，直到达到限制
		for _, cachedTool := range tr.cachedToolsList {
			if len(newToolsList) >= maxToolsLimit {
				break
			}
			// 如果这个工具不在新匹配列表中，添加它
			if _, exists := toolMap[cachedTool.Name]; !exists {
				newToolsList = append(newToolsList, cachedTool)
				toolMap[cachedTool.Name] = cachedTool
			}
		}

		// 更新缓存
		tr.cachedToolsList = newToolsList
		log.Infof("updated tools cache: matched %d tools, total cache size %d", len(matchedTools), len(tr.cachedToolsList))
	}

	// ============ 更新 forge 缓存 ============
	if len(matchedForges) > 0 {
		// 创建 forge 名称映射，用于去重
		forgeMap := make(map[string]*schema.AIForge)
		var newForgesList []*schema.AIForge

		// 首先添加新匹配的 forges（限制数量）
		limitedMatched := tr.limitForges(matchedForges, maxForgesLimit)
		for _, forge := range limitedMatched {
			forgeMap[forge.ForgeName] = forge
			newForgesList = append(newForgesList, forge)
		}

		// 然后从原缓存中添加未匹配的 forges，直到达到限制
		for _, cachedForge := range tr.cachedForgesList {
			if len(newForgesList) >= maxForgesLimit {
				break
			}
			// 如果这个 forge 不在新匹配列表中，添加它
			if _, exists := forgeMap[cachedForge.ForgeName]; !exists {
				newForgesList = append(newForgesList, cachedForge)
				forgeMap[cachedForge.ForgeName] = cachedForge
			}
		}

		// 更新缓存
		tr.cachedForgesList = newForgesList
		log.Infof("updated forge cache: matched %d forges, total cache size %d", len(matchedForges), len(tr.cachedForgesList))
	}
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

// SearchAndUpdateCache 统一的搜索方法，使用 BM25 搜索工具和 forges，关键词搜索 loops
// 搜索完成后更新缓存
func (tr *ToolRecommender) SearchAndUpdateCache(query string, toolLimit, forgeLimit, loopLimit int) error {
	// 执行 BM25 搜索
	tools, forges, loops, err := tr.SearchCapabilitiesBM25(query, toolLimit, forgeLimit, loopLimit)
	if err != nil {
		return err
	}

	// 更新工具缓存（应用优先级排序）
	if len(tools) > 0 {
		tr.cachedToolsListMutex.Lock()
		prioritizedTools := tr.prioritizeAndLimitTools(tools, toolLimit)

		// 创建工具映射用于去重
		toolMap := make(map[string]*aitool.Tool)
		var newToolsList []*aitool.Tool

		// 首先添加新搜索到的工具
		for _, tool := range prioritizedTools {
			toolMap[tool.Name] = tool
			newToolsList = append(newToolsList, tool)
		}

		// 然后从原缓存中添加未匹配的工具，直到达到限制
		for _, cachedTool := range tr.cachedToolsList {
			if len(newToolsList) >= toolLimit {
				break
			}
			if _, exists := toolMap[cachedTool.Name]; !exists {
				newToolsList = append(newToolsList, cachedTool)
				toolMap[cachedTool.Name] = cachedTool
			}
		}

		tr.cachedToolsList = newToolsList
		tr.cachedToolsListMutex.Unlock()
		log.Infof("updated tools cache via BM25: found %d tools, total cache size %d", len(tools), len(tr.cachedToolsList))
	}

	// 更新 forge 缓存
	if len(forges) > 0 {
		tr.cachedForgesListMutex.Lock()
		limitedForges := tr.limitForges(forges, forgeLimit)

		// 创建 forge 映射用于去重
		forgeMap := make(map[string]*schema.AIForge)
		var newForgesList []*schema.AIForge

		// 首先添加新搜索到的 forges
		for _, forge := range limitedForges {
			forgeMap[forge.ForgeName] = forge
			newForgesList = append(newForgesList, forge)
		}

		// 然后从原缓存中添加未匹配的 forges，直到达到限制
		for _, cachedForge := range tr.cachedForgesList {
			if len(newForgesList) >= forgeLimit {
				break
			}
			if _, exists := forgeMap[cachedForge.ForgeName]; !exists {
				newForgesList = append(newForgesList, cachedForge)
				forgeMap[cachedForge.ForgeName] = cachedForge
			}
		}

		tr.cachedForgesList = newForgesList
		tr.cachedForgesListMutex.Unlock()
		log.Infof("updated forges cache via BM25: found %d forges, total cache size %d", len(forges), len(tr.cachedForgesList))
	}

	// 更新 loops 缓存
	if len(loops) > 0 {
		tr.UpdateCachedLoops(loops)
		log.Infof("updated loops cache: found %d loops", len(loops))
	}

	return nil
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

		if err := tr.SearchAndUpdateCache(query, toolLimit, forgeLimit, loopLimit); err != nil {
			log.Errorf("async capability search failed: %v", err)
		}
	}()
}
