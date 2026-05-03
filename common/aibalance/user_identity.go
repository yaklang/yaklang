package aibalance

import (
	"crypto/sha1"
	"encoding/hex"
	"net"
	"strings"

	"github.com/yaklang/yaklang/common/utils/lowhttp"
)

// 客户端 IP 提取相关常量与帮助函数。
//
// 背景:
//   aibalance 在生产环境通常部署在 nginx / Cloudflare 等反向代理之后,
//   conn.RemoteAddr() 拿到的是反代节点的内网地址 (例如 127.0.0.1 / 10.x.x.x),
//   而不是真实客户端 IP。如果直接拿 conn.RemoteAddr() 做 free_ip DAU 去重,
//   所有 free 用户都会被聚成 1 个, DAU 永远等于 1, 用户数指标完全失真。
//
// 修复思路:
//   按业界主流反代头优先级抽取真实客户端 IP, 全部 fallback 才回到 conn.RemoteAddr。
//   头部优先级 (任一命中即采用, 无需依赖 trusted-proxy 列表, 反代节点
//   一般会主动覆盖客户端伪造的同名头, 所以这里直接信任):
//     1) CF-Connecting-IP   (Cloudflare 官方推荐, 永远是真实客户端 IP)
//     2) True-Client-IP     (Akamai / Cloudflare Enterprise)
//     3) X-Real-IP          (nginx 默认 ngx_http_realip_module 输出)
//     4) X-Forwarded-For    (最广泛, 取最左一个非私有/非 loopback 的)
//     5) Forwarded          (RFC 7239)
//     6) conn.RemoteAddr()  (终极兜底)
//
// 关键词: aibalance 真实客户端 IP, nginx 反代 IP 修复, X-Forwarded-For,
//        CF-Connecting-IP, X-Real-IP, RFC 7239 Forwarded, free_ip DAU 修复

// clientIPHeaderCandidates 列出按优先级查找单值客户端 IP 头的名单,
// X-Forwarded-For / Forwarded 因为是多值或键值对结构, 单独走专用解析.
// 关键词: 客户端 IP 头优先级
var clientIPHeaderCandidates = []string{
	"CF-Connecting-IP",
	"True-Client-IP",
	"X-Real-IP",
	"X-Real-Ip",
	"X-Client-IP",
}

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

	// 优先级 4：客户端真实 IP 兜底。
	// 关键修复：先解析反代头 (CF-Connecting-IP / True-Client-IP /
	// X-Real-IP / X-Forwarded-For / Forwarded), 全部缺失才回落
	// conn.RemoteAddr。否则 nginx 反代后所有 free 用户都会被识别成
	// 同一个 nginx 内网 IP, free_ip DAU 永远 = 1。
	// 关键词: extractClientIP, free_ip 真实 IP 修复
	ip := extractClientIP(rawPacket, conn)
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

// extractClientIP 按反代头优先级提取真实客户端 IP, 全部缺失才回落 conn.RemoteAddr。
// 反代头按 CF-Connecting-IP > True-Client-IP > X-Real-IP > X-Forwarded-For >
// Forwarded(RFC 7239) 顺序检查; 任一命中且能解析出有效 IP 即采用。
//
// 对 X-Forwarded-For 这类多值头, 从最左到最右逐个尝试, 优先返回第一个
// "公网 IP"; 全部都是私有 / loopback / link-local 时返回最左一个。
// 这是因为反代链路上往往是: client(公网)->edge(公网)->lb(私有)->upstream,
// XFF 形如 "client_ip, edge_ip, lb_ip", 客户端 IP 总在最左。
//
// 关键词: extractClientIP, 客户端真实 IP 提取, 反代头优先级,
//        X-Forwarded-For 多值解析, 公网 IP 优先, 私有 IP 跳过
func extractClientIP(rawPacket []byte, conn net.Conn) string {
	// 优先级 1: 单值反代头 (CF-Connecting-IP / True-Client-IP / X-Real-IP 等)
	for _, h := range clientIPHeaderCandidates {
		val := strings.TrimSpace(lowhttp.GetHTTPPacketHeader(rawPacket, h))
		if val == "" {
			continue
		}
		if ip := normalizeIP(val); ip != "" {
			return ip
		}
	}

	// 优先级 2: X-Forwarded-For (多值, 用 , 分隔)
	if xff := strings.TrimSpace(lowhttp.GetHTTPPacketHeader(rawPacket, "X-Forwarded-For")); xff != "" {
		if ip := pickClientIPFromXFF(xff); ip != "" {
			return ip
		}
	}

	// 优先级 3: RFC 7239 Forwarded
	if fw := strings.TrimSpace(lowhttp.GetHTTPPacketHeader(rawPacket, "Forwarded")); fw != "" {
		if ip := pickClientIPFromForwarded(fw); ip != "" {
			return ip
		}
	}

	// 优先级 4: conn.RemoteAddr 终极兜底
	return remoteIPOf(conn)
}

