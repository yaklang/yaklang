package aitool

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils/omap"

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

// InvokeWithOrderedJSON 使用JSON字符串调用工具，并保持参数顺序
func (t *Tool) InvokeWithOrderedJSON(jsonStr string, opts ...ToolInvokeOptions) (*ToolResult, error) {
	// 使用 OrderedMap 解析 JSON 以保持顺序
	orderedParams := omap.NewEmptyOrderedMap[string, any]()
	err := orderedParams.UnmarshalJSON([]byte(jsonStr))
	if err != nil {
		return &ToolResult{
			Name:        t.Name,
			Description: t.Description,
			Param:       nil,
			Success:     false,
			Error:       fmt.Sprintf("JSON解析失败: %v", err),
		}, fmt.Errorf("JSON解析失败: %v", err)
	}

	// 提取工具名称
	toolVal, hasTool := orderedParams.Get("tool")
	if !hasTool {
		return &ToolResult{
			Name:        t.Name,
			Description: t.Description,
			Param:       nil,
			Success:     false,
			Error:       "缺少 tool 字段",
		}, fmt.Errorf("缺少 tool 字段")
	}

	toolName, ok := toolVal.(string)
	if !ok {
		return &ToolResult{
			Name:        t.Name,
			Description: t.Description,
			Param:       nil,
			Success:     false,
			Error:       "tool 字段类型错误",
		}, fmt.Errorf("tool 字段类型错误")
	}

	// 验证工具名称
	if toolName != t.Name {
		return &ToolResult{
			Name:        t.Name,
			Description: t.Description,
			Param:       nil,
			Success:     false,
			Error:       fmt.Sprintf("工具名称不匹配: 期望 %s, 实际 %s", t.Name, toolName),
		}, fmt.Errorf("工具名称不匹配: 期望 %s, 实际 %s", t.Name, toolName)
	}

	// 提取参数
	paramsVal, hasParams := orderedParams.Get("params")
	if !hasParams {
		// 如果没有 params 字段，使用空的 OrderedMap
		return t.InvokeWithOrderedParams(omap.NewEmptyOrderedMap[string, any](), opts...)
	}

	// 尝试将参数转换为 OrderedMap
	var toolParams *omap.OrderedMap[string, any]
	switch p := paramsVal.(type) {
	case *omap.OrderedMap[string, any]:
		toolParams = p
	case map[string]any:
		// 如果是普通 map，转换为 OrderedMap（会丢失顺序，但至少能工作）
		toolParams = omap.NewEmptyOrderedMap[string, any]()
		for k, v := range p {
			toolParams.Set(k, v)
		}
	default:
		return &ToolResult{
			Name:        t.Name,
			Description: t.Description,
			Param:       nil,
			Success:     false,
			Error:       "params 字段类型错误",
		}, fmt.Errorf("params 字段类型错误")
	}

	// 使用有序参数调用工具
	return t.InvokeWithOrderedParams(toolParams, opts...)
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
	cfg := NewToolInvokeConfig()
	for _, opt := range opts {
		opt(cfg)
	}
	execResult, err := t.ExecuteToolWithCapture(cfg.ctx, params, cfg)
	if err != nil {
		if cb := cfg.GetErrCallback(); cb != nil {
			return cb(err)
		}
		return &ToolResult{
			Param:       params,
			Name:        t.Name,
			Description: t.Description,
			Success:     false,
			Error:       fmt.Sprintf("工具执行失败: %v", err),
		}, err
	}

	handleLargeContent(&execResult.Stdout, "stdout", func(s string) {
		log.Infof("large stdout content saved to file: %v", s)
	})
	handleLargeContent(&execResult.Stderr, "stderr", func(filename string) {
		log.Infof("large stderr content saved to file: %s", filename)
	})

	if jsonResultRaw := utils.Jsonify(execResult.Result); len(jsonResultRaw) > 10*1024 {
		originJsonResult := string(jsonResultRaw)
		jsonResult := utils.ShrinkString(originJsonResult, 2000)
		filename := handleLargeContentToFile(originJsonResult, "json")
		execResult.Result = fmt.Sprintf("%s (total: %v, saved in file[%v]) see file use some other filesystem tool",
			jsonResult, len(originJsonResult), filename)
		log.Infof("large json result content saved to file: %s", filename)
	}

	if cb := cfg.resCallback; cb != nil {
		return cb(execResult)
	}

	return &ToolResult{
		Name:        t.Name,
		Description: t.Description,
		Param:       params,
		Success:     true,
		Data:        execResult,
	}, nil
}

