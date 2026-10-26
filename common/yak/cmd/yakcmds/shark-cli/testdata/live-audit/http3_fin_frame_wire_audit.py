"""Independently audit clean-FIN frame faults, complete frames and connection scope.
The reference QUIC STREAM bytes and actual raw-peer receipts determine wire
layout; reference application/expert views remain separate, without normalizing
counts or treating absent reference diagnostics as a conformity certificate.
"""
import argparse,base64,collections,hashlib,json,struct
from pathlib import Path
ap=argparse.ArgumentParser(description=__doc__)
ap.add_argument('--expect-operations',type=int,default=36)
ap.add_argument('--require-wire-observations',action='store_true')
for n in['capture','manifest','oracle','events','out']:ap.add_argument('--'+n,required=True,type=Path)
a=ap.parse_args();checks=0
for p in[a.capture,a.out]:assert not any((q/'.git').exists()for q in[p.resolve(),*p.resolve().parents])
def check(ok,why):
 global checks
 checks+=1;assert ok,why
def eq(x,y,why):check(x==y,(why,x,y))
def sha(w):return hashlib.sha256(w).hexdigest()
def load(p):return json.loads(p.read_text(encoding='utf-8-sig'))
def vals(x,k):
 if isinstance(x,dict):return[z for kk,v in x.items()for z in((v if isinstance(v,list)else[v])if kk==k else vals(v,k))]
 if isinstance(x,list):return[z for v in x for z in vals(v,k)]
 return[]
def vi(w,at):
 if at>=len(w):return None,at
 n=1<<(w[at]>>6)
 if at+n>len(w):return None,at
 return int.from_bytes(w[at:at+n],'big')&((1<<(8*n-2))-1),at+n
def frames(w):
 at=0;fs=[]
 while at<len(w):
  start=at;t,at=vi(w,at)
  if t is None:return fs,{'offset':start,'component':'type-varint'}
  n,at=vi(w,at)
  if n is None:return fs,{'offset':start,'component':'length-varint'}
  if at+n>len(w):return fs,{'offset':start,'component':'payload','type':t,'length':n,'available_payload':len(w)-at}
  fs.append({'offset':start,'type':t,'header':w[start:at],'payload':w[at:at+n],'sha256':sha(w[start:at+n])});at+=n
 return fs,None

w=a.capture.read_bytes();check(w[:4]in[bytes.fromhex('d4c3b2a1'),bytes.fromhex('4d3cb2a1')],'classic LE PCAP, microsecond or nanosecond');eq(struct.unpack_from('<I',w,20)[0],0,'NULL link');physical={};at=24
while at<len(w):
 check(at+16<=len(w),'capture record');_,_,n,original=struct.unpack_from('<IIII',w,at);at+=16;eq(n,original,'complete original record');check(at+n<=len(w),'bounded record');physical[len(physical)+1]=w[at:at+n];at+=n
m=load(a.manifest);eq(m['capture_ready_observed'],True,'capture actually observed readiness');eq(len(m['operations']),a.expect_operations,'all declared live operations');ports=(m['client_port'],m['server_port']);reference={};finals={};wire_views=collections.defaultdict(list);packet_conn={};packet_dir={};first={};expert=collections.Counter();raw_app=collections.Counter()
for row in load(a.oracle):
 l=row['_source']['layers'];no=int(l['frame']['frame.number']);eq(bytes.fromhex(l['frame_raw'][0]),physical[no],'reference raw frame equals original capture');expert.update(vals(l,'_ws.expert.message'));raw_app.update(str(int(x,0))for x in vals(l.get('http3',{}),'http3.frame_type'));cs=set(vals(l.get('quic',{}),'quic.connection.number'))
 if not cs:continue
 eq(len(cs),1,'one reference connection per datagram');conn=next(iter(cs));packet_conn[no]=conn;first.setdefault(conn,no);raw=physical[no];eq(raw[:4],b'\x02\0\0\0','IPv4 NULL record');ip=raw[4:];ihl=(ip[0]&15)*4;eq(ip[9],17,'UDP');eq(int.from_bytes(ip[6:8],'big')&0x3fff,0,'unfragmented selected datagram');udp=ip[ihl:int.from_bytes(ip[2:4],'big')];eq(int.from_bytes(udp[4:6],'big'),len(udp),'exact UDP extent');sp,dp=int.from_bytes(udp[:2],'big'),int.from_bytes(udp[2:4],'big');d=0 if sp==ports[0]else 1;eq((sp,dp),ports if d==0 else ports[::-1],'owned endpoint pair');packet_dir[no]=d
 qs=l.get('quic',[]);qs=qs if isinstance(qs,list)else[qs]
 for q in qs:
  fs=q.get('quic.frame',[]);fs=fs if isinstance(fs,list)else[fs]
  for f in fs:
   typ=int(f.get('quic.frame_type','0'),0)
   if not 8<=typ<=15:continue
   sid=int(f['quic.stream.stream_id']);off=int(f.get('quic.stream.offset','0'));body=bytes.fromhex((f.get('quic.stream_data_raw')or[''])[0]);key=(conn,d,sid);cells=reference.setdefault(key,{})
   for i,b in enumerate(body,off):
    if i in cells:eq(cells[i],b,'consistent reference overlap')
    cells[i]=b
   fin=bool(typ&1);wire_views[(no,key)].append((off,body,fin))
   if fin:finals.setdefault(key,off+len(body));eq(finals[key],off+len(body),'stable observed final size')
