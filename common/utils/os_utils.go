package utils

import (
	"bufio"
	"context"
	"crypto/tls"
	"fmt"
	"math/rand"
	"net"
	"net/http"
	"os"
	"os/user"
	"strings"
	"sync"
	"time"

	ws "github.com/gorilla/websocket"
	"golang.org/x/net/websocket"

	"github.com/miekg/dns"
	"github.com/pkg/errors"
	"github.com/yaklang/yaklang/common/gmsm/gmtls"
	"github.com/yaklang/yaklang/common/log"
	"golang.org/x/net/http2"
)

type handleTCPFunc func(ctx context.Context, lis net.Listener, conn net.Conn)

func IsUDPPortAvailable(p int) bool {
	return IsPortAvailableWithUDP("127.0.0.1", p)
}

func IsTCPPortAvailable(p int) bool {
	return IsPortAvailable("127.0.0.1", p)
}

func GetRandomAvailableTCPPort() int {
RESET:
	lis, err := net.Listen("tcp", ":0")
	if err == nil {
		port := lis.Addr().(*net.TCPAddr).Port
		_ = lis.Close()
		return port
	} else {
		// fallback
		randPort := 55000 + rand.Intn(10000)
		if !IsTCPPortOpen("127.0.0.1", randPort) && IsTCPPortAvailable(randPort) {
			return randPort
		} else {
			goto RESET
		}
	}
}

func GetRangeAvailableTCPPort(startPort, endPort, maxRetries int) (int, error) {
	if startPort > endPort {
		return 0, Errorf("start port must be less than end port")
	}
	if endPort > 65535 {
		endPort = 65535
	}
	src := rand.NewSource(time.Now().UnixNano())
	r := rand.New(src)

	for i := 0; i < maxRetries; i++ {
		randPort := startPort + r.Intn(endPort-startPort+1)
		if !IsTCPPortOpen("127.0.0.1", randPort) && IsTCPPortAvailable(randPort) {
			return randPort, nil
		}
	}

	return 0, Errorf("unable to find an available port after %d retries", maxRetries)
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
	// return HostPort("127.0.0.1", 161)
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

func InGithubActions() bool {
	return os.Getenv("GITHUB_ACTIONS") != ""
}

func InTestcase() bool {
	if len(os.Args) > 0 {
		if strings.HasSuffix(strings.ToLower(os.Args[1]), ".test") {
			return true
		}
	}
	for _, v := range os.Args {
		if strings.Contains(v, "-test.v") {
			return true
		}
		if strings.Contains(v, "-test.run") {
			return true
		}
	}
	return false
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
			log.Errorf("mock http server serve failed: %s", err)
			return
		}
	}()
	err = WaitConnect(HostPort(host, port), 3)
	if err != nil {
		panic(err)
	}
	return "127.0.0.1", port
}

func DebugMockTCPHandlerFuncContext(ctx context.Context, handlerFunc handleTCPFunc) (string, int) {
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
		for {
			select {
			case <-ctx.Done():
				return
			default:
				conn, err := lis.Accept()
				if err != nil {
					log.Errorf("mock tcp server accept failed: %v", err)
					return
				}
				go handlerFunc(ctx, lis, conn)
			}
		}
	}()

	err = WaitConnect(HostPort(host, port), 3)
	if err != nil {
		panic(err)
	}
	return "127.0.0.1", port
}

func DebugMockTCP(rsp []byte) (string, int) {
	return DebugMockTCPHandlerFuncContext(TimeoutContext(time.Second*30), func(ctx context.Context, lis net.Listener, conn net.Conn) {
		_, err := conn.Write(rsp)
		if err != nil {
			log.Errorf("write tcp failed: %v", err)
		}
		_ = conn.(*net.TCPConn).CloseWrite()
		//_ = lis.Close()
	},
	)
}

func DebugMockWs(handler func(conn *websocket.Conn)) (string, int) {
	addr := GetRandomLocalAddr()

	go func() {
		server := &websocket.Server{
			Handler: websocket.Handler(handler),
			Handshake: func(config *websocket.Config, req *http.Request) error {
				// 不执行任何 Origin 检查
				return nil
			},
		}
		if err := http.ListenAndServe(addr, server); err != nil {
			log.Fatal("ListenAndServe:", err)
		}
	}()

	host, port, _ := ParseStringToHostPort(addr)
	return host, port
}

func DebugMockTCPEx(handleFunc handleTCPFunc) (string, int) {
	return DebugMockTCPHandlerFuncContext(TimeoutContext(time.Minute*5), handleFunc)
}

func DebugMockHTTP(rsp []byte) (string, int) {
	return DebugMockHTTPWithTimeout(time.Minute, rsp)
}

func DebugMockHTTPS(rsp []byte) (string, int) {
	return DebugMockHTTPServerWithContext(TimeoutContext(time.Minute), true, false, false, false, false, func(bytes []byte) []byte {
		return rsp
	})
}

func DebugMockHTTPEx(handle func(req []byte) []byte) (string, int) {
	return DebugMockHTTPServerWithContext(TimeoutContext(time.Minute*5), false, false, false, false, false, handle)
}

func DebugMockHTTPExContext(ctx context.Context, handle func(req []byte) []byte) (string, int) {
	return DebugMockHTTPServerWithContext(ctx, false, false, false, false, false, handle)
}

func DebugMockHTTPKeepAliveEx(handle func(req []byte) []byte) (string, int) {
	return DebugMockHTTPServerWithContext(TimeoutContext(time.Minute), false, false, false, false, true, handle)
}

func DebugMockHTTP2(ctx context.Context, handler func(req []byte) []byte) (string, int) {
	return DebugMockHTTPServerWithContext(ctx, true, true, false, false, false, handler)
}

