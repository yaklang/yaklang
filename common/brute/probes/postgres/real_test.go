package postgres

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/brute/core"
)

// 真实服务器测试（YAK_BRUTE_REAL=1 启用）：
//
//	docker run -d --rm --name yak-pg16 -p 35432:5432 -e 'POSTGRES_PASSWORD=PgPass123!' -e 'POSTGRES_USER=pgadmin' postgres:16
//	docker run -d --rm --name yak-pg12 -p 35433:5432 -e 'POSTGRES_PASSWORD=PgPass123!' -e 'POSTGRES_USER=pgadmin' postgres:12
//
// PG14+ 默认 SCRAM-SHA-256，PG12 默认 MD5 —— 覆盖两种服务端认证方式。
func TestRealPostgresServers(t *testing.T) {
	if os.Getenv("YAK_BRUTE_REAL") != "1" {
		t.Skip("set YAK_BRUTE_REAL=1 to run against real PostgreSQL servers")
	}
	for _, srv := range []struct {
		name, addr string
		sslEnabled bool
	}{
		{"pg16-scram", "127.0.0.1:35432", true},
		{"pg12-md5", "127.0.0.1:35433", false},
	} {
		t.Run(srv.name, func(t *testing.T) {
			probeOnce := func(user, pass string, policy core.TLSPolicy) core.Result {
				target, err := core.ParseTarget(srv.addr)
				if err != nil {
					t.Fatal(err)
				}
				var p Prober
				return p.Probe(context.Background(), target, core.Credential{Username: user, Password: pass},
					core.Options{Timeout: 10 * time.Second, TLSPolicy: policy})
			}

			// 就绪等待
			deadline := time.Now().Add(90 * time.Second)
			for {
				res := probeOnce("pgadmin", "PgPass123!", core.TLSOpportunistic)
				if res.Outcome == core.OutcomeAuthSuccess || res.Outcome == core.OutcomeAuthFailed {
					break
				}
				if time.Now().After(deadline) {
					t.Fatalf("server not ready: %v (%s)", res.Outcome, res.ErrDetail)
				}
				time.Sleep(2 * time.Second)
			}

			// 1. 正确凭证（Opportunistic TLS：容器自签证书，验证升级路径）
			res := probeOnce("pgadmin", "PgPass123!", core.TLSOpportunistic)
			if res.Outcome != core.OutcomeAuthSuccess {
				t.Errorf("correct creds: want success got %v (%s)", res.Outcome, res.ErrDetail)
			}

			// 2. 错误密码
			if res := probeOnce("pgadmin", "wrong-password", core.TLSOpportunistic); res.Outcome != core.OutcomeAuthFailed {
				t.Errorf("wrong password: want auth-failed got %v (%s)", res.Outcome, res.ErrDetail)
			}

			// 3. 不存在的用户（应可分类 + UserEliminated）
			if res := probeOnce("ghost-user-xyz", "x", core.TLSOpportunistic); res.Outcome != core.OutcomeAuthFailed {
				t.Errorf("unknown user: want auth-failed got %v (%s)", res.Outcome, res.ErrDetail)
			}

			// 4. Unicode 密码（错误密码场景，不应崩溃）
			if res := probeOnce("pgadmin", "密码错误🔐", core.TLSOpportunistic); res.Outcome != core.OutcomeAuthFailed {
				t.Errorf("unicode wrong password: want auth-failed got %v (%s)", res.Outcome, res.ErrDetail)
			}

			// 5. 空密码
			if res := probeOnce("pgadmin", "", core.TLSOpportunistic); res.Outcome != core.OutcomeAuthFailed {
				t.Errorf("empty password: want auth-failed got %v (%s)", res.Outcome, res.ErrDetail)
			}

			// 6. 明文策略（不尝试 TLS）
			res = probeOnce("pgadmin", "PgPass123!", core.PlaintextAllowed)
			if res.Outcome != core.OutcomeAuthSuccess {
				t.Errorf("plaintext policy: want success got %v (%s)", res.Outcome, res.ErrDetail)
			}
			if res.Transport != core.TransportPlainTCP {
				t.Errorf("transport=%v want tcp", res.Transport)
			}

			// 7. 严格 TLS：仅 SSL 开启的容器预期成功；未开启的预期 TLSRequired。
			res = probeOnce("pgadmin", "PgPass123!", core.TLSStrict)
			if srv.sslEnabled {
				if res.Outcome != core.OutcomeAuthSuccess {
					t.Errorf("strict tls: want success got %v (%s)", res.Outcome, res.ErrDetail)
				}
				if res.Transport != core.TransportTLS {
					t.Errorf("strict tls transport=%v", res.Transport)
				}
			} else if res.Outcome != core.OutcomeTLSRequired {
				t.Errorf("strict tls on ssl-off server: want tls-required got %v", res.Outcome)
			}
		})
	}
}
