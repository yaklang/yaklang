package yakgrpc

import (
	"context"
	"io"
	"slices"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/syntaxflow/sfdb"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestGRPCMUSTPASS_SyntaxFlow_Save_And_Resume_Task(t *testing.T) {
	client, err := NewLocalClient()
	require.NoError(t, err)

	pauseTask := func(stream ypb.Yak_SyntaxFlowScanClient) {
		stream.Send(&ypb.SyntaxFlowScanRequest{
			ControlMode: "pause",
		})
	}

	startScan := func(progIds []string) (string, ypb.Yak_SyntaxFlowScanClient) {
		stream, err := client.SyntaxFlowScan(context.Background())
		require.NoError(t, err)

		stream.Send(&ypb.SyntaxFlowScanRequest{
			ControlMode: "start",
			Filter:      &ypb.SyntaxFlowRuleFilter{},
			ProgramName: progIds,
		})

		resp, err := stream.Recv()
		require.NoError(t, err)
		log.Infof("resp: %v", resp)
		taskID := resp.TaskID
		return taskID, stream
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
		_, err := ssadb.DeleteResultByTaskID(taskId)
		require.NoError(t, err)
		err = schema.DeleteSyntaxFlowScanTask(ssadb.GetDB(), taskId)
		require.NoError(t, err)
	}

	statusTask := func(taskId string) ypb.Yak_SyntaxFlowScanClient {
		stream, err := client.SyntaxFlowScan(context.Background())
		require.NoError(t, err)
		stream.Send(&ypb.SyntaxFlowScanRequest{
			ControlMode:  "status",
			ResumeTaskId: taskId,
		})
		return stream
	}
	t.Run("test start, pause, resume, status", func(t *testing.T) {
		// save prog
		vf := filesys.NewVirtualFs()
		vf.AddFile("example/src/main/java/com/example/apackage/a.java", `
		package com.example.apackage; 
		import com.example.bpackage.sub.B;
		class A {
			public static void main(String[] args) {
				B b = new B();
				target1(b.get());
				b.show(1);
			}
		}
		`)
		progID := uuid.NewString()
		prog, err := ssaapi.ParseProjectWithFS(vf,
			ssaapi.WithLanguage(consts.JAVA),
			ssaapi.WithProgramPath("example"),
			ssaapi.WithProgramName(progID),
		)
		defer func() {
			ssadb.DeleteProgram(ssadb.GetDB(), progID)
		}()
		require.NoError(t, err)
		require.NotNil(t, prog)
		//start
		taskID, stream := startScan([]string{progID})
		defer deleteTask(taskID)

		finishProcess := 0.0
		var finishStatus string

		// pause task
		checkSfScanRecvMsg(t, stream, func(status string) {
			finishStatus = status
		}, func(process float64) {
			if 0.5 < process {
				pauseTask(stream)
			}
			finishProcess = process
		})
		require.LessOrEqual(t, finishProcess, 1.0)
		require.GreaterOrEqual(t, finishProcess, 0.5)
		require.Equal(t, "paused", finishStatus)

		// status
		var havePause bool
		var processStatus float64
		statusStream := statusTask(taskID)
		checkSfScanRecvMsg(t, statusStream, func(status string) {
			if status == "paused" {
				havePause = true
			}
		}, func(process float64) {
			processStatus = process
		})
		require.True(t, havePause)
		require.Equal(t, finishProcess, processStatus)
		// resume
		resumeStream := resumeTask(taskID)
		haveExecute := false
		checkSfScanRecvMsg(t, resumeStream, func(status string) {
			if status == "executing" {
				haveExecute = true
			}
			finishStatus = status
		}, func(process float64) {
			finishProcess = process
		})
		require.True(t, haveExecute)
		require.Equal(t, "done", finishStatus)
		require.Equal(t, 1.0, finishProcess)
	})

	t.Run("test save two task and resume one of them", func(t *testing.T) {
		// save prog
		vf1 := filesys.NewVirtualFs()
		vf1.AddFile("example/src/main/java/com/example/bpackage/sub/b.java", `
		package com.example.bpackage.sub; 
		class B {
			public  int get1() {
				return 	 1;
			}
			public void show1(int a) {
				target2(a);
			}
		}
		`)

		vf2 := filesys.NewVirtualFs()
		vf2.AddFile("example/src/main/java/com/example/bpackage/sub/b.java", `
		package com.example.bpackage.sub; 
		class B {
			public  int get2() {
				return 	 1;
			}
			public void show2(int a) {
				target2(a);
			}
		}
		`)

		progID1 := uuid.NewString()
		prog1, err := ssaapi.ParseProjectWithFS(vf1,
			ssaapi.WithLanguage(consts.JAVA),
			ssaapi.WithProgramPath("example"),
			ssaapi.WithProgramName(progID1),
		)

		defer func() {
			ssadb.DeleteProgram(ssadb.GetDB(), progID1)
		}()
		require.NoError(t, err)
		require.NotNil(t, prog1)

		progID2 := uuid.NewString()
		prog2, err := ssaapi.ParseProjectWithFS(vf1,
			ssaapi.WithLanguage(consts.JAVA),
			ssaapi.WithProgramPath("example"),
			ssaapi.WithProgramName(progID2),
		)
		require.NoError(t, err)
		require.NotNil(t, prog2)
		defer func() {
			ssadb.DeleteProgram(ssadb.GetDB(), progID2)
		}()

		var taskID1, taskID2 string
		var stream1 ypb.Yak_SyntaxFlowScanClient
		{
			//start
			taskID1, stream1 = startScan([]string{progID1})
			defer deleteTask(taskID1)

			taskID2, _ = startScan([]string{progID2})
			defer deleteTask(taskID2)
		}
		{
			// pause task 1
			finishProcess := 0.0
			var finishStatus string
			checkSfScanRecvMsg(t, stream1, func(status string) {
				finishStatus = status
			}, func(process float64) {
				if 0.5 < process {
					pauseTask(stream1)
				}
				finishProcess = process
			})
			require.LessOrEqual(t, finishProcess, 1.0)
			require.GreaterOrEqual(t, finishProcess, 0.5)
			require.Equal(t, "paused", finishStatus)

			// status task 1
			var havePause bool
			var processStatus1 float64
			statusStream1 := statusTask(taskID1)
			checkSfScanRecvMsg(t, statusStream1, func(status string) {
				if status == "paused" {
					havePause = true
				}
			}, func(process float64) {
				processStatus1 = process
			})
			require.True(t, havePause)
			require.Equal(t, finishProcess, processStatus1)
		}

		{
			// status task 2
			var haveExecute bool
			statusStream2 := statusTask(taskID2)
			checkSfScanRecvMsg(t, statusStream2, func(status string) {
				if status == "executing" {
					haveExecute = true // query status when executing
				}
			}, func(process float64) {})
			require.True(t, haveExecute)

		}
		{
			// resume task 1
			finishProcess := 0.0
			var finishStatus string
			haveExecute := false
			resumeStream := resumeTask(taskID1)
			checkSfScanRecvMsg(t, resumeStream, func(status string) {
				if status == "executing" {
					haveExecute = true
				}
				finishStatus = status
			}, func(process float64) {
				finishProcess = process
			})
			require.True(t, haveExecute)
			require.Equal(t, "done", finishStatus)
			require.Equal(t, 1.0, finishProcess)
		}
	})
}

