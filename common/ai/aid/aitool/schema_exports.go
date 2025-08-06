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

func newObjectSchema(opts ...any) string {
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

var SchemaGeneratorExports = map[string]any{
	"NewObjectSchema":      newObjectSchema,
	"NewObjectArraySchema": newObjectArraySchema,

	"paramString":             WithStringParam,
	"paramInt":                WithIntegerParam,
	"paramBool":               WithBoolParam,
	"paramNumber":             WithNumberParam,
	"paramStringArray":        WithStringArrayParam,
	"paramNumberArray":        WithNumberArrayParam,
	"paramKeyValuePairsArray": WithKVPairsParam,
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
