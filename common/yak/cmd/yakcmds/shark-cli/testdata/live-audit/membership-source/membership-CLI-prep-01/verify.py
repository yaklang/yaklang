"""Strict37-case whole CLI bridge using pre-runtime388-row source expectations."""
import argparse,collections,copy,hashlib,importlib.util,itertools,json,sys
from pathlib import Path
from membership_cli_expected import same,expected_row

def sha(p):
 h=hashlib.sha256()
 with Path(p).open('rb')as f:
  for x in iter(lambda:f.read(1048576),b''):h.update(x)
 return h.hexdigest()
def rows(p):
 with Path(p).open(encoding='utf-8-sig')as f:
  for line in f:yield json.loads(line)
def module(name,p):
 s=importlib.util.spec_from_file_location(name,p);m=importlib.util.module_from_spec(s);s.loader.exec_module(m);return m

def changes(a,b,path=''):
 if type(a)is not type(b):return[path+' [type]']
 if isinstance(a,dict):
  result=[]
  for k in sorted(a.keys()|b.keys()):result +=[path+'/'+k+' [missing]']if k not in a or k not in b else changes(a[k],b[k],path+'/'+k)
  return result
 if isinstance(a,list):
  result=[]if len(a)==len(b)else[path+' [length]']
  for i,(x,y)in enumerate(zip(a,b)):result+=changes(x,y,path+'/'+str(i))
  return result
 return[]if a==b else[path]

