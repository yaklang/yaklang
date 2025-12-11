package netstackvm

import "testing"

func TestSystemRouteManagerBlackList(t *testing.T) {
	testCases := []struct {
		ip             string
		expected       bool
		expectedReason string
	}{
		// 黑名单IP测试
		{"127.0.0.1", true, "本地回环地址 (Loopback)"},
		{"::1", true, "本地回环地址 (Loopback)"},
		{"169.254.100.200", true, "链路本地地址 (Link-local)"},
		{"fe80::1", true, "链路本地地址 (Link-local)"},
		{"0.0.0.0", true, "未指定地址 (Unspecified)"},
		{"255.255.255.255", true, "全局广播地址"},
		{"192.0.2.123", true, "为文档和示例保留的地址 (RFC 5737)"},
		{"not-an-ip", true, "无效IP地址格式"},

		// 安全IP测试
		{"8.8.8.8", false, ""},
		{"1.1.1.1", false, ""},
		{"208.67.222.222", false, ""},
		{"2606:4700:4700::1111", false, ""},
	}

	for _, tc := range testCases {
		t.Run(tc.ip, func(t *testing.T) {
			isBlacklisted, reason := IsHijackBlacklisted(tc.ip)
			if isBlacklisted != tc.expected || reason != tc.expectedReason {
				t.Errorf("IP: %s, expected: (%v, %s), got: (%v, %s)", tc.ip, tc.expected, tc.expectedReason, isBlacklisted, reason)
			}
		})
	}
}
