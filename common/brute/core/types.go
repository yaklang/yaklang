// Package core 提供与具体协议彻底解耦的爆破调度核心：
// 结构化结果模型、凭证脱敏、惰性组合迭代器和流式有界调度器。
//
// 本包禁止导入任何数据库驱动或具体协议实现包（这是构建精简版 yak 的前提，
// 由 TestNoDriverImports 在 CI 中强制保证）。
package core

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Target 表示一个待探测的服务端点。
type Target struct {
	// Host 为主机名或 IP（不含端口）。
	Host string
	// Port 为端口；0 表示未知，由协议层补默认端口。
	Port int
	// Raw 为用户输入的原始目标串。
	Raw string
}

// String 返回 host:port 形式的目标标识。
func (t Target) String() string {
	if t.Port <= 0 {
		return t.Host
	}
	return net_JoinHostPort(t.Host, t.Port)
}

// net_JoinHostPort 避免为 JoinHostPort 引入 net 包的连带符号（net 包本身允许导入，
// 这里只是保持最小依赖面）。IPv6 会加方括号。
func net_JoinHostPort(host string, port int) string {
	if strings.Contains(host, ":") && !strings.HasPrefix(host, "[") {
		return "[" + host + "]:" + strconv.Itoa(port)
	}
	return host + ":" + strconv.Itoa(port)
}

// Credential 表示一次认证尝试使用的凭证。
// 凭证的字符串表示永远不包含明文密码。
type Credential struct {
	Username string
	Password string
	// Index 是调度器赋予的组合序号，用于结果关联，替代记录明文。
	Index int64
}

// ID 返回凭证的不可逆摘要（用户名明文 + 密码摘要），用于审计日志与结果关联。
// 用户名不属于高敏信息，保留明文以便定位；密码只保留 8 字节摘要。
func (c Credential) ID() string {
	sum := sha256.Sum256([]byte(c.Password))
	return c.Username + ":" + hex.EncodeToString(sum[:8])
}

// String 返回脱敏后的凭证表示，永不含明文密码。
func (c Credential) String() string {
	return fmt.Sprintf("%s:%s", c.Username, redactPassword(c.Password))
}

// redactPassword 把密码替换为固定长度掩码；空密码与短密码同样只暴露长度特征的最小信息。
func redactPassword(password string) string {
	if password == "" {
		return "<empty>"
	}
	sum := sha256.Sum256([]byte(password))
	return "<sha256:" + hex.EncodeToString(sum[:4]) + ">"
}

// TLSPolicy 显式声明传输层行为，杜绝静默降级。
// 注意：本模块默认跳过证书校验（InsecureSkipVerify），证书有效性不是关注点；
// 策略只控制"是否允许在非加密信道上发送凭证"。
type TLSPolicy int

const (
	// TLSOpportunistic 先尝试 TLS，失败后回退明文并如实记录传输方式。
	// 这是旧行为的兼容默认值，回退会被记录，不再是静默降级。
	TLSOpportunistic TLSPolicy = iota
	// TLSStrict 要求 TLS：握手失败即终止，绝不发送明文凭证。
	TLSStrict
	// PlaintextAllowed 直接使用明文 TCP。
	PlaintextAllowed
)

// String 实现 fmt.Stringer。
func (p TLSPolicy) String() string {
	switch p {
	case TLSStrict:
		return "tls-strict"
	case PlaintextAllowed:
		return "plaintext"
	default:
		return "tls-opportunistic"
	}
}

// Transport 记录本次认证实际使用的传输方式。
type Transport string

const (
	TransportPlainTCP Transport = "tcp"
	TransportTLS      Transport = "tls"
	TransportUnknown  Transport = "unknown"
)

// Outcome 是结构化认证结果枚举，替代布尔字段 + 错误字符串判断。
type Outcome int

const (
	// OutcomeAuthSuccess 凭证正确。
	OutcomeAuthSuccess Outcome = iota
	// OutcomeAuthFailed 凭证错误（密码错误/用户不存在等标准认证拒绝）。
	OutcomeAuthFailed
	// OutcomeTargetUnavailable 目标不可达：拒绝连接、网络不可达、超时、服务端持续断开。
	OutcomeTargetUnavailable
	// OutcomeProtocolMismatch 端口上运行的不是预期协议（畸形 banner、协议解析失败）。
	OutcomeProtocolMismatch
	// OutcomeAccountLocked 服务端报告账户锁定。
	OutcomeAccountLocked
	// OutcomeRateLimited 服务端限流（含 429/Retry-After 场景）。
	OutcomeRateLimited
	// OutcomeMFARequired 凭证有效但需要第二步认证。
	OutcomeMFARequired
	// OutcomeTLSRequired 服务端要求 TLS 而策略禁止明文回退。
	OutcomeTLSRequired
	// OutcomeUnsafeDowngradeBlocked TLS 失败且策略禁止向该目标发送明文凭证。
	OutcomeUnsafeDowngradeBlocked
	// OutcomeCancelled 调用方取消。
	OutcomeCancelled
	// OutcomeUnknown 无法分类的失败，需要保留原始错误类别。
	OutcomeUnknown
)

