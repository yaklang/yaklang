"""Constrain complete SDK/CLI changes to independently reviewed QUIC token fields.
All physical bytes, flows, application records, transactions and source graphs
must remain identical. The separate header/crypto audits validate every new
token value/range; this comparator cannot establish packet or peer validity.
"""
import argparse,base64,collections,hashlib,itertools,json
from pathlib import Path
ap=argparse.ArgumentParser(description=__doc__)
ap.add_argument('--kind',choices=['SDK','CLI'],required=True)
for n in['before','after','out']:ap.add_argument('--'+n,type=Path,required=True)
a=ap.parse_args();checks=0;changed=0;added=collections.Counter();metadata=collections.Counter();wide=0;counts=[collections.Counter(),collections.Counter()]
for p in[a.before,a.after,a.out]:assert not any((q/'.git').exists()for q in[p.resolve(),*p.resolve().parents])
aliases={'quic.token_length','quic.token','quic.retry_token','quic.retry_integrity_tag'};facts={'Initial Token Length','Initial Token Length Encoding Bytes','Initial Token','Retry Token','Retry Integrity Tag'};range_keys=facts-{'Initial Token Length Encoding Bytes'}
def check(ok,why):
 global checks
 checks+=1;assert ok,why
def eq(x,y,why):check(x==y,(why,x,y))
def snapshot(x,y):
 for name in aliases:
  old=[v for v in x.get('values')or[]if v['name']==name];new=[v for v in y.get('values')or[]if v['name']==name]
  check(not old,'baseline must predate these token aliases')
  if new:check(len(new)==1,'single token field occurrence');added[name]+=1;y['values'][:]=[v for v in y['values']if v['name']!=name]
 eq(x,y,'all existing typed values, origins, ranges and completeness unchanged')
def session(x,y):
 for key in facts:
  check(key not in x,'baseline must predate these token facts')
  if key in y:metadata[key]+=1;y.pop(key)
 xr,yr=x.get('Wire Field Ranges')or{},y.get('Wire Field Ranges')or{}
 for key in range_keys:
  check(key not in xr,'baseline has no token field range');yr.pop(key,None)
 eq(x,y,'all existing session state, frames and source graphs unchanged')
def envelope(old,new,raw):
 global wide
 if not isinstance(old,dict)or not isinstance(new,dict)or old==new:return
 check(raw and raw[0]&128,'changed legacy envelope is a long packet');dl=raw[5];sl=raw[6+dl];off=7+dl+sl;width=1<<(raw[off]>>6);check(width>1,'one-byte legacy envelopes must remain identical');length=int.from_bytes(raw[off:off+width],'big')&((1<<(width*8-2))-1);end=off+width+length;check(end<=len(raw),'complete wide token in unchanged native PDU')
 check('QUICToken'not in old,'old wide profile was an opaque fallback');token=new.pop('QUICToken');eq(token['Token Length'],length,'new wide token semantic length');eq(token['Length Encoding'],raw[off]>>6,'literal two-bit width code');eq(base64.b64decode(token.get('Token')or''),raw[off+width:end],'new token exact wire bytes');eq(set(token),{'Token Length','Length Encoding'}|({'Token'}if length else set()),'only decoded token fields')
 eq(old.get('Protected Payload'),list(raw[off:]),'original fallback contained the entire token encoding');eq(new.get('Protected Payload'),list(raw[end:]),'new protected envelope begins after the complete token');new['Protected Payload']=old['Protected Payload'];eq(old,new,'all other legacy envelope fields identical');wide+=1
with a.before.open(encoding='utf-8-sig')as f,a.after.open(encoding='utf-8-sig')as g:
 for no,(left,right)in enumerate(itertools.zip_longest(f,g),1):
  check(left is not None and right is not None,'same complete record count');x,y=json.loads(left),json.loads(right)
  for c,e in zip(counts,[x,y]):c[e['protocol']]+=1
  if x==y:continue
  changed+=1
  if a.kind=='SDK':
   check(x['protocol']in['quic','http3','doq'],'only native QUIC carrier records may change');eq(x['protocol'],y['protocol'],'same progressive application identity');session(x.get('session')or{},y.get('session')or{});snapshot(x['field_values'],y['field_values']);envelope(x.get('fields'),y.get('fields'),base64.b64decode(x.get('raw')or''))
  else:
   xp,yp=x.get('pdus')or[],y.get('pdus')or[];eq(len(xp),len(yp),'same logical PDUs per physical packet')
   for u,v in zip(xp,yp):
    if u==v:continue
    check(u['protocol']in['quic','http3','doq'],'only QUIC carrier field projections change');snapshot(u['field_values'],v['field_values'])
    for name in aliases:
     check(name not in(u.get('fields')or{}),'no previous token field alias');(v.get('fields')or{}).pop(name,None)
  eq(x,y,('all other complete export evidence unchanged',a.kind,no))
out={'scope':__doc__,'kind':a.kind,'checks':checks,'records':no,'changed_records':changed,'added_token_aliases':dict(added),'new_literal_session_facts':dict(metadata),'wide_token_envelopes_fixed':wide,'before_protocol_counts':dict(counts[0]),'after_protocol_counts':dict(counts[1]),'before_sha256':hashlib.sha256(a.before.read_bytes()).hexdigest(),'after_sha256':hashlib.sha256(a.after.read_bytes()).hexdigest(),'other_export_differences':[]};a.out.parent.mkdir(parents=True,exist_ok=True);a.out.write_text(json.dumps(out,indent=2)+'\n',encoding='utf8');print(json.dumps({k:v for k,v in out.items()if k not in['scope','before_protocol_counts','after_protocol_counts']}))
