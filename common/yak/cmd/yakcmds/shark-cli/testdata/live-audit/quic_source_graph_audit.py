"""Independently verify native HTTP/3 byte witnesses against reference STREAMs.
Scope: owned, single-UDP-view capture; complete bidi HTTP streams and uni control
streams. Encoder/decoder instruction semantics and peer identity are not certified.
"""
import argparse,base64,collections,hashlib,ipaddress,json,struct
from pathlib import Path
ap=argparse.ArgumentParser(description=__doc__)
for n in ['capture','oracle','events','out']:ap.add_argument('--'+n,required=True,type=Path)
a=ap.parse_args();checks=0
for p in [a.capture,a.out]:
 assert not any((q/'.git').exists() for q in [p.resolve(),*p.resolve().parents]),'captures and outputs stay outside Git'
def check(ok,why):
 global checks
 checks+=1;assert ok,why
def eq(x,y,why):check(x==y,(why,x,y))
def digest(b):return hashlib.sha256(b).hexdigest()

# This certificate is deliberately limited to this owned, classic-PCAP NULL
# capture. Bind each oracle frame and native packet reference to literal input
# records before trusting a decoded endpoint or a capture-domain identifier.
wire=a.capture.read_bytes();check(wire[:4] in [bytes.fromhex('d4c3b2a1'),bytes.fromhex('4d3cb2a1')],'selected little-endian classic PCAP')
eq(struct.unpack_from('<HH',wire,4),(2,4),'PCAP version');eq(struct.unpack_from('<I',wire,20)[0],0,'selected NULL datalink')
captured={};at=24
while at<len(wire):
 check(at+16<=len(wire),'capture record header');_,_,n,original=struct.unpack_from('<IIII',wire,at);at+=16
 check(at+n<=len(wire) and n==original,'complete owned capture record');captured[len(captured)+1]=wire[at:at+n];at+=n
eq(at,len(wire),'capture end')

def endpoints(raw):
 check(raw[:4]==b'\x02\0\0\0','selected IPv4 NULL header');w=raw[4:];check(len(w)>=20 and w[0]>>4==4,'IPv4 header');ihl=(w[0]&15)*4
 check(20<=ihl<=len(w) and w[9]==17,'literal IPv4 UDP');eq(int.from_bytes(w[6:8],'big')&0x3fff,0,'unfragmented owned UDP')
 total=int.from_bytes(w[2:4],'big');check(ihl+8<=total<=len(w),'IP extent');u=w[ihl:total];eq(int.from_bytes(u[4:6],'big'),len(u),'UDP extent')
 return (str(ipaddress.ip_address(w[12:16]))+':'+str(int.from_bytes(u[:2],'big')),str(ipaddress.ip_address(w[16:20]))+':'+str(int.from_bytes(u[2:4],'big')))
def vals(x,key):
 if isinstance(x,dict):return [z for k,v in x.items() for z in ((v if isinstance(v,list) else [v]) if k==key else vals(v,key))]
 if isinstance(x,list):return [z for v in x for z in vals(v,key)]
 return []
def vi(w,at):
 check(at<len(w),'varint prefix');n=1<<(w[at]>>6);check(at+n<=len(w),'complete varint');return int.from_bytes(w[at:at+n],'big')&((1<<(8*n-2))-1),at+n
def covered(spans,lo,hi):
 at=lo
 for x,y in sorted(spans):
  if x>at:return False
  at=max(at,y)
 return at>=hi
