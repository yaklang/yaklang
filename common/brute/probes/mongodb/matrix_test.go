package mongodb_test

import (
	"context"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/brute/core"
	"github.com/yaklang/yaklang/common/brute/probes/mongodb"
)

// 多版本矩阵（YAK_BRUTE_REAL=1；按可达性自动跳过）。
//
//	docker run -d --rm --name yak-mongo7  -p 37017:27017 -e 'MONGO_INITDB_ROOT_USERNAME=mongoadmin' -e 'MONGO_INITDB_ROOT_PASSWORD=MongoPass123!' mongo:7
//	docker run -d --rm --name yak-mongo44 -p 37018:27017 -e 'MONGO_INITDB_ROOT_USERNAME=mongoadmin' -e 'MONGO_INITDB_ROOT_PASSWORD=MongoPass123!' mongo:4.4
//	docker run -d --rm --name yak-mongo6  -p 37417:27017 -e 'MONGO_INITDB_ROOT_USERNAME=mongoadmin' -e 'MONGO_INITDB_ROOT_PASSWORD=MongoPass123!' mongo:6.0
//	docker run -d --rm --name yak-mongo5  -p 37418:27017 -e 'MONGO_INITDB_ROOT_USERNAME=mongoadmin' -e 'MONGO_INITDB_ROOT_PASSWORD=MongoPass123!' mongo:5.0
var mongoVersionMatrix = []struct{ name, addr string }{
	{"mongo7", "127.0.0.1:37017"},
	{"mongo44", "127.0.0.1:37018"},
	{"mongo6", "127.0.0.1:37417"},
	{"mongo5", "127.0.0.1:37418"},
}

func TestMongoVersionMatrix(t *testing.T) {
	if os.Getenv("YAK_BRUTE_REAL") != "1" {
		t.Skip("set YAK_BRUTE_REAL=1")
	}
	for _, srv := range mongoVersionMatrix {
		t.Run(srv.name, func(t *testing.T) {
			if !mongoHostReachable(srv.addr) {
				t.Skipf("%s not reachable", srv.addr)
			}
			probeOnce := func(user, pass string) core.Result {
				target, _ := core.ParseTarget(srv.addr)
				var p mongodb.Prober
				return p.Probe(context.Background(), target, core.Credential{Username: user, Password: pass},
					core.Options{Timeout: 15 * time.Second})
			}

			deadline := time.Now().Add(120 * time.Second)
			for {
				res := probeOnce("mongoadmin", "MongoPass123!")
				if res.Outcome == core.OutcomeAuthSuccess || res.Outcome == core.OutcomeAuthFailed {
					break
				}
				if time.Now().After(deadline) {
					t.Fatalf("not ready: %v (%s)", res.Outcome, res.ErrDetail)
				}
				time.Sleep(3 * time.Second)
			}

			if res := probeOnce("mongoadmin", "MongoPass123!"); res.Outcome != core.OutcomeAuthSuccess {
				t.Errorf("correct: want success got %v (%s)", res.Outcome, res.ErrDetail)
			}
			if res := probeOnce("mongoadmin", "wrong-pass"); res.Outcome != core.OutcomeAuthFailed {
				t.Errorf("wrong password: want auth-failed got %v (%s)", res.Outcome, res.ErrDetail)
			}
			if res := probeOnce("ghost-user", "x"); res.Outcome != core.OutcomeAuthFailed {
				t.Errorf("unknown user: want auth-failed got %v (%s)", res.Outcome, res.ErrDetail)
			}
			if res := probeOnce("mongoadmin", ""); res.Outcome != core.OutcomeAuthFailed {
				t.Errorf("empty password: want auth-failed got %v (%s)", res.Outcome, res.ErrDetail)
			}
			if res := probeOnce("mongoadmin", "密码错误🔐"); res.Outcome != core.OutcomeAuthFailed {
				t.Errorf("unicode wrong: want auth-failed got %v (%s)", res.Outcome, res.ErrDetail)
			}
			long := strings.Repeat("z", 256)
			if res := probeOnce("mongoadmin", long); res.Outcome != core.OutcomeAuthFailed {
				t.Errorf("long password: want auth-failed got %v (%s)", res.Outcome, res.ErrDetail)
			}
			// 未授权 ping：认证开启的库必须拒绝
			target, _ := core.ParseTarget(srv.addr)
			up := mongodb.UnauthProber{}
			if ping := up.Probe(context.Background(), target, core.Credential{}, core.Options{Timeout: 15 * time.Second}); ping.Outcome == core.OutcomeAuthSuccess {
				t.Errorf("unauth probe must not succeed on auth-enabled server")
			}
		})
	}
}

func mongoHostReachable(addr string) bool {
	conn, err := net.DialTimeout("tcp", addr, 3*time.Second)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}
