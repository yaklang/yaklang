package lowhttp

import (
	"github.com/stretchr/testify/assert"
	"strconv"
	"testing"
	"time"
)

func TestParseStringToHttpRequest2(t *testing.T) {
	req, err := ParseStringToHttpRequest(`
GET / HTTP/1.1
Host: www.baidu.com

teadfasdfasd
`)
	if err != nil {
		t.FailNow()
		return
	}
	_ = req
}

func TestSplitHTTPHeader(t *testing.T) {
	key, value := SplitHTTPHeader("abc")
	if !(key == "abc" && value == "") {
		panic("111")
	}

	key, value = SplitHTTPHeader("abc:111")
	if !(key == "abc" && value == "111") {
		panic("111")
	}

	key, value = SplitHTTPHeader("abc: 111")
	if !(key == "abc" && value == "111") {
		panic("111")
	}

	key, value = SplitHTTPHeader("abc: 111\r\n")
	if !(key == "abc" && value == "111") {
		panic("111")
	}

	key, value = SplitHTTPHeader("Abc: 111\r\n")
	if !(key == "Abc" && value == "111") {
		panic("111")
	}

	key, value = SplitHTTPHeader("Abc: 1::11\r\n")
	if !(key == "Abc" && value == "1::11") {
		panic("111")
	}
}

func TestParseStringToHttpRequest(t *testing.T) {
	test := assert.New(t)

	req, err := ParseStringToHttpRequest(`
GET / HTTP/1.1
Host: www.baidu.com
Connection: close
User-Agent: Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_4) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/84.0.4147.135 Safari/537.36

`)
	if err != nil {
		test.FailNow(err.Error())
	}

	u, err := ExtractURLFromHTTPRequest(req, true)
	if err != nil {
		test.FailNow(err.Error())
		return
	}
	_ = u

	raw, err := SendHTTPRequestRaw(true, "www.baidu.com", 443, req, 10*time.Second)
	if err != nil {
		test.FailNow(err.Error())
		return
	}
	_ = raw
	//log.Infof("\n\n%v", string(raw))
}

func TestConvertContentToChunked(t *testing.T) {
	raws := fixInvalidHTTPHeaders([]byte(`
GET / HTTP/1.1
Host: www.baidu.com
Content-Length: 12

123123123123
`))
	println(string(raws))
}

func TestSendHTTPRequestWithRawPacket(t *testing.T) {

	rsp, _ := SendHTTPRequestWithRawPacket(false, "www.baidu.com", 80, []byte(`GET / HTTP/1.1
Host: www.baidu.com

`), 5*time.Second)
	println(string(rsp))
}

func TestGetRedirectFromHTTPResponse(t *testing.T) {
	target := GetRedirectFromHTTPResponse([]byte(`HTTP/1.1 300 ...
Location: /target`), false)
	println(target)
	if target != "/target" {
		t.FailNow()
		return
	}
}

func TestRemoveZeroContentLengthHTTPHeader(t *testing.T) {
	target := RemoveZeroContentLengthHTTPHeader([]byte(`GET / HTTP/1.1
Host: www.baidu.com
Content-Length: 0

`))
	println(string(target))
	println(strconv.Quote(string(target)))
}

func TestFixHTTPResponse(t *testing.T) {
	rap, _, err := FixHTTPResponse([]byte(`HTTP/1.1 200 OK
Connection: close
Bdpagetype: 3
Bdqid: 0x9efbfb790011d570
Cache-Control: private
Ckpacknum: 2
Ckrndstr: 90011d570
Content-Encoding: gzip
Content-Type: text/html;charset=utf-8
Date: Sat, 27 Nov 2021 04:20:29 GMT
P3p: CP=" OTI DSP COR IVA OUR IND COM "
Server: BWS/1.1
Set-Cookie: BDRCVFR[S4-dAuiWMmn]=I67x6TjHwwYf0; path=/; domain=.baidu.com
Set-Cookie: delPer=0; path=/; domain=.baidu.com
Set-Cookie: BD_CK_SAM=1;path=/
Set-Cookie: PSINO=2; domain=.baidu.com; path=/
Set-Cookie: BDSVRTM=12; path=/
Set-Cookie: H_PS_PSSID=34445_35104_35239_34584_34517_35245_34606_35320_26350_35209_35312_35145; path=/; domain=.baidu.com
Strict-Transport-Security: max-age=172800
Traceid: 1637986829039139149811456026574257771888
Vary: Accept-Encoding
X-Frame-Options: sameorigin
X-Ua-Compatible: IE=Edge,chrome=1
Content-Length: 12

aaaaaaaaaaaa` + "\r\n\r\n"))
	if err != nil {
		return
	}
	println(string(rap))
}

// https://www.baidu.com/s?cl=3&tn=baidutop10&fr=top1000&wd=%E9%92%9F%E5%8D%97%E5%B1%B1%E8%B0%88%E5%8D%97%E9%9D%9E%E5%8F%91%E7%8E%B0%E7%9A%84%E6%96%B0%E5%8F%98%E7%A7%8D%E7%97%85%E6%AF%92&rsv_idx=2&rsv_dl=fyb_n_homepage&hisfilter=1
func TestSendHTTPRequestWithRawPacket3(t *testing.T) {
	//	test := assert.New(t)
	//
	//	// EXP 数据包
	//	packet := `
	//GET /beidou-sdk/browser/bundle.min_v20211124165842.js?id=asdfasdfasdf HTTP/1.1
	//Host: {{json(TARGET)}}
	//User-Agent: Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:94.0) Gecko/20100101 Firefox/94.0
	//
	//`
	//
	//	// 填充参数
	//	packets, err := mutate.QuickMutate(packet, nil, mutate.MutateWithExtraParams(map[string][]string{
	//		"TARGET": {"www.baidu.com"},
	//	}))
	//	if err != nil {
	//		return
	//	}
	//	req, err := SendHTTPRequestWithRawPacket(true, "", 443, []byte(packets[0]), 5*time.Second)
	//	if err != nil {
	//		test.FailNow(err.Error())
	//	}
	//
	//	println(string(req))
}
