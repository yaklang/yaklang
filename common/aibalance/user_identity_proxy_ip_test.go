package aibalance

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// 关键词: nginx 反代 IP 修复 测试, X-Forwarded-For 解析,
//        CF-Connecting-IP, X-Real-IP, RFC 7239 Forwarded, free_ip DAU 修复

// TestNormalizeIP 覆盖 IPv4/IPv6 / 带端口 / 非法字符串 / 空串 等情况。
// 关键词: normalizeIP 边界
func TestNormalizeIP(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"203.0.113.7", "203.0.113.7"},
		{"  203.0.113.7  ", "203.0.113.7"},
		{"203.0.113.7:8080", "203.0.113.7"},
		{"2001:db8::1", "2001:db8::1"},
		{"[2001:db8::1]:443", "2001:db8::1"},
		{"[2001:db8::1]", "2001:db8::1"},
		{"::1", "::1"},
		{"not-an-ip", ""},
		{"", ""},
		{"   ", ""},
	}
	for _, c := range cases {
		got := normalizeIP(c.in)
		assert.Equalf(t, c.want, got, "normalizeIP(%q)", c.in)
	}
}

// TestIsPublicIP 覆盖公网 / 私有 / loopback / link-local / 文档地址。
// 关键词: isPublicIP 公网判定
func TestIsPublicIP(t *testing.T) {
	cases := []struct {
		ip   string
		want bool
	}{
		// 公网
		{"203.0.113.7", true},
		{"8.8.8.8", true},
		{"2001:4860:4860::8888", true},
		// 私有 (RFC1918)
		{"10.0.0.1", false},
		{"172.16.0.1", false},
		{"172.31.255.254", false},
		{"192.168.0.1", false},
		// IPv6 ULA
		{"fc00::1", false},
		{"fd00::1", false},
		// loopback
		{"127.0.0.1", false},
		{"::1", false},
		// link-local
		{"169.254.1.1", false},
		{"fe80::1", false},
		// unspecified
		{"0.0.0.0", false},
		{"::", false},
		// multicast
		{"224.0.0.1", false},
		{"ff02::1", false},
		// 非法
		{"not-an-ip", false},
		{"", false},
	}
	for _, c := range cases {
		got := isPublicIP(c.ip)
		assert.Equalf(t, c.want, got, "isPublicIP(%q)", c.ip)
	}
}

// TestPickClientIPFromXFF 覆盖 X-Forwarded-For 多值/全私有/带空格/带端口 等。
// 关键词: X-Forwarded-For 解析, 公网优先, 私有兜底
func TestPickClientIPFromXFF(t *testing.T) {
	cases := []struct {
		name string
		xff  string
		want string
	}{
		{"single public", "203.0.113.7", "203.0.113.7"},
		{"public then private", "203.0.113.7, 10.0.0.1, 192.168.1.1", "203.0.113.7"},
		// 公网在中间也要被找到
		{"private then public", "10.0.0.1, 203.0.113.7", "203.0.113.7"},
		// 客户端在最左 (典型 nginx 链路: client -> edge -> lb)
		{"typical client+edge+lb", "1.2.3.4, 198.51.100.10, 10.0.0.5", "1.2.3.4"},
		// 全私有 -> 最左有效兜底
		{"all private", "10.0.0.1, 172.16.0.1, 192.168.1.1", "10.0.0.1"},
		// 带端口
		{"with port", "203.0.113.7:8080, 10.0.0.1", "203.0.113.7"},
		// 含空格 / 大小写
		{"messy spaces", "  203.0.113.7  ,  10.0.0.1  ", "203.0.113.7"},
		// 仅一个非法
		{"only invalid", "not-an-ip", ""},
		// 全部非法
		{"all invalid", "abc, def, ghi", ""},
		// 空值
		{"empty", "", ""},
		// IPv6
		{"ipv6 public", "2001:4860:4860::8888, fc00::1", "2001:4860:4860::8888"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := pickClientIPFromXFF(c.xff)
			assert.Equal(t, c.want, got)
		})
	}
}

