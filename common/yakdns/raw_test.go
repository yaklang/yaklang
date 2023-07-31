package yakdns

import (
	"github.com/yaklang/yaklang/common/utils"
	"testing"
)

func TestBASIC(t *testing.T) {
	reliableLookupHost("baidu.com")
	reliableLookupHost("baidu.com")
	reliableLookupHost("baidu.com")
	reliableLookupHost("www.uestc.edu.cn")
	reliableLookupHost("www.uestc.edu.cn")
	reliableLookupHost("www.uestc.edu.cn")
	reliableLookupHost("www.uestc.edu.cn")
}

func TestNotExisted(t *testing.T) {
	reliableLookupHost(utils.RandNumberStringBytes(100)+".com", WithFallbackDoH(true))
}

func TestNotExisted_Prefer(t *testing.T) {
	reliableLookupHost(utils.RandNumberStringBytes(100)+".com", WithPreferDoH(true))
}

func TestNotExisted_Prefer1(t *testing.T) {
	reliableLookupHost("baidu.com", WithPreferDoH(true), WithDisableSystemResolver(true))
}
