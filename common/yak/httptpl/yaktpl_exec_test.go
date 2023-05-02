package httptpl

import (
	"github.com/davecgh/go-spew/spew"
	"testing"
	"time"
	"yaklang/common/log"
	"yaklang/common/utils"
	"yaklang/common/utils/lowhttp"
)

func TestMockTest_SmokingTest(t *testing.T) {
	server, port := utils.DebugMockHTTP([]byte(`HTTP/1.1 200 OK
TestDebug: 111`))
	spew.Dump(server, port)

	demo := `
id: test1
info:
  name: test1
  author: v1ll4n

requests:
  - raw:
    - |
      GET / HTTP/1.1
      Host: {{Hostname}}
      
      abc
    matchers:
    - type: word
      words:
        - "abc"
`
	ytpl, err := CreateYakTemplateFromNucleiTemplateRaw(demo)
	if err != nil {
		panic(err)
	}

	_, err = ytpl.Exec(
		nil, false,
		[]byte("GET / HTTP/1.1\r\nHost: www.baidu.com\r\n\r\n"),
		lowhttp.WithHost(server), lowhttp.WithPort(port),
	)
	if err != nil {
		panic(err)
	}
}

func TestMockTest_BasicWordMatcher(t *testing.T) {
	server, port := utils.DebugMockHTTP([]byte(`HTTP/1.1 200 OK
TestDebug: 111

ccc`))
	spew.Dump(server, port)

	demo := `
id: test1
info:
  name: test1
  author: v1ll4n

requests:
  - raw:
    - |
      GET / HTTP/1.1
      Host: {{Hostname}}
      
      abc
    matchers:
    - type: word
      words:
        - "ccc"
`
	ytpl, err := CreateYakTemplateFromNucleiTemplateRaw(demo)
	if err != nil {
		panic(err)
	}

	checked := false
	config := NewConfig(WithResultCallback(func(y *YakTemplate, reqBulk *YakRequestBulkConfig, rsp []*lowhttp.LowhttpResponse, result bool, extractor map[string]interface{}) {
		if result {
			checked = true
		}
	}))
	_, err = ytpl.Exec(
		config, false,
		[]byte("GET / HTTP/1.1\r\nHost: www.baidu.com\r\n\r\n"),
		lowhttp.WithHost(server), lowhttp.WithPort(port),
	)
	if err != nil {
		panic(err)
	}

	if !checked {
		t.Error("not checked")
		t.FailNow()
	}
}

func TestMockTest_BasicWordMatcher_ReqCondition(t *testing.T) {
	server, port := utils.DebugMockHTTP([]byte(`HTTP/1.1 200 OK
TestDebug: 111

ccc`))
	spew.Dump(server, port)

	demo := `
id: test1
info:
  name: test1
  author: v1ll4n

requests:
  - raw:
    - |
      GET / HTTP/1.1
      Host: {{Hostname}}
      
      abc
    req-condition: true
    matchers:
    - type: word
      words:
        - "ccc"
`
	ytpl, err := CreateYakTemplateFromNucleiTemplateRaw(demo)
	if err != nil {
		panic(err)
	}

	checked := false
	config := NewConfig(WithResultCallback(func(y *YakTemplate, reqBulk *YakRequestBulkConfig, rsp []*lowhttp.LowhttpResponse, result bool, extractor map[string]interface{}) {
		if result {
			checked = true
		}
	}))
	_, err = ytpl.Exec(
		config, false,
		[]byte("GET / HTTP/1.1\r\nHost: www.baidu.com\r\n\r\n"),
		lowhttp.WithHost(server), lowhttp.WithPort(port),
	)
	if err != nil {
		panic(err)
	}

	if !checked {
		t.Error("not checked")
		t.FailNow()
	}
}

