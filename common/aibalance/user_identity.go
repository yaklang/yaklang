package aibalance

import (
	"crypto/sha1"
	"encoding/hex"
	"net"
	"strings"

	"github.com/yaklang/yaklang/common/utils/lowhttp"
)

// extractUserIdentity 从原始 HTTP 请求里识别"客户端身份"，用于日活去重。
// 返回 (sourceKind, userHash)：
//   - 当请求带可识别 API key（即 paid 用户）时：sourceKind = "api_key"，userHash = hash(key)。
//   - 否则若带 Trace-ID 头（free 用户主要标识）：sourceKind = "free_trace"，userHash = hash(trace_id)。
//   - 否则用 conn.RemoteAddr 的 IP 兜底：sourceKind = "free_ip"，userHash = hash(ip)。
//   - 三类身份的 hash 互不交叉（同一原始字符串落到不同 source_kind 也得到不同 user_hash），
//     避免免费用户和付费用户的同名指纹串台到同一日活桶。
//
// userHash 取 sha1(rawIdentity + "|" + sourceKind) 前 16 字节十六进制（共 32 字符），
// 与 ai_daily_user_seen.user_hash size:32 对齐。
//
// 关键词: aibalance user identity, dau 去重, api_key 优先, trace-id 兜底, remote_ip 终极兜底
func extractUserIdentity(rawPacket []byte, conn net.Conn, key *Key, isFreeModel bool) (string, string) {
	// 优先级 1：付费用户（已经过 KeyManager 校验、key 非 nil 且 key.Key 不为空）
	if !isFreeModel && key != nil && key.Key != "" {
		return SourceKindAPIKey, fingerprintIdentity(SourceKindAPIKey, key.Key)
	}

	// 优先级 2：未通过 KeyManager 但 Authorization 头里有 Bearer XXX，
	// 仍按 api_key 桶（这种情况理论上 chat 入口已经 401 拒绝了，
	// 但为了健壮性这里再兜一道，不会写出无效记录）。
	if rawAuth := lowhttp.GetHTTPPacketHeader(rawPacket, "Authorization"); rawAuth != "" {
		if token := parseBearerToken(rawAuth); token != "" && !isFreeModel {
			return SourceKindAPIKey, fingerprintIdentity(SourceKindAPIKey, token)
		}
	}

	// 优先级 3：Trace-ID（免费用户在 web search / amap 已经走 Trace-ID 协议，
	// chat 入口没强制要求，但客户端如果带了我们就用）。
	if traceID := lookupHeader(rawPacket, "Trace-ID", "Trace-Id", "trace-id", "X-Trace-ID", "X-Trace-Id"); traceID != "" {
		return SourceKindFreeTrace, fingerprintIdentity(SourceKindFreeTrace, traceID)
	}

	// 优先级 4：RemoteAddr IP 兜底。
	ip := remoteIPOf(conn)
	if ip == "" {
		ip = "unknown"
	}
	return SourceKindFreeIP, fingerprintIdentity(SourceKindFreeIP, ip)
}

// fingerprintIdentity 把 (sourceKind, raw) 哈希为 32 字符十六进制串。
// sourceKind 参与哈希，避免不同 bucket 的同名 raw 串台到同一 user_hash。
// 关键词: fingerprintIdentity, sha1 prefix 32, source_kind 拌料
func fingerprintIdentity(sourceKind, raw string) string {
	sum := sha1.Sum([]byte(sourceKind + "|" + raw))
	return hex.EncodeToString(sum[:])[:32]
}

// parseBearerToken 从 "Bearer xxx" 抽 token，否则返回空串。
// 关键词: parseBearerToken, Authorization 头解析
func parseBearerToken(authHeader string) string {
	parts := strings.SplitN(strings.TrimSpace(authHeader), " ", 2)
	if len(parts) != 2 {
		return ""
	}
	if !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}
	return strings.TrimSpace(parts[1])
}

// lookupHeader 多 key 候选查找一个 header 值（按顺序，第一个非空命中）。
// 关键词: lookupHeader, 多名候选头
func lookupHeader(rawPacket []byte, candidates ...string) string {
	for _, name := range candidates {
		if v := lowhttp.GetHTTPPacketHeader(rawPacket, name); v != "" {
			return v
		}
	}
	return ""
}

// remoteIPOf 从 net.Conn.RemoteAddr 抽出客户端 IP（不含端口），支持 IPv4 / IPv6。
// 关键词: remoteIPOf, conn.RemoteAddr IP 提取, IPv6 兼容
func remoteIPOf(conn net.Conn) string {
	if conn == nil {
		return ""
	}
	addr := conn.RemoteAddr()
	if addr == nil {
		return ""
	}
	switch a := addr.(type) {
	case *net.TCPAddr:
		if a.IP == nil {
			return ""
		}
		return a.IP.String()
	case *net.UDPAddr:
		if a.IP == nil {
			return ""
		}
		return a.IP.String()
	}
	host, _, err := net.SplitHostPort(addr.String())
	if err != nil {
		return addr.String()
	}
	return host
}
