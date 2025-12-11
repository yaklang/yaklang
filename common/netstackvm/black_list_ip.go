package netstackvm

import (
	"net"
)

// ipCheck 定义了一个检查规则，包含一个检查函数和一个原因描述。
// 这种结构使得添加新的黑名单规则变得非常容易。
type ipCheck struct {
	// reason 描述了为何此IP被列入黑名单。
	reason string
	// isBlacklisted 是一个函数，接收一个 net.IP 对象，如果IP匹配规则则返回 true。
	isBlacklisted func(ip net.IP) bool
}

var hijackBlacklistChecks = []ipCheck{
	{
		reason:        "无效IP地址格式",
		isBlacklisted: func(ip net.IP) bool { return ip == nil },
	},
	{
		reason:        "本地回环地址 (Loopback)", // e.g., 127.0.0.1, ::1
		isBlacklisted: func(ip net.IP) bool { return ip.IsLoopback() },
	},
	{
		reason:        "链路本地地址 (Link-local)", // e.g., 169.254.0.0/16, fe80::/10
		isBlacklisted: func(ip net.IP) bool { return ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() },
	},
	{
		reason:        "组播地址 (Multicast)", // e.g., 224.0.0.0/4, ff00::/8
		isBlacklisted: func(ip net.IP) bool { return ip.IsMulticast() },
	},
	{
		reason:        "未指定地址 (Unspecified)", // 0.0.0.0, ::
		isBlacklisted: func(ip net.IP) bool { return ip.IsUnspecified() },
	},
	{
		reason: "全局广播地址", // 255.255.255.255
		isBlacklisted: func(ip net.IP) bool {
			// net.IPv4bcast 仅对IPv4有意义
			return ip.Equal(net.IPv4bcast)
		},
	},
	{
		reason: "为文档和示例保留的地址 (RFC 5737)", // 192.0.2.0/24, 198.51.100.0/24, 203.0.113.0/24
		isBlacklisted: func(ip net.IP) bool {
			// 定义这些特殊的CIDR块
			cidrs := []string{
				"192.0.2.0/24",    // TEST-NET-1
				"198.51.100.0/24", // TEST-NET-2
				"203.0.113.0/24",  // TEST-NET-3
			}
			for _, cidr := range cidrs {
				_, ipNet, err := net.ParseCIDR(cidr)
				// 这个错误理论上不应发生，因为我们硬编码了合法的CIDR
				if err == nil && ipNet.Contains(ip) {
					return true
				}
			}
			return false
		},
	},
	// 更多规则可以按此格式轻松添加...
}

// IsHijackBlacklisted 检查给定的IP地址字符串是否在劫持黑名单中
// IsHijackBlacklisted 检查给定的IP地址字符串是否在劫持黑名单中。
// 如果IP在黑名单中，返回 (true, "原因")。
// 如果IP是安全的、可公开路由的普通单播地址，返回 (false, "")。
func IsHijackBlacklisted(ipStr string) (bool, string) {
	ip := net.ParseIP(ipStr)
	// ip为nil意味着字符串不是一个合法的IP地址，也应视为黑名单。
	if ip == nil {
		return true, "无效IP地址格式"
	}
	for _, check := range hijackBlacklistChecks {
		if check.isBlacklisted(ip) {
			// 一旦匹配任何一个黑名单规则，立即返回
			return true, check.reason
		}
	}
	// 如果所有检查都通过了，说明这个IP是安全的
	return false, ""
}