func TestMockTest_BasicWordMatcher_ReqConditionMultiReq(t *testing.T) {
	server, port := utils.DebugMockHTTPWithTimeout(10*time.Second, []byte(`HTTP/1.1 200 OK
TestDebug: 111

ccc`))
	spew.Dump(server, port)

	demo := `
id: test1
info:
  name: test1
  author: v1ll4n

requests:
  - raw:
    - |
      GET / HTTP/1.1
      Host: {{Hostname}}
      
      abc
    - |
      GET / HTTP/1.1
      Host: {{Hostname}}
      
      abc
    req-condition: true
    matchers:
    - type: word
      condition: and
      words:
        - "ccc"
`
	ytpl, err := CreateYakTemplateFromNucleiTemplateRaw(demo)
	if err != nil {
		panic(err)
	}

	checked := false
	config := NewConfig(WithResultCallback(func(y *YakTemplate, reqBulk *YakRequestBulkConfig, rsp []*lowhttp.LowhttpResponse, result bool, extractor map[string]interface{}) {
		if result {
			checked = true
		}
	}))
	_, err = ytpl.Exec(
		config, false,
		[]byte("GET / HTTP/1.1\r\nHost: www.baidu.com\r\n\r\n"),
		lowhttp.WithHost(server), lowhttp.WithPort(port),
	)
	if err != nil {
		panic(err)
	}

	if !checked {
		t.Error("not checked")
		t.FailNow()
	}
}

func TestMockTest_BasicWordMatcher_ReqConditionMultiReq_MULTICOND(t *testing.T) {
	server, port := utils.DebugMockHTTPWithTimeout(10*time.Second, []byte(`HTTP/1.1 200 OK
TestDebug: 111

ccc`))
	spew.Dump(server, port)

	demo := `
id: test1
info:
  name: test1
  author: v1ll4n

requests:
  - raw:
    - |
      GET / HTTP/1.1
      Host: {{Hostname}}
      
      abc
    - |
      GET / HTTP/1.1
      Host: {{Hostname}}
      
      abc
    req-condition: true
    matchers:
    - type: word
      condition: and
      words:
        - "ccc"
        - "HQ@"
`
	ytpl, err := CreateYakTemplateFromNucleiTemplateRaw(demo)
	if err != nil {
		panic(err)
	}

	checked := false
	config := NewConfig(WithResultCallback(func(y *YakTemplate, reqBulk *YakRequestBulkConfig, rsp []*lowhttp.LowhttpResponse, result bool, extractor map[string]interface{}) {
		if !result {
			checked = true
		}
	}))
	_, err = ytpl.Exec(
		config, false,
		[]byte("GET / HTTP/1.1\r\nHost: www.baidu.com\r\n\r\n"),
		lowhttp.WithHost(server), lowhttp.WithPort(port),
	)
	if err != nil {
		panic(err)
	}

	if !checked {
		t.Error("not checked")
		t.FailNow()
	}
}

func TestMockTest_BasicWordMatcher_EXPR(t *testing.T) {
	server, port := utils.DebugMockHTTPWithTimeout(10*time.Second, []byte(`HTTP/1.1 200 OK
TestDebug: 111

ccc`))
	spew.Dump(server, port)

	demo := `
id: test1
info:
  name: test1
  author: v1ll4n

requests:
  - raw:
    - |
      GET / HTTP/1.1
      Host: {{Hostname}}
      
      abc
    - |
      GET / HTTP/1.1
      Host: {{Hostname}}
      
      abc
    matchers:
    - type: dsl
      condition: or
      dsl:
        - "dump(body); contains(body, \"cc\")"
`
	ytpl, err := CreateYakTemplateFromNucleiTemplateRaw(demo)
	if err != nil {
		panic(err)
	}

	checked := false
	config := NewConfig(WithResultCallback(func(y *YakTemplate, reqBulk *YakRequestBulkConfig, rsp []*lowhttp.LowhttpResponse, result bool, extractor map[string]interface{}) {
		if result {
			checked = true
		}
	}))
	_, err = ytpl.Exec(
		config, false,
		[]byte("GET / HTTP/1.1\r\nHost: www.baidu.com\r\n\r\n"),
		lowhttp.WithHost(server), lowhttp.WithPort(port),
	)
	if err != nil {
		panic(err)
	}

	if !checked {
		t.Error("not checked")
		t.FailNow()
	}
}

func TestMockTest_BasicWordMatcher_EXPR2(t *testing.T) {
	server, port := utils.DebugMockHTTPWithTimeout(10*time.Second, []byte(`HTTP/1.1 200 OK
TestDebug: 111

ccc`))
	spew.Dump(server, port)

	demo := `
id: test1
info:
  name: test1
  author: v1ll4n

requests:
  - raw:
    - |
      GET / HTTP/1.1
      Host: {{Hostname}}
      
      abc
    - |
      GET / HTTP/1.1
      Host: {{Hostname}}
      
      abc
    matchers:
    - type: dsl
      condition: or
      dsl:
        - "dump(body_2); contains(body_2, \"cc\")"
`
	ytpl, err := CreateYakTemplateFromNucleiTemplateRaw(demo)
	if err != nil {
		panic(err)
	}

	checked := false
	config := NewConfig(WithResultCallback(func(y *YakTemplate, reqBulk *YakRequestBulkConfig, rsp []*lowhttp.LowhttpResponse, result bool, extractor map[string]interface{}) {
		if result {
			checked = true
		}
	}))
	_, err = ytpl.Exec(
		config, false,
		[]byte("GET / HTTP/1.1\r\nHost: www.baidu.com\r\n\r\n"),
		lowhttp.WithHost(server), lowhttp.WithPort(port),
	)
	if err != nil {
		panic(err)
	}

	if !checked {
		t.Error("not checked")
		t.FailNow()
	}
}

