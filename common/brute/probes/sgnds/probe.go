package sgnds

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/brute/core"
)

// Prober 通过本地 yakit-sg-jdbc-bridge 进程探测 SG-JDBC 服务。
// 目标地址中的 Host/Port 被忽略，真正的隔离装置信息通过 JDBC URL 传入 bridge。
type Prober struct {
	// BridgeAddr 是本地代理监听地址，默认 127.0.0.1:19090。
	BridgeAddr string
	// JdbcURL 模板；支持占位符 {HOST} {PORT} {DB} {APP}，由 Probe 时动态填充。
	// 示例：jdbc:nds://{HOST}:18600/v_{DB}?appname={APP}
	JdbcURL string
	// AppName 是隔离装置要求的应用系统名称。
	AppName string
}


// Probe 执行一次 SG-JDBC 认证探测。
// 如果未指定 BridgeAddr，会自动从 Go 二进制内嵌的 jar 启动本地代理。
func (p Prober) Probe(ctx context.Context, target core.Target, cred core.Credential, opts core.Options) core.Result {
	addr := p.BridgeAddr
	if addr == "" {
		var err error
		addr, err = EnsureBridgeRunning(ctx)
		if err != nil {
			return core.Result{
				Outcome:   core.OutcomeTargetUnavailable,
				Transport: core.TransportPlainTCP,
				Err:       core.ErrDial,
				ErrDetail: sanitize(err),
			}
		}
	}

	jdbcURL := p.buildURL(target)
	client := NewBridgeClient(addr, timeoutOf(opts))

	connID, err := client.Open(ctx, jdbcURL, cred.Username, cred.Password)
	if err != nil {
		return classifyError(ctx, err, target, jdbcURL)
	}
	defer client.Close(context.Background(), connID)

	// 连上了即视为认证成功；再用 pingConnection 确认连接有效。
	res, err := client.Query(ctx, connID, "SELECT 1 FROM DUAL", nil)
	if err != nil {
		// 认证过了但 SQL 执行失败：可能是隔离装置规则拒绝或目标不可用。
		return classifyError(ctx, err, target, jdbcURL)
	}
	_ = res

	return core.Result{
		Outcome:   core.OutcomeAuthSuccess,
		Transport: core.TransportPlainTCP,
		Extra:     []byte("sg-jdbc bridge " + addr),
	}
}

func (p Prober) buildURL(target core.Target) string {
	url := p.JdbcURL
	if url == "" {
		url = "jdbc:nds://{HOST}:18600/v_{DB}?appname={APP}"
	}
	host := target.Host
	port := target.Port
	if port <= 0 {
		port = 18600
	}
	// 默认虚拟数据库名取目标 host，应用名取默认值。
	url = strings.ReplaceAll(url, "{HOST}", net.JoinHostPort(host, strconv.Itoa(port)))
	url = strings.ReplaceAll(url, "{PORT}", strconv.Itoa(port))
	url = strings.ReplaceAll(url, "{DB}", safeDBName(host))
	if p.AppName != "" {
		url = strings.ReplaceAll(url, "{APP}", p.AppName)
	} else {
		url = strings.ReplaceAll(url, "{APP}", "yakit")
	}
	return url
}

func safeDBName(host string) string {
	// 虚拟数据库名只允许字母数字下划线。
	s := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '.' || r == ':' {
			return '_'
		}
		return r
	}, host)
	if s == "" {
		s = "yakit_db"
	}
	return s
}

func timeoutOf(opts core.Options) time.Duration {
	if opts.Timeout > 0 {
		return opts.Timeout
	}
	return core.DefaultTimeout
}

func classifyError(ctx context.Context, err error, target core.Target, jdbcURL string) core.Result {
	if ctx.Err() != nil {
		return core.Result{Outcome: core.OutcomeCancelled, Transport: core.TransportPlainTCP, Err: core.ErrCancelled}
	}
	msg := strings.ToLower(err.Error())
	res := core.Result{Transport: core.TransportPlainTCP}

	switch {
	// 认证拒绝
	case strings.Contains(msg, "invalid username or password"),
		strings.Contains(msg, "access denied"),
		strings.Contains(msg, "denyed rule_id=1763"):
		res.Outcome = core.OutcomeAuthFailed
		res.Err = core.ErrAuthRejected
	// 目标不可达 / 网络层失败
	case strings.Contains(msg, "connection refused"),
		strings.Contains(msg, "actively refused"),
		strings.Contains(msg, "try time out"),
		strings.Contains(msg, "timeout"),
		strings.Contains(msg, "broken pipe"),
		strings.Contains(msg, "no route"),
		strings.Contains(msg, "unreachable"):
		res.Outcome = core.OutcomeTargetUnavailable
		res.Err = core.ErrDial
	// URL/应用名错误
	case strings.Contains(msg, "ip address or application name is invalid"):
		res.Outcome = core.OutcomeProtocolMismatch
		res.Err = core.ErrProtocolParse
	default:
		res.Outcome = core.OutcomeUnknown
		res.Err = core.ErrIO
	}

	res.ErrDetail = sanitize(err)
	res.Extra = []byte(fmt.Sprintf("url=%s", jdbcURL))
	return res
}

func sanitize(err error) string {
	msg := err.Error()
	if len(msg) > 200 {
		msg = msg[:200]
	}
	return msg
}
