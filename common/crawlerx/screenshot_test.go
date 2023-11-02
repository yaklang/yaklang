// Package crawlerx
// @Author bcy2007  2023/11/1 11:04
package crawlerx

import (
	"testing"
)

const responseStr = "HTTP/1.1 200\r\nSet-Cookie: JSESSSIONID=E8ECA470AF9F5385159DE0E8E9BD6726; Path=/; HttpOnly\r\nContent-Type: text/html; charset=utf-8\r\nDate: Wed, 01 Nov2023 03:44:53GMT\r\nContent-Length: 35\r\n\r\ne165421110ba03099a1c393373c5b43\n\r\n"

func TestNewPageScreenShot(t *testing.T) {
	code, err := NewPageScreenShot("http://testphp.vulnweb.com/", WithResponse("http://testphp.vulnweb.com/", responseStr))
	if err != nil {
		t.Error(err)
		return
	}
	t.Log(code)
}
