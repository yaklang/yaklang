package utils

import (
	"crypto/tls"
	"fmt"
	"github.com/miekg/dns"
	"github.com/pkg/errors"
	"github.com/yaklang/yaklang/common/log"
	"math/rand"
	"net"
	"os"
	"os/user"
	"sync"
	"time"
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

func IsUDPPortAvailable(p int) bool {
	return IsPortAvailableWithUDP("0.0.0.0", p)
}

func IsTCPPortAvailable(p int) bool {
	return IsPortAvailable("0.0.0.0", p)
}

func GetRandomAvailableTCPPort() int {
RESET:
	randPort := 55000 + rand.Intn(10000)
	if !IsTCPPortOpen("127.0.0.1", randPort) && IsTCPPortAvailable(randPort) {
		return randPort
	} else {
		goto RESET
	}
}

func GetRandomAvailableUDPPort() int {
RESET:
	randPort := 55000 + rand.Intn(10000)
	if IsUDPPortAvailable(randPort) {
		return randPort
	} else {
		goto RESET
	}
}

func IsUDPPortAvailableWithLoopback(p int) bool {
	return IsPortAvailableWithUDP("127.0.0.1", p)
}

func IsTCPPortAvailableWithLoopback(p int) bool {
	return IsPortAvailable("127.0.0.1", p)
}

func IsPortAvailable(host string, p int) bool {
	lis, err := net.Listen("tcp", HostPort(host, p))
	if err != nil {
		return false
	}
	_ = lis.Close()
	return true
}

func IsTCPPortOpen(host string, p int) bool {
	dialer := net.Dialer{}
	dialer.Timeout = 10 * time.Second
	conn, err := dialer.Dial("tcp", HostPort(host, p))
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

func IsPortAvailableWithUDP(host string, p int) bool {
	addr := fmt.Sprintf("%s:%v", host, p)
	lis, err := net.ListenPacket("udp", addr)
	if err != nil {
		log.Infof("%s is unavailable: %s", addr, err)
		return false
	}
	defer func() {
		_ = lis.Close()
	}()
	return true
}

func GetRandomLocalAddr() string {
	return HostPort("127.0.0.1", GetRandomAvailableTCPPort())
}

func GetSystemNameServerList() ([]string, error) {
	client, err := dns.ClientConfigFromFile("/etc/resolv.conf")
	if err != nil {
		return nil, errors.Errorf("get system nameserver list failed: %s", err)
	}
	return client.Servers, nil
}

func GetHomeDir() (string, error) {
	h, _ := os.UserHomeDir()
	if h != "" {
		return h, nil
	}

	homeDir := os.Getenv("HOME")
	if homeDir == "" {
		usr, err := user.Current()
		if err != nil {
			return "", errors.Errorf("get os use failed: %s", err)
		} else {
			homeDir = usr.HomeDir
		}
	}
	return homeDir, nil
}

func GetHomeDirDefault(d string) string {
	home, err := GetHomeDir()
	if err != nil {
		return d
	}
	return home
}

func InDebugMode() bool {
	return os.Getenv("DEBUG") != "" || os.Getenv("PALMDEBUG") != "" || os.Getenv("YAKLANGDEBUG") != ""
}

func Debug(f func()) {
	if InDebugMode() {
		f()
	}
}

func DebugMockHTTP(rsp []byte) (string, int) {
	return DebugMockHTTPWithTimeout(time.Minute, rsp)
}

func DebugMockHTTPEx(handle func(req []byte) []byte) (string, int) {
	return DebugMockHTTPExWithTimeout(time.Minute, false, handle)
}

func DebugMockHTTPSEx(handle func(req []byte) []byte) (string, int) {
	return DebugMockHTTPExWithTimeout(time.Minute, true, handle)
}

var (
	tlsTestConfig *tls.Config
	tlsTestOnce   sync.Once
)

func RegisterDefaultTLSConfigGenerator(h func() *tls.Config) {
	go tlsTestOnce.Do(func() {
		tlsTestConfig = h()
	})
}

func GetDefaultTLSConfig(i float64) *tls.Config {
	expectedEnd := time.Now().Add(FloatSecondDuration(i))
	for {
		if tlsTestConfig != nil {
			log.Infof("fetch default tls config finished: %p", tlsTestConfig)
			return tlsTestConfig
		}
		time.Sleep(50 * time.Millisecond)
		if !expectedEnd.After(time.Now()) {
			break
		}
	}
	log.Error("fetch default tls config failed")
	return nil
}

func DebugMockHTTPExWithTimeout(du time.Duration, https bool, handle func([]byte) []byte) (string, int) {
	addr := GetRandomLocalAddr()
	time.Sleep(300 * time.Millisecond)
	var host, port, _ = ParseStringToHostPort(addr)
	go func() {
		var (
			lis net.Listener
			err error
		)
		if https {
			tlsConfig := GetDefaultTLSConfig(5)
			if tlsConfig == nil {
				panic(1)
			}
			lis, err = tls.Listen("tcp", addr, tlsConfig)
		} else {
			lis, err = net.Listen("tcp", addr)
		}
		if err != nil {
			panic(err)
		}
		go func() {
			time.Sleep(du)
			lis.Close()
		}()

		for {
			conn, err := lis.Accept()
			if err != nil {
				break
			}
			go func() {
				ctx := TimeoutContextSeconds(10)
				for {
					select {
					case <-ctx.Done():
						conn.Close()
					default:
						data := StableReaderEx(conn, 10*time.Second, 10240)
						conn.Write(handle(data))
						time.Sleep(50 * time.Millisecond)
						conn.Close()
					}
				}
			}()
		}
		lis.Close()
	}()
	_ = WaitConnect(addr, 3.0)
	return host, port
}

func DebugMockHTTPWithTimeout(du time.Duration, rsp []byte) (string, int) {
	addr := GetRandomLocalAddr()
	time.Sleep(time.Millisecond * 300)
	var host, port, _ = ParseStringToHostPort(addr)
	go func() {
		lis, err := net.Listen("tcp", addr)
		if err != nil {
			panic(err)
		}
		go func() {
			time.Sleep(du)
			lis.Close()
		}()

		for {
			conn, err := lis.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				c.Write(rsp)
				time.Sleep(50 * time.Millisecond)
				c.Close()
			}(conn)
		}
		lis.Close()
	}()
	time.Sleep(time.Millisecond * 100)
	return host, port
}
