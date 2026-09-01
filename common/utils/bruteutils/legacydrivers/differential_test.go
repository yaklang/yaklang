package legacydrivers_test

import (
	"context"
	"net"
	"os"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/utils/bruteutils"
	"github.com/yaklang/yaklang/common/utils/bruteutils/legacydrivers"
)

// TestDifferentialOldVsNewProbes 在真实服务上对照旧驱动实现与新最小探针：
//   - 正确凭证判定一致（都成功）
//   - 密码错误判定一致（都失败）
//   - 用户不存在判定一致
//   - 空密码判定一致
//
// 差分全部通过是删除旧驱动与 go.mod 依赖的前置条件（任务验收标准）。
func TestDifferentialOldVsNewProbes(t *testing.T) {
	if os.Getenv("YAK_BRUTE_REAL") != "1" {
		t.Skip("set YAK_BRUTE_REAL=1 to run differential tests against real servers")
	}

	type server struct {
		name  string
		addr  string
		proto string
		user  string
		pass  string
	}
	servers := []server{
		{"mysql8", "127.0.0.1:33306", "mysql", "root", "RootPass123!"},
		{"mysql57", "127.0.0.1:33307", "mysql", "root", "RootPass123!"},
		{"mariadb", "127.0.0.1:33308", "mysql", "root", "RootPass123!"},
		{"pg16", "127.0.0.1:35432", "postgres", "pgadmin", "PgPass123!"},
		{"pg12", "127.0.0.1:35433", "postgres", "pgadmin", "PgPass123!"},
		{"mongo7", "127.0.0.1:37017", "mongodb", "mongoadmin", "MongoPass123!"},
		{"mongo44", "127.0.0.1:37018", "mongodb", "mongoadmin", "MongoPass123!"},
		{"mssql2022", "127.0.0.1:31433", "mssql", "sa", "MssqlPass123!"},
	}

	cases := []struct {
		name string
		user string
		pass string
	}{
		{"correct", "", ""}, // 使用服务器正确凭证（在循环中替换）
		{"wrong-password", "", "definitely-wrong-pass"},
		{"unknown-user", "no-such-user-q1", "whatever"},
		{"empty-password", "", ""},
	}

	for _, srv := range servers {
		t.Run(srv.name, func(t *testing.T) {
			// 可达性探测：容器未启动时跳过（差分依赖真实服务）
			if !hostReachable(srv.addr) {
				t.Skipf("server %s at %s not reachable (start the container first)", srv.name, srv.addr)
			}

			// 新实现：bruteutils（已切换到最小探针）
			newProbe := func(user, pass string) (ok bool) {
				switch srv.proto {
				case "mysql":
					ok, _, _ := bruteutils.MYSQLAuth(srv.addr, user, pass, true)
					return ok
				case "mssql":
					ok, _, _ := bruteutils.MSSQLAuth(srv.addr, user, pass, true)
					return ok
				case "mongodb":
					ok, _ := bruteutils.MongoDBAuth(srv.addr, user, pass, true)
					return ok
				case "postgres":
					item := &bruteutils.BruteItem{Target: srv.addr, Username: user, Password: pass, Context: context.Background()}
					handler := postgresHandler()
					res := handler(item)
					return res.Ok
				}
				return false
			}
			// 旧实现：legacydrivers
			oldProbe := func(user, pass string) bool {
				ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
				defer cancel()
				_ = ctx
				switch srv.proto {
				case "mysql":
					ok, _, _ := legacydrivers.MYSQLAuthLegacy(srv.addr, user, pass, true)
					return ok
				case "mssql":
					ok, _, _ := legacydrivers.MSSQLAuthLegacy(srv.addr, user, pass, true)
					return ok
				case "mongodb":
					ok, _ := legacydrivers.MongoDBAuthLegacy(ctx, srv.addr, user, pass, true)
					return ok
				case "postgres":
					ok, _, _ := legacydrivers.PostgresAuthLegacy(srv.addr, user, pass)
					return ok
				}
				return false
			}

			for _, tc := range cases {
				user, pass := tc.user, tc.pass
				if tc.name == "correct" {
					user, pass = srv.user, srv.pass
				}
				t.Run(tc.name, func(t *testing.T) {
					newOK := newProbe(user, pass)
					oldOK := oldProbe(user, pass)
					if newOK != oldOK {
						t.Errorf("differential mismatch: old=%v new=%v (user=%q pass-hidden)", oldOK, newOK, user)
					}
					if tc.name == "correct" && !newOK {
						t.Errorf("both implementations failed on correct credentials (server may be down?)")
					}
				})
			}
		})
	}
}

// hostReachable 探测目标端口是否可连接。
func hostReachable(addr string) bool {
	conn, err := net.DialTimeout("tcp", addr, 3*time.Second)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

// postgresHandler 返回 postgres 的 BrutePass 处理器。
func postgresHandler() func(*bruteutils.BruteItem) *bruteutils.BruteItemResult {
	handler, err := bruteutils.GetBruteFuncByType("postgres")
	if err != nil {
		panic(err)
	}
	return handler
}
