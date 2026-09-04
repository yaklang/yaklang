package bruteutils

import (
	"context"
	"errors"
	"fmt"
	"io"
	stdlog "log"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/netx"
	"github.com/yaklang/yaklang/common/utils"

	//"github.com/shadow1ng/fscan/common"

	"github.com/yaklang/yaklang/common/utils/bruteutils/grdp/core"
	"github.com/yaklang/yaklang/common/utils/bruteutils/grdp/glog"
	"github.com/yaklang/yaklang/common/utils/bruteutils/grdp/protocol/nla"
	"github.com/yaklang/yaklang/common/utils/bruteutils/grdp/protocol/pdu"
	"github.com/yaklang/yaklang/common/utils/bruteutils/grdp/protocol/rfb"
	"github.com/yaklang/yaklang/common/utils/bruteutils/grdp/protocol/sec"
	"github.com/yaklang/yaklang/common/utils/bruteutils/grdp/protocol/t125"
	"github.com/yaklang/yaklang/common/utils/bruteutils/grdp/protocol/tpkt"
	"github.com/yaklang/yaklang/common/utils/bruteutils/grdp/protocol/x224"
)

var rdpAuth = &DefaultServiceAuthInfo{
	ServiceName:      "rdp",
	DefaultPorts:     "3389",
	DefaultPasswords: append([]string{"123456", "admin", "admin123", "administrator", "guest"}, CommonUsernames...),
	DefaultUsernames: []string{"administrator", "guest", "admin"},
	UnAuthVerify: func(i *BruteItem) *BruteItemResult {
		i.Target = appendDefaultPort(i.Target, 3389)
		result := i.Result()
		// NLA 没有“未授权访问”：空密码也是一次认证失败。
		// 这里只做 TCP 可达性，避免 3s sleep + administrator:"" 误伤爆破节奏。
		conn, err := defaultDialer.DialContext(utils.TimeoutContext(defaultTimeout), "tcp", i.Target)
		if err != nil {
			res := i.Result()
			res.Finished = true
			return res
		}
		_ = conn.Close()
		return result
	},
	BrutePass: func(i *BruteItem) (result *BruteItemResult) {
		result = i.Result()
		defer func() {
			if err := recover(); err != nil {
				log.Errorf("brute item failed: %s", err)
			}
		}()

		i.Target = appendDefaultPort(i.Target, 3389)
		host, port, err := utils.ParseStringToHostPort(i.Target)
		if err != nil {
			log.Errorf("parse target[%v] failed: %s", i.Target, err)
			result.Finished = true
			return result
		}

		var r bool
		// 本地帐号爆破必须空域：Client Info 的 Domain 填 IP 时 XP 不会按
		// Administrator 自动登录，后续也收不到 Save Session Info。
		if utils.IsIPv4(host) {
			r, err = rdpLoginContext(i.Context, host, "", i.Username, i.Password, port)
		} else {
			ip := netx.LookupFirst(host, netx.WithTimeout(defaultTimeout))
			r, err = rdpLoginContext(i.Context, ip, "", i.Username, i.Password, port)
		}

		if err != nil {
			var auth *rdpAuthError
			if errors.As(err, &auth) {
				return result
			}
			var cssp *nla.CredSSPError
			if errors.As(err, &cssp) {
				if cssp.AccountLocked() {
					result.Finished = true
				}
				return result
			}
			autoSetFinishedByConnectionError(err, result)
			return result
		}
		if r {
			result.Finished = true
			result.Ok = true
			return result
		}

		return result
	},
}

//func RdpScan(info *common.HostInfo) (tmperr error) {
//	if common.IsBrute {
//		return
//	}
//	starttime := time.Now().Unix()
//	for _, user := range common.Userdict["rdp"] {
//		for _, pass := range common.Passwords {
//			pass = strings.Replace(pass, "{user}", user, -1)
//			port, err := strconv.Atoi(info.Ports)
//			flag, err := RdpConn(info.Host, info.Domain, user, pass, port)
//			if flag == true && err == nil {
//				result := fmt.Sprintf("[+] RDP:%v:%v:%v %v", info.Host, info.Ports, user, pass)
//				common.LogSuccess(result)
//				return err
//			} else {
//				errlog := fmt.Sprintf("[-] rdp %v:%v %v %v %v", info.Host, info.Ports, user, pass, err)
//				common.LogError(errlog)
//				tmperr = err
//				if common.CheckErrs(err) {
//					return err
//				}
//				if time.Now().Unix()-starttime > (int64(len(common.Userdict["rdp"])*len(common.Passwords)) * info.Timeout) {
//					return err
//				}
//			}
//		}
//	}
//	return tmperr
//}

