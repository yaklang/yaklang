package aibalance

import (
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/ai/ytoken"
	"github.com/yaklang/yaklang/common/utils"
)

// fallback 估算扣费用：当上游不返 usage 帧时，每个 image_url 按 4096 token
// 预扣。多模态 token 计费的实际数量通常在 700~2000 之间，这里取上限保守扣，
// 避免被 vision 模型刷免费桶。
// 关键词: aibalance usage fallback image token 预扣, vision 估算
const fallbackImageTokenEstimate = 4096

// inFlightCompletionTokenBudget 是 in-flight 预扣阶段对"上游 completion 输出长度"
// 的保守估算。一般 chat completion 控制在 1K~4K，这里取 8K 是为了让"过冲防御"
// 偏严，宁可让边缘请求被拒、也不能让 100 并发同时穿透 daily check 引发 30% 过冲。
// stream 结束后真实 weighted 会替代估算入账，预扣同步释放，所以这个偏严的估算
// 只影响"daily check 闸门紧迫期"的判决，不影响最终计费的准确性。
// 关键词: inFlightCompletionTokenBudget, in-flight 预扣 completion 估算, 偏严过冲防御
const inFlightCompletionTokenBudget = 8192

// computeInFlightTokenEstimate 估算一次 free model 请求的 in-flight 预扣 token，
// 用与 onUsageForward 正路一致的 ComputeWeightedTokens 倍率体系。
//
// 估算公式：
//   estPromptTokens     = ytoken(promptText) + imageCount * fallbackImageTokenEstimate
//   estCompletionTokens = inFlightCompletionTokenBudget  // 8192 保守
//   estUsage            = ChatUsage{PromptTokens, CompletionTokens, TotalTokens}
//   返回 ComputeWeightedTokens(meta, estUsage)
//
// 入参 promptText 应该传 prompt.String() (chat 入口已拼好的文本)。
// 关键词: computeInFlightTokenEstimate ytoken 预扣估算, 与 fallback 同体系
func computeInFlightTokenEstimate(modelName, promptText string, imageCount int) int64 {
	estPromptTokens := int64(ytoken.CalcTokenCount(promptText)) +
		int64(imageCount)*fallbackImageTokenEstimate
	estCompletionTokens := int64(inFlightCompletionTokenBudget)
	estUsage := &aispec.ChatUsage{
		PromptTokens:     int(estPromptTokens),
		CompletionTokens: int(estCompletionTokens),
		TotalTokens:      int(estPromptTokens + estCompletionTokens),
	}
	meta, _ := GetModelMeta(modelName)
	return ComputeWeightedTokens(meta, estUsage)
}

// resolveInFlightBucketKey 把 modelName 映射到 daily token 桶 key，与
// CheckFreeUserDailyTokenLimit 内部的桶选择逻辑保持一致：
//   - exempt=true                → "", exempt=true（调用方应不预扣）
//   - 模型有 override 且 limit_m>0 → modelName, exempt=false
//   - 其它                         → "", exempt=false（全局共享池）
//
// 关键词: resolveInFlightBucketKey daily 桶对齐
func resolveInFlightBucketKey(modelName string) (bucketKey string, exempt bool) {
	overrides := parseFreeUserTokenModelOverridesFromConfig()
	if ov, ok := overrides[modelName]; ok {
		if ov.Exempt {
			return "", true
		}
		if ov.LimitM > 0 {
			return modelName, false
		}
	}
	return "", false
}

// checkFreeUserDailyTokenLimitWithInFlight 在 CheckFreeUserDailyTokenLimit
// 的基础上把进程内的 in-flight 预扣加进 used，硬卡死并发过冲。
//
// 行为：
//   - DB 错误：透传给调用方（不阻塞业务，与原行为一致）
//   - exempt：透传（永远 Allowed=true）
//   - TokensLimit<=0：透传（无限额）
//   - 其它：effective_used = decision.TokensUsed + inFlightTokens.Get(bucket)
//           decision.TokensUsed 被替换为 effective_used，
//           decision.Allowed 重新按 effective_used < TokensLimit 计算
//
// 注意 decision.TokensUsed 字段在 429 响应里会被回写给客户端
// (X-AIBalance-Token-Used 头)，把 in-flight 加上去能让客户端看到"真实压力"。
//
// 关键词: checkFreeUserDailyTokenLimitWithInFlight in-flight 防过冲, daily check 包装
func (c *ServerConfig) checkFreeUserDailyTokenLimitWithInFlight(modelName string) (*FreeUserTokenLimitDecision, error) {
	decision, err := CheckFreeUserDailyTokenLimit(modelName)
	if err != nil || decision == nil || c == nil || c.inFlightTokens == nil {
		return decision, err
	}
	if decision.Exempt || decision.TokensLimit <= 0 {
		return decision, nil
	}
	bucketKey := ""
	if decision.ModelHasOwn {
		bucketKey = modelName
	}
	inFlight := c.inFlightTokens.Get(bucketKey)
	if inFlight <= 0 {
		return decision, nil
	}
	decision.TokensUsed += inFlight
	decision.Allowed = decision.TokensUsed < decision.TokensLimit
	return decision, nil
}

