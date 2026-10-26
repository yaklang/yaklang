"""Literal capture, peer receipt and reference-STREAM audit of HTTP3 body controls.
All declared directions are checked. Rejected-message unparsed frame tails remain
explicit differences; they are not substituted with native labels or zero counts.
"""
import argparse,base64,collections,hashlib,ipaddress,json,struct
from pathlib import Path
ap=argparse.ArgumentParser(description=__doc__)
for n in ['capture','manifest','oracle','events','out']:ap.add_argument('--'+n,required=True,type=Path)
a=ap.parse_args();checks=0
for p in [a.capture,a.out]:assert not any((q/'.git').exists() for q in [p.resolve(),*p.resolve().parents])
def check(ok,why):
 global checks
 checks+=1;assert ok,why
def eq(x,y,why):check(x==y,(why,x,y))
def sha(w):return hashlib.sha256(w).hexdigest()
def load(p):return json.loads(p.read_text(encoding='utf-8-sig'))
def vals(x,key):
 if isinstance(x,dict):return [z for k,v in x.items() for z in ((v if isinstance(v,list) else [v]) if k==key else vals(v,key))]
 if isinstance(x,list):return [z for v in x for z in vals(v,key)]
 return []

def nodes(x,key):
 if isinstance(x,dict):
  if key in x:yield x
  for v in x.values():yield from nodes(v,key)
 elif isinstance(x,list):
  for v in x:yield from nodes(v,key)
def vi(w,at):
 check(at<len(w),'varint prefix');n=1<<(w[at]>>6);check(at+n<=len(w),'complete varint');return int.from_bytes(w[at:at+n],'big')&((1<<(n*8-2))-1),at+n
def split(w):
 at=0;out=[]
 while at<len(w):
  start=at;typ,at=vi(w,at);n,at=vi(w,at);end=at+n;check(end<=len(w),'complete peer frame');out.append({'offset':start,'type':typ,'payload':w[at:end],'header':w[start:at],'sha256':sha(w[start:end])});at=end
 return out

capture=a.capture.read_bytes();check(capture[:4] in [bytes.fromhex('d4c3b2a1'),bytes.fromhex('4d3cb2a1')],'classic little-endian capture');eq(struct.unpack_from('<I',capture,20)[0],0,'NULL datalink');physical={};at=24
while at<len(capture):
 check(at+16<=len(capture),'record header');_,_,n,original=struct.unpack_from('<IIII',capture,at);at+=16;check(n==original and at+n<=len(capture),'complete physical packet');physical[len(physical)+1]=capture[at:at+n];at+=n
m=load(a.manifest);eq(m['capture_ready_observed'],True,'actual capture readiness');eq(len(m['operations']),84,'all declared operations');ports=(str(m['client_port']),str(m['server_port']));reference={};reference_finals={};wire_frames=collections.defaultdict(list);packet_conn={};packet_endpoints={};first={};ws_counts=collections.Counter()
for row in load(a.oracle):
 l=row['_source']['layers'];no=int(l['frame']['frame.number']);raw=physical[no];eq(bytes.fromhex(l['frame_raw'][0]),raw,'reference bytes equal literal capture')
 cs=set(vals(l.get('quic',{}),'quic.connection.number'))
 if not cs:continue
 eq(len(cs),1,'one reference connection per datagram');conn=next(iter(cs));packet_conn[no]=conn;first.setdefault(conn,no)
 eq(raw[:4],b'\x02\0\0\0','IPv4 loopback header');ip=raw[4:];ihl=(ip[0]&15)*4;eq(ip[9],17,'literal UDP');eq(int.from_bytes(ip[6:8],'big')&0x3fff,0,'unfragmented selected packet');udp=ip[ihl:int.from_bytes(ip[2:4],'big')];eq(int.from_bytes(udp[4:6],'big'),len(udp),'UDP extent')
 sp,dp=str(int.from_bytes(udp[:2],'big')),str(int.from_bytes(udp[2:4],'big'));direction=0 if sp==ports[0] else 1;eq((sp,dp),ports if direction==0 else ports[::-1],'same owned endpoints');packet_endpoints[no]=(str(ipaddress.ip_address(ip[12:16]))+':'+sp,str(ipaddress.ip_address(ip[16:20]))+':'+dp)
 for h in vals(l.get('http3',{}),'http3.frame_type'):ws_counts[str(int(h,0))]+=1
 qs=l.get('quic',[]);qs=qs if isinstance(qs,list) else [qs]
 for q in qs:
  frames=q.get('quic.frame',[]);frames=frames if isinstance(frames,list) else [frames]
  for fr in frames:
   if not 8<=int(fr.get('quic.frame_type','0'),0)<=15:continue
   sid=int(fr['quic.stream.stream_id']);off=int(fr.get('quic.stream.offset','0'));body=bytes.fromhex((fr.get('quic.stream_data_raw')or[''])[0]);key=(conn,direction,sid);cells=reference.setdefault(key,{})
   for i,b in enumerate(body,off):
    if i in cells:eq(cells[i],b,'identical reference overlap')
    cells[i]=b
   fin=bool(int(fr['quic.frame_type'],0)&1)
   if fin:reference_finals.setdefault(key,off+len(body));eq(reference_finals[key],off+len(body),'consistent reference final size')
   wire_frames[(no,key)].append((off,body,fin))