func DebugMockGMHTTP(ctx context.Context, handler func(req []byte) []byte) (string, int) {
	return DebugMockHTTPServerWithContext(ctx, true, false, true, false, false, handler)
}

func DebugMockOnlyGMHTTP(ctx context.Context, handler func(req []byte) []byte) (string, int) {
	return DebugMockHTTPServerWithContext(ctx, true, false, false, true, false, handler)
}

func DebugMockHTTPSEx(handle func(req []byte) []byte) (string, int) {
	return DebugMockHTTPServerWithContext(TimeoutContext(time.Minute), true, false, false, false, false, handle)
}

func DebugMockHTTPSKeepAliveEx(handle func(req []byte) []byte) (string, int) {
	return DebugMockHTTPServerWithContext(TimeoutContext(time.Minute), true, false, false, false, true, handle)
}

var (
	tlsTestConfig       *tls.Config
	mtlsTestConfig      *tls.Config
	tlsTestOnce         sync.Once
	gmtlsTestConfig     *gmtls.Config
	onlyGmtlsTestConfig *gmtls.Config
	mgmtlsTestConfig    *gmtls.Config
	clientCrt           []byte
	clientKey           []byte
)

func RegisterDefaultTLSConfigGenerator(h func() (*tls.Config, *gmtls.Config, *gmtls.Config, *tls.Config, *gmtls.Config, []byte, []byte)) {
	go tlsTestOnce.Do(func() {
		tlsTestConfig, gmtlsTestConfig, onlyGmtlsTestConfig, mtlsTestConfig, mgmtlsTestConfig, clientCrt, clientKey = h()
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

func GetDefaultOnlyGMTLSConfig(i float64) *gmtls.Config {
	expectedEnd := time.Now().Add(FloatSecondDuration(i))
	for {
		if tlsTestConfig != nil {
			log.Infof("fetch default tls config finished: %p", tlsTestConfig)
			return onlyGmtlsTestConfig
		}
		time.Sleep(50 * time.Millisecond)
		if !expectedEnd.After(time.Now()) {
			break
		}
	}
	log.Error("fetch default tls config failed")
	return nil
}

func DebugMockHTTPServerWithContext(ctx context.Context, https, h2, gmtlsFlag, onlyGmtls, keepAlive bool, handle func([]byte) []byte) (string, int) {
	addr := GetRandomLocalAddr()
	return DebugMockHTTPServerWithContextWithAddress(ctx, addr, https, h2, gmtlsFlag, onlyGmtls, keepAlive, handle)
}

func DebugMockHTTPServerWithContextWithAddress(ctx context.Context, addr string, https, h2, gmtlsFlag, onlyGmtls, keepAlive bool, handle func([]byte) []byte) (string, int) {
	time.Sleep(300 * time.Millisecond)
	host, port, _ := ParseStringToHostPort(addr)
	go func() {
		var (
			lis net.Listener
			err error
		)
		if https && !h2 && !gmtlsFlag && !onlyGmtls {
			tlsConfig := GetDefaultTLSConfig(5)
			if tlsConfig == nil {
				panic(1)
			}
			lis, err = tls.Listen("tcp", addr, tlsConfig)
		} else if h2 {
			origin := GetDefaultTLSConfig(5)
			copied := *origin
			copied.NextProtos = []string{"h2"}
			lis, err = tls.Listen("tcp", addr, &copied)
		} else if gmtlsFlag {
			log.Infof("gmtlsFlag: %v", gmtlsFlag)
			lis, err = gmtls.Listen("tcp", addr, GetDefaultGMTLSConfig(5))
		} else if onlyGmtls {
			log.Infof("onlyGmtlsFlag: %v", onlyGmtls)
			lis, err = gmtls.Listen("tcp", addr, GetDefaultOnlyGMTLSConfig(5))
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
			err := http2.ConfigureServer(srv, &http2.Server{})
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

		if gmtlsFlag || onlyGmtls {
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
						req, err := ReadHTTPRequestFromBufioReader(bufio.NewReader(conn))
						if err != nil {
							log.Errorf("read http request failed: %v", err)
							conn.Close()
							return
						}
						raw, err := DumpHTTPRequest(req, true)
						if err != nil {
							conn.Close()
							return
						}
						conn.Write(handle(raw))
						if !keepAlive {
							time.Sleep(500 * time.Millisecond)
							conn.Close()
							return
						}
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
				time.Sleep(50 * time.Millisecond)
				c.(*net.TCPConn).CloseWrite() // FIN
				// c.Close() // RST
			}(conn)
		}
		lis.Close()
	}()

	time.Sleep(time.Millisecond * 100)
	return host, port
}

func DebugMockEchoWs(point string) (string, int) {
	addr := GetRandomLocalAddr()
	time.Sleep(time.Millisecond * 300)
	host, port, _ := ParseStringToHostPort(addr)

	upgrader := ws.Upgrader{
		ReadBufferSize:    1024,
		WriteBufferSize:   1024,
		EnableCompression: true, // 启用压缩
	}

	http.HandleFunc("/"+point, func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Println(err)
			return
		}
		defer conn.Close()

		for {
			mt, message, err := conn.ReadMessage()
			if err != nil && message == nil {
				log.Errorf("read: %v", err)
				return
			}
			serverMessage := []byte("server: " + string(message))
			if err := conn.WriteMessage(mt, serverMessage); err != nil {
				log.Errorf("write: %v", err)
				return
			}
		}
	})

	server := &http.Server{Addr: addr}

	go func() {
		err := server.ListenAndServe()
		if err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()

	return host, port
}
