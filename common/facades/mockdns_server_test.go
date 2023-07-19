package facades

import (
	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/utils"
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
	var result = utils.GetFirstIPByDnsWithCache(randomStr+".baidu.com", 5*time.Second, a)
	spew.Dump(result)
	if !check {
		panic("GetFirstIPByDnsWithCache failed")
	}
}
