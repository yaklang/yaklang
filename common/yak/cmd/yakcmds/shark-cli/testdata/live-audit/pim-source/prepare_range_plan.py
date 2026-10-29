"""Freeze Native24->25 exact PIM per-occurrence spans without candidate output.
All original value names, values, scalar types and array order are invariant.
Only each owning source span and 23 already-read Register IP Version range keys
are added. Context verification receives no PDU span.
"""
import argparse,base64,collections,copy,hashlib,json,sys
from pathlib import Path

def sha(p):return hashlib.sha256(p.read_bytes()).hexdigest()
def literal(x):return json.dumps(x,ensure_ascii=False,sort_keys=True,separators=(',',':'))
def stripped(values):return[{k:v for k,v in x.items()if k!='pdu_range'}for x in values]
def main():
 p=argparse.ArgumentParser(description=__doc__)
 for n in('prefix-plan','bindings','out'):p.add_argument('--'+n,type=Path,required=True)
 a=p.parse_args();a.out.mkdir(exist_ok=False);sys.path.insert(0,str(Path(__file__).parent));from pim_range_expected import projection
 api=a.bindings.read_text(encoding='utf8');cases={};totals=collections.Counter();neg=[]
 for case in('weber','sharon'):
  records=[];changed=[];counts=collections.Counter()
  for l in(a.prefix_plan/(case+'-all-prefix-whole-plan.jsonl')).open(encoding='utf8'):
   source=json.loads(l);old=source['whole_prefix_expected'];new=copy.deepcopy(old);body=bytes.fromhex(source['source']['body_hex']);session=new.get('session')
   if session is not None:
    header=session.get('Register IP Header')
    if header is not None and'IP Version'in header:
     assert len(body)>=9 and header['IP Version']==body[8]>>4
     header['Wire Field Ranges']['IP Version']=[8,9]
     if new['fields']is not None:new['fields']['Register IP Header']['Wire Field Ranges']['IP Version']=[8,9]
     counts['Register_IP_Version_Session_ranges_added']+=1
    pim=projection(session,api);oldpim=[v for v in old['field_values']['values']if v['name'].startswith('pim.')];assert stripped(pim)==stripped(oldpim),'value/type/name/order changed'
    for value in pim:
     if value['origin']=='pdu':
      span=value['pdu_range'];assert 0<=span['offset']<span['offset']+span['length']<=len(body);counts['wire_values_with_exact_source_span']+=1
     else:assert'pdu_range'not in value;counts['context_values_without_wire_span']+=1
    new['field_values']['values']=[v for v in new['field_values']['values']if not v['name'].startswith('pim.')]+pim
   item=dict(frame=source['frame'],id=old['id'],source=source['source'],baseline_prefix24_whole_expected=old,whole_range25_expected=new);records.append(item)
   if literal(old)!=literal(new):changed.append(item)
  for kind,rows in[('all',records),('changed',changed)]:
   fp=a.out/(case+'-'+kind+'-ranges-whole-plan.jsonl');fp.write_text(''.join(json.dumps(x,ensure_ascii=False,separators=(',',':'))+'\n'for x in rows),encoding='utf8')
  counts['events']=len(records);counts['whole_expected_changes']=len(changed);counts['whole_literal']=len(records)-len(changed);cases[case]=dict(counts,all_plan_SHA256=sha(a.out/(case+'-all-ranges-whole-plan.jsonl')),changed_plan_SHA256=sha(a.out/(case+'-changed-ranges-whole-plan.jsonl')));totals.update(counts)
  for item in records:
   values=item['whole_range25_expected']['field_values']['values'];byname=collections.defaultdict(list)
   for v in values:
    if v['name'].startswith('pim.')and v['origin']=='pdu':byname[v['name']].append(v)
   for name,instances in byname.items():
    if len(instances)>1:
     spans=[x['pdu_range']for x in instances]
     if len({literal(s)for s in spans})>1:
      # This negative cannot pass by value-only or first-match Raw lookup.
      wrong=copy.deepcopy(instances);wrong[1]['pdu_range']=wrong[0]['pdu_range'];assert literal(wrong)!=literal(instances);neg.append(dict(case=case,frame=item['frame'],name=name,original_spans=spans,mutation='reuse first occurrence span'))
 assert totals['Register_IP_Version_Session_ranges_added']==23
 assert totals['wire_values_with_exact_source_span']==7898 and totals['context_values_without_wire_span']==316
 result=dict(scope=__doc__,driver_SHA256=sha(Path(__file__)),constructor_SHA256=sha(Path(__file__).with_name('pim_range_expected.py')),prior_source_plan_SHA256=sha(a.prefix_plan/'summary.json'),unchanged_binding_API_SHA256=sha(a.bindings),cases=cases,total=dict(totals),per_occurrence_span_reuse_negative_controls=len(neg),negative_witnesses=neg)
 (a.out/'summary.json').write_text(json.dumps(result,ensure_ascii=False,indent=2)+'\n',encoding='utf8');print(json.dumps(dict(summary=str(a.out/'summary.json'),SHA256=sha(a.out/'summary.json'),total=result['total'],negative_controls=len(neg)),ensure_ascii=False))
if __name__=='__main__':main()
