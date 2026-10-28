"""Independent23-field byte decoder. Carrier selection and source binding are caller-owned.
No Native.Raw search, native label, session map or candidate program output is used here.
All coordinates in location describe the original supplied wire; returned ranges are Source1-relative.
"""
import ipaddress

SPEC={
'arp.opcode':('arp','unsigned',16),'arp.src.hw_mac':('arp','string',0),'arp.src.proto_ipv4':('arp','address',32),'arp.dst.proto_ipv4':('arp','address',32),
'icmp.type':('icmp','unsigned',8),'icmp.code':('icmp','unsigned',8),'icmp.ident':('icmp','unsigned',16),'icmp.seq':('icmp','unsigned',16),'icmp.mtu':('icmp','unsigned',16),'icmp.redir_gw':('icmp','address',32),'icmp.pointer':('icmp','unsigned',8),'icmp.originate_timestamp':('icmp','unsigned',32),'icmp.receive_timestamp':('icmp','unsigned',32),'icmp.transmit_timestamp':('icmp','unsigned',32),
'icmpv6.type':('icmpv6','unsigned',8),'icmpv6.code':('icmpv6','unsigned',8),'icmpv6.echo.identifier':('icmpv6','unsigned',16),'icmpv6.echo.sequence_number':('icmpv6','unsigned',16),'icmpv6.mtu':('icmpv6','unsigned',32),'icmpv6.nd.ns.target_address':('icmpv6','address',128),'icmpv6.nd.na.target_address':('icmpv6','address',128),'icmpv6.nd.rd.target_address':('icmpv6','address',128),'icmpv6.rd.na.destination_address':('icmpv6','address',128)}

def project(protocol,wire,location):
 """Return sorted DisplayFieldValue-shaped objects for one externally proven own header.
 location = {'raw_start': int, 'raw_end': int, 'own_offset': int, 'own_end': int}.
 Bounds are absolute original-wire coordinates. own_end must already honor the parent envelope;
 raw_start/raw_end describe the exact Native.Source1 byte slice, not guessed by searching output.
 A caller must separately prove carrier admission, original-record provenance and snapshot metadata.
 """
 assert type(protocol)is str and protocol in('arp','icmp','icmpv6')and type(wire)is bytes
 keys=('raw_start','raw_end','own_offset','own_end');assert all(type(location[k])is int for k in keys)
 start,end,at,own_end=(location[k]for k in keys);assert 0<=start<=at<=own_end<=end<=len(wire)
 body=wire[at:own_end];items=[]
 def add(name,offset,length):
  owner,typ,bits=SPEC[name];assert owner==protocol and 0<=offset and offset+length<=len(body)
  value=body[offset:offset+length]
  if typ=='unsigned':value=int.from_bytes(value,'big');assert 0<=value<(1<<bits)
  elif typ=='address':
   addr=ipaddress.ip_address(value);assert addr.version==(4 if bits==32 else 6)
   value='::ffff:'+str(addr.ipv4_mapped)if addr.version==6 and addr.ipv4_mapped is not None else str(addr)
  else:assert name=='arp.src.hw_mac'and length==6;value=value.hex(':')
  items.append(dict(name=name,value=value,origin='pdu',pdu_range=dict(offset=at-start+offset,length=length)))
 if protocol=='arp':
  if len(body)<8:return[]
  h,p=body[4:6]
  if len(body)<8+2*(h+p):return[]
  add('arp.opcode',6,2)
  if h==6:add('arp.src.hw_mac',8,6)
  if p==4 and body[2:4]==b'\x08\x00':add('arp.src.proto_ipv4',8+h,4);add('arp.dst.proto_ipv4',8+2*h+p,4)
 elif protocol=='icmp':
  if len(body)<8:return[]
  typ,code=body[:2];add('icmp.type',0,1);add('icmp.code',1,1)
  if typ in(0,8,13,14,15,16,17,18):add('icmp.ident',4,2);add('icmp.seq',6,2)
  if typ==3 and code==4:add('icmp.mtu',6,2)
  if typ==5:add('icmp.redir_gw',4,4)
  if typ==12:add('icmp.pointer',4,1)
  if typ in(13,14):
   for name,offset in(('icmp.originate_timestamp',8),('icmp.receive_timestamp',12),('icmp.transmit_timestamp',16)):
    if len(body)>=offset+4:add(name,offset,4)
 else:
  if len(body)<4:return[]
  typ=body[0];add('icmpv6.type',0,1);add('icmpv6.code',1,1)
  if typ in(128,129)and len(body)>=8:add('icmpv6.echo.identifier',4,2);add('icmpv6.echo.sequence_number',6,2)
  if typ==2 and len(body)>=8:add('icmpv6.mtu',4,4)
  if typ in(135,136,137)and len(body)>=24:add({135:'icmpv6.nd.ns.target_address',136:'icmpv6.nd.na.target_address',137:'icmpv6.nd.rd.target_address'}[typ],8,16)
  if typ==137 and len(body)>=40:add('icmpv6.rd.na.destination_address',24,16)
 return sorted(items,key=lambda x:x['name'])
