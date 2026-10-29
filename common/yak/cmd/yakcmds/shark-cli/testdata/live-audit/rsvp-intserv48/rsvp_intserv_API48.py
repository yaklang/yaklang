"""Bind frozen neutral Intserv source48 to candidate field occurrences.

Prepare must precede any candidate export. It reads only the independently
frozen neutral facts, original bytes and this explicit schema policy. Verify
checks literal values/spans, derived values without invented spans, exact
IEEE32 text/bits, opaque/residual ownership and independent error classes.
"""
from pathlib import Path
import argparse,base64,collections,copy,hashlib,json,re

ROOT=Path(__file__).parent
SOURCE=ROOT/'Intserv-source48-01/frozen-plan01'
SOURCE_MANIFEST_SHA='1b57dd45ed6ea226d62569a1f9c6f06981bc85bef2210d115e30d68fc3e7ad66'
FLOAT_NAMES={'Token Bucket Rate','Token Bucket Size','Peak Data Rate','Composed Available Path Bandwidth'}
WR='Wire Field Ranges'

def sha(p):return hashlib.sha256(Path(p).read_bytes()).hexdigest()
def read(p):return json.loads(Path(p).read_text('utf8'))
def put(p,v):Path(p).write_text(json.dumps(v,ensure_ascii=False,indent=2,allow_nan=False)+'\n',encoding='utf8')
def source_guard():
 assert sha(SOURCE/'manifest.json')==SOURCE_MANIFEST_SHA
 m=read(SOURCE/'manifest.json')
 for path,digest in m['inputs'].items():assert sha(path)==digest,path
 for path,digest in m['outputs'].items():assert sha(SOURCE/path)==digest,path
 for path,digest in m['authored_wire_files'].items():assert sha(SOURCE/'authored-wire'/path)==digest,path
def tokens(path):return [int(x)if x.isdigit()else x for x in re.split(r'\.|\[|\]',path)if x!='']
def get(root,path):
 for part in tokens(path):root=root[part]
 return root
def parent(root,path):
 parts=tokens(path);node=root
 for p in parts[:-1]:node=node[p]
 return node,parts[-1]
def mapped(path):
 if path.startswith('Header.'):return 'Intserv Header.'+path[len('Header.'):]
 if path.startswith('Root.'):
  return {'Root.Opaque Message Data':'Opaque Intserv Data','Root.Unparsed Message Data':'Unparsed Intserv Data','Root.Trailing Message Bytes':'Trailing Intserv Bytes'}[path]
 if re.fullmatch(r'Services\[\d+\]\.Opaque Service Data',path):return path.replace('.Opaque Service Data','.Body.Opaque Service Data')
 return path

def semantic_code(issue):
 code=issue['code'];name=issue['path'].rsplit('.',1)[1]
 table={'token-bucket-must-be-positive':'token-rate-and-bucket-size-must-be-positive','minimum-policed-unit-zero-explicitly-illegal':'minimum-policed-unit-must-be-positive','maximum-packet-size-not-positive':'maximum-packet-size-must-be-positive','invalid-peak-rate-representation':'invalid-nonnegative-float-representation','invalid-finite-nonnegative-representation':'path-bandwidth-requires-finite-nonnegative'if name=='Composed Available Path Bandwidth'else'invalid-nonnegative-float-representation'}
 return table.get(code,code)

