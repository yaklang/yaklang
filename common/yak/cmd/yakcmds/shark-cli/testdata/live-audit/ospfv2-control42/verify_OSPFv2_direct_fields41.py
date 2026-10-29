"""Importable independent source/range checker for the frozen598 OSPFv2 scenarios.

No candidate values create expectations. Every wire scalar/blob is recomputed
from the original selected PDU bytes, checked at its original logical span and
accounted separately from metadata. Only explicit API member renames are used.
"""
import argparse,base64,collections,copy,hashlib,ipaddress,itertools,json,re
from pathlib import Path

def sha(p):return hashlib.sha256(Path(p).read_bytes()).hexdigest()
def digest(w):return hashlib.sha256(w).hexdigest()
def read(p):return json.loads(Path(p).read_text('utf8'))
def rows(p):return[json.loads(s)for s in Path(p).read_text('utf8').splitlines()]
def write(p,x):
 with Path(p).open('x',encoding='utf8')as f:json.dump(x,f,indent=2);f.write('\n')
def pattern(path):return tuple('*'if type(k)is int else k for k in path)
def get(node,path):
 for key in path:node=node[key]
 return node

def load_plan(plan_dir):
 p=Path(plan_dir);m=read(p/'manifest.json');assert sha(p/'probe-input.jsonl')==m['probe_input_SHA256'];assert sha(p/'source-case-facts.jsonl')==m['source_facts_SHA256'];assert sha(p/'comparison-rules.json')==m['comparison_rules_SHA256'];inputs=rows(p/'probe-input.jsonl');facts=rows(p/'source-case-facts.jsonl');assert len(inputs)==len(facts)==m['scenarios']==598
 assert [r['sourcecase']for r in inputs]==[r['sourcecase']for r in facts];assert len({r['sourcecase']for r in inputs})==598;patterns={pattern(x['path'])for f in facts for x in f['wire_leaves']};return inputs,facts,patterns,m

def source_raw(item):
 p=Path(item['input_path']);assert sha(p)==item['input_file_SHA256'];raw=p.read_bytes()[item['byte_offset']:item['byte_offset']+item['byte_length']];assert len(raw)==item['byte_length']and digest(raw)==item['rawSHA'];return raw

def actual_ranges(fields):
 ranges={};scalars=[];collections_checked=0
 collection_keys={'LSAs','LSA Headers','Requests','Neighbors','Links','Attached Routers','TLVs','TOS/MT-ID Metrics'}
 def visit(node,path):
  nonlocal collections_checked
  if isinstance(node,dict):
   if 'Wire Field Ranges'in node:
    rr=node['Wire Field Ranges'];assert type(rr)is dict,('range map type',path)
    for name,span in rr.items():
     assert type(name)is str and type(span)is list and len(span)==2 and all(type(n)is int for n in span),('range schema',path,name);assert name in node,('range without own field',path,name);key=tuple(path+[name]);assert key not in ranges;ranges[key]=span
   for name,value in node.items():
    if name=='Wire Field Ranges':continue
    if name in collection_keys and value is not None:
     assert type(value)is list and all(type(v)is dict for v in value),('nil/default collection row',path,name);collections_checked+=1
    visit(value,path+[name])
  elif isinstance(node,list):
   for i,item in enumerate(node):visit(item,path+[i])
  else:scalars.append((tuple(path),node))
 visit(fields,[]);return ranges,scalars,collections_checked

