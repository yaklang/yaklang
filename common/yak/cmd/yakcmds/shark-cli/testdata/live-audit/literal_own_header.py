"""Frozen literal source walker; no tunnel/quote recursion."""
import ipaddress,struct,zlib

def ip(w):
 address=ipaddress.ip_address(w)
 return str(address.ipv4_mapped)if isinstance(address,ipaddress.IPv6Address)and address.ipv4_mapped is not None else str(address)

def parent(rec):
 w=rec['wire'];end=len(w);at=0;path=[];vlans=[];kind=None
 def stop(reason):return dict(covered=False,reason=reason,path=path,offset=at,vlans=vlans)
 if rec['link']in(1,274):
  if rec['link']==274:
   if w and w[0]==0x50:at=1
   while at<end and w[at]==0x55:
    at+=1
    if at>64:return stop('MPacket-preamble-profile-limit')
   if at>=end or w[at]!=0xd5:return stop('MPacket-unselected-delimiter')
   at+=1
   if end-at<18:return stop('MPacket-short-express-FCS')
   if zlib.crc32(w[at:end-4])!=int.from_bytes(w[end-4:end],'little'):return stop('MPacket-invalid-FCS')
   end-=4;path=['FPP']
  if at+14>end:return stop('short-Ethernet')
  kind=int.from_bytes(w[at+12:at+14],'big');at+=14;path+=['Ethernet']
  for _ in range(8):
   if kind not in(0x8100,0x88a8):break
   if at+4>end:return stop('short-VLAN')
   vlans.append(int.from_bytes(w[at:at+2],'big')&0xfff);kind=int.from_bytes(w[at+2:at+4],'big');at+=4;path.append('802.1Q')
  if kind in(0x8100,0x88a8):return stop('VLAN-depth-not-covered')
 elif rec['link']in(101,12):
  if not w:return stop('empty-raw-IP')
  kind={4:0x800,6:0x86dd}.get(w[0]>>4)
 elif rec['link']in(228,229):kind=0x800 if rec['link']==228 else 0x86dd
 elif rec['link']in(0,108):
  if end<4:return stop('short-Loopback')
  family=next((v for v in[struct.unpack('<I',w[:4])[0],struct.unpack('>I',w[:4])[0]]if v in(2,10,24,28,30)),None)
  if family is None:return stop('unselected-Loopback-family')
  at=4;kind=0x800 if family==2 else 0x86dd;path=['Loopback']
 elif rec['link']==113:
  if end<16:return stop('short-SLL')
  kind=int.from_bytes(w[14:16],'big');at=16;path=['LinuxSLL']
 else:return stop('link-not-covered-'+str(rec['link']))
 if kind is not None and kind<=1500:
  if at+kind>end:return stop('short-802.3-declared-LLC')
  end=at+kind
  if at+8>end or w[at:at+6]!=b'\xaa\xaa\x03\x00\x00\x00':return stop('unselected-LLC-SNAP-profile')
  kind=int.from_bytes(w[at+6:at+8],'big');at+=8;path+=['LLC','SNAP']
 if kind==0x806:return dict(covered=True,protocol='arp',offset=at,end=end,available=end-at,path=path+['ARP'],vlans=vlans,parent_extent_complete=True,family_consistent=True)
 if kind==0x800:
  if at+20>end or w[at]>>4!=4:return stop('invalid-IPv4-fixed')
  size=(w[at]&15)*4;total=int.from_bytes(w[at+2:at+4],'big');fragment=int.from_bytes(w[at+6:at+8],'big')
  if size<20 or at+size>end or total<size:return stop('invalid-IPv4-length')
  pos=at+20
  while pos<at+size:
   typ=w[pos]
   if typ==0:break
   if typ==1:pos+=1;continue
   if pos+2>at+size or w[pos+1]<=2 or pos+w[pos+1]>at+size:return stop('invalid-IPv4-options')
   pos+=w[pos+1]
  if fragment&0x3fff:return stop('IPv4-fragment-not-reassembled')
  source,destination=ip(w[at+12:at+16]),ip(w[at+16:at+20]);version=4;next_header=w[at+9];complete=at+total<=end;end=min(end,at+total);at+=size;path+=['IPv4']
 elif kind==0x86dd:
  if at+40>end or w[at]>>4!=6:return stop('invalid-IPv6-fixed')
  length=int.from_bytes(w[at+4:at+6],'big')
  if not length:return stop('IPv6-jumbo-not-covered')
  source,destination=ip(w[at+8:at+24]),ip(w[at+24:at+40]);version=6;next_header=w[at+6];complete=at+40+length<=end;end=min(end,at+40+length);at+=40;path+=['IPv6'];extensions=0
  for _ in range(16):
   if next_header not in(0,43,44,51,60):break
   typ=next_header
   if at+2>end:return stop('short-IPv6-extension')
   if typ==0 and extensions!=0:return stop('HBH-order-invalid')
   size=8 if typ==44 else(w[at+1]+2)*4 if typ==51 else(w[at+1]+1)*8
   if typ==51 and(size<12 or size%8):return stop('invalid-IPv6-AH-length')
   if at+size>end:return stop('short-IPv6-extension-envelope')
   if typ in(0,60):
    pos=at+2
    while pos<at+size:
     if w[pos]==0:pos+=1;continue
     if pos+2>at+size or pos+2+w[pos+1]>at+size:return stop('invalid-IPv6-TLV')
     pos+=2+w[pos+1]
   if typ==44 and int.from_bytes(w[at+2:at+4],'big')&0xfff9:return stop('IPv6-fragment-not-reassembled')
   path.append({0:'IPv6HopByHop',43:'IPv6Routing',44:'IPv6Fragment',51:'AH',60:'IPv6Destination'}[typ]);next_header=w[at];at+=size;extensions+=1
  else:return stop('IPv6-extension-depth-not-covered')
 else:return stop('non-IP-or-ARP-EtherType-'+str(kind))
 if next_header not in(1,58):return stop('non-simple-IP-child-'+str(next_header))
 return dict(covered=True,protocol='icmp'if next_header==1 else'icmpv6',offset=at,end=end,available=end-at,path=path+['ICMPv4'if next_header==1 else'ICMPv6'],vlans=vlans,parent_extent_complete=complete,source=source,destination=destination,family_consistent=(version==4 and next_header==1 or version==6 and next_header==58))

