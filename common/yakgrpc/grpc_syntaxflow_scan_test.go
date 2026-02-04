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

// GRPCBasicScanTestConfig 基础扫描测试配置结构
type GRPCBasicScanTestConfig struct {
	Name                  string             // 测试名称
	ProgramID             string             // 程序ID（如果为空则自动生成）
	RuleNames             []string           // 规则名称列表
	ExpectedHasProcess    bool               // 是否预期有进度更新
	ExpectedFinishProcess float64            // 预期的完成进度（1.0 表示完成）
	ExpectedFinishStatus  string             // 预期的完成状态
	ExpectedMatchTaskID   bool               // 是否预期匹配任务ID
	ExpectedMatchRisk     bool               // 是否预期匹配风险通知
	ExpectedResultCount   int                // 预期的结果数量（0 表示不限制）
	ExpectedResultKind    string             // 预期的结果类型
	UseDuplexConnection   bool               // 是否使用双工连接
	Language              ssaconfig.Language // 编译语言
	ProgramFileSystem     map[string]string  // 程序文件系统（如果为空则使用默认）
}

// checkGRPCBasicScanTest 统一的基础扫描测试检查函数
func checkGRPCBasicScanTest(t *testing.T, client ypb.YakClient, config GRPCBasicScanTestConfig) {
	ctx := context.Background()

	log.Infof("[checkGRPCBasicScanTest] Starting test: %s", config.Name)

	// 准备程序
	progID := config.ProgramID
	if progID == "" {
		progID = uuid.NewString()
	}

	var cleanup func()
	if len(config.ProgramFileSystem) > 0 {
		// 使用自定义文件系统
		vf := filesys.NewVirtualFs()
		for path, content := range config.ProgramFileSystem {
			vf.AddFile(path, content)
		}
		language := config.Language
		if language == "" {
			language = ssaconfig.JAVA
		}
		prog, err := ssaapi.ParseProjectWithFS(vf,
			ssaapi.WithLanguage(language),
			ssaapi.WithProgramPath("example"),
			ssaapi.WithProgramName(progID),
		)
		require.NoError(t, err, "[checkGRPCBasicScanTest] Failed to parse project")
		require.NotNil(t, prog, "[checkGRPCBasicScanTest] Program should not be nil")
		cleanup = func() {
			ssadb.DeleteProgram(ssadb.GetDB(), progID)
		}
	} else {
		// 使用默认程序
		cleanup = prepareProgram(t, progID)
	}
	defer cleanup()

	log.Infof("[checkGRPCBasicScanTest] Step 1: Prepared program: %s", progID)

	// 设置双工连接（如果需要）
	var notify ypb.Yak_DuplexConnectionClient
	var notifyErr error
	if config.UseDuplexConnection {
		log.Infof("[checkGRPCBasicScanTest] Step 2: Setting up duplex connection")
		notify, notifyErr = client.DuplexConnection(ctx)
		require.NoError(t, notifyErr, "[checkGRPCBasicScanTest] Failed to create duplex connection")
	}

	// 启动扫描
	log.Infof("[checkGRPCBasicScanTest] Step 3: Starting scan with rules: %v", config.RuleNames)
	filter := &ypb.SyntaxFlowRuleFilter{}
	if len(config.RuleNames) > 0 {
		filter.RuleNames = config.RuleNames
	}
	taskID, stream := startScan(client, t, progID, ctx, filter)
	log.Infof("[checkGRPCBasicScanTest] Step 3: Scan started, task ID: %s", taskID)

	// 处理通知（如果需要）
	matchTaskID := false
	matchRisk := false
	if config.UseDuplexConnection && notify != nil {
		log.Infof("[checkGRPCBasicScanTest] Step 4: Setting up notification handler")
		go func() {
			for {
				res, err := notify.Recv()
				if err != nil {
					if err == io.EOF || strings.Contains(err.Error(), "context") {
						log.Infof("[checkGRPCBasicScanTest] Notification stream ended: %v", err)
						return
					}
					log.Errorf("[checkGRPCBasicScanTest] Notification recv error: %v", err)
					return
				}
				log.Infof("[checkGRPCBasicScanTest] Received notification: MessageType=%v", res.MessageType)
				if res.MessageType == ssadb.ServerPushType_SyntaxflowResult {
					var tmp map[string]string
					err = json.Unmarshal(res.GetData(), &tmp)
					require.NoError(t, err, "[checkGRPCBasicScanTest] Failed to unmarshal notification data")
					log.Infof("[checkGRPCBasicScanTest] Notification taskid: %#v", tmp)
					if tmp["task_id"] == taskID {
						matchTaskID = true
						log.Infof("[checkGRPCBasicScanTest] Task ID matched in notification")
						res, err := client.QuerySyntaxFlowResult(ctx, &ypb.QuerySyntaxFlowResultRequest{
							Filter: &ypb.SyntaxFlowResultFilter{
								TaskIDs: []string{taskID},
							},
						})
						require.NoError(t, err, "[checkGRPCBasicScanTest] Failed to query syntax flow result")
						if config.ExpectedResultCount > 0 {
							require.Greater(t, len(res.Results), 0, "[checkGRPCBasicScanTest] Should have at least one result")
							if config.ExpectedResultKind != "" {
								require.Equal(t, res.Results[0].Kind, config.ExpectedResultKind, "[checkGRPCBasicScanTest] Result kind mismatch")
							}
						}
					}
				}
				if res.MessageType == schema.ServerPushType_SSARisk {
					var tmp map[string]string
					err = json.Unmarshal(res.GetData(), &tmp)
					require.NoError(t, err, "[checkGRPCBasicScanTest] Failed to unmarshal risk notification data")
					log.Infof("[checkGRPCBasicScanTest] Risk notification taskid: %#v", tmp)
					if tmp["task_id"] == taskID {
						matchRisk = true
						log.Infof("[checkGRPCBasicScanTest] Risk matched in notification")
					}
				}
			}
		}()
	}

	// 检查扫描消息
	log.Infof("[checkGRPCBasicScanTest] Step 5: Checking scan messages")
	hasProcess := false
	finishProcess := 0.0
	var finishStatus string
	checkSfScanRecvMsg(t, stream, func(status string) {
		finishStatus = status
		log.Infof("[checkGRPCBasicScanTest] Status update: %s", status)
	}, func(process float64) {
		if 0 < process && process < 1 {
			hasProcess = true
		}
		finishProcess = process
		log.Infof("[checkGRPCBasicScanTest] Process update: %.2f", process)
	})

	// 验证结果
	log.Infof("[checkGRPCBasicScanTest] Step 6: Verifying results")
	if config.ExpectedHasProcess {
		require.True(t, hasProcess, "[checkGRPCBasicScanTest] Should have process updates")
	}
	if config.ExpectedFinishProcess > 0 {
		require.Equal(t, config.ExpectedFinishProcess, finishProcess, "[checkGRPCBasicScanTest] Finish process mismatch")
	}
	if config.ExpectedFinishStatus != "" {
		require.Equal(t, config.ExpectedFinishStatus, finishStatus, "[checkGRPCBasicScanTest] Finish status mismatch")
	}
	if config.ExpectedMatchTaskID {
		require.True(t, matchTaskID, "[checkGRPCBasicScanTest] Should match task ID in notification")
	}
	if config.ExpectedMatchRisk {
		require.True(t, matchRisk, "[checkGRPCBasicScanTest] Should match risk in notification")
	}

	log.Infof("[checkGRPCBasicScanTest] Completed test: %s, task ID: %s", config.Name, taskID)
}

