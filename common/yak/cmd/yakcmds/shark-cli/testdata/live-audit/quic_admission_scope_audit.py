"""Compare complete SDK exports and bind QUIC tail scope to literal datagrams.
Only admission and a refused remainder's summary may change. Without --capture,
this proves export stability, not a fresh physical-byte/source-graph assertion.
"""
import argparse,base64,collections,hashlib,itertools,json,re,struct
from pathlib import Path
ap=argparse.ArgumentParser(description=__doc__)
for n in ['before','after','out']:ap.add_argument('--'+n,required=True,type=Path)
ap.add_argument('--capture',type=Path);a=ap.parse_args();checks=0
assert not any((p/'.git').exists()for p in[a.out.resolve(),*a.out.resolve().parents])
def check(ok,why):
 global checks
 checks+=1;assert ok,why
def eq(x,y,why):check(x==y,(why,x,y))
def sha(p):
 h=hashlib.sha256()
 with p.open('rb')as f:
  for b in iter(lambda:f.read(1<<20),b''):h.update(b)
 return h.hexdigest()
physical={}
if a.capture:
 w=a.capture.read_bytes();no=0
 if w[:4]==bytes.fromhex('0a0d0d0a'):
  at=0;section=-1;interfaces={};endian=None
  while at<len(w):
   check(at+12<=len(w),'complete PCAPNG block')
   if w[at:at+4]==bytes.fromhex('0a0d0d0a'):
    magic=w[at+8:at+12];check(magic in[bytes.fromhex('4d3c2b1a'),bytes.fromhex('1a2b3c4d')],'PCAPNG byte order');endian='<'if magic==bytes.fromhex('4d3c2b1a')else'>';section+=1;interfaces={}
   check(endian is not None,'section precedes packet');typ,n=struct.unpack_from(endian+'II',w,at);check(n>=12 and n%4==0 and at+n<=len(w),'bounded PCAPNG block');eq(struct.unpack_from(endian+'I',w,at+n-4)[0],n,'trailing PCAPNG block length');body=w[at+8:at+n-4];at+=n
   if typ==1:interfaces[len(interfaces)]=struct.unpack_from(endian+'H',body)[0]
   elif typ in[2,6]:
    interface=struct.unpack_from(endian+('H'if typ==2 else'I'),body)[0];caplen,original=struct.unpack_from(endian+'II',body,12);check(20+caplen<=len(body),'bounded packet bytes');no+=1;physical[no]=(section,interface,interfaces[interface],body[20:20+caplen],original)
   elif typ==3:
    # Simple blocks have no caplen: retained padding is never usable as UDP.
    original=struct.unpack_from(endian+'I',body)[0];no+=1;physical[no]=(section,0,interfaces[0],body[4:4+min(original,len(body)-4)],original)
 else:
  check(w[:4]in[bytes.fromhex('d4c3b2a1'),bytes.fromhex('4d3cb2a1')],'classic LE capture');link=struct.unpack_from('<I',w,20)[0];at=24
  while at<len(w):
   _,_,n,original=struct.unpack_from('<IIII',w,at);at+=16;check(at+n<=len(w),'bounded classic packet');no+=1;physical[no]=(0,0,link,w[at:at+n],original);at+=n