// InvokeWithOrderedParams 使用 OrderedMap 参数调用工具
func (t *Tool) InvokeWithOrderedParams(params *omap.OrderedMap[string, any], opts ...ToolInvokeOptions) (*ToolResult, error) {
	// 验证参数
	valid, validationErrors := t.ValidateOrderedParams(params)
	if !valid {
		// 转换为普通 map 用于结果显示
		paramMap := make(map[string]any)
		if params != nil {
			params.ForEach(func(k string, v any) bool {
				paramMap[k] = v
				return true
			})
		}
		return &ToolResult{
			Name:        t.Name,
			Description: t.Description,
			Param:       paramMap,
			Success:     false,
			Error:       fmt.Sprintf("参数验证失败: %v", validationErrors),
		}, fmt.Errorf("参数验证失败: %v", validationErrors)
	}

	// 转换为普通 map 用于执行
	paramMap := make(map[string]any)
	if params != nil {
		params.ForEach(func(k string, v any) bool {
			paramMap[k] = v
			return true
		})
	}

	// 调用已有的实现
	return t.InvokeWithParams(paramMap, opts...)
}

// handleLargeContent 处理大文本内容，将其截断并保存到临时文件
// content: 要处理的内容指针
// contentType: 内容类型(stdout/stderr/json)
// logCallback: 可选的日志回调函数
func handleLargeContent(content *string, contentType string, logCallback func(string)) {
	if len(*content) <= 10*1024 {
		return
	}

	origContent := *content
	newData := utils.ShrinkString(origContent, 1024)
	filename := handleLargeContentToFile(origContent, contentType)

	newData += fmt.Sprintf(
		"\n___________\n"+
			" (total: %v, saved in file[%v]) see file use some other filesystem tool\n"+
			"___________",
		utils.ByteSize(uint64(len(origContent))),
		filename)
	*content = newData

	if logCallback != nil {
		logCallback(filename)
	}
}

// handleLargeContentToFile 将大文本内容保存到临时文件并返回文件名
func handleLargeContentToFile(content string, contentType string) string {
	filename := fmt.Sprintf("*-result.%s.txt", contentType)
	fp, err := consts.TempFile(filename)
	if err != nil {
		return consts.TempFileFast(content)
	}

	fp.Write([]byte(content))
	fp.Close()
	return fp.Name()
}

// ValidateParams 验证参数 - 内部方法支持两种参数类型
func (t *Tool) validate(iSchema any, params any) (valid bool, errs []string) {
	// 将参数转换为普通 map 用于验证
	var paramMap map[string]any
	switch p := params.(type) {
	case *omap.OrderedMap[string, any]:
		paramMap = make(map[string]any)
		if p != nil {
			p.ForEach(func(k string, v any) bool {
				paramMap[k] = v
				return true
			})
		}
	case map[string]any:
		paramMap = p
	default:
		return false, []string{fmt.Sprintf("unsupported params type: %T", params)}
	}
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
	applyDefault(schema, paramMap)

	err = schema.Validate(paramMap)
	valid = err == nil
	if !valid {
		validationError := err.(*jsonschema.ValidationError)
		validationErrorStr := trimErrorFirstLine(validationError.Error())
		errs = strings.Split(validationErrorStr, "\n")
	}

	return valid, errs
}

// ValidateParams 验证普通 map 参数
func (t *Tool) ValidateParams(params map[string]any) (bool, []string) {
	return t.validateWithSchema(params)
}

// ValidateOrderedParams 验证 OrderedMap 参数
func (t *Tool) ValidateOrderedParams(params *omap.OrderedMap[string, any]) (bool, []string) {
	return t.validateWithSchema(params)
}

// validateWithSchema 内部通用验证方法
func (t *Tool) validateWithSchema(params any) (bool, []string) {
	// Convert OrderedMap to regular map for JSON schema validation
	// First serialize to JSON then deserialize to get plain Go structures
	schemaMap := t.Tool.InputSchema.ToMap()
	jsonBytes, err := json.Marshal(schemaMap)
	if err != nil {
		return false, []string{fmt.Sprintf("Failed to marshal schema: %v", err)}
	}

	var plainSchema any
	err = json.Unmarshal(jsonBytes, &plainSchema)
	if err != nil {
		return false, []string{fmt.Sprintf("Failed to unmarshal schema: %v", err)}
	}

	return t.validate(plainSchema, params)
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
func NewToolFromJSON(jsonStr string, callback func(params InvokeParams, stdout io.Writer, stderr io.Writer) (any, error)) (*Tool, error) {
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
		WithSimpleCallback(callback),
	)

	if err != nil {
		return nil, err
	}

	for _, paramDef := range toolDef.Params {
		name := utils.MapGetString(paramDef, "name")
		tool.Tool.InputSchema.Properties.Set(name, paramDef)
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
