// Package httpbrute
// @Author bcy2007  2023/7/31 14:02
package httpbrute

import (
	"fmt"
	"github.com/yaklang/yaklang/common/netx"
	"io"
	"net/http"
	"net/url"
)

func ConnectTest(urlStr string, proxy *url.URL) (bool, string) {
	client := netx.NewDefaultHTTPClient(proxy.String())
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
