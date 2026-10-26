"""Compare declared real-socket HTTP3 fields with literal peer and reference bytes.
Requires the complete capture-ready three-generation http3_field_live experiment.
Frame byte-source coverage is checked separately by quic_source_graph_audit.py.
"""
import argparse,base64,collections,hashlib,json
from pathlib import Path
ap=argparse.ArgumentParser(description=__doc__)
for n in ['manifest','oracle','events','out']:ap.add_argument('--'+n,required=True,type=Path)
ap.add_argument('--profile',choices=['malformed69','declared-valid'],default='malformed69')
a=ap.parse_args();checks=0
assert not any((p/'.git').exists() for p in [a.out.resolve(),*a.out.resolve().parents])
def load(p):return json.loads(p.read_text(encoding='utf-8-sig'))
def check(ok,reason):
 global checks
 checks+=1;assert ok,reason
def eq(x,y,reason):check(x==y,(reason,x,y))
def sha(w):return hashlib.sha256(w).hexdigest()
def vals(x,key):
 if isinstance(x,dict):return [z for k,v in x.items() for z in ((v if isinstance(v,list) else [v]) if k==key else vals(v,key))]
 if isinstance(x,list):return [z for v in x for z in vals(v,key)]
 return []
def maps(x,key):
 if isinstance(x,dict):return [z for k,v in x.items() for z in ((v if isinstance(v,list) else [v]) if k==key else maps(v,key))]
 if isinstance(x,list):return [z for v in x for z in maps(v,key)]
 return []
def vi(w,at):
 check(at<len(w),'literal varint prefix');n=1<<(w[at]>>6);check(at+n<=len(w),'complete literal varint');return int.from_bytes(w[at:at+n],'big')&((1<<(n*8-2))-1),at+n
def pref(w,at,bits):
 check(at<len(w),'QPACK integer prefix');n=w[at]&((1<<bits)-1);at+=1
 if n==(1<<bits)-1:
  shift=0
  while True:
   check(at<len(w) and shift<56,'bounded literal QPACK integer');b=w[at];at+=1;n+=(b&127)<<shift
   if b<128:break
   shift+=7
 return n,at
def literal_fields(w):
 eq(w[:2],b'\0\0','declared no-dynamic-table prefix');at=2;hs=[]
 while at<len(w):
  eq(w[at]&0xf8,0x20,'literal-name uncompressed field');n,at=pref(w,at,3);check(at+n<len(w),'complete name and value prefix');name=w[at:at+n];at+=n
  eq(w[at]&128,0,'literal value is not Huffman');n,at=pref(w,at,7);check(at+n<=len(w),'complete literal value');value=w[at:at+n];at+=n;hs.append((name,value))
 return hs
def split_frames(w):
 at=0;out=[]
 while at<len(w):
  start=at;typ,at=vi(w,at);n,at=vi(w,at);check(at+n<=len(w),'complete peer application frame');out.append((start,typ,w[at:at+n],sha(w[start:at+n])));at+=n
 return out

def reference_frames(node):
 if isinstance(node,list):
  for v in node:yield from reference_frames(v)
 elif isinstance(node,dict):
  if 'http3.frame' in node:
   rows=node['http3.frame'];rows=rows if isinstance(rows,list) else [rows];raws=node.get('http3.frame_raw',[])
   if raws and isinstance(raws[0],str):raws=[raws]
   eq(len(rows),len(raws),'reference frame/raw pairing')
   for h,raw in zip(rows,raws):yield h,bytes.fromhex(raw[0])
  for k,v in node.items():
   if k not in ['http3.frame','http3.frame_raw']:yield from reference_frames(v)

m=load(a.manifest);eq(m['capture_ready_observed'],True,'capture readiness observed')
if a.profile=='malformed69':eq(len(m['operations']),69,'all declared malformed-matrix operations')
else:
 eq(len(m['profiles']),3,'three declared reused endpoint epochs');eq(sum(p['requests']for p in m['profiles']),len(m['operations']),'every declared peer request has a receipt');check(bool(m['operations'])and all(o['expected_base_scope']=='base'for o in m['operations']),'declared-valid profile cannot skip malformed cases')
