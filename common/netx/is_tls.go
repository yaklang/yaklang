package netx

import (
	"context"
	"crypto/tls"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/utils"
)

var isTlsCached = utils.NewTTLCache[bool](30 * time.Second)

func IsTLSServiceContext(ctx context.Context, addr string, proxies ...string) bool {
	result, ok := isTlsCached.Get(addr)
	if ok {
		return result
	}

	if ctx == nil {
		ctx = context.Background()
	}

	isHttps := false
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	conn, err := DialTCPTimeout(5*time.Second, addr, proxies...)
	if err == nil {
		defer conn.Close()
		host, _, _ := utils.ParseStringToHostPort(addr)
		loopBack := utils.IsLoopback(host)
		tlsConn := tls.Client(conn, &tls.Config{
			InsecureSkipVerify: true,
			MinVersion:         tls.VersionSSL30,
			MaxVersion:         tls.VersionTLS13,
			ServerName:         host,
		})

		err = tlsConn.HandshakeContext(ctx)
		if err == nil {
			isHttps = true // 握手成功，设置 isHttps 为 true
			//// 获取连接状态
			//state := tlsConn.ConnectionState()
			//// 打印使用的密码套件
			//log.Infof("Cipher Suite: %s\n", tls.CipherSuiteName(state.CipherSuite))
		} else {
			// 检查错误消息中是否包含特定的TLS错误
			if strings.Contains(err.Error(), "handshake failure") || strings.Contains(err.Error(), "protocol version not supported") || strings.HasSuffix(err.Error(), "unsupported elliptic curve") {
				isHttps = true
			}
		}

		// 根据 isHttps 的值设置缓存
		if !loopBack {
			isTlsCached.SetWithTTL(addr, isHttps, 30*time.Second)
		}
	}

	return isHttps
}

func IsTLSService(addr string, proxies ...string) bool {
	return IsTLSServiceContext(context.Background(), addr, proxies...)
}
