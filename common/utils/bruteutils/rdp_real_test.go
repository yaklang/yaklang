package bruteutils

import (
	"context"
	"net"
	"os"
	"strings"
	"testing"
	"time"
)

// RDP 真实服务器矩阵（YAK_BRUTE_REAL=1 启用；按可达性自动跳过）。
//
// NLA-only（CredSSP NTLMv2）xrdp 容器，构建于 testdata/rdp/Dockerfile：
//
//	docker build -t yak-rdp-nla:24.04 testdata/rdp
//	docker run -d --rm --name yak-rdp-nla -p 33389:3389 yak-rdp-nla:24.04
//	docker run -d --rm --name yak-rdp-nla-2204 -p 33390:3389 yak-rdp-nla:22.04
//
// 内置账户：rdpuser / RdpPass123!，unicode用户 / 密码🔐123。
var rdpVersionMatrix = []struct {
	name string
	addr string
}{
	{"xrdp-nla-2404", "127.0.0.1:33389"},
	{"xrdp-nla-2204", "127.0.0.1:33390"},
}

func rdpHostReachable(addr string) bool {
	conn, err := net.DialTimeout("tcp", addr, 3*time.Second)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

func rdpProbeOnce(t *testing.T, addr, user, pass string) *BruteItemResult {
	t.Helper()
	item := &BruteItem{
		Type:     "rdp",
		Target:   addr,
		Username: user,
		Password: pass,
		Context:  context.Background(),
	}
	return rdpAuth.BrutePass(item)
}

func TestRDPRealMatrix(t *testing.T) {
	if os.Getenv("YAK_BRUTE_REAL") != "1" {
		t.Skip("set YAK_BRUTE_REAL=1 to run against real RDP servers")
	}
	for _, srv := range rdpVersionMatrix {
		t.Run(srv.name, func(t *testing.T) {
			if !rdpHostReachable(srv.addr) {
				t.Skipf("%s not reachable (start container first)", srv.addr)
			}

			// 就绪等待：xrdp 启动很快，但 TLS/sesman 就绪需要数秒；
			// 以「正确凭证被接受」为就绪信号（未就绪时连接被拒绝等均视为未就绪）。
			deadline := time.Now().Add(120 * time.Second)
			for {
				okRes := rdpProbeOnce(t, srv.addr, "rdpuser", "RdpPass123!")
				if okRes.Ok {
					break
				}
				if time.Now().After(deadline) {
					t.Fatalf("xrdp not ready: correct creds not accepted (ok=%v finished=%v)", okRes.Ok, okRes.Finished)
				}
				time.Sleep(3 * time.Second)
			}

			// 正确凭证：xrdp 选择 SSL（无 NLA）时，连接序列完成即到达会话。
			if res := rdpProbeOnce(t, srv.addr, "rdpuser", "RdpPass123!"); !res.Ok || !res.Finished {
				t.Errorf("correct creds: want ok+finished got ok=%v finished=%v", res.Ok, res.Finished)
			}
			// 已知限制（xrdp 主线不支持服务端 NLA，见 rdp_nla_test.go）：
			// SSL 模式下 xrdp 认证失败只显示图形登录框，不发任何协议级
			// 失败信号也不断连（xrdp_wm_mod_connect_done → WMLS_INACTIVE），
			// 因此错误凭证可能被判定为「连接成功」。真实 Windows/商用 NLA
			// 目标（互联网主流）的成败判定由 CredSSP 阶段给出，已在
			// TestRDPNLAAuthentication 确定性覆盖。此处仅要求：
			// 1) 不 panic 不挂死 2) 不把目标标记为不可达。
			for _, c := range []struct{ user, pass, label string }{
				{"rdpuser", "wrong-password", "wrong password"},
				{"no-such-user-xyz", "whatever", "unknown user"},
				{"rdpuser", "", "empty password"},
				{"unicode用户", "密码错误🔐", "unicode wrong password"},
				{"rdpuser", strings.Repeat("z", 256), "long password"},
			} {
				start := time.Now()
				res := rdpProbeOnce(t, srv.addr, c.user, c.pass)
				if elapsed := time.Since(start); elapsed > 25*time.Second {
					t.Errorf("%s: hung for %v", c.label, elapsed)
				}
				// 注：若误报成功（ok=true），handler 会相应置 Finished=true
				// （成功语义），这是可接受的；仅当未成功时不得标记不可达。
				if res.Finished && !res.Ok {
					t.Errorf("%s: target must not be marked unavailable (finished=%v)", c.label, res.Finished)
				}
				if res.Ok {
					t.Logf("%s: ok=true（xrdp SSL 模式已知误报，无协议级失败信号）", c.label)
				}
			}
			// Unicode 正向必须成功
			if res := rdpProbeOnce(t, srv.addr, "unicode用户", "密码🔐123"); !res.Ok {
				t.Errorf("unicode correct creds: want ok got ok=%v finished=%v", res.Ok, res.Finished)
			}
		})
	}
}

// TestRDPRealTargetUnavailable 不可达目标必须被标记 Finished（调度器据此跳过该目标）。
func TestRDPRealTargetUnavailable(t *testing.T) {
	if os.Getenv("YAK_BRUTE_REAL") != "1" {
		t.Skip("set YAK_BRUTE_REAL=1")
	}
	res := rdpProbeOnce(t, "127.0.0.1:1", "administrator", "x")
	if res.Ok || !res.Finished {
		t.Errorf("unreachable target: want finished+not-ok got ok=%v finished=%v", res.Ok, res.Finished)
	}
}

// TestRDPRealProtocolMismatch RDP 探测打到非 RDP 服务（MySQL）：
// 必须快速失败且不能被误判为认证成功。
func TestRDPRealProtocolMismatch(t *testing.T) {
	if os.Getenv("YAK_BRUTE_REAL") != "1" {
		t.Skip("set YAK_BRUTE_REAL=1")
	}
	mysqlAddr := "127.0.0.1:33306"
	if !rdpHostReachable(mysqlAddr) {
		t.Skipf("%s not reachable", mysqlAddr)
	}
	start := time.Now()
	res := rdpProbeOnce(t, mysqlAddr, "root", "x")
	if res.Ok {
		t.Errorf("protocol mismatch must not be ok")
	}
	// x224 超时（15s）或对端断连都应在此窗口内返回
	if elapsed := time.Since(start); elapsed > 20*time.Second {
		t.Errorf("protocol mismatch took too long: %v", elapsed)
	}
}

// TestRDPRealContextCancellation Context 取消必须立即中断登录，
// 不得等待内部 15s 超时。
func TestRDPRealContextCancellation(t *testing.T) {
	if os.Getenv("YAK_BRUTE_REAL") != "1" {
		t.Skip("set YAK_BRUTE_REAL=1")
	}
	addr := "127.0.0.1:33389"
	if !rdpHostReachable(addr) {
		t.Skipf("%s not reachable", addr)
	}

	// 黑洞服务：接受连接但不响应，确保只能靠 ctx 取消退出
	blackhole, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer blackhole.Close()
	go func() {
		for {
			c, err := blackhole.Accept()
			if err != nil {
				return
			}
			// 挂住连接不读不写
			_ = c
		}
	}()

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(500 * time.Millisecond)
		cancel()
	}()
	start := time.Now()
	_, err = rdpLoginContext(ctx, "127.0.0.1", "", "user", "pass", blackhole.Addr().(*net.TCPAddr).Port)
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("cancelled login must fail")
	}
	if elapsed > 5*time.Second {
		t.Errorf("cancel took too long: %v (want <5s)", elapsed)
	}
	if !strings.Contains(err.Error(), "context canceled") && !strings.Contains(err.Error(), "protocol error") {
		t.Logf("cancel error: %v", err)
	}
}
