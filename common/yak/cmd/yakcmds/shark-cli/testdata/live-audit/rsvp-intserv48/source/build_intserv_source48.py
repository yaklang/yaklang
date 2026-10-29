"""Freeze independent real16 + authored Intserv source48 observations.

Offline Python source preparation only. Original source46 files are immutable.
No Go/candidate SDK/TShark output is imported. Probe inputs are full RSVP PDUs;
expected observations are neutral object subtrees, not a product API schema.
"""
from pathlib import Path
import argparse,collections,copy,hashlib,importlib.util,json,struct
ROOT=Path(__file__).parent;PHASE=ROOT.parent

def sha(p):return hashlib.sha256(Path(p).read_bytes()).hexdigest()
def load(p,name):
 s=importlib.util.spec_from_file_location(name,p);m=importlib.util.module_from_spec(s);s.loader.exec_module(m);return m
def read(p):return json.loads(Path(p).read_text(encoding='utf-8-sig'))
def put(p,x):
 with Path(p).open('x',encoding='utf-8')as f:json.dump(x,f,ensure_ascii=False,indent=2,allow_nan=False);f.write('\n')
def canonical(x):return json.dumps(x,ensure_ascii=False,sort_keys=True,separators=(',',':'),allow_nan=False).encode()
def u32(x):return struct.pack('!I',x)
def param(n,data,flags=0,words=None):
 assert len(data)%4==0;return struct.pack('!BBH',n,flags,len(data)//4 if words is None else words)+data
def service(n,data,flags=0,words=None):
 assert len(data)%4==0;return struct.pack('!BBH',n,flags,len(data)//4 if words is None else words)+data
def message(data,version=0,words=None):
 assert len(data)%4==0;return struct.pack('!HH',version<<12,len(data)//4 if words is None else words)+data
def bucket(r=0x45bb8000,b=0x45bb8000,p=0x45bb8000,m=64,M=1500):return b''.join(u32(v)for v in(r,b,p,m,M))
def tspec(**kw):return message(service(1,param(127,bucket(**kw))))
def checksum(w):
 if len(w)%2:w+=b'\0'
 total=sum(struct.unpack('!'+str(len(w)//2)+'H',w))
 while total>>16:total=(total&65535)+(total>>16)
 return(~total)&65535

def run(out):
 assert not out.exists();out.mkdir();(out/'authored-wire').mkdir()
 model=load(ROOT/'intserv_neutral_source48.py','independent_intserv48');old=load(PHASE/'rsvp_intserv_source46.py','immutable_intserv46')
 original=read(PHASE/'RSVP-Intserv-source46-01/source-facts.json')
 before={str(p):sha(p)for p in[Path(__file__),ROOT/'intserv_neutral_source48.py',PHASE/'rsvp_intserv_source46.py',PHASE/'build_RSVP_Intserv_source46.py',PHASE/'RSVP-Intserv-source46-01/manifest.json',PHASE/'RSVP-Intserv-source46-01/source-facts.json',PHASE/'RSVP-Intserv-source46-01/reference-comparison.json',PHASE/'RSVP-Intserv-primary46-01/manifest.json']+[PHASE/'RSVP-Intserv-primary46-01'/f'rfc{n}.txt'for n in(2210,2215,2212)]}
 rows=[];probes=[];bridge=[];totals=collections.Counter();issuecounts=collections.Counter();expected_by_id={}
 def add(case,raw,path,start,end,cls,nested_budget=4095,global_budget=4096,origin='authored',metadata=None):
  source=model.decode(raw,start,end,cls,nested_budget);counts=model.validate_source_leaves(raw,source)
  totals[origin+'_evaluations']+=1
  for k,v in counts.items():totals[origin+'_'+k]+=v
  totals[origin+'_services_and_parameters_observed']+=source['collection_tokens_consumed']
  for issue in source['issues']:issuecounts[origin+':'+issue['kind']+':'+issue['code']]+=1
  row={'sourcecase':case,'origin':origin,'input_path':str(path),'raw_SHA256':hashlib.sha256(raw).hexdigest(),'byte_offset':0,'byte_length':len(raw),'object_class':cls,'object_C_Type':2,'target_object_body_start':start,'target_object_declared_end':end,'neutral_nested_budget':nested_budget,'probe_global_budget':global_budget,'neutral_remaining_budget_not_global_API_value':True,'source':source,'metadata':metadata or{}}
  rows.append(row);expected_by_id[case]=row
  probes.append({k:row[k]for k in('sourcecase','input_path','byte_offset','byte_length','raw_SHA256')}|{'budget':global_budget})
  return row
 for row in original:
  raw=Path(row['PDU_path']).read_bytes();assert hashlib.sha256(raw).hexdigest()==row['PDU_SHA256'];start,end=row['body_range'];cls=row['object_class']
  new=add('real-frame'+str(row['frame'])+'-object'+str(row['object_index']),raw,Path(row['PDU_path']),start,end,cls,origin='real',metadata={k:row[k]for k in('frame','domain','object_index','object_header_range','PDU_physical_start','physical_body_range','body_SHA256')})
  assert model.equal(new['source']['fields'],row['source']['fields']),'original real fields changed'
  old_leaves=sorted(row['source']['leaves'],key=lambda x:x['path']);new_leaves=sorted(new['source']['leaves'],key=lambda x:x['path']);assert model.equal(old_leaves,new_leaves),'original real wire leaves changed'
  bridge.append({'sourcecase':new['sourcecase'],'old_source_fields_SHA256':hashlib.sha256(canonical(row['source']['fields'])).hexdigest(),'new_source_fields_SHA256':hashlib.sha256(canonical(new['source']['fields'])).hexdigest(),'old_and_new_all_fields_literal_equal':True,'all_old_leaf_values_spans_masks_preserved':True})
 def authored(name,body,cls=12,cut=None,nested_budgets=(4095,),suffix=b''):
  obj=struct.pack('!HBB',4+len(body),cls,2)+body
  packet=struct.pack('!BBHBBH',0x10,1,0,64,0,8+len(obj)+len(suffix))+obj+suffix
  packet=packet[:2]+struct.pack('!H',checksum(packet))+packet[4:]
  if cut is not None:packet=packet[:12+cut]
  path=out/'authored-wire'/(name+'.pdu');path.write_bytes(packet)
  for n in nested_budgets:add(name+('-nested-budget'+str(n)if len(nested_budgets)>1 else''),packet,path,12,12+len(body),cls,n,n+1,metadata={'authored_scene':name,'capture_short':cut is not None,'complete_parent_boundary_is_independent_object_header':True,'global_budget_includes_first_outer_object_token':True,'sibling_bytes_not_inside_target_object':bool(suffix)})
 # The matrix is intentionally small and independent, not a Cartesian fuzz set.
 authored('message-header-empty',tspec(),cut=0)
 authored('message-header-one-byte',tspec(),cut=1)
 authored('message-header-three-bytes',tspec(),cut=3)
 authored('service-header-one-byte',tspec(),cut=5)
 authored('service-header-three-bytes',tspec(),cut=7)
 authored('parameter-header-two-bytes',tspec(),cut=10)
 authored('parameter-header-three-bytes',tspec(),cut=11)
 authored('float-prefix-three-bytes',tspec(),cut=15)
 authored('float-prefix-seven-bytes',tspec(),cut=19)
 authored('message-crosses-object-not-time-sibling',message(service(1,b'',words=6),words=7),suffix=struct.pack('!HBBI',8,5,1,30000))
 authored('service-crosses-message',message(service(99,b'ABCD',words=2)),cls=13)
 authored('parameter-crosses-service',message(service(1,param(4,u32(3),words=2))),cls=13)
 authored('selected-parameter-short-fixed-layout',message(service(1,param(127,bucket()[:8],words=2))))
 authored('unknown-service-before-known',message(service(99,b'ABCD')+service(1,param(4,u32(3)))),cls=13)
 authored('guaranteed-service-remains-opaque',message(service(2,param(133,u32(3)))+service(1,param(4,u32(3)))),cls=13)
 authored('unknown-parameter-before-known',message(service(1,param(250,b'ABCD')+param(4,u32(3)))),cls=13)
 authored('future-message-version',message(service(1,param(127,bucket())),version=1))
 authored('r-positive-zero',tspec(r=0))
 authored('b-positive-zero',tspec(b=0))
 authored('r-negative-zero',tspec(r=0x80000000))
 authored('b-nan-retain-payload-bits',tspec(b=0x7fc12345))
 authored('p-positive-infinity',tspec(p=0x7f800000))
 authored('p-negative-infinity',tspec(p=0xff800000))
 authored('p-nan-retain-payload-bits',tspec(p=0x7fa54321))
 authored('flags-are-observed-not-validity',message(service(1,param(127,bucket(),flags=0x81),flags=0x81)))
 authored('controlled-load-empty-service',message(service(5,b'')),cls=13)
 shared=message(service(1,param(4,u32(3))+param(6,u32(0x49989680)))+service(5,b''))
 authored('shared-service-parameter-budget',shared,cls=13,nested_budgets=(0,1,2,3,4))
 authored('general-mtu32-and-indeterminate-latency',message(service(1,param(8,u32(0xffffffff))+param(10,u32(0x12345678)))),cls=13)
 authored('known-parameter-zero-words',message(service(1,param(127,b''))))
 # Specific invariant assertions do not use a candidate implementation.
 for row in rows:
  source=row['source'];raw=Path(row['input_path']).read_bytes();model.check_observation(raw,source,copy.deepcopy(source))
  for leaf in source['leaves']:
   a,b=leaf.get('range',leaf.get('source_range'));assert 12<=a<=b<=row['target_object_declared_end']if row['origin']=='authored'else 0<=a<=b<=row['target_object_declared_end']
 for cid in('r-positive-zero','b-positive-zero'):assert any(x['code']=='token-bucket-must-be-positive'for x in expected_by_id[cid]['source']['issues'])
 assert not expected_by_id['p-positive-infinity']['source']['issues']
 assert not expected_by_id['controlled-load-empty-service']['source']['issues']
 assert expected_by_id['unknown-service-before-known']['source']['wire_envelopes_complete']and not expected_by_id['unknown-service-before-known']['source']['field_layouts_complete']
 assert len(expected_by_id['unknown-service-before-known']['source']['fields']['Services'])==2
 assert expected_by_id['guaranteed-service-remains-opaque']['source']['fields']['Services'][0]['Layout Selected']is False
 assert expected_by_id['shared-service-parameter-budget-nested-budget0']['source']['collection_tokens_consumed']==0
 assert expected_by_id['shared-service-parameter-budget-nested-budget4']['source']['wire_envelopes_complete']
 failures=[]
 def rejects(label,fn):
  try:fn()
  except(AssertionError,ValueError,TypeError):failures.append(label)
  else:raise AssertionError('negative control unexpectedly passed: '+label)
 sample=expected_by_id['p-positive-infinity'];raw=Path(sample['input_path']).read_bytes();expected=sample['source']
 changed=copy.deepcopy(expected);next(x for x in changed['leaves']if x['kind']=='ieee32')['value']['wire_uint32']^=1
 rejects('wrong IEEE32 payload bits',lambda:model.validate_source_leaves(raw,changed))
 changed=copy.deepcopy(expected);next(x for x in changed['leaves']if x['literal'])['range'][0]+=1
 rejects('mis-owned literal range',lambda:model.validate_source_leaves(raw,changed))
 changed=copy.deepcopy(expected);changed['fields']['invented default']=0
 rejects('invented field',lambda:model.check_observation(raw,expected,changed))
 changed=copy.deepcopy(expected);changed['authentication_or_reservation_verified']=True
 rejects('false reservation verification',lambda:model.check_observation(raw,expected,changed))
 changed=copy.deepcopy(expected);changed['fields']['Services'][0]['Parameters'][0]['Body']['Peak Data Rate']['exact_decimal']=float('inf')
 rejects('nonfinite JSON numeric value',lambda:canonical(changed))
 short=expected_by_id['float-prefix-three-bytes'];changed=copy.deepcopy(short['source']);changed['fields']['Services'][0]['Parameters'][0]['Body']['Token Bucket Rate']={'wire_uint32':0,'classification':'positive-zero','exact_decimal':'0','display':'0'}
 rejects('manufactured zero from short float',lambda:model.check_observation(Path(short['input_path']).read_bytes(),short['source'],changed))
 bud=expected_by_id['shared-service-parameter-budget-nested-budget1'];changed=copy.deepcopy(bud['source']);changed['collection_tokens_consumed']=2
 rejects('budget bypass',lambda:model.check_observation(Path(bud['input_path']).read_bytes(),bud['source'],changed))
 put(out/'source-facts.json',rows);put(out/'real46-to48-field-bridge.json',bridge)
 with(out/'probe-input.jsonl').open('x',encoding='utf-8')as f:
  for p in probes:f.write(json.dumps(p,ensure_ascii=False,separators=(',',':'))+'\n')
 put(out/'matrix-and-policy.json',{'scope':'neutral source observations, not a product API or runtime pass','selected':{'12':{'1':[127]},'9':{'5':[127]},'13':{'1':[4,6,8,10],'5':[4,6,8,10]}},'unselected_service2':'complete header and owned opaque data; no Guaranteed service parameter semantics claimed','word_lengths':'message/service/parameter each counts data32bitwords excluding its own4B header','error_classes':['format','shortage','budget','context','semantic'],'opaque_is_not_automatically_malformed':True,'budget':'one shared token per service or parameter; header/scalar costs0; authored global RSVP budget is nested+1 outer object token','float':'owned IEEE32 unsigned bits + exact decimal/classification strings; no JSON numeric NaN/Inf','domain':'r/b representation is finite nonnegative excluding negative-zero; semantic positive requirement is separate; p may +Infinity; Guaranteed-specific exponent/rate rules NOT applied to this selected subset','unknown_validity':'I is observed reserved-future marker, not current validity proof. BW0 unknown, latency0xffffffff indeterminate, MTU stays uint32. No installed reservation/path authentication inferred.','partial':'complete primitives preserved; trailing partial bytes owned at deepest proved enclosing boundary; no borrowing a sibling to finish fields','source46_bridge':'all16 complete source field trees and every old literal/derived value/span/mask equal; original46 files untouched','no_candidate_implementation_or_outputs_read':True,'source_primary':[{'RFC':2210,'URL':'https://www.rfc-editor.org/rfc/rfc2210.html','sections':['3.1','3.2.1','3.3.2','3.3.4','3.3.5','A1.1','A1.2','A1.3','A1.4']},{'RFC':2215,'URL':'https://www.rfc-editor.org/rfc/rfc2215.html','sections':['1','2','3.3','3.4','3.5','3.6']},{'RFC':2212,'URL':'https://www.rfc-editor.org/rfc/rfc2212.html','use':'Guaranteed service is unselected; its service-specific constraints must not leak into current selected namespaces'}]})
 assert before=={p:sha(p)for p in before}
 put(out/'manifest.json',{'inputs':before,'outputs':{p.name:sha(p)for p in out.iterdir()if p.is_file()},'authored_wire_files':{p.name:sha(p)for p in(out/'authored-wire').iterdir()},'no_Go_native_CLI_TShark_execution':True,'no_candidate_implementation_or_values_read':True,'original_source46_unchanged':True})
 put(out/'selftest-summary.json',{'negative_controls_rejected':failures,'all_real16_whole_neutral_field_trees_equal_source46':True,'original_real_literal_leaves':totals['real_literal_leaves'],'original_real_derived_leaves':totals['real_derived_leaves'],'independent_float_value_check':'struct.unpack binary32 widened exactly to binary64, Decimal.from_float exact equality; nonfinite classified separately','no_product_pass_claimed':True})
 put(out/'summary.json',{'manifest_SHA256':sha(out/'manifest.json'),'counts':dict(totals),'issue_counts':dict(issuecounts),'probe_evaluations':len(probes),'real_source_objects':16,'real_physical_frames':len({r['metadata']['frame']for r in rows if r['origin']=='real'}),'real_distinct_PDU_SHA256':len({r['raw_SHA256']for r in rows if r['origin']=='real'}),'authored_scenes':len(list((out/'authored-wire').iterdir())),'negative_controls':len(failures),'all34_original_IEEE_values_bits_and_exact_decimal_unchanged':True,'source_only_no_product_pass':True})
 print(json.dumps({'out':str(out),'summary_SHA256':sha(out/'summary.json'),'manifest_SHA256':sha(out/'manifest.json'),'selftest_SHA256':sha(out/'selftest-summary.json'),'counts':dict(totals),'probe_evaluations':len(probes)}))

if __name__=='__main__':
 p=argparse.ArgumentParser();p.add_argument('--out',type=Path,required=True);run(p.parse_args().out)