func TestGRPCMUSTPASS_SyntaxFlow_Query_And_Delete_Task(t *testing.T) {
	client, err := NewLocalClient()
	require.NoError(t, err)

	startScan := func(progIds []string) (string, ypb.Yak_SyntaxFlowScanClient) {
		stream, err := client.SyntaxFlowScan(context.Background())
		require.NoError(t, err)

		stream.Send(&ypb.SyntaxFlowScanRequest{
			ControlMode: "start",
			Filter:      &ypb.SyntaxFlowRuleFilter{},
			ProgramName: progIds,
		})

		resp, err := stream.Recv()
		require.NoError(t, err)
		log.Infof("resp: %v", resp)
		taskID := resp.TaskID
		return taskID, stream
	}

	deleteTasks := func(taskIds []string) {
		_, err := client.DeleteSyntaxFlowScanTask(context.Background(), &ypb.DeleteSyntaxFlowScanTaskRequest{
			Filter: &ypb.SyntaxFlowScanTaskFilter{
				TaskIds: taskIds,
			},
		})
		require.NoError(t, err)
	}

	queryTasks := func(taskIds []string) []*ypb.SyntaxFlowScanTask {
		rsp, err := client.QuerySyntaxFlowScanTask(context.Background(), &ypb.QuerySyntaxFlowScanTaskRequest{
			Pagination: &ypb.Paging{},
			Filter: &ypb.SyntaxFlowScanTaskFilter{
				TaskIds: taskIds,
			},
		})
		require.NoError(t, err)
		return rsp.GetData()
	}
	t.Run("test query and delete after starting a scan task", func(t *testing.T) {
		// save prog
		vf := filesys.NewVirtualFs()
		vf.AddFile("example/src/main/java/com/example/apackage/a.java", `
		package com.example.apackage; 
		import com.example.bpackage.sub.B;
		class A {
			public static void main(String[] args) {
				B b = new B();
				target1(b.get());
				b.show(1);
			}
		}
		`)
		progID := uuid.NewString()
		prog, err := ssaapi.ParseProjectWithFS(vf,
			ssaapi.WithLanguage(consts.JAVA),
			ssaapi.WithProgramPath("example"),
			ssaapi.WithProgramName(progID),
		)
		defer func() {
			ssadb.DeleteProgram(ssadb.GetDB(), progID)
		}()
		require.NoError(t, err)
		require.NotNil(t, prog)
		//start and finish task
		taskID, stream := startScan([]string{progID})
		defer deleteTasks([]string{taskID})
		for {
			_, err := stream.Recv()
			if err == io.EOF {
				break
			}
		}
		data := queryTasks([]string{taskID})
		require.Equal(t, 1, len(data))
		require.Equal(t, "done", data[0].Status)
		require.NotNil(t, data[0].Config)

		{
			// test query by program name
			rsp, err := client.QuerySyntaxFlowScanTask(context.Background(), &ypb.QuerySyntaxFlowScanTaskRequest{
				Pagination: &ypb.Paging{},
				Filter: &ypb.SyntaxFlowScanTaskFilter{
					Programs: []string{progID},
				},
			})
			require.NoError(t, err)
			data := rsp.GetData()
			require.Equal(t, 1, len(data))
			require.Equal(t, "done", data[0].Status)
		}
		{
			// query by status
			rsp, err := client.QuerySyntaxFlowScanTask(context.Background(), &ypb.QuerySyntaxFlowScanTaskRequest{
				Pagination: &ypb.Paging{},
				Filter: &ypb.SyntaxFlowScanTaskFilter{
					Status: []string{"done"},
				},
			})
			require.NoError(t, err)
			data := rsp.GetData()
			hasProgram := slices.ContainsFunc(data, func(item *ypb.SyntaxFlowScanTask) bool {
				return slices.Contains(item.Programs, progID)
			})
			require.True(t, hasProgram)
		}
		{
			// query by fuzz search keyword
			rsp, err := client.QuerySyntaxFlowScanTask(context.Background(), &ypb.QuerySyntaxFlowScanTaskRequest{
				Pagination: &ypb.Paging{},
				Filter: &ypb.SyntaxFlowScanTaskFilter{
					Keyword: progID[:len(progID)-5],
				},
			})
			require.NoError(t, err)
			data := rsp.GetData()
			require.Equal(t, 1, len(data))
			require.Equal(t, "done", data[0].Status)
		}
	})

	t.Run("test query and delete mutli tasks", func(t *testing.T) {
		taskIds := make([]string, 0)
		tasksMap := make(map[string]*SyntaxFlowScanManager)
		for i := 0; i < 10; i++ {
			taskId := uuid.NewString()
			taskIds = append(taskIds, taskId)
			task, err := createEmptySyntaxFlowTaskByID(taskId, context.Background())
			if i%3 == 1 {
				task.status = schema.SYNTAXFLOWSCAN_PAUSED // flag
			}
			require.NoError(t, err)
			task.SaveTask()
			tasksMap[taskId] = task
		}

		gotTasks := queryTasks(taskIds)
		require.Equal(t, 10, len(gotTasks))
		for _, gotTask := range gotTasks {
			if tasksMap[gotTask.TaskId].status != gotTask.Status {
				t.Errorf("task status not match")
			}
		}
	})

	startScanWithGroup := func(progIds []string, groupName string, isIgnoreLanguage bool) (string, ypb.Yak_SyntaxFlowScanClient) {
		stream, err := client.SyntaxFlowScan(context.Background())
		require.NoError(t, err)

		stream.Send(&ypb.SyntaxFlowScanRequest{
			ControlMode: "start",
			Filter: &ypb.SyntaxFlowRuleFilter{
				GroupNames: []string{groupName},
			},
			IgnoreLanguage: isIgnoreLanguage,
			ProgramName:    progIds,
		})

		resp, err := stream.Recv()
		require.NoError(t, err)
		log.Infof("resp: %v", resp)
		taskID := resp.TaskID
		return taskID, stream
	}

	t.Run("test ignore language", func(t *testing.T) {
		vf := filesys.NewVirtualFs()
		vf.AddFile("/a.java", `package com.example.apackage;`)

		languages := []string{string(consts.JAVA), string(consts.General), string(consts.PHP), string(consts.GO)}
		db := consts.GetGormProfileDatabase()
		groupName := uuid.NewString()
		_, err := sfdb.CreateGroup(db, groupName)
		require.NoError(t, err)
		t.Cleanup(func() {
			err = sfdb.DeleteGroup(db, groupName)
			require.NoError(t, err)
		})

		// create some rule with different language
		for _, language := range languages {
			rule, err := sfdb.CreateRule(&schema.SyntaxFlowRule{
				RuleName: uuid.NewString(),
				Language: language,
			})
			_, err = sfdb.BatchAddGroupsForRules(db, []string{rule.RuleName}, []string{groupName})
			require.NoError(t, err)
			t.Cleanup(func() {
				err = sfdb.DeleteRuleByRuleName(rule.RuleName)
				require.NoError(t, err)
			})
		}

		progIDA := uuid.NewString()
		progA, err := ssaapi.ParseProjectWithFS(vf,
			ssaapi.WithLanguage(consts.JAVA),
			ssaapi.WithProgramName(progIDA),
		)
		defer func() {
			ssadb.DeleteProgram(ssadb.GetDB(), progIDA)
		}()
		require.NoError(t, err)
		require.NotNil(t, progA)
		//start without ignore language
		taskIDA, stream := startScanWithGroup([]string{progIDA}, groupName, false)
		defer deleteTasks([]string{taskIDA})
		for {
			_, err := stream.Recv()
			if err == io.EOF {
				break
			}
		}
		dataA := queryTasks([]string{taskIDA})
		require.Equal(t, 1, len(dataA))
		require.Equal(t, int64(2), dataA[0].SkipQuery)

		// start without ignore language and have general language rule
		progIdB := uuid.NewString()
		progB, err := ssaapi.ParseProjectWithFS(vf,
			ssaapi.WithLanguage(consts.JAVA),
			ssaapi.WithProgramName(progIdB),
		)
		require.NoError(t, err)
		require.NotNil(t, progB)
		defer func() {
			ssadb.DeleteProgram(ssadb.GetDB(), progIdB)
		}()
		require.NoError(t, err)
		require.NotNil(t, progA)

		taskIdB, streamB := startScanWithGroup([]string{progIdB}, groupName, true)
		defer deleteTasks([]string{taskIdB})
		for {
			_, err := streamB.Recv()
			if err == io.EOF {
				break
			}
		}
		dataB := queryTasks([]string{taskIdB})
		require.Equal(t, 1, len(dataB))
		require.Equal(t, int64(0), dataB[0].SkipQuery)
	})

}

