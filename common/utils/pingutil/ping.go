package pingutil

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/icmp"
	"github.com/yaklang/yaklang/common/netstackvm"
	"github.com/yaklang/yaklang/common/netx"
	"github.com/yaklang/yaklang/common/utils"
)

type PingResult struct {
	IP     string
	Ok     bool
	RTT    int64
	Reason string
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
	if err := parentCtx.Err(); err != nil {
		return &PingResult{IP: ip, Reason: err.Error()}
	}

	// Loopback addresses (127.0.0.0/8, ::1) are always alive — they are the
	// local machine itself. Running ICMP/TCP probing against them is unreliable:
	// netstack ICMP ping times out on some platforms, and TCP-ping on default
	// ports (22/80/443) fails when those ports aren't listening. This would
	// cause the host to be judged dead and the entire downstream scan to be
	// skipped. Return alive immediately — but only for the default auto-probe
	// path (no forceTcpPing, no custom handlers, no proxy), so tests that
	// inject handlers to exercise error paths are unaffected.
	if utils.IsLoopback(ip) && !config.forceTcpPing && config.pingNativeHandler == nil && config.tcpDialHandler == nil && len(proxies) == 0 {
		return &PingResult{
			IP:     ip,
			Ok:     true,
			RTT:    0,
			Reason: "loopback",
		}
	}

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

	var icmpErr error
	if !config.forceTcpPing && len(proxies) == 0 {
		if config.pingNativeHandler != nil {
			if result := config.pingNativeHandler(ip, timeout); result != nil {
				return result
			}
		} else {
			subCtx, cancel := context.WithTimeout(parentCtx, timeout)
			result, err := NetstackPing(subCtx, ip, config.linkAddressResolveTimeout)
			cancel()
			if result != nil {
				return result
			}
			icmpErr = err
			log.Debugf("netstack ping failed: %v", err)
		}
	}

	if err := parentCtx.Err(); err != nil {
		return &PingResult{IP: ip, Reason: err.Error()}
	}

	if len(testPorts) == 0 {
		reason := "no TCP probe ports configured"
		if icmpErr != nil {
			reason = icmpErr.Error()
		}
		return &PingResult{IP: ip, Reason: reason}
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
			if conn != nil {
				defer conn.Close()
			}
			if err != nil && !utils.IContains(err.Error(), "refused") { // if err is refused ,mean host is alive
				return
			}
			isAlive.Set()
			cancel()
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
	if err := parentCtx.Err(); err != nil {
		return &PingResult{IP: ip, Reason: err.Error()}
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

// PingNativeBase probes ICMP through the shared netstack client.
func PingNativeBase(ip string, ctx context.Context, timeout time.Duration) *PingResult {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	result, err := NetstackPing(ctx, ip, timeout)
	if err != nil {
		return &PingResult{IP: ip, Reason: err.Error()}
	}
	return result
}

func PingNative(ip string, timeout time.Duration) *PingResult {
	return PingNativeBase(ip, context.Background(), timeout)
}

func NetstackPing(ctx context.Context, ip string, timeout time.Duration) (*PingResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
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
