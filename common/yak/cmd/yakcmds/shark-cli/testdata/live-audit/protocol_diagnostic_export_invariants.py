"""Allow only newly retained native refusal metadata in complete CLI exports.
Every physical row, PDU, field occurrence, byte witness, flow, transaction and
presentation value must remain equal. Added summary/error/expert strings must
match the separately exported native observation, never a rewritten reference.
"""
import argparse,collections,hashlib,itertools,json
from pathlib import Path
ap=argparse.ArgumentParser(description=__doc__)
for n in['before','after','events','out']:ap.add_argument('--'+n,type=Path,required=True)
a=ap.parse_args();checks=0
for p in vars(a).values():assert not any((q/'.git').exists()for q in[p.resolve(),*p.resolve().parents])
def check(ok,why):
 global checks
 checks+=1;assert ok,why
def eq(x,y,why):check(x==y,why)
def sha(p):
 h=hashlib.sha256()
 with p.open('rb')as f:
  for w in iter(lambda:f.read(1<<20),b''):h.update(w)
 return h.hexdigest()
def rows(p):
 with p.open(encoding='utf-8-sig')as f:
  for line in f:yield json.loads(line)
keys=['summary','error','expert'];native={}
for e in rows(a.events):
 if not e.get('error')and not e.get('expert'):continue
 v={k:e.get(k,'')for k in keys};v.update(protocol=e['protocol'],status=e['status'])
 check(e['id']not in native,'native diagnostic IDs are unique');native[e['id']]=v
physical=0;pdus=0;changed_rows=0;added=0;profiles=collections.Counter()
for b,c in itertools.zip_longest(rows(a.before),rows(a.after)):
 check(b is not None and c is not None,'same complete physical row count');physical+=1;eq('pdus'in b,'pdus'in c,('same PDU presence',physical));bs=b.pop('pdus',[]);cs=c.pop('pdus',[]);eq(b,c,('no physical row/label/flow/summary change',physical));eq(len(bs),len(cs),('same logical PDU count',physical));changed=False
 for old,new in zip(bs,cs):
  pdus+=1;ov={k:old.pop(k)for k in keys if k in old};nv={k:new.pop(k)for k in keys if k in new};eq(old,new,('all original typed values and owned byte metadata retained',physical,old.get('id')))
  if ov==nv:continue
  eq(ov,{},('only absent diagnostic metadata may be added',physical,old['id']));eq(set(nv),set(keys),('complete diagnostic triple',physical,old['id']));check(old['id']in native,('new metadata needs an observed native reason',physical,old['id']));e=native[old['id']];eq(e['protocol'],old['protocol'],('native protocol',physical));eq(e['status'],old['status'],('native status',physical));eq(nv,{k:e[k]for k in keys},('exact separately observed native diagnostic',physical,old['id']));added+=1;changed=True;profiles[(e['protocol'],e['status'],e['expert'])]+=1
 changed_rows+=changed
out={'scope':__doc__,'checks':checks,'physical_rows':physical,'logical_PDUs':pdus,'rows_with_added_diagnostics':changed_rows,'PDUs_with_added_diagnostics':added,'diagnostic_profiles':{str(k):v for k,v in profiles.items()},'before_sha256':sha(a.before),'after_sha256':sha(a.after),'events_sha256':sha(a.events),'other_differences':[]};a.out.parent.mkdir(parents=True,exist_ok=True);a.out.write_text(json.dumps(out,indent=2)+'\n',encoding='utf8');print(json.dumps(out))
