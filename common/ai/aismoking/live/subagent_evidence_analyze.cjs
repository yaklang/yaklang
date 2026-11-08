const fs = require('fs');
const path = require('path');
const experiment = process.argv[2];
if (!experiment) { console.error('Usage: node subagent_evidence_analyze.cjs <experiment-root>'); process.exit(2); }
const read = (file, fallback = null) => { try { return JSON.parse(fs.readFileSync(file, 'utf8').replace(/^\uFEFF/, '')); } catch { return fallback; } };
const parse = value => { try { return typeof value === 'string' ? JSON.parse(value) : value || {}; } catch { return {}; } };
const normalize = value => String(value || '').replace(/\\/g, '/');
const summaries = [];
for (const name of ['baseline', 'current', 'default']) {
  const dir = path.join(experiment, name);
  const rows = read(path.join(dir, 'requests.json'), []);
  if (!rows.length) continue;
  const events = read(path.join(dir, 'events.json'), []);
  const facts = Object.values(read(path.join(dir, 'tool-facts.json'), {}));
  const result = read(path.join(dir, 'result.json'));
  const actions = rows.flatMap(row => (row.tool_calls || []).map(call => ({...row, action:call.name, args:parse(call.arguments)})));
  const markers = events.filter(event => event.node === 'loop_marker').map(event => ({...parse(event.content), seconds:event.seconds}));
  const childTasks = new Set(markers.filter(marker => (marker.loop_kind === 'subagent' || marker.parent_task_id) && marker.marker === 'enter').map(marker => marker.task_id));
  for (const event of events.filter(event=>event.node==='react_task_created')) { const value=parse(event.content); if(value.react_task_is_sub_agent) childTasks.add(value.react_task_id); }
  const classify = fact => childTasks.has(fact.task_id) ? 'child' : 'parent';
  const toolCounts = {parent:{}, child:{}};
  let untypedToolEnvelopes = 0;
  for (const fact of facts.filter(fact => fact.done)) {
    if (!fact.tool) { untypedToolEnvelopes++; continue; }
    const bucket = toolCounts[classify(fact)];
    bucket[fact.tool] = (bucket[fact.tool] || 0) + 1;
  }
  const firstDispatch = actions.find(row => row.action === 'dispatch_sub_react_agents' && row.role === 'parent');
  const jobBatches = actions.filter(row => row.action === 'dispatch_sub_react_agents' && row.role === 'parent').map(row => ({seq:row.seq, seconds:row.started_seconds, jobs:(row.args.dispatches || []).map(job => ({identifier:job.identifier, goal:job.goal, context_mode:job.context_mode || 'fork'}))}));
  const prompts = new Map();
  function promptFor(row) {
    if (!prompts.has(row.seq)) {
      const body = read(path.join(dir, `request-${String(row.seq).padStart(3,'0')}.json`), {});
      prompts.set(row.seq, (body.messages || []).map(message => typeof message.content === 'string' ? message.content : JSON.stringify(message.content || '')).join('\n'));
    }
    return prompts.get(row.seq);
  }
  const rootInputs = rows.filter(row => row.role === 'parent' && row.caller.startsWith('react-loop:'));
  const saves = actions.filter(row => row.role === 'child' && row.action === 'save_evidence').map(row => {
    const args = row.args.next_action || row.args;
    const content = (args.evidence_content || '').trim();
    const id = args.evidence_id || null;
    const finish = actions.find(other => other.runtime_id === row.runtime_id && other.action === 'finish' && other.seq > row.seq);
    const parent = content ? rootInputs.find(other => other.seq > row.seq && (other.parent_evidence_at_start || '').includes(content) && promptFor(other).includes(content)) : null;
    const beforeFinish = !!(parent && finish && parent.started_seconds < finish.completed_seconds);
    const priorParentSave = actions.some(other => other.role === 'parent' && other.action === 'save_evidence' && other.seq < (parent?.seq || Infinity) && (other.args.evidence_content || '') === content);
    const matches = [...promptFor(row).matchAll(/CURRENT TASK \[task_index=[^\],]+, task_id=([^\]]+)\]/g)];
    const taskID = matches.at(-1)?.[1] || null;
    const terminal = events.find(event => { const value=parse(event.content); return event.node==='react_task_status_changed' && value.react_task_id===taskID && value.react_task_now_status==='completed'; });
    return {save_seq:row.seq,runtime_id:row.runtime_id,task_id:taskID,evidence_id:id,content,save_provider_completed_seconds:row.completed_seconds,parent_first_input_seq:parent?.seq ?? null,parent_first_input_seconds:parent?.started_seconds ?? null,child_finish_seq:finish?.seq ?? null,child_finish_response_seconds:finish?.completed_seconds ?? null,child_completed_seconds:terminal?.seconds??null,visible_before_child_finish:beforeFinish,advance_seconds:beforeFinish ? +(finish.completed_seconds-parent.started_seconds).toFixed(3) : null,advance_before_completed_seconds:parent&&terminal ? +(terminal.seconds-parent.started_seconds).toFixed(3) : null,parent_had_saved_same_content:priorParentSave};
  });
  const lifecycle = events.filter(event => event.node === 'react_task_status_changed').map(event => ({seconds:event.seconds, ...parse(event.content)}));
  const sourceReadCounts = {parent:0, child:0};
  const inputCoverage = new Set();
  for (const fact of facts.filter(fact => fact.done)) {
    const file = normalize(fact.params?.file);
    if ((fact.tool === 'read_file' || fact.tool === 'read_file_lines') && file.includes('/input/')) {
      sourceReadCounts[classify(fact)]++;
      inputCoverage.add(file.split('/input/')[1]);
    }
  }
  const groups = {};
  let missingUsage = 0;
  for (const row of rows) {
    const bucket = row.caller.startsWith('react-loop:') ? row.role : 'helper';
    const group = groups[bucket] ||= {calls:0,input_tokens:0,output_tokens:0,provider_seconds:0};
    group.calls++;
    group.provider_seconds += row.seconds || 0;
    if (!row.usage) missingUsage++;
    else { group.input_tokens += row.usage.prompt_tokens || 0; group.output_tokens += row.usage.completion_tokens || 0; }
  }
  for (const group of Object.values(groups)) { group.total_tokens=group.input_tokens+group.output_tokens; group.provider_seconds=+group.provider_seconds.toFixed(3); }
  const responseModels = new Set();
  for (const row of rows.filter(row => row.completed)) {
    const file = path.join(dir, `response-${String(row.seq).padStart(3,'0')}.sse`);
    if (!fs.existsSync(file)) continue;
    for (const line of fs.readFileSync(file,'utf8').split('\n')) {
      if (line.trim().startsWith('data:')) { const frame=parse(line.trim().slice(5).trim()); if(frame.model) responseModels.add(frame.model); }
    }
  }
  const privateInputs = rootInputs.filter(row => row.private_billing_in_prompt || row.private_access_in_prompt).map(row => row.seq);
  const metadata = read(path.join(dir,'metadata.json'),{});
  const audit = read(path.join(dir,'audit.json')) || read(path.join(dir,'work','output','audit.json')) || (metadata.work ? read(path.join(metadata.work,'output','audit.json')) : null);
  const alternateDryRunCategory = result?.failed_checks?.length===1 && result.failed_checks[0]==='pending_job_ids' && JSON.stringify(audit?.pending_job_ids)==='["R02"]' && JSON.stringify(audit?.dry_run_job_ids)==='["R04"]' && result.parent_finish && result.runtime_error==='<nil>';
  const semanticPassed = !!(result && (result.business_passed || alternateDryRunCategory));
  const summary = {name,finished:!!result,result,semantic_business_passed:semanticPassed,requests:rows.length,completed_requests:rows.filter(row=>row.completed).length,request_models:[...new Set(rows.map(row=>row.request_model).filter(Boolean))],response_models:[...responseModels],initial_dispatch_declared:rows[0]?.declared_actions?.includes('dispatch_sub_react_agents'),first_dispatch_seq:firstDispatch?.seq??null,first_dispatch_parent_decision_index:firstDispatch?rootInputs.filter(row=>row.seq<=firstDispatch.seq).length:null,job_batches:jobBatches,child_task_ids:[...childTasks],tool_counts:toolCounts,untyped_tool_envelopes:untypedToolEnvelopes,source_read_counts:sourceReadCounts,input_read_coverage:[...inputCoverage].sort(),groups,total_tokens:Object.values(groups).reduce((sum,g)=>sum+g.total_tokens,0),missing_usage:missingUsage,child_saves:saves,early_deliveries:saves.filter(save=>save.visible_before_child_finish&&!save.parent_had_saved_same_content).length,evidence_available_parent_input_seqs:rootInputs.filter(row=>row.evidence_available_feedback).map(row=>row.seq),private_marker_parent_input_seqs:privateInputs,lifecycle};
  fs.writeFileSync(path.join(dir,'analysis.json'),JSON.stringify(summary,null,2));
  summaries.push(summary);
}
const compact = summaries.map(summary => ({name:summary.name,finished:summary.finished,result:summary.result,semantic_business_passed:summary.semantic_business_passed,requests:summary.requests,models:summary.response_models,first_dispatch_seq:summary.first_dispatch_seq,batches:summary.job_batches.map(batch=>({seq:batch.seq,jobs:batch.jobs.map(job=>job.identifier)})),child_tasks:summary.child_task_ids.length,tool_counts:summary.tool_counts,source_reads:summary.source_read_counts,input_read_coverage_count:summary.input_read_coverage.length,groups:summary.groups,total_tokens:summary.total_tokens,missing_usage:summary.missing_usage,early_deliveries:summary.early_deliveries,child_save_count:summary.child_saves.length,early_delivery_details:summary.child_saves.map(({content,...rest})=>rest),private_marker_parent_input_seqs:summary.private_marker_parent_input_seqs}));
fs.writeFileSync(path.join(experiment,'comparison.json'),JSON.stringify(compact,null,2));
console.log(JSON.stringify(compact,null,2));
