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

// maxMockSchemaDepth caps schema mock expansion for deep/circular graphs.
const maxMockSchemaDepth = 32

func v2_parameterToValue(t openapi2.T, target any) (*openapi2.Parameter, error) {
	if target == nil {
		return nil, nil
	}

	switch param := target.(type) {
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
	return v2_SchemeRefToObjectVisited(t, target, make(map[string]struct{}), make(map[*openapi3.Schema]struct{}), 0, fields...)
}

func v2_SchemeRefToObjectVisited(
	t openapi2.T,
	target any,
	visitedRefs map[string]struct{},
	visitedSchemas map[*openapi3.Schema]struct{},
	depth int,
	fields ...string,
) any {
	if target == nil || depth > maxMockSchemaDepth {
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
		if !ok || result == nil {
			return nil
		}
		ref = result.Ref
		if ref == "" {
			return schemaValueVisited(t, result.Value, visitedRefs, visitedSchemas, depth, field)
		}
	case *openapi3.Schema:
		return schemaValueVisited(t, target.(*openapi3.Schema), visitedRefs, visitedSchemas, depth, field)
	default:
		log.Warnf("unsupported ref type: %T", target)
		return "{}"
	}
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil
	}
	if _, seen := visitedRefs[ref]; seen {
		return map[string]any{}
	}
	visitedRefs[ref] = struct{}{}
	defer delete(visitedRefs, ref)

	trimDef := strings.TrimPrefix(ref, "#/definitions/")
	obj, ok := t.Definitions[trimDef]
	if !ok {
		return nil
	}
	if obj.Ref != "" {
		return v2_SchemeRefToObjectVisited(t, obj.Ref, visitedRefs, visitedSchemas, depth+1, field)
	}
	if obj.Value == nil {
		return nil
	}
	return schemaValueVisited(t, obj.Value, visitedRefs, visitedSchemas, depth+1, field)
}

func schemaValue(data openapi2.T, i *openapi3.Schema, fieldName ...string) any {
	return schemaValueVisited(data, i, make(map[string]struct{}), make(map[*openapi3.Schema]struct{}), 0, fieldName...)
}

func schemaValueVisited(
	data openapi2.T,
	i *openapi3.Schema,
	visitedRefs map[string]struct{},
	visitedSchemas map[*openapi3.Schema]struct{},
	depth int,
	fieldName ...string,
) any {
	if i == nil || depth > maxMockSchemaDepth {
		return nil
	}
	if _, seen := visitedSchemas[i]; seen {
		return map[string]any{}
	}
	visitedSchemas[i] = struct{}{}
	defer delete(visitedSchemas, i)

	var field string
	if len(fieldName) > 0 {
		field = fieldName[0]
	}

	switch i.Type {
	case "array":
		m := omap.NewGeneralOrderedMap()
		if i.Items == nil {
			return m
		}
		if i.Items.Ref != "" {
			m.Add(v2_SchemeRefToObjectVisited(data, i.Items.Ref, visitedRefs, visitedSchemas, depth+1))
			return m
		}
		m.Add(schemaValueVisited(data, i.Items.Value, visitedRefs, visitedSchemas, depth+1, field))
		return m
	case "object":
		m := omap.NewGeneralOrderedMap()
		for propName, pt := range i.Properties {
			if pt == nil {
				continue
			}
			if pt.Ref != "" {
				m.Set(propName, v2_SchemeRefToObjectVisited(data, pt.Ref, visitedRefs, visitedSchemas, depth+1, propName))
			} else {
				m.Set(propName, schemaValueVisited(data, pt.Value, visitedRefs, visitedSchemas, depth+1, propName))
			}
		}
		return m
	default:
		return ValueViaField(field, i.Type, i.Default)
	}
}
