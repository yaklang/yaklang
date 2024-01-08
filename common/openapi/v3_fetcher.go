package openapi

import (
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/openapi/openapi3"
	"github.com/yaklang/yaklang/common/utils/omap"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"strings"
)

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
	default:
		log.Warnf("unsupported ref type: %T", target)
		return "{}"
	}
	ref = strings.TrimSpace(ref)

	trimDef := strings.TrimPrefix(ref, "#/definitions/")
	_ = trimDef
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
