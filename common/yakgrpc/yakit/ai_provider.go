package yakit

import (
	"hash/fnv"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

const (
	aiProviderTypeAIBalance = "aibalance"
	aiProviderDefaultAPIKey = "free-user"
	aiProviderDefaultDomain = "aibalance.yaklang.com"
)

func DefaultAIBalanceProviderConfig() *ypb.ThirdPartyApplicationConfig {
	return &ypb.ThirdPartyApplicationConfig{
		Type:   aiProviderTypeAIBalance,
		APIKey: aiProviderDefaultAPIKey,
		Domain: aiProviderDefaultDomain,
	}
}

func ListAIProviders(db *gorm.DB) ([]*ypb.AIProvider, error) {
	if db == nil {
		return nil, utils.Error("no set database")
	}
	cfg, err := GetAIGlobalConfig(db)
	if err != nil {
		return nil, err
	}
	return buildProvidersFromGlobalConfig(cfg), nil
}

func QueryAIProviders(db *gorm.DB, filter *ypb.AIProviderFilter, paging *ypb.Paging) (*bizhelper.Paginator, []*ypb.AIProvider, error) {
	if db == nil {
		return nil, nil, utils.Error("no set database")
	}

	providers, err := ListAIProviders(db)
	if err != nil {
		return nil, nil, err
	}

	filtered := filterAIProviders(providers, filter)
	paging = normalizePaging(paging)
	sortAIProviders(filtered, paging.GetOrderBy(), paging.GetOrder())

	total := len(filtered)
	limit := int(paging.GetLimit())
	page := int(paging.GetPage())
	offset := 0
	result := filtered

	if limit != -1 {
		offset = (page - 1) * limit
		if offset >= total {
			result = nil
		} else {
			end := offset + limit
			if end > total {
				end = total
			}
			result = filtered[offset:end]
		}
	}

	totalPage := 0
	if limit != 0 {
		totalPage = int(math.Ceil(float64(total) / float64(limit)))
	}

	prevPage := page
	if page > 1 {
		prevPage = page - 1
	}
	nextPage := page + 1
	if page == totalPage {
		nextPage = page
	}

	return &bizhelper.Paginator{
		TotalRecord: total,
		TotalPage:   totalPage,
		Records:     result,
		Offset:      offset,
		Limit:       limit,
		Page:        page,
		PrevPage:    prevPage,
		NextPage:    nextPage,
	}, result, nil
}

func UpsertAIProvider(db *gorm.DB, provider *ypb.AIProvider) (*ypb.AIProvider, error) {
	if db == nil {
		return nil, utils.Error("no set database")
	}
	return nil, utils.Error("upsert ai provider is deprecated; update AIGlobalConfig instead")
}

func DeleteAIProvider(db *gorm.DB, id int64) error {
	if db == nil {
		return utils.Error("no set database")
	}
	return utils.Error("delete ai provider is deprecated; update AIGlobalConfig instead")
}

func buildProvidersFromGlobalConfig(cfg *ypb.AIGlobalConfig) []*ypb.AIProvider {
	if cfg == nil {
		return nil
	}

	providers := make(map[string]*ypb.AIProvider)
	addProvider := func(model *ypb.AIModelConfig) {
		if model == nil {
			return
		}
		provider := model.GetProvider()
		if provider == nil {
			return
		}
		hash := providerSignature(provider)
		if hash == "" {
			return
		}
		if _, exists := providers[hash]; exists {
			return
		}
		providerID := model.GetProviderId()
		if providerID == 0 {
			providerID = providerIDFromHash(hash)
		}
		providers[hash] = &ypb.AIProvider{
			Id:     providerID,
			Config: cloneThirdPartyConfig(provider),
		}
	}

	for _, model := range cfg.GetIntelligentModels() {
		addProvider(model)
	}
	for _, model := range cfg.GetLightweightModels() {
		addProvider(model)
	}
	for _, model := range cfg.GetVisionModels() {
		addProvider(model)
	}

	result := make([]*ypb.AIProvider, 0, len(providers))
	for _, provider := range providers {
		result = append(result, provider)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].GetId() == result[j].GetId() {
			return result[i].GetConfig().GetType() < result[j].GetConfig().GetType()
		}
		return result[i].GetId() < result[j].GetId()
	})

	return result
}

