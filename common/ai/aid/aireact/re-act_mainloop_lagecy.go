package aireact

/*
这里是一个旧代码保存器，旧的代码已经被注释掉了，会被随时删除
*/

//// generateMainLoopPrompt generates the prompt for the main ReAct loop
//func (r *ReAct) generateMainLoopPrompt(
//	userQuery string,
//	tools []*aitool.Tool,
//	disablePlanAndExec bool,
//) string {
//	// Generate prompt for main loop
//	var enableUserInteractive = r.config.enableUserInteract
//	if enableUserInteractive && (r.currentUserInteractiveCount >= r.config.userInteractiveLimitedTimes) {
//		enableUserInteractive = false
//	}
//
//	// Use the prompt manager to generate the prompt
//	prompt, err := r.promptManager.GenerateLoopPrompt(
//		userQuery,
//		enableUserInteractive,
//		r.config.enablePlanAndExec && !disablePlanAndExec,
//		r.config.enhanceKnowledgeManager != nil && !r.config.disableEnhanceDirectlyAnswer,
//		true,
//		r.currentUserInteractiveCount,
//		r.config.userInteractiveLimitedTimes,
//		tools,
//	)
//	if err != nil {
//		// Fallback to basic prompt if template fails
//		log.Errorf("Failed to generate loop prompt from template: %v", err)
//		return fmt.Sprintf("User Query: %s\nPlease respond with a JSON object for ReAct action.", userQuery)
//	}
//	return prompt
//}

