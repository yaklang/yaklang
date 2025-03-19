package taskstack

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"

	"github.com/yaklang/yaklang/common/jsonextractor"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

// ToolResult 表示工具调用的结果
type ToolResult struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Param       any    `json:"param"`
	Success     bool   `json:"success"`
	Data        any    `json:"data,omitempty"`
	Error       string `json:"error,omitempty"`
}

func (t *ToolResult) QuoteName() string {
	return strconv.Quote(t.Name)
}

func (t *ToolResult) QuoteDescription() string {
	return strconv.Quote(t.Description)
}

func (t *ToolResult) QuoteError() string {
	return strconv.Quote(t.Error)
}

func (t *ToolResult) QuoteResult() string {
	raw, _ := json.Marshal(t.Data)
	return string(raw)
}

func (t *ToolResult) QuoteParams() string {
	raw, _ := json.Marshal(t.Param)
	return string(raw)
}

func (t *ToolResult) Dump() string {
	bytes, _ := json.Marshal(t)
	return string(bytes)
}

// ToolInvokeParams 表示工具调用的参数
type ToolInvokeParams struct {
	Tool   string                 `json:"tool"`
	Action string                 `json:"@action"`
	Params map[string]interface{} `json:"params,omitempty"`
}

// InvokeWithJSON 使用JSON字符串调用工具
func (t *Tool) InvokeWithJSON(jsonStr string) (*ToolResult, error) {
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
	return t.InvokeWithParams(params.Params)
}

func (t *Tool) InvokeWithRaw(raw string) (*ToolResult, error) {
	for _, params := range jsonextractor.ExtractStandardJSON(raw) {
		var rawParam = make(map[string]interface{})
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
		result, err := t.InvokeWithParams(params)
		if result != nil {
			result.Param = params
		}
		return result, err
	}
	return nil, utils.Errorf("no valid params found: %#v", raw)
}

