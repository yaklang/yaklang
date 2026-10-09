// Shared artifact assertions for live and offline replay. No model calls.
checks=[]
if runtimeState.runtime_error_known && runtimeState.error!="<nil>" { checks=append(checks,"runtime_error") }
businessCompleted=false
for event in events {
    if event.node=="react_task_status_changed" && event.type=="structured" {
        status=json.loads(event.content)
        if status.react_task_now_status=="completed" { businessCompleted=true }
    }
}
if !businessCompleted { checks=append(checks,"missing_completed_status") }
for event in events { if event.type=="fail_react_task" { checks=append(checks,"business_failure") } }
for i=1;i<=12;i++ {
    filename=file.Join(root,sprintf("checkpoint-%02d.json",i))
    raw,err=file.ReadFile(filename)
    if err!=nil { checks=append(checks,sprintf("missing_checkpoint_%02d",i)); continue }
    parsed,err=json.New(string(raw))
    if err!=nil { checks=append(checks,sprintf("invalid_checkpoint_%02d",i)); continue }
    cp=parsed.Value(); rawRows,uniqueOrders=4*i,4*i
    if i>=2 { rawRows++ }
    if i>=5 { rawRows++; uniqueOrders++ }
    paid=59*i+5*i*(i+1)/2; refunded=3*i*(i+1)/2; effective=paid
    state,applied="none",false
    if i>=4 { state="accepted" }
    if i==12 { effective-=33; state,applied="succeeded",true }
    for key,value in {"wave":i,"raw_rows":rawRows,"unique_orders":uniqueOrders,"paid_total":paid,"refund_total":refunded,"net_total":paid-refunded,"effective_paid_total":effective,"effective_net_total":effective-refunded,"adjustment_state":state,"adjustment_applied":applied} {
        if !(key in cp) || cp[key]!=value { checks=append(checks,sprintf("checkpoint_%02d_%s",i,key)) }
    }
    for key,value in {"ledger":sprintf("LEDGER-%02d",i),"meta":sprintf("META-%02d",i),"erp":sprintf("ERP-%02d",i)} { if !("source_versions" in cp) || cp.source_versions==nil || !(key in cp.source_versions) || cp.source_versions[key]!=value { checks=append(checks,sprintf("checkpoint_%02d_source_%s",i,key)) } }
}
auditRaw,auditErr=file.ReadFile(file.Join(root,"audit.json"))
if auditErr!=nil { checks=append(checks,"missing_audit") } else {
    parsed,err=json.New(string(auditRaw))
    if err!=nil { checks=append(checks,"invalid_audit") } else {
        audit=parsed.Value()
        for key,value in {"raw_rows":50,"unique_orders":49,"correlation_id":"AUDIT-CORR-2026","receipt_id":"ERP-EXECUTED-12"} { if !(key in audit) || audit[key]!=value { checks=append(checks,"audit_"+key) } }
        wavesOK=false
        if "completed_waves" in audit {
            waves=audit.completed_waves
            if waves==12 { wavesOK=true } else if len(waves)==12 {
                wavesOK=true
                for wave=1;wave<=12;wave++ { found=false; for value in waves { if value==wave { found=true } }; if !found { wavesOK=false } }
            }
        }
        if !wavesOK { checks=append(checks,"audit_completed_waves") }
        for group,values in {"before_adjustment":{"paid_total":1098,"refund_total":234,"net_total":864},"after_adjustment":{"paid_total":1065,"refund_total":234,"net_total":831}} { for key,value in values { if !(group in audit) || audit[group]==nil || !(key in audit[group]) || audit[group][key]!=value { checks=append(checks,"audit_"+group+"_"+key) } } }
        if len(audit.duplicates)!=1 || audit.duplicates[0]!="A01" { checks=append(checks,"audit_duplicates") }
        if len(audit.unknown_skus)!=1 || audit.unknown_skus[0]!="U05" { checks=append(checks,"audit_unknown_skus") }
        if len(audit.pending_orders)!=12 { checks=append(checks,"audit_pending_orders") }
    }
}
if !file.IsExisted(file.Join(root,"audit.md")) { checks=append(checks,"missing_report") }
completed,failed,counts=0,0,{}
for id,fact in toolFacts {
    if !fact.done && fact.error==nil && !fact.cancelled { checks=append(checks,"missing_terminal_event_"+id) }
    if fact.done { completed++ }
    if fact.error!=nil || fact.cancelled { failed++ }
    if fact.done && fact.params!=nil {
        key=""
        if fact.tool=="read_file" || fact.tool=="write_file" { key=str.ReplaceAll(fact.params.file,"\\","/") }
        if fact.tool=="do_http_request" { key=fact.params.url }
        if key!="" { counts[key]=int(counts[key])+1 }
    }
}
for i=1;i<=12;i++ {
    for prefix,extension in {"ledger":"csv","meta":"json"} {
        key=path+sprintf("/fixture/%s-%02d.%s",prefix,i,extension)
        if counts[key]!=1 { checks=append(checks,sprintf("read_count_%s_%02d",prefix,i)) }
    }
    if counts[path+sprintf("/checkpoint-%02d.json",i)]!=1 { checks=append(checks,sprintf("write_count_%02d",i)) }
    successes=0
    for receipt in httpReceipts { if receipt.wave==i && receipt.status==200 { successes++ } }
    expected=1; if i==7 { expected=2 }
    if successes!=1 || httpAttempts[string(i)]!=expected { checks=append(checks,sprintf("http_attempts_%02d",i)) }
}
for name in ["audit.json","audit.md"] { if counts[path+"/"+name]!=1 { checks=append(checks,"report_write_count_"+name) } }
main,helper={"requests":0,"input_tokens":0,"cached_tokens":0,"output_tokens":0,"provider_seconds":0,"hit_ratio":nil},{"requests":0,"input_tokens":0,"cached_tokens":0,"output_tokens":0,"provider_seconds":0,"hit_ratio":nil}
groups,readGroups,reportGroups,oldParams,protocolErrors,retries=0,0,0,0,0,0
for row in requests {
    if !row.completed { checks=append(checks,sprintf("incomplete_request_%03d",row.seq)) }
    bucket=helper
    if str.HasPrefix(row.caller,"react-loop:") { bucket=main }
    bucket.requests++; bucket.provider_seconds+=float(row.seconds)
    if row.usage!=nil {
        bucket.input_tokens+=int(row.usage.prompt_tokens); bucket.output_tokens+=int(row.usage.completion_tokens)
        if row.usage.prompt_tokens_details!=nil { bucket.cached_tokens+=int(row.usage.prompt_tokens_details.cached_tokens) }
    }
    if row.retry_prompt { retries++ }
    if row.caller=="toolcall-params" || ("param" in row.caller && "generate" in row.caller) { oldParams++ }
    if row.invalid_arguments!=0 || row.unadvertised!=0 || len(row.parse_errors)>0 || row.transport_error!=nil { protocolErrors++ }
    for call in row.tool_calls {
        parsed,err=json.New(call.arguments)
        if err!=nil { continue }
        args=parsed.Value()
        if call.name=="directly_call_tool" && "directly_call_tool_params_group" in args && args.directly_call_tool_params_group!=nil {
            groups++
            if len(args.directly_call_tool_params_group)==3 { readGroups++ }
            if len(args.directly_call_tool_params_group)==2 { reportGroups++ }
        }
    }
}
if oldParams!=0 { checks=append(checks,"retired_argument_generation") }
if protocolErrors!=0 { checks=append(checks,"provider_protocol_errors") }
if readGroups!=12 { checks=append(checks,"read_groups_not_twelve") }
if reportGroups!=1 { checks=append(checks,"report_group_not_one") }
for receipt in receipts {
    if receipt.memory_error!="<nil>" { checks=append(checks,"compression_tail_incomplete") }
    if !receipt.previous_summary_visible || !receipt.previous_memory_visible { checks=append(checks,"compression_previous_state_missing") }
}
for commit in commits { for retained in commit.retained_checks { if !retained.unchanged { checks=append(checks,"retained_original_changed") } } }
if len(receipts)<2 { checks=append(checks,"missing_threshold_compression") }
pending=runtimeState.memory_finalization_pending
if pending==true { checks=append(checks,"memory_finalization_pending") }
memories,searchErr=aimemory.Search("报告",aimemory.memoryNamespace(namespace),aimemory.memorySearchMode("bm25"),aimemory.memoryTokenLimit(8192))
if searchErr!=nil { checks=append(checks,"memory_search_failed") }
file.SaveJson(file.Join(root,"search-memories.json"),memories)~
main.hit_ratio,helper.hit_ratio=nil,nil
if main.input_tokens>0 { main.hit_ratio=float(main.cached_tokens)/float(main.input_tokens) }
if helper.input_tokens>0 { helper.hit_ratio=float(helper.cached_tokens)/float(helper.input_tokens) }
result={"passed":len(checks)==0,"checks":checks,"model":model,"namespace":namespace,"error":runtimeState.error,"seconds":runtimeState.seconds,"capture_replayed":runtimeState.capture_replayed,"runtime_error_known":runtimeState.runtime_error_known,"memory_finalization_state_verified":pending!=nil,"business_completed_observed":businessCompleted,"tool_operations_completed":completed,"tool_errors_or_cancels":failed,"parameter_groups":groups,"read_groups":readGroups,"report_groups":reportGroups,"old_argument_requests":oldParams,"protocol_errors":protocolErrors,"retry_prompts":retries,"main":main,"helper":helper,"compressions":len(receipts),"memory_saves":memorySaves,"memory_indexes":memoryIndexes,"searchable_report_memories":len(memories),"memory_finalization_pending":pending}
businessChecks=[]
for check in checks { if !(check in ["provider_protocol_errors","read_groups_not_twelve","report_group_not_one"]) { businessChecks=append(businessChecks,check) } }
result.business_passed=len(businessChecks)==0
result.business_checks=businessChecks
file.SaveJson(file.Join(root,"result.json"),result)~
printf("TOOL_LONGRUN_RESULT %s\n",json.dumps(result))
assert result.passed,"Tool system long-run validation failed; inspect captured artifacts"
println("AISMOKING_PASS tool_system_longrun")
