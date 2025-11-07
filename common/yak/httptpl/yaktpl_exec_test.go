package httptpl

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/facades"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/netx"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/utils/lowhttp/poc"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
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

func TestMockTest_BasicWordMatcher_EXPR_WithExtractor(t *testing.T) {
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
    extractors:
    - type: dsl
      name: test
      dsl:
        - '"abc" + "123"'
    - type: dsl
      name: test1
      dsl:
        - 'test + "cccc"'
`
	ytpl, err := CreateYakTemplateFromNucleiTemplateRaw(demo)
	if err != nil {
		panic(err)
	}

	checked := false
	var varChecking bool
	config := NewConfig(WithResultCallback(func(y *YakTemplate, reqBulk *YakRequestBulkConfig, rsp []*lowhttp.LowhttpResponse, result bool, extractor map[string]interface{}) {
		if result {
			checked = true
		}
		spew.Dump(extractor)
		if extractor["test"] == "abc123" && extractor["test1"] == (utils.InterfaceToString(extractor["test"])+"cccc") {
			varChecking = true
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

	if !varChecking {
		t.Error("variables from extractor error")
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

	for index, caseItem := range [][]any{
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
id: test2
info:
  name: test2
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
id: test3
info:
  name: test3
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
id: test4
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
		_ = index
		if index != 2 {
			continue
		}
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
			if ExtractResultToString(extractor["a2"]) == expected {
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
			if ExtractResultToString(extractor["a2"]) == expected {
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

func TestMockTest_Extractor_BasicCase_Matcher_RandStr(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	hasToken1, hasToken2 := false, false
	token := ""
	server, port := utils.DebugMockHTTPExContext(ctx, func(req []byte) []byte {
		reqIns, err := lowhttp.ParseBytesToHttpRequest(req)
		if err == nil {
			token = reqIns.URL.Query().Get("token")
			if len(token) > 0 {
				hasToken1 = true
			}
			token2 := reqIns.URL.Query().Get("token2")
			if len(token) > 0 {
				hasToken2 = true
				token = token + token2
			}
		}
		return []byte(fmt.Sprintf("HTTP/1.1 200 OK\r\nContent-Length: %d\r\n\r\n%s", len(token), token))
	})
	spew.Dump(server, port)

	tpl := `id: test1
info:
  name: test1
  author: v1ll4n

requests:
  - raw:
    - |
      GET /?token={{randstr}}&token2={{randstr_1}} HTTP/1.1
      Host: {{Hostname}}

    req-condition: true
    matchers:
      - type: word
        words:
          - "{{randstr}}"
          - "123"
      - type: status
        status:    
          - 200
`
	expected := true

	ytpl, err := CreateYakTemplateFromNucleiTemplateRaw(tpl)
	if err != nil {
		panic(err)
	}

	checked := false
	config := NewConfig(WithResultCallback(func(y *YakTemplate, reqBulk *YakRequestBulkConfig, rsp []*lowhttp.LowhttpResponse, result bool, extractor map[string]interface{}) {
		checked = true
	}))
	_, err = ytpl.Exec(
		config, false,
		[]byte("GET / HTTP/1.1\r\nHost: www.baidu.com\r\n\r\n"),
		lowhttp.WithHost(server), lowhttp.WithPort(port),
	)

	require.Equal(t, expected, checked)
	require.True(t, hasToken1, "no randstr token")
	require.True(t, hasToken2, "no randstr_1 token")
}

func TestMockTest_Extractor_BasicCase_Matcher_StatusCode(t *testing.T) {
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
    
    matchers:
      - type: word
        words:
          - ">aaa</"
      - type: status
        status:    
          - 200
          - 500

`, true},
	} {
		demo, expected := caseItem[0].(string), caseItem[1].(bool)
		expectedMatched := expected
		if len(caseItem) > 2 {
			expectedMatched = caseItem[2].(bool)
		}

		ytpl, err := CreateYakTemplateFromNucleiTemplateRaw(demo)
		if err != nil {
			panic(err)
		}

		checked := false
		config := NewConfig(WithResultCallback(func(y *YakTemplate, reqBulk *YakRequestBulkConfig, rsp []*lowhttp.LowhttpResponse, result bool, extractor map[string]interface{}) {
			if result != expectedMatched {
				panic(1)
			}

			checked = true
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

func TestMockTest_Extractor_BasicCase_Matcher_Raw(t *testing.T) {
	/*


	   # Enhanced by mp on 2022/05/11

	*/
	server, port := utils.DebugMockHTTPWithTimeout(10000*time.Second, []byte(`HTTP/1.1 200 OK
TestDebug: 111

<html>ClassCastException
<head>
<ccc abc="123">aaa</ccc>
</head>
<html>`))
	spew.Dump(server, port)

	for _, caseItem := range [][]any{
		{`
id: CVE-2017-12149

info:
  name: Jboss Application Server - Remote Code Execution
  author: fopina,s0obi
  severity: critical
  description: Jboss Application Server as shipped with Red Hat Enterprise Application Platform 5.2 is susceptible to a remote code execution vulnerability because  the doFilter method in the ReadOnlyAccessFilter of the HTTP Invoker does not restrict classes for which it performs deserialization, thus allowing an attacker to execute arbitrary code via crafted serialized data.
  reference:
    - https://chowdera.com/2020/12/20201229190934023w.html
    - https://github.com/vulhub/vulhub/tree/master/jboss/CVE-2017-12149
    - https://nvd.nist.gov/vuln/detail/CVE-2017-12149
    - https://bugzilla.redhat.com/show_bug.cgi?id=1486220
  classification:
    cvss-metrics: CVSS:3.0/AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:H
    cvss-score: 9.8
    cve-id: CVE-2017-12149
    cwe-id: CWE-502
  tags: java,rce,deserialization,kev,vulhub,cve,cve2017,jboss

requests:
  - raw:
      - |
        POST /invoker/JMXInvokerServlet/ HTTP/1.1
        Host: {{Hostname}}
        Content-Type: application/octet-stream

        {{ base64_decode("rO0ABXNyABNqYXZhLnV0aWwuQXJyYXlMaXN0eIHSHZnHYZ0DAAFJAARzaXpleHAAAAACdwQAAAACdAAJZWxlbWVudCAxdAAJZWxlbWVudCAyeA==") }}

      - |
        POST /invoker/EJBInvokerServlet/ HTTP/1.1
        Host: {{Hostname}}
        Content-Type: application/octet-stream

        {{ base64_decode("rO0ABXNyABNqYXZhLnV0aWwuQXJyYXlMaXN0eIHSHZnHYZ0DAAFJAARzaXpleHAAAAACdwQAAAACdAAJZWxlbWVudCAxdAAJZWxlbWVudCAyeA==") }}

      - |
        POST /invoker/readonly HTTP/1.1
        Host: {{Hostname}}
        Content-Type: application/octet-stream

        {{ base64_decode("rO0ABXNyABNqYXZhLnV0aWwuQXJyYXlMaXN0eIHSHZnHYZ0DAAFJAARzaXpleHAAAAACdwQAAAACdAAJZWxlbWVudCAxdAAJZWxlbWVudCAyeA==") }}

    matchers-condition: and
    matchers:
      - type: word
        part: body
        words:
          - "ClassCastException"

      - type: status
        status:
          - 200
          - 500
`, true},
	} {
		demo, expected := caseItem[0].(string), caseItem[1].(bool)
		expectedMatched := expected
		if len(caseItem) > 2 {
			expectedMatched = caseItem[2].(bool)
		}

		ytpl, err := CreateYakTemplateFromNucleiTemplateRaw(demo)
		if err != nil {
			panic(err)
		}

		checked := false
		config := NewConfig(WithResultCallback(func(y *YakTemplate, reqBulk *YakRequestBulkConfig, rsp []*lowhttp.LowhttpResponse, result bool, extractor map[string]interface{}) {
			if result != expectedMatched {
				panic(1)
			}

			checked = true
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

func TestRenderPackage(t *testing.T) {
	server, port := utils.DebugMockHTTPWithTimeout(10000*time.Second, []byte(`HTTP/1.1 200 OK
TestDebug: 111
`))
	spew.Dump(server, port)

	for _, caseItem := range [][]any{
		{`
id: CVE-2017-12149

info:
  name: Jboss Application Server - Remote Code Execution
  author: fopina,s0obi
  severity: critical
  description: Jboss Application Server as shipped with Red Hat Enterprise Application Platform 5.2 is susceptible to a remote code execution vulnerability because  the doFilter method in the ReadOnlyAccessFilter of the HTTP Invoker does not restrict classes for which it performs deserialization, thus allowing an attacker to execute arbitrary code via crafted serialized data.
  reference:
    - https://chowdera.com/2020/12/20201229190934023w.html
    - https://github.com/vulhub/vulhub/tree/master/jboss/CVE-2017-12149
    - https://nvd.nist.gov/vuln/detail/CVE-2017-12149
    - https://bugzilla.redhat.com/show_bug.cgi?id=1486220
  classification:
    cvss-metrics: CVSS:3.0/AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:H
    cvss-score: 9.8
    cve-id: CVE-2017-12149
    cwe-id: CWE-502
  tags: java,rce,deserialization,kev,vulhub,cve,cve2017,jboss

requests:
  - raw:
      - |
        POST /invoker/JMXInvokerServlet/ HTTP/1.1
        Host: {{Hostname}}
        Content-Type: application/octet-stream

        aaa

      - |
        POST /invoker/EJBInvokerServlet/ HTTP/1.1
        Host: {{Hostname}}
        Content-Type: application/octet-stream

        aaa
    matchers:
      - type: word
        part: body
        words:
          - "ClassCastException"

      - type: status
        status:
          - 200
          - 500
  - method: GET
    path:
      - '{{BaseURL}}/wp-content/'
    matchers-condition: and
    matchers:
      - type: word
        part: body
        words:
          - "ClassCastException"

      - type: status
        status:
          - 200
          - 500
`, true},
	} {
		demo, expected := caseItem[0].(string), caseItem[1].(bool)
		expectedMatched := expected
		_ = expectedMatched
		if len(caseItem) > 2 {
			expectedMatched = caseItem[2].(bool)
		}

		ytpl, err := CreateYakTemplateFromNucleiTemplateRaw(demo)
		if err != nil {
			panic(err)
		}

		expect := []string{
			`POST /invoker/JMXInvokerServlet/ HTTP/1.1
Host: www.baidu.com
Content-Type: application/octet-stream

aaa`,
			`POST /invoker/EJBInvokerServlet/ HTTP/1.1
Host: www.baidu.com
Content-Type: application/octet-stream

aaa`,
			`GET /wp-content/ HTTP/1.1
Host: www.baidu.com
User-Agent: Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/83.0.4103.116 Safari/537.36

`,
		}
		i := 0
		_ = expect
		config := NewConfig(WithBeforeSendPackage(func(data []byte, isHttps bool) []byte {
			defer func() { i++ }()
			stringData := string(bytes.Replace(data, []byte("\r"), []byte{}, -1))
			assert.Equal(t, expect[i], stringData, "unexpect packet")
			return data
		}), WithConcurrentInTemplates(1))
		n, err := ytpl.ExecWithUrl("http://www.baidu.com", config, lowhttp.WithHost(server), lowhttp.WithPort(port))
		if err != nil {
			panic(err)
		}
		assert.Equal(t, 3, n, "send packet number is wrong")

	}
}

func TestMockTest_OOB(t *testing.T) {
	dnsserver := facades.MockDNSServer(context.Background(), "aaa.asdgiqwfkbas.com", 8901, func(record string, domain string) string {
		return "1.1.1.1"
	})
	server, port := utils.DebugMockHTTPHandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Write([]byte("ok"))
		u := request.URL.Query().Get("consumerUri")
		urlIns, err := url.Parse(u)
		if err != nil {
			t.Fatal(err)
			return
		}
		netx.LookupFirst(urlIns.Host, netx.WithTimeout(time.Second), netx.WithDNSServers(dnsserver), netx.WithDNSDisableSystemResolver(true))
	})
	tmp := `id: CVE-2017-9506

info:
  name: Atlassian Jira IconURIServlet - Cross-Site Scripting/Server-Side Request Forgery
  author: pdteam
  severity: medium
  description: The Atlassian Jira IconUriServlet of the OAuth Plugin from version 1.3.0 before version 1.9.12 and from version 2.0.0 before version 2.0.4 contains a cross-site scripting vulnerability which allows remote attackers to access the content of internal network resources and/or perform an attack via Server Side Request Forgery.
  remediation: |
    Apply the latest security patches provided by Atlassian to mitigate these vulnerabilities.
  reference:
    - http://dontpanic.42.nl/2017/12/there-is-proxy-in-your-atlassian.html
    - https://ecosystem.atlassian.net/browse/OAUTH-344
    - https://medium.com/bugbountywriteup/piercing-the-veil-server-side-request-forgery-to-niprnet-access-171018bca2c3
    - https://nvd.nist.gov/vuln/detail/CVE-2017-9506
  classification:
    cvss-metrics: CVSS:3.0/AV:N/AC:L/PR:N/UI:R/S:C/C:L/I:L/A:N
    cvss-score: 6.1
    cve-id: CVE-2017-9506
    cwe-id: CWE-918
    epss-score: 0.00575
    epss-percentile: 0.75469
    cpe: cpe:2.3:a:atlassian:oauth:1.3.0:*:*:*:*:*:*:*
  metadata:
    max-request: 1
    vendor: atlassian
    product: oauth
    shodan-query: http.component:"Atlassian Jira"
  tags: cve,cve2017,atlassian,jira,ssrf,oast

http:
  - raw:
      - |
        GET /plugins/servlet/oauth/users/icon-uri?consumerUri=http://{{interactsh-url}} HTTP/1.1
        Host: {{Hostname}}
        Origin: {{BaseURL}}

    matchers:
      - type: word
        part: interactsh_protocol # Confirms the HTTP Interaction
        words:
          - "http"

# digest: 4a0a0047304502203f149b24ebd177d43629ee418d28fc0878939ccdd4283537cbaced55a753b59f0221008b8e75e9de7c7ddd6fd2ffe85e574fc9b523f0980011ed7a71df7e6d8475ec4a:922c64590222798bb761d5b6d8e72950`
	tmpIns, err := CreateYakTemplateFromNucleiTemplateRaw(tmp)
	if err != nil {
		t.Fatal(err)
	}
	ok := false
	config := NewConfig(WithOOBRequireCallback(func(f ...float64) (string, string, error) {
		return "a.aaa.asdgiqwfkbas.com", "token", nil
	}), WithOOBRequireCheckingTrigger(func(s string, runtimeID string, f ...float64) (string, []byte) {
		if s == "token" {
			ok = true
			return "dns", []byte("")
		}
		return "", []byte("")
	}))
	tmpIns.ExecWithUrl("http://www.baidu.com", config, lowhttp.WithHost(server), lowhttp.WithPort(port))
	if !ok {
		t.Error("test oob error")
	}
}

func TestMockTest_Body(t *testing.T) {
	server, port := utils.DebugMockHTTPWithTimeout(10000*time.Second, []byte(`HTTP/1.1 200 OK
TestDebug: 111

Post Meta Setting Deleted Successfully
`))
	spew.Dump(server, port)

	for _, caseItem := range [][]any{
		{`id: CVE-2022-0693

info:
  name: WordPress Master Elements <=8.0 - SQL Injection
  author: theamanrawat
  severity: critical
  description: |
    WordPress Master Elements plugin through 8.0 contains a SQL injection vulnerability. The plugin does not validate and escape the meta_ids parameter of its remove_post_meta_condition AJAX action, available to both unauthenticated and authenticated users, before using it in a SQL statement. An attacker can possibly obtain sensitive information, modify data, and/or execute unauthorized administrative operations in the context of the affected site.
  remediation: |
    Update to the latest version of WordPress Master Elements plugin (>=8.1) to mitigate the SQL Injection vulnerability.
  reference:
    - https://wpscan.com/vulnerability/a72bf075-fd4b-4aa5-b4a4-5f62a0620643
    - https://wordpress.org/plugins/master-elements
    - https://nvd.nist.gov/vuln/detail/CVE-2022-0693
  classification:
    cvss-metrics: CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:H
    cvss-score: 9.8
    cve-id: CVE-2022-0693
    cwe-id: CWE-89
    epss-score: 0.01519
    epss-percentile: 0.85576
    cpe: cpe:2.3:a:devbunch:master_elements:*:*:*:*:*:wordpress:*:*
  metadata:
    verified: true
    max-request: 1
    vendor: devbunch
    product: master_elements
    framework: wordpress
  tags: unauth,wpscan,wp-plugin,wp,sqli,wordpress,master-elements,cve,cve2022

http:
  - raw:
      - |
        @timeout: 10s
        GET /wp-admin/admin-ajax.php?meta_ids=1+AND+(SELECT+3066+FROM+(SELECT(SLEEP(6)))CEHy)&action=remove_post_meta_condition HTTP/1.1
        Host: {{Hostname}}

    matchers:
      - type: dsl
        dsl:
          - 'duration>=0'
          - 'status_code == 200'
          - 'contains(body, "Post Meta Setting Deleted Successfully")'
          - 'contains(body_1, "Post Meta Setting Deleted Successfully")'
        condition: and
# digest: 4a0a00473045022100d388bf1ba27db50c2339d0dfda041fa175e2b526fdf0eaa555ce4f128caa2c3e02206509a935080f2a103a7539246f094281fdee05b4f25403196fa77f93a3880b40:922c64590222798bb761d5b6d8e72950`, true},
	} {
		demo, expected := caseItem[0].(string), caseItem[1].(bool)
		expectedMatched := expected
		_ = expectedMatched
		if len(caseItem) > 2 {
			expectedMatched = caseItem[2].(bool)
		}

		ytpl, err := CreateYakTemplateFromNucleiTemplateRaw(demo)
		if err != nil {
			panic(err)
		}
		check := false
		config := NewConfig(WithResultCallback(func(y *YakTemplate, reqBulk *YakRequestBulkConfig, rsp []*lowhttp.LowhttpResponse, result bool, extractor map[string]interface{}) {
			check = result
		}))
		_, err = ytpl.ExecWithUrl("http://www.baidu.com", config, lowhttp.WithHost(server), lowhttp.WithPort(port))
		if err != nil {
			t.Fatal(err)
		}
		if !check {
			t.Fatal("check body error")
		}
	}
}

func TestMockTest_StopAtFirstMatch(t *testing.T) {
	server, port := utils.DebugMockHTTPWithTimeout(10000*time.Second, []byte(`HTTP/1.1 200 OK
TestDebug: 111

Post Meta Setting Deleted Successfully
`))
	spew.Dump(server, port)

	for _, caseItem := range [][]any{
		{`http:
  - method: GET
    path:
      - "{{BaseURL}}///////../../../etc/passwd"
      - "{{BaseURL}}/static///////../../../../etc/passwd"
      - "{{BaseURL}}///../app.js"

    stop-at-first-match: true

    matchers-condition: and
    matchers:
      - type: regex
        regex:
          - "root:.*:0:0:"
          - "app.listen"
        part: body
        condition: or

      - type: status
        status:
          - 200`, true},
	} {
		demo, expected := caseItem[0].(string), caseItem[1].(bool)
		expectedMatched := expected
		_ = expectedMatched
		if len(caseItem) > 2 {
			expectedMatched = caseItem[2].(bool)
		}

		ytpl, err := CreateYakTemplateFromNucleiTemplateRaw(demo)
		if err != nil {
			panic(err)
		}
		check := false
		config := NewConfig(WithResultCallback(func(y *YakTemplate, reqBulk *YakRequestBulkConfig, rsp []*lowhttp.LowhttpResponse, result bool, extractor map[string]interface{}) {
			check = result
		}))
		_, err = ytpl.ExecWithUrl("http://www.baidu.com", config, lowhttp.WithHost(server), lowhttp.WithPort(port))
		if err != nil {
			t.Fatal(err)
		}
		if check {
			t.Fatal("check stop-at-first error")
		}
	}
}

func TestMatcherContainsTag(t *testing.T) {
	host, port := utils.DebugMockHTTPHandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Write([]byte(request.Header["A"][0]))
	})
	addr := fmt.Sprintf("http://%s:%d", host, port)
	tmpl := `
variables:
    flag: "{{md5('{{unix_time(10)}}')}}"
http:
- method: POST
  path:
  - '{{RootURL}}'
  headers:
    a: "{{flag}}"
  matchers:
    - type: word
      part: body
      words:
        - "{{flag}}"
`
	ytpl, err := CreateYakTemplateFromNucleiTemplateRaw(tmpl)
	if err != nil {
		panic(err)
	}
	var ok bool
	config := NewConfig(WithResultCallback(func(y *YakTemplate, reqBulk *YakRequestBulkConfig, rsp []*lowhttp.LowhttpResponse, result bool, extractor map[string]interface{}) {
		ok = result
	}))
	_, err = ytpl.ExecWithUrl(addr, config)
	if !ok {
		t.FailNow()
	}
}

func TestHTTPTpl_VariableType(t *testing.T) {
	host, port := utils.DebugMockHTTPHandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Write([]byte(request.Header["A"][0]))
		writer.Write([]byte(request.Header["B"][0]))
		body, _ := io.ReadAll(request.Body)
		writer.Write([]byte(body))
	})
	addr := fmt.Sprintf("http://%s:%d", host, port)
	token, token2, token3 := utils.RandStringBytes(5), utils.RandStringBytes(5), utils.RandStringBytes(5)
	tmpl := fmt.Sprintf(`
variables:
    fuzztag: "@fuzztag{{regen(%[1]s)}}"
    nuclei: "{{md5('%[2]s')}}"
    raw: "@raw%[3]s"
http:
- method: POST
  path:
  - '{{RootURL}}'
  headers:
    a: "{{fuzztag}}"
    b: "{{nuclei}}"
  body: '{{raw}}'
  matchers:
    - type: word
      part: body
      words:
        - "%[1]s"
    - type: word
      part: body
      words:
        - "%[4]s"
    - type: word
      part: body
      words:
        - "%[3]s"
  matchers-condition: and
`, token, token2, token3, codec.Md5(token2))
	ytpl, err := CreateYakTemplateFromNucleiTemplateRaw(tmpl)
	require.NoError(t, err)
	var ok bool
	var rspRaw []byte
	config := NewConfig(WithResultCallback(func(y *YakTemplate, reqBulk *YakRequestBulkConfig, rsp []*lowhttp.LowhttpResponse, result bool, extractor map[string]interface{}) {
		ok = result
		rspRaw = rsp[0].RawPacket
	}))
	_, err = ytpl.ExecWithUrl(addr, config)
	require.Truef(t, ok, "not matched, Response:\n%s", string(rspRaw))
}

func TestHTTPTpl_Variable_With_Fuzztag_Params(t *testing.T) {
	host, port := utils.DebugMockHTTPHandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Write([]byte(request.Header["A"][0]))
	})
	addr := fmt.Sprintf("http://%s:%d", host, port)
	token := utils.RandStringBytes(5)
	tmpl := fmt.Sprintf(`
id: WebFuzzer-Template-gsKlwUxp

info:
  name: WebFuzzer Template gsKlwUxp
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
    sign: c0abc6a540717b4dec61cd347b30ccaa

variables:
  a: '@raw%[1]s'
  payload1: '@fuzztag{{p(a)}}'
http:
- raw:
  - |-
    @timeout: 30s
    GET / HTTP/1.1
    Host: {{Hostname}}
    A: {{payload1}}
  matchers:
    - type: word
      part: body
      words:
        - "%[1]s"

  max-redirects: 3
  matchers-condition: and
`, token)
	ytpl, err := CreateYakTemplateFromNucleiTemplateRaw(tmpl)
	require.NoError(t, err)
	var ok bool
	var rspRaw []byte
	config := NewConfig(WithResultCallback(func(y *YakTemplate, reqBulk *YakRequestBulkConfig, rsp []*lowhttp.LowhttpResponse, result bool, extractor map[string]interface{}) {
		ok = result
		rspRaw = rsp[0].RawPacket
	}))
	_, err = ytpl.ExecWithUrl(addr, config)
	require.Truef(t, ok, "not matched, Response:\n%s", string(rspRaw))
}

func TestHTTPTpl_Variable_With_List_Fuzztag(t *testing.T) {
	host, port := utils.DebugMockHTTPHandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Write([]byte(request.Header["A"][0]))
	})
	addr := fmt.Sprintf("http://%s:%d", host, port)

	var randFlags []string
	for i := 0; i < 10; i++ {
		randFlags = append(randFlags, utils.RandStringBytes(5))
	}
	sort.Strings(randFlags)
	randFlagsStr := strings.Join(randFlags, "|")
	tmpl := fmt.Sprintf(`
variables:
  payload1: '@fuzztag{{list(%s)}}'
http:
- raw:
  - |-
    @timeout: 30s
    GET / HTTP/1.1
    Host: {{Hostname}}
    A: {{payload1}}
  max-redirects: 3
`, randFlagsStr)
	ytpl, err := CreateYakTemplateFromNucleiTemplateRaw(tmpl)
	require.NoError(t, err)

	// 所有请求提取到的Header A
	var allHeadersA []string

	config := NewConfig(WithResultCallback(func(y *YakTemplate, reqBulk *YakRequestBulkConfig, rsp []*lowhttp.LowhttpResponse, result bool, extractor map[string]interface{}) {
		for _, r := range rsp {
			val := lowhttp.GetHTTPPacketHeader(r.RawRequest, "A")
			if val == "" {
				continue
			}
			allHeadersA = append(allHeadersA, val)
		}
	}))
	_, err = ytpl.ExecWithUrl(addr, config)

	sort.Strings(allHeadersA)
	require.ElementsMatch(t, allHeadersA, randFlags)
}

