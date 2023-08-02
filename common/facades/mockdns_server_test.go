package facades

import (
	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakdns"
	"strings"
	"testing"
	"time"
)

func TestMockDNSServerDefault(t *testing.T) {
	for i := 0; i < 10; i++ {

	}
	randomStr := utils.RandStringBytes(10)
	var check = false
	var a = MockDNSServerDefault("", func(record string, domain string) string {
		spew.Dump(domain)
		if strings.Contains(domain, randomStr) {
			check = true
		}
		return "1.1.1.1"
	})
	var result = yakdns.LookupFirst(randomStr+".baidu.com", yakdns.WithTimeout(5*time.Second), yakdns.WithDNSServers(a))

	spew.Dump(result)
	if !check {
		panic("GetFirstIPByDnsWithCache failed")
	}
}
