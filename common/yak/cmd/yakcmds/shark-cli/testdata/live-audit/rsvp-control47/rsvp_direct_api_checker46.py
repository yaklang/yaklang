"""Frozen independent RSVP direct API per-leaf validator.

No Go/candidate parser or runtime is used to prepare expectations. It consumes
only the independently frozen official baseline and authored-boundary facts.
All full plans/expected trees/captures remain external. This Python source has
no embedded real capture wire, secret, or native expected-object export.
"""
import argparse
import base64
import collections
import copy
import hashlib
import importlib.util
import json
from pathlib import Path
import re

WR='Wire Field Ranges'
WIRE_NAMES={'Version and Flags','Message Type','Checksum','Send TTL','Reserved','Message Length','Version','Flags','Length','Class','C-Type','Destination Address','Protocol ID','Destination Port','Hop Address','Logical Interface Handle','Refresh Period Milliseconds','Error Node Address','Error Code','Error Value','Option Vector','Source Address','Unused','Source Port','Receiver Address','Address','Opaque Body','Unparsed Body','Trailing Bytes','Unselected Address Tail'}
NO_FALSE_TRUE={'Authentication Verified','Procedure Semantics Verified','Reservation State Inferred'}


def digest(data):return hashlib.sha256(data).hexdigest()
def sha(path):return digest(Path(path).read_bytes())
def read(path):return json.loads(Path(path).read_text(encoding='utf-8-sig'))
def put(path,obj):Path(path).write_text(json.dumps(obj,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
def load_module(path):
 spec=importlib.util.spec_from_file_location('frozen_rsvp_source_model',path)
 mod=importlib.util.module_from_spec(spec);spec.loader.exec_module(mod);return mod

def tokens(path):
 return [int(b) if b else a for a,b in re.findall(r'([^\.\[\]]+)|\[(\d+)\]',path) if a or b]

def node(fields,path):
 parts=tokens(path)
 if parts and parts[0]=='Root':parts=parts[1:]
 value=fields
 for part in parts:value=value[part]
 return value

def map_path(path,key):return (path+'.' if path else 'Root.')+key

def walk_maps(value,path=''):
 if not isinstance(value,dict):return
 yield path,value
 for key,item in value.items():
  if key==WR:continue
  newpath=(path+'.'+key)if path else key
  if isinstance(item,dict):yield from walk_maps(item,newpath)
  elif isinstance(item,list):
   for index,child in enumerate(item):
    if isinstance(child,dict):yield from walk_maps(child,newpath+'['+str(index)+']')


def verify_fields(expected,raw,actual):
 assert isinstance(actual,dict),'fields must retain the observed map even on error'
 assert digest(raw)==expected['raw_SHA256']
 expected_tree=expected['fields']
 source_leaves=expected['leaves']
 literal={leaf['path']:leaf for leaf in source_leaves if leaf['literal']}
 derived={leaf['path']:leaf for leaf in source_leaves if not leaf['literal']}
 assert len(literal)+len(derived)==len(source_leaves)
 ranges={}
 metadata=0
 for path,item in walk_maps(actual):
  reference={}
  try:reference=node(expected_tree,path)if path else expected_tree
  except(KeyError,IndexError,TypeError):pass
  if not isinstance(reference,dict):reference={}
  for key in WIRE_NAMES:
   if key in item:assert key in reference,'invented or mis-owned primitive '+map_path(path,key)
  for name,value in item.get(WR,{}).items():
   assert name in item,'range has no owned literal value'
   full=map_path(path,name)
   assert full not in ranges,'duplicate literal path'
   assert isinstance(value,list)and len(value)==2 and all(type(x)is int for x in value),'range shape'
   assert 0<=value[0]<=value[1]<=len(raw),'range outside captured PDU'
   ranges[full]=(item[name],value)
  for key,value in item.items():
   if key not in WIRE_NAMES and key!=WR and not isinstance(value,(dict,list)):metadata+=1
 assert set(ranges)==set(literal),'literal range set mismatch missing='+str(sorted(set(literal)-set(ranges)))+' extra='+str(sorted(set(ranges)-set(literal)))
 for path,leaf in literal.items():
  value,span=ranges[path]
  assert span==leaf['range'],'range mismatch '+path
  start,end=span
  assert digest(raw[start:end])==leaf['raw_SHA256'],'source leaf SHA mismatch'
  if leaf['kind']=='uint':
   assert type(value)is int and value>=0 and value==leaf['value']==int.from_bytes(raw[start:end],'big'),'unsigned literal '+path
  elif leaf['kind']=='address':
   assert isinstance(value,str)and value==leaf['value'],'address literal '+path
  elif leaf['kind']=='bytes':
   assert isinstance(value,str)and base64.b64decode(value,validate=True)==raw[start:end]and value==leaf['value'],'owned bytes '+path
  else:raise AssertionError('unselected source kind')
 for path,leaf in derived.items():
  value=node(actual,path)
  start,end=leaf['source_range']
  proved=(int.from_bytes(raw[start:end],'big')&leaf['mask'])>>leaf['shift']
  assert type(value)is int and value==leaf['value']==proved,'derived bit field '+path
 # Exact retained collection shape; absent primitive fields cannot be created
 # without ranges to escape the literal-range check above.
 for path,item in walk_maps(expected_tree):
  for key in ('Objects','Addresses'):
   if key not in item:continue
   wanted=item[key]
   try:observed=node(actual,path)[key]if path else actual[key]
   except(KeyError,TypeError):observed=None
   if wanted in(None,[]):assert observed in(None,[]),'nonempty invented collection '+map_path(path,key)
   else:assert isinstance(observed,list)and len(observed)==len(wanted)and all(isinstance(row,dict)for row in observed),'collection shape '+map_path(path,key)
 if expected['required_categories']['is_context']:
  assert 'Objects'not in actual,'unselected message must not acquire an Objects chain'
 # Optional metadata cannot prove a wire leaf, but false positive claims are
 # still rejected. Missing/negative completion metadata remains conservative.
 for _,item in walk_maps(actual):
  for name in NO_FALSE_TRUE:
   if name in item:assert item[name]is False,'false semantic verification '+name
 if 'Field Layouts Complete'in actual and expected_tree.get('Field Layouts Complete')is False:
  assert actual['Field Layouts Complete']is False,'false complete layout claim'
 if 'Checksum Verified'in actual:assert actual['Checksum Verified']is expected_tree['Checksum Verified'],'checksum verdict lost or fabricated'
 if 'Checksum Omitted'in actual:
  omitted=expected_tree.get('Checksum Omitted')
  if omitted is None:assert actual['Checksum Omitted']in(None,False),'omission invented without checksum primitive'
  else:assert actual['Checksum Omitted']is omitted,'checksum omission policy'
 hard=any(expected['required_categories'].values())or expected['shortage']
 if hard and actual.get('Wire Grammar Complete')is True:raise AssertionError('wire completeness fabricated after a proved refusal/shortage')
 return {'literal_wire_leaves':len(literal),'derived_bit_values':len(derived),'unverified_metadata_keys':metadata}


def verify_case(expected,raw,budget,fields,error=None,categories=None,check_errors=True):
 assert budget==expected['budget'],'source budget mismatch'
 counts=verify_fields(expected,raw,fields)
 if check_errors:
  assert isinstance(categories,dict)
  for key in ('is_format','is_integrity','is_budget','is_context'):
   assert type(categories.get(key))is bool,'missing explicit bool '+key
  for key in ('is_format','is_integrity','is_budget'):
   assert categories[key]is expected['required_categories'][key],'error identity lost/invented '+key
  if expected['required_categories']['is_context']:assert categories['is_context']is True,'required unselected-message context missing'
  elif categories['is_context']:
   assert expected['fields'].get('Field Layouts Complete')is False,'context refusal invented for wholly selected source'
  if any(categories.values())or expected['shortage']:assert isinstance(error,str)and bool(error.strip()),'proved refusal/shortage requires nonempty err'
  elif expected['fields'].get('Field Layouts Complete')is True:assert not error,'error invented for complete selected wire'
  counts['error_contracts']=1
 return counts


def expected_row(sourcecase,raw_path,budget,source,kind,parent_complete=True):
 raw=Path(raw_path).read_bytes()
 issues=source['issues']
 categories={name:any(i['kind']==kind_ for i in issues)for name,kind_ in [('is_format','format'),('is_integrity','integrity'),('is_budget','budget'),('is_context','context')]}
 return {'sourcecase':sourcecase,'raw_path':str(raw_path),'raw_SHA256':digest(raw),'budget':budget,'kind':kind,
         'fields':copy.deepcopy(source['fields']),'leaves':copy.deepcopy(source['leaves']),
         'source_issue_facts':issues,'required_categories':categories,
         'shortage':any(i['kind']=='need_more'for i in issues),'parent_complete':parent_complete}


def prepare(phase,out):
 assert not out.exists();out.mkdir()
 base=phase/'RSVP-baseline-source45-01'
 boundary=phase/'RSVP-boundary-source45-01'
 modelpath=phase/'rsvp_boundary_source45.py'
 paths=[Path(__file__),base/'source-grid.json',base/'manifest.json',base/'summary.json',
        boundary/'manifest.json',boundary/'source-facts.json',boundary/'extra-budget-evaluation.json',boundary/'source-contract.json',modelpath]
 assert sha(base/'source-grid.json')=='9d31302c649ded298b26474aa98d1e245e2116086ac839bd48355852606262ad'
 assert sha(boundary/'summary.json')=='a0cd5bcfdaec5cd16232ae37c0a7478afe2e601657246ef24f0256602842d927'
 model=load_module(modelpath)
 basefacts=read(base/'source-grid.json');boundaryfacts=read(boundary/'source-facts.json');extra=read(boundary/'extra-budget-evaluation.json')
 paths+=[Path(row['PDU_path'])for row in basefacts+boundaryfacts]+[Path(extra['PDU_path'])]
 before={str(p):sha(p)for p in paths}
 expected=[]
 for row in basefacts:
  raw=Path(row['PDU_path']).read_bytes();source=model.decode_source(raw,4096)
  cleaned=[{k:v for k,v in leaf.items()if k!='reference_alias'}for leaf in row['leaves']]
  assert cleaned==source['leaves']
  expected.append(expected_row('RSVP-official-frame'+str(row['frame']),row['PDU_path'],4096,source,'official actual capture'))
 for row in boundaryfacts:
  expected.append(expected_row(row['sourcecase'],row['PDU_path'],row['budget'],row['source'],'authored synthetic wire',row['carrier']['parent_complete']))
 expected.append(expected_row(extra['sourcecase'],extra['PDU_path'],extra['budget'],extra['source'],'same authored wire budget0'))
 assert len(expected)==34
 # Pure API ownership/name projection requested before any candidate output:
 # a captured partial SCOPE address is a body-owned tail. The budget-refusal
 # case remains the original whole Root.Unparsed Body and is not split/read.
 mapping=[]
 selected=next(row for row in expected if row['sourcecase']=='RSVP-authored-scene18-frame19')
 root=selected['fields'];body=root['Objects'][0]['Body']
 rawvalue=root.pop('Unparsed Body');span=root[WR].pop('Unparsed Body')
 assert span==[28,32]
 body['Unselected Address Tail']=rawvalue;body.setdefault(WR,{})['Unselected Address Tail']=span
 old=next(leaf for leaf in selected['leaves']if leaf['path']=='Root.Unparsed Body')
 oldpath=old['path'];old['path']='Objects[0].Body.Unselected Address Tail'
 mapping.append({'sourcecase':selected['sourcecase'],'old_path':oldpath,'API_path':old['path'],'range':span,'raw_SHA256':old['raw_SHA256'],'bytes_and_value_unchanged':True})
 # The residual for budget2 includes unread SCOPE + sibling TIME bytes.
 budget_scope=next(row for row in expected if row['sourcecase']=='RSVP-authored-scene17-frame18')
 assert budget_scope['fields'][WR]['Unparsed Body']==[16,28]
 assert 'Unselected Address Tail'not in budget_scope['fields']['Objects'][0]['Body']
 inputs=[]
 for row in expected:
  raw=Path(row['raw_path']).read_bytes()
  assert digest(raw)==row['raw_SHA256']
  inputs.append({'sourcecase':row['sourcecase'],'input_path':row['raw_path'],'byte_offset':0,'byte_length':len(raw),'raw_SHA256':row['raw_SHA256'],'budget':row['budget'],'parent_complete':row['parent_complete']})
 put(out/'source-cases.json',expected)
 put(out/'API-ownership-projection.json',mapping)
 (out/'probe-input.jsonl').write_text(''.join(json.dumps(row)+'\n'for row in inputs),encoding='utf-8')
 put(out/'probe-schema.json',{'input_keys':['sourcecase','input_path','byte_offset','byte_length','raw_SHA256','budget','parent_complete'],
 'output_required':['sourcecase','raw_SHA256','budget','fields','error','is_format','is_integrity','is_budget','is_context'],
 'output_types':{'fields':'complete retained direct-codec JSON map','error':'string or null; nonempty on proved error/shortage','is_*':'explicit JSON boolean'},
 'parent_complete':'input provenance only; direct decoder is not expected to simulate native IP ingress',
 'source_values':'frozen source facts only; no Go/candidate/native output read',
 'wire_coverage':'all expected literal names/value/ranges and exact WR set plus known names reject default fields without a range; derived Version/Flags have no WR',
 'metadata':'unverified optional metadata not wire coverage; false positive verification/completeness claims rejected',
 'errors':'required format/integrity/budget exact independent booleans; unknown message context required; full opaque layouts can have no error/context; pure shortage requires nonempty err',
 'not_claimed':['Go reflect.Type','native IP admission/callback','full native event object','private aliases','CLI','real network faults']})
 assert before=={p:sha(p)for p in before}
 put(out/'manifest.json',{'inputs':before,'outputs':{str(p):sha(p)for p in out.iterdir()if p.is_file()},'cases':34,'official':9,'authored_evaluations':25,'checker':str(Path(__file__)),'checker_SHA256':sha(__file__),'no_candidate_source_or_output_read':True,'no_Go_binary_TShark_or_native_execution':True,'frozen_source_facts_unchanged':True})
 # Self-tests use exactly these frozen source projections and synthetic error
 # witnesses, not output from any production implementation.
 totals=collections.Counter()
 for row in expected:
  flags=row['required_categories']
  err='source refusal/shortage witness'if any(flags.values())or row['shortage']else None
  totals.update(verify_case(row,Path(row['raw_path']).read_bytes(),row['budget'],copy.deepcopy(row['fields']),err,dict(flags)))
 failures=[]
 def rejects(label,row,mutator=None,flags=None):
  actual=copy.deepcopy(row['fields'])
  if mutator:mutator(actual)
  cats=dict(row['required_categories'])if flags is None else flags
  err='source self-test refusal'if any(cats.values())or row['shortage']else None
  try:verify_case(row,Path(row['raw_path']).read_bytes(),row['budget'],actual,err,cats)
  except(AssertionError,KeyError,ValueError,TypeError):failures.append(label)
  else:raise AssertionError('self-test accepted '+label)
 rejects('wrong literal scalar',expected[0],lambda tree:tree['Objects'][0]['Body'].update({'Protocol ID':6}))
 rejects('shifted literal checksum range',expected[0],lambda tree:tree['Header'][WR].update({'Checksum':[3,5]}))
 short=next(row for row in expected if row['sourcecase']=='RSVP-authored-scene01-frame01')
 rejects('default Type without any range',short,lambda tree:tree['Header'].update({'Message Type':0}))
 rejects('invented second SCOPE address',budget_scope,lambda tree:tree['Objects'][0]['Body']['Addresses'].append(copy.deepcopy(tree['Objects'][0]['Body']['Addresses'][0])))
 mixed=next(row for row in expected if row['sourcecase']=='RSVP-authored-scene13-frame13')
 flags=dict(mixed['required_categories']);flags['is_integrity']=False
 rejects('integrity masked by budget',mixed,flags=flags)
 unknown=next(row for row in expected if row['sourcecase']=='RSVP-authored-scene14-frame14')
 rejects('manufactured Objects under Bundle',unknown,lambda tree:tree.update({'Objects':None}))
 summary={'manifest_SHA256':sha(out/'manifest.json'),'source_cases_SHA256':sha(out/'source-cases.json'),'probe_input_SHA256':sha(out/'probe-input.jsonl'),'schema_SHA256':sha(out/'probe-schema.json'),'ownership_projection_SHA256':sha(out/'API-ownership-projection.json'),'counts':dict(totals),'source_cases':34,'checker_negative_mutations_rejected':failures,'candidate_run_not_performed':True}
 put(out/'selftest-summary.json',summary)
 print(json.dumps({'plan':str(out),'manifest_SHA256':sha(out/'manifest.json'),'selftest_SHA256':sha(out/'selftest-summary.json'),'counts':dict(totals),'negatives':len(failures)}))


def load_plan(plan):
 manifest=read(Path(plan)/'manifest.json')
 for path,d in manifest['inputs'].items():assert sha(path)==d,'frozen input changed '+path
 for path,d in manifest['outputs'].items():assert sha(path)==d,'frozen projection changed '+path
 rows=read(Path(plan)/'source-cases.json')
 return {row['sourcecase']:row for row in rows},manifest


def verify_export(plan,actual_path,out):
 assert not out.exists();out.mkdir()
 expected,manifest=load_plan(plan)
 before={str(path):sha(path)for path in [Path(actual_path),Path(__file__),Path(plan)/'manifest.json']}
 totals=collections.Counter();seen=set();failed=[]
 for line in Path(actual_path).read_bytes().splitlines():
  if not line.strip():continue
  record=json.loads(line);key=record['sourcecase']
  assert key not in seen and key in expected,'unknown/duplicate sourcecase'
  seen.add(key);row=expected[key]
  try:
   assert record['raw_SHA256']==row['raw_SHA256']and record['budget']==row['budget']
   raw=Path(row['raw_path']).read_bytes()
   counts=verify_case(row,raw,row['budget'],record['fields'],record.get('error'),{key:record[key]for key in ('is_format','is_integrity','is_budget','is_context')})
   totals.update(counts)
  except(Exception)as error:failed.append({'sourcecase':key,'reason':str(error),'exception':type(error).__name__})
 assert seen==set(expected),'missing source cases'
 assert before=={path:sha(path)for path in before}
 load_plan(plan)
 put(out/'differences.json',failed)
 put(out/'summary.json',{'plan_manifest_SHA256':sha(Path(plan)/'manifest.json'),'checker_SHA256':sha(__file__),'actual':str(actual_path),'actual_SHA256':sha(actual_path),'source_cases':len(seen),'counts':dict(totals),'failures':len(failed),'differences_SHA256':sha(out/'differences.json'),'all_frozen_inputs_unchanged':True,'metadata_not_counted_as_wire_proof':True})
 print(json.dumps({'summary':str(out/'summary.json'),'SHA256':sha(out/'summary.json'),'failures':len(failed),'counts':dict(totals)}))
 assert not failed,'independent direct API source differences remain'


def main():
 parser=argparse.ArgumentParser(description=__doc__)
 parser.add_argument('--prepare',action='store_true')
 parser.add_argument('--phase',type=Path)
 parser.add_argument('--plan',type=Path)
 parser.add_argument('--actual',type=Path)
 parser.add_argument('--out',required=True,type=Path)
 args=parser.parse_args()
 if args.prepare:assert args.phase;prepare(args.phase,args.out)
 else:assert args.plan and args.actual;verify_export(args.plan,args.actual,args.out)
if __name__=='__main__':main()