// TestPickClientIPFromForwarded 覆盖 RFC 7239 Forwarded 头多种写法。
// 关键词: RFC 7239 Forwarded 解析
func TestPickClientIPFromForwarded(t *testing.T) {
	cases := []struct {
		name string
		hdr  string
		want string
	}{
		{"basic", "for=192.0.2.60;proto=http;by=203.0.113.43", "192.0.2.60"},
		{"quoted", `for="192.0.2.60"`, "192.0.2.60"},
		{"with port", `for="192.0.2.60:8080"`, "192.0.2.60"},
		{"ipv6 bracketed", `for="[2001:db8::1]:8080"`, "2001:db8::1"},
		{"ipv6 bracketed no port", `for="[2001:db8::1]"`, "2001:db8::1"},
		{"chain with public+private", `for=203.0.113.7, for=10.0.0.1`, "203.0.113.7"},
		{"private then public", `for=10.0.0.1, for=203.0.113.7`, "203.0.113.7"},
		{"all private", `for=10.0.0.1, for=192.168.1.1`, "10.0.0.1"},
		{"empty", "", ""},
		{"no for kv", "by=10.0.0.1;proto=https", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := pickClientIPFromForwarded(c.hdr)
			assert.Equal(t, c.want, got)
		})
	}
}

// TestExtractClientIP_HeaderPriority 覆盖反代头优先级。
// 关键词: extractClientIP 优先级
func TestExtractClientIP_HeaderPriority(t *testing.T) {
	conn := newFakeTCPConn("127.0.0.1") // 模拟 nginx 反代后 conn 是本地

	// 优先级 1: CF-Connecting-IP 最高
	pkt := packetWith(
		"CF-Connecting-IP: 1.1.1.1",
		"True-Client-IP: 2.2.2.2",
		"X-Real-IP: 3.3.3.3",
		"X-Forwarded-For: 4.4.4.4, 10.0.0.1",
	)
	assert.Equal(t, "1.1.1.1", extractClientIP(pkt, conn))

	// CF 缺失 -> True-Client-IP
	pkt = packetWith(
		"True-Client-IP: 2.2.2.2",
		"X-Real-IP: 3.3.3.3",
		"X-Forwarded-For: 4.4.4.4",
	)
	assert.Equal(t, "2.2.2.2", extractClientIP(pkt, conn))

	// CF + True-Client-IP 缺失 -> X-Real-IP
	pkt = packetWith(
		"X-Real-IP: 3.3.3.3",
		"X-Forwarded-For: 4.4.4.4",
	)
	assert.Equal(t, "3.3.3.3", extractClientIP(pkt, conn))

	// 仅 X-Forwarded-For
	pkt = packetWith("X-Forwarded-For: 4.4.4.4, 10.0.0.1, 192.168.1.1")
	assert.Equal(t, "4.4.4.4", extractClientIP(pkt, conn))

	// 仅 Forwarded
	pkt = packetWith(`Forwarded: for=5.5.5.5;proto=https`)
	assert.Equal(t, "5.5.5.5", extractClientIP(pkt, conn))

	// 全部缺失 -> conn.RemoteAddr (本例返回 127.0.0.1, 但能复现 free_ip 收敛 bug)
	pkt = packetWith()
	assert.Equal(t, "127.0.0.1", extractClientIP(pkt, conn))
}

