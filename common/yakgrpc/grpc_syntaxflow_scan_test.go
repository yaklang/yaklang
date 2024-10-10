package yakgrpc

import (
	"context"
	"encoding/json"
	"io"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestGRPCMUSTPASS_SyntaxFlow_Scan(t *testing.T) {
	local, err := NewLocalClient()
	require.NoError(t, err)

	vf := filesys.NewVirtualFs()
	vf.AddFile("example/src/main/java/com/example/apackage/a.java", `
		package com.example.apackage; 
		import com.example.bpackage.sub.B;
		class A {
			public static void main(String[] args) {
				B b = new B();
				// for test 1: A->B
				target1(b.get());
				// for test 2: B->A
				b.show(1);
			}
		}
		`)

	vf.AddFile("example/src/main/java/com/example/bpackage/sub/b.java", `
		package com.example.bpackage.sub; 
		class B {
			public  int get() {
				return 	 1;
			}
			public void show(int a) {
				target2(a);
			}
		}
		`)
	progID := uuid.NewString()
	prog, err := ssaapi.ParseProject(vf,
		ssaapi.WithLanguage(consts.JAVA),
		ssaapi.WithProgramPath("example"),
		ssaapi.WithProgramName(progID),
	)
	defer func() {
		ssadb.DeleteProgram(ssadb.GetDB(), progID)
	}()
	require.NoError(t, err)
	require.NotNil(t, prog)

	stream, err := local.SyntaxFlowScan(context.Background())
	require.NoError(t, err)

	stream.Send(&ypb.SyntaxFlowScanRequest{
		ControlMode: "start",
		Filter:      &ypb.SyntaxFlowRuleFilter{},
		ProgramName: []string{
			progID,
		},
	})

	resp, err := stream.Recv()
	require.NoError(t, err)
	log.Infof("resp: %v", resp)
	taskID := resp.TaskID

	go func() {
		notify, err := local.DuplexConnection(context.Background())
		require.NoError(t, err)
		for {
			res, err := notify.Recv()
			require.NoError(t, err)
			if res.MessageType == "syntaxflow_result" {
				var tmp map[string]string
				err = json.Unmarshal(res.GetData(), &tmp)
				require.NoError(t, err)
				require.Equal(t, tmp["task_id"], taskID)

				res, err := local.QuerySyntaxFlowResult(context.Background(), &ypb.QuerySyntaxFlowResultRequest{
					Filter: &ypb.SyntaxFlowResultFilter{
						TaskIDs: []string{taskID},
					},
				})
				require.NoError(t, err)
				require.Greater(t, len(res.Results), 0)
			}
		}

	}()

	hasProcess := false
	finishProcess := 0.0
	for {
		resp, err := stream.Recv()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		log.Infof("resp %v", resp)
		if resp.ExecResult != nil && resp.ExecResult.IsMessage {
			rawMsg := resp.ExecResult.GetMessage()
			var msg msg
			json.Unmarshal(rawMsg, &msg)
			if msg.Type == "progress" {
				log.Infof("msg: %v", msg)
				if 0 < msg.Content.Process && msg.Content.Process < 1 {
					hasProcess = true
				}
				finishProcess = msg.Content.Process
			}
		}
	}
	require.True(t, hasProcess)
	require.Equal(t, 1.0, finishProcess)

	log.Infof("wait for task %v", taskID)

}

type msg struct {
	Type    string `json:"type"`
	Content struct {
		Level   string  `json:"level"`
		Data    string  `json:"data"`
		ID      string  `json:"id"`
		Process float64 `json:"progress"`
	}
}