streams={};packet_conn={};packet_endpoints={};physical=collections.defaultdict(list);coalesced_bindings=0;expert_only_blocks=0
for packet in json.loads(a.oracle.read_text(encoding='utf-8-sig')):
 l=packet['_source']['layers'];number=int(l['frame']['frame.number']);sp,dp=vals(l.get('udp',{}),'udp.srcport'),vals(l.get('udp',{}),'udp.dstport');eq(len(sp),1,'single reference UDP source');eq(len(dp),1,'single reference UDP destination')
 eq(bytes.fromhex(l['frame_raw'][0]),captured[number],'oracle bytes are literal physical capture record');packet_endpoints[number]=endpoints(captured[number]);eq(tuple(x.rsplit(':',1)[1] for x in packet_endpoints[number]),(sp[0],dp[0]),'oracle ports equal capture bytes')
 qs=l.get('quic',[]);qs=qs if isinstance(qs,list) else [qs]
 raw_packets=l.get('quic_raw',[]);raw_packets=[raw_packets]if raw_packets and isinstance(raw_packets[0],str)else raw_packets
 packet_connections=set(vals(qs,'quic.connection.number'));dcids=set(vals(qs,'quic.dcid'))
 if not packet_connections:continue
 eq(len(packet_connections),1,'one reference connection per UDP datagram')
 for qi,q in enumerate(qs):
  # TShark's JSON array can contain a separate expert-only annotation rather
  # than another QUIC packet. It has no wire fields or STREAM bytes to bind.
  if set(q)=={'_ws.expert'}:expert_only_blocks+=1;continue
  cs=vals(q,'quic.connection.number')
  if not cs:
   # TShark prints connection metadata on only the first coalesced packet.
   # Bind the remaining packet only when all dissected DCIDs agree too.
   explicit_zero=False
   if not dcids and vals(q,'quic.dcil')==['0']:
    check(qi<len(raw_packets),'coalesced zero-CID packet has a literal raw view');r=raw_packets[qi];raw=bytes.fromhex(r[0]);start=int(r[1]);eq(captured[number][start:start+len(raw)],raw,'coalesced zero-CID raw span')
    check(len(raw)>=7 and raw[0]&0xc0==0xc0 and raw[1:5]==b'\0\0\0\1'and raw[5]==0,'literal v1 long header carries zero DCID length');explicit_zero=True
   check(len(qs)>1 and (len(dcids)==1 and bool(vals(q,'quic.dcid'))or explicit_zero),'unambiguous coalesced reference connection/DCID')
   cs=list(packet_connections);coalesced_bindings+=1
  eq(len(set(cs)),1,'reference connection');conn=cs[0];check(number not in packet_conn or packet_conn[number]==conn,'single connection datagram');packet_conn[number]=conn
  frames=q.get('quic.frame',[]);frames=frames if isinstance(frames,list) else [frames]
  for f in frames:
   typ=int(f.get('quic.frame_type','0'),0)
   if not 8<=typ<=15:continue
   sid=int(f['quic.stream.stream_id']);off=int(f.get('quic.stream.offset','0'));raw=f.get('quic.stream_data_raw');body=bytes.fromhex(raw[0]) if raw else b'';key=(conn,sp[0],dp[0],sid)
   s=streams.setdefault(key,{})
   for i,b in enumerate(body,off):
    if i in s:eq(s[i],b,'identical overlapping STREAM bytes')
    s[i]=b
   physical[(number,key)].append((off,body))
expected={};skipped=[]
for key,cells in streams.items():
 end=max(cells,default=-1)+1;eq(len(cells),end,'complete selected STREAM bytes');w=bytes(cells[i] for i in range(end));at=0
 if key[3]&2:
  if not w:continue
  kind,at=vi(w,0)
  if kind!=0:skipped.append({'stream':list(key),'kind':kind});continue
 while at<len(w):
  start=at;typ,at=vi(w,at);length,payload=vi(w,at);end=payload+length;check(end<=len(w),'complete independent HTTP3 frame');expected[(*key,start)]={'type':typ,'length':length,'header':w[start:payload],'header_ranges':{'Frame Type':[0,at-start],'Frame Type Value':[0,at-start],'Payload Length':[at-start,payload-start]},'payload':w[payload:end],'sha256':digest(w[start:end]),'end':end};at=end
