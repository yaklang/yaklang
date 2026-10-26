"""Allow only removal of the invented PN-space value on unnumbered QUIC packets.
All bytes, flows, fields, transactions, application frames and CLI export bytes
must remain identical. Retry public integrity is distinct from packet protection.
"""
import argparse,hashlib,itertools,json
from pathlib import Path
ap=argparse.ArgumentParser(description=__doc__)
ap.add_argument('--kind',choices=['SDK','CLI'],required=True)
for n in ['before','after','out']:ap.add_argument('--'+n,type=Path,required=True)
a=ap.parse_args();checks=0;changed=[]
for p in [a.before,a.after,a.out]:assert not any((q/'.git').exists()for q in[p.resolve(),*p.resolve().parents])
def check(ok,why):
 global checks
 checks+=1;assert ok,why
def eq(x,y,why):check(x==y,(why,x,y))
def digest(p):
 h=hashlib.sha256()
 with p.open('rb')as f:
  for b in iter(lambda:f.read(1<<20),b''):h.update(b)
 return h.hexdigest()
if a.kind=='CLI':eq(digest(a.before),digest(a.after),'all CLI packet exports remain byte-identical');records=sum(1 for _ in a.after.open(encoding='utf-8-sig'))
else:
 with a.before.open(encoding='utf-8-sig')as f,a.after.open(encoding='utf-8-sig')as g:
  for records,(l,r)in enumerate(itertools.zip_longest(f,g),1):
   check(l is not None and r is not None,'same full event count');x,y=json.loads(l),json.loads(r)
   if x==y:continue
   sx,sy=x.get('session')or{},y.get('session')or{};check(sx.get('Packet Name')in['Retry','Version Negotiation'],'only unnumbered packet metadata changes');eq(x['protocol'],'quic','same native carrier');eq(sx.pop('Packet Number Space'),'initial','reviewed prior zero-value error');check('Packet Number Space'not in sy,'unnumbered packet has no PN space')
   for s in [sx,sy]:
    eq(s.get('Authentication Verified'),False,'public Retry/VN context cannot claim packet protection')
    for k in ['Packet Number','Packet Number Length']:check(k not in s,'unnumbered header cannot carry '+k)
   check(not any(v['name']=='quic.packet_number'for v in y['field_values']['values']),'no decoded number alias');eq(x,y,'all other native metadata, octets and identities unchanged');changed.append({'PDU':y['id'],'frames':[r['Number']for r in y['source_bytes']['PacketRefs']],'packet':sy['Packet Name']})
out={'scope':__doc__,'kind':a.kind,'checks':checks,'records':records,'changed_PDUs':changed,'before_sha256':digest(a.before),'after_sha256':digest(a.after),'other_export_differences':[]};a.out.parent.mkdir(parents=True,exist_ok=True);a.out.write_text(json.dumps(out,indent=2)+'\n',encoding='utf8');print(json.dumps(out))
