package aibalance

import (
	"encoding/json"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"

	"github.com/yaklang/yaklang/common/schema"
)

// defaultModelStatsMinRPM is the default minimum RPM threshold used by the
// "hot models" stats endpoint when the caller does not override it. The
// previous value of 20 was too high to surface most tuning candidates, so
// the threshold was lowered to 3 to expose anything beyond trivial traffic.
const defaultModelStatsMinRPM int64 = 3

// handleGetRateLimitConfig returns the global rate-limit configuration.
func (c *ServerConfig) handleGetRateLimitConfig(conn net.Conn, request *http.Request) {
	c.logInfo("processing get rate limit config request")

	if !c.checkAuth(request) {
		c.writeJSONResponse(conn, http.StatusUnauthorized, map[string]string{"error": "Unauthorized"})
		return
	}

	config, err := GetRateLimitConfig()
	if err != nil {
		c.logError("failed to get rate limit config: %v", err)
		c.writeJSONResponse(conn, http.StatusInternalServerError, map[string]string{"error": "failed to get rate limit config"})
		return
	}

	var modelOverrides map[string]int64
	if config.ModelRPMOverrides != "" {
		json.Unmarshal([]byte(config.ModelRPMOverrides), &modelOverrides)
	}
	if modelOverrides == nil {
		modelOverrides = make(map[string]int64)
	}

	// model_delay_overrides 兼容老 int / 新 {min,max} 两种格式。
	// 对外统一返回新格式 map[string]{min,max}，前端按对象解析；旧的
	// 标量 N 自动展开为 {min:N, max:0}，前端识别 max==0 时显示"老 N~2N 兼容"。
	// 关键词: handleGetRateLimitConfig model_delay_overrides 兼容输出
	modelDelayOverrides := parseModelDelayOverrides(config.ModelDelayOverrides)
	if modelDelayOverrides == nil {
		modelDelayOverrides = make(map[string]DelayRange)
	}

	var modelOutputTPSOverrides map[string]int64
	if config.ModelOutputTPSOverrides != "" {
		json.Unmarshal([]byte(config.ModelOutputTPSOverrides), &modelOutputTPSOverrides)
	}
	if modelOutputTPSOverrides == nil {
		modelOutputTPSOverrides = make(map[string]int64)
	}

	// 免费用户 Token 限额相关配置（全局 + 模型级覆盖）
	// 关键词: handleGetRateLimitConfig free_user_token_limit_m, model overrides
	freeUserTokenOverrides := parseFreeUserTokenModelOverrides(config.FreeUserTokenModelOverrides)

	c.writeJSONResponse(conn, http.StatusOK, map[string]interface{}{
		"success": true,
		"config": map[string]interface{}{
			"default_rpm":                     config.DefaultRPM,
			"free_user_delay_sec":             config.FreeUserDelaySec,
			"free_user_delay_max_sec":         config.FreeUserDelayMaxSec,
			"model_rpm_overrides":             modelOverrides,
			"model_delay_overrides":           modelDelayOverrides,
			"free_user_token_limit_m":         config.FreeUserTokenLimitM,
			"free_user_token_model_overrides": freeUserTokenOverrides,
			"free_user_output_tps":            config.FreeUserOutputTPS,
			"model_output_tps_overrides":      modelOutputTPSOverrides,
			"free_user_token_soft_limit_m":    config.FreeUserTokenSoftLimitM,
			"free_user_soft_limit_tps":        config.FreeUserSoftLimitTPS,
		},
	})
}

