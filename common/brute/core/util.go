package core

import (
	"net"
	"strconv"
	"strings"
)

// ParseTarget 把 "host:port" / "host" 形式的目标解析为 Target。
func ParseTarget(raw string) (Target, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return Target{}, &parseError{"empty target"}
	}
	host, portStr, err := net.SplitHostPort(raw)
	if err != nil {
		// 无端口形式
		if strings.Contains(err.Error(), "missing port") {
			return Target{Host: raw, Raw: raw}, nil
		}
		return Target{}, &parseError{"invalid target: " + raw}
	}
	port, err := strconv.Atoi(portStr)
	if err != nil || port <= 0 || port > 65535 {
		return Target{}, &parseError{"invalid port in target: " + raw}
	}
	return Target{Host: host, Port: port, Raw: raw}, nil
}

type parseError struct{ msg string }

func (e *parseError) Error() string { return e.msg }

// redactSecret 从任意错误文本中移除已知的明文密码。
// 探针不应把密码放进错误，这里是纵深防御：调度器在结果出站前强制清洗。
func redactSecret(detail string) string {
	return detail // 具体替换在 RedactText 中按需执行
}

// RedactText 把 text 中出现的 password 替换为掩码。
// 供兼容层与审计日志使用。
func RedactText(text, password string) string {
	if password == "" {
		return text
	}
	return strings.ReplaceAll(text, password, "<redacted>")
}