func TestMockTest_BasicWordMatcher_EXPR2_N(t *testing.T) {
	server, port := utils.DebugMockHTTPWithTimeout(10000*time.Second, []byte(`HTTP/1.1 200 OK
TestDebug: 111

ccc`))
	spew.Dump(server, port)

	demo := `
id: test1
info:
  name: test1
  author: v1ll4n

requests:
  - raw:
    - |
      GET / HTTP/1.1
      Host: {{Hostname}}
      
      abc
    - |
      GET / HTTP/1.1
      Host: {{Hostname}}
      
      abc
    matchers:
    - type: dsl
      condition: or
      dsl:
        - "dump(body_2); contains(body_2, \"ccccccccccc\")"
`
	ytpl, err := CreateYakTemplateFromNucleiTemplateRaw(demo)
	if err != nil {
		panic(err)
	}

	checked := false
	config := NewConfig(WithResultCallback(func(y *YakTemplate, reqBulk *YakRequestBulkConfig, rsp []*lowhttp.LowhttpResponse, result bool, extractor map[string]interface{}) {
		if !result {
			checked = true
		}
	}))
	_, err = ytpl.Exec(
		config, false,
		[]byte("GET / HTTP/1.1\r\nHost: www.baidu.com\r\n\r\n"),
		lowhttp.WithHost(server), lowhttp.WithPort(port),
	)
	if err != nil {
		panic(err)
	}

	if !checked {
		t.Error("not checked")
		t.FailNow()
	}
}

func TestMockTest_BasicWordMatcher_EXPR2_N2(t *testing.T) {
	server, port := utils.DebugMockHTTPWithTimeout(10000*time.Second, []byte(`HTTP/1.1 200 OK
TestDebug: 111

ccc`))
	spew.Dump(server, port)

	demo := `
id: test1
info:
  name: test1
  author: v1ll4n

requests:
  - raw:
    - |
      GET / HTTP/1.1
      Host: {{Hostname}}
      
      abc
    - |
      GET / HTTP/1.1
      Host: {{Hostname}}
      
      abc
    matchers:
    - type: dsl
      condition: or
      dsl:
        - "dump(body_2); contains(body_2, \"ccccccccccc\")"
        - "dump(body_2); contains(body_2, \"cc\")"
`
	ytpl, err := CreateYakTemplateFromNucleiTemplateRaw(demo)
	if err != nil {
		panic(err)
	}

	checked := false
	config := NewConfig(WithResultCallback(func(y *YakTemplate, reqBulk *YakRequestBulkConfig, rsp []*lowhttp.LowhttpResponse, result bool, extractor map[string]interface{}) {
		if result {
			checked = true
		}
	}))
	_, err = ytpl.Exec(
		config, false,
		[]byte("GET / HTTP/1.1\r\nHost: www.baidu.com\r\n\r\n"),
		lowhttp.WithHost(server), lowhttp.WithPort(port),
	)
	if err != nil {
		panic(err)
	}

	if !checked {
		t.Error("not checked")
		t.FailNow()
	}
}

