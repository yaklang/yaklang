package aibalance

import (
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/yaklang/yaklang/common/log"
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
}

// ProviderDataJSON is the JSON representation of provider data
type ProviderDataJSON struct {
	ID                uint    `json:"id"`
	WrapperName       string  `json:"wrapper_name"`
	ModelName         string  `json:"model_name"`
	TypeName          string  `json:"type_name"`
	DomainOrURL       string  `json:"domain_or_url"`
	APIKey            string  `json:"api_key"`
	TotalRequests     int64   `json:"total_requests"`
	SuccessRate       float64 `json:"success_rate"`
	LastLatency       int64   `json:"last_latency"`
	IsHealthy         bool    `json:"is_healthy"`
	HealthStatusClass string  `json:"health_status_class"`
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
			ID:                p.ID,
			WrapperName:       p.WrapperName,
			ModelName:         p.ModelName,
			TypeName:          p.TypeName,
			DomainOrURL:       p.DomainOrURL,
			APIKey:            p.APIKey,
			TotalRequests:     p.TotalRequests,
			SuccessRate:       successRate,
			LastLatency:       p.LastLatency,
			IsHealthy:         p.IsHealthy,
			HealthStatusClass: healthClass,
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

	c.writeJSONResponse(conn, 200, data)
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