// executeMainLoop executes the main ReAct loop
//func (r *ReAct) executeMainLoop_lagecy(userQuery string) (skipTaskStatusChange bool, err error) {
//	currentTask := r.GetCurrentTask()
//
//	skipContextCancel := utils.NewAtomicBool()
//	defer func() {
//		if skipContextCancel.IsSet() {
//			return
//		}
//		currentTask.Cancel()
//	}()
//
//	// show start of iteration in timeline
//	iterationTimelineInfo := utils.NewAtomicBool()
//	openIterationRecordingOnce := new(sync.Once)
//	endIterationRecordingOnce := new(sync.Once)
//	endIterationCall := func() {
//		if iterationTimelineInfo.IsSet() {
//			endIterationRecordingOnce.Do(func() {
//				r.AddToTimeline("iteration", "======= ReAct loop finished END["+fmt.Sprint(r.currentIteration)+"] =======")
//			})
//		}
//	}
//	defer func() {
//		endIterationCall()
//	}()
//
//	// Reset iteration state for new conversation
//	r.currentIteration = 0
//	skipTaskStatusChange = false
//
//	r.AddToTimeline("USER-Original-Query", userQuery)
//LOOP:
//	for r.currentIteration < r.config.maxIterations {
//		r.SaveTimeline()
//
//		if currentTask.IsFinished() {
//			break LOOP
//		}
//
//		r.currentIteration++
//		r.EmitIteration(r.currentIteration, r.config.maxIterations)
//
//		havePlanExecuting := r.GetCurrentPlanExecutionTask() != nil
//
//		// Get available tools
//		tools, err := r.config.aiToolManager.GetEnableTools()
//		if err != nil {
//			log.Errorf("Failed to get available tools: %v", err)
//			return false, utils.Errorf("failed to get available tools: %v", err)
//		}
//		log.Infof("start to generate main loop prompt with %d tools", len(tools))
//		prompt := r.generateMainLoopPrompt(userQuery, tools, havePlanExecuting)
//		// Use aid.CallAITransaction for robust AI calling with retry and error handling
//		var action *aicommon.WaitableAction
//		var nextAction aitool.InvokeParams
//		var actionErr error
//		var writeYaklangCodeApproach string // for write_yaklang_code_approach in ActionWriteYaklangCode
//
//		// Temporarily release lock for AI transaction to prevent deadlocks
//		transactionErr := aicommon.CallAITransaction(
//			r.config, prompt, r.config.CallAI,
//			func(resp *aicommon.AIResponse) error {
//				stream := resp.GetOutputStreamReader("re-act-loop", true, r.config.Emitter)
//				subCtx, cancel := context.WithCancel(r.config.ctx)
//				defer cancel()
//				action, actionErr = aicommon.ExtractWaitableActionFromStream(
//					subCtx,
//					stream,
//					ReActActionObject,
//					[]string{},
//					[]jsonextractor.CallbackOption{
//						jsonextractor.WithRegisterMultiFieldStreamHandler([]string{
//							"plan_request_payload",
//							"blueprint_payload",
//							"tool_require_payload",
//							"write_yaklang_code_approach",
//							"human_readable_thought",
//						}, func(key string, reader io.Reader, parents []string) {
//							var output bytes.Buffer
//							outputThought := utils.NewAtomicBool()
//							pr, pw := utils.NewPipe()
//							go func() {
//								defer pw.Close()
//
//								switch key {
//								case "plan_request_payload":
//									pw.WriteString("开始任务规划：")
//								case "blueprint_payload":
//									pw.WriteString("决定调用其他AI智能应用（智能体）：")
//								case "tool_require_payload":
//									pw.WriteString("决定调用工具：")
//								case "write_yaklang_code_approach":
//									pw.WriteString("决定编写Yaklang代码：")
//								default:
//									outputThought.Set()
//								}
//								io.Copy(pw, utils.JSONStringReader(io.TeeReader(reader, &output)))
//							}()
//							r.config.Emitter.EmitStreamEvent(
//								"re-act-loop-thought",
//								time.Now(),
//								pr,
//								resp.GetTaskIndex(),
//								func() {
//									if outputThought.IsSet() {
//										r.AddToTimeline("thought", fmt.Sprintf("AI Thought:\n%v", output.String()))
//									}
//								},
//							)
//						}),
//						jsonextractor.WithRegisterFieldStreamHandler(
//							"answer_payload", func(key string, reader io.Reader, parents []string) {
//								var o bytes.Buffer
//								reader = io.TeeReader(utils.JSONStringReader(reader), &o)
//								r.config.Emitter.EmitStreamEventEx(
//									"re-act-loop-answer-payload",
//									time.Now(),
//									reader,
//									resp.GetTaskIndex(),
//									false,
//								)
//							},
//						),
//					})
//				if actionErr != nil {
//					return utils.Errorf("Failed to parse action: %v", actionErr)
//				}
//
//				nextAction = action.WaitObject("next_action")
//				actionType := nextAction.GetString("type")
//				if actionType == "" {
//					return utils.Errorf("Invalid action type: %s", actionType)
//				}
//
//				if actionType == string(ActionRequireAIBlueprintForge) {
//					blueprintName := nextAction.GetString("blueprint_payload")
//					if blueprintName == "" {
//						return utils.Error("blueprint_payload is required for ActionRequireAIBlueprintForge but empty")
//					}
//					if ret, err := r.config.aiBlueprintManager.GetAIForge(blueprintName); ret == nil || err != nil {
//						return utils.Errorf("blueprint %s does not exist", blueprintName)
//					}
//				} else if actionType == string(ActionRequireTool) {
//					toolName := nextAction.GetString("tool_require_payload")
//					if toolName == "" {
//						return utils.Error("tool_require_payload is required for ActionRequireTool but empty")
//					}
//					_, err := r.config.aiToolManager.GetToolByName(toolName)
//					if err != nil {
//						return utils.Errorf("tool[%s] does not exist try another one.", toolName)
//					}
//				} else if actionType == string(ActionWriteYaklangCode) {
//					writeYaklangCodeApproach = nextAction.GetString("write_yaklang_code_approach")
//					if writeYaklangCodeApproach == "" {
//						return utils.Error("write_yaklang_code_approach is required for ActionWriteYaklangCode but empty")
//					}
//				}
//
//				return nil
//			})
//
//		if transactionErr != nil {
//			log.Errorf("AI transaction failed (内置错误学习功能): %v", transactionErr)
//			continue
//		}
//
//		saveIterationInfoIntoTimeline := func() {
//			// allow iteration info to be added to timeline
//			r.AddToTimeline("iteration", fmt.Sprintf(
//				"======== ReAct iteration %d ========\n"+
//					"%v", r.currentIteration, action.WaitString("human_readable_thought"),
//			))
//			openIterationRecordingOnce.Do(func() {
//				iterationTimelineInfo.SetTo(true)
//			})
//		}
//
//		r.PushCumulativeSummaryHandle(func() string {
//			return action.WaitString("cumulative_summary")
//		})
//		actionType := ActionType(nextAction.GetString("type"))
//
//		switch actionType {
//		case ActionDirectlyAnswer:
//			answerPayload := nextAction.GetString("answer_payload")
//			r.EmitTextArtifact("directly_answer", answerPayload)
//			r.EmitResultAfterStream(answerPayload)
//			currentTask.SetResult(strings.TrimSpace(answerPayload))
//			r.AddToTimeline("directly_answer", fmt.Sprintf("user input: \n"+
//				"%s\n"+
//				"ai directly answer:\n"+
//				"%v",
//				utils.PrefixLines(currentTask.GetUserInput(), "  > "),
//				utils.PrefixLines(answerPayload, "  | "),
//			))
//			endIterationCall()
//			currentTask.SetStatus(aicommon.AITaskState_Completed)
//			continue
//		case ActionKnowledgeEnhanceAnswer:
//			enhanceResult, err := r.EnhanceKnowledgeAnswer(currentTask.GetContext(), userQuery)
//			if err != nil {
//				return false, err
//			}
//			satisfied, err := r.VerifyUserSatisfaction(userQuery, false, enhanceResult)
//			if err != nil {
//				endIterationCall()
//				currentTask.SetStatus(aicommon.AITaskState_Aborted)
//				continue
//			} else if satisfied {
//				r.EmitResult("** 知识增强结果已经初步满足用户需求(Knowledge enhancement results have initially met the user's needs) **")
//				r.EmitResultAfterStream(enhanceResult)
//				currentTask.SetResult(strings.TrimSpace(enhanceResult))
//				endIterationCall()
//				if err != nil {
//					r.EmitError("Failed to require directly answer after knowledge enhance: %v", err)
//					r.AddToTimeline("error", fmt.Sprintf("Failed to require directly answer after knowledge enhance: %v", err))
//				}
//				currentTask.SetStatus(aicommon.AITaskState_Completed)
//				continue
//			} else {
//				// User needs not satisfied, continue loop
//				log.Infof("User needs not fully satisfied, continuing analysis...")
//			}
//		case ActionRequireTool:
//			saveIterationInfoIntoTimeline()
//
//			toolPayload := nextAction.GetString("tool_require_payload")
//			log.Infof("Requesting tool: %s", toolPayload)
//			toolcallResult, directlyAnswerRequired, err := r.ExecuteToolRequiredAndCall(toolPayload)
//			if err != nil {
//				r.AddToTimeline("error-calling-tool", fmt.Sprintf("Failed to handle require tool[%v]: %v", toolPayload, err))
//				currentTask.SetStatus(aicommon.AITaskState_Processing)
//				log.Errorf("Failed to handle require tool: %v, retry it", err)
//				continue
//			}
//
//			var payload bytes.Buffer
//			if !directlyAnswerRequired {
//				if toolcallResult != nil && toolcallResult.Error != "" {
//					// 工具返回了错误信息
//					r.AddToTimeline("error-calling-tool", fmt.Sprintf("Tool[%v] returned error: %v", toolPayload, toolcallResult.Error))
//					currentTask.SetStatus(aicommon.AITaskState_Processing)
//					log.Errorf("Tool[%v] returned error: %v", toolPayload, toolcallResult.Error)
//					continue
//				}
//				payload.WriteString(toolcallResult.StringWithoutID())
//			} else {
//				// handle directly answer required
//				// forcely set satisfied to true
//				r.AddToTimeline(
//					"directly_answer_required",
//					"ai call-tool step is aborted due user requirement",
//				)
//				result, err := r.DirectlyAnswer(
//					userQuery+
//						"\n===========\n"+
//						"**用户要求 AI 直接回答，所以在本次回答中，不允许使用工具和其他复杂方法手段回答**", tools)
//				if err != nil {
//					endIterationCall()
//					currentTask.SetStatus(aicommon.AITaskState_Aborted)
//					log.Errorf("Failed to require directly answer: %v", err)
//					continue
//				}
//				if result == "" {
//					endIterationCall()
//					currentTask.SetStatus(aicommon.AITaskState_Aborted)
//					log.Errorf("Failed to require directly answer: %v", err)
//					continue
//				}
//				currentTask.SetResult(strings.TrimSpace(result) + " (force directly answer)")
//				endIterationCall()
//				currentTask.SetStatus(aicommon.AITaskState_Completed)
//				continue
//			}
//
//			if nextAction.GetBool("middle_step") {
//				r.EmitInfo("middle step, tool")
//				continue
//			}
//
//			// Tool executed successfully, now verify if user needs are satisfied
//			// Temporarily release the lock before calling verification to avoid deadlock
//			satisfied, err := r.VerifyUserSatisfaction(userQuery, true, toolPayload)
//			if err != nil {
//				endIterationCall()
//				currentTask.SetStatus(aicommon.AITaskState_Aborted)
//				continue
//			} else if satisfied {
//				endIterationCall()
//				currentTask.SetStatus(aicommon.AITaskState_Completed)
//				continue
//			} else {
//				// User needs not satisfied, continue loop
//				log.Infof("User needs not fully satisfied, continuing analysis...")
//			}
//		case ActionRequireAIBlueprintForge:
//			saveIterationInfoIntoTimeline()
//			forgeName := nextAction.GetString("blueprint_payload")
//			r.AddToTimeline("plan", fmt.Sprintf("ai-forge-name(blueprint): %v is requested", forgeName))
//			r.RequireAIForgeAndAsyncExecute(currentTask.GetContext(), forgeName, func(err error) {
//				currentTask.Finish(err)
//				r.SetCurrentPlanExecutionTask(nil)
//			})
//			return true, nil
//
//			//if havePlanExecuting {
//			//	r.Emitter.EmitWarning("existed plan execution task is running, cannot start a new one")
//			//	r.AddToTimeline("plan_warning", "a plan execution task is already running, cannot start a new one")
//			//	return false, utils.Errorf("a plan execution task is already running, cannot start a new one (even through ai-blueprint is requested)")
//			//}
//			//
//			//ins, forgeParams, err := r.invokeBlueprint(forgeName)
//			//if err != nil {
//			//	r.AddToTimeline("plan_error", fmt.Sprintf("failed to invoke ai-blueprint[%v]: %v", forgeName, err))
//			//	return false, utils.Errorf("failed to invoke ai-blueprint[%v]: %v", forgeName, err)
//			//}
//			//forgeName = ins.ForgeName // use the real name from schema manager
//			//
//			//r.AddToTimeline("ai-blueprint", fmt.Sprintf(
//			//	`ai-blueprint: %v is invoked with params: %v`,
//			//	forgeName, utils.ShrinkString(utils.InterfaceToString(forgeParams), 256),
//			//))
//			//
//			//r.SetCurrentPlanExecutionTask(currentTask)
//			//skipContextCancel.SetTo(true) // Plan execution will manage the context
//			//taskStarted := make(chan struct{})
//			//timelineStartPlanChan := make(chan struct{})
//			//go func() {
//			//	defer func() {
//			//		select {
//			//		case <-timelineStartPlanChan:
//			//			r.AddToTimeline("plan_execution", fmt.Sprintf("ai-blueprint: %v is finished", utils.ShrinkString(forgeName, 128)))
//			//		}
//			//		currentTask.Cancel() // Ensure the task context is cancelled after plan execution.
//			//		currentTask.SetStatus(string(TaskStatus_Completed))
//			//		r.SetCurrentPlanExecutionTask(nil)
//			//	}()
//			//	if err := r.invokePlanAndExecute(taskStarted, currentTask.GetContext(), "", ins.ForgeName, forgeParams); err != nil {
//			//		log.Errorf("Plan execution failed: %v", err)
//			//	}
//			//}()
//			//select {
//			//case <-taskStarted:
//			//	r.AddToTimeline("plan_execution", fmt.Sprintf("ai-blueprint: %v is started", forgeName))
//			//	close(timelineStartPlanChan)
//			//	log.Infof("plan execution task started")
//			//	skipTaskStatusChange = true
//			//	break LOOP
//			//}
//		case ActionRequestPlanExecution:
//			saveIterationInfoIntoTimeline()
//
//			planPayload := nextAction.GetString("plan_request_payload")
//			if havePlanExecuting {
//				r.Emitter.EmitWarning("existed plan execution task is running, cannot start a new one")
//				r.AddToTimeline("plan_warning", "a plan execution task is already running, cannot start a new one")
//				return false, utils.Errorf("a plan execution task is already running, cannot start a new one")
//			}
//			r.SetCurrentPlanExecutionTask(currentTask)
//			log.Infof("Requesting plan execution: %s, start to create p-e coordinator", planPayload)
//			skipContextCancel.SetTo(true) // Plan execution will manage the context
//
//			r.AsyncPlanAndExecute(
//				currentTask.GetContext(),
//				planPayload,
//				func(err error) {
//					currentTask.Finish(err)
//				},
//			)
//			return true, nil
//			//taskStarted := make(chan struct{})
//			//timelineStartPlanChan := make(chan struct{})
//			//go func() {
//			//	defer func() {
//			//		select {
//			//		case <-timelineStartPlanChan:
//			//			r.AddToTimeline("plan_execution", fmt.Sprintf("plan: %v is finished", utils.ShrinkString(planPayload, 128)))
//			//		}
//			//		currentTask.Cancel() // Ensure the task context is cancelled after plan execution.
//			//		currentTask.SetStatus(string(TaskStatus_Completed))
//			//		r.SetCurrentPlanExecutionTask(nil)
//			//	}()
//			//	if err := r.invokePlanAndExecute(taskStarted, currentTask.GetContext(), planPayload, "", nil); err != nil {
//			//		log.Errorf("Plan execution failed: %v", err)
//			//	}
//			//}()
//			//select {
//			//case <-taskStarted:
//			//	r.AddToTimeline("plan_execution", fmt.Sprintf("plan: %v is started", planPayload))
//			//	close(timelineStartPlanChan)
//			//	log.Infof("plan execution task started")
//			//	skipTaskStatusChange = true
//			//	break LOOP
//			//}
//		case ActionAskForClarification:
//			saveIterationInfoIntoTimeline()
//			obj := nextAction.GetObject("ask_for_clarification_payload")
//			payloads := obj.GetStringSlice("options")
//			question := obj.GetString("question")
//			suggestion := r.AskForClarification(question, payloads)
//			if suggestion == "" {
//				suggestion = "user did not provide a valid suggestion, using default 'continue' action"
//			}
//			satisfied, err := r.VerifyUserSatisfaction(userQuery, false, suggestion)
//			if err != nil {
//				endIterationCall()
//				currentTask.SetStatus(aicommon.AITaskState_Aborted)
//				continue
//			} else if satisfied {
//				endIterationCall()
//				currentTask.SetStatus(aicommon.AITaskState_Completed)
//				continue
//			} else {
//				// User needs not satisfied, continue loop
//				log.Infof("User needs not fully satisfied, continuing analysis...")
//				continue
//			}
//		case ActionWriteYaklangCode:
//			saveIterationInfoIntoTimeline()
//			filename, err := r.invokeWriteYaklangCode(currentTask, writeYaklangCodeApproach)
//			if err != nil {
//				r.AddToTimeline("error", fmt.Sprintf("Failed to invoke write yaklang code: %v", err))
//				return false, err
//			}
//			if filename != "" {
//				log.Infof("========== [WRITE YAKLANG CODE] ==========\nFile written: %v\n=========================================", filename)
//				r.AddToTimeline("write_yaklang_code", fmt.Sprintf("write yaklang code: %v", filename))
//			}
//			return false, nil
//		default:
//			r.EmitError("unknown action type: %v", actionType)
//			r.AddToTimeline("error", fmt.Sprintf("unknown action type: %v", actionType))
//		}
//	}
//	if r.currentIteration >= r.config.maxIterations {
//		r.EmitWarning("Too many iterations[%v] is reached, stopping ReAct loop, max: %v", r.currentIteration, r.config.maxIterations)
//	}
//	return skipTaskStatusChange, nil
//}

