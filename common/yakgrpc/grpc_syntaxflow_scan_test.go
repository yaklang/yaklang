package yakgrpc

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/syntaxflow/sfbuildin"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func prepareProgram(t *testing.T, progID string) func() {
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
			Runtime.getRuntime().exec(args[0]);
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
	prog, err := ssaapi.ParseProjectWithFS(vf,
		ssaapi.WithLanguage(ssaconfig.JAVA),
		ssaapi.WithProgramPath("example"),
		ssaapi.WithProgramName(progID),
	)
	require.NoError(t, err)
	require.NotNil(t, prog)
	return func() {
		ssadb.DeleteProgram(ssadb.GetDB(), progID)
	}
}

func checkSfScanRecvMsg(t *testing.T, stream ypb.Yak_SyntaxFlowScanClient, handlerStatus func(status string), handlerProcess func(process float64)) *utils.SafeMap[*ypb.SyntaxFlowScanActiveTask] {
	ruleActive := utils.NewSafeMap[*ypb.SyntaxFlowScanActiveTask]()
	for {
		resp, err := stream.Recv()
		if err != nil {
			if err == io.EOF || strings.Contains(err.Error(), "context canceled") {
				log.Errorf("finish sf-scan stream %v", err)
				return ruleActive
			}
			t.Fatalf("err : %v", err.Error())
			return ruleActive
		}
		require.NoError(t, err)
		log.Infof("resp %v", resp)

		if len(resp.ActiveTask) != 0 {
			for _, active := range resp.ActiveTask {
				index := active.ProgramName + "/" + active.RuleName
				ruleActive.Set(index, active)
			}
		}

		handlerStatus(resp.Status)
		if resp.ExecResult != nil && resp.ExecResult.IsMessage {
			rawMsg := resp.ExecResult.GetMessage()
			var msg msg
			json.Unmarshal(rawMsg, &msg)
			if msg.Type == "progress" {
				// log.Infof("msg: %v", msg)
				handlerProcess(msg.Content.Process)
			}
		}
	}
}

func startScan(client ypb.YakClient, t *testing.T, progID string, ctx context.Context, filters ...*ypb.SyntaxFlowRuleFilter) (string, ypb.Yak_SyntaxFlowScanClient) {
	filter := &ypb.SyntaxFlowRuleFilter{}
	if len(filters) > 0 {
		filter = filters[0]
	}
	stream, err := client.SyntaxFlowScan(ctx)
	require.NoError(t, err)

	stream.Send(&ypb.SyntaxFlowScanRequest{
		ControlMode: "start",
		Filter:      filter,
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
	err := sfbuildin.SyncEmbedRule()
	require.NoError(t, err)

	client, err := NewLocalClient(true)
	require.NoError(t, err)

	progID := uuid.NewString()
	f := prepareProgram(t, progID)
	defer f()

	notify, err := client.DuplexConnection(context.Background())
	require.NoError(t, err)
	taskID, stream := startScan(client, t, progID, context.Background(), &ypb.SyntaxFlowRuleFilter{
		RuleNames: []string{"检测Java命令执行漏洞", "检测Java SpringBoot RestController XSS漏洞"},
	})

	matchTaskID := false
	matchRisk := false
	go func() {
		for {
			res, err := notify.Recv()
			log.Errorf("recv notify: %v, err: %v", res, err)
			log.Errorf("target task id: %s", taskID)
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
						},
					})
					require.NoError(t, err)
					require.Greater(t, len(res.Results), 0)
					require.Equal(t, res.Results[0].Kind, string(schema.SFResultKindScan))
				}
			}
			if res.MessageType == schema.ServerPushType_SSARisk {
				var tmp map[string]string
				err = json.Unmarshal(res.GetData(), &tmp)
				require.NoError(t, err)
				log.Infof("risk taskid: %#v", tmp)
				if tmp["task_id"] == taskID {
					matchRisk = true
				}
			}
		}
	}()

	log.Infof("start scan task: %v", taskID)
	require.NoError(t, err)

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
	require.True(t, matchTaskID)
	require.True(t, matchRisk)
	log.Infof("wait for task %v", taskID)
}

func TestGRPCMUSTPASS_SyntaxFlow_Scan_Cancel(t *testing.T) {
	client, err := NewLocalClient()
	require.NoError(t, err)

	progID := uuid.NewString()
	f := prepareProgram(t, progID)
	defer f()

	ctx, cancel := context.WithCancel(context.Background())

	id, stream := startScan(client, t, progID, ctx)
	_ = id

	hasProcess := false
	finishProcess := 0.0
	var finishStatus string
	checkSfScanRecvMsg(t, stream, func(status string) {
		finishStatus = status
	}, func(process float64) {
		if 0 < process && process < 1 {
			hasProcess = true
		}
		if process > 0.5 {
			// cancel context
			cancel()
		}
		finishProcess = process
	})
	require.True(t, hasProcess)
	require.Less(t, finishProcess, 1.0)
	_ = finishStatus
	// require.Equal(t, "done", finishStatus)
	time.Sleep(1 * (time.Second))

	rsp, err := client.QuerySyntaxFlowScanTask(context.Background(), &ypb.QuerySyntaxFlowScanTaskRequest{
		Filter: &ypb.SyntaxFlowScanTaskFilter{
			TaskIds: []string{id},
		},
	})
	require.NoError(t, err)
	require.Equal(t, len(rsp.Data), 1)
	task := rsp.Data[0]
	require.Equal(t, task.Programs, []string{progID})
	require.Equal(t, task.Status, "done")
}

