package bruteutils

import (
	"context"

	"github.com/yaklang/yaklang/common/brute/core"
	"github.com/yaklang/yaklang/common/brute/probes/mongodb"
)

// MongoDBAuth 使用最小 OP_MSG+SCRAM 探针执行认证探测（不再依赖 mongo-driver）。
func MongoDBAuth(target, username, password string, needAuth bool) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	if !needAuth {
		username, password = "", ""
	}
	res := probeMongo(ctx, target, username, password)
	return res.Outcome == core.OutcomeAuthSuccess, legacyError(res)
}

func probeMongo(ctx context.Context, target, username, password string) core.Result {
	ptarget, err := core.ParseTarget(appendDefaultPort(target, 27017))
	if err != nil {
		return core.Result{Outcome: core.OutcomeProtocolMismatch, Err: core.ErrProtocolParse}
	}
	var prober mongodb.Prober
	return prober.Probe(ctx, ptarget, core.Credential{Username: username, Password: password},
		core.Options{Timeout: defaultTimeout, TLSPolicy: core.TLSOpportunistic})
}

// probeMongoUnauth 用需要权限的命令探测真实未授权访问。
// 注意：旧实现的 Ping 在开启认证的 MongoDB 上也会成功（ping 允许匿名），
// 会把所有可达实例误报为未授权；新实现使用 listDatabases 判定。
func probeMongoUnauth(ctx context.Context, target string) core.Result {
	ptarget, err := core.ParseTarget(appendDefaultPort(target, 27017))
	if err != nil {
		return core.Result{Outcome: core.OutcomeProtocolMismatch, Err: core.ErrProtocolParse}
	}
	var prober mongodb.UnauthProber
	return prober.Probe(ctx, ptarget, core.Credential{}, core.Options{Timeout: defaultTimeout, TLSPolicy: core.TLSOpportunistic})
}

var mongoAuth = &DefaultServiceAuthInfo{
	ServiceName:      "mongodb",
	DefaultPorts:     "27017",
	DefaultUsernames: append([]string{"root", "admin", "mongodb"}, CommonUsernames...),
	DefaultPasswords: CommonPasswords,
	UnAuthVerify: func(i *BruteItem) *BruteItemResult {
		i.Target = appendDefaultPort(i.Target, 27017)
		res := probeMongoUnauth(itemCtx(i), i.Target)
		return legacyFromCore(i, res)
	},
	BrutePass: func(i *BruteItem) *BruteItemResult {
		i.Target = appendDefaultPort(i.Target, 27017)
		res := probeMongo(itemCtx(i), i.Target, i.Username, i.Password)
		return legacyFromCore(i, res)
	},
}
