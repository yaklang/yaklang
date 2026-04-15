package aibalance

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

// PublicStatsResponse is the public (unauthenticated) stats response.
// It intentionally excludes sensitive data like API keys, TOTP, domain URLs, etc.
type PublicStatsResponse struct {
	CurrentTime        string              `json:"current_time"`
	TotalProviders     int                 `json:"total_providers"`
	HealthyProviders   int                 `json:"healthy_providers"`
	TotalRequests      int64               `json:"total_requests"`
	SuccessRate        float64             `json:"success_rate"`
	TotalTrafficStr    string              `json:"total_traffic_str"`
	TotalTrafficBytes  int64               `json:"total_traffic_bytes"`
	EstimatedTokens    string              `json:"estimated_tokens"`
	ConcurrentRequests int64               `json:"concurrent_requests"`
	QueueCount         int64               `json:"queue_count"`
	WebSearchCount     int64               `json:"web_search_count"`
	AmapCount          int64               `json:"amap_count"`
	MemoryMB           uint64                        `json:"memory_mb"`
	Models             []PublicModelInfo             `json:"models"`
	UptimeSummary      []PublicUptimeEntry           `json:"uptime_summary"`
	LatencyHistory     map[string][]LatencyPoint     `json:"latency_history"`
}

// PublicModelInfo is a sanitized model entry for public display.
type PublicModelInfo struct {
	DisplayName   string  `json:"display_name"`
	OriginalName  string  `json:"original_name"`
	IsMemfit      bool    `json:"is_memfit"`
	IsFree        bool    `json:"is_free"`
	ProviderCount int     `json:"provider_count"`
	IsHealthy     bool    `json:"is_healthy"`
	SuccessRate   float64 `json:"success_rate"`
	Description   string  `json:"description"`
	Tags          string  `json:"tags"`
}

// PublicUptimeEntry is a per-model uptime summary for public display.
type PublicUptimeEntry struct {
	ModelName   string  `json:"model_name"`
	UptimeRate  float64 `json:"uptime_rate"`
	TotalChecks int64   `json:"total_checks"`
}

var (
	publicStatsCache    *PublicStatsResponse
	publicStatsCacheMu  sync.RWMutex
	publicStatsCoolDown *utils.CoolDown
	publicStatsCDOnce   sync.Once
)

func getPublicStatsCoolDown() *utils.CoolDown {
	publicStatsCDOnce.Do(func() {
		publicStatsCoolDown = utils.NewCoolDown(30 * time.Second)
	})
	return publicStatsCoolDown
}

// servePublicStats handles GET /public/stats with 30s CoolDown caching
func (c *ServerConfig) servePublicStats(conn net.Conn, request *http.Request) {
	defer func() {
		if r := recover(); r != nil {
			log.Errorf("panic in servePublicStats: %v", r)
			c.writeJSONResponse(conn, 500, map[string]string{"error": "internal error"})
		}
	}()

	cd := getPublicStatsCoolDown()
	cd.DoOr(
		func() {
			start := time.Now()
			log.Infof("public stats: cooldown elapsed, refreshing cache...")
			data := c.computePublicStats()
			publicStatsCacheMu.Lock()
			publicStatsCache = data
			publicStatsCacheMu.Unlock()
			log.Infof("public stats: cache refreshed in %v", time.Since(start))
		},
		func() {},
	)

	publicStatsCacheMu.RLock()
	cached := publicStatsCache
	publicStatsCacheMu.RUnlock()

	if cached == nil {
		log.Infof("public stats: cache not yet populated, computing initial data...")
		data := c.computePublicStats()
		publicStatsCacheMu.Lock()
		if publicStatsCache == nil {
			publicStatsCache = data
		}
		cached = publicStatsCache
		publicStatsCacheMu.Unlock()
	}

	c.writeJSONResponse(conn, 200, cached)
}

