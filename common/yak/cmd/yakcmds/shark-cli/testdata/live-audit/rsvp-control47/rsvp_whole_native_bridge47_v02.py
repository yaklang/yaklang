"""Frozen-source whole-native RSVP parent refusal bridge.

Offline prepare/verify only: no binary execution. Source936 diagnostics are
complete DTO constructions; one legitimate new callback279605 is admitted.
All other whole events follow only the explicitly audited generated-PDU-ID
schema. Complete flow/session data is literal except one documented capture
DecodeErrors delta, which does not turn malformed IP parents into successes.
CLI whole-row projection is a separate contract, not silently exempted here.
"""
from pathlib import Path
import argparse,collections,copy,hashlib,importlib.util,json,shutil,sys
ROOT=Path(__file__).parent
REPO=Path(r'C:\Users\V\.codex\worktrees\shark-protocol-infrastructure\yaklang')
AUDIT=REPO/'common/yak/cmd/yakcmds/shark-cli/testdata/live-audit'
CAPTURE_SHA='aa4fcc81af216e2b788171c64271b43681974d9156aa9ddbdc1a7a25e730714c'
FACTORY_SHA='7885952e38f4561bd058ce6320060e1a5efacc0dc73554bfb7a93943636a4d12'
_hashcache={}

def sha(path):
 path=Path(path);stat=path.stat();key=(str(path),stat.st_size,stat.st_mtime_ns)
 if key not in _hashcache:
  h=hashlib.sha256()
  with path.open('rb')as f:
   for b in iter(lambda:f.read(1048576),b''):h.update(b)
  _hashcache[key]=h.hexdigest()
 return _hashcache[key]
def read(path):return json.loads(Path(path).read_text(encoding='utf-8-sig'))
def rows(path):
 with Path(path).open(encoding='utf-8-sig')as f:
  for line in f:
   if line.strip():yield json.loads(line)
def write(path,obj):
 with Path(path).open('x',encoding='utf-8')as f:json.dump(obj,f,ensure_ascii=False,indent=2);f.write('\n')
def same(a,b):
 if type(a)is not type(b):return False
 if isinstance(a,dict):return a.keys()==b.keys()and all(same(a[k],b[k])for k in a)
 if isinstance(a,list):return len(a)==len(b)and all(same(x,y)for x,y in zip(a,b))
 return a==b
def load(name,path):
 spec=importlib.util.spec_from_file_location(name,path);m=importlib.util.module_from_spec(spec);spec.loader.exec_module(m);return m

def binding(path,summary):
 p=read(path);assert p in summary['outputs']and p['source']==summary['source']and p['exit_code']==0
 kind=path.name.split('-producer')[0];export=path.with_name(kind+('.json'if kind=='flows'else'.jsonl'));binary=Path(p['arguments'][0]);build=read(binary.parent/(kind+'-receipt.json'))
 assert build['source']==p['source']and build['exit_code']==0 and sha(binary)==build['binary_SHA256']==p['binary_SHA256']
 assert sha(p['capture'])==p['capture_SHA256']and sha(export)==p['export_SHA256']
 return p,export

def source_key(event,sourcecases):
 refs=event.get('source_bytes',{}).get('PacketRefs')or[]
 if len(refs)!=1:return None
 ref=refs[0];frame=ref['Number'];case=sourcecases.get(frame)
 if not case:return None
 expected={'Section':case['section'],'Interface':case['interface'],'Encapsulation':''}
 if not same(ref.get('Domain'),expected):return None
 return frame

def literal_export_proof(before,after,expected_rows):
 assert sha(before)==sha(after),'full export differs; no literal fastpath'
 count=sum(1 for _ in rows(after))
 assert count==expected_rows,'literal export row count differs'
 return count

def expected_flows(before,capture_sha,sourcecases):
 result=copy.deepcopy(before)
 if capture_sha==CAPTURE_SHA:
  delta=sum(case['available_payload_bytes']>0 for case in sourcecases.values());assert delta==935
  assert result['stats']['DecodeErrors']==81595
  result['stats']['DecodeErrors']-=delta
  assert result['stats']['DecodeErrors']==80660
 return result

