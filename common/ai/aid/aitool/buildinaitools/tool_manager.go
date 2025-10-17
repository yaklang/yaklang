package buildinaitools

import (
	"fmt"

	"github.com/samber/lo"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/searchtools"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

// AiToolManager 是工具管理器的默认实现
type AiToolManager struct {
	toolsGetter           func() []*aitool.Tool
	toolEnabled           map[string]bool // 记录工具是否开启
	enableSearchTool      bool            // 是否开启工具搜索
	enableForgeSearchTool bool            // 是否开启forge工具搜索
	aiToolsSearcher       searchtools.AISearcher[*aitool.Tool]
	aiForgeSearcher       searchtools.AISearcher[*schema.AIForge]
	disableTools          map[string]struct{} // 禁用的工具列表 优先级最高
	searchTool            []*aitool.Tool
	forgeSearchTool       []*aitool.Tool
}

// ToolManagerOption 定义工具管理器的配置选项
type ToolManagerOption func(*AiToolManager)

// WithAIToolsSearcher 设置搜索器
func WithAIToolsSearcher(searcher searchtools.AISearcher[*aitool.Tool]) ToolManagerOption {
	return func(m *AiToolManager) {
		m.aiToolsSearcher = searcher
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
		toolsGetter: getter,
		toolEnabled: make(map[string]bool),
	}

	// 应用选项
	for _, option := range options {
		option(manager)
	}

	return manager
}

// NewToolManager 创建一个新的默认工具管理器实例
func NewToolManager(options ...ToolManagerOption) *AiToolManager {
	manager := NewToolManagerByToolGetter(GetAllTools, options...)
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
		if m.toolEnabled[tool.Name] {
			enabledTools = append(enabledTools, tool)
		}
	}
	if m.enableSearchTool {
		if m.aiToolsSearcher == nil {
			log.Errorf("searcher is not set")
			return enabledTools, nil
		}
		tool, err := m.getSearchTools()
		if err != nil {
			return nil, err
		}
		enabledTools = append(enabledTools, tool...)
	}
	if m.enableForgeSearchTool {
		if m.aiToolsSearcher == nil {
			log.Errorf("searcher is not set")
			return enabledTools, nil
		}
		tool, err := m.getForgeSearchTools()
		if err != nil {
			return nil, err
		}
		enabledTools = append(enabledTools, tool...)
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
	return nil, fmt.Errorf("找不到名为 %s 的工具", name)
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
