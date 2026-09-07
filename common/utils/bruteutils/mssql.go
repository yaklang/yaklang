package bruteutils

import (
	"context"

	"github.com/yaklang/yaklang/common/brute/core"
	"github.com/yaklang/yaklang/common/brute/probes/mssql"
)

// MSSQLAuth 使用最小 TDS 探针执行认证探测（不再依赖 go-mssqldb 驱动）。
func MSSQLAuth(target, username, password string, needAuth bool) (ok, finished bool, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	if !needAuth {
		username, password = "", ""
	}
	res := probeMSSQL(ctx, target, username, password)
	return res.Outcome == core.OutcomeAuthSuccess, res.Outcome.IsFinalForTarget(), legacyError(res)
}

func probeMSSQL(ctx context.Context, target, username, password string) core.Result {
	ptarget, err := core.ParseTarget(appendDefaultPort(target, 1433))
	if err != nil {
		return core.Result{Outcome: core.OutcomeProtocolMismatch, Err: core.ErrProtocolParse}
	}
	var prober mssql.Prober
	return prober.Probe(ctx, ptarget, core.Credential{Username: username, Password: password},
		core.Options{Timeout: defaultTimeout, TLSPolicy: core.TLSOpportunistic})
}

var mssqlAuth = &DefaultServiceAuthInfo{
	ServiceName:      "mssql",
	DefaultPorts:     "1433",
	DefaultUsernames: []string{"administrator", "admin", "root", "mssql", "manager", "sa"},
	DefaultPasswords: CommonPasswords,
	UnAuthVerify: func(i *BruteItem) *BruteItemResult {
		i.Target = appendDefaultPort(i.Target, 1433)
		res := probeMSSQL(itemCtx(i), i.Target, "", "")
		return legacyFromCore(i, res)
	},
	BrutePass: func(i *BruteItem) *BruteItemResult {
		i.Target = appendDefaultPort(i.Target, 1433)
		res := probeMSSQL(itemCtx(i), i.Target, i.Username, i.Password)
		return legacyFromCore(i, res)
	},
}
