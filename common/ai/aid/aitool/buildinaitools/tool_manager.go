package buildinaitools

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/samber/lo"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/searchtools"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/yakscripttools"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

const defaultRecentToolCacheMaxBytes = 20480 // 20KB

// RecentToolEntry records a recently used tool for directly_call_tool action.
type RecentToolEntry struct {
	Name          string `json:"name"`
	Description   string `json:"description"`
	SchemaSnippet string `json:"schema_snippet"`
	Usage         string `json:"usage"`
	Size          int    `json:"size"`
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

	recentToolsCache []*RecentToolEntry
	recentToolsMu    sync.Mutex
	maxCacheBytes    int
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
		}, searchtools.SearchForgeName)
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
		aiToolSearchTools, err := searchtools.CreateAISearchTools(m.aiToolsSearcher, m.safeToolsGetter, searchtools.SearchToolName)
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

	// 从数据库中查找工具
	toolFromDB, err := yakit.GetAIYakTool(consts.GetGormProfileDatabase(), name)
	if err != nil {
		return nil, fmt.Errorf("cannot found [%v] neithor in database nor in enable tools: %v", name, err)
	}

	// 将 schema.AIYakTool 转换为 aitool.Tool
	convertedTools := yakscripttools.ConvertTools([]*schema.AIYakTool{toolFromDB})
	if len(convertedTools) == 0 {
		log.Errorf("convert tool [%s] from database failed", name)
		return nil, fmt.Errorf("convert tool [%v] from database failed", name)
	}

	return convertedTools[0], nil
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
}

// DisableTool 关闭单个工具
func (m *AiToolManager) DisableTool(name string) {
	m.toolEnabled[name] = false
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

func (m *AiToolManager) getMaxCacheBytes() int {
	if m.maxCacheBytes > 0 {
		return m.maxCacheBytes
	}
	return defaultRecentToolCacheMaxBytes
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
func (m *AiToolManager) AddRecentlyUsedTool(tool *aitool.Tool) {
	if tool == nil {
		return
	}
	m.recentToolsMu.Lock()
	defer m.recentToolsMu.Unlock()

	name := tool.GetName()
	desc := tool.GetDescription()
	schemaStr := tool.ToJSONSchemaString()
	usage := tool.GetUsage()
	entrySize := len(name) + len(desc) + len(schemaStr) + len(usage)

	// remove existing entry with same name (will be re-appended at tail)
	filtered := make([]*RecentToolEntry, 0, len(m.recentToolsCache))
	for _, e := range m.recentToolsCache {
		if e.Name != name {
			filtered = append(filtered, e)
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

	maxBytes := m.getMaxCacheBytes()
	for m.totalCacheSize() > maxBytes && len(m.recentToolsCache) > 1 {
		m.recentToolsCache = m.recentToolsCache[1:]
	}
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

const recentToolEntryTemplate = `<|TOOL_{{ .Name }}_{{ .Nonce }}|>
## Tool: {{ .Name }}
Description: {{ .Description }}
Params Schema:
{{ .SchemaSnippet }}
{{ if .Usage }}__USAGE__: {{ .Usage }}
{{ end }}<|TOOL_{{ .Name }}_END_{{ .Nonce }}|>

`

const recentToolSummaryFooter = `## How to use directly_call_tool

To call a tool listed above, respond with:
{"@action": "directly_call_tool", "directly_call_tool_name": "<name>", "directly_call_identifier": "<snake_case_intent>", "directly_call_expectations": "<timing and fallback>", "directly_call_tool_params": <params object matching the Params Schema>}
`

// ExportRecentToolCache serializes the recent-tool cache entries to a JSON string
// for persistent storage (e.g. in AIAgentRuntime.RecentToolsCache).
func (m *AiToolManager) ExportRecentToolCache() string {
	m.recentToolsMu.Lock()
	defer m.recentToolsMu.Unlock()

	if len(m.recentToolsCache) == 0 {
		return ""
	}
	raw, err := json.Marshal(m.recentToolsCache)
	if err != nil {
		log.Errorf("ExportRecentToolCache marshal error: %v", err)
		return ""
	}
	return string(raw)
}

// ImportRecentToolCache restores cache entries from a JSON string produced by ExportRecentToolCache.
// Existing entries with the same name are replaced; FIFO eviction still applies.
func (m *AiToolManager) ImportRecentToolCache(jsonStr string) {
	if jsonStr == "" {
		return
	}
	var entries []*RecentToolEntry
	if err := json.Unmarshal([]byte(jsonStr), &entries); err != nil {
		log.Errorf("ImportRecentToolCache unmarshal error: %v", err)
		return
	}

	m.recentToolsMu.Lock()
	defer m.recentToolsMu.Unlock()

	existing := make(map[string]struct{})
	for _, e := range m.recentToolsCache {
		existing[e.Name] = struct{}{}
	}
	for _, entry := range entries {
		if _, ok := existing[entry.Name]; ok {
			continue
		}
		m.recentToolsCache = append(m.recentToolsCache, entry)
		existing[entry.Name] = struct{}{}
	}

	maxBytes := m.getMaxCacheBytes()
	for m.totalCacheSize() > maxBytes && len(m.recentToolsCache) > 1 {
		m.recentToolsCache = m.recentToolsCache[1:]
	}
}

// GetRecentToolsSummary builds a prompt-friendly summary of cached tools within maxBytes.
// Each tool is wrapped in AITAG boundaries <|TOOL_{name}_{nonce}|> to prevent confusion.
func (m *AiToolManager) GetRecentToolsSummary(maxBytes int, nonce string) string {
	m.recentToolsMu.Lock()
	defer m.recentToolsMu.Unlock()

	if len(m.recentToolsCache) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("# Recently Used Tools (available for directly_call_tool)\n\n")
	totalLen := sb.Len()
	entryWritten := false
	for i, entry := range m.recentToolsCache {
		block := utils.MustRenderTemplate(recentToolEntryTemplate, map[string]interface{}{
			"Name":          entry.Name,
			"Nonce":         nonce,
			"Description":   entry.Description,
			"SchemaSnippet": entry.SchemaSnippet,
			"Usage":         entry.Usage,
		})
		// always include at least the first entry even if it exceeds maxBytes
		if i > 0 && maxBytes > 0 && totalLen+len(block) > maxBytes {
			break
		}
		sb.WriteString(block)
		totalLen += len(block)
		entryWritten = true
	}
	if !entryWritten {
		return ""
	}
	sb.WriteString(recentToolSummaryFooter)
	return sb.String()
}
