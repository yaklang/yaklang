package netx

import (
	"context"
	"fmt"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"net"
	"net/url"
	"os"
	"strings"
	"time"
)

func DialTCPTimeoutForceProxy(timeout time.Duration, target string, proxy string) (net.Conn, error) {
	return connectForceProxy(nil, target, proxy, timeout)
}

func FixProxy(i string) string {
	if i == "" {
		return ""
	}

	if !strings.Contains(i, "://") {
		var host, port, _ = utils.ParseStringToHostPort(i)
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
			return FixProxy(p)
		}
	}
	return ""
}

func UnsetProxyFromEnv() {
	for _, k := range []string{
		"YAK_PROXY", "yak_proxy",
		"HTTP_PROXY", "http_proxy",
		"HTTPS_PROXY", "https_proxy",
		"all_proxy", "all_proxy",
		"proxy", "proxy",
	} {
		os.Unsetenv(k)
	}
}

func DialContext(ctx context.Context, target string, proxies ...string) (net.Conn, error) {
	if proxies := utils.StringArrayFilterEmpty(proxies); len(proxies) <= 0 {
		return DialContextWithoutProxy(ctx, "tcp", target)
	} else {
		for _, proxy := range proxies {
			conn, err := getConnForceProxyContext(ctx, target, proxy)
			if err != nil {
				log.Errorf("proxy %v conn failed: %s", err, proxy)
				continue
			}
			return conn, nil
		}
		return nil, utils.Errorf("connect: %v failed: no proxy available (in %v)", target, proxies)
	}
}

func getConnForceProxy(target string, proxy string, connectTimeout time.Duration) (net.Conn, error) {
	return connectForceProxy(context.Background(), target, proxy, connectTimeout)
}

func getConnForceProxyContext(ctx context.Context, target, proxy string) (net.Conn, error) {
	return connectForceProxy(ctx, target, proxy, 10*time.Second)
}

func connectForceProxy(ctx context.Context, target string, proxy string, connectTimeout time.Duration) (net.Conn, error) {
	// use dialx
	// remember disallow proxy!!!
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, _ = context.WithTimeout(ctx, connectTimeout)

	host, port, _ := utils.ParseStringToHostPort(proxy)
	if host == "" || port <= 0 {
		return nil, utils.Errorf("proxy need host:port... at least[%v]", proxy)
	}

	proxyAddr := utils.HostPort(host, port)
	switch true {
	case utils.IHasPrefix(proxy, "https://"):
		urlIns, err := url.Parse(proxy)
		if err != nil {
			return nil, utils.Errorf("parse proxy url failed: %s", err)
		}
		conn, err := DialX(
			proxyAddr,
			DialX_WithDisableProxy(true),
			DialX_WithTLS(true),
		)
		if err != nil {
			return nil, err
		}
		if urlIns.User != nil && urlIns.User.String() != "" {
			// 有密码
			_, _ = conn.Write(generateHTTPProxyConnectWithCredential(target, urlIns.User.String()))
		} else {
			// 无密码
			_, _ = conn.Write(generateHTTPProxyConnect(target))
		}
		if isHTTPConnectWork(conn) {
			return conn, nil
		}
		conn.Close()
		return nil, utils.Errorf("connect proxy(https) [%s] failed", proxy)
	case utils.IHasPrefix(proxy, "socks://"):
		fallthrough
	case utils.IHasPrefix(proxy, "socks5://"):
		fallthrough
	case utils.IHasPrefix(proxy, "s5://"):
		username, password := parseProxyCredential(proxy)
		conn, err := DialSocksProxy(SOCKS5, proxyAddr, username, password)("tcp", target)
		if err != nil {
			return nil, err
		}
		return conn, nil
	case utils.IHasPrefix(proxy, "s4://") || utils.IHasPrefix(proxy, "socks4://"):
		username, password := parseProxyCredential(proxy)
		conn, err := DialSocksProxy(SOCKS4, proxyAddr, username, password)("tcp", target)
		if err != nil {
			return nil, err
		}
		return conn, nil
	case utils.IHasPrefix(proxy, "s4a://") || utils.IHasPrefix(proxy, "socks4a://"):
		username, password := parseProxyCredential(proxy)
		conn, err := DialSocksProxy(SOCKS4A, proxyAddr, username, password)("tcp", target)
		if err != nil {
			return nil, err
		}
		return conn, nil
	case utils.IHasPrefix(proxy, "http://"):
		fallthrough
	default:
		urlIns, err := url.Parse(proxy)
		if err != nil {
			return nil, utils.Errorf("parse proxy url failed: %s", err)
		}

		// conn, err := DialContextWithoutProxy(ctx, "tcp", proxyAddr)
		conn, err := DialX(proxyAddr, DialX_WithTLS(false), DialX_WithDisableProxy(true))
		if err != nil {
			return nil, err
		}

		if urlIns.User != nil && urlIns.User.String() != "" {
			_, err = conn.Write(generateHTTPProxyConnectWithCredential(target, urlIns.User.String()))
		} else {
			_, err = conn.Write(generateHTTPProxyConnect(target))
		}
		if isHTTPConnectWork(conn) {
			return conn, nil
		}
		conn.Close()
		if err != nil {
			return nil, utils.Errorf("connect proxy(http) [%s] failed: %s", proxy, err)
		}
		return nil, utils.Errorf("connect proxy(http) [%s] failed", proxy)
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

func isHTTPConnectWork(c net.Conn) bool {
	firstLine, err := utils.ReadConnUntil(c, 5*time.Second, '\n')
	if err != nil {
		return false
	}
	_, code, _, _ := utils.ParseHTTPResponseLine(strings.TrimSpace(string(firstLine)))
	if code < 200 || code > 400 {
		return false
	}
	for {
		line, err := utils.ReadConnUntil(c, 5*time.Second, '\n')
		if err != nil {
			return false
		}
		lineStr := string(line)
		k, v, ok := strings.Cut(lineStr, ":")
		if ok {
			switch strings.ToLower(strings.TrimSpace(k)) {
			case "content-length":
				if codec.Atoi(strings.TrimSpace(v)) != 0 {
					return false
				}
			case "transfer-encoding":
				return false
			}
		}
		if strings.TrimSuffix(lineStr, "\r\n") == "" || strings.TrimSuffix(lineStr, "\n") == "" {
			break
		}
	}
	return true
}

func generateHTTPProxyConnect(target string) []byte {
	return []byte(fmt.Sprintf(
		"CONNECT %v HTTP/1.1\r\nHost: %v\r\nUser-Agent: %v\r\nConnection: keep-alive\r\nProxy-Connection: keep-alive\r\n\r\n",
		target, target, DefaultUserAgent,
	))
}

func generateHTTPProxyConnectWithCredential(target string, cred string) []byte {
	return []byte(fmt.Sprintf(
		"CONNECT %v HTTP/1.1\r\nHost: %v\r\nUser-Agent: %v\r\n"+
			fmt.Sprintf("Proxy-Authorization: Basic %v\r\n", codec.EncodeBase64(cred))+
			"Connection: keep-alive\r\nProxy-Connection: keep-alive\r\n\r\n",
		target, target, DefaultUserAgent,
	))
}

const DefaultUserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_13_4) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/75.0.3770.142 Safari/537.36"
