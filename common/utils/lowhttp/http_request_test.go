package lowhttp

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/utils/multipart"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"

	"github.com/davecgh/go-spew/spew"
)

const _multipartDemo = `
POST / HTTP/1.1
Host: localhost:8000
User-Agent: Mozilla/5.0 (X11; Ubuntu; Linux i686; rv:29.0) Gecko/20100101 Firefox/29.0
Accept: text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8
Accept-Language: en-US,en;q=0.5
Accept-Encoding: gzip, deflate
Cookie: __atuvc=34%7C7; permanent=0; _gitlab_session=226ad8a0be43681acf38c2fab9497240; __profilin=p%3Dt; request_method=GET
Connection: keep-alive
Content-Type: multipart/form-data; boundary=---------------------------9051914041544843365972754266
Content-Length: 554

-----------------------------9051914041544843365972754266
Content-Disposition: form-data; name="text"

text default
-----------------------------9051914041544843365972754266
Content-Disposition: form-data; name="text2"

text defaultads
-----------------------------9051914041544843365972754266
Content-Disposition: form-data; name="text"

text defaultadsfasdf
-----------------------------9051914041544843365972754266
Content-Disposition: form-data; name="file1"; filename="a.txt"
Content-Type: text/plain

Content of a.txt.

-----------------------------9051914041544843365972754266
Content-Disposition: form-data; name="file2"; filename="a.html"
Content-Type: text/html

<!DOCTYPE html><title>Content of a.html.</title>

-----------------------------9051914041544843365972754266--
`

func TestFixHTTPPacketCRLFKeepOtherParams(t *testing.T) {
	postPacket := `POST / HTTP/1.1
Content-Type: multipart/form-data; charset=utf-16le; boundary=317f9668d26388b5cf93fb07be5ea390b366616e303d2b73c6ee4c53a8b5
Host: www.example.com
Content-Length: 178

--317f9668d26388b5cf93fb07be5ea390b366616e303d2b73c6ee4c53a8b5
Content-Disposition: form-data; name="key"

123
--317f9668d26388b5cf93fb07be5ea390b366616e303d2b73c6ee4c53a8b5--
`
	raw := FixHTTPPacketCRLF([]byte(postPacket), false)
	spew.Dump(raw)
	for _, line := range strings.Split(string(raw), "\r\n") {
		if !strings.HasPrefix(line, "Content-Type") {
			continue
		}
		require.Contains(t, line, "multipart/form-data", "mime-type should be kept")
		require.Contains(t, line, "charset=utf-16le", "charset should be kept")
		require.Contains(t, line, "boundary=317f9668d26388b5cf93fb07be5ea390b366616e303d2b73c6ee4c53a8b5", "boundary should be kept")
	}
}

func TestFixHTTPPacketCRLF6(t *testing.T) {
	postPacket := `POST / HTTP/1.1
Host: localhost:8000
User-Agent: Mozilla/5.0 (X11; Ubuntu; Linux i686; rv:29.0) Gecko/20100101 Firefox/29.0



`
	println(strconv.Quote(postPacket))
	header, body := SplitHTTPPacketFast(postPacket)
	if string(body) != "" {
		t.Fatal("body should be empty")
	}

	spew.Dump(header, body)
	raw := FixHTTPPacketCRLF([]byte(postPacket), false)
	spew.Dump(raw)
	if !bytes.HasSuffix(raw, []byte("Firefox/29.0\r\nContent-Length: 0\r\n\r\n")) {
		t.FailNow()
	}
}

