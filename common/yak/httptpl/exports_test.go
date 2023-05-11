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
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"strings"
	"testing"
	"time"
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
	Scan := func(target any, opt ...interface{}) (chan *tools.PocVul, error) {
		var vCh = make(chan *tools.PocVul)
		//var targetVul *tools.PocVul
		filterVul := filter.NewFilter()
		calcSha1 := utils.CalcSha1(target)

		opt = append(opt, _callback(func(i map[string]interface{}) {

			if i["match"].(bool) {

				tpl := i["template"].(*YakTemplate)
				resp := i["responses"].([]*lowhttp.LowhttpResponse)
				reqBulk := i["requests"].(*YakRequestBulkConfig)
				//log.Infof("Scan callback: %#v", tpl)
				details := make(map[string]interface{})
				if len(resp) == 1 {
					details["request"] = string(resp[0].RawRequest)
					details["response"] = string(resp[0].RawPacket)
				} else {
					for idx, r := range resp {
						details[fmt.Sprintf("request_%d", idx+1)] = string(r.RawRequest)
						details[fmt.Sprintf("response_%d", idx+1)] = string(r.RawPacket)
					}
				}
				payloads, err := payloadsToString(reqBulk.Payloads)
				if err != nil {
					log.Errorf("payloadsToString failed: %v", err)
				}
				pv := &tools.PocVul{
					Source:        "nuclei",
					Target:        resp[0].RemoteAddr,
					PocName:       tpl.Name,
					MatchedAt:     utils.DatetimePretty(),
					Tags:          strings.Join(tpl.Tags, ","),
					Timestamp:     time.Now().Unix(),
					Severity:      tpl.Severity,
					Details:       details,
					CVE:           tpl.CVE,
					DescriptionZh: tpl.DescriptionZh,
					Description:   tpl.Description,
					Payload:       payloads,
				}
				log.Infof("calcSha1: %s", calcSha1)
				if !filterVul.Exist(calcSha1) {
					filterVul.Insert(calcSha1)
					risk := tools.PocVulToRisk(pv)
					err = yakit.SaveRisk(risk)
					log.Info("save risk")
					if err != nil {
						log.Errorf("save risk failed: %s", err)
					}
					vCh <- pv
				}

			}
		}))
		go func() {
			defer close(vCh)
			ScanAuto(target, opt...)
		}()

		return vCh, nil
	}
	res, _ := Scan(
		"http://192.168.3.113:8085/S2-032/index.action",
		//WithTemplateName("[CVE-2016-3081]: Apache S2-032 Struts - Remote Code Execution"),
		WithTemplateRaw(rawTemp),
	)
	for r := range res {
		fmt.Println("xxx : ", r)
	}
}
