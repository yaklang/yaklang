package pingutil

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"
)

type pingTestCase struct {
	name   string
	ip     string
	config []PingConfigOpt
	expect bool
}

func TestPingAutoConfig(t *testing.T) {
	const timeout = 50 * time.Millisecond
	testCase := []pingTestCase{
		{
			name: "tcp timeout err test case",
			ip:   "127.0.0.1",
			config: []PingConfigOpt{
				WithTimeout(timeout),
				WithForceTcpPing(),
				WithTcpDialHandler(tcpTimeoutHandlerMaker(getTestTimeout("timeout"))),
			},
			expect: false,
		},
		{
			name: "tcp attempt failed err test case",
			ip:   "127.0.0.1",
			config: []PingConfigOpt{
				WithTimeout(timeout),
				WithForceTcpPing(),
				WithTcpDialHandler(tcpTimeoutHandlerMaker(getTestTimeout("attempt failed"))),
			},
			expect: false,
		},
		{
			name: "tcp refused err test case",
			ip:   "127.0.0.1",
			config: []PingConfigOpt{
				WithTimeout(timeout),
				WithForceTcpPing(),
				WithTcpDialHandler(tcpTimeoutHandlerMaker(getTestTimeout("refused"))),
			},
			expect: true,
		},
		{
			name: "native handler timeout",
			ip:   "127.0.0.1",
			config: []PingConfigOpt{
				WithTimeout(timeout),
				WithPingNativeHandler(pingSleepHandlerMaker()),
			},
			expect: false,
		},
	}
	for _, test := range testCase {
		t.Run(test.name, func(t *testing.T) {
			start := time.Now()
			res := PingAutoConfig(test.ip, test.config...)
			if elapsed := time.Since(start); elapsed > time.Second {
				t.Fatalf("probe exceeded timeout %v: elapsed %v", timeout, elapsed)
			}
			if res.Ok != test.expect {
				t.Fatalf("Expect %v but get %v", test.expect, res.Ok)
			}
		})
	}
}

func tcpTimeoutHandlerMaker(err error) func(ctx context.Context, addr string, proxies ...string) (net.Conn, error) {

	return func(ctx context.Context, addr string, proxies ...string) (net.Conn, error) {
		return nil, err
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

func getTestTimeout(errName string) error {
	switch errName {
	case "timeout":
		return context.DeadlineExceeded
	case "attempt failed":
		return errors.New("dial tcp 127.0.0.1:80: connectex: A connection attempt failed because the connected party did not properly respond after a period of time, or established connection failed because connected host has failed to respond")
	case "refused":
		return errors.New("dial tcp 127.0.0.1:80: connectex: No connection could be made because the target machine actively refused it")
	}
	return nil
}