func TestFixHTTPPacketCRLF(t *testing.T) {
	var raw []byte
	raw = FixHTTPPacketCRLF([]byte(`GET / HTTP/1.1
Host: www.baidu.com

Test
`), false)
	if !utils.MatchAllOfSubString(strconv.Quote(string(raw)), `\r\n\r\n`, `Content-Length: 5`, "Test") {
		panic("ERROR for FIX CRLF")
	}

	raw = FixHTTPPacketCRLF([]byte(`GET / HTTP/1.1
Host: www.baidu.com
Transfer-Encoding: chunked

Test`), false)
	if utils.MatchAllOfSubString(strconv.Quote(string(raw)), `Content-Length:`) && !utils.MatchAllOfGlob(strconv.Quote(string(raw)), `*0\r\n"`) {
		panic("CHUNKED WITH CL ERROR!")
	}
	// trigger
	println(string(raw))

	raw = FixHTTPPacketCRLF([]byte(_multipartDemo), false)
	if !utils.MatchAllOfSubString(strconv.Quote(string(raw)), `9051914041544843365972754266--\r\n"`) {
		println(strconv.Quote(string(raw)))
		panic("MULTIPART ERROR")
	}
	println(string(raw))

	raw = HTTPPacketForceChunked(raw)
	println(strconv.Quote(string(raw)))
	if utils.MatchAnyOfSubString(strconv.Quote(string(raw)), `9051914041544843365972754266--\r\n"`) {
		panic("MULTIPART ERROR 2")
	}
	if !strings.HasSuffix(strconv.Quote(string(raw)), `\r\n0\r\n\r\n"`) {
		println(strconv.Quote(string(raw)))
		panic("MULTIPART ERROR 2-1")
	}

	rawPacket := "0a504f5354202f6d616e616765722f68746d6c2f75706c6f61643f6f72672e6170616368652e636174616c696e612e66696c746572732e435352465f4e4f4e43453d454346414436413744373233363431373139423432353334354230303345323520485454502f312e310a486f73743a20637962657274756e6e656c2e72756e3a383038300a4163636570743a20746578742f68746d6c2c6170706c69636174696f6e2f7868746d6c2b786d6c2c6170706c69636174696f6e2f786d6c3b713d302e392c696d6167652f617669662c696d6167652f776562702c696d6167652f61706e672c2a2f2a3b713d302e382c6170706c69636174696f6e2f7369676e65642d65786368616e67653b763d62333b713d302e390a4163636570742d456e636f64696e673a206465666c6174650a4163636570742d4c616e67756167653a207a682d434e2c7a683b713d302e390a417574686f72697a6174696f6e3a2042617369632064473974593246304f6e527662574e6864413d3d0a43616368652d436f6e74726f6c3a206d61782d6167653d300a436f6e74656e742d4c656e6774683a203836300a436f6f6b69653a204a53455353494f4e49443d45423135373232364444413135393434344230323532314243364641463936330a436f6e74656e742d547970653a206d756c7469706172742f666f726d2d646174613b20626f756e646172793d2d2d2d2d5765624b6974466f726d426f756e6461727930796f5435656667556a5942426334670a557067726164652d496e7365637572652d52657175657374733a20310a557365722d4167656e743a204d6f7a696c6c612f352e30202857696e646f7773204e542031302e303b2057696e36343b2078363429204170706c655765624b69742f3533372e333620284b48544d4c2c206c696b65204765636b6f29204368726f6d652f39392e302e343834342e3531205361666172692f3533372e33360a0a2d2d2d2d2d2d5765624b6974466f726d426f756e6461727930796f5435656667556a5942426334670a436f6e74656e742d446973706f736974696f6e3a20666f726d2d646174613b206e616d653d226465706c6f79576172223b2066696c656e616d653d224f4547436a494e6d2e776172220a436f6e74656e742d547970653a206170706c69636174696f6e2f6f637465742d73747265616d0a0a504b03041400080808003d796a54000000000000000000000000090004004d4554412d494e462fefbfbdefbfbd00000300504b0708000000000200000000000000504b03041400080808003d796a54000000000000000000000000140000004d4554412d494e462f4d414e49464553542e4d46efbfbd4defbfbdefbfbd4c4b2d2eefbfbd0d4b2d2aefbfbdefbfbdcfb35230efbfbd33efbfbdefbfbd722e4a4d2c494defbfbd75efbfbd040aefbfbdefbfbd19efbfbd192968efbfbd172526efbfbdefbfbd2a38efbfbd1715efbfbd1725efbfbd00156befbfbd72efbfbd720100504b070814efbfbd2a104200000042000000504b030414000808080058776a5400000000000000000000000008000000746573742e6a737075efbfbd310befbfbd3010efbfbd77efbfbdefbfbd1003efbfbd6530efbfbdefbfbd56efbfbdefbfbd263aefbfbd43efbfbd67efbfbdefbfbdefbfbdc6ab55efbfbdefbfbd6e42452bd4b724efbfbd7befbfbdefbfbd23efbfbd7eefbfbdc39cefbfbd1e7810efbfbdefbfbdefbfbd73efbfbdefbfbd0b5877efbfbdefbfbd64efbfbdefbfbd5056efbfbd4868efbfbd17d58e0b211eefbfbdefbfbdefbfbd515defbfbdefbfbd27393745492befbfbdefbfbd72efbfbd0defbfbdd9b234efbfbd73efbfbdefbfbdefbfbd15efbfbdefbfbd1b6eefbfbdefbfbdefbfbdefbfbd47efbfbdefbfbd060a44efbfbd5defbfbd0d31efbfbdc88320621f7defbfbdc99d70efbfbd61efbfbd7318efbfbdefbfbd3218efbfbdefbfbd4defbfbdefbfbd54efbfbd2cefbfbd4302efbfbd1616675cefbfbddaaaefbfbdefbfbd1040efbfbdefbfbd48176807efbfbd10efbfbd781034efbfbdefbfbd07efbfbd19efbfbdefbfbd5d7c6d52efbfbd6eefbfbd3edb830cefbfbd49efbfbdefbfbdefbfbd0defbfbd672f504b0708efbfbdefbfbd4eefbfbdefbfbd000000efbfbd010000504b010214001400080808003d796a540000000002000000000000000900040000000000000000000000000000004d4554412d494e462fefbfbdefbfbd0000504b010214001400080808003d796a5414efbfbd2a10420000004200000014000000000000000000000000003d0000004d4554412d494e462f4d414e49464553542e4d46504b0102140014000808080058776a54efbfbdefbfbd4eefbfbdefbfbd000000efbfbd0100000800000000000000000000000000efbfbd000000746573742e6a7370504b05060000000003000300efbfbd000000efbfbd01000000000a2d2d2d2d2d2d5765624b6974466f726d426f756e6461727930796f5435656667556a5942426334672d2d"
	raw, err := codec.DecodeHex(rawPacket)
	if err != nil {
		panic("no packet decoded from hex")
	}
	println(len(string(raw)))
	println(codec.EncodeToHex(string(raw)))
	packet := FixHTTPPacketCRLF(raw, false)
	if strings.Count(string(packet), `PK`) <= 1 {
		panic("FILE FAILED")
	}
}

func TestParseBytesToHttpRequest(t *testing.T) {
	packet := "GET /\x00 HTTP/1.1\r\nHost: www.baidu.com\r\n\r\n"
	req, err := ParseBytesToHttpRequest([]byte(packet))
	if err != nil {
		log.Error(err)
		t.FailNow()
	}
	spew.Dump(req)
	url, err := ExtractURLFromHTTPRequestRaw([]byte(packet), true)
	if err != nil {
		panic(err)
	}
	println(string(url.String()))
}

func wait500ms() {
	time.Sleep(time.Millisecond * 500)
}