def datagram(no,domain):
 section,interface,link,raw,original=physical[no];eq(domain,{'Section':section,'Interface':interface,'Encapsulation':''},'literal selected capture domain');eq(len(raw),original,'complete selected physical record');at=0
 if link==1:
  check(len(raw)>=14,'Ethernet header');typ=int.from_bytes(raw[12:14],'big');at=14
  for _ in range(8):
   if typ not in[0x8100,0x88a8]:break
   check(at+4<=len(raw),'VLAN header');typ=int.from_bytes(raw[at+2:at+4],'big');at+=4
  check(typ in[0x0800,0x86dd],'selected IPv4/IPv6 Ethernet payload')
 elif link in[0,108]:at=4
 elif link not in[101,228,229]:check(False,('selected link not independently supported',link))
 ip=raw[at:];check(bool(ip),'selected IP header')
 if ip[0]>>4==4:
  ihl=(ip[0]&15)*4;check(20<=ihl<=len(ip),'IPv4 header extent');total=int.from_bytes(ip[2:4],'big');check(ihl<=total<=len(ip),'IPv4 extent');eq(int.from_bytes(ip[6:8],'big')&0x3fff,0,'unfragmented selected UDP');eq(ip[9],17,'literal UDP');udp=ip[ihl:total]
 else:
  eq(ip[0]>>4,6,'IPv6 version');check(len(ip)>=40,'IPv6 header');end=40+int.from_bytes(ip[4:6],'big');check(40<=end<=len(ip),'IPv6 extent');nh,at=ip[6],40
  for _ in range(8):
   if nh==17:break
   check(nh in[0,43,60,44,51]and at+8<=end,'supported bounded IPv6 extension');n=8 if nh==44 else(ip[at+1]+2)*4 if nh==51 else(ip[at+1]+1)*8
   if nh==44:eq(int.from_bytes(ip[at+2:at+4],'big')&0xfff9,0,'unfragmented IPv6')
   nh,at=ip[at],at+n;check(at<=end,'IPv6 extension extent')
  eq(nh,17,'literal IPv6 UDP');udp=ip[at:end]
 check(len(udp)>=8,'UDP header');eq(len(udp),int.from_bytes(udp[4:6],'big'),'literal UDP extent');return udp[8:]
changes=[];counts=collections.Counter();events=0
with a.before.open(encoding='utf-8-sig')as bf,a.after.open(encoding='utf-8-sig')as af:
 for before,after in itertools.zip_longest(bf,af):
  check(before is not None and after is not None,'same number of exported events');old,new=json.loads(before),json.loads(after);events+=1;eq(old['id'],new['id'],'stable PDU ordering');changed={k for k in set(old)|set(new)if old.get(k)!=new.get(k)};check(changed<=set(['admission','summary']),('unexpected exported field change',old['id'],sorted(changed)))
  if new['protocol']not in['quic','http3','doq']:eq(changed,set(),'unrelated protocol unchanged');continue
  s=new.get('session')or{};ad=new['admission'];raw=base64.b64decode(new['raw']);counts[ad]+=1
  if ad=='authenticated-wire-and-observed-CID':eq(s.get('Authentication Verified'),True,'authentication label requires actual native packet verification')
  if ad=='unparsed-datagram-remainder':
   eq(s,{},'aggregate/raw tail has no authenticated frame session');check(bool(new.get('error')),'retained typed refusal');m=re.fullmatch(r'Unparsed QUIC datagram remainder · offset=(\d+) · (\d+) bytes · authentication not verified',new['summary']);check(bool(m),'visible offset, extent and unauthenticated scope');off,n=map(int,m.groups());eq(n,len(raw),'retained range length');eq(hashlib.sha256(raw).hexdigest(),new['source_bytes']['SHA256'],'literal range digest')
   if a.capture:
    refs=new['source_bytes']['PacketRefs'];check(bool(refs),'physical input witnesses')
    for r in refs:eq(datagram(r['Number'],r['Domain'])[off:],raw,'retained exact UDP remainder at declared offset')
  elif'summary'in changed:check(False,'only unparsed remainders change their summary')
  if changed:changes.append({'PDU':new['id'],'frames':[r['Number']for r in new['source_bytes'].get('PacketRefs')or[]],'before_admission':old['admission'],'after_admission':ad,'changed_keys':sorted(changed),'expert':new['expert']})
out={'scope':__doc__,'events':events,'checks':checks,'admission_counts':dict(counts),'changed_events':changes,'unexpected_export_differences':[],'physical_datagram_binding':bool(a.capture),'before_sha256':sha(a.before),'after_sha256':sha(a.after)}
if a.capture:out['capture_sha256']=sha(a.capture)
a.out.parent.mkdir(parents=True,exist_ok=True);a.out.write_text(json.dumps(out,indent=2)+'\n',encoding='utf8');print(json.dumps({k:v for k,v in out.items()if k not in['scope','changed_events']}));print('Changed events',len(changes))