// Login 尝试登录 RDP（远程桌面）服务，用于验证给定凭据是否有效
// 参数:
//   - ip: 目标主机 IP 地址
//   - domain: 登录所属的域，无域时可传空字符串
//   - user: 登录用户名
//   - password: 登录密码
//   - port: RDP 服务端口，通常为 3389
//
// 返回值:
//   - 登录是否成功
//   - 错误信息，连接失败或认证失败时返回非空
//
// Example:
// ```
// // 验证 RDP 凭据，依赖目标服务，此处仅作示意
// ok, err = rdp.Login("192.168.1.1", "", "administrator", "123456", 3389)
// println(ok)
// ```
// rdpLoginContext 在给定 ctx 内尝试 RDP 登录；ctx 取消/超时会立即中断
// 拨号与连接上的阻塞读写（deadline 贯通到 TLS/CredSSP 层）。
func rdpLoginContext(ctx context.Context, ip, domain, user, password string, port int) (_ bool, err error) {
	defer func() {
		if err1 := recover(); err1 != nil {
			err = utils.Errorf("recover rdp login from panic: %s", err1)
		}
	}()
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, time.Second*15)
	defer cancel()
	target := fmt.Sprintf("%s:%d", ip, port)
	g := newRDPClient(target, glog.NONE)
	err = g.Login(ctx, domain, user, password)
	if err != nil {
		return false, err
	}
	return true, nil
}

func rdpLogin(ip, domain, user, password string, port int) (_ bool, err error) {
	return rdpLoginContext(context.Background(), ip, domain, user, password, port)
}

var RDPLogin = rdpLogin

// rdpAuthError 是协议已连通、但帐密被拒绝。调度器应继续字典，不得 Finished。
type rdpAuthError struct{ msg string }

func (e *rdpAuthError) Error() string { return e.msg }

type rdpClient struct {
	Host string // ip:port
	tpkt *tpkt.TPKT
	x224 *x224.X224
	mcs  *t125.MCSClient
	sec  *sec.Client
	pdu  *pdu.Client
	vnc  *rfb.RFB
}

func newRDPClient(host string, logLevel glog.LEVEL) *rdpClient {
	if os.Getenv("YAK_RDP_GLOG") == "debug" {
		logLevel = glog.DEBUG
	} else if os.Getenv("YAK_RDP_GLOG") != "" {
		logLevel = glog.INFO
	}
	glog.SetLevel(logLevel)
	logger := stdlog.New(os.Stdout, "", 0)
	glog.SetLogger(logger)
	return &rdpClient{
		Host: host,
	}
}

