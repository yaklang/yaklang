// Package crawlerx
// @Author bcy2007  2023/7/17 14:25
package crawlerx

import "testing"

func TestUrlCheck(t *testing.T) {
	urlA := `baidu.com`
	urlAResult, err := TargetUrlCheck(urlA, nil)
	t.Log(urlAResult, err)

	urlB := `111111111111111.com`
	urlBResult, err := TargetUrlCheck(urlB, nil)
	t.Log(urlBResult, err)

	urlC := `http://www.baidu.com`
	urlCResult, err := TargetUrlCheck(urlC, nil)
	t.Log(urlCResult, err)
}
