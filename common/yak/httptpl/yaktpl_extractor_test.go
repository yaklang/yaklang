package httptpl

import (
	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/log"
	utils2 "github.com/yaklang/yaklang/common/utils"
	"testing"
)

func TestYakExtractor_Execute(t *testing.T) {
	for index, extractor := range [][]any{
		{ // extractor_test: 正则提取一条数据
			`HTTP/1.1 200 Ok
Content-Type: text/html; charset=utf-8

<!DOCTYPE html>
<html></html>`,
			&YakExtractor{
				Name:   "k1",
				Type:   "regex",
				Groups: []string{`DOCTYPE \w{4}`},
			},
			"k1",
			"DOCTYPE html",
		},
		{ // extractor_test: 使用正则捕获提取一条数据
			`HTTP/1.1 200 Ok
Content-Type: text/html; charset=utf-8

<!DOCTYPE html>
<html></html>`,
			&YakExtractor{
				Name:             "k1",
				Type:             "regex",
				RegexpMatchGroup: []int{1},
				Groups:           []string{`DOCTYPE (\w{4})`},
			},
			"k1",
			"html",
		},
		{ // extractor_test: 使用正则捕获，从header提取一条数据
			`HTTP/1.1 200 Ok
Content-Type: text/html; charset=utf-8

<!DOCTYPE html>
<html></html>`,
			&YakExtractor{
				Name:             "k1",
				Type:             "regex",
				RegexpMatchGroup: []int{1},
				Scope:            "header",
				Groups:           []string{`DOCTYPE (\w{4})`},
			},
			"k1",
			"",
		},
		{ // extractor_test: 使用json提取器，从body提取一条数据
			`HTTP/1.1 200 Ok
Content-Type: text/html; charset=utf-8

{"abc": "12312312", "ccc": 123}`,
			&YakExtractor{
				Name:   "k1",
				Type:   "json",
				Scope:  "body",
				Groups: []string{`.abc`},
			},
			"k1",
			"12312312",
		},
		{ // extractor_test: 使用json提取器，从body提取一条数据(测试提取不同变量)
			`HTTP/1.1 200 Ok
Content-Type: text/html; charset=utf-8

{"abc": "12312312", "ccc": 123}`,
			&YakExtractor{
				Name:   "k1",
				Type:   "json",
				Scope:  "body",
				Groups: []string{`.ccc`},
			},
			"k1",
			"123",
		},
		{ // extractor_test: 使用xpath提取器，从body提取元素属性
			`HTTP/1.1 200 Ok
Content-Type: text/html; charset=utf-8

<html>
	<head><title>ABC</title></head>
</html>`,
			&YakExtractor{
				Name:   "k1",
				Type:   "xpath",
				Scope:  "body",
				Groups: []string{`//title/text()`},
			},
			"k1",
			"ABC",
		},
		{ // extractor_test: 使用xpath提取器，从body提取多条元素属性
			`HTTP/1.1 200 Ok
Content-Type: text/html; charset=utf-8

<html>
	<head><title>ABC</title></head>
	<div>abc</div>
	<div>def</div>
</html>`,
			&YakExtractor{
				Name:   "k1",
				Type:   "xpath",
				Scope:  "body",
				Groups: []string{`//div/text()`},
			},
			"k1",
			"abc,def",
		},
		{ // extractor_test: 使用xpath提取器，使用更复杂的xpath语法从body提取元素属性
			`HTTP/1.1 200 Ok
Content-Type: text/html; charset=utf-8

<?xml version="1.0" encoding="UTF-8"?>
<products>
  <product>
    <name>iPhone 13</name>
    <price>999.00</price>
    <description>The latest iPhone from Apple.</description>
    <reviews>
      <review>
        <rating>4.5</rating>
        <comment>Great phone, but a bit expensive.</comment>
      </review>
      <review>
        <rating>3.0</rating>
        <comment>Not impressed, I expected more.</comment>
      </review>
    </reviews>
  </product>
  <product>
    <name>Samsung Galaxy S21</name>
    <price>799.00</price>
    <description>The latest Galaxy phone from Samsung.</description>
    <reviews>
      <review>
        <rating>5.0</rating>
        <comment>Amazing phone, great value for money.</comment>
      </review>
      <review>
        <rating>4.0</rating>
        <comment>Good phone, but battery life could be better.</comment>
      </review>
    </reviews>
  </product>
</products>
`,
			&YakExtractor{
				Name:   "k1",
				Type:   "xpath",
				Scope:  "body",
				Groups: []string{`/products/product[name='Samsung Galaxy S21']/price/text()`},
			},
			"k1",
			"799.00",
		},
		{
			`HTTP/1.1 200 Ok
Content-Type: text/html; charset=utf-8

<products>
  <product>
    <name>iPhone 13</name>
    <price>999.00</price>
    <description>The latest iPhone from Apple.</description>
    <reviews>
      <review>
        <rating>4.5</rating>
        <comment>Great phone, but a bit expensive.</comment>
      </review>
      <review>
        <rating>3.0</rating>
        <comment>Not impressed, I expected more.</comment>
      </review>
    </reviews>
  </product>
  <product>
    <name>Samsung Galaxy S21</name>
    <price>799.00</price>
    <description>The latest Galaxy phone from Samsung.</description>
    <reviews>
      <review>
        <rating>5.0</rating>
        <comment>Amazing phone, great value for money.</comment>
      </review>
      <review>
        <rating>4.0</rating>
        <comment>Good phone, but battery life could be better.</comment>
      </review>
    </reviews>
  </product>
</products>
`,
			&YakExtractor{
				Name:   "cc",
				Type:   "xpath",
				Scope:  "body",
				Groups: []string{`/products/product[name='Samsung Galaxy S21']/price/text()`},
			},
			"cc",
			"799.00",
		},
		{ // 使用nuclei-dsl提取并生成数据
			`HTTP/1.1 200 Ok
Content-Type: text/html; charset=utf-8

<products>
  <product>
    <name>iPhone 13</name>
    <price>999.00</price>
    <description>The latest iPhone from Apple.</description>
    <reviews>
      <review>
        <rating>4.5</rating>
        <comment>Great phone, but a bit expensive.</comment>
      </review>
      <review>
        <rating>3.0</rating>
        <comment>Not impressed, I expected more.</comment>
      </review>
    </reviews>
  </product>
  <product>
    <name>Samsung Galaxy S21</name>
    <price>799.00</price>
    <description>The latest Galaxy phone from Samsung.</description>
    <reviews>
      <review>
        <rating>5.0</rating>
        <comment>Amazing phone, great value for money.</comment>
      </review>
      <review>
        <rating>4.0</rating>
        <comment>Good phone, but battery life could be better.</comment>
      </review>
    </reviews>
  </product>
</products>
`,
			&YakExtractor{
				Name:   "cc",
				Type:   "nuclei-dsl",
				Scope:  "body",
				Groups: []string{`dump(body); contains(body, "rating>4.0") ? "abc": "def"`},
			},
			"cc",
			"abc",
		},
	} {
		data, extractor, name, value := extractor[0].(string), extractor[1].(*YakExtractor), extractor[2].(string), extractor[3].(string)
		results, err := extractor.Execute([]byte(data))
		if err != nil {
			log.Infof("INDEX: %v failed: %v", index, err)
			panic(err)
		}
		if v, ok := results[name]; ok {
			resStr := ExtractResultToString(v)
			if resStr != value {
				panic(utils2.Errorf("INDEX: %v failed, expect: %v, got: %v", index, value, resStr))
			}
		} else {
			panic(spew.Sprintf("INDEX: %v failed,not found key: %v", index, name))
		}
		spew.Dump(results)
	}
}

