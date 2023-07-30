package yakdns

import (
	"testing"
)

func TestDoH(t *testing.T) {
	reliableLookupHost("baidu.com")
	reliableLookupHost("baidu.com")
	reliableLookupHost("baidu.com")
	reliableLookupHost("www.uestc.edu.cn")
	reliableLookupHost("www.uestc.edu.cn")
	reliableLookupHost("www.uestc.edu.cn")
	reliableLookupHost("www.uestc.edu.cn")
}