func TestHTTPTpl_Path_Support_Variable(t *testing.T) {
	host, port := utils.DebugMockHTTPHandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Write([]byte(request.URL.Path))
	})
	addr := fmt.Sprintf("http://%s:%d", host, port)
	token := utils.RandStringBytes(5)
	tmpl := fmt.Sprintf(`
variables:
    raw: "@raw%[1]s"
http:
- method: POST
  path:
  - '{{RootURL}}/{{raw}}'
  matchers:
    - type: word
      part: body
      words:
        - "%[1]s"
  matchers-condition: and
`, token)
	ytpl, err := CreateYakTemplateFromNucleiTemplateRaw(tmpl)
	require.NoError(t, err)
	var ok bool
	var rspRaw []byte
	config := NewConfig(WithResultCallback(func(y *YakTemplate, reqBulk *YakRequestBulkConfig, rsp []*lowhttp.LowhttpResponse, result bool, extractor map[string]interface{}) {
		ok = result
		rspRaw = rsp[0].RawPacket
	}))
	_, err = ytpl.ExecWithUrl(addr, config)
	require.Truef(t, ok, "not matched, Response:\n%s", string(rspRaw))
}

func TestMockTest_interactsh(t *testing.T) {
	for i := 0; i < 5; i++ {
		ok := item_testMockTest_interactsh(t)
		if ok {
			return
		}
	}
}