// FallbackEstimateResult 记录一次 fallback 估算扣费的结果，便于测试断言与日志诊断。
// 关键词: FallbackEstimateResult, ytoken 兜底扣费结果
type FallbackEstimateResult struct {
	// EstPromptTokens 估算的 prompt token 数 (text BPE token + image_count * 4096)
	EstPromptTokens int64
	// EstCompletionTokens 估算的 completion token 数 (output + reason BPE token 之和)
	EstCompletionTokens int64
	// Weighted 经过 ComputeWeightedTokens 四维倍率加权后的最终入账值
	Weighted int64
	// Billed 实际是否扣过费 (exempt / 没 key / weighted=0 均不扣)
	Billed bool
	// Bucket 实际扣费走的桶："global" / "model" / "apikey" / ""(未扣)
	Bucket string
}

// applyUsageFallbackEstimate 用 ytoken (Qwen BPE) 估算 prompt + completion token，
// 按与 onUsageForward 正路完全相同的 ComputeWeightedTokens 倍率与桶分发逻辑，
// 在上游 SSE 末帧 usage 缺失或为 0 时兜底扣费。
//
// 估算口径：
//   - prompt token  = ytoken(promptText) + imageCount * fallbackImageTokenEstimate
//     (vision 模型的 image 部分 ytoken 无法精确估算，按 4096 token 保守预扣)
//   - completion token = ytoken(outputText) + ytoken(reasonText)
//
// 与 onUsageForward 正路的差异：
//   - 不调用 RecordDailyCacheStats / RecordDailySummaryDelta (那些只反映真实上游数据)
//   - 不调用 writer.WriteUsage (estUsage 是估算值，不应下发给客户端)
//
// 关键词: applyUsageFallbackEstimate, ytoken 兜底, fallback 扣费分发
func (c *ServerConfig) applyUsageFallbackEstimate(
	modelName string,
	isFreeModel bool,
	key *Key,
	providerTypeName string,
	promptText string,
	imageCount int,
	outputText, reasonText string,
) FallbackEstimateResult {
	result := FallbackEstimateResult{}

	result.EstPromptTokens = int64(ytoken.CalcTokenCount(promptText)) +
		int64(imageCount)*fallbackImageTokenEstimate
	result.EstCompletionTokens = int64(ytoken.CalcTokenCount(outputText)) +
		int64(ytoken.CalcTokenCount(reasonText))

	if result.EstPromptTokens+result.EstCompletionTokens <= 0 {
		return result
	}

	estUsage := &aispec.ChatUsage{
		PromptTokens:     int(result.EstPromptTokens),
		CompletionTokens: int(result.EstCompletionTokens),
		TotalTokens:      int(result.EstPromptTokens + result.EstCompletionTokens),
	}
	meta, _ := GetModelMeta(modelName)
	result.Weighted = ComputeWeightedTokens(meta, estUsage)
	if result.Weighted <= 0 {
		return result
	}

	c.logInfo("upstream usage missing, ytoken fallback estimate: model=%s provider=%s prompt=%d (text+%d*4K image) completion=%d weighted=%d",
		modelName, providerTypeName, result.EstPromptTokens, imageCount, result.EstCompletionTokens, result.Weighted)

	if isFreeModel {
		overrides := parseFreeUserTokenModelOverridesFromConfig()
		ov, hasOverride := overrides[modelName]
		if hasOverride && ov.Exempt {
			c.logInfo("fallback Token billing skipped (exempt) for free model %s, weighted=%d", modelName, result.Weighted)
			return result
		}
		modelHasOwnBucket := hasOverride && ov.LimitM > 0
		if err := AddFreeUserDailyTokenUsage(modelName, result.Weighted, modelHasOwnBucket); err != nil {
			c.logWarn("fallback AddFreeUserDailyTokenUsage failed (model=%s weighted=%d): %v",
				modelName, result.Weighted, err)
			return result
		}
		result.Billed = true
		if modelHasOwnBucket {
			result.Bucket = "model"
		} else {
			result.Bucket = "global"
		}
		c.logInfo("fallback free user token usage updated: model=%s weighted=%d bucket_kind=%s",
			modelName, result.Weighted, result.Bucket)
		return result
	}

	if key == nil {
		c.logWarn("fallback billing skipped: non-free model has no API key (model=%s weighted=%d)",
			modelName, result.Weighted)
		return result
	}
	if err := UpdateAiApiKeyTokenUsed(key.Key, result.Weighted); err != nil {
		c.logWarn("fallback UpdateAiApiKeyTokenUsed failed (key=%s weighted=%d): %v",
			utils.ShrinkString(key.Key, 8), result.Weighted, err)
		return result
	}
	result.Billed = true
	result.Bucket = "apikey"
	c.logInfo("fallback API key token usage updated: key=%s model=%s weighted=%d",
		utils.ShrinkString(key.Key, 8), modelName, result.Weighted)
	return result
}
