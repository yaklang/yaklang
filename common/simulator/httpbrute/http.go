// Package httpbrute
// @Author bcy2007  2023/7/31 14:02
package httpbrute

import (
	"crypto/tls"
	"fmt"
	"github.com/yaklang/yaklang/common/netx"
	"io"
	"net/http"
	"net/url"
	"time"
)

func ConnectTest(urlStr string, proxy *url.URL) (bool, string) {
	transport := http.Transport{
		TLSClientConfig:       &tls.Config{InsecureSkipVerify: true},
		DialContext:           netx.NewDialContextFunc(15 * time.Second),
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
	if proxy != nil {
		transport.Proxy = http.ProxyURL(proxy)
	}
	client := http.Client{}
	client.Transport = &transport
	req, err := http.NewRequest("HEAD", urlStr, nil)
	if err != nil {
		return false, fmt.Sprintf(`create request error: %s`, err.Error())
	}
	res, err := client.Do(req)
	if err != nil {
		return false, fmt.Sprintf(`get response error: %s`, err.Error())
	}
	_, err = io.ReadAll(res.Body)
	if err != nil {
		return false, fmt.Sprintf(`get response body error: %v`, err.Error())
	}
	return true, ""
}
