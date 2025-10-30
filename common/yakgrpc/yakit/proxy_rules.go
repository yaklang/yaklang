package yakit

import (
	"strings"

	"github.com/jinzhu/gorm"
	"github.com/samber/lo"
	"github.com/segmentio/ksuid"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func ensureProxyID(id string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return ksuid.New().String()
	}
	return id
}

func normalizePatterns(patterns []string) []string {
	res := make([]string, 0, len(patterns))
	for _, pattern := range patterns {
		pattern = strings.TrimSpace(pattern)
		if pattern == "" {
			continue
		}
		res = append(res, pattern)
	}
	return lo.Uniq(res)
}

func GetGlobalProxyRulesConfig() (*ypb.GlobalProxyRulesConfig, error) {
	db := consts.GetGormProfileDatabase()
	if db == nil {
		return &ypb.GlobalProxyRulesConfig{}, nil
	}
	cfg := &ypb.GlobalProxyRulesConfig{}

	var endpoints []schema.ProxyEndpoint
	if err := db.Find(&endpoints).Error; err != nil && !gorm.IsRecordNotFoundError(err) {
		return nil, err
	}
	for _, endpoint := range endpoints {
		cfg.Endpoints = append(cfg.Endpoints, endpoint.ToProto())
	}

	var routes []schema.ProxyRoute
	if err := db.Find(&routes).Error; err != nil && !gorm.IsRecordNotFoundError(err) {
		return nil, err
	}
	for _, route := range routes {
		cfg.Routes = append(cfg.Routes, route.ToProto())
	}

	return cfg, nil
}

func SetGlobalProxyRulesConfig(cfg *ypb.GlobalProxyRulesConfig) (*ypb.GlobalProxyRulesConfig, error) {
	if cfg == nil {
		cfg = &ypb.GlobalProxyRulesConfig{}
	}
	db := consts.GetGormProfileDatabase()
	if db == nil {
		return cfg, utils.Error("profile database not initialized")
	}

	normalized := &ypb.GlobalProxyRulesConfig{}
	endpointIDs := make(map[string]struct{})
	for _, endpoint := range cfg.GetEndpoints() {
		if endpoint == nil {
			continue
		}
		id := ensureProxyID(endpoint.GetId())
		endpointIdsLenBefore := len(endpointIDs)
		endpointIDs[id] = struct{}{}
		if len(endpointIDs) == endpointIdsLenBefore {
			// duplicated id, append random suffix
			id = ensureProxyID(id + "-" + ksuid.New().String())
			endpointIDs[id] = struct{}{}
		}
		url := strings.TrimSpace(endpoint.GetUrl())
		if url == "" {
			continue
		}
		normalized.Endpoints = append(normalized.Endpoints, &ypb.ProxyEndpoint{
			Id:   id,
			Name: strings.TrimSpace(endpoint.GetName()),
			Url:  url,
		})
	}

	validEndpointIDs := make(map[string]struct{}, len(normalized.Endpoints))
	for _, endpoint := range normalized.Endpoints {
		validEndpointIDs[endpoint.GetId()] = struct{}{}
	}

	routeIDs := make(map[string]struct{})
	for _, route := range cfg.GetRoutes() {
		if route == nil {
			continue
		}
		id := ensureProxyID(route.GetId())
		if _, exists := routeIDs[id]; exists {
			id = ensureProxyID(id + "-" + ksuid.New().String())
		}
		routeIDs[id] = struct{}{}
		filteredPatterns := normalizePatterns(route.GetPatterns())
		if len(filteredPatterns) == 0 {
			continue
		}
		filteredEndpointIDs := lo.Filter(route.GetEndpointIds(), func(item string, _ int) bool {
			_, ok := validEndpointIDs[item]
			return ok
		})
		if len(filteredEndpointIDs) == 0 {
			continue
		}
		normalized.Routes = append(normalized.Routes, &ypb.ProxyRoute{
			Id:          id,
			Name:        strings.TrimSpace(route.GetName()),
			Patterns:    filteredPatterns,
			EndpointIds: lo.Uniq(filteredEndpointIDs),
		})
	}

	tx := db.Begin()
	if tx.Error != nil {
		return nil, tx.Error
	}
	err := func(tx *gorm.DB) error {
		if err := tx.Unscoped().Where("1 = 1").Delete(&schema.ProxyRoute{}).Error; err != nil {
			return err
		}
		if err := tx.Unscoped().Where("1 = 1").Delete(&schema.ProxyEndpoint{}).Error; err != nil {
			return err
		}

		for _, endpoint := range normalized.Endpoints {
			model := &schema.ProxyEndpoint{
				ExternalID: endpoint.GetId(),
				Name:       endpoint.GetName(),
				URL:        endpoint.GetUrl(),
			}
			if err := tx.Create(model).Error; err != nil {
				return err
			}
		}

		for _, route := range normalized.Routes {
			model := &schema.ProxyRoute{
				ExternalID: route.GetId(),
				Name:       route.GetName(),
			}
			model.UpdatePatterns(route.GetPatterns())
			model.UpdateEndpointIDs(route.GetEndpointIds())
			if err := tx.Create(model).Error; err != nil {
				return err
			}
		}
		return nil
	}(tx)

	if err != nil {
		tx.Rollback()
		return nil, err
	}
	if commitErr := tx.Commit().Error; commitErr != nil {
		return nil, commitErr
	}

	return normalized, nil
}
