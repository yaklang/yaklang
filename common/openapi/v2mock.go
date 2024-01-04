package openapi

import (
	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/openapi/openapi2"
	"github.com/yaklang/yaklang/common/openapi/openapi3"
	"github.com/yaklang/yaklang/common/utils/omap"
	"strings"
)

func OpenAPITypeToMockDataLiteral(t string, defaults ...any) any {
	defaults = funk.Filter(defaults, funk.NotEmpty).([]any)
	if len(defaults) > 0 {
		return defaults[0]
	}
	switch ret := strings.ToLower(t); ret {
	case "string":
		return "mock_string_data"
	case "integer", "int":
		return 1
	case "number":
		return 1
	case "boolean", "bool":
		return false
	}
	return "{}"
}

func OpenAPI2RefToObject(t openapi2.T, ref string) any {
	ref = strings.TrimSpace(ref)

	trimDef := strings.TrimPrefix(ref, "#/definitions/")
	obj, ok := t.Definitions[trimDef]
	if !ok {
		return nil
	}
	if obj.Ref != "" {
		return OpenAPI2RefToObject(t, obj.Ref)
	}

	if obj.Value == nil {
		return nil
	}
	val := obj.Value
	return schemaValue(t, val)
}

func schemaValue(data openapi2.T, i *openapi3.Schema) any {
	if i == nil {
		return nil
	}

	switch i.Type {
	case "array":
		m := omap.NewGeneralOrderedMap()
		if i.Items.Ref != "" {
			m.Add(OpenAPI2RefToObject(data, i.Items.Ref))
			return m
		}
		m.Add(schemaValue(data, i.Items.Value))
		return m
	case "object":
		m := omap.NewGeneralOrderedMap()
		for field, pt := range i.Properties {
			if pt.Ref != "" {
				m.Set(field, OpenAPI2RefToObject(data, pt.Ref))
			} else {
				m.Set(field, schemaValue(data, pt.Value))
			}
		}
		return m
	default:
		return OpenAPITypeToMockDataLiteral(i.Type, i.Default)
	}
}
