package openapi

import (
	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/openapi/openapi2"
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
	for _, pro := range val.Properties {
		if pro.Value == nil {
			continue
		}
		pt := pro.Value
		_ = pt
	}
	return nil
}
