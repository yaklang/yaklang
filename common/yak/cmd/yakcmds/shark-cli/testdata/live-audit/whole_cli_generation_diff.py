"""Diagnostic whole CLI comparison; changed physical projections need a source oracle."""
from pathlib import Path
import argparse,collections,hashlib,itertools,json
ap=argparse.ArgumentParser(description=__doc__)
ap.add_argument('--before',required=True,type=Path);ap.add_argument('--after',required=True,type=Path);ap.add_argument('--out',required=True,type=Path)
a=ap.parse_args();a.out.mkdir()
def sha(p):
 h=hashlib.sha256()
 with p.open('rb')as f:
  for b in iter(lambda:f.read(1<<20),b''):h.update(b)
 return h.hexdigest()
def same(x,y):
 if type(x)is not type(y):return False
 if isinstance(x,dict):return x.keys()==y.keys()and all(same(x[k],y[k])for k in x)
 if isinstance(x,list):return len(x)==len(y)and all(same(v,w)for v,w in zip(x,y))
 return x==y
def rows(p):
 with p.open(encoding='utf8')as f:
  for line in f:yield json.loads(line)
def key(v):return(v.get('number'),v.get('record_type'),v.get('publication_phase'))
def changes(x,y,path=''):
 if type(x)is not type(y):yield path+' [type]';return
 if isinstance(x,dict):
  for k in sorted(x.keys()|y.keys()):
   if k not in x:yield path+'/'+k+' [+]'
   elif k not in y:yield path+'/'+k+' [-]'
   else:yield from changes(x[k],y[k],path+'/'+k)
 elif isinstance(x,list):
  if len(x)!=len(y):yield path+' [length]'
  for i,(u,v)in enumerate(zip(x,y)):yield from changes(u,v,path+'/'+str(i))
 elif x!=y:yield path
cases=sorted(a.before.glob('*/*/yak-producer.json'));assert len(cases)==37
summaries=[]
for p in cases:
 q=a.after/p.relative_to(a.before);before=json.loads(p.read_text());after=json.loads(q.read_text())
 assert before['exit_code']==after['exit_code']==0 and before['capture_SHA256']==after['capture_SHA256']and before['keylog_used']==after['keylog_used']and before['arguments'][1:]==after['arguments'][1:]
 old=p.with_name('yak.jsonl');new=q.with_name('yak.jsonl');assert before['export_SHA256']==sha(old)and after['export_SHA256']==sha(new)
 name=p.parent.name;count=equal=0;paths=collections.Counter();signatures=collections.Counter();examples={}
 target=a.out/(name+'-changed.jsonl')
 with target.open('x',encoding='utf8')as log:
  for x,y in itertools.zip_longest(rows(old),rows(new)):
   assert x is not None and y is not None and key(x)==key(y),(name,'record order/identity changed')
   count+=1
   for group in ['pdus','published_pdus']:assert same(x.get(group),y.get(group)),(name,key(x),group)
   if same(x,y):equal+=1;continue
   delta=list(changes(x,y));paths.update(delta)
   top=tuple(sorted(k for k in x.keys()|y.keys()if not same(x.get(k),y.get(k))))
   signatures[(top,x.get('protocol'),y.get('protocol'))]+=1
   examples.setdefault(top,dict(frame=y.get('number'),old={k:x.get(k)for k in top},new={k:y.get(k)for k in top}))
   log.write(json.dumps(dict(frame=y.get('number'),old=x,new=y))+'\n')
 summary=dict(case=name,source_before=before['source'],source_after=after['source'],records=count,whole_equal=equal,changed=count-equal,all_native_PDUs_whole_literal=True,changed_paths=dict(paths),signatures=[dict(paths=k[0],protocol_before=k[1],protocol_after=k[2],count=v)for k,v in signatures.most_common()],examples=list(examples.values()),before_SHA256=sha(old),after_SHA256=sha(new))
 (a.out/(name+'.json')).write_text(json.dumps(summary,indent=2)+'\n',encoding='utf8');summaries.append({k:v for k,v in summary.items()if k not in ['changed_paths','signatures','examples']})
 if summary['changed']:print(json.dumps({k:v for k,v in summary.items()if k not in ['examples','changed_paths']}),flush=True)
(a.out/'summary.json').write_text(json.dumps(summaries,indent=2)+'\n',encoding='utf8')
