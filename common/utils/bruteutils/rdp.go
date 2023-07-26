package bruteutils

import (
	"errors"
	"fmt"
	"github.com/ReneKroon/ttlcache"
	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	stdlog "log"
	"net"
	"os"
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
	"sync"
	"time"
)

var (
	rdpTLSCache = ttlcache.NewCache()
)

func init() {
	rdpTLSCache.SetTTL(5 * time.Minute)
}

var rdpAuth = &DefaultServiceAuthInfo{
	ServiceName:      "rdp",
	DefaultPorts:     "3389",
	DefaultPasswords: append([]string{"123456", "admin", "admin123", "administrator", "guest"}, CommonUsernames...),
	DefaultUsernames: []string{"administrator", "guest", "admin"},
	UnAuthVerify: func(i *BruteItem) *BruteItemResult {
		result := i.Result()
		i.Target = appendDefaultPort(i.Target, 3389)

		conn, err := DefaultDailer.Dial("tcp", i.Target)
		if err != nil {
			res := i.Result()
			res.Finished = true
			return res
		}
		conn.Close()
		//if utils.IsTLSService(i.Target) {
		//	rdpTLSCache.Set(i.Target, true)
		//}

		time.Sleep(3 * time.Second)

		host, port, err := utils.ParseStringToHostPort(i.Target)
		if err != nil {
			log.Errorf("parse target[%v] failed: %s", i.Target, err)
			result.Finished = true
			return result
		}

		var r bool
		if utils.IsIPv4(host) {
			r, err = rdpLogin(host, host, "administrator", "", port)

		} else {
			ip := utils.GetFirstIPByDnsWithCache(host, 5*time.Second)
			r, err = rdpLogin(ip, host, "administrator", "", port)
		}

		if err != nil {
			// 192.3.138.219/24
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
		if utils.IsIPv4(host) {
			r, err = rdpLogin(host, host, i.Username, i.Password, port)

		} else {
			ip := utils.GetFirstIPByDnsWithCache(host, 5*time.Second)
			r, err = rdpLogin(ip, host, i.Username, i.Password, port)
		}

		if err != nil {
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

func rdpLogin(ip, domain, user, password string, port int) (_ bool, err error) {
	defer func() {
		if err1 := recover(); err1 != nil {
			err = utils.Errorf("recover rdp login from panic: %s", err1)
		}
	}()
	target := fmt.Sprintf("%s:%d", ip, port)
	g := newRDPClient(target, glog.NONE)
	err = g.Login(domain, user, password)
	if err != nil {
		return false, err
	}
	return true, nil
}

var RDPLogin = rdpLogin

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
	glog.SetLevel(logLevel)
	logger := stdlog.New(os.Stdout, "", 0)
	glog.SetLogger(logger)
	return &rdpClient{
		Host: host,
	}
}

func (g *rdpClient) Login(domain, user, pwd string) error {
	conn, err := net.DialTimeout("tcp", g.Host, 5*time.Second)
	if err != nil {
		return fmt.Errorf("dial error: %v", err)
	}
	defer conn.Close()
	glog.Info(conn.LocalAddr().String())

	g.tpkt = tpkt.New(core.NewSocketLayer(conn), nla.NewNTLMv2(domain, user, pwd))
	g.x224 = x224.New(g.tpkt)
	g.mcs = t125.NewMCSClient(g.x224)
	g.sec = sec.NewClient(g.mcs)
	g.pdu = pdu.NewClient(g.sec)

	g.sec.SetUser(user)
	g.sec.SetPwd(pwd)
	g.sec.SetDomain(domain)
	//g.sec.SetClientAutoReconnect()

	g.tpkt.SetFastPathListener(g.sec)
	g.sec.SetFastPathListener(g.pdu)
	g.pdu.SetFastPathSender(g.tpkt)

	//g.x224.SetRequestedProtocol(x224.PROTOCOL_SSL)
	//g.x224.SetRequestedProtocol(x224.PROTOCOL_RDP)

	err = g.x224.Connect()
	if err != nil {
		return fmt.Errorf("[x224 connect err] %v", err)
	}
	glog.Info("wait connect ok")
	wg := &sync.WaitGroup{}
	breakFlag := false
	wg.Add(1)

	g.pdu.On("error", func(e error) {
		err = e
		log.Errorf("error: %v", e)
		g.pdu.Emit("done")
	})
	g.pdu.On("close", func() {
		err = errors.New("close")
		log.Errorf("closed: %v", err)
		g.pdu.Emit("done")
	})
	g.pdu.On("success", func() {
		err = nil
		log.Error("on success")
		g.pdu.Emit("done")
	})
	g.pdu.On("ready", func() {
		log.Error("on ready")
		g.pdu.Emit("done")
	})
	g.pdu.On("update", func(rectangles []pdu.BitmapData) {
		log.Infof("on update: %v", spew.Sdump(rectangles))
	})
	g.pdu.On("done", func() {
		if breakFlag == false {
			breakFlag = true
			wg.Done()
		}
	})

	wg.Wait()
	return err
}