func TestParseBytesToHttpRequest2(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
	}))
	wait500ms()
	host, port, _ := utils.ParseStringToHostPort(server.URL)
	rsp, err := HTTPWithoutRedirect(
		WithHttps(false), WithHost(host),
		WithPort(port),
		WithPacketBytes([]byte("GET /\x00 HTTP/1.1\r\nHost: www.baidu.com\r\n\r\n")),
		WithTimeout(5*time.Second),
	)
	if err != nil {
		log.Error(err)
		t.FailNow()
	}
	_ = rsp
	rsp, err = HTTPWithoutRedirect(
		WithHttps(false), WithHost(host),
		WithPort(port), WithPacketBytes([]byte("GET /\x00 HTTP/1.1\r\nHost: www.baidu.com\r\n\r\n")),
		WithTimeout(5*time.Second),
	)
	if err != nil {
		log.Error(err)
		t.FailNow()
	}
	_ = rsp
}

func TestParseBytesToHttpRequestForHTTP2(t *testing.T) {
	packet := "GET / HTTP/2\r\nHost: www.baidu.com\r\n\r\n"
	req, err := ParseBytesToHttpRequest([]byte(packet))
	if err != nil {
		log.Error(err)
		t.FailNow()
	}
	if req.ProtoMajor != 2 && req.ProtoMinor != 0 {
		t.Fatalf("prase request proto version failed, got %d.%d", req.ProtoMajor, req.ProtoMinor)
	}
	raw, err := utils.DumpHTTPRequest(req, true)
	if err != nil {
		log.Error(err)
		t.FailNow()
	}
	if !strings.Contains(string(raw), "HTTP/2.0") {
		t.Fatalf("prase request proto version failed, got raw packet: %s", string(raw))
	}
}

func TestFixHTTPRequestOut(t *testing.T) {
	raw := FixHTTPRequest([]byte(`POST /jmx-console/HtmlAdaptor HTTP/1.1
Host: cybertunnel.run:8080
Accept: text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8
Accept-Encoding: deflate
Accept-Language: zh-CN,zh;q=0.8,zh-TW;q=0.7,zh-HK;q=0.5,en-US;q=0.3,en;q=0.2
Authorization: Basic YWRtaW46YWRtaW4=
Connection: close
Content-Length: 170
Content-Type: application/x-www-form-urlencoded
Cookie: JSESSIONID=BB3A44F36C5C275313FB689AEDB1F088
Upgrade-Insecure-Requests: 1
User-Agent: Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:97.0) Gecko/20100101 Firefox/97.0

action=invokeOp&name=jboss.ain%3Aservice%3DDeploymentFileRepository&methodIndex=5&arg0=b.war&arg1=b&arg2=.jsp&arg3=%3C%25out.println%28%22test%22%29%25%3E&arg4=True`))
	if !strings.Contains(string(raw), `Content-Length: 164`) {
		panic("Content-Length Fix Failed")
	}
}

func TestReadHTTPRequest(t *testing.T) {
	packet := `GET /?unix:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA|http://baidu.com/api/v1/targets HTTP/1.1
Host: 127.0.0.1:3333
User-Agent: Mozilla/5.0 (Windows NT 6.1; Win64; x64; rv:47.0) Gecko/20100101 Firefox/47.0

`
	req, err := ReadHTTPRequestFromBytes([]byte(packet))
	if err != nil {
		panic(err)
	}
	if !strings.Contains(spew.Sdump(req), "|http://baidu.com/api/v1/targets") {
		panic("4K URL FAILED")
	}

	url, err := ExtractURLFromHTTPRequestRaw([]byte(packet), false)
	if err != nil {
		log.Error(err)
		panic("TOO LONG URL FAILED")
	}
	if !strings.HasSuffix(url.String(), "AAAAAAAAAAAAAAA|http://baidu.com/api/v1/targets") {
		panic("TOO LONG URL FAILED: extract url")
	}
}

func TestFixMultipartWired(t *testing.T) {
	packet := `POST /solr/core1/dataimport?command=full-import&verbose=false&clean=false&commit=false&debug=true&core=tika&name=dataimport&dataConfig=%0A%3CdataConfig%3E%0A%3CdataSource%20name%3D%22streamsrc%22%20type%3D%22ContentStreamDataSource%22%20loggerLevel%3D%22TRACE%22%20%2F%3E%0A%0A%20%20%3Cscript%3E%3C%21%5BCDATA%5B%0A%20%20%20%20%20%20%20%20%20%20function%20poc%28row%29%7B%0A%20var%20bufReader%20%3D%20new%20java%2Eio%2EBufferedReader%28new%20java%2Eio%2EInputStreamReader%28java%2Elang%2ERuntime%2EgetRuntime%28%29%2Eexec%28%22whoami%22%29%2EgetInputStream%28%29%29%29%3B%0A%0Avar%20result%20%3D%20%5B%5D%3B%0A%0Awhile%28true%29%20%7B%0Avar%20oneline%20%3D%20bufReader%2EreadLine%28%29%3B%0Aresult%2Epush%28%20oneline%20%29%3B%0Aif%28%21oneline%29%20break%3B%0A%7D%0A%0Arow%2Eput%28%22title%22%2Cresult%2Ejoin%28%22%5Cn%5Cr%22%29%29%3B%0Areturn%20row%3B%0A%0A%7D%0A%0A%5D%5D%3E%3C%2Fscript%3E%0A%0A%3Cdocument%3E%0A%20%20%20%20%3Centity%0A%20%20%20%20%20%20%20%20stream%3D%22true%22%0A%20%20%20%20%20%20%20%20name%3D%22entity1%22%0A%20%20%20%20%20%20%20%20datasource%3D%22streamsrc1%22%0A%20%20%20%20%20%20%20%20processor%3D%22XPathEntityProcessor%22%0A%20%20%20%20%20%20%20%20rootEntity%3D%22true%22%0A%20%20%20%20%20%20%20%20forEach%3D%22%2FRDF%2Fitem%22%0A%20%20%20%20%20%20%20%20transformer%3D%22script%3Apoc%22%3E%0A%20%20%20%20%20%20%20%20%20%20%20%20%20%3Cfield%20column%3D%22title%22%20xpath%3D%22%2FRDF%2Fitem%2Ftitle%22%20%2F%3E%0A%20%20%20%20%3C%2Fentity%3E%0A%3C%2Fdocument%3E%0A%3C%2FdataConfig%3E%0A%20%20%20%20%0A%20%20%20%20%20%20%20%20%20%20%20 HTTP/1.1
Host: 61.53.69.199:8878
Cache-Control: max-age=0
Upgrade-Insecure-Requests: 1
User-Agent: Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/104.0.0.0 Safari/537.36
Accept: text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.9
Accept-Encoding: gzip, deflate
Accept-Language: zh-CN,zh;q=0.9
Cookie: JSESSIONID=67A74C3EA432929A9FBBA57D1265BFA2
Connection: close
content-type: multipart/form-data; boundary=------------------------aceb88c2159f183f
Content-Length: 21




--------------------------aceb88c2159f
Content-Disposition: form-data; name="stream.body"

<?xml version="1.0" encoding="UTF-8"?>
<RDF>
<item/>
</RDF>

--------------------------aceb88c2159f--`
	r := FixHTTPPacketCRLF([]byte(packet), false)
	if !bytes.Contains(r, []byte(`boundary=------------------------aceb88c2159f`)) {
		t.FailNow()
	}
	req, _ := ParseBytesToHttpRequest(r)
	println(string(r))
	println(req.ContentLength)
	mutlipartReader := multipart.NewReader(req.Body)
	for {
		part, err := mutlipartReader.NextPart()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			t.Fatal(err)
		}
		if !strings.Contains(part.GetHeader("Content-Disposition"), "stream.body") {
			t.Fatal("multipart error")
		}
		body := make([]byte, 100)
		_, err = part.Read(body)
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			t.Fatal(err)
		}
		if !strings.Contains(string(body), `<?xml version="1.0" encoding="UTF-8"?>`) {
			t.Fatal("multipart error")
		}

	}
}