order=sorted(first,key=first.get);eq(len(order),3,'three connection generations');expected={};partial={};plans={};peer={}
for op in m['operations']:
 conn=order[op['epoch']];sid=op['stream_id'];plan=op['declared_scenario'];plans[(conn,sid)]=plan
 for d,name in[(0,'request_hex'),(1,'reply_hex')]:
  key=(conn,d,sid);wire=bytes.fromhex(op[name]);peer[key]=wire;check(reference[key]==dict(enumerate(wire)),'reference STREAM bytes equal exact peer receipt');eq(finals[key],len(wire),'FIN covers all declared bytes');expected[key],partial[key]=frames(wire);eq(partial[key]is not None,plan['request_expect'if d==0 else'reply_expect']=='frame-error','independent partial-frame classification')

es=[json.loads(s)for s in a.events.read_text(encoding='utf-8-sig').splitlines()];by_id={e['id']:e for e in es};eq(len(by_id),len(es),'unique native PDUs');bindings={};native_wire={};native_prefix={};native_finals={};native_finished=set();known_prefix=set();apps=collections.defaultdict(dict);faults={};mixed=[];wire_observations=[];raw_native=collections.Counter()
for e in es:
 s=e.get('session')or{};streams=[f for f in s.get('Frames',[])if f.get('Frame Type')=='STREAM']
 if a.require_wire_observations and s.get('Native Carrier'):
  auth=[v for v in e['field_values']['values']if v['name']=='quic.packet.authenticated'];eq([v['value']for v in auth],[s['Authentication Verified']],'underlying QUIC authentication field survives application refinement');check(all(v.get('range')is None for v in auth),'authentication is derived, not a current ciphertext byte span')
 if not streams:continue
 eq(s.get('Authentication Verified'),True,'application source authenticated');eq(s.get('Native Carrier'),True,'native wire source');eq(s.get('ALPN'),'h3','selected ALPN');eq(e['transport'],'udp','native UDP');refs=e['source_bytes']['PacketRefs'];cs={packet_conn[r['Number']]for r in refs};eq(len(cs),1,'unambiguous native/reference connection');conn=next(iter(cs));bindings.setdefault(e['flow'],conn);eq(bindings[e['flow']],conn,'stable connection generation');eq(e['domain'],{'Section':0,'Interface':0,'Encapsulation':''},'literal source domain')
 for r in refs:eq(r['Domain'],e['domain'],'physical witness domain');eq(packet_dir[r['Number']],e['direction'],'literal native direction')
 packet_wire=[]
 eq(len(refs),1,'selected UDP packet has one physical capture witness')
 for f in streams:
  key=(conn,e['direction'],int(f['Stream ID']));off=int(f['Offset']);body=base64.b64decode(f.get('Stream Data')or'');check(any((off,body,bool(f['FIN']))in wire_views[(r['Number'],key)]for r in refs),'native STREAM appears literally in its physical datagram');check('Byte Source'in f,('native omitted a STREAM source after another frame failed',e['id'],key));eq(f['Byte Source']['Offset'],off,'native wire offset');eq(f['Byte Source']['Length'],len(body),'native wire length');cells=native_wire.setdefault(key,{})
  before_prefix=native_prefix.get(key,0);was_finished=key in native_finished;known=key in known_prefix or key in faults or key[2]&2==0 and(key[0],1-key[1],key[2])in known_prefix
  for i,b in enumerate(body,off):
   if i in cells:eq(cells[i],b,'consistent native retransmission')
   cells[i]=b
  after_prefix=before_prefix
  while after_prefix in cells:after_prefix+=1
  native_prefix[key]=after_prefix
  if f['FIN']:native_finals.setdefault(key,off+len(body));eq(native_finals[key],off+len(body),'stable native wire final size')
  done=key in native_finals and native_finals[key]==after_prefix
  if done:native_finished.add(key)
  if a.require_wire_observations:
   eq(f['Contiguous Offset'],before_prefix,'independent contiguous offset');eq(f['Delivered Bytes'],after_prefix-before_prefix,'independent newly delivered byte count');eq(f['Stream Complete'],done,'independent drained final extent')
   should_mark=bool(body)and known and after_prefix==before_prefix and not(done and not was_finished)
   marker=f.get('HTTP3 Wire Observation');eq(marker is not None,should_mark,'known-stream identity follows own prefix/fault, never another stream or future bytes')
   if should_mark:
    kind='retransmission'if off+len(body)<=after_prefix else'buffered-range';eq(marker,kind,'literal retransmission versus waiting gap');eq(f.get('HTTP3 Frames'),None,'wire observation never replays a logical frame');eq(f.get('HTTP3'),True,'known stream identity retained');packet_wire.append(kind);wire_observations.append({'PDU':e['id'],'frame':refs[0]['Number'],'connection':conn,'direction':key[1],'stream':key[2],'offset':off,'bytes':len(body),'kind':kind})
  raw_native[(refs[0]['Number'],key,off,sha(body),bool(f['FIN']))]+=1
  for h in f.get('HTTP3 Frames',[]):
   hk=(conn,int(h['Direction']),int(h['Stream ID']));offset=int(h['Stream Offset'])
   if hk not in expected:
    check(hk[2]&2,'non-message frame uses declared control stream');eq(h['Frame Type Value'],4,'declared SETTINGS control');eq(h['Payload Length'],0,'empty declared SETTINGS');eq(base64.b64decode(h['Frame Header Bytes']),b'\x04\0','literal SETTINGS header');check(all(native_wire[hk].get(i)==b for i,b in enumerate(b'\0\x04\0')),'control type and SETTINGS prefix actually observed');known_prefix.add(hk);continue
   check(offset not in apps[hk],'single application-frame delivery');want=next(x for x in expected[hk]if x['offset']==offset);eq(h['Frame Type Value'],want['type'],'complete frame type');eq(h['Payload Length'],len(want['payload']),'complete frame length');eq(base64.b64decode(h['Frame Header Bytes']),want['header'],'literal application header');eq(h['Frame SHA256'],want['sha256'],'complete application frame digest')
   if h['Frame Type']=='HEADERS':
    eq([[v['Name'],v['Value']]for v in h['Headers']],plans[(hk[0],hk[2])]['request_headers'if hk[1]==0 else'reply_headers'],'literal decoded field section');request_key=(hk[0],0,hk[2])
    if hk[1]==1 and request_key in faults:check(h.get('Associated Request')is not True,'a rejected request cannot certify a response association');eq(f.get('Request PDU ID',0),0,'no transaction from rejected request')
   if h['Frame Type']=='DATA':eq(base64.b64decode(h['Payload']or''),want['payload'],'complete DATA payload')
   apps[hk][offset]=h;known_prefix.add(hk)
  if 'HTTP3 Frame Error'not in f:continue
  check(key in partial and partial[key]is not None,'fault belongs to an independently truncated frame');check(key not in faults,'one new framing verdict per direction');fault=f['HTTP3 Frame Error'];p=partial[key];raw=peer[key][p['offset']:];eq(fault['Error Name'],'H3_FRAME_ERROR','RFC connection layout error');eq(fault['Error Code'],0x106,'RFC error code');eq(fault['Error Scope'],'connection','connection error, not stream error');eq(fault['First PDU ID'],e['id'],'current fault observation PDU');eq(fault['Stream ID'],key[2],'fault stream');eq(fault['Direction'],key[1],'fault direction');eq(fault['Frame Offset'],p['offset'],'partial frame offset');eq(fault['FIN Stream Offset'],len(peer[key]),'observed final stream size');eq(fault['Incomplete Component'],p['component'],'type/length/payload distinction');eq(fault['Available Wire Bytes'],len(raw),'partial frame extent');eq(base64.b64decode(fault['Partial Frame Bytes']),raw,'exact retained partial bytes');eq(fault['Partial Frame SHA256'],sha(raw),'partial bytes digest');eq(f['Stream Complete'],True,'FIN reassembly drained');eq(f['HTTP3 Message Conformance']['Length Check'],'unverified-frame-layout','no final body success on truncated frame');eq(e['protocol'],'http3','native selected application identity retained');eq(e['expert'],'MalformedMessage','not NeedMore after a clean final size');check('H3_FRAME_ERROR'in e['error'],'explicit layout diagnostic');check('HTTP3 Stream Error'not in f,'layout error is not mislabeled as a stream-message error')
  if p['component']=='payload':eq(fault['Declared Payload Length'],p['length'],'literal declared payload extent');eq(fault['Available Payload Bytes'],p['available_payload'],'literal short payload extent')
  src=fault['Byte Source'];eq(src['Stream ID'],key[2],'partial source stream');eq(src['Direction'],key[1],'partial source direction');eq(src['Offset'],p['offset'],'partial byte-source offset');eq(src['Length'],len(raw),'partial byte-source extent');eq(src['Coverage'],'complete','sufficient byte and physical witnesses');eq(src['Byte Coverage Complete'],True,'byte coverage flag');eq(src['Packet Coverage Complete'],True,'physical coverage flag');covered=set();parents=set()
  for rg in src['Ranges']:
   parent=by_id[rg['Parent PDU']];parents.add(parent['id']);eq(parent['flow'],e['flow'],'witness connection');eq(parent['direction'],e['direction'],'witness direction');eq(parent['domain'],e['domain'],'witness domain');lo,n=int(rg['Offset']),int(rg['Length']);check(p['offset']<=lo<lo+n<=len(peer[key]),'witness bounded by partial frame');check(any(ff.get('Frame Type')=='STREAM'and ff['Stream ID']==key[2]and ff['Offset']<=lo and lo+n<=ff['Offset']+len(base64.b64decode(ff.get('Stream Data')or''))for ff in(parent.get('session')or{}).get('Frames',[])),'witness actually supplied the partial byte range');covered.update(range(lo,lo+n))
   for r in rg['Packet References']:eq(r['Section'],0,'witness section');eq(r['Interface'],0,'witness interface');eq(r['Encapsulation'],'','witness encapsulation');check(any(pr['Number']==r['Packet']for pr in parent['source_bytes']['PacketRefs']),'physical witness belongs to original contributor')
  eq(covered,set(range(p['offset'],len(peer[key]))),'complete partial-frame source graph');eq(sorted(parents),src['Parent PDUs'],'exact retained parent set');faults[key]={'PDU':e['id'],'frame_refs':[r['Number']for r in refs],'case':plans[(key[0],key[2])]['name'],'component':p['component'],'offset':p['offset'],'available_wire_bytes':len(raw)}
 if any('HTTP3 Frame Error'in f for f in streams)and any(f.get('HTTP3 Frames')and'HTTP3 Frame Error'not in f for f in streams):mixed.append(e['id'])
 if a.require_wire_observations:
  fields=e['field_values']['values']
  conformance=s.get('HTTP3 Message Conformance')or{}
  if 'Stream ID'in conformance:eq(s.get('HTTP3 Stream ID'),conformance['Stream ID'],'packet summary and message conformance retain the same application stream')
  if packet_wire:
   kind=packet_wire[0]if len(set(packet_wire))==1 else'mixed';eq(s['HTTP3 Wire Observation'],kind,'packet range classification');only=not any(f.get('HTTP3 Frames')for f in streams)and not any(r.get('HTTP3 Frames')for r in s.get('QPACK Resumed Streams',[]));eq(s['HTTP3 Wire Only'],only,'a mixed packet does not lose its new application frames');eq([v['value']for v in fields if v['name']=='http3.wire_only'],[only],'registered wire-only mode');eq([v['value']for v in fields if v['name']=='http3.wire_observation'],[kind],'registered range classification')
   if only:eq(e['protocol'],'http3','no protocol rollback on a known stream');eq(e['status'],'decoded','wire bytes decoded, application not replayed')
  else:check('HTTP3 Wire Only'not in s,'no wire-only claim on an unrelated application event')
