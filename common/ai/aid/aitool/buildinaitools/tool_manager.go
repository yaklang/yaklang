package buildinaitools

import (
	"fmt"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

// ToolManager 是工具管理器的接口
// 提供查询所有工具和查询特定工具的功能
type ToolManager interface {
	// GetAllTools 获取所有可用的工具
	GetAllTools() ([]*aitool.Tool, error)
	// SearchToolByName 通过工具名搜索工具
	SearchToolByName(name string) (*aitool.Tool, error)
	// SearchTools 通过字符串搜索相关工具
	SearchTools(method string, query string) ([]*aitool.Tool, error)
}

var _ ToolManager = &DefaultToolManager{}

// DefaultToolManager 是工具管理器的默认实现
type DefaultToolManager struct {
	tools []*aitool.Tool
}

// NewDefaultToolManager 创建一个新的默认工具管理器实例
func NewDefaultToolManager(tools []*aitool.Tool) *DefaultToolManager {
	return &DefaultToolManager{
		tools: tools,
	}
}

// NewToolManagerWithTools 使用指定的工具列表创建工具管理器
func NewToolManagerWithTools(tools []*aitool.Tool) *DefaultToolManager {
	return &DefaultToolManager{
		tools: tools,
	}
}

// GetAllTools 获取所有可用的工具
func (m *DefaultToolManager) GetAllTools() ([]*aitool.Tool, error) {
	return m.tools, nil
}

// GetToolByName 通过工具名获取特定工具
func (m *DefaultToolManager) GetToolByName(name string) (*aitool.Tool, error) {
	for _, tool := range m.tools {
		if tool.Name == name {
			return tool, nil
		}
	}
	return nil, fmt.Errorf("找不到名为 %s 的工具", name)
}

// SearchTools 通过字符串搜索相关工具
func (m *DefaultToolManager) SearchTools(method string, query string) ([]*aitool.Tool, error) {
	switch method {
	case "name":
		tool, err := m.SearchToolByName(query)
		if err != nil {
			return nil, err
		}
		return []*aitool.Tool{tool}, nil
	default:
		tool, err := m.SearchToolByName(query)
		if err != nil {
			return nil, err
		}
		return []*aitool.Tool{tool}, nil
	}
}

func (m *DefaultToolManager) SearchToolByName(query string) (*aitool.Tool, error) {
	for _, tool := range m.tools {
		if tool.Name == query {
			return tool, nil
		}
	}
	return nil, fmt.Errorf("找不到名为 %s 的工具", query)
}
