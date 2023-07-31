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
