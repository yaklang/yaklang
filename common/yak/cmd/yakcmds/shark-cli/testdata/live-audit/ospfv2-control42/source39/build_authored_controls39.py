"""Small authored OSPFv2 boundary/absent-grammar controls, not real captured traffic.

No router keys or real packet bytes are embedded. Public deterministic opaque
placeholders must never be reported as authentication success. No native output
is consumed. Complete source facts and explicit strict-source refusals only.
"""
import argparse,hashlib,importlib.util,ipaddress,json,shutil,struct
from pathlib import Path

def sha(p):return hashlib.sha256(Path(p).read_bytes()).hexdigest()
def h(w):return hashlib.sha256(w).hexdigest()
def u16(v):return struct.pack('!H',v)
def u32(v):return struct.pack('!I',v)
def ip(v):return ipaddress.IPv4Address(v).packed
def sum16(w):
 total=0
 for i in range(0,len(w),2):total+=(w[i]<<8)+(w[i+1]if i+1<len(w)else 0)
 while total>65535:total=(total&65535)+(total>>16)
 return(~total)&65535

def lsa(typ,body,identifier='192.0.2.1',options=2,age=1):
 wire=bytearray(u16(age)+bytes([options,typ])+ip(identifier)+ip('192.0.2.10')+u32(0x80000001)+b'\0\0'+u16(20+len(body))+body)
 c0=c1=0
 for b in wire[2:]:c0=(c0+b)%255;c1=(c1+c0)%255
 x=((len(wire)-17)*c0-c1)%255
 if x==0:x=255
 y=510-c0-x
 if y>255:y-=255
 wire[16:18]=bytes([x,y]);return bytes(wire)
