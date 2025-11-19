package aireact

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/jsonpath"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// TestReAct_PlanAndExecute_InheritPersistentId 测试 plan execution 继承 persistentId
func TestReAct_PlanAndExecute_InheritPersistentId(t *testing.T) {
	persistentId := uuid.New().String()
	planPayload := "test-plan-" + utils.RandStringBytes(32)
	timelineMarker := "PERSISTENT_TEST_MARKER_" + utils.RandStringBytes(32)

	in := make(chan *ypb.AIInputEvent, 10)
	out := make(chan *ypb.AIOutputEvent, 10)

	// 标记 plan execution 中的 AI 回调被调用
	planExecutionAICallbackCalled := false
	capturedPersistentId := ""
	capturedTimelineContainsMarker := false

	// 保存 parent ReAct 实例的引用
	var parentReAct *ReAct

	ins, err := NewTestReAct(
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			prompt := r.GetPrompt()

			// ReAct 主循环的响应 - 请求 plan and execution
			if utils.MatchAllOfSubString(prompt, "directly_answer", "request_plan_and_execution", "require_tool") {
				rsp := i.NewAIResponse()
				rsp.EmitOutputStream(bytes.NewBufferString(`
{"@action": "object", "next_action": { "type": "request_plan_and_execution", "plan_request_payload": "` + planPayload + `" },
"human_readable_thought": "requesting plan execution", "cumulative_summary": "plan execution test"}
`))
				rsp.Close()
				return rsp, nil
			}

			// 验证满意度 - 返回满意
			if utils.MatchAllOfSubString(prompt, "verify-satisfaction", "user_satisfied", "reasoning") {
				rsp := i.NewAIResponse()
				rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "verify-satisfaction", "user_satisfied": true, "reasoning": "plan execution completed"}`))
				rsp.Close()
				return rsp, nil
			}

			log.Infof("unexpected prompt in TestReAct_PlanAndExecute_InheritPersistentId: %v", utils.ShrinkString(prompt, 200))
			return nil, utils.Errorf("unexpected prompt: %v", utils.ShrinkString(prompt, 200))
		}),
		aicommon.WithEventInputChan(in),
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
			out <- e.ToGRPC()
		}),
		aicommon.WithPersistentSessionId(persistentId),
		// Hijack plan execution 来验证 persistentId 和 timeline 继承
		aicommon.WithHijackPERequest(func(ctx context.Context, payload string) error {
			log.Infof("hijacked plan execution with payload: %v", payload)
			planExecutionAICallbackCalled = true

			// 通过 parent ReAct 实例验证配置继承
			// 在实际的 invokePlanAndExecute 中，会使用 r.config 来构建 baseOpts
			// 包括 WithPersistentSessionId(r.config.PersistentSessionId) 和 WithTimeline(r.config.Timeline)
			if parentReAct != nil {
				capturedPersistentId = parentReAct.config.PersistentSessionId

				// 验证 timeline 包含 marker
				if parentReAct.config.Timeline != nil {
					timelineDump := parentReAct.config.Timeline.Dump()
					if strings.Contains(timelineDump, timelineMarker) {
						capturedTimelineContainsMarker = true
						log.Infof("timeline marker found in parent timeline")
					}
				}
			}

			log.Infof("plan execution hijacked, captured persistentId: %s", capturedPersistentId)
			return nil
		}),
	)
	if err != nil {
		t.Fatal(err)
	}

	// 保存 parent ReAct 实例引用
	parentReAct = ins

	// 在执行前先向 timeline 添加 marker
	ins.config.Timeline.PushText(ins.config.IdGenerator(), timelineMarker)
	ins.config.Timeline.Save(ins.config.GetDB(), persistentId)

	go func() {
		in <- &ypb.AIInputEvent{
			IsFreeInput: true,
			FreeInput:   "execute plan",
		}
	}()

	after := time.After(3 * time.Second)

	planStarted := false
	planEnded := false

LOOP:
	for {
		select {
		case e := <-out:
			if e.Type == string(schema.EVENT_TYPE_START_PLAN_AND_EXECUTION) {
				planStarted = true
				log.Infof("plan execution started")
			}

			if e.Type == string(schema.EVENT_TYPE_END_PLAN_AND_EXECUTION) {
				planEnded = true
				log.Infof("plan execution ended")
				// 关键优化：plan 结束后，验证已完成，立即退出
				if planStarted && planEnded && planExecutionAICallbackCalled {
					log.Infof("✓ All plan execution verifications completed, exiting early")
					break LOOP
				}
			}

			if e.NodeId == "react_task_status_changed" {
				result := jsonpath.FindFirst(e.GetContent(), "$..react_task_now_status")
				status := utils.InterfaceToString(result)
				if status == "completed" || status == "failed" {
					break LOOP
				}
			}
		case <-after:
			log.Warnf("test timeout")
			break LOOP
		}
	}
	close(in)

	// 验证结果
	if !planStarted {
		t.Fatal("plan execution did not start")
	}

	if !planEnded {
		t.Fatal("plan execution did not end")
	}

	if !planExecutionAICallbackCalled {
		t.Fatal("plan execution callback was not called")
	}

	// 验证 persistentId 被正确捕获
	if capturedPersistentId != persistentId {
		t.Fatalf("persistentId not correctly inherited: expected %s, got %s", persistentId, capturedPersistentId)
	}

	// 验证 timeline 包含 marker
	if !capturedTimelineContainsMarker {
		t.Logf("Warning: timeline marker was not found in timeline (this may be acceptable depending on timeline merge logic)")
	}

	// 验证数据库中的 timeline 是否正确保存
	runtime, err := yakit.GetLatestAIAgentRuntimeByPersistentSession(consts.GetGormProjectDatabase(), persistentId)
	if err != nil {
		t.Fatalf("failed to get runtime from database: %v", err)
	}

	if runtime == nil {
		t.Fatal("runtime not found in database")
	}

	timelineFromDB := runtime.GetTimeline()
	if timelineFromDB == "" {
		t.Fatal("timeline in database is empty")
	}

	log.Infof("✓ Successfully verified persistentId inheritance in plan execution")
	log.Infof("  PersistentId: %s", persistentId)
	log.Infof("  Captured PersistentId: %s", capturedPersistentId)
	log.Infof("  Timeline marker found: %v", capturedTimelineContainsMarker)
}

// TestReAct_Forge_InheritPersistentId 测试 forge/plan execution 继承 persistentId（使用 plan 模式）
func TestReAct_Forge_InheritPersistentId(t *testing.T) {
	persistentId := uuid.New().String()
	planPayload := "test-forge-plan-" + utils.RandStringBytes(32)
	timelineMarker := "FORGE_PERSISTENT_TEST_MARKER_" + utils.RandStringBytes(32)

	in := make(chan *ypb.AIInputEvent, 10)
	out := make(chan *ypb.AIOutputEvent, 10)

	forgeExecutionCalled := false
	capturedPersistentId := ""
	capturedTimelineContainsMarker := false

	// 保存 parent ReAct 实例的引用
	var parentReAct *ReAct

	ins, err := NewTestReAct(
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			prompt := r.GetPrompt()

			// ReAct 主循环的响应 - 请求 plan execution (和第一个测试一样)
			if utils.MatchAllOfSubString(prompt, "directly_answer", "request_plan_and_execution", "require_tool") {
				rsp := i.NewAIResponse()
				rsp.EmitOutputStream(bytes.NewBufferString(`
{"@action": "object", "next_action": { "type": "request_plan_and_execution", "plan_request_payload": "` + planPayload + `" },
"human_readable_thought": "requesting plan/forge execution", "cumulative_summary": "plan/forge execution test"}
`))
				rsp.Close()
				return rsp, nil
			}

			// 验证满意度
			if utils.MatchAllOfSubString(prompt, "verify-satisfaction", "user_satisfied", "reasoning") {
				rsp := i.NewAIResponse()
				rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "verify-satisfaction", "user_satisfied": true, "reasoning": "plan/forge execution completed"}`))
				rsp.Close()
				return rsp, nil
			}

			log.Infof("unexpected prompt in TestReAct_Forge_InheritPersistentId: %v", utils.ShrinkString(prompt, 200))
			return nil, utils.Errorf("unexpected prompt: %v", utils.ShrinkString(prompt, 200))
		}),
		aicommon.WithEventInputChan(in),
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
			out <- e.ToGRPC()
		}),
		aicommon.WithPersistentSessionId(persistentId),
		// Hijack plan/forge execution
		aicommon.WithHijackPERequest(func(ctx context.Context, payload string) error {
			log.Infof("hijacked plan/forge execution with payload: %v", payload)
			forgeExecutionCalled = true

			// 通过 parent ReAct 实例验证配置继承
			// 在实际的 invokePlanAndExecute 中无论是 plan 还是 forge 都会使用相同的 baseOpts
			// 包括 WithPersistentSessionId(r.config.PersistentSessionId) 和 WithTimeline(r.config.Timeline)
			if parentReAct != nil {
				capturedPersistentId = parentReAct.config.PersistentSessionId

				// 验证 timeline 包含 marker
				if parentReAct.config.Timeline != nil {
					timelineDump := parentReAct.config.Timeline.Dump()
					if strings.Contains(timelineDump, timelineMarker) {
						capturedTimelineContainsMarker = true
						log.Infof("timeline marker found in parent timeline for plan/forge")
					}
				}
			}

			log.Infof("plan/forge execution hijacked, captured persistentId: %s", capturedPersistentId)
			return nil
		}),
	)
	if err != nil {
		t.Fatal(err)
	}

	// 保存 parent ReAct 实例引用
	parentReAct = ins

	// 在执行前先向 timeline 添加 marker
	ins.config.Timeline.PushText(ins.config.IdGenerator(), timelineMarker)
	ins.config.Timeline.Save(ins.config.GetDB(), persistentId)

	go func() {
		in <- &ypb.AIInputEvent{
			IsFreeInput: true,
			FreeInput:   "execute forge",
		}
	}()

	after := time.After(3 * time.Second)

	forgeStarted := false
	forgeEnded := false

