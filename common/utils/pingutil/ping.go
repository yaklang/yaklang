package pingutil

import (
	"context"
	"fmt"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/icmp"
	"github.com/yaklang/yaklang/common/netstackvm"
	"github.com/yaklang/yaklang/common/netx"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/pcapfix"
	"net"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"
)

import "github.com/tatsushid/go-fastping"

type PingResult struct {
	IP     string
	Ok     bool
	RTT    int64
	Reason string
}

var canCaptureOnce sync.Once
var icmpPingIsNotAvailable = utils.NewBool(false)

func init() {
	canCaptureOnce.Do(func() {
		if runtime.GOOS == "windows" {
			return
		}
		if pcapfix.IsPrivilegedForNetRaw() {
			icmpPingIsNotAvailable.Set()
		}
	})
}

func PingAutoConfig(ip string, opts ...PingConfigOpt) *PingResult {
	config := NewPingConfig()
	for _, f := range opts {
		f(config)
	}
	if config.Ctx == nil {
		config.Ctx = context.Background()
	}
	defaultTcpPort := config.defaultTcpPort
	proxies := config.proxies
	timeout := config.timeout
	parentCtx := config.Ctx

	start := time.Now()
	defer func() {
		if time.Since(start).Seconds() > 6 {
			log.Debugf("ping-auto cost: %v, too long!", time.Since(start).Seconds())
		}
	}()

	testPorts := utils.ParseStringToPorts(defaultTcpPort)
	if len(testPorts) > 5 {
		testPorts = testPorts[:5]
		log.Infof("tcp-ping[%s] too many ports, only test first 5 most", defaultTcpPort)
	}

	if !icmpPingIsNotAvailable.IsSet() && !config.forceTcpPing && len(proxies) == 0 {
		if config.pingNativeHandler != nil {
			return config.pingNativeHandler(ip, timeout)
		} else {
			subCtx, _ := context.WithTimeout(parentCtx, timeout)
			result, err := NetstackPing(subCtx, ip, config.linkAddressResolveTimeout)
			if result != nil {
				return result
			}
			log.Errorf("netstack ping fail %v", err)
		}
	}

	// tcp ping
	wg := new(sync.WaitGroup)
	isAlive := utils.NewBool(false)
	ctx, cancel := context.WithTimeout(parentCtx, config.timeout)
	defer cancel()
	for _, p := range testPorts {
		p := p
		wg.Add(1)
		go func() {
			defer wg.Done()
			var conn net.Conn
			var err error
			if config.tcpDialHandler != nil {
				conn, err = config.tcpDialHandler(ctx, utils.HostPort(ip, p), config.proxies...)
			} else {
				conn, err = netx.DialContext(ctx, utils.HostPort(ip, p), config.proxies...)
			}
			if err != nil && !utils.IContains(err.Error(), "refused") { // if err is refused ,mean host is alive
				return
			}
			isAlive.Set()
			cancel()
			if conn != nil {
				_ = conn.Close()
			}
		}()
	}
	wg.Wait()
	if isAlive.IsSet() {
		return &PingResult{
			IP:  ip,
			Ok:  true,
			RTT: 0,
		}
	}
	return &PingResult{
		IP:     ip,
		Ok:     false,
		RTT:    0,
		Reason: "tcp timeout",
	}
}

func PingAuto(ip string, opts ...PingConfigOpt) *PingResult {
	return PingAutoConfig(ip, opts...)
}

func PingNativeBase(ip string, cxt context.Context, timeout time.Duration) *PingResult {
	if icmpPingIsNotAvailable.IsSet() {
		return &PingResult{
			IP:     ip,
			Ok:     false,
			RTT:    0,
			Reason: "raw:icmp is not available",
		}
	}
	core := fastping.NewPinger()
	err := core.AddIP(ip)
	if err != nil {
		return &PingResult{
			IP:     ip,
			Ok:     false,
			RTT:    0,
			Reason: err.Error(),
		}
	}

	var result = &PingResult{IP: ip, Reason: "initialized"}

	core.OnRecv = func(addr *net.IPAddr, rtt time.Duration) {
		if addr.String() == ip {
			result.Ok = true
			result.RTT = int64(rtt) / int64(time.Millisecond)
			result.Reason = ""
		}
	}
	core.OnIdle = func() {

	}

	errChan := make(chan error, 1)
	go func() {
		defer close(errChan)
		err := core.Run()
		if err != nil {
			switch ret := err.(type) {
			case *net.OpError:
				if ret2, ok := ret.Err.(*os.SyscallError); ok {
					if strings.Contains(strings.ToLower(ret2.Error()), "operation not permitted") {
						icmpPingIsNotAvailable.Set()
					}
				}
			}
			result.Reason = err.Error()
			return
		}
	}()

	select {
	case err, _ := <-errChan:
		if err != nil {
			log.Errorf("ping native mode failed: %s", err)
			return &PingResult{
				IP:     ip,
				Ok:     false,
				RTT:    0,
				Reason: err.Error(),
			}
		}
	case <-time.After(timeout):
		log.Infof("timeout ping for %v", ip)
		core.Stop()
	case <-cxt.Done():
		log.Infof("timeout ping for %v", ip)
		core.Stop()
	}

	return result
}

func PingNative(ip string, timeout time.Duration) *PingResult {
	return PingNativeBase(ip, context.Background(), timeout)
}

func NetstackPing(ctx context.Context, ip string, timeout time.Duration) (*PingResult, error) {
	if client := netstackvm.GetDefaultICMPClient(); client != nil {
		res, err := client.Ping(ctx, ip, timeout)
		if err != nil {
			return nil, err
		}
		return CreatePingResult(res), nil
	}
	return nil, fmt.Errorf("netstack icmp client is not available")
}

func CreatePingResult(result *icmp.Result) *PingResult {
	res := &PingResult{
		IP:  result.Address.String(),
		Ok:  result.Ok,
		RTT: result.RTT.Milliseconds(),
	}

	if !result.Ok {
		res.Reason = fmt.Sprintf("recv icmp type %d , code %d", result.MessageType, result.MessageCode)
	}
	return res
}
