package buildinaitools

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"

	"github.com/samber/lo"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/searchtools"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/yakscripttools"
	"github.com/yaklang/yaklang/common/ai/ytoken"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/omap"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

const defaultRecentToolCacheMaxTokens = 30 * 1024

// RecentToolEntry records a recently used tool for directly_call_tool action.
type RecentToolEntry struct {
	Name          string `json:"name"`
	Description   string `json:"description"`
	SchemaSnippet string `json:"schema_snippet"`
	Usage         string `json:"usage"`
	Size          int    `json:"size"`
}

// RecentToolCacheMutation is the prompt-visible delta caused by an LRU update.
// Reusing an unchanged tool may still refresh execution-side LRU order, but
// intentionally returns no Upsert so the prompt prefix remains byte-stable.
type RecentToolCacheMutation struct {
	Upsert  *RecentToolEntry
	Deleted []*RecentToolEntry
}

// AiToolManager 是工具管理器的默认实现
type AiToolManager struct {
	toolsGetter           func() []*aitool.Tool
	toolEnabled           map[string]bool // 记录工具是否开启
	enableSearchTool      bool            // 是否开启工具搜索 (legacy)
	enableForgeSearchTool bool            // 是否开启forge工具搜索 (legacy)
	aiToolsSearcher       searchtools.AISearcher[*aitool.Tool]
	aiForgeSearcher       searchtools.AISearcher[*schema.AIForge]
	disableTools          map[string]struct{} // 禁用的工具列表 优先级最高
	searchTool            []*aitool.Tool
	forgeSearchTool       []*aitool.Tool
	noCacheTools          bool // 是否不缓存工具
	enableAllTools        bool // 是否开启所有工具
	disallowMCPServers    bool // when true, hide MCP tools from search/list/lookup paths

	recentToolsCache []*RecentToolEntry
	recentToolsMu    sync.Mutex
	maxCacheTokens   int
}

// ToolManagerOption 定义工具管理器的配置选项
type ToolManagerOption func(*AiToolManager)

// WithAIToolsSearcher 设置搜索器
func WithAIToolsSearcher(searcher searchtools.AISearcher[*aitool.Tool]) ToolManagerOption {
	return func(m *AiToolManager) {
		m.aiToolsSearcher = searcher
	}
}

// WithNoToolsCache 设置不缓存工具
func WithNoToolsCache() ToolManagerOption {
	return func(m *AiToolManager) {
		m.noCacheTools = true
	}
}

// WithEnableAllTools 设置开启所有工具
func WithEnableAllTools() ToolManagerOption {
	return func(m *AiToolManager) {
		m.enableAllTools = true
	}
}

// WithAiForgeSearcher 设置forge搜索器
func WithAiForgeSearcher(searcher searchtools.AISearcher[*schema.AIForge]) ToolManagerOption {
	return func(m *AiToolManager) {
		m.aiForgeSearcher = searcher
	}
}

func WithForgeSearchToolEnabled(enabled bool) ToolManagerOption {
	return func(m *AiToolManager) {
		m.enableForgeSearchTool = enabled
	}
}

func WithDisableTools(toolsName []string) ToolManagerOption {
	return func(m *AiToolManager) {
		if m.disableTools == nil {
			m.disableTools = make(map[string]struct{})
		}
		for _, name := range toolsName {
			m.disableTools[name] = struct{}{}
		}
	}
}

func WithExtendTools(tools []*aitool.Tool, suggested ...bool) ToolManagerOption {
	return func(m *AiToolManager) {
		var enable = len(suggested) > 0 && suggested[0]
		var allTools []*aitool.Tool
		if m.toolsGetter != nil {
			allTools = m.toolsGetter()
		}

		toolsMap := map[string]*aitool.Tool{}
		for _, tool := range allTools {
			toolsMap[tool.Name] = tool
		}

		var extTools []*aitool.Tool
		lo.ForEach(tools, func(tool *aitool.Tool, _ int) {
			if enable {
				m.EnableTool(tool.Name)
			}
			if _, ok := toolsMap[tool.Name]; !ok {
				extTools = append(extTools, tool)
			}
		})

		originGetter := m.toolsGetter
		m.toolsGetter = func() []*aitool.Tool {
			return append(originGetter(), extTools...)
		}
	}
}

