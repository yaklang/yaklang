package aitool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/davecgh/go-spew/spew"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/yaklang/yaklang/common/jsonextractor"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

// ToolInvokeParams 表示工具调用的参数
type ToolInvokeParams struct {
	Tool   string         `json:"tool"`
	Action string         `json:"@action"`
	Params map[string]any `json:"params,omitempty"`
}

// InvokeWithJSON 使用JSON字符串调用工具
func (t *Tool) InvokeWithJSON(jsonStr string, opts ...ToolInvokeOptions) (*ToolResult, error) {
	// 解析JSON
	var params ToolInvokeParams
	if err := json.Unmarshal([]byte(jsonStr), &params); err != nil {
		return &ToolResult{
			Name:        t.Name,
			Description: t.Description,
			Param:       params,
			Success:     false,
			Error:       fmt.Sprintf("JSON解析错误: %v", err),
		}, err
	}

	// 验证工具名称
	if params.Tool != t.Name {
		return &ToolResult{
			Name:        t.Name,
			Description: t.Description,
			Param:       params,
			Success:     false,
			Error:       fmt.Sprintf("工具名称不匹配: 期望 %s, 实际 %s", t.Name, params.Tool),
		}, fmt.Errorf("工具名称不匹配: 期望 %s, 实际 %s", t.Name, params.Tool)
	}

	// 使用参数调用工具
	return t.InvokeWithParams(params.Params, opts...)
}

func (t *Tool) InvokeWithRaw(raw string, opts ...ToolInvokeOptions) (*ToolResult, error) {
	for _, params := range jsonextractor.ExtractStandardJSON(raw) {
		var rawParam = make(map[string]any)
		err := json.Unmarshal([]byte(params), &rawParam)
		if err != nil {
			log.Errorf("parse params failed: %v", err)
			continue
		}
		if utils.MapGetString(rawParam, "tool") != t.Name {
			continue
		}
		actionName := utils.MapGetString(rawParam, "@action")
		if actionName != "" {
			log.Infof("actionName: %s", actionName)
		}
		params := utils.MapGetMapRaw(rawParam, "params")
		result, err := t.InvokeWithParams(params, opts...)
		if result != nil {
			result.Param = params
		}
		return result, err
	}
	return nil, utils.Errorf("no valid params found: %#v", raw)
}

// InvokeWithParams 使用参数映射调用工具
func (t *Tool) InvokeWithParams(params map[string]any, opts ...ToolInvokeOptions) (*ToolResult, error) {
	// 验证参数
	valid, validationErrors := t.ValidateParams(params)
	if !valid {
		return &ToolResult{
			Name:        t.Name,
			Description: t.Description,
			Param:       params,
			Success:     false,
			Error:       fmt.Sprintf("参数验证失败: %v", validationErrors),
		}, fmt.Errorf("参数验证失败: %v", validationErrors)
	}
	if _, ok := params["@action"]; ok {
		delete(params, "@action")
	}
	cfg := NewToolInvokeConfig(context.Background())
	for _, opt := range opts {
		opt(cfg)
	}

	// 执行工具并捕获stdout和stderr
	execResult, err := t.ExecuteToolWithCapture(params, cfg.stdout, cfg.stderr)
	if err != nil {
		return &ToolResult{
			Param:       params,
			Name:        t.Name,
			Description: t.Description,
			Success:     false,
			Error:       fmt.Sprintf("工具执行失败: %v", err),
		}, err
	}

	return &ToolResult{
		Name:        t.Name,
		Description: t.Description,
		Param:       params,
		Success:     true,
		Data:        execResult,
	}, nil
}

