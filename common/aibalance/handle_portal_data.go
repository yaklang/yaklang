package aibalance

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/healthinfo"
)

// PortalDataResponse is the JSON response structure for portal data API
type PortalDataResponse struct {
	CurrentTime        string             `json:"current_time"`
	TotalProviders     int                `json:"total_providers"`
	HealthyProviders   int                `json:"healthy_providers"`
	TotalRequests      int64              `json:"total_requests"`
	SuccessRate        float64            `json:"success_rate"`
	TotalTraffic       int64              `json:"total_traffic"`
	TotalTrafficStr    string             `json:"total_traffic_str"`
	ConcurrentRequests int64              `json:"concurrent_requests"` // Current in-flight AI requests (chat + embedding)
	WebSearchCount     int64              `json:"web_search_count"`    // Persistent cumulative web-search request count (from database)
	AmapCount          int64              `json:"amap_count"`          // Persistent cumulative amap request count (from database)
	Providers          []ProviderDataJSON `json:"providers"`
	APIKeys            []APIKeyDataJSON   `json:"api_keys"`
	ModelMetas         []ModelInfoJSON    `json:"model_metas"`
	TOTPSecret         string             `json:"totp_secret"`
	TOTPWrapped        string             `json:"totp_wrapped"`
	TOTPCode           string             `json:"totp_code"`

	// 日活与缓存统计：portal 顶部「日活与缓存」单数字卡 + 同名 tab 的 60 天折线图所需。
	// 关键词: PortalDataResponse 日活缓存扩展, today_dau, daily_summary_60_days, dau_60_days, today_cache_stats
	TodayDate           string               `json:"today_date"`
	TodayDAU            int64                `json:"today_dau"`
	TodayDAUBreakdown   DAUBreakdownJSON     `json:"today_dau_breakdown"`
	DailySummary60Days  []DailySummaryJSON   `json:"daily_summary_60_days"`
	DAU60Days           []DAUDailyJSON       `json:"dau_60_days"`
	TodayCacheStats     TodayCacheStatsJSON  `json:"today_cache_stats"`
	TodayCacheBreakdown []CacheBreakdownJSON `json:"today_cache_breakdown"`
	CacheTrend60Days    []CacheTrendDayJSON  `json:"cache_trend_60_days"`

	// 按对外模型 wrapper_name 聚合的 180 天趋势（扁平行），
	// 前端按 180 天日期轴透视后绘制堆叠图与每模型缓存命中折线。
	// 关键词: PortalDataResponse model_trend_180_days, 模型堆叠数据源
	ModelTrend180Days []CacheModelDayJSON `json:"model_trend_180_days"`

	// 主机/数据库分区磁盘空间，避免数据库写爆。
	// 关键词: PortalDataResponse disk_info, 主机可用空间 KPI
	Disk DiskInfoJSON `json:"disk_info"`
}

// DAUBreakdownJSON 是「今日日活按 source_kind 拆分」结构。
// 关键词: today_dau_breakdown, source_kind 拆分
type DAUBreakdownJSON struct {
	APIKey    int64 `json:"api_key"`
	FreeTrace int64 `json:"free_trace"`
	FreeIP    int64 `json:"free_ip"`
	Total     int64 `json:"total"`
}

// DailySummaryJSON 是「日聚合快照」单日 JSON 结构。
// 关键词: daily_summary_60_days, prompt_tokens / completion_tokens / cached_tokens
type DailySummaryJSON struct {
	Date             string `json:"date"`
	TotalRequests    int64  `json:"total_requests"`
	PromptTokens     int64  `json:"prompt_tokens"`
	CompletionTokens int64  `json:"completion_tokens"`
	CachedTokens     int64  `json:"cached_tokens"`
}

// DAUDailyJSON 是「日活 60 天折线」单日点结构。
// 关键词: dau_60_days, api_key/free_trace/free_ip/total 折线
type DAUDailyJSON struct {
	Date      string `json:"date"`
	APIKey    int64  `json:"api_key"`
	FreeTrace int64  `json:"free_trace"`
	FreeIP    int64  `json:"free_ip"`
	Total     int64  `json:"total"`
}

