package bruteutils

import (
	"context"

	"github.com/yaklang/yaklang/common/brute/core"
	"github.com/yaklang/yaklang/common/brute/probes/postgres"
)

// postgresAuth 使用最小 PG 探针（cleartext/MD5/SCRAM-SHA-256），不再依赖 go-pg。
var postgresAuth = &DefaultServiceAuthInfo{
	ServiceName:      "postgres",
	DefaultPorts:     "5432",
	DefaultUsernames: append([]string{"postgres"}, CommonUsernames...),
	DefaultPasswords: CommonPasswords,
	UnAuthVerify: func(i *BruteItem) *BruteItemResult {
		i.Target = appendDefaultPort(i.Target, 5432)
		res := probePostgres(itemCtx(i), i.Target, "postgres", "")
		// 旧语义：能建立协议连接（收到认证质询）即视为目标可达
		out := legacyFromCore(i, res)
		out.Finished = false
		return out
	},
	BrutePass: func(i *BruteItem) *BruteItemResult {
		i.Target = appendDefaultPort(i.Target, 5432)
		res := probePostgres(itemCtx(i), i.Target, i.Username, i.Password)
		return legacyFromCore(i, res)
	},
}

func probePostgres(ctx context.Context, target, username, password string) core.Result {
	ptarget, err := core.ParseTarget(appendDefaultPort(target, 5432))
	if err != nil {
		return core.Result{Outcome: core.OutcomeProtocolMismatch, Err: core.ErrProtocolParse}
	}
	var prober postgres.Prober
	return prober.Probe(ctx, ptarget, core.Credential{Username: username, Password: password},
		core.Options{Timeout: defaultTimeout, TLSPolicy: core.TLSOpportunistic})
}
