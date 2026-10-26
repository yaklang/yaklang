"""Bind NTP observations to literal capture records, reference fields and peer bytes.
Raw reference views, absent native views and bounded control assembly outcomes
remain separate. No era inference, authenticated sender or full NTP parity claim.
"""
import argparse,base64,collections,hashlib,ipaddress,json,struct,zlib
from pathlib import Path
ap=argparse.ArgumentParser(description=__doc__)
for n in['capture','oracle','events','out']:ap.add_argument('--'+n,type=Path,required=True)
ap.add_argument('--manifest',type=Path)
ap.add_argument('--require-association',action='store_true')
ap.add_argument('--require-explained-gaps',action='store_true')
ap.add_argument('--prior-events',type=Path)
ap.add_argument('--retired-oracle',type=Path)
a=ap.parse_args();checks=0
for p in[a.capture,a.events,a.out]:assert not any((q/'.git').exists()for q in[p.resolve(),*p.resolve().parents])
def check(ok,why):
 global checks
 checks+=1;assert ok,why
def eq(x,y,why):check(x==y,(why,x,y))
def load(p):return json.loads(p.read_text(encoding='utf-8-sig'))
def sha(w):return hashlib.sha256(w).hexdigest()
def captures(p):
 w=p.read_bytes();out={};at=0
 if w[:4]in[bytes.fromhex(x)for x in['d4c3b2a1','4d3cb2a1','a1b2c3d4','a1b23c4d']]:
  endian='<'if w[:4]in[bytes.fromhex('d4c3b2a1'),bytes.fromhex('4d3cb2a1')]else'>';at=24;link=struct.unpack_from(endian+'I',w,20)[0]&0xffff
  while at<len(w):
   check(at+16<=len(w),'classic record header');n=struct.unpack_from(endian+'I',w,at+8)[0];at+=16;check(at+n<=len(w),'classic record body');out[len(out)+1]=(w[at:at+n],0,0,link);at+=n
  return out
 section=-1;interfaces=[];endian=None
 while at<len(w):
  check(at+12<=len(w),'PCAPNG block header')
  if w[at:at+4]==bytes.fromhex('0a0d0d0a'):
   endian='<'if w[at+8:at+12]==bytes.fromhex('4d3c2b1a')else'>';section+=1;interfaces=[]
  check(endian is not None,'section precedes blocks');typ,n=struct.unpack_from(endian+'II',w,at);check(n>=12 and n%4==0 and at+n<=len(w),'bounded aligned block');eq(struct.unpack_from(endian+'I',w,at+n-4)[0],n,'block footer');body=w[at+8:at+n-4]
  if typ==1:interfaces.append(struct.unpack_from(endian+'H',body)[0])
  if typ in[2,6]:
   iface=struct.unpack_from(endian+('I'if typ==6 else'H'),body)[0];cap=struct.unpack_from(endian+'I',body,12)[0];check(iface<len(interfaces)and 20+cap<=len(body),'captured packet bounds/interface');out[len(out)+1]=(body[20:20+cap],section,iface,interfaces[iface])
  check(typ!=3,'Simple Packet Blocks are outside this certificate')
  at+=n
 return out

