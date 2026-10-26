"""Permit exact diagnostic deduplication in a remainder-only QUIC list row.
Every other packet/PDU/source/field/protocol value must remain identical.
"""
import argparse,hashlib,itertools,json
from pathlib import Path
ap=argparse.ArgumentParser(description=__doc__)
for n in ['before','after','out']:ap.add_argument('--'+n,type=Path,required=True)
a=ap.parse_args();checks=0;changes=[]
for p in [a.before,a.after,a.out]:assert not any((q/'.git').exists()for q in[p.resolve(),*p.resolve().parents])
def check(ok,why):
 global checks
 checks+=1;assert ok,why
def digest(p):
 h=hashlib.sha256()
 with p.open('rb')as f:
  for b in iter(lambda:f.read(1<<20),b''):h.update(b)
 return h.hexdigest()
with a.before.open(encoding='utf-8-sig')as f,a.after.open(encoding='utf-8-sig')as g:
 for no,(l,r)in enumerate(itertools.zip_longest(f,g),1):
  check(l is not None and r is not None,'same complete packet count');x,y=json.loads(l),json.loads(r)
  if x==y:continue
  check(x['info']==y['info']+' · '+y['info'],'only exact repeated diagnostic removed');check(x['protocol']=='QUIC'and'authentication not verified'in y['info'],'unverified QUIC range warning retained');check(bool(y.get('pdus'))and all(p['protocol']=='quic'and p['status']in['malformed','limited','context-required']for p in y['pdus']),'row has refused QUIC PDUs');x['info']=y['info'];check(x==y,'all other full packet/PDU/field/source values identical');changes.append(y['number'])
out={'scope':__doc__,'checks':checks,'records':no,'changed_packet_numbers':changes,'before_sha256':digest(a.before),'after_sha256':digest(a.after),'other_export_differences':[]};a.out.parent.mkdir(parents=True,exist_ok=True);a.out.write_text(json.dumps(out,indent=2)+'\n',encoding='utf8');print(json.dumps(out))
