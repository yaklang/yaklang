package openapi

import (
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/openapi/openapi3"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/omap"
	"strings"
)

func v3_requestBodyToValue(t openapi3.T, p any) (*openapi3.RequestBody, error) {
	return v3_requestBodyToValueVisited(t, p, make(map[string]struct{}))
}

func v3_requestBodyToValueVisited(t openapi3.T, p any, visitedRefs map[string]struct{}) (*openapi3.RequestBody, error) {
	if p == nil {
		return nil, utils.Errorf("unsupported parameter type: %T or nil", p)
	}
	switch param := p.(type) {
	case *openapi3.RequestBodyRef:
		if param == nil {
			return nil, utils.Error("nil request body ref")
		}
		if param.Ref != "" {
			if _, seen := visitedRefs[param.Ref]; seen {
				return &openapi3.RequestBody{}, nil
			}
			visitedRefs[param.Ref] = struct{}{}
			var ret = strings.TrimPrefix(param.Ref, "#/components/requestBodies/")
			if t.Components == nil || len(t.Components.RequestBodies) <= 0 {
				return &openapi3.RequestBody{}, nil
			}
			return v3_requestBodyToValueVisited(t, t.Components.RequestBodies[ret], visitedRefs)
		}
		return param.Value, nil
	case *openapi3.RequestBody:
		return param, nil
	case string:
		if _, seen := visitedRefs[param]; seen {
			return &openapi3.RequestBody{}, nil
		}
		visitedRefs[param] = struct{}{}
		param = strings.TrimPrefix(param, "#/components/requestBodies/")
		if t.Components == nil || len(t.Components.RequestBodies) <= 0 {
			return &openapi3.RequestBody{}, nil
		}
		return v3_requestBodyToValueVisited(t, t.Components.RequestBodies[param], visitedRefs)
	default:
		return nil, utils.Errorf("unsupported parameter type: %T", p)
	}
}

func v3_responseToValue(t openapi3.T, p any) (*openapi3.Response, error) {
	return v3_responseToValueVisited(t, p, make(map[string]struct{}))
}

func v3_responseToValueVisited(t openapi3.T, p any, visitedRefs map[string]struct{}) (*openapi3.Response, error) {
	if p == nil {
		return nil, utils.Errorf("unsupported parameter type: %T or nil", p)
	}

	switch param := p.(type) {
	case *openapi3.ResponseRef:
		if param == nil {
			return nil, utils.Error("nil request body ref")
		}
		if param.Ref != "" {
			if _, seen := visitedRefs[param.Ref]; seen {
				return &openapi3.Response{}, nil
			}
			visitedRefs[param.Ref] = struct{}{}
			var ret = strings.TrimPrefix(param.Ref, "#/components/responses/")
			if t.Components == nil || len(t.Components.Responses) <= 0 {
				return &openapi3.Response{}, nil
			}
			return v3_responseToValueVisited(t, t.Components.Responses[ret], visitedRefs)
		}
		return param.Value, nil
	case *openapi3.Response:
		return param, nil
	case string:
		if _, seen := visitedRefs[param]; seen {
			return &openapi3.Response{}, nil
		}
		visitedRefs[param] = struct{}{}
		param = strings.TrimPrefix(param, "#/components/responses/")
		if t.Components == nil || len(t.Components.Responses) <= 0 {
			return &openapi3.Response{}, nil
		}
		return v3_responseToValueVisited(t, t.Components.Responses[param], visitedRefs)
	default:
		return nil, utils.Errorf("unsupported parameter type: %T", p)
	}
}

func v3_schemaToValue(t openapi3.T, p any) (*openapi3.Schema, error) {
	return v3_schemaToValueVisited(t, p, make(map[string]struct{}))
}

func v3_schemaToValueVisited(t openapi3.T, p any, visitedRefs map[string]struct{}) (*openapi3.Schema, error) {
	if p == nil {
		return nil, utils.Errorf("unsupported parameter type: %T or nil", p)
	}

	switch param := p.(type) {
	case *openapi3.SchemaRef:
		if param == nil {
			return nil, utils.Error("nil request body ref")
		}
		if param.Ref != "" {
			if _, seen := visitedRefs[param.Ref]; seen {
				return &openapi3.Schema{}, nil
			}
			visitedRefs[param.Ref] = struct{}{}
			var ret = strings.TrimPrefix(param.Ref, "#/components/schemas/")
			if t.Components == nil || len(t.Components.Schemas) <= 0 {
				return &openapi3.Schema{}, nil
			}
			return v3_schemaToValueVisited(t, t.Components.Schemas[ret], visitedRefs)
		}
		return param.Value, nil
	case *openapi3.Schema:
		return param, nil
	case string:
		if _, seen := visitedRefs[param]; seen {
			return &openapi3.Schema{}, nil
		}
		visitedRefs[param] = struct{}{}
		param = strings.TrimPrefix(param, "#/components/schemas/")
		if t.Components == nil || len(t.Components.Schemas) <= 0 {
			return &openapi3.Schema{}, nil
		}
		return v3_schemaToValueVisited(t, t.Components.Schemas[param], visitedRefs)
	default:
		return nil, utils.Errorf("unsupported parameter type: %T", p)
	}
}

