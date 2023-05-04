package pingutil

import (
	"context"
	"fmt"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"net"
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

func PingAuto(ip string, defaultTcpPort string, timeout time.Duration, proxies ...string) *PingResult {
	if defaultTcpPort == "" {
		defaultTcpPort = "22,80,443"
	}

	swg := utils.NewSizedWaitGroup(5)
	var timeoutResult []bool
	var feedbackResultLock = new(sync.Mutex)
	var counter int

	var alive = utils.NewBool(false)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// icmp ping
	swg.Add()
	go func() {
		defer swg.Done()
		result := PingNative(ip, timeout)
		if result.Reason == "" {
			alive.IsSet()
			cancel()
		}
	}()

	// tcp ping
	for _, port := range utils.ParseStringToPorts(defaultTcpPort) {
		err := swg.AddWithContext(ctx)
		if err != nil {
			continue
		}
		counter++
		addr := utils.HostPort(ip, port)
		go func() {
			defer swg.Done()
			conn, err := utils.TCPConnect(addr, timeout, proxies...)
			//conn, err := dialer.DialContext(ctx, "tcp", addr)
			if err != nil {
				if utils.MatchAnyOfRegexp(err, `(?i)i/?o\s+timeout`) {
					feedbackResultLock.Lock()
					timeoutResult = append(timeoutResult, true)
					feedbackResultLock.Unlock()
				} else {
					cancel()
					alive.Set()
				}
				return
			}
			cancel()
			alive.Set()
			conn.Close()
		}()
	}

	go func() {
		swg.Wait()
		cancel()
	}()

LOOP:
	for {
		select {
		case <-ctx.Done():
			break LOOP
		default:
			if alive.IsSet() {
				return &PingResult{IP: ip, Ok: true}
			}
			time.Sleep(200 * time.Millisecond)
		}
	}

	if alive.IsSet() {
		return &PingResult{
			IP: ip,
			Ok: true,
		}
	}

	if len(timeoutResult) == counter {
		return &PingResult{
			IP:     ip,
			Ok:     false,
			RTT:    0,
			Reason: fmt.Sprintf("tcp-ping[%s] all timeout", defaultTcpPort),
		}
	}
	return &PingResult{
		IP: ip,
		Ok: true,
	}
}

func PingNative(ip string, timeout time.Duration) *PingResult {
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
			//log.Errorf("pingscan failed: %s", err.Error())
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
		log.Info("timeout ping for %v", ip)
		core.Stop()
	}

	return result
}
