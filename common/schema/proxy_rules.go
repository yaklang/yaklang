package schema

import (
	"strings"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type ProxyEndpoint struct {
	gorm.Model
	ExternalID string `gorm:"column:external_id;unique_index"`
	Name       string `gorm:"column:name"`
	Url        string `gorm:"column:url"`
}

func (p *ProxyEndpoint) ToProto() *ypb.ProxyEndpoint {
	if p == nil {
		return nil
	}
	return &ypb.ProxyEndpoint{
		Id:   p.ExternalID,
		Name: p.Name,
		Url:  p.Url,
	}
}

type ProxyRoute struct {
	gorm.Model
	ExternalID string `gorm:"column:external_id;unique_index"`
	Name       string `gorm:"column:name"`
}

func (p *ProxyRoute) ToProto(patterns []string, endpointIds []string) *ypb.ProxyRoute {
	if p == nil {
		return nil
	}
	return &ypb.ProxyRoute{
		Id:          p.ExternalID,
		Name:        p.Name,
		Patterns:    patterns,
		EndpointIds: endpointIds,
	}
}

type ProxyRoutePattern struct {
	gorm.Model
	RouteID uint   `gorm:"column:route_id;index"`
	Pattern string `gorm:"column:pattern"`
}

func (p *ProxyRoutePattern) NormalizedPattern() string {
	if p == nil {
		return ""
	}
	return strings.TrimSpace(p.Pattern)
}

type ProxyRouteEndpoint struct {
	gorm.Model
	RouteID    uint `gorm:"column:route_id;index"`
	EndpointID uint `gorm:"column:endpoint_id;index"`
}