// WithEnabledTools 设置开启的工具列表
func WithEnabledTools(toolNames []string) ToolManagerOption {
	return func(m *AiToolManager) {
		toolEnabled := map[string]bool{}
		// 开启指定工具
		for _, name := range toolNames {
			toolEnabled[name] = true
		}
		m.toolEnabled = toolEnabled
	}
}

// WithToolEnabled 设置单个工具的开启状态
func WithToolEnabled(name string, enabled bool) ToolManagerOption {
	return func(m *AiToolManager) {
		m.toolEnabled[name] = enabled
	}
}

// WithSearchToolEnabled 设置是否开启工具搜索
func WithSearchToolEnabled(enabled bool) ToolManagerOption {
	return func(m *AiToolManager) {
		m.enableSearchTool = enabled
	}
}

// WithDisallowMCPServers hides MCP tools from search, prompt inventory, and name lookup.
func WithDisallowMCPServers(disallow bool) ToolManagerOption {
	return func(m *AiToolManager) {
		m.disallowMCPServers = disallow
		m.searchTool = nil
	}
}

func NewToolManagerByToolGetter(getter func() []*aitool.Tool, options ...ToolManagerOption) *AiToolManager {
	manager := &AiToolManager{
		toolsGetter:           getter,
		toolEnabled:           make(map[string]bool),
		enableSearchTool:      true,
		enableForgeSearchTool: true,
	}

	// 应用选项
	for _, option := range options {
		option(manager)
	}

	return manager
}

// SetDisallowMCPServers updates MCP visibility policy and invalidates cached search tools.
func (m *AiToolManager) SetDisallowMCPServers(disallow bool) {
	if m == nil {
		return
	}
	m.disallowMCPServers = disallow
	m.searchTool = nil
}

// DisallowMCPServers reports whether MCP tools are hidden from this manager.
func (m *AiToolManager) DisallowMCPServers() bool {
	if m == nil {
		return false
	}
	return m.disallowMCPServers
}

// NewToolManager 创建一个新的默认工具管理器实例
func NewToolManager(options ...ToolManagerOption) *AiToolManager {
	basicToolsOptions := []ToolManagerOption{
		WithExtendTools(GetBasicBuildInTools(), true),
	} // 默认开启基础工具
	options = append(basicToolsOptions, options...)

	manager := NewToolManagerByToolGetter(GetAllTools, options...) //候选工具由GetAllTools提供
	if manager.enableAllTools {
		manager = NewToolManagerByToolGetter(func() []*aitool.Tool {
			return GetAllToolsDynamically(consts.GetGormProfileDatabase())
		}, options...)
	}
	return manager
}

func (m *AiToolManager) safeToolsGetter() []*aitool.Tool {
	if m.toolsGetter == nil {
		return []*aitool.Tool{}
	}
	allTools := m.toolsGetter()
	if m.disallowMCPServers {
		allTools = lo.Filter(allTools, func(tool *aitool.Tool, _ int) bool {
			return !IsMCPToolName(tool.Name)
		})
	}
	if len(m.disableTools) > 0 {
		allTools = lo.Filter(allTools, func(tool *aitool.Tool, _ int) bool {
			_, ok := m.disableTools[tool.Name]
			return !ok
		})
	}
	return allTools
}

// GetEnableTools 获取所有可用的工具
func (m *AiToolManager) GetEnableTools() ([]*aitool.Tool, error) {

	var enabledTools []*aitool.Tool
	for _, tool := range m.safeToolsGetter() {
		if m.enableAllTools || m.toolEnabled[tool.Name] {
			enabledTools = append(enabledTools, tool)
		}
	}

	if m.enableSearchTool {
		tool, err := m.getSearchTools()
		if err != nil {
			log.Errorf("getSearchTools err: %v", err)
		}
		if !utils.IsNil(tool) {
			enabledTools = append(enabledTools, tool...)
		}
	}
	if m.enableForgeSearchTool {
		tool, err := m.getForgeSearchTools()
		if err != nil {
			log.Errorf("getForgeSearchTools err: %v", err)
		}
		if !utils.IsNil(tool) {
			enabledTools = append(enabledTools, tool...)
		}
	}
	return enabledTools, nil
}