cp,sp=str(m['client_port']),str(m['server_port'])
streams={};packet_connection={};first={};ws_headers={};ws_frame_count=collections.Counter();ws_frame_keys=collections.defaultdict(list)
for p in load(a.oracle):
 l=p['_source']['layers'];no=int(l['frame']['frame.number']);cs=set(vals(l.get('quic',{}),'quic.connection.number'))
 if not cs:continue
 eq(len(cs),1,'single reference connection per captured datagram');conn=next(iter(cs));packet_connection[no]=conn;first.setdefault(conn,no)
 src=vals(l.get('udp',{}),'udp.srcport');dst=vals(l.get('udp',{}),'udp.dstport');eq(len(src),1,'single UDP source');eq(len(dst),1,'single UDP destination');direction=0 if src[0]==cp else 1;eq((src[0],dst[0]),(cp,sp) if direction==0 else (sp,cp),'literal experiment endpoints')
 for fr in maps(l.get('quic',{}),'quic.frame'):
  if not 8<=int(fr.get('quic.frame_type','0'),0)<=15:continue
  sid=int(fr['quic.stream.stream_id']);off=int(fr.get('quic.stream.offset',0));body=bytes.fromhex((fr.get('quic.stream_data_raw')or[''])[0]);cells=streams.setdefault((conn,direction,sid),{})
  for i,b in enumerate(body,off):
   if i in cells:eq(cells[i],b,'identical reference STREAM overlap')
   cells[i]=b
 for h,wire in reference_frames(l.get('http3',{})):
  typ=int(h['http3.frame_type'],0);ws_frame_count[str(typ)]+=1
  ws_frame_keys[(conn,direction,int(h['http3.frame_streamid']),typ,sha(wire))].append(no)
  if typ!=1:continue
  sid=int(h['http3.frame_streamid']);hs=[];values=h.get('http3.headers.header',[]);values=values if isinstance(values,list) else [values]
  for v in values:
   if isinstance(v,str):raise AssertionError('expected reference header maps')
   name=bytes.fromhex(v['http3.header.header.name_raw'][0]);eq(len(name),int(v['http3.headers.header.name.length']),'literal reference field-name length')
   raw=v.get('http3.headers.header.value_raw')
   if raw:value=bytes.fromhex(raw[0])
   else:
    eq(v['http3.headers.header.value.length'],'0','omitted raw value has explicit zero length');eq(v['http3.headers.header.value'],'','omitted raw value is empty');value=b''
   eq(len(value),int(v['http3.headers.header.value.length']),'literal reference field-value length');hs.append((name,value))
  key=(conn,direction,sid,sha(wire));ws_headers.setdefault(key,[]).append(hs)

order=sorted(first,key=first.get);eq(len(order),3,'all three reused-socket generations captured');expected={};cases={}
for op in m['operations']:
 conn=order[op['epoch']];sid=op['stream_id'];cases[(conn,sid)]=op
 for direction,keyname in [(0,'request_hex'),(1,'reply_hex')]:
  wire=bytes.fromhex(op[keyname]);cells=streams[(conn,direction,sid)];eq(len(cells),len(wire),'all peer bytes observed, with no invented suffix');eq(bytes(cells[i] for i in range(len(wire))),wire,'reference STREAM bytes equal actual peer receipt');fs=split_frames(wire)
  for index,(offset,typ,payload,digest) in enumerate(fs):
   if typ!=1:continue
   status='base-shape-valid'
   if direction==0 and op['expected_base_scope']!='base' and (index==len(fs)-1 or op['case'] not in ['pseudo-in-trailers','third-header-section']):status=op['expected_base_scope']
   expected[(conn,direction,sid,offset)]={'headers':literal_fields(payload),'status':status,'sha256':digest,'case':op['case']}

