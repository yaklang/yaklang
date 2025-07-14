package yakgrpc

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestGRPCMUSTPASS_LANGUAGE_SMOKING_EVALUATE_PLUGIN(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}

	type testCase struct {
		code      string
		err       string
		codeTyp   string
		zeroScore bool // is score == 0 ?
	}
	TestSmokingEvaluatePlugin := func(tc testCase) {
		name, clearFunc, err := yakit.CreateTemporaryYakScriptEx(tc.codeTyp, tc.code)
		require.NoError(t, err)
		defer clearFunc()
		rsp, err := client.SmokingEvaluatePlugin(context.Background(), &ypb.SmokingEvaluatePluginRequest{
			PluginName: name,
		})
		require.NoError(t, err)

		checking := false
		fmt.Printf("result: %#v \n", rsp.String())
		if tc.zeroScore && rsp.Score != 0 {
			// want score == 0 but get !0
			t.Fatal("this test should have score = 0")
		}
		if !tc.zeroScore && rsp.Score == 0 {
			// want score != 0 but get 0
			t.Fatal("this test shouldn't have score = 0")
		}
		if tc.err == "" {
			if len(rsp.Results) != 0 {
				t.Fatal("this test should have no result")
			}
		} else {
			for _, r := range rsp.Results {
				if strings.Contains(r.String(), tc.err) {
					checking = true
				}
			}
			if !checking {
				t.Fatalf("should have %s", tc.err)
			}
		}
	}

	t.Run("test negative alarm", func(t *testing.T) {
		TestSmokingEvaluatePlugin(testCase{
			code: `
yakit.AutoInitYakit()
handle = result => {
	yakit.Info("HELLO")
	risk.NewRisk("http://baidu.com", risk.cve(""))
}`,
			err:       "[Negative Alarm]",
			codeTyp:   "port-scan",
			zeroScore: false,
		})
	})

	t.Run("test undefine variable", func(t *testing.T) {
		TestSmokingEvaluatePlugin(testCase{
			code: `
yakit.AutoInitYakit()
handle = result => {
	yakit.Info(bacd)
	risk.NewRisk("http://baidu.com", risk.cve(""))
}`,
			codeTyp:   "port-scan",
			err:       "Value undefine",
			zeroScore: true,
		})
	})

	t.Run("test just warning", func(t *testing.T) {
		TestSmokingEvaluatePlugin(testCase{
			code: `
yakit.AutoInitYakit()
handle = result => {
}`,
			err:       "empty block",
			codeTyp:   "port-scan",
			zeroScore: false,
		})
	})

	t.Run("test yak ", func(t *testing.T) {
		TestSmokingEvaluatePlugin(testCase{
			code: `
yakit.AutoInitYakit()

# Input your code!
			`,

			err:       "",
			codeTyp:   "yak",
			zeroScore: false,
		})
	})

	t.Run("test nuclei false positive", func(t *testing.T) {
		TestSmokingEvaluatePlugin(testCase{
			code: `id: CVE-2024-32030

info:
  name: CVE-2024-32030 JMX Metrics Collection JNDI RCE
  severity: critical

requests:
  - method: GET
    path:
      - "{{BaseURL}}/api/clusters/malicious-cluster"
    matchers:
      - type: word
        part: body
        words:
          - "malicious-cluster"
`,
			err:       "误报",
			codeTyp:   "nuclei",
			zeroScore: false,
		})
	})

	t.Run("test nuclei positive", func(t *testing.T) {
		TestSmokingEvaluatePlugin(testCase{
			code: `id: WebFuzzer-Template-idZEfBnT
info:
  name: WebFuzzer Template idZEfBnT
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
    sign: 39724ac438ac2b32ae79defc1f3eac22
variables:
  aa: '{{rand_char(5)}}'
  bb: '{{rand_char(6)}}'
http:
- method: POST
  path:
  - '{{RootURL}}/'
  headers:
    Content-Type: application/json
  body: "echo {{aa}}+{{bb}}"
  # attack: pitchfork
  max-redirects: 3
  matchers-condition: and
  matchers:
  - id: 1
    type: dsl
    part: body
    dsl:
    - '{{contains(body,aa+bb)}}'
    condition: and`,
			err:       "",
			codeTyp:   "nuclei",
			zeroScore: false,
		})
	})

	t.Run("test localhost bypass", func(t *testing.T) {
		TestSmokingEvaluatePlugin(testCase{
			code: `
yakit.AutoInitYakit()
mirrorHTTPFlow = func(isHttps /*bool*/, url /*string*/, req /*[]byte*/, rsp /*[]byte*/, body /*[]byte*/) {
    if str.Contains(url, '127.0.0.1') {
        return
    }
    risk.NewRisk(url, risk.solution("b"),risk.description("a"))
}`,
			err:       "误报",
			codeTyp:   "mitm",
			zeroScore: false,
		})
	})

	t.Run("test loose conditions", func(t *testing.T) {
		TestSmokingEvaluatePlugin(testCase{
			code: `
yakit.AutoInitYakit()
mirrorHTTPFlow = func(isHttps /*bool*/, url /*string*/, req /*[]byte*/, rsp /*[]byte*/, body /*[]byte*/) {
   if rsp.Contains("HTTP/1.1 200") && len(body) > 0{
	risk.NewRisk(url, risk.solution("b"),risk.description("a"))
	}
}`,
			err:       "误报",
			codeTyp:   "mitm",
			zeroScore: false,
		})
		TestSmokingEvaluatePlugin(testCase{
			code: `
yakit.AutoInitYakit()
mirrorHTTPFlow = func(isHttps /*bool*/, url /*string*/, req /*[]byte*/, rsp /*[]byte*/, body /*[]byte*/) {
	if str.MatchAllOfSubString(rsp, "121"){
		risk.NewRisk(url, risk.solution("b"),risk.description("a"))
	}
}`,
			err:       "误报",
			codeTyp:   "mitm",
			zeroScore: false,
		})

		TestSmokingEvaluatePlugin(testCase{
			code: `
yakit.AutoInitYakit()
mirrorHTTPFlow = func(isHttps /*bool*/, url /*string*/, req /*[]byte*/, rsp /*[]byte*/, body /*[]byte*/) {
	if str.MatchAllOfSubString(rsp, "application/json"){
		risk.NewRisk(url, risk.solution("b"),risk.description("a"))
	}
}`,
			err:       "误报",
			codeTyp:   "mitm",
			zeroScore: false,
		})

	})

	t.Run("test httpflow check", func(t *testing.T) {
		TestSmokingEvaluatePlugin(testCase{
			code: `
yakit.AutoInitYakit()
mirrorHTTPFlow = func(isHttps /*bool*/, url /*string*/, req /*[]byte*/, rsp /*[]byte*/, body /*[]byte*/) {
	panic("a")
}`,
			err:       "冒烟测试失败",
			codeTyp:   "mitm",
			zeroScore: false, // has request check will not check httpflow count,so score is not 0
		})
	})

	t.Run("test http flow count", func(t *testing.T) {
		TestSmokingEvaluatePlugin(testCase{
			code: fmt.Sprintf(`
yakit.AutoInitYakit()
mirrorHTTPFlow = func(isHttps /*bool*/, url /*string*/, req /*[]byte*/, rsp /*[]byte*/, body /*[]byte*/) {
	packet1 = %s 
	begin_time=time.now()
    rsp,req,_ = poc.HTTP(packet1, 
    poc.params({"target":"127.0.0.1"}),
    poc.https(isHttps),
    poc.redirectTimes(0),
    )
}`, "`GET / HTTP/1.1\nHost: `"),
			err:       "逻辑测试失败",
			codeTyp:   "mitm",
			zeroScore: false,
		})
	})

	t.Run("test cli mitm plugin", func(t *testing.T) {
		TestSmokingEvaluatePlugin(testCase{
			code: `
url = cli.String("url",cli.setRequired(true))
cli.check()
mirrorHTTPFlow = func(isHttps /*bool*/, url /*string*/, req /*[]byte*/, rsp /*[]byte*/, body /*[]byte*/) {
	println(url)
}`,
			err:       "",
			codeTyp:   "mitm",
			zeroScore: false,
		})
	})

	t.Run("test static analyze rule", func(t *testing.T) {
		TestSmokingEvaluatePlugin(testCase{
			code: `
yakit.AutoInitYakit()
handle = result => {
	yakit.Info(bacd)
	risk.NewRisk("http://baidu.com" )
}`,
			codeTyp:   "port-scan",
			err:       "risk.NewRisk should",
			zeroScore: true,
		})
	})

	t.Run("test static analyze score rule", func(t *testing.T) {
		TestSmokingEvaluatePlugin(testCase{
			code: `
yakit.AutoInitYakit()
handle = result => {
	yakit.Info(bacd)
	exec.System("a")
}`,
			codeTyp:   "port-scan",
			err:       "forbid command exec library",
			zeroScore: true,
		})
	})

	t.Run("has send http request check", func(t *testing.T) {
		TestSmokingEvaluatePlugin(testCase{
			code: `
mirrorHTTPFlow = func(isHttps /*bool*/, url /*string*/, req /*[]byte*/, rsp /*[]byte*/, body /*[]byte*/) {
	println("1")
}
`,
			err:       "",
			codeTyp:   "mitm",
			zeroScore: false,
		})
	})
	t.Run("bad syntax prog should not panic backend", func(t *testing.T) {
		TestSmokingEvaluatePlugin(testCase{
			code: `
    select {
    case resp = <-doneChan:
    case <-time.after(timeout * 1000):
      yakit.Warn("请求超时，放弃本次请求: %s %s", method, url)
      resp = nil
    }

    debugLogRequest(method, url, headers, body)
    debugLogResponse(resp)

    if resp != nil && resp.statusCode() < 500 {
      break
    }
    time.sleep(delay)
  }
  return resp
}
`,
			err:       "编译失败",
			codeTyp:   "yak",
			zeroScore: true,
		})
	})
}