def prepare(out):
 assert not out.exists();out.mkdir()
 source_guard()
 rows=read(SOURCE/'source-facts.json');assert len(rows)==49
 inputs=SOURCE/'probe-input.jsonl';assert sha(inputs)=='970f5e64c59b76eefb96c861d6834e0dd24ca6a36159f122ba2b91aa256448c6'
 policy=[];counts=collections.Counter()
 for row in rows:
  raw=Path(row['input_path']).read_bytes()[row['byte_offset']:row['byte_offset']+row['byte_length']]
  assert hashlib.sha256(raw).hexdigest()==row['raw_SHA256']
  source=row['source'];leaves=[]
  for leaf in source['leaves']:
   item=copy.deepcopy(leaf);item['API_path']=mapped(item['path']);leaves.append(item)
   counts[('real'if row['origin']=='real'else'authored')+('_literal'if leaf['literal']else'_derived')]+=1
  kinds={x['kind']for x in source['issues']}
  error_flags={'is_format':'format'in kinds,'is_budget':'budget'in kinds,'is_context':'context'in kinds,'is_integrity':False}
  # These complete input checksums are independently computed, not inferred
  # from the neutral child decoder or from any candidate status.
  n=int.from_bytes(raw[6:8],'big')
  if len(raw)>=8 and n<=len(raw)and n>=8 and raw[2:4]!=b'\0\0':
   total=sum(int.from_bytes(raw[i:i+2],'big')for i in range(0,n,2))
   while total>>16:total=(total&65535)+(total>>16)
   error_flags['is_integrity']=total!=65535
  issues=[dict(owner=mapped(x['path'].rsplit('.',1)[0]),field=x['path'].rsplit('.',1)[1],code=semantic_code(x))for x in source['issues']if x['kind']=='semantic']
  policy.append(dict(sourcecase=row['sourcecase'],origin=row['origin'],raw_SHA256=row['raw_SHA256'],input_path=row['input_path'],byte_offset=row['byte_offset'],byte_length=row['byte_length'],budget=row['probe_global_budget'],body_start=row['target_object_body_start'],object_end=row['target_object_declared_end'],source=source,leaves=leaves,error_flags=error_flags,error_expected=bool(kinds&{'format','shortage','context','budget'})or error_flags['is_integrity'],semantic_issues=issues))
 put(out/'API-policy.json',policy)
 put(out/'manifest.json',dict(checker_SHA256=sha(__file__),source_manifest_SHA256=sha(SOURCE/'manifest.json'),source_facts_SHA256=sha(SOURCE/'source-facts.json'),probe_input_SHA256=sha(inputs),API_policy_SHA256=sha(out/'API-policy.json'),candidate_export_read=False,wire_values_ranges_and_error_flags_not_normalized=True,source_only_policy=True,counts=dict(counts)))
 print(json.dumps(dict(plan=str(out),evaluations=len(policy),manifest_SHA256=sha(out/'manifest.json'),counts=dict(counts))))

def inventory(value,path=''):
 result={}
 if isinstance(value,dict):
  for key,span in value.get(WR,{}).items():
   assert key in value,(path,key,'range without field')
   result[path+'.'+key if path else key]=(value[key],span)
  for key,child in value.items():
   if key!=WR:result.update(inventory(child,path+'.'+key if path else key))
 elif isinstance(value,list):
  for i,child in enumerate(value):result.update(inventory(child,path+'['+str(i)+']'))
 return result