// TodayCacheStatsJSON 是「今日缓存命中聚合」单数字 KPI 结构。
// 关键词: today_cache_stats, hit_ratio
type TodayCacheStatsJSON struct {
	RequestCount     int64   `json:"request_count"`
	PromptTokens     int64   `json:"prompt_tokens"`
	CompletionTokens int64   `json:"completion_tokens"`
	TotalTokens      int64   `json:"total_tokens"`
	CachedTokens     int64   `json:"cached_tokens"`
	HitRatio         float64 `json:"hit_ratio"`
}

// CacheBreakdownJSON 是「今日 (model, provider, key) 拆分」表行 JSON 结构。
// 关键词: today_cache_breakdown, 模型 + provider + key 拆分明细
type CacheBreakdownJSON struct {
	WrapperName      string  `json:"wrapper_name"`
	ModelName        string  `json:"model_name"`
	ProviderTypeName string  `json:"provider_type_name"`
	ProviderDomain   string  `json:"provider_domain"`
	APIKeyShrink     string  `json:"api_key_shrink"`
	RequestCount     int64   `json:"request_count"`
	PromptTokens     int64   `json:"prompt_tokens"`
	CompletionTokens int64   `json:"completion_tokens"`
	TotalTokens      int64   `json:"total_tokens"`
	CachedTokens     int64   `json:"cached_tokens"`
	HitRatio         float64 `json:"hit_ratio"`
}

// CacheTrendDayJSON 是「缓存命中比例 60 天折线」单点结构。
// 关键词: cache_trend_60_days, hit_ratio 折线点
type CacheTrendDayJSON struct {
	Date         string  `json:"date"`
	PromptTokens int64   `json:"prompt_tokens"`
	CachedTokens int64   `json:"cached_tokens"`
	HitRatio     float64 `json:"hit_ratio"`
}

// CacheModelDayJSON 是「(date, model) 二维聚合点」JSON 结构，
// model 字段对应 ai_daily_cache_stats.wrapper_name（对外模型名）。
// 关键词: model_trend_180_days, 按模型每日聚合 JSON
type CacheModelDayJSON struct {
	Date             string `json:"date"`
	Model            string `json:"model"`
	RequestCount     int64  `json:"request_count"`
	PromptTokens     int64  `json:"prompt_tokens"`
	CompletionTokens int64  `json:"completion_tokens"`
	TotalTokens      int64  `json:"total_tokens"`
	CachedTokens     int64  `json:"cached_tokens"`
}

// DiskInfoJSON 是主机/DB 分区可用空间快照。
// 关键词: disk_info, 主机磁盘空间, healthinfo.DiskUsageWithContext
type DiskInfoJSON struct {
	Path        string  `json:"path"`
	Total       uint64  `json:"total"`
	Free        uint64  `json:"free"`
	Used        uint64  `json:"used"`
	UsedPercent float64 `json:"used_percent"`
	FreePercent float64 `json:"free_percent"`
	Available   bool    `json:"available"` // 取数成功则为 true; 失败时其余字段为零值
}

// ProviderDataJSON is the JSON representation of provider data
// 关键词: ProviderDataJSON, /portal/api/data 输出, active_cache_control 暴露给前端
type ProviderDataJSON struct {
	ID                 uint    `json:"id"`
	WrapperName        string  `json:"wrapper_name"`
	ModelName          string  `json:"model_name"`
	TypeName           string  `json:"type_name"`
	DomainOrURL        string  `json:"domain_or_url"`
	APIKey             string  `json:"api_key"`
	TotalRequests      int64   `json:"total_requests"`
	SuccessRate        float64 `json:"success_rate"`
	LastLatency        int64   `json:"last_latency"`
	IsHealthy          bool    `json:"is_healthy"`
	HealthStatusClass  string  `json:"health_status_class"`
	ActiveCacheControl bool    `json:"active_cache_control"`
}

// APIKeyDataJSON is the JSON representation of API key data
type APIKeyDataJSON struct {
	ID                    uint    `json:"id"`
	Key                   string  `json:"key"`
	DisplayKey            string  `json:"display_key"`
	AllowedModels         string  `json:"allowed_models"`
	CreatedAt             string  `json:"created_at"`
	LastUsedAt            string  `json:"last_used_at"`
	UsageCount            int64   `json:"usage_count"`
	SuccessCount          int64   `json:"success_count"`
	FailureCount          int64   `json:"failure_count"`
	InputBytes            int64   `json:"input_bytes"`
	OutputBytes           int64   `json:"output_bytes"`
	InputBytesFormatted   string  `json:"input_bytes_formatted"`
	OutputBytesFormatted  string  `json:"output_bytes_formatted"`
	Active                bool    `json:"active"`
	TrafficLimit          int64   `json:"traffic_limit"`
	TrafficUsed           int64   `json:"traffic_used"`
	TrafficLimitEnable    bool    `json:"traffic_limit_enable"`
	TrafficLimitFormatted string  `json:"traffic_limit_formatted"`
	TrafficUsedFormatted  string  `json:"traffic_used_formatted"`
	TrafficPercent        float64 `json:"traffic_percent"`
}

