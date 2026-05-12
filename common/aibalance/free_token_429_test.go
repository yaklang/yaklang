package aibalance

import (
	"encoding/json"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 关键词: free_token_429_test, writeDailyTokenLimitResponse format

// TestWriteDailyTokenLimitResponse_Format 验证 429 响应字段和 header 与 plan 中的契约一致。
func TestWriteDailyTokenLimitResponse_Format(t *testing.T) {
	cfg := NewServerConfig()
	defer cfg.Close()

	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	go func() {
		cfg.writeDailyTokenLimitResponse(server, "memfit-standard-free", "global", 1234567890, 1200000000)
		server.Close()
	}()

	var result []byte
	buf := make([]byte, 8192)
	for {
		client.SetReadDeadline(time.Now().Add(2 * time.Second))
		n, err := client.Read(buf)
		if n > 0 {
			result = append(result, buf[:n]...)
		}
		if err != nil {
			break
		}
	}
	resp := string(result)

	assert.Contains(t, resp, "HTTP/1.1 429 Too Many Requests")
	assert.Contains(t, resp, "X-AIBalance-Limit-Kind: daily_token")
	assert.Contains(t, resp, "X-AIBalance-Token-Used: 1234567890")
	assert.Contains(t, resp, "X-AIBalance-Token-Limit: 1200000000")
	assert.Contains(t, resp, "Retry-After: 3600")

	bodyIdx := strings.Index(resp, "\r\n\r\n")
	require.Greater(t, bodyIdx, 0)

	var parsed map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(resp[bodyIdx+4:]), &parsed))
	errObj, ok := parsed["error"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "daily_token_limit_exceeded", errObj["type"])
	assert.Equal(t, "daily_token_quota", errObj["limit_kind"])
	assert.Equal(t, "日限额已满", errObj["limit_kind_zh"])
	assert.Equal(t, "global", errObj["bucket"])
	assert.Equal(t, "memfit-standard-free", errObj["model"])
	assert.Equal(t, float64(1234567890), errObj["tokens_used"])
	assert.Equal(t, float64(1200000000), errObj["tokens_limit"])
	assert.Equal(t, float64(1200), errObj["tokens_limit_m"])
}

// TestWriteRPMRateLimitResponse_HasLimitKindHeader 验证拆分后的 RPM 429 也带 X-AIBalance-Limit-Kind 头。
func TestWriteRPMRateLimitResponse_HasLimitKindHeader(t *testing.T) {
	cfg := NewServerConfig()
	defer cfg.Close()

	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	go func() {
		cfg.writeRPMRateLimitResponse(server, 7)
		server.Close()
	}()

	var result []byte
	buf := make([]byte, 4096)
	for {
		client.SetReadDeadline(time.Now().Add(2 * time.Second))
		n, err := client.Read(buf)
		if n > 0 {
			result = append(result, buf[:n]...)
		}
		if err != nil {
			break
		}
	}
	resp := string(result)

	assert.Contains(t, resp, "X-AIBalance-Limit-Kind: rpm")
	assert.Contains(t, resp, "X-AIBalance-Info: 7")

	bodyIdx := strings.Index(resp, "\r\n\r\n")
	require.Greater(t, bodyIdx, 0)
	var parsed map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(resp[bodyIdx+4:]), &parsed))
	errObj := parsed["error"].(map[string]interface{})
	assert.Equal(t, "rate_limit_exceeded", errObj["type"])
	assert.Equal(t, "rpm", errObj["limit_kind"])
}

