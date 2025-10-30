package tool_mocker

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"html/template"
	"io"

	"github.com/yaklang/yaklang/common/ai"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/searchtools"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/jsonextractor"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/mcp"
)

//go:embed mock_prompts/tool_suggestion.txt
var toolSuggestionPrompt string

//go:embed mock_prompts/mock_tool.txt
var mockToolPrompt string

//go:embed mock_prompts/tool_invoke.txt
var toolInvokePrompt string

type ToolSuggestion struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Reason      string `json:"reason"`
}

// ToolParameter represents a parameter for a tool
type ToolParameter struct {
	ParameterName        string `json:"parameter_name"`
	ParameterDescription string `json:"parameter_description"`
	ParameterType        string `json:"parameter_type"` // can be "string", "bool", or "integer"
}

// ToolDefinition represents the structure matching the updated JSON schema
type ToolDefinition struct {
	ToolName        string          `json:"tool_name"`
	ToolDescription string          `json:"tool_description"`
	ToolParameters  []ToolParameter `json:"tool_parameters"`
}

func UnmarshalAiRspToStruct(raw string, v any) error {
	indexes := jsonextractor.ExtractObjectIndexes(raw)
	for _, index := range indexes {
		start, end := index[0], index[1]
		jsonRaw := raw[start:end]
		err := json.Unmarshal([]byte(jsonRaw), v)
		if err != nil {
			continue
		}
		return nil
	}
	return errors.New("no object indexes found")
}

type AiToolMockServer struct {
	aiOptions   []aispec.AIConfigOption
	Suggestions []*ToolSuggestion
	Tools       []*aitool.Tool
	ToolManager *buildinaitools.AiToolManager
}
type SubTaskInfo struct {
	SubTaskName string `json:"subtask_name"`
	SubTaskGoal string `json:"subtask_goal"`
}

func NewMockerSearcher[T searchtools.AISearchable](getter func(ctx context.Context, query string) (T, error)) searchtools.AISearcher[T] {
	return func(query string, searchList []T) ([]T, error) {
		tool, err := getter(context.Background(), query)
		if err != nil {
			return nil, err
		}
		return []T{tool}, nil
	}
}

