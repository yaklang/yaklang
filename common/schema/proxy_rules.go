package schema

import (
	"encoding/json"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type ProxyEndpoint struct {
	gorm.Model
	ExternalID string `gorm:"column:external_id;unique_index"`
	Name       string `gorm:"column:name"`
	URL        string `gorm:"column:url"`
}

func (p *ProxyEndpoint) ToProto() *ypb.ProxyEndpoint {
	if p == nil {
		return nil
	}
	return &ypb.ProxyEndpoint{
		Id:   p.ExternalID,
		Name: p.Name,
		Url:  p.URL,
	}
}

type ProxyRoute struct {
	gorm.Model
	ExternalID   string `gorm:"column:external_id;unique_index"`
	Name         string `gorm:"column:name"`
	PatternsRaw  string `gorm:"column:patterns;type:text"`
	EndpointsRaw string `gorm:"column:endpoint_ids;type:text"`
}

func (p *ProxyRoute) Patterns() []string {
	if p == nil || p.PatternsRaw == "" {
		return nil
	}
	var patterns []string
	if err := json.Unmarshal([]byte(p.PatternsRaw), &patterns); err != nil {
		return nil
	}
	return patterns
}

func (p *ProxyRoute) EndpointIDs() []string {
	if p == nil || p.EndpointsRaw == "" {
		return nil
	}
	var ids []string
	if err := json.Unmarshal([]byte(p.EndpointsRaw), &ids); err != nil {
		return nil
	}
	return ids
}

func (p *ProxyRoute) ToProto() *ypb.ProxyRoute {
	if p == nil {
		return nil
	}
	return &ypb.ProxyRoute{
		Id:          p.ExternalID,
		Name:        p.Name,
		Patterns:    p.Patterns(),
		EndpointIds: p.EndpointIDs(),
	}
}

func (p *ProxyRoute) UpdatePatterns(patterns []string) {
	raw, _ := json.Marshal(patterns)
	p.PatternsRaw = string(raw)
}

func (p *ProxyRoute) UpdateEndpointIDs(ids []string) {
	raw, _ := json.Marshal(ids)
	p.EndpointsRaw = string(raw)
}
