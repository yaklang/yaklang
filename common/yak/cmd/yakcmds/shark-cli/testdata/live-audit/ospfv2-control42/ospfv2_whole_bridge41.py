"""Freeze556 original-source OSPFv2 additions and verify37 complete replays.

Only the established generated-PDU-reference schema may follow an injective
publisher ledger. All556 new native objects and CLI rows are source-constructed;
every other whole object stays literal outside that ledger. No wire integers,
frame numbers, flows, session keys or arbitrary error/statistics are normalized.
"""
from pathlib import Path
import argparse,collections,copy,hashlib,importlib.util,itertools,json,shutil,sys
ROOT=Path(__file__).parent
REPO=Path(r'C:\Users\V\.codex\worktrees\shark-protocol-infrastructure\yaklang')
AUDIT=REPO/'common/yak/cmd/yakcmds/shark-cli/testdata/live-audit'

def sha(p):
 h=hashlib.sha256()
 with Path(p).open('rb')as f:
  for b in iter(lambda:f.read(1<<20),b''):h.update(b)
 return h.hexdigest()
def read(p):return json.loads(Path(p).read_text('utf-8-sig'))
def rows(p):
 with Path(p).open(encoding='utf-8-sig')as f:
  for line in f:yield json.loads(line)
def canonical(x):return json.dumps(x,sort_keys=True,separators=(',',':'),ensure_ascii=False).encode()
def write(p,v):
 with Path(p).open('x',encoding='utf8')as f:json.dump(v,f,indent=2);f.write('\n')
def same(a,b):
 if type(a)is not type(b):return False
 if isinstance(a,dict):return a.keys()==b.keys()and all(same(a[k],b[k])for k in a)
 if isinstance(a,list):return len(a)==len(b)and all(same(x,y)for x,y in zip(a,b))
 return a==b
def load(name,p):
 s=importlib.util.spec_from_file_location(name,p);m=importlib.util.module_from_spec(s);s.loader.exec_module(m);return m

def selected(plan):
 result={}
 for p,digest in plan['source_grids'].items():
  assert sha(p)==digest
  for fact in read(p):
   key=(fact['capture_SHA256'],fact['frame']);assert key not in result;result[key]=fact
 assert len(result)==556;return result

def binding(path,summary):
 p=read(path);assert p in summary['outputs']and p['source']==summary['source']and p['exit_code']==0
 kind=path.name.split('-producer')[0];export=path.with_name(kind+('.json'if kind=='flows'else'.jsonl'))
 binary=Path(p['arguments'][0]);build=read(binary.parent/(kind+'-receipt.json'))
 assert build['source']==p['source']and build['exit_code']==0
 assert sha(binary)==build['binary_SHA256']==p['binary_SHA256']and sha(p['capture'])==p['capture_SHA256']and sha(export)==p['export_SHA256']
 return p,export

