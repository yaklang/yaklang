package utils

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"github.com/miekg/dns"
	"github.com/pkg/errors"
	"github.com/yaklang/yaklang/common/gmsm/gmtls"
	"github.com/yaklang/yaklang/common/log"
	"golang.org/x/net/http2"
	"io"
	"math/rand"
	"net"
	"net/http"
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
		log.Errorf("%s is unavailable: %s", addr, err)
		return false
	}
	defer func() {
		_ = lis.Close()
	}()
	return true
}

func GetRandomLocalAddr() string {
	return HostPort("127.0.0.1", GetRandomAvailableTCPPort())
	//return HostPort("127.0.0.1", 161)
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

func EnableDebug() {
	os.Setenv("YAKLANGDEBUG", "1")
}

func DebugMockHTTPHandlerFunc(handlerFunc http.HandlerFunc) (string, int) {
	return DebugMockHTTPHandlerFuncContext(TimeoutContext(time.Minute*5), handlerFunc)
}

func DebugMockHTTPHandlerFuncContext(ctx context.Context, handlerFunc http.HandlerFunc) (string, int) {
	host := "127.0.0.1"
	port := GetRandomAvailableTCPPort()
	lis, err := net.Listen("tcp", HostPort(host, port))
	if err != nil {
		panic(err)
	}
	go func() {
		select {
		case <-ctx.Done():
		}
		lis.Close()
	}()
	go func() {
		server := &http.Server{
			Addr:    HostPort(host, port),
			Handler: handlerFunc,
		}
		err := server.Serve(lis)
		if err != nil {
			panic(err)
		}
	}()
	err = WaitConnect(HostPort(host, port), 3)
	if err != nil {
		panic(err)
	}
	return host, port
}

func DebugMockHTTP(rsp []byte) (string, int) {
	return DebugMockHTTPWithTimeout(time.Minute, rsp)
}

func DebugMockHTTPEx(handle func(req []byte) []byte) (string, int) {
	return DebugMockHTTPServerWithContext(TimeoutContext(time.Minute*5), false, false, false, handle)
}

func DebugMockHTTPExContext(ctx context.Context, handle func(req []byte) []byte) (string, int) {
	return DebugMockHTTPServerWithContext(ctx, false, false, false, handle)
}

func DebugMockHTTP2(ctx context.Context, handler func(req []byte) []byte) (string, int) {
	return DebugMockHTTPServerWithContext(ctx, true, true, false, handler)
}

func DebugMockGMHTTP(ctx context.Context, handler func(req []byte) []byte) (string, int) {
	return DebugMockHTTPServerWithContext(ctx, true, false, false, handler)
}

func DebugMockHTTPSEx(handle func(req []byte) []byte) (string, int) {
	return DebugMockHTTPServerWithContext(TimeoutContext(time.Minute), true, false, false, handle)
}

var (
	tlsTestConfig    *tls.Config
	mtlsTestConfig   *tls.Config
	tlsTestOnce      sync.Once
	gmtlsTestConfig  *gmtls.Config
	mgmtlsTestConfig *gmtls.Config
	clientCrt        []byte
	clientKey        []byte
)

func RegisterDefaultTLSConfigGenerator(h func() (*tls.Config, *gmtls.Config, *tls.Config, *gmtls.Config, []byte, []byte)) {
	go tlsTestOnce.Do(func() {
		tlsTestConfig, gmtlsTestConfig, mtlsTestConfig, mgmtlsTestConfig, clientCrt, clientKey = h()
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

func GetDefaultGMTLSConfig(i float64) *gmtls.Config {
	expectedEnd := time.Now().Add(FloatSecondDuration(i))
	for {
		if tlsTestConfig != nil {
			log.Infof("fetch default tls config finished: %p", tlsTestConfig)
			return gmtlsTestConfig
		}
		time.Sleep(50 * time.Millisecond)
		if !expectedEnd.After(time.Now()) {
			break
		}
	}
	log.Error("fetch default tls config failed")
	return nil
}

func DebugMockHTTPServerWithContext(ctx context.Context, https bool, h2 bool, gmtlsFlag bool, handle func([]byte) []byte) (string, int) {
	addr := GetRandomLocalAddr()
	time.Sleep(300 * time.Millisecond)
	var host, port, _ = ParseStringToHostPort(addr)
	go func() {
		var (
			lis net.Listener
			err error
		)
		if https && !h2 && !gmtlsFlag {
			tlsConfig := GetDefaultTLSConfig(5)
			if tlsConfig == nil {
				panic(1)
			}
			lis, err = tls.Listen("tcp", addr, tlsConfig)
		} else if h2 {
			origin := GetDefaultTLSConfig(5)
			var copied = *origin
			copied.NextProtos = []string{"h2"}
			lis, err = tls.Listen("tcp", addr, &copied)
		} else if gmtlsFlag {
			log.Infof("gmtlsFlag: %v", gmtlsFlag)
			lis, err = gmtls.Listen("tcp", addr, GetDefaultGMTLSConfig(5))
		} else {
			lis, err = net.Listen("tcp", addr)
		}
		if err != nil {
			panic(err)
		}
		go func() {
			select {
			case <-ctx.Done():
			}
			lis.Close()
		}()

		if h2 {
			if !https {
				log.Error("h2 only support https")
			}

			srv := &http.Server{Addr: HostPort(host, port), Handler: http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				r, err := HttpDumpWithBody(request, true)
				if err != nil {
					log.Error(err)
					return
				}
				fmt.Println(string(r))
				if handle != nil {
					raw := handle(r)
					writer.Write(raw)
					return
				}
				writer.Write([]byte("HELLO HTTP2"))
			})}
			var err = http2.ConfigureServer(srv, &http2.Server{})
			if err != nil {
				log.Error(err)
				return
			}
			go func() {
				log.Infof("START TO SERVE HTTP2")
				srv.Serve(lis)
			}()
			return
		}

		if gmtlsFlag {
			if !https {
				log.Error("gmtls only support https")
			}

			srv := &http.Server{Addr: HostPort(host, port), Handler: http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				r, err := HttpDumpWithBody(request, true)
				if err != nil {
					log.Error(err)
					return
				}
				fmt.Println(string(r))
				if handle != nil {
					raw := handle(r)
					writer.Write(raw)
					return
				}
				writer.Write([]byte("HELLO GMTLS"))
			})}

			go func() {
				log.Infof("START TO SERVE GMTLS HTTP SERVER")
				srv.Serve(lis)
			}()
			return
		}

		// http / tls
		for {
			conn, err := lis.Accept()
			if err != nil {
				log.Error(err)
				break
			}
			go func() {
				ctx := TimeoutContextSeconds(10)
				for {
					select {
					case <-ctx.Done():
						conn.Close()
						return
					default:
						conn.SetReadDeadline(time.Now().Add(10 * time.Second))
						var buf bytes.Buffer
						req, _ := http.ReadRequest(bufio.NewReader(io.TeeReader(conn, &buf)))
						if req != nil && req.Body != nil {
							_, _ = io.ReadAll(req.Body)
						}
						reqBytes := buf.Bytes()
						if len(reqBytes) == 0 {
							conn.Close()
							return
						}
						conn.Write(handle(reqBytes))
						time.Sleep(50 * time.Millisecond)
						conn.Close()
						return
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
	host, port, _ := ParseStringToHostPort(addr)

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
				time.Sleep(50 * time.Millisecond)
				c.Write(rsp)
				c.Close()
			}(conn)
		}
		lis.Close()
	}()

	time.Sleep(time.Millisecond * 100)
	return host, port
}