// ModelInfoJSON is the JSON representation of model metadata
type ModelInfoJSON struct {
	Name              string  `json:"name"`
	Description       string  `json:"description"`
	Tags              string  `json:"tags"`
	ProviderCount     int     `json:"provider_count"`
	TrafficMultiplier float64 `json:"traffic_multiplier"`

	// 4 \u7ef4 Token \u500d\u7387\uff080 \u8868\u793a\u672a\u914d\u7f6e\uff0cUI \u8868\u73b0\u4e3a\u300c\u9ed8\u8ba4\u300d\uff09
	// \u5173\u952e\u8bcd: ModelInfoJSON Token \u500d\u7387\u66b4\u9732
	InputTokenMultiplier    float64 `json:"input_token_multiplier"`
	OutputTokenMultiplier   float64 `json:"output_token_multiplier"`
	CacheCreationMultiplier float64 `json:"cache_creation_multiplier"`
	CacheHitMultiplier      float64 `json:"cache_hit_multiplier"`
}

// servePortalDataAPI handles the /portal/api/data endpoint
// Returns all portal data as JSON for client-side rendering
func (c *ServerConfig) servePortalDataAPI(conn net.Conn, request *http.Request) {
	log.Infof("Serving portal data API")

	// Get all providers
	providers, err := GetAllAiProviders()
	if err != nil {
		c.writeJSONResponse(conn, 500, map[string]string{"error": "Failed to get providers"})
		return
	}

	// Prepare response data
	data := PortalDataResponse{
		CurrentTime:   time.Now().Format("2006-01-02 15:04:05"),
		TotalRequests: 0,
		Providers:     make([]ProviderDataJSON, 0),
		APIKeys:       make([]APIKeyDataJSON, 0),
		ModelMetas:    make([]ModelInfoJSON, 0),
	}

	// Process provider data
	var totalSuccess int64
	healthyCount := 0

	for _, p := range providers {
		successRate := 0.0
		if p.TotalRequests > 0 {
			successRate = float64(p.SuccessCount) / float64(p.TotalRequests) * 100
		}

		var healthClass string
		if !p.IsFirstCheckCompleted {
			healthClass = "unknown"
		} else if p.IsHealthy {
			healthClass = "healthy"
		} else {
			healthClass = "unhealthy"
		}

		data.Providers = append(data.Providers, ProviderDataJSON{
			ID:                 p.ID,
			WrapperName:        p.WrapperName,
			ModelName:          p.ModelName,
			TypeName:           p.TypeName,
			DomainOrURL:        p.DomainOrURL,
			APIKey:             p.APIKey,
			TotalRequests:      p.TotalRequests,
			SuccessRate:        successRate,
			LastLatency:        p.LastLatency,
			IsHealthy:          p.IsHealthy,
			HealthStatusClass:  healthClass,
			ActiveCacheControl: p.ActiveCacheControl,
		})

		data.TotalRequests += p.TotalRequests
		totalSuccess += p.SuccessCount
		if p.IsHealthy && p.IsFirstCheckCompleted {
			healthyCount++
		}
	}

	data.TotalProviders = len(providers)
	data.HealthyProviders = healthyCount

	if data.TotalRequests > 0 {
		data.SuccessRate = float64(totalSuccess) / float64(data.TotalRequests) * 100
	}

	// Get Model Metadata
	allMetas, err := GetAllModelMetas()
	if err != nil {
		log.Errorf("Failed to get model metas: %v", err)
	} else {
		modelCounts := make(map[string]int)
		for _, p := range providers {
			name := p.WrapperName
			if name == "" {
				name = p.ModelName
			}
			if name != "" {
				modelCounts[name]++
			}
		}

		for name, count := range modelCounts {
			info := ModelInfoJSON{
				Name:              name,
				ProviderCount:     count,
				TrafficMultiplier: 1.0,
			}
			if meta, ok := allMetas[name]; ok {
				info.Description = meta.Description
				info.Tags = meta.Tags
				if meta.TrafficMultiplier > 0 {
					info.TrafficMultiplier = meta.TrafficMultiplier
				}
				info.InputTokenMultiplier = meta.InputTokenMultiplier
				info.OutputTokenMultiplier = meta.OutputTokenMultiplier
				info.CacheCreationMultiplier = meta.CacheCreationMultiplier
				info.CacheHitMultiplier = meta.CacheHitMultiplier
			}
			data.ModelMetas = append(data.ModelMetas, info)
		}
	}

	// Get API key data from database
	dbApiKeys, err := GetAllAiApiKeys()
	var totalTraffic int64 = 0
	if err == nil {
		for _, apiKey := range dbApiKeys {
			displayKey := apiKey.APIKey
			if len(displayKey) > 8 {
				displayKey = displayKey[:4] + "..." + displayKey[len(displayKey)-4:]
			}

			inputBytesFormatted := formatBytes(apiKey.InputBytes)
			outputBytesFormatted := formatBytes(apiKey.OutputBytes)

			totalTraffic += apiKey.InputBytes + apiKey.OutputBytes

			var trafficPercent float64 = 0
			if apiKey.TrafficLimitEnable && apiKey.TrafficLimit > 0 {
				trafficPercent = float64(apiKey.TrafficUsed) / float64(apiKey.TrafficLimit) * 100
			}

			keyData := APIKeyDataJSON{
				ID:                    apiKey.ID,
				Key:                   apiKey.APIKey,
				DisplayKey:            displayKey,
				AllowedModels:         apiKey.AllowedModels,
				CreatedAt:             apiKey.CreatedAt.Format("2006-01-02 15:04:05"),
				UsageCount:            apiKey.UsageCount,
				SuccessCount:          apiKey.SuccessCount,
				FailureCount:          apiKey.FailureCount,
				InputBytes:            apiKey.InputBytes,
				OutputBytes:           apiKey.OutputBytes,
				InputBytesFormatted:   inputBytesFormatted,
				OutputBytesFormatted:  outputBytesFormatted,
				Active:                apiKey.Active,
				TrafficLimit:          apiKey.TrafficLimit,
				TrafficUsed:           apiKey.TrafficUsed,
				TrafficLimitEnable:    apiKey.TrafficLimitEnable,
				TrafficLimitFormatted: formatBytes(apiKey.TrafficLimit),
				TrafficUsedFormatted:  formatBytes(apiKey.TrafficUsed),
				TrafficPercent:        trafficPercent,
			}

			if !apiKey.LastUsedTime.IsZero() {
				keyData.LastUsedAt = apiKey.LastUsedTime.Format("2006-01-02 15:04:05")
			}

			data.APIKeys = append(data.APIKeys, keyData)
		}
	}

	data.TotalTraffic = totalTraffic
	data.TotalTrafficStr = formatBytes(totalTraffic)

	// Fill concurrent request stats
	chatReqs := atomic.LoadInt64(&c.concurrentChatRequests)
	embeddingReqs := atomic.LoadInt64(&c.concurrentEmbeddingRequests)
	data.ConcurrentRequests = chatReqs + embeddingReqs
	// WebSearchCount reads from the persistent database counter (survives process restarts)
	data.WebSearchCount = GetTotalWebSearchRequests()
	// AmapCount reads from the persistent database counter (survives process restarts)
	data.AmapCount = GetTotalAmapRequests()

	// Fill TOTP data
	data.TOTPSecret = GetTOTPSecret()
	data.TOTPWrapped = GetWrappedTOTPUUID()
	data.TOTPCode = GetCurrentTOTPCode()

	// 日活与缓存统计填充：先把内存 acc 强制 flush 一次，
	// 让 portal 读到的 60 天折线包含「最近 30 秒内还没 flush」的请求。
	// 失败仅 logWarn，不阻塞 portal 数据返回。
	// 关键词: portal data 日活与缓存填充, flushSummaryAccumulator before query
	if err := flushSummaryAccumulator(); err != nil {
		log.Warnf("flush summary accumulator before portal data failed: %v", err)
	}
	c.fillDAUAndCacheStats(&data)

	c.writeJSONResponse(conn, 200, data)
}

