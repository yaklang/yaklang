"""Constrain full export changes to independently certified NTP observations.
All other records, transport identities, capture domains, TCP flows and packet
columns must remain identical. This is not a substitute for ntp_wire_audit.py.
"""
import argparse,base64,collections,hashlib,itertools,json
from pathlib import Path

ap=argparse.ArgumentParser(description=__doc__)
ap.add_argument('--kind',choices=['SDK','CLI'],required=True)
for n in['before','after','out']:ap.add_argument('--'+n,type=Path,required=True)
a=ap.parse_args();checks=0;changed=0;counts=[collections.Counter(),collections.Counter()];patterns=collections.Counter();retired=[]
for p in[a.before,a.after,a.out]:assert not any((q/'.git').exists()for q in[p.resolve(),*p.resolve().parents])

def check(ok,why):
 global checks
 checks+=1;assert ok,why

def eq(x,y,why):check(x==y,(why,x,y))

def source(x,y):
 for k in x.keys()|y.keys():
  if k in['Kind','SHA256','CompletedIn']:continue
  eq(x.get(k),y.get(k),'unchanged physical source identity '+k)
 if x.get('CompletedIn'):eq(x['CompletedIn'],y.get('CompletedIn'),'existing completion witness')
 if y.get('CompletedIn'):check(y['CompletedIn']in(y.get('PacketRefs')or[]),'completion is an actual physical witness')

def unchanged(x,y,allowed,why):
 eq({k:v for k,v in x.items()if k not in allowed},{k:v for k,v in y.items()if k not in allowed},why)

sdk_allowed={'admission','completeness','entry','expert','field_values','fields','fields_error','profile','protocol','session','status','summary','transaction','response_to','error','raw','source_bytes'}
cli_allowed={'pdus','protocol_stack','protocol_evidence','protocol_status','protocol_scope','visibility','protocol','info','message_source','message_destination','message_transport','message_domain'}
with a.before.open(encoding='utf-8-sig')as f,a.after.open(encoding='utf-8-sig')as g:
 for no,(l,r)in enumerate(itertools.zip_longest(f,g),1):
  check(l is not None and r is not None,'same full record count');x,y=json.loads(l),json.loads(r)
  for c,e in zip(counts,[x,y]):c[e['protocol']]+=1
  if x==y:continue
  changed+=1;patterns[x['protocol']+' -> '+y['protocol']]+=1
  if a.kind=='SDK':
   check(x['protocol']=='ntp'or y['protocol']=='ntp',('unrelated native record changed',no,x['protocol'],y['protocol']));unchanged(x,y,sdk_allowed,'all other native event metadata identical');source(x['source_bytes'],y['source_bytes']);old=base64.b64decode(x['raw']or'');new=base64.b64decode(y['raw']or'')
   if y['protocol']=='ntp':
    check(new.startswith(old),'new exact message retains the previous captured probe');eq(y['source_bytes']['SHA256'],hashlib.sha256(new).hexdigest(),'native message digest')
    if x['protocol']=='ntp':eq(old,new,'existing NTP octets do not change')
   else:
    check(x['protocol']=='ntp'and y['protocol']=='','only rejected legacy candidate can retire to unknown UDP');check(old.startswith(new),'unknown probe retains literal prefix');eq(y['response_to'],0,'retired heuristic has no request owner');eq(y['transaction'],0,'retired heuristic has no transaction');retired.append({'PDU':y['id'],'frames':[r['Number']for r in y['source_bytes']['PacketRefs']],'previous_bytes':len(old),'retained_probe_bytes':len(new)})
  else:
   xp,yp=x.get('pdus')or[],y.get('pdus')or[];eq(len(xp),len(yp),'same physical packet PDU count');check(any(p.get('protocol')=='ntp'for p in xp+yp),'only NTP packet presentation changes');unchanged(x,y,cli_allowed,'all physical, network and transport columns identical')
   for px,py in zip(xp,yp):
    if px==py:continue
    check(px.get('protocol')=='ntp'or py.get('protocol')=='ntp','no unrelated PDU changes inside a carrier packet');eq(px['id'],py['id'],'same observed PDU identity');source(px['byte_source'],py['byte_source'])
   for k in['message_source','message_destination','message_transport','message_domain']:
    if k in x and k in y:eq(x[k],y[k],'existing message endpoint/domain metadata')
out={'scope':__doc__,'kind':a.kind,'checks':checks,'records':no,'changed_records':changed,'protocol_changes':dict(patterns),'before_protocol_counts':dict(counts[0]),'after_protocol_counts':dict(counts[1]),'retired_legacy_candidates':retired,'before_sha256':hashlib.sha256(a.before.read_bytes()).hexdigest(),'after_sha256':hashlib.sha256(a.after.read_bytes()).hexdigest(),'other_export_differences':[]};a.out.parent.mkdir(parents=True,exist_ok=True);a.out.write_text(json.dumps(out,indent=2)+'\n',encoding='utf8');print(json.dumps({k:v for k,v in out.items()if k not in['scope','before_protocol_counts','after_protocol_counts']}))
