package aibalance

import (
	"github.com/yaklang/yaklang/common/log"
)

// EvaluateFreeUserSoftLimit 在 free model 请求即将创建 writer 时调用：
// 查询当日全局共享池已用 token 数，若 >= SoftLimitM*1M 且 SoftLimitTPS>0，
// 则返回 (true, SoftLimitTPS)；否则返回 (false, 0)。
//
// 注意：
//   - 仅在「全局共享池」模式下生效。如果模型在 FreeUserTokenModelOverrides
//     里配置了独立桶（LimitM>0），即视为该模型走自己的桶，不受全局软限额影响。
//   - exempt=true 的模型完全豁免软限额。
//   - 任何 DB 异常都返回 (false, 0)，不阻塞主链路。
//
// 关键词: EvaluateFreeUserSoftLimit, 全局共享池软限额, 免费模型限速
func EvaluateFreeUserSoftLimit(modelName string) (triggered bool, softLimitTPS int64) {
	cfg, err := GetRateLimitConfig()
	if err != nil {
		log.Warnf("EvaluateFreeUserSoftLimit: GetRateLimitConfig failed: %v", err)
		return false, 0
	}
	if cfg.FreeUserTokenSoftLimitM <= 0 || cfg.FreeUserSoftLimitTPS <= 0 {
		return false, 0
	}

	// 模型独立桶或豁免：不受全局软限额影响
	// 关键词: EvaluateFreeUserSoftLimit 模型独立桶豁免
	overrides := parseFreeUserTokenModelOverrides(cfg.FreeUserTokenModelOverrides)
	if ov, ok := overrides[modelName]; ok {
		if ov.Exempt {
			return false, 0
		}
		if ov.LimitM > 0 {
			return false, 0
		}
	}

	used, err := GetFreeUserDailyTokenUsage(freeTokenNowDate(), freeUserGlobalBucketModel)
	if err != nil {
		log.Warnf("EvaluateFreeUserSoftLimit: GetFreeUserDailyTokenUsage failed: %v", err)
		return false, 0
	}
	threshold := cfg.FreeUserTokenSoftLimitM * FreeUserTokenMUnit
	if used >= threshold {
		return true, cfg.FreeUserSoftLimitTPS
	}
	return false, 0
}

// pickStricterTPS 在两个候选 TPS（0 视作"无限制"）中挑出更严格的（更小的非零值）。
// 关键词: pickStricterTPS, TPS 取最严
func pickStricterTPS(a, b int64) int64 {
	if a <= 0 {
		return b
	}
	if b <= 0 {
		return a
	}
	if a < b {
		return a
	}
	return b
}

// ResolveEffectiveOutputTPS 综合「模型级 TPS」「全局免费 TPS」「软限额 TPS」
// 三档配置，返回最终生效的 TPS 限速：
//
//   - 非免费模型 -> 返回 0（不限速）
//   - 模型级 TPS 与全局免费 TPS 取最严
//   - 若全局共享池软限额触发，再与软限额 TPS 取最严
//
// 0 表示"无限制"。任何一档为 0 都不参与"取最严"运算。
// 关键词: ResolveEffectiveOutputTPS, TPS 综合判定
func (c *ServerConfig) ResolveEffectiveOutputTPS(modelName string, isFreeModel bool) int64 {
	if !isFreeModel {
		return 0
	}
	var modelTPS int64
	if c != nil && c.chatRateLimiter != nil {
		modelTPS = c.chatRateLimiter.GetEffectiveOutputTPS(modelName, 0)
	}
	var globalTPS int64
	if c != nil {
		globalTPS = c.freeUserOutputTPS
	}
	effective := pickStricterTPS(modelTPS, globalTPS)

	if triggered, softTPS := EvaluateFreeUserSoftLimit(modelName); triggered {
		effective = pickStricterTPS(effective, softTPS)
	}
	return effective
}
