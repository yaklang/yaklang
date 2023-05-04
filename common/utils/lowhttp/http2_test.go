package lowhttp

import (
	"bytes"
	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/utils"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/hpack"
	"net/http"
	"strings"
	"testing"
)

func TestHpack(t *testing.T) {
	var originBuf bytes.Buffer
	headers := make(http.Header)
	headers["User-Agent"] = []string{
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/83.0.4103.116 Safari/537.36",
	}
	headers.Write(&originBuf)

	println(originBuf.String())

	println()
	println()
	spew.Dump(originBuf.Bytes())
	var buf bytes.Buffer
	encoder := hpack.NewEncoder(&buf)
	for k, vv := range headers {
		for _, v := range vv {
			encoder.WriteField(hpack.HeaderField{Name: k, Value: v})
		}
	}
	spew.Dump(buf.Bytes())
}

func TestFramer(t *testing.T) {
	var buf bytes.Buffer
	framer := http2.NewFramer(&buf, &buf)
	for _, block := range funk.Chunk([]byte(`hello world
hello world
hello worldhello worldhello worldhello worldasdfasdfh
`), 10).([][]byte) {
		framer.WriteData(1, false, block)
	}
	framer.WriteHeaders(http2.HeadersFrameParam{
		StreamID:      0,
		BlockFragment: nil,
		EndStream:     false,
		EndHeaders:    false,
		PadLength:     0,
		Priority:      http2.PriorityParam{},
	})
	framer.WriteData(1, true, nil)
	spew.Dump(buf.Bytes())
}

func TestFixHTTPResponse3(t *testing.T) {
	gzipData, err := utils.GzipCompress("你好")
	if err != nil {
		panic(err)
	}
	h2packet := `HTTP/2 200 Ok
Test: 111
Content-Encoding: gzip

` + string(gzipData)
	rsp, body, err := FixHTTPResponse([]byte(h2packet))
	if err != nil {
		panic(err)
	}
	if string(body) != "你好" {
		panic("GZIP FAILED")
	}
	rspStr := string(rsp)
	if strings.Contains(rspStr, "gzip") {
		panic("gzip has been not removed")
	}

	if strings.Contains(rspStr, "Content-Encoding") || strings.Contains(rspStr, "content-encoding") {
		panic("content-encoding has been not removed")
	}

	// keep content-length for h2 for now
	//if strings.Contains(rspStr, "content-length") || strings.Contains(rspStr, "Content-Length") {
	//	panic("Content-Length has been not removed")
	//}

	if !strings.Contains(rspStr, "你好") {
		panic("Fix PacketGzip Error")
	}
}
