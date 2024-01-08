package openapi

import (
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/mutate"
	"github.com/yaklang/yaklang/common/openapi/openapi3"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/omap"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"strings"
)

func v3_schemaToValue(t openapi3.T, p any) (*openapi3.Schema, error) {
	switch param := p.(type) {
	case *openapi3.SchemaRef:
		if param.Ref != "" {
			var ret = strings.TrimPrefix(param.Ref, "#/components/schemas/")
			return v3_schemaToValue(t, t.Components.Schemas[ret])
		}
		return param.Value, nil
	case *openapi3.Schema:
		return param, nil
	case string:
		param = strings.TrimPrefix(param, "#/components/schemas/")
		return v3_schemaToValue(t, t.Components.Schemas[param])
	default:
		return nil, utils.Errorf("unsupported parameter type: %T", p)
	}
}

func v3_parameterToValue(t openapi3.T, p any) (*openapi3.Parameter, error) {
	switch param := p.(type) {
	case *openapi3.ParameterRef:
		if param.Ref != "" {
			var ret = strings.TrimPrefix(param.Ref, "#/components/parameters/")
			return v3_parameterToValue(t, t.Components.Parameters[ret])
		}
		return param.Value, nil
	case *openapi3.Parameter:
		return param, nil
	case string:
		param = strings.TrimPrefix(param, "#/components/parameters/")
		return v3_parameterToValue(t, t.Components.Parameters[param])
	default:
		return nil, utils.Errorf("unsupported parameter type: %T", p)
	}
}

func v3_SchemeRefToObject(t openapi3.T, target any, fields ...string) any {
	if target == nil {
		return nil
	}

	var field string
	if len(fields) > 0 {
		field = fields[0]
	}

	var ref string
	switch target.(type) {
	case string, []byte:
		ref = codec.AnyToString(target)
	case *openapi3.SchemaRef:
		result, ok := target.(*openapi3.SchemaRef)
		if !ok {
			return nil
		}

		if result == nil {
			return nil
		}
		ref = result.Ref
		if ref == "" {
			return v3_schemaValue(t, target.(*openapi3.SchemaRef).Value, field)
		}
	case *openapi3.Schema:
		return v3_schemaValue(t, target.(*openapi3.Schema), field)
	case *openapi3.ParameterRef:
		result, ok := target.(*openapi3.ParameterRef)
		if !ok {
			return nil
		}
		if result == nil {
			return nil
		}
		ref = result.Ref
		if ref == "" {
			return v3_parametersValue(t, result.Value, field)
		}
	default:
		log.Warnf("unsupported ref type: %T", target)
		return "{}"
	}
	ref = strings.TrimSpace(ref)

	switch {
	case strings.HasPrefix(ref, `#/components/parameters/`):
		name := strings.TrimPrefix(ref, `#/components/parameters/`)
		return v3_SchemeRefToObject(t, t.Components.Parameters[name], field)
	case strings.HasPrefix(ref, `#/components/schemas/`):
		name := strings.TrimPrefix(ref, `#/components/schemas/`)
		return v3_SchemeRefToObject(t, t.Components.Schemas[name], field)
	}
	log.Infof("met ref: %v", ref)
	//trimDef := strings.TrimPrefix(ref, "#/definitions/")
	//_ = trimDef
	//obj, ok := t.Definitions[trimDef]
	//if !ok {
	//	return nil
	//}
	//if obj.Ref != "" {
	//	return v2_SchemeRefToObject(t, obj.Ref, field)
	//}
	//
	//if obj.Value == nil {
	//	return nil
	//}
	//val := obj.Value
	//return v3_schemaValue(t, val, field)
	return nil
}

func v3_schemaValue(data openapi3.T, i *openapi3.Schema, fieldName ...string) any {
	if i == nil {
		return nil
	}

	var field string
	if len(fieldName) > 0 {
		field = fieldName[0]
	}

	switch i.Type {
	case "array":
		m := omap.NewGeneralOrderedMap()
		if i.Items.Ref != "" {
			m.Add(v3_SchemeRefToObject(data, i.Items.Ref))
			return m
		}
		m.Add(v3_schemaValue(data, i.Items.Value, field))
		return m
	case "object":
		m := omap.NewGeneralOrderedMap()
		for field, pt := range i.Properties {
			if pt.Ref != "" {
				m.Set(field, v3_SchemeRefToObject(data, pt.Ref, field))
			} else {
				m.Set(field, v3_schemaValue(data, pt.Value, field))
			}
		}
		return m
	default:
		return ValueViaField(field, i.Type, i.Default)
	}
}