func TestFixHTTPPacketCRLF2(t *testing.T) {
	results := FixHTTPRequest([]byte(`POST / HTTP/1.1
Host: www.example.com
Content-Length: 203
Content-Type: multipart/form-data; boundary=------------------------cDkWacGqpxxAkcXkxoWoNItodEKPxryzekgvPhwK

--------------------------123
Content-Disposition: form-data; name="{\"key\": \"value\"}"


--------------------------123--`))
	spew.Dump(results)
	if !strings.Contains(string(results), `boundary=------------------------123`) {
		panic(1)
	}

	if !strings.Contains(string(spew.Sdump(results)), `00000c0  2d 64 61 74 61 3b 20 6e  61 6d 65 3d 22 7b 5c 22  |-data; name="{\"|
 000000d0  6b 65 79 5c 22 3a 20 5c  22 76 61 6c 75 65 5c 22  |key\": \"value\"|
 000000e0  7d 22 0d 0a 0d 0a 0d 0a  2d 2d 2d 2d 2d 2d 2d 2d  |}"......--------|
 000000f0  2d 2d 2d 2d 2d 2d 2d 2d  2d 2d 2d 2d 2d 2d 2d 2d  |----------------|
 00000100  2d 2d 31 32 33 2d 2d 0d  0a                       |--123--..|`) {
		panic("CRLF Fix Failed")
	}
}

func TestFixHTTPPacketCRLF3(t *testing.T) {
	results := FixHTTPRequest([]byte(`POST / HTTP/1.1
Host: www.example.com
Content-Length: 203
Content-Type: multipart/form-data; boundary=------------------------cDkWacGqpxxAkcXkxoWoNItodEKPxryzekgvPhwK

--------------------------123
Content-Disposition: form-data; name="{\"key\": \"value\"}"


--------------------------123--`))
	spew.Dump(results)
	if !strings.Contains(string(results), `boundary=------------------------123`) {
		panic(1)
	}

	if !strings.Contains(string(spew.Sdump(results)), `00000c0  2d 64 61 74 61 3b 20 6e  61 6d 65 3d 22 7b 5c 22  |-data; name="{\"|
 000000d0  6b 65 79 5c 22 3a 20 5c  22 76 61 6c 75 65 5c 22  |key\": \"value\"|
 000000e0  7d 22 0d 0a 0d 0a 0d 0a  2d 2d 2d 2d 2d 2d 2d 2d  |}"......--------|
 000000f0  2d 2d 2d 2d 2d 2d 2d 2d  2d 2d 2d 2d 2d 2d 2d 2d  |----------------|
 00000100  2d 2d 31 32 33 2d 2d 0d  0a                       |--123--..|`) {
		panic("CRLF Fix Failed")
	}
}

func TestFixHTTPPacketCRLF4(t *testing.T) {
	results := FixHTTPRequest([]byte(`POST / HTTP/1.1
Host: www.example.com
Content-Length: 203
Content-Type: 123123

   asdf`))
	spew.Dump(results)
	if !strings.Contains(string(results), "\r\n\r\n   asdf") {
		panic(1)
	}

	if !strings.Contains(string(spew.Sdump(results)), ` 00000000  50 4f 53 54 20 2f 20 48  54 54 50 2f 31 2e 31 0d  |POST / HTTP/1.1.|
 00000010  0a 48 6f 73 74 3a 20 77  77 77 2e 65 78 61 6d 70  |.Host: www.examp|
 00000020  6c 65 2e 63 6f 6d 0d 0a  43 6f 6e 74 65 6e 74 2d  |le.com..Content-|
 00000030  4c 65 6e 67 74 68 3a 20  37 0d 0a 43 6f 6e 74 65  |Length: 7..Conte|
 00000040  6e 74 2d 54 79 70 65 3a  20 31 32 33 31 32 33 0d  |nt-Type: 123123.|
 00000050  0a 0d 0a 20 20 20 61 73  64 66                    |...   asdf|`) {
		panic("CRLF Fix Failed")
	}
}

