package yakgrpc

import (
	"context"
	"encoding/json"
	"io"
	"sync"
	"testing"

	"github.com/yaklang/yaklang/common/yakgrpc/yakit"

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
	client, err := NewLocalClient()
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

	startScan := func() (string, ypb.Yak_SyntaxFlowScanClient) {
		stream, err := client.SyntaxFlowScan(context.Background())
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
		return taskID, stream
	}

	pauseTask := func(stream ypb.Yak_SyntaxFlowScanClient) {
		err := stream.Send(&ypb.SyntaxFlowScanRequest{
			ControlMode: "pause",
		})
		require.NoError(t, err)
	}

	resumeTask := func(taskId string) ypb.Yak_SyntaxFlowScanClient {
		stream, err := client.SyntaxFlowScan(context.Background())
		require.NoError(t, err)
		stream.Send(&ypb.SyntaxFlowScanRequest{
			ControlMode:  "resume",
			ResumeTaskId: taskId,
		})
		return stream
	}

	deleteTask := func(taskId string) {
		err := ssadb.DeleteResultByTaskID(taskId)
		require.NoError(t, err)
		err = yakit.DeleteSyntaxFlowScanTask(consts.GetGormProjectDatabase(), taskId)
		require.NoError(t, err)
	}

	checkRecvMsg := func(stream ypb.Yak_SyntaxFlowScanClient, handlerStatus func(status string), handlerProcess func(process float64)) {
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
					handlerProcess(msg.Content.Process)
				}
			}
			handlerStatus(resp.Status)
		}
	}

	t.Run("start a syntax flow scan", func(t *testing.T) {
		taskID, stream := startScan()
		require.NoError(t, err)

		var wg sync.WaitGroup
		var c = make(chan struct{})
		wg.Add(1)
		go func() {
			defer wg.Done()
			notify, err := client.DuplexConnection(context.Background())
			require.NoError(t, err)
			for {
				select {
				case <-c:
					return
				default:
				}
				res, err := notify.Recv()
				require.NoError(t, err)
				if res.MessageType == "syntaxflow_result" {
					var tmp map[string]string
					err = json.Unmarshal(res.GetData(), &tmp)
					require.NoError(t, err)
					require.Equal(t, tmp["task_id"], taskID)
					res, err := client.QuerySyntaxFlowResult(context.Background(), &ypb.QuerySyntaxFlowResultRequest{
						Filter: &ypb.SyntaxFlowResultFilter{
							TaskIDs: []string{taskID},
							Keyword: "java",
						},
					})
					require.NoError(t, err)
					require.Greater(t, len(res.Results), 0)
				}
			}
		}()

		hasProcess := false
		finishProcess := 0.0
		var finishStatus string
		handlerStatus := func(status string) {
			finishStatus = status
		}

		handlerProcess := func(process float64) {
			if 0 < process && process < 1 {
				hasProcess = true
			}
			finishProcess = process
		}

		checkRecvMsg(stream, handlerStatus, handlerProcess)
		require.True(t, hasProcess)
		require.Equal(t, 1.0, finishProcess)
		require.Equal(t, "done", finishStatus)
		close(c)
		wg.Wait()
		log.Infof("wait for task %v", taskID)
	})

	t.Run("test pause and resume syntax flow scan", func(t *testing.T) {
		taskID, stream := startScan()
		defer deleteTask(taskID)

		finishProcess := 0.0
		var finishStatus string

		handlerStatus := func(status string) {
			finishStatus = status
		}
		handlerProcess := func(process float64) {
			if 0.5 < process && process < 0.7 {
				pauseTask(stream)
			}
			finishProcess = process
		}
		checkRecvMsg(stream, handlerStatus, handlerProcess)
		require.LessOrEqual(t, finishProcess, 0.7)
		require.GreaterOrEqual(t, finishProcess, 0.5)
		require.Equal(t, "paused", finishStatus)
		// resume
		newStream := resumeTask(taskID)
		haveExecute := false
		handlerAfterResumeStatus := func(status string) {
			if status == "executing" {
				haveExecute = true
			}
			finishStatus = status
		}
		handlerAfterResumeProcess := func(process float64) {
			finishProcess = process
		}
		checkRecvMsg(newStream, handlerAfterResumeStatus, handlerAfterResumeProcess)
		require.True(t, haveExecute)
		require.Equal(t, "done", finishStatus)
		require.Equal(t, 1.0, finishProcess)
	})
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