func TestMockTest_BasicWordMatcher_EXPR2_N2q(t *testing.T) {
	server, port := utils.DebugMockHTTPWithTimeout(10000*time.Second, []byte(`HTTP/1.1 200 OK
TestDebug: 111

ccc`))
	spew.Dump(server, port)

	demo := `
id: test1
info:
  name: test1
  author: v1ll4n

requests:
  - raw:
    - |
      GET / HTTP/1.1
      Host: {{Hostname}}
      
      abc
    - |
      GET / HTTP/1.1
      Host: {{Hostname}}
      
      abc
    matchers:
    - type: dsl
      condition: and
      dsl:
        - "dump(body_2); contains(body_2, \"ccccccccccc\")"
        - "dump(body_2); contains(body_2, \"cc\")"
`
	ytpl, err := CreateYakTemplateFromNucleiTemplateRaw(demo)
	if err != nil {
		panic(err)
	}

	checked := false
	config := NewConfig(WithResultCallback(func(y *YakTemplate, reqBulk *YakRequestBulkConfig, rsp []*lowhttp.LowhttpResponse, result bool, extractor map[string]interface{}) {
		if !result {
			checked = true
		}
	}))
	_, err = ytpl.Exec(
		config, false,
		[]byte("GET / HTTP/1.1\r\nHost: www.baidu.com\r\n\r\n"),
		lowhttp.WithHost(server), lowhttp.WithPort(port),
	)
	if err != nil {
		panic(err)
	}

	if !checked {
		t.Error("not checked")
		t.FailNow()
	}
}

func TestMockTest_BasicWordMatcher_EXPR2_N2q2(t *testing.T) {
	server, port := utils.DebugMockHTTPWithTimeout(10000*time.Second, []byte(`HTTP/1.1 200 OK
TestDebug: 111

ccc`))
	spew.Dump(server, port)

	demo := `
id: test1
info:
  name: test1
  author: v1ll4n

requests:
  - raw:
    - |
      GET / HTTP/1.1
      Host: {{Hostname}}
      
      abc
    - |
      GET / HTTP/1.1
      Host: {{Hostname}}
      
      abc
    req-condition: true
    matchers:
    - type: dsl
      condition: and
      dsl:
        - "dump(body_2); contains(body_2, \"ccccccccccc\")"
        - "dump(body_2); contains(body_2, \"cc\")"
`
	ytpl, err := CreateYakTemplateFromNucleiTemplateRaw(demo)
	if err != nil {
		panic(err)
	}

	checked := false
	config := NewConfig(WithResultCallback(func(y *YakTemplate, reqBulk *YakRequestBulkConfig, rsp []*lowhttp.LowhttpResponse, result bool, extractor map[string]interface{}) {
		if !result {
			checked = true
		}
	}))
	_, err = ytpl.Exec(
		config, false,
		[]byte("GET / HTTP/1.1\r\nHost: www.baidu.com\r\n\r\n"),
		lowhttp.WithHost(server), lowhttp.WithPort(port),
	)
	if err != nil {
		panic(err)
	}

	if !checked {
		t.Error("not checked")
		t.FailNow()
	}
}

