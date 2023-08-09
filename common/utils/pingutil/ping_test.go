package pingutil

import (
	"context"
	"errors"
	"math"
	"net"
	"testing"
	"time"
)

type pingTestCase struct {
	name   string
	ip     string
	config PingConfig
	expect bool
}

func TestPingAutoConfig(t *testing.T) {
	testCase := []pingTestCase{
		{
			name: "tcp timeout err test case",
			ip:   "127.0.0.1",
			config: PingConfig{
				defaultTcpPort:    "",
				timeout:           5 * time.Second,
				proxies:           nil,
				pingNativeHandler: pingEmpty,
				tcpDialHandler:    tcpTimeoutHandlerMaker(getTestTimeout("timeout")),
			},
			expect: false,
		},
		{
			name: "tcp attempt failed err test case",
			ip:   "127.0.0.1",
			config: PingConfig{
				defaultTcpPort:    "",
				timeout:           5 * time.Second,
				proxies:           nil,
				pingNativeHandler: pingEmpty,
				tcpDialHandler:    tcpTimeoutHandlerMaker(getTestTimeout("attempt failed")),
			},
			expect: false,
		},
		{
			name: "tcp refused err test case",
			ip:   "127.0.0.1",
			config: PingConfig{
				defaultTcpPort:    "",
				timeout:           5 * time.Second,
				proxies:           nil,
				pingNativeHandler: pingEmpty,
				tcpDialHandler:    tcpTimeoutHandlerMaker(getTestTimeout("refused")),
			},
			expect: true,
		},
		{
			name: "global timeout test case",
			ip:   "127.0.0.1",
			config: PingConfig{
				defaultTcpPort:    "",
				timeout:           5 * time.Second,
				proxies:           nil,
				pingNativeHandler: pingSleepHandlerMaker(),
				tcpDialHandler:    tcpSleepHandlerMaker(),
			},
			expect: false,
		},
	}
	for _, test := range testCase {
		start := time.Now()
		res := PingAutoConfig("127.0.0.1", &test.config)
		useTime := time.Since(start).Seconds()
		if math.Floor(useTime) > math.Floor(test.config.timeout.Seconds()) {
			t.Fatalf("timeout is %v,but use %v[%v]", test.config.timeout.Seconds(), useTime, test.name)
		}
		if res.Ok != test.expect {
			t.Fatalf("Expect %v but get %v at [%v]", test.expect, res.Ok, test.name)
		}

	}
}

func tcpTimeoutHandlerMaker(err error) func(ctx context.Context, addr string, proxies ...string) (net.Conn, error) {

	return func(ctx context.Context, addr string, proxies ...string) (net.Conn, error) {
		return nil, err
	}
}

func pingEmpty(ip string, timeout time.Duration) *PingResult {
	return &PingResult{
		IP:     "",
		Ok:     false,
		RTT:    0,
		Reason: "",
	}
}

func pingSleepHandlerMaker() func(ip string, timeout time.Duration) *PingResult {
	return func(ip string, timeout time.Duration) *PingResult {
		time.Sleep(timeout)
		return &PingResult{
			IP:     "",
			Ok:     false,
			RTT:    0,
			Reason: "",
		}
	}
}

func tcpSleepHandlerMaker() func(ctx context.Context, addr string, proxies ...string) (net.Conn, error) {
	return func(ctx context.Context, addr string, proxies ...string) (net.Conn, error) {
		var timeout time.Duration
		ddl, ok := ctx.Deadline()
		if ok {
			if du := ddl.Sub(time.Now()); du.Seconds() > 0 {
				timeout = du
			}
		}
		time.Sleep(timeout)
		return nil, getTestTimeout("timeout")
	}
}

func getTestTimeout(errName string) error {
	switch errName {
	case "timeout":
		_, err := net.DialTimeout("tcp", "127.0.0.1:80", 1*time.Nanosecond)
		return err
	case "attempt failed":
		return errors.New("dial tcp 127.0.0.1:80: connectex: A connection attempt failed because the connected party did not properly respond after a period of time, or established connection failed because connected host has failed to respond")
	case "refused":
		return errors.New("dial tcp 127.0.0.1:80: connectex: No connection could be made because the target machine actively refused it")
	}
	return nil
}