def physical_encapsulation(w,link):
 if link==274:
  # Wireshark packet-fpp.c / IEEE802.3br: alignment0x50, preamble0x55,
  # express SMD-E0xd5, Ethernet MAC frame and trailing little-endian FCS.
  at=1 if w and w[0]==0x50 else 0
  while at<len(w)and w[at]==0x55:at+=1;check(at<=64,'bounded express preamble')
  check(at<len(w)and w[at]==0xd5 and len(w)-at>=19,'complete express mPacket');body=w[at+1:-4];eq(zlib.crc32(body),int.from_bytes(w[-4:],'little'),'literal express Ethernet FCS');return '/mpacket:express'+physical_encapsulation(body,1)
 result=''
 if link in[0,108]:at=4
 elif link in[101,228,229]:at=0
 else:
  check(link==1,'NTP witness link type must have an independently checked domain representation');check(len(w)>=14,'Ethernet header');typ=int.from_bytes(w[12:14],'big');at=14
  while typ in[0x8100,0x88a8,0x9100]:
   check(at+4<=len(w),'VLAN header');result+='/vlan:'+str(int.from_bytes(w[at:at+2],'big')&0xfff);typ=int.from_bytes(w[at+2:at+4],'big');at+=4
  check(typ in[0x0800,0x86dd],'NTP physical certificate selects IPv4/IPv6 plus literal VLAN tags and IP tunnels')
 def addr(b):
  ip=ipaddress.ip_address(b);return str(ip.ipv4_mapped or ip)if isinstance(ip,ipaddress.IPv6Address)else str(ip)
 for depth in range(9):
  check(at<len(w),'network header prefix');v=w[at]>>4
  if v==4:
   check(at+20<=len(w),'IPv4 header');n=(w[at]&15)*4;check(n>=20 and at+n<=len(w),'IPv4 IHL');proto=w[at+9];outer='~'.join(sorted([addr(w[at+12:at+16]),addr(w[at+16:at+20])]))
   if proto not in[4,41]or int.from_bytes(w[at+6:at+8],'big')&0x3fff:return result
   result+=('/ipip:'if proto==4 else'/6in4:')+outer;at+=n
  elif v==6:
   check(at+40<=len(w),'IPv6 header');proto=w[at+6]
   if proto not in[4,41]:return result
   outer='~'.join(sorted([addr(w[at+8:at+24]),addr(w[at+24:at+40])]))
   result+='/ip6-tunnel:'+outer+':'+str(proto);at+=40
  else:check(False,'selected physical network version')
 check(False,'bounded physical IP tunnel depth')

physical=captures(a.capture);ref={};payload={};reference_counts=collections.Counter();oracle_rows=load(a.oracle);nonroot=[];derived=[];reference_context={}
for row in oracle_rows:
 l=row['_source']['layers'];no=int(l['frame']['frame.number']);eq(bytes.fromhex(l['frame_raw'][0]),physical[no][0],'reference physical bytes equal input capture');raw=l.get('ntp_raw')or[];raws=[raw]if raw and isinstance(raw[0],str)else raw;views=l.get('ntp')or[];views=views if isinstance(views,list)else[views]
 reference_context[no]=l
 if not raws and not a.manifest:nonroot.append({'frame':no,'scope':'reference filter matched a nested/quoted or nonroot NTP view; no root UDP NTP certification'})
 u=l.get('udp')or{};us=u if isinstance(u,list)else[u]
 if a.manifest and us:
  eq(len(us),1,'owned client-facing profile has one UDP view');u=us[0];body=bytes.fromhex(u['udp.payload'].replace(':',''));payload[no]=(int(u['udp.srcport']),int(u['udp.dstport']),body)
 for i,r in enumerate(raws):
  wire=bytes.fromhex(r[0]);eq(len(wire),int(r[2]),'reference NTP extent');reference_counts[str((wire[0]>>3&7,wire[0]&7))]+=1
  if len(r)>5 and int(r[5])!=0:
   check(not a.manifest,'owned direct UDP controls cannot silently acquire a different data source');derived.append({'frame':no,'data_source':int(r[5]),'offset':int(r[1]),'bytes':len(wire),'sha256':sha(wire),'version':wire[0]>>3&7,'low_bits':wire[0]&7,'reference_protocol_stack':l['frame']['frame.protocols'],'scope':'reference derived/decompressed/reassembled data source; raw offsets are not physical-frame offsets and this direct-UDP certificate does not establish its carrier lineage'});continue
  eq(physical[no][0][int(r[1]):int(r[1])+len(wire)],wire,'literal NTP captured range');view=views[i]if i<len(views)else{};ref.setdefault(no,[]).append((wire,view,int(r[1])))

