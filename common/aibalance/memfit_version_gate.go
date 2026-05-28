package aibalance

import (
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/log"
)

// MemfitVersionGateResult 描述一次 memfit 客户端版本控流的判定结果。
// 关键词: MemfitVersionGateResult 版本控流判定
type MemfitVersionGateResult struct {
	Blocked         bool   // 是否拦截
	Reason          string // 拦截原因: missing_version | outdated_buildtime | empty(放行)
	ClientVersion   string // 客户端上报版本（dev/unknown/v1.x.y）
	ClientBuildTime string // 客户端上报的 BuildTime
	MinBuildTime    string // 当前生效的最小允许 BuildTime（仅当启用时有意义）
}

// checkMemfitVersionGate 判断是否要因客户端版本控流而拒绝 memfit-* 请求。
//
// 规则（按顺序）:
//  1. 读 DB 配置失败 → 不拦截（降级放行，永不影响主链路）
//  2. 开关未启用    → 不拦截
//  3. version 为空 / "unknown" → 拦截 (reason=missing_version)
//  4. version 含 "dev" 字样   → 不拦截（开发版本豁免）
//  5. 配置了 MinBuildTime 但客户端未上报 BuildTime → 拦截 (reason=missing_version)
//  6. 客户端 BuildTime 早于 MinBuildTime → 拦截 (reason=outdated_buildtime)
//  7. 其余情况 → 不拦截
//
// 关键词: checkMemfitVersionGate memfit 版本控流核心判定, dev 豁免, BuildTime 比较
func (c *ServerConfig) checkMemfitVersionGate(version, buildTime string) MemfitVersionGateResult {
	res := MemfitVersionGateResult{
		ClientVersion:   version,
		ClientBuildTime: buildTime,
	}

	cfg, err := GetRateLimitConfig()
	if err != nil || cfg == nil {
		if err != nil {
			c.logWarn("checkMemfitVersionGate: GetRateLimitConfig failed: %v (pass-through)", err)
		}
		return res
	}
	if !cfg.MemfitVersionGateEnabled {
		return res
	}
	res.MinBuildTime = cfg.MemfitVersionMinBuildTime

	v := strings.TrimSpace(version)
	if v == "" || strings.EqualFold(v, "unknown") {
		res.Blocked = true
		res.Reason = "missing_version"
		return res
	}
	// dev 版本豁免（包含子串即可，例如 dev / v1.2.3-dev / dev-build）
	if strings.Contains(strings.ToLower(v), "dev") {
		return res
	}

	minBT := strings.TrimSpace(cfg.MemfitVersionMinBuildTime)
	if minBT == "" {
		return res
	}

	minTime, parseErr := parseFlexibleBuildTime(minBT)
	if parseErr != nil {
		// 后台配置错误 → 不拦截，仅告警，避免误伤
		c.logWarn("checkMemfitVersionGate: failed to parse MemfitVersionMinBuildTime %q: %v (pass-through)", minBT, parseErr)
		return res
	}

	bt := strings.TrimSpace(buildTime)
	if bt == "" {
		res.Blocked = true
		res.Reason = "missing_version"
		return res
	}
	clientTime, parseErr := parseFlexibleBuildTime(bt)
	if parseErr != nil {
		// 客户端 BuildTime 无法解析 → 视为旧版本拦截
		res.Blocked = true
		res.Reason = "outdated_buildtime"
		return res
	}
	if clientTime.Before(minTime) {
		res.Blocked = true
		res.Reason = "outdated_buildtime"
		return res
	}
	return res
}

// parseFlexibleBuildTime 兼容多种 BuildTime 字符串格式：
//   - RFC3339 / RFC3339Nano（推荐）
//   - "2006-01-02 15:04:05 +0800 CST"（Go time.Now().String() 默认）
//   - "2006-01-02 15:04:05"
//   - "2006-01-02"
//
// 关键词: parseFlexibleBuildTime BuildTime 多格式解析
func parseFlexibleBuildTime(s string) (time.Time, error) {
	candidates := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02 15:04:05.999999999 -0700 MST",
		"2006-01-02 15:04:05 -0700 MST",
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05",
		"2006-01-02",
	}
	var lastErr error
	for _, layout := range candidates {
		if t, err := time.Parse(layout, s); err == nil {
			return t, nil
		} else {
			lastErr = err
		}
	}
	return time.Time{}, lastErr
}

// writeMemfitVersionRateLimitResponse 写 429 响应，专用于 memfit 客户端版本控流。
//
// 文案需要突出「低版本或未知版本限额已满 + 升级提示」，按 PRD 写死中文文案；
// 同时给一个 X-AIBalance-Limit-Kind: memfit_client_version 头, 便于 yakit
// 客户端识别后做专门提示。
//
// 关键词: writeMemfitVersionRateLimitResponse, memfit 客户端版本控流 429, X-AIBalance-Limit-Kind memfit_client_version
func (c *ServerConfig) writeMemfitVersionRateLimitResponse(conn net.Conn, reason string) {
	const message = "针对旧版本的 Memfit/Yak 系统使用量已达到最大上限，最大上限为 1 亿。请更新最新版本 Yak 引擎或最新版本 Memfit/Yak Project 系统以提升用户体验。"
	if reason == "" {
		reason = "unknown"
	}
	// 显式手写 JSON 以避免对外部 marshaler 的依赖；message 全角标点本身在 JSON 字符串内合法。
	body := fmt.Sprintf(
		`{"error":{"type":"memfit_client_version_limited","limit_kind":"memfit_client_version","limit_kind_zh":"\u5ba2\u6237\u7aef\u7248\u672c\u9650\u6d41","reason":%q,"message":%q}}`,
		reason, message,
	)
	header := fmt.Sprintf(
		"HTTP/1.1 429 Too Many Requests\r\n"+
			"Content-Type: application/json; charset=utf-8\r\n"+
			"X-AIBalance-Limit-Kind: memfit_client_version\r\n"+
			"X-AIBalance-Memfit-Version-Reason: %s\r\n"+
			"Content-Length: %d\r\n"+
			"\r\n",
		reason, len(body),
	)
	if _, err := conn.Write([]byte(header)); err != nil {
		log.Debugf("writeMemfitVersionRateLimitResponse: write header failed: %v", err)
		return
	}
	if _, err := conn.Write([]byte(body)); err != nil {
		log.Debugf("writeMemfitVersionRateLimitResponse: write body failed: %v", err)
	}
}
