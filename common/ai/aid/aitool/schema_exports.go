package aitool

import (
	"encoding/json"
	"strings"

	"github.com/segmentio/ksuid"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/omap"
	"github.com/yaklang/yaklang/common/yak/static_analyzer"
	"github.com/yaklang/yaklang/common/yak/static_analyzer/information"
)

func _withParamObject(objectName string, opts ...any) ToolOption {
	var params []ToolOption
	var currentProperties []PropertyOption
	for _, i := range opts {
		switch ret := i.(type) {
		case ToolOption:
			params = append(params, ret)
		case PropertyOption:
			currentProperties = append(currentProperties, ret)
		default:
			log.Warnf("with object param unknown opt type: %T", ret)
		}
	}
	return WithStructParam(objectName, currentProperties, params...)
}

func NewObjectSchemaWithAction(opts ...any) string {
	return NewObjectSchemaWithActionName("object", opts...)
}

func NewObjectSchemaWithActionName(name string, opts ...any) string {
	var params []any
	params = append(params, WithAction(name))
	params = append(params, opts...)
	return NewObjectSchema(params...)
}

func NewObjectSchemaFrameOmap(opts ...any) *omap.OrderedMap[string, any] {
	var params []ToolOption
	var props []PropertyOption
	for _, i := range opts {
		switch ret := i.(type) {
		case ToolOption:
			params = append(params, ret)
		case PropertyOption:
			props = append(props, ret)
		default:
			log.Warnf("new object schema unknown opt type: %T", ret)
		}
	}
	t := newTool(ksuid.New().String(), params...)
	if t.InputSchema.Properties == nil {
		t.InputSchema.Properties = omap.NewEmptyOrderedMap[string, any]()
	}
	// Create a temp map for properties to apply PropertyOptions
	tempProps := make(map[string]any)
	for _, i := range props {
		i(tempProps)
	}
	// Copy properties to OrderedMap
	for k, v := range tempProps {
		t.InputSchema.Properties.Set(k, v)
	}

	paramActually := t.Params()
	var baseFrame = omap.NewGeneralOrderedMap()
	baseFrame.Set("$schema", "http://json-schema.org/draft-07/schema#")
	baseFrame.Set("type", "object")
	if len(t.InputSchema.Required) > 0 {
		baseFrame.Set("required", t.InputSchema.Required)
	}
	extra := make(map[string]interface{})
	for _, i := range props {
		i(extra)
	}
	for k, v := range extra {
		baseFrame.Set(k, v)
	}
	baseFrame.Set("properties", paramActually)
	baseFrame.Set("additionalProperties", true)
	return baseFrame
}

func NewObjectSchema(opts ...any) string {
	baseFrame := NewObjectSchemaFrameOmap(opts...)
	results, _ := json.MarshalIndent(baseFrame, "", "  ")
	return string(results)
}

func newObjectArraySchema(opts ...any) string {
	var params []ToolOption
	var props []PropertyOption
	for _, i := range opts {
		switch ret := i.(type) {
		case ToolOption:
			params = append(params, ret)
		case PropertyOption:
			props = append(props, ret)
		default:
			log.Warnf("new object array schema unknown opt type: %T", ret)
		}
	}
	t := newTool(ksuid.New().String(), params...)

	if t.InputSchema.Properties == nil {
		t.InputSchema.Properties = omap.NewEmptyOrderedMap[string, any]()
	}
	// Create a temp map for properties to apply PropertyOptions
	tempProps := make(map[string]any)
	for _, i := range props {
		i(tempProps)
	}
	// Copy properties to OrderedMap
	for k, v := range tempProps {
		t.InputSchema.Properties.Set(k, v)
	}

	paramActually := t.Params()

	var itemFrame = omap.NewGeneralOrderedMap()
	itemFrame.Set("type", "object")
	if len(t.InputSchema.Required) > 0 {
		itemFrame.Set("required", t.InputSchema.Required)
	}
	itemFrame.Set("properties", paramActually)

	var baseFrame = omap.NewGeneralOrderedMap()
	baseFrame.Set("$schema", "http://json-schema.org/draft-07/schema#")
	baseFrame.Set("type", "array")
	baseFrame.Set("items", itemFrame)

	extra := make(map[string]interface{})
	for _, i := range props {
		i(extra)
	}
	for k, v := range extra {
		baseFrame.Set(k, v)
	}
	baseFrame.Set("additionalProperties", true)
	results, _ := json.MarshalIndent(baseFrame, "", "  ")
	return string(results)
}

func WithAction(action string) ToolOption {
	return WithStringParam(
		"@action",
		WithParam_Description(`set '@action' can help the AI identify the output json object`),
		WithParam_Raw("const", action),
		WithParam_Required(true),
	)
}

func _withObjectArrayEx(name string, arrayPropsRaw []any, opts ...any) ToolOption {
	var params []ToolOption

	var arrayProps []PropertyOption
	for _, ap := range arrayPropsRaw {
		switch ret := ap.(type) {
		case PropertyOption:
			arrayProps = append(arrayProps, ret)
		default:
			log.Warnf("with object array ex unknown array prop type: %T", ret)
		}
	}

	var currentProperties []PropertyOption
	for _, i := range opts {
		switch ret := i.(type) {
		case ToolOption:
			params = append(params, ret)
		case PropertyOption:
			currentProperties = append(currentProperties, ret)
		default:
			log.Warnf("with object array unknown opt type: %T", ret)
		}
	}
	return WithStructArrayParam(name, arrayProps, currentProperties, params...)
}

func _withObjectArray(name string, opts ...any) ToolOption {
	var params []ToolOption
	var currentProperties []PropertyOption
	for _, i := range opts {
		switch ret := i.(type) {
		case ToolOption:
			params = append(params, ret)
		case PropertyOption:
			currentProperties = append(currentProperties, ret)
		}
	}
	return WithStructArrayParam(name, nil, currentProperties, params...)
}

// ConvertYaklangCliCodeToToolOptions 将 Yaklang CLI 代码转换为 aitool.ToolOption
func ConvertYaklangCliCodeToToolOptions(yakCode string) []ToolOption {
	if yakCode == "" {
		return []ToolOption{}
	}

	// 首先尝试直接解析，不依赖 ForgeBlueprint
	prog, err := static_analyzer.SSAParse(yakCode, "yak")
	if err != nil {
		log.Warnf("failed to parse yaklang CLI code: %v", err)
		return []ToolOption{}
	}

	cliInfo, _, _ := information.ParseCliParameter(prog)

	// 如果没有解析到参数，尝试手动解析
	if len(cliInfo) == 0 {
		manualParams := parseYakCliCodeManually(yakCode)
		return convertCliInfoToToolOptions(manualParams)
	}

	result := convertCliInfoToToolOptions(cliInfo)
	return result
}

// parseYakCliCodeManually 手动解析 Yak CLI 代码
func parseYakCliCodeManually(yakCode string) []*information.CliParameter {
	var params []*information.CliParameter

	lines := strings.Split(yakCode, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "cli.") && strings.Contains(line, "(") {
			param := parseCliLine(line)
			if param != nil {
				params = append(params, param)
			}
		}
	}

	return params
}

// parseCliLine 解析单个 CLI 行
func parseCliLine(line string) *information.CliParameter {
	// 匹配 cli.String("name", ...) 格式
	if strings.HasPrefix(line, "cli.String(") ||
		strings.HasPrefix(line, "cli.Int(") ||
		strings.HasPrefix(line, "cli.Bool(") ||
		strings.HasPrefix(line, "cli.Float(") {

		param := &information.CliParameter{}

		// 提取参数名
		start := strings.Index(line, `"`)
		if start == -1 {
			return nil
		}
		end := strings.Index(line[start+1:], `"`)
		if end == -1 {
			return nil
		}
		param.Name = line[start+1 : start+1+end]

		// 设置参数类型
		if strings.HasPrefix(line, "cli.String(") {
			param.Type = "string"
		} else if strings.HasPrefix(line, "cli.Int(") || strings.HasPrefix(line, "cli.Integer(") {
			param.Type = "int"
		} else if strings.HasPrefix(line, "cli.Bool(") {
			param.Type = "boolean"
		} else if strings.HasPrefix(line, "cli.Float(") || strings.HasPrefix(line, "cli.Double(") {
			param.Type = "float"
		}

		// 解析选项
		content := line[strings.Index(line, "(")+1 : strings.LastIndex(line, ")")]
		parts := strings.Split(content, ",")

		for i, part := range parts {
			if i == 0 {
				continue // 跳过参数名
			}
			part = strings.TrimSpace(part)

			if strings.Contains(part, "setRequired(true)") {
				param.Required = true
			}

			if strings.Contains(part, "setDefault(") {
				start := strings.Index(part, `setDefault(`)
				if start != -1 {
					valueStart := start + len(`setDefault(`)
					valueEnd := strings.Index(part[valueStart:], ")")
					if valueEnd != -1 {
						defaultValue := part[valueStart : valueStart+valueEnd]
						param.Default = strings.Trim(defaultValue, `"`)
					}
				}
			}

			if strings.Contains(part, "setHelp(") {
				start := strings.Index(part, `setHelp(`)
				if start != -1 {
					valueStart := start + len(`setHelp(`)
					valueEnd := strings.Index(part[valueStart:], ")")
					if valueEnd != -1 {
						help := part[valueStart : valueStart+valueEnd]
						param.Help = strings.Trim(help, `"`)
					}
				}
			}

			if strings.Contains(part, "setVerboseName(") {
				start := strings.Index(part, `setVerboseName(`)
				if start != -1 {
					valueStart := start + len(`setVerboseName(`)
					valueEnd := strings.Index(part[valueStart:], ")")
					if valueEnd != -1 {
						verboseName := part[valueStart : valueStart+valueEnd]
						param.NameVerbose = strings.Trim(verboseName, `"`)
					}
				}
			}
		}

		return param
	}

	return nil
}

// convertCliInfoToToolOptions 将 CLI 信息转换为 ToolOption
func convertCliInfoToToolOptions(cliInfo []*information.CliParameter) []ToolOption {
	var options []ToolOption

	for _, param := range cliInfo {
		if param == nil {
			continue
		}

		// 构建参数选项
		var opts []PropertyOption

		// 添加描述
		if param.Help != "" {
			opts = append(opts, WithParam_Description(param.Help))
		}

		// 添加默认值
		if param.Default != nil {
			opts = append(opts, WithParam_Default(param.Default))
		}

		// 添加标题
		if param.NameVerbose != "" {
			opts = append(opts, WithParam_Title(param.NameVerbose))
		}

		// 如果是必需参数
		if param.Required {
			opts = append(opts, WithParam_Required(true))
		}

		// 根据参数类型创建相应的 ToolOption
		var option ToolOption
		switch param.Type {
		case "string":
			option = WithStringParam(param.Name, opts...)
		case "int", "int64", "int32":
			option = WithIntegerParam(param.Name, opts...)
		case "float", "float64", "float32":
			option = WithNumberParam(param.Name, opts...)
		case "bool", "boolean":
			option = WithBoolParam(param.Name, opts...)
		default:
			// 默认当作字符串处理
			option = WithStringParam(param.Name, opts...)
		}

		options = append(options, option)
	}

	return options
}

var SchemaGeneratorExports = map[string]any{
	"ActionObject":         NewObjectSchemaWithAction,
	"Object":               NewObjectSchema,
	"ObjectArray":          newObjectArraySchema,
	"NewObjectSchema":      NewObjectSchema,
	"NewObjectArraySchema": newObjectArraySchema,

	"action":                  WithAction,
	"paramString":             WithStringParam,
	"paramInt":                WithIntegerParam,
	"paramBool":               WithBoolParam,
	"paramNumber":             WithNumberParam,
	"paramStringArray":        WithStringArrayParam,
	"paramNumberArray":        WithNumberArrayParam,
	"paramKeyValuePairsArray": WithKVPairsParam,
	"paramObjectArray":        _withObjectArray,
	"paramObjectArrayEx":      _withObjectArrayEx,
	"paramObject":             _withParamObject,
	"paramRaw":                WithRawParam,

	"description": WithParam_Description,
	"required":    WithParam_Required,
	"min":         WithParam_Min,
	"max":         WithParam_Max,
	"maxLength":   WithParam_MaxLength,
	"minLength":   WithParam_MinLength,
	"const":       WithParam_Const,
	"enum":        WithParam_Enum,
	"title":       WithParam_Title,
	"raw":         WithParam_Raw,
	"example":     WithParam_Example,
}