def prepare(a):
 a.plan.mkdir(exist_ok=False);frozen=a.plan/'frozen';frozen.mkdir()
 source_dir=ROOT/'RSVP-parent-diagnostic-source47-02';manifest=read(source_dir/'manifest.json')
 assert sha(source_dir/'manifest.json')=='78d5722c7f428bf1cbbea658289350d973a48ba1cd0a19e798291a437c2d013c'
 for path,d in manifest['inputs'].items():assert sha(path)==d
 for path,d in manifest['outputs'].items():assert sha(path)==d
 sourcecases={row['frame']:row for row in read(source_dir/'source-cases.json')};assert len(sourcecases)==936 and 218724 not in sourcecases and 279605 in sourcecases
 factory_path=ROOT/'rsvp_parent_diagnostic_source47_v02.py';assert sha(factory_path)==FACTORY_SHA
 factory=load('RSVP_source_DTO',factory_path)
 oldroot=ROOT/'runtime-public-native-45';summary=read(oldroot/'summary.json');assert summary['source']=='df30fc4bd909d6489844f8e0d1cd0ab03826f35c'and summary['cases']==37
 cases=[];targets=[]
 for pp in sorted(oldroot.glob('*/*/*-producer.json')):
  prod,export=binding(pp,summary);kind=pp.name.split('-producer')[0];count=0
  if kind=='events':
   seen=set()
   for e in rows(export):
    count+=1
    if prod['capture_SHA256']!=CAPTURE_SHA:continue
    frame=source_key(e,sourcecases)
    if frame is not None:
     assert frame!=279605 and frame not in seen;seen.add(frame)
     assert e['protocol']=='ip'and e['status']=='unrecognized'and e['expert']=='UnsupportedOrMalformedNetworkLayer'and e['profile']=='ip'and e['source']==e['destination']==''and not e['raw']
     assert all(e.get(k,0)==0 for k in('flow','tcp_flow_id','transaction','response_to','direction'))
     targets.append({'frame':frame,'old_PDU_ID':e['id'],'old_object_canonical_SHA256':hashlib.sha256(json.dumps(e,sort_keys=True,separators=(',',':')).encode()).hexdigest(),'source_DTO_placeholder_ID1_SHA256':hashlib.sha256(json.dumps(factory.expected_diagnostic(sourcecases[frame],1),sort_keys=True,separators=(',',':')).encode()).hexdigest()})
   if prod['capture_SHA256']==CAPTURE_SHA:assert len(seen)==935
  else:assert kind=='flows';expected_flows(read(export),prod['capture_SHA256'],sourcecases)
  cases.append({'kind':kind,'relative':export.relative_to(oldroot).as_posix(),'producer_relative':pp.relative_to(oldroot).as_posix(),'producer_SHA256':sha(pp),'export_SHA256':sha(export),'rows':count,'capture':prod['capture'],'capture_SHA256':prod['capture_SHA256'],'arguments':prod['arguments'][1:],'keylog_used':prod['keylog_used']})
 assert len(cases)==39 and len(targets)==935 and sum(c['rows']for c in cases)==307818
 files={}
 for path,name in[(Path(__file__),'rsvp_whole_native_bridge47.py'),(factory_path,'rsvp_parent_diagnostic_source47_v02.py'),(AUDIT/'known_pdu_reference_paths.py','known_pdu_reference_paths.py'),(AUDIT/'known_pdu_reference_paths_v2.py','known_pdu_reference_paths_v2.py')]:
  shutil.copyfile(path,frozen/name);assert sha(path)==sha(frozen/name);files[name]=sha(path)
 write(a.plan/'cases.json',cases);write(a.plan/'old-source-target-witnesses.json',targets)
 write(a.plan/'plan.json',{'scope':__doc__,'before':{'root':str(oldroot),'summary_SHA256':sha(oldroot/'summary.json'),'source':summary['source']},'source_manifest':str(source_dir/'manifest.json'),'source_manifest_SHA256':sha(source_dir/'manifest.json'),'source_cases':str(source_dir/'source-cases.json'),'source_cases_SHA256':sha(source_dir/'source-cases.json'),'files':files,'cases_SHA256':sha(a.plan/'cases.json'),'old_target_witnesses_SHA256':sha(a.plan/'old-source-target-witnesses.json'),'old_events':307818,'retained_whole_events':306883,'source_changed_old_events':935,'source_expected_diagnostics':936,'only_new_source_frame':279605,'whole_flow_objects':2,'flow_delta':{'capture_SHA256':CAPTURE_SHA,'path':['stats','DecodeErrors'],'old':81595,'new':80660,'reason':'935 malformed direct46 parents no longer invoke unsupported child decoder; not repaired IP parents'},'literal_fastpath_policy':'closed producer/binary/capture binding, entire export SHA equal, equal row count; no reference mapper; only changed Sharon uses source+schema ledger','prior_auditor01_failure_preserved':'unnecessary unsupported http3 transaction interpretation for wholeliteral owned/fresh-QUIC export','CLI_not_certified_by_this_native_plan':True,'no_future47_output_read':True})
 print(json.dumps({'plan':str(a.plan/'plan.json'),'SHA256':sha(a.plan/'plan.json'),'old_events':307818,'source_diagnostics':936}))