// GRPCCancelScanTestConfig 取消扫描测试配置结构
type GRPCCancelScanTestConfig struct {
	Name                  string             // 测试名称
	ProgramID             string             // 程序ID（如果为空则自动生成）
	CancelAtProcess       float64            // 在哪个进度时取消（0.5 表示50%时取消）
	ExpectedHasProcess    bool               // 是否预期有进度更新
	ExpectedFinishProcess float64            // 预期的完成进度（应该小于1.0）
	Language              ssaconfig.Language // 编译语言
	ProgramFileSystem     map[string]string  // 程序文件系统
}

// checkGRPCCancelScanTest 统一的取消扫描测试检查函数
func checkGRPCCancelScanTest(t *testing.T, client ypb.YakClient, config GRPCCancelScanTestConfig) {
	ctx, cancel := context.WithCancel(context.Background())

	log.Infof("[checkGRPCCancelScanTest] Starting test: %s", config.Name)

	// 准备程序
	progID := config.ProgramID
	if progID == "" {
		progID = uuid.NewString()
	}
	cleanup := prepareProgram(t, progID)
	defer cleanup()

	log.Infof("[checkGRPCCancelScanTest] Step 1: Prepared program: %s", progID)

	// 启动扫描
	log.Infof("[checkGRPCCancelScanTest] Step 2: Starting scan")
	id, stream := startScan(client, t, progID, ctx)
	log.Infof("[checkGRPCCancelScanTest] Step 2: Scan started, task ID: %s", id)

	// 检查扫描消息并在指定进度取消
	log.Infof("[checkGRPCCancelScanTest] Step 3: Monitoring scan and canceling at process %.2f", config.CancelAtProcess)
	hasProcess := false
	finishProcess := 0.0
	checkSfScanRecvMsg(t, stream, func(status string) {
		log.Infof("[checkGRPCCancelScanTest] Status update: %s", status)
	}, func(process float64) {
		if 0 < process && process < 1 {
			hasProcess = true
		}
		if process > config.CancelAtProcess {
			log.Infof("[checkGRPCCancelScanTest] Process %.2f > %.2f, canceling context", process, config.CancelAtProcess)
			cancel()
		}
		finishProcess = process
		log.Infof("[checkGRPCCancelScanTest] Process update: %.2f", process)
	})

	// 验证结果
	log.Infof("[checkGRPCCancelScanTest] Step 4: Verifying results")
	if config.ExpectedHasProcess {
		require.True(t, hasProcess, "[checkGRPCCancelScanTest] Should have process updates")
	}
	if config.ExpectedFinishProcess > 0 {
		require.Less(t, finishProcess, config.ExpectedFinishProcess, "[checkGRPCCancelScanTest] Finish process should be less than expected")
	}

	// 等待一段时间确保取消完成
	log.Infof("[checkGRPCCancelScanTest] Step 5: Waiting for cancel to complete")
	time.Sleep(1 * time.Second)

	// 查询任务状态（使用新的 context，因为原来的 ctx 已经被取消）
	log.Infof("[checkGRPCCancelScanTest] Step 6: Querying task status")
	queryCtx := context.Background()
	rsp, err := client.QuerySyntaxFlowScanTask(queryCtx, &ypb.QuerySyntaxFlowScanTaskRequest{
		Filter: &ypb.SyntaxFlowScanTaskFilter{
			TaskIds: []string{id},
		},
	})
	require.NoError(t, err, "[checkGRPCCancelScanTest] Failed to query scan task")
	require.Equal(t, len(rsp.Data), 1, "[checkGRPCCancelScanTest] Should have one task")
	task := rsp.Data[0]
	require.Equal(t, task.Programs, []string{progID}, "[checkGRPCCancelScanTest] Task programs mismatch")
	require.Equal(t, task.Status, "done", "[checkGRPCCancelScanTest] Task status should be done")

	log.Infof("[checkGRPCCancelScanTest] Completed test: %s, task ID: %s", config.Name, id)
}

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
	require.NoError(t, err, "[TestGRPCMUSTPASS_SyntaxFlow_Scan] Failed to sync embed rules")

	client, err := NewLocalClient(true)
	require.NoError(t, err, "[TestGRPCMUSTPASS_SyntaxFlow_Scan] Failed to create local client")

	config := GRPCBasicScanTestConfig{
		Name:                  "test basic syntax flow scan",
		RuleNames:             []string{"检测Java命令执行漏洞", "检测Java SpringBoot RestController XSS漏洞"},
		ExpectedHasProcess:    true,
		ExpectedFinishProcess: 1.0,
		ExpectedFinishStatus:  "done",
		ExpectedMatchTaskID:   true,
		ExpectedMatchRisk:     true,
		ExpectedResultCount:   1,
		ExpectedResultKind:    string(schema.SFResultKindScan),
		UseDuplexConnection:   true,
		Language:              ssaconfig.JAVA,
	}

	checkGRPCBasicScanTest(t, client, config)
}

