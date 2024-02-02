package openapigen

import (
	"github.com/asaskevich/govalidator"
	"github.com/tidwall/gjson"
	"github.com/yaklang/yaklang/common/openapi/openapi3"
	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakvm"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

func gjsonResultToScheme(result gjson.Result) *openapi3.Schema {
	switch result.Type {
	case gjson.String:
		return openapi3.NewStringSchema().WithExample(result.String())
	case gjson.Number:
		num := result.String()
		if govalidator.IsFloat(num) {
			return openapi3.NewFloat64Schema().WithExample(result.Float())
		}
		return openapi3.NewIntegerSchema().WithExample(result.Int())
	case gjson.True, gjson.False:
		return openapi3.NewBoolSchema().WithExample(result.Bool())
	case gjson.Null:
		return openapi3.NewObjectSchema().WithNullable()
	case gjson.JSON:
		scheme := openapi3.NewObjectSchema()
		result.ForEach(func(key, value gjson.Result) bool {
			if !key.Exists() {
				return true
			}
			if scheme.Properties == nil {
				scheme.Properties = make(openapi3.Schemas)
			}
			scheme.Properties[key.String()] = &openapi3.SchemaRef{Value: gjsonResultToScheme(value)}
			return true
		})
		if scheme == nil {
			return openapi3.NewObjectSchema().WithNullable()
		}
		return scheme
	default:
		return openapi3.NewObjectSchema().WithNullable()
	}
}

func jsonToScheme(i any) *openapi3.Schema {
	raw := codec.AnyToString(i)
	result := gjson.Parse(raw)
	if !result.Exists() {
		return openapi3.NewStringSchema().WithExample(raw)
	}
	return gjsonResultToScheme(result)
}

func anyToScheme(i any) *openapi3.Schema {
	if yakvm.IsInt(i) {
		return openapi3.NewIntegerSchema().WithExample(i)
	} else if yakvm.IsFloat(i) {
		return openapi3.NewFloat64Schema().WithExample(i)
	}

	switch i.(type) {
	case bool:
		return openapi3.NewBoolSchema().WithExample(i)
	}

	if i == nil {
		return openapi3.NewObjectSchema().WithNullable()
	}
	return jsonToScheme(i)
}