es=[json.loads(s)for s in a.events.read_text(encoding='utf-8-sig').splitlines()];by_id={e['id']:e for e in es};eq(len(by_id),len(es),'unique PDU identifiers');matched=set();native=[];states=collections.Counter();logical=[];sdk_by_frame=collections.defaultdict(list);unmatched_native=[];occurrences=collections.Counter()
for e in es:
 if e['protocol']!='ntp':continue
 w=base64.b64decode(e['raw']);refs=e['source_bytes']['PacketRefs'];check(bool(refs),'NTP physical witness');numbers=[r['Number']for r in refs];candidates=[(no,v,start)for no in numbers for rw,v,start in ref.get(no,[])if rw==w]
 if not candidates:unmatched_native.append({'PDU':e['id'],'frames':numbers,'bytes':len(w),'sha256':sha(w),'status':e['status']});continue
 no,view,reference_start=candidates[0];matched.add((no,sha(w)));native.append(e);sdk_by_frame[no].append(e);m=e.get('session')or{};v,mode=w[0]>>3&7,w[0]&7;eq(m.get('Version'),v,'wire version');eq(m.get('Leap Indicator'),w[0]>>6,'wire LI')
 occurrences[(no,sha(w))]+=1;check(occurrences[(no,sha(w))]<=sum(rw==w for rw,_,_ in ref[no]),'native cannot duplicate a root message occurrence')
 eq(e['source_bytes']['SHA256'],sha(w),'native raw PDU source digest')
 for r in refs:
  packet,sec,iface,link=physical[r['Number']];eq((r['Domain']['Section'],r['Domain']['Interface']),(sec,iface),'physical capture domain');eq(r['Domain']['Encapsulation'],physical_encapsulation(packet,link),'literal physical encapsulation domain');eq(r['Domain'],e['domain'],'PDU witness domain')
 aliases={x['name']:x for x in e['field_values']['values']};ranges=m.get('Wire Field Ranges')or{}
 def scalar(key,alias,value,lo,hi):
  eq(m.get(key),value,'native '+key);eq(aliases[alias]['value'],value,'registered '+alias);eq(aliases[alias]['pdu_range'],{'offset':lo,'length':hi-lo},'literal field range '+alias);eq(ranges[key],[lo,hi],'owned wire range '+key)
 def reference_scalar(key,value,lo,size):
  values=view[key];values=values if isinstance(values,list)else[values];raw=view[key+'_raw'];raw=[raw]if raw and isinstance(raw[0],str)else raw;eq(len(values),len(raw),'reference field values and spans pair exactly');indices=[i for i,r in enumerate(raw)if int(r[1])==reference_start+lo and int(r[2])==size];eq(len(indices),1,'one reference field at the literal header position, distinct from data-list fields');x=values[indices[0]];eq(int(x,0)if isinstance(x,str)else int(x),value,'reference '+key)
 scalar('Version','ntp.flags.vn',v,0,1);scalar('Leap Indicator','ntp.flags.li',w[0]>>6,0,1)
 if v==1:
  scalar('Legacy Reserved','ntp.v1.reserved',mode,0,1);check('Mode'not in m and'ntp.flags.mode'not in aliases,'v1 reserved bits are not a modern mode')
 else:scalar('Mode','ntp.flags.mode',mode,0,1)
 flags=view.get('ntp.flags_tree')or{}
 if flags:
  eq(int(flags['ntp.flags.vn']),v,'reference version');eq(int(flags['ntp.flags.mode']),mode,'reference mode');eq(int(flags['ntp.flags.li']),w[0]>>6,'reference LI')
 if v==1 or mode!=6:
  check(len(w)>=48,'complete time header');scalar('Stratum','ntp.stratum',w[1],1,2);eq(m['Poll'],int.from_bytes(w[2:3],'big',signed=True),'signed poll');eq(m['Precision'],int.from_bytes(w[3:4],'big',signed=True),'signed precision')
  for key,value,lo,size in[('ntp.stratum',w[1],1,1),('ntp.ppoll',int.from_bytes(w[2:3],'big',signed=True),2,1),('ntp.precision',int.from_bytes(w[3:4],'big',signed=True),3,1),('ntp.rootdelay',int.from_bytes(w[4:8],'big'),4,4),('ntp.rootdispersion',int.from_bytes(w[8:12],'big'),8,4)]:
   if key in view:reference_scalar(key,value,lo,size)
  if v<=2:
   scalar('Synchronizing Distance Raw','ntp.legacy.distance.raw',int.from_bytes(w[4:8],'big'),4,8);eq(m['Synchronizing Distance Seconds'],int.from_bytes(w[4:8],'big')/65536,'legacy unsigned16.16 distance');check('Root Delay Seconds'not in m and'ntp.rootdelay.raw'not in aliases,'legacy distance cannot become a signed modern root delay')
   if v==1:
    scalar('Estimated Drift Rate Raw','ntp.v1.drift.raw',int.from_bytes(w[8:12],'big'),8,12);eq(m['Estimated Drift Rate'],int.from_bytes(w[8:12],'big')/4294967296,'v1 unsigned0.32 drift');check('Root Dispersion Raw'not in m and'ntp.rootdispersion.raw'not in aliases,'v1 drift cannot become modern dispersion')
   else:
    scalar('Synchronizing Dispersion Raw','ntp.v2.dispersion.raw',int.from_bytes(w[8:12],'big'),8,12);eq(m['Synchronizing Dispersion Seconds'],int.from_bytes(w[8:12],'big')/65536,'v2 unsigned16.16 dispersion')
  else:
   eq(m['Root Delay Seconds'],int.from_bytes(w[4:8],'big',signed=True)/65536,'signed16.16 delay');eq(m['Root Dispersion Seconds'],int.from_bytes(w[8:12],'big')/65536,'unsigned dispersion')
   for key,alias,lo in[('Root Delay Raw','ntp.rootdelay.raw',4),('Root Dispersion Raw','ntp.rootdispersion.raw',8)]:scalar(key,alias,int.from_bytes(w[lo:lo+4],'big'),lo,lo+4)
  eq(m['Reference ID'],w[12:16].hex(),'reference identifier octets')
  for i,(name,alias,rkey)in enumerate([('Reference','reftime','ntp.reftime'),('Origin','org','ntp.org'),('Receive','rec','ntp.rec'),('Transmit','xmt','ntp.xmt')]):
   lo=16+8*i;eq(base64.b64decode(m[name+' Timestamp']),w[lo:lo+8],'exact timestamp octets');scalar(name+' Timestamp Seconds','ntp.'+alias+'.seconds',int.from_bytes(w[lo:lo+4],'big'),lo,lo+4);scalar(name+' Timestamp Fraction','ntp.'+alias+'.fraction',int.from_bytes(w[lo+4:lo+8],'big'),lo+4,lo+8)
   if rkey+'_raw'in view:eq(bytes.fromhex(view[rkey+'_raw'][0]),w[lo:lo+8],'reference timestamp octets')
  check(not any(k.startswith('ntp.ctrl.')for k in aliases),'time packet has no invented control fields')
  if len(w)>48:eq(m['Trailing Bytes'],len(w)-48,'unselected time trailer extent');check(e['status']!='decoded','unselected extension is not a fully decoded time packet')
 else:
  check(len(w)>=12,'complete control header');check('Transmit Timestamp'not in m and'ntp.stratum'not in aliases,'control header is not decoded as a time header');fv=view.get('ntp.ctrl.flags2_tree')or{}
  if a.require_association and m.get('Association')in['observed-request','identical-active-request']:check(m.get('Request PDU ID',0)>0,'request owns a nonzero ID before installing passive state')
  if e['response_to']:
   owner=by_id[e['response_to']];ow=base64.b64decode(owner['raw']);eq(owner['protocol'],'ntp','observed owner protocol');check(owner['id']<e['id']and ow[0]&7==6 and ow[1]&128==0,'response points to an earlier captured control request');eq((owner['source'],owner['destination']),(e['destination'],e['source']),'reversed request endpoints');eq(owner['domain'],e['domain'],'request capture domain');eq(ow[0],w[0],'request/control version and mode');eq(ow[1]&31,w[1]&31,'request opcode');eq(ow[2:4],w[2:4],'request sequence');requested=int.from_bytes(ow[6:8],'big');selected_peer=requested==0 and w[1]&31 in[2,3]and w[1]&64==0;check(ow[6:8]==w[6:8]or selected_peer,'response association matches request or the documented selected-peer response');eq(m['Requested Association ID'],requested,'literal request association retained');eq(aliases['ntp.control.request_association']['value'],requested,'registered request association');check('pdu_range'not in aliases['ntp.control.request_association'],'request association has no invented current-packet span');eq(e['transaction'],owner['id'],'one observed request transaction');eq(m['Request PDU ID'],owner['id'],'session request field');eq(aliases['ntp.control.request_pdu']['value'],owner['id'],'registered request field')
  for key,alias,bit in[('Response','r',128),('Control Error','error',64),('More Fragments','more',32)]:
   scalar(key,'ntp.ctrl.flags2.'+alias,bool(w[1]&bit),1,2)
   if fv:eq(bool(int(fv['ntp.ctrl.flags2.'+alias])),bool(w[1]&bit),'reference control flag')
  scalar('Opcode','ntp.ctrl.flags2.opcode',w[1]&31,1,2)
  for i,(key,alias)in enumerate([('Sequence Number','sequence'),('Status Word','status'),('Association ID','associd'),('Fragment Offset','offset'),('Data Count','count')]):
   lo=2+2*i;value=int.from_bytes(w[lo:lo+2],'big');scalar(key,'ntp.ctrl.'+alias,value,lo,lo+2)
   if'ntp.ctrl.'+alias in view:reference_scalar('ntp.ctrl.'+alias,value,lo,2)
  n=int.from_bytes(w[10:12],'big')
  if n<=468 and n<=len(w)-12:eq(base64.b64decode(m['Control Data']or''),w[12:12+n],'exact fragment data')
  if'Status Word Type'in m:
   response,error,op=bool(w[1]&128),bool(w[1]&64),w[1]&31;status=int.from_bytes(w[4:6],'big');assoc=int.from_bytes(w[6:8],'big');reserved=op not in[*range(1,13),31]
   expected='request-reserved'if not response else'error'if error else'insignificant-set-trap-response'if op==6 else'unselected-opcode'if reserved else'clock'if op in[4,5]else'system'if assoc==0 else'peer';eq(m['Status Word Type'],expected,'control status meaning follows response/error/opcode/association')
   if expected=='clock':eq(m['Clock Status High Byte'],status>>8,'literal clock high byte');eq(m['Clock Status Low Byte'],status&255,'literal clock low byte');check('Reported Clock Status'not in m,'unnegotiated clock-word variant cannot imply hardware state')
   if expected=='error':eq(m['Reported Error Code'],status>>8,'reported control error')
   if expected=='system':eq(m['Reported Leap Indicator'],status>>14,'reported system leap');eq(m['Reported Clock Source'],status>>8&63,'reported system source')
   if expected=='peer':eq(m['Reported Peer Flags'],status>>11,'reported peer flags');eq(m['Reported Peer Selection'],status>>8&7,'reported peer selection')
   if response and not error and op not in[4,5,6]and not reserved:eq(m['Reported Event Count'],status>>4&15,'reported event count');eq(m['Reported Event Code'],status&15,'reported event code')
  if e['status']=='decoded'and w[1]&128==0:
   eq(m['Status Word Type'],'request-reserved','request status is reserved, not a reported clock state');check(not any(k.startswith('Reported ')for k in m),'request cannot imply server health')
  if m.get('Authentication Verified')is not None:eq(m['Authentication Verified'],False,'optional MAC is not verified')
  states[m.get('Control Reassembly','wire-refused')]+=1
  if'Reassembly Complete'in m:
   eq(m['Reassembly Complete'],m['Control Reassembly']=='complete','assembled bytes and reassembly verdict have distinct, consistent scope');eq(aliases['ntp.control.complete']['value'],m['Reassembly Complete'],'registered completion flag')
   if not m['Reassembly Complete']:check('Reassembled Control Data'not in m,'pending/refused/duplicate cannot replay a logical payload')
  if m.get('Reassembly Complete'):
   check(e['response_to']>0 or e['status']=='malformed'and'Control Payload Failure'in m,'complete response bytes require an observed request unless semantic failure explicitly refuses association')
   data=base64.b64decode(m['Reassembled Control Data']or'');eq(sha(data),m['Reassembled Control SHA256'],'assembled digest');src=m['Reassembled Byte Source'];eq(src['Length'],len(data),'assembled extent');eq(src['Offset'],0,'assembly starts at zero');eq(src['Capture Domain'],e['domain'],'assembly domain');eq(src['Byte Coverage Complete'],True,'complete observed byte coverage');eq(src['Coverage'],'complete','complete physical and byte witnesses');covered=set();parents=set()
   for r in src['Ranges']:
    parent=by_id[r['Parent PDU']];pw=base64.b64decode(parent['raw']);lo,n=r['Offset'],r['Length'];po=int.from_bytes(pw[8:10],'big');eq(parent['source'],e['source'],'same response sender');eq(parent['destination'],e['destination'],'same response receiver');eq(parent['domain'],e['domain'],'same capture domain');eq(pw[2:4],w[2:4],'same control sequence');eq(pw[6:8],w[6:8],'same association');eq(pw[1]&31,w[1]&31,'same opcode');check(po<=lo<=lo+n<=po+int.from_bytes(pw[10:12],'big'),'contributor data extent');eq(pw[12+lo-po:12+lo-po+n],data[lo:lo+n],'literal contributed bytes');covered.update(range(lo,lo+n));parents.add(parent['id'])
    for pr in r['Packet References']:check(any(rr['Number']==pr['Packet']and rr['Domain']=={'Section':pr['Section'],'Interface':pr['Interface'],'Encapsulation':pr['Encapsulation']}for rr in parent['source_bytes']['PacketRefs']),'contributor physical witness')
   eq(covered,set(range(len(data))),'assembled byte coverage');eq(set(src['Parent PDUs']),parents,'assembled parent set');eq(src['Packet Coverage Complete'],True,'physical coverage retained');logical.append({'PDU':e['id'],'frame':no,'bytes':len(data),'sha256':sha(data),'request_PDU':e['response_to']})

