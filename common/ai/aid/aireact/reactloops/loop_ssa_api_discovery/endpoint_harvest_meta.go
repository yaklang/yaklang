package loop_ssa_api_discovery

import (
	"encoding/json"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
)

// EndpointHarvestReport 多手段端点搜集后的完备性摘要（写入 discovery_sessions.endpoint_harvest_meta_json）。
type EndpointHarvestReport struct {
	GeneratedAt           time.Time         `json:"generated_at"`
	Language              string            `json:"language"`
	SourcesRun            []string          `json:"sources_run"`
	StaticSpringEndpoints int               `json:"static_spring_endpoints"` // Java Spring 条数（兼容旧事件字段）
	StaticHarvestBySource map[string]int    `json:"static_harvest_by_source,omitempty"`
	InsertedRows          int               `json:"inserted_rows"`
	UpdatedRows           int               `json:"updated_rows"`
	TotalHttpEndpoints    int               `json:"total_http_endpoints_after_merge"`
	BySourceCount         map[string]int    `json:"by_source_count"`
	AIOrphanHints         []EndpointOrphan    `json:"ai_orphan_hints,omitempty"`
	Warnings              []string          `json:"warnings,omitempty"`
	Notes                 string            `json:"notes,omitempty"`
}

// EndpointOrphan AI 已入库但在本轮静态抽取合并键 (method,path) 下未命中同一路由的条目（提示人工或规则盲区）。
type EndpointOrphan struct {
	ID           uint   `json:"id"`
	Method       string `json:"method"`
	PathPattern  string `json:"path_pattern"`
	HandlerClass string `json:"handler_class,omitempty"`
	Reason       string `json:"reason"`
}

func persistEndpointHarvestReport(sess *store.DiscoverySession, repo *store.Repository, rep *EndpointHarvestReport) error {
	if sess == nil || repo == nil || rep == nil {
		return nil
	}
	b, err := json.Marshal(rep)
	if err != nil {
		return err
	}
	sess.EndpointHarvestMetaJSON = string(b)
	return repo.UpdateSession(sess)
}
