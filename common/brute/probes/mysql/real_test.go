package mysql

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/brute/core"
)

// 真实服务器测试：跳过条件是未设置 YAK_BRUTE_REAL=1。
// 容器：mysql:8.0 (caching_sha2 默认)、mysql:5.7 (native)、mariadb:11。
//
//	docker run -d --rm --name yak-mysql8 -p 33306:3306 -e 'MYSQL_ROOT_PASSWORD=RootPass123!' mysql:8.0
//	docker run -d --rm --platform linux/amd64 --name yak-mysql57 -p 33307:3306 -e 'MYSQL_ROOT_PASSWORD=RootPass123!' mysql:5.7
//	docker run -d --rm --name yak-mariadb -p 33308:3306 -e 'MARIADB_ROOT_PASSWORD=RootPass123!' mariadb:11
func realServersEnabled() bool { return os.Getenv("YAK_BRUTE_REAL") == "1" }

var realServers = []struct {
	name string
	addr string
}{
	{"mysql8", "127.0.0.1:33306"},
	{"mysql57", "127.0.0.1:33307"},
	{"mariadb11", "127.0.0.1:33308"},
}

func TestRealMySQLServers(t *testing.T) {
	if !realServersEnabled() {
		t.Skip("set YAK_BRUTE_REAL=1 to run against real MySQL servers")
	}
	for _, srv := range realServers {
		t.Run(srv.name, func(t *testing.T) {
			probeOnce := func(user, pass string) core.Result {
				target, err := core.ParseTarget(srv.addr)
				if err != nil {
					t.Fatal(err)
				}
				var p Prober
				return p.Probe(waitCtx(t), target, core.Credential{Username: user, Password: pass},
					core.Options{Timeout: 10 * time.Second})
			}

			// 等待服务器就绪
			deadline := time.Now().Add(60 * time.Second)
			for {
				res := probeOnce("root", "RootPass123!")
				if res.Outcome == core.OutcomeAuthSuccess || res.Outcome == core.OutcomeAuthFailed {
					break
				}
				if time.Now().After(deadline) {
					t.Fatalf("server not ready: %v (%s)", res.Outcome, res.ErrDetail)
				}
				time.Sleep(2 * time.Second)
			}

			// 1. 正确凭证
			if res := probeOnce("root", "RootPass123!"); res.Outcome != core.OutcomeAuthSuccess {
				t.Errorf("correct creds: want success got %v (%s)", res.Outcome, res.ErrDetail)
			}

			// 2. 错误密码
			if res := probeOnce("root", "wrong-password"); res.Outcome != core.OutcomeAuthFailed {
				t.Errorf("wrong password: want auth-failed got %v (%s)", res.Outcome, res.ErrDetail)
			}

			// 3. 不存在的用户
			if res := probeOnce("no-such-user-xyz", "whatever"); res.Outcome != core.OutcomeAuthFailed {
				t.Errorf("unknown user: want auth-failed got %v (%s)", res.Outcome, res.ErrDetail)
			}

			// 4. Unicode 密码错误（不应崩溃，应正常分类）
			res := probeOnce("root", "密码错误🔐")
			if res.Outcome != core.OutcomeAuthFailed {
				t.Errorf("unicode wrong password: want auth-failed got %v (%s)", res.Outcome, res.ErrDetail)
			}

			// 5. 空密码错误
			if res := probeOnce("root", ""); res.Outcome != core.OutcomeAuthFailed {
				t.Errorf("empty password: want auth-failed got %v (%s)", res.Outcome, res.ErrDetail)
			}

			// 6. 传输方式必须被记录
			res = probeOnce("root", "RootPass123!")
			if res.Transport != core.TransportPlainTCP && res.Transport != core.TransportTLS {
				t.Errorf("transport not recorded: %q", res.Transport)
			}
			if !strings.Contains(string(res.Extra), "server") {
				t.Errorf("server version not captured: %s", res.Extra)
			}
		})
	}
}

// waitCtx 真实服务器测试用带超时的 ctx。
func waitCtx(t *testing.T) context.Context {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)
	return ctx
}
