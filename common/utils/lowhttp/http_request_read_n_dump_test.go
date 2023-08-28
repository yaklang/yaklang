package lowhttp

import (
	"bufio"
	"bytes"
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/utils"
	"strings"
	"testing"
)

func TestReadAndDump_TransferEncoding(t *testing.T) {
	packet := []byte(`GET / HTTP/1.1
Host: www.example.com
`)
	rs := utils.RandStringBytes(100)
	packet = ReplaceHTTPPacketBody(packet, []byte(rs), true)
	var a, err = utils.ReadHTTPRequestFromReader(bufio.NewReader(bytes.NewReader(packet)))
	if err != nil {
		spew.Dump(err)
		t.FailNow()
	}
	var raw, _ = utils.DumpHTTPRequest(a, true)
	fmt.Println(string(packet))
	fmt.Println("-------------------------------")
	fmt.Println(string(raw))
	if string(raw) == "" {
		t.Errorf("empty dump: %v", spew.Sdump(a))
		t.FailNow()
	}
	if strings.Contains(string(raw), "Content-Length") {
		t.Errorf("should not contain Content-Length: %v", spew.Sdump(a))
		t.FailNow()
	}
	if !bytes.HasSuffix(raw, []byte("\r\n0\r\n\r\n")) {
		t.Errorf("should end with \r\n0\\r\\n\\r\\n: %v", spew.Sdump(a))
		t.FailNow()
	}
}

func TestReadAndDump_ContentLength(t *testing.T) {
	packet := []byte(`GET / HTTP/1.1
Host: www.example.com
`)
	rs := utils.RandStringBytes(100)
	packet = ReplaceHTTPPacketBody(packet, []byte(rs), false)
	var a, err = utils.ReadHTTPRequestFromReader(bufio.NewReader(bytes.NewReader(packet)))
	if err != nil {
		spew.Dump(err)
		t.FailNow()
	}
	var raw, _ = utils.DumpHTTPRequest(a, true)
	fmt.Println(string(packet))
	fmt.Println("-------------------------------")
	fmt.Println(string(raw))
	if string(raw) == "" {
		t.Errorf("empty dump: %v", spew.Sdump(a))
		t.FailNow()
	}
	if !strings.Contains(string(raw), "Content-Length") {
		t.Errorf("should contain Content-Length: %v", spew.Sdump(a))
		t.FailNow()
	}
	if !bytes.HasSuffix(raw, []byte("\r\n"+rs)) {
		t.Errorf("should end with rs: %v", spew.Sdump(a))
		t.FailNow()
	}
}

func TestReadAndDump_ContentLength2(t *testing.T) {
	packet := []byte(`GET / HTTP/1.1
User-Agent: abc/1
Host: www.example.com
content-length: 1
`)
	rs := utils.RandStringBytes(100)
	packet = ReplaceHTTPPacketBody(packet, []byte(rs), false)
	var a, err = utils.ReadHTTPRequestFromReader(bufio.NewReader(bytes.NewReader(packet)))
	if err != nil {
		spew.Dump(err)
		t.FailNow()
	}
	var raw, _ = utils.DumpHTTPRequest(a, true)
	fmt.Println(string(packet))
	fmt.Println("-------------------------------")
	fmt.Println(string(raw))
	if string(raw) == "" {
		t.Errorf("empty dump: %v", spew.Sdump(a))
		t.FailNow()
	}
	if !strings.Contains(string(raw), "Content-Length") {
		t.Errorf("should contain Content-Length: %v", spew.Sdump(a))
		t.FailNow()
	}
	if !bytes.HasSuffix(raw, []byte("\r\n"+rs)) {
		t.Errorf("should end with rs: %v", spew.Sdump(a))
		t.FailNow()
	}
}

func TestReadAndDump_ContentLength2_MultiContentType(t *testing.T) {
	packet := []byte(`GET / HTTP/1.1
User-Agent: abc/1
Host: www.example.com
content-type: abc/a
Content-Type: abc/1b
content-length: 1
`)
	rs := utils.RandStringBytes(100)
	packet = ReplaceHTTPPacketBody(packet, []byte(rs), false)
	var a, err = utils.ReadHTTPRequestFromReader(bufio.NewReader(bytes.NewReader(packet)))
	if err != nil {
		spew.Dump(err)
		t.FailNow()
	}
	var raw, _ = utils.DumpHTTPRequest(a, true)
	fmt.Println(string(packet))
	fmt.Println("-------------------------------")
	fmt.Println(string(raw))
	if string(raw) == "" {
		t.Errorf("empty dump: %v", spew.Sdump(a))
		t.FailNow()
	}
	if !strings.Contains(string(raw), "Content-Length") {
		t.Errorf("should contain Content-Length: %v", spew.Sdump(a))
		t.FailNow()
	}
	if !bytes.HasSuffix(raw, []byte("\r\n"+rs)) {
		t.Errorf("should end with rs: %v", spew.Sdump(a))
		t.FailNow()
	}

	if bytes.Contains(raw, []byte(`content-type`)) {
		t.Errorf("should not contain content-type: %v", spew.Sdump(a))
		t.FailNow()
	}
}

func TestReadAndDump_TranferEncoding_2(t *testing.T) {
	packet := []byte(`GET / HTTP/1.1
User-Agent: abc/1
Host: www.example.com
transfer-encoding: chunked

1
a
0

`)
	var a, err = utils.ReadHTTPRequestFromReader(bufio.NewReader(bytes.NewReader(packet)))
	if err != nil {
		spew.Dump(err)
		t.FailNow()
	}
	var raw, _ = utils.DumpHTTPRequest(a, true)
	fmt.Println(string(packet))
	fmt.Println("-------------------------------")
	fmt.Println(string(raw))
	if string(raw) == "" {
		t.Errorf("empty dump: %v", spew.Sdump(a))
		t.FailNow()
	}
	if strings.Contains(string(raw), "Content-Length") {
		t.Errorf("should not contain Content-Length: %v", spew.Sdump(a))
		t.FailNow()
	}
	if !bytes.HasSuffix(raw, []byte("\r\n\r\n1\r\na\r\n0\r\n\r\n")) {
		t.Errorf("should end with rs: %v", spew.Sdump(raw))
		t.FailNow()
	}
}
