"""Source-only identity slots for old protected QUIC recovery captures.

No candidate/native/WS protocol values are read. Original ODCID protection gives
ClientHello/ServerHello; bound peer secrets authenticate Handshake ALPN and short
packets. CID routes and STREAM coverage are updated only after successful AEAD.
Slots prove h3 + client-initiated bidirectional stream with captured prefix0
missing; they do not invent HEADERS, request/transaction or delivered plaintext.
"""
import argparse,collections,hashlib,hmac,importlib.util,ipaddress,json,shutil,sys
from pathlib import Path

def sha(p):return hashlib.sha256(Path(p).read_bytes()).hexdigest()
def digest(w):return hashlib.sha256(w).hexdigest()
def read(p):return json.loads(Path(p).read_text('utf8'))
def write(p,x):
 with Path(p).open('x',encoding='utf8')as f:json.dump(x,f,indent=2);f.write('\n')
def load(name,p):
 s=importlib.util.spec_from_file_location(name,p);m=importlib.util.module_from_spec(s);s.loader.exec_module(m);return m
def alpn(w):
 assert len(w)>=2 and int.from_bytes(w[:2],'big')==len(w)-2;at=2;out=[]
 while at<len(w):
  n=w[at];at+=1;assert n and at+n<=len(w);out.append(w[at:at+n].decode('ascii'));at+=n
 return out

