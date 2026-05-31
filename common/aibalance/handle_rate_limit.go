package aibalance

import (
	"encoding/json"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// defaultModelStatsMinRPM is the default minimum RPM threshold used by the
// "hot models" stats endpoint when the caller does not override it. The
// previous value of 20 was too high to surface most tuning candidates, so
// the threshold was lowered to 3 to expose anything beyond trivial traffic.
const defaultModelStatsMinRPM int64 = 3

// freeIPUsageDisplayThresholdM 是「免费 IP 用量」面板的展示阈值（单位 M token）。
// 仅当某 IP 当日加权 Token 严格超过该值时才在面板展示，避免低用量 IP 把列表撑得过长。
// 关键词: freeIPUsageDisplayThresholdM, 免费 IP 用量展示阈值, >10M 才展示
const freeIPUsageDisplayThresholdM float64 = 10

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

	// 自定义 429 文案：按 limit_kind 覆盖 map（关闭时仍返回当前配置值供前端回填）
	// 关键词: handleGetRateLimitConfig custom_429_kind_overrides
	var custom429Overrides map[string]string
	if config.Custom429KindOverrides != "" {
		json.Unmarshal([]byte(config.Custom429KindOverrides), &custom429Overrides)
	}
	if custom429Overrides == nil {
		custom429Overrides = make(map[string]string)
	}

	// 轻量降级规则（tier + 源模型 -> 目标模型）
	// 关键词: handleGetRateLimitConfig model_downgrade_rules
	downgradeRules := parseModelDowngradeRules(config.ModelDowngradeRules)

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
			"paid_user_token_limit_m":         config.PaidUserTokenLimitM,
			"free_user_output_tps":            config.FreeUserOutputTPS,
			"model_output_tps_overrides":      modelOutputTPSOverrides,
			"free_user_token_soft_limit_m":    config.FreeUserTokenSoftLimitM,
			"free_user_soft_limit_tps":        config.FreeUserSoftLimitTPS,
			// memfit-* 客户端版本控流配置
			// 关键词: handleGetRateLimitConfig memfit_version_gate_enabled, memfit_version_min_build_time
			"memfit_version_gate_enabled":   config.MemfitVersionGateEnabled,
			"memfit_version_min_build_time": config.MemfitVersionMinBuildTime,
			// 自定义 429/错误文案配置
			// 关键词: handleGetRateLimitConfig custom_429_enabled, custom_429_notice, custom_429_kind_overrides
			"custom_429_enabled":        config.Custom429Enabled,
			"custom_429_notice":         config.Custom429Notice,
			"custom_429_kind_overrides": custom429Overrides,
			// 各 limit_kind 的默认文案/中文名/触发原因（唯一真相源），供前端编辑时展示默认文案。
			// 关键词: handleGetRateLimitConfig custom_429_kind_defaults, Custom429Kinds
			"custom_429_kind_defaults": Custom429Kinds(),
			// 轻量降级规则
			// 关键词: handleGetRateLimitConfig model_downgrade_rules
			"model_downgrade_rules": downgradeRules,
			// 单 IP 免费模型每日用量限额
			// 关键词: handleGetRateLimitConfig free_user_ip_limit
			"free_user_ip_limit_enable":        config.FreeUserIPLimitEnable,
			"free_user_ip_daily_request_limit": config.FreeUserIPDailyRequestLimit,
			"free_user_ip_daily_token_limit_m": config.FreeUserIPDailyTokenLimitM,
			// 一键限流 IP 默认参数（RPM / 输出 TPS）
			// 关键词: handleGetRateLimitConfig throttled_ip_default_rpm/tps
			"throttled_ip_default_rpm": config.ThrottledIPDefaultRPM,
			"throttled_ip_default_tps": config.ThrottledIPDefaultTPS,
		},
	})
}

