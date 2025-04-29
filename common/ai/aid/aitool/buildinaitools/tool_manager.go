package buildinaitools

import (
	"fmt"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/searchtools"
	"github.com/yaklang/yaklang/common/log"
)

// AiToolManager 是工具管理器的默认实现
type AiToolManager struct {
	toolsGetter  func() []*aitool.Tool
	toolEnabled  map[string]bool // 记录工具是否开启
	enableSearch bool            // 是否开启工具搜索
	searcher     searchtools.AiToolSearcher
}

// ToolManagerOption 定义工具管理器的配置选项
type ToolManagerOption func(*AiToolManager)

// WithSearcher 设置搜索器
func WithSearcher(searcher searchtools.AiToolSearcher) ToolManagerOption {
	return func(m *AiToolManager) {
		m.searcher = searcher
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

// WithSearchEnabled 设置是否开启工具搜索
func WithSearchEnabled(enabled bool) ToolManagerOption {
	return func(m *AiToolManager) {
		m.enableSearch = enabled
	}
}
func NewToolManagerByToolGetter(getter func() []*aitool.Tool, options ...ToolManagerOption) *AiToolManager {
	manager := &AiToolManager{
		toolsGetter:  getter,
		toolEnabled:  make(map[string]bool),
		enableSearch: true, // 默认开启搜索
	}

	// 应用选项
	for _, option := range options {
		option(manager)
	}

	return manager
}

// NewToolManager 创建一个新的默认工具管理器实例
func NewToolManager(tools []*aitool.Tool, options ...ToolManagerOption) *AiToolManager {
	allTools := GetAllTools()
	toolsMap := map[string]*aitool.Tool{}
	for _, tool := range allTools {
		toolsMap[tool.Name] = tool
	}
	var extTools []*aitool.Tool
	for _, tool := range tools {
		if _, ok := toolsMap[tool.Name]; !ok {
			extTools = append(extTools, tool)
		}
	}
	manager := NewToolManagerByToolGetter(func() []*aitool.Tool {
		return append(GetAllTools(), extTools...)
	}, options...)
	for _, tool := range tools {
		manager.EnableTool(tool.Name)
	}
	return manager
}

// GetAllTools 获取所有可用的工具
func (m *AiToolManager) GetAllTools() ([]*aitool.Tool, error) {
	var enabledTools []*aitool.Tool
	for _, tool := range m.toolsGetter() {
		if m.toolEnabled[tool.Name] {
			enabledTools = append(enabledTools, tool)
		}
	}
	if m.enableSearch {
		if m.searcher == nil {
			log.Errorf("searcher is not set")
			return enabledTools, nil
		}
		tool, err := searchtools.CreateAiToolsSearchTools(m.toolsGetter, func(req *searchtools.ToolSearchRequest) ([]*aitool.Tool, error) {
			req.Tools = m.toolsGetter()
			res, err := m.searcher(req)
			for _, tool := range res {
				m.EnableTool(tool.Name)
			}
			if err != nil {
				return nil, err
			}
			return res, nil
		})
		if err != nil {
			return nil, err
		}
		enabledTools = append(enabledTools, tool...)
	}
	return enabledTools, nil
}

// GetToolByName 通过工具名获取特定工具
func (m *AiToolManager) GetToolByName(name string) (*aitool.Tool, error) {
	tools, err := m.GetAllTools()
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
	if !m.enableSearch {
		return nil, fmt.Errorf("工具搜索功能已被禁用")
	}

	return m.searcher(&searchtools.ToolSearchRequest{
		Query: query,
	})
}

// IsToolEnabled 检查工具是否开启
func (m *AiToolManager) IsToolEnabled(name string) bool {
	return m.toolEnabled[name]
}

// EnableTool 开启单个工具
func (m *AiToolManager) EnableTool(name string) {
	m.toolEnabled[name] = true
}

// DisableTool 关闭单个工具
func (m *AiToolManager) DisableTool(name string) {
	m.toolEnabled[name] = false
}