// fillDAUAndCacheStats 把 4 类持久化统计批量填充进 PortalDataResponse。
// 任何子查询失败都会被 Warn 日志吞掉，不影响其他字段返回。
// 关键词: fillDAUAndCacheStats, portal 一次性填充 4 类统计
func (c *ServerConfig) fillDAUAndCacheStats(data *PortalDataResponse) {
	today := time.Now().Format("2006-01-02")
	data.TodayDate = today

	if total, err := QueryTodayDAUTotal(); err != nil {
		log.Warnf("portal QueryTodayDAUTotal failed: %v", err)
	} else {
		data.TodayDAU = total
	}

	if dauList, err := QueryDAU60Days(); err != nil {
		log.Warnf("portal QueryDAU60Days failed: %v", err)
		data.DAU60Days = make([]DAUDailyJSON, 0)
	} else {
		data.DAU60Days = make([]DAUDailyJSON, 0, len(dauList))
		for _, d := range dauList {
			data.DAU60Days = append(data.DAU60Days, DAUDailyJSON{
				Date:      d.Date,
				APIKey:    d.APIKey,
				FreeTrace: d.FreeTrace,
				FreeIP:    d.FreeIP,
				Total:     d.Total,
			})
			if d.Date == today {
				data.TodayDAUBreakdown = DAUBreakdownJSON{
					APIKey:    d.APIKey,
					FreeTrace: d.FreeTrace,
					FreeIP:    d.FreeIP,
					Total:     d.Total,
				}
			}
		}
	}

	if summaries, err := QuerySummary60Days(); err != nil {
		log.Warnf("portal QuerySummary60Days failed: %v", err)
		data.DailySummary60Days = make([]DailySummaryJSON, 0)
	} else {
		data.DailySummary60Days = make([]DailySummaryJSON, 0, len(summaries))
		for _, s := range summaries {
			data.DailySummary60Days = append(data.DailySummary60Days, DailySummaryJSON{
				Date:             s.Date,
				TotalRequests:    s.TotalRequests,
				PromptTokens:     s.PromptTokens,
				CompletionTokens: s.CompletionTokens,
				CachedTokens:     s.CachedTokens,
			})
		}
	}

	if total, err := QueryTodayCacheStatsTotal(); err != nil {
		log.Warnf("portal QueryTodayCacheStatsTotal failed: %v", err)
	} else {
		hit := 0.0
		if total.PromptTokens > 0 {
			hit = float64(total.CachedTokens) / float64(total.PromptTokens)
		}
		data.TodayCacheStats = TodayCacheStatsJSON{
			RequestCount:     total.RequestCount,
			PromptTokens:     total.PromptTokens,
			CompletionTokens: total.CompletionTokens,
			TotalTokens:      total.TotalTokens,
			CachedTokens:     total.CachedTokens,
			HitRatio:         hit,
		}
	}

	if rows, err := QueryTodayCacheBreakdown(); err != nil {
		log.Warnf("portal QueryTodayCacheBreakdown failed: %v", err)
		data.TodayCacheBreakdown = make([]CacheBreakdownJSON, 0)
	} else {
		data.TodayCacheBreakdown = make([]CacheBreakdownJSON, 0, len(rows))
		for _, r := range rows {
			hit := 0.0
			if r.PromptTokens > 0 {
				hit = float64(r.CachedTokens) / float64(r.PromptTokens)
			}
			data.TodayCacheBreakdown = append(data.TodayCacheBreakdown, CacheBreakdownJSON{
				WrapperName:      r.WrapperName,
				ModelName:        r.ModelName,
				ProviderTypeName: r.ProviderTypeName,
				ProviderDomain:   r.ProviderDomain,
				APIKeyShrink:     r.APIKeyShrink,
				RequestCount:     r.RequestCount,
				PromptTokens:     r.PromptTokens,
				CompletionTokens: r.CompletionTokens,
				TotalTokens:      r.TotalTokens,
				CachedTokens:     r.CachedTokens,
				HitRatio:         hit,
			})
		}
	}

	if trend, err := QueryCacheTrend60Days(); err != nil {
		log.Warnf("portal QueryCacheTrend60Days failed: %v", err)
		data.CacheTrend60Days = make([]CacheTrendDayJSON, 0)
	} else {
		data.CacheTrend60Days = make([]CacheTrendDayJSON, 0, len(trend))
		for _, t := range trend {
			data.CacheTrend60Days = append(data.CacheTrend60Days, CacheTrendDayJSON{
				Date:         t.Date,
				PromptTokens: t.PromptTokens,
				CachedTokens: t.CachedTokens,
				HitRatio:     t.HitRatio,
			})
		}
	}

	// 按对外模型聚合的 180 天趋势 (扁平行)。前端透视成 series 后绘制堆叠图。
	// 关键词: portal fill ModelTrend180Days, 按模型堆叠数据
	if modelTrend, err := QueryCacheModelTrend180Days(); err != nil {
		log.Warnf("portal QueryCacheModelTrend180Days failed: %v", err)
		data.ModelTrend180Days = make([]CacheModelDayJSON, 0)
	} else {
		data.ModelTrend180Days = make([]CacheModelDayJSON, 0, len(modelTrend))
		for _, m := range modelTrend {
			data.ModelTrend180Days = append(data.ModelTrend180Days, CacheModelDayJSON{
				Date:             m.Date,
				Model:            m.WrapperName,
				RequestCount:     m.RequestCount,
				PromptTokens:     m.PromptTokens,
				CompletionTokens: m.CompletionTokens,
				TotalTokens:      m.TotalTokens,
				CachedTokens:     m.CachedTokens,
			})
		}
	}

	// 主机/数据库分区磁盘可用空间, 避免 DB 写爆。
	// 失败仅 Warn, 前端会按 available=false 显示「N/A」。
	// 关键词: portal fill disk_info, healthinfo.DiskUsageWithContext, GetDefaultYakitBaseDir
	data.Disk = collectDiskInfo()
}