def validate_plan(plan):
 p=read(plan/'plan.json');assert sha(__file__)==p['files']['rsvp_whole_native_bridge47.py'],'executed checker differs from frozen source';assert sha(plan/'cases.json')==p['cases_SHA256']and sha(plan/'old-source-target-witnesses.json')==p['old_target_witnesses_SHA256']
 for name,d in p['files'].items():assert sha(plan/'frozen'/name)==d
 assert sha(p['source_manifest'])==p['source_manifest_SHA256']and sha(p['source_cases'])==p['source_cases_SHA256']
 sm=read(p['source_manifest'])
 for path,d in sm['inputs'].items():assert sha(path)==d
 for path,d in sm['outputs'].items():assert sha(path)==d
 cases={row['frame']:row for row in read(p['source_cases'])}
 factory=load('RSVP_frozen_expected_DTO',plan/'frozen/rsvp_parent_diagnostic_source47_v02.py');refs=load('RSVP_audited_generated_refs',plan/'frozen/known_pdu_reference_paths_v2.py')
 return p,cases,factory,refs

def verify(a):
 p,sources,factory,refs=validate_plan(a.plan);a.out.mkdir(exist_ok=False);oldroot=Path(p['before']['root']);assert sha(oldroot/'summary.json')==p['before']['summary_SHA256'];after_summary=read(a.native/'summary.json');assert after_summary['source']==a.source and after_summary['cases']==37
 totals=collections.Counter();checks=[]
 for case in read(a.plan/'cases.json'):
  old=oldroot/case['relative'];new=a.native/case['relative'];assert sha(old)==case['export_SHA256']and sha(oldroot/case['producer_relative'])==case['producer_SHA256']
  prod,export=binding(a.native/case['producer_relative'],after_summary);assert export==new and prod['source']==a.source
  for key in('capture','capture_SHA256','keylog_used'):assert prod[key]==case[key]
  assert prod['arguments'][1:]==case['arguments']
  if case['kind']=='flows':
   assert same(expected_flows(read(old),case['capture_SHA256'],sources),read(new)),'unplanned whole flow/session/statistics change'
   totals['whole_flow_objects_checked']+=1
   if case['capture_SHA256']!=CAPTURE_SHA:assert old.read_bytes()==new.read_bytes();totals['whole_literal_flow_exports']+=1
   else:totals['explicit_source_explained_capture_counter_delta']+=1
  else:
   selected=case['capture_SHA256']==CAPTURE_SHA
   if sha(old)==sha(new):
    assert not selected,'required source936 change cannot pass as unchanged Sharon'
    count=literal_export_proof(old,new,case['rows'])
    totals['retained_whole_native_objects']+=count;totals['whole_literal_native_exports']+=1
    relative=Path(case['relative']).parent.as_posix()
    write(a.out/(relative.replace('/','-')+'-ID-ledger.json'),{'old_to_new':{},'new_source_frames_to_ID':{},'literal_export_identity':True,'rows':count,'whole_old_export_SHA256':sha(old),'whole_new_export_SHA256':sha(new),'unknown_transaction_semantics_not_interpreted':True})
    checks.append({'relative':case['relative'],'new_export_SHA256':sha(new),'new_producer_SHA256':sha(a.native/case['producer_relative']),'proof':'closed bound producer + complete file SHA identity + equal row count; no ID mapper called'})
    print(json.dumps({'relative':case['relative'],'passed':True,'whole_literal_fastpath':True}),flush=True)
    continue
   assert selected,'non-Sharon whole export changed outside frozen source scope'
   mapping={};seen=set();source_seen=set();added={};left=iter(rows(old));unchanged=0;modified=0
   for event in rows(new):
    identifier=event['id'];assert type(identifier)is int and identifier>0 and identifier not in seen;seen.add(identifier)
    frame=source_key(event,sources)if selected else None
    if frame is not None:
     assert frame not in source_seen;source_seen.add(frame)
     assert same(factory.expected_diagnostic(sources[frame],identifier),event),('source-constructed whole diagnostic',frame)
     if frame==279605:added[frame]=identifier;totals['source_new_whole_diagnostic']+=1;continue
     before=next(left,None);assert before is not None and source_key(before,sources)==frame,'target source identity/order mismatch'
     assert before['protocol']=='ip'and before['status']=='unrecognized'and before['expert']=='UnsupportedOrMalformedNetworkLayer'
     mapping[before['id']]=identifier;modified+=1
    else:
     before=next(left,None);assert before is not None and before['id']not in mapping
     assert not(selected and source_key(before,sources)is not None),'missing source diagnostic'
     for key in('protocol','transport','profile','status','raw','domain','flow','source','destination'):assert same(before.get(key),event.get(key)),('retained original identity/order',case['relative'],key)
     for key in('Kind','SHA256','PacketRefs','CompletedIn'):assert same(before['source_bytes'].get(key),event['source_bytes'].get(key))
     mapping[before['id']]=identifier;unchanged+=1
   assert next(left,None)is None and len(mapping)==case['rows']and len(set(mapping.values()))==len(mapping)
   if selected:assert source_seen==set(sources)and set(added)=={279605}and modified==935
   else:assert not source_seen and not added
   # Second pass: every retained complete object uses only the exact generated
   # reference schema. No exception for unknown keys, endpoints, wire integers,
   # flow IDs, source frames, timestamps or opaque fields.
   left=iter(rows(old))
   for event in rows(new):
    frame=source_key(event,sources)if selected else None
    if frame==279605 and selected:continue
    before=next(left)
    if frame is not None:continue
    assert same(refs.remap_known_pdu_references(before,mapping),event),('unplanned whole retained event',case['relative'],event['id'])
   assert next(left,None)is None
   totals['retained_whole_native_objects']+=unchanged;totals['source_changed_whole_old_diagnostics']+=modified;totals['source_whole_diagnostics_checked']+=len(source_seen)
   if not selected:assert old.read_bytes()==new.read_bytes();totals['whole_literal_native_exports']+=1
   relative=Path(case['relative']).parent.as_posix();write(a.out/(relative.replace('/','-')+'-ID-ledger.json'),{'old_to_new':mapping,'new_source_frames_to_ID':added})
  checks.append({'relative':case['relative'],'new_export_SHA256':sha(new),'new_producer_SHA256':sha(a.native/case['producer_relative'])});print(json.dumps({'relative':case['relative'],'passed':True}),flush=True)
 assert totals['retained_whole_native_objects']==306883 and totals['source_changed_whole_old_diagnostics']==935 and totals['source_new_whole_diagnostic']==1 and totals['source_whole_diagnostics_checked']==936 and totals['whole_flow_objects_checked']==2
 validate_plan(a.plan)
 write(a.out/'summary.json',{'source':a.source,'plan_SHA256':sha(a.plan/'plan.json'),'counts':dict(totals),'unplanned_differences':0,'checks':checks,'only_explicit_generated_ID_schema_substitution':True,'935_bad_IP_parents_not_repaired':True,'CLI_whole_rows_not_claimed':True})
 print(json.dumps({'summary':str(a.out/'summary.json'),'SHA256':sha(a.out/'summary.json'),'counts':dict(totals)}))

