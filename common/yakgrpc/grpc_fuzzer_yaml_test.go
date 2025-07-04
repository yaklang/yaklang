package yakgrpc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"golang.org/x/exp/maps"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/lowhttp"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/httptpl"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"gopkg.in/yaml.v2"
)

func CompareNucleiYaml(yaml1, yaml2 string) error {
	temp1, err := httptpl.CreateYakTemplateFromNucleiTemplateRaw(yaml1)
	if err != nil {
		panic(err)
	}
	temp2, err := httptpl.CreateYakTemplateFromNucleiTemplateRaw(yaml2)
	if err != nil {
		panic(err)
	}
	if temp1 == nil || temp2 == nil {
		panic("create template failed")
	}
	// 比较签名
	if temp1.SignMainParams() != temp2.SignMainParams() {
		return errors.New("sign main params not equal")
	}

	// 比较其它字段
	yaml1Map := map[string]interface{}{}
	err = yaml.Unmarshal([]byte(yaml1), yaml1Map)
	if err != nil {
		panic(err)
	}
	yaml2Map := map[string]interface{}{}
	err = yaml.Unmarshal([]byte(yaml2), yaml2Map)
	if err != nil {
		panic(err)
	}
	for k, v := range yaml1Map {
		switch k {
		case "self-contained", `{{interactsh-url}}`, `{{interactsh}}`, `{{interactsh_url}}`, `interactsh`:
			if v != yaml2Map[k] {
				return errors.New(fmt.Sprintf("key %s not equal", k))
			}
		default:

		}
	}
	requests1 := utils.InterfaceToSliceInterface(utils.MapGetFirstRaw(yaml1Map, "requests", "http"))
	requests2 := utils.InterfaceToSliceInterface(utils.MapGetFirstRaw(yaml2Map, "requests", "http"))
	if len(requests1) != len(utils.InterfaceToSliceInterface(requests2)) {
		return errors.New("requests length not equal")
	}
	for i := 0; i < len(requests1); i++ {
		req1 := requests1[i].(map[any]any)
		req2 := requests2[i].(map[any]any)
		if len(req1) != len(req2) {
			return errors.New(fmt.Sprintf("request %d field length not equal", i+1))
		}
		for k, v := range req1 {
			switch k {
			case "stop-at-first-macth", "disable-cookie", "max-size", "unsafe", "redirects", "max-redirects":
				if v != req2[k] {
					return errors.New(fmt.Sprintf("key %s not equal", k))
				}
			}
		}
	}
	return nil
}

func TestGRPCMUSTPASS_HTTPFuzzer_CompareNucleiYamlFunc(t *testing.T) {
	testCases := []struct {
		content string
		expect  string
		err     string
	}{
		{
			content: `http:
  - raw:
      - |
        GET /fuel/login/ HTTP/1.1
        Host: {{Hostname}}
      - |
        POST /fuel/login/ HTTP/1.1
        Host: {{Hostname}}
        Content-Type: application/x-www-form-urlencoded
        Referer: {{RootURL}}

        user_name={{username}}&password={{password}}&Login=Login&forward=
      - |
        @timeout: 10s
        GET /fuel/pages/items/?search_term=&published=&layout=&limit=50&view_type=list&offset=0&order=asc&col=location+AND+(SELECT+1340+FROM+(SELECT(SLEEP(6)))ULQV)&fuel_inline=0 HTTP/1.1
        Host: {{Hostname}}
        X-Requested-With: XMLHttpRequest
        Referer: {{RootURL}}

    payloads:
      username:
        - admin
      password:
        - admin
    attack: pitchfork
    disable-cookie: true
    matchers:
      - type: dsl
        dsl:
          - 'duration>=6'
          - 'status_code_3 == 200'
          - 'contains(body_1, "FUEL CMS")'
        condition: and`,
			expect: `http:
  - raw:
      - |
        GET /fuel/login/ HTTP/1.1
        Host: {{Hostname}}
      - |
        POST /fuel/login/ HTTP/1.1
        Host: {{Hostname}}
        Content-Type: application/x-www-form-urlencoded
        Referer: {{RootURL}}

        user_name={{username}}&password={{password}}&Login=Login&forward=
      - |
        @timeout: 10s
        GET /fuel/pages/items/?search_term=&published=&layout=&limit=50&view_type=list&offset=0&order=asc&col=location+AND+(SELECT+1340+FROM+(SELECT(SLEEP(6)))ULQV)&fuel_inline=0 HTTP/1.1
        Host: {{Hostname}}
        X-Requested-With: XMLHttpRequest
        Referer: {{RootURL}}

    payloads:
      username:
        - admin
      password:
        - admin
    attack: pitchfork
    disable-cookie: true
    matchers:
      - type: dsl
        dsl:
          - 'duration>=6'
          - 'status_code_3 == 200'
          - 'contains(body_1, "FUEL CMS")'
        condition: and`,
			err: "",
		}, {
			content: `http:
  - raw:
      - |
        GET /fuel/login/ HTTP/1.1
        Host: {{Hostname}}
      - |
        POST /fuel/login/ HTTP/1.1
        Host: {{Hostname}}
        Content-Type: application/x-www-form-urlencoded
        Referer: {{RootURL}}

        user_name={{username}}&password={{password}}&Login=Login&forward=
      - |
        @timeout: 10s
        GET /fuel/pages/items/?search_term=&published=&layout=&limit=50&view_type=list&offset=0&order=asc&col=location+AND+(SELECT+1340+FROM+(SELECT(SLEEP(6)))ULQV)&fuel_inline=0 HTTP/1.1
        Host: {{Hostname}}
        X-Requested-With: XMLHttpRequest
        Referer: {{RootURL}}

    payloads:
      username:
        - admin
      password:
        - admin
    attack: pitchfork
    disable-cookie: true
    redirects: true
    max-redirects: 10
    matchers:
      - type: dsl
        dsl:
          - 'duration>=6'
          - 'status_code_3 == 200'
          - 'contains(body_1, "FUEL CMS")'
        condition: and`,
			expect: `http:
  - raw:
      - |
        GET /fuel/login/ HTTP/1.1
        Host: {{Hostname}}
      - |
        POST /fuel/login/ HTTP/1.1
        Host: {{Hostname}}
        Content-Type: application/x-www-form-urlencoded
        Referer: {{RootURL}}

        user_name={{username}}&password={{password}}&Login=Login&forward=
      - |
        @timeout: 10s
        GET /fuel/pages/items/?search_term=&published=&layout=&limit=50&view_type=list&offset=0&order=asc&col=location+AND+(SELECT+1340+FROM+(SELECT(SLEEP(6)))ULQV)&fuel_inline=0 HTTP/1.1
        Host: {{Hostname}}
        X-Requested-With: XMLHttpRequest
        Referer: {{RootURL}}

    payloads:
      username:
        - admin
      password:
        - admin
    attack: pitchfork
    disable-cookie: true
    matchers:
      - type: dsl
        dsl:
          - 'duration>=6'
          - 'status_code_3 == 200'
          - 'contains(body_1, "FUEL CMS")'
        condition: and`,
			err: "request 1 field length not equal",
		}, {
			content: `http:
  - raw:
      - |
        GET /fuel/login/ HTTP/1.1
        Host: {{Hostname}}
      - |
        POST /fuel/login/ HTTP/1.1
        Host: {{Hostname}}
        Content-Type: application/x-www-form-urlencoded
        Referer: {{RootURL}}

        user_name={{username}}&password={{password}}&Login=Login&forward=
      - |
        @timeout: 10s
        GET /fuel/pages/items/?search_term=&published=&layout=&limit=50&view_type=list&offset=0&order=asc&col=location+AND+(SELECT+1340+FROM+(SELECT(SLEEP(6)))ULQV)&fuel_inline=0 HTTP/1.1
        Host: {{Hostname}}
        X-Requested-With: XMLHttpRequest
        Referer: {{RootURL}}

    payloads:
      username:
        - admin
      password:
        - admin
    attack: pitchfork
    disable-cookie: true
    redirects: true
    max-redirects: 10
    matchers:
      - type: dsl
        dsl:
          - 'duration>=6'
          - 'status_code_3 == 200'
          - 'contains(body_1, "FUEL CMS")'
        condition: and`,
			expect: `http:
  - raw:
      - |
        GET /fuel/login/ HTTP/1.1
        Host: {{Hostname}}
      - |
        POST /fuel/login/ HTTP/1.1
        Host: {{Hostname}}
        Content-Type: application/x-www-form-urlencoded
        Referer: {{RootURL}}

        user_name={{username}}&password={{password}}&Login=Login&forward=
      - |
        @timeout: 10s
        GET /fuel/pages/items/?search_term=&published=&layout=&limit=50&view_type=list&offset=0&order=asc&col=location+AND+(SELECT+1340+FROM+(SELECT(SLEEP(6)))ULQV)&fuel_inline=0 HTTP/1.1
        Host: {{Hostname}}
        X-Requested-With: XMLHttpRequest
        Referer: {{RootURL}}

    payloads:
      username:
        - admina
      password:
        - admin
    attack: pitchfork
    disable-cookie: true
    redirects: true
    max-redirects: 10
    matchers:
      - type: dsl
        dsl:
          - 'duration>=6'
          - 'status_code_3 == 200'
          - 'contains(body_1, "FUEL CMS")'
        condition: and`,
			err: "sign main params not equal",
		},
	}
	for _, testCase := range testCases {
		err := CompareNucleiYaml(testCase.content, testCase.expect)
		if err != nil {
			if err.Error() != testCase.err {
				t.Fatal(fmt.Sprintf("expect error: %s, got: %s", testCase.err, err.Error()))
			}
		} else {
			if testCase.err != "" {
				t.Fatal(fmt.Sprintf("expect error: %s, got: nil", testCase.err))
			}
		}
	}
}

