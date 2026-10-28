"""Literal source positions for remaining ARP/ICMPv6 carrier observations.
Compatibility profiles are explicitly observations, not valid tunnel admission.
Never search Native.Raw, a decoded value, or a quoted child for the source offset.
"""
import base64,hashlib,ipaddress,struct,zlib

def locate(record):
 w=record['wire'];end=len(w);at=0;headers=[];networks=[];scopes=[]
 kind='ethernet'
 if record['link']==274:
  if w and w[0]==0x50:at=1
  while at<end and w[at]==0x55:
   at+=1
   assert at<=64,'MPacket preamble depth'
  assert at<end and w[at]==0xd5,'MPacket delimiter'
  at+=1
  assert end-at>=18,'MPacket express frame with FCS'
  assert zlib.crc32(w[at:end-4])==int.from_bytes(w[end-4:end],'little'),'MPacket FCS'
  end-=4;headers.append(dict(kind='FPP',offset=0,length=at))
 elif record['link']==113:
  assert end>=16,'short SLL'
  kind=int.from_bytes(w[14:16],'big');at=16;headers.append(dict(kind='LinuxSLL',offset=0,length=16))
 elif record['link']==276:
  assert end>=20,'short SLL2'
  kind=int.from_bytes(w[:2],'big');at=20;headers.append(dict(kind='LinuxSLL2',offset=0,length=20))
 elif record['link']in(0,108):
  assert end>=4,'short Loopback'
  families=[int.from_bytes(w[:4],order)for order in('little','big')]
  family=next((v for v in families if v in(2,10,24,28,30)),None)
  assert family is not None,'unselected Loopback family'
  kind=0x800 if family==2 else 0x86dd;at=4;headers.append(dict(kind='Loopback',offset=0,length=4,family=family))
 elif record['link']in(101,12):
  assert w and w[0]>>4 in(4,6),'unselected RAW family'
  kind=0x800 if w[0]>>4==4 else 0x86dd
 elif record['link']in(228,229):kind=0x800 if record['link']==228 else 0x86dd
 else:assert record['link']==1,'unselected record link'
 for depth in range(20):
  if kind=='ethernet':
   assert at+14<=end,'short Ethernet'
   start=at;kind=int.from_bytes(w[at+12:at+14],'big');at+=14
   headers.append(dict(kind='Ethernet',offset=start,length=14,ethertype=kind))
   for _ in range(8):
    if kind not in(0x8100,0x88a8):break
    assert at+4<=end,'short VLAN'
    headers.append(dict(kind='VLAN',offset=at,length=4,identifier=int.from_bytes(w[at:at+2],'big')&4095));kind=int.from_bytes(w[at+2:at+4],'big');at+=4
   if kind<=1500:
    assert at+kind<=end and at+8<=end and w[at:at+6]==b'\xaa\xaa\x03\x00\x00\x00','unselected LLC'
    end=at+kind;kind=int.from_bytes(w[at+6:at+8],'big');headers.append(dict(kind='LLC-SNAP',offset=at,length=8));at+=8
  if kind==0x806:
   proto='arp';break
  if kind in(0x800,0x86dd):
   start=at
   if kind==0x800:
    assert at+20<=end and w[at]>>4==4,'short or wrong IPv4'
    hlen=(w[at]&15)*4;total=int.from_bytes(w[at+2:at+4],'big');assert hlen>=20 and at+hlen<=end,'short IPv4 options'
    assert int.from_bytes(w[at+6:at+8],'big')&0x3fff==0,'fragment source not covered'
    p=at+20
    while p<at+hlen:
     if w[p]==0:break
     if w[p]==1:p+=1;continue
     assert p+2<=at+hlen and w[p+1]>=2 and p+w[p+1]<=at+hlen,'invalid IPv4 option'
     p+=w[p+1]
    if total==0:scopes.append('observed IPv4 totalLength0 compatibility; no valid declared IPv4 extent')
    else:assert total>=hlen;end=min(end,at+total)
    nxt=w[at+9];src=str(ipaddress.ip_address(w[at+12:at+16]));dst=str(ipaddress.ip_address(w[at+16:at+20]));at+=hlen
    headers.append(dict(kind='IPv4',offset=start,length=hlen,declared_length=total,observed_end=end,source=src,destination=dst,next_protocol=nxt))
   else:
    assert at+40<=end and w[at]>>4==6,'short or wrong IPv6'
    size=int.from_bytes(w[at+4:at+6],'big');assert size>0,'IPv6 jumbo not covered';end=min(end,at+40+size)
    nxt=w[at+6];src=str(ipaddress.ip_address(w[at+8:at+24]));dst=str(ipaddress.ip_address(w[at+24:at+40]));at+=40
    headers.append(dict(kind='IPv6',offset=start,length=40,declared_payload=size,observed_end=end,source=src,destination=dst,next_protocol=nxt))
    for index in range(16):
     if nxt not in(0,43,44,51,60):break
     assert at+2<=end,'short extension prefix'
     typ=nxt;size=8 if typ==44 else(w[at+1]+2)*4 if typ==51 else(w[at+1]+1)*8
     assert at+size<=end,'short extension envelope'
     if typ==0:assert index==0,'HBH out of order'
     if typ==44:assert int.from_bytes(w[at+2:at+4],'big')&0xfff9==0,'non-atomic IPv6 fragment'
     headers.append(dict(kind='IPv6 extension',type=typ,offset=at,length=size,next_protocol=w[at]));nxt=w[at];at+=size
    else:raise AssertionError('extension depth')
   networks.append(dict(start=start,end=end,version=4 if kind==0x800 else 6))
   if nxt in(1,58):
    proto='icmp'if nxt==1 else'icmpv6'
    if(kind==0x800 and nxt==58)or(kind==0x86dd and nxt==1):scopes.append('cross-family observed child; no canonical IP-family admission')
    break
   if nxt in(4,41):kind=0x800 if nxt==4 else 0x86dd;continue
   if nxt==47:kind='gre';continue
   if nxt==97:kind='etherip';continue
   raise AssertionError('unselected IP child%d'%nxt)
  if kind=='etherip':
   assert at+2<=end,'short EtherIP'
   value=int.from_bytes(w[at:at+2],'big')
   if value!=0x3000:scopes.append('invalid EtherIP version/reserved bits; old child observed, not valid tunnel admission')
   else:scopes.append('EtherIP v3 observed; current application parent profile remains unselected')
   headers.append(dict(kind='EtherIP',offset=at,length=2,version=value>>12,reserved=value&4095));at+=2;kind='ethernet';continue
  if kind==0x22eb:
   assert at+12<=end and w[at]>>4==2,'short or wrong ERSPANIII'
   last=int.from_bytes(w[at+10:at+12],'big');frame_type=(last>>10)&31;size=20 if last&1 else 12
   assert at+size<=end and frame_type in(0,2),'unselected ERSPANIII body'
   platform=w[at+12]>>2 if last&1 else None
   headers.append(dict(kind='ERSPANIII',offset=at,length=size,version=2,optional_platform=bool(last&1),platform=platform,frame_type=frame_type))
   at+=size
   if frame_type==0:kind='ethernet'
   else:
    assert at<end and w[at]>>4 in(4,6),'wrong ERSPAN raw IP'
    kind=0x800 if w[at]>>4==4 else 0x86dd
   continue
  if kind=='gre':
   assert at+4<=end,'short GRE'
   start=at;flags=int.from_bytes(w[at:at+2],'big');kind=int.from_bytes(w[at+2:at+4],'big');assert flags&7==0,'unselected GRE version';at+=4
   if flags&0xc000:at+=4
   key=None
   if flags&0x2000:assert at+4<=end;key=int.from_bytes(w[at:at+4],'big');at+=4
   if flags&0x1000:at+=4
   assert at<=end,'short GRE optional words'
   if flags&0x4000:
    scopes.append('legacy RFC1701 source-route layout observed; route execution not verified')
    for _ in range(128):
     assert at+4<=end,'short GRE source-route prefix'
     family=int.from_bytes(w[at:at+2],'big');length=w[at+3];at+=4;assert at+length<=end,'short GRE source-route bytes';at+=length
     if family==0 and length==0:break
    else:raise AssertionError('GRE source-route limit')
   if flags&0x0ff8:scopes.append('GRE ignored/legacy flag bits observed; no sender conformance claim')
   headers.append(dict(kind='GREv0',offset=start,length=at-start,flags=flags,protocol=kind,key=key))
   if kind==0x6558:kind='ethernet'
   continue
  raise AssertionError('unselected carrier kind%s'%kind)
 else:raise AssertionError('carrier depth')
 raw_start=networks[-1]['start']if len(networks)>1 else 0
 raw_end=networks[-1]['end']if len(networks)>1 else len(w)
 assert raw_start<=at<=end<=raw_end
 return dict(protocol=proto,offset=at,end=end,raw_start=raw_start,raw_end=raw_end,PDU_offset=at-raw_start,headers=headers,profiles=sorted(set(scopes))or['bounded declared carrier layout; checksum and peer trust unverified'])

def bind(event,record):
 loc=locate(record);assert loc['protocol']==event['protocol'];raw=record['wire'][loc['raw_start']:loc['raw_end']]
 assert raw==base64.b64decode(event['raw']),'source-derived contiguous Raw extraction differs'
 source=event['source_bytes'];assert source['Kind']=='captured'and source['SHA256']==hashlib.sha256(raw).hexdigest()
 assert source['PacketRefs']==[dict(Number=record['frame'],Domain=event['domain'])]
 assert event['domain']['Section']==record['section']and event['domain']['Interface']==record['interface']
 return loc
