package mssql_test

import (
	"context"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/brute/core"
	"github.com/yaklang/yaklang/common/brute/probes/mssql"
)

// 多版本矩阵（YAK_BRUTE_REAL=1；按可达性自动跳过）。
//
//	docker run -d --platform linux/amd64 --name yak-mssql2022 -p 31433:1433 \
//	  -e 'ACCEPT_EULA=Y' -e 'MSSQL_SA_PASSWORD=MssqlPass123!' mcr.microsoft.com/mssql/server:2022-latest
//	docker run -d --platform linux/amd64 --name yak-mssql2019 -p 32433:1433 \
//	  -e 'ACCEPT_EULA=Y' -e 'MSSQL_SA_PASSWORD=MssqlPass123!' mcr.microsoft.com/mssql/server:2019-latest
var mssqlVersionMatrix = []struct{ name, addr string }{
	{"mssql2022", "127.0.0.1:31433"},
	{"mssql2019", "127.0.0.1:32433"},
}

func TestMSSQLVersionMatrix(t *testing.T) {
	if os.Getenv("YAK_BRUTE_REAL") != "1" {
		t.Skip("set YAK_BRUTE_REAL=1")
	}
	for _, srv := range mssqlVersionMatrix {
		t.Run(srv.name, func(t *testing.T) {
			if !mssqlHostReachable(srv.addr) {
				t.Skipf("%s not reachable", srv.addr)
			}
			probeOnce := func(user, pass string) core.Result {
				target, _ := core.ParseTarget(srv.addr)
				var p mssql.Prober
				return p.Probe(context.Background(), target, core.Credential{Username: user, Password: pass},
					core.Options{Timeout: 20 * time.Second})
			}

			// 就绪等待（amd64 模拟下启动较慢）
			deadline := time.Now().Add(240 * time.Second)
			for {
				res := probeOnce("sa", "MssqlPass123!")
				if res.Outcome == core.OutcomeAuthSuccess || res.Outcome == core.OutcomeAuthFailed {
					break
				}
				if time.Now().After(deadline) {
					t.Fatalf("not ready: %v (%s)", res.Outcome, res.ErrDetail)
				}
				time.Sleep(5 * time.Second)
			}

			if res := probeOnce("sa", "MssqlPass123!"); res.Outcome != core.OutcomeAuthSuccess {
				t.Errorf("correct: want success got %v (%s)", res.Outcome, res.ErrDetail)
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
				t.Errorf("unicode wrong: want auth-failed got %v (%s)", res.Outcome, res.ErrDetail)
			}
			long := strings.Repeat("w", 256)
			if res := probeOnce("sa", long); res.Outcome != core.OutcomeAuthFailed {
				t.Errorf("long password: want auth-failed got %v (%s)", res.Outcome, res.ErrDetail)
			}
		})
	}
}

func mssqlHostReachable(addr string) bool {
	conn, err := net.DialTimeout("tcp", addr, 3*time.Second)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}