func (m *AiToolManager) getForgeSearchTools() ([]*aitool.Tool, error) {
	if m.forgeSearchTool == nil {
		// aiforge search tools
		aiforgeSearchTools, err := searchtools.CreateAISearchTools(m.aiForgeSearcher, func() []*schema.AIForge {
			forgeList, err := yakit.GetAllAIForge(consts.GetGormProfileDatabase())
			if err != nil {
				log.Errorf("get all ai forge error: %v", err)
			}
			return forgeList
		}, searchtools.SearchForgeName, false)
		if err != nil {
			return nil, utils.Errorf("create ai forge search tools: %v", err)
		}
		m.forgeSearchTool = aiforgeSearchTools
	}
	return m.forgeSearchTool, nil
}

func (m *AiToolManager) getSearchTools() ([]*aitool.Tool, error) {
	if m.searchTool == nil {
		var err error
		// ai tool search tools
		aiToolSearchTools, err := searchtools.CreateAISearchTools(
			m.aiToolsSearcher,
			m.safeToolsGetter,
			searchtools.SearchToolName,
			!m.disallowMCPServers,
		)
		if err != nil {
			log.Error(err)
		}
		m.searchTool = aiToolSearchTools
	}
	return m.searchTool, nil
}

// GetToolByName 通过工具名获取特定工具
func (m *AiToolManager) GetToolByName(name string) (*aitool.Tool, error) {
	tools, err := m.GetEnableTools()
	if err != nil {
		return nil, err
	}
	for _, tool := range tools {
		if tool.Name == name {
			return tool, nil
		}
	}

	db := consts.GetGormProfileDatabase()

	// 从 ai_yak_tools 数据库中查找工具
	toolFromDB, err := yakit.GetAIYakTool(db, name)
	if err == nil {
		convertedTools := yakscripttools.ConvertTools([]*schema.AIYakTool{toolFromDB})
		if len(convertedTools) > 0 {
			return convertedTools[0], nil
		}
		log.Errorf("convert tool [%s] from ai_yak_tools database failed", name)
	}

	// 从 yak_scripts 数据库中查找 enable_for_ai=true 的 Yakit 插件
	scriptFromDB, scriptErr := yakit.GetYakScriptByNameForAI(db, name)
	if scriptErr == nil {
		convertedTool, convertErr := yakscripttools.ConvertYakScriptPlugin(scriptFromDB)
		if convertErr == nil && convertedTool != nil {
			return convertedTool, nil
		}
		if convertErr != nil {
			log.Errorf("convert YakScript plugin [%s] failed: %v", name, convertErr)
		}
	}

	// Look up cached MCP tool metadata using the compound name "mcp_{server}_{tool}".
	// Skip when MCP servers are disabled for this runtime.
	if !m.disallowMCPServers {
		mcpCfg, mcpErr := yakit.GetMCPServerToolConfigByFullName(db, name)
		if mcpErr == nil && mcpCfg.Enable && mcpCfg.Description != "" {
			stub := buildStubToolFromMCPCache(name, mcpCfg)
			if stub != nil {
				return stub, nil
			}
		}
	}

	return nil, fmt.Errorf("cannot find [%v] in ai_yak_tools, yak_scripts, mcp cache, or enabled tools", name)
}

// SearchTools 通过字符串搜索相关工具
func (m *AiToolManager) SearchTools(method string, query string) ([]*aitool.Tool, error) {
	if !m.enableSearchTool {
		return nil, fmt.Errorf("工具搜索功能已被禁用")
	}
	res, err := m.aiToolsSearcher(query, m.safeToolsGetter())
	if err != nil {
		return nil, err
	}
	return res, nil
}