func TestFixHTTPPacketTrim1(t *testing.T) {
	results := FixHTTPRequest([]byte("\t" + `GET / HTTP/1.1` + CRLF +
		"\t" + `Host: www.baidu.com` + CRLF + CRLF))
	fmt.Println(string(results))
	spew.Dump(results)
	if bytes.Contains(results, []byte("\t")) {
		panic("trim Fix Failed")
	}
	if !strings.Contains(string(spew.Sdump(results)), `00000000  47 45 54 20 2f 20 48 54  54 50 2f 31 2e 31 0d 0a  |GET / HTTP/1.1..|
 00000010  48 6f 73 74 3a 20 77 77  77 2e 62 61 69 64 75 2e  |Host: www.baidu.|
 00000020  63 6f 6d 0d 0a 0d 0a                              |com....|`) {
		panic("trim fix fail")
	}
}

func TestFixHTTPPacketTrim2(t *testing.T) {
	results := FixHTTPRequest([]byte("\t" + "\t" + `GET / HTTP/1.1` + CRLF +
		"\t" + `Host: www.baidu.com` + CRLF + CRLF))
	fmt.Println(string(results))
	spew.Dump(results)
	if bytes.Contains(results, []byte("\t")) {
		panic("trim Fix Failed")
	}
	if !strings.Contains(string(spew.Sdump(results)), `00000000  47 45 54 20 2f 20 48 54  54 50 2f 31 2e 31 0d 0a  |GET / HTTP/1.1..|
 00000010  48 6f 73 74 3a 20 77 77  77 2e 62 61 69 64 75 2e  |Host: www.baidu.|
 00000020  63 6f 6d 0d 0a 0d 0a                              |com....|`) {
		panic("trim fix fail")
	}
}

func TestFixHTTPPacketTrim3(t *testing.T) {
	results := FixHTTPRequest([]byte(`GET / HTTP/1.1` + CRLF +
		"\t" + `Host: www.baidu.com` + CRLF + CRLF))
	fmt.Println(string(results))
	spew.Dump(results)
	if !bytes.Contains(results, []byte("\t")) {
		panic("trim Fix Failed")
	}
	if !strings.Contains(string(spew.Sdump(results)), `00000000  47 45 54 20 2f 20 48 54  54 50 2f 31 2e 31 0d 0a  |GET / HTTP/1.1..|
 00000010  09 48 6f 73 74 3a 20 77  77 77 2e 62 61 69 64 75  |.Host: www.baidu|
 00000020  2e 63 6f 6d 0d 0a 0d 0a                           |.com....|`) {
		panic("trim fix fail")
	}
}

func TestFixHTTPPacketBoundary(t *testing.T) {
	results := FixHTTPRequest([]byte(`POST / HTTP/1.1
Host: www.example.com
Content-Length: 203
Content-Type: multipart/form-data; boundary=------------------------cDkWacGqpxxAkcXkxoWoNItodEKPxryzekgvPhwK

--------------------------123
Content-Disposition: form-data; name="{\"key\": \"value\"}"
--------------------------123
Content-Disposition: form-data; name="{\"key\": \"value\"}"


--------------------------123--`))
	spew.Dump(results)
	println(string(results))
	if !strings.Contains(string(results), `boundary=------------------------123`) {
		panic(1)
	}

	if !strings.Contains(spew.Sdump(results), `3a 20 5c  22 76 61 6c 75 65 5c 22  |key\": \"value\"|
 000000e0  7d 22 0d 0a 2d 2d 2d 2d  2d 2d 2d 2d 2d 2d 2d 2d  |}"..------------|
 000000f0  2d 2d 2d 2d 2d 2d 2d 2d  2d 2d 2d 2d 2d 2d 31 32  |--------------12|
 00000100  33 0d 0a 43 6f 6e 74 65  6e 74 2d 44 69 73 70 6f  |3..Content-Dispo|
 00000110  73 69 74 69 6f 6e 3a 20  66 6f 72 6d 2d 64 61 74  |sition: form-dat|
 00000120  61 3b 20 6e 61 6d 65 3d  22 7b 5c 22 6b 65 79 5c  |a; name="{\"key\|
 00000130  22 3a 20 5c 22 76 61 6c  75 65 5c 22 7d 22 0d 0a  |": \"value\"}"..|
 00000140  0d 0a 0d 0a 2d 2d 2d 2d  2d 2d 2d 2d 2d 2d 2d 2d  |....------------|
 00000150  2d 2d 2d 2d 2d 2d 2d 2d  2d 2d 2d 2d 2d 2d 31 32  |--------------12|
 00000160  33 2d 2d 0d 0a                                    |3--..|`) {
		panic("CRLF Fix Failed")
	}
}