// TestSaveModelMetaWithMultipliers_FullRoundTrip 验证 4 维 Token 倍率读写
func TestSaveModelMetaWithMultipliers_FullRoundTrip(t *testing.T) {
	require.NoError(t, EnsureModelMetaTable())

	modelName := "test-model-mul-" + time.Now().Format("150405.000000")
	defer GetDB().Unscoped().Where("model_name = ?", modelName).Delete(&AiModelMeta{})

	require.NoError(t, SaveModelMetaWithMultipliers(modelName, "desc", "tag1,tag2", 2.0, 1.5, 1.8, 2.0, 0.05))

	meta, err := GetModelMeta(modelName)
	require.NoError(t, err)
	require.NotNil(t, meta)
	assert.Equal(t, "desc", meta.Description)
	assert.Equal(t, "tag1,tag2", meta.Tags)
	assert.Equal(t, 2.0, meta.TrafficMultiplier)
	assert.Equal(t, 1.5, meta.InputTokenMultiplier)
	assert.Equal(t, 1.8, meta.OutputTokenMultiplier)
	assert.Equal(t, 2.0, meta.CacheCreationMultiplier)
	assert.Equal(t, 0.05, meta.CacheHitMultiplier)

	// 部分更新：传 -1 表示不更新
	require.NoError(t, SaveModelMetaWithMultipliers(modelName, "desc2", "newtag", -1, -1, 9.9, -1, -1))
	meta2, err := GetModelMeta(modelName)
	require.NoError(t, err)
	assert.Equal(t, "desc2", meta2.Description)
	assert.Equal(t, "newtag", meta2.Tags)
	assert.Equal(t, 2.0, meta2.TrafficMultiplier, "TrafficMultiplier should NOT change when -1")
	assert.Equal(t, 1.5, meta2.InputTokenMultiplier, "InputMul should NOT change when -1")
	assert.Equal(t, 9.9, meta2.OutputTokenMultiplier, "OutputMul SHOULD update")
	assert.Equal(t, 2.0, meta2.CacheCreationMultiplier, "CacheCreate should NOT change when -1")
	assert.Equal(t, 0.05, meta2.CacheHitMultiplier, "CacheHit should NOT change when -1")
}

// TestSaveModelMetaWithMultiplier_BackwardCompat 老接口仍然只更新 TrafficMultiplier。
func TestSaveModelMetaWithMultiplier_BackwardCompat(t *testing.T) {
	require.NoError(t, EnsureModelMetaTable())

	modelName := "test-bw-compat-" + time.Now().Format("150405.000000")
	defer GetDB().Unscoped().Where("model_name = ?", modelName).Delete(&AiModelMeta{})

	require.NoError(t, SaveModelMetaWithMultipliers(modelName, "init", "", 1.0, 2.0, 3.0, 4.0, 5.0))

	require.NoError(t, SaveModelMetaWithMultiplier(modelName, "updated", "newtag", 7.5))

	meta, err := GetModelMeta(modelName)
	require.NoError(t, err)
	assert.Equal(t, "updated", meta.Description)
	assert.Equal(t, "newtag", meta.Tags)
	assert.Equal(t, 7.5, meta.TrafficMultiplier)
	// 4 维倍率不动
	assert.Equal(t, 2.0, meta.InputTokenMultiplier)
	assert.Equal(t, 3.0, meta.OutputTokenMultiplier)
	assert.Equal(t, 4.0, meta.CacheCreationMultiplier)
	assert.Equal(t, 5.0, meta.CacheHitMultiplier)
}

// TestModelOverridePriority_ExemptVsLimitVsGlobal 验证优先级顺序：exempt > 模型独立桶 > 全局共享池
func TestModelOverridePriority_ExemptVsLimitVsGlobal(t *testing.T) {
	require.NoError(t, EnsureFreeUserDailyTokenUsageTable())

	date := time.Now().AddDate(0, 0, 320).Format("2006-01-02")
	defer cleanupFreeUserTokenForDate(t, date)
	defer setFreeTokenNowDate(date)()

	// 全局 1M, 模型独立桶 model-iso 5M, 模型豁免 model-exempt
	setRateLimitConfigForFreeTokenTest(t, 1, `{"model-iso":{"limit_m":5,"exempt":false},"model-exempt":{"limit_m":100,"exempt":true}}`)
	defer setRateLimitConfigForFreeTokenTest(t, 1200, "{}")

	// 全局桶用满
	require.NoError(t, AddFreeUserDailyTokenUsage("foo", 1*FreeUserTokenMUnit, false))

	// 1) exempt 永远放行（即使 limit_m 和全局都被忽略）
	d1, err := CheckFreeUserDailyTokenLimit("model-exempt")
	require.NoError(t, err)
	assert.True(t, d1.Allowed)
	assert.True(t, d1.Exempt)

	// 2) 模型独立桶不受全局影响
	d2, err := CheckFreeUserDailyTokenLimit("model-iso")
	require.NoError(t, err)
	assert.True(t, d2.Allowed)
	assert.True(t, d2.ModelHasOwn)
	assert.Equal(t, "model", d2.Bucket)

	// 3) 没有覆盖的模型走全局桶（已超额）
	d3, err := CheckFreeUserDailyTokenLimit("unknown-free")
	require.NoError(t, err)
	assert.False(t, d3.Allowed)
	assert.Equal(t, "global", d3.Bucket)
}