func TestExtractKValFromResponse(t *testing.T) {
	tests := []struct {
		response string
		key      string
		expected string
	}{
		{
			response: `HTTP/1.1 200 Ok
Content-Type: text/html; charset=utf-8
`,
			key:      "charset",
			expected: "utf-8",
		},
		{
			response: `HTTP/1.1 200 Ok
Content-Type: text/html; charset=utf-8
Cookie: JSE=1111; CCC=11112
`,
			key:      "JSE",
			expected: "1111",
		},
		{
			response: `HTTP/1.1 200 Ok
Content-Type: text/html; charset=utf-8
Cookie: JSE=%251; CCC=11112
`,
			key:      "JSE",
			expected: "%1",
		},
		{
			response: `HTTP/1.1 200 Ok
Content-Type: text/html; charset=utf-8
Cookie: JSE=1111; CCC=A12
`,
			key:      "CCC",
			expected: "A12",
		},
		{
			response: `HTTP/1.1 200 Ok
Content-Type: text/html; charset=utf-8
Cookie: JSE=1111; CCC=A12

{
   "store": {
       "book": [
           {
               "category": "reference",
               "author": "Nigel Rees",
               "title": "Sayings of the Century",
               "price": 8.95
           },
           {
               "category": "fiction",
               "author": "Evelyn Waugh",
               "title": "Sword of Honour",
               "price": 12.99
           },
           {
               "category": "fiction",
               "author": "Herman Melville",
               "title": "Moby Dick",
               "isbn": "0-553-21311-3",
               "price": 8.99
           },
           {
               "category": "fiction",
               "author": "J. R. R. Tolkien",
               "title": "The Lord of the Rings",
               "isbn": "0-395-19395-8",
               "price": 22.99
           }
       ],
       "bicycle": {
           "color": "red",
           "price": 19.95
       }
   },
   "expensive": 10,
	"cc1": 111
}
`,
			key:      "cc1",
			expected: "111",
		},
		{
			response: `HTTP/1.1 200 Ok
Content-Type: text/html; charset=utf-8
Cookie: JSE=1111; CCC=A12

{
   "store": {
       "book": [
           {
               "category": "reference",
               "author": "Nigel Rees",
               "title": "Sayings of the Century",
               "price": 8.95
           },
           {
               "category": "fiction",
               "author": "Evelyn Waugh",
               "title": "Sword of Honour",
               "price": 12.99
           },
           {
               "category": "fiction",
               "author": "Herman Melville",
               "title": "Moby Dick",
               "isbn": "0-553-21311-3",
               "price": 8.99
           },
           {
               "category": "fiction",
               "author": "J. R. R. Tolkien",
               "title": "The Lord of the Rings",
               "isbn": "0-395-19395-8",
               "price": 22.99
           }
       ],
       "bicycle": {
           "color": "red",
           "price": 19.95
       }
   },
   "expensive": 10,
	"cc1": 111
}
`,
			key:      "expensive",
			expected: "10",
		},
		{
			response: `HTTP/1.1 200 Ok
Content-Type: text/html; charset=utf-8
Cookie: JSE=1111; CCC=A12

asdfjkasdjklfasjdf
expensive=10
as
12
312
31
23


`,
			key:      "expensive",
			expected: "10",
		},
		{
			response: `HTTP/1.1 200 Ok
Content-Type: text/html; charset=utf-8
Cookie: JSE=1111; CCC=A12

asdfjkasdjklfasjdf
expensive=10
"abcc": 10
as
12
312
31
23


`,
			key:      "abcc",
			expected: "10",
		},
		{
			response: `HTTP/1.1 200 Ok

{"json":"1%201"}
`,
			key:      "json",
			expected: "1%201",
		},
		{
			response: `HTTP/1.1 200 Ok

{"json":{"json_1":"1%201"}}
`,
			key:      "json_1",
			expected: "1%201",
		},
	}

	for i, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			results := ExtractKValFromResponse([]byte(tt.response))
			if ExtractResultToString(results[tt.key]) != tt.expected {
				log.Printf("INDEX: %v failed: %v", i, spew.Sdump(results))
				t.FailNow()
			}
		})
	}
}