func NewAiToolMockServer(aiOptions ...aispec.AIConfigOption) *AiToolMockServer {
	mocker := &AiToolMockServer{
		aiOptions: aiOptions,
	}
	mocker.ToolManager = buildinaitools.NewToolManagerByToolGetter(func() []*aitool.Tool {
		allTools := mocker.AllTools()
		factory := aitool.NewFactory()
		factory.RegisterTool("tools_search",
			aitool.WithDescription("Search for more supported tools."),
			aitool.WithStringParam("query",
				aitool.WithParam_Required(true),
				aitool.WithParam_Description("The name of the tool to query, can describe tool requirements using natural language."),
			),
			aitool.WithSimpleCallback(func(params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (any, error) {
				query := params.GetString("query")
				suggestions, err := mocker.QueryToolSuggestion(context.Background(), query)
				if err != nil {
					return nil, err
				}
				return suggestions, nil
			}))
		allTools = append(allTools, factory.Tools()...)
		return allTools
	}, buildinaitools.WithAIToolsSearcher(NewMockerSearcher[*aitool.Tool](func(ctx context.Context, query string) (*aitool.Tool, error) {
		return mocker.SearchTool(ctx, query)
	})), buildinaitools.WithToolEnabled("tools_search", true))
	return mocker
}

func (s *AiToolMockServer) AllTools() []*aitool.Tool {
	allTools := buildinaitools.GetAllTools()
	allTools = funk.Filter(allTools, func(tool *aitool.Tool) bool {
		return tool.Name != "tools_search" && tool.Name != "web_search"
	}).([]*aitool.Tool)
	var descTools []*aitool.Tool
	for _, tool := range s.Suggestions {
		descTools = append(descTools, &aitool.Tool{
			Tool: &mcp.Tool{
				Name:        tool.Name,
				Description: tool.Description,
			},
		})
	}
	allTools = append(allTools, descTools...)
	return allTools
}

func (s *AiToolMockServer) GetToolManager() *buildinaitools.AiToolManager {
	return s.ToolManager
}
func (s *AiToolMockServer) QueryToolSuggestion(ctx context.Context, query string, opts ...aicommon.ConfigOption) ([]*ToolSuggestion, error) {
	var promptBuffer bytes.Buffer
	promptTemplate := template.Must(template.New("query").Parse(toolSuggestionPrompt))
	err := promptTemplate.Execute(&promptBuffer, map[string]any{
		"UserParams": query,
	})
	if err != nil {
		return nil, err
	}
	rspMsg, err := ai.Chat(promptBuffer.String(), s.aiOptions...)
	if err != nil {
		log.Errorf("chat error: %v", err)
		return nil, err
	}
	result := struct {
		Result []*ToolSuggestion `json:"result"`
	}{
		Result: []*ToolSuggestion{},
	}
	err = UnmarshalAiRspToStruct(rspMsg, &result)
	if err != nil {
		return nil, err
	}
	s.Suggestions = append(s.Suggestions, result.Result...)
	return result.Result, nil
}
func (s *AiToolMockServer) DumpMockToolInfos() {
	for _, tool := range s.Tools {
		fmt.Println("tool name: ", tool.Name)
		fmt.Println("tool description: ", tool.Description)
		fmt.Println("tool parameters: ", tool.Params())
		println("--------------------------------")
	}
}
func (s *AiToolMockServer) CallTool(tool *aitool.Tool, params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (any, error) {
	var promptBuffer bytes.Buffer
	promptTemplate := template.Must(template.New("query").Parse(toolInvokePrompt))
	err := promptTemplate.Execute(&promptBuffer, map[string]any{
		"Name":        tool.Name,
		"Description": tool.Description,
		"Params":      params,
	})
	if err != nil {
		return nil, err
	}
	rspMsg, err := ai.Chat(promptBuffer.String(), s.aiOptions...)
	if err != nil {
		log.Errorf("chat error: %v", err)
		return nil, err
	}
	var result struct {
		Result any `json:"result"`
	}
	err = UnmarshalAiRspToStruct(rspMsg, &result)
	if err != nil {
		return nil, err
	}
	return result.Result, nil
}

func (s *AiToolMockServer) SearchTool(ctx context.Context, name string) (*aitool.Tool, error) {
	allTools, err := s.ToolManager.GetEnableTools()
	if err != nil {
		return nil, err
	}
	for _, tool := range allTools {
		if tool.Name == name {
			return tool, nil
		}
	}
	var toolSuggestion *ToolSuggestion
	for _, tool := range s.Suggestions {
		if tool.Name == name {
			toolSuggestion = tool
			break
		}
	}
	if toolSuggestion == nil {
		return nil, errors.New("tool not found")
	}
	var promptBuffer bytes.Buffer
	promptTemplate := template.Must(template.New("query").Parse(mockToolPrompt))
	err = promptTemplate.Execute(&promptBuffer, map[string]any{
		"Name":        toolSuggestion.Name,
		"Description": toolSuggestion.Description,
	})
	if err != nil {
		return nil, err
	}
	rspMsg, err := ai.Chat(promptBuffer.String(), s.aiOptions...)
	if err != nil {
		log.Errorf("chat error: %v", err)
		return nil, err
	}
	var mockDefinition ToolDefinition
	err = UnmarshalAiRspToStruct(rspMsg, &mockDefinition)
	if err != nil {
		return nil, err
	}
	factory := aitool.NewFactory()
	opts := []aitool.ToolOption{aitool.WithDescription(mockDefinition.ToolDescription)}
	for _, param := range mockDefinition.ToolParameters {
		switch param.ParameterType {
		case "string":
			opts = append(opts, aitool.WithStringParam(param.ParameterName, aitool.WithParam_Description(param.ParameterDescription)))
		case "bool":
			opts = append(opts, aitool.WithBoolParam(param.ParameterName, aitool.WithParam_Description(param.ParameterDescription)))
		case "integer":
			opts = append(opts, aitool.WithIntegerParam(param.ParameterName, aitool.WithParam_Description(param.ParameterDescription)))
		default:
			return nil, errors.New("unsupported parameter type: " + param.ParameterType)
		}
	}
	var mockTool *aitool.Tool
	opts = append(opts, aitool.WithSimpleCallback(func(params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (any, error) {
		return s.CallTool(mockTool, params, stdout, stderr)
	}))
	factory.RegisterTool(mockDefinition.ToolName, opts...)
	mockTool = factory.Tools()[0]
	s.Tools = append(s.Tools, mockTool)
	return mockTool, nil
}