func TestFixHTTPPacketBoundary_2(t *testing.T) {
	results := FixHTTPRequest([]byte(`POST / HTTP/1.1
Host: www.example.com
Content-Length: 203
Content-Type: multipart/form-data; boundary=------------------------cDkWacGqpxxAkcXkxoWoNItodEKPxryzekgvPhwK

--------------------------123
Content-Disposition: form-data; name="{\"key\": \"value\"}"
--------------------------123
Content-Disposition: form-data; name="{\"key\": \"value\"}"

--------------------------123--`))
	spew.Dump(results)
	println(string(results))
	if !strings.Contains(string(results), `boundary=------------------------123`) {
		panic(1)
	}

	if !strings.Contains(spew.Sdump(results), `.--------|
 00000090  2d 2d 2d 2d 2d 2d 2d 2d  2d 2d 2d 2d 2d 2d 2d 2d  |----------------|
 000000a0  2d 2d 31 32 33 0d 0a 43  6f 6e 74 65 6e 74 2d 44  |--123..Content-D|
 000000b0  69 73 70 6f 73 69 74 69  6f 6e 3a 20 66 6f 72 6d  |isposition: form|
 000000c0  2d 64 61 74 61 3b 20 6e  61 6d 65 3d 22 7b 5c 22  |-data; name="{\"|
 000000d0  6b 65 79 5c 22 3a 20 5c  22 76 61 6c 75 65 5c 22  |key\": \"value\"|
 000000e0  7d 22 0d 0a 2d 2d 2d 2d  2d 2d 2d 2d 2d 2d 2d 2d  |}"..------------|
 000000f0  2d 2d 2d 2d 2d 2d 2d 2d  2d 2d 2d 2d 2d 2d 31 32  |--------------12|
 00000100  33 0d 0a 43 6f 6e 74 65  6e 74 2d 44 69 73 70 6f  |3..Content-Dispo|
 00000110  73 69 74 69 6f 6e 3a 20  66 6f 72 6d 2d 64 61 74  |sition: form-dat|
 00000120  61 3b 20 6e 61 6d 65 3d  22 7b 5c 22 6b 65 79 5c  |a; name="{\"key\|
 00000130  22 3a 20 5c 22 76 61 6c  75 65 5c 22 7d 22 0d 0a  |": \"value\"}"..|
 00000140  0d 0a 2d 2d 2d 2d 2d 2d  2d 2d 2d 2d 2d 2d 2d 2d  |..--------------|
 00000150  2d 2d 2d 2d 2d 2d 2d 2d  2d 2d 2d 2d 31 32 33 2d  |------------123-|
 00000160  2d 0d 0a                                          |-..|`) {
		panic("Fix CRLF Error Boundary Prototext")
	}
}

func TestFixHTTPPacketBoundary_3(t *testing.T) {
	results := FixHTTPRequest([]byte(`POST / HTTP/1.1
Host: www.example.com
Content-Length: 203
Content-Type: multipart/form-data; boundary=------------------------cDkWacGqpxxAkcXkxoWoNItodEKPxryzekgvPhwK

--------------------------123
Content-Disposition: form-data; name="{\"key\": \"value\"}"

123
--------------------------123
Content-Disposition: form-data; name="{\"key\": \"value\"}"

abc
--------------------------123--`))
	spew.Dump(results)
	println(string(results))
	if !strings.Contains(string(results), `boundary=------------------------123`) {
		panic(1)
	}

	if !strings.Contains(spew.Sdump(results), `00000e0  7d 22 0d 0a 0d 0a 31 32  33 0d 0a 2d 2d 2d 2d 2d  |}"....123..-----|
 000000f0  2d 2d 2d 2d 2d 2d 2d 2d  2d 2d 2d 2d 2d 2d 2d 2d  |----------------|
 00000100  2d 2d 2d 2d 2d 31 32 33  0d 0a 43 6f 6e 74 65 6e  |-----123..Conten|
 00000110  74 2d 44 69 73 70 6f 73  69 74 69 6f 6e 3a 20 66  |t-Disposition: f|
 00000120  6f 72 6d 2d 64 61 74 61  3b 20 6e 61 6d 65 3d 22  |orm-data; name="|
 00000130  7b 5c 22 6b 65 79 5c 22  3a 20 5c 22 76 61 6c 75  |{\"key\": \"valu|
 00000140  65 5c 22 7d 22 0d 0a 0d  0a 61 62 63 0d 0a 2d 2d  |e\"}"....abc..--|`) {
		panic("CRLF Fix Failed")
	}
}

func TestFixHTTPPacketBoundary_WithChunk_3(t *testing.T) {
	results := FixHTTPRequest([]byte(`POST / HTTP/1.1
Host: www.example.com
Content-Length: 203
Content-Type: multipart/form-data; boundary=------------------------cDkWacGqpxxAkcXkxoWoNItodEKPxryzekgvPhwK
Transfer-Encoding: chunked

--------------------------123
Content-Disposition: form-data; name="{\"key\": \"value\"}"

123
--------------------------123
Content-Disposition: form-data; name="{\"key\": \"value\"}"

abc
--------------------------123--`))
	results2 := FixHTTPRequest(results)
	spew.Dump(results)
	spew.Dump(results2)
	if codec.Md5(results) != codec.Md5(results2) {
		panic("FixHTTPRequest is unstable")
	}
	spew.Dump(results)
	println(string(results))
	if !strings.Contains(string(results), `boundary=------------------------123`) {
		t.FailNow()
	}

	if strings.Contains(string(results), `Content-Length: 203`) {
		t.FailNow()
	}

	if !strings.HasSuffix(strings.TrimSpace(string(results)), "123--") {
		t.FailNow()
	}
}

/*
POST / HTTP/1.1
Host: 0ac200e70432a698c2a2de6500bb00ff.web-security-academy.net
Connection: keep-alive
Content-Type: application/x-www-form-urlencoded
Content-Length: 6
Transfer-Encoding: chunked

0

G
*/
func TestFixHTTPPacketBoundary_WithChunk_4(t *testing.T) {
	results := FixHTTPRequest([]byte(`POST / HTTP/1.1
Host: 0ac200e70432a698c2a2de6500bb00ff.web-security-academy.net
Connection: keep-alive
Content-Type: application/x-www-form-urlencoded
Content-Length: 6
Transfer-Encoding: chunked

0

G` + "\r\n"))
	spew.Dump(results)
	println(string(results))
	if !strings.Contains(string(results), `Content-Length: 6`) {
		t.FailNow()
	}
}

func TestExtractStatusCode(t *testing.T) {
	spew.Dump(ExtractStatusCodeFromResponse([]byte(`HTTP/1.1 200 Ok`)))
	if ExtractStatusCodeFromResponse([]byte(`HTTP/1.1 200 Ok`)) != 200 {
		panic(1)
	}
	if ExtractStatusCodeFromResponse([]byte(`HTTP/1.1 200`)) != 200 {
		panic(1)
	}
	if ExtractStatusCodeFromResponse([]byte(`HTTP/1 200`)) != 200 {
		panic(1)
	}
	if ExtractStatusCodeFromResponse([]byte(`HTTP/1 199`)) != 199 {
		panic(1)
	}
	if ExtractStatusCodeFromResponse([]byte(`HTTP/1 199 Ok asdfasidfas 
asdfhasdfasdf
as
df
asdf
as
df`)) != 199 {
		panic(1)
	}
}

