package netx

import (
	"context"
	"crypto/tls"
	"github.com/ReneKroon/ttlcache"
	"github.com/yaklang/yaklang/common/utils"
	"time"
)

var isTlsCached = ttlcache.NewCache()

func IsTLSService(addr string, proxies ...string) bool {
	result, ok := isTlsCached.Get(addr)
	if ok {
		return result.(bool)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := DialTCPTimeout(5*time.Second, addr, proxies...)
	if err == nil {
		defer conn.Close()
		host, _, _ := utils.ParseStringToHostPort(addr)
		loopBack := utils.IsLoopback(host)
		tlsConn := tls.Client(conn, &tls.Config{InsecureSkipVerify: true, MinVersion: tls.VersionSSL30, ServerName: host})
		if err = tlsConn.HandshakeContext(ctx); err != nil {
			if !loopBack {
				isTlsCached.SetWithTTL(addr, false, 30*time.Second)
			}
			return false
		}
		if !loopBack {
			isTlsCached.SetWithTTL(addr, true, 30*time.Second)
		}
		return true
	}
	return false
}
