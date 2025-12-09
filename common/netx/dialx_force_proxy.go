package netx

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

var ErrorProxyAuthFailed = utils.Error("invalid proxy username or password")

func DialTCPTimeoutForceProxy(timeout time.Duration, target string, proxy string) (net.Conn, error) {
	return connectForceProxy(nil, target, proxy, &dialXConfig{Timeout: timeout})
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
		return DialContextWithoutProxy(ctx, target)
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

func getConnForceProxy(target string, proxy string, config *dialXConfig) (net.Conn, error) {
	return connectForceProxy(context.Background(), target, proxy, config)
}

func getConnForceProxyContext(ctx context.Context, target, proxy string) (net.Conn, error) {
	return connectForceProxy(ctx, target, proxy, &dialXConfig{
		Timeout: 10 * time.Second,
	})
}

type ProxyCredential struct {
	username  string
	password  string
	proxyAddr string
	schema    string
	proxyUrl  string
	dialCfg   *dialXConfig
}

func (c *ProxyCredential) dialProxyTCP(ctx context.Context, target string) (net.Conn, error) {
	ddl, ok := ctx.Deadline()
	timeout := 15 * time.Second
	if ok {
		timeout = ddl.Sub(time.Now())
	}
	return DialX(
		target,
		DialX_WithDisableProxy(true),
		DialX_WithTLS(c.schema == "https"),
		DialX_WithTimeout(timeout),
		DialX_WithDialer(c.dialCfg.Dialer),
	)
}

func (c *ProxyCredential) getCredentialString() string {
	return fmt.Sprintf("%s:%s", c.username, c.password)
}

func (c *ProxyCredential) proxyDial(ctx context.Context, target string) (net.Conn, error) {
	switch strings.ToLower(c.schema) {
	case "socks", "socks5", "s5":
		return c.socksProxyDial(ctx, target, SOCKS5)
	case "s4a":
		return c.socksProxyDial(ctx, target, SOCKS4A)
	case "s4", "socks4":
		return c.socksProxyDial(ctx, target, SOCKS4)
	default:
		return c.httpProxyDial(ctx, target)
	}
}

func (c *ProxyCredential) httpProxyDial(ctx context.Context, target string) (net.Conn, error) {
	if target == "" {
		target = "/"
	}
	schema := c.schema
	if schema == "" {
		schema = "http"
	}
	ddl, _ := ctx.Deadline()
	conn, err := c.dialProxyTCP(ctx, c.proxyAddr)
	if err != nil {
		return nil, err
	}
	conn.SetDeadline(ddl)
	if c.username != "" {
		// 有密码
		_, _ = conn.Write(generateHTTPProxyConnectWithCredential(target, c.getCredentialString()))
	} else {
		// 无密码
		_, err = conn.Write(generateHTTPProxyConnect(target))
	}
	if err = isHTTPConnectWork(conn); err == nil {
		conn.SetDeadline(time.Time{}) // 置空取消 deadline
		return conn, nil
	} else {
		conn.Close()
		return nil, utils.Wrapf(err, "connect proxy(%s) [%s] failed", schema, c.proxyUrl)
	}
}

// DialSocksProxy returns the dial function to be used in http.Transport object.
// Argument socksType should be one of SOCKS4, SOCKS4A and SOCKS5.
// Argument proxy should be in this format "127.0.0.1:1080".
func (c *ProxyCredential) socksProxyDial(ctx context.Context, target string, socksType int) (net.Conn, error) {
	cfg := &config{Context: ctx, Proto: socksType, Host: c.proxyAddr, ProxyDialer: c.dialProxyTCP}
	if c.username != "" {
		cfg.Auth = &auth{c.username, c.password}
	}
	return cfg.dialFunc()(target)
}

func connectForceProxy(ctx context.Context, target string, proxy string, config *dialXConfig) (net.Conn, error) {
	// use dialx
	// remember disallow proxy!!!
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, _ = context.WithTimeout(ctx, config.Timeout)

	host, port, _ := utils.ParseStringToHostPort(proxy)
	if host == "" || port <= 0 {
		return nil, utils.Errorf("proxy need host:port... at least[%v]", proxy)
	}
	credential, err := newProxyCredential(proxy, config)
	if err != nil {
		return nil, err
	}

	return credential.proxyDial(ctx, target)
}

func newProxyCredential(proxyURL string, cfg *dialXConfig) (*ProxyCredential, error) {
	urlIns, err := url.Parse(proxyURL)
	if err != nil {
		return nil, utils.Errorf("parse proxy url failed: %v", err)
	}
	username := urlIns.User.Username()
	password, _ := urlIns.User.Password()
	return &ProxyCredential{
		username:  username,
		password:  password,
		proxyAddr: urlIns.Host,
		schema:    strings.ToLower(urlIns.Scheme),
		proxyUrl:  proxyURL,
		dialCfg:   cfg,
	}, nil
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
				// return utils.Error("Transfer-Encoding response header should not exist")
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

func ProxyCheck(proxy string, connectTimeout time.Duration) (net.Conn, error) { // check proxy func
	ctx := context.Background()
	ctx, _ = context.WithTimeout(ctx, connectTimeout)

	host, port, _ := utils.ParseStringToHostPort(proxy)
	if host == "" || port <= 0 {
		return nil, utils.Errorf("proxy need host:port... at least[%v]", proxy)
	}

	credential, err := newProxyCredential(proxy, &dialXConfig{Timeout: connectTimeout})
	if err != nil {
		return nil, err
	}
	return credential.proxyDial(ctx, "example.com:80")
}
