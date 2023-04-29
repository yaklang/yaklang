/*
Copyright Suzhou Tongji Fintech Research Institute 2017 All Rights Reserved.
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

                 http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package gmtls

import (
	"bufio"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"yaklang/common/log"
	"yaklang/common/utils/socksproxy"
	"strconv"
	"strings"
	"sync"
	"time"
)

// SimpleRoundTripper 简单的单次HTTP/HTTPS（国密） 连接往返器
// 每次建立新的连接
type SimpleRoundTripper struct {
	lock      sync.Mutex
	tlsConfig *Config
	preferGM  bool
	gmSupport bool
	// Proxy specifies a function to return a proxy for a given
	// Request. If the function returns a non-nil error, the
	// request is aborted with the provided error.
	//
	// The proxy type is determined by the URL scheme. "http",
	// "https", and "socks5" are supported. If the scheme is empty,
	// "http" is assumed.
	//
	// If Proxy is nil or returns a nil *URL, no proxy is used.
	Proxy func(*http.Request) (*url.URL, error)
}

func NewSimpleRoundTripper(cfg *Config) *SimpleRoundTripper {
	return &SimpleRoundTripper{tlsConfig: cfg}
}

func NewSimpleRoundTripperWithProxy(cfg *Config, proxy func(*http.Request) (*url.URL, error)) *SimpleRoundTripper {
	return &SimpleRoundTripper{tlsConfig: cfg, Proxy: proxy}
}

func (s *SimpleRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	// 加锁保证线程安全
	s.lock.Lock()
	defer s.lock.Unlock()

	scheme := req.URL.Scheme
	isHTTP := scheme == "http" || scheme == "https"
	if !isHTTP {
		return nil, fmt.Errorf("仅支持http/https协议")
	}

	// 获取主机名 和 端口
	hostname := req.URL.Hostname()
	port := req.URL.Port()
	if port == "" {
		switch scheme {
		case "http":
			port = "80"
		case "https":
			port = "443"

		}
	}
	address := net.JoinHostPort(hostname, port)

	var conn io.ReadWriteCloser
	var err error
	var proxyURL *url.URL

	if s.Proxy != nil {
		proxyURL, err = s.Proxy(req)
	}

	if err != nil {
		return nil, err
	}

	if proxyURL == nil {
		// 根据协议建立连接
		if scheme == "http" {
			// HTTP 协议建立TCP连接
			conn, err = net.Dial("tcp", address)
			if err != nil {
				return nil, err
			}
		} else {
			// HTTPS 协议建立TLS连接
			conn, err = Dial("tcp", address, s.tlsConfig)
			if err != nil {
				return nil, err
			}
		}
	} else {
		conn, err = getAutoProxyConnWithGMTLS(address, proxyURL.String(), time.Second*10, s.tlsConfig)
		if err != nil {
			return nil, err
		}
	}
	//defer conn.Close()

	// 把请求写入连接中，发起请求
	err = req.Write(conn)
	if err != nil {
		return nil, err
	}
	// 从连接中读取
	response, err := http.ReadResponse(bufio.NewReader(conn), req)
	if err != nil {
		return nil, err
	}
	// 协议升级时，替换Body实现
	if response.StatusCode == http.StatusSwitchingProtocols {
		response.Body = conn
	}
	return response, nil
}

func getAutoProxyConnWithGMTLS(target string, proxy string, connectTimeout time.Duration, c *Config) (net.Conn, error) {
	if c == nil {
		c = NewDefaultGMTLSConfig()
	}
	if proxy != "" {
		return getProxyConnGMTLS(target, proxy, connectTimeout, c)
	}
	proxy = getProxyFromEnv()
	if proxy != "" {
		return getProxyConnGMTLS(target, proxy, connectTimeout, c)
	}

	d := &net.Dialer{Timeout: connectTimeout}
	return DialWithDialer(d, "tcp", target, c)
}

func fixProxy(i string) string {
	if i == "" {
		return ""
	}

	if !strings.Contains(i, "://") {
		var host, port, _ = parseStringToHostPort(i)
		host = strings.Trim(host, `"' \r\n:`)
		if host != "" && port > 0 {
			return fmt.Sprintf("http://%v:%v", host, port)
		}
	}
	return i
}

func getProxyFromEnv() string {
	for _, k := range []string{
		"YAK_PROXY", "yak_proxy",
		"HTTP_PROXY", "http_proxy",
		"HTTPS_PROXY", "https_proxy",
		"all_proxy", "all_proxy",
		"proxy", "proxy",
	} {
		if p := strings.Trim(os.Getenv(k), `"`); p != "" {
			return fixProxy(p)
		}
	}
	return ""
}

func getProxyConnGMTLS(target string, proxy string, connectTimeout time.Duration, tlsConfig *Config) (net.Conn, error) {
	conn, err := getProxyConn(target, proxy, connectTimeout)
	if err != nil {
		return nil, err
	}
	var tlsConn = Client(conn, tlsConfig)
	//t := time.Now()
	//err = tlsConn.SetReadDeadline(t.Add(connectTimeout))
	err = tlsConn.Handshake()
	if err != nil {
		return nil, err
	}
	return tlsConn, nil
}

func getProxyConn(target string, proxy string, connectTimeout time.Duration) (net.Conn, error) {
	tcpDailer := &net.Dialer{Timeout: connectTimeout}
	proxy = strings.ToLower(proxy)

	host, port, _ := parseStringToHostPort(proxy)
	if host == "" || port <= 0 {
		return nil, errorf("proxy need host:port... at least[%v]", proxy)
	}

	proxyAddr := hostPort(host, port)
	switch true {

	case strings.HasPrefix(proxy, "https://"):
		conn, err := tls.DialWithDialer(tcpDailer, "tcp", proxyAddr, &tls.Config{
			InsecureSkipVerify: true,
			MinVersion:         tls.VersionSSL30, // nolint[:staticcheck]
			MaxVersion:         tls.VersionTLS13,
		})
		if err != nil {
			return nil, err
		}
		_, _ = conn.Write(generateHTTPProxyConnect(target))
		if readHTTP200(conn) {
			return conn, nil
		}
		return nil, errorf("connect proxy(https) [%s] failed", proxy)
	case strings.HasPrefix(proxy, "socks://"):
		fallthrough
	case strings.HasPrefix(proxy, "socks5://"):
		fallthrough
	case strings.HasPrefix(proxy, "s5://"):
		username, password := parseProxyCredential(proxy)
		conn, err := socksproxy.DialSocksProxy(socksproxy.SOCKS5, proxyAddr, username, password)("tcp", target)
		if err != nil {
			return nil, err
		}
		return conn, nil
	case strings.HasPrefix(proxy, "s4://") || strings.HasPrefix(proxy, "socks4://"):
		username, password := parseProxyCredential(proxy)
		conn, err := socksproxy.DialSocksProxy(socksproxy.SOCKS4, proxyAddr, username, password)("tcp", target)
		if err != nil {
			return nil, err
		}
		return conn, nil
	case strings.HasPrefix(proxy, "s4a://") || strings.HasPrefix(proxy, "socks4a://"):
		username, password := parseProxyCredential(proxy)
		conn, err := socksproxy.DialSocksProxy(socksproxy.SOCKS4A, proxyAddr, username, password)("tcp", target)
		if err != nil {
			return nil, err
		}
		return conn, nil
	case strings.HasPrefix(proxy, "http://"):
		fallthrough
	default:
		conn, err := tcpDailer.Dial("tcp", proxyAddr)
		if err != nil {
			return nil, err
		}
		_, err = conn.Write(generateHTTPProxyConnect(target))
		if readHTTP200(conn) {
			return conn, nil
		}
		if err != nil {
			return nil, errorf("connect proxy(http) [%s] failed: %s", proxy, err)
		}
		return nil, errorf("connect proxy(http) [%s] failed", proxy)
	}
}

func parseProxyCredential(proxyURL string) (string, string) {
	urlIns, err := url.Parse(proxyURL)
	if err != nil {
		return "", ""
	}
	username := urlIns.User.Username()
	password, _ := urlIns.User.Password()
	return username, password
}

func readHTTP200(c net.Conn) bool {
	//rspBytes := StableReaderEx(c, 5*time.Second, 4096)
	//rsp, err := http.ReadResponse(bufio.NewReader(bytes.NewReader(rspBytes)), nil)
	rsp, err := http.ReadResponse(bufio.NewReader(c), nil)
	if err != nil {
		log.Errorf("read response(readHTTP200) failed: %s", err)
		return false
	}
	return rsp.StatusCode >= 200 && rsp.StatusCode < 400
}

func generateHTTPProxyConnect(target string) []byte {
	return []byte(fmt.Sprintf(
		"CONNECT %v HTTP/1.1\r\nHost: %v\r\nUser-Agent: %v\r\nConnection: keep-alive\r\nProxy-Connection: keep-alive\r\n\r\n",
		target, target, userAgent,
	))
}

var userAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_13_4) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/75.0.3770.142 Safari/537.36"

func parseStringToHostPort(raw string) (host string, port int, err error) {
	if strings.Contains(raw, "://") {
		urlObject, _ := url.Parse(raw)
		if urlObject != nil {
			// 处理 URL
			portRaw := urlObject.Port()
			portInt64, err := strconv.ParseInt(portRaw, 10, 32)
			if err != nil || portInt64 <= 0 {
				switch urlObject.Scheme {
				case "http", "ws":
					port = 80
				case "https", "wss":
					port = 443
				}
			} else {
				port = int(portInt64)
			}

			host = urlObject.Hostname()
			err = nil
			return host, port, err
		}
	}

	host = stripPort(raw)
	portStr := portOnly(raw)
	if len(portStr) <= 0 {
		return "", 0, errorf("unknown port for [%s]", raw)
	}

	portStr = strings.TrimSpace(portStr)
	portInt64, err := strconv.ParseInt(portStr, 10, 32)
	if err != nil {
		return "", 0, errorf("%s parse port(%s) failed: %s", raw, portStr, err)
	}

	port = int(portInt64)
	err = nil
	return
}

func errorf(origin string, args ...interface{}) error {
	return errors.New(fmt.Sprintf(origin, args...))
}
func stripPort(hostport string) string {
	colon := strings.IndexByte(hostport, ':')
	if colon == -1 {
		return hostport
	}
	if i := strings.IndexByte(hostport, ']'); i != -1 {
		return strings.TrimPrefix(hostport[:i], "[")
	}
	return hostport[:colon]
}

func portOnly(hostport string) string {
	colon := strings.IndexByte(hostport, ':')
	if colon == -1 {
		return ""
	}
	if i := strings.Index(hostport, "]:"); i != -1 {
		return hostport[i+len("]:"):]
	}
	if strings.Contains(hostport, "]") {
		return ""
	}
	return hostport[colon+len(":"):]
}

func hostPort(host string, port interface{}) string {
	return fmt.Sprintf("%v:%v", parseHostToAddrString(host), port)
}

func parseHostToAddrString(host string) string {
	ip := net.ParseIP(host)
	if ip == nil {
		return host
	}

	if ret := ip.To4(); ret == nil {
		return fmt.Sprintf("[%v]", ip.String())
	}

	return host
}

func NewDefaultGMTLSConfig() *Config {
	return &Config{
		InsecureSkipVerify: true,
		GMSupport:          &GMSupport{},
	}
}