func TestGRPCMUSTPASS_HTTPFuzzer_CheckSignParam(t *testing.T) {
	template := `http:
- raw:
  - |-
    @timeout: 30s
    POST / HTTP/1.1
    Content-Type: application/json
    Host: {{Hostname}}
    Content-Length: 16

    {"key": "value"}
`
	// 对 method, paths, headers, body、raw、matcher、extractor、payloads 签名检查
	testCase := []func(req *httptpl.YakRequestBulkConfig){
		func(req *httptpl.YakRequestBulkConfig) {
		},
		func(req *httptpl.YakRequestBulkConfig) {
			req.Method = "GET"
		},
		func(req *httptpl.YakRequestBulkConfig) {
			req.Paths = []string{"/a"}
		},
		func(req *httptpl.YakRequestBulkConfig) {
			req.Headers = map[string]string{"a": "b"}
		},
		func(req *httptpl.YakRequestBulkConfig) {
			req.Body = "a"
		},
		func(req *httptpl.YakRequestBulkConfig) {
			req.HTTPRequests = []*httptpl.YakHTTPRequestPacket{
				{
					Request: "a",
				},
			}
		},
		func(req *httptpl.YakRequestBulkConfig) {
			req.Matcher = &httptpl.YakMatcher{
				MatcherType: "a",
			}
		},
		func(req *httptpl.YakRequestBulkConfig) {
			req.Extractor = []*httptpl.YakExtractor{
				{
					Name: "a",
				},
			}
		},
		func(req *httptpl.YakRequestBulkConfig) {
			payloads, err := httptpl.NewYakPayloads(map[string]interface{}{
				"a": []string{"b"},
			})
			if err != nil {
				t.Fatal(err)
			}
			req.Payloads = payloads
		},
	}
	signMap := map[string]struct{}{}
	for i, testCaseFunc := range testCase {
		tmp, err := httptpl.CreateYakTemplateFromNucleiTemplateRaw(template)
		if err != nil {
			t.Fatal(err)
		}
		testCaseFunc(tmp.HTTPRequestSequences[0])
		sign := tmp.SignMainParams()
		if _, ok := signMap[sign]; ok {
			t.Fatal(fmt.Sprintf("test case %d sign repeat", i))
		}
		signMap[sign] = struct{}{}
	}
}

func TestGRPCMUSTPASS_HTTPFuzzer_SignProtectCheck_Negative(t *testing.T) {
	testCases := []struct {
		content string
		expect  bool
	}{
		{
			content: `id: WebFuzzer-Template-FlTJsZDz

info:
  name: WebFuzzer Template FlTJsZDz
  author: god
  severity: low
  description: write your description here
  reference:
  - https://github.com/
  - https://cve.mitre.org/
  metadata:
    max-request: 1
    shodan-query: ""
    verified: true
  yakit-info:
    sign: 1820dd76ddcad69364bc85df273a0d7a

http:
- raw:
  - |-
    @timeout: 30s
    POST / HTTP/1.1
    Content-Type: application/json
    Host: {{Hostname}}
    Content-Length: 16

    {"key": "value"}

  max-redirects: 3
  matchers-condition: and

# Generated From WebFuzzer on 2023-10-09 11:42:55
`,
			expect: true,
		},
		{
			content: `id: WebFuzzer-Template-FlTJsZDz

info:
  name: WebFuzzer Template FlTJsZDz
  author: god
  severity: low
  description: write your description here
  reference:
  - https://github.com/
  - https://cve.mitre.org/
  metadata:
    max-request: 1
    shodan-query: ""
    verified: true
  yakit-info:
    sign: 6fa568785c723e6d1003e7d3bba408fe

http:
- raw:
  - |-
    @timeout: 30s
    POST / HTTP/1.1
    Content-Type: application/json
    Host: {{Hostname}}
    Content-Length: 16

    {"key": "value "}

  max-redirects: 3
  matchers-condition: and`,
			expect: false,
		},
	}
	for _, testCase := range testCases {
		tmp, err := httptpl.CreateYakTemplateFromNucleiTemplateRaw(testCase.content)
		if err != nil {
			t.Fatal(err)
		}
		err = tmp.CheckTemplateRisks()
		if err == nil {
			continue
		}
		t.Fatal("no extractor and matcher should not panic")
		//
		//if testCase.expect && err != nil && strings.Contains(err.Error(), "签名错误") {
		//	t.Fatal(fmt.Sprintf("expect no signature error, got: %s", err.Error()))
		//}
		//if !testCase.expect && (err == nil || !strings.Contains(err.Error(), "签名错误")) {
		//	t.Fatal(fmt.Sprintf("expect signature error: not nil, got: %s", err.Error()))
		//}
	}
}

