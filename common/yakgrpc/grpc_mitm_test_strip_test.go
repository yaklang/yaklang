package yakgrpc

import (
	"bytes"
	"github.com/davecgh/go-spew/spew"
	"testing"
	"yaklang.io/yaklang/common/yakgrpc/ypb"
)

func TestMITMTestStripTest(t *testing.T) {
	_, raw := StripHTTPRequestGzip(nil, []byte(`POST / HTTP/1.1
Host: www.example.com
User-Agent: Go-http-client/1.1
Content-Length: 16
Content-Type: application/json
Test: abcasdfasdfasdf
Accept-Encoding: gzip

{"key": "value"}`))
	println(string(raw))
}

func TestReplaceString(t *testing.T) {
	replacer := NewMITMReplacer()
	replacer.SetRules(&ypb.MITMContentReplacer{
		Rule:             `Cookie: \w+\=\w+`,
		NoReplace:        false,
		Result:           "111111111111111",
		Color:            "",
		EnableForRequest: true,
		EnableForHeader:  true,
		Index:            0,
		ExtraTag:         nil,
		Disabled:         false,
		VerboseName:      "",
	})
	matched, data, _ := replacer.hook(true, false, []byte(`GET / HTTP/1.1
Host: www.baidu.com
Cookie: abc=123

`))
	spew.Dump(matched)
	spew.Dump(data)
	if !bytes.Contains(data, []byte(`111111111111`)) {
		panic("rule failed")
	}
}

func TestReplaceString2(t *testing.T) {
	replacer := NewMITMReplacer()
	replacer.SetRules(&ypb.MITMContentReplacer{
		Rule:             `Cookie: BAIDUID_BFESS=[^\n]*`,
		NoReplace:        false,
		Result:           "111111111111111",
		Color:            "",
		EnableForRequest: true,
		EnableForHeader:  true,
		Index:            0,
		ExtraTag:         nil,
		Disabled:         false,
		VerboseName:      "",
	})
	matched, data, _ := replacer.hook(true, false, []byte(`GET /content-search.xml HTTP/1.1
Host: www.baidu.com
Accept-Encoding: gzip, deflate, br
Accept-Language: zh-CN,zh;q=0.9
Cookie: BAIDUID_BFESS=D541A87Daaa50ACC658F7405F62B195D8AA:FG=1; ZFY=Xx1VJGFY2aaHQ2vrOIEsC83loAk0wEEIPY3nVfBgtxymQ:C
Sec-Fetch-Dest: empty
Sec-Fetch-Mode: no-cors
Sec-Fetch-Site: same-origin
User-Agent: Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/111.0.0.0 Safari/537.36



`))
	spew.Dump(matched)
	spew.Dump(data)
	if !bytes.Contains(data, []byte(`111111111111`)) {
		panic("rule failed")
	}
}

func TestReplaceString23(t *testing.T) {
	replacer := NewMITMReplacer()
	replacer.SetRules(&ypb.MITMContentReplacer{
		Rule:             `\{"product": ".+?"`,
		NoReplace:        false,
		Result:           `{"product": ["abc", "23123"]`,
		Color:            "",
		EnableForRequest: true,
		EnableForHeader:  true,
		EnableForBody:    true,
		Index:            0,
		ExtraTag:         nil,
		Disabled:         false,
		VerboseName:      "",
	})
	matched, data, _ := replacer.hook(true, false, []byte(`GET /content-search.xml HTTP/1.1
Host: www.baidu.com
Accept-Encoding: gzip, deflate, br
Accept-Language: zh-CN,zh;q=0.9
Cookie: BAIDUID_BFESS=D541A87Daaa50ACC658F7405F62B195D8AA:FG=1; ZFY=Xx1VJGFY2aaHQ2vrOIEsC83loAk0wEEIPY3nVfBgtxymQ:C
Sec-Fetch-Dest: empty
Sec-Fetch-Mode: no-cors
Sec-Fetch-Site: same-origin
User-Agent: Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/111.0.0.0 Safari/537.36

{"product": "asdfasdfasdfasd"}

`))
	spew.Dump(matched)
	spew.Dump(data)
	if !bytes.Contains(data, []byte(`23123`)) {
		panic("rule failed")
	}
}

func TestReplaceString233(t *testing.T) {
	replacer := NewMITMReplacer()
	replacer.SetRules(&ypb.MITMContentReplacer{
		Rule:             `百度`,
		NoReplace:        false,
		Result:           `谷歌`,
		Color:            "",
		EnableForRequest: true,
		EnableForHeader:  true,
		EnableForBody:    true,
		Index:            0,
		ExtraTag:         nil,
		Disabled:         false,
		VerboseName:      "",
	})
	matched, data, _ := replacer.hook(true, false, []byte(`GET /content-search.xml HTTP/1.1
Host: www.baidu.com
Accept-Encoding: gzip, deflate, br
Accept-Language: zh-CN,zh;q=0.9
Cookie: BAIDUID_BFESS=D541A87Daaa50ACC658F7405F62B195D8AA:FG=1; ZFY=Xx1VJGFY2aaHQ2vrOIEsC83loAk0wEEIPY3nVfBgtxymQ:C
Sec-Fetch-Dest: empty
Sec-Fetch-Mode: no-cors
Sec-Fetch-Site: same-origin
User-Agent: Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/111.0.0.0 Safari/537.36

{"product": "百度"}

`))
	spew.Dump(matched)
	spew.Dump(data)
	println(string(data))
	if !bytes.Contains(data, []byte(`谷歌`)) {
		panic("rule failed")
	}
}