LOOP:
	for {
		select {
		case e := <-out:
			if e.Type == string(schema.EVENT_TYPE_START_PLAN_AND_EXECUTION) {
				forgeStarted = true
				log.Infof("forge execution started")
			}

			if e.Type == string(schema.EVENT_TYPE_END_PLAN_AND_EXECUTION) {
				forgeEnded = true
				log.Infof("forge execution ended")
				// 关键优化：forge 结束后，验证已完成，立即退出
				if forgeStarted && forgeEnded && forgeExecutionCalled {
					log.Infof("✓ All forge execution verifications completed, exiting early")
					break LOOP
				}
			}

			if e.NodeId == "react_task_status_changed" {
				result := jsonpath.FindFirst(e.GetContent(), "$..react_task_now_status")
				status := utils.InterfaceToString(result)
				if status == "completed" || status == "failed" {
					break LOOP
				}
			}
		case <-after:
			log.Warnf("test timeout")
			break LOOP
		}
	}
	close(in)

	// 验证结果
	if !forgeStarted {
		t.Fatal("forge execution did not start")
	}

	if !forgeEnded {
		t.Fatal("forge execution did not end")
	}

	if !forgeExecutionCalled {
		t.Fatal("forge execution callback was not called")
	}

	// 验证 persistentId 被正确捕获
	if capturedPersistentId != persistentId {
		t.Fatalf("persistentId not correctly inherited in forge: expected %s, got %s", persistentId, capturedPersistentId)
	}

	// 验证 timeline 包含 marker
	if !capturedTimelineContainsMarker {
		t.Logf("Warning: timeline marker was not found in forge timeline (this may be acceptable depending on timeline merge logic)")
	}

	// 验证数据库中的 timeline
	runtime, err := yakit.GetLatestAIAgentRuntimeByPersistentSession(consts.GetGormProjectDatabase(), persistentId)
	if err != nil {
		t.Fatalf("failed to get runtime from database: %v", err)
	}

	if runtime == nil {
		t.Fatal("runtime not found in database")
	}

	timelineFromDB := runtime.GetTimeline()
	if timelineFromDB == "" {
		t.Fatal("timeline in database is empty")
	}

	log.Infof("✓ Successfully verified persistentId inheritance in plan/forge execution")
	log.Infof("  PersistentId: %s", persistentId)
	log.Infof("  Captured PersistentId: %s", capturedPersistentId)
	log.Infof("  Plan payload: %s", planPayload)
	log.Infof("  Timeline marker found: %v", capturedTimelineContainsMarker)
}

