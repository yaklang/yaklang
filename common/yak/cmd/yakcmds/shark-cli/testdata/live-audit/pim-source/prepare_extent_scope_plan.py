"""Freeze precise next-stage native whole objects from source and agreed contract.
Inputs are the pre-repair literal source plan plus the independent minimal
21-field removal plan. No next-stage candidate output is read or accepted.
"""
import argparse,copy,hashlib,json
from pathlib import Path

PREFIX_SCOPE='captured field prefixes only; control message not fully validated; no installed routing or verified neighbor state'
UNVERIFIED_SCOPE='unverified: enclosing IP message extent unavailable or incomplete'
CONTEXT_ERROR='ContextRequired: observed wire context required: enclosing IP message extent unavailable or incomplete'
def sha(p):return hashlib.sha256(p.read_bytes()).hexdigest()
def canonical(x):return json.dumps(x,ensure_ascii=False,sort_keys=True,separators=(',',':'))
def main():
 p=argparse.ArgumentParser(description=__doc__)
 for n in('prefix-plan','minimal-plan','out'):p.add_argument('--'+n,type=Path,required=True)
 a=p.parse_args();a.out.mkdir(exist_ok=False);minimal={x['old_id']:x['minimal_no_fabricated_fields_whole_expected']for x in map(json.loads,a.minimal_plan.open(encoding='utf8'))};cases={};allcount=0
 for case in('weber','sharon'):
  rows=[];changes=[];unchanged=0;status_changes=[]
  for line in(a.prefix_plan/(case+'-whole-prefix-plan.jsonl')).open(encoding='utf8'):
   x=json.loads(line);old=copy.deepcopy(minimal.get(x['whole_old_native']['id'],x['whole_old_native']))if case=='sharon'else copy.deepcopy(x['whole_old_native']);new=copy.deepcopy(old);s=x['source'];parsed=x['literal_prefix_expected'];session=new.get('session')or{}
   if old['status']!='decoded'and'Observation Scope'in session:session['Observation Scope']=PREFIX_SCOPE
   # Public source has exactly one fully parsed message whose enclosing IP
   # declaration exceeds its observed bytes. Its old checksum failure is not
   # a complete-message validation result. No new keys are invented.
   if parsed.get('grammar_complete')and s['extent']!='complete':
    assert case=='sharon'and s['frame']==308490 and old['id']==249045
    assert old['error']=='MalformedMessage: pim: checksum failed'
    new.update(status='context-required',expert='ContextRequired',completeness='context-required',error=CONTEXT_ERROR,fields_error=CONTEXT_ERROR)
    session['Checksum Verified']=False;session['Checksum Scope']=UNVERIFIED_SCOPE
    for v in new['field_values']['values']:
     if v['name']=='status'and v['origin']=='event':v['value']='context-required'
    status_changes.append(dict(frame=s['frame'],id=old['id'],from_status=old['status'],to_status=new['status']))
   if old.get('session')is not None:new['session']=session
   item=dict(frame=s['frame'],id=old['id'],source=s,baseline_minimal_native22_expected=old,whole_next_extent_scope_expected=new)
   rows.append(item)
   if canonical(old)!=canonical(new):changes.append(item);allcount+=1
   else:unchanged+=1
  fp=a.out/(case+'-all-PIM-whole-plan.jsonl');fp.write_text(''.join(json.dumps(x,ensure_ascii=False,separators=(',',':'))+'\n'for x in rows),encoding='utf8')
  cp=a.out/(case+'-changed-PIM-whole-plan.jsonl');cp.write_text(''.join(json.dumps(x,ensure_ascii=False,separators=(',',':'))+'\n'for x in changes),encoding='utf8');cases[case]=dict(events=len(rows),whole_expected_changes=len(changes),whole_literal=unchanged,status_changes=status_changes,all_plan_SHA256=sha(fp),changed_plan_SHA256=sha(cp))
 assert allcount==143 and sum(len(x['status_changes'])for x in cases.values())==1
 result=dict(scope=__doc__,constructor_SHA256=sha(Path(__file__)),prefix_source_plan_SHA256=sha(a.prefix_plan/'summary.json'),minimal_six_plan_SHA256=sha(a.minimal_plan),contract=dict(ObservationScope=PREFIX_SCOPE,ChecksumScope=UNVERIFIED_SCOPE,ContextRequiredError=CONTEXT_ERROR,new_Session_keys=False,existing_grammar_failure_error_status_preserved=True,complete_decoded_trees_unchanged=315,Register_header_checksum_scope='register-header',Register_unknown_parent_child_admission=False),cases=cases,total_native_whole_expected_changes=allcount,flow_expectation='Both entire default flow exports remain literal; source-specific decoder-error receipts for all 12 Sharon Register errors are frozen separately. No generic metric or namespace waiver.')
 f=a.out/'summary.json';f.write_text(json.dumps(result,ensure_ascii=False,indent=2)+'\n',encoding='utf8');print(json.dumps(dict(summary=str(f),SHA256=sha(f),cases=cases),ensure_ascii=False))
if __name__=='__main__':main()
