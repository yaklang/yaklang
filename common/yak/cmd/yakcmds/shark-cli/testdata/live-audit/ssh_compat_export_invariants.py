"""Constrain public SSH compatibility recovery to one observed connection.
Match every previous PDU by its exact raw bytes, endpoints, capture domain and
physical contributors, then construct a bijection for its generated PDU ID.
Only explicit PDU-reference fields use this bijection; packet numbers, stream
IDs, wire transaction fields, sequences and lengths stay exact. Separately
certified SSH field corrections require an exact-file literal-field certificate.
The added initial messages require a separate independent wire/reference audit.
"""
import argparse,base64,collections,hashlib,json
from pathlib import Path
ap=argparse.ArgumentParser(description=__doc__)
for name in['before','after','out']:ap.add_argument('--'+name,type=Path,required=True)
ap.add_argument('--target-flow',type=int,required=True);ap.add_argument('--field-certificate',type=Path);a=ap.parse_args();checks=0
for p in[a.before,a.after,a.out]:assert not any((q/'.git').exists()for q in[p.resolve(),*p.resolve().parents])
def check(ok,why):
 global checks
 checks+=1;assert ok,why
def eq(x,y,why):check(x==y,why)
def rows(p):
 for line in p.open(encoding='utf-8-sig'):yield json.loads(line)
def difference_paths(x,y,path=''):
 if x==y:return []
 if isinstance(x,dict)and isinstance(y,dict):return sum((difference_paths(x.get(k),y.get(k),path+'/'+str(k))for k in x.keys()|y.keys()),[])
 if isinstance(x,list)and isinstance(y,list)and len(x)==len(y):return sum((difference_paths(b,c,path+'/'+str(i))for i,(b,c)in enumerate(zip(x,y))),[])
 return [path]
def sha(p):
 h=hashlib.sha256()
 with p.open('rb')as f:
  for w in iter(lambda:f.read(1<<20),b''):h.update(w)
 return h.hexdigest()
field_certificate={}
if a.field_certificate:
 check(not any((q/'.git').exists()for q in[a.field_certificate.resolve(),*a.field_certificate.resolve().parents]),'raw-derived field certificate remains outside Git')
 fc=json.loads(a.field_certificate.read_text());eq(fc['events_sha256'],sha(a.after),'independent literal field audit binds this exact complete SDK export');field_certificate={v['PDU']:v for v in fc['compared_PDUs']+fc['generic_PDUs']}
def check_field_proof(e):
 proof=field_certificate.get(e['id']);check(proof is not None,'each corrected/new field tree has an independent exact-wire/reference certificate');eq(proof['raw_sha256'],hashlib.sha256(base64.b64decode(e['raw'])).hexdigest(),'field proof binds original logical bytes');eq(proof['field_tree_sha256'],hashlib.sha256(json.dumps(e['fields'],sort_keys=True,separators=(',',':')).encode()).hexdigest(),'entire field tree matches the independently decoded layout');eq(proof['physical_frames'],sorted({r['Number']for r in e['source_bytes']['PacketRefs']}),'field proof keeps every physical contributor');eq((proof['source'],proof['destination'],proof['capture_domain']),(e['source'],e['destination'],e['domain']),'field proof keeps the exact observed direction and capture domain');return proof
def target(e):return e['protocol']=='ssh'and e['flow']==a.target_flow
def key(e):return tuple(json.dumps(e.get(k),sort_keys=True)for k in['protocol','transport','flow','domain','direction','source','destination','raw'])+(json.dumps(e['source_bytes']['PacketRefs'],sort_keys=True),)
def pairs():
 it=iter(rows(a.after));extras=[]
 for old in rows(a.before):
  new=next(it,None)
  while new and key(old)!=key(new)and target(new):extras.append(new);new=next(it,None)
  check(new is not None and key(old)==key(new),('same exact message in original order',old['id'],new and new['id']))
  yield old,new
 for new in it:extras.append(new)
 for new in extras:
  check(target(new),'all inserted PDUs belong to the declared original SSH connection')
  yield None,new
mapping={};inverse={};extra=[];facts={};transaction_protocols=set()
for old,new in pairs():
 check(new['id']not in inverse,'every new PDU identity is unique');inverse[new['id']]=old['id']if old else None
 if old:
  check(old['id']not in mapping,'every old PDU identity is unique');mapping[old['id']]=new['id'];facts[old['id']]=(old['protocol'],old['flow'],old['domain'])
  if old['transaction']!=new['transaction']:transaction_protocols.add(old['protocol'])
 else:extra.append(new)