func (g *rdpClient) Login(ctx context.Context, domain, user, pwd string) error {
	if ctx == nil {
		ctx = context.Background()
	}

	conn, err := defaultDialer.DialTCPContext(ctx, "tcp", g.Host)
	if err != nil {
		return fmt.Errorf("dial error: %v", err)
	}
	defer conn.Close()
	// deadline 贯通：ctx 的截止时间作用于连接上所有读写（含 TLS 与 CredSSP 阶段），
	// 取消时 defer Close 会强制中断仍在阻塞的读写。
	if d, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(d)
	}
	g.tpkt = tpkt.New(core.NewSocketLayer(conn), nla.NewNTLMv2(domain, user, pwd))
	g.x224 = x224.New(g.tpkt)
	g.mcs = t125.NewMCSClient(g.x224)
	g.sec = sec.NewClient(g.mcs)
	g.pdu = pdu.NewClient(g.sec)

	g.sec.SetUser(user)
	g.sec.SetPwd(pwd)
	g.sec.SetDomain(domain)

	g.tpkt.SetFastPathListener(g.sec)
	g.sec.SetFastPathListener(g.pdu)
	g.pdu.SetFastPathSender(g.tpkt)

	wg := &sync.WaitGroup{}
	// 竞态治理：第一个到达的事件（error/close/success/ready/nla-ok/ctx 超时）
	// 通过 doneOnce 唯一写入 finalErr 并完成 wg；后续事件成为 no-op，
	// 消除旧实现中多 handler 并发写共享 err 的 data race。
	var doneOnce sync.Once
	var finalErr error
	var sessionReady atomic.Bool
	loginDone := make(chan struct{})
	wg.Add(1)
	finish := func(e error) {
		doneOnce.Do(func() {
			if e != nil {
				log.Errorf("rdp error: %v", e)
			}
			finalErr = e
			wg.Done()
		})
	}

	// 必须在 Connect 之前注册：NLA 在 localhost 上可能快于后续 On()。
	g.x224.On("error", func(e error) { finish(classicDropOr(e, &sessionReady, g)) })
	g.x224.On("nla-ok", func() {
		log.Info("rdp nla authentication succeeded")
		finish(nil)
	})
	g.pdu.On("error", func(e error) {
		if e != nil && strings.Contains(e.Error(), "rdp logon failed") {
			finish(&rdpAuthError{msg: e.Error()})
			return
		}
		finish(classicDropOr(e, &sessionReady, g))
	})
	g.pdu.On("close", func() {
		finish(classicDropOr(errors.New("close"), &sessionReady, g))
	})
	g.pdu.On("success", func() {
		log.Info("rdp login success")
		finish(nil)
	})
	g.pdu.On("ready", func() {
		// 标准 RDP 加密（XP 等）在 FontMap 时就会 ready，此时尚未校验帐密。
		// 成功必须再等正向信号：0x26、会话切换（Deactivate/Demand Active）、
		// 或 ncrack 成功绘图订单。失败对话框是失败。超时不是成功。
		// SSL/xrdp 通常没有后续 logon PDU，FontMap 即结束。
		if g.x224.SelectedProtocol() == x224.PROTOCOL_RDP {
			log.Info("rdp session ready, waiting for logon")
			sessionReady.Store(true)
			return
		}
		log.Info("rdp session ready")
		finish(nil)
	})
	g.pdu.On("logon", func(si *pdu.SaveSessionInfo) {
		if si != nil && si.AuthOK() {
			log.Info("rdp logon succeeded")
			finish(nil)
			return
		}
		infoType := uint32(0)
		fields := uint32(0)
		if si != nil {
			infoType = si.InfoType
			fields = si.FieldsPresent
		}
		finish(&rdpAuthError{msg: fmt.Sprintf("rdp logon failed: infotype=%d fields=%d", infoType, fields)})
	})
	g.pdu.On("update", func(rectangles []pdu.BitmapData) {
		_ = rectangles
	})

	err = g.x224.Connect()
	if err != nil {
		return fmt.Errorf("[x224 connect err] %v", err)
	}
	glog.Info("wait connect ok")

	// ctx 截止后仍无事件：协议不对或对端无响应；Login 返回后 goroutine 立即退出。
	go func() {
		select {
		case <-ctx.Done():
			if sessionReady.Load() {
				g.pdu.Emit("error", &rdpAuthError{msg: "rdp logon failed: no session-info or share restart"})
			} else {
				g.pdu.Emit("error", utils.Errorf("protocol error or no response: %v", ctx.Err()))
			}
		case <-loginDone:
		}
	}()

	wg.Wait()
	close(loginDone)
	return finalErr
}

// classicDropOr：XP/2003 标准 RDP 在 FontMap 之后才会 ready，此时帐密尚未
// 被协议确认。对端随后 EOF/复位/关连接（停在登录界面、拒绝 AUTOLOGON）
// 是认证失败，不能当成目标不可达。
func classicDropOr(err error, ready *atomic.Bool, g *rdpClient) error {
	if err == nil || ready == nil || g == nil || g.x224 == nil {
		return err
	}
	if !ready.Load() || g.x224.SelectedProtocol() != x224.PROTOCOL_RDP {
		return err
	}
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return &rdpAuthError{msg: "rdp logon failed: session dropped without save-session-info"}
	}
	msg := strings.ToLower(err.Error())
	if msg == "close" || strings.Contains(msg, "eof") ||
		strings.Contains(msg, "broken pipe") || strings.Contains(msg, "connection reset") ||
		strings.Contains(msg, "use of closed") || strings.Contains(msg, "i/o timeout") ||
		strings.Contains(msg, "deadline exceeded") {
		return &rdpAuthError{msg: "rdp logon failed: session dropped without save-session-info"}
	}
	return err
}