events=[json.loads(s) for s in a.events.read_text(encoding='utf-8-sig').splitlines()];by_id={int(e['id']):e for e in events};eq(len(by_id),len(events),'unique native IDs');flow_conn={};actual={};ranges_checked=0;metadata=[]
def source_refs(e):return [{'Packet':r['Number'],'Section':r['Domain']['Section'],'Interface':r['Domain']['Interface'],'Encapsulation':r['Domain']['Encapsulation']} for r in e['source_bytes']['PacketRefs']]

def physical_identity(e):
 eq(e['domain'],{'Section':0,'Interface':0,'Encapsulation':''},'literal outer capture domain')
 for ref in e['source_bytes']['PacketRefs']:
  eq(ref['Domain'],e['domain'],'outer reference domain');eq((e['source'],e['destination']),packet_endpoints[ref['Number']],'endpoints equal literal source packet bytes')
def proof(src,key,lo,hi,e):
 global ranges_checked
 eq(src['Kind'],'quic-authenticated-stream-reassembly','proof coordinate system')
 eq(src['Stream ID'],key[3],'proof stream');eq(src['Direction'],e['direction'],'proof direction');eq(src['Offset'],lo,'proof range start');eq(src['Length'],hi-lo,'proof range length');spans=[];parents=set();packets=[]
 for r in (src['Ranges'] or []):
  pdu=int(r['Parent PDU']);check(pdu in by_id,'known source PDU');p=by_id[pdu];check(pdu<=int(e['id']),'source witness observed before application emission');eq(p['flow'],e['flow'],'same native connection');eq(p['domain'],e['domain'],'same capture domain');eq(p['direction'],e['direction'],'same native direction');eq((p['source'],p['destination']),(e['source'],e['destination']),'same source endpoints');eq(p['session'].get('Authentication Verified'),True,'source bytes authenticated');physical_identity(p)
  x,y=int(r['Offset']),int(r['Offset'])+int(r['Length']);check(lo<=x<y<=hi,'witness within requested range');refs=source_refs(p);eq(r['Packet References'],refs,'exact source PDU physical references');eq(r['Packet References Complete'],True,'complete selected physical references')
  found=False
  for ref in refs:
   eq(packet_conn[ref['Packet']],key[0],'same reference connection')
   for off,body in physical[(ref['Packet'],key)]:
    if off<=x and y<=off+len(body):
     eq(body[x-off:y-off],bytes(streams[key][i] for i in range(x,y)),'witness literal reference bytes');found=True
  check(found,'source range exists in its actual physical STREAM frame');spans.append((x,y));parents.add(pdu);packets+=refs;ranges_checked+=1
 eq(src['Parent PDUs'],sorted(parents),'exact parent PDU set');unique={tuple(r[k] for k in ['Packet','Section','Interface','Encapsulation']):r for r in packets};flat=[unique[k] for k in sorted(unique)]
 eq(src['Packet References'],flat,'exact physical packet set');check(covered(spans,lo,hi),'complete retained witness byte coverage');eq(src['Byte Coverage Complete'],True,'byte coverage flag');eq(src['Packet Coverage Complete'],True,'physical coverage flag');eq(src['Coverage'],'complete','coverage status')