eq(len(extra),10,'eight newly decoded initial packets and two opaque transport diagnostics')
eq([(e.get('session')or{}).get('Message Number')for e in extra],[20,20,34,31,32,33,21,21,None,None],'expected observed recovered message sequence')
eq([max(r['Number']for r in e['source_bytes']['PacketRefs'])for e in extra],[5217,5221,5222,5224,5227,5247,5249,5250,5258,5259],'original physical completion/contribution frames')
warning="endpoint 1: softwareversion uses '-' (RFC4253 4.2)"
for e in extra:
 s=e.get('session')or{};eq(s.get('Identification Conformance'),'nonconformant','new messages retain the literal identification warning');eq(s.get('Identification Warning'),warning,'persistent original endpoint warning')
 if'Packet Name'in s:
  eq(e['status'],'decoded','complete clear initial packet is decoded');check(not e.get('error')and not e.get('expert'),'complete recovered packet has no refusal')
  if a.field_certificate:check_field_proof(e)
  for name in['Signature Verified','Peer Identity Verified']:
   if name in s:eq(s[name],False,'literal host key/signature parsing does not verify peer identity')
 else:eq((e['status'],e['expert']),('context-required','Encrypted'),'encrypted suffix stays opaque')
id_aliases={'pdu.id','pdu.response_to','ntp.control.request_pdu','http3.connection.error_pdu'}
remapped=collections.Counter()
def mid(v,label):
 if not v:return v
 check(type(v)is int and v in mapping,('reference resolves to an original emitted PDU',label,v));new=mapping[v]
 if new!=v:remapped[label]+=1
 return new
def source(s):
 if not s:return
 s['ParentPDU']=mid(s['ParentPDU'],'byte-source parent')
 if s.get('ParentPDUs')is not None:s['ParentPDUs']=[mid(v,'byte-source parents')for v in s['ParentPDUs']]
def semantic(o,protocol):
 if isinstance(o,dict):
  for k,v in o.items():
   if k=='Request PDU ID'and protocol=='ntp':o[k]=mid(v,'NTP request PDU')
   elif k=='Data Transfer ID'and protocol in['ftp','ftp-data']:o[k]=mid(v,'FTP transfer PDU')
   elif k=='Original Carrier PDU'and protocol=='rdpudp':o[k]=mid(v,'RDPUDP original carrier')
   elif k=='RTSP SETUP Request Event IDs'and protocol=='rtsp':o[k]=[mid(i,'RTSP SETUP evidence')for i in v]
   elif k=='SDP Event IDs'and protocol in['rtp','rtcp']:o[k]=[mid(i,'SDP negotiation evidence')for i in v]
   elif k=='Reassembled Byte Source'and protocol=='ntp':
    v['Parent PDUs']=[mid(i,'NTP fragment parent')for i in v['Parent PDUs']]
    for r in v['Ranges']:r['Parent PDU']=mid(r['Parent PDU'],'NTP fragment range')
   else:semantic(v,protocol)
 elif isinstance(o,list):
  for v in o:semantic(v,protocol)
def normalize(e):
 old_id=e['id'];e['id']=mid(e['id'],'PDU identity');e['response_to']=mid(e['response_to'],'response association')
 # These selected protocols store observed request PDU IDs here. Wire IDs
 # (DNS ID, NTP sequence, RTMP transaction number, etc.) remain in exact fields.
 if e['protocol']in['dns','dot','doh','mdns','llmnr','nbns','http','http2','http3','ipp','ntp','ftp','ftp-data','ajp','kafka','ocpp','rtmp','slmp','hart-ip','enip','dhcp','dhcpv6','radius','bfd']and e['transaction']:
  owner=facts.get(e['transaction']);expected=facts[old_id]
  if e['protocol']=='ftp-data':expected=('ftp',e['session']['Control Flow ID'],e['domain']);eq(e['session']['Data Transfer ID'],e['transaction'],'FTP payload stays attached to its exact control-command PDU')
  eq(owner,expected,'request association retains its exact protocol, connection and capture domain');e['transaction']=mid(e['transaction'],'request transaction PDU')
 source(e.get('source_bytes'));f=e.get('field_values')or{}
 if f:
  f['pdu']=mid(f['pdu'],'typed PDU identity');source(f.get('source'))
  for v in f.get('values',[]):
   if v['name']in id_aliases:
    old=v['value'];v['value']=mid(old,'typed '+v['name'])
    if'exact_value'in v:eq(v['exact_value'],str(old),'typed exact identity matches its numeric value');v['exact_value']=str(v['value'])
 semantic(e.get('fields'),e['protocol']);semantic(e.get('session'),e['protocol'])