// collectDiskInfo 探测「yakit base dir 所在分区」的磁盘空间，
// 用 healthinfo 跨平台 Statfs 实现。任何失败一律 Warn 日志吞掉，
// 返回 available=false 的零值结构以便前端友好降级。
// 关键词: collectDiskInfo, healthinfo, GetDefaultYakitBaseDir 分区
func collectDiskInfo() DiskInfoJSON {
	out := DiskInfoJSON{Available: false}
	defer func() {
		if r := recover(); r != nil {
			log.Warnf("collectDiskInfo recovered: %v", r)
		}
	}()

	path := consts.GetDefaultYakitBaseDir()
	if path == "" {
		path = "."
	}
	out.Path = path

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	stat, err := healthinfo.DiskUsageWithContext(ctx, path)
	if err != nil || stat == nil {
		if err != nil {
			log.Warnf("collectDiskInfo failed for path %s: %v", path, err)
		}
		return out
	}
	out.Available = true
	out.Total = stat.Total
	out.Free = stat.Free
	out.Used = stat.Used
	out.UsedPercent = stat.UsedPercent
	if stat.Total > 0 {
		out.FreePercent = float64(stat.Free) / float64(stat.Total) * 100.0
	}
	return out
}

// serveAvailableModelsAPI returns list of unique model wrapper names for dropdowns
func (c *ServerConfig) serveAvailableModelsAPI(conn net.Conn, request *http.Request) {
	providers, err := GetAllAiProviders()
	if err != nil {
		c.writeJSONResponse(conn, 500, map[string]string{"error": "Failed to get providers"})
		return
	}

	// Get unique wrapper names
	modelSet := make(map[string]bool)
	for _, p := range providers {
		name := p.WrapperName
		if name == "" {
			name = p.ModelName
		}
		if name != "" {
			modelSet[name] = true
		}
	}

	models := make([]string, 0, len(modelSet))
	for name := range modelSet {
		models = append(models, name)
	}

	c.writeJSONResponse(conn, 200, map[string]interface{}{
		"models": models,
	})
}

// escapeHTML escapes HTML special characters
func escapeHTML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	s = strings.ReplaceAll(s, "'", "&#39;")
	return s
}

// serveStaticPortalHTML serves the static portal.html file
func (c *ServerConfig) serveStaticPortalHTML(conn net.Conn) {
	// Read static HTML file
	htmlContent, err := templatesFS.ReadFile("templates/portal.html")
	if err != nil {
		log.Errorf("Failed to read portal.html: %v", err)
		c.writeJSONResponse(conn, 500, map[string]string{"error": "Failed to load portal page"})
		return
	}

	header := fmt.Sprintf("HTTP/1.1 200 OK\r\n"+
		"Content-Type: text/html; charset=utf-8\r\n"+
		"Content-Length: %d\r\n"+
		"\r\n", len(htmlContent))

	conn.Write([]byte(header))
	conn.Write(htmlContent)
}