func item_testMockTest_interactsh(t *testing.T) bool {
	rootDomain := utils.RandStringBytes(15) + ".com"
	token := strings.ToLower(utils.RandStringBytes(15))
	tokenDomain := token + "." + rootDomain

	interactshProtocol := make(map[string]string)
	interactshRequest := make(map[string][][]byte)

	port := utils.GetRandomAvailableUDPPort()
	dnsServer := facades.MockDNSServer(context.Background(), rootDomain, port, func(record string, domain string) string {
		if strings.Contains(domain, token) {
			interactshProtocol[token] = "dns"
		}
		return "127.0.0.1"
	})

	utils.WaitConnect(dnsServer, 3)

	httpServerHost, httpServerPort := utils.DebugMockHTTPEx(func(req []byte) []byte {
		reqStr := string(req)
		if strings.Contains(reqStr, token) {
			interactshProtocol[token] = "http"
			interactshRequest[token] = append(interactshRequest[token], req)
		}
		return []byte("HTTP/1.1 200 OK\r\n\r\n")
	})

	sendToken := utils.RandStringBytes(5)
	server, port := utils.DebugMockHTTPHandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Write([]byte("ok"))
		u := request.URL.Query().Get("consumerUri")
		urlIns, err := url.Parse(u)
		if err != nil {
			t.Fatal(err)
			return
		}
		urlIns.RawQuery = fmt.Sprintf("a=%s", sendToken)
		a := urlIns.String()
		_ = a
		_, _, err = poc.DoGET(urlIns.String(), poc.WithDNSServers(dnsServer), poc.WithDNSNoCache(true))
		require.NoError(t, err)
		if err != nil {
			t.Fatal(err)
			t.FailNow()
		}
	})
	tmp := fmt.Sprintf(`id: CVE-2017-9506

info:
  name: Atlassian Jira IconURIServlet - Cross-Site Scripting/Server-Side Request Forgery
  author: pdteam
  severity: medium
  description: The Atlassian Jira IconUriServlet of the OAuth Plugin from version 1.3.0 before version 1.9.12 and from version 2.0.0 before version 2.0.4 contains a cross-site scripting vulnerability which allows remote attackers to access the content of internal network resources and/or perform an attack via Server Side Request Forgery.
  remediation: |
    Apply the latest security patches provided by Atlassian to mitigate these vulnerabilities.
  reference:
    - http://dontpanic.42.nl/2017/12/there-is-proxy-in-your-atlassian.html
    - https://ecosystem.atlassian.net/browse/OAUTH-344
    - https://medium.com/bugbountywriteup/piercing-the-veil-server-side-request-forgery-to-niprnet-access-171018bca2c3
    - https://nvd.nist.gov/vuln/detail/CVE-2017-9506
  classification:
    cvss-metrics: CVSS:3.0/AV:N/AC:L/PR:N/UI:R/S:C/C:L/I:L/A:N
    cvss-score: 6.1
    cve-id: CVE-2017-9506
    cwe-id: CWE-918
    epss-score: 0.00575
    epss-percentile: 0.75469
    cpe: cpe:2.3:a:atlassian:oauth:1.3.0:*:*:*:*:*:*:*
  metadata:
    max-request: 1
    vendor: atlassian
    product: oauth
    shodan-query: http.component:"Atlassian Jira"
  tags: cve,cve2017,atlassian,jira,ssrf,oast

http:
  - raw:
      - |
        GET /plugins/servlet/oauth/users/icon-uri?consumerUri=http://{{interactsh-url}} HTTP/1.1
        Host: {{Hostname}}
        Origin: {{BaseURL}}

    matchers:
      - type: word
        part: interactsh_protocol # Confirms the HTTP Interaction
        words:
          - "http"
      - type: word
        part: interactsh_request
        words:
          - "%s"
          `, sendToken)
	tmpIns, err := CreateYakTemplateFromNucleiTemplateRaw(tmp)
	if err != nil {
		t.Fatal(err)
	}
	ok := false
	config := NewConfig(WithOOBRequireCallback(func(f ...float64) (string, string, error) {
		return fmt.Sprintf("%s:%d", tokenDomain, httpServerPort), token, nil
	}), WithOOBRequireCheckingTrigger(func(s string, runtimeID string, f ...float64) (string, []byte) {
		log.Infof("interactsh protocol:%v\n", interactshProtocol[s])
		if interactshProtocol[s] == "http" {
			for _, request := range interactshRequest[s] {
				if strings.Contains(string(request), sendToken) {
					ok = true
					return "http,dns", request
				}
			}
		}
		ok = false
		return "", []byte("")
	}))
	log.Infof("vul http server:%s:%d\n", server, port)
	log.Infof("interactsh http server:%s:%d\n", httpServerHost, httpServerPort)
	log.Infof("interactsh dns server:%s\n", dnsServer)
	tmpIns.ExecWithUrl("http://www.baidu.com", config, lowhttp.WithHost(server), lowhttp.WithPort(port))
	return ok
}