events=[json.loads(s) for s in a.events.read_text(encoding='utf-8-sig').splitlines()];actual={};bindings={};counts=collections.Counter();ws_missing=[];ws_differences=[];mixed=[]
for e in events:
 session=e.get('session')or{};frames=session.get('Frames',[])
 if not any(fr.get('HTTP3 Frames') for fr in frames):continue
 eq(e['protocol'],'http3','native protocol matches authenticated application frames');eq(e['transport'],'udp','native wire transport');eq(session.get('Authentication Verified'),True,'authenticated application observation');eq(session.get('ALPN'),'h3','observed application selection')
 refs=e['source_bytes']['PacketRefs'];conns={packet_connection[r['Number']] for r in refs};eq(len(conns),1,'native/reference connection binding');conn=next(iter(conns));bindings.setdefault(e['flow'],conn);eq(bindings[e['flow']],conn,'stable native generation');statuses=set()
 for fr in frames:
  for h in fr.get('HTTP3 Frames',[]):
   if h['Frame Type']!='HEADERS':continue
   key=(conn,int(h['Direction']),int(h['Stream ID']),int(h['Stream Offset']));check(key in expected,('known peer header section',key));check(key not in actual,'single native header delivery');x=expected[key];hs=[(v['Name'].encode('ascii'),v['Value'].encode('ascii')) for v in h.get('Headers')or[]]
   eq(hs,x['headers'],'native ordered fields equal literal QPACK input');eq(h['Frame SHA256'],x['sha256'],'exact application frame bytes');eq(h['Header Conformance Status'],x['status'],'declared base conformance result');check('not certified' in h['Header Conformance Scope'],'limited conformance scope explicit');actual[key]=True;counts[x['status']]+=1;statuses.add(x['status'])
   if x['status']!='base-shape-valid':
    check(e['status'] not in ['decoded','deferred'],'invalid message not advertised as successful');check(bool(h.get('Header Conformance Error')),'specific field diagnostic');check(any(r['Stream ID']==h['Stream ID'] for r in session.get('HTTP3 Stream Errors',[])),'diagnostic attributed to its actual stream')
   if key[1]==1:
    good_request=cases[(conn,key[2])]['expected_base_scope']=='base';eq(h.get('Associated Request')is True,good_request,'response association not inherited from failed request')
   wk=(conn,key[1],key[2],x['sha256']);options=ws_headers.get(wk)
   if not options:ws_missing.append({'connection':conn,'direction':key[1],'stream':key[2],'case':x['case']})
   elif hs not in options:ws_differences.append({'connection':conn,'direction':key[1],'stream':key[2],'case':x['case'],'reason':'ordered decompressed raw fields differ'})
 if len(statuses)>1:mixed.append({'PDU':e['id'],'frames':[r['Number'] for r in refs],'statuses':sorted(statuses)})
eq(set(actual),set(expected),'all original field sections delivered');eq(len(bindings),3,'three independent native connections');eq(len(set(bindings.values())),3,'one native generation per reference connection')
if a.profile=='malformed69':check(bool(mixed),'actual mixed valid/error header packet observed')
out={'scope':__doc__,'profile':a.profile,'checks':checks,'peer_operations':len(m['operations']),'native_connections':len(bindings),'header_sections':len(actual),'native_conformance':dict(counts),'reference_frame_views':dict(ws_frame_count),'reference_repeated_frame_views':[{'connection':k[0],'direction':k[1],'stream':k[2],'type':k[3],'sha256':k[4],'frames':v} for k,v in sorted(ws_frame_keys.items()) if len(v)>1],'reference_header_views_unavailable':ws_missing,'reference_raw_field_differences':ws_differences,'mixed_status_packets':mixed,'network':m['network'],'native_selected_differences':[],'manifest_sha256':sha(a.manifest.read_bytes()),'oracle_sha256':sha(a.oracle.read_bytes()),'events_sha256':sha(a.events.read_bytes())}
a.out.parent.mkdir(parents=True,exist_ok=True);a.out.write_text(json.dumps(out,indent=2)+'\n',encoding='utf8');print(json.dumps({k:v for k,v in out.items() if k not in ['scope','mixed_status_packets','reference_header_views_unavailable','reference_raw_field_differences']}))
