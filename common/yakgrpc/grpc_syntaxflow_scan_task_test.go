package yakgrpc

import (
	"context"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"testing"
)

func TestGRPCMUSTPASS_SyntaxFlow_Task(t *testing.T) {
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
		err := ssadb.DeleteResultByTaskID(taskId)
		require.NoError(t, err)
		err = schema.DeleteSyntaxFlowScanTask(consts.GetGormProjectDatabase(), taskId)
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
		//start
		taskID, stream := startScan([]string{progID})
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
		checkSfScanRecvMsg(t, stream, handlerStatus, handlerProcess)
		require.LessOrEqual(t, finishProcess, 0.7)
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
		handlerAfterResumeStatus := func(status string) {
			if status == "executing" {
				haveExecute = true
			}
			finishStatus = status
		}
		handlerAfterResumeProcess := func(process float64) {
			finishProcess = process
		}
		checkSfScanRecvMsg(t, resumeStream, handlerAfterResumeStatus, handlerAfterResumeProcess)
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
		prog1, err := ssaapi.ParseProject(vf1,
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
		prog2, err := ssaapi.ParseProject(vf1,
			ssaapi.WithLanguage(consts.JAVA),
			ssaapi.WithProgramPath("example"),
			ssaapi.WithProgramName(progID2),
		)
		require.NoError(t, err)
		require.NotNil(t, prog2)
		defer func() {
			ssadb.DeleteProgram(ssadb.GetDB(), progID2)
		}()
		//start
		taskID1, stream1 := startScan([]string{progID1})
		defer deleteTask(taskID1)

		taskID2, _ := startScan([]string{progID2})
		defer deleteTask(taskID2)

		finishProcess := 0.0
		var finishStatus string

		handlerStatus := func(status string) {
			finishStatus = status
		}
		handlerProcess := func(process float64) {
			if 0.5 < process && process < 0.7 {
				pauseTask(stream1)
			}
			finishProcess = process
		}
		checkSfScanRecvMsg(t, stream1, handlerStatus, handlerProcess)
		require.LessOrEqual(t, finishProcess, 0.7)
		require.GreaterOrEqual(t, finishProcess, 0.5)
		require.Equal(t, "paused", finishStatus)
		// status
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

		var haveExecute bool
		statusStream2 := statusTask(taskID2)
		checkSfScanRecvMsg(t, statusStream2, func(status string) {
			if status == "executing" {
				haveExecute = true // query status when executing
			}
		}, func(process float64) {})
		require.True(t, haveExecute)
		// resume
		resumeStream := resumeTask(taskID1)
		haveExecute = false
		handlerAfterResumeStatus := func(status string) {
			if status == "executing" {
				haveExecute = true
			}
			finishStatus = status
		}
		handlerAfterResumeProcess := func(process float64) {
			finishProcess = process
		}
		checkSfScanRecvMsg(t, resumeStream, handlerAfterResumeStatus, handlerAfterResumeProcess)
		require.True(t, haveExecute)
		require.Equal(t, "done", finishStatus)
		require.Equal(t, 1.0, finishProcess)
	})
}