def own(w,location):
 b=w[location['offset']:location['end']];proto=location['protocol'];available=len(b);required=8 if proto in('arp','icmp')else 4;reason=None;facts={}
 if proto=='arp'and available>=8:
  hlen,plen=b[4],b[5];required=8+2*(hlen+plen);facts=dict(hardware_length=hlen,protocol_length=plen,operation=int.from_bytes(b[6:8],'big'))
  if available<required:reason='ARP-address-tuple-short'
 elif proto=='icmpv6'and available>=4:
  typ=b[0];minimum={1:8,2:8,3:8,4:8,128:8,129:8,133:8,134:16,135:24,136:24,137:40}.get(typ,4);required=minimum;facts=dict(type=typ,code=b[1])
  if available<required:reason='ICMPv6-type-fixed-body-short'
  if 133<=typ<=137 and available>=required:
   pos=required
   while pos<available:
    if pos+2>available or b[pos+1]==0 or pos+8*b[pos+1]>available:reason='NDP-option-bounds';break
    pos+=8*b[pos+1]
 elif proto=='icmp'and available>=8:facts=dict(type=b[0],code=b[1],identifier=int.from_bytes(b[4:6],'big'),sequence=int.from_bytes(b[6:8],'big'))
 if available<(8 if proto in('arp','icmp')else 4):reason=proto+'-base-header-short'
 return dict(available=available,required=required,reason=reason,wire_prefix_hex=b[:min(40,len(b))].hex(),facts=facts)
