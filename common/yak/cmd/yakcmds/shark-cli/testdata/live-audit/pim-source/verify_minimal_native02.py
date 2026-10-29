"""Strict whole Native17 -> minimal PIM derived-field correction.
Exactly six frozen full objects may change. Every other event, generated ID,
reference, value type, source/domain/Raw and ordered array remains literal.
No path waiver, alias normalization, or endpoint/time based pairing is used.
"""
import argparse,collections,copy,hashlib,itertools,json
from pathlib import Path

def sha(p):
 h=hashlib.sha256()
 with p.open('rb')as f:
  for b in iter(lambda:f.read(1<<20),b''):h.update(b)
 return h.hexdigest()
def literal(x):return json.dumps(x,sort_keys=True,separators=(',',':'),ensure_ascii=False)
def rows(p):
 for line in p.open(encoding='utf8'):yield json.loads(line)
def difference(a,b,path=''):
 if type(a)is not type(b):return[dict(path=path,expected=a,actual=b)]
 if isinstance(a,dict):
  out=[]
  for k in sorted(set(a)|set(b)):
   if k not in a or k not in b:out.append(dict(path=path+'/'+k,expected=a.get(k,'<absent>'),actual=b.get(k,'<absent>')))
   else:out.extend(difference(a[k],b[k],path+'/'+k))
  return out
 if isinstance(a,list):
  if len(a)!=len(b):return[dict(path=path,expected=a,actual=b)]
  return sum((difference(x,y,path+'/'+str(i))for i,(x,y)in enumerate(zip(a,b))),[])
 return[]if a==b else[dict(path=path,expected=a,actual=b)]
def load_plan(path):
 plan={x['old_id']:x for x in rows(path)};assert len(plan)==6
 assert sum(len(x['required_absent_typed_aliases'])for x in plan.values())==21
 for x in plan.values():
  old=x['whole_old_native'];new=x['minimal_no_fabricated_fields_whole_expected'];assert old['id']==new['id']==x['old_id'];assert old['raw']==new['raw']and old['source_bytes']==new['source_bytes']
  assert all(f['alias']not in x['required_absent_typed_aliases']for f in x['source_confirmed_prefix_fields'])
 return plan

def controls(plan):
 positive=0;negative=[]
 for item in plan.values():
  expected=item['minimal_no_fabricated_fields_whole_expected'];assert not difference(expected,copy.deepcopy(expected));positive+=1
  mutations=[('retained false derived alias',lambda x:x['field_values']['values'].append(dict(name=item['required_absent_typed_aliases'][0],value=0,origin='pdu'))),('restored false Session field',lambda x:x['session'].__setitem__(item['required_absent_session_keys'][0],False)),('domain changed',lambda x:x['domain'].__setitem__('Encapsulation','/forged')),('Raw changed',lambda x:x.__setitem__('raw','AA==')),('status changed',lambda x:x.__setitem__('status','decoded')),('expert removed',lambda x:x.__setitem__('expert','')),('PDU ID shifted',lambda x:x.__setitem__('id',x['id']+1)),('snapshot source modified',lambda x:x['field_values']['source'].__setitem__('Kind','decrypted')),('snapshot completeness modified',lambda x:x['field_values'].__setitem__('incomplete',False)),('source peer changed',lambda x:x.__setitem__('source','192.0.2.99')),('Source Ref Number changed',lambda x:x['source_bytes']['PacketRefs'][0].__setitem__('Number',item['frame']+1))]
  for name,mutate in mutations:
   x=copy.deepcopy(expected);mutate(x);assert difference(expected,x),(name,item['old_id']);negative.append(dict(id=item['old_id'],mutation=name))
 return dict(positive_full_objects=positive,semantic_mutations_rejected=len(negative),negative_controls=negative)

