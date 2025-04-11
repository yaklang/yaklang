package testutils

import (
	"bufio"
	"context"
	"crypto/tls"
	"fmt"
	"github.com/yaklang/yaklang/common/gmsm/gmtls"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/tlsutils"
	"golang.org/x/net/http2"
	"net"
	"net/http"
	"time"
)

func DebugMockHTTPHandlerFunc(handlerFunc http.HandlerFunc) (string, int) {
	return DebugMockHTTPHandlerFuncContext(utils.TimeoutContext(time.Minute*5), handlerFunc)
}

func DebugMockHTTPHandlerFuncContext(ctx context.Context, handlerFunc http.HandlerFunc) (string, int) {
	host := "127.0.0.1"
	port := utils.GetRandomAvailableTCPPort()
	lis, err := net.Listen("tcp", utils.HostPort(host, port))
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
			Addr:    utils.HostPort(host, port),
			Handler: handlerFunc,
		}
		err := server.Serve(lis)
		if err != nil {
			log.Errorf("mock http server serve failed: %s", err)
			return
		}
	}()
	err = utils.WaitConnect(utils.HostPort(host, port), 3)
	if err != nil {
		panic(err)
	}
	return "127.0.0.1", port
}

func DebugMockHTTP(rsp []byte) (string, int) {
	return DebugMockHTTPWithTimeout(time.Minute, rsp)
}

func DebugMockHTTPS(rsp []byte) (string, int) {
	return DebugMockHTTPServerWithContext(utils.TimeoutContext(time.Minute), true, false, false, false, false, func(bytes []byte) []byte {
		return rsp
	})
}

func DebugMockHTTPEx(handle func(req []byte) []byte) (string, int) {
	return DebugMockHTTPServerWithContext(utils.TimeoutContext(time.Minute*5), false, false, false, false, false, handle)
}

func DebugMockHTTPExContext(ctx context.Context, handle func(req []byte) []byte) (string, int) {
	return DebugMockHTTPServerWithContext(ctx, false, false, false, false, false, handle)
}

func DebugMockHTTPKeepAliveEx(handle func(req []byte) []byte) (string, int) {
	return DebugMockHTTPServerWithContext(utils.TimeoutContext(time.Minute), false, false, false, false, true, handle)
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
	return DebugMockHTTPServerWithContext(utils.TimeoutContext(time.Minute), true, false, false, false, false, handle)
}

func DebugMockHTTPSKeepAliveEx(handle func(req []byte) []byte) (string, int) {
	return DebugMockHTTPServerWithContext(utils.TimeoutContext(time.Minute), true, false, false, false, true, handle)
}

func DebugMockHTTPServerWithContext(ctx context.Context, https, h2, gmtlsFlag, onlyGmtls, keepAlive bool, handle func([]byte) []byte) (string, int) {
	addr := utils.GetRandomLocalAddr()
	return DebugMockHTTPServerWithContextWithAddress(ctx, addr, https, h2, gmtlsFlag, onlyGmtls, keepAlive, false, handle)
}

func DebugMockHTTPServerWithContextWithAddress(ctx context.Context, addr string, https, h2, gmtlsFlag, onlyGmtls, keepAlive bool, checkServerName bool, handle func([]byte) []byte) (string, int) {
	time.Sleep(300 * time.Millisecond)
	host, port, _ := utils.ParseStringToHostPort(addr)
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
			if checkServerName {
				tlsutils.TLSConfigSetCheckServerName(tlsConfig, host)
			}
			lis, err = tls.Listen("tcp", addr, tlsConfig)
		} else if h2 {
			origin := GetDefaultTLSConfig(5)
			if checkServerName {
				tlsutils.TLSConfigSetCheckServerName(origin, host)
			}
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

			srv := &http.Server{Addr: utils.HostPort(host, port), Handler: http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				r, err := utils.HttpDumpWithBody(request, true)
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

			srv := &http.Server{Addr: utils.HostPort(host, port), Handler: http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				r, err := utils.HttpDumpWithBody(request, true)
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
			utils.TCPNoDelay(conn)
			if err != nil {
				log.Error(err)
				break
			}
			go func() {
				ctx := utils.TimeoutContextSeconds(10)
				for {
					select {
					case <-ctx.Done():
						conn.Close()
						return
					default:
						conn.SetReadDeadline(time.Now().Add(10 * time.Second))
						req, err := utils.ReadHTTPRequestFromBufioReader(bufio.NewReader(conn))
						if err != nil {
							log.Errorf("read http request failed: %v", err)
							conn.Close()
							return
						}
						raw, err := utils.DumpHTTPRequest(req, true)
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

	err := utils.WaitConnect(addr, 3)
	if err != nil {
		panic(err)
	}
	return host, port
}

func DebugMockHTTPWithTimeout(du time.Duration, rsp []byte) (string, int) {
	addr := utils.GetRandomLocalAddr()
	time.Sleep(time.Millisecond * 300)
	host, port, _ := utils.ParseStringToHostPort(addr)

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
			utils.TCPNoDelay(conn)
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

	err := utils.WaitConnect(addr, 3)
	if err != nil {
		panic(err)
	}
	return host, port
}