func TestGZIPCHUNKED(t *testing.T) {
	a := HTTPPacketForceChunked([]byte(`GET / HTTP/1.1
Host: www.baidu.com

abcdadasdfabcdadasdfabcdadasdfabcdadasdfabcdadasdf`))
	fmt.Println(string(a))
	fmt.Println(strconv.Quote(string(a)))
	require.Containsf(t, string(a), "Transfer-Encoding: chunked", "chunked not found")
	require.Containsf(t, string(a), "\r\n0\r\n\r\n", "chunked not found")
}

func TestHTTPPacketCRLF_EmptyResult(t *testing.T) {
	as := FixHTTPPacketCRLF(nil, true)
	spew.Dump(as)
}

func TestHTTPHeaderForceChunked(t *testing.T) {
	// 测试将Content-Length转换为chunked传输编码
	t.Run("Content-Length to chunked", func(t *testing.T) {
		rawRequest := `POST /api/test HTTP/1.1
Host: example.com
Content-Type: application/json
Content-Length: 17

{"message":"test"}`

		result := HTTPHeaderForceChunked([]byte(rawRequest))
		resultStr := string(result)

		// 验证Transfer-Encoding: chunked存在
		require.Contains(t, resultStr, "Transfer-Encoding: chunked", "should have Transfer-Encoding: chunked")

		// 验证Content-Length不存在
		require.NotContains(t, resultStr, "Content-Length:", "should not contain Content-Length")

		// 验证body保持不变
		require.Contains(t, resultStr, `{"message":"test"}`, "body should be preserved")

		// 验证其他头部保持不变
		require.Contains(t, resultStr, "Host: example.com", "Host header should be preserved")
		require.Contains(t, resultStr, "Content-Type: application/json", "Content-Type header should be preserved")
	})

	// 测试已经是chunked的情况
	t.Run("Already chunked", func(t *testing.T) {
		rawRequest := `POST /api/test HTTP/1.1
Host: example.com
Transfer-Encoding: chunked

test data`

		result := HTTPHeaderForceChunked([]byte(rawRequest))
		resultStr := string(result)

		// 验证Transfer-Encoding: chunked存在
		require.Contains(t, resultStr, "Transfer-Encoding: chunked", "should have Transfer-Encoding: chunked")

		// 验证body保持不变
		require.Contains(t, resultStr, "test data", "body should be preserved")
	})

	// 测试空body
	t.Run("Empty body", func(t *testing.T) {
		rawRequest := `GET /api/test HTTP/1.1
Host: example.com
Content-Length: 0

`

		result := HTTPHeaderForceChunked([]byte(rawRequest))
		resultStr := string(result)

		// 验证Transfer-Encoding: chunked存在
		require.Contains(t, resultStr, "Transfer-Encoding: chunked", "should have Transfer-Encoding: chunked")

		// 验证Content-Length不存在
		require.NotContains(t, resultStr, "Content-Length:", "should not contain Content-Length")
	})

	// 测试multipart body
	t.Run("Multipart body", func(t *testing.T) {
		rawRequest := `POST /upload HTTP/1.1
Host: example.com
Content-Type: multipart/form-data; boundary=----WebKitFormBoundary
Content-Length: 145

------WebKitFormBoundary
Content-Disposition: form-data; name="file"; filename="test.txt"

file content
------WebKitFormBoundary--`

		result := HTTPHeaderForceChunked([]byte(rawRequest))
		resultStr := string(result)

		// 验证Transfer-Encoding: chunked存在
		require.Contains(t, resultStr, "Transfer-Encoding: chunked", "should have Transfer-Encoding: chunked")

		// 验证Content-Length不存在
		require.NotContains(t, resultStr, "Content-Length:", "should not contain Content-Length")

		// 验证multipart内容保持不变
		require.Contains(t, resultStr, "------WebKitFormBoundary", "multipart boundary should be preserved")
		require.Contains(t, resultStr, "file content", "file content should be preserved")
	})
}

func TestTrimTestForFixCRLF(t *testing.T) {
	packet := "GET / HTTP/1.1\r\nHost: www.example.com\r\nTest: 1 \r\n\r\na"
	result := FixHTTPPacketCRLF([]byte(packet), false)
	spew.Dump(result)
	require.Contains(t, string(result), "\r\nTest: 1 \r\n")
}

func TestTrimTestForFixCRLF_For_multipart(t *testing.T) {
	packet := "GET / HTTP/1.1\r\nHost: www.example.com\r\nContent-Type: multipart/form-data; boundary=----WebKitFormBoundaryO8YOixBrea99FhJk  \r\nContent-Length: 139\r\n\r\n" +
		"------WebKitFormBoundaryO8YOixBrea99FhJk\r\n" +
		"Content-Disposition: form-data; name=\"key\"\r\n\r\n" +
		"value\r\n------WebKitFormBoundaryO8YOixBrea99FhJk--\r\n"
	result := FixHTTPPacketCRLF([]byte(packet), false)
	spew.Dump(result)
	require.Contains(t, string(result), "ea99FhJk  \r\nContent-Length")
}

func TestTrimTestForFixCRLF_For_multipart2(t *testing.T) {
	packet := "GET / HTTP/1.1\r\nHost: www.example.com\r\nContent-Type: multipart/form-data; boundary=----WebKitFormBoundaryO8YOixBrea99FhJk\r\nContent-Length: 139\r\n\r\n" +
		"------WebKitFormBoundaryO8YOixBrea99FhJk\r\n" +
		"Content-Disposition: form-data; name=\"key\"\r\n\r\n" +
		"value\r\n------WebKitFormBoundaryO8YOixBrea99FhJk--\r\n"
	result := FixHTTPPacketCRLF([]byte(packet), false)
	spew.Dump(result)
	require.Contains(t, string(result), "ea99FhJk\r\nContent-Length")
}