// TestExtractUserIdentity_RealClientIPFromXFF 关键回归: nginx 反代后,
// conn.RemoteAddr 是 nginx 内网地址 (127.0.0.1), 但 extractUserIdentity
// 应当从 X-Forwarded-For 提取真实公网 IP, 不同公网 IP 落到不同 user_hash。
// 关键词: nginx 反代 free_ip DAU 修复 回归保护
func TestExtractUserIdentity_RealClientIPFromXFF(t *testing.T) {
	connNginx := newFakeTCPConn("127.0.0.1")

	pkt1 := packetWith("X-Forwarded-For: 1.2.3.4, 10.0.0.1")
	pkt2 := packetWith("X-Forwarded-For: 5.6.7.8, 10.0.0.1")
	pkt3 := packetWith("X-Forwarded-For: 1.2.3.4, 10.0.0.5") // 同一客户端不同链路

	k1, h1 := extractUserIdentity(pkt1, connNginx, nil, true)
	k2, h2 := extractUserIdentity(pkt2, connNginx, nil, true)
	k3, h3 := extractUserIdentity(pkt3, connNginx, nil, true)

	assert.Equal(t, SourceKindFreeIP, k1)
	assert.Equal(t, SourceKindFreeIP, k2)
	assert.Equal(t, SourceKindFreeIP, k3)
	assert.NotEqual(t, h1, h2, "different real client IPs must produce different user_hash")
	assert.Equal(t, h1, h3, "same real client IP must produce same user_hash regardless of edge IP")
}

// TestExtractUserIdentity_CFConnectingIP 验证 Cloudflare 部署下也能拿到真实 IP。
// 关键词: Cloudflare CF-Connecting-IP 兼容
func TestExtractUserIdentity_CFConnectingIP(t *testing.T) {
	connEdge := newFakeTCPConn("10.0.0.1") // CF 边缘节点连过来
	pkt := packetWith(
		"CF-Connecting-IP: 8.8.8.8",
		"X-Forwarded-For: 8.8.8.8, 10.0.0.1",
	)
	kind, hash := extractUserIdentity(pkt, connEdge, nil, true)
	assert.Equal(t, SourceKindFreeIP, kind)
	assert.Len(t, hash, 32)

	// 同一真实 IP 不同 edge 链路应当 user_hash 相同
	pkt2 := packetWith(
		"CF-Connecting-IP: 8.8.8.8",
		"X-Forwarded-For: 8.8.8.8, 10.99.99.99",
	)
	conn2 := newFakeTCPConn("10.99.99.99")
	_, hash2 := extractUserIdentity(pkt2, conn2, nil, true)
	assert.Equal(t, hash, hash2)
}

// TestExtractUserIdentity_TraceIDStillWinsOverProxyIP 验证 Trace-ID 仍然
// 优先于 IP 提取 (优先级未被破坏)。
// 关键词: Trace-ID 优先级保持
func TestExtractUserIdentity_TraceIDStillWinsOverProxyIP(t *testing.T) {
	conn := newFakeTCPConn("127.0.0.1")
	pkt := packetWith(
		"X-Trace-ID: trace-aaa",
		"X-Forwarded-For: 1.2.3.4",
		"CF-Connecting-IP: 5.6.7.8",
	)
	kind, _ := extractUserIdentity(pkt, conn, nil, true)
	assert.Equal(t, SourceKindFreeTrace, kind, "Trace-ID 必须比反代头优先")
}

// TestExtractUserIdentity_AllPrivateXFFFallsBackToConn 验证 XFF 全是私有 IP
// 时, 取最左私有 IP 而不是退回 conn.RemoteAddr (因为 conn.RemoteAddr 才是
// 内网, XFF 最左至少代表反代链路上更靠近真实客户端的一跳)。
// 关键词: XFF 全私有兜底, 最左有效 IP
func TestExtractUserIdentity_AllPrivateXFFFallsBackToConn(t *testing.T) {
	conn := newFakeTCPConn("127.0.0.1")
	pkt := packetWith("X-Forwarded-For: 10.0.0.1, 192.168.1.1")
	kind, _ := extractUserIdentity(pkt, conn, nil, true)
	assert.Equal(t, SourceKindFreeIP, kind)

	pkt2 := packetWith("X-Forwarded-For: 192.168.1.1, 10.0.0.1")
	_, h1 := extractUserIdentity(pkt, conn, nil, true)
	_, h2 := extractUserIdentity(pkt2, conn, nil, true)
	assert.NotEqual(t, h1, h2, "10.0.0.1 vs 192.168.1.1 应当落到不同 user_hash")
}