// parseModelDelayOverrides 解析 ModelDelayOverrides JSON 字符串，支持两种历史格式：
//
//	1) 老格式 map[string]int64  -> {"slow-free": 30}
//	2) 新格式 map[string]DelayRange -> {"slow-free": {"min": 0, "max": 5}}
//
// 老格式自动转换为 DelayRange{Min: N, Max: 0}，前端展示时识别 Max=0 显示成
// "N~2N 兼容"占位即可。任何异常都返回空 map，不阻塞业务。
// 关键词: parseModelDelayOverrides, 老 int 兼容, 新 DelayRange
func parseModelDelayOverrides(raw string) map[string]DelayRange {
	out := make(map[string]DelayRange)
	if strings.TrimSpace(raw) == "" {
		return out
	}
	rawMap := make(map[string]json.RawMessage)
	if err := json.Unmarshal([]byte(raw), &rawMap); err != nil {
		return out
	}
	for k, v := range rawMap {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		var legacy int64
		if err := json.Unmarshal(v, &legacy); err == nil {
			if legacy < 0 {
				legacy = 0
			}
			out[k] = DelayRange{Min: legacy, Max: 0}
			continue
		}
		var rng struct {
			Min int64 `json:"min"`
			Max int64 `json:"max"`
		}
		if err := json.Unmarshal(v, &rng); err == nil {
			if rng.Min < 0 {
				rng.Min = 0
			}
			if rng.Max < 0 {
				rng.Max = 0
			}
			out[k] = DelayRange{Min: rng.Min, Max: rng.Max}
		}
	}
	return out
}

