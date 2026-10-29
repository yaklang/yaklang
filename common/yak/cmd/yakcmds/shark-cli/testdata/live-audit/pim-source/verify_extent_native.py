"""Strict whole Native22 -> parent extent and prefix Scope correction.
143 source-frozen full native objects may change; all others and all default
flow exports remain literal. Flow exporter has no native protocol status totals:
its captured/TCP counters are independently frozen whole objects, not waived.
"""
import argparse,collections,copy,hashlib,itertools,json,sys
from pathlib import Path

def sha(p):
 h=hashlib.sha256()
 with p.open('rb')as f:
  for b in iter(lambda:f.read(1<<20),b''):h.update(b)
 return h.hexdigest()
def rows(p):
 for l in p.open(encoding='utf8'):yield json.loads(l)
def main():
 p=argparse.ArgumentParser(description=__doc__)
 for n in('before','plan','out'):p.add_argument('--'+n,type=Path,required=True)
 p.add_argument('--after',type=Path);p.add_argument('--after-binary-sha');a=p.parse_args();a.out.mkdir(exist_ok=False);sys.path.insert(0,str(Path(__file__).parent));from verify_minimal_native02 import difference,literal
 targets={}
 for case in('weber','sharon'):
  for x in rows(a.plan/(case+'-changed-PIM-whole-plan.jsonl')):targets[('preservation/'+case,x['id'])]=x
 assert len(targets)==143;neg=[];pos=0
 for key,item in targets.items():
  expected=item['whole_next_extent_scope_expected'];assert not difference(expected,copy.deepcopy(expected));pos+=1
  if key[1]!=249045:continue
  controls=[('wrong status',lambda x:x.__setitem__('status','decoded')),('wrong checksum scope',lambda x:x['session'].__setitem__('Checksum Scope','message')),('checksum asserted true',lambda x:x['session'].__setitem__('Checksum Verified',True)),('scope falsely complete',lambda x:x['session'].__setitem__('Observation Scope','complete selected wire fields')),('error hidden',lambda x:x.__setitem__('error','')),('new serialized private marker',lambda x:x['session'].__setitem__('Message Extent Complete',True)),('snapshot status stale',lambda x:next(v for v in x['field_values']['values']if v['name']=='status').__setitem__('value','malformed')),('Raw changed',lambda x:x.__setitem__('raw','AA==')),('Source changed',lambda x:x.__setitem__('source','192.0.2.99')),('id shifted',lambda x:x.__setitem__('id',x['id']+1))]
  for name,mutate in controls:
   value=copy.deepcopy(expected);mutate(value);assert difference(expected,value);neg.append(name)
 flow_plan={}
 for case in('weber','sharon'):
  f=a.before/'preservation'/case/'flows.json';value=json.loads(f.read_text(encoding='utf8'));assert set(value)=={'capture_SHA256','closed','error','flows','packets','stats'};assert not any('malformed'in k.lower()or'contextrequired'in k.lower()for k in value['stats'])
  flow_plan[case]=dict(path=str(f),SHA256=sha(f),whole_stats=value['stats'],scope='Capture/TCP reassembly statistics only; native status 308490 is not an exported counter here')
  for label,mutate in[('DecodeErrors decrement',lambda x:x['stats'].__setitem__('DecodeErrors',x['stats']['DecodeErrors']-2)),('bool accounting turned int',lambda x:x['stats'].__setitem__('CaptureAccountingAvailable',1)),('invented status statistic',lambda x:x['stats'].__setitem__('Malformed',0))]:
   changed=copy.deepcopy(value);mutate(changed);assert difference(value,changed);neg.append(case+': '+label)
 result=dict(scope=__doc__,auditor_SHA256=sha(Path(__file__)),prior_whole_comparator_SHA256=sha(Path(__file__).with_name('verify_minimal_native02.py')),source_plan_SHA256=sha(a.plan/'summary.json'),flow_plan=flow_plan,controls=dict(source_full_object_positives=pos,semantic_mutations_rejected=len(neg),negative_controls=neg),cases={})
 if a.after is None:result['mode']='frozen controls and whole flow expectations; no candidate read'
 else:
  assert(a.after/'summary.json').is_file();bp=sorted(a.before.rglob('events.jsonl'));assert len(bp)==37;assert{str(f.relative_to(a.before))for f in bp}=={str(f.relative_to(a.after))for f in a.after.rglob('events.jsonl')};seen=[];total=collections.Counter()
  for before in bp:
   relative=before.relative_to(a.before);after=a.after/relative;oldp=json.loads(before.with_name('events-producer.json').read_text(encoding='utf8'));newp=json.loads(after.with_name('events-producer.json').read_text(encoding='utf8'));assert oldp['exit_code']==newp['exit_code']==0;assert oldp['capture_SHA256']==newp['capture_SHA256'];assert oldp['arguments'][2:]==newp['arguments'][2:];assert oldp['export_SHA256']==sha(before)and newp['export_SHA256']==sha(after)
   if a.after_binary_sha:assert newp['binary_SHA256']==a.after_binary_sha
   case=relative.parent.as_posix();counts=collections.Counter();bad=[]
   for ordinal,(old,new)in enumerate(itertools.zip_longest(rows(before),rows(after)),1):
    counts['rows']+=1
    if old is None or new is None:bad.append(dict(ordinal=ordinal,reason='event count mismatch'));continue
    item=targets.get((case,old['id']));expected=old
    if item:
     assert literal(old)==literal(item['baseline_minimal_native22_expected']),'baseline is not the source-frozen complete object';expected=item['whole_next_extent_scope_expected'];counts['source_expected_rows']+=1;seen.append((case,old['id']))
    else:counts['literal_rows']+=1
    diff=difference(expected,new)
    if diff:bad.append(dict(ordinal=ordinal,id=old['id'],differences=diff))
   counts['mismatches']=len(bad);file=a.out/(case.replace('/','__')+'-differences.json');file.write_text(json.dumps(bad,ensure_ascii=False,indent=2)+'\n',encoding='utf8');result['cases'][case]=dict(counts,before_SHA256=sha(before),after_SHA256=sha(after),producer_source=newp['source'],differences_SHA256=sha(file));total.update(counts)
  assert sorted(seen)==sorted(targets);result['total']=dict(total);result['flows']={}
  for case,expected in flow_plan.items():
   f=a.after/'preservation'/case/'flows.json';producer=json.loads(f.with_name('flows-producer.json').read_text(encoding='utf8'));assert producer['exit_code']==0 and producer['export_SHA256']==sha(f);same=sha(f)==expected['SHA256'];diff=[]if same else difference(json.loads(Path(expected['path']).read_text(encoding='utf8')),json.loads(f.read_text(encoding='utf8')));result['flows'][case]=dict(whole_bytes_identical=same,differences=diff,SHA256=sha(f))
  result['passed']=not total['mismatches']and all(x['whole_bytes_identical']for x in result['flows'].values())
 (a.out/'summary.json').write_text(json.dumps(result,ensure_ascii=False,indent=2)+'\n',encoding='utf8');print(json.dumps({'summary':str(a.out/'summary.json'),'SHA256':sha(a.out/'summary.json'),'total':result.get('total'),'passed':result.get('passed'),'mode':result.get('mode')},ensure_ascii=False));assert result.get('passed',True)
if __name__=='__main__':main()
