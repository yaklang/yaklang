package aibalance

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
)

// AmapKeyHealthResult stores the result of a single amap API key health check
type AmapKeyHealthResult struct {
	KeyID     uint   `json:"key_id"`
	APIKey    string `json:"api_key"` // masked
	IsHealthy bool   `json:"is_healthy"`
	LatencyMs int64  `json:"latency_ms"`
	Error     string `json:"error"` // empty means success
}

// amapWeatherProbeResponse is the minimal response structure for health check probe
type amapWeatherProbeResponse struct {
	Status   string `json:"status"`
	Info     string `json:"info"`
	Infocode string `json:"infocode"`
}

// CheckSingleAmapApiKey checks if a single amap API key is valid by calling the weather API.
// Uses GET https://restapi.amap.com/v3/weather/weatherInfo?city=110000&key=xxx
// Returns: healthy, latencyMs, error
func CheckSingleAmapApiKey(apiKey string) (bool, int64, error) {
	rawReq := []byte("GET /v3/weather/weatherInfo?city=110000&key=" + apiKey + "&output=JSON HTTP/1.1\r\nHost: restapi.amap.com\r\nUser-Agent: yaklang-aibalance\r\n\r\n")

	startTime := time.Now()
	rspIns, err := lowhttp.HTTPWithoutRedirect(
		lowhttp.WithHttps(true),
		lowhttp.WithRequest(rawReq),
		lowhttp.WithHost("restapi.amap.com"),
		lowhttp.WithTimeout(10*time.Second),
	)
	latencyMs := time.Since(startTime).Milliseconds()

	if err != nil {
		return false, latencyMs, fmt.Errorf("http request failed: %v", err)
	}

	statusCode := lowhttp.GetStatusCodeFromResponse(rspIns.RawPacket)
	if statusCode != http.StatusOK {
		return false, latencyMs, fmt.Errorf("http status %d", statusCode)
	}

	body := lowhttp.GetHTTPPacketBody(rspIns.RawPacket)
	if len(body) == 0 {
		return false, latencyMs, fmt.Errorf("empty response body")
	}

	var probeResp amapWeatherProbeResponse
	if err := json.Unmarshal(body, &probeResp); err != nil {
		return false, latencyMs, fmt.Errorf("failed to parse response: %v", err)
	}

	if probeResp.Status != "1" {
		return false, latencyMs, fmt.Errorf("amap returned error: info=%s, infocode=%s", probeResp.Info, probeResp.Infocode)
	}

	return true, latencyMs, nil
}

// CheckAllAmapApiKeys checks all active amap API keys concurrently and updates their health status in the DB.
func CheckAllAmapApiKeys() ([]AmapKeyHealthResult, error) {
	keys, err := GetAllActiveAmapApiKeys()
	if err != nil {
		return nil, fmt.Errorf("failed to get active amap api keys: %v", err)
	}

	if len(keys) == 0 {
		log.Infof("no active amap api keys to check")
		return []AmapKeyHealthResult{}, nil
	}

	log.Infof("starting health check for %d amap api keys", len(keys))

	var results []AmapKeyHealthResult
	var resultsMu sync.Mutex
	var wg sync.WaitGroup

	// Limit concurrent checks
	maxConcurrent := 5
	semaphore := make(chan struct{}, maxConcurrent)

	for _, key := range keys {
		wg.Add(1)
		semaphore <- struct{}{}

		go func(k *schema.AmapApiKey) {
			defer wg.Done()
			defer func() { <-semaphore }()

			healthy, latencyMs, checkErr := CheckSingleAmapApiKey(k.APIKey)

			checkErrorStr := ""
			if checkErr != nil {
				checkErrorStr = checkErr.Error()
			}

			// Update health status and statistics in DB
			if healthy {
				// Reset consecutive failures on success, increment stats
				if dbErr := GetDB().Model(&schema.AmapApiKey{}).Where("id = ?", k.ID).
					Updates(map[string]interface{}{
						"is_healthy":           true,
						"health_check_time":    time.Now(),
						"last_check_error":     "",
						"last_latency":         latencyMs,
						"consecutive_failures": 0,
						"total_requests":       k.TotalRequests + 1,
						"success_count":        k.SuccessCount + 1,
						"last_used_time":       time.Now(),
					}).Error; dbErr != nil {
					log.Errorf("failed to update amap key health (ID: %d): %v", k.ID, dbErr)
				}
			} else {
				// Increment consecutive failures and failure stats
				newConsecutiveFailures := k.ConsecutiveFailures + 1
				isHealthy := newConsecutiveFailures < 3

				if dbErr := GetDB().Model(&schema.AmapApiKey{}).Where("id = ?", k.ID).
					Updates(map[string]interface{}{
						"is_healthy":           isHealthy,
						"health_check_time":    time.Now(),
						"last_check_error":     checkErrorStr,
						"last_latency":         latencyMs,
						"consecutive_failures": newConsecutiveFailures,
						"total_requests":       k.TotalRequests + 1,
						"failure_count":        k.FailureCount + 1,
						"last_used_time":       time.Now(),
					}).Error; dbErr != nil {
					log.Errorf("failed to update amap key health (ID: %d): %v", k.ID, dbErr)
				}

				if !isHealthy {
					log.Warnf("amap api key (ID: %d) marked as unhealthy after %d consecutive failures", k.ID, newConsecutiveFailures)
				}

				healthy = isHealthy
			}

			// Increment global amap request counter
			if incErr := IncrementAmapConfigTotalRequests(); incErr != nil {
				log.Errorf("failed to increment amap counter during health check: %v", incErr)
			}

			result := AmapKeyHealthResult{
				KeyID:     k.ID,
				APIKey:    maskAPIKey(k.APIKey),
				IsHealthy: healthy,
				LatencyMs: latencyMs,
				Error:     checkErrorStr,
			}

			resultsMu.Lock()
			results = append(results, result)
			resultsMu.Unlock()

			if healthy {
				log.Infof("amap key health check passed (ID: %d), latency=%dms", k.ID, latencyMs)
			} else {
				log.Warnf("amap key health check failed (ID: %d): %s, latency=%dms", k.ID, checkErrorStr, latencyMs)
			}
		}(key)
	}

	wg.Wait()

	healthyCount := 0
	for _, r := range results {
		if r.IsHealthy {
			healthyCount++
		}
	}
	log.Infof("amap health check completed: %d/%d keys healthy", healthyCount, len(results))

	return results, nil
}

// StartAmapHealthCheckScheduler starts a background scheduler that checks all amap API keys every 1 hour.
// It runs an immediate check on startup, then every 1 hour.
// Pass stopCh to signal the scheduler to stop.
func StartAmapHealthCheckScheduler(stopCh chan struct{}) {
	// Immediate first check
	go func() {
		log.Infof("running initial amap api key health check...")
		results, err := CheckAllAmapApiKeys()
		if err != nil {
			log.Errorf("initial amap health check failed: %v", err)
		} else {
			log.Infof("initial amap health check completed, checked %d keys", len(results))
		}
	}()

	// Periodic check every 1 hour
	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				log.Infof("running periodic amap api key health check...")
				results, err := CheckAllAmapApiKeys()
				if err != nil {
					log.Errorf("periodic amap health check failed: %v", err)
				} else {
					log.Infof("periodic amap health check completed, checked %d keys", len(results))
				}
			case <-stopCh:
				log.Infof("amap health check scheduler stopped")
				return
			}
		}
	}()

	log.Infof("amap health check scheduler started, interval: 1 hour")
}
