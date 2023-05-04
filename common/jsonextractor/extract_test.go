package jsonextractor

import (
	"github.com/davecgh/go-spew/spew"
	"testing"
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
