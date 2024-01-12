package openapi

import (
	"encoding/json"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/openapi/openapi2"
	"github.com/yaklang/yaklang/common/openapi/openapi3"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/omap"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"strings"
)

func v2_parameterToValue(t openapi2.T, target any) (*openapi2.Parameter, error) {
	if target == nil {
		return nil, nil
	}

	switch param := target.(type) {
	//case *openapi2.ParameterRef:
	//	if param == nil {
	//		return nil, nil
	//	}
	//	if param.Ref != "" {
	//		ret := strings.TrimPrefix(param.Ref, "#/parameters/")
	//		ret = strings.TrimPrefix(ret, "#/components/parameters/")
	//		return v2_parameterToValue(t, t.Parameters[ret])
	//	}
	//	return param.Value, nil
	case *openapi2.Parameter:
		return param, nil
	case string:
		param = strings.TrimPrefix(param, "#/components/parameters/")
		param = strings.TrimPrefix(param, "#/parameters/")
		return v2_parameterToValue(t, t.Parameters[param])
	default:
		return nil, utils.Errorf("unsupported parameter type: %T", target)
	}
}

func v2_SchemeRefToBytes(t openapi2.T, target any) []byte {
	raw, err := json.Marshal(v2_SchemeRefToObject(t, target))
	if err != nil {
		return nil
	}
	if string(raw) == `null` {
		return nil
	}
	return raw
}

func v2_SchemeRefToObject(t openapi2.T, target any, fields ...string) any {
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
			return schemaValue(t, target.(*openapi3.SchemaRef).Value, field)
		}
	case *openapi3.Schema:
		return schemaValue(t, target.(*openapi3.Schema), field)
	default:
		log.Warnf("unsupported ref type: %T", target)
		return "{}"
	}
	ref = strings.TrimSpace(ref)

	trimDef := strings.TrimPrefix(ref, "#/definitions/")
	obj, ok := t.Definitions[trimDef]
	if !ok {
		return nil
	}
	if obj.Ref != "" {
		return v2_SchemeRefToObject(t, obj.Ref, field)
	}

	if obj.Value == nil {
		return nil
	}
	val := obj.Value
	return schemaValue(t, val, field)
}

func schemaValue(data openapi2.T, i *openapi3.Schema, fieldName ...string) any {
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
			m.Add(v2_SchemeRefToObject(data, i.Items.Ref))
			return m
		}
		m.Add(schemaValue(data, i.Items.Value, field))
		return m
	case "object":
		m := omap.NewGeneralOrderedMap()
		for field, pt := range i.Properties {
			if pt.Ref != "" {
				m.Set(field, v2_SchemeRefToObject(data, pt.Ref, field))
			} else {
				m.Set(field, schemaValue(data, pt.Value, field))
			}
		}
		return m
	default:
		return ValueViaField(field, i.Type, i.Default)
	}
}