peer_checks=0
if a.manifest:
 m=load(a.manifest);eq(m['capture_ready_observed'],True,'capture-observed readiness');queues=collections.defaultdict(list)
 for row in m['trace']:queues[(row['source_port'],row['destination_port'],row['wire_hex'])].append(row)
 for no,(sp,dp,w)in payload.items():
  key=(sp,dp,w.hex());check(bool(queues[key]),'every client-facing captured datagram has an exact send/receive receipt');row=queues[key].pop(0);events=sdk_by_frame[no];eq(len(events),1,'one native PDU per selected datagram');e=events[0];s=e.get('session')or{}
  if'expected_status'in row:eq(e['status'],row['expected_status'],row['name']+' wire status')
  if'expected_reassembly'in row:eq(s.get('Control Reassembly'),row['expected_reassembly'],row['name']+' assembly verdict')
  if'expected_data_hex'in row:eq(base64.b64decode(s['Reassembled Control Data']or''),bytes.fromhex(row['expected_data_hex']),row['name']+' exact received logical bytes')
  if'expected_authentication'in row:eq(s['Authentication Verified'],row['expected_authentication'],row['name']+' optional MAC flag')
  if'expected_response_to'in row:eq(e['response_to'],row['expected_response_to'],row['name']+' refused owner')
  if'expected_requested_association'in row:eq(s['Requested Association ID'],row['expected_requested_association'],row['name']+' requested association')
  peer_checks+=1
 for rows in queues.values():check(all(r['name']=='readiness-request'for r in rows),'only pre-readiness attempts may be outside capture')