def lls(tlvs,crypto=False):
 values=b''.join(u16(t)+u16(len(w))+w+b'\0'*((-len(w))%4)for t,w in tlvs);wire=bytearray(b'\0\0'+u16((4+len(values))//4)+values)
 if not crypto:wire[:2]=u16(sum16(wire))
 return bytes(wire)
def hello(options=2):return ip('255.255.255.0')+u16(10)+bytes([options,1])+u32(40)+ip('192.0.2.10')+ip('192.0.2.11')+ip('192.0.2.12')
def packet(typ,body,auth=0,authfield=b'\0'*8,digest=b'',tail=b'',instance=0):
 header=bytearray(bytes([2,typ])+u16(24+len(body))+ip('192.0.2.10')+ip('0.0.0.0')+b'\0\0'+bytes([instance,auth])+authfield+body)
 if auth in(0,1):header[12:14]=u16(sum16(header[:16]+header[24:]))
 return bytes(header)+digest+tail

def frame(w,number):
 eth=bytes.fromhex('01005e0000050200000000010800');head=bytearray(b'\x45\0'+u16(20+len(w))+u16(number)+b'\0\0\x01\x59\0\0'+ip('192.0.2.10')+ip('224.0.0.5'));head[10:12]=u16(sum16(head));return eth+bytes(head)+w

def main():
 ap=argparse.ArgumentParser(description=__doc__);ap.add_argument('--codec',type=Path,required=True);ap.add_argument('--out',type=Path,required=True);a=ap.parse_args();assert not a.out.exists();assert shutil.disk_usage('C:/').free>20*2**30;a.out.mkdir()
 binding={str(Path(__file__)):sha(__file__),str(a.codec):sha(a.codec)};spec=importlib.util.spec_from_file_location('independent_OSPFv2_source',a.codec);codec=importlib.util.module_from_spec(spec);spec.loader.exec_module(codec)
 cases=[]
 def add(name,wire,reason,refusal=False):cases.append(dict(name=name,wire=wire,purpose=reason,strict_source_refusal_expected=refusal))
 eof=(1,u32(1));llsgood=lls([eof]);badlls=bytearray(llsgood);badlls[0]^=1
 add('hello-auth0-no-LLS',packet(1,hello()),'ordinary null authentication, base checksum valid')
 add('hello-auth0-LLS-good',packet(1,hello(0x12),tail=llsgood),'OSPFn excludes complete LLS; both independent checksums valid')
 add('hello-auth0-LLS-bad-base-unchanged',packet(1,hello(0x12),tail=bytes(badlls)),'exact base OSPF bytes identical to previous; bad LLS must not make base checksum bad')
 add('hello-auth1-public-placeholder',packet(1,hello(),1,b'\0'*8),'simple auth observed, auth eight bytes excluded from checksum, no auth verification')
 add('hello-auth1-mutated-auth-only',packet(1,hello(),1,b'\xa5'*8),'same base non-auth bytes/checksum, only publicly generated auth field differs')
 seq=0x10203040;opaque16=bytes((i*7+3)&255 for i in range(16));opaque32=bytes((i*11+5)&255 for i in range(32));af=b'\0\0'+bytes([7,16])+u32(seq)
 add('hello-auth2-digest-then-LLS',packet(1,hello(0x12),2,af,opaque16,lls([eof,(2,u32(seq)+opaque16)],True)),'OSPFn then digest16 then LLS; CA seq matches; both mathematical checksums omitted; no MAC/algorithm claim')
 add('hello-auth2-LLS-CA-sequence-mismatch',packet(1,hello(0x12),2,af,opaque16,lls([eof,(2,u32(seq+1)+opaque16)],True)),'valid boundaries but CA sequence must not be deemed authentic')
 add('hello-auth2-digest32-algorithm-unselected',packet(1,hello(),2,b'\0\0'+bytes([9,32])+u32(seq),opaque32),'length32 observed, algorithm requires configured SA, not infer verified HMAC')
 add('hello-auth2-short-digest',packet(1,hello(),2,b'\0\0'+bytes([9,32])+u32(seq),opaque16),'declared32 digest has16 captured; complete source decode must refuse',True)
 routerlinks=[]
 for typ,linkid,linkdata,cost,tos in[(1,'192.0.2.2','0.0.0.7',8,[(4,12)]),(2,'192.0.2.254','192.0.2.10',2,[]),(3,'198.51.100.0','255.255.255.0',3,[]),(4,'192.0.2.99','192.0.2.10',5,[])]:
  routerlinks.append(ip(linkid)+ip(linkdata)+bytes([typ,len(tos)])+u16(cost)+b''.join(bytes([t,0])+u16(m)for t,m in tos))
 rawlsas=[lsa(1,bytes([3,0])+u16(len(routerlinks))+b''.join(routerlinks)),lsa(2,ip('255.255.255.0')+ip('192.0.2.10')+ip('192.0.2.11'),identifier='192.0.2.254'),lsa(3,ip('255.255.255.0')+b'\0'+(17).to_bytes(3,'big')+bytes([4])+(23).to_bytes(3,'big'),identifier='198.51.100.9'),lsa(4,b'\0'*4+b'\0'+(19).to_bytes(3,'big')+bytes([4])+(29).to_bytes(3,'big'),identifier='203.0.113.2'),lsa(5,ip('255.255.255.0')+bytes([0])+(10).to_bytes(3,'big')+ip('0.0.0.0')+u32(12345)+bytes([0x84])+(20).to_bytes(3,'big')+ip('192.0.2.9')+u32(54321),identifier='198.51.100.4'),lsa(7,ip('255.255.255.0')+bytes([0x80])+(30).to_bytes(3,'big')+ip('192.0.2.9')+u32(67890),identifier='203.0.113.9',options=0x0a)]
 for typ,raw in zip((1,2,3,4,5,7),rawlsas):add('LSUpdate-type%d-full'%typ,packet(4,u32(1)+raw),'complete LSA header/body/Fletcher; type4 LSID is ASBR RouterID, no mask-derived prefix'if typ==4 else'complete advertised LSA source/body/ranges/Fletcher; no installed-route claim')
 summaries=b''.join(raw[:20]for raw in rawlsas)
 add('DD-six-advertised-headers',packet(2,u16(1500)+bytes([2,7])+u32(100)+summaries),'all six header types without body; Fletcher verification unavailable')
 requests=b''.join(u32(raw[3])+raw[4:12]for raw in rawlsas)
 add('LSRequest-six-types',packet(3,requests),'type32, LinkStateID, AdvertisingRouter each exact12-byte request')
 add('LSAck-six-advertised-headers',packet(5,summaries),'header-only observation, no body/LSDB acknowledgement-state inference')
 add('hello-instance32',packet(1,hello(),instance=32),'RFC6549 InstanceID8 separate from AuthType8')
 badbase=bytearray(packet(1,hello(0x12),tail=llsgood));badbase[12]^=1
 add('hello-base-bad-LLS-good',bytes(badbase),'bad base checksum despite independent good LLS')
 badlsa=bytearray(rawlsas[0]);badlsa[-1]^=1
 add('LSUpdate-bad-Fletcher-base-good',packet(4,u32(1)+bytes(badlsa)),'base checksum repaired; independent complete LSA Fletcher fails')
 add('hello-LLS-unknown-padded-TLV',packet(1,hello(0x12),tail=lls([eof,(300,b'\x7f')])),'unknown TLV opaque, value length1 plus3 padding included in block words but not TLV Length')
 add('hello-LLS-declared-length-short',packet(1,hello(0x12),tail=llsgood[:-1]),'declared LLS words require one byte beyond actual capture; complete source must refuse',True)
 zero=bytearray(packet(1,hello()));zero[12:14]=b'\0\0';zero[28:30]=b'\0\0';adjust=sum16(zero[:16]+zero[24:]);zero[28:30]=u16(adjust);assert sum16(zero[:16]+zero[24:])==0
 add('hello-checksum-field-zero-valid',bytes(zero),'mathematically valid zero checksum is not equivalent to authentication/checksum omission')
 pcap=bytearray(struct.pack('<IHHIIII',0xa1b2c3d4,2,4,0,0,65535,1));rows=[];refusals=[];leaves=0
 for number,case in enumerate(cases,1):
  w=case.pop('wire');raw=frame(w,number);pcap+=struct.pack('<IIII',1700000000+number,0,len(raw),len(raw))+raw;fp=a.out/('authored-frame-%02d.bin'%number);fp.write_bytes(raw);row=dict(case,frame=number,original_frame=str(fp),original_frame_SHA256=sha(fp),PDU_SHA256=h(w),PDU_range=[34,len(raw)],wire_bytes=len(w),source_origin='authored public deterministic control;not live or public capture')
  try:
   row['source']=codec.decode(w,34);assert not case['strict_source_refusal_expected'];leaves+=len(row['source']['source_leaves'])
  except Exception as ex:
   if not case['strict_source_refusal_expected']:raise
   row['strict_source_refusal']={'exception':type(ex).__name__,'reason':'declared authentication or LLS extent exceeds capture; no complete decoded source tree supplied'};refusals.append(number)
  rows.append(row)
 cap=a.out/'authored-controls.pcap';cap.write_bytes(pcap);write(a.out/'source-facts.json',rows)
 assert binding=={p:sha(p)for p in binding}
 report={'scope':__doc__,'authored_frames':len(rows),'fully_constructed_source_rows':len(rows)-len(refusals),'source_scalar_or_opaque_leaves':leaves,'strict_source_refusal_frames':refusals,'capture':str(cap),'capture_SHA256':sha(cap),'source_facts':str(a.out/'source-facts.json'),'source_facts_SHA256':sha(a.out/'source-facts.json'),'inputs':binding,'inputs_unchanged':True,'ordinary_no_LSDB_neighbor_SA_replay_or_route_verified':True,'no_native_output_read':True,'real_capture_status':False}
 write(a.out/'summary.json',report);print(json.dumps({'summary':str(a.out/'summary.json'),'SHA256':sha(a.out/'summary.json'),'frames':len(rows),'complete_rows':len(rows)-len(refusals),'leaves':leaves,'refusal_frames':refusals,'pcap_bytes':len(pcap)}))
def write(p,x):
 with Path(p).open('x',encoding='utf8')as f:json.dump(x,f,indent=2);f.write('\n')
if __name__=='__main__':main()
