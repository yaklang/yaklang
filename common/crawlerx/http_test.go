// Package crawlerx
// @Author bcy2007  2023/7/17 11:06
package crawlerx

import (
	"testing"
)

func TestHTTPHead(t *testing.T) {
	url := "http://www.baidu.com"
	r := CreateGetRequest(url)
	r.Request()
	r.Do()
	result, _ := r.Show()
	t.Log(result)
}