// InvokeWithParams 使用参数映射调用工具
func (t *Tool) InvokeWithParams(params map[string]interface{}) (*ToolResult, error) {
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

	// 执行工具并捕获stdout和stderr
	execResult, err := t.ExecuteToolWithCapture(params)
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
func (t *Tool) ValidateParams(params map[string]interface{}) (bool, []string) {
	errors := []string{}

	// 检查必须的参数
	for _, param := range t.Params {
		if param.Required {
			if value, exists := params[param.Name]; !exists || value == nil {
				errors = append(errors, fmt.Sprintf("缺少必须的参数: %s", param.Name))
			}
		}
	}

	// 验证参数类型
	for _, param := range t.Params {
		if value, exists := params[param.Name]; exists && value != nil {
			// 获取参数值的类型
			valueType := reflect.TypeOf(value).Kind().String()

			// 检查类型匹配
			switch param.Type {
			case "string":
				if valueType != "string" {
					errors = append(errors, fmt.Sprintf("参数 %s 应为字符串类型，实际为 %s", param.Name, valueType))
				}
			case "number", "integer":
				if valueType != "float64" && valueType != "int" && valueType != "int64" && valueType != "float32" {
					errors = append(errors, fmt.Sprintf("参数 %s 应为数字类型，实际为 %s", param.Name, valueType))
				}
			case "boolean":
				if valueType != "bool" {
					errors = append(errors, fmt.Sprintf("参数 %s 应为布尔类型，实际为 %s", param.Name, valueType))
				}
			case "array":
				if !isArrayType(value) {
					errors = append(errors, fmt.Sprintf("参数 %s 应为数组类型，实际为 %s", param.Name, valueType))
				} else if len(param.ArrayItem) > 0 {
					// 验证数组元素类型
					expectedItemType := param.ArrayItem[0].Type
					items := reflect.ValueOf(value)

					for i := 0; i < items.Len(); i++ {
						item := items.Index(i).Interface()
						itemType := reflect.TypeOf(item).Kind().String()

						switch expectedItemType {
						case "string":
							if itemType != "string" {
								errors = append(errors, fmt.Sprintf("参数 %s 的数组项[%d]应为字符串类型，实际为 %s", param.Name, i, itemType))
							}
						case "number", "integer":
							if itemType != "float64" && itemType != "int" && itemType != "int64" && itemType != "float32" {
								errors = append(errors, fmt.Sprintf("参数 %s 的数组项[%d]应为数字类型，实际为 %s", param.Name, i, itemType))
							}
						case "boolean":
							if itemType != "bool" {
								errors = append(errors, fmt.Sprintf("参数 %s 的数组项[%d]应为布尔类型，实际为 %s", param.Name, i, itemType))
							}
						case "array":
							if !isArrayType(item) {
								errors = append(errors, fmt.Sprintf("参数 %s 的数组项[%d]应为数组类型，实际为 %s", param.Name, i, itemType))
							}
							// 这里可以进一步验证嵌套数组的元素类型，但为简化起见，省略
						case "object":
							if itemType != "map" {
								errors = append(errors, fmt.Sprintf("参数 %s 的数组项[%d]应为对象类型，实际为 %s", param.Name, i, itemType))
							}
						}
					}
				}
			case "object":
				if valueType != "map" {
					errors = append(errors, fmt.Sprintf("参数 %s 应为对象类型，实际为 %s", param.Name, valueType))
				}
			}
		} else if param.Required {
			// 如果参数是必需的但不存在，添加错误
			errors = append(errors, fmt.Sprintf("缺少必需参数: %s", param.Name))
		} else if !exists && param.Default != nil {
			// 如果参数不存在但有默认值，使用默认值
			params[param.Name] = param.Default
		}
	}

	// 如果有错误，返回false
	if len(errors) > 0 {
		return false, errors
	}

	return true, nil
}

// isArrayType 检查值是否为数组类型
func isArrayType(value interface{}) bool {
	kind := reflect.TypeOf(value).Kind()
	return kind == reflect.Array || kind == reflect.Slice
}

// ValidateJSONString 验证JSON字符串是否符合工具的要求
func (t *Tool) ValidateJSONString(jsonStr string) (bool, []string) {
	// 解析JSON
	var data map[string]interface{}
	if err := json.Unmarshal([]byte(jsonStr), &data); err != nil {
		return false, []string{fmt.Sprintf("无法解析JSON: %v", err)}
	}

	// 检查工具名称
	toolName, ok := data["tool"].(string)
	if !ok || toolName != t.Name {
		return false, []string{fmt.Sprintf("工具名称不匹配: 期望 %s, 实际 %s", t.Name, toolName)}
	}

	// 检查action
	action, ok := data["@action"].(string)
	if !ok || action != "invoke" {
		return false, []string{"@action 字段缺失或不是 'invoke'"}
	}

	// 检查params
	params, ok := data["params"].(map[string]interface{})
	if !ok {
		return false, []string{"params 字段缺失或不是对象"}
	}

	// 验证参数
	return t.ValidateParams(params)
}

// NewToolFromJSON 从JSON定义创建工具
func NewToolFromJSON(jsonStr string, callback InvokeCallback) (*Tool, error) {
	var toolDef struct {
		Name        string                   `json:"name"`
		Description string                   `json:"description"`
		Params      []map[string]interface{} `json:"params"`
	}

	if err := json.Unmarshal([]byte(jsonStr), &toolDef); err != nil {
		return nil, fmt.Errorf("解析工具定义失败: %v", err)
	}

	// 创建工具
	tool, err := NewTool(toolDef.Name,
		WithTool_Description(toolDef.Description),
		WithTool_Callback(callback),
	)

	if err != nil {
		return nil, err
	}

	// 添加参数
	for _, paramDef := range toolDef.Params {
		// 从map中提取参数信息
		name, _ := paramDef["name"].(string)
		paramType, _ := paramDef["type"].(string)
		description, _ := paramDef["description"].(string)
		required, _ := paramDef["required"].(bool)
		defaultValue := paramDef["default"]

		// 创建参数选项
		paramOptions := []ToolParamOption{
			WithTool_ParamDescription(description),
			WithTool_ParamRequired(required),
		}

		if defaultValue != nil {
			paramOptions = append(paramOptions, WithTool_ParamDefault(defaultValue))
		}

		// 处理数组类型
		if paramType == "array" && paramDef["items"] != nil {
			items, ok := paramDef["items"].(map[string]interface{})
			if ok {
				itemType, _ := items["type"].(string)
				itemDesc, _ := items["description"].(string)

				arrayItemOptions := []ToolParamValueOption{
					WithTool_ValueDescription(itemDesc),
				}

				if items["default"] != nil {
					arrayItemOptions = append(arrayItemOptions, WithTool_ValueDefault(items["default"]))
				}

				paramOptions = append(paramOptions, WithTool_ArrayItem(
					NewToolParamValue(itemType, arrayItemOptions...),
				))
			}
		}

		param := NewToolParam(name, paramType, paramOptions...)
		tool.Params = append(tool.Params, param)
	}

	return tool, nil
}