def main():
 p=argparse.ArgumentParser(description=__doc__)
 for n in('before','plan','out'):p.add_argument('--'+n,type=Path,required=True)
 p.add_argument('--after',type=Path);p.add_argument('--after-binary-sha');a=p.parse_args();a.out.mkdir(exist_ok=False);plan=load_plan(a.plan);result={'scope':__doc__,'auditor_SHA256':sha(Path(__file__)),'frozen_plan_SHA256':sha(a.plan),'controls':controls(plan),'cases':{},'total':{}}
 if a.after is None:
  result['mode']='prepared controls only; candidate not read'
 else:
  assert(a.after/'summary.json').is_file(),'candidate corpus has not closed';before_paths=sorted(a.before.rglob('events.jsonl'));assert len(before_paths)==37;assert {str(x.relative_to(a.before))for x in before_paths}=={str(x.relative_to(a.after))for x in a.after.rglob('events.jsonl')}
  total=collections.Counter();allseen=[]
  for before in before_paths:
   relative=before.relative_to(a.before);after=a.after/relative;bp=json.loads(before.with_name('events-producer.json').read_text(encoding='utf8'));ap=json.loads(after.with_name('events-producer.json').read_text(encoding='utf8'));assert bp['exit_code']==ap['exit_code']==0
   assert bp['capture_SHA256']==ap['capture_SHA256'];assert bp['arguments'][2:]==ap['arguments'][2:]
   assert bp['export_SHA256']==sha(before)and ap['export_SHA256']==sha(after)
   if a.after_binary_sha:assert ap['binary_SHA256']==a.after_binary_sha
   counts=collections.Counter();mismatch=[]
   for ordinal,(old,new)in enumerate(itertools.zip_longest(rows(before),rows(after)),1):
    counts['rows']+=1
    if old is None or new is None:mismatch.append(dict(ordinal=ordinal,reason='event count mismatch'));continue
    expected=old
    if relative.as_posix()=='preservation/sharon/events.jsonl'and old['id']in plan:
     item=plan[old['id']];assert literal(old)==literal(item['whole_old_native']),'frozen old object is not exact corpus baseline';expected=item['minimal_no_fabricated_fields_whole_expected'];counts['source_expected_rows']+=1;allseen.append(old['id'])
    else:counts['literal_rows']+=1
    diff=difference(expected,new)
    if diff:mismatch.append(dict(ordinal=ordinal,id=old['id'],differences=diff))
   counts['mismatches']=len(mismatch);name=relative.parent.as_posix();fp=a.out/(name.replace('/','__')+'-differences.json');fp.write_text(json.dumps(mismatch,ensure_ascii=False,indent=2)+'\n',encoding='utf8');result['cases'][name]=dict(counts,before_export_SHA256=sha(before),after_export_SHA256=sha(after),after_producer_source=ap['source'],differences_SHA256=sha(fp));total.update(counts)
  assert sorted(allseen)==sorted(plan),'targets were not selected exactly once';result['total']=dict(total);result['flows']={}
  for case in('weber','sharon'):
   bf=a.before/'preservation'/case/'flows.json';af=a.after/'preservation'/case/'flows.json';bp=json.loads(bf.with_name('flows-producer.json').read_text(encoding='utf8'));ap=json.loads(af.with_name('flows-producer.json').read_text(encoding='utf8'));assert bp['exit_code']==ap['exit_code']==0;assert bp['capture_SHA256']==ap['capture_SHA256'];assert bp['export_SHA256']==sha(bf)and ap['export_SHA256']==sha(af);assert bf.read_bytes()==af.read_bytes(),'minimal alias correction changed whole flow export';result['flows'][case]=dict(whole_bytes_identical=True,SHA256=sha(bf))
  result['passed']=not total['mismatches']
 (a.out/'summary.json').write_text(json.dumps(result,ensure_ascii=False,indent=2)+'\n',encoding='utf8');print(json.dumps({'summary':str(a.out/'summary.json'),'SHA256':sha(a.out/'summary.json'),'total':result['total'],'mode':result.get('mode'),'passed':result.get('passed')}));assert result.get('passed',True)
if __name__=='__main__':main()
