"""Compare complete exports while checking the bounded HTTP3 wire-only changes.
No raw bytes, stream reassembly, application frame, source, body counter or
association may change. This complements the independent reference wire audit;
it does not itself establish application identity from the public snapshot.
"""
import argparse,collections,copy,hashlib,itertools,json
from pathlib import Path

ap=argparse.ArgumentParser(description=__doc__)
ap.add_argument('--kind',choices=['SDK','CLI'],required=True)
for n in ['before','after','out']:ap.add_argument('--'+n,type=Path,required=True)
a=ap.parse_args();checks=0;changed=0;refinements=0;aliases=0;counts=[collections.Counter(),collections.Counter()]
assert not any((q/'.git').exists()for p in[a.before,a.after,a.out]for q in[p.resolve(),*p.resolve().parents])

def check(ok,why):
 global checks
 checks+=1;assert ok,why

def eq(x,y,why):check(x==y,(why,x,y))

def strip_added(x,y,key):
 if key not in x:y.pop(key,None)
 else:eq(x[key],y.get(key),'existing metadata cannot be replaced: '+key)

def field_snapshot(x,y,new_protocol,session=None):
 global aliases
 xv=x['values'];yv=y['values'];old_auth=[v for v in xv if v['name']=='quic.packet.authenticated'];new_auth=[v for v in yv if v['name']=='quic.packet.authenticated']
 if not old_auth and new_auth:
  eq(len(new_auth),1,'one added carrier authentication value');eq(new_auth[0]['value'],True,'native authenticated application carrier');eq(new_auth[0]['origin'],'derived','carrier authentication is derived');check('range'not in new_auth[0],'no invented ciphertext span')
  if session is not None:eq(session.get('Authentication Verified'),True,'retained native authentication')
  yv[:]=[v for v in yv if v['name']!='quic.packet.authenticated'];aliases+=1
 for name in ['http3.wire_only','http3.wire_observation','http3.streamid']:
  prev=[v for v in xv if v['name']==name];current=[v for v in yv if v['name']==name]
  if not prev:yv[:]=[v for v in yv if v['name']!=name]
  elif name=='http3.streamid':
   check(all(v in current for v in prev),'old stream field witnesses retained');yv[:]=[v for v in yv if v['name']!=name]+copy.deepcopy(prev)
 if new_protocol:
  for v in yv:
   if v['name']=='protocol':eq(v['value'],'http3','refined protocol field');v['value']='quic'
 xv.sort(key=lambda v:json.dumps(v,sort_keys=True));yv.sort(key=lambda v:json.dumps(v,sort_keys=True));eq(x,y,'all remaining typed field values and source metadata identical')

def pdu_fields(x,y,new_protocol):
 for name in ['quic.packet.authenticated','http3.wire_only','http3.wire_observation','http3.streamid']:
  if name not in x:y.pop(name,None)
  elif name=='http3.streamid':
   check(all(v in y.get(name,[])for v in x[name]),'CLI stream witnesses retained');y[name]=x[name]
 if new_protocol:eq(y['protocol'],['http3'],'CLI refined protocol field');y['protocol']=['quic']
 eq(x,y,'all other CLI registered fields identical')

with a.before.open(encoding='utf-8-sig')as f,a.after.open(encoding='utf-8-sig')as g:
 for no,(l,r)in enumerate(itertools.zip_longest(f,g),1):
  check(l is not None and r is not None,'exact record count');x,y=json.loads(l),json.loads(r)
  for c,e in zip(counts,[x,y]):c[e['protocol']]+=1
  if x==y:continue
  changed+=1
  if a.kind=='SDK':
   old_s,new_s=x.get('session')or{},y.get('session')or{};wire=new_s.get('HTTP3 Wire Observation');refined=x['protocol']=='quic'and y['protocol']=='http3'
   if wire:
    check(wire in ['retransmission','buffered-range','mixed'],'bounded range classification');eq(new_s['Authentication Verified'],True,'authenticated wire observation');eq(new_s['ALPN'],'h3','selected application');check(any(f.get('HTTP3 Wire Observation')for f in new_s['Frames']),'own STREAM observation')
    for before,after in zip(old_s['Frames'],new_s['Frames']):
     if after.get('HTTP3 Wire Observation'):
      check(not after.get('HTTP3 Frames'),'no logical application replay')
      for key in ['HTTP3','HTTP3 Stream Kind','HTTP3 Wire Observation']:strip_added(before,after,key)
    for key in ['HTTP3','HTTP3 Stream Kind','HTTP3 Stream ID','Protocol Transition','HTTP3 Wire Only','HTTP3 Wire Observation']:strip_added(old_s,new_s,key)
   if refined:
    check(wire is not None and(y['session'].get('Frames')is not None),'new identity only on known wire observation');eq(y['profile'],'http3-native','native refined profile');eq(y['summary'],'http3','native refined summary');y['protocol']=x['protocol'];y['profile']=x['profile'];y['summary']=x['summary'];refinements+=1
   field_snapshot(x['field_values'],y['field_values'],refined,new_s)
  else:
   eq(len(x.get('pdus',[])),len(y.get('pdus',[])),'same observed PDUs per packet');packet_refined=False
   for px,py in zip(x.get('pdus',[]),y.get('pdus',[])):
    refined=px['protocol']=='quic'and py['protocol']=='http3';fields=py.get('fields',{})
    if refined:
     eq(fields.get('http3.wire_only'),[True],'CLI refinement is wire-only');check(fields.get('http3.wire_observation')in[['retransmission'],['buffered-range'],['mixed']],'CLI range classification');py['protocol']='quic';packet_refined=True;refinements+=1
    field_snapshot(px['field_values'],py['field_values'],refined);pdu_fields(px.get('fields',{}),fields,refined)
   if packet_refined:
    eq(x['protocol'],'QUIC','old packet carrier');eq(y['protocol'],'HTTP/3','new packet application');check('no application frames replayed'in y['info'],'wire-only summary, not application replay');eq(y['visibility'],'authenticated-plaintext','authenticated selected wire bytes');eq(y['protocol_stack'],x['protocol_stack']+['HTTP/3'],'progressive stack appends the observed application')
    if x['protocol_scope']!='connection protocol violation observed':eq(y['protocol_scope'],'authenticated STREAM observation','wire observation scope')
    for key in ['protocol','info','visibility','protocol_stack','protocol_scope','protocol_evidence']:
     if key in x:y[key]=x[key]
     else:y.pop(key,None)
  eq(x,y,('all other exported data identical',a.kind,no))

out={'scope':__doc__,'kind':a.kind,'checks':checks,'records':no,'changed_records':changed,'refined_PDUs':refinements,'added_authentication_aliases':aliases,'before_protocol_counts':dict(counts[0]),'after_protocol_counts':dict(counts[1]),'before_sha256':hashlib.sha256(a.before.read_bytes()).hexdigest(),'after_sha256':hashlib.sha256(a.after.read_bytes()).hexdigest(),'other_export_differences':[]}
a.out.parent.mkdir(parents=True,exist_ok=True);a.out.write_text(json.dumps(out,indent=2)+'\n',encoding='utf8');print(json.dumps(out))