def absence_reason(no,w,start):
 v,mode=w[0]>>3&7,w[0]&7
 if not 1<=v<=4:return 'unselected-version'
 if v==1 and mode!=0:return 'v1-nonzero-reserved-low-bits'
 if v!=1 and mode not in range(1,7):return 'unselected-mode'
 if mode==6:
  if len(w)<12:return 'truncated-control-header'
  if w[0]>>6:return 'control-LI-must-be-zero'
  if int.from_bytes(w[2:4],'big')==0:return 'zero-sequence-auto-admission-unselected'
  if w[1]&31 not in[*range(1,13),31]:return 'reserved-control-opcode-auto-admission-unselected'
 else:
  if len(w)<48:return 'truncated-time-header'
  if v!=1 and w[1]>(15 if v==2 else 16):return 'invalid-reported-stratum'
 context=reference_context[no];us=context.get('udp_raw')or[];us=[us]if us and isinstance(us[0],str)else us
 for u in us:
  if (len(u)<6 or int(u[5])==0)and int(u[1])+8==start:
   physical_header=physical[no][0][start-8:start];eq(physical_header,bytes.fromhex(u[0]),'absent candidate literal UDP header');declared=int.from_bytes(physical_header[4:6],'big')
   if declared>len(physical[no][0])-(start-8):return 'outer-UDP-length-exceeds-captured-extent'
 if ':tcp:ntp'in context['frame']['frame.protocols']:return 'TCP-control-layout-outside-selected-UDP-profile'
 return 'unresolved-selected-profile-gap'