def main():
 ap=argparse.ArgumentParser(description=__doc__)
 for name in('phase','audit','helper','recovery','out'):ap.add_argument('--'+name,type=Path,required=True)
 a=ap.parse_args();assert not a.out.exists();assert shutil.disk_usage('C:/').free>20*2**30;a.out.mkdir();sys.path.insert(0,str(a.audit));q=load('independent_source_QUIC',a.helper);crypto=load('source_KDF',a.recovery);wire=load('source_capture',a.audit/'capture_clock_wire_audit.py');tls=load('source_TLS_hello',a.audit/'tls_resumption_audit.py')
 p1=a.phase/'QUIC-sparse-epoch-plan-before-01/plan-before.json';p2=a.phase/'QUIC-sparse-recovery-controls-before-01/plan-before.json';first=read(p1);second=read(p2);cases=first['cases']+second['cases'];manifest=Path(first['source_capture']).parent/'manifest.json';peer=read(manifest);server=peer['server_port']
 dep=[Path(__file__),a.helper,a.recovery,p1,p2,manifest,a.audit/'capture_clock_wire_audit.py',a.audit/'tls_resumption_audit.py',a.audit/'owned_tls_clock_audit.py',a.audit/'owned_quic_clock_audit.py']
 for c in cases:dep.extend([Path(c['capture']),Path(c['keylog'])])
 bound={str(p):sha(p)for p in dep}
 for c in cases:assert sha(c['capture'])==c['capture_SHA256']and sha(c['keylog'])==c['keylog_SHA256']
 write(a.out/'plan-before.json',{'scope':__doc__,'source_only_before_candidate_details_read':True,'inputs':bound,'required_source_gates':['successful packet AEAD','literal CID + direction bound to source CH random and server EE ALPN','ALPN h3 captured and authenticated before current packet','STREAM SID modulo4 ==0','no captured contiguous prefix byte0 for current directional stream','current STREAM does not fill prefix0','never infer HEADERS/request-PDU/transaction/old plaintext'],'native_product_output_paths_read':[],'whole_future_event_expectations_defined':False})
 results=[]
 for case in cases:
  contexts=[];routes=collections.defaultdict(set);cryptocells=collections.defaultdict(dict);cursor=collections.defaultdict(int);streamcells=collections.defaultdict(dict);secrets=crypto.keys(Path(case['keylog']));packets=[];slots=[];handshakes=[];tails=[];counts=collections.Counter();records=wire.physical(Path(case['capture']))
  def route(c,cid,d):
   assert 1<=len(cid)<=20;routes[cid,d].add(c['connection']);c['CIDs'].add((cid.hex(),d))
  def handshake(c,d,space):
   key=(c['connection'],d,space);data=q.prefix(cryptocells[key]);at=cursor[key]
   while at+4<=len(data):
    n=int.from_bytes(data[at+1:at+4],'big');end=at+4+n
    if end>len(data):break
    typ=data[at];body=data[at+4:end];origin=sorted(set().union(*(cryptocells[key][i][1]for i in range(at,end))));fact={'connection':c['connection'],'direction':d,'space':space,'type':typ,'crypto_range':[at,end],'message_SHA256':digest(data[at:end]),'source_packets':[{'frame':x[0],'packet_SHA256':x[1],'packet_range':[x[2],x[3]]}for x in origin]}
    if typ==1:
     assert d==0 and space=='initial';h=tls.hello(body,True);assert('CLIENT_TRAFFIC_SECRET_0',h['random'])in secrets;c['random']=h['random'];c['offered_ALPN']=alpn(h['extensions'][16]);fact['offered_ALPN']=c['offered_ALPN'];c['ClientHello_source']=fact
    elif typ==2:
     assert d==1 and space=='initial';h=tls.hello(body,False);assert h['cipher']in q.SUITES;c['cipher']=h['cipher'];fact['selected_cipher']=q.SUITES[h['cipher']];c['ServerHello_source']=fact
    elif typ==8:
     assert d==1 and space=='handshake';assert len(body)>=2 and int.from_bytes(body[:2],'big')==len(body)-2;pos=2;extensions={}
     while pos<len(body):
      assert pos+4<=len(body);t=int.from_bytes(body[pos:pos+2],'big');n=int.from_bytes(body[pos+2:pos+4],'big');pos+=4;assert t not in extensions and pos+n<=len(body);extensions[t]=body[pos:pos+n];pos+=n
     selected=alpn(extensions[16]);assert len(selected)==1 and selected[0]in c['offered_ALPN'];c['ALPN']=selected[0];fact['selected_ALPN']=selected[0];fact['authentication']='server Handshake packet AEAD bound to literal Initial ClientHello random';c['EncryptedExtensions_source']=fact
    handshakes.append(fact);at=end
   cursor[key]=at
  for frame,r in enumerate(records,1):
   _,iph,udp=wire.ip(r);assert iph[9]==17 and len(udp)>=8;sp,dp=int.from_bytes(udp[:2],'big'),int.from_bytes(udp[2:4],'big');assert server in(sp,dp)and (sp==server)!=(dp==server);assert int.from_bytes(udp[4:6],'big')==len(udp);direction=int(sp==server);data=udp[8:];base=len(r['wire'])-len(data);at=0
   while at<len(data):
    try:h=q.packet_header(data[at:])
    except(AssertionError,IndexError):tails.append({'frame':frame,'physical_range':[base+at,len(r['wire'])],'bytes':len(data)-at,'SHA256':digest(data[at:]),'role':'unprotected datagram remainder; no AEAD/identity claim'});break
    raw=data[at:at+h['end']];space=h.get('space','application');candidates=[]
    if h['long']:
     candidates=[(contexts[i-1],h['dcid'])for i in routes.get((h['dcid'],direction),())]
     if not candidates and space=='initial'and direction==0:
      c={'connection':len(contexts)+1,'ODCID':h['dcid'],'CIDs':set(),'largest':collections.defaultdict(lambda:-1),'cipher':None,'random':None,'ALPN':None,'first_frame':frame};contexts.append(c);route(c,h['dcid'],0);route(c,h['scid'],1);candidates=[(c,h['dcid'])]
    else:
     for(cid,d),ids in routes.items():
      if d==direction and raw[1:1+len(cid)]==cid:candidates.extend((contexts[i-1],cid)for i in ids)
    good=[]
    for c,cid in candidates:
     if space=='initial':initial=hmac.new(q.SALT,c['ODCID'],hashlib.sha256).digest();secret=crypto.hkdf(initial,b'server in'if direction else b'client in',32,hashlib.sha256);cipher=0x1301;epochs=[0]
     elif c['random']is None or c['cipher']is None:continue
     else:secret=secrets[('SERVER'if direction else'CLIENT')+('_HANDSHAKE_TRAFFIC_SECRET'if space=='handshake'else'_TRAFFIC_SECRET_0'),c['random']];cipher=c['cipher'];epochs=[0]if space=='handshake'else range(5)
     pnoff=h['pn_offset']if h['long']else 1+len(cid)
     for epoch in epochs:
      opened=q.open_packet(raw,pnoff,h['long'],secret,cipher,epoch,c['largest'][direction,space],crypto.hkdf)
      if opened is not None:good.append((c,cid,opened));break
    assert len(good)<=1,'ambiguous source AEAD connection';packet={'frame':frame,'domain':{'Section':r['section'],'Interface':r['interface'],'Encapsulation':''},'time_ns':r['time_ns'],'packet_range':[base+at,base+at+len(raw)],'packet_SHA256':digest(raw),'packet_bytes':len(raw),'source_IP':str(ipaddress.IPv4Address(iph[12:16])),'destination_IP':str(ipaddress.IPv4Address(iph[16:20])),'source_port':sp,'destination_port':dp,'direction':direction,'space':space,'authenticated':False};counts['protected_packets']+=1
    if good:
     c,cid,opened=good[0];c['largest'][direction,space]=max(c['largest'][direction,space],opened['PN']);fs=q.frames(opened['plaintext']);source=(frame,digest(raw),base+at,base+at+len(raw));packet.update(connection=c['connection'],destination_CID=cid.hex(),authenticated=True,PN=opened['PN'],PN_bytes=opened['PN_bytes'],epoch=opened['epoch'],key_phase=opened['key_phase'],plaintext_SHA256=digest(opened['plaintext']),frames=[q.frame_public(f)for f in fs],ALPN_confirmed_before_packet=c['ALPN']);counts['authenticated_packets']+=1
     if h['long']:route(c,h['scid'],1-direction)
     for fi,f in enumerate(fs):
      if f['name']=='CRYPTO':q.cells_add(cryptocells[c['connection'],direction,space],f['offset'],f['data'],source)
      elif f['name']=='NEW_CONNECTION_ID':assert f['retire_prior_to']<=f['sequence'];route(c,f['CID'],1-direction)
      elif f['name']=='STREAM':
       key=(c['connection'],direction,f['stream']);cells=streamcells[key];prefix_before=len(q.prefix(cells));q.cells_add(cells,f['offset'],f['data'],source);prefix_after=len(q.prefix(cells));eligible=c['ALPN']=='h3'and f['stream']%4==0 and prefix_before==prefix_after==0 and f['offset']>0
       if eligible:
        slots.append({'frame':frame,'domain':packet['domain'],'packet_range':packet['packet_range'],'packet_SHA256':packet['packet_SHA256'],'source_connection':c['connection'],'destination_CID':cid.hex(),'direction':direction,'source_IP':packet['source_IP'],'destination_IP':packet['destination_IP'],'source_port':sp,'destination_port':dp,'PN':opened['PN'],'PN_bytes':opened['PN_bytes'],'epoch':opened['epoch'],'key_phase':opened['key_phase'],'plaintext_SHA256':packet['plaintext_SHA256'],'STREAM_frame_index':fi,'STREAM_plaintext_range':f['plaintext_range'],'Stream_ID':f['stream'],'Stream_Offset':f['offset'],'Stream_Length':f['length'],'FIN':f['FIN'],'Stream_Data_SHA256':digest(f['data']),'captured_contiguous_prefix_before':prefix_before,'captured_contiguous_prefix_after':prefix_after,'source_ALPN':'h3','ALPN_source_EncryptedExtensions_SHA256':c['EncryptedExtensions_source']['message_SHA256'],'identity_lower_bound':'HTTP/3 wire observation; client-initiated bidirectional stream; prefix missing','known_HEADERS_or_request_anchor_or_transaction_inferred':False,'source_byte_delivery_to_HTTP3_parser':0})
     handshake(c,direction,space)
    else:packet['candidate_source_connections']=sorted(set(c['connection']for c,cid in candidates));packet['failure']='source packet AEAD failed; no PN/plaintext/STREAM inferred';counts['failed_authentication_packets']+=1
    packets.append(packet);at+=len(raw)
  assert len(contexts)==3 and {c['ALPN']for c in contexts}=={'h3'}
  dest=a.out/case['case'];dest.mkdir()
  for name,data in [('packet-source.jsonl',packets),('prefix-identity-slots.jsonl',slots),('handshake-source.jsonl',handshakes)]:
   with(dest/name).open('x',encoding='utf8')as out:
    for row in data:out.write(json.dumps(row,separators=(',',':'))+'\n')
  write(dest/'connections.json',[{'connection':c['connection'],'ODCID':c['ODCID'].hex(),'client_random_SHA256':digest(c['random']),'suite':q.SUITES[c['cipher']],'first_frame':c['first_frame'],'ALPN':c['ALPN'],'ClientHello_source':c['ClientHello_source'],'ServerHello_source':c['ServerHello_source'],'EncryptedExtensions_source':c['EncryptedExtensions_source'],'authenticated_CID_routes':[{'CID':cid,'direction':d}for cid,d in sorted(c['CIDs'])]}for c in contexts]);write(dest/'unprotected-tail-source.json',tails)
  result={'case':case['case'],'capture_SHA256':case['capture_SHA256'],'keylog_SHA256':case['keylog_SHA256'],'physical_frames':len(records),'counts':dict(counts),'unprotected_tails':len(tails),'prefix_identity_STREAM_slots':len(slots),'prefix_identity_packet_slots':len(set((s['frame'],s['packet_SHA256'])for s in slots)),'slots_by_connection_direction_stream':[{'connection':k[0],'direction':k[1],'stream':k[2],'count':v}for k,v in collections.Counter((s['source_connection'],s['direction'],s['Stream_ID'])for s in slots).items()],'artifacts':{name:sha(dest/name)for name in('packet-source.jsonl','prefix-identity-slots.jsonl','handshake-source.jsonl','connections.json','unprotected-tail-source.json')}};write(dest/'summary.json',result);results.append(result);print(json.dumps({'case':case['case'],'counts':dict(counts),'prefix_identity_STREAM_slots':len(slots),'prefix_identity_packet_slots':result['prefix_identity_packet_slots']}),flush=True)
 assert bound=={p:sha(p)for p in bound};write(a.out/'summary.json',{'scope':__doc__,'plan_before_SHA256':sha(a.out/'plan-before.json'),'results':results,'inputs':bound,'inputs_unchanged':True,'no_candidate_or_native_or_WS_output_read':True,'complete_future_event_expectations_defined':False});print(json.dumps({'summary':str(a.out/'summary.json'),'SHA256':sha(a.out/'summary.json')}),flush=True)
if __name__=='__main__':main()