// ValidateParams 验证参数
func (t *Tool) validate(iSchema any, params map[string]any) (valid bool, errs []string) {
	trimErrorFirstLine := func(err string) string {
		lines := strings.Split(err, "\n")
		if len(lines) > 0 && strings.HasPrefix(lines[0], "jsonschema validation ") {
			return strings.TrimSpace(strings.Join(lines[1:], "\n"))
		}
		return err
	}
	compiler := jsonschema.NewCompiler()
	err := compiler.AddResource("schema.json", iSchema)
	if err != nil {
		return false, []string{fmt.Sprintf("JSON Schema AddResource failed: %v", trimErrorFirstLine(err.Error()))}
	}
	schema, err := compiler.Compile("schema.json")
	if err != nil {
		spew.Dump(err)
		return false, []string{fmt.Sprintf("JSON Schema Compile: %v", trimErrorFirstLine(err.Error()))}
	}
	applyDefault(schema, params)

	err = schema.Validate(params)
	valid = err == nil
	if !valid {
		validationError := err.(*jsonschema.ValidationError)
		validationErrorStr := trimErrorFirstLine(validationError.Error())
		errs = strings.Split(validationErrorStr, "\n")
	}

	return valid, errs
}

func (t *Tool) ValidateParams(params map[string]any) (bool, []string) {
	return t.validate(t.Tool.InputSchema.ToMap(), params)
}

func (t *Tool) Validate(params map[string]any) (bool, []string) {
	return t.validate(t.ToJSONSchema(), params)
}

// ValidateJSONString 验证JSON字符串是否符合工具的要求
func (t *Tool) ValidateJSONString(jsonStr string) (bool, []string) {
	// 解析JSON
	var data map[string]any
	if err := json.Unmarshal([]byte(jsonStr), &data); err != nil {
		return false, []string{fmt.Sprintf("无法解析JSON: %v", err)}
	}

	// 验证参数
	return t.validate(t.ToJSONSchema(), data)
}

// NewToolFromJSON 从JSON定义创建工具
func NewToolFromJSON(jsonStr string, callback InvokeCallback) (*Tool, error) {
	var toolDef struct {
		Name        string           `json:"name"`
		Description string           `json:"description"`
		Params      []map[string]any `json:"params"`
	}

	if err := json.Unmarshal([]byte(jsonStr), &toolDef); err != nil {
		return nil, fmt.Errorf("解析工具定义失败: %v", err)
	}

	// 创建工具
	tool, err := New(toolDef.Name,
		WithDescription(toolDef.Description),
		WithCallback(callback),
	)

	if err != nil {
		return nil, err
	}

	for _, paramDef := range toolDef.Params {
		name := utils.MapGetString(paramDef, "name")
		tool.Tool.InputSchema.Properties[name] = paramDef
		if required, ok := paramDef["required"].(bool); ok && required {
			delete(paramDef, "required")
			tool.Tool.InputSchema.Required = append(tool.Tool.InputSchema.Required, name)
		}
	}

	return tool, nil
}

func applyDefault(schema *jsonschema.Schema, params map[string]any) {
	for key, prop := range schema.Properties {
		// 检查字段是否存在
		if _, exists := params[key]; !exists && prop.Default != nil {
			params[key] = *prop.Default
		}

		// 处理嵌套对象
		types := prop.Types.String()
		if types == "[object]" && prop.Properties != nil {
			subParams, ok := params[key].(map[string]any)
			if ok {
				applyDefault(prop, subParams)
			}
		}

		// 处理数组
		var (
			arraySchema *jsonschema.Schema
			realParams  []any
			ok          bool
		)
		if types == "[array]" {
			if prop.Items2020 != nil {
				realParams, ok = params[key].([]any)
				if ok {
					arraySchema = prop.Items2020
				}
			} else if prop.Items != nil {
				switch items := prop.Items.(type) {
				case *jsonschema.Schema:
					realParams, ok = params[key].([]any)
					if ok {
						arraySchema = items
					}
				case []*jsonschema.Schema:
					// TODO handle this case
				}
			}
		}

		if arraySchema != nil && len(realParams) > 0 {
			for _, item := range realParams {
				if realParamMap, ok := item.(map[string]any); ok {
					applyDefault(arraySchema, realParamMap)
				}
			}
		}
	}
}