func TestGRPCMUSTPASS_HTTPFuzzer_SignProtectCheck_Positive(t *testing.T) {
	testCases := []struct {
		content string
		expect  bool
	}{
		{
			content: `id: WebFuzzer-Template-yiXBTuUG

info:
  name: WebFuzzer Template yiXBTuUG
  author: god
  severity: low
  description: write your description here
  reference:
  - https://github.com/
  - https://cve.mitre.org/
  metadata:
    max-request: 1
    shodan-query: ""
    verified: true
  yakit-info:
    sign: 14b1cc1e454465de897bad573d53c69c

http:
- method: POST
  path:
  - '{{RootURL}}/'
  headers:
    Content-Type: application/json
  body: '{"key": "value"}'

  redirects: true
  matchers-condition: and
  matchers:
  - id: 1
    type: word
    part: body
    words:
    - keyword
    condition: and


# Generated From WebFuzzer on 2024-03-23 18:08:56`,
			expect: true,
		},
		{
			content: `id: WebFuzzer-Template-yiXBTuUG

info:
  name: WebFuzzer Template yiXBTuUG
  author: god
  severity: low
  description: write your description here
  reference:
  - https://github.com/
  - https://cve.mitre.org/
  metadata:
    max-request: 1
    shodan-query: ""
    verified: true
  yakit-info:
    sign: 273d34bc38497dacb0b2dcacc403093c

http:
- method: POST
  path:
  - '{{RootURL}}/'
  headers:
    Content-Type: application/json
  body: '{"key": "value"}'

  redirects: true
  matchers-condition: and
  matchers:
  - id: 1
    type: word
    part: body
    words:
    - keyword
    - ab
    condition: and


# Generated From WebFuzzer on 2024-03-23 18:08:56`,
			expect: false,
		},
	}
	for _, testCase := range testCases {
		tmp, err := httptpl.CreateYakTemplateFromNucleiTemplateRaw(testCase.content)
		if err != nil {
			t.Fatal(err)
		}
		err = tmp.CheckTemplateRisks()
		if testCase.expect && err != nil && strings.Contains(err.Error(), "签名错误") {
			t.Fatal(fmt.Sprintf("expect no signature error, got: %s, should: %v", err.Error(), tmp.SignMainParams()))
		}
		if !testCase.expect && (err == nil || !strings.Contains(err.Error(), "签名错误")) {
			t.Fatal(fmt.Sprintf("expect signature error: not nil, got: %s", err.Error()))
		}
	}
}

