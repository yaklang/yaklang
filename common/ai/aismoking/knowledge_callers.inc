// Shared real execution/assertions for mock-provider and explicit live runs.
callerCase = {}
callerResults = []
planCase = {}
planResults = []
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
        if !restricted && manager!=nil {
            if os.Getenv("AISMOKING_UNRESTRICTED_KNOWLEDGE")!="1" { manager.RestrictToTools("manage_knowledge_base","query_knowledge_base") }
            restricted=true
        }
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
        if model!=nil { c.AiAutoRetry,c.AiTransactionAutoRetry=1,1 }
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
        if params.token_limit<256 || params.token_limit>1024 || params.limit<1 || (params.mode!="get" && params.limit>3) || params.per_entry_token_limit<128 || params.per_entry_token_limit>256 { errors=append(errors,"query violated the requested budget") }
        result=json.loads(string(file.ReadFile(manifest.files.stdout.path)~))
        if result.used_tokens>result.token_limit || len(result.results)>3 { errors=append(errors,"query exceeded token/result budget") }
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
    summary={"passed":len(errors)==0,"focus":focus,"requested_function_call":native,"effective_function_call":native && focus!="report_generating","unrestricted_tools":os.Getenv("AISMOKING_UNRESTRICTED_KNOWLEDGE")=="1","real_provider":model==nil,"real_embedding":model==nil,"duration_ms":time.Now().UnixMilli()-started,"knowledge_base":name,"decoy_base":decoy,"actions":actions,"loops":loops,"queries":queries,"answer":answer,"report_path":reportPath,"errors":errors}
    file.SaveJson(file.Join(root,"summary.json"),summary)~
    file.SaveJson(file.Join(root,"events.json"),events)~
    callerResults=append(callerResults,summary)
    file.SaveJson(file.Join(os.Getenv("AISMOKING_OUTPUT"),"caller-results.json"),callerResults)~
    assert len(errors)==0,json.dumps(errors)
    println("KNOWLEDGE_CALLER_PASS ",focus," requested_function_call=",native)
}