// computePublicStats does the actual data collection (called at most once per 30s)
func (c *ServerConfig) computePublicStats() *PublicStatsResponse {
	start := time.Now()

	data := &PublicStatsResponse{
		CurrentTime: time.Now().Format("2006-01-02 15:04:05"),
		Models:      make([]PublicModelInfo, 0),
	}

	// Step 1: Get providers
	providers, err := GetAllAiProviders()
	if err != nil {
		log.Warnf("public stats: GetAllAiProviders failed in %v: %v", time.Since(start), err)
		return data
	}
	log.Infof("public stats: got %d providers in %v", len(providers), time.Since(start))

	type modelAgg struct {
		providerCount   int
		totalRequests   int64
		successCount    int64
		allHealthy      bool
		anyFirstChecked bool
	}
	modelMap := make(map[string]*modelAgg)

	var totalSuccess int64
	healthyCount := 0

	for _, p := range providers {
		name := p.WrapperName
		if name == "" {
			name = p.ModelName
		}
		if name == "" {
			continue
		}

		data.TotalRequests += p.TotalRequests
		totalSuccess += p.SuccessCount

		if p.IsHealthy && p.IsFirstCheckCompleted {
			healthyCount++
		}

		agg, ok := modelMap[name]
		if !ok {
			agg = &modelAgg{allHealthy: true}
			modelMap[name] = agg
		}
		agg.providerCount++
		agg.totalRequests += p.TotalRequests
		agg.successCount += p.SuccessCount
		if p.IsFirstCheckCompleted {
			agg.anyFirstChecked = true
			if !p.IsHealthy {
				agg.allHealthy = false
			}
		}
	}

	data.TotalProviders = len(providers)
	data.HealthyProviders = healthyCount
	if data.TotalRequests > 0 {
		data.SuccessRate = float64(totalSuccess) / float64(data.TotalRequests) * 100
	}

	// Step 2: Traffic from API keys
	dbApiKeys, err := GetAllAiApiKeys()
	var totalTraffic int64
	if err == nil {
		for _, apiKey := range dbApiKeys {
			totalTraffic += apiKey.InputBytes + apiKey.OutputBytes
		}
		log.Infof("public stats: traffic from %d API keys, total %s in %v", len(dbApiKeys), formatBytes(totalTraffic), time.Since(start))
	} else {
		log.Warnf("public stats: GetAllAiApiKeys failed: %v", err)
	}
	data.TotalTrafficBytes = totalTraffic
	data.TotalTrafficStr = formatBytes(totalTraffic)
	data.EstimatedTokens = estimateTokens(totalTraffic)

	// Step 3: Concurrent requests (in-memory, fast)
	chatReqs := atomic.LoadInt64(&c.concurrentChatRequests)
	embeddingReqs := atomic.LoadInt64(&c.concurrentEmbeddingRequests)
	data.ConcurrentRequests = chatReqs + embeddingReqs

	// Step 3.5: Queue count from rate limiter
	if c.chatRateLimiter != nil {
		data.QueueCount = c.chatRateLimiter.GetQueueCount()
	}

	// Step 4: Web search / Amap counts
	func() {
		defer func() {
			if r := recover(); r != nil {
				log.Warnf("public stats: web search/amap count panic: %v", r)
			}
		}()
		data.WebSearchCount = GetTotalWebSearchRequests()
		data.AmapCount = GetTotalAmapRequests()
	}()

	// Step 5: Memory (in-memory, fast)
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	data.MemoryMB = memStats.Alloc / 1024 / 1024

	// Step 6: Model metadata
	allMetas, _ := GetAllModelMetas()

	for name, agg := range modelMap {
		if agg.providerCount == 0 {
			continue
		}

		displayName, isMemfit, isFree := processModelName(name)
		successRate := 0.0
		if agg.totalRequests > 0 {
			successRate = float64(agg.successCount) / float64(agg.totalRequests) * 100
		}
		isHealthy := agg.allHealthy && agg.anyFirstChecked

		info := PublicModelInfo{
			DisplayName:   displayName,
			OriginalName:  name,
			IsMemfit:      isMemfit,
			IsFree:        isFree,
			ProviderCount: agg.providerCount,
			IsHealthy:     isHealthy,
			SuccessRate:   successRate,
		}
		if allMetas != nil {
			if meta, ok := allMetas[name]; ok {
				info.Description = meta.Description
				info.Tags = meta.Tags
			}
		}
		data.Models = append(data.Models, info)
	}

	sort.SliceStable(data.Models, func(i, j int) bool {
		a, b := data.Models[i], data.Models[j]
		if a.IsMemfit != b.IsMemfit {
			return a.IsMemfit
		}
		if a.IsFree != b.IsFree {
			return !a.IsFree
		}
		if a.ProviderCount != b.ProviderCount {
			return a.ProviderCount > b.ProviderCount
		}
		return a.DisplayName < b.DisplayName
	})

	log.Infof("public stats: core data ready in %v, fetching health/latency...", time.Since(start))

	// Step 7: Uptime summary (synchronous, CoolDown protects from frequent calls)
	healthStart := time.Now()
	summaries, err := GetAllHealthSummary(time.Now().Add(-24 * time.Hour))
	if err == nil {
		for _, s := range summaries {
			displayName, _, _ := processModelName(s.WrapperName)
			data.UptimeSummary = append(data.UptimeSummary, PublicUptimeEntry{
				ModelName:   displayName,
				UptimeRate:  s.UptimeRate,
				TotalChecks: s.TotalChecks,
			})
		}
		log.Infof("public stats: GetAllHealthSummary returned %d entries in %v", len(summaries), time.Since(healthStart))
	} else {
		log.Warnf("public stats: GetAllHealthSummary failed in %v: %v", time.Since(healthStart), err)
	}

	// Step 8: Latency history (only for current active models, not stale health_records entries)
	latencyStart := time.Now()
	var currentModelNames []string
	for name := range modelMap {
		currentModelNames = append(currentModelNames, name)
	}
	latencyMap, err := GetRecentLatencyByModel(20, currentModelNames)
	if err == nil && len(latencyMap) > 0 {
		displayLatency := make(map[string][]LatencyPoint)
		totalPoints := 0
		for name, points := range latencyMap {
			displayName, _, _ := processModelName(name)
			displayLatency[displayName] = points
			totalPoints += len(points)
		}
		data.LatencyHistory = displayLatency
		log.Infof("public stats: GetRecentLatencyByModel returned %d models, %d total points in %v",
			len(latencyMap), totalPoints, time.Since(latencyStart))
	} else if err != nil {
		log.Warnf("public stats: GetRecentLatencyByModel failed in %v: %v", time.Since(latencyStart), err)
	}

	log.Infof("public stats: full computation completed in %v (providers=%d, models=%d)",
		time.Since(start), len(providers), len(data.Models))
	return data
}