func TestGRPCMUSTPASS_HTTPFuzzer_WebFuzzerSequenceConvertYaml(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}
	testCases := []struct {
		content string
		expect  string
	}{
		{ // 一个请求节点包含两个请求，预期解析为两个请求节点
			content: `
variables:
  username: admin
  password: admin
http:
  - raw:
      - |
        POST /wp-login.php HTTP/1.1
        Host: {{Hostname}}
        Content-Type: application/x-www-form-urlencoded

        log={{username}}&pwd={{password}}&wp-submit=Log+In

      - |
        @timeout: 10s
        POST /wp-admin/admin-ajax.php HTTP/1.1
        Host: {{Hostname}}
        content-type: application/x-www-form-urlencoded

        action=parse-media-shortcode&shortcode=[wptripadvisor_usetemplate+tid="1+AND+(SELECT+42+FROM+(SELECT(SLEEP(6)))b)"]

    disable-cookie: true
    matchers:
      - type: dsl
        dsl:
          - 'duration_2>=6'
          - 'status_code_2 == 200'
          - 'contains(content_type_2, "application/json")'
          - 'contains(body_2, "\"data\":{")'
        condition: and
`,
			expect: `
variables:
  username: admin
  password: admin
http:
  - raw:
      - |
        POST /wp-login.php HTTP/1.1
        Host: {{Hostname}}
        Content-Type: application/x-www-form-urlencoded

        log=admin&pwd=admin&wp-submit=Log+In

      - |
        @timeout: 10s
        POST /wp-admin/admin-ajax.php HTTP/1.1
        Host: {{Hostname}}
        content-type: application/x-www-form-urlencoded

        action=parse-media-shortcode&shortcode=[wptripadvisor_usetemplate+tid="1+AND+(SELECT+42+FROM+(SELECT(SLEEP(6)))b)"]

    disable-cookie: true
    matchers:
      - type: dsl
        dsl:
          - 'duration_2>=6'
          - 'status_code_2 == 200'
          - 'contains(content_type_2, "application/json")'
          - 'contains(body_2, "\"data\":{")'
        condition: and
`,
		},
		{ // path请求，预期解析为raw请求，匹配器不变
			content: `http:
  - method: GET
    path:
      - '{{BaseURL}}/images//////////////////../../../../../../../../etc/passwd'

    matchers-condition: and
    matchers:
      - type: regex
        regex:
          - "root:[x*]:0:0"

      - type: word
        part: header
        words:
          - content/unknown

      - type: status
        status:
          - 200`,
			expect: `http:
- raw:
  - |+
    GET /images//////////////////../../../../../../../../etc/passwd HTTP/1.1
    Host: {{Hostname}}
    User-Agent: Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/83.0.4103.116 Safari/537.36


  matchers-condition: and
  matchers:
  - type: regex
    regex:
    - root:[x*]:0:0

  - type: word
    part: header
    words:
    - content/unknown

  - type: status
    status:
    - "200"
`,
		},
		{ // 一些配置
			content: `http:
  - raw:
      - |
        POST /search-locker-details.php HTTP/1.1
        Host: {{Hostname}}
        Content-Type: application/x-www-form-urlencoded

        searchinput=%E2%80%9C%2F%3E%3Cscript%3Ealert%28document.domain%29%3C%2Fscript%3E&submit=

    disable-cookie: true
    redirects: true
    matchers:
      - type: dsl
        dsl:
          - 'status_code == 200'
          - 'contains(body, "/><script>alert(document.domain)</script>")'
          - 'contains(body, "Bank Locker Management System")'
        condition: and`,
			expect: `http:
  - raw:
      - |
        POST /search-locker-details.php HTTP/1.1
        Host: {{Hostname}}
        Content-Type: application/x-www-form-urlencoded

        searchinput=%E2%80%9C%2F%3E%3Cscript%3Ealert%28document.domain%29%3C%2Fscript%3E&submit=

    disable-cookie: true
    redirects: true
    matchers:
      - type: dsl
        dsl:
          - 'status_code == 200'
          - 'contains(body, "/><script>alert(document.domain)</script>")'
          - 'contains(body, "Bank Locker Management System")'
        condition: and`,
		},
		{ // 包含payload等其它配置，验证生成配置完整且有序
			content: `
variables:
  username: admin
  password: admin
http:
  - raw:
      - |
        GET /fuel/login/ HTTP/1.1
        Host: {{Hostname}}
      - |
        POST /fuel/login/ HTTP/1.1
        Host: {{Hostname}}
        Content-Type: application/x-www-form-urlencoded
        Referer: {{RootURL}}

        user_name={{username}}&password={{password}}&Login=Login&forward=
      - |
        @timeout: 10s
        GET /fuel/pages/items/?search_term=&published=&layout=&limit=50&view_type=list&offset=0&order=asc&col=location+AND+(SELECT+1340+FROM+(SELECT(SLEEP(6)))ULQV)&fuel_inline=0 HTTP/1.1
        Host: {{Hostname}}
        X-Requested-With: XMLHttpRequest
        Referer: {{RootURL}}

    payloads:
      username:
        - admin
      password:
        - admin
    attack: pitchfork
    disable-cookie: true
    matchers:
      - type: dsl
        dsl:
          - 'duration>=6'
          - 'status_code_3 == 200'
          - 'contains(body_1, "FUEL CMS")'
        condition: and`,
			expect: `
variables:
  username: admin
  password: admin
http:
  - raw:
      - |
        GET /fuel/login/ HTTP/1.1
        Host: {{Hostname}}
      - |
        POST /fuel/login/ HTTP/1.1
        Host: {{Hostname}}
        Content-Type: application/x-www-form-urlencoded
        Referer: http://www.example.com
        Content-Length: 65

        user_name=admin&password=admin&Login=Login&forward=
      - |
        @timeout: 10s
        GET /fuel/pages/items/?search_term=&published=&layout=&limit=50&view_type=list&offset=0&order=asc&col=location+AND+(SELECT+1340+FROM+(SELECT(SLEEP(6)))ULQV)&fuel_inline=0 HTTP/1.1
        Host: {{Hostname}}
        X-Requested-With: XMLHttpRequest
        Referer: http://www.example.com

    disable-cookie: true
    matchers:
      - type: dsl
        dsl:
          - 'duration>=6'
          - 'status_code_3 == 200'
          - 'contains(body_1, "FUEL CMS")'
        condition: and
`,
		},
	}
	for i, testCase := range testCases {
		t.Run(fmt.Sprintf("case %d", i), func(t *testing.T) {
			rsp, err := client.ImportHTTPFuzzerTaskFromYaml(context.Background(), &ypb.ImportHTTPFuzzerTaskFromYamlRequest{
				YamlContent: testCase.content,
			})
			if err != nil {
				t.Fatal(err)
			}
			res, err := client.ExportHTTPFuzzerTaskToYaml(context.Background(), &ypb.ExportHTTPFuzzerTaskToYamlRequest{
				Requests: rsp.Requests,
			})

			if err := CompareNucleiYaml(res.YamlContent, testCase.expect); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestGRPCMUSTPASS_HTTPFuzzer_ExtractWithId(t *testing.T) {
	tests := []struct{ raw, expect string }{
		{
			raw: `id: WebFuzzer-Template-wZJQIpvW
http:
- method: GET
  path:
  - '{{RootURL}}/?action=getTitle1'
  - '{{RootURL}}/?action=getTitle2'
  - '{{RootURL}}/?check={{title}}'
  max-redirects: 3
  disable-cookie: true
  matchers-condition: and
  extractors:
  - id: 1
    name: title
    scope: raw
    type: kval
    kval:
    - title


# Generated From WebFuzzer on 2024-01-19 16:42:48`,
			expect: `title1`,
		}, // bind extractor and matcher by id
		{
			raw: `id: WebFuzzer-Template-wZJQIpvW
http:
- method: GET
  path:
  - '{{RootURL}}/?action=getTitle1'
  - '{{RootURL}}/?action=getTitle2'
  - '{{RootURL}}/?check={{title}}'
  max-redirects: 3
  disable-cookie: true
  matchers-condition: and
  extractors:
  - name: title
    scope: raw
    type: kval
    kval:
    - title


# Generated From WebFuzzer on 2024-01-19 16:42:48`,
			expect: `title2`,
		}, // default action: match all package
	}
	for _, test := range tests {
		t.Run(test.raw, func(t *testing.T) {
			yamlRaw := test.raw
			var extractedTitle string
			host, port := utils.DebugMockHTTPHandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				request.ParseForm()
				action := request.Form.Get("action")
				if action != "" {
					switch action {
					case "getTitle1":
						writer.Write([]byte(`{"title": "title1"}`))
					case "getTitle2":
						writer.Write([]byte(`{"title": "title2"}`))
					}
				}
				if v := request.Form.Get("check"); v != "" {
					extractedTitle = v
					writer.Write([]byte(`ok`))
				}
			})
			addr := utils.HostPort(host, port)
			yakTemplate, err := httptpl.CreateYakTemplateFromNucleiTemplateRaw(yamlRaw)
			if err != nil {
				t.Fatal(err)
			}
			sendN, err := yakTemplate.ExecWithUrl("http://"+addr, httptpl.NewConfig())
			if err != nil {
				t.Fatal(err)
			}
			assert.Equal(t, 3, sendN)
			assert.Equal(t, test.expect, extractedTitle)
		})
	}
}

func TestGRPCMUSTPASS_HTTPFuzzer_MatchWithId(t *testing.T) {
	tests := []struct {
		raw    string
		expect bool
	}{
		{
			raw: `id: WebFuzzer-Template-wZJQIpvW
http:
- method: GET
  path:
  - '{{RootURL}}/?action=getTitle1'
  - '{{RootURL}}/?action=getTitle2'
  max-redirects: 3
  disable-cookie: true
  matchers-condition: and
  matchers:
  - id: 1
    type: word
    part: body
    words:
    - title1
    condition: and


# Generated From WebFuzzer on 2024-01-19 16:42:48`,
			expect: true,
		},
		{
			raw: `id: WebFuzzer-Template-wZJQIpvW
http:
- method: GET
  path:
  - '{{RootURL}}/?action=getTitle1'
  - '{{RootURL}}/?action=getTitle2'
  max-redirects: 3
  disable-cookie: true
  matchers-condition: and
  matchers:
  - id: 2
    type: word
    part: body
    words:
    - title1
    condition: and


# Generated From WebFuzzer on 2024-01-19 16:42:48`,
			expect: false,
		},
		{
			raw: `id: WebFuzzer-Template-wZJQIpvW
http:
- method: GET
  path:
  - '{{RootURL}}/?action=getTitle1'
  - '{{RootURL}}/?action=getTitle2'
  max-redirects: 3
  disable-cookie: true
  matchers-condition: and
  matchers:
  - type: word
    part: body
    words:
    - title1
    condition: and


# Generated From WebFuzzer on 2024-01-19 16:42:48`,
			expect: true,
		},
		{
			raw: `id: WebFuzzer-Template-wZJQIpvW
http:
- method: GET
  path:
  - '{{RootURL}}/?action=getTitle1'
  - '{{RootURL}}/?action=getTitle2'
  max-redirects: 3
  disable-cookie: true
  matchers-condition: and
  matchers:
  - type: word
    part: body
    words:
    - title2
    condition: and


# Generated From WebFuzzer on 2024-01-19 16:42:48`,
			expect: true,
		}, // default action: match all package
		{
			raw: `id: WebFuzzer-Template-wZJQIpvW
http:
- method: GET
  path:
  - '{{RootURL}}/?action=getTitle1'
  - '{{RootURL}}/?action=getTitle2'
  max-redirects: 3
  disable-cookie: true
  matchers-condition: and
  matchers:
  - id: 1
    type: word
    part: body
    words:
    - title1
    condition: and
  - id: 1
    type: word
    part: body
    negative: true
    words:
    - title2
    condition: and
  - id: 2
    type: word
    part: body
    words:
    - title2
    condition: and
  - id: 2
    type: word
    part: body
    words:
    - title1
    condition: and

# Generated From WebFuzzer on 2024-01-19 16:42:48`,
			expect: false,
		}, // (package1 has title1) and (package1 not has title2) and (package2 has title2) and (package2 not has title1) => false
		{
			raw: `id: WebFuzzer-Template-wZJQIpvW
http:
- method: GET
  path:
  - '{{RootURL}}/?action=getTitle1'
  - '{{RootURL}}/?action=getTitle2'
  max-redirects: 3
  disable-cookie: true
  matchers-condition: and
  matchers:
  - id: 1
    type: word
    part: body
    words:
    - title1
    condition: and
  - id: 1
    type: word
    part: body
    negative: true
    words:
    - title2
    condition: and
  - id: 2
    type: word
    part: body
    words:
    - title2
    condition: and
  - id: 2
    type: word
    part: body
    negative: true
    words:
    - title1
    condition: and

# Generated From WebFuzzer on 2024-01-19 16:42:48`,
			expect: true,
		}, // (package1 has title1) and (package1 not has title2) and (package2 has title2) and (package2 not has title1) => true
	}
	for _, test := range tests {
		t.Run(test.raw, func(t *testing.T) {
			yamlRaw := test.raw
			host, port := utils.DebugMockHTTPHandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				request.ParseForm()
				action := request.Form.Get("action")
				if action != "" {
					switch action {
					case "getTitle1":
						writer.Write([]byte(`{"title": "title1"}`))
					case "getTitle2":
						writer.Write([]byte(`{"title": "title2"}`))
					default:
						writer.Write(nil)
					}
					return
				}
				writer.Write(nil)
			})
			addr := utils.HostPort(host, port)
			yakTemplate, err := httptpl.CreateYakTemplateFromNucleiTemplateRaw(yamlRaw)
			if err != nil {
				t.Fatal(err)
			}
			sendN, err := yakTemplate.ExecWithUrl("http://"+addr, httptpl.NewConfig(httptpl.WithResultCallback(func(y *httptpl.YakTemplate, reqBulk *httptpl.YakRequestBulkConfig, rsp []*lowhttp.LowhttpResponse, result bool, extractor map[string]interface{}) {
				assert.Equal(t, test.expect, result)
			})))
			if err != nil {
				t.Fatal(err)
			}
			assert.Equal(t, 2, sendN)
		})
	}
}