func TestMockTest_BasicWordMatcher_EXPR2_N2q3(t *testing.T) {
	server, port := utils.DebugMockHTTPWithTimeout(10000*time.Second, []byte(`HTTP/1.1 200 OK
TestDebug: 111

ccc`))
	spew.Dump(server, port)

	for _, caseItem := range [][]any{
		{`
id: test1
info:
  name: test1
  author: v1ll4n

requests:
  - raw:
    - |
      GET / HTTP/1.1
      Host: {{Hostname}}
      
      abc
    - |
      GET / HTTP/1.1
      Host: {{Hostname}}
      
      abc
    req-condition: true
    matchers:
    - type: dsl
      condition: or
      dsl:
        - "dump(body_2); contains(body_2, \"ccccccccccc\")"
        - "dump(body_2); contains(body_2, \"cc\")"
        - "dump(body_1); contains(body_1, \"cc\")"
`, true},

		{`
id: test1
info:
  name: test1
  author: v1ll4n

requests:
  - raw:
    - |
      GET / HTTP/1.1
      Host: {{Hostname}}
      
      abc
    - |
      GET / HTTP/1.1
      Host: {{Hostname}}
      
      abc
    req-condition: true

    matchers-condition: true
    matchers:
    - type: dsl
      condition: or
      dsl:
        - "dump(body_2); contains(body_2, \"ccccccccccc\")"
        - "dump(body_2); contains(body_2, \"cc\")"
        - "dump(body_1); contains(body_1, \"cc\")"
    - type: word
      words:
        - hhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhh
`, false},
		{`
id: test1
info:
  name: test1
  author: v1ll4n

requests:
  - raw:
    - |
      GET / HTTP/1.1
      Host: {{Hostname}}
      
      abc
    - |
      GET / HTTP/1.1
      Host: {{Hostname}}
      
      abc
    req-condition: true

    matchers-condition: or
    matchers:
    - type: dsl
      condition: or
      dsl:
        - "dump(body_2); contains(body_2, \"ccccccccccc\")"
        - "dump(body_2); contains(body_2, \"cc\")"
        - "dump(body_1); contains(body_1, \"cc\")"
    - type: word
      words:
        - hhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhh
`, true},
		{`
id: test1
info:
  name: test1
  author: v1ll4n

requests:
  - raw:
    - |
      GET / HTTP/1.1
      Host: {{Hostname}}
      
      abc
    - |
      GET / HTTP/1.1
      Host: {{Hostname}}
      
      abc
    req-condition: true

    matchers-condition: or
    matchers:
    - type: dsl
      condition: and
      dsl:
        - "dump(body_2); contains(body_2, \"ccccccccccc\")"
        - "dump(body_2); contains(body_2, \"cc\")"
        - "dump(body_1); contains(body_1, \"cc\")"
    - type: word
      words:
        - hhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhh
`, false},
		{`
id: test1
info:
  name: test1
  author: v1ll4n

variables:
  a1: "ccc"

requests:
  - raw:
    - |
      GET / HTTP/1.1
      Host: {{Hostname}}
      
      abc
    - |
      GET / HTTP/1.1
      Host: {{Hostname}}
      
      abc
    req-condition: true

    matchers-condition: or
    matchers:
    - type: dsl
      condition: and
      dsl:
        - "contains(body_1, a1)"
        - "contains(body_2, a1)"
    - type: word
      words:
        - hhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhh
`, true},
	} {
		demo, expected := caseItem[0].(string), caseItem[1].(bool)

		ytpl, err := CreateYakTemplateFromNucleiTemplateRaw(demo)
		if err != nil {
			panic(err)
		}

		checked := false
		config := NewConfig(WithResultCallback(func(y *YakTemplate, reqBulk *YakRequestBulkConfig, rsp []*lowhttp.LowhttpResponse, result bool, extractor map[string]interface{}) {
			if result == expected {
				checked = true
			}
		}))
		_, err = ytpl.Exec(
			config, false,
			[]byte("GET / HTTP/1.1\r\nHost: www.baidu.com\r\n\r\n"),
			lowhttp.WithHost(server), lowhttp.WithPort(port),
		)
		if err != nil {
			panic(err)
		}

		if !checked {
			t.Error("not checked")
			println(demo)
			t.FailNow()
		}
	}
}

func TestMockTest_Extractor_BasicCase(t *testing.T) {
	server, port := utils.DebugMockHTTPWithTimeout(10000*time.Second, []byte(`HTTP/1.1 200 OK
TestDebug: 111

cccabbbccc

dddddd`))
	spew.Dump(server, port)

	for _, caseItem := range [][]any{
		{`
id: test1
info:
  name: test1
  author: v1ll4n

requests:
  - raw:
    - |
      GET / HTTP/1.1
      Host: {{Hostname}}
      
      abc
    - |
      GET / HTTP/1.1
      Host: {{Hostname}}
      
      abc
    req-condition: true
    extractors:
      - type: regex
        name: a2
        regex: ccc([^c]*)cc
      - type: regex
        name: a3
        group: 1
        regex: ccc([^c]*)cc

`, "cccabbbcc"},
		{`
id: test1
info:
  name: test1
  author: v1ll4n

requests:
  - raw:
    - |
      GET / HTTP/1.1
      Host: {{Hostname}}
      
      abc
    - |
      GET / HTTP/1.1
      Host: {{Hostname}}
      
      abc
    req-condition: true
    extractors:
      - type: regex
        name: a3
        regex: ccc([^c]*)cc
      - type: regex
        name: a2
        group: 1
        regex: ccc([^c]*)cc

`, "abbb"},
		{`
id: test1
info:
  name: test1
  author: v1ll4n

requests:
  - raw:
    - |
      GET / HTTP/1.1
      Host: {{Hostname}}
      
      abc
    - |
      GET / HTTP/1.1
      Host: {{Hostname}}
      
      abc
    req-condition: true
    matchers:
      - type: dsl
        condition: and
        dsl:
          - a2=="abbb"
          - a3=="cccabbbcc"
    extractors:
      - type: regex
        name: a3
        regex: ccc([^c]*)cc
      - type: regex
        name: a2
        group: 1
        regex: ccc([^c]*)cc

`, "abbb", true},
		{`
id: test1
info:
  name: test1
  author: v1ll4n

variables:
  a1: "dddddd"
  a4: "{{rand_base(100)}}{{a1}}"

requests:
  - raw:
    - |
      GET / HTTP/1.1
      Host: {{Hostname}}
      
      abc
    - |
      GET / HTTP/1.1
      Host: {{Hostname}}
      
      abc
    req-condition: true
    matchers:
      - type: word
        words:
          - "{{a1}}"
      - type: dsl
        condition: and
        dsl:
          - a2=="abbb"
          - a3=="cccabbbcc"
          - dump(a4,a1,a2,a3); dump(len(a4)); contains(a4, "dddddd") && len(a4) == 106
          - a1=="dddddd"
    extractors:
      - type: regex
        name: a3
        regex: ccc([^c]*)cc
      - type: regex
        name: a2
        group: 1
        regex: ccc([^c]*)cc

`, "abbb", true},
	} {
		demo, expected := caseItem[0].(string), caseItem[1].(string)
		expectedMatched := false
		if len(caseItem) > 2 {
			expectedMatched = caseItem[2].(bool)
		}

		ytpl, err := CreateYakTemplateFromNucleiTemplateRaw(demo)
		if err != nil {
			panic(err)
		}

		checked := false
		config := NewConfig(WithResultCallback(func(y *YakTemplate, reqBulk *YakRequestBulkConfig, rsp []*lowhttp.LowhttpResponse, result bool, extractor map[string]interface{}) {
			spew.Dump(extractor)
			if extractor["a2"].(string) == expected {
				checked = true
			}

			if len(caseItem) == 3 {
				log.Info("extract with matcher")
			}

			if len(caseItem) == 3 && result != expectedMatched {
				checked = false
				panic("not matched（matcher with extractor）")
			}
		}))
		_, err = ytpl.Exec(
			config, false,
			[]byte("GET / HTTP/1.1\r\nHost: www.baidu.com\r\n\r\n"),
			lowhttp.WithHost(server), lowhttp.WithPort(port),
		)
		if err != nil {
			panic(err)
		}

		if !checked {
			t.Error("not checked")
			println(demo)
			t.FailNow()
		}
	}
}