def prepare(a):
 a.plan.mkdir(exist_ok=False);frozen=a.plan/'frozen';frozen.mkdir()
 source_dir=ROOT/'OSPFv2-independent-source39-01/source-grid03'
 grids={str(p):sha(p)for p in sorted(source_dir.glob('input-*-source-grid.json'))}
 assert sha(source_dir/'summary.json')=='2aaf89759d9b4a2e44ff1b075778694d2b2c9eea8db3a3f12b19bae0ffdbbc65'
 old_native=ROOT/'runtime-public-native-41';old_cli=ROOT/'runtime-public-CLI-41'
 contract=load('ospfv2_pure_source_contract',ROOT/'ospfv2_source_contract41.py')
 targets=selected(dict(source_grids=grids));cases=[];before={};witnesses=[]
 for name,folder,kind in [('native',old_native,'events'),('CLI',old_cli,'yak')]:
  summary=read(folder/'summary.json');assert summary['source']=='5c608d4a642731a665feab7acbcd1feb8439461b'and summary['cases']==37
  before[name]=dict(root=str(folder),summary_SHA256=sha(folder/'summary.json'),source=summary['source'])
  for path in sorted(folder.glob('*/*/*-producer.json')):
   p,export=binding(path,summary);k=path.name.split('-producer')[0]
   count=0
   for r in rows(export)if k!='flows'else[]:
    count+=1
    if k=='events':assert r.get('profile')!='ospfv2-bounded-control'
    if k=='yak'and(p['capture_SHA256'],r.get('number'))in targets:
     fact=targets[p['capture_SHA256'],r['number']]
     e=contract.event(1,fact);expected=contract.cli_row(r,1,fact)
     witnesses.append(dict(capture_SHA256=p['capture_SHA256'],frame=r['number'],native_placeholder_ID1_SHA256=hashlib.sha256(canonical(e)).hexdigest(),CLI_placeholder_ID1_SHA256=hashlib.sha256(canonical(expected)).hexdigest()))
   cases.append(dict(mode=name,kind=k,relative=export.relative_to(folder).as_posix(),rows=count,producer_relative=path.relative_to(folder).as_posix(),producer_SHA256=sha(path),export_SHA256=sha(export),capture=p['capture'],capture_SHA256=p['capture_SHA256'],arguments=p['arguments'][1:],keylog_used=p['keylog_used']))
 assert len(witnesses)==556 and len(cases)==76
 copied={}
 for src,name in [(Path(__file__),'ospfv2_whole_bridge41.py'),(ROOT/'ospfv2_source_contract41.py','ospfv2_source_contract41.py'),(AUDIT/'known_pdu_reference_paths.py','known_pdu_reference_paths.py'),(AUDIT/'known_pdu_reference_paths_v2.py','known_pdu_reference_paths_v2.py')]:
  shutil.copyfile(src,frozen/name);copied[name]=sha(frozen/name);assert copied[name]==sha(src)
 write(a.plan/'cases.json',cases);write(a.plan/'source-construction-witnesses.json',witnesses)
 p=dict(scope=__doc__,source_grids=grids,source_summary_SHA256=sha(source_dir/'summary.json'),before=before,files=copied,cases_SHA256=sha(a.plan/'cases.json'),witnesses_SHA256=sha(a.plan/'source-construction-witnesses.json'),source_additions=556,native_old_objects=sum(c['rows']for c in cases if c['kind']=='events'),CLI_rows=sum(c['rows']for c in cases if c['kind']=='yak'),flow_files=2,no_candidate_output_read=True,flow_statistics_policy='entire flow object must remain literal; no inferred deltas')
 assert p['native_old_objects']==307262 and p['CLI_rows']==384518
 write(a.plan/'plan.json',p);print(json.dumps(dict(plan=str(a.plan/'plan.json'),SHA256=sha(a.plan/'plan.json'),source_constructions=556,native_old_objects=p['native_old_objects'],CLI_rows=p['CLI_rows'])))

def remap_cli(row,mapping,contract):
 out=copy.deepcopy(row)
 def one(v):
  assert type(v)is int and v>=0
  if v==0:return 0
  assert v in mapping;return mapping[v]
 def source(s):
  if s is None:return
  if 'ParentPDU'in s:s['ParentPDU']=one(s['ParentPDU'])
  if s.get('ParentPDUs')is not None:s['ParentPDUs']=[one(v)for v in s['ParentPDUs']]
 for group in('pdus','published_pdus'):
  for e in out.get(group,[]):
   e['id']=one(e['id'])
   if 'response_to'in e:e['response_to']=one(e['response_to'])
   source(e.get('byte_source'));snap=e['field_values'];assert same(e['fields'],contract.field_map(snap));snap['pdu']=one(snap['pdu']);source(snap.get('source'))
   for item in snap.get('values')or[]:
    if item['name']not in('pdu.id','pdu.response_to')and not(e['protocol']=='ntp'and item['name']=='ntp.control.request_pdu'):continue
    prior=item['value'];item['value']=one(prior)
    if 'exact_value'in item:assert item['exact_value']==str(prior);item['exact_value']=str(item['value'])
   e['fields']=contract.field_map(snap)
 return out