func TestGRPCMUSTPASS_SyntaxFlow_Query(t *testing.T) {

	createTask := func(t *testing.T, program []string) string {
		taskID := uuid.NewString()
		task := &schema.SyntaxFlowScanTask{
			TaskId:    taskID,
			Programs:  strings.Join(program, schema.SYNTAXFLOWSCAN_PROGRAM_SPLIT),
			RiskCount: 10,
		}
		err := schema.SaveSyntaxFlowScanTask(ssadb.GetDB(), task)
		require.NoError(t, err)
		return taskID
	}

	t.Run("test normal", func(t *testing.T) {
		taskID1 := createTask(t, nil)
		taskID2 := createTask(t, nil)

		_, resp, err := yakit.QuerySyntaxFlowScanTask(ssadb.GetDB(), &ypb.QuerySyntaxFlowScanTaskRequest{
			Filter: &ypb.SyntaxFlowScanTaskFilter{
				TaskIds: []string{taskID1, taskID2},
			},
		})
		require.NoError(t, err)
		require.Equal(t, 2, len(resp))
		require.Equal(t, taskID2, resp[0].TaskId)
		require.Equal(t, taskID1, resp[1].TaskId)
	})

	t.Run("test multiple program", func(t *testing.T) {
		prog1 := uuid.NewString()
		prog2 := uuid.NewString()

		task1 := createTask(t, []string{prog1, prog2})
		task2 := createTask(t, []string{prog1})
		task3 := createTask(t, []string{prog2})
		_ = task3

		_, resp, err := yakit.QuerySyntaxFlowScanTask(ssadb.GetDB(), &ypb.QuerySyntaxFlowScanTaskRequest{
			Filter: &ypb.SyntaxFlowScanTaskFilter{
				Programs: []string{prog1},
			},
		})
		require.NoError(t, err)
		require.Equal(t, 2, len(resp))
		require.Equal(t, task2, resp[0].TaskId)
		require.Equal(t, task1, resp[1].TaskId)
	})

	t.Run("test filter risk count", func(t *testing.T) {
		task1 := createTask(t, nil)
		task2 := createTask(t, nil)

		_, resp, err := yakit.QuerySyntaxFlowScanTask(ssadb.GetDB(), &ypb.QuerySyntaxFlowScanTaskRequest{
			Filter: &ypb.SyntaxFlowScanTaskFilter{
				TaskIds:  []string{task1, task2},
				HaveRisk: true,
			},
		})
		require.NoError(t, err)
		require.Equal(t, 0, len(resp))
	})
}