def header(old,new):
 eq(base64.b64decode(new['raw']),b'SSH-2.0-Cisco-1.25\n','literal public nonconformant identifier')
 eq((old['entry'],old['status'],old['expert'],old['completeness']),('','malformed','MalformedMessage','malformed'),'original strict refusal scope')
 eq(old['summary'],'MalformedMessage: SSH software version is invalid','original strict refusal reason')
 eq((new['entry'],new['status'],new['expert'],new['completeness']),('SSHIdentification','decoded','','envelope'),'guarded compatibility framing stays an envelope view')
 eq(new['summary'],'SSH identification','observed identification summary')
 expected={'Context Level':'observed','Identification':'SSH-2.0-Cisco-1.25','Identification Conformance':'nonconformant','Identification Warning':warning,'Packet Name':'identification','Version':'2.0'}
 for k in['fields','session']:eq(old[k],None,'no old parsed identity');eq(new[k],expected,'literal owned header and limitation fields');new[k]=None
 eq(old['fields_error'],'protocol parser: no exact rule for this message','old missing exact rule');eq(new.get('fields_error'),None,'new exact identity fields available')
 for s in[new['source_bytes'],new['field_values']['source']]:eq(s.pop('CompletedIn'),s['PacketRefs'][0],'completion is the exact captured identity frame')
 f=new['field_values'];expected_alias={'pdu.completed_in':5213,'ssh.protocol':'SSH-2.0-Cisco-1.25','ssh.identification.conformance':'nonconformant','ssh.identification.warning':warning};seen={}
 vals=[]
 for v in f['values']:
  if v['name']in expected_alias:check(v['name']not in seen,'one added identity evidence alias');eq(v['value'],expected_alias[v['name']],'exact identity evidence value');eq(v['origin'],'event'if v['name']=='pdu.completed_in'else'context','explicit evidence origin');check('pdu_range'not in v,'no manufactured wire range');seen[v['name']]=True
  else:
   if v['name']=='status':eq(v['value'],'decoded','typed observed header status');v['value']='malformed'
   vals.append(v)
 eq(set(seen),set(expected_alias),'all expected new identity evidence aliases');f['values']=vals;f['completeness']='malformed'
 for k in['entry','status','expert','completeness','summary','fields_error']:new[k]=old[k]
matched=0;header_count=0;field_corrections=[]
for old,new in pairs():
 if old is None:continue
 matched+=1
 if target(old)and old['status']=='malformed':header(old,new);header_count+=1
 normalize(old)
 if old.get('fields')!=new.get('fields'):
  eq((old['protocol'],new['protocol'],old['entry'],new['entry']),('ssh','ssh','SSHPacket','SSHPacket'),'only complete SSH packet field layouts receive independent corrections')
  check(old['status']in['decoded','deferred']and new['status']in['decoded','deferred'],'a refusal cannot become a parsed field correction');proof=check_field_proof(new);field_corrections.append({'PDU':new['id'],'message':proof['message'],'physical_frames':proof['physical_frames']});old['fields']=new['fields']
 if old!=new:check(False,('all remaining original bytes, field trees, roles, flows and reference associations are exact',new['id'],new['protocol'],difference_paths(old,new)[:20]))
 checks+=1
eq(header_count,1,'exactly one original nonconformant identity refusal recovered')
out={'scope':__doc__,'checks':checks,'original_PDUs':matched,'new_PDUs':len(inverse),'recovered_identity_views':header_count,'added_initial_packets':8,'added_opaque_diagnostics':2,'independently_certified_original_field_tree_corrections':field_corrections,'field_certificate_sha256':sha(a.field_certificate)if a.field_certificate else None,'remapped_explicit_PDU_references':dict(remapped),'changed_transaction_protocols':sorted(transaction_protocols),'PDU_identity_map':{str(k):v for k,v in mapping.items()},'added_PDU_ids':[e['id']for e in extra],'before_sha256':sha(a.before),'after_sha256':sha(a.after),'other_differences':[]};a.out.parent.mkdir(parents=True,exist_ok=True);a.out.write_text(json.dumps(out,indent=2)+'\n');print(json.dumps({k:v for k,v in out.items()if k not in['scope','PDU_identity_map','independently_certified_original_field_tree_corrections']}));print('independently certified original field tree corrections',len(field_corrections))