// EnableTool 开启单个工具
func (m *AiToolManager) EnableTool(name string) {
	m.toolEnabled[name] = true
	if m.disableTools != nil {
		delete(m.disableTools, name)
	}
}

// DisableTool 关闭单个工具
func (m *AiToolManager) DisableTool(name string) {
	m.toolEnabled[name] = false
	if m.disableTools == nil {
		m.disableTools = make(map[string]struct{})
	}
	m.disableTools[name] = struct{}{}
}

func (m *AiToolManager) AppendTools(tools ...*aitool.Tool) error {
	var allTools []*aitool.Tool
	if m.toolsGetter != nil {
		allTools = m.toolsGetter()
	}

	toolsMap := map[string]*aitool.Tool{}
	for _, tool := range allTools {
		toolsMap[tool.Name] = tool
	}

	var extTools []*aitool.Tool
	lo.ForEach(tools, func(tool *aitool.Tool, _ int) {
		m.EnableTool(tool.Name)
		if _, ok := toolsMap[tool.Name]; !ok {
			extTools = append(extTools, tool)
		}
	})

	originGetter := m.toolsGetter
	m.toolsGetter = func() []*aitool.Tool {
		return append(originGetter(), extTools...)
	}
	return nil
}

// RestrictToTools confines the manager to exactly the named tools: it clears the
// "enable all" shortcut and both searchers, then enables only the given names.
// Session-scoped MCP mounts use this so the agent cannot reach builtin/profile
// tools (e.g. the local "ssa-risk" yak tool) and is limited to the injected set.
func (m *AiToolManager) RestrictToTools(names ...string) {
	if m == nil {
		return
	}
	m.enableAllTools = false
	m.enableSearchTool = false
	m.enableForgeSearchTool = false
	enabled := make(map[string]bool, len(names))
	for _, name := range names {
		enabled[name] = true
	}
	m.toolEnabled = enabled
}

// OverrideToolByName replaces all tools with the given name, keeping only the new one.
// If no tool with that name exists, the new tool is appended.
func (m *AiToolManager) OverrideToolByName(newTool *aitool.Tool) {
	originGetter := m.toolsGetter
	m.toolsGetter = func() []*aitool.Tool {
		var result []*aitool.Tool
		found := false
		for _, t := range originGetter() {
			if t.Name == newTool.Name {
				if !found {
					result = append(result, newTool)
					found = true
				}
			} else {
				result = append(result, t)
			}
		}
		if !found {
			result = append(result, newTool)
		}
		return result
	}
	m.EnableTool(newTool.Name)
}

// RemoveToolByName drops every tool with the given name from the in-memory registry.
func (m *AiToolManager) RemoveToolByName(name string) {
	if name == "" {
		return
	}
	originGetter := m.toolsGetter
	m.toolsGetter = func() []*aitool.Tool {
		var result []*aitool.Tool
		for _, t := range originGetter() {
			if t.Name != name {
				result = append(result, t)
			}
		}
		return result
	}
	m.DisableTool(name)
}

func (m *AiToolManager) EnableAIToolSearch(searcher searchtools.AISearcher[*aitool.Tool]) error {
	m.enableSearchTool = true
	m.aiToolsSearcher = searcher
	return nil
}

func (m *AiToolManager) EnableAIForgeSearch(searcher searchtools.AISearcher[*schema.AIForge]) error {
	m.enableForgeSearchTool = true
	m.aiForgeSearcher = searcher
	return nil
}

func (m *AiToolManager) getMaxCacheTokens() int {
	if m.maxCacheTokens > 0 {
		return m.maxCacheTokens
	}
	return defaultRecentToolCacheMaxTokens
}

func (m *AiToolManager) GetRecentToolCacheMaxTokens() int {
	return m.getMaxCacheTokens()
}

func (m *AiToolManager) totalCacheSize() int {
	total := 0
	for _, entry := range m.recentToolsCache {
		total += entry.Size
	}
	return total
}