// handleSetRateLimitConfig updates the global rate-limit configuration.
func (c *ServerConfig) handleSetRateLimitConfig(conn net.Conn, request *http.Request) {
	c.logInfo("processing set rate limit config request")

	if !c.checkAuth(request) {
		c.writeJSONResponse(conn, http.StatusUnauthorized, map[string]string{"error": "Unauthorized"})
		return
	}

	bodyBytes, err := io.ReadAll(request.Body)
	if err != nil {
		c.logError("failed to read request body: %v", err)
		c.writeJSONResponse(conn, http.StatusBadRequest, map[string]string{"error": "failed to read request body"})
		return
	}
	defer request.Body.Close()

	var reqBody struct {
		DefaultRPM                  *int64           `json:"default_rpm,omitempty"`
		FreeUserDelaySec            *int64           `json:"free_user_delay_sec,omitempty"`
		FreeUserDelayMaxSec         *int64           `json:"free_user_delay_max_sec,omitempty"`
		ModelRPMOverrides           map[string]int64 `json:"model_rpm_overrides,omitempty"`
		// ModelDelayOverrides 接受 map[string]any，按值类型动态处理：
		//   - 数值       -> 视作 {min: n, max: 0}（老语义 N~2N 兜底）
		//   - 对象 {min,max} -> 直接采用
		ModelDelayOverrides         map[string]json.RawMessage            `json:"model_delay_overrides,omitempty"`
		FreeUserTokenLimitM         *int64                                `json:"free_user_token_limit_m,omitempty"`
		FreeUserTokenModelOverrides map[string]FreeUserTokenModelOverride `json:"free_user_token_model_overrides,omitempty"`
		FreeUserOutputTPS           *int64                                `json:"free_user_output_tps,omitempty"`
		ModelOutputTPSOverrides     map[string]int64                      `json:"model_output_tps_overrides,omitempty"`
		FreeUserTokenSoftLimitM     *int64                                `json:"free_user_token_soft_limit_m,omitempty"`
		FreeUserSoftLimitTPS        *int64                                `json:"free_user_soft_limit_tps,omitempty"`
	}
	if err := json.Unmarshal(bodyBytes, &reqBody); err != nil {
		c.logError("failed to parse request body: %v", err)
		c.writeJSONResponse(conn, http.StatusBadRequest, map[string]string{"error": "invalid request format"})
		return
	}

	config, err := GetRateLimitConfig()
	if err != nil {
		c.logError("failed to get rate limit config: %v", err)
		c.writeJSONResponse(conn, http.StatusInternalServerError, map[string]string{"error": "failed to get rate limit config"})
		return
	}

	if reqBody.DefaultRPM != nil {
		config.DefaultRPM = *reqBody.DefaultRPM
	}
	if reqBody.FreeUserDelaySec != nil {
		config.FreeUserDelaySec = *reqBody.FreeUserDelaySec
	}
	if reqBody.FreeUserDelayMaxSec != nil {
		if *reqBody.FreeUserDelayMaxSec < 0 {
			config.FreeUserDelayMaxSec = 0
		} else {
			config.FreeUserDelayMaxSec = *reqBody.FreeUserDelayMaxSec
		}
	}
	if reqBody.ModelRPMOverrides != nil {
		overridesJSON, _ := json.Marshal(reqBody.ModelRPMOverrides)
		config.ModelRPMOverrides = string(overridesJSON)
	}
	if reqBody.ModelDelayOverrides != nil {
		// 规整化：把每个 entry 强制成 {min,max} 形态写回 DB。
		// 老 int 自动展开为 {min:n, max:0}；新对象保留原样。
		// 关键词: handleSetRateLimitConfig 规整化 ModelDelayOverrides
		cleaned := make(map[string]DelayRange, len(reqBody.ModelDelayOverrides))
		for k, v := range reqBody.ModelDelayOverrides {
			k = strings.TrimSpace(k)
			if k == "" {
				continue
			}
			var legacy int64
			if err := json.Unmarshal(v, &legacy); err == nil {
				if legacy < 0 {
					legacy = 0
				}
				cleaned[k] = DelayRange{Min: legacy, Max: 0}
				continue
			}
			var rng struct {
				Min int64 `json:"min"`
				Max int64 `json:"max"`
			}
			if err := json.Unmarshal(v, &rng); err == nil {
				if rng.Min < 0 {
					rng.Min = 0
				}
				if rng.Max < 0 {
					rng.Max = 0
				}
				cleaned[k] = DelayRange{Min: rng.Min, Max: rng.Max}
			}
		}
		delayJSON, _ := json.Marshal(cleaned)
		config.ModelDelayOverrides = string(delayJSON)
	}
	if reqBody.FreeUserTokenLimitM != nil {
		// <=0 视作未配置；GetRateLimitConfig 会按 1200 兜底
		if *reqBody.FreeUserTokenLimitM < 0 {
			config.FreeUserTokenLimitM = 0
		} else {
			config.FreeUserTokenLimitM = *reqBody.FreeUserTokenLimitM
		}
	}
	if reqBody.FreeUserOutputTPS != nil {
		if *reqBody.FreeUserOutputTPS < 0 {
			config.FreeUserOutputTPS = 0
		} else {
			config.FreeUserOutputTPS = *reqBody.FreeUserOutputTPS
		}
	}
	if reqBody.ModelOutputTPSOverrides != nil {
		clean := make(map[string]int64, len(reqBody.ModelOutputTPSOverrides))
		for k, v := range reqBody.ModelOutputTPSOverrides {
			k = strings.TrimSpace(k)
			if k == "" {
				continue
			}
			if v < 0 {
				v = 0
			}
			clean[k] = v
		}
		tpsJSON, _ := json.Marshal(clean)
		config.ModelOutputTPSOverrides = string(tpsJSON)
	}
	if reqBody.FreeUserTokenSoftLimitM != nil {
		if *reqBody.FreeUserTokenSoftLimitM < 0 {
			config.FreeUserTokenSoftLimitM = 0
		} else {
			config.FreeUserTokenSoftLimitM = *reqBody.FreeUserTokenSoftLimitM
		}
	}
	if reqBody.FreeUserSoftLimitTPS != nil {
		if *reqBody.FreeUserSoftLimitTPS < 0 {
			config.FreeUserSoftLimitTPS = 0
		} else {
			config.FreeUserSoftLimitTPS = *reqBody.FreeUserSoftLimitTPS
		}
	}
	if reqBody.FreeUserTokenModelOverrides != nil {
		// 过滤掉空模型名，避免脏数据；同时把 limit_m<0 钳到 0
		clean := make(map[string]FreeUserTokenModelOverride, len(reqBody.FreeUserTokenModelOverrides))
		for k, v := range reqBody.FreeUserTokenModelOverrides {
			k = strings.TrimSpace(k)
			if k == "" {
				continue
			}
			if v.LimitM < 0 {
				v.LimitM = 0
			}
			clean[k] = v
		}
		ovJSON, _ := json.Marshal(clean)
		config.FreeUserTokenModelOverrides = string(ovJSON)
	}

	if err := SaveRateLimitConfig(config); err != nil {
		c.logError("failed to save rate limit config: %v", err)
		c.writeJSONResponse(conn, http.StatusInternalServerError, map[string]string{"error": "failed to save rate limit config"})
		return
	}

	// Apply to in-memory rate limiter
	c.applyRateLimitConfig(config)

	c.logInfo("successfully updated rate limit config: default_rpm=%d, free_user_delay=%d~%ds, free_user_token_limit_m=%d, output_tps=%d, soft_limit_m=%d, soft_tps=%d",
		config.DefaultRPM, config.FreeUserDelaySec, config.FreeUserDelayMaxSec,
		config.FreeUserTokenLimitM, config.FreeUserOutputTPS,
		config.FreeUserTokenSoftLimitM, config.FreeUserSoftLimitTPS)
	c.writeJSONResponse(conn, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "rate limit config updated successfully",
	})
}