missing=[{'frame':no,'sha256':sha(w),'version':w[0]>>3&7,'low_bits':w[0]&7,'bytes':len(w),'reason':absence_reason(no,w,start)}for no,vs in ref.items()for w,v,start in vs if(no,sha(w))not in matched]
if a.require_explained_gaps:check(all(x['reason']!='unresolved-selected-profile-gap'for x in missing),'every absent physical reference candidate needs a literal grammar/carrier or explicit selected-profile explanation')
retired=[]
if a.prior_events or a.retired_oracle:
 check(bool(a.prior_events and a.retired_oracle),'prior export and independently regenerated retired-frame reference are paired');retired_ref={int(r['_source']['layers']['frame']['frame.number']):r['_source']['layers']for r in load(a.retired_oracle)}
 for line in a.prior_events.open(encoding='utf-8-sig'):
  old=json.loads(line)
  if old['protocol']!='ntp':continue
  new=by_id[old['id']]
  if new['protocol']=='ntp':continue
  eq(new['protocol'],'','legacy NTP candidate retires to unknown UDP');eq(new['transport'],'udp','retired datagram transport');eq(new['response_to'],0,'retired candidate cannot own a response');eq(new['transaction'],0,'retired candidate cannot own a transaction');refs=new['source_bytes']['PacketRefs'];eq(len(refs),1,'one physical datagram witness');no=refs[0]['Number'];l=retired_ref[no];packet=physical[no][0];eq(bytes.fromhex(l['frame_raw'][0]),packet,'retired reference literal physical bytes');check('ntp'not in l,'independent reference does not select NTP');w=base64.b64decode(old['raw']);check(len(w)>48,'rejected candidate relied on an extended-message legacy probe');us=l.get('udp')or{};us=us if isinstance(us,list)else[us];us=[u for u in us if bytes.fromhex(u.get('udp.payload','').replace(':',''))==w];eq(len(us),1,'reference retains the exact rejected UDP payload');u=us[0];raw=u['udp.payload_raw'];eq(int(raw[5]),0,'retired payload is in the physical source');at=int(raw[1]);eq(packet[at:at+len(w)],w,'literal rejected payload extent');eq(int.from_bytes(packet[at-4:at-2],'big'),len(w)+8,'complete UDP extent');check(123 not in[int.from_bytes(packet[at-8:at-6],'big'),int.from_bytes(packet[at-6:at-4],'big')],'rejected prefix has no NTP service selection');eq(base64.b64decode(new['raw']),w[:len(base64.b64decode(new['raw']))],'unknown packet preserves the literal probe');eq(new['domain'],old['domain'],'retirement preserves capture identity');retired.append({'PDU':new['id'],'frame':no,'bytes':len(w),'sha256':sha(w),'reference_protocol_stack':l['frame']['frame.protocols'],'verdict':'legacy prefix no longer bypasses native UDP admission'})
 eq(set(retired_ref),{r['frame']for r in retired},'all supplied retired-frame references are reviewed')