def main():
 p=argparse.ArgumentParser(description=__doc__)
 for n in('before-cli','after-cli','native-proof','native-plan','cli-plan','igmp-modules','mld-modules','carrier','audit-modules','capture-weber','capture-sharon','bins','out'):p.add_argument('--'+n,type=Path,required=True)
 p.add_argument('--source-after-cli',required=True);a=p.parse_args();a.out.mkdir(exist_ok=False)
 plan=json.loads((a.cli_plan/'summary.json').read_text());nativeproof=json.loads((a.native_proof/'summary.json').read_text())
 assert plan['candidate_CLI_consumed']is False and plan['native_whole_proof_SHA256']==sha(a.native_proof/'summary.json')and nativeproof['whole_object_differences']==0
 assert plan['native_plan_SHA256']==sha(a.native_plan/'summary.json')and nativeproof['source_after']==plan['source_cli']
 for path,checksum in plan['source_files'].items():assert sha(Path(path))==checksum,('frozen relevant source changed',path)
 sys.path[:0]=[str(a.igmp_modules),str(a.mld_modules),str(a.audit_modules)]
 from vxlan_endpoint_projection_audit import capture_records
 igmp=module('igmpExpected',a.igmp_modules/'igmp_native_expected.py');mld=module('mldExpected',a.mld_modules/'mld_native_expected.py');carrier=module('sourceCarrier',a.carrier);refs=module('CLIreferences',a.audit_modules/'known_cli_pdu_reference_paths.py')
 binary=json.loads((a.bins/'yak-receipt.json').read_text());assert binary['source']==a.source_after_cli and binary['exit_code']==0 and binary['binary_SHA256']==sha(a.bins/'yak.exe')
 assert(a.after_cli/'summary.json').is_file(),'no closed whole replay summary'
 targets={};records={};groups={}
 for name,cap in [('weber',a.capture_weber),('sharon',a.capture_sharon)]:
  groups[name]={f:{x['frame']:x for x in json.loads((a.native_plan/name/(f.upper()+'-source-plan.json')).read_text())}for f in('igmp','mld')}
  targets[name]={x['frame']:x for x in rows(a.cli_plan/name/'membership-rows.jsonl')};records[name]={n:r for n,r in enumerate(capture_records(cap),1)if n in targets[name]}
 stats=[];negative=[]
 nativefiles={x['case']:x for x in nativeproof['files']}
 for case in plan['cases']:
  name=case['case'];op=a.before_cli/case['relative_producer'];np=a.after_cli/case['relative_producer'];oldfile=op.with_name('yak.jsonl');newfile=np.with_name('yak.jsonl')
  oldpr,newpr=json.loads(op.read_text()),json.loads(np.read_text());assert sha(op)==case['old_producer_SHA256']and sha(oldfile)==case['old_export_SHA256']
  assert newpr['exit_code']==0 and newpr['source']==a.source_after_cli and newpr['binary_SHA256']==binary['binary_SHA256']and newpr['export_SHA256']==sha(newfile)
  assert oldpr['capture_SHA256']==newpr['capture_SHA256']==case['capture_SHA256']and oldpr['keylog_used']==newpr['keylog_used']and oldpr['arguments'][1:]==newpr['arguments'][1:]
  mapfile=a.native_proof/name/'pdu-map.json';assert sha(mapfile)==case['native_map_SHA256']==nativefiles[name]['map_SHA256'];mapping={int(k):v for k,v in json.loads(mapfile.read_text()).items()};assert all(type(v)is int and v>0 for v in mapping.values())and len(set(mapping.values()))==len(mapping)
  selected=targets.get(name,{});seen=set();difference=[];checked=collections.Counter();positive=None;mapped_identifiers=set(mapping.values())
  assert sha(a.cli_plan/name/'membership-rows.jsonl')==case['membership_plan_SHA256']
  if not selected:assert sha(oldfile)==sha(newfile)and all(k==v for k,v in mapping.items()),'unplanned small scope must be whole-byte literal with identity IDs'
  for index,(old,new)in enumerate(itertools.zip_longest(rows(oldfile),rows(newfile)),1):
   assert old is not None and new is not None,'CLI physical/publication row count changed';frame=old.get('number')
   if frame in selected:
    assert frame not in seen;seen.add(frame);t=selected[frame];family=t['family'];source=groups[name][family][frame];r=records[name][frame];r['frame']=frame
    if family=='igmp':event=igmp.event(t['id'],r,source);loc=source['location'];assert t['id']not in mapped_identifiers
    else:assert mapping[source['id']]==t['id'];event,_,_=mld.event(t['id'],r,source,carrier.locate);loc=source['source_profile']
    assert same(event,t['native_event'])and same(loc,t['location'])and hashlib.sha256(r['wire']).hexdigest()==t['wire_SHA256']
    want=expected_row(old,r,loc,event,family);assert same(want,t['expected_row']),'pre-runtime source expectation drift';kind='source-'+family
   else:want=refs.remap_cli_row_pdu_references(old,mapping)if selected else copy.deepcopy(old);kind='known-reference-only'if selected else'whole-byte-literal'
   checked[kind]+=1
   if not same(want,new):difference.append(dict(row=index,frame=frame,kind=kind,paths=changes(want,new),expected=want,actual=new))
   if positive is None and 'number'in want:positive=want
  assert seen==selected.keys()and sum(checked.values())==case['old_rows']
  folder=a.out/name;folder.mkdir();f=folder/'differences.json';f.write_text(json.dumps(difference,indent=2)+'\n');assert not difference,str(f)
  if positive is not None:
   for label,mutate in [('physical-frame',lambda e:e.update(number=e['number']+1)),('source',lambda e:e.update(source='203.0.113.222')),('domain',lambda e:e['capture_domain'].update(Interface=e['capture_domain']['Interface']+1)),('new-public-field',lambda e:e.update(invented=True)),('number-type',lambda e:e.update(number=float(e['number'])))]:
    bad=copy.deepcopy(positive);mutate(bad);assert not same(positive,bad);negative.append(name+'/'+label)
  stats.append(dict(case=name,rows=sum(checked.values()),checks=dict(checked),old_SHA256=sha(oldfile),after_SHA256=sha(newfile),producer_SHA256=sha(np),differences=0))
 summary=dict(scope=__doc__,CLI_source=a.source_after_cli,native_source=nativeproof['source_after'],native_proof_SHA256=sha(a.native_proof/'summary.json'),CLI_plan_SHA256=sha(a.cli_plan/'summary.json'),binary_receipt_SHA256=sha(a.bins/'yak-receipt.json'),auditor_SHA256=sha(Path(__file__)),files=stats,rows=sum(x['rows']for x in stats),whole_object_differences=0,negative_controls=negative)
 f=a.out/'summary.json';f.write_text(json.dumps(summary,indent=2)+'\n');print(json.dumps(dict(summary=str(f),SHA256=sha(f),rows=summary['rows'],differences=0)))
if __name__=='__main__':main()