/*


// Embed template files
//
//go:embed prompts/loop/loop.txt
var loopPromptTemplate string


// GenerateLoopPrompt generates the main ReAct loop prompt using template
func (pm *PromptManager) GenerateLoopPrompt(
	userQuery string,
	allowUserInteractive, allowPlan, allowKnowledgeEnhanceAnswer, allowWriteYaklangCode bool,
	currentUserInteractiveCount,
	userInteractiveLimitedTimes int64,
	tools []*aitool.Tool,
) (string, error) {
	forges := pm.GetAvailableAIForgeBlueprints()

	// Build template data
	data := &LoopPromptData{
		AllowAskForClarification:       allowUserInteractive,
		AllowPlan:                      allowPlan,
		AllowKnowledgeEnhanceAnswer:    allowKnowledgeEnhanceAnswer,
		AllowWriteYaklangCode:          allowWriteYaklangCode,
		AskForClarificationCurrentTime: currentUserInteractiveCount,
		AskForClarificationMaxTimes:    userInteractiveLimitedTimes,
		CurrentTime:                    time.Now().Format("2006-01-02 15:04:05"),
		OSArch:                         fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
		UserQuery:                      userQuery,
		Nonce:                          utils.RandStringBytes(4),
		Language:                       pm.react.config.language,
		AIForgeList:                    forges,
		Tools:                          tools,
		ToolsCount:                     len(tools),
		TopToolsCount:                  pm.react.config.topToolsCount,
		DynamicContext:                 pm.DynamicContext(),
	}

	data.Schema = getLoopSchema(!allowUserInteractive, !allowPlan, !allowKnowledgeEnhanceAnswer, !allowWriteYaklangCode, data.AIForgeList != "")

	data.WorkingDir = pm.workdir
	if data.WorkingDir != "" {
		data.WorkingDirGlance = pm.GetGlanceWorkdir(data.WorkingDir)
	}

	// Get prioritized tools
	if len(tools) > 0 {
		data.TopTools = pm.react.getPrioritizedTools(tools, pm.react.config.topToolsCount)
		data.HasMoreTools = len(tools) > len(data.TopTools)
	}

	// Set conversation memory
	if pm.react.cumulativeSummary != "" {
		data.ConversationMemory = pm.react.cumulativeSummary
	}

	// Set timeline memory
	if pm.react.config.memory != nil {
		data.Timeline = pm.react.config.memory.Timeline()
	}

	return pm.executeTemplate("loop", loopPromptTemplate, data)
}


func getLoopSchema(
	disallowAskForClarification,
	disableKnowledgeEnhanceAnswer,
	disallowPlanAndExecution,
	disallowWriteYaklangCode bool,
	haveAIForgeList bool,
) string {
	var opts []any
	mode := []any{
		ActionDirectlyAnswer, ActionRequireTool,
	}
	if !disallowPlanAndExecution {
		mode = append(mode, ActionRequestPlanExecution)
		if haveAIForgeList {
			mode = append(mode, ActionRequireAIBlueprintForge)
		}
	}
	if !disallowAskForClarification {
		mode = append(mode, ActionAskForClarification)
	}

	if !disableKnowledgeEnhanceAnswer {
		mode = append(mode, ActionKnowledgeEnhanceAnswer)
	}

	if !disallowWriteYaklangCode {
		mode = append(mode, ActionWriteYaklangCode)
	}

	actionFields := []aitool.ToolOption{
		aitool.WithStringParam(
			"type",
			aitool.WithParam_Description("You MUST choose one of following action types. The value you select here determines which of the other fields in this 'action' object you should populate."),
			aitool.WithParam_Enum(mode...),
			aitool.WithParam_Required(true),
		),
	}
	actionFields = append(
		actionFields,
		aitool.WithStringParam("answer_payload", aitool.WithParam_Description("USE THIS FIELD ONLY IF type is 'directly_answer'. Provide the final, complete answer for the user here. The content should be self-contained and ready to be displayed.")),
		aitool.WithStringParam("tool_require_payload", aitool.WithParam_Description("USE THIS FIELD ONLY IF type is 'require_tool'. Provide the exact name of the tool you need to use (e.g., 'web_search', 'database_query'). Another system will handle the parameter generation based on this name. Do NOT include tool arguments here.")),
		aitool.WithBoolParam(
			"middle_step",
			aitool.WithParam_Description("CRUCIAL for multi-tool tasks. Use ONLY with 'tool_require_payload'. Set to 'true' if this tool call is an intermediate step in a sequence to solve a complex task. Set to 'false' if this is the FINAL tool call needed before you can provide the complete answer. Before starting, you should outline your multi-step plan in 'cumulative_summary'."),
			aitool.WithParam_Required(true),
		),
	)
	if !disallowPlanAndExecution {
		actionFields = append(
			actionFields,
			aitool.WithStringParam(
				"plan_request_payload",
				aitool.WithParam_Description(
					"USE THIS FIELD ONLY IF type is 'request_plan_and_execution'. Provide a one-sentence summary of the complex task that needs a multi-step plan. This summary will trigger a more advanced planning system. Example: 'Create a marketing plan for a new product launch.'",
				),
			),
		)
		if haveAIForgeList {
			aitool.WithStringParam(
				"blueprint_payload",
				aitool.WithParam_Description(
					"USE THIS FIELD ONLY IF type is 'require_ai_blueprint'. Provide a forge blueprint ID from the available AI blueprint list to execute the task.",
				),
			)
		}
	}
	if !disallowAskForClarification {
		actionFields = append(actionFields, aitool.WithStructParam(
			"ask_for_clarification_payload",
			[]aitool.PropertyOption{
				aitool.WithParam_Description("Use this action when user's intent is ambiguous or incomplete."),
			},
			aitool.WithStringParam("question", aitool.WithParam_Required(true), aitool.WithParam_Description("A clear, concise question to ask the user for more information. This should help clarify their intent or provide necessary details.")),
			aitool.WithStringArrayParam(
				"options",
				aitool.WithParam_Description(
					`Optional additional context that may help the user understand what information is needed. This can include examples or explanations of why the clarification is necessary.`,
				),
			),
		))
	}

	if !disallowWriteYaklangCode {
		actionFields = append(actionFields, aitool.WithStringParam(
			"write_yaklang_code_approach",
			aitool.WithParam_Description("USE THIS FIELD ONLY IF type is 'write_yaklang_code' 编写Yaklang代码的具体思路和策略。应该包含：1) 代码的核心逻辑设计思路和主要执行流程；2) 需要使用的Yaklang内置库和关键函数（如poc、synscan、servicescan、crawler、等安全库）；3) 数据处理方式和结果输出策略；4) 并发控制方案（是否使用go关键字、SizedWaitGroup限制并发数等）；5) 错误处理策略（优先使用~操作符进行简洁的错误处理）；6) 用户交互设计（如cli参数接收、进度显示等）。重点阐述如何充分利用Yaklang的安全特性、语法糖和内置能力来高效实现目标功能，确保代码既简洁又强大。"),
		))
	}

	opts = append(opts, aitool.WithStringParam(
		"human_readable_thought",
		aitool.WithParam_Required(false),
		aitool.WithParam_Description("[Not a must-being] Provide a brief, user-friendly status message here, explaining what you are currently doing. This will be shown to the user in real-time. Examples: 'Okay, I understand. Searching for the requested information now...', 'I need to use a tool to get the current stock price.', 'This is a complex request, I will try to execute tool step by step.' if direct-answer mode, no need to fill this field"),
	), aitool.WithStructParam(
		"next_action",
		[]aitool.PropertyOption{
			aitool.WithParam_Description("Contains the specific action the AI has decided to take. You must choose one action type and provide its corresponding payload."),
		},
		actionFields...,
	), aitool.WithStringParam(
		"cumulative_summary",
		aitool.WithParam_Required(true),
		aitool.WithParam_Description("An evolving summary of the conversation. Update this field to include key information from the current interaction that should be remembered for future responses. Include topics discussed, user preferences, important context, and relevant details. If this is the first interaction, create a new summary. If there's existing context, build upon it."),
	))
	return aitool.NewObjectSchemaWithAction(opts...)
}

*/
