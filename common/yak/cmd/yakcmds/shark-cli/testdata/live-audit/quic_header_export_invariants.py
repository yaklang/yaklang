"""Constrain complete exports to the reviewed QUIC binary/header projection.
Every raw PDU, packet, flow, transaction, application frame and byte source must
stay identical. Exact header values/ranges are separately checked on wire by
quic_header_wire_audit.py; this comparison never declares field parity itself.
"""
import argparse,base64,collections,hashlib,itertools,json
from pathlib import Path
ap=argparse.ArgumentParser(description=__doc__)
ap.add_argument('--kind',choices=['SDK','CLI'],required=True)
for n in ['before','after','out']:ap.add_argument('--'+n,type=Path,required=True)
a=ap.parse_args();checks=0;changed=0;added=collections.Counter();binary=collections.Counter();counts=[collections.Counter(),collections.Counter()]
for p in [a.before,a.after,a.out]:assert not any((q/'.git').exists()for q in[p.resolve(),*p.resolve().parents])
names={'quic.version','quic.dcid','quic.scid','quic.dcil','quic.scil','quic.long.packet_type','quic.packet_number'}
def check(ok,why):
 global checks
 checks+=1;assert ok,why
def eq(x,y,why):check(x==y,(why,x,y))
def old_go_string(w):
 # encoding/json replaces each invalid UTF-8 byte, including incomplete
 # multibyte tails. Python's default replacement groups some invalid tails.
 out=[];at=0
 while at<len(w):
  b=w[at];n=1 if b<128 else 2 if 0xc2<=b<=0xdf else 3 if 0xe0<=b<=0xef else 4 if 0xf0<=b<=0xf4 else 0
  try:
   if not n or at+n>len(w):raise UnicodeError()
   out.append(w[at:at+n].decode('utf8'));at+=n
  except UnicodeError:out.append('\ufffd');at+=1
 return ''.join(out)
def snapshot(x,y):
 old=x.get('values')or[];new=y.get('values')or[]
 for name in names:
  prev=[v for v in old if v['name']==name];now=[v for v in new if v['name']==name]
  if prev:eq(prev,now,'existing typed header values cannot be rewritten')
  elif now:
   eq(len(now),1,'one added header scalar');added[name]+=1;new[:]=[v for v in new if v['name']!=name]
 old.sort(key=lambda v:json.dumps(v,sort_keys=True));new.sort(key=lambda v:json.dumps(v,sort_keys=True));eq(x,y,'all other typed values and owned source metadata unchanged')
def octets(old,new,key):
 if key not in old and key not in new:return
 eq(key in old,key in new,'binary field presence unchanged');w=base64.b64decode(new[key],validate=True);eq(old[key],old_go_string(w),'old field is exactly the historical lossy JSON spelling');new[key]=old[key];binary[key]+=1
with a.before.open(encoding='utf-8-sig')as f,a.after.open(encoding='utf-8-sig')as g:
 for no,(l,r)in enumerate(itertools.zip_longest(f,g),1):
  check(l is not None and r is not None,'exact complete record count');x,y=json.loads(l),json.loads(r)
  for c,e in zip(counts,[x,y]):c[e['protocol']]+=1
  if x==y:continue
  changed+=1
  if a.kind=='SDK':
   check(x['protocol']in['quic','http3','doq'],'only native QUIC carrier projections may differ');eq(x['protocol'],y['protocol'],'application identity unchanged')
   xs,ys=x.get('session')or{},y.get('session')or{}
   if'Wire Field Ranges'not in xs:ys.pop('Wire Field Ranges',None)
   snapshot(x['field_values'],y['field_values'])
   xf,yf=x.get('fields')or{},y.get('fields')or{}
   for key in ['DCID','SCID']:octets(xf,yf,key)
   octets(xf.get('QUICToken')or{},yf.get('QUICToken')or{},'Token')
  else:
   xp,yp=x.get('pdus')or[],y.get('pdus')or[];eq(len(xp),len(yp),'same PDU occurrences in every physical packet')
   for px,py in zip(xp,yp):
    if px==py:continue
    check(px['protocol']in['quic','http3','doq'],'only native QUIC carrier PDUs may differ');eq(px['protocol'],py['protocol'],'same progressive application identity');snapshot(px['field_values'],py['field_values']);xf,yf=px.get('fields')or{},py.get('fields')or{}
    for name in names:
     if name not in xf:yf.pop(name,None)
  eq(x,y,('all remaining full export values identical',a.kind,no))
out={'scope':__doc__,'kind':a.kind,'checks':checks,'records':no,'changed_records':changed,'added_header_aliases':dict(added),'binary_representation_changes':dict(binary),'before_protocol_counts':dict(counts[0]),'after_protocol_counts':dict(counts[1]),'before_sha256':hashlib.sha256(a.before.read_bytes()).hexdigest(),'after_sha256':hashlib.sha256(a.after.read_bytes()).hexdigest(),'other_export_differences':[]};a.out.parent.mkdir(parents=True,exist_ok=True);a.out.write_text(json.dumps(out,indent=2)+'\n',encoding='utf8');print(json.dumps({k:v for k,v in out.items()if k not in['scope','before_protocol_counts','after_protocol_counts']}))