order=sorted(first,key=first.get);eq(len(order),3,'three socket-reuse generations observed')
expected={};peer_wire={};plans={}
for op in m['operations']:
 conn=order[op['epoch']];sid=op['stream_id'];plan=op['declared_scenario'];check(bool(plan),'declared body scenario');plans[(conn,sid)]=plan
 for direction,name in [(0,'request_hex'),(1,'reply_hex')]:
  key=(conn,direction,sid);wire=bytes.fromhex(op[name]);cells=reference[key];eq(len(cells),len(wire),'complete reference application stream');eq(reference_finals.get(key),len(wire),'reference FIN marks complete peer direction');eq(bytes(cells[i] for i in range(len(wire))),wire,'reference STREAMs equal actual peer receipt');expected[key]=split(wire);peer_wire[key]=wire

reference_midframe_views=[]
for row in load(a.oracle):
 l=row['_source']['layers'];no=int(l['frame']['frame.number'])
 for node in nodes(l.get('http3',{}),'http3.frame_type'):
  typ=int(node['http3.frame_type'],0)
  if typ in[0,1,4]:continue
  sid=int(node['http3.frame_streamid']);raw=bytes.fromhex(node['http3.frame_type_raw'][0]+node['http3.frame_length_raw'][0]+node['http3.frame_payload_raw'][0]);matches=[(key,w.find(raw))for key,w in peer_wire.items()if key[0]==packet_conn[no]and key[2]==sid and raw in w];eq(len(matches),1,'reference unknown frame has a unique literal peer-stream byte position');key,offset=matches[0];container=next(f for f in expected[key]if f['offset']<offset<f['offset']+len(f['header'])+len(f['payload']));eq(container['type'],1,'reference unknown frame begins inside original HEADERS');eq(raw[:4],b'vali','reference type/length actually spell part of authority value');reference_midframe_views.append({'physical_frame':no,'connection':key[0],'direction':key[1],'stream':sid,'raw_type':typ,'raw_length':int(node['http3.frame_length']),'raw_header_hex':raw[:4].hex(),'peer_stream_offset':offset,'actual_enclosing_type':container['type'],'actual_enclosing_offset':container['offset'],'actual_frame_end':container['offset']+len(container['header'])+len(container['payload']),'scope':'raw reference view retained; independently received stream locates it inside HEADERS, not at an application-frame boundary'})

framing=dict(expected)
for key,cells in reference.items():
 if key in framing:continue
 check(key[2]&2,'only declared bidirectional messages or control streams');eq(len(cells),max(cells,default=-1)+1,'complete control-stream bytes');w=bytes(cells[i]for i in range(len(cells)));typ,n=vi(w,0);eq(typ,0,'selected HTTP3 control stream');fs=split(w[n:])
 for f in fs:f['offset']+=n;eq(f['type'],4,'declared SETTINGS control')
 framing[key]=fs

