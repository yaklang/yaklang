package yaklib

import (
	"testing"

	"github.com/davecgh/go-spew/spew"
)

func TestPoC(t *testing.T) {
	rsp, req, err := pocHTTP(`GET / HTTP/1.1
Host: dppt98.guangdong.chinatax.gov.cn:8443

`)
	spew.Dump(rsp, req, err)
}
