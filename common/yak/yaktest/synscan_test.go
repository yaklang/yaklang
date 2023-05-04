package yaktest

import (
	"os"
	"testing"
)

func TestRedis2(t *testing.T) {
	os.Setenv("YAKMODE", "vm")
	cases := []YakTestCase{
		{Name: "Yak Redis", Src: `
for in synscan.Scan("47.52.100.1/24", "22")~ {}
`},
	}
	Run("Yak Redis 测试", t, cases...)
}

func TestRedis(t *testing.T) {
	os.Setenv("YAKMODE", "vm")
	cases := []YakTestCase{
		{Name: "Yak Redis", Src: `
loglevel("info");

client = redis.New()

client.Set("abc", "123")~

data = client.Get("abc")~
dump(data)


`},
	}
	Run("Yak Redis 测试", t, cases...)
}

func TestSynScan(t *testing.T) {
	cases := []YakTestCase{
		{Name: "syn限速器", Src: `
loglevel("info");
//res, err = synscan.Scan("47.52.100.1/24", "1-65535", synscan.rateLimit(10,1000))
res, err = synscan.Scan("3.12.2.128/24", "22,80", synscan.concurrent(2000), synscan.callback(func(i){
	db.SavePortFromResult(i)
}), synscan.submitTaskCallback(func(i){
	println(i)
}), synscan.excludeHosts("3.12.2.128/25"), synscan.excludePorts("22"))
die(err)
count = 0
for r = range res {
count++
r.Show() 
}
println(count)
`},
	}
	Run("Yak SYNSCAN 测试", t, cases...)
}

func TestSynScan1(t *testing.T) {
	cases := []YakTestCase{
		{Name: "syn限速器", Src: `
loglevel("info");
res, err = synscan.Scan("www.baidu.com:80", "", synscan.concurrent(2000), synscan.callback(func(i){
	db.SavePortFromResult(i)
}), synscan.submitTaskCallback(func(i){
	println(i)
}), synscan.excludeHosts("3.12.2.128/25"), synscan.excludePorts("22"))
die(err)
count = 0
for r = range res {
count++
r.Show() 
}
println(count)
`},
	}
	Run("Yak SYNSCAN 测试", t, cases...)
}

func TestSynScan_FromPing(t *testing.T) {
	cases := []YakTestCase{
		{Name: "syn限速器", Src: `
loglevel("error");

hosts = "192.168.101.146"
pingResult = ping.Scan(
	hosts, ping.skip(true),
) 

res, err = synscan.ScanFromPing(pingResult, "1-65535")
die(err)
count = 0
for r = range res {
count++
r.Show() 
}
println(count)
`},
	}
	Run("Yak SYNSCAN 测试", t, cases...)
}

func TestSuricata(t *testing.T) {
	var data = `
ruleMaker := suricata.TrafficGenerator()~
for rules := range suricata.YieldRules() {
    ruleMaker.FeedRule(rules)
}
for result = range ruleMaker.Generate() {
    pcapx.InjectChaosTraffic(result)
}
`
	Run("Suricata Test", t, YakTestCase{Src: data, Name: "inject"})
}