def check_one(plan,actual):
 assert actual['sourcecase']==plan['sourcecase']and actual['raw_SHA256']==plan['raw_SHA256']and actual['budget']==plan['budget']
 for key,expected in plan['error_flags'].items():assert actual[key]is expected,(key,expected,actual[key])
 assert bool(actual['error'])is plan['error_expected'],('error-presence',plan['error_expected'],actual['error'])
 fields=actual['fields'];assert fields['Authentication Verified']is False and fields['Reservation State Inferred']is False and fields['Procedure Semantics Verified']is False
 rows=fields.get('Objects')or[];target=None
 for row in rows:
  if row.get('Header',{}).get(WR,{}).get('Length',[None])[0]==plan['body_start']-4:
   assert target is None;target=row
 assert target is not None,'target object missing'
	
 assert target['Header']['Class']==plan['source']['fields']['Object Class']and target['Header']['C-Type']==2,'object namespace differs'
 body=target.get('Body',{});expected_inventory=set();counts=collections.Counter()
 for leaf in plan['leaves']:
  path=leaf['API_path'];node,key=parent(body,path);got=node[key]
  if not leaf['literal']:
   assert type(got)is type(leaf['value'])and got==leaf['value'],(path,got,leaf['value'])
   assert key not in node.get(WR,{}),(path,'invented derived octet range')
   counts['derived']+=1;continue
  a,b=leaf['range'];expected_inventory.add(path)
  assert node.get(WR,{}).get(key)==[a,b],(path,'literal span',node.get(WR,{}).get(key),[a,b])
  if leaf['kind']=='ieee32':
   expected=leaf['value'];assert isinstance(got,dict)
   assert got['IEEE754 Bits']==expected['wire_uint32']and got['Classification']==expected['classification']
   assert got['Value']==(expected['exact_decimal']if expected['exact_decimal']is not None else expected['display'])
   assert got[WR]['Value']==got[WR]['IEEE754 Bits']==[a,b]
   assert got['Units']==('bytes'if key=='Token Bucket Size'else'bytes/s')
   expected_inventory.update((path+'.IEEE754 Bits',path+'.Value'))
   counts['IEEE32']+=1
  elif leaf['kind']=='bytes':
   assert base64.b64decode(got,validate=True)==base64.b64decode(leaf['value'],validate=True),(path,'owned bytes')
   counts['owned_byte_leaves']+=1
  else:
   assert type(got)is int and got==leaf['value'],(path,got,leaf['value'])
  counts['literal']+=1
 observed=inventory(body)
 assert set(observed)==expected_inventory,('wire-inventory',sorted(set(observed)-expected_inventory),sorted(expected_inventory-set(observed)))
 known={x['API_path'].rsplit('.',1)[-1]for x in plan['leaves']if x['kind']!='bytes'}|FLOAT_NAMES|{'IEEE754 Bits','Value'}
 allowed={x['API_path']for x in plan['leaves']}
 for x in plan['leaves']:
  if x['kind']=='ieee32':allowed.update((x['API_path']+'.IEEE754 Bits',x['API_path']+'.Value'))
 def reject_invented(v,path=''):
  if isinstance(v,dict):
   for key,child in v.items():
    if key==WR:continue
    where=path+'.'+key if path else key
    if key in known:assert where in allowed,('invented known field',where)
    reject_invented(child,where)
  elif isinstance(v,list):
   for i,c in enumerate(v):reject_invented(c,path+'['+str(i)+']')
 reject_invented(body)
 # No empty/ghost service or parameter rows may be fabricated.
 source_services=plan['source']['fields'].get('Services',[])
 services=body.get('Services')or[];assert len(services)==len(source_services)
 for s,want in zip(services,source_services):assert len(s.get('Parameters')or[])==len(want.get('Parameters')or[])
 expected_issues={(x['owner'],x['field'],x['code'])for x in plan['semantic_issues']};observed_issues=set()
 def issues(v,path=''):
  if isinstance(v,dict):
   for x in v.get('Observed Value Domain Issues',[]):observed_issues.add((path,x['Field'],x['Code']))
   for k,c in v.items():
    if k not in(WR,'Observed Value Domain Issues'):issues(c,path+'.'+k if path else k)
  elif isinstance(v,list):
   for i,c in enumerate(v):issues(c,path+'['+str(i)+']')
 issues(body);assert observed_issues==expected_issues,('semantic-issues',observed_issues,expected_issues)
 if plan['origin']!='real':assert fields['Field Layouts Complete']is plan['source']['field_layouts_complete']
 return dict(counts)

def assign(root,path,value):
 parts=tokens(path);node=root
 for i,part in enumerate(parts[:-1]):
  next_part=parts[i+1]
  if isinstance(node,list):
   while len(node)<=part:node.append({})
   node=node[part]
  else:
   node=node.setdefault(part,[]if isinstance(next_part,int)else{})
 node[parts[-1]]=value

