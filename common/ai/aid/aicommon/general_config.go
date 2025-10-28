package aicommon

import "github.com/yaklang/yaklang/common/utils/omap"

type GeneralKVConfig struct {
	config *omap.OrderedMap[string, any]
}

type GeneralKVConfigOption func(*GeneralKVConfig)

func NewGeneralKVConfig(opts ...GeneralKVConfigOption) *GeneralKVConfig {
	c := &GeneralKVConfig{
		config: omap.NewOrderedMap[string, any](make(map[string]any)),
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

type streamableField struct {
	aiNodeId string
	fieldKey string
}

func (f *streamableField) AINodeId() string {
	return f.aiNodeId
}

func (f *streamableField) FieldKey() string {
	return f.fieldKey
}

func (g *GeneralKVConfig) GetStreamableFields() []interface {
	AINodeId() string
	FieldKey() string
} {
	result, ok := g.config.Get("streamFields")
	if !ok {
		return nil
	}
	fields := result.([]map[string]string)
	var res []interface {
		AINodeId() string
		FieldKey() string
	}
	for _, f := range fields {
		field := &streamableField{
			aiNodeId: f["aiNodeId"],
			fieldKey: f["fieldKey"],
		}
		if field.FieldKey() == "" {
			continue
		}
		res = append(res, field)
	}
	return res
}
