package netx

import (
	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/utils"
	"io"
	"testing"
)

func TestBadCase(t *testing.T) {
	rsp, err := utils.NewDefaultHTTPClient().Get(`https://xfxcx.hnloudi.gov.cn/wxmp/wx/YW/GetXfListByIdCard`)
	if err != nil {
		panic(err)
	}

	var raw, _ = io.ReadAll(rsp.Body)
	spew.Dump(raw)
}