// handleGetRateLimitStatus returns real-time rate-limit status (queue count, etc).
func (c *ServerConfig) handleGetRateLimitStatus(conn net.Conn, request *http.Request) {
	c.logInfo("processing get rate limit status request")

	if !c.checkAuth(request) {
		c.writeJSONResponse(conn, http.StatusUnauthorized, map[string]string{"error": "Unauthorized"})
		return
	}

	var queueCount int64
	var defaultRPM int64
	if c.chatRateLimiter != nil {
		queueCount = c.chatRateLimiter.GetQueueCount()
		defaultRPM = c.chatRateLimiter.defaultRPM.Load()
	}

	// 免费用户日 Token 用量快照（全局 + per-model 桶 + 重置日期）
	// 关键词: handleGetRateLimitStatus free_user_token_usage 快照
	freeTokenUsage := map[string]interface{}{}
	if global, perModel, date, err := QueryFreeUserTokenUsageSnapshot(); err == nil {
		freeTokenUsage["reset_date"] = date
		freeTokenUsage["global"] = map[string]interface{}{
			"tokens_used": global.TokensUsed,
			"used_m":      global.UsedM,
			"limit_m":     global.LimitM,
		}
		perModelOut := make([]map[string]interface{}, 0, len(perModel))
		for _, m := range perModel {
			perModelOut = append(perModelOut, map[string]interface{}{
				"model":       m.Model,
				"tokens_used": m.TokensUsed,
				"used_m":      m.UsedM,
				"limit_m":     m.LimitM,
				"exempt":      m.Exempt,
			})
		}
		freeTokenUsage["per_model"] = perModelOut
	} else {
		c.logWarn("QueryFreeUserTokenUsageSnapshot failed: %v", err)
	}

	c.writeJSONResponse(conn, http.StatusOK, map[string]interface{}{
		"success":               true,
		"queue_count":           queueCount,
		"default_rpm":           defaultRPM,
		"free_user_token_usage": freeTokenUsage,
	})
}

// handleGetRateLimitModelStats returns aggregated per-model RPM across all
// API keys for the current 60-second sliding window. Only models with
// RPM >= min_rpm (default 20) are returned. Admin only.
func (c *ServerConfig) handleGetRateLimitModelStats(conn net.Conn, request *http.Request) {
	c.logInfo("processing get rate limit model stats request")

	if !c.checkAuth(request) {
		c.writeJSONResponse(conn, http.StatusUnauthorized, map[string]string{"error": "Unauthorized"})
		return
	}

	minRPM := defaultModelStatsMinRPM
	if raw := strings.TrimSpace(request.URL.Query().Get("min_rpm")); raw != "" {
		if parsed, err := strconv.ParseInt(raw, 10, 64); err == nil && parsed >= 1 {
			minRPM = parsed
		}
	}

	var models []ModelRPMStat
	if c.chatRateLimiter != nil {
		models = c.chatRateLimiter.GetModelRPMStats(minRPM)
	}
	if models == nil {
		models = []ModelRPMStat{}
	}

	c.writeJSONResponse(conn, http.StatusOK, map[string]interface{}{
		"success":        true,
		"models":         models,
		"min_rpm":        minRPM,
		"window_seconds": int64(rpmWindowDuration / 1e9),
	})
}

