package tlsutils

import "testing"

func TestTLSInspect(t *testing.T) {
	_, err := TLSInspect("baidu.com")
	if err != nil {
		panic(err)
	}
}