// AddRecentlyUsedTool caches a tool for later directly_call_tool usage.
// Duplicates are moved to the tail (most recent); FIFO eviction when over budget.
func (m *AiToolManager) AddRecentlyUsedTool(tool *aitool.Tool) RecentToolCacheMutation {
	var mutation RecentToolCacheMutation
	if tool == nil {
		return mutation
	}
	m.recentToolsMu.Lock()
	defer m.recentToolsMu.Unlock()

	name := tool.GetName()
	desc := tool.GetDescription()
	schemaStr := tool.ToJSONSchemaString()
	usage := tool.GetUsage()
	entrySize := ytoken.CalcTokenCount(name) + ytoken.CalcTokenCount(desc) + ytoken.CalcTokenCount(schemaStr) + ytoken.CalcTokenCount(usage)

	// remove existing entry with same name (will be re-appended at tail)
	filtered := make([]*RecentToolEntry, 0, len(m.recentToolsCache))
	var previous *RecentToolEntry
	for _, e := range m.recentToolsCache {
		if e.Name != name {
			filtered = append(filtered, e)
		} else {
			previous = e
		}
	}
	m.recentToolsCache = filtered

	newEntry := &RecentToolEntry{
		Name:          name,
		Description:   desc,
		SchemaSnippet: schemaStr,
		Usage:         usage,
		Size:          entrySize,
	}
	m.recentToolsCache = append(m.recentToolsCache, newEntry)
	if previous == nil || previous.Description != newEntry.Description || previous.SchemaSnippet != newEntry.SchemaSnippet || previous.Usage != newEntry.Usage {
		cp := *newEntry
		mutation.Upsert = &cp
	}

	maxTokens := m.getMaxCacheTokens()
	for m.totalCacheSize() > maxTokens && len(m.recentToolsCache) > 1 {
		evicted := m.recentToolsCache[0]
		m.recentToolsCache = m.recentToolsCache[1:]
		if evicted != nil && evicted.Name != name {
			cp := *evicted
			mutation.Deleted = append(mutation.Deleted, &cp)
		}
	}
	return mutation
}

// RenderRecentToolEntryForPromotion renders one stable, params-only schema.
// Collection ordering and the shared routing instructions are owned by the
// Timeline promoted-state projection.
func RenderRecentToolEntryForPromotion(entry *RecentToolEntry) string {
	if entry == nil {
		return ""
	}
	return strings.TrimSpace(utils.MustRenderTemplate(recentToolEntryTemplate, map[string]interface{}{
		"Name":                 entry.Name,
		"Nonce":                RecentToolCacheStableNonce,
		"Description":          entry.Description,
		"DisplaySchemaSnippet": renderDirectlyCallParamsSchema(entry.SchemaSnippet),
		"Usage":                entry.Usage,
	}))
}

func (m *AiToolManager) IsRecentlyUsedTool(name string) bool {
	m.recentToolsMu.Lock()
	defer m.recentToolsMu.Unlock()
	for _, e := range m.recentToolsCache {
		if e.Name == name {
			return true
		}
	}
	return false
}

func (m *AiToolManager) GetRecentToolNames() []string {
	m.recentToolsMu.Lock()
	defer m.recentToolsMu.Unlock()
	names := make([]string, 0, len(m.recentToolsCache))
	for _, e := range m.recentToolsCache {
		names = append(names, e.Name)
	}
	return names
}

func (m *AiToolManager) HasRecentlyUsedTools() bool {
	m.recentToolsMu.Lock()
	defer m.recentToolsMu.Unlock()
	return len(m.recentToolsCache) > 0
}

// RecentToolCacheStableNonce 是 CACHE_TOOL_CALL 块及其内部所有 AITAG (TOOL_xxx /
// TOOL_PARAM_xxx) 渲染时使用的稳定 nonce 字面量. 跨 react turn 不变, 让承载
// 该块的 prompt 段保持字节级稳定, 进入 prefix cache.
//
// 字面量必须与 aicommon.RecentToolCacheStableNonce 严格一致 (两边互不 import,
// 各自定义本地副本; 不一致会导致渲染端写一种, 解析端注册另一种, callback
// 不命中, 内容丢失). 当前两边都是 "[current-nonce]".
//
// 关键词: RecentToolCacheStableNonce, [current-nonce], 占位符语义,
//
//	与 aicommon.RecentToolCacheStableNonce 字面量严格一致
const RecentToolCacheStableNonce = "[current-nonce]"

