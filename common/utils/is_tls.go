package utils

import (
	"bufio"
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
	"yaklang.io/yaklang/common/gmsm/gmtls"
	"yaklang.io/yaklang/common/log"
	"yaklang.io/yaklang/common/utils/socksproxy"
	"yaklang.io/yaklang/common/yak/yaklib/codec"
)

func IsTLSService(addr string, proxies ...string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, err := net.DialTimeout("tcp", addr, time.Second*5)
	if err == nil {
		defer conn.Close()
		host, _, _ := ParseStringToHostPort(addr)
		tlsConn := tls.Client(conn, &tls.Config{InsecureSkipVerify: true, MinVersion: tls.VersionSSL30, ServerName: host})
		if err = tlsConn.HandshakeContext(ctx); err != nil {
			return false
		}
		return true
	}
	return false
}

var proxyDailer = net.Dialer{Timeout: 5 * time.Second}

func GetProxyConn(target string, proxy string, connectTimeout time.Duration) (net.Conn, error) {
	return getProxyConn(target, proxy, connectTimeout)
}
func GetProxyConnWithContext(ctx context.Context, target string, proxy string, connectTimeout time.Duration) (net.Conn, error) {
	return getProxyConnWithContext(ctx, target, proxy, connectTimeout)
}

func GetAutoProxyConn(target string, proxy string, connectTimeout time.Duration) (net.Conn, error) {
	if proxy != "" {
		return getProxyConn(target, proxy, connectTimeout)
	}
	proxy = GetProxyFromEnv()
	if proxy != "" {
		return getProxyConn(target, proxy, connectTimeout)
	}

	return net.DialTimeout("tcp", target, connectTimeout)
}

func GetAutoProxyConnWithTLS(target string, proxy string, connectTimeout time.Duration, c *tls.Config) (net.Conn, error) {
	if c == nil {
		c = NewDefaultTLSConfig()
	}
	if proxy != "" {
		return getProxyConnTLS(target, proxy, connectTimeout, c)
	}
	proxy = GetProxyFromEnv()
	if proxy != "" {
		return getProxyConnTLS(target, proxy, connectTimeout, c)
	}

	d := &net.Dialer{Timeout: connectTimeout}
	return tls.DialWithDialer(d, "tcp", target, c)
}

func GetAutoProxyConnWithGMTLS(target string, proxy string, connectTimeout time.Duration, c *gmtls.Config) (net.Conn, error) {
	if c == nil {
		c = NewDefaultGMTLSConfig()
	}
	if proxy != "" {
		return getProxyConnGMTLS(target, proxy, connectTimeout, c)
	}
	proxy = GetProxyFromEnv()
	if proxy != "" {
		return getProxyConnGMTLS(target, proxy, connectTimeout, c)
	}

	d := &net.Dialer{Timeout: connectTimeout}
	return gmtls.DialWithDialer(d, "tcp", target, c)
}

func FixProxy(i string) string {
	if i == "" {
		return ""
	}

	if !strings.Contains(i, "://") {
		var host, port, _ = ParseStringToHostPort(i)
		host = strings.Trim(host, `"' \r\n:`)
		if host != "" && port > 0 {
			return fmt.Sprintf("http://%v:%v", host, port)
		}
	}
	return i
}

func GetProxyFromEnv() string {
	for _, k := range []string{
		"YAK_PROXY", "yak_proxy",
		"HTTP_PROXY", "http_proxy",
		"HTTPS_PROXY", "https_proxy",
		"all_proxy", "all_proxy",
		"proxy", "proxy",
	} {
		if p := strings.Trim(os.Getenv(k), `"`); p != "" {
			return FixProxy(p)
		}
	}
	return ""
}

func TCPConnect(target string, timeout time.Duration, proxies ...string) (net.Conn, error) {
	dialer := &net.Dialer{Timeout: timeout}
	if len(proxies) <= 0 {
		return dialer.Dial("tcp", target)
	}

	for _, proxy := range proxies {
		conn, err := getProxyConn(target, proxy, timeout)
		if err != nil {
			log.Errorf("proxy conn failed: %s", err)
			continue
		}
		return conn, nil
	}
	return nil, Errorf("connect: %v failed: no proxy available", target)
}