// String 实现 fmt.Stringer。
func (o Outcome) String() string {
	switch o {
	case OutcomeAuthSuccess:
		return "auth-success"
	case OutcomeAuthFailed:
		return "auth-failed"
	case OutcomeTargetUnavailable:
		return "target-unavailable"
	case OutcomeProtocolMismatch:
		return "protocol-mismatch"
	case OutcomeAccountLocked:
		return "account-locked"
	case OutcomeRateLimited:
		return "rate-limited"
	case OutcomeMFARequired:
		return "mfa-required"
	case OutcomeTLSRequired:
		return "tls-required"
	case OutcomeUnsafeDowngradeBlocked:
		return "unsafe-downgrade-blocked"
	case OutcomeCancelled:
		return "cancelled"
	default:
		return "unknown"
	}
}

// IsFinalForTarget 表示该结果意味着"这个目标没必要再试了"，
// 调度器据此提前终止该目标的全部剩余组合。
func (o Outcome) IsFinalForTarget() bool {
	switch o {
	case OutcomeTargetUnavailable, OutcomeProtocolMismatch, OutcomeTLSRequired:
		return true
	default:
		return false
	}
}

// ErrCategory 结构化错误类别，用于结果聚合与统计。
type ErrCategory string

const (
	ErrNone             ErrCategory = ""
	ErrDial             ErrCategory = "dial"
	ErrHandshake        ErrCategory = "handshake"
	ErrAuthRejected     ErrCategory = "auth-rejected"
	ErrIO               ErrCategory = "io"
	ErrDeadline         ErrCategory = "deadline"
	ErrCancelled        ErrCategory = "cancelled"
	ErrProtocolParse    ErrCategory = "protocol-parse"
	ErrTLSPolicyBlocked ErrCategory = "tls-policy-blocked"
	ErrPanic            ErrCategory = "probe-panic"
)

// Result 是一次认证探测的结构化结果。
// 除了 Username（用于定位）外不含明文凭证。
type Result struct {
	Outcome   Outcome
	Protocol  string
	TargetID  string
	CredID    string // Credential.ID() 的快照
	Transport Transport
	Attempts  int
	Elapsed   time.Duration
	Err       ErrCategory
	// ErrDetail 是脱敏后的错误描述（不含密码），用于调试。
	ErrDetail string
	// RetryAfter 服务端明示的退避时长（429/Retry-After 或等价信号），0 表示无。
	RetryAfter time.Duration
	// UserEliminated 表示该用户名在该目标上已无意义（例如 Oracle 的
	// "用户不存在"信号），调度器将跳过该用户名的剩余组合。
	UserEliminated bool
	// OnlyNeedPassword 表示该服务只验密码（如部分 Telnet/网络设备），
	// 调度器据此跳过重复密码。
	OnlyNeedPassword bool
	// Extra 携带 banner 等非敏感附加信息。
	Extra []byte
	// RawCredentialIndex 回填组合序号，便于外部把结果与输入字典对齐。
	RawCredentialIndex int64
}

// Ok 返回是否认证成功。
func (r *Result) Ok() bool { return r.Outcome == OutcomeAuthSuccess }

// String 返回脱敏的结果摘要。
func (r *Result) String() string {
	transport := ""
	if r.Transport != "" {
		transport = " via " + string(r.Transport)
	}
	if r.ErrDetail != "" {
		return fmt.Sprintf("[%s] %s://%s%s cred#%d (%s): %s",
			r.Outcome, r.Protocol, r.TargetID, transport, r.RawCredentialIndex, r.CredID, r.ErrDetail)
	}
	return fmt.Sprintf("[%s] %s://%s%s cred#%d (%s)",
		r.Outcome, r.Protocol, r.TargetID, transport, r.RawCredentialIndex, r.CredID)
}

// Options 是探测选项，由调度器下发（不由调用方每次手工构造）。
type Options struct {
	Timeout   time.Duration
	TLSPolicy TLSPolicy
}

// Prober 是统一的协议探测接口。
// 实现方必须：
//  1. 使用传入 ctx 控制全部网络 I/O（拨号、读、写），取消时立即关闭连接；
//  2. 对每次调用自带 panic 恢复或保证不 panic（调度器仍有兜底）；
//  3. 绝不把明文密码写入 error、日志或 Result.ErrDetail。
type Prober interface {
	Probe(ctx context.Context, target Target, cred Credential, opts Options) Result
}

// ProberFunc 适配函数签名。
type ProberFunc func(ctx context.Context, target Target, cred Credential, opts Options) Result

// Probe 实现 Prober。
func (f ProberFunc) Probe(ctx context.Context, target Target, cred Credential, opts Options) Result {
	return f(ctx, target, cred, opts)
}