func filterAIProviders(providers []*ypb.AIProvider, filter *ypb.AIProviderFilter) []*ypb.AIProvider {
	if filter == nil {
		return providers
	}

	idSet := map[int64]struct{}{}
	for _, id := range filter.GetIds() {
		idSet[id] = struct{}{}
	}
	typeSet := map[string]struct{}{}
	for _, t := range filter.GetAIType() {
		typeSet[t] = struct{}{}
	}

	result := make([]*ypb.AIProvider, 0, len(providers))
	for _, provider := range providers {
		if provider == nil || provider.GetConfig() == nil {
			continue
		}
		if len(idSet) > 0 {
			if _, ok := idSet[provider.GetId()]; !ok {
				continue
			}
		}
		if len(typeSet) > 0 {
			if _, ok := typeSet[provider.GetConfig().GetType()]; !ok {
				continue
			}
		}
		result = append(result, provider)
	}
	return result
}

func normalizePaging(paging *ypb.Paging) *ypb.Paging {
	if paging == nil {
		return &ypb.Paging{Page: 1, Limit: 10, OrderBy: "id", Order: "asc"}
	}
	if paging.GetPage() <= 0 {
		paging.Page = 1
	}
	if paging.GetLimit() == 0 {
		paging.Limit = 10
	}
	if paging.GetRawOrder() == "" && paging.GetOrderBy() == "" {
		paging.OrderBy = "id"
	}
	if paging.GetRawOrder() == "" && paging.GetOrder() == "" {
		paging.Order = "asc"
	}
	return paging
}

func sortAIProviders(providers []*ypb.AIProvider, orderBy, order string) {
	orderBy = strings.ToLower(strings.TrimSpace(orderBy))
	order = strings.ToLower(strings.TrimSpace(order))
	if orderBy == "" {
		orderBy = "id"
	}
	desc := order == "desc"

	sort.Slice(providers, func(i, j int) bool {
		a := providers[i]
		b := providers[j]
		if a == nil || b == nil {
			return a != nil
		}

		var less bool
		switch orderBy {
		case "type":
			at := a.GetConfig().GetType()
			bt := b.GetConfig().GetType()
			if at == bt {
				less = a.GetId() < b.GetId()
			} else {
				less = at < bt
			}
		default:
			if a.GetId() == b.GetId() {
				less = a.GetConfig().GetType() < b.GetConfig().GetType()
			} else {
				less = a.GetId() < b.GetId()
			}
		}

		if desc {
			return !less
		}
		return less
	})
}

func filterModelsByProvider(models []*ypb.AIModelConfig, id int64, removed bool) ([]*ypb.AIModelConfig, bool) {
	if len(models) == 0 {
		return nil, removed
	}
	result := models[:0]
	for _, model := range models {
		if providerMatches(model, id, "") {
			removed = true
			continue
		}
		result = append(result, model)
	}
	if len(result) == 0 {
		return nil, removed
	}
	return result, removed
}

func providerMatches(model *ypb.AIModelConfig, id int64, hash string) bool {
	if model == nil || model.GetProvider() == nil {
		return false
	}
	if id != 0 {
		if model.GetProviderId() == id {
			return true
		}
		modelHash := providerSignature(model.GetProvider())
		if providerIDFromHash(modelHash) == id {
			return true
		}
	}
	if hash == "" {
		return false
	}
	return providerSignature(model.GetProvider()) == hash
}

func providerSignature(cfg *ypb.ThirdPartyApplicationConfig) string {
	if cfg == nil {
		return ""
	}
	extra := make(map[string]string)
	keys := make([]string, 0, len(cfg.GetExtraParams()))
	for _, kv := range cfg.GetExtraParams() {
		if kv == nil {
			continue
		}
		key := kv.GetKey()
		extra[key] = kv.GetValue()
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var builder strings.Builder
	for _, k := range keys {
		builder.WriteString(k)
		builder.WriteString("=")
		builder.WriteString(extra[k])
		builder.WriteString(";")
	}

	return utils.CalcSha256(
		cfg.GetType(),
		cfg.GetAPIKey(),
		cfg.GetUserIdentifier(),
		cfg.GetUserSecret(),
		cfg.GetNamespace(),
		cfg.GetDomain(),
		cfg.GetBaseURL(),
		cfg.GetWebhookURL(),
		builder.String(),
		cfg.GetProxy(),
		cfg.GetNoHttps(),
	)
}

func providerIDFromHash(hash string) int64 {
	if hash == "" {
		return 0
	}
	if len(hash) >= 16 {
		if v, err := strconv.ParseUint(hash[:16], 16, 64); err == nil {
			return int64(v & math.MaxInt64)
		}
	}
	h := fnv.New64a()
	_, _ = h.Write([]byte(hash))
	return int64(h.Sum64() & math.MaxInt64)
}
