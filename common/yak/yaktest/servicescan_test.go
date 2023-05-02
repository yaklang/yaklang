package yaktest

import (
	"github.com/davecgh/go-spew/spew"
	"testing"
	"yaklang/common/utils"
	"yaklang/common/utils/tlsutils"
)

func TestServiceScanOne(t *testing.T) {
	cases := []YakTestCase{
		{Name: "ServiceScanOne", Src: `

loglevel("debug")

for r in servicescan.Scan("127.0.0.1", "7891", servicescan.nmapRule(` + "`" + `
Probe TCP socks5 q|\x05\x02\x00\x02|
rarity 2

match socks5 m|\x05\x00|s p/Socks5 Non-auth/ v/5/ cpe:/a:*:socks:5:unauth/a
match socks5 m|\x05\x02|s p/Socks5 Auth/ v/5/ cpe:/a:*:socks5:5:auth/a
		` + "`" + `))~ {
    dump(r)
}
`},
	}
	Run("Yak ScanOne", t, cases...)
}

func TestServiceScan(t *testing.T) {
	cases := []YakTestCase{
		{Name: "Servicescan", Src: `
for result in servicescan.Scan("47.52.100.1/24", "80")~ {
    dump(result)
}
`},
	}
	Run("Yak SYNSCAN 测试", t, cases...)
}

func TestServiceScan2(t *testing.T) {
	cases := []YakTestCase{
		{Name: "Servicescan", Src: `
loglevel("debug");
res, err = servicescan.Scan("127.0.0.1:80", "")
die(err)
count = 0
for r = range res {
count++
println(r.String()) 
}
println(count)
`},
	}
	Run("Yak SYNSCAN 测试", t, cases...)
}

func TestServiceScan3(t *testing.T) {
	cases := []YakTestCase{
		{Name: "Servicescan", Src: `
loglevel("debug");
res, err = servicescan.Scan("127.0.0.1:80", "", servicescan.active(false), servicescan.service())
die(err)
count = 0
for r = range res {
count++
println(r.String()) 
}
println(count)
`},
	}
	Run("Yak SYNSCAN 测试", t, cases...)
}

func TestServiceScan3_TLS(t *testing.T) {
	cases := []YakTestCase{
		{Name: "Servicescan", Src: `
loglevel("debug");
res, err = servicescan.Scan("www.baidu.com", "443", servicescan.active(true), servicescan.web())
die(err)
count = 0
for r = range res {
count++
println(r.String()) 
}
println(count)
`},
	}
	Run("Yak SYNSCAN 测试", t, cases...)
}

func TestServiceScan4_RDP(t *testing.T) {
	cases := []YakTestCase{
		{Name: "Servicescan", Src: `
loglevel("info");
res, err = servicescan.Scan("49.234.9.49/24", "25", servicescan.excludeHosts("49.234.9.49/25"))
die(err)
count = 0
for r = range res {
count++
println(r.String()) 
}
println(count)
`},
	}
	Run("Yak SYNSCAN 测试", t, cases...)
}

func TestServiceScan4_RDP1(t *testing.T) {
	var a, err = tlsutils.TLSInspect(`64.19.180.94:8443`)
	if err != nil {
		panic(err)
	}
	for _, a1 := range a {
		a1.Show()
	}

	rsp, err := utils.NewDefaultHTTPClient().Get("https://64.19.180.94:8443/webmanagement/WebManagement.html")
	if err != nil {
		panic(err)
		return
	}
	spew.Dump(rsp)
}