// pickClientIPFromXFF 从 X-Forwarded-For 多值字符串里挑出最可能的客户端 IP。
// 规则: 从左到右扫, 优先返回第一个公网 IP; 全部为私有 / loopback / link-local
// 时返回最左一个有效 IP; 没有任何有效 IP 返回空串。
// 关键词: X-Forwarded-For 解析, 公网 IP 优先, 私有 IP 兜底
func pickClientIPFromXFF(xff string) string {
	parts := strings.Split(xff, ",")
	var firstValid string
	for _, p := range parts {
		ip := normalizeIP(strings.TrimSpace(p))
		if ip == "" {
			continue
		}
		if firstValid == "" {
			firstValid = ip
		}
		if isPublicIP(ip) {
			return ip
		}
	}
	return firstValid
}

// pickClientIPFromForwarded 从 RFC 7239 Forwarded 头里抽 for=xxx 参数。
// 例: Forwarded: for=192.0.2.60;proto=http;by=203.0.113.43, for=10.0.0.1
// 同样取最左一个有效 IP, 优先公网。
// 关键词: RFC 7239 Forwarded 解析
func pickClientIPFromForwarded(fwHeader string) string {
	parts := strings.Split(fwHeader, ",")
	var firstValid string
	for _, segment := range parts {
		for _, kv := range strings.Split(segment, ";") {
			kv = strings.TrimSpace(kv)
			if !strings.HasPrefix(strings.ToLower(kv), "for=") {
				continue
			}
			val := strings.TrimSpace(kv[4:])
			val = strings.Trim(val, `"`)
			// for="[2001:db8::1]:8080" -> 去掉 [ ] 端口
			if strings.HasPrefix(val, "[") {
				if rb := strings.Index(val, "]"); rb > 0 {
					val = val[1:rb]
				}
			} else if h, _, err := net.SplitHostPort(val); err == nil {
				val = h
			}
			ip := normalizeIP(val)
			if ip == "" {
				continue
			}
			if firstValid == "" {
				firstValid = ip
			}
			if isPublicIP(ip) {
				return ip
			}
			break
		}
	}
	return firstValid
}

// normalizeIP 校验输入是合法 IP, 并返回标准化字符串(IPv4 / IPv6 都支持)。
// 不合法返回空串。同时剥离可能附带的端口号。
// 关键词: normalizeIP, IP 合法性校验, 端口剥离
func normalizeIP(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	// 直接是合法 IP
	if ip := net.ParseIP(s); ip != nil {
		return ip.String()
	}
	// 形如 "1.2.3.4:5678" / "[::1]:5678"
	if h, _, err := net.SplitHostPort(s); err == nil {
		if ip := net.ParseIP(h); ip != nil {
			return ip.String()
		}
	}
	// "[::1]" (no port)
	if strings.HasPrefix(s, "[") && strings.HasSuffix(s, "]") {
		if ip := net.ParseIP(s[1 : len(s)-1]); ip != nil {
			return ip.String()
		}
	}
	return ""
}

// isPublicIP 判断给定 IP 是否为「真正的公网 IP」, 即不是 loopback /
// 私有(RFC1918) / link-local / unspecified / multicast / 文档保留地址。
// 关键词: isPublicIP, 公网 IP 判定, 私有地址过滤
func isPublicIP(s string) bool {
	ip := net.ParseIP(s)
	if ip == nil {
		return false
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsInterfaceLocalMulticast() ||
		ip.IsMulticast() || ip.IsUnspecified() {
		return false
	}
	// IPv4 169.254.0.0/16 link-local 已被 IsLinkLocalUnicast 覆盖,
	// 不再额外判断。
	return true
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