events=[json.loads(s) for s in a.events.read_text(encoding='utf-8-sig').splitlines()];by_id={e['id']:e for e in events};eq(len(by_id),len(events),'unique native PDU IDs');native_input={};application=collections.defaultdict(dict);contexts=collections.defaultdict(list);errors=collections.defaultdict(list);flow_bind={};diagnostics=[];app_delivery={}
for e in events:
 session=e.get('session')or{};streams=[fr for fr in session.get('Frames',[]) if fr.get('Frame Type')=='STREAM']
 if not streams:
  if e['protocol']=='quic' and e['status'] not in ['decoded','deferred']:
   raw=base64.b64decode(e['raw']);refs=e['source_bytes'].get('PacketRefs')or[];check(bool(raw)and not any(raw),'selected unprotected datagram tail is literal zero bytes');check(all(physical[r['Number']].endswith(raw)for r in refs),'diagnostic retains actual datagram tail');diagnostics.append({'PDU':e['id'],'status':e['status'],'frames':[r['Number']for r in refs],'error':e['error'],'raw_tail_bytes':len(raw),'raw_tail_sha256':sha(raw),'admission_before_followup_fix':e['admission'],'scope':'unprotected datagram bytes after a valid packet, not authenticated QUIC PADDING frames'})
  continue
 eq(session.get('Authentication Verified'),True,'native application bytes authenticated');eq(session.get('Native Carrier'),True,'native carrier');eq(session.get('ALPN'),'h3','observed ALPN');eq(e['transport'],'udp','native transport');eq(e['protocol'],'http3'if session.get('HTTP3')else'quic','identity follows observed application progress, including message errors');refs=e['source_bytes']['PacketRefs'];conns={packet_conn[r['Number']] for r in refs};eq(len(conns),1,'native/reference connection');conn=next(iter(conns));flow_bind.setdefault(e['flow'],conn);eq(flow_bind[e['flow']],conn,'stable native generation');eq(e['domain'],{'Section':0,'Interface':0,'Encapsulation':''},'literal capture domain')
 for r in refs:eq(r['Domain'],e['domain'],'reference domain');eq((e['source'],e['destination']),packet_endpoints[r['Number']],'literal native endpoints')
 for fr in streams:
  key=(conn,e['direction'],int(fr['Stream ID']));off=int(fr['Offset']);body=base64.b64decode(fr.get('Stream Data')or'');cells=native_input.setdefault(key,{})
  eq(fr['Byte Source']['Offset'],off,'literal STREAM source offset');eq(fr['Byte Source']['Length'],len(body),'literal source length');eq(fr['Byte Source']['Parent PDU'],e['id'],'original wire PDU')
  check(any(o==off and w==body and fin==bool(fr['FIN'])for r in refs for o,w,fin in wire_frames[(r['Number'],key)]),'native STREAM bytes, offset and FIN actually occur in their physical reference packet')
  for i,b in enumerate(body,off):
   if i in cells:eq(cells[i],b,'identical native overlap')
   cells[i]=b
  if 'HTTP3 Stream Error' in fr:errors[key].append({'PDU':e['id'],'reason':fr['HTTP3 Stream Error']})
  if 'HTTP3 Message Conformance' in fr:
   c=fr['HTTP3 Message Conformance'];eq(c['Stream ID'],key[2],'conformance stream');eq(c['Direction'],key[1],'conformance direction');check('not certified' in c['Scope'],'conformance scope retained')
   header=int(c['Header PDU ID']);check(header==0 or header in by_id and header<=e['id'],'observed header dependency');check(not c['Drained FIN']or bool(fr.get('Stream Complete')),'FIN only after native stream gaps are drained; rejected messages can stop before final body checking')
   if header:
    he=by_id[header];eq(he['flow'],e['flow'],'header dependency connection');eq(he['domain'],e['domain'],'header dependency capture domain');eq(he['direction'],e['direction'],'header dependency direction')
    check(any(h['Frame Type']=='HEADERS' and int(h['Stream ID'])==key[2] and int(h['Direction'])==key[1] and h.get('Header Kind')in['request','response'] for f in (he.get('session')or{}).get('Frames',[])for h in f.get('HTTP3 Frames',[])),'header dependency actually delivered final headers for this stream')
   contexts[key].append((e['id'],c))
  for h in fr.get('HTTP3 Frames',[]):
   hk=(conn,int(h['Direction']),int(h['Stream ID']));offset=int(h['Stream Offset']);check(hk in framing,('known peer/control stream',hk));check(offset not in application[hk],'single application-frame delivery');wanted=next(x for x in framing[hk] if x['offset']==offset)
   eq(h['Frame Type Value'],wanted['type'],'literal application type');eq(h['Payload Length'],len(wanted['payload']),'literal application length');eq(base64.b64decode(h['Frame Header Bytes']),wanted['header'],'literal application varints');eq(h['Frame SHA256'],wanted['sha256'],'complete app-frame bytes')
   if h['Frame Type']=='DATA':eq(base64.b64decode(h['Payload']or''),wanted['payload'],'exact DATA bytes')
   if h['Frame Type']=='HEADERS':
    eq(base64.b64decode(h['QPACK Block']),wanted['payload'],'exact QPACK field section');plan=plans[(hk[0],hk[2])];sections=([plan['request_headers']]+([plan['request_trailers']]if'request_trailers'in plan else[]))if hk[1]==0 else plan.get('reply_interim_headers',[])+[plan['reply_headers']];index=[f['offset']for f in framing[hk]if f['type']==1].index(offset);eq([[x['Name'],x['Value']]for x in h['Headers']],sections[index],'literal decoded fields equal raw-peer input')
   application[hk][offset]=h;app_delivery[(hk,offset)]=e['id']

