package postgres_test

import (
	"context"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/brute/core"
	"github.com/yaklang/yaklang/common/brute/probes/postgres"
)

// 多版本真实服务器矩阵（YAK_BRUTE_REAL=1；按可达性自动跳过）。
//
//	docker run -d --rm --name yak-pg16 -p 35432:5432 -e 'POSTGRES_PASSWORD=PgPass123!' -e 'POSTGRES_USER=pgadmin' postgres:16 \
//	  -c ssl=on -c ssl_cert_file=/opt/certs/server.crt -c ssl_key_file=/opt/certs/server.key   # 挂载自签证书
//	docker run -d --rm --name yak-pg12 -p 35433:5432 -e 'POSTGRES_PASSWORD=PgPass123!' -e 'POSTGRES_USER=pgadmin' postgres:12
//	docker run -d --rm --name yak-pg17 -p 36432:5432 -e 'POSTGRES_PASSWORD=PgPass123!' -e 'POSTGRES_USER=pgadmin' postgres:17
//	docker run -d --rm --name yak-pg15 -p 36433:5432 -e 'POSTGRES_PASSWORD=PgPass123!' -e 'POSTGRES_USER=pgadmin' postgres:15
//	docker run -d --rm --name yak-pg14 -p 36434:5432 -e 'POSTGRES_PASSWORD=PgPass123!' -e 'POSTGRES_USER=pgadmin' postgres:14
//	docker run -d --rm --name yak-pg13 -p 36435:5432 -e 'POSTGRES_PASSWORD=PgPass123!' -e 'POSTGRES_USER=pgadmin' postgres:13
var pgVersionMatrix = []struct {
	name       string
	addr       string
	sslEnabled bool
}{
	{"pg16-scram-tls", "127.0.0.1:35432", true},
	{"pg12-md5", "127.0.0.1:35433", false},
	{"pg17", "127.0.0.1:36432", false},
	{"pg15", "127.0.0.1:36433", false},
	{"pg14", "127.0.0.1:36434", false},
	{"pg13", "127.0.0.1:36435", false},
}

func TestPostgresVersionMatrix(t *testing.T) {
	if os.Getenv("YAK_BRUTE_REAL") != "1" {
		t.Skip("set YAK_BRUTE_REAL=1")
	}
	for _, srv := range pgVersionMatrix {
		t.Run(srv.name, func(t *testing.T) {
			if !pgHostReachable(srv.addr) {
				t.Skipf("%s not reachable", srv.addr)
			}
			probeOnce := func(user, pass string, policy core.TLSPolicy) core.Result {
				target, _ := core.ParseTarget(srv.addr)
				var p postgres.Prober
				return p.Probe(context.Background(), target, core.Credential{Username: user, Password: pass},
					core.Options{Timeout: 15 * time.Second, TLSPolicy: policy})
			}

			deadline := time.Now().Add(120 * time.Second)
			for {
				res := probeOnce("pgadmin", "PgPass123!", core.TLSOpportunistic)
				if res.Outcome == core.OutcomeAuthSuccess || res.Outcome == core.OutcomeAuthFailed {
					break
				}
				if time.Now().After(deadline) {
					t.Fatalf("not ready: %v (%s)", res.Outcome, res.ErrDetail)
				}
				time.Sleep(3 * time.Second)
			}

			if res := probeOnce("pgadmin", "PgPass123!", core.TLSOpportunistic); res.Outcome != core.OutcomeAuthSuccess {
				t.Errorf("correct: want success got %v (%s)", res.Outcome, res.ErrDetail)
			}
			if res := probeOnce("pgadmin", "wrong", core.TLSOpportunistic); res.Outcome != core.OutcomeAuthFailed {
				t.Errorf("wrong password: want auth-failed got %v (%s)", res.Outcome, res.ErrDetail)
			}
			if res := probeOnce("ghost-user", "x", core.TLSOpportunistic); res.Outcome != core.OutcomeAuthFailed {
				t.Errorf("unknown user: want auth-failed got %v (%s)", res.Outcome, res.ErrDetail)
			}
			if res := probeOnce("pgadmin", "", core.TLSOpportunistic); res.Outcome != core.OutcomeAuthFailed {
				t.Errorf("empty password: want auth-failed got %v (%s)", res.Outcome, res.ErrDetail)
			}
			if res := probeOnce("pgadmin", "密码错误🔐", core.TLSOpportunistic); res.Outcome != core.OutcomeAuthFailed {
				t.Errorf("unicode wrong: want auth-failed got %v (%s)", res.Outcome, res.ErrDetail)
			}
			long := strings.Repeat("y", 256)
			if res := probeOnce("pgadmin", long, core.TLSOpportunistic); res.Outcome != core.OutcomeAuthFailed {
				t.Errorf("long password: want auth-failed got %v (%s)", res.Outcome, res.ErrDetail)
			}

			// 严格 TLS：仅 SSL 开启的容器预期成功；未开启的预期 TLSRequired
			res := probeOnce("pgadmin", "PgPass123!", core.TLSStrict)
			if srv.sslEnabled {
				if res.Outcome != core.OutcomeAuthSuccess || res.Transport != core.TransportTLS {
					t.Errorf("strict tls: want success/tls got %v/%v (%s)", res.Outcome, res.Transport, res.ErrDetail)
				}
			} else if res.Outcome != core.OutcomeTLSRequired {
				t.Errorf("strict tls on ssl-off: want tls-required got %v", res.Outcome)
			}
			// 明文策略
			if res := probeOnce("pgadmin", "PgPass123!", core.PlaintextAllowed); res.Outcome != core.OutcomeAuthSuccess {
				t.Errorf("plaintext: want success got %v (%s)", res.Outcome, res.ErrDetail)
			}
		})
	}
}

func pgHostReachable(addr string) bool {
	conn, err := net.DialTimeout("tcp", addr, 3*time.Second)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}