eq(len(bindings),3,'three native generations');eq(len(set(bindings.values())),3,'generation mapping bijective');verdicts=collections.Counter()
raw_reference=collections.Counter((no,key,off,sha(body),fin)for(no,key),fs in wire_views.items()for off,body,fin in fs);eq(raw_native,raw_reference,'all raw STREAM frame occurrences retained exactly once, including retransmissions')
for key,fs in expected.items():
 check(native_wire[key]==reference[key],'all peer STREAM bytes retained');eq(sorted(apps[key]),[f['offset']for f in fs],'every complete application frame delivered, even after same-packet fault');want=plans[(key[0],key[2])]['request_expect'if key[1]==0 else'reply_expect'];eq(key in faults,want=='frame-error','all declared directions checked');verdicts[want]+=1
first_fault={conn:min(v['PDU']for k,v in faults.items()if k[0]==conn)for conn in order}
for e in es:
 if e['flow']not in bindings or not e.get('session'):continue
 conn=bindings[e['flow']];root=e['session'].get('HTTP3 Connection Error');first_pdu=first_fault[conn]
 if e['id']<first_pdu:check(root is None,'no future connection fault imported into an earlier observation');continue
 check(bool(root),'connection finding persists on later bounded observations, including duplicates');eq(root['First PDU ID'],first_pdu,'first connection fault remains stable');eq(root['Error Code'],0x106,'retained connection error code');eq(root['Error Name'],'H3_FRAME_ERROR','retained error name');eq(root['Error Scope'],'connection','retained scope');fk=next(k for k,v in faults.items()if v['PDU']==first_pdu);eq(root['Stream ID'],fk[2],'first fault stream');eq(root['Direction'],fk[1],'first fault direction');eq(root['Frame Offset'],partial[fk]['offset'],'first partial frame offset');eq(root['FIN Stream Offset'],len(peer[fk]),'first fault final extent');check('not inferred'in root['Scope'],'peer closure not invented');fields=e['field_values']['values'];eq([f['value']for f in fields if f['name']=='http3.connection.error_code'],[0x106],'registered error field');eq([f['value']for f in fields if f['name']=='http3.connection.error_pdu'],[first_pdu],'registered first-PDU field');eq([f['value']for f in fields if f['name']=='http3.connection.error_streamid'],[fk[2]],'registered error stream');eq([f['value']for f in fields if f['name']=='http3.connection.error_direction'],[fk[1]],'registered error direction')
