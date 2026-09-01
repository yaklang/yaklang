package mysql_test

import (
	"context"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/brute/core"
	"github.com/yaklang/yaklang/common/brute/probes/mysql"
)

// 多版本真实服务器矩阵（YAK_BRUTE_REAL=1 启用；按可达性自动跳过）。
//
//	docker run -d --rm --name yak-mysql8  -p 33306:3306 -e 'MYSQL_ROOT_PASSWORD=RootPass123!' mysql:8.0
//	docker run -d --rm --platform linux/amd64 --name yak-mysql57 -p 33307:3306 -e 'MYSQL_ROOT_PASSWORD=RootPass123!' mysql:5.7
//	docker run -d --rm --name yak-mariadb -p 33308:3306 -e 'MARIADB_ROOT_PASSWORD=RootPass123!' mariadb:11
//	docker run -d --rm --name yak-mysql84 -p 33406:3306 -e 'MYSQL_ROOT_PASSWORD=RootPass123!' mysql:8.4
//	docker run -d --rm --name yak-mariadb10 -p 33408:3306 -e 'MARIADB_ROOT_PASSWORD=RootPass123!' mariadb:10.11
var mysqlVersionMatrix = []struct {
	name string
	addr string
}{
	{"mysql8", "127.0.0.1:33306"},
	{"mysql57", "127.0.0.1:33307"},
	{"mariadb11", "127.0.0.1:33308"},
	{"mysql84", "127.0.0.1:33406"},
	{"mariadb10", "127.0.0.1:33408"},
}

func TestMySQLVersionMatrix(t *testing.T) {
	if os.Getenv("YAK_BRUTE_REAL") != "1" {
		t.Skip("set YAK_BRUTE_REAL=1 to run against real MySQL servers")
	}
	for _, srv := range mysqlVersionMatrix {
		t.Run(srv.name, func(t *testing.T) {
			if !hostReachable(srv.addr) {
				t.Skipf("%s not reachable (start container first)", srv.addr)
			}
			probeOnce := func(user, pass string) core.Result {
				target, err := core.ParseTarget(srv.addr)
				if err != nil {
					t.Fatal(err)
				}
				var p mysql.Prober
				return p.Probe(context.Background(), target, core.Credential{Username: user, Password: pass},
					core.Options{Timeout: 15 * time.Second})
			}

			// 就绪等待
			deadline := time.Now().Add(120 * time.Second)
			for {
				res := probeOnce("root", "RootPass123!")
				if res.Outcome == core.OutcomeAuthSuccess || res.Outcome == core.OutcomeAuthFailed {
					break
				}
				if time.Now().After(deadline) {
					t.Fatalf("server not ready: %v (%s)", res.Outcome, res.ErrDetail)
				}
				time.Sleep(3 * time.Second)
			}

			// 正反案例
			if res := probeOnce("root", "RootPass123!"); res.Outcome != core.OutcomeAuthSuccess {
				t.Errorf("correct creds: want success got %v (%s)", res.Outcome, res.ErrDetail)
			}
			if res := probeOnce("root", "wrong-password"); res.Outcome != core.OutcomeAuthFailed {
				t.Errorf("wrong password: want auth-failed got %v (%s)", res.Outcome, res.ErrDetail)
			}
			if res := probeOnce("no-such-user-xyz", "whatever"); res.Outcome != core.OutcomeAuthFailed {
				t.Errorf("unknown user: want auth-failed got %v (%s)", res.Outcome, res.ErrDetail)
			}
			if res := probeOnce("root", ""); res.Outcome != core.OutcomeAuthFailed {
				t.Errorf("empty password: want auth-failed got %v (%s)", res.Outcome, res.ErrDetail)
			}
			if res := probeOnce("root", "密码错误🔐"); res.Outcome != core.OutcomeAuthFailed {
				t.Errorf("unicode wrong password: want auth-failed got %v (%s)", res.Outcome, res.ErrDetail)
			}
			// 长密码（<=SHA 输入任意长度均可计算 scramble）
			long := strings.Repeat("x", 256)
			if res := probeOnce("root", long); res.Outcome != core.OutcomeAuthFailed {
				t.Errorf("long password: want auth-failed got %v (%s)", res.Outcome, res.ErrDetail)
			}
			// 传输方式必须被记录
			if res := probeOnce("root", "RootPass123!"); res.Transport != core.TransportPlainTCP && res.Transport != core.TransportTLS {
				t.Errorf("transport not recorded: %q", res.Transport)
			}
		})
	}
}

func hostReachable(addr string) bool {
	conn, err := net.DialTimeout("tcp", addr, 3*time.Second)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}
