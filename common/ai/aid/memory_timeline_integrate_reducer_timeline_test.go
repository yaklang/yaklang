package aid

//func TestCoordinator_Timeline_ToolUse_BatchCompression_Reducer(t *testing.T) {
//	t.Skip(true)
//	inputChan := chanx.NewUnlimitedChan[*ypb.AIInputEvent](context.Background(),10)
//	outputChan := make(chan *schema.AiOutputEvent)
//
//	requireMoreToolCount := 0
//
//	timelineBatchCompressTrigger := false
//	timelineBatchCompressApplyCount := 0
//
//	tokenBatchCompressed := utils.RandStringBytes(100)
//
//	coordinator, err := NewCoordinator(
//		"test",
//		aicommon.WithEventInputChan(inputChan),
//		aicommon.WithSystemFileOperator(),
//		aicommon.WithEventHandler(func(event *schema.AiOutputEvent) {
//			outputChan <- event
//		}),
//		aicommon.WithTimelineContentLimit(500), // 设置更小的内容大小限制以更快触发compression
//		aicommon.WithAICallback(func(config aicommon.AICallerConfigIf, request *aicommon.AIRequest) (*aicommon.AIResponse, error) {
//			rsp := config.NewAIResponse()
//			defer func() {
//				rsp.Close()
//			}()
//
//			if utils.MatchAllOfRegexp(request.GetPrompt(), tokenBatchCompressed) {
//				timelineBatchCompressApplyCount++
//			}
//
//			// Count continue-current-task calls
//			if strings.Contains(request.GetPrompt(), `"continue-current-task"`) {
//				requireMoreToolCount++
//			}
//
//			if utils.MatchAllOfSubString(request.GetPrompt(), `@action`, `"timeline-reducer"`) ||
//				strings.Contains(request.GetPrompt(), "批量精炼与浓缩") {
//				rsp.EmitOutputStream(strings.NewReader(`{"@action": "timeline-reducer", "reducer_memory": "` + tokenBatchCompressed + `"}`))
//				log.Info("timeline batch compress triggered")
//				timelineBatchCompressTrigger = true
//				return rsp, nil
//			}
//
//			// After multiple continue-current-task calls, skip the task to trigger compression check
//			if requireMoreToolCount >= 6 { // Reduced threshold since compression should trigger earlier
//				rsp.EmitOutputStream(strings.NewReader(`{"@action": "direct-answer", "direct_answer": "任务执行失败，无法完成目录扫描"}`))
//				return rsp, nil
//			}
//
//			fmt.Println("========================================================")
//			fmt.Println(request.GetPrompt())
//
//			if strings.Contains(request.GetPrompt(), `"continue-current-task"`) {
//				rsp.EmitOutputStream(strings.NewReader(`{"@action": "continue-current-task"}`))
//				requireMoreToolCount++
//				if requireMoreToolCount > 10 {
//					log.Info("requireMoreToolCount reached 10")
//				}
//				return rsp, nil
//			}
//
//			// AI should directly fail and give up to trigger compression
//			if strings.Contains(request.GetPrompt(), "扫描目录结构") {
//				rsp.EmitOutputStream(strings.NewReader(`{"@action": "direct-answer", "direct_answer": "无法扫描目录，无法完成任务"}`))
//				requireMoreToolCount++
//				return rsp, nil
//			}
//
//			// Always respond with continue-current-task to execution prompts that might trigger compression check
//			// But NOT for planning prompts
//			if !strings.Contains(request.GetPrompt(), "任务规划") &&
//				!strings.Contains(request.GetPrompt(), "任务设计") &&
//				(strings.Contains(request.GetPrompt(), "timeline") ||
//					strings.Contains(request.GetPrompt(), "compression") ||
//					requireMoreToolCount >= 2) {
//				rsp.EmitOutputStream(strings.NewReader(`{"@action": "continue-current-task"}`))
//				requireMoreToolCount++
//				return rsp, nil
//			}
//
//			if utils.MatchAllOfSubString(request.GetPrompt(), `工具名称: ls`, `"call-tool"`, "const") {
//				rsp.EmitOutputStream(strings.NewReader(`{"@action": "call-tool", "tool": "ls", "params": {"path": "/abc-target"}}`))
//				return rsp, nil
//			} else if utils.MatchAllOfSubString(request.GetPrompt(), `当前任务: "扫描目录结构"`) {
//				// 限制require-tool的次数，避免无限循环
//				if requireMoreToolCount < 5 {
//					rsp.EmitOutputStream(strings.NewReader(`{"@action": "require-tool", "tool": "ls"}`))
//					requireMoreToolCount++
//				} else {
//					// 超过限制次数后，改为跳过任务
//					rsp.EmitOutputStream(strings.NewReader(`{"@action": "task-skipped", "task_short_summary": "无法执行目录扫描，工具调用失败"}`))
//				}
//				return rsp, nil
//			}
//
//			fmt.Println("===========" + "request:" + "===========\n" + request.GetPrompt())
//			rsp.EmitOutputStream(strings.NewReader(`
//{
//    "@action": "plan",
//    "query": "找出 /Users/v1ll4n/Projects/yaklang 目录中最大的文件",
//    "main_task": "在给定路径下寻找体积最大的文件",
//    "main_task_goal": "识别 /Users/v1ll4n/Projects/yaklang 目录中占用存储空间最多的文件，并展示其完整路径与大小信息",
//    "tasks": [
//        {
//            "subtask_name": "扫描目录结构",
//            "subtask_goal": "递归遍历 /Users/v1ll4n/Projects/yaklang 目录下所有文件，记录每个文件的位置和占用空间"
//        },
//        {
//            "subtask_name": "计算文件大小",
//            "subtask_goal": "遍历所有文件，计算每个文件的大小"
//        }
//    ]
//}
//			`))
//			return rsp, nil
//		}),
//	)
//	if err != nil {
//		t.Fatalf("NewCoordinator failed: %v", err)
//	}
//
//	// Pre-populate timeline with content to trigger compression
//	// Add multiple items to exceed the 500 byte content limit
//	for i := 0; i < 10; i++ {
//		coordinator.config.memory.PushText(int64(i+1000), fmt.Sprintf("test content item %d with some additional text to increase size", i))
//	}
//
//	go coordinator.Run()
//
//	// Wait a short time for compression to complete
//	time.Sleep(1 * time.Second)
//
//	// Check if compression was triggered after adding content
//	if !timelineBatchCompressTrigger {
//		// Wait a bit more for compression to trigger
//		time.Sleep(2 * time.Second)
//	}
//
//	// Test passes if compression was triggered
//	if !timelineBatchCompressTrigger {
//		t.Fatal("timeline batch compress not triggered")
//	}
//
//	// Success: timeline batch compression was successfully triggered
//	// Force exit the test since we confirmed the main functionality works
//	panic("Test passed: timeline batch compression was triggered as expected")
//}