// TestYakExtractor_REGEXP_LookbehindLookahead 测试 lookbehind/lookahead 断言正则的匹配效果
func TestYakExtractor_REGEXP_LookbehindLookahead(t *testing.T) {
	tests := []struct {
		name     string
		data     string
		regex    string
		key      string
		expected string
	}{
		{
			name:     "lookbehind lookahead 提取 JSON text 字段值（带空格）",
			data:     `HTTP/1.1 200 OK\r\nContent-Type: application/json\r\n\r\n{"text": "ZZXS", "id": 1}`,
			regex:    `(?<="text": ")[^"]+(?=")`,
			key:      "data",
			expected: "ZZXS",
		},
		{
			name:     "lookbehind lookahead 提取 JSON text 字段值（无空格）",
			data:     `HTTP/1.1 200 OK\r\nContent-Type: application/json\r\n\r\n{"text":"value123","id":1}`,
			regex:    `(?<="text":")[^"]+(?=")`,
			key:      "data",
			expected: "value123",
		},
		{
			name:     "lookbehind lookahead 多个匹配",
			data:     `HTTP/1.1 200 OK\r\n\r\n{"a": "first", "b": "second", "a": "third"}`,
			regex:    `(?<="a": ")[^"]+(?=")`,
			key:      "data",
			expected: "first,third",
		},
		{
			name: "Werkzeug 实际响应 - 带空格的 JSON 格式",
			data: `HTTP/1.1 200 OK
Content-Type: application/json
Date: Thu, 12 Mar 2026 06:23:46 GMT

{
  "img": "data:image/png;base64,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",
  "ok": true,
  "text": "ZZXS"
}`,
			regex:    `(?<="text": ")[^"]+(?=")`,
			key:      "data",
			expected: "ZZXS",
		},
		{
			name: "Werkzeug 实际响应 - 用户正则无空格版（不匹配带空格的 JSON，提取为空）",
			data: `HTTP/1.1 200 OK
Content-Type: application/json

{"text": "ZZXS"}`,
			regex:    `(?<="text":")[^"]+(?=")`,
			key:      "data",
			expected: "", // 正则要求 "text":" 无空格，但实际 JSON 是 "text": " 有空格，故不匹配
		},
		{
			name: "Werkzeug 实际响应 - 兼容带/不带空格的正则",
			data: `HTTP/1.1 200 OK
Content-Type: application/json

{"text": "ZZXS"}`,
			regex:    `(?<="text":\s*")[^"]+(?=")`,
			key:      "data",
			expected: "ZZXS",
		},
		{
			name:     "用户场景 - POST 请求 body 中提取 key 的值",
			data:     "POST / HTTP/1.1\r\nContent-Type: application/json\r\nHost: www.example.com\r\n\r\n{\"key\": \"value\"}",
			regex:    `(?<="key": ")[^"]+(?=")`,
			key:      "data",
			expected: "value",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			extractor := &YakExtractor{
				Name:   "data",
				Type:   "regex",
				Scope:  "all",
				Groups: []string{tt.regex},
			}
			results, err := extractor.Execute([]byte(tt.data))
			if err != nil {
				t.Fatalf("Execute failed: %v", err)
			}
			got := ExtractResultToString(results[tt.key])
			if got != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, got)
			}
		})
	}
}