func TestMatcher_KeepDSLReturnType(t *testing.T) {
	randomKey := utils.RandStringBytes(16)

	host, port := utils.DebugMockHTTPHandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		a, b := request.URL.Query().Get("a"), request.URL.Query().Get("b")
		aInt, err := strconv.Atoi(a)
		writer.Write([]byte(fmt.Sprintf("a=%s\n", a)))
		writer.Write([]byte(fmt.Sprintf("b=%s\n", b)))
		if err != nil {
			writer.Write([]byte("a not int"))
			return
		}
		bInt, err := strconv.Atoi(b)
		if err != nil {
			writer.Write([]byte("b not int"))
			return
		}
		if aInt == bInt {
			writer.Write([]byte("a should not same as b"))
			return
		}
		writer.Write([]byte(fmt.Sprintf("%s=%s", randomKey, strconv.Itoa(aInt+bInt))))
	})

	//
	addr := fmt.Sprintf("http://%s:%d", host, port)
	tmpl := fmt.Sprintf(`
id: WebFuzzer-Template-rce-hex_decode
info:
  name: Struts2 046
  author: admin
  severity: high
  metadata:
    max-request: 1
    shodan-query: ""
    verified: true
  yakit-info:
    sign: 52dc9bdb52d04dc20036dbd8313ed085
variables:
  r1: '{{rand_int(10000)}}'
  r2: '{{rand_int(10000)}}'
http:
- raw:
  - |
    @timeout: 30s
    GET /?a={{r1}}&b={{r2}} HTTP/1.1
    Host: {{Hostname}}
    User-Agent: Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/127.0.0.0 Safari/537.36
  max-redirects: 3
  matchers-condition: and
  matchers:
  - type: dsl
    part: body
    dsl:
    - contains(raw,r1+r2)
    condition: and
  extractors:
  - id: 1
    name: v
    scope: raw
    type: kval
    kval:
    - %s
`, randomKey)
	ytpl, err := CreateYakTemplateFromNucleiTemplateRaw(tmpl)
	if err != nil {
		panic(err)
	}
	var ok bool
	config := NewConfig(WithResultCallback(func(y *YakTemplate, reqBulk *YakRequestBulkConfig, rsp []*lowhttp.LowhttpResponse, result bool, extractor map[string]interface{}) {
		spew.Dump(extractor)
		if len(rsp) > 0 {
			spew.Dump(rsp[0].RawPacket)
		}
		ok = result
	}))
	_, err = ytpl.ExecWithUrl(addr, config)
	if !ok {
		t.FailNow()
	}
}