def selftest(a):
 p,sources,factory,refs=validate_plan(a.plan);a.out.mkdir(exist_ok=False);count=0
 for frame,case in sources.items():
  event=factory.expected_diagnostic(case,1);assert event['protocol']=='ip'and event['status']=='malformed'and event['session']['Child Header Identity']=='unconfirmed'and not event['raw'];assert not any(v['name'].startswith('rsvp.')for v in event['field_values']['values']);assert all(event[k]==0 for k in('flow','transaction','response_to','direction'));count+=1
 demo={'id':7,'protocol':'ip','flow':7,'source_bytes':{'ParentPDU':0,'PacketRefs':[{'Number':7}]},'field_values':{'pdu':7,'source':{'ParentPDU':0,'PacketRefs':[{'Number':7}]},'values':[{'name':'pdu.id','value':7,'exact_value':'7'},{'name':'flow.id','value':7,'exact_value':'7'},{'name':'wire.arbitrary','value':7}]},'fields':{'wire_identifier':7},'session':{'Sequence':7}}
 mapped=refs.remap_known_pdu_references(demo,{7:17});assert mapped['id']==17 and mapped['field_values']['pdu']==17 and mapped['field_values']['values'][0]['value']==17;assert mapped['flow']==7 and mapped['source_bytes']['PacketRefs'][0]['Number']==7 and mapped['fields']==demo['fields']and mapped['session']==demo['session']and mapped['field_values']['values'][1:]==demo['field_values']['values'][1:]
 example=factory.expected_diagnostic(sources[279605],1);negatives=[]
 for name,key,value in [('false child protocol','protocol','rsvp'),('inherited flow','flow',1),('wrong wire source','source','forged'),('unknown extra key','unspecified_new_metadata',True)]:
  changed=copy.deepcopy(example);changed[key]=value;assert not same(changed,example);negatives.append(name)
 before={'stats':{'DecodeErrors':81595,'CapturedPackets':328127},'flows':[{'flow_id':7,'session':{'Sequence':7}}]};after=expected_flows(before,CAPTURE_SHA,sources);assert after['stats']['DecodeErrors']==80660 and before['stats']['DecodeErrors']==81595 and after['flows']==before['flows'];changed=copy.deepcopy(after);changed['flows'][0]['session']['Sequence']=8;assert not same(after,changed);negatives.append('flow session change cannot hide under counter delta')
 # Synthetic unknown transaction semantics remain untouched when the entire
 # file is proved literal. A changed transaction cannot use that fastpath.
 unknown={'id':7,'protocol':'http3','transaction':7,'wire_value':7}
 line=json.dumps(unknown,sort_keys=True).encode()+b'\n'
 first=a.out/'unknown-transaction-old.jsonl';second=a.out/'unknown-transaction-literal.jsonl';changedfile=a.out/'unknown-transaction-mutated.jsonl'
 first.write_bytes(line);second.write_bytes(line)
 assert literal_export_proof(first,second,1)==1
 mutated=copy.deepcopy(unknown);mutated['transaction']=8;changedfile.write_bytes(json.dumps(mutated,sort_keys=True).encode()+b'\n')
 try:literal_export_proof(first,changedfile,1)
 except AssertionError:negatives.append('unknown transaction change cannot wholeliteral-fastpass')
 else:raise AssertionError('changed unknown transaction fastpassed')
 write(a.out/'summary.json',{'plan_SHA256':sha(a.plan/'plan.json'),'unknown_transaction_wholeliteral_without_semantic_interpretation':True,'source_DTOs_checked':count,'explicit_ID_schema_keeps_wire_frames_flows_literal':True,'negative_mutations_rejected':negatives,'candidate47_output_not_read':True,'actual_native_CLI_pass_not_claimed':True})
 print(json.dumps({'selftest':str(a.out/'summary.json'),'SHA256':sha(a.out/'summary.json'),'source_DTOs':count,'negative_mutations':len(negatives)}))

def main():
 ap=argparse.ArgumentParser(description=__doc__);ap.add_argument('mode',choices=['prepare','verify','selftest']);ap.add_argument('--plan',type=Path,required=True);ap.add_argument('--native',type=Path);ap.add_argument('--source');ap.add_argument('--out',type=Path);a=ap.parse_args()
 if a.mode=='prepare':prepare(a)
 elif a.mode=='selftest':assert a.out;selftest(a)
 else:assert a.native and a.source and a.out;verify(a)
if __name__=='__main__':main()