for e in events:
 session=e.get('session') or {};native=[]
 stream_views=[fr for fr in session.get('Frames',[]) if fr.get('Frame Type')=='STREAM']
 if stream_views:
  physical_identity(e)
  refs=e['source_bytes']['PacketRefs'];conns={packet_conn[r['Number']] for r in refs};eq(len(conns),1,'STREAM reference connection');stream_conn=next(iter(conns));ssp=e['source'].rsplit(':',1)[1];sdp=e['destination'].rsplit(':',1)[1]
 for fr in session.get('Frames',[]):
  if fr.get('Frame Type')=='STREAM':
   eq(fr['Byte Source']['Parent PDU'],e['id'],'raw STREAM belongs to its current PDU');eq(fr['Byte Source']['Stream ID'],fr['Stream ID'],'raw STREAM source ID');eq(fr['Byte Source']['Offset'],fr['Offset'],'raw STREAM offset is not delivery suffix');eq(fr['Byte Source']['Length'],len(base64.b64decode(fr.get('Stream Data')or'')),'raw STREAM source length')
  if int(fr.get('Delivered Bytes',0))>0:
   lo=int(fr['Contiguous Offset']);proof(fr['Delivered Byte Source'],(stream_conn,ssp,sdp,int(fr['Stream ID'])),lo,lo+int(fr['Delivered Bytes']),e)
  native+=fr.get('HTTP3 Frames',[])
 for resumed in session.get('QPACK Resumed Streams',[]):native+=resumed.get('HTTP3 Frames',[])
 if not native:continue
 refs=e['source_bytes']['PacketRefs'];check(bool(refs),'capture binding');conns={packet_conn[r['Number']] for r in refs};eq(len(conns),1,'application connection');conn=next(iter(conns));flow_conn.setdefault(e['flow'],conn);eq(flow_conn[e['flow']],conn,'stable native/reference connection binding');sp=e['source'].rsplit(':',1)[1];dp=e['destination'].rsplit(':',1)[1]
 eq([v['value'] for v in e['field_values']['values'] if v['name']=='http3.frame_type'],[h['Frame Type Value'] for h in native],'typed frame-type projection')
 eq([v['value'] for v in e['field_values']['values'] if v['name']=='http3.frame_length'],[h['Payload Length'] for h in native],'typed frame-length projection')
 for h in native:
  key=(conn,sp,dp,int(h['Stream ID']));start=int(h['Stream Offset']);fk=(*key,start);check(fk in expected,('independent application frame',fk,e['id'],h['Frame Type'],[k for k in expected if k[:4]==key]));check(fk not in actual,'single application frame delivery');actual[fk]=True;r=expected[fk]
  eq(h['Frame Type Value'],r['type'],'literal type');eq(h['Payload Length'],r['length'],'literal payload length');header=base64.b64decode(h['Frame Header Bytes']);eq(header,r['header'],'literal varint header');eq(h['Frame Wire Length'],len(header)+r['length'],'whole frame wire length');eq(h['Frame SHA256'],r['sha256'],'whole logical frame digest')
  eq(h['Frame Header Ranges'],r['header_ranges'],'literal local header-byte selection ranges')
  if h['Frame Type']=='DATA':eq(base64.b64decode(h['Payload']),r['payload'],'DATA bytes')
  if h['Frame Type']=='HEADERS':eq(base64.b64decode(h['QPACK Block']),r['payload'],'QPACK wire block')
  proof(h['Byte Source'],key,start,r['end'],e);proof(h['Payload Byte Source'],key,start+len(header),r['end'],e)
  metadata.append({'completed_physical_frame':refs[-1]['Number'],'parent_PDU':e['id'],'stream_id':key[3],'stream_offset':start,'type':r['type'],'payload_length':r['length'],'source_PDUs':h['Byte Source']['Parent PDUs'],'physical_source_packets':[x['Packet'] for x in h['Byte Source']['Packet References']]})
eq(set(actual),set(expected),'all independently reconstructed selected HTTP3 frames')
out={'scope':__doc__,'capture_sha256':digest(wire),'checks':checks,'connections':len(flow_conn),'reference_coalesced_bindings':coalesced_bindings,'reference_expert_only_blocks':expert_only_blocks,'application_frames':len(actual),'types':dict(collections.Counter(str(expected[k]['type']) for k in actual)),'source_ranges':ranges_checked,'unselected_instruction_streams':skipped,'selected_differences':[],'frames':metadata}
a.out.parent.mkdir(parents=True,exist_ok=True);a.out.write_text(json.dumps(out,indent=2)+'\n',encoding='utf8');print(json.dumps({k:v for k,v in out.items() if k not in ['frames','unselected_instruction_streams']}))