func TestTrimTestForFixCRLF_For_multipart3(t *testing.T) {
	packet := "GET / HTTP/1.1\r\nHost: www.example.com\r\nContent-Type: multipart/form-data; boundary=----WebKitFormBoundaryO8YOixBrea99FhJk   \r\nContent-Length: 139\r\n\r\n" +
		"------WebKitFormBoundaryO8YOixBrea99FhJk   \r\n" +
		"Content-Disposition: form-data; name=\"key\"\r\n\r\n" +
		"value\r\n------WebKitFormBoundaryO8YOixBrea99FhJk   --\r\n"
	result := FixHTTPPacketCRLF([]byte(packet), false)
	spew.Dump(result)
	require.Contains(t, string(result), "ea99FhJk   \r\nContent-Length")
	require.Contains(t, string(result), "ea99FhJk   --")
}

func TestFixHTTPPacketQueryEscape(t *testing.T) {
	t.Run("basic query escape", func(t *testing.T) {
		// 测试基本的 query 参数转义
		packet := `GET /test?name=hello world&age=18 HTTP/1.1
Host: www.example.com

`
		result := FixHTTPPacketQueryEscape([]byte(packet))
		resultStr := string(result)
		spew.Dump(resultStr)

		// 空格应该被转义为 %20
		require.Contains(t, resultStr, "name=hello+world", "space should be escaped")
		require.Contains(t, resultStr, "age=18", "normal param should be kept")
		require.NotContains(t, resultStr, "hello world", "unescaped space should not exist")
	})

	t.Run("special characters escape", func(t *testing.T) {
		// 测试特殊字符转义
		packet := `GET /api?key=value&special=<script>alert(1)</script>&data=a&b HTTP/1.1
Host: www.example.com

`
		result := FixHTTPPacketQueryEscape([]byte(packet))
		resultStr := string(result)
		spew.Dump(resultStr)

		// 特殊字符应该被转义
		require.Contains(t, resultStr, "key=value", "normal param should be kept")
		require.NotContains(t, resultStr, "<script>", "< should be escaped")
		require.NotContains(t, resultStr, "</script>", "> should be escaped")
		require.Contains(t, resultStr, "%3C", "< should be escaped to %3C")
		require.Contains(t, resultStr, "%3E", "> should be escaped to %3E")
	})

	t.Run("chinese characters escape", func(t *testing.T) {
		// 测试中文字符转义
		packet := `GET /search?q=测试&lang=中文 HTTP/1.1
Host: www.example.com

`
		result := FixHTTPPacketQueryEscape([]byte(packet))
		resultStr := string(result)
		spew.Dump(resultStr)

		// 中文字符应该被转义
		require.NotContains(t, resultStr, "测试", "chinese characters should be escaped")
		require.NotContains(t, resultStr, "中文", "chinese characters should be escaped")
		require.Contains(t, resultStr, "%", "escaped characters should contain %")
	})

	t.Run("already escaped parameters", func(t *testing.T) {
		// 测试已经转义的参数应该被重新规范化
		packet := `GET /test?name=hello%20world&key=%3Cvalue%3E HTTP/1.1
Host: www.example.com

`
		result := FixHTTPPacketQueryEscape([]byte(packet))
		resultStr := string(result)
		spew.Dump(resultStr)

		// 已转义的参数应该被保持或重新编码
		require.Contains(t, resultStr, "name=hello", "param should exist")
		require.Contains(t, resultStr, "key=", "key should exist")
	})

	t.Run("empty query string", func(t *testing.T) {
		// 测试没有 query 参数的情况
		packet := `GET /test HTTP/1.1
Host: www.example.com

`
		result := FixHTTPPacketQueryEscape([]byte(packet))
		resultStr := string(result)
		spew.Dump(resultStr)

		require.Contains(t, resultStr, "GET /test HTTP/1.1", "request line should be kept")
		require.Contains(t, resultStr, "Host: www.example.com", "headers should be kept")
	})

	t.Run("multiple same key params", func(t *testing.T) {
		// 测试多个相同 key 的参数
		packet := `GET /api?tag=go&tag=test&tag=hello world HTTP/1.1
Host: www.example.com

`
		result := FixHTTPPacketQueryEscape([]byte(packet))
		resultStr := string(result)
		spew.Dump(resultStr)

		// 所有 tag 参数都应该存在
		require.Contains(t, resultStr, "tag=go", "first tag should exist")
		require.Contains(t, resultStr, "tag=test", "second tag should exist")
		require.Contains(t, resultStr, "tag=hello", "third tag should exist and be escaped")
	})

	t.Run("query with equals sign in value", func(t *testing.T) {
		// 测试值中包含等号的情况
		packet := `GET /test?formula=a=b+c&x=1 HTTP/1.1
Host: www.example.com

`
		result := FixHTTPPacketQueryEscape([]byte(packet))
		resultStr := string(result)
		spew.Dump(resultStr)

		require.Contains(t, resultStr, "formula=", "param key should exist")
		require.Contains(t, resultStr, "x=1", "other param should exist")
	})

	t.Run("preserve body", func(t *testing.T) {
		// 测试应该保留 body 内容
		packet := `GET /test?key=hello world HTTP/1.1
Host: www.example.com
Content-Length: 13

{"test":"ok"}`
		result := FixHTTPPacketQueryEscape([]byte(packet))
		resultStr := string(result)
		spew.Dump(resultStr)

		require.Contains(t, resultStr, `{"test":"ok"}`, "body should be preserved")
		require.Contains(t, resultStr, "key=hello", "query should be escaped")
	})
}