func TestMatcherPathContainsPayload(t *testing.T) {
	host, port := utils.DebugMockHTTPHandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Write([]byte(request.Header["A"][0]))
	})
	addr := fmt.Sprintf("http://%s:%d", host, port)
	tmpl := `
variables:
    flag: "{{md5('{{unix_time(10)}}')}}"
http:
- method: POST
  path:
  - '{{RootURL}}{{filepath}}'
  payloads:
    filepath:
      - /a
      - /b
  headers:
    a: "{{flag}}"
  matchers:
    - type: word
      part: body
      words:
        - "{{flag}}"
`
	ytpl, err := CreateYakTemplateFromNucleiTemplateRaw(tmpl)
	if err != nil {
		panic(err)
	}
	var ok bool
	config := NewConfig(WithResultCallback(func(y *YakTemplate, reqBulk *YakRequestBulkConfig, rsp []*lowhttp.LowhttpResponse, result bool, extractor map[string]interface{}) {
		ok = result
	}))
	n, err := ytpl.ExecWithUrl(addr, config)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, 2, n)
}

func TestHttpTplDisableCookie(t *testing.T) {
	token := utils.RandStringBytes(10)
	cookieCheck := false
	server, port := utils.DebugMockHTTPEx(func(req []byte) []byte {
		if bytes.Contains(req, []byte(token)) {
			cookieCheck = true
		}
		return []byte(fmt.Sprintf("HTTP/1.1 200 OK\r\nSet-Cookie: Test=%s; \r\n\r\n", token))
	})
	spew.Dump(server, port)

	demo := `
id: test1
info:
  name: test1
  author: v1ll4n

http:
- method: GET
  path:
  - '{{RootURL}}/'
  - '{{RootURL}}/1'
  headers: {}

  max-redirects: 3
  disable-cookie: true
  matchers-condition: and
`
	ytpl, err := CreateYakTemplateFromNucleiTemplateRaw(demo)
	require.NoError(t, err)

	_, err = ytpl.Exec(
		nil, false,
		[]byte("GET / HTTP/1.1\r\nHost: www.baidu.com\r\n\r\n"),
		lowhttp.WithHost(server), lowhttp.WithPort(port),
	)
	require.NoError(t, err)
	require.False(t, cookieCheck)

	demo = `
id: test1
info:
  name: test1
  author: v1ll4n

http:
- method: GET
  path:
  - '{{RootURL}}/'
  - '{{RootURL}}/1'
  headers: {}

  max-redirects: 3
  matchers-condition: and
`
	ytpl, err = CreateYakTemplateFromNucleiTemplateRaw(demo)
	require.NoError(t, err)

	_, err = ytpl.Exec(
		nil, false,
		[]byte("GET / HTTP/1.1\r\nHost: www.baidu.com\r\n\r\n"),
		lowhttp.WithHost(server), lowhttp.WithPort(port),
	)
	require.NoError(t, err)
	require.True(t, cookieCheck)

}
