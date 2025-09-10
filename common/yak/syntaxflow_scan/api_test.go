package syntaxflow_scan

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func prepareTestProgram(t *testing.T, progID string) func() {
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
	prog, err := ssaapi.ParseProjectWithFS(vf,
		ssaapi.WithLanguage(consts.JAVA),
		ssaapi.WithProgramPath("example"),
		ssaapi.WithProgramName(progID),
	)
	require.NoError(t, err)
	require.NotNil(t, prog)
	return func() {
		ssadb.DeleteProgram(ssadb.GetDB(), progID)
	}
}

func TestStartScan_WithProcessCallback(t *testing.T) {
	progID := uuid.NewString()
	f := prepareTestProgram(t, progID)
	defer f()

	var finalStatus string
	var taskID string
	var finalProcess float64

	err := StartScan(context.Background(), func(result *ScanResult) error {
		finalStatus = result.Status
		taskID = result.TaskID
		log.Infof("扫描结果: TaskID=%s, Status=%s", result.TaskID, result.Status)
		return nil
	},
		WithProgramNames(progID),
		WithRuleFilter(&ypb.SyntaxFlowRuleFilter{}),
		WithProcessCallback(func(progress float64) {
			if progress > finalProcess {
				finalProcess = progress
			}
			log.Infof("总体进度更新: %.2f%%", progress*100)
		}),
	)

	require.NoError(t, err)

	// 验证进度回调被调用
	require.Greater(t, finalProcess, 0.0, "进度回调应该被调用")
	require.NotEmpty(t, taskID, "应该有任务ID")
	require.Equal(t, "done", finalStatus, "任务应该完成")

	// 最后的进度应该是1.0（100%）
	if finalProcess > 0 {
		require.Equal(t, 1.0, finalProcess, "最终进度应该是100%")
	}

	log.Infof("测试完成: 任务ID=%s, 进度回调次数=%v", taskID, finalProcess)
}

func TestStartScan_WithRuleProcessCallback(t *testing.T) {
	progID := uuid.NewString()
	f := prepareTestProgram(t, progID)
	defer f()

	ruleProgressCalls := *utils.NewSafeMapWithKey[string, float64]()
	var finalStatus string
	var taskID string

	err := StartScan(context.Background(), func(result *ScanResult) error {
		finalStatus = result.Status
		taskID = result.TaskID
		log.Infof("扫描结果: TaskID=%s, Status=%s", result.TaskID, result.Status)
		return nil
	},
		WithProgramNames(progID),
		WithRuleFilter(&ypb.SyntaxFlowRuleFilter{}),
		WithRuleProcessCallback(func(progName, ruleName string, progress float64) {
			p, _ := ruleProgressCalls.Get(ruleName)
			if progress > p {
				ruleProgressCalls.Set(ruleName, progress)
			}
			log.Infof("规则进度更新: 程序=%s, 规则=%s, 进度=%.2f%%", progName, ruleName, progress*100)
		}),
	)

	require.NoError(t, err)

	// 验证规则进度回调被调用
	require.Greater(t, ruleProgressCalls.Count(), 0, "规则进度回调应该被调用")
	require.NotEmpty(t, taskID, "应该有任务ID")
	require.Equal(t, "done", finalStatus, "任务应该完成")

	// 验证规则进度调用包含有效数据
	for _, call := range ruleProgressCalls.GetAll() {
		require.Equal(t, 1.0, call, "进度应该等于1.0")
	}

	log.Infof("测试完成: 任务ID=%s, 规则进度回调次数=%d", taskID, ruleProgressCalls.Count())
}
