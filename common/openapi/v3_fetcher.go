package openapi

import (
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/openapi/openapi3"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/omap"
	"strings"
)

func v3_requestBodyToValue(t openapi3.T, p any) (*openapi3.RequestBody, error) {
	if p == nil {
		return nil, utils.Errorf("unsupported parameter type: %T or nil", p)
	}
	switch param := p.(type) {
	case *openapi3.RequestBodyRef:
		if param == nil {
			return nil, utils.Error("nil request body ref")
		}
		if param.Ref != "" {
			var ret = strings.TrimPrefix(param.Ref, "#/components/requestBodies/")
			return v3_requestBodyToValue(t, t.Components.RequestBodies[ret])
		}
		return param.Value, nil
	case *openapi3.RequestBody:
		return param, nil
	case string:
		param = strings.TrimPrefix(param, "#/components/requestBodies/")
		return v3_requestBodyToValue(t, t.Components.RequestBodies[param])
	default:
		return nil, utils.Errorf("unsupported parameter type: %T", p)
	}
}

func v3_responseToValue(t openapi3.T, p any) (*openapi3.Response, error) {
	if p == nil {
		return nil, utils.Errorf("unsupported parameter type: %T or nil", p)
	}

	switch param := p.(type) {
	case *openapi3.ResponseRef:
		if param == nil {
			return nil, utils.Error("nil request body ref")
		}
		if param.Ref != "" {
			var ret = strings.TrimPrefix(param.Ref, "#/components/responses/")
			return v3_responseToValue(t, t.Components.Responses[ret])
		}
		return param.Value, nil
	case *openapi3.Response:
		return param, nil
	case string:
		param = strings.TrimPrefix(param, "#/components/responses/")
		return v3_responseToValue(t, t.Components.Responses[param])
	default:
		return nil, utils.Errorf("unsupported parameter type: %T", p)
	}
}

func v3_schemaToValue(t openapi3.T, p any) (*openapi3.Schema, error) {
	if p == nil {
		return nil, utils.Errorf("unsupported parameter type: %T or nil", p)
	}

	switch param := p.(type) {
	case *openapi3.SchemaRef:
		if param == nil {
			return nil, utils.Error("nil request body ref")
		}
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
	if p == nil {
		return nil, utils.Errorf("unsupported parameter type: %T or nil", p)
	}

	switch param := p.(type) {
	case *openapi3.ParameterRef:
		if param == nil {
			return nil, utils.Error("nil request body ref")
		}
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

func v3_mockSchemaValue(data openapi3.T, i *openapi3.Schema, fieldName ...string) *omap.OrderedMap[string, any] {
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
			scheme, err := v3_schemaToValue(data, i.Items.Ref)
			if err != nil {
				log.Errorf("v3_schemaToValue [%v] failed: %v", i.Items.Ref, err)
				return nil
			}
			m.Add(v3_mockSchemaValue(data, scheme, field))
			return m
		}
		m.Add(v3_mockSchemaValue(data, i.Items.Value, field))
		return m
	case "object":
		m := omap.NewGeneralOrderedMap()
		for field, pt := range i.Properties {
			if pt.Ref != "" {
				scheme, err := v3_schemaToValue(data, pt.Ref)
				if err != nil {
					log.Errorf("v3_schemaToValue [%v] failed: %v", i.Items.Ref, err)
					return nil
				}
				m.Set(field, v3_mockSchemaValue(data, scheme, field))
			} else {
				m.Set(field, v3_mockSchemaValue(data, pt.Value, field))
			}
		}
		return m
	default:
		m := omap.NewGeneralOrderedMap()
		m.SetLiteralValue(ValueViaField(field, i.Type, i.Default))
		return m
	}
}

func v3_mockSchemaJson(data openapi3.T, i *openapi3.Schema, fieldName ...string) []byte {
	return v3_mockSchemaValue(data, i, fieldName...).Jsonify()
}
