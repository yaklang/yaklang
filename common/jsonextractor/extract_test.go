package jsonextractor

import (
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/assert"
)

func TestFixJson(t *testing.T) {
	var a = FixJson([]byte(`\x12\x32`))
	spew.Dump(a)
	if string(a) != `\u0012\u0032` {
		panic(1)
	}
	a = FixJson([]byte(`{"badi": "\r\x11"} `))
	if string(a) != `{"badi": "\r\u0011"} ` {
		panic(2)
	}
}

func TestNewStream(t *testing.T) {
	raw := `<html>

aasdfasd
df
{
  "code" : "0",
  "message" : "success",
  "responseTime" : 2,
  "traceId" : "a469b12c7d7aaca5",
  "returnCode" : null,
  "result" : {
    "total" : 0,
    "navigatePages" : 8,
    "navigatepageNums" : [ ],
    "navigateFirstPage" : 0,
    "navigateLastPage" : 0
  }
}

</html>
{{
{"abc": 123}

{{{{{{   }} {"test":                     123}}
`
	results, rawStr := ExtractJSONWithRaw(raw)

	spew.Dump(results)
	spew.Dump(rawStr)

	if results[1] != `{"abc": 123}` {
		panic(1)
	}
	if results[2] != `{   }` {
		panic(1)
	}
	if results[3] != `{"test":                     123}` {
		panic(1)
	}
	if rawStr[0] != `{{{   }} {"test":                     123}}` {
		panic(2)
	}
	if rawStr[1] != `{{   }}` {
		panic(2)
	}

}

func TestExtractJSONWithRaw(t *testing.T) {
	raw, ok := JsonValidObject([]byte(`{"abc": 123,}`))
	if !ok {
		panic("abc")
	}
	if string(raw) != "{\"abc\": 123}" {
		panic("abc")
	}
	println(string(raw))
}
func TestExtractJsonWithQuote(t *testing.T) {
	res := ExtractStandardJSON("`" + `{"a":1}`)
	assert.Equal(t, 1, len(res))
	assert.Equal(t, "{\"a\":1}", res[0])
}

func TestExtractJsonStringQuote(t *testing.T) {
	res := ExtractStandardJSON("`" + `{"a":"c:\\"}`)
	assert.Equal(t, 1, len(res))
	assert.Equal(t, "{\"a\":\"c:\\\\\"}", res[0])
}

func TestExtractJsonArray(t *testing.T) {
	// 测试简单的JSON数组
	res := ExtractStandardJSON(`[{"key": "value"}]`)
	assert.Equal(t, 1, len(res))
	assert.Equal(t, `[{"key": "value"}]`, res[0])

	// 测试包含多个对象的数组
	res = ExtractStandardJSON(`[{"key1": "value1"}, {"key2": "value2"}]`)
	assert.Equal(t, 1, len(res))
	assert.Equal(t, `[{"key1": "value1"}, {"key2": "value2"}]`, res[0])

	// 测试混合文本中的JSON数组
	res = ExtractStandardJSON(`some text [{"key": "value"}] more text`)
	assert.Equal(t, 1, len(res))
	assert.Equal(t, `[{"key": "value"}]`, res[0])

	// 测试对象包含数组
	res = ExtractStandardJSON(`{"array": [{"key": "value"}]}`)
	assert.Equal(t, 1, len(res))
	assert.Equal(t, `{"array": [{"key": "value"}]}`, res[0])

	// 测试数组和对象混合
	res = ExtractStandardJSON(`text [{"key": "value"}] and {"another": "object"}`)
	assert.Equal(t, 2, len(res))
	assert.Equal(t, `[{"key": "value"}]`, res[0])
	assert.Equal(t, `{"another": "object"}`, res[1])
}

func TestExtractObjectsOnly(t *testing.T) {
	// 测试单个对象
	res := ExtractObjectsOnly(`{"key": "value"}`)
	assert.Equal(t, 1, len(res))
	assert.Equal(t, `{"key": "value"}`, res[0])

	// 测试数组中包含对象
	res = ExtractObjectsOnly(`[{"key": "value"}]`)
	assert.Equal(t, 1, len(res))
	assert.Equal(t, `{"key": "value"}`, res[0])

	// 测试数组中包含多个对象
	res = ExtractObjectsOnly(`[{"key1": "value1"}, {"key2": "value2"}]`)
	assert.Equal(t, 2, len(res))
	assert.Equal(t, `{"key1": "value1"}`, res[0])
	assert.Equal(t, `{"key2": "value2"}`, res[1])

	// 测试数组中包含对象和其他类型（应该只返回对象）
	res = ExtractObjectsOnly(`[{"key": "value"}, "string", 123, true]`)
	assert.Equal(t, 1, len(res))
	assert.Equal(t, `{"key": "value"}`, res[0])

	// 测试混合文本中的对象和数组
	res = ExtractObjectsOnly(`text {"name": "Alice"} more [{"age": 25}, "ignore", {"city": "NYC"}]`)
	assert.Equal(t, 3, len(res))
	assert.Equal(t, `{"name": "Alice"}`, res[0])
	assert.Equal(t, `{"age": 25}`, res[1])
	assert.Equal(t, `{"city": "NYC"}`, res[2])

	// 测试空数组
	res = ExtractObjectsOnly(`[]`)
	assert.Equal(t, 0, len(res))

	// 测试只包含非对象的数组
	res = ExtractObjectsOnly(`["string", 123, true, null]`)
	assert.Equal(t, 0, len(res))

	// 测试嵌套对象
	res = ExtractObjectsOnly(`{"outer": {"inner": "value"}}`)
	assert.Equal(t, 1, len(res))
	assert.Equal(t, `{"outer": {"inner": "value"}}`, res[0])
}