out={'scope':__doc__,'checks':checks,'physical_frames':len(physical),'reference_filter_rows':len(oracle_rows),'reference_raw_views':sum(len(v)for v in ref.values())+len(derived),'reference_views':sum(len(v)for v in ref.values()),'reference_nonroot_views':nonroot,'reference_derived_source_views':derived,'matched_native':len(native),'missing_native_reference_views':missing,'missing_reference_reasons':dict(collections.Counter(x['reason']for x in missing)),'unmatched_native_views':unmatched_native,'retired_legacy_candidates':retired,'native_control_states':dict(states),'complete_control_responses':logical,'peer_datagrams_checked':peer_checks,'raw_reference_profiles':dict(reference_counts),'capture_sha256':sha(a.capture.read_bytes()),'oracle_sha256':sha(a.oracle.read_bytes()),'events_sha256':sha(a.events.read_bytes())};a.out.parent.mkdir(parents=True,exist_ok=True);a.out.write_text(json.dumps(out,indent=2)+'\n',encoding='utf8');print(json.dumps({k:v for k,v in out.items()if k not in['scope','complete_control_responses','missing_native_reference_views','reference_nonroot_views','reference_derived_source_views','unmatched_native_views']}));print('Missing/unmatched/nonroot/derived-source views',len(missing),len(unmatched_native),len(nonroot),len(derived))