// parseModelDelayOverrides 解析 ModelDelayOverrides JSON 字符串，支持两种历史格式：
//
//  1. 老格式 map[string]int64  -> {"slow-free": 30}
//  2. 新格式 map[string]DelayRange -> {"slow-free": {"min": 0, "max": 5}}
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
		DefaultRPM          *int64           `json:"default_rpm,omitempty"`
		FreeUserDelaySec    *int64           `json:"free_user_delay_sec,omitempty"`
		FreeUserDelayMaxSec *int64           `json:"free_user_delay_max_sec,omitempty"`
		ModelRPMOverrides   map[string]int64 `json:"model_rpm_overrides,omitempty"`
		// ModelDelayOverrides 接受 map[string]any，按值类型动态处理：
		//   - 数值       -> 视作 {min: n, max: 0}（老语义 N~2N 兜底）
		//   - 对象 {min,max} -> 直接采用
		ModelDelayOverrides         map[string]json.RawMessage            `json:"model_delay_overrides,omitempty"`
		FreeUserTokenLimitM         *int64                                `json:"free_user_token_limit_m,omitempty"`
		FreeUserTokenModelOverrides map[string]FreeUserTokenModelOverride `json:"free_user_token_model_overrides,omitempty"`
		PaidUserTokenLimitM         *int64                                `json:"paid_user_token_limit_m,omitempty"`
		FreeUserOutputTPS           *int64                                `json:"free_user_output_tps,omitempty"`
		ModelOutputTPSOverrides     map[string]int64                      `json:"model_output_tps_overrides,omitempty"`
		FreeUserTokenSoftLimitM     *int64                                `json:"free_user_token_soft_limit_m,omitempty"`
		FreeUserSoftLimitTPS        *int64                                `json:"free_user_soft_limit_tps,omitempty"`
		// memfit-* 客户端版本控流配置
		// 关键词: handleSetRateLimitConfig memfit_version_gate_enabled, memfit_version_min_build_time
		MemfitVersionGateEnabled  *bool   `json:"memfit_version_gate_enabled,omitempty"`
		MemfitVersionMinBuildTime *string `json:"memfit_version_min_build_time,omitempty"`
		// 自定义 429/错误文案配置
		// 关键词: handleSetRateLimitConfig custom_429_enabled, custom_429_notice, custom_429_kind_overrides
		Custom429Enabled       *bool             `json:"custom_429_enabled,omitempty"`
		Custom429Notice        *string           `json:"custom_429_notice,omitempty"`
		Custom429KindOverrides map[string]string `json:"custom_429_kind_overrides,omitempty"`
		// 轻量降级规则（传入空数组 [] 表示显式关闭降级）
		// 关键词: handleSetRateLimitConfig model_downgrade_rules
		ModelDowngradeRules []ModelDowngradeRule `json:"model_downgrade_rules,omitempty"`
		// 单 IP 免费模型每日用量限额
		// 关键词: handleSetRateLimitConfig free_user_ip_limit
		FreeUserIPLimitEnable       *bool  `json:"free_user_ip_limit_enable,omitempty"`
		FreeUserIPDailyRequestLimit *int64 `json:"free_user_ip_daily_request_limit,omitempty"`
		FreeUserIPDailyTokenLimitM  *int64 `json:"free_user_ip_daily_token_limit_m,omitempty"`
		// 一键限流 IP 默认参数
		// 关键词: handleSetRateLimitConfig throttled_ip_default_rpm/tps
		ThrottledIPDefaultRPM *int64 `json:"throttled_ip_default_rpm,omitempty"`
		ThrottledIPDefaultTPS *int64 `json:"throttled_ip_default_tps,omitempty"`
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
	if reqBody.PaidUserTokenLimitM != nil {
		// 付费用户全局日 Token 总额度：<0 归零（0=不限制）
		if *reqBody.PaidUserTokenLimitM < 0 {
			config.PaidUserTokenLimitM = 0
		} else {
			config.PaidUserTokenLimitM = *reqBody.PaidUserTokenLimitM
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
	// memfit-* 客户端版本控流配置写入
	// 关键词: handleSetRateLimitConfig 写入 MemfitVersionGate 字段
	if reqBody.MemfitVersionGateEnabled != nil {
		config.MemfitVersionGateEnabled = *reqBody.MemfitVersionGateEnabled
	}
	if reqBody.MemfitVersionMinBuildTime != nil {
		config.MemfitVersionMinBuildTime = strings.TrimSpace(*reqBody.MemfitVersionMinBuildTime)
	}

	// 自定义 429/错误文案配置写入
	// 关键词: handleSetRateLimitConfig 写入 Custom429 字段
	if reqBody.Custom429Enabled != nil {
		config.Custom429Enabled = *reqBody.Custom429Enabled
	}
	if reqBody.Custom429Notice != nil {
		config.Custom429Notice = strings.TrimSpace(*reqBody.Custom429Notice)
	}
	if reqBody.Custom429KindOverrides != nil {
		// 过滤空 kind；空字符串 value 表示该 kind 不覆盖（保持默认文案）
		clean := make(map[string]string, len(reqBody.Custom429KindOverrides))
		for k, v := range reqBody.Custom429KindOverrides {
			k = strings.TrimSpace(k)
			if k == "" {
				continue
			}
			clean[k] = v
		}
		ovJSON, _ := json.Marshal(clean)
		config.Custom429KindOverrides = string(ovJSON)
	}

	// 单 IP 免费模型每日用量限额写入（负数钳到 0，0 = 不限）
	// 关键词: handleSetRateLimitConfig 写入 FreeUserIPLimit
	if reqBody.FreeUserIPLimitEnable != nil {
		config.FreeUserIPLimitEnable = *reqBody.FreeUserIPLimitEnable
	}
	if reqBody.FreeUserIPDailyRequestLimit != nil {
		if *reqBody.FreeUserIPDailyRequestLimit < 0 {
			config.FreeUserIPDailyRequestLimit = 0
		} else {
			config.FreeUserIPDailyRequestLimit = *reqBody.FreeUserIPDailyRequestLimit
		}
	}
	if reqBody.FreeUserIPDailyTokenLimitM != nil {
		if *reqBody.FreeUserIPDailyTokenLimitM < 0 {
			config.FreeUserIPDailyTokenLimitM = 0
		} else {
			config.FreeUserIPDailyTokenLimitM = *reqBody.FreeUserIPDailyTokenLimitM
		}
	}

	// 一键限流 IP 默认参数写入（<=0 视作未配置，GetRateLimitConfig 会按 3/15 兜底）
	// 关键词: handleSetRateLimitConfig 写入 ThrottledIPDefault
	if reqBody.ThrottledIPDefaultRPM != nil {
		if *reqBody.ThrottledIPDefaultRPM < 0 {
			config.ThrottledIPDefaultRPM = 0
		} else {
			config.ThrottledIPDefaultRPM = *reqBody.ThrottledIPDefaultRPM
		}
	}
	if reqBody.ThrottledIPDefaultTPS != nil {
		if *reqBody.ThrottledIPDefaultTPS < 0 {
			config.ThrottledIPDefaultTPS = 0
		} else {
			config.ThrottledIPDefaultTPS = *reqBody.ThrottledIPDefaultTPS
		}
	}

	// 轻量降级规则写入：from/to 为空的规则被丢弃；空数组 [] 表示显式关闭（不回退内置默认）
	// 关键词: handleSetRateLimitConfig 写入 ModelDowngradeRules
	if reqBody.ModelDowngradeRules != nil {
		clean := make([]ModelDowngradeRule, 0, len(reqBody.ModelDowngradeRules))
		for _, r := range reqBody.ModelDowngradeRules {
			r.Tier = strings.TrimSpace(r.Tier)
			r.From = strings.TrimSpace(r.From)
			r.To = strings.TrimSpace(r.To)
			if r.From == "" || r.To == "" {
				continue
			}
			clean = append(clean, r)
		}
		rulesJSON, _ := json.Marshal(clean)
		config.ModelDowngradeRules = string(rulesJSON)
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

	// 付费用户全局日 Token 总额度快照（第二道硬门），供实时面板展示
	// 关键词: handleGetRateLimitStatus paid_user_token_usage 快照
	paidTokenUsage := map[string]interface{}{}
	if snap, err := QueryPaidUserTokenUsageSnapshot(); err == nil {
		paidTokenUsage["reset_date"] = snap.Date
		paidTokenUsage["tokens_used"] = snap.TokensUsed
		paidTokenUsage["used_m"] = snap.UsedM
		paidTokenUsage["limit_m"] = snap.LimitM
	} else {
		c.logWarn("QueryPaidUserTokenUsageSnapshot failed: %v", err)
	}

	// 一键限流 IP：默认参数 + 当前已限流 IP 列表（用于面板展示/解除/标记）。
	// 关键词: handleGetRateLimitStatus throttled_ips, throttle 默认值
	throttleDefaultRPM := int64(3)
	throttleDefaultTPS := int64(15)
	if cfg, err := GetRateLimitConfig(); err == nil {
		throttleDefaultRPM = cfg.ThrottledIPDefaultRPM
		throttleDefaultTPS = cfg.ThrottledIPDefaultTPS
	}
	throttledSet := map[string]bool{}
	throttledOut := make([]map[string]interface{}, 0)
	if rows, err := ListThrottledIPs(); err == nil {
		for _, r := range rows {
			throttledSet[r.IP] = true
			throttledOut = append(throttledOut, map[string]interface{}{
				"ip":     r.IP,
				"rpm":    r.RPM,
				"tps":    r.TPS,
				"reason": r.Reason,
			})
		}
	} else {
		c.logWarn("ListThrottledIPs failed: %v", err)
	}

	// 单 IP 免费模型用量快照：今日有多少个 IP 在用 + Top 用量榜。
	// 仅展示加权 Token 超过阈值（freeIPUsageDisplayThresholdM=10M）的 IP，减少前端噪声；
	// 同时标注每个 IP 是否已被一键限流。
	// 关键词: handleGetRateLimitStatus free_ip_usage 快照, >10M 过滤, throttled 标记
	freeIPUsage := map[string]interface{}{
		"display_threshold_m":  freeIPUsageDisplayThresholdM,
		"throttle_default_rpm": throttleDefaultRPM,
		"throttle_default_tps": throttleDefaultTPS,
		"throttled_ips":        throttledOut,
	}
	if distinctCount, top, date, err := QueryFreeUserIPUsageSnapshot(50); err == nil {
		freeIPUsage["reset_date"] = date
		freeIPUsage["distinct_ip_count"] = distinctCount
		// 先筛出超阈值（>10M）的展示 IP，再批量取这些 IP 的「按模型 TOP3 用量」附给每行，
		// 避免 N 次小查询。top_models 仅展示用，不影响任何限额逻辑。
		// 关键词: handleGetRateLimitStatus top_models, per-IP TOP3 模型展示
		displayedIPs := make([]string, 0, len(top))
		for _, r := range top {
			if r.UsedM <= freeIPUsageDisplayThresholdM {
				continue
			}
			displayedIPs = append(displayedIPs, r.IP)
		}
		topModelsByIP, tmErr := QueryFreeIPTopModelsBatch(displayedIPs, 3)
		if tmErr != nil {
			c.logWarn("QueryFreeIPTopModelsBatch failed: %v", tmErr)
			topModelsByIP = map[string][]FreeIPModelUsageRow{}
		}
		topOut := make([]map[string]interface{}, 0, len(displayedIPs))
		for _, r := range top {
			if r.UsedM <= freeIPUsageDisplayThresholdM {
				continue
			}
			models := make([]map[string]interface{}, 0, 3)
			for _, m := range topModelsByIP[r.IP] {
				models = append(models, map[string]interface{}{
					"model":           m.Model,
					"request_count":   m.RequestCount,
					"tokens_used":     m.TokensUsed,
					"used_m":          m.UsedM,
					"weighted_tokens": m.WeightedTokens,
					"weighted_m":      m.WeightedM,
				})
			}
			topOut = append(topOut, map[string]interface{}{
				"ip":            r.IP,
				"request_count": r.RequestCount,
				"tokens_used":   r.TokensUsed,
				"used_m":        r.UsedM,
				"throttled":     throttledSet[r.IP],
				"top_models":    models,
			})
		}
		freeIPUsage["top"] = topOut
	} else {
		c.logWarn("QueryFreeUserIPUsageSnapshot failed: %v", err)
	}

	c.writeJSONResponse(conn, http.StatusOK, map[string]interface{}{
		"success":               true,
		"queue_count":           queueCount,
		"default_rpm":           defaultRPM,
		"free_user_token_usage": freeTokenUsage,
		"paid_user_token_usage": paidTokenUsage,
		"free_ip_usage":         freeIPUsage,
	})
}

// handleThrottleIP 一键限流某个 IP：把该 IP 的 RPM / 输出 TPS 压到指定值
// （默认取配置的 ThrottledIPDefaultRPM / ThrottledIPDefaultTPS）。Admin only。
//
// 请求体：{"ip":"1.2.3.4", "rpm":3, "tps":15, "reason":"..."}（rpm/tps/reason 可选）。
// rpm/tps 省略或 <=0 时回退配置默认值。写入持久化 + 刷新进程内缓存。
// 关键词: handleThrottleIP, 一键限流 IP, POST /portal/api/throttle-ip
func (c *ServerConfig) handleThrottleIP(conn net.Conn, request *http.Request) {
	c.logInfo("processing throttle ip request")

	if !c.checkAuth(request) {
		c.writeJSONResponse(conn, http.StatusUnauthorized, map[string]string{"error": "Unauthorized"})
		return
	}

	bodyBytes, err := io.ReadAll(request.Body)
	if err != nil {
		c.logError("failed to read throttle ip request body: %v", err)
		c.writeJSONResponse(conn, http.StatusBadRequest, map[string]string{"error": "failed to read request body"})
		return
	}
	var reqBody struct {
		IP     string `json:"ip"`
		RPM    *int64 `json:"rpm,omitempty"`
		TPS    *int64 `json:"tps,omitempty"`
		Reason string `json:"reason,omitempty"`
	}
	if err := json.Unmarshal(bodyBytes, &reqBody); err != nil {
		c.logError("failed to parse throttle ip request body: %v", err)
		c.writeJSONResponse(conn, http.StatusBadRequest, map[string]string{"error": "invalid request format"})
		return
	}

	ip := strings.TrimSpace(reqBody.IP)
	if ip == "" {
		c.writeJSONResponse(conn, http.StatusBadRequest, map[string]string{"error": "ip is required"})
		return
	}

	// 默认值取自配置；请求显式给正值则覆盖。
	rpm := int64(3)
	tps := int64(15)
	if cfg, cErr := GetRateLimitConfig(); cErr == nil {
		rpm = cfg.ThrottledIPDefaultRPM
		tps = cfg.ThrottledIPDefaultTPS
	}
	if reqBody.RPM != nil && *reqBody.RPM > 0 {
		rpm = *reqBody.RPM
	}
	if reqBody.TPS != nil && *reqBody.TPS > 0 {
		tps = *reqBody.TPS
	}

	if err := UpsertThrottledIP(ip, rpm, tps, strings.TrimSpace(reqBody.Reason)); err != nil {
		c.logError("UpsertThrottledIP failed (ip=%s): %v", ip, err)
		c.writeJSONResponse(conn, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	c.logInfo("throttled ip applied: ip=%s rpm=%d tps=%d", ip, rpm, tps)
	c.writeJSONResponse(conn, http.StatusOK, map[string]interface{}{
		"success": true,
		"ip":      ip,
		"rpm":     rpm,
		"tps":     tps,
	})
}

// handleUnthrottleIP 解除某个 IP 的一键限流（删除持久化行 + 缓存条目）。Admin only。
// 请求体：{"ip":"1.2.3.4"}。
// 关键词: handleUnthrottleIP, 解除限流, POST /portal/api/unthrottle-ip
func (c *ServerConfig) handleUnthrottleIP(conn net.Conn, request *http.Request) {
	c.logInfo("processing unthrottle ip request")

	if !c.checkAuth(request) {
		c.writeJSONResponse(conn, http.StatusUnauthorized, map[string]string{"error": "Unauthorized"})
		return
	}

	bodyBytes, err := io.ReadAll(request.Body)
	if err != nil {
		c.logError("failed to read unthrottle ip request body: %v", err)
		c.writeJSONResponse(conn, http.StatusBadRequest, map[string]string{"error": "failed to read request body"})
		return
	}
	var reqBody struct {
		IP string `json:"ip"`
	}
	if err := json.Unmarshal(bodyBytes, &reqBody); err != nil {
		c.logError("failed to parse unthrottle ip request body: %v", err)
		c.writeJSONResponse(conn, http.StatusBadRequest, map[string]string{"error": "invalid request format"})
		return
	}

	ip := strings.TrimSpace(reqBody.IP)
	if ip == "" {
		c.writeJSONResponse(conn, http.StatusBadRequest, map[string]string{"error": "ip is required"})
		return
	}

	if err := DeleteThrottledIP(ip); err != nil {
		c.logError("DeleteThrottledIP failed (ip=%s): %v", ip, err)
		c.writeJSONResponse(conn, http.StatusInternalServerError, map[string]string{"error": "failed to unthrottle ip"})
		return
	}

	c.logInfo("throttled ip removed: ip=%s", ip)
	c.writeJSONResponse(conn, http.StatusOK, map[string]interface{}{
		"success": true,
		"ip":      ip,
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

// handleGetClientVersionStats returns the most recent client versions (Top N) seen
// by memfit-* requests. Admin-only.
//
// Query params:
//   - limit: optional int (1..200), default 20
//
// Response: {"success": true, "total": <int>, "items": [...]}
// Each item: {version, build_time, first_seen_unix, last_seen_unix, request_count,
//
//	first_seen_text, last_seen_text}
//
// 关键词: handleGetClientVersionStats portal /portal/api/client-version-stats 返回结构
func (c *ServerConfig) handleGetClientVersionStats(conn net.Conn, request *http.Request) {
	c.logInfo("processing get client version stats request")

	if !c.checkAuth(request) {
		c.writeJSONResponse(conn, http.StatusUnauthorized, map[string]string{"error": "Unauthorized"})
		return
	}

	limit := 20
	if raw := strings.TrimSpace(request.URL.Query().Get("limit")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed >= 1 {
			limit = parsed
		}
	}
	if limit > 200 {
		limit = 200
	}

	rows, err := QueryTopClientVersions(limit)
	if err != nil {
		c.logError("failed to query top client versions: %v", err)
		c.writeJSONResponse(conn, http.StatusInternalServerError, map[string]string{"error": "failed to query client version stats"})
		return
	}
	if rows == nil {
		rows = []AiBalanceClientVersionStat{}
	}

	items := make([]map[string]interface{}, 0, len(rows))
	for _, r := range rows {
		items = append(items, map[string]interface{}{
			"version":         r.Version,
			"build_time":      r.BuildTime,
			"first_seen_unix": r.FirstSeenUnix,
			"last_seen_unix":  r.LastSeenUnix,
			"first_seen_text": time.Unix(r.FirstSeenUnix, 0).Format("2006-01-02 15:04:05"),
			"last_seen_text":  time.Unix(r.LastSeenUnix, 0).Format("2006-01-02 15:04:05"),
			"request_count":   r.RequestCount,
		})
	}

	c.writeJSONResponse(conn, http.StatusOK, map[string]interface{}{
		"success": true,
		"total":   len(items),
		"limit":   limit,
		"items":   items,
	})
}

// handleClearClientVersionStats handles POST /portal/api/client-version-stats/clear.
// 清空 ai_balance_client_versions 表，便于运维手动重置垃圾/陈旧数据。
// 需要 portal admin 鉴权。
//
// Response: {"success": true, "removed": <int64>}
//
// 关键词: handleClearClientVersionStats, portal 清空客户端版本记录路由
func (c *ServerConfig) handleClearClientVersionStats(conn net.Conn, request *http.Request) {
	c.logInfo("processing clear client version stats request")

	if !c.checkAuth(request) {
		c.writeJSONResponse(conn, http.StatusUnauthorized, map[string]string{"error": "Unauthorized"})
		return
	}

	removed, err := ClearAllClientVersions()
	if err != nil {
		c.logError("clear client version stats failed: %v", err)
		c.writeJSONResponse(conn, http.StatusInternalServerError, map[string]string{
			"error": "failed to clear client version stats",
		})
		return
	}

	c.writeJSONResponse(conn, http.StatusOK, map[string]interface{}{
		"success": true,
		"removed": removed,
	})
}

// applyRateLimitConfig applies the DB config to the in-memory rate limiter.
func (c *ServerConfig) applyRateLimitConfig(cfg *AiBalanceRateLimitConfig) {
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

	// 刷新自定义 429 文案 + 模型降级规则缓存（供 resolveLimit429 / resolveModelDowngrade 使用）
	// 关键词: applyRateLimitConfig 刷新 custom429 缓存, modelDowngradeRules 缓存
	kindOverrides := make(map[string]string)
	if cfg.Custom429KindOverrides != "" {
		var parsed map[string]string
		if err := json.Unmarshal([]byte(cfg.Custom429KindOverrides), &parsed); err == nil {
			for k, v := range parsed {
				k = strings.TrimSpace(k)
				if k == "" {
					continue
				}
				kindOverrides[k] = v
			}
		}
	}
	downgradeRules := parseModelDowngradeRules(cfg.ModelDowngradeRules)
	c.limitPolicyMu.Lock()
	c.custom429Enabled = cfg.Custom429Enabled
	c.custom429Notice = cfg.Custom429Notice
	c.custom429KindOverrides = kindOverrides
	c.modelDowngradeRules = downgradeRules
	c.limitPolicyMu.Unlock()
}

// resolveLimit429 统一解析某个限流类型(kind)对外返回的 429/错误文案：
//   - 自定义文案关闭(默认)时：返回原始默认文案、空 notice，行为与历史完全一致；
//   - 开启时：若该 kind 配置了非空 override 则替换 message，并附带全局 notice 文案。
//
// kind 取值与各写出点约定（详见 custom_429.go Custom429Kinds）：
// rpm / token / daily_token / free_ip / paid_daily_token / memfit_version。
// 关键词: resolveLimit429, 自定义 429 文案, notice 注入, limit_kind 覆盖
func (c *ServerConfig) resolveLimit429(kind, defaultMessage string) (message, notice string) {
	message = defaultMessage
	c.limitPolicyMu.RLock()
	enabled := c.custom429Enabled
	globalNotice := c.custom429Notice
	override, ok := c.custom429KindOverrides[kind]
	c.limitPolicyMu.RUnlock()
	if !enabled {
		return message, ""
	}
	if ok {
		if v := strings.TrimSpace(override); v != "" {
			message = v
		}
	}
	return message, globalNotice
}