eq(len(flow_bind),3,'native connection generations');eq(len(set(flow_bind.values())),3,'one native generation per reference connection');verdicts=collections.Counter();tails=[];body_summaries=[]
for key,frames in expected.items():
 cells=native_input[key];ref=reference[key];eq(cells,ref,'all literal peer STREAM bytes retained natively, including rejected-message bytes');plan=plans[(key[0],key[2])];want=plan['request_expect' if key[1]==0 else 'reply_expect'];actual=application[key];offsets=sorted(actual);ordered=[f['offset'] for f in frames];eq(offsets,ordered[:len(offsets)],'decoded application frames form an exact prefix')
 if want=='base':
  eq(offsets,ordered,'every valid-direction app frame delivered');eq(errors[key],[],'valid direction has no inherited stream error');check(bool(contexts[key]),'body context observed');_,c=contexts[key][-1];eq(c['Drained FIN'],True,'final size fully reassembled');eq(c['Observed DATA Bytes'],sum(len(f['payload']) for f in frames if f['type']==0),'independent DATA sum');check(c['Length Check']not in ['malformed','pending-FIN','awaiting-final-headers'],'valid completed direction classified honestly')
  hs=plan['request_headers'if key[1]==0 else'reply_headers'];lengths=[part.strip(' \t') for name,value in hs if name=='content-length'for part in value.split(',')];canonical=(lengths[0].lstrip('0')or'0')if lengths else None
  eq(c.get('Declared Content-Length'),canonical,'declared decimal length exact, including absent field');normalized=bool(lengths)and(len(lengths)>1 or any(value!=canonical for name,value in hs if name=='content-length'));eq('Length Normalization'in c,normalized,'normalization policy visible only when used')
  method=next(v for n,v in plan['request_headers']if n==':method');status=int(next(v for n,v in plan['reply_headers']if n==':status'));method_observed=any(h['Frame Type']=='HEADERS'and h.get('Header Kind')=='request'and h.get('Header Conformance Status')=='base-shape-valid'for h in application[(key[0],0,key[2])].values())
  if key[1]==1 and(status in[204,304]or method=='HEAD'and method_observed):expected_check='exempt-no-content'
  elif method=='CONNECT'and method_observed and(key[1]==0 or 200<=status<300):expected_check='CONNECT-stream; tunnel bytes are not HTTP content length'
  else:expected_check='matched'if canonical is not None else'not-declared'
  eq(c['Length Check'],expected_check,'independent completed-message exception and length verdict')
  for pdu,cc in contexts[key]:
   available=sum(h['Payload Length']for off,h in actual.items()if h['Frame Type']=='DATA'and app_delivery[(key,off)]<=pdu);check(cc['Observed DATA Bytes']<=available,'no future DATA included in body accounting');check(cc['Length Check']!='matched'or cc['Drained FIN'],'a matching prefix is not a completed body')
  body_summaries.append({'connection':key[0],'direction':key[1],'stream':key[2],'case':plan['name'],'data_bytes':c['Observed DATA Bytes'],'declared_length':canonical,'length_check':c['Length Check']})
 else:
  check(bool(errors[key]),('required message error',key,plan['name']));check(all('MalformedMessage' in x['reason'] for x in errors[key]),'declared body errors keep their taxonomy')
  for f in frames[len(offsets):]:tails.append({'connection':key[0],'direction':key[1],'stream':key[2],'case':plan['name'],'offset':f['offset'],'type':f['type'],'payload_bytes':len(f['payload']),'reason':'unparsed suffix after explicit stream-local message rejection'})
 verdicts[want]+=1
out={'scope':__doc__,'checks':checks,'connections':len(flow_bind),'operations':len(m['operations']),'checked_directions':len(expected),'direction_verdicts':dict(verdicts),'native_selected_differences':[],'native_unparsed_rejected_message_tail_frames':tails,'raw_reference_HTTP3_views':dict(ws_counts),'reference_midframe_views':reference_midframe_views,'native_selected_app_frames':sum(len(v)for v in application.values()),'native_selected_app_types':dict(collections.Counter(str(h['Frame Type Value'])for fs in application.values()for h in fs.values())),'nonapplication_QUIC_diagnostics':diagnostics,'network':m['network'],'completed_body_summaries':body_summaries,'capture_sha256':sha(capture),'events_sha256':sha(a.events.read_bytes()),'reference_sha256':sha(a.oracle.read_bytes())}
a.out.parent.mkdir(parents=True,exist_ok=True);a.out.write_text(json.dumps(out,indent=2)+'\n',encoding='utf8');print(json.dumps({k:v for k,v in out.items()if k not in ['scope','completed_body_summaries','native_unparsed_rejected_message_tail_frames','nonapplication_QUIC_diagnostics']}));print('Retained unparsed tail frames',len(tails),'and nonapplication diagnostics',len(diagnostics))
