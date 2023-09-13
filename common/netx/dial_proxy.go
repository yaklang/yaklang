package netx

import (
	"bufio"
	"context"
	"fmt"
	tls "github.com/refraction-networking/utls"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"net"
	"net/url"
	"os"
	"strings"
	"time"
)

func NewDefaultTLSConfig() *tls.Config {
	return &tls.Config{
		InsecureSkipVerify: true,
		MinVersion:         tls.VersionSSL30, // nolint[:staticcheck]
		MaxVersion:         tls.VersionTLS13,
	}
}

func DialTCPTimeoutForceProxy(timeout time.Duration, target string, proxy string) (net.Conn, error) {
	return connectForceProxy(nil, target, proxy, timeout)
}

func DialTimeout(connectTimeout time.Duration, target string, proxy ...string) (net.Conn, error) {
	if len(proxy) <= 0 {
		return DialTimeoutWithoutProxy(connectTimeout, "tcp", target)
	}
	if len(proxy) > 0 {
		proxy = utils.StringArrayFilterEmpty(proxy)
		if len(proxy) > 0 {
			for _, p := range proxy {
				conn, err := DialTCPTimeoutForceProxy(connectTimeout, target, p)
				if err != nil {
					log.Infof("DialTimeoutForceProxy %s %s not available: %v", target, p, err)
					continue
				}
				return conn, nil
			}
			return nil, utils.Errorf("DialTimeoutForceProxy %s %v all not available", target, proxy)
		}
	}
	return DialTimeoutWithoutProxy(connectTimeout, "tcp", target)
}

func DialTLSTimeout(timeout time.Duration, target string, tlsConfig any, proxy ...string) (net.Conn, error) {
	plainConn, err := DialTimeout(timeout, target, proxy...)
	if err != nil {
		return nil, err
	}
	if tlsConfig == nil {
		tlsConfig = NewDefaultTLSConfig()
	}
	conn, err := UpgradeToTLSConnection(plainConn, utils.ExtractHost(target), tlsConfig)
	if err != nil {
		plainConn.Close()
		return nil, err
	}
	return conn, nil
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

/*
DialTCPTimeout dial tcp with timeout

1. if no proxy, dial directly, timeout for
*/
func DialTCPTimeout(timeout time.Duration, target string, proxies ...string) (net.Conn, error) {
	proxies = utils.StringArrayFilterEmpty(proxies)
	if len(proxies) <= 0 {
		return DialTimeoutWithoutProxy(timeout, "tcp", target)
	}

	for _, proxy := range proxies {
		conn, err := getConnForceProxy(target, proxy, timeout)
		if err != nil {
			log.Errorf("proxy conn failed: %s", err)
			continue
		}
		return conn, nil
	}
	return nil, utils.Errorf("connect: %v failed: no proxy available (in %v)", target, proxies)
}

func getConnForceProxy(target string, proxy string, connectTimeout time.Duration) (net.Conn, error) {
	return connectForceProxy(context.Background(), target, proxy, connectTimeout)
}

func getConnForceProxyContext(ctx context.Context, target, proxy string) (net.Conn, error) {
	return connectForceProxy(ctx, target, proxy, 10*time.Second)
}

func connectForceProxy(ctx context.Context, target string, proxy string, connectTimeout time.Duration) (net.Conn, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, _ = context.WithTimeout(ctx, connectTimeout)

	proxy = strings.ToLower(proxy)
	host, port, _ := utils.ParseStringToHostPort(proxy)
	if host == "" || port <= 0 {
		return nil, utils.Errorf("proxy need host:port... at least[%v]", proxy)
	}

	proxyAddr := utils.HostPort(host, port)
	switch true {
	case strings.HasPrefix(proxy, "https://"):
		urlIns, err := url.Parse(proxy)
		if err != nil {
			return nil, utils.Errorf("parse proxy url failed: %s", err)
		}
		conn, err := DialTLSContextWithoutProxy(ctx, "tcp", proxyAddr, NewDefaultTLSConfig())
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
		return nil, utils.Errorf("connect proxy(https) [%s] failed", proxy)
	case strings.HasPrefix(proxy, "socks://"):
		fallthrough
	case strings.HasPrefix(proxy, "socks5://"):
		fallthrough
	case strings.HasPrefix(proxy, "s5://"):
		username, password := parseProxyCredential(proxy)
		conn, err := DialSocksProxy(SOCKS5, proxyAddr, username, password)("tcp", target)
		if err != nil {
			return nil, err
		}
		return conn, nil
	case strings.HasPrefix(proxy, "s4://") || strings.HasPrefix(proxy, "socks4://"):
		username, password := parseProxyCredential(proxy)
		conn, err := DialSocksProxy(SOCKS4, proxyAddr, username, password)("tcp", target)
		if err != nil {
			return nil, err
		}
		return conn, nil
	case strings.HasPrefix(proxy, "s4a://") || strings.HasPrefix(proxy, "socks4a://"):
		username, password := parseProxyCredential(proxy)
		conn, err := DialSocksProxy(SOCKS4A, proxyAddr, username, password)("tcp", target)
		if err != nil {
			return nil, err
		}
		return conn, nil
	case strings.HasPrefix(proxy, "http://"):
		fallthrough
	default:
		urlIns, err := url.Parse(proxy)
		if err != nil {
			return nil, utils.Errorf("parse proxy url failed: %s", err)
		}

		conn, err := DialContextWithoutProxy(ctx, "tcp", proxyAddr)
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

func readHTTP200(c net.Conn) bool {
	rsp, err := utils.ReadHTTPResponseFromBufioReader(bufio.NewReader(c), nil)
	if err != nil {
		log.Debugf("read response(readHTTP200) failed: %s", err)
		return false
	}
	return rsp.StatusCode >= 200 && rsp.StatusCode < 400
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
