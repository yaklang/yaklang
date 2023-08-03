package facades

// 这个测试和raw_net_dns.go测试内容重复，防止循环导包，注释掉了
//func TestMockDNSServerDefault(t *testing.T) {
//	for i := 0; i < 10; i++ {
//
//	}
//	randomStr := utils.RandStringBytes(10)
//	var check = false
//	var a = MockDNSServerDefault("", func(record string, domain string) string {
//		spew.Dump(domain)
//		if strings.Contains(domain, randomStr) {
//			check = true
//		}
//		return "1.1.1.1"
//	})
//	var result = yakdns.LookupFirst(randomStr+".baidu.com", yakdns.WithTimeout(5*time.Second), yakdns.WithDNSServers(a))
//
//	spew.Dump(result)
//	if !check {
//		panic("GetFirstIPByDnsWithCache failed")
//	}
//}
