package trafficguard

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// Finding 是一次超级正则组命中的结构化结果。
type Finding struct {
	// RuleID 命中的规则编号(对应 builtinRules 的 ID)。
	RuleID int
	// RuleName / Category / Severity / Description / Solution 来自规则元信息。
	RuleName    string
	Category    string
	Severity    string
	Description string
	Solution    string

	// Direction 命中所在方向: "request" / "response"。
	Direction string
	// Surface 命中所在表面: "header" / "body"。
	Surface string

	// From/To 命中在所属扫描缓冲区(请求或响应)中的字节偏移。
	From int
	To   int

	// RawValue 命中的原始明文(仅用于生成脱敏展示与指纹,不会被写入普通 Risk 日志)。
	RawValue []byte
	// MaskedValue 脱敏后的展示值。
	MaskedValue string
	// Fingerprint 命中值的 SHA-256 指纹(稳定去重用),不含明文。
	Fingerprint string
	// ValueLength 命中明文长度。
	ValueLength int
}

// redact 按规则配置对命中明文脱敏: 普通凭证保留首尾少量字符,私钥等只给长度与指纹。
// raw 为命中明文, head/tail 为保留的首尾明文长度。
func redact(raw string, head, tail int) string {
	n := len(raw)
	if n == 0 {
		return ""
	}
	if head <= 0 && tail <= 0 {
		// 私钥等不保留任何明文片段。
		return fmt.Sprintf("REDACTED(len=%d)", n)
	}
	if head+tail >= n {
		// 明文过短,仅给长度,避免泄露全部。
		return fmt.Sprintf("REDACTED(len=%d)", n)
	}
	return raw[:head] + "…" + fmt.Sprintf("[len=%d]", n-head-tail) + "…" + raw[n-tail:]
}

// fingerprint 计算明文的 SHA-256 指纹(十六进制),用于稳定去重,不含明文。
func fingerprint(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// severityVerbose 把英文等级映射为中文,写入 Risk 标题,让用户一眼感知危险度。
func severityVerbose(sev string) string {
	switch sev {
	case severityCritical:
		return "严重"
	case severityHigh:
		return "高危"
	case severityMedium:
		return "中危"
	default:
		return "低危"
	}
}