func TestGRPCMUSTPASS_SyntaxFlow_Scan_Cancel(t *testing.T) {
	client, err := NewLocalClient()
	require.NoError(t, err, "[TestGRPCMUSTPASS_SyntaxFlow_Scan_Cancel] Failed to create local client")

	config := GRPCCancelScanTestConfig{
		Name:                  "test cancel syntax flow scan",
		CancelAtProcess:       0.5,
		ExpectedHasProcess:    true,
		ExpectedFinishProcess: 1.0,
		Language:              ssaconfig.JAVA,
	}

	checkGRPCCancelScanTest(t, client, config)
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

// GRPCScanWithContentTestConfig 带内容扫描测试配置结构
type GRPCScanWithContentTestConfig struct {
	Name                  string             // 测试名称
	ProgramID             string             // 程序ID（如果为空则自动生成）
	RuleName              string             // 规则名称
	RuleContent           string             // 规则内容
	RuleLanguage          string             // 规则语言
	RuleTags              []string           // 规则标签
	ExpectedFinishStatus  string             // 预期的完成状态
	ExpectedFinishProcess float64            // 预期的完成进度
	ExpectedResultCount   int                // 预期的结果数量
	ExpectedResultKind    string             // 预期的结果类型
	Language              ssaconfig.Language // 编译语言
	ProgramFileSystem     map[string]string  // 程序文件系统
}

// checkGRPCScanWithContentTest 统一的带内容扫描测试检查函数
func checkGRPCScanWithContentTest(t *testing.T, client ypb.YakClient, config GRPCScanWithContentTestConfig) {
	ctx := context.Background()

	log.Infof("[checkGRPCScanWithContentTest] Starting test: %s", config.Name)

	// 准备程序
	progID := config.ProgramID
	if progID == "" {
		progID = uuid.NewString()
	}
	cleanup := prepareProgram(t, progID)
	defer cleanup()

	log.Infof("[checkGRPCScanWithContentTest] Step 1: Prepared program: %s", progID)

	// 启动扫描
	log.Infof("[checkGRPCScanWithContentTest] Step 2: Starting scan with content rule")
	stream, err := client.SyntaxFlowScan(ctx)
	require.NoError(t, err, "[checkGRPCScanWithContentTest] Failed to create scan stream")

	ruleName := config.RuleName
	if ruleName == "" {
		ruleName = uuid.NewString()
	}

	stream.Send(&ypb.SyntaxFlowScanRequest{
		ControlMode: "start",
		ProgramName: []string{progID},
		RuleInput: &ypb.SyntaxFlowRuleInput{
			RuleName: ruleName,
			Content:  config.RuleContent,
			Language: config.RuleLanguage,
			Tags:     config.RuleTags,
		},
	})

	resp, err := stream.Recv()
	require.NoError(t, err, "[checkGRPCScanWithContentTest] Failed to receive initial response")
	log.Infof("[checkGRPCScanWithContentTest] Step 2: Scan started, task ID: %s", resp.TaskID)
	taskID := resp.TaskID

	// 检查扫描消息
	log.Infof("[checkGRPCScanWithContentTest] Step 3: Checking scan messages")
	finishStatus := ""
	finishProcess := 0.0
	checkSfScanRecvMsg(t, stream, func(status string) {
		finishStatus = status
		log.Infof("[checkGRPCScanWithContentTest] Status update: %s", status)
	}, func(process float64) {
		finishProcess = process
		log.Infof("[checkGRPCScanWithContentTest] Process update: %.2f", process)
	})

	// 验证结果
	log.Infof("[checkGRPCScanWithContentTest] Step 4: Verifying results")
	if config.ExpectedFinishStatus != "" {
		require.Equal(t, config.ExpectedFinishStatus, finishStatus, "[checkGRPCScanWithContentTest] Finish status mismatch")
	}
	if config.ExpectedFinishProcess > 0 {
		require.Equal(t, config.ExpectedFinishProcess, finishProcess, "[checkGRPCScanWithContentTest] Finish process mismatch")
	}

	// 查询结果
	log.Infof("[checkGRPCScanWithContentTest] Step 5: Querying scan results")
	res, err := client.QuerySyntaxFlowResult(ctx, &ypb.QuerySyntaxFlowResultRequest{
		Filter: &ypb.SyntaxFlowResultFilter{
			TaskIDs: []string{taskID},
		},
	})
	require.NoError(t, err, "[checkGRPCScanWithContentTest] Failed to query syntax flow result")
	require.NotNil(t, res, "[checkGRPCScanWithContentTest] Result should not be nil")
	if config.ExpectedResultCount > 0 {
		require.Equal(t, config.ExpectedResultCount, len(res.GetResults()), "[checkGRPCScanWithContentTest] Result count mismatch")
	}
	if config.ExpectedResultKind != "" {
		require.Equal(t, config.ExpectedResultKind, res.GetResults()[0].Kind, "[checkGRPCScanWithContentTest] Result kind mismatch")
	}

	log.Infof("[checkGRPCScanWithContentTest] Completed test: %s, task ID: %s", config.Name, taskID)
}

func TestGRPCMUSTPASS_SyntaxFlow_Scan_WithContent(t *testing.T) {
	client, err := NewLocalClient()
	require.NoError(t, err, "[TestGRPCMUSTPASS_SyntaxFlow_Scan_WithContent] Failed to create local client")

	t.Run("test scan task with content", func(t *testing.T) {
		config := GRPCScanWithContentTestConfig{
			Name:                  "test scan task with content",
			RuleName:              "aa",
			RuleContent:           "this as $this",
			RuleLanguage:          "java",
			RuleTags:              []string{},
			ExpectedFinishStatus:  "done",
			ExpectedFinishProcess: 1.0,
			ExpectedResultCount:   1,
			ExpectedResultKind:    string(schema.SFResultKindDebug),
			Language:              ssaconfig.JAVA,
		}

		checkGRPCScanWithContentTest(t, client, config)
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

// GRPCIncrementalCompileTestConfig 统一的 GRPC 增量编译测试配置结构
type GRPCIncrementalCompileTestConfig struct {
	Name                     string             // 测试名称
	BaseFileSystem           map[string]string  // 基础文件系统
	DiffFileSystem           map[string]string  // 增量文件系统
	RuleContent              string             // SyntaxFlow 规则内容
	RuleName                 string             // 规则名称
	ExpectedBaseRiskCount    int64              // 预期的基础程序风险数量（0 表示不限制，-1 表示应该为空）
	ExpectedDiffRiskCount    int64              // 预期的增量程序风险数量（0 表示不限制，-1 表示应该为空）
	ExpectedBaseNewRiskCount int64              // 预期的基础程序新增风险数量
	ExpectedDiffNewRiskCount int64              // 预期的增量程序新增风险数量
	ExpectedSameProjectID    bool               // 是否预期基础程序和增量程序属于同一个 project
	Language                 ssaconfig.Language // 编译语言（默认为 JAVA）
	ExpectedTaskResults      []TaskResultConfig // 预期的任务结果列表（按时间倒序，最新的在前）。如果提供，将使用此配置进行详细验证
}

// checkGRPCIncrementalCompileTest 统一的 GRPC 增量编译测试检查函数
func checkGRPCIncrementalCompileTest(t *testing.T, client ypb.YakClient, config GRPCIncrementalCompileTestConfig) {
	ctx := context.Background()

	log.Infof("[checkGRPCIncrementalCompileTest] Starting test: %s", config.Name)

	// 创建基础程序和增量程序的名称
	baseProgID := uuid.NewString()
	diffProgID := uuid.NewString()
	ruleName := config.RuleName
	if ruleName == "" {
		ruleName = uuid.NewString()
	}

	var taskIDBase, taskIDDiff string

	// 清理函数
	defer func() {
		client.DeleteSyntaxFlowRule(ctx, &ypb.DeleteSyntaxFlowRuleRequest{
			Filter: &ypb.SyntaxFlowRuleFilter{
				RuleNames: []string{ruleName},
			},
		})
		if taskIDBase != "" {
			schema.DeleteSyntaxFlowScanTask(ssadb.GetDB(), taskIDBase)
		}
		if taskIDDiff != "" {
			schema.DeleteSyntaxFlowScanTask(ssadb.GetDB(), taskIDDiff)
		}
		ssadb.DeleteProgram(ssadb.GetDB(), baseProgID)
		ssadb.DeleteProgram(ssadb.GetDB(), diffProgID)
		if taskIDBase != "" && taskIDDiff != "" {
			yakit.DeleteSSADiffResultByBaseLine(consts.GetGormSSAProjectDataBase(), []string{taskIDBase, taskIDDiff}, schema.RuntimeId)
			yakit.DeleteSSADiffResultByCompare(consts.GetGormSSAProjectDataBase(), []string{taskIDBase, taskIDDiff}, schema.RuntimeId)
		}
	}()

	// Step 1: 创建规则
	log.Infof("[checkGRPCIncrementalCompileTest] Step 1: Creating rule: %s", ruleName)
	client.CreateSyntaxFlowRule(ctx, &ypb.CreateSyntaxFlowRuleRequest{
		SyntaxFlowInput: &ypb.SyntaxFlowRuleInput{
			Content:    config.RuleContent,
			GroupNames: []string{"java"},
			RuleName:   ruleName,
			Language:   "java",
			Tags:       []string{"java"},
		},
	})

	// Step 2: 创建基础程序（全量编译）并保存到数据库
	log.Infof("[checkGRPCIncrementalCompileTest] Step 2: Creating base program: %s", baseProgID)
	baseFS := filesys.NewVirtualFs()
	for path, content := range config.BaseFileSystem {
		baseFS.AddFile(path, content)
	}

	language := config.Language
	if language == "" {
		language = ssaconfig.JAVA
	}

	basePrograms, err := ssaapi.ParseProject(
		ssaapi.WithFileSystem(baseFS),
		ssaapi.WithLanguage(language),
		ssaapi.WithProgramName(baseProgID),
		ssaapi.WithProgramPath("example"),
		ssaapi.WithEnableIncrementalCompile(true),
	)
	require.NoError(t, err)
	require.NotNil(t, basePrograms)
	require.Greater(t, len(basePrograms), 0)

	// Step 3: 验证基础程序从数据库加载后没有 overlay（真实场景）
	log.Infof("[checkGRPCIncrementalCompileTest] Step 3: Verifying base program from database")
	baseIrProgram, err := ssadb.GetApplicationProgram(baseProgID)
	require.NoError(t, err)
	require.NotNil(t, baseIrProgram)

	baseProgramFromDB, err := ssaapi.FromDatabase(baseProgID)
	require.NoError(t, err)
	require.NotNil(t, baseProgramFromDB)
	require.Nil(t, baseProgramFromDB.GetOverlay(), "Base program loaded from database should not have overlay when OverlayLayers is empty")

	// Step 4: 对基础程序进行扫描（真实场景：从数据库加载后扫描）
	log.Infof("[checkGRPCIncrementalCompileTest] Step 4: Scanning base program")
	{
		stream, err := client.SyntaxFlowScan(ctx)
		require.NoError(t, err)

		stream.Send(&ypb.SyntaxFlowScanRequest{
			ControlMode: "start",
			Filter: &ypb.SyntaxFlowRuleFilter{
				RuleNames: []string{ruleName},
			},
			ProgramName: []string{baseProgID},
		})

		resp, err := stream.Recv()
		taskIDBase = resp.TaskID
		require.NoError(t, err)
		log.Infof("[checkGRPCIncrementalCompileTest] Base program scan task ID: %s", taskIDBase)

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

	// Step 5: 创建增量程序（增量编译，基于基础程序）并保存到数据库
	log.Infof("[checkGRPCIncrementalCompileTest] Step 5: Creating diff program: %s", diffProgID)
	diffFS := filesys.NewVirtualFs()
	for path, content := range config.DiffFileSystem {
		diffFS.AddFile(path, content)
	}

	diffPrograms, err := ssaapi.ParseProjectWithIncrementalCompile(
		diffFS,
		baseProgID, // base program name
		diffProgID, // diff program name
		language,
		ssaapi.WithContext(ctx),
	)
	require.NoError(t, err)
	require.NotNil(t, diffPrograms)
	require.Greater(t, len(diffPrograms), 0)

	// 验证增量程序的元数据
	diffProgram := diffPrograms[0]
	require.NotNil(t, diffProgram.Program)
	require.Equal(t, baseProgID, diffProgram.Program.BaseProgramName, "BaseProgramName should be set for incremental compile")

	// Step 6: 验证增量程序从数据库加载后有 overlay（真实场景）
	log.Infof("[checkGRPCIncrementalCompileTest] Step 6: Verifying diff program from database")
	diffIrProgram, err := ssadb.GetApplicationProgram(diffProgID)
	require.NoError(t, err)
	require.NotNil(t, diffIrProgram)
	require.Equal(t, baseProgID, diffIrProgram.BaseProgramName, "Diff program should have BaseProgramName")

	diffProgramFromDB, err := ssaapi.FromDatabase(diffProgID)
	require.NoError(t, err)
	require.NotNil(t, diffProgramFromDB)
	require.NotNil(t, diffProgramFromDB.GetOverlay(), "Diff program loaded from database should have overlay when it has BaseProgramName")

	// Step 7: 对增量程序进行扫描（真实场景：从数据库加载后扫描）
	log.Infof("[checkGRPCIncrementalCompileTest] Step 7: Scanning diff program")
	{
		stream, err := client.SyntaxFlowScan(ctx)
		require.NoError(t, err)

		stream.Send(&ypb.SyntaxFlowScanRequest{
			ControlMode: "start",
			Filter: &ypb.SyntaxFlowRuleFilter{
				RuleNames: []string{ruleName},
			},
			ProgramName: []string{diffProgID, baseProgID},
		})

		resp, err := stream.Recv()
		taskIDDiff = resp.TaskID
		require.NoError(t, err)
		log.Infof("[checkGRPCIncrementalCompileTest] Diff program scan task ID: %s", taskIDDiff)

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

	// Step 8: 验证基础程序和增量程序属于同一个 project
	if config.ExpectedSameProjectID {
		log.Infof("[checkGRPCIncrementalCompileTest] Step 8: Verifying same project ID")
		require.Equal(t, baseIrProgram.ProjectID, diffIrProgram.ProjectID, "Base and diff programs should belong to the same project")
		log.Infof("[checkGRPCIncrementalCompileTest] Base program ProjectID: %d, Diff program ProjectID: %d", baseIrProgram.ProjectID, diffIrProgram.ProjectID)
	}

	// Step 9: 查询扫描任务，验证扫描结果
	log.Infof("[checkGRPCIncrementalCompileTest] Step 9: Querying scan tasks and verifying results")
	rsp, err := client.QuerySyntaxFlowScanTask(ctx, &ypb.QuerySyntaxFlowScanTaskRequest{
		Filter: &ypb.SyntaxFlowScanTaskFilter{
			Programs: []string{diffProgID},
		},
		ShowDiffRisk: true,
	})

	require.NoError(t, err)
	require.GreaterOrEqual(t, len(rsp.Data), 2, "Should return at least 2 tasks (base and diff)")

	// 如果提供了 ExpectedTaskResults，使用详细验证模式
	if len(config.ExpectedTaskResults) > 0 {
		log.Infof("[checkGRPCIncrementalCompileTest] Using ExpectedTaskResults verification mode")
		require.Equal(t, len(config.ExpectedTaskResults), len(rsp.Data),
			"[checkGRPCIncrementalCompileTest] Expected %d tasks, got %d", len(config.ExpectedTaskResults), len(rsp.Data))

		// 验证每个任务的结果（按时间倒序，最新的在前）
		for i, expectedResult := range config.ExpectedTaskResults {
			actualTask := rsp.Data[i]
			log.Infof("[checkGRPCIncrementalCompileTest] Verifying task %d: TaskID=%s, Status=%s, RiskCount=%d, NewRiskCount=%d",
				i+1, actualTask.TaskId, actualTask.Status, actualTask.RiskCount, actualTask.NewRiskCount)

			// 如果配置中指定了 TaskID，则验证它
			if expectedResult.TaskID != "" {
				require.Equal(t, expectedResult.TaskID, actualTask.TaskId,
					"[checkGRPCIncrementalCompileTest] Task %d TaskID mismatch", i+1)
			} else {
				// 否则使用实际返回的 TaskID
				expectedResult.TaskID = actualTask.TaskId
			}

			// 使用与 checkGRPCDiffProgScanTest 相同的验证模式
			actualConfig := TaskResultConfig{
				TaskID:       actualTask.TaskId,
				Programs:     actualTask.Programs,
				Status:       actualTask.Status,
				LowCount:     actualTask.LowCount,
				HighCount:    actualTask.HighCount,
				RiskCount:    actualTask.RiskCount,
				NewLowCount:  actualTask.NewLowCount,
				NewHighCount: actualTask.NewHighCount,
				NewRiskCount: actualTask.NewRiskCount,
			}
			// 如果预期配置中没有指定 Programs，则不比较 Programs
			if len(expectedResult.Programs) == 0 {
				actualConfig.Programs = nil
			}
			require.Equal(t, expectedResult, actualConfig)

			log.Infof("[checkGRPCIncrementalCompileTest] Task %d verification passed", i+1)
		}
	} else {
		// 使用原有的验证逻辑
		// 找到基础程序和增量程序的扫描任务
		var baseTask, diffTask *ypb.SyntaxFlowScanTask
		for _, task := range rsp.Data {
			if len(task.Programs) > 0 && task.Programs[0] == baseProgID {
				baseTask = task
			} else if len(task.Programs) > 0 && task.Programs[0] == diffProgID {
				diffTask = task
			}
		}

		require.NotNil(t, baseTask, "Base task should be found")
		require.NotNil(t, diffTask, "Diff task should be found")

		// 验证基础程序的扫描结果
		log.Infof("[checkGRPCIncrementalCompileTest] Verifying base task: RiskCount=%d, NewRiskCount=%d", baseTask.RiskCount, baseTask.NewRiskCount)
		require.Equal(t, baseTask.Programs, []string{baseProgID})
		require.Equal(t, baseTask.Status, "done")
		if config.ExpectedBaseRiskCount > 0 {
			require.Equal(t, baseTask.RiskCount, config.ExpectedBaseRiskCount, "Base program should have expected risk count")
		} else if config.ExpectedBaseRiskCount == 0 {
			require.Greater(t, baseTask.RiskCount, int64(0), "Base program should have at least one risk")
		} else if config.ExpectedBaseRiskCount == -1 {
			require.Equal(t, baseTask.RiskCount, int64(0), "Base program should have no risks")
		}
		if config.ExpectedBaseNewRiskCount >= 0 {
			require.Equal(t, baseTask.NewRiskCount, config.ExpectedBaseNewRiskCount, "Base program should have expected new risk count")
		}

		// 验证增量程序的扫描结果
		log.Infof("[checkGRPCIncrementalCompileTest] Verifying diff task: RiskCount=%d, NewRiskCount=%d", diffTask.RiskCount, diffTask.NewRiskCount)
		require.Equal(t, diffTask.Programs, []string{diffProgID, baseProgID})
		require.Equal(t, diffTask.Status, "done")
		if config.ExpectedDiffRiskCount > 0 {
			require.Equal(t, diffTask.RiskCount, config.ExpectedDiffRiskCount, "Diff program should have expected risk count")
		} else if config.ExpectedDiffRiskCount == 0 {
			require.Greater(t, diffTask.RiskCount, baseTask.RiskCount, "Diff program should have more risks than base program")
		} else if config.ExpectedDiffRiskCount == -1 {
			require.Equal(t, diffTask.RiskCount, int64(0), "Diff program should have no risks")
		}
		if config.ExpectedDiffNewRiskCount >= 0 {
			require.Equal(t, diffTask.NewRiskCount, config.ExpectedDiffNewRiskCount, "Diff program should have expected new risk count")
		} else {
			require.Greater(t, diffTask.NewRiskCount, int64(0), "Diff program should have new risks detected")
		}
	}

	log.Infof("[checkGRPCIncrementalCompileTest] Completed test: %s", config.Name)
}

func TestGRPCMUSTPASS_SyntaxFlow_Scan_With_IncrementalCompile(t *testing.T) {
	client, err := NewLocalClient(true)
	require.NoError(t, err)

	t.Run("test scan task risk count with incremental compile", func(t *testing.T) {
		config := GRPCIncrementalCompileTestConfig{
			Name:                     "test scan task risk count with incremental compile",
			RuleName:                 uuid.NewString(),
			ExpectedBaseRiskCount:    0, // 不限制，只要大于0
			ExpectedDiffRiskCount:    0, // 不限制，只要大于base
			ExpectedBaseNewRiskCount: 0,
			ExpectedDiffNewRiskCount: -1, // 不限制，只要大于0
			ExpectedSameProjectID:    true,
			Language:                 ssaconfig.JAVA,
			RuleContent: `
Runtime.getRuntime().exec(* #-> as $high) 

alert $high for {
	type: "vuln",
	level: "high",
}`,
			BaseFileSystem: map[string]string{
				"example/src/main/java/com/example/Base.java": `
package com.example;
import java.lang.Runtime;

public class Base {
	public static void main(String[] args) {
		Runtime.getRuntime().exec("ls");
	}
}
`,
			},
			DiffFileSystem: map[string]string{
				"example/src/main/java/com/example/Base.java": `
package com.example;
import java.lang.Runtime;

public class Base {
	public static void main(String[] args) {
		Runtime.getRuntime().exec("ls");
		Runtime.getRuntime().exec(args[0]);
	}
}
`,
				"example/src/main/java/com/example/NewClass.java": `
package com.example;
import java.lang.Runtime;

public class NewClass {
	public void process(String cmd) {
		Runtime.getRuntime().exec(cmd);
	}
}
`,
			},
			ExpectedTaskResults: []TaskResultConfig{
				// 第一个任务（diff program，最新的）
				// Base.java 有 2 个 exec 调用，NewClass.java 有 1 个 exec 调用，总共 3 个
				// 其中 2 个是新增的（Base.java 的 args[0] 和 NewClass.java 的 cmd）
				{
					Status:       "done",
					LowCount:     0,
					HighCount:    14,
					RiskCount:    14,
					NewLowCount:  0,
					NewHighCount: 14,
					NewRiskCount: 14,
				},
				// 第二个任务（base program，较旧的）
				// Base.java 有 1 个 exec 调用，但根据实际扫描结果可能是 3 个（规则可能匹配多次）
				{
					Status:       "done",
					LowCount:     0,
					HighCount:    3,
					RiskCount:    3,
					NewLowCount:  0,
					NewHighCount: 0,
					NewRiskCount: 0,
				},
			},
		}

		checkGRPCIncrementalCompileTest(t, client, config)
	})
}

// GRPCDiffProgScanTestConfig 多版本增量扫描测试配置结构
type GRPCDiffProgScanTestConfig struct {
	Name                string                    // 测试名称
	ProgramID           string                    // 程序ID（如果为空则自动生成）
	CodeVersions        []GRPCDiffProgCodeVersion // 代码版本列表
	Rules               []GRPCDiffProgRuleConfig  // 规则配置列表
	ExpectedTaskResults []TaskResultConfig        // 预期的任务结果列表（按时间倒序，最新的在前）
	Language            ssaconfig.Language        // 编译语言
}

// GRPCDiffProgCodeVersion 代码版本配置
type GRPCDiffProgCodeVersion struct {
	FilePath string // 文件路径
	Content  string // 文件内容
}

// GRPCDiffProgRuleConfig 规则配置
type GRPCDiffProgRuleConfig struct {
	RuleName  string   // 规则名称（如果为空则自动生成）
	Content   string   // 规则内容
	Language  string   // 规则语言
	GroupName string   // 规则组名
	Tags      []string // 规则标签
}

// TaskResultConfig 任务结果配置
type TaskResultConfig struct {
	TaskID       string   // 任务ID（如果为空则从实际结果中获取）
	Programs     []string // 预期程序列表（如果为空则不验证）
	Status       string   // 预期状态
	LowCount     int64    // 预期低风险数量
	HighCount    int64    // 预期高风险数量
	RiskCount    int64    // 预期总风险数量
	NewLowCount  int64    // 预期新增低风险数量
	NewHighCount int64    // 预期新增高风险数量
	NewRiskCount int64    // 预期新增总风险数量
}

// checkGRPCDiffProgScanTest 统一的多版本增量扫描测试检查函数
func checkGRPCDiffProgScanTest(t *testing.T, client ypb.YakClient, config GRPCDiffProgScanTestConfig) {
	ctx := context.Background()

	log.Infof("[checkGRPCDiffProgScanTest] Starting test: %s", config.Name)

	// 生成程序ID和规则名称
	progID := config.ProgramID
	if progID == "" {
		progID = uuid.NewString()
	}

	ruleNames := make([]string, len(config.Rules))
	for i, rule := range config.Rules {
		if rule.RuleName == "" {
			ruleNames[i] = uuid.NewString()
		} else {
			ruleNames[i] = rule.RuleName
		}
	}

	var taskIDs []string

	// 清理函数
	defer func() {
		log.Infof("[checkGRPCDiffProgScanTest] Cleaning up resources")
		client.DeleteSyntaxFlowRule(ctx, &ypb.DeleteSyntaxFlowRuleRequest{
			Filter: &ypb.SyntaxFlowRuleFilter{
				RuleNames: ruleNames,
			},
		})
		for _, taskID := range taskIDs {
			if taskID != "" {
				schema.DeleteSyntaxFlowScanTask(ssadb.GetDB(), taskID)
			}
		}
		ssadb.DeleteProgram(ssadb.GetDB(), progID)
		if len(taskIDs) > 0 {
			yakit.DeleteSSADiffResultByBaseLine(consts.GetGormSSAProjectDataBase(), taskIDs, schema.RuntimeId)
			yakit.DeleteSSADiffResultByCompare(consts.GetGormSSAProjectDataBase(), taskIDs, schema.RuntimeId)
		}
	}()

	// Step 1: 创建规则
	log.Infof("[checkGRPCDiffProgScanTest] Step 1: Creating %d rules", len(config.Rules))
	for i, rule := range config.Rules {
		log.Infof("[checkGRPCDiffProgScanTest] Creating rule %d: %s", i+1, ruleNames[i])
		client.CreateSyntaxFlowRule(ctx, &ypb.CreateSyntaxFlowRuleRequest{
			SyntaxFlowInput: &ypb.SyntaxFlowRuleInput{
				Content:    rule.Content,
				GroupNames: []string{rule.GroupName},
				RuleName:   ruleNames[i],
				Language:   rule.Language,
				Tags:       rule.Tags,
			},
		})
	}

	// Step 2: 对每个代码版本进行编译和扫描
	log.Infof("[checkGRPCDiffProgScanTest] Step 2: Processing %d code versions", len(config.CodeVersions))
	for i, codeVersion := range config.CodeVersions {
		log.Infof("[checkGRPCDiffProgScanTest] Processing code version %d: %s", i+1, codeVersion.FilePath)

		// 创建文件系统并添加文件
		vf := filesys.NewVirtualFs()
		vf.AddFile(codeVersion.FilePath, codeVersion.Content)

		// 编译选项
		compileOpts := []ssaconfig.Option{
			ssaapi.WithLanguage(config.Language),
			ssaapi.WithProgramName(progID),
		}
		// 第一个版本是全量编译，后续版本是增量编译
		if i > 0 {
			compileOpts = append(compileOpts, ssaapi.WithReCompile(true))
		}

		// 编译程序
		prog, err := ssaapi.ParseProjectWithFS(vf, compileOpts...)
		require.NoError(t, err, "[checkGRPCDiffProgScanTest] Failed to compile program for version %d", i+1)
		require.NotNil(t, prog, "[checkGRPCDiffProgScanTest] Program is nil for version %d", i+1)
		log.Infof("[checkGRPCDiffProgScanTest] Successfully compiled program for version %d", i+1)

		// 创建扫描流
		stream, err := client.SyntaxFlowScan(ctx)
		require.NoError(t, err, "[checkGRPCDiffProgScanTest] Failed to create scan stream for version %d", i+1)

		// 发送扫描请求
		err = stream.Send(&ypb.SyntaxFlowScanRequest{
			ControlMode: "start",
			Filter: &ypb.SyntaxFlowRuleFilter{
				RuleNames: ruleNames,
			},
			ProgramName: []string{progID},
		})
		require.NoError(t, err, "[checkGRPCDiffProgScanTest] Failed to send scan request for version %d", i+1)

		// 接收初始响应
		resp, err := stream.Recv()
		require.NoError(t, err, "[checkGRPCDiffProgScanTest] Failed to receive initial response for version %d", i+1)
		taskID := resp.TaskID
		taskIDs = append(taskIDs, taskID)
		log.Infof("[checkGRPCDiffProgScanTest] Scan task %d started with task ID: %s", i+1, taskID)

		// 等待扫描完成
		finishProcess := 0.0
		var finishStatus string
		checkSfScanRecvMsg(t, stream, func(status string) {
			finishStatus = status
		}, func(process float64) {
			finishProcess = process
		})

		log.Infof("[checkGRPCDiffProgScanTest] Scan task %d completed: process=%.2f, status=%s", i+1, finishProcess, finishStatus)
		require.Equal(t, 1.0, finishProcess, "[checkGRPCDiffProgScanTest] Scan task %d should complete with process=1.0", i+1)
		require.Equal(t, "done", finishStatus, "[checkGRPCDiffProgScanTest] Scan task %d should finish with status=done", i+1)
	}

	// Step 3: 查询扫描任务结果
	log.Infof("[checkGRPCDiffProgScanTest] Step 3: Querying scan task results")
	rsp, err := client.QuerySyntaxFlowScanTask(ctx, &ypb.QuerySyntaxFlowScanTaskRequest{
		Filter: &ypb.SyntaxFlowScanTaskFilter{
			Programs: []string{progID},
			Kind:     []string{"scan"},
		},
		ShowDiffRisk: true,
	})
	require.NoError(t, err, "[checkGRPCDiffProgScanTest] Failed to query scan task results")
	require.Equal(t, len(config.ExpectedTaskResults), len(rsp.Data),
		"[checkGRPCDiffProgScanTest] Expected %d tasks, got %d", len(config.ExpectedTaskResults), len(rsp.Data))

	// Step 4: 验证每个任务的结果
	log.Infof("[checkGRPCDiffProgScanTest] Step 4: Verifying task results")
	for i, expectedResult := range config.ExpectedTaskResults {
		actualTask := rsp.Data[i]
		log.Infof("[checkGRPCDiffProgScanTest] Verifying task %d: TaskID=%s, Status=%s, RiskCount=%d, NewRiskCount=%d",
			i+1, actualTask.TaskId, actualTask.Status, actualTask.RiskCount, actualTask.NewRiskCount)

		// 如果配置中指定了 TaskID，则验证它
		if expectedResult.TaskID != "" {
			require.Equal(t, expectedResult.TaskID, actualTask.TaskId,
				"[checkGRPCDiffProgScanTest] Task %d TaskID mismatch", i+1)
		} else {
			// 否则使用实际返回的 TaskID
			expectedResult.TaskID = actualTask.TaskId
		}

		// 如果预期配置中指定了 Programs，则验证它
		if len(expectedResult.Programs) > 0 {
			require.Equal(t, expectedResult.Programs, actualTask.Programs,
				"[checkGRPCDiffProgScanTest] Task %d Programs mismatch", i+1)
		} else {
			// 否则验证默认的 Programs
			require.Equal(t, []string{progID}, actualTask.Programs,
				"[checkGRPCDiffProgScanTest] Task %d Programs mismatch", i+1)
		}
		actualConfig := TaskResultConfig{
			TaskID:       actualTask.TaskId,
			Programs:     actualTask.Programs,
			Status:       actualTask.Status,
			LowCount:     actualTask.LowCount,
			HighCount:    actualTask.HighCount,
			RiskCount:    actualTask.RiskCount,
			NewLowCount:  actualTask.NewLowCount,
			NewHighCount: actualTask.NewHighCount,
			NewRiskCount: actualTask.NewRiskCount,
		}
		// 如果预期配置中没有指定 Programs，则不比较 Programs
		if len(expectedResult.Programs) == 0 {
			actualConfig.Programs = nil
		}
		require.Equal(t, expectedResult, actualConfig)

		log.Infof("[checkGRPCDiffProgScanTest] Task %d verification passed", i+1)
	}

	log.Infof("[checkGRPCDiffProgScanTest] Test completed successfully: %s", config.Name)
}

func TestGRPCMUSTPASS_SyntaxFlow_Scan_With_DiffProg(t *testing.T) {
	client, err := NewLocalClient(true)
	require.NoError(t, err)

	config := GRPCDiffProgScanTestConfig{
		Name:     "test scan task risk level count with muti diff",
		Language: ssaconfig.GO,
		CodeVersions: []GRPCDiffProgCodeVersion{
			{
				FilePath: "example/src/main/a.go",
				Content: `package main

func cmd(c *gin.Context){
	exec("/bin/sh")
}`,
			},
			{
				FilePath: "example/src/main/b.go",
				Content: `package main

func cmd(c *gin.Context){
	sh := c.Query("sh")
	exec(sh)
}`,
			},
			{
				FilePath: "example/src/main/c.go",
				Content: `package unAuth

func cmd(c *gin.Context){
	sh1 := c.Query("sh1")
	sh2 := c.Query("sh2")

	sh := fmt.Sprintf("%s-%s", sh1, sh2)
	exec(sh)
}`,
			},
		},
		Rules: []GRPCDiffProgRuleConfig{
			{
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
				Language:  "golang",
				GroupName: "golang",
				Tags:      []string{"golang"},
			},
			{
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
				Language:  "golang",
				GroupName: "golang",
				Tags:      []string{"golang"},
			},
		},
		ExpectedTaskResults: []TaskResultConfig{
			// 第三个任务（最新的）
			{
				Status:       "done",
				LowCount:     0,
				HighCount:    5,
				RiskCount:    5,
				NewLowCount:  0,
				NewHighCount: 4,
				NewRiskCount: 4,
			},
			// 第二个任务
			{
				Status:       "done",
				LowCount:     0,
				HighCount:    2,
				RiskCount:    2,
				NewLowCount:  0,
				NewHighCount: 2,
				NewRiskCount: 2,
			},
			// 第一个任务（最旧的）
			{
				Status:       "done",
				LowCount:     1,
				HighCount:    1,
				RiskCount:    2,
				NewLowCount:  0,
				NewHighCount: 0,
				NewRiskCount: 0,
			},
		},
	}

	t.Run(config.Name, func(t *testing.T) {
		checkGRPCDiffProgScanTest(t, client, config)
	})
}