// Exercise the public PLAN entry, real workers, shared evidence and delivery.
// The model only receives the natural request and fixture paths, not hidden facts.
func runKnowledgePlan(native,model,ctx) {
    root=file.Join(os.Getenv("AISMOKING_OUTPUT"),"plan-"+string(native))
    file.MkdirAll(root)~
    prefix="knowledge-plan-"+string(time.Now().UnixNano())+"-"+randstr(8)
    name,decoy=prefix+"-项目",prefix+"-对照"
    header,marker="X-Plan-"+randstr(12),"PLAN_FILE_"+randstr(16)
    oldNote,newNote="PLAN_OLD_"+randstr(16),"PLAN_UPDATED_"+randstr(16)
    source=file.Join(root,"项目鉴权资料.txt")
    workerReport=file.Join(root,"knowledge-report.md")
    file.Save(source,"本项目 API 鉴权请求头是 "+header+"，验证标记是 "+marker+"。")~
    planCase={"root":root,"prefix":prefix,"name":name,"decoy":decoy,"source":source,"header":header,"marker":marker,"old":oldNote,"updated":newNote,"calls":[]}
    file.SaveJson(file.Join(root,"fixtures.json"),{"name":name,"decoy":decoy})~
    defer func(){
        for owned in [name,decoy] {
            _,err=aiknowledge.Get(owned)
            if err==nil { aiknowledge.Delete(owned)~ }
            page=aiknowledge.List(aiknowledge.keyword(owned),aiknowledge.limit(1))~
            assert page.total==0,"PLAN fixture cleanup failed"
        }
    }()
    aiknowledge.Add(decoy)~
    aiknowledge.AddEntry(decoy,"对照资料","DECOY_PLAN_MUST_SURVIVE")~
    question=sprintf("请进入 Plan 模式，分三个有依赖关系的任务执行并交付报告。第一步：创建知识库「%s」，将文件「%s」按直接切块方式入库（每块512、并发1），再新增标题「临时维护说明」、正文「%s」的条目。第二步：查询这个库的 API 鉴权请求头及验证标记，读出临时维护条目的 ID，以该 ID 更新正文为「%s」，将库描述更新为「PLAN 验证已更新」，再查询核对更新结果。第三步：删除临时维护条目，查询确认文件中的 API 鉴权资料仍在，然后删除本次创建的库，按名称前缀「%s」列库（最多五个）核对仅余对照库「%s」。最终报告必须包含查到的请求头、验证标记、完整来源库名及实际修改和删除结果。只允许修改「%s」，不修改其他库。每次知识查询必须明确选这个库，最多三条、总预算1024 tokens、每条256 tokens。",name,source,oldNote,newNote,prefix,decoy,name)
    if model==nil { question+="请保持三个可执行任务，不再细拆。第三个任务包含收尾报告，报告保持简短并保存到「"+workerReport+"」，不要额外创建仅负责报告的任务。" }
    planCase.question=question
    mu=sync.NewMutex()
    events,cards,errors,operations=[],[],[],[]
    approvals,pushes,pops,ends=0,0,0,0
    restricted=false
    started=time.Now().UnixMilli()
    timeout=450
    if model==nil { timeout=840 }
    opts=[aim.focus("coordinator"),aim.sessionID(prefix),aim.workdir(root),aim.attachedKnowledgeBase(name),aim.attachedFileContent("只操作本次指定库，保留对照库。查询必须显式选库并限制结果和 token。"),aim.context(ctx),aim.timeout(timeout),aim.maxIteration(60),aim.reviewPolicy("yolo"),aim.disableMCPServers(true),aim.onEvent(func(op,e){
        manager=op.GetConfig().GetAiToolManager()
        if !restricted && manager!=nil { manager.RestrictToTools("manage_knowledge_base","query_knowledge_base","read_file","write_file");restricted=true }
        typ,data=string(e.Type),{}
        if e.IsJson { data=json.loads(string(e.Content)) }
        mu.Lock()
        events=append(events,{"type":typ,"node":string(e.NodeId),"content":string(e.Content),"delta":string(e.StreamDelta)})
        if typ=="tool_call_log_dir" { planCase.calls=append(planCase.calls,data) }
        if typ=="report_finish" { cards=append(cards,data) }
        if typ=="plan_review_require" { approvals++ }
        if typ=="end_plan_and_execution" { ends++ }
        if e.NodeId=="system" && data.type=="push_task" { pushes++ }
        if e.NodeId=="system" && data.type=="pop_task" { pops++ }
        mu.Unlock()
        if typ=="plan_review_require" {
            op.SendInputEvent(aim.NewInputEvent({"IsInteractiveMessage":true,"InteractiveId":data.id,"InteractiveJSONInput":json.dumps({"suggestion":"continue"})})~)~
        }
    }),aim.configOptions(aiagent.functionCallMode(native),func(c){
        c.DisablePerception,c.DisableMemoryTriage,c.DisableIntervalReview=true,true,true
        c.ForceManualPlanReview,c.PlanExecTaskConcurrency=true,1
        if model!=nil { c.AiAutoRetry,c.AiTransactionAutoRetry=1,1 }
        c.GetTimeline().SetTimelineBucketByteSize(-1)
        return nil
    })]
    if model!=nil { opts=append(opts,aim.aiCallback(model),aim.speedPriorityAICallback(model),aim.configOptions(aiagent.aiCallback(model))) }
    err=aim.InvokeReAct(question,opts...)
    if err!=nil { errors=append(errors,string(err)) }
    seen,queries,entryID,hiddenRead,survivedDelete,updatedRead,lastListTotal={},0,"",false,false,false,-1
    for call in planCase.calls {
        manifest=json.loads(string(file.ReadFile(file.Join(call.dir_path,"..","manifest.json"))~))
        params=manifest.params
        if !manifest.success { errors=append(errors,"PLAN tool call failed: "+manifest.tool);continue }
        if manifest.tool in ["read_file","write_file"] {
            if manifest.tool=="read_file" && params.file!=source && params.file!=workerReport { errors=append(errors,"PLAN read escaped the owned files") }
            if manifest.tool=="write_file" && params.file!=workerReport { errors=append(errors,"PLAN write escaped the owned report") }
            operations=append(operations,{"tool":manifest.tool,"params":params})
            continue
        }
        value=json.loads(string(file.ReadFile(manifest.files.stdout.path)~))
        operations=append(operations,{"tool":manifest.tool,"params":params,"result":value})
        if manifest.tool=="manage_knowledge_base" {
            seen[params.op]=true
            if params.op=="list" {
                lastListTotal=value.total
                if params.keyword!=prefix || params.limit<1 || params.limit>5 { errors=append(errors,"PLAN list scope/result mismatch") }
            } else if params.name!=name { errors=append(errors,"PLAN management escaped owned scope") }
            if params.op=="build" && (value.entries_added<1 || value.mode!="chunks" || params.build_options.chunk_size!=512 || params.build_options.concurrency!=1) { errors=append(errors,"PLAN file build mismatch") }
            if params.op=="add_entry" { entryID=value.entry_id }
            if params.op=="update_entry" && (params.entry_id!=entryID || value.entry_id!=entryID || params.entry.content!=newNote) { errors=append(errors,"PLAN update lost the stable entry ID") }
            if params.op=="update" && value.knowledge_base.description!="PLAN 验证已更新" { errors=append(errors,"PLAN metadata update mismatch") }
            if params.op=="delete_entry" && params.entry_id!=entryID { errors=append(errors,"PLAN deleted the wrong entry") }
        } else if manifest.tool=="query_knowledge_base" {
            queries++
            if params.all || (params.name!=name && (len(params.names)!=1 || params.names[0]!=name)) { errors=append(errors,"PLAN query escaped explicit scope") }
            if params.token_limit<256 || params.token_limit>1024 || params.limit<1 || (params.mode!="get" && params.limit>3) || params.per_entry_token_limit<128 || params.per_entry_token_limit>256 || value.used_tokens>value.token_limit || len(value.results)>3 || len(value.results)>params.limit { errors=append(errors,"PLAN query violated budgets") }
            for hit in value.results {
                if hit.knowledge_base!=name { errors=append(errors,"PLAN query returned decoy knowledge") }
                if header in hit.content && marker in hit.content {
                    hiddenRead=true
                    if seen["delete_entry"]==true && seen["delete"]!=true { survivedDelete=true }
                }
                if hit.entry_id==entryID && hit.content==newNote { updatedRead=true }
            }
        } else { errors=append(errors,"unexpected PLAN tool: "+manifest.tool) }
    }
    for op in ["create","build","add_entry","update","update_entry","delete_entry","delete","list"] { if seen[op]!=true { errors=append(errors,"PLAN did not execute "+op) } }
    if queries<3 || !hiddenRead || !survivedDelete || !updatedRead || entryID=="" { errors=append(errors,"PLAN did not query actual hidden facts/updated entries/surviving file") }
    if approvals!=1 || pushes<3 || pops!=pushes || (model!=nil && pushes!=3) || ends!=1 || len(cards)!=1 { errors=append(errors,"PLAN approval/worker/completion events mismatch") }
    report=""
    if len(cards)==1 {
        report=string(file.ReadFile(cards[0].report_path)~)
        if !(header in report) || !(marker in report) || !(name in report) || "DECOY_PLAN_MUST_SURVIVE" in report { errors=append(errors,"PLAN report lost the actual source facts") }
    }
    page=aiknowledge.List(aiknowledge.keyword(name),aiknowledge.limit(1))~
    untouched=aiknowledge.Get(decoy)~
    if page.total!=0 || untouched.entry_count!=1 || lastListTotal!=1 { errors=append(errors,"PLAN final database state mismatch") }
    result={"passed":len(errors)==0,"function_call":native,"real_provider":model==nil,"duration_ms":time.Now().UnixMilli()-started,"approvals":approvals,"pushes":pushes,"pops":pops,"ends":ends,"queries":queries,"operations":operations,"report":report,"errors":errors}
    file.SaveJson(file.Join(root,"summary.json"),result)~
    file.SaveJson(file.Join(root,"events.json"),events)~
    planResults=append(planResults,result)
    file.SaveJson(file.Join(os.Getenv("AISMOKING_OUTPUT"),"plan-results.json"),planResults)~
    assert len(errors)==0,json.dumps(errors)
    println("KNOWLEDGE_PLAN_PASS native=",native," queries=",queries," operations=",len(operations))
}