// servePublicAPI dispatches /public/* routes
func (c *ServerConfig) servePublicAPI(conn net.Conn, request *http.Request, uri *url.URL) {
	switch {
	case uri.Path == "/public/stats":
		c.servePublicStats(conn, request)
	case strings.HasPrefix(uri.Path, "/public/static/"):
		c.serveStaticFile(conn, uri.Path)
	default:
		conn.Write([]byte("HTTP/1.1 404 Not Found\r\n\r\n"))
	}
}

// processModelName handles memfit- prefix removal and -free suffix detection.
// Returns (displayName, isMemfit, isFree).
func processModelName(name string) (string, bool, bool) {
	displayName := name
	isMemfit := false
	isFree := false

	if strings.HasPrefix(name, "memfit-") {
		displayName = strings.TrimPrefix(name, "memfit-")
		isMemfit = true
	}

	if strings.HasSuffix(name, "-free") {
		isFree = true
	}

	return displayName, isMemfit, isFree
}

// estimateTokens provides a rough token estimate from byte count.
// Assumes ~4 bytes per token on average for mixed content.
func estimateTokens(totalBytes int64) string {
	if totalBytes <= 0 {
		return "0"
	}
	tokens := totalBytes / 4
	if tokens >= 1_000_000_000 {
		return fmt.Sprintf("%.1fB", float64(tokens)/1_000_000_000)
	}
	if tokens >= 1_000_000 {
		return fmt.Sprintf("%.1fM", float64(tokens)/1_000_000)
	}
	if tokens >= 1_000 {
		return fmt.Sprintf("%.1fK", float64(tokens)/1_000)
	}
	return fmt.Sprintf("%d", tokens)
}
