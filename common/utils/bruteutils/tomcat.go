package bruteutils

import (
	"fmt"
	"github.com/ReneKroon/ttlcache"
	"regexp"
	"time"
	"yaklang.io/yaklang/common/log"
	"yaklang.io/yaklang/common/utils"
	"yaklang.io/yaklang/common/utils/lowhttp"
)

var defaultPassTomcat = utils.ParseStringToLines(`tomcat
manager
apache
password
administrator
root
admin
admin1
admin111
admin123
admin1234
admin222
admin666
admin888
admin123!@#
tomcat123
tomcat1234
tomcat666
tomcat888
manager123
manager1234
manager666
manager888
abc123
abc123!@#
abcd1234
asd123
password123
password123!@#
qwe123
qwe123!@#
qwer1234
qweasd
qweasdzxc
1q2w3e
1q2w3e4r
000000
111111
123123
123456
1234567
12345678
123456789
147258
258369
654321
666666
66666666
7654321
888888
88888888
87654321
987654321`)
var defaultUserTomcat = utils.ParseStringToLines(`tomcat
manager
root
user
test
apache
admin
administrator`)

const tomcatAuthPacket = `GET /{{rs}}/..;/host-manager/html HTTP/1.1
Host: {{param(target)}}
Accept: text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.9
Accept-Encoding: deflate
Accept-Language: zh-CN,zh;q=0.9
Authorization: Basic {{base64({{param(user)}}:{{param(pass)}})}}
Cache-Control: max-age=0
Content-Length: 0
Cookie: Hm_lvt_deaeca6802357287fb453f342ce28dda=1637767370,1637768217,1637768433,1639209876; vue_admin_template_token=eyJ0eXAiOiJKV1QiLCJhbGciOiJIUzI1NiJ9.eyJ1c2VyX2lkIjo3NjA5LCJ1c2VybmFtZSI6InYxbGw0biIsImV4cCI6MTYzOTI5NjI4MSwiZW1haWwiOiJ2MWxsNG5AcXEuY29tIn0.lvYgqxzKXMbi0bivKIm4LAwSUrU4PxDfZSt0BfIrxTU; Hm_lpvt_deaeca6802357287fb453f342ce28dda=1639212249
Upgrade-Insecure-Requests: 1
User-Agent: Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/96.0.4664.55 Safari/537.36
`

const tomcatProbePacket = `GET /{{rs}}/..;/host-manager/html HTTP/1.1
Host: {{param(target)}}
Accept: text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.9
Accept-Encoding: deflate
Accept-Language: zh-CN,zh;q=0.9
Cache-Control: max-age=0
Content-Length: 0
Cookie: Hm_lvt_deaeca6802357287fb453f342ce28dda=1637767370,1637768217,1637768433,1639209876; vue_admin_template_token=eyJ0eXAiOiJKV1QiLCJhbGciOiJIUzI1NiJ9.eyJ1c2VyX2lkIjo3NjA5LCJ1c2VybmFtZSI6InYxbGw0biIsImV4cCI6MTYzOTI5NjI4MSwiZW1haWwiOiJ2MWxsNG5AcXEuY29tIn0.lvYgqxzKXMbi0bivKIm4LAwSUrU4PxDfZSt0BfIrxTU; Hm_lpvt_deaeca6802357287fb453f342ce28dda=1639212249
Upgrade-Insecure-Requests: 1
User-Agent: Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/96.0.4664.55 Safari/537.36
`

var tomcatUnLoginRegexp = regexp.MustCompile(fmt.Sprintf(`(?i)%v`, regexp.QuoteMeta(`<h1>401 Unauthorized</h1>`)))
var tomcatManagerRegexp = regexp.MustCompile(fmt.Sprintf(`(?i)%v`, regexp.QuoteMeta(`alt="The Tomcat Servlet/JSP Container"`)))

var tomcatTlsTTLcache = ttlcache.NewCache()

func init() {
	tomcatTlsTTLcache.SetTTL(30 * time.Minute)
}

var tomcat = &DefaultServiceAuthInfo{
	ServiceName:      "tomcat",
	DefaultPorts:     "80,81,7001,7002,8080,8081,8082,8088,8089,8090,8443,8888,9080,9090",
	DefaultUsernames: defaultUserTomcat,
	DefaultPasswords: defaultPassTomcat,
	UnAuthVerify: func(i *BruteItem) *BruteItemResult {
		result := i.Result()
		// /host-manager/html
		i.Target = appendDefaultPort(i.Target, 8080)
		host, port, err := utils.ParseStringToHostPort(i.Target)
		if err != nil {
			result.Finished = true
			return result
		}
		addr := utils.HostPort(host, port)
		isTls := utils.IsTLSService(addr)
		if isTls {
			tomcatTlsTTLcache.Set(addr, true)
		}
		rsp, _, err := packetToBrute(tomcatProbePacket, map[string][]string{
			"target": {utils.HostPort(host, port)},
			"user":   {i.Username},
			"pass":   {i.Password},
		}, 10, isTls)
		if err != nil {
			log.Errorf("send packet to tomcat brute failed: %s", err)
			return result
		}
		rspIns, err := lowhttp.ParseBytesToHTTPResponse(rsp)
		if err != nil {
			log.Errorf("parse tomcat packet failed: %s", err)
			return result
		}
		if rspIns.StatusCode == 401 {
			return result
		}
		result.Finished = true
		return result
	},
	BrutePass: func(i *BruteItem) *BruteItemResult {
		result := i.Result()

		host, port, err := utils.ParseStringToHostPort(i.Target)
		if err != nil {
			port = 8080
		}

		addr := utils.HostPort(host, port)
		var _, isTls = tomcatTlsTTLcache.Get(addr)
		rsp, _, _ := packetToBrute(
			tomcatAuthPacket,
			map[string][]string{
				"target": {utils.HostPort(host, port)},
				"user":   {i.Username},
				"pass":   {i.Password},
			},
			10, isTls,
		)
		if tomcatManagerRegexp.Match(rsp) {
			result.Ok = true
			return result
		}

		return result
	},
}