func TestMockTest_Extractor_BasicCase_Extractor_XPATH(t *testing.T) {
	server, port := utils.DebugMockHTTPWithTimeout(10000*time.Second, []byte(`HTTP/1.1 200 OK
TestDebug: 111

<html>
<head>
<ccc abc="123">aaa</ccc>
</head>
<html>`))
	spew.Dump(server, port)

	for _, caseItem := range [][]any{
		{`
id: test1
info:
  name: test1
  author: v1ll4n

requests:
  - raw:
    - |
      GET / HTTP/1.1
      Host: {{Hostname}}
      
      abc
    - |
      GET / HTTP/1.1
      Host: {{Hostname}}
      
      abc
    req-condition: true
    extractors:
      - type: xpath
        name: a2
        attribute: abc
        xpath: 
          - //ccc

`, "123"},
		{`
id: test1
info:
  name: test1
  author: v1ll4n

requests:
  - raw:
    - |
      GET / HTTP/1.1
      Host: {{Hostname}}
      
      abc
    - |
      GET / HTTP/1.1
      Host: {{Hostname}}
      
      abc
    req-condition: true
    extractors:
      - type: xpath
        name: a2
        xpath: 
          - //ccc

`, "aaa"},
	} {
		demo, expected := caseItem[0].(string), caseItem[1].(string)
		expectedMatched := false
		if len(caseItem) > 2 {
			expectedMatched = caseItem[2].(bool)
		}

		ytpl, err := CreateYakTemplateFromNucleiTemplateRaw(demo)
		if err != nil {
			panic(err)
		}

		checked := false
		config := NewConfig(WithResultCallback(func(y *YakTemplate, reqBulk *YakRequestBulkConfig, rsp []*lowhttp.LowhttpResponse, result bool, extractor map[string]interface{}) {
			spew.Dump(extractor)
			if extractor["a2"].(string) == expected {
				checked = true
			}

			if len(caseItem) == 3 {
				log.Info("extract with matcher")
			}

			if len(caseItem) == 3 && result != expectedMatched {
				checked = false
				panic("not matched（matcher with extractor）")
			}
		}))
		_, err = ytpl.Exec(
			config, false,
			[]byte("GET / HTTP/1.1\r\nHost: www.baidu.com\r\n\r\n"),
			lowhttp.WithHost(server), lowhttp.WithPort(port),
		)
		if err != nil {
			panic(err)
		}

		if !checked {
			t.Error("not checked")
			println(demo)
			t.FailNow()
		}
	}
}
