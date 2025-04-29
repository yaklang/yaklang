package tool_mocker

import (
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

type MockToolManager struct {
	handleGetAllTools      func() ([]*aitool.Tool, error)
	handleSearchTools      func(method string, query string) ([]*aitool.Tool, error)
	handleSearchToolByName func(name string) (*aitool.Tool, error)
}

func NewMockToolManager(handleGetAllTools func() ([]*aitool.Tool, error), handleSearchTools func(method string, query string) ([]*aitool.Tool, error), handleSearchToolByName func(name string) (*aitool.Tool, error)) *MockToolManager {
	return &MockToolManager{
		handleGetAllTools:      handleGetAllTools,
		handleSearchTools:      handleSearchTools,
		handleSearchToolByName: handleSearchToolByName,
	}
}

func (m *MockToolManager) SearchTools(method string, query string) ([]*aitool.Tool, error) {
	return m.handleSearchTools(method, query)
}

func (m *MockToolManager) GetAllTools() ([]*aitool.Tool, error) {
	return m.handleGetAllTools()
}

func (m *MockToolManager) SearchToolByName(name string) (*aitool.Tool, error) {
	return m.handleSearchToolByName(name)
}
