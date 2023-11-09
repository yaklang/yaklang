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

var ErrorProxyAuthFailed = utils.Error("invalid proxy username or password")

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
	username, password := parseProxyCredential(proxy)
	credential := fmt.Sprintf("%s:%s", username, password)

	switch true {
	case utils.IHasPrefix(proxy, "https://"):
		return httpProxyDial(proxyAddr, username, credential, proxy, target, true)
	case utils.IHasPrefix(proxy, "socks://"):
		fallthrough
	case utils.IHasPrefix(proxy, "socks5://"):
		fallthrough
	case utils.IHasPrefix(proxy, "s5://"):
		conn, err := DialSocksProxy(SOCKS5, proxyAddr, username, password)("tcp", target)
		if err != nil {
			return nil, err
		}
		return conn, nil
	case utils.IHasPrefix(proxy, "s4://") || utils.IHasPrefix(proxy, "socks4://"):
		conn, err := DialSocksProxy(SOCKS4, proxyAddr, username, password)("tcp", target)
		if err != nil {
			return nil, err
		}
		return conn, nil
	case utils.IHasPrefix(proxy, "s4a://") || utils.IHasPrefix(proxy, "socks4a://"):
		conn, err := DialSocksProxy(SOCKS4A, proxyAddr, username, password)("tcp", target)
		if err != nil {
			return nil, err
		}
		return conn, nil
	case utils.IHasPrefix(proxy, "http://"):
		fallthrough
	default:
		return httpProxyDial(proxyAddr, username, credential, proxy, target, false)
	}
}

func httpProxyDial(proxyAddr string, username string, credential string, proxy string, target string, https bool) (net.Conn, error) {
	httpsString := "https"
	if https {
		httpsString = "http"
	}
	conn, err := DialX(
		proxyAddr,
		DialX_WithDisableProxy(true),
		DialX_WithTLS(https),
	)
	if err != nil {
		return nil, err
	}
	if username != "" {
		// 有密码
		_, _ = conn.Write(generateHTTPProxyConnectWithCredential(target, credential))
	} else {
		// 无密码
		_, _ = conn.Write(generateHTTPProxyConnect(target))
	}
	if err = isHTTPConnectWork(conn); err == nil {
		return conn, nil
	}
	conn.Close()
	if err != nil {
		return nil, utils.Wrapf(err, "connect proxy(%s) [%s] failed", httpsString, proxy)
	}
	return nil, utils.Errorf("connect proxy(%s) [%s] failed", httpsString, proxy)
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

func isHTTPConnectWork(c net.Conn) error {
	firstLine, err := utils.ReadConnUntil(c, 5*time.Second, '\n')
	if err != nil {
		return err
	}
	_, code, _, _ := utils.ParseHTTPResponseLine(strings.TrimSpace(string(firstLine)))
	if code < 200 || code > 400 {
		if code == 407 {
			return ErrorProxyAuthFailed
		}
		return utils.Errorf("invalid statue code: %d", code)
	}
	for {
		line, err := utils.ReadConnUntil(c, 5*time.Second, '\n')
		if err != nil {
			return err
		}
		lineStr := string(line)
		k, v, ok := strings.Cut(lineStr, ":")
		if ok {
			switch strings.ToLower(strings.TrimSpace(k)) {
			case "content-length":
				if codec.Atoi(strings.TrimSpace(v)) != 0 {
					return utils.Error("Content-Length should be 0")
				}
			case "transfer-encoding":
				return utils.Error("Transfer-Encoding response header should not exist")
			}
		}
		if strings.TrimSuffix(lineStr, "\r\n") == "" || strings.TrimSuffix(lineStr, "\n") == "" {
			break
		}
	}
	return nil
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

func ProxyCheck(proxy string, connectTimeout time.Duration) (net.Conn, error) {
	ctx := context.Background()
	ctx, _ = context.WithTimeout(ctx, connectTimeout)

	host, port, _ := utils.ParseStringToHostPort(proxy)
	if host == "" || port <= 0 {
		return nil, utils.Errorf("proxy need host:port... at least[%v]", proxy)
	}

	proxyAddr := utils.HostPort(host, port)
	username, password := parseProxyCredential(proxy)
	credential := fmt.Sprintf("%s:%s", username, password)

	switch true {
	case utils.IHasPrefix(proxy, "https://"):
		return httpProxyDial(proxyAddr, username, credential, proxy, "", true)
	case utils.IHasPrefix(proxy, "socks://"):
		fallthrough
	case utils.IHasPrefix(proxy, "socks5://"):
		fallthrough
	case utils.IHasPrefix(proxy, "s5://"):
		conn, err := DialSocksProxyCheck(SOCKS5, proxyAddr, username, password, true)("tcp", "")
		if err != nil {
			return nil, err
		}
		return conn, nil
	case utils.IHasPrefix(proxy, "s4://") || utils.IHasPrefix(proxy, "socks4://"):
		conn, err := DialSocksProxyCheck(SOCKS4, proxyAddr, username, password, true)("tcp", "")
		if err != nil {
			return nil, err
		}
		return conn, nil
	case utils.IHasPrefix(proxy, "s4a://") || utils.IHasPrefix(proxy, "socks4a://"):
		conn, err := DialSocksProxyCheck(SOCKS4A, proxyAddr, username, password, true)("tcp", "")
		if err != nil {
			return nil, err
		}
		return conn, nil
	case utils.IHasPrefix(proxy, "http://"):
		fallthrough
	default:
		return httpProxyDial(proxyAddr, username, credential, proxy, "", false)
	}
}
