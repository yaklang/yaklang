"""Compare closed before/after SSH payload exports with the unchanged source oracle."""
from pathlib import Path
import collections,hashlib,importlib.util,json

root=Path(__file__).parent
def sha(p):return hashlib.sha256(p.read_bytes()).hexdigest()
def read(p):return json.loads(p.read_text('utf-8-sig'))
def put(p,v):
 with p.open('x',encoding='utf8')as f:json.dump(v,f,indent=2,sort_keys=True);f.write('\n')
checker=root/'SSH-SDK-CLI-source49-01'/'ssh_native_cli49.py'
spec=importlib.util.spec_from_file_location('source_delta49',checker);m=importlib.util.module_from_spec(spec);spec.loader.exec_module(m)
plan=checker.parent/'plan01';_,rows,_,_=m.load(plan)
before=root/'SSH-before-c04a49-01'/'events.jsonl'
after=root/'SSH-native-cc9d49-01'/'events.jsonl'
out=root/'SSH-source-delta49-01';out.mkdir(exist_ok=False)
inputs={str(p):sha(p)for p in(checker,plan/'manifest.json',plan/'source-rows.json',before,after,Path(__file__))}
def load(p):
 records=m.lines(p);payloads=[e for e in records if e.get('raw')]
 assert len(records)==87 and len(payloads)==76
 byframe={e['source_bytes']['PacketRefs'][0]['Number']:e for e in payloads}
 assert len(byframe)==76
 return byframe
def mismatches(actual,expected,path=''):
 if type(actual)is not type(expected):return[path+' (type)']
 if isinstance(expected,dict):
  out=[]
  for k in sorted(set(actual)|set(expected)):
   p=path+'/'+k
   if k not in actual:out.append(p+' (missing)')
   elif k not in expected:out.append(p+' (extra)')
   else:out+=mismatches(actual[k],expected[k],p)
  return out
 if isinstance(expected,list):
  if len(actual)!=len(expected):return[path+' (length)']
  return[p for i,(a,b)in enumerate(zip(actual,expected))for p in mismatches(a,b,path+'/'+str(i))]
 return[]if actual==expected else[path+' (value)']
old,new=load(before),load(after);cases=collections.OrderedDict();details=[]
for r in rows:
 c=cases.setdefault(r['sourcecase'],dict(payloads=0,before_pass=0,after_pass=0,before_failure_frames=[],before_failure_predicates=[]))
 c['payloads']+=1
 for label,events in [('before',old),('after',new)]:
  try:m.verify_event(events[r['frame']],r)
  except(AssertionError,KeyError,ValueError,TypeError)as e:
   if label=='after':raise
   c['before_failure_frames'].append(r['frame']);c['before_failure_predicates'].append(str(e))
   details.append(dict(sourcecase=r['sourcecase'],frame=r['frame'],predicate=str(e),
        Session_paths=mismatches(events[r['frame']].get('session'),r['session']),
        GetFields_paths=mismatches(events[r['frame']].get('fields'),r['fields'])))
  else:c[label+'_pass']+=1
assert sum(c['before_pass']for c in cases.values())<76
assert sum(c['after_pass']for c in cases.values())==76
assert inputs=={p:sha(Path(p))for p in inputs}
put(out/'case-differences.json',details)
summary=dict(inputs=inputs,source_cases=12,physical_source_frames=148,payloads=76,
   before_exact_payloads=sum(c['before_pass']for c in cases.values()),
   after_exact_payloads=76,cases=cases,source_values_changed=False,
   direct_scope='Full per-payload source predicates; separate whole87/CLI validator accounts11 exact FIN diagnostics.',
   not_live_or_authenticated=True,differences_SHA256=sha(out/'case-differences.json'))
put(out/'summary.json',summary)
print(json.dumps(dict(before_exact_payloads=summary['before_exact_payloads'],after_exact_payloads=76,
 summary_SHA256=sha(out/'summary.json'),cases=cases)))