def verify(a):
 p=read(a.plan/'plan.json');assert sha(__file__)==p['files']['ospfv2_whole_bridge41.py']
 for name,digest in p['files'].items():assert sha(a.plan/'frozen'/name)==digest
 assert sha(a.plan/'cases.json')==p['cases_SHA256']and sha(a.plan/'source-construction-witnesses.json')==p['witnesses_SHA256']
 targets=selected(p);contract=load('ospfv2_source_expected',a.plan/'frozen/ospfv2_source_contract41.py');refs=load('ospfv2_public_generated_refs',a.plan/'frozen/known_pdu_reference_paths_v2.py')
 a.out.mkdir(exist_ok=False);ledgers={};new_ledgers={};checks=[];stats=collections.Counter()
 cases=read(a.plan/'cases.json')
 for case in sorted(cases,key=lambda r:(r['mode']!='native',r['relative'])):
  mode=case['mode'];oldroot=Path(p['before'][mode]['root']);after=a.native if mode=='native'else a.cli
  assert sha(oldroot/'summary.json')==p['before'][mode]['summary_SHA256']
  old=oldroot/case['relative'];new=after/case['relative'];op=oldroot/case['producer_relative'];np=after/case['producer_relative']
  assert sha(old)==case['export_SHA256']and sha(op)==case['producer_SHA256']
  producer,export=binding(np,read(after/'summary.json'));assert export==new and producer['source']==a.source
  for key in('capture','capture_SHA256','keylog_used'):assert producer[key]==case[key]
  assert producer['arguments'][1:]==case['arguments']
  selected_rows={f:r for(c,f),r in targets.items()if c==case['capture_SHA256']};relative=Path(case['relative']).parent.as_posix();kind=case['kind']
  if kind=='flows':assert old.read_bytes()==new.read_bytes(),'flow object changed';stats['whole_literal_flow_files']+=1
  elif kind=='events':
   mapping={};added={};seen=set();left=iter(rows(old));pending=sorted(selected_rows)
   for e in rows(new):
    identifier=e['id'];assert type(identifier)is int and identifier>0 and identifier not in seen;seen.add(identifier)
    if e.get('profile')=='ospfv2-bounded-control':
     rs=e['source_bytes']['PacketRefs'];assert len(rs)==1;frame=rs[0]['Number'];assert frame in selected_rows and frame not in added
     assert frame==pending[len(added)];assert same(contract.event(identifier,selected_rows[frame]),e),('source-constructed native differs',relative,frame)
     added[frame]=identifier;stats['source_new_native_objects']+=1
    else:
     before=next(left,None);assert before is not None and before['id']not in mapping
     # Complete original source identity must pair before any ID substitution.
     for key in('protocol','transport','profile','status','raw','domain','flow','source','destination'):assert same(before.get(key),e.get(key)),('retained identity/order',relative,key)
     for key in('Kind','SHA256','PacketRefs','CompletedIn'):assert same(before['source_bytes'].get(key),e['source_bytes'].get(key))
     mapping[before['id']]=identifier
   assert next(left,None)is None and len(mapping)==case['rows']and len(set(mapping.values()))==len(mapping)and set(added)==set(selected_rows)
   left=iter(rows(old))
   for e in rows(new):
    if e.get('profile')=='ospfv2-bounded-control':continue
    assert same(refs.remap_known_pdu_references(next(left),mapping),e),('unplanned retained whole event',relative,e['id'])
    stats['retained_native_whole_objects']+=1
   if not selected_rows:assert old.read_bytes()==new.read_bytes();stats['whole_literal_native_exports']+=1
   ledgers[relative]=mapping;new_ledgers[relative]=added
   write(a.out/(relative.replace('/','-')+'-ID-ledgers.json'),dict(old_to_new=mapping,source_frame_to_new=added))
  else:
   assert kind=='yak'and relative in ledgers;mapping=ledgers[relative];added=new_ledgers[relative];found=set()
   for before,e in itertools.zip_longest(rows(old),rows(new)):
    assert before is not None and e is not None,'CLI record count changed'
    frame=before.get('number')
    if frame in selected_rows:
     assert frame not in found;found.add(frame);assert same(contract.cli_row(before,added[frame],selected_rows[frame]),e),('source-constructed CLI differs',relative,frame)
     stats['source_new_CLI_rows']+=1
    else:assert same(remap_cli(before,mapping,contract),e),('unplanned whole CLI row',relative,frame);stats['retained_CLI_whole_rows']+=1
   assert found==set(selected_rows)
   if not selected_rows:assert old.read_bytes()==new.read_bytes();stats['whole_literal_CLI_exports']+=1
  checks.append(dict(mode=mode,relative=case['relative'],export_SHA256=sha(new),producer_SHA256=sha(np),selected_source_frames=len(selected_rows)))
  print(json.dumps(dict(mode=mode,relative=case['relative'],passed=True)),flush=True)
 assert stats['source_new_native_objects']==stats['source_new_CLI_rows']==556 and stats['retained_native_whole_objects']==307262 and stats['retained_CLI_whole_rows']==383962 and stats['whole_literal_flow_files']==2
 result=dict(scope=__doc__,source=a.source,plan_SHA256=sha(a.plan/'plan.json'),counts=dict(stats),unplanned_differences=0,generated_ID_substitution_only=True,checks=checks)
 write(a.out/'summary.json',result);print(json.dumps(dict(summary=str(a.out/'summary.json'),SHA256=sha(a.out/'summary.json'),counts=dict(stats),differences=0)))

def main():
 ap=argparse.ArgumentParser(description=__doc__);ap.add_argument('mode',choices=['prepare','verify']);ap.add_argument('--plan',type=Path,required=True)
 for name in('native','cli','out'):ap.add_argument('--'+name,type=Path)
 ap.add_argument('--source');a=ap.parse_args();(prepare if a.mode=='prepare'else verify)(a)
if __name__=='__main__':main()