func v3_parameterToValue(t openapi3.T, p any) (*openapi3.Parameter, error) {
	return v3_parameterToValueVisited(t, p, make(map[string]struct{}))
}

func v3_parameterToValueVisited(t openapi3.T, p any, visitedRefs map[string]struct{}) (*openapi3.Parameter, error) {
	if p == nil {
		return nil, utils.Errorf("unsupported parameter type: %T or nil", p)
	}

	switch param := p.(type) {
	case *openapi3.ParameterRef:
		if param == nil {
			return nil, utils.Error("nil request body ref")
		}
		if param.Ref != "" {
			if _, seen := visitedRefs[param.Ref]; seen {
				return &openapi3.Parameter{}, nil
			}
			visitedRefs[param.Ref] = struct{}{}
			var ret = strings.TrimPrefix(param.Ref, "#/components/parameters/")
			if t.Components == nil || len(t.Components.Parameters) <= 0 {
				return &openapi3.Parameter{}, nil
			}
			return v3_parameterToValueVisited(t, t.Components.Parameters[ret], visitedRefs)
		}
		return param.Value, nil
	case *openapi3.Parameter:
		return param, nil
	case string:
		if _, seen := visitedRefs[param]; seen {
			return &openapi3.Parameter{}, nil
		}
		visitedRefs[param] = struct{}{}
		param = strings.TrimPrefix(param, "#/components/parameters/")
		if t.Components == nil || len(t.Components.Parameters) <= 0 {
			return &openapi3.Parameter{}, nil
		}
		return v3_parameterToValueVisited(t, t.Components.Parameters[param], visitedRefs)
	default:
		return nil, utils.Errorf("unsupported parameter type: %T", p)
	}
}

func v3_mockSchemaValue(data openapi3.T, i *openapi3.Schema, fieldName ...string) *omap.OrderedMap[string, any] {
	return v3_mockSchemaValueVisited(data, i, make(map[string]struct{}), make(map[*openapi3.Schema]struct{}), 0, fieldName...)
}

func v3_mockSchemaValueVisited(
	data openapi3.T,
	i *openapi3.Schema,
	visitedRefs map[string]struct{},
	visitedSchemas map[*openapi3.Schema]struct{},
	depth int,
	fieldName ...string,
) *omap.OrderedMap[string, any] {
	if i == nil || depth > maxMockSchemaDepth {
		return nil
	}
	if _, seen := visitedSchemas[i]; seen {
		return omap.NewGeneralOrderedMap()
	}
	visitedSchemas[i] = struct{}{}
	defer delete(visitedSchemas, i)

	var field string
	if len(fieldName) > 0 {
		field = fieldName[0]
	}

	m := omap.NewGeneralOrderedMap()
	switch i.Type {
	case "array":
		if i.Items == nil {
			return m
		}
		if i.Items.Ref != "" {
			if _, seen := visitedRefs[i.Items.Ref]; seen {
				return m
			}
			visitedRefs[i.Items.Ref] = struct{}{}
			scheme, err := v3_schemaToValue(data, i.Items.Ref)
			if err != nil {
				log.Errorf("v3_schemaToValue [%v] failed: %v", i.Items.Ref, err)
				return nil
			}
			m.Add(v3_mockSchemaValueVisited(data, scheme, visitedRefs, visitedSchemas, depth+1, field))
			return m
		}
		m.Add(v3_mockSchemaValueVisited(data, i.Items.Value, visitedRefs, visitedSchemas, depth+1, field))
		return m
	case "object":
		for propName, pt := range i.Properties {
			if pt == nil {
				continue
			}
			if pt.Ref != "" {
				if _, seen := visitedRefs[pt.Ref]; seen {
					m.Set(propName, map[string]any{})
					continue
				}
				visitedRefs[pt.Ref] = struct{}{}
				scheme, err := v3_schemaToValue(data, pt.Ref)
				if err != nil {
					log.Errorf("v3_schemaToValue [%v] failed: %v", pt.Ref, err)
					continue
				}
				m.Set(propName, v3_mockSchemaValueVisited(data, scheme, visitedRefs, visitedSchemas, depth+1, propName))
			} else {
				m.Set(propName, v3_mockSchemaValueVisited(data, pt.Value, visitedRefs, visitedSchemas, depth+1, propName))
			}
		}
		return m
	default:
		m.SetLiteralValue(ValueViaField(field, i.Type, i.Default))
		return m
	}
}

func v3_mockSchemaJson(data openapi3.T, i *openapi3.Schema, fieldName ...string) []byte {
	mocked := v3_mockSchemaValue(data, i, fieldName...)
	if mocked == nil {
		return nil
	}
	return mocked.Jsonify()
}