const recentToolEntryTemplate = `<|TOOL_{{ .Name }}_{{ .Nonce }}|>
## Tool: {{ .Name }}
Description: {{ .Description }}
Direct Params Schema (for directly_call_tool only):
{{ .DisplaySchemaSnippet }}
{{ if .Usage }}__USAGE__: {{ .Usage }}
{{ end }}<|TOOL_{{ .Name }}_END_{{ .Nonce }}|>
`

func extractDirectlyCallParamsSchema(schemaSnippet string) aitool.InvokeParams {
	if strings.TrimSpace(schemaSnippet) == "" {
		return nil
	}

	var fullSchema aitool.InvokeParams
	if err := json.Unmarshal([]byte(schemaSnippet), &fullSchema); err != nil {
		return nil
	}

	if paramsSchema := fullSchema.GetObject("properties").GetObject("params"); len(paramsSchema) > 0 {
		return paramsSchema
	}

	if fullSchema.GetString("type") == "object" && len(fullSchema.GetObject("properties")) > 0 {
		return fullSchema
	}

	return nil
}

func extractDirectlyCallParamNamesFromSchema(schemaSnippet string) []string {
	paramsSchema := extractDirectlyCallParamsSchema(schemaSnippet)
	if len(paramsSchema) == 0 {
		return nil
	}

	properties := paramsSchema.GetObject("properties")
	if len(properties) == 0 {
		return nil
	}

	names := make([]string, 0, len(properties))
	for key := range properties {
		names = append(names, key)
	}
	sort.Strings(names)
	return names
}

func renderDirectlyCallParamsSchema(schemaSnippet string) string {
	paramsSchema := extractDirectlyCallParamsSchema(schemaSnippet)
	if len(paramsSchema) == 0 {
		return schemaSnippet
	}

	rendered := omap.NewEmptyOrderedMap[string, any]()
	rendered.Set("$schema", "http://json-schema.org/draft-07/schema#")
	rendered.Set("type", "object")
	rendered.Set("description", "Only for directly_call_tool. Pass this object directly as directly_call_tool_params. Do not include @action, tool, or params wrapper. For multi-line content, use TOOL_PARAM_* AITAG blocks with the literal nonce \""+RecentToolCacheStableNonce+"\" (a fixed string, NOT the per-turn nonce that other tags in this prompt use).")
	if properties, ok := paramsSchema["properties"]; ok {
		rendered.Set("properties", properties)
	}
	if required, ok := paramsSchema["required"]; ok {
		rendered.Set("required", required)
	}
	if additionalProperties, ok := paramsSchema["additionalProperties"]; ok {
		rendered.Set("additionalProperties", additionalProperties)
	}

	jsonBytes, err := json.MarshalIndent(rendered, "", "  ")
	if err != nil {
		return schemaSnippet
	}
	return string(jsonBytes)
}

func (m *AiToolManager) GetRecentToolParamNames() []string {
	m.recentToolsMu.Lock()
	defer m.recentToolsMu.Unlock()
	return m.getRecentToolParamNamesLocked()
}

func (m *AiToolManager) getRecentToolParamNamesLocked() []string {
	if len(m.recentToolsCache) == 0 {
		return nil
	}

	seen := make(map[string]struct{})
	names := make([]string, 0)
	for _, entry := range m.recentToolsCache {
		for _, paramName := range extractDirectlyCallParamNamesFromSchema(entry.SchemaSnippet) {
			if _, ok := seen[paramName]; ok {
				continue
			}
			seen[paramName] = struct{}{}
			names = append(names, paramName)
		}
	}
	sort.Strings(names)
	return names
}

