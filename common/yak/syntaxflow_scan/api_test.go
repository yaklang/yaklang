package syntaxflow_scan

import (
	"context"
	"sync"
	"testing"
	"time"

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

	var status string
	var taskID string
	var recordProcess float64
	var lock = sync.Mutex{}

	err := StartScan(context.Background(),
		WithScanResultCallback(func(sr *ScanResult) {
			status = sr.Status
			taskID = sr.TaskID
			// log.Infof("扫描结果: TaskID=%s, Status=%s", taskID, status)
		}),
		WithProgramNames(progID),
		WithRuleFilter(&ypb.SyntaxFlowRuleFilter{}),
		WithProcessCallback(func(progress float64, info *RuleProcessInfoList) {
			lock.Lock()
			defer lock.Unlock()
			log.Infof("=%s== %.2f%%, status: %s ", time.UnixMicro(info.Time), progress*100, status)
			require.GreaterOrEqual(t, progress, recordProcess, "总体进度不应该减少")
			recordProcess = progress
		}),
	)

	require.NoError(t, err)

	// 验证进度回调被调用
	require.Greater(t, recordProcess, 0.0, "进度回调应该被调用")
	require.NotEmpty(t, taskID, "应该有任务ID")
	require.Equal(t, "done", status, "任务应该完成")

	// 最后的进度应该是1.0（100%）
	if recordProcess > 0 {
		require.Equal(t, 1.0, recordProcess, "最终进度应该是100%")
	}

	log.Infof("测试完成: 任务ID=%s, 进度回调次数=%v", taskID, recordProcess)
}

func TestStartScan_WithRuleProcessCallback(t *testing.T) {
	progID := uuid.NewString()
	f := prepareTestProgram(t, progID)
	defer f()

	ruleProgress := utils.NewSafeMapWithKey[string, float64]()
	var status string
	var taskID string

	totalProcess := 0.0

	err := StartScan(context.Background(),
		WithScanResultCallback(func(sr *ScanResult) {
			status = sr.Status
			taskID = sr.TaskID
			// log.Infof("扫描结果: TaskID=%s, Status=%s", taskID, status)
		}),
		WithProgramNames(progID),
		WithRuleFilter(&ypb.SyntaxFlowRuleFilter{}),
		WithProcessMonitorTTL(100*time.Millisecond), // 设置回调间隔
		WithProcessCallback(func(progress float64, infos *RuleProcessInfoList) {
			// log.Infof("=%s== %.2f%%, status: %s --\n%s\n", time.UnixMicro(infos.Time), progress*100, status, infos)
			log.Infof("=%s== %.2f%%, status: %s --", time.UnixMicro(infos.Time), progress*100, status)
			require.False(t, progress < totalProcess, "总体进度不应该减少")
			totalProcess = progress
			for _, info := range infos.Rules {
				if oldProgress, ok := ruleProgress.Get(info.Key()); ok {
					log.Infof("  规则进度(历史): %s %.2f%% vs %.2f%%", info.Key(), oldProgress*100, info.Progress*100)
					require.GreaterOrEqual(t, info.Progress, oldProgress, "规则进度不应该减少")
				}
				ruleProgress.Set(info.Key(), info.Progress)
			}
		}),
	)
	log.Infof("======================= 最终总体进度: %.2f%%", totalProcess*100)

	require.NoError(t, err)
	require.Equal(t, 1.0, totalProcess, "最终总体进度应该是100%")

	// 验证规则进度回调被调用
	require.Greater(t, ruleProgress.Count(), 0, "规则进度回调应该被调用")
	require.NotEmpty(t, taskID, "应该有任务ID")
	require.Equal(t, "done", status, "任务应该完成")

	// 验证规则进度调用包含有效数据
	for key, process := range ruleProgress.GetAll() {
		require.Equal(t, 1.0, process, "进度应该等于1.0 %s", key)
	}

	log.Infof("测试完成: 任务ID=%s, 规则进度回调次数=%d", taskID, ruleProgress.Count())
}
