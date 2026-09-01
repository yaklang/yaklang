package mssql

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/brute/core"
)

// 真实服务器测试（YAK_BRUTE_REAL=1）：
//
//	docker run -d --rm --platform linux/amd64 --name yak-mssql -p 31433:1433 \
//	  -e 'ACCEPT_EULA=Y' -e 'MSSQL_SA_PASSWORD=MssqlPass123!' mcr.microsoft.com/mssql/server:2022-latest
//
// SQL Server 2022 默认强制加密（encryptReq）→ 验证 TDS 内嵌 TLS 升级路径。
func TestRealMSSQLServers(t *testing.T) {
	if os.Getenv("YAK_BRUTE_REAL") != "1" {
		t.Skip("set YAK_BRUTE_REAL=1 to run against real MSSQL servers")
	}
	for _, srv := range []struct{ name, addr string }{
		{"mssql2022", "127.0.0.1:31433"},
	} {
		t.Run(srv.name, func(t *testing.T) {
			probeOnce := func(user, pass string) core.Result {
				target, err := core.ParseTarget(srv.addr)
				if err != nil {
					t.Fatal(err)
				}
				var p Prober
				return p.Probe(context.Background(), target, core.Credential{Username: user, Password: pass},
					core.Options{Timeout: 15 * time.Second})
			}

			// 就绪等待（amd64 模拟下启动较慢）
			deadline := time.Now().Add(180 * time.Second)
			for {
				res := probeOnce("sa", "MssqlPass123!")
				if res.Outcome == core.OutcomeAuthSuccess || res.Outcome == core.OutcomeAuthFailed {
					break
				}
				if time.Now().After(deadline) {
					t.Fatalf("server not ready: %v (%s)", res.Outcome, res.ErrDetail)
				}
				time.Sleep(5 * time.Second)
			}

			correct := probeOnce("sa", "MssqlPass123!")
			if correct.Outcome != core.OutcomeAuthSuccess {
				t.Errorf("correct creds: want success got %v (%s)", correct.Outcome, correct.ErrDetail)
			}
			// 传输方式跟随服务器 PRELOGIN 声明：要求加密则 TLS，不支持则明文。
			if correct.Transport != core.TransportTLS && correct.Transport != core.TransportPlainTCP {
				t.Errorf("transport not recorded: %v", correct.Transport)
			}
			if res := probeOnce("sa", "wrong-password"); res.Outcome != core.OutcomeAuthFailed {
				t.Errorf("wrong password: want auth-failed got %v (%s)", res.Outcome, res.ErrDetail)
			}
			if res := probeOnce("ghost-user", "x"); res.Outcome != core.OutcomeAuthFailed {
				t.Errorf("unknown user: want auth-failed got %v (%s)", res.Outcome, res.ErrDetail)
			}
			if res := probeOnce("sa", ""); res.Outcome != core.OutcomeAuthFailed {
				t.Errorf("empty password: want auth-failed got %v (%s)", res.Outcome, res.ErrDetail)
			}
			if res := probeOnce("sa", "密码错误🔐"); res.Outcome != core.OutcomeAuthFailed {
				t.Errorf("unicode wrong password: want auth-failed got %v (%s)", res.Outcome, res.ErrDetail)
			}
		})
	}
}