def verify_case(fact,raw,budget,fields,error=None,categories=None,known_patterns=None,check_errors=True):
 """Check a frozen case against direct/native fields without output-derived facts.

Native callers may pass check_errors=False when only exported fields/session are
available. That skips error-category assertions only; raw, budget, all wire leaves,
metadata and owned residual bytes remain strict. Return counts, never packet data.
 """
 assert type(raw)is bytes and digest(raw)==fact['rawSHA'],'raw/source binding';assert type(budget)is int and budget==fact['budget'],'budget/source binding';assert type(fields)is dict,'observed complete-prefix map required';count=collections.Counter();expected={tuple(l['path']):l for l in fact['wire_leaves']};assert len(expected)==len(fact['wire_leaves']);ranges,scalars,collections_checked=actual_ranges(fields);assert set(ranges)==set(expected),('wire leaf set differs','missing',[list(p)for p in set(expected)-set(ranges)],'extra',[list(p)for p in set(ranges)-set(expected)])
 for path,leaf in expected.items():
  start,end=leaf['PDU_span'];assert 0<=start<=end<=len(raw);assert ranges[path]==[start,end],('source span',path);wire=raw[start:end];assert digest(wire)==leaf['wire_SHA256'];actual=get(fields,path);encoding=leaf['encoding']
  if encoding=='opaque':
   assert type(actual)is str,('opaque JSON base64 type',path);blob=base64.b64decode(actual,validate=True);assert blob==wire and {'bytes':len(blob),'SHA256':digest(blob)}==leaf['value'],('opaque bytes/hash',path);count['wire_opaque_leaves']+=1
  elif encoding in('ipv4','id','mask'):
   value=str(ipaddress.IPv4Address(wire));assert type(actual)is str and actual==value==leaf['value'],('wire IPv4 bits/ID/mask',path);count['wire_ID_or_IPv4_bits_leaves']+=1
  else:
   assert encoding=='uint';value=int.from_bytes(wire,'big');assert type(actual)is int and actual==value==leaf['value'],('wire unsigned value',path);count['wire_unsigned_leaves']+=1
  count['wire_leaves']+=1
 # Guard against an unobserved scalar being emitted as a default zero without a
 # Wire Field Ranges item. Derived external metric-type/selector values are the
 # one explicit schema exception and are independently checked, not wire-counted.
 if known_patterns is None:known_patterns={pattern(p)for p in expected}
 for path,value in scalars:
  if path and path[-1]=='Authentication Verified':assert value is False,('authentication must not be certified',path)
  if pattern(path)not in known_patterns or path in ranges:continue
  if path[-1]=='TOS/MT-ID Selector':
   parent=get(fields,path[:-1]);flags=parent.get('Flags and TOS/MT-ID');assert type(flags)is int and type(value)is int and value==flags&127,('derived external selector',path);count['derived_not_wire_checks']+=1
  else:raise AssertionError(('unobserved known scalar/default lacks source span',path))
 for item in fact['metadata_checks']:
  actual=get(fields,item['path'])
  if 'contains'in item:assert type(actual)is str and item['contains']in actual,('metadata scope',item['path'])
  else:assert type(actual)is type(item['value'])and actual==item['value'],('metadata value',item['path'])
  count['metadata_checks_not_wire_coverage']+=1
 for residual in fact.get('owned_residuals',[]):
  actual=get(fields,residual['path']);assert type(actual)is str;blob=base64.b64decode(actual,validate=True);start,end=residual['PDU_span'];assert blob==raw[start:end]and len(blob)==residual['bytes']and digest(blob)==residual['SHA256'];count['owned_residual_checks_not_wire_coverage']+=1
 if fact['unknown_auth_no_LLS']:assert fields.get('LLS')is None,'unknown auth cannot supply LLS placement or fields'
 if check_errors:
  assert type(error)is str and type(categories)is dict and set(categories)=={'context','budget','integrity','format'}and all(type(v)is bool for v in categories.values()),'error schema';assert bool(error)==fact['error_required'],('error presence',fact['sourcecase'])
  if fact['identities']is not None:assert categories==fact['identities'],('error identities',fact['sourcecase'],categories,fact['identities'])
  count['error_contract_checks']+=1
 if fact['no_CA_absence_after_budget']:
  strings=[str(error or'')]+[v for path,v in scalars if type(v)is str]
  forbidden=re.compile(r'\b(?:missing|absent|lacks|no)\b.{0,45}\b(?:CA|cryptographic.{0,20}TLV)\b|\b(?:CA|cryptographic.{0,20}TLV)\b.{0,45}\b(?:missing|absent)\b',re.I)
  assert not any(forbidden.search(s)for s in strings),'budget cannot assert CA absence'
  for path,v in scalars:
   if path and re.fullmatch(r'(?:CA(?:.TLV)?|Cryptographic(?: Authentication)? TLV) Present',str(path[-1]),re.I):assert v is not False,'budget cannot assert false CA presence'
 count['collections_without_nil_rows']=collections_checked;return dict(count)

def put(node,path,value):
 for i,key in enumerate(path[:-1]):
  nextkey=path[i+1]
  if isinstance(node,list):
   while len(node)<=key:node.append({})
   node=node[key]
  else:node=node.setdefault(key,[]if type(nextkey)is int else{})
 key=path[-1]
 if isinstance(node,list):
  while len(node)<=key:node.append(None)
  node[key]=value
 else:node[key]=value

def synthetic_record(item,fact,raw):
 fields={}
 for leaf in fact['wire_leaves']:
  start,end=leaf['PDU_span'];value=base64.b64encode(raw[start:end]).decode()if leaf['encoding']=='opaque'else leaf['value'];put(fields,leaf['path'],value);owner=get(fields,leaf['path'][:-1]);owner.setdefault('Wire Field Ranges',{})[leaf['path'][-1]]=leaf['PDU_span']
 for meta in fact['metadata_checks']:put(fields,meta['path'],meta.get('value',meta.get('contains')))
 for meta in fact.get('owned_residuals',[]):
  start,end=meta['PDU_span'];put(fields,meta['path'],base64.b64encode(raw[start:end]).decode())
 identities=fact['identities']or{'context':False,'budget':False,'integrity':False,'format':False};return {'sourcecase':item['sourcecase'],'rawSHA':item['rawSHA'],'budget':item['budget'],'fields':fields,'error':'source-synthetic-error'if fact['error_required']else'',**{'is_'+k:v for k,v in identities.items()}}

