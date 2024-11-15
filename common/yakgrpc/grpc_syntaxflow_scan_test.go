package yakgrpc

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"sync"
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

func checkSfScanRecvMsg(t *testing.T, stream ypb.Yak_SyntaxFlowScanClient, handlerStatus func(status string), handlerProcess func(process float64)) {
	for {
		resp, err := stream.Recv()
		if err == io.EOF {
			return
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

func startScan(client ypb.YakClient, t *testing.T, progID string, ctx context.Context) (string, ypb.Yak_SyntaxFlowScanClient) {
	stream, err := client.SyntaxFlowScan(ctx)
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

	taskID, stream := startScan(client, t, progID, context.Background())
	require.NoError(t, err)

	go func() {
		notify, err := client.DuplexConnection(context.Background())
		require.NoError(t, err)
		matchTaskID := false
		for {
			res, err := notify.Recv()
			require.NoError(t, err)
			if res.MessageType == ssadb.ServerPushType_SyntaxflowResult {
				var tmp map[string]string
				err = json.Unmarshal(res.GetData(), &tmp)
				require.NoError(t, err)
				log.Infof("taskid: %#v", tmp)
				if tmp["task_id"] == taskID {
					matchTaskID = true
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
		}
		require.True(t, matchTaskID)
	}()

	hasProcess := false
	finishProcess := 0.0
	var finishStatus string
	checkSfScanRecvMsg(t, stream, func(status string) {
		finishStatus = status
	}, func(process float64) {
		if 0 < process && process < 1 {
			hasProcess = true
		}
		finishProcess = process
	})
	require.True(t, hasProcess)
	require.Equal(t, 1.0, finishProcess)
	require.Equal(t, "done", finishStatus)
	log.Infof("wait for task %v", taskID)
}

func TestGRPCMUSTPASS_SyntaxFlow_Scan_Cancel(t *testing.T) {
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

	ctx, cancel := context.WithCancel(context.Background())

	id, stream := startScan(client, t, progID, ctx)
	_ = id

	hasProcess := false
	finishProcess := 0.0
	var finishStatus string

	for {
		resp, err := stream.Recv()
		if err != nil {
			if err == io.EOF {
				break
			}
			if strings.Contains(err.Error(), "context canceled") {
				break
			}
			require.NoError(t, err)
		}
		log.Infof("resp %v", resp)
		if resp.ExecResult != nil && resp.ExecResult.IsMessage {
			rawMsg := resp.ExecResult.GetMessage()
			var msg msg
			json.Unmarshal(rawMsg, &msg)
			if msg.Type == "progress" {
				log.Infof("msg: %v", msg)
				process := msg.Content.Process
				// check process
				if 0 < process && process < 1 {
					hasProcess = true
				}
				if process > 0.5 {
					// cancel context
					cancel()
				}
				finishProcess = process
			}
		}
		finishStatus = resp.Status
	}
	require.True(t, hasProcess)
	require.Less(t, finishProcess, 1.0)
	require.Equal(t, "executing", finishStatus)
}

func TestGRPCMUSTPASS_Syntaxflow_Scan_Cancel_Multiple(t *testing.T) {
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

	ctx, cancel := context.WithCancel(context.Background())
	id1, stream1 := startScan(client, t, progID, ctx)
	id2, stream2 := startScan(client, t, progID, context.Background())
	_ = id1
	_ = id2

	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		hasProcess := false
		finishProcess := 0.0
		var finishStatus string
		checkSfScanRecvMsg(t, stream2, func(status string) {
			finishStatus = status
		}, func(process float64) {
			if 0 < process && process < 1 {
				hasProcess = true
			}
			finishProcess = process
		})
		require.Equal(t, "done", finishStatus)
		require.True(t, hasProcess)
		require.Equal(t, 1.0, finishProcess)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		hasProcess := false
		finishProcess := 0.0
		var finishStatus string

		for {
			resp, err := stream1.Recv()
			if err != nil {
				if err == io.EOF {
					break
				}
				if strings.Contains(err.Error(), "context canceled") {
					break
				}
				require.NoError(t, err)
			}
			log.Infof("resp %v", resp)
			if resp.ExecResult != nil && resp.ExecResult.IsMessage {
				rawMsg := resp.ExecResult.GetMessage()
				var msg msg
				json.Unmarshal(rawMsg, &msg)
				if msg.Type == "progress" {
					log.Infof("msg: %v", msg)
					process := msg.Content.Process
					// check process
					if 0 < process && process < 1 {
						hasProcess = true
					}
					if process > 0.5 {
						// cancel context
						cancel()
					}
					finishProcess = process
				}
			}
			finishStatus = resp.Status
		}
		require.True(t, hasProcess)
		require.Less(t, finishProcess, 1.0)
		require.Equal(t, "executing", finishStatus)
	}()
	wg.Wait()
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