// TestYakExtractor_REGEXP_Scope_RequestBody 验证 request_body Scope：从 POST 请求体提取时需用此 Scope
func TestYakExtractor_REGEXP_Scope_RequestBody(t *testing.T) {
	req := []byte("POST / HTTP/1.1\r\nContent-Type: application/json\r\nHost: www.example.com\r\n\r\n{\"key\": \"value\"}")
	rsp := []byte("HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\n\r\nok") // 响应不含 JSON

	extractor := &YakExtractor{
		Name:   "data",
		Type:   "regex",
		Scope:  "request_body",
		Groups: []string{`(?<="key": ")[^"]+(?=")`},
	}
	results, err := extractor.ExecuteWithRequest(rsp, req, false)
	if err != nil {
		t.Fatalf("ExecuteWithRequest failed: %v", err)
	}
	got := ExtractResultToString(results["data"])
	if got != "value" {
		t.Errorf("request_body scope: expected %q, got %q", "value", got)
	}

	// 对比：Scope=body 时只查响应，应提取不到
	extractorBody := &YakExtractor{
		Name:   "data",
		Type:   "regex",
		Scope:  "body",
		Groups: []string{`(?<="key": ")[^"]+(?=")`},
	}
	resultsBody, _ := extractorBody.ExecuteWithRequest(rsp, req, false)
	if ExtractResultToString(resultsBody["data"]) != "" {
		t.Errorf("body scope should not match request JSON, got %q", resultsBody["data"])
	}
}