func TestMatcherId(t *testing.T) {
	raw := `id: WebFuzzer-Template-wZJQIpvW
http:
- method: GET
  path:
  - '{{RootURL}}/?action=getTitle1'
  max-redirects: 3
  disable-cookie: true
  matchers-condition: and
  matchers:
  - id: 1
    type: word
    part: body
    words:
    - title1
    condition: and`
	raw1 := `id: WebFuzzer-Template-wZJQIpvW
http:
- method: GET
  path:
  - '{{RootURL}}/?action=getTitle1'
  - '{{RootURL}}/?action=getTitle1'
  max-redirects: 3
  disable-cookie: true
  matchers-condition: and
  matchers:
  - id: 1
    type: word
    part: body
    words:
    - title1
    condition: and`
	client, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	rsp, err := client.ImportHTTPFuzzerTaskFromYaml(ctx, &ypb.ImportHTTPFuzzerTaskFromYamlRequest{YamlContent: raw})
	if err != nil {
		t.Fatal(err)
	}
	yamlRsp, err := client.ExportHTTPFuzzerTaskToYaml(ctx, &ypb.ExportHTTPFuzzerTaskToYamlRequest{
		Requests:     rsp.Requests,
		TemplateType: "raw",
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(yamlRsp.YamlContent, "- id:") {
		t.Fatal("hide id test failed")
	}

	rsp, err = client.ImportHTTPFuzzerTaskFromYaml(ctx, &ypb.ImportHTTPFuzzerTaskFromYamlRequest{YamlContent: raw1})
	if err != nil {
		t.Fatal(err)
	}
	yamlRsp, err = client.ExportHTTPFuzzerTaskToYaml(ctx, &ypb.ExportHTTPFuzzerTaskToYamlRequest{
		Requests:     rsp.Requests,
		TemplateType: "raw",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(yamlRsp.YamlContent, "- id:") {
		t.Fatal("hide id test failed")
	}
}

func TestGRPCMUSTPASS_HTTPFuzzerTaskToYaml(t *testing.T) {
	client, err := NewLocalClient()
	require.NoError(t, err)
	ctx := context.Background()
	expect := map[string]string{
		"params":       "rand_char",
		"doubleUrl":    "%25%33%31%25%33%32%25%33%33",
		"simpleTag":    "%75%72%6c%28%31%32%33%29",
		"backSlashTag": "%7b%7b%61%61%61",
	}
	actual := map[string]string{}
	host, port := utils.DebugMockHTTPHandlerFunc(func(writer http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		body, err := io.ReadAll(r.Body)
		if err == nil {
			r.Body.Close()
			json.Unmarshal(body, &actual)
		}
	})
	for i := 0; i < 100; i++ {
		rsp, err := client.ExportHTTPFuzzerTaskToYaml(ctx, &ypb.ExportHTTPFuzzerTaskToYamlRequest{
			Requests: &ypb.FuzzerRequests{
				Requests: []*ypb.FuzzerRequest{
					{
						RequestRaw: []byte(`POST / HTTP/1.1
Host: www.example.com

{
	"params":"{{p(a)}}",
	"doubleUrl":"{{url({{url(123)}})}}",
	"simpleTag":"{{url(url(123))}}",
	"backSlashTag":"{{url(\{{aaa)}}"
}`),
						IsHTTPS:                  false,
						PerRequestTimeoutSeconds: 5,
						RedirectTimes:            3,
						Params: []*ypb.FuzzerParamItem{
							{
								Key:   "a",
								Value: "rand_char",
								Type:  "raw",
							},
						},
					},
				},
			},
			TemplateType: "raw",
		})
		require.NoError(t, err)
		templateIns, err := httptpl.CreateYakTemplateFromNucleiTemplateRaw(rsp.YamlContent)
		require.NoError(t, err)
		_, err = templateIns.ExecWithUrl("http://"+utils.HostPort(host, port), httptpl.NewConfig())
		require.NoError(t, err)
		expectVals := utils.NewSet(maps.Values(expect))
		actualVals := utils.NewSet(maps.Values(actual))
		if expectVals.Diff(actualVals).Len() != 0 {
			t.Fatal("expect: ", expect, "actual: ", actual)
		}
	}
}

func TestGRPCMUSTPASS_HTTPFuzzerTaskToYaml_RenderFuzztagParamsToNucleiParams(t *testing.T) {
	packet := `
POST / HTTP/1.1
Content-Type: application/json
Host: www.example.com

{{params(data)}}
`
	packet = RenderFuzztagParamsToNucleiParams(packet)
	assert.Equal(t, packet, `
POST / HTTP/1.1
Content-Type: application/json
Host: www.example.com

{{data}}
`)
}
