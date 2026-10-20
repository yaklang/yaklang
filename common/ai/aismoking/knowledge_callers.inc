// Shared real execution/assertions for mock-provider and explicit live runs.
callerCase = {}
callerResults = []
func runKnowledgeCaller(focus, native, model) {
    root = file.Join(os.Getenv("AISMOKING_OUTPUT"), focus + "-" + string(native))
    file.MkdirAll(root)~
    name, decoy = "knowledge-callers-项目-"+randstr(12), "knowledge-callers-decoy-"+randstr(12)
    header, marker = "X-Project-"+randstr(12), "KB_CALLER_"+randstr(16)
    defer func(){ aiknowledge.Delete(name); aiknowledge.Delete(decoy) }()
    aiknowledge.Add(name)~
    aiknowledge.Add(decoy)~
    aiknowledge.AddEntry(name,"API 鉴权说明","本项目 API 鉴权请求头为 "+header+"，验证标记为 "+marker+"。")~
    aiknowledge.AddEntry(decoy,"API 鉴权说明","另一个项目使用 X-Decoy-Invalid 和 DECOY_MARKER。")~
    callerCase.focus,callerCase.name,callerCase.header,callerCase.marker,callerCase.root=focus,name,header,marker,root
    callerCase.stage,callerCase.operator=0,nil
    calls,events,answerParts,actions,loops,cards=[],[],[],[],{},[]
    restricted,skillSeen,schemaSeen=false,false,false
    mu=sync.NewMutex()
    question="请根据附加项目资料，查询本项目 API 的鉴权请求头和验证标记，并给出完整来源库名。只读查询，资料总预算不超过 1024 tokens，最多三条，每条不超过 256 tokens。"
    if focus=="report_generating" { question+="请生成并保存简短 Markdown 报告，正文必须包含查询到的鉴权头、验证标记和完整来源库名。" }
    started=time.Now().UnixMilli()
    options=[aim.focus(focus),aim.attachedKnowledgeBase(name),aim.attachedFileContent("CALLER_SELECTION_PREFERENCE：仅使用附加项目库的完整名称，不要扩展到别的库。"),aim.context(context.WithTimeoutSeconds(240)),aim.sessionID("knowledge-callers-"+randstr(12)),aim.workdir(root),aim.reviewPolicy("yolo"),aim.maxIteration(16),aim.timeout(210),aim.disableAIForge(true),aim.disableMCPServers(true),aim.onEvent(func(op,e){
        callerCase.operator=op
        manager=op.GetConfig().GetAiToolManager()
        if !restricted && manager!=nil { manager.RestrictToTools("manage_knowledge_base","query_knowledge_base");restricted=true }
        loop=op.GetCurrentLoop()
        mu.Lock()
        defer mu.Unlock()
        if loop!=nil {
            loops[loop.GetLoopName()]=true
            skills=loop.GetSkillsContextManager()
            if skills!=nil && skills.IsSkillLoaded("yaklang-system") { skillSeen=true }
            if manager!=nil && manager.IsRecentlyUsedTool("query_knowledge_base") { schemaSeen=true }
            last=loop.GetLastValidAction()
            if last!=nil && (len(actions)==0 || actions[len(actions)-1]!=last.ActionType) { actions=append(actions,last.ActionType) }
        }
        events=append(events,{"type":string(e.Type),"node":string(e.NodeId),"content":string(e.Content),"delta":string(e.StreamDelta)})
        if string(e.Type)=="tool_call_log_dir" { calls=append(calls,json.loads(string(e.Content))) }
        if string(e.Type)=="report_finish" { cards=append(cards,json.loads(string(e.Content))) }
        if string(e.Type)=="stream" && string(e.NodeId)=="re-act-loop-answer-payload" { answerParts=append(answerParts,string(e.StreamDelta)) }
    }),aim.configOptions(aiagent.functionCallMode(native),func(c){
        c.DisablePerception,c.DisableMemoryTriage,c.DisableIntervalReview=true,true,true
        // Keep the selected ordinary/focused caller; do not auto-route attached KBs.
        c.SetConfig("MustProcessAttachedData",false)
        c.SetConfig("AllowSyncInitContext",false)
        c.SetConfig("output_file",file.Join(root,"report.md"))
        c.AiAutoRetry,c.AiTransactionAutoRetry=1,1
        return nil
    })]
    if model!=nil { options=append(options,aim.aiCallback(model),aim.configOptions(aiagent.aiCallback(model))) }
    runError=aim.InvokeReAct(question,options...)
    errors,queries,matched=[],[],false
    if runError!=nil { errors=append(errors,string(runError)) }
    if !skillSeen || !schemaSeen { errors=append(errors,"system Skill or query schema was never loaded") }
    if loops[focus]!=true || loops["knowledge_enhance"]==true { errors=append(errors,"task did not stay in the requested loop") }
    for action in actions { if action in ["knowledge_enhance_answer","search_knowledge","search_knowledge_infosec"] { errors=append(errors,"legacy knowledge action executed") } }
    for call in calls {
        manifest=json.loads(string(file.ReadFile(file.Join(call.dir_path,"..","manifest.json"))~))
        if !manifest.success { errors=append(errors,"tool call failed");continue }
        if manifest.tool=="manage_knowledge_base" {
            if manifest.params.op!="list" && manifest.params.op!="get" { errors=append(errors,"read-only task modified a knowledge base") }
            continue
        }
        if manifest.tool!="query_knowledge_base" { errors=append(errors,"unexpected tool");continue }
        params=manifest.params
        if params.all || (params.name!=name && (len(params.names)!=1 || params.names[0]!=name)) { errors=append(errors,"query broadened the attached scope") }
        if params.token_limit<256 || params.token_limit>1024 || params.limit<1 || params.limit>3 || params.per_entry_token_limit<128 || params.per_entry_token_limit>256 { errors=append(errors,"query violated the requested budget") }
        result=json.loads(string(file.ReadFile(manifest.files.stdout.path)~))
        if result.used_tokens>result.token_limit { errors=append(errors,"query exceeded token budget") }
        if len(result.results)>0 { matched=true }
        for hit in result.results { if hit.knowledge_base!=name { errors=append(errors,"query returned another project") } }
        queries=append(queries,{"mode":params.mode,"token_limit":params.token_limit,"limit":params.limit,"per_entry_token_limit":params.per_entry_token_limit,"used_tokens":result.used_tokens})
    }
    answer=str.Join(answerParts,"")
    reportPath=""
    if focus=="report_generating" {
        if len(cards)!=1 { errors=append(errors,"report finish card missing") } else {
            reportPath=cards[0].report_path
            answer=string(file.ReadFile(reportPath)~)
            if cards[0].title=="" || cards[0].summary_markdown=="" { errors=append(errors,"report card is incomplete") }
        }
    }
    if len(queries)==0 || !matched { errors=append(errors,"AI answered without retrieving knowledge") }
    if !(header in answer) || !(marker in answer) || !(name in answer) || "DECOY_MARKER" in answer { errors=append(errors,"answer did not reproduce the hidden source facts and exact source name") }
    summary={"passed":len(errors)==0,"focus":focus,"requested_function_call":native,"effective_function_call":native && focus!="report_generating","real_provider":model==nil,"real_embedding":model==nil,"duration_ms":time.Now().UnixMilli()-started,"knowledge_base":name,"decoy_base":decoy,"actions":actions,"loops":loops,"queries":queries,"answer":answer,"report_path":reportPath,"errors":errors}
    file.SaveJson(file.Join(root,"summary.json"),summary)~
    file.SaveJson(file.Join(root,"events.json"),events)~
    callerResults=append(callerResults,summary)
    file.SaveJson(file.Join(os.Getenv("AISMOKING_OUTPUT"),"caller-results.json"),callerResults)~
    assert len(errors)==0,json.dumps(errors)
    println("KNOWLEDGE_CALLER_PASS ",focus," requested_function_call=",native)
}