def selftest(out):
 assert not out.exists();out.mkdir()
 facts=read(out.parent/'RSVP-Intserv-API-plan48-01/API-policy.json');p=facts[0]
 body={}
 for leaf in p['leaves']:
  path=leaf['API_path'];value=copy.deepcopy(leaf['value'])
  if leaf['kind']=='ieee32':
   value={'IEEE754 Bits':value['wire_uint32'],'Value':value['exact_decimal']if value['exact_decimal']is not None else value['display'],'Classification':value['classification'],'Units':'bytes'if path.endswith('Token Bucket Size')else'bytes/s',WR:{'Value':leaf['range'],'IEEE754 Bits':leaf['range']}}
  assign(body,path,value)
  if leaf['literal']:
   node,key=parent(body,path);node.setdefault(WR,{})[key]=leaf['range']
 for issue in p['semantic_issues']:
  owner=get(body,issue['owner']);owner.setdefault('Observed Value Domain Issues',[]).append({'Field':issue['field'],'Code':issue['code']})
 row=dict(sourcecase=p['sourcecase'],raw_SHA256=p['raw_SHA256'],budget=p['budget'],fields={'Authentication Verified':False,'Reservation State Inferred':False,'Procedure Semantics Verified':False,'Objects':[{'Header':{'Class':p['source']['fields']['Object Class'],'C-Type':2,WR:{'Length':[p['body_start']-4,p['body_start']-2]}},'Body':body}]},error='',**p['error_flags'])
 check_one(p,row)
 cases=[]
 rate='Services[0].Parameters[0].Body.Token Bucket Rate'
 def negative(name,mutate):
  bad=copy.deepcopy(row);mutate(bad)
  try:check_one(p,bad)
  except (AssertionError,KeyError,TypeError,ValueError,IndexError):cases.append({'name':name,'rejected':True});return
  raise AssertionError('negative checker control accepted: '+name)
 negative('wrong-IEEE32-bits',lambda x:get(x['fields']['Objects'][0]['Body'],rate).__setitem__('IEEE754 Bits',0))
 negative('wrong-exact-decimal',lambda x:get(x['fields']['Objects'][0]['Body'],rate).__setitem__('Value','6000.1'))
 negative('aggregate-highlight-span',lambda x:get(x['fields']['Objects'][0]['Body'],rate)[WR].__setitem__('Value',[p['body_start'],p['object_end']]))
 negative('derived-bit-fake-range',lambda x:x['fields']['Objects'][0]['Body']['Intserv Header'][WR].__setitem__('Version',[p['body_start'],p['body_start']+1]))
 negative('wrong-object-namespace',lambda x:x['fields']['Objects'][0]['Header'].__setitem__('Class',13))
 negative('drop-value-domain-warning',lambda x:get(x['fields']['Objects'][0]['Body'],'Services[0].Parameters[0].Body').pop('Observed Value Domain Issues'))
 negative('invent-missing-float-field',lambda x:get(x['fields']['Objects'][0]['Body'],'Services[0].Parameters[0].Body').__setitem__('Composed Available Path Bandwidth',{}))
 negative('invent-empty-parameter-row',lambda x:get(x['fields']['Objects'][0]['Body'],'Services[0].Parameters').append({'Header':{}}))
 put(out/'summary.json',dict(checker_SHA256=sha(__file__),source_manifest_SHA256=SOURCE_MANIFEST_SHA,synthetic_checker_selftest_not_product_evaluation=True,negative_controls=cases))
 print(json.dumps(dict(selftest=str(out),negative_controls=len(cases),all_rejected=True,SHA256=sha(out/'summary.json'))))

def verify(plan,actual,out):
 assert not out.exists();out.mkdir()
 source_guard()
 manifest=read(plan/'manifest.json');assert manifest['checker_SHA256']==sha(__file__)
 assert manifest['API_policy_SHA256']==sha(plan/'API-policy.json')
 assert manifest['source_manifest_SHA256']==sha(SOURCE/'manifest.json')
 expected=read(plan/'API-policy.json');rows=[json.loads(x)for x in actual.read_text('utf8').splitlines()]
 assert len(rows)==len(expected)==49
 results=[];counts=collections.Counter();failures=[]
 for source,row in zip(expected,rows):
  try:
   result=check_one(source,row);counts.update(result);results.append(dict(sourcecase=source['sourcecase'],passed=True,counts=result))
  except (AssertionError,KeyError,TypeError,ValueError,IndexError)as e:
   failures.append(dict(sourcecase=source['sourcecase'],error=str(e)));results.append(dict(sourcecase=source['sourcecase'],passed=False,error=str(e)))
 put(out/'evaluations.json',results)
 summary=dict(plan_SHA256=sha(plan/'manifest.json'),export_SHA256=sha(actual),evaluations=len(rows),passed=len(rows)-len(failures),failures=failures,counts=dict(counts),candidate_source_to_be_bound_by_producer=True,exact_literal_values_ranges_and_private_semantics=True)
 put(out/'summary.json',summary);print(json.dumps(summary,ensure_ascii=False));return not failures

def main():
 p=argparse.ArgumentParser(description=__doc__);sub=p.add_subparsers(dest='mode',required=True)
 q=sub.add_parser('prepare');q.add_argument('--out',type=Path,required=True)
 q=sub.add_parser('selftest');q.add_argument('--out',type=Path,required=True)
 q=sub.add_parser('verify');q.add_argument('--plan',type=Path,required=True);q.add_argument('--actual',type=Path,required=True);q.add_argument('--out',type=Path,required=True)
 a=p.parse_args()
 if a.mode=='prepare':prepare(a.out)
 elif a.mode=='selftest':selftest(a.out)
 else:raise SystemExit(0 if verify(a.plan,a.actual,a.out)else 1)
if __name__=='__main__':main()
