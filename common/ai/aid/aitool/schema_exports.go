package aitool

import (
	"encoding/json"

	"github.com/segmentio/ksuid"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/omap"
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
	var params []any
	params = append(params, WithAction("object"))
	params = append(params, opts...)
	return NewObjectSchema(params...)
}

func NewObjectSchema(opts ...any) string {
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
	if utils.IsNil(t.InputSchema.Properties) {
		t.InputSchema.Properties = make(map[string]any)
	}
	for _, i := range props {
		i(t.InputSchema.Properties)
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

	if utils.IsNil(t.InputSchema.Properties) {
		t.InputSchema.Properties = make(map[string]any)
	}
	for _, i := range props {
		i(t.InputSchema.Properties)
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
