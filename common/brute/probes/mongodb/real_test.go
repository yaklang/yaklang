package mongodb

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/brute/core"
)

// 真实服务器测试（YAK_BRUTE_REAL=1）：
//
//	docker run -d --rm --name yak-mongo7 -p 37017:27017 \
//	  -e 'MONGO_INITDB_ROOT_USERNAME=mongoadmin' -e 'MONGO_INITDB_ROOT_PASSWORD=MongoPass123!' mongo:7
//	docker run -d --rm --name yak-mongo44 -p 37018:27017 \
//	  -e 'MONGO_INITDB_ROOT_USERNAME=mongoadmin' -e 'MONGO_INITDB_ROOT_PASSWORD=MongoPass123!' mongo:4.4
func TestRealMongoServers(t *testing.T) {
	if os.Getenv("YAK_BRUTE_REAL") != "1" {
		t.Skip("set YAK_BRUTE_REAL=1 to run against real MongoDB servers")
	}
	for _, srv := range []struct{ name, addr string }{
		{"mongo7", "127.0.0.1:37017"},
		{"mongo44", "127.0.0.1:37018"},
	} {
		t.Run(srv.name, func(t *testing.T) {
			probeOnce := func(user, pass string) core.Result {
				target, err := core.ParseTarget(srv.addr)
				if err != nil {
					t.Fatal(err)
				}
				var p Prober
				return p.Probe(context.Background(), target, core.Credential{Username: user, Password: pass},
					core.Options{Timeout: 10 * time.Second})
			}

			// 就绪等待
			deadline := time.Now().Add(90 * time.Second)
			for {
				res := probeOnce("mongoadmin", "MongoPass123!")
				if res.Outcome == core.OutcomeAuthSuccess || res.Outcome == core.OutcomeAuthFailed {
					break
				}
				if time.Now().After(deadline) {
					t.Fatalf("server not ready: %v (%s)", res.Outcome, res.ErrDetail)
				}
				time.Sleep(2 * time.Second)
			}

			if res := probeOnce("mongoadmin", "MongoPass123!"); res.Outcome != core.OutcomeAuthSuccess {
				t.Errorf("correct creds: want success got %v (%s)", res.Outcome, res.ErrDetail)
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
			// Unicode 错误密码（走完整 SCRAM 流程后失败）
			if res := probeOnce("mongoadmin", "密码🔐错误"); res.Outcome != core.OutcomeAuthFailed {
				t.Errorf("unicode wrong password: want auth-failed got %v (%s)", res.Outcome, res.ErrDetail)
			}

			// 未授权 ping：认证开启的库必须拒绝
			target, _ := core.ParseTarget(srv.addr)
			up := UnauthProber{}
			ping := up.Probe(context.Background(), target, core.Credential{}, core.Options{Timeout: 10 * time.Second})
			if ping.Outcome == core.OutcomeAuthSuccess {
				t.Errorf("ping without creds on auth-enabled server should not succeed")
			}
		})
	}
}