// lack testcase for kval and xpath attribute
func TestYakExtractor_REGEXP(t *testing.T) {
	for index, extractor := range [][]any{
		{
			`HTTP/1.1 200 Ok
Content-Type: text/html; charset=utf-8

<!DOCTYPE html>
<html></html>`,
			&YakExtractor{
				Name:   "k1",
				Type:   "regex",
				Groups: []string{`DOCTYPE \w{4}`},
			},
			"k1",
			"DOCTYPE html",
		},
	} {
		data, extractor, key, value := extractor[0].(string), extractor[1].(*YakExtractor), extractor[2].(string), extractor[3].(string)
		vars, err := extractor.Execute([]byte(data))
		if err != nil {
			log.Infof("INDEX: %v failed: %v", index, err)
			panic(err)
		}
		ret, _ := vars[key]
		if ExtractResultToString(ret) != value {
			log.Infof("INDEX: %v failed: %v", index, spew.Sdump(vars))
			panic("failed")
		}
		spew.Dump(vars)
	}
}

func TestYakExtractor_XPATH_ATTR(t *testing.T) {
	for index, extractor := range [][]any{
		{
			`HTTP/1.1 200 Ok
Content-Type: text/html; charset=utf-8

<html>
	<head><title value="999">ABC</title></head>
</html>`,
			&YakExtractor{
				Name:           "k1",
				Type:           "xpath",
				Scope:          "body",
				XPathAttribute: "value",
				Groups:         []string{`//title`},
			},
			"k1",
			"999",
		},
		{
			`HTTP/1.1 200 Ok
Content-Type: text/html; charset=utf-8

<html>
	<head><title value="999">ABC</title></head>
</html>`,
			&YakExtractor{
				Type:           "xpath",
				Scope:          "body",
				XPathAttribute: "value",
				Groups:         []string{`//title`},
			},
			"data",
			"999",
		},
	} {
		data, extractor, key, value := extractor[0].(string), extractor[1].(*YakExtractor), extractor[2].(string), extractor[3].(string)
		vars, err := extractor.Execute([]byte(data))
		if err != nil {
			log.Infof("INDEX: %v failed: %v", index, err)
			panic(err)
		}
		ret, _ := vars[key]
		if ExtractResultToString(ret) != value {
			log.Infof("INDEX: %v failed,expect: %v,get: %v", index, spew.Sdump(map[string]string{key: value}), spew.Sdump(vars))
			panic("failed")
		}
		spew.Dump(vars)
	}
}

func TestYakExtractor_KVAL(t *testing.T) {
	for index, extractor := range [][]any{
		{
			`HTTP/1.1 200 OK
Date: Mon, 23 May 2005 22:38:34 GMT
Content-Type: text/html; charset=UTF-8
Content-Encoding: UTF-8

<html><!doctype html>
<html>
<body>
 <div id="result">%d</div>
</body>
</html></html>`,
			&YakExtractor{
				Name:   "k1",
				Type:   "kv",
				Groups: []string{`id`},
			},
			"k1",
			"result",
		},
	} {
		data, extractor, key, value := extractor[0].(string), extractor[1].(*YakExtractor), extractor[2].(string), extractor[3].(string)
		vars, err := extractor.Execute([]byte(data))
		if err != nil {
			log.Infof("INDEX: %v failed: %v", index, err)
			panic(err)
		}
		ret, _ := vars[key]
		if ExtractResultToString(ret) != value {
			log.Infof("INDEX: %v failed: %v", index, spew.Sdump(vars))
			panic("failed")
		}
		spew.Dump(vars)
	}
}
