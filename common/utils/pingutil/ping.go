package pingutil

import (
	"context"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/netx"
	"github.com/yaklang/yaklang/common/utils"
	"net"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

import "github.com/tatsushid/go-fastping"

type PingResult struct {
	IP     string
	Ok     bool
	RTT    int64
	Reason string
}

var promptICMPNotAvailableOnce = new(sync.Once)

type PingConfig struct {
	defaultTcpPort string
	timeout        time.Duration
	proxies        []string

	// for test
	pingNativeHandler func(ip string, timeout time.Duration) *PingResult
	tcpDialHandler    func(ctx context.Context, addr string, proxies ...string) (net.Conn, error)
}

func PingAutoConfig(ip string, config *PingConfig) *PingResult {
	var (
		defaultTcpPort = config.defaultTcpPort
		timeout        = config.timeout
		proxies        = config.proxies
	)

	if defaultTcpPort == "" {
		defaultTcpPort = "22,80,443"
	}

	start := time.Now()
	defer func() {
		if time.Since(start).Seconds() > 6 {
			log.Warnf("ping-auto cost: %v, too long!", time.Since(start).Seconds())
		}
	}()

	log.Debugf("ping-auto cost timeout: %v", timeout.Seconds())
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	testPorts := utils.ParseStringToPorts(defaultTcpPort)
	if len(testPorts) > 5 {
		testPorts = testPorts[:5]
		log.Infof("tcp-ping[%s] too many ports, only test first 5 most", defaultTcpPort)
	}
	swg := utils.NewSizedWaitGroup(6)
	var alive = utils.NewBool(false)

	if len(proxies) > 0 {
		if !icmpPingIsNotAvailable.IsSet() {
			icmpPingIsNotAvailable.Set()
		}
	}

	swg.Add()
	go func() {
		defer swg.Done()
		// if icmp is available, do it
		if !icmpPingIsNotAvailable.IsSet() {
			var result *PingResult
			if config.pingNativeHandler != nil {
				result = config.pingNativeHandler(ip, timeout)
			} else {
				result = PingNative(ip, timeout)
			}
			if result.Ok {
				alive.Set()
				cancel()

			}
		} else {
			promptICMPNotAvailableOnce.Do(func() {
				log.Infof("icmp is not available, fallback to use tcp-ping")
			})
		}
	}()

	var unreachableCount int64
	addUnreachableCount := func() {
		atomic.AddInt64(&unreachableCount, 1)
	}
	for _, port := range testPorts {
		err := swg.AddWithContext(ctx)
		if err != nil {
			break
		}
		addr := utils.HostPort(ip, port)
		go func() {
			defer swg.Done()
			var (
				conn net.Conn
				err  error
			)
			if config.tcpDialHandler != nil {
				conn, err = config.tcpDialHandler(ctx, addr, proxies...)
			} else {
				conn, err = netx.DialContext(ctx, addr, proxies...)
			}
			if err != nil {
				switch ret := err.(type) {
				case *net.OpError:
					if ret.Timeout() {
						addUnreachableCount()
					} else {
						alive.Set()
						cancel()
					}
				default:
					if utils.MatchAnyOfSubString(
						strings.ToLower(err.Error()),
						"timeout", "deadline exceeded", " A connection attempt failed",
						"no proxy available",
					) {
						addUnreachableCount()
					}
				}
				return
			}
			alive.Set()
			cancel()
			conn.Close()
		}()
	}
	swg.Wait()

	if alive.IsSet() {
		return &PingResult{
			IP: ip,
			Ok: true,
		}
	}

	if unreachableCount == int64(len(testPorts)) {
		log.Debugf("tcp-ping %v -> [%s] all timeout, it seems down", ip, defaultTcpPort)
		return &PingResult{
			IP:     ip,
			Ok:     false,
			Reason: "tcp timeout",
		}
	}

	return &PingResult{
		IP:     ip,
		Ok:     true,
		Reason: "unknown(fallback)",
	}
}

func PingAuto(ip string, defaultTcpPort string, timeout time.Duration, proxies ...string) *PingResult {
	return PingAutoConfig(ip, &PingConfig{
		defaultTcpPort: defaultTcpPort,
		timeout:        timeout,
		proxies:        proxies,
	})
}

var icmpPingIsNotAvailable = utils.NewBool(false)

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
