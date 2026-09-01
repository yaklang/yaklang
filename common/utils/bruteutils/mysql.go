package bruteutils

import (
	"context"

	"github.com/yaklang/yaklang/common/brute/core"
	"github.com/yaklang/yaklang/common/brute/probes/mysql"
	"github.com/yaklang/yaklang/common/log"
)

// MYSQLAuth 使用最小 MySQL 探针执行认证探测。
// 旧实现依赖 go-sql-driver/mysql 完整驱动；现在只做协议级认证握手，
// 数据库驱动不再进入爆破依赖闭包（旧驱动已删除，真实服务器上的
// 差分验证结论记录于 common/brute/README.md）。
func MYSQLAuth(target, username, password string, needAuth bool) (ok, finished bool, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	if !needAuth {
		username, password = "", ""
	}
	res := probeMySQL(ctx, target, username, password)
	return res.Outcome == core.OutcomeAuthSuccess, res.Outcome.IsFinalForTarget(), legacyError(res)
}

func probeMySQL(ctx context.Context, target, username, password string) core.Result {
	ptarget, err := core.ParseTarget(appendDefaultPort(target, 3306))
	if err != nil {
		return core.Result{Outcome: core.OutcomeProtocolMismatch, Err: core.ErrProtocolParse}
	}
	var prober mysql.Prober
	return prober.Probe(ctx, ptarget, core.Credential{Username: username, Password: password},
		core.Options{Timeout: defaultTimeout, TLSPolicy: core.TLSOpportunistic})
}

var mysqlAuth = &DefaultServiceAuthInfo{
	ServiceName:      "mysql",
	DefaultPorts:     "3306",
	DefaultUsernames: []string{"mysql", "root", "guest", "op", "ops"},
	DefaultPasswords: append(CommonPasswords, ""),
	UnAuthVerify: func(i *BruteItem) *BruteItemResult {
		i.Target = appendDefaultPort(i.Target, 3306)
		res := probeMySQL(itemCtx(i), i.Target, "", "")
		return legacyFromCore(i, res)
	},
	BrutePass: func(i *BruteItem) *BruteItemResult {
		i.Target = appendDefaultPort(i.Target, 3306)
		res := probeMySQL(itemCtx(i), i.Target, i.Username, i.Password)
		return legacyFromCore(i, res)
	},
}

// legacyError 把结构化结果转成旧签名错误（nil 表示无错误）。
func legacyError(res core.Result) error {
	if res.Outcome == core.OutcomeAuthSuccess || res.ErrDetail == "" {
		return nil
	}
	return &probeResultError{res: res}
}

// probeResultError 承载结构化结果的旧式错误（Error 文本脱敏）。
type probeResultError struct{ res core.Result }

func (e *probeResultError) Error() string {
	msg := e.res.ErrDetail
	if msg == "" {
		return e.res.Outcome.String()
	}
	return msg
}

// legacyFromCore 把核心结果转旧 BruteItemResult。
func legacyFromCore(i *BruteItem, res core.Result) *BruteItemResult {
	out := i.Result()
	out.Ok = res.Outcome == core.OutcomeAuthSuccess
	out.Finished = res.Outcome.IsFinalForTarget()
	out.UserEliminated = res.UserEliminated
	out.ExtraInfo = res.Extra
	if res.Outcome == core.OutcomeAuthSuccess && res.Extra != nil {
		log.Debugf("mysql auth success: %s", string(res.Extra))
	}
	return out
}