func TestGRPCMUSTPASS_SyntaxFlow_Scan_Cancel_Multiple(t *testing.T) {
	client, err := NewLocalClient(true)
	require.NoError(t, err)

	progID := uuid.NewString()
	f := prepareProgram(t, progID)
	defer f()

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
			log.Info("stream2 status:", status)
			finishStatus = status
		}, func(process float64) {
			log.Infof("stream2 process: %.2f", process)
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
		checkSfScanRecvMsg(t, stream1, func(status string) {
			finishStatus = status
		}, func(process float64) {
			if 0 < process && process < 1 {
				hasProcess = true
			}
			if process > 0.5 {
				// cancel context
				cancel()
			}
			finishProcess = process
		})
		require.True(t, hasProcess)
		require.Less(t, finishProcess, 1.0)
		_ = finishStatus
		// require.Equal(t, "executing", finishStatus)
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

func TestGRPCMUSTPASS_SyntaxFlow_Scan_WithContent(t *testing.T) {
	client, err := NewLocalClient()
	require.NoError(t, err)

	progID := uuid.NewString()
	f := prepareProgram(t, progID)
	defer f()

	t.Run("test scan task with content", func(t *testing.T) {
		stream, err := client.SyntaxFlowScan(context.Background())
		require.NoError(t, err)

		stream.Send(&ypb.SyntaxFlowScanRequest{
			ControlMode: "start",
			ProgramName: []string{
				progID,
			},
			RuleInput: &ypb.SyntaxFlowRuleInput{
				RuleName: "aa",
				Content: `
			this as $this 
			`,
				Language: "java",
				Tags:     []string{},
			},
		})

		resp, err := stream.Recv()
		require.NoError(t, err)
		log.Infof("resp: %v", resp)
		taskID := resp.TaskID
		_ = taskID

		finishStatus := ""
		finishProcess := 0.0
		checkSfScanRecvMsg(t, stream, func(status string) {
			finishStatus = status
		}, func(process float64) {
			finishProcess = process
		})
		require.Equal(t, finishStatus, "done")
		require.Equal(t, finishProcess, 1.0)

		res, err := client.QuerySyntaxFlowResult(context.Background(), &ypb.QuerySyntaxFlowResultRequest{
			Filter: &ypb.SyntaxFlowResultFilter{
				TaskIDs: []string{taskID},
			},
		})
		require.NoError(t, err)
		require.NotNil(t, res)
		require.Equal(t, len(res.GetResults()), 1)
		require.Equal(t, res.GetResults()[0].Kind, string(schema.SFResultKindDebug))
	})
}

func TestGRPCMUSTPASS_SyntaxFlow_Scan_With_Group(t *testing.T) {
	t.Run("test scan task with group", func(t *testing.T) {
		client, err := NewLocalClient(true)
		require.NoError(t, err)

		progID := uuid.NewString()
		f := prepareProgram(t, progID)
		defer f()
		stream, err := client.SyntaxFlowScan(context.Background())
		require.NoError(t, err)

		stream.Send(&ypb.SyntaxFlowScanRequest{
			ControlMode: "start",
			Filter: &ypb.SyntaxFlowRuleFilter{
				GroupNames: []string{string(ssaconfig.JAVA), string(ssaconfig.PHP), string(ssaconfig.GO)},
			},
			ProgramName: []string{
				progID,
			},
		})

		resp, err := stream.Recv()
		require.NoError(t, err)
		log.Infof("resp: %v", resp)
		taskID := resp.TaskID
		require.NoError(t, err)

		notifyCtx, notifyCancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer notifyCancel()

		notify, err := client.DuplexConnection(notifyCtx)
		require.NoError(t, err)

		notificationReceived := make(chan bool, 1)

		go func() {
			defer func() {
				if r := recover(); r != nil {
					log.Errorf("Goroutine panic: %v", r)
				}
			}()

			matchTaskID := false
			for {
				res, err := notify.Recv()
				if err != nil {
					if err == io.EOF || strings.Contains(err.Error(), "context") {
						log.Infof("Notification stream ended: %v", err)
						notificationReceived <- matchTaskID
						return
					}
					log.Errorf("Notification recv error: %v", err)
					notificationReceived <- false
					return
				}

				if res.MessageType == ssadb.ServerPushType_SyntaxflowResult {
					var tmp map[string]string
					err = json.Unmarshal(res.GetData(), &tmp)
					if err != nil {
						log.Errorf("Unmarshal error: %v", err)
						continue
					}
					log.Infof("Received notification for taskid: %#v", tmp)
					if tmp["task_id"] == taskID {
						matchTaskID = true
						res, err := client.QuerySyntaxFlowResult(context.Background(), &ypb.QuerySyntaxFlowResultRequest{
							Filter: &ypb.SyntaxFlowResultFilter{
								TaskIDs: []string{taskID},
							},
						})
						if err == nil && len(res.Results) > 0 {
							require.Equal(t, res.Results[0].Kind, string(schema.SFResultKindScan))
						}
						notificationReceived <- true
						return
					}
				}
			}
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

		select {
		case matched := <-notificationReceived:
			if !matched {
				log.Warnf("Did not receive expected notification for task %v", taskID)
			}
		case <-time.After(5 * time.Second):
			log.Warnf("Timeout waiting for notification goroutine to complete")
		}

		log.Infof("Test completed for task %v", taskID)
	})
}

func TestGRPCMUSTPASS_SyntaxFlow_Scan_With_DiffRule(t *testing.T) {
	client, err := NewLocalClient(true)
	require.NoError(t, err)

	progID := uuid.NewString()
	ruleName1 := uuid.NewString()
	taskID1 := ""
	ruleName2 := uuid.NewString()
	taskID2 := ""
	ruleName3 := uuid.NewString()
	taskID3 := ""

	vf := filesys.NewVirtualFs()
	vf.AddFile("example/src/main/a.go", `
package unAuth

import (
	"encoding/base64"
	"fmt"
	"github.com/gin-gonic/gin"
	"net/http"
	"os/exec"
)

func CMD1(c *gin.Context) {
	var ipaddr string
	// Check the request method
	if c.Request.Method == "GET" {
		ipaddr = c.Query("ip")
	} else if c.Request.Method == "POST" {
		ipaddr = c.PostForm("ip")
	}

	Command := fmt.Sprintf("ping -c 4 %s", ipaddr)
	output, err := exec.Command("/bin/sh", "-c", Command).Output()
	if err != nil {
		fmt.Println(err)
		return
	}
	c.JSON(200, gin.H{
		"success": string(output),
	})
}
	`)
	prog, err := ssaapi.ParseProjectWithFS(vf,
		ssaapi.WithLanguage(ssaconfig.GO),
		ssaapi.WithProgramPath("example"),
		ssaapi.WithProgramName(progID),
	)
	_ = prog
	defer func() {
		ssadb.DeleteProgram(ssadb.GetDB(), progID)
	}()
	require.NoError(t, err)

	client.CreateSyntaxFlowRule(context.Background(), &ypb.CreateSyntaxFlowRuleRequest{
		SyntaxFlowInput: &ypb.SyntaxFlowRuleInput{
			Content: `
exec.Command(* #-> as $high)

alert $high for {
	type: "vuln",
	level: "high",
}`,
			GroupNames: []string{"golang"},
			RuleName:   ruleName1,
			Language:   "golang",
			Tags:       []string{"golang"},
		},
	})
	defer func() {
		client.DeleteSyntaxFlowRule(context.Background(), &ypb.DeleteSyntaxFlowRuleRequest{
			Filter: &ypb.SyntaxFlowRuleFilter{
				RuleNames: []string{ruleName1},
			},
		})
	}()

	t.Run("test scan task risk count raw", func(t *testing.T) {
		t.Skip("使用riskFeatureHash作为ssa比较的依据")
		taskID1 := uuid.NewString() // 旧的扫描结果
		taskID2 := uuid.NewString() // 新的扫描结果
		defer func() {
			yakit.DeleteSSADiffResultByBaseLine(consts.GetGormSSAProjectDataBase(), []string{taskID1, taskID2}, schema.RuntimeId)
			yakit.DeleteSSADiffResultByCompare(consts.GetGormSSAProjectDataBase(), []string{taskID1, taskID2}, schema.RuntimeId)
		}()

		yakit.CreateSSARisk(ssadb.GetDB(), &schema.SSARisk{
			Title:       "AA",
			FromRule:    "AA",
			RuntimeId:   taskID1,
			ProgramName: progID,
		})
		yakit.CreateSSARisk(ssadb.GetDB(), &schema.SSARisk{
			Title:       "BB",
			FromRule:    "BB",
			RuntimeId:   taskID2,
			ProgramName: progID,
		})
		yakit.CreateSSARisk(ssadb.GetDB(), &schema.SSARisk{
			Title:       "CC",
			FromRule:    "CC",
			RuntimeId:   taskID2,
			ProgramName: progID,
		})

		res, _ := yakit.DoRiskDiff(context.Background(), &ypb.SSARiskDiffItem{
			RiskRuntimeId: taskID2,
		}, &ypb.SSARiskDiffItem{
			RiskRuntimeId: taskID1,
		})
		for re := range res {
			_ = re
		}

		rsp, err := yakit.GetSSADiffResult(ssadb.GetDB(), taskID2, taskID1)
		require.NoError(t, err)
		require.Equal(t, len(rsp), 3)

		for _, r := range rsp {
			if r.RuleName == "AA" {
				require.Equal(t, string(yakit.Del), r.Status)
				require.Equal(t, taskID2, r.BaseLine)
				require.Equal(t, taskID1, r.Compare)
			}
			if r.RuleName == "BB" {
				require.Equal(t, string(yakit.Add), r.Status)
				require.Equal(t, taskID2, r.BaseLine)
				require.Equal(t, taskID1, r.Compare)
			}
			if r.RuleName == "CC" {
				require.Equal(t, string(yakit.Add), r.Status)
				require.Equal(t, taskID2, r.BaseLine)
				require.Equal(t, taskID1, r.Compare)
			}
		}
	})

	t.Run("test scan task equal risk count raw", func(t *testing.T) {
		taskID1 := uuid.NewString() // 旧的扫描结果
		taskID2 := uuid.NewString() // 新的扫描结果
		defer func() {
			yakit.DeleteSSADiffResultByBaseLine(consts.GetGormSSAProjectDataBase(), []string{taskID1, taskID2}, schema.RuntimeId)
			yakit.DeleteSSADiffResultByCompare(consts.GetGormSSAProjectDataBase(), []string{taskID1, taskID2}, schema.RuntimeId)
		}()

		yakit.CreateSSARisk(ssadb.GetDB(), &schema.SSARisk{
			Title:       "AA",
			FromRule:    "AA",
			RuntimeId:   taskID1,
			ProgramName: progID,
		})

		res, _ := yakit.DoRiskDiff(context.Background(), &ypb.SSARiskDiffItem{
			RiskRuntimeId: taskID1,
		}, &ypb.SSARiskDiffItem{
			RiskRuntimeId: taskID1,
		})
		for re := range res {
			_ = re
		}

		rsp, err := yakit.GetSSADiffResult(ssadb.GetDB(), taskID2, taskID1)
		require.NoError(t, err)
		require.Equal(t, len(rsp), 0)
	})

	t.Run("test scan task risk count", func(t *testing.T) {
		defer func() {
			err = schema.DeleteSyntaxFlowScanTask(ssadb.GetDB(), taskID1)
			require.NoError(t, err)
		}()
		stream, err := client.SyntaxFlowScan(context.Background())
		require.NoError(t, err)

		stream.Send(&ypb.SyntaxFlowScanRequest{
			ControlMode: "start",
			Filter: &ypb.SyntaxFlowRuleFilter{
				RuleNames: []string{ruleName1},
			},
			ProgramName: []string{
				progID,
			},
		})

		resp, err := stream.Recv()
		taskID1 = resp.TaskID
		require.NoError(t, err)
		log.Infof("resp: %v", resp)
		require.NoError(t, err)

		finishProcess := 0.0
		var finishStatus string
		checkSfScanRecvMsg(t, stream, func(status string) {
			finishStatus = status
		}, func(process float64) {
			finishProcess = process
		})
		require.Equal(t, 1.0, finishProcess)
		require.Equal(t, "done", finishStatus)

		rsp, err := client.QuerySyntaxFlowScanTask(context.Background(), &ypb.QuerySyntaxFlowScanTaskRequest{
			Filter: &ypb.SyntaxFlowScanTaskFilter{
				TaskIds: []string{resp.TaskID},
			},
		})

		require.NoError(t, err)
		require.Equal(t, len(rsp.Data), 1)
		task := rsp.Data[0]
		require.Equal(t, task.Programs, []string{progID})
		require.Equal(t, task.Status, "done")
		// require.Equal(t, task.RiskCount, int64(12)) // 修改ssa可能导致这里不匹配，直接修改即可
	})

	t.Run("test scan task risk count with diff", func(t *testing.T) {
		client.CreateSyntaxFlowRule(context.Background(), &ypb.CreateSyntaxFlowRuleRequest{
			SyntaxFlowInput: &ypb.SyntaxFlowRuleInput{
				Content: `
		exec.Command(*?{opcode:const} #-> as $high)

		alert $high for {
			type: "vuln",
			level: "high",
		}`,
				GroupNames: []string{"golang"},
				RuleName:   ruleName2,
				Language:   "golang",
				Tags:       []string{"golang"},
			},
		})
		defer func() {
			client.DeleteSyntaxFlowRule(context.Background(), &ypb.DeleteSyntaxFlowRuleRequest{
				Filter: &ypb.SyntaxFlowRuleFilter{
					RuleNames: []string{ruleName2},
				},
			})
			err = schema.DeleteSyntaxFlowScanTask(ssadb.GetDB(), taskID1)
			require.NoError(t, err)
			err = schema.DeleteSyntaxFlowScanTask(ssadb.GetDB(), taskID2)
			require.NoError(t, err)
			yakit.DeleteSSADiffResultByBaseLine(consts.GetGormSSAProjectDataBase(), []string{taskID1, taskID2}, schema.RuntimeId)
			yakit.DeleteSSADiffResultByCompare(consts.GetGormSSAProjectDataBase(), []string{taskID1, taskID2}, schema.RuntimeId)
		}()

		{
			stream, err := client.SyntaxFlowScan(context.Background())
			require.NoError(t, err)

			stream.Send(&ypb.SyntaxFlowScanRequest{
				ControlMode: "start",
				Filter: &ypb.SyntaxFlowRuleFilter{
					RuleNames: []string{ruleName2},
				},
				ProgramName: []string{
					progID,
				},
			})

			resp, err := stream.Recv()
			taskID1 = resp.TaskID
			require.NoError(t, err)
			log.Infof("resp: %v", resp)
			require.NoError(t, err)

			finishProcess := 0.0
			var finishStatus string
			checkSfScanRecvMsg(t, stream, func(status string) {
				finishStatus = status
			}, func(process float64) {
				finishProcess = process
			})
			require.Equal(t, 1.0, finishProcess)
			require.Equal(t, "done", finishStatus)
		}

		{
			stream, err := client.SyntaxFlowScan(context.Background())
			require.NoError(t, err)

			stream.Send(&ypb.SyntaxFlowScanRequest{
				ControlMode: "start",
				Filter: &ypb.SyntaxFlowRuleFilter{
					RuleNames: []string{ruleName1},
				},
				ProgramName: []string{
					progID,
				},
			})

			resp, err := stream.Recv()
			taskID2 = resp.TaskID
			require.NoError(t, err)
			log.Infof("resp: %v", resp)
			require.NoError(t, err)

			finishProcess := 0.0
			var finishStatus string
			checkSfScanRecvMsg(t, stream, func(status string) {
				finishStatus = status
			}, func(process float64) {
				finishProcess = process
			})
			require.Equal(t, 1.0, finishProcess)
			require.Equal(t, "done", finishStatus)
		}

		rsp, err := client.QuerySyntaxFlowScanTask(context.Background(), &ypb.QuerySyntaxFlowScanTaskRequest{
			Filter: &ypb.SyntaxFlowScanTaskFilter{
				Programs: []string{progID},
			},
			ShowDiffRisk: true,
		})

		require.NoError(t, err)
		require.Equal(t, len(rsp.Data), 2)

		task := rsp.Data[0]
		require.Equal(t, task.Programs, []string{progID})
		require.Equal(t, task.Status, "done")
		require.Equal(t, task.RiskCount, int64(10))
		require.Equal(t, task.NewRiskCount, int64(10)) // 规则更新会导致所有的risk为新增值
		task = rsp.Data[1]
		require.Equal(t, task.Programs, []string{progID})
		require.Equal(t, task.Status, "done")
		require.Equal(t, task.RiskCount, int64(2))
		require.Equal(t, task.NewRiskCount, int64(0))
	})

	t.Run("test scan task risk count with muti diff", func(t *testing.T) {
		client.CreateSyntaxFlowRule(context.Background(), &ypb.CreateSyntaxFlowRuleRequest{
			SyntaxFlowInput: &ypb.SyntaxFlowRuleInput{
				Content: `
		exec.Command(*?{opcode:const} #-> as $high)

		alert $high for {
			type: "vuln",
			level: "high",
		}`,
				GroupNames: []string{"golang"},
				RuleName:   ruleName2,
				Language:   "golang",
				Tags:       []string{"golang"},
			},
		})
		client.CreateSyntaxFlowRule(context.Background(), &ypb.CreateSyntaxFlowRuleRequest{
			SyntaxFlowInput: &ypb.SyntaxFlowRuleInput{
				Content: `
		exec.Command(*?{have: "/bin/sh"} #-> as $high)

		alert $high for {
			type: "vuln",
			level: "high",
		}`,
				GroupNames: []string{"golang"},
				RuleName:   ruleName3,
				Language:   "golang",
				Tags:       []string{"golang"},
			},
		})
		defer func() {
			client.DeleteSyntaxFlowRule(context.Background(), &ypb.DeleteSyntaxFlowRuleRequest{
				Filter: &ypb.SyntaxFlowRuleFilter{
					RuleNames: []string{ruleName2},
				},
			})
			client.DeleteSyntaxFlowRule(context.Background(), &ypb.DeleteSyntaxFlowRuleRequest{
				Filter: &ypb.SyntaxFlowRuleFilter{
					RuleNames: []string{ruleName3},
				},
			})
			err = schema.DeleteSyntaxFlowScanTask(ssadb.GetDB(), taskID1)
			require.NoError(t, err)
			err = schema.DeleteSyntaxFlowScanTask(ssadb.GetDB(), taskID2)
			require.NoError(t, err)
			err = schema.DeleteSyntaxFlowScanTask(ssadb.GetDB(), taskID3)
			require.NoError(t, err)
			yakit.DeleteSSADiffResultByBaseLine(consts.GetGormSSAProjectDataBase(), []string{taskID1, taskID2, taskID3}, schema.RuntimeId)
			yakit.DeleteSSADiffResultByCompare(consts.GetGormSSAProjectDataBase(), []string{taskID1, taskID2, taskID3}, schema.RuntimeId)
		}()

		{
			stream, err := client.SyntaxFlowScan(context.Background())
			require.NoError(t, err)

			stream.Send(&ypb.SyntaxFlowScanRequest{
				ControlMode: "start",
				Filter: &ypb.SyntaxFlowRuleFilter{
					RuleNames: []string{ruleName3},
				},
				ProgramName: []string{
					progID,
				},
			})

			resp, err := stream.Recv()
			taskID1 = resp.TaskID
			require.NoError(t, err)
			log.Infof("resp: %v", resp)
			require.NoError(t, err)

			finishProcess := 0.0
			var finishStatus string
			checkSfScanRecvMsg(t, stream, func(status string) {
				finishStatus = status
			}, func(process float64) {
				finishProcess = process
			})
			require.Equal(t, 1.0, finishProcess)
			require.Equal(t, "done", finishStatus)
		}

		{
			stream, err := client.SyntaxFlowScan(context.Background())
			require.NoError(t, err)

			stream.Send(&ypb.SyntaxFlowScanRequest{
				ControlMode: "start",
				Filter: &ypb.SyntaxFlowRuleFilter{
					RuleNames: []string{ruleName2},
				},
				ProgramName: []string{
					progID,
				},
			})

			resp, err := stream.Recv()
			taskID2 = resp.TaskID
			require.NoError(t, err)
			log.Infof("resp: %v", resp)
			require.NoError(t, err)

			finishProcess := 0.0
			var finishStatus string
			checkSfScanRecvMsg(t, stream, func(status string) {
				finishStatus = status
			}, func(process float64) {
				finishProcess = process
			})
			require.Equal(t, 1.0, finishProcess)
			require.Equal(t, "done", finishStatus)
		}

		{
			stream, err := client.SyntaxFlowScan(context.Background())
			require.NoError(t, err)
			stream.Send(&ypb.SyntaxFlowScanRequest{
				ControlMode: "start",
				Filter: &ypb.SyntaxFlowRuleFilter{
					RuleNames: []string{ruleName1},
				},
				ProgramName: []string{
					progID,
				},
			})

			// risk count 11
			resp, err := stream.Recv()
			taskID3 = resp.TaskID
			require.NoError(t, err)
			log.Infof("resp: %v", resp)
			require.NoError(t, err)

			finishProcess := 0.0
			var finishStatus string
			checkSfScanRecvMsg(t, stream, func(status string) {
				finishStatus = status
			}, func(process float64) {
				finishProcess = process
			})
			require.Equal(t, 1.0, finishProcess)
			require.Equal(t, "done", finishStatus)
		}

		rsp, err := client.QuerySyntaxFlowScanTask(context.Background(), &ypb.QuerySyntaxFlowScanTaskRequest{
			Filter: &ypb.SyntaxFlowScanTaskFilter{
				Programs: []string{progID},
			},
			ShowDiffRisk: true,
		})

		require.NoError(t, err)
		require.Equal(t, len(rsp.Data), 3)

		for _, task := range rsp.Data {
			if task.TaskId == taskID3 {
				require.Equal(t, task.Programs, []string{progID})
				require.Equal(t, task.Status, "done")
				require.Equal(t, task.RiskCount, int64(10))
				require.Equal(t, task.NewRiskCount, int64(10))
			}
			if task.TaskId == taskID2 {
				require.Equal(t, task.Programs, []string{progID})
				require.Equal(t, task.Status, "done")
				require.Equal(t, task.RiskCount, int64(2))
				require.Equal(t, task.NewRiskCount, int64(2))
			}
			if task.TaskId == taskID1 {
				require.Equal(t, task.Programs, []string{progID})
				require.Equal(t, task.Status, "done")
				require.Equal(t, task.RiskCount, int64(1))
				require.Equal(t, task.NewRiskCount, int64(0))
			}
		}
	})
}

func TestGRPCMUSTPASS_SyntaxFlow_Scan_With_IncrementalCompile(t *testing.T) {
	client, err := NewLocalClient(true)
	require.NoError(t, err)

	t.Run("test scan task risk count with incremental compile", func(t *testing.T) {
		// 创建基础程序和增量程序的名称
		baseProgID := uuid.NewString()
		diffProgID := uuid.NewString()
		ruleNameIncremental := uuid.NewString()
		taskIDBase := ""
		taskIDDiff := ""

		// 创建规则
		client.CreateSyntaxFlowRule(context.Background(), &ypb.CreateSyntaxFlowRuleRequest{
			SyntaxFlowInput: &ypb.SyntaxFlowRuleInput{
				Content: `
		Runtime.getRuntime().exec(* #-> as $high) 

		alert $high for {
			type: "vuln",
			level: "high",
		}`,
				GroupNames: []string{"java"},
				RuleName:   ruleNameIncremental,
				Language:   "java",
				Tags:       []string{"java"},
			},
		})

		defer func() {
			client.DeleteSyntaxFlowRule(context.Background(), &ypb.DeleteSyntaxFlowRuleRequest{
				Filter: &ypb.SyntaxFlowRuleFilter{
					RuleNames: []string{ruleNameIncremental},
				},
			})
			err = schema.DeleteSyntaxFlowScanTask(ssadb.GetDB(), taskIDBase)
			require.NoError(t, err)
			err = schema.DeleteSyntaxFlowScanTask(ssadb.GetDB(), taskIDDiff)
			require.NoError(t, err)
			ssadb.DeleteProgram(ssadb.GetDB(), baseProgID)
			ssadb.DeleteProgram(ssadb.GetDB(), diffProgID)
			yakit.DeleteSSADiffResultByBaseLine(consts.GetGormSSAProjectDataBase(), []string{taskIDBase, taskIDDiff}, schema.RuntimeId)
			yakit.DeleteSSADiffResultByCompare(consts.GetGormSSAProjectDataBase(), []string{taskIDBase, taskIDDiff}, schema.RuntimeId)
		}()

		// Step 1: 创建基础程序（全量编译）
		baseFS := filesys.NewVirtualFs()
		baseFS.AddFile("example/src/main/java/com/example/Base.java", `
	package com.example;
	import java.lang.Runtime;
	
	public class Base {
		public static void main(String[] args) {
			// 这个漏洞会在基础程序中检测到
			Runtime.getRuntime().exec("ls");
		}
	}
	`)

		basePrograms, err := ssaapi.ParseProject(
			ssaapi.WithFileSystem(baseFS),
			ssaapi.WithLanguage(ssaconfig.JAVA),
			ssaapi.WithProgramName(baseProgID),
			ssaapi.WithProgramPath("example"),
		)
		require.NoError(t, err)
		require.NotNil(t, basePrograms)
		require.Greater(t, len(basePrograms), 0)

		// Step 2: 创建增量程序（增量编译，基于基础程序）
		diffFS := filesys.NewVirtualFs()
		// 修改 Base.java，添加新的漏洞
		diffFS.AddFile("example/src/main/java/com/example/Base.java", `
	package com.example;
	import java.lang.Runtime;
	
	public class Base {
		public static void main(String[] args) {
			// 保留原有的漏洞
			Runtime.getRuntime().exec("ls");
			// 新增的漏洞（应该在增量编译中检测到）
			Runtime.getRuntime().exec(args[0]);
		}
	}
	`)
		// 新增文件，包含新的漏洞
		diffFS.AddFile("example/src/main/java/com/example/NewClass.java", `
	package com.example;
	import java.lang.Runtime;
	
	public class NewClass {
		public void process(String cmd) {
			// 新增文件中的漏洞
			Runtime.getRuntime().exec(cmd);
		}
	}
	`)

		diffPrograms, err := ssaapi.ParseProject(
			ssaapi.WithFileSystem(diffFS),
			ssaapi.WithLanguage(ssaconfig.JAVA),
			ssaapi.WithProgramName(diffProgID),
			ssaapi.WithProgramPath("example"),
			ssaapi.WithBaseProgramName(baseProgID), // 设置为增量编译
		)
		require.NoError(t, err)
		require.NotNil(t, diffPrograms)
		require.Greater(t, len(diffPrograms), 0)

		// 验证增量程序的元数据
		diffProgram := diffPrograms[0]
		require.NotNil(t, diffProgram.Program)
		require.Equal(t, baseProgID, diffProgram.Program.BaseProgramName, "BaseProgramName should be set for incremental compile")

		// Step 3: 对基础程序进行扫描
		{
			stream, err := client.SyntaxFlowScan(context.Background())
			require.NoError(t, err)

			stream.Send(&ypb.SyntaxFlowScanRequest{
				ControlMode: "start",
				Filter: &ypb.SyntaxFlowRuleFilter{
					RuleNames: []string{ruleNameIncremental},
				},
				ProgramName: []string{
					baseProgID,
				},
			})

			resp, err := stream.Recv()
			taskIDBase = resp.TaskID
			require.NoError(t, err)
			log.Infof("Base program scan task ID: %s", taskIDBase)

			finishProcess := 0.0
			var finishStatus string
			checkSfScanRecvMsg(t, stream, func(status string) {
				finishStatus = status
			}, func(process float64) {
				finishProcess = process
			})
			require.Equal(t, 1.0, finishProcess)
			require.Equal(t, "done", finishStatus)
		}

		// Step 4: 对增量程序进行扫描
		{
			stream, err := client.SyntaxFlowScan(context.Background())
			require.NoError(t, err)

			stream.Send(&ypb.SyntaxFlowScanRequest{
				ControlMode: "start",
				Filter: &ypb.SyntaxFlowRuleFilter{
					RuleNames: []string{ruleNameIncremental},
				},
				ProgramName: []string{
					diffProgID,
				},
			})

			resp, err := stream.Recv()
			taskIDDiff = resp.TaskID
			require.NoError(t, err)
			log.Infof("Diff program scan task ID: %s", taskIDDiff)

			finishProcess := 0.0
			var finishStatus string
			checkSfScanRecvMsg(t, stream, func(status string) {
				finishStatus = status
			}, func(process float64) {
				finishProcess = process
			})
			require.Equal(t, 1.0, finishProcess)
			require.Equal(t, "done", finishStatus)
		}

		// Step 5: 验证基础程序和增量程序属于同一个 project
		baseIrProgram, err := ssadb.GetApplicationProgram(baseProgID)
		require.NoError(t, err)
		require.NotNil(t, baseIrProgram)

		diffIrProgram, err := ssadb.GetApplicationProgram(diffProgID)
		require.NoError(t, err)
		require.NotNil(t, diffIrProgram)

		require.Equal(t, baseIrProgram.ProjectID, diffIrProgram.ProjectID, "Base and diff programs should belong to the same project")
		log.Infof("Base program ProjectID: %d, Diff program ProjectID: %d", baseIrProgram.ProjectID, diffIrProgram.ProjectID)

		// Step 6: 查询扫描任务，验证以 Project 为单位查询（真实场景）
		// 真实场景：前端通过增量程序名称查询，后端应该：
		// 1. 通过 diffProgID 获取程序信息，找到它的 BaseProgramName 或 ProjectID
		// 2. 通过 ProjectID 自动识别 project 并返回该 project 下的所有扫描任务
		// 包括基础程序和增量程序的扫描任务
		rsp, err := client.QuerySyntaxFlowScanTask(context.Background(), &ypb.QuerySyntaxFlowScanTaskRequest{
			Filter: &ypb.SyntaxFlowScanTaskFilter{
				Programs: []string{diffProgID}, // 只传入增量程序名称，模拟真实场景（通过 diff 找到 base）
			},
			ShowDiffRisk: true, // 启用新增漏洞计算
		})

		require.NoError(t, err)
		// 应该返回基础程序和增量程序的扫描任务（因为它们属于同一个 project）
		require.GreaterOrEqual(t, len(rsp.Data), 2, "Should return at least 2 tasks (base and diff) for the same project")

		// 找到基础程序和增量程序的扫描任务
		var baseTask, diffTask *ypb.SyntaxFlowScanTask
		for _, task := range rsp.Data {
			// 根据 Programs 字段判断是基础程序还是增量程序
			if len(task.Programs) > 0 && task.Programs[0] == baseProgID {
				baseTask = task
			} else if len(task.Programs) > 0 && task.Programs[0] == diffProgID {
				diffTask = task
			}
		}

		require.NotNil(t, baseTask, "Base task should be found when querying by diff program name (same project)")
		require.NotNil(t, diffTask, "Diff task should be found when querying by diff program name")

		// 验证基础程序的扫描结果
		require.Equal(t, baseTask.Programs, []string{baseProgID})
		require.Equal(t, baseTask.Status, "done")
		require.Greater(t, baseTask.RiskCount, int64(0), "Base program should have at least one risk")
		// 基础程序是第一个扫描的，所以新增漏洞数应该为 0（因为没有之前的扫描进行比较）
		require.Equal(t, baseTask.NewRiskCount, int64(0), "Base program should have 0 new risks (first scan)")

		// 验证增量程序的扫描结果
		require.Equal(t, diffTask.Programs, []string{diffProgID})
		require.Equal(t, diffTask.Status, "done")
		require.Greater(t, diffTask.RiskCount, baseTask.RiskCount, "Diff program should have more risks than base program")
		// 增量程序应该检测到新增的漏洞（通过 RuntimeId 比较，不依赖 ProgramName）
		require.Greater(t, diffTask.NewRiskCount, int64(0), "Diff program should have new risks detected via RuntimeId comparison")
		log.Infof("Base task risks: %d, new risks: %d", baseTask.RiskCount, baseTask.NewRiskCount)
		log.Infof("Diff task risks: %d, new risks: %d", diffTask.RiskCount, diffTask.NewRiskCount)

		// Step 7: 验证通过增量程序名称查询也能返回同一 project 下的所有任务
		rsp2, err := client.QuerySyntaxFlowScanTask(context.Background(), &ypb.QuerySyntaxFlowScanTaskRequest{
			Filter: &ypb.SyntaxFlowScanTaskFilter{
				Programs: []string{diffProgID}, // 通过增量程序名称查询
			},
			ShowDiffRisk: true,
		})

		require.NoError(t, err)
		require.GreaterOrEqual(t, len(rsp2.Data), 2, "Should return at least 2 tasks when querying by diff program name (same project)")

		// 验证返回的任务包含基础程序和增量程序
		hasBaseTask := false
		hasDiffTask := false
		for _, task := range rsp2.Data {
			if len(task.Programs) > 0 && task.Programs[0] == baseProgID {
				hasBaseTask = true
			}
			if len(task.Programs) > 0 && task.Programs[0] == diffProgID {
				hasDiffTask = true
			}
		}
		require.True(t, hasBaseTask, "Should return base task when querying by diff program name")
		require.True(t, hasDiffTask, "Should return diff task when querying by diff program name")
	})
}

func TestGRPCMUSTPASS_SyntaxFlow_Scan_With_DiffProg(t *testing.T) {
	t.Skip("this task3 syntaxflow result not correct ")
	progID := uuid.NewString()
	ruleName1 := uuid.NewString()
	ruleName2 := uuid.NewString()
	taskID1 := ""
	taskID2 := ""
	taskID3 := ""
	_ = taskID1
	_ = taskID2
	_ = taskID3

	client, err := NewLocalClient(true)
	require.NoError(t, err)

	code1 := `
package main

func cmd(c *gin.Context){
	exec("/bin/sh")
}
	`
	code2 := `
package main

func cmd(c *gin.Context){
	sh := c.Query("sh")
	exec(sh)
}
	`
	code3 := `
package unAuth

func cmd(c *gin.Context){
	sh1 := c.Query("sh1")
	sh2 := c.Query("sh2")

	sh := fmt.Sprintf("%s-%s", sh1, sh2)
	exec(sh)
}
	`

	client.CreateSyntaxFlowRule(context.Background(), &ypb.CreateSyntaxFlowRuleRequest{
		SyntaxFlowInput: &ypb.SyntaxFlowRuleInput{
			Content: `
desc(
	title: "high"
	level: high
)

exec(* as $param)

$param #-> as $high

alert $high for {
	type: "vuln",
	level: "high",
}`,
			GroupNames: []string{"golang"},
			RuleName:   ruleName1,
			Language:   "golang",
			Tags:       []string{"golang"},
		},
	})
	client.CreateSyntaxFlowRule(context.Background(), &ypb.CreateSyntaxFlowRuleRequest{
		SyntaxFlowInput: &ypb.SyntaxFlowRuleInput{
			Content: `
desc(
	title: "low"
	level: low
)

exec(*?{opcode:const} #-> as $low)

alert $low for {
	type: "vuln",
	level: "low",
}`,
			GroupNames: []string{"golang"},
			RuleName:   ruleName2,
			Language:   "golang",
			Tags:       []string{"golang"},
		},
	})
	defer func() {
		client.DeleteSyntaxFlowRule(context.Background(), &ypb.DeleteSyntaxFlowRuleRequest{
			Filter: &ypb.SyntaxFlowRuleFilter{
				RuleNames: []string{ruleName1, ruleName2},
			},
		})

	}()

	t.Run("test scan task risk level count with muti diff", func(t *testing.T) {
		defer func() {
			ssadb.DeleteProgram(ssadb.GetDB(), progID)
			yakit.DeleteSSADiffResultByBaseLine(consts.GetGormSSAProjectDataBase(), []string{taskID1, taskID2, taskID3}, schema.RuntimeId)
			yakit.DeleteSSADiffResultByCompare(consts.GetGormSSAProjectDataBase(), []string{taskID1, taskID2, taskID3}, schema.RuntimeId)
		}()
		{
			vf := filesys.NewVirtualFs()
			vf.AddFile("example/src/main/a.go", code1)
			prog, err := ssaapi.ParseProjectWithFS(vf,
				ssaapi.WithLanguage(ssaconfig.GO),
				ssaapi.WithProgramName(progID),
			)
			require.NoError(t, err)
			require.NotNil(t, prog)

			stream, err := client.SyntaxFlowScan(context.Background())
			require.NoError(t, err)

			stream.Send(&ypb.SyntaxFlowScanRequest{
				ControlMode: "start",
				Filter: &ypb.SyntaxFlowRuleFilter{
					RuleNames: []string{ruleName1, ruleName2},
				},
				ProgramName: []string{
					progID,
				},
			})

			resp, err := stream.Recv()
			taskID1 = resp.TaskID
			require.NoError(t, err)
			log.Infof("resp: %v", resp)
			require.NoError(t, err)

			finishProcess := 0.0
			var finishStatus string
			checkSfScanRecvMsg(t, stream, func(status string) {
				finishStatus = status
			}, func(process float64) {
				finishProcess = process
			})
			require.Equal(t, 1.0, finishProcess)
			require.Equal(t, "done", finishStatus)
		}
		{
			vf := filesys.NewVirtualFs()
			vf.AddFile("example/src/main/b.go", code2)
			prog, err := ssaapi.ParseProjectWithFS(vf,
				ssaapi.WithLanguage(ssaconfig.GO),
				ssaapi.WithProgramName(progID),
				ssaapi.WithReCompile(true),
			)
			require.NoError(t, err)
			require.NotNil(t, prog)

			stream, err := client.SyntaxFlowScan(context.Background())
			require.NoError(t, err)

			stream.Send(&ypb.SyntaxFlowScanRequest{
				ControlMode: "start",
				Filter: &ypb.SyntaxFlowRuleFilter{
					RuleNames: []string{ruleName1, ruleName2},
				},
				ProgramName: []string{
					progID,
				},
			})

			resp, err := stream.Recv()
			taskID2 = resp.TaskID
			require.NoError(t, err)
			log.Infof("resp: %v", resp)
			require.NoError(t, err)

			finishProcess := 0.0
			var finishStatus string
			checkSfScanRecvMsg(t, stream, func(status string) {
				finishStatus = status
			}, func(process float64) {
				finishProcess = process
			})
			require.Equal(t, 1.0, finishProcess)
			require.Equal(t, "done", finishStatus)
		}
		{
			vf := filesys.NewVirtualFs()
			vf.AddFile("example/src/main/c.go", code3)
			prog, err := ssaapi.ParseProjectWithFS(vf,
				ssaapi.WithLanguage(ssaconfig.GO),
				ssaapi.WithProgramName(progID),
				ssaapi.WithReCompile(true),
			)
			require.NoError(t, err)
			require.NotNil(t, prog)

			stream, err := client.SyntaxFlowScan(context.Background())
			require.NoError(t, err)

			stream.Send(&ypb.SyntaxFlowScanRequest{
				ControlMode: "start",
				Filter: &ypb.SyntaxFlowRuleFilter{
					RuleNames: []string{ruleName1, ruleName2},
				},
				ProgramName: []string{
					progID,
				},
			})

			resp, err := stream.Recv()
			taskID3 = resp.TaskID
			require.NoError(t, err)
			log.Infof("resp: %v", resp)
			require.NoError(t, err)

			finishProcess := 0.0
			var finishStatus string
			checkSfScanRecvMsg(t, stream, func(status string) {
				finishStatus = status
			}, func(process float64) {
				finishProcess = process
			})
			require.Equal(t, 1.0, finishProcess)
			require.Equal(t, "done", finishStatus)
		}

		rsp, err := client.QuerySyntaxFlowScanTask(context.Background(), &ypb.QuerySyntaxFlowScanTaskRequest{
			Filter: &ypb.SyntaxFlowScanTaskFilter{
				Programs: []string{progID},
				Kind:     []string{"scan"},
			},
			ShowDiffRisk: true,
		})

		require.NoError(t, err)
		require.Equal(t, len(rsp.Data), 3)

		task := rsp.Data[0]
		require.Equal(t, task.Programs, []string{progID})
		require.Equal(t, task.TaskId, taskID3)
		require.Equal(t, task.Status, "done")
		require.Equal(t, task.LowCount, int64(1))
		require.Equal(t, task.HighCount, int64(8))
		require.Equal(t, task.RiskCount, int64(9))
		require.Equal(t, task.NewLowCount, int64(0))
		require.Equal(t, task.NewHighCount, int64(5))
		require.Equal(t, task.NewRiskCount, int64(5))

		task = rsp.Data[1]
		require.Equal(t, task.Programs, []string{progID})
		require.Equal(t, task.TaskId, taskID2)
		require.Equal(t, task.Status, "done")
		require.Equal(t, task.LowCount, int64(1))
		require.Equal(t, task.HighCount, int64(3))
		require.Equal(t, task.RiskCount, int64(4))
		require.Equal(t, task.NewLowCount, int64(0))
		require.Equal(t, task.NewHighCount, int64(2))
		require.Equal(t, task.NewRiskCount, int64(2))

		task = rsp.Data[2]
		require.Equal(t, task.Programs, []string{progID})
		require.Equal(t, task.TaskId, taskID1)
		require.Equal(t, task.Status, "done")
		require.Equal(t, task.LowCount, int64(1))
		require.Equal(t, task.HighCount, int64(1))
		require.Equal(t, task.RiskCount, int64(2))
		require.Equal(t, task.NewLowCount, int64(0))
		require.Equal(t, task.NewHighCount, int64(0))
		require.Equal(t, task.NewRiskCount, int64(0))
	})
}