func (m *AiToolManager) GetRecentToolParamNamesByTool(name string) []string {
	m.recentToolsMu.Lock()
	for _, entry := range m.recentToolsCache {
		if entry.Name == name {
			paramNames := extractDirectlyCallParamNamesFromSchema(entry.SchemaSnippet)
			m.recentToolsMu.Unlock()
			return paramNames
		}
	}
	m.recentToolsMu.Unlock()

	tool, err := m.GetToolByName(name)
	if err != nil || tool == nil {
		return nil
	}
	return extractDirectlyCallParamNamesFromSchema(tool.ToJSONSchemaString())
}

// BuildStubToolFromMCPCachePublic is the exported wrapper for buildStubToolFromMCPCache.
// Use this when pre-loading MCP stubs from outside the package (e.g., aireact).
func BuildStubToolFromMCPCachePublic(fullName string, cfg *schema.MCPServerToolConfig) *aitool.Tool {
	return buildStubToolFromMCPCache(fullName, cfg)
}

// buildStubToolFromMCPCache constructs a minimal aitool.Tool from cached MCP metadata.
// The stub carries a real Callback that returns TOOL_INITIALIZING so the AI receives a
// meaningful error and can fall back gracefully. If the MCP server comes back online,
// the live tool (with a real network Callback) takes precedence via the in-memory tool
// list populated by loadMCPServers.
func buildStubToolFromMCPCache(fullName string, cfg *schema.MCPServerToolConfig) *aitool.Tool {
	serverName := cfg.ServerName
	toolName := cfg.ToolName

	// Normalize description to match live tool format: [MCP:server] desc.
	desc := cfg.Description
	prefix := fmt.Sprintf("[MCP:%s] ", serverName)
	if desc == "" {
		desc = fmt.Sprintf("[MCP:%s] Tool from MCP server: %s (not yet connected)", serverName, serverName)
	} else if !strings.HasPrefix(desc, prefix) {
		desc = prefix + desc
	}

	opts := []aitool.ToolOption{
		aitool.WithMCPPendingStub(true),
		aitool.WithDescription(desc),
		aitool.WithKeywords([]string{"mcp", serverName, toolName, "external", "remote"}),
		aitool.WithVerboseName(fmt.Sprintf("%s (MCP:%s)", toolName, serverName)),
		// Callback returns a retryable error so AI can degrade gracefully
		// instead of crashing when the MCP server is still initializing.
		aitool.WithSimpleCallback(func(params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (any, error) {
			msg := fmt.Sprintf(
				"MCP tool %q (server: %s) is not yet available — the MCP server is still connecting or unreachable. "+
					"Please try a different approach or wait for the server to become ready.",
				toolName, serverName,
			)
			fmt.Fprintln(stderr, msg)
			return nil, utils.Errorf("%s %s", MCPToolInitializingErrPrefix, msg)
		}),
	}

	// Reconstruct parameters from cached JSON.
	if cfg.ParamsJSON != "" && cfg.ParamsJSON != "[]" {
		type paramEntry struct {
			Name        string `json:"name"`
			Type        string `json:"type"`
			Description string `json:"description"`
			Default     string `json:"default"`
			Required    bool   `json:"required"`
		}
		var entries []paramEntry
		if err := json.Unmarshal([]byte(cfg.ParamsJSON), &entries); err == nil {
			for _, e := range entries {
				name := e.Name
				paramOpts := []aitool.PropertyOption{
					aitool.WithParam_Description(e.Description),
				}
				if e.Required {
					paramOpts = append(paramOpts, aitool.WithParam_Required(true))
				}
				if e.Default != "" {
					paramOpts = append(paramOpts, aitool.WithParam_Default(e.Default))
				}
				// Cached params are serialized as string type; numeric/boolean params
				// will still work since MCP arguments are passed as interface{} values.
				opts = append(opts, aitool.WithStringParam(name, paramOpts...))
			}
		}
	}

	tool := aitool.NewWithoutCallback(fullName, opts...)
	return tool
}
