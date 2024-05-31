package qwen

import (
	"encoding/json"
)

// ======= parameters dtype =======
// Function describes a function tool with its name, description, and parameters.

type PropertieDefinition struct {
	Type        string `json:"type"`
	Description string `json:"description"`
}

type ToolCallParameter struct {
	Type       string                         `json:"type"`
	Properties map[string]PropertieDefinition `json:"properties"`
}

type ToolFunction struct {
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Parameters  ToolCallParameter `json:"parameters,omitempty"` // Using interface{} to handle both empty parameters and structured ones
	Required    []string          `json:"required,omitempty"`
}

// Tool represents a tool with its type and function details.
type Tool struct {
	Type     string       `json:"type"`
	Function ToolFunction `json:"function"`
}

// ======= function_call message =======

// type TollCallArgument struct {
// 	Properties map[string]string `json:"properties"`
// }

type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// API 接口返回的是 string, 这里转换为 map.
func (f FunctionCall) GetArguments() map[string]string {
	argMap := make(map[string]string)
	err := json.Unmarshal([]byte(f.Arguments), &argMap)
	if err != nil {
		panic(err)
	}

	return argMap
}

type ToolCalls struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function FunctionCall `json:"function"`
}