// applyRateLimitConfig applies the DB config to the in-memory rate limiter.
func (c *ServerConfig) applyRateLimitConfig(cfg *schema.AiBalanceRateLimitConfig) {
	if cfg == nil {
		return
	}
	if c.chatRateLimiter != nil {
		c.chatRateLimiter.SetDefaultRPM(cfg.DefaultRPM)
		c.chatRateLimiter.ClearModelRPM()
		if cfg.ModelRPMOverrides != "" {
			var overrides map[string]int64
			if err := json.Unmarshal([]byte(cfg.ModelRPMOverrides), &overrides); err == nil {
				for model, rpm := range overrides {
					model = strings.TrimSpace(model)
					if model != "" && rpm > 0 {
						c.chatRateLimiter.SetModelRPM(model, rpm)
					}
				}
			}
		}
		c.chatRateLimiter.ClearModelDelay()
		if cfg.ModelDelayOverrides != "" {
			// 同时兼容两种 JSON 形态：
			//   1) 老格式 map[string]int64       -> {"slow-free": 30}
			//   2) 新格式 map[string]DelayRange  -> {"slow-free": {"min": 0, "max": 5}}
			// 解析顺序：先尝试 raw -> map[string]json.RawMessage，逐项判别。
			// 关键词: ModelDelayOverrides 解析, 老 int 兼容, 新 DelayRange
			rawMap := make(map[string]json.RawMessage)
			if err := json.Unmarshal([]byte(cfg.ModelDelayOverrides), &rawMap); err == nil {
				for model, raw := range rawMap {
					model = strings.TrimSpace(model)
					if model == "" {
						continue
					}
					var legacy int64
					if err := json.Unmarshal(raw, &legacy); err == nil {
						if legacy >= 0 {
							c.chatRateLimiter.SetModelDelay(model, legacy, 0)
						}
						continue
					}
					var rng struct {
						Min int64 `json:"min"`
						Max int64 `json:"max"`
					}
					if err := json.Unmarshal(raw, &rng); err == nil {
						if rng.Min < 0 {
							rng.Min = 0
						}
						if rng.Max < 0 {
							rng.Max = 0
						}
						c.chatRateLimiter.SetModelDelay(model, rng.Min, rng.Max)
					}
				}
			}
		}

		// 模型级输出 TPS 覆盖
		// 关键词: ModelOutputTPSOverrides applyRateLimitConfig
		c.chatRateLimiter.ClearModelOutputTPS()
		if cfg.ModelOutputTPSOverrides != "" {
			var tpsOverrides map[string]int64
			if err := json.Unmarshal([]byte(cfg.ModelOutputTPSOverrides), &tpsOverrides); err == nil {
				for model, tps := range tpsOverrides {
					model = strings.TrimSpace(model)
					if model != "" && tps > 0 {
						c.chatRateLimiter.SetModelOutputTPS(model, tps)
					}
				}
			}
		}
	}
	c.freeUserDelayMinSec = cfg.FreeUserDelaySec
	c.freeUserDelayMaxSec = cfg.FreeUserDelayMaxSec
	c.freeUserOutputTPS = cfg.FreeUserOutputTPS
	c.freeUserTokenSoftLimitM = cfg.FreeUserTokenSoftLimitM
	c.freeUserSoftLimitTPS = cfg.FreeUserSoftLimitTPS
}