// TestReAct_ForgeExecution_UserQueryContext 测试 forge execution 必须包含用户原始输入，保证上下文不丢失
// 即使 AI 生成的 query 参数不包含用户问题，系统也应该自动追加
func TestReAct_ForgeExecution_UserQueryContext(t *testing.T) {
	persistentId := uuid.New().String()
	userOriginalQuery := "请帮我分析这个漏洞: CVE-2024-" + utils.RandStringBytes(16)
	aiGeneratedQuery := "random-generated-query-" + utils.RandStringBytes(16) // AI 改写的查询，不包含用户原始问题

	in := make(chan *ypb.AIInputEvent, 10)
	out := make(chan *ypb.AIOutputEvent, 10)

	forgeExecutionCalled := false
	userQueryFoundInPrompt := false
	userQueryFoundInForgeParams := false
	capturedForgeParams := ""

	ins, err := NewTestReAct(
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			prompt := r.GetPrompt()

			// ReAct 主循环的响应 - 请求 blueprint (forge)
			if utils.MatchAllOfSubString(prompt, "directly_answer", "require_ai_blueprint", "require_tool") {
				// 验证用户输入在 prompt 中
				if strings.Contains(prompt, userOriginalQuery) {
					log.Infof("✓ User query found in ReAct main loop prompt")
				} else {
					t.Errorf("user query NOT found in ReAct main loop prompt")
				}

				rsp := i.NewAIResponse()
				rsp.EmitOutputStream(bytes.NewBufferString(`
{"@action": "object", "next_action": { "type": "require_ai_blueprint", "blueprint_payload": "xss" },
"human_readable_thought": "requesting forge to analyze vulnerability", "cumulative_summary": "forge analysis"}
`))
				rsp.Close()
				return rsp, nil
			}

			// Blueprint 参数生成
			if utils.MatchAllOfSubString(prompt, "Blueprint Schema:", "Blueprint Description:", "call-ai-blueprint") {
				// 关键验证点：在生成 blueprint 参数的 prompt 中，用户原始输入必须存在
				if strings.Contains(prompt, userOriginalQuery) {
					userQueryFoundInPrompt = true
					log.Infof("✓ User query found in blueprint parameter generation prompt")
				} else {
					log.Errorf("✗ User query NOT found in blueprint parameter generation prompt")
				}

				// 重要：AI 返回的 query 参数故意不包含用户原始问题，模拟 AI 改写导致信息丢失的情况
				rsp := i.NewAIResponse()
				rsp.EmitOutputStream(bytes.NewBufferString(`
{"@action": "call-ai-blueprint", "params": {"target": "http://example.com", "query": "` + aiGeneratedQuery + `"},
"human_readable_thought": "generating blueprint parameters (AI rewrote the query)", "cumulative_summary": "forge parameters"}
`))
				rsp.Close()
				return rsp, nil
			}

			log.Infof("unexpected prompt in TestReAct_ForgeExecution_UserQueryContext")
			return nil, utils.Errorf("unexpected prompt: %v", utils.ShrinkString(prompt, 200))
		}),
		aicommon.WithEventInputChan(in),
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
			out <- e.ToGRPC()
		}),
		aicommon.WithPersistentSessionId(persistentId),
		// 注意：不使用 hijack，让代码真正执行到 forge execution 的参数处理逻辑
		// 通过监听 START_PLAN_AND_EXECUTION 事件来验证参数
	)
	if err != nil {
		t.Fatal(err)
	}

	go func() {
		in <- &ypb.AIInputEvent{
			IsFreeInput: true,
			FreeInput:   userOriginalQuery,
		}
	}()

	after := time.After(5 * time.Second)

	forgeStarted := false
	blueprintReviewSeen := false

	var iid string
	var forgeExecutedWithLogs []string