def main():
 ap=argparse.ArgumentParser(description=__doc__);ap.add_argument('--plan',type=Path,required=True);ap.add_argument('--actual',type=Path);ap.add_argument('--out',type=Path,required=True);ap.add_argument('--selftest',action='store_true');a=ap.parse_args();assert a.selftest!=(a.actual is not None);assert not a.out.exists();a.out.mkdir();inputs,facts,patterns,manifest=load_plan(a.plan);binding={str(p):sha(p)for p in(Path(__file__),a.plan/'manifest.json',a.plan/'probe-input.jsonl',a.plan/'source-case-facts.jsonl',a.plan/'comparison-rules.json')};raws=[]
 for item in inputs:raw=source_raw(item);raws.append(raw);binding.setdefault(item['input_path'],item['input_file_SHA256'])
 if a.selftest:actual=[synthetic_record(i,f,w)for i,f,w in zip(inputs,facts,raws)]
 else:binding[str(a.actual)]=sha(a.actual);actual=rows(a.actual)
 assert len(actual)==len(inputs)==598;totals=collections.Counter();fail=[];bygroup=collections.defaultdict(collections.Counter)
 expected_keys={'sourcecase','rawSHA','budget','fields','error','is_context','is_budget','is_integrity','is_format'}
 for index,(item,fact,w,row)in enumerate(zip(inputs,facts,raws,actual),1):
  try:
   assert type(row)is dict and set(row)==expected_keys,'probe output key schema';assert row['sourcecase']==item['sourcecase']and row['rawSHA']==item['rawSHA'],'ordered literal sourcecase/rawSHA binding';stats=verify_case(fact,w,row['budget'],row['fields'],row['error'],{k:row['is_'+k]for k in('context','budget','integrity','format')},patterns);totals.update(stats);totals['scenarios']+=1;bygroup[fact['group']].update(stats);bygroup[fact['group']]['scenarios']+=1
  except Exception as ex:fail.append({'line':index,'sourcecase':item['sourcecase'],'reason':repr(ex)})
 negatives=[]
 if a.selftest and not fail:
  def reject(label,index,change):
   row=copy.deepcopy(actual[index]);change(row)
   try:verify_case(facts[index],raws[index],row['budget'],row['fields'],row['error'],{k:row['is_'+k]for k in('context','budget','integrity','format')},patterns)
   except Exception:negatives.append(label)
   else:raise AssertionError(('accepted negative',label))
  reject('wire scalar value',0,lambda r:r['fields']['Header'].__setitem__('Version',3))
  reject('missing complete scalar',0,lambda r:r['fields']['Header'].pop('Router ID'))
  reject('wrong logical span',0,lambda r:r['fields']['Header']['Wire Field Ranges'].__setitem__('Type',[2,3]))
  reject('forged opaque bytes',0,lambda r:r['fields']['Authentication'].__setitem__('Observed Authentication Field',base64.b64encode(b'x'*8).decode()))
  short=next(i for i,f in enumerate(facts)if'short-header-cut4-'in f['sourcecase']);reject('missing source scalar default zero without range',short,lambda r:r['fields']['Header'].__setitem__('Router ID','0.0.0.0'))
  unknown=next(i for i,f in enumerate(facts)if f['unknown_auth_no_LLS']);reject('unknown auth manufactured LLS',unknown,lambda r:r['fields'].__setitem__('LLS',{'Checksum':1,'Wire Field Ranges':{'Checksum':[48,50]}}))
  noCA=next(i for i,f in enumerate(facts)if f['no_CA_absence_after_budget']);reject('CA absence claimed after budget',noCA,lambda r:r.__setitem__('error','budget: missing CA TLV'))
  mixed=next(i for i,f in enumerate(facts)if f['identities']and f['identities']['budget']and f['identities']['integrity']);reject('budget erased independent integrity',mixed,lambda r:r.__setitem__('is_integrity',False))
  reject('false authentication verification',0,lambda r:r['fields'].__setitem__('Authentication Verified',True))
 assert binding=={p:sha(p)for p in binding};write(a.out/'failures.json',fail);result={'scope':__doc__,'mode':'source-synthetic verifier selftest;not candidate'if a.selftest else'direct candidate source/range validation','plan_manifest_SHA256':sha(a.plan/'manifest.json'),'verifier_SHA256':sha(__file__),'counts':dict(totals),'groups':{k:dict(v)for k,v in bygroup.items()},'failure_count':len(fail),'failures_SHA256':sha(a.out/'failures.json'),'synthetic_negative_controls_rejected':negatives,'inputs':binding,'inputs_unchanged':True,'Go_reflect_type_or_native_ingress_or_UI_verified':False,'candidate_read':not a.selftest,'new_cases_or_output_derived_expectations_created':False};write(a.out/'summary.json',result);print(json.dumps({'summary':str(a.out/'summary.json'),'SHA256':sha(a.out/'summary.json'),'counts':dict(totals),'failures':len(fail),'mode':result['mode'],'negative_controls':negatives}))
 if fail:raise SystemExit(1)
if __name__=='__main__':main()