out={'scope':__doc__,'checks':checks,'physical_frames':len(physical),'connections':len(bindings),'operations':len(m['operations']),'directions':len(expected),'direction_verdicts':dict(verdicts),'faults':[dict(connection=k[0],direction=k[1],stream=k[2],**v)for k,v in faults.items()],'same_PDU_new_fault_and_other_decoded_streams':mixed,'first_fault_PDU_by_connection':first_fault,'complete_peer_application_frames':sum(len(v)for v in expected.values()),'wire_observations_checked':a.require_wire_observations,'wire_observation_counts':dict(collections.Counter(v['kind']for v in wire_observations)),'wire_observations':wire_observations,'raw_STREAM_occurrences':sum(raw_native.values()),'raw_reference_application_views':dict(raw_app),'raw_reference_expert_messages':dict(expert),'network':m['network'],'native_selected_differences':[],'capture_sha256':sha(w),'reference_sha256':sha(a.oracle.read_bytes()),'events_sha256':sha(a.events.read_bytes())};a.out.parent.mkdir(parents=True,exist_ok=True);a.out.write_text(json.dumps(out,indent=2)+'\n',encoding='utf8');print(json.dumps({k:v for k,v in out.items()if k not in['scope','faults','raw_reference_expert_messages','wire_observations']}))