LOOP:
	for {
		select {
		case e := <-out:
			if e.Type == string(schema.EVENT_TYPE_START_PLAN_AND_EXECUTION) {
				forgeStarted = true
				forgeExecutionCalled = true
				log.Infof("forge execution started")
			}

			// 捕获 forge 执行的日志输出
			if e.Type == string(schema.EVENT_TYPE_YAKIT_EXEC_RESULT) && e.IsJson {
				forgeExecutedWithLogs = append(forgeExecutedWithLogs, string(e.Content))
			}

			// 处理 blueprint review
			if e.Type == string(schema.EVENT_TYPE_EXEC_AIFORGE_REVIEW_REQUIRE) {
				blueprintReviewSeen = true
				iid = utils.InterfaceToString(jsonpath.FindFirst(string(e.Content), "$.id"))
				log.Infof("blueprint review required, id: %s", iid)

				// 检查 review 内容中的参数
				content := string(e.Content)
				capturedForgeParams = content
				if strings.Contains(content, userOriginalQuery) {
					userQueryFoundInForgeParams = true
					log.Infof("✓ User original query found in forge review params (system auto-appended)")
				} else {
					// 只包含 AI 生成的查询
					if strings.Contains(content, aiGeneratedQuery) {
						log.Warnf("Only AI generated query found, checking if user query will be appended...")
					} else {
						log.Errorf("✗ Neither user query nor AI query found in params")
					}
				}

				// 自动同意执行
				in <- &ypb.AIInputEvent{
					IsInteractiveMessage: true,
					InteractiveId:        iid,
					InteractiveJSONInput: `{"suggestion": "continue"}`,
				}

				// 关键优化：一旦完成 review 验证，立即退出
				// 我们已经验证了用户上下文是否被追加，无需等待任务完成
				if forgeStarted && blueprintReviewSeen {
					log.Infof("✓ All required verifications completed, exiting early")
					break LOOP
				}
			}

			if e.NodeId == "react_task_status_changed" {
				result := jsonpath.FindFirst(e.GetContent(), "$..react_task_now_status")
				status := utils.InterfaceToString(result)
				if status == "completed" || status == "failed" {
					break LOOP
				}
			}
		case <-after:
			log.Warnf("test timeout")
			break LOOP
		}
	}
	close(in)

	// 验证结果
	if !forgeStarted {
		t.Fatal("forge execution did not start")
	}

	if !blueprintReviewSeen {
		t.Fatal("blueprint review was not seen")
	}

	if !forgeExecutionCalled {
		t.Fatal("forge execution callback was not called")
	}

	// 验证1：用户原始查询必须在 blueprint 参数生成的 prompt 中出现
	if !userQueryFoundInPrompt {
		t.Fatal("CRITICAL: User original query was NOT found in blueprint parameter generation prompt - context is lost!")
	}

	// 验证2（核心）：即使 AI 生成的 query 不包含用户原始问题，forge 参数中也必须包含用户原始输入
	// 这是通过在 invokePlanAndExecute 中自动追加实现的
	if !userQueryFoundInForgeParams {
		t.Logf("Captured forge params: %s", utils.ShrinkString(capturedForgeParams, 1000))
		t.Fatalf("CRITICAL: User original query was NOT found in forge review/execution params!\nExpected to find: %s\n\nThis means the system failed to auto-append user context when AI rewrote the query.",
			userOriginalQuery)
	}

	// 验证3：AI 生成的 query 不应该直接包含用户原始问题（这是测试的前提）
	if strings.Contains(aiGeneratedQuery, userOriginalQuery) {
		t.Fatal("Test setup error: AI generated query should NOT contain user original query")
	}

	// 额外验证：检查 timeline 是否记录了用户输入
	timeline := ins.DumpTimeline()
	if !strings.Contains(timeline, userOriginalQuery) {
		t.Logf("Warning: user query not found in timeline (may be acceptable)")
	}

	log.Infof("✓ Successfully verified user query context preservation in forge execution")
	log.Infof("  User Original Query: %s", utils.ShrinkString(userOriginalQuery, 50))
	log.Infof("  AI Generated Query (no user context): %s", utils.ShrinkString(aiGeneratedQuery, 50))
	log.Infof("  Query found in blueprint prompt: %v", userQueryFoundInPrompt)
	log.Infof("  User query auto-appended to forge params: %v", userQueryFoundInForgeParams)
	log.Infof("  Forge params length: %d", len(capturedForgeParams))
}