func getProxyConnTLS(target string, proxy string, connectTimeout time.Duration, tlsConfig *tls.Config) (net.Conn, error) {
	conn, err := getProxyConn(target, proxy, connectTimeout)
	if err != nil {
		return nil, err
	}
	var tlsConn = tls.Client(conn, tlsConfig)
	timeoutCtx, cancel := context.WithTimeout(context.Background(), connectTimeout)
	defer cancel()
	err = tlsConn.HandshakeContext(timeoutCtx)
	if err != nil {
		return nil, err
	}
	return tlsConn, nil
}

func getProxyConnGMTLS(target string, proxy string, connectTimeout time.Duration, tlsConfig *gmtls.Config) (net.Conn, error) {
	conn, err := getProxyConn(target, proxy, connectTimeout)
	if err != nil {
		return nil, err
	}
	var tlsConn = gmtls.Client(conn, tlsConfig)
	timeoutCtx, cancel := context.WithTimeout(context.Background(), connectTimeout)
	defer cancel()
	err = tlsConn.HandshakeContext(timeoutCtx)
	if err != nil {
		return nil, err
	}
	return tlsConn, nil
}
func getProxyConn(target string, proxy string, connectTimeout time.Duration) (net.Conn, error) {
	return getProxyConnWithContext(context.Background(), target, proxy, connectTimeout)
}
func getProxyConnWithContext(ctx context.Context, target string, proxy string, connectTimeout time.Duration) (net.Conn, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	tcpDailer := &net.Dialer{Timeout: connectTimeout}
	proxy = strings.ToLower(proxy)
	host, port, _ := ParseStringToHostPort(proxy)
	if host == "" || port <= 0 {
		return nil, Errorf("proxy need host:port... at least[%v]", proxy)
	}

	proxyAddr := HostPort(host, port)
	switch true {
	case strings.HasPrefix(proxy, "https://"):
		urlIns, err := url.Parse(proxy)
		if err != nil {
			return nil, Errorf("parse proxy url failed: %s", err)
		}
		conn, err := tcpDailer.DialContext(ctx, "tcp", proxyAddr)
		if err != nil {
			return nil, err
		}
		conn = tls.Client(conn, NewDefaultTLSConfig())
		if urlIns.User != nil && urlIns.User.String() != "" {
			// 有密码
			_, _ = conn.Write(generateHTTPProxyConnectWithCredential(target, urlIns.User.String()))
		} else {
			// 无密码
			_, _ = conn.Write(generateHTTPProxyConnect(target))
		}
		if readHTTP200(conn) {
			return conn, nil
		}
		return nil, Errorf("connect proxy(https) [%s] failed", proxy)
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
		urlIns, err := url.Parse(proxy)
		if err != nil {
			return nil, Errorf("parse proxy url failed: %s", err)
		}

		conn, err := tcpDailer.DialContext(ctx, "tcp", proxyAddr)
		if err != nil {
			return nil, err
		}

		if urlIns.User != nil && urlIns.User.String() != "" {
			_, err = conn.Write(generateHTTPProxyConnectWithCredential(target, urlIns.User.String()))
		} else {
			_, err = conn.Write(generateHTTPProxyConnect(target))
		}
		if readHTTP200(conn) {
			return conn, nil
		}
		if err != nil {
			return nil, Errorf("connect proxy(http) [%s] failed: %s", proxy, err)
		}
		return nil, Errorf("connect proxy(http) [%s] failed", proxy)
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

func generateHTTPProxyConnectWithCredential(target string, cred string) []byte {
	return []byte(fmt.Sprintf(
		"CONNECT %v HTTP/1.1\r\nHost: %v\r\nUser-Agent: %v\r\n"+
			fmt.Sprintf("Proxy-Authorization: Basic %v\r\n", codec.EncodeBase64(cred))+
			"Connection: keep-alive\r\nProxy-Connection: keep-alive\r\n\r\n",
		target, target, userAgent,
	))
}

var userAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_13_4) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/75.0.3770.142 Safari/537.36"
