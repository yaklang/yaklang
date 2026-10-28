"""Pure bounded source interpreter for the ESP public-corpus audit scope."""
import hashlib,ipaddress,struct

def ipstr(w):return str(ipaddress.ip_address(w))
def network(w,link):
 at=0;end=len(w);path=[]
 if link==1:
  if len(w)<14:return {'reason':'short-Ethernet'}
  typ=int.from_bytes(w[12:14],'big');at=14;path=['Ethernet']
  for _ in range(8):
   if typ not in(0x8100,0x88a8,0x9100):break
   if at+4>end:return {'reason':'short-VLAN'}
   typ=int.from_bytes(w[at+2:at+4],'big');at+=4;path.append('802.1Q')
  version={0x800:4,0x86dd:6}.get(typ)
 elif link in(101,228,229):version=w[0]>>4 if w else None
 else:return {'reason':'unselected-link','link':link}
 result={'path':path,'ip_offset':at}
 if version==4:
  if at+20>end or w[at]>>4!=4:return dict(result,reason='invalid-IPv4-fixed')
  size=(w[at]&15)*4;total=int.from_bytes(w[at+2:at+4],'big')
  if size<20 or at+size>end or total<size:return dict(result,reason='invalid-IPv4-IHL-length')
  flags=int.from_bytes(w[at+6:at+8],'big');next_header=w[at+9]
  result.update(source=ipstr(w[at+12:at+16]),destination=ipstr(w[at+16:at+20]),ip_version=4,ip_header_complete=True,ip_extent_complete=at+total<=end,ip_total_length=total,ip_fragment_word=flags,path=path+['IPv4'])
  end=min(end,at+total);at+=size
  if flags&0x3fff:return dict(result,reason='fragment-not-reassembled',declared_next=next_header)
 elif version==6:
  if at+40>end or w[at]>>4!=6:return dict(result,reason='invalid-IPv6-fixed')
  length=int.from_bytes(w[at+4:at+6],'big');next_header=w[at+6]
  result.update(source=ipstr(w[at+8:at+24]),destination=ipstr(w[at+24:at+40]),ip_version=6,ip_header_complete=True,ip_extent_complete=at+40+length<=end,ip_total_length=40+length,path=path+['IPv6'])
  end=min(end,at+40+length);at+=40
  if length==0:return dict(result,reason='unselected-zero-payload-IPv6',declared_next=next_header)
 else:return dict(result,reason='not-IP')
 for _ in range(16):
  if next_header not in(0,43,44,51,60):break
  if at+2>end:return dict(result,reason='short-extension-fixed',declared_next=next_header)
  kind=next_header;size=8 if kind==44 else(w[at+1]+2)*4 if kind==51 else(w[at+1]+1)*8
  if at+size>end:return dict(result,reason='short-extension-envelope',declared_next=next_header)
  if kind==51 and size<12:return dict(result,reason='short-AH-fixed',declared_next=next_header)
  if kind in(0,60):
   pos=at+2
   while pos<at+size:
    if w[pos]==0:pos+=1;continue
    if pos+2>at+size or pos+2+w[pos+1]>at+size:return dict(result,reason='malformed-extension-TLV',declared_next=w[at])
    pos+=2+w[pos+1]
  if kind==44 and int.from_bytes(w[at+2:at+4],'big')&0xfff9:return dict(result,reason='fragment-not-reassembled',declared_next=w[at])
  result['path'].append({0:'IPv6HopByHop',43:'IPv6Routing',44:'IPv6Fragment',51:'AH',60:'IPv6Destination'}[kind]);next_header=w[at];at+=size
 result.update(declared_next=next_header,payload_offset=at,payload_length=end-at)
 if next_header==50:result.update(selection='explicit-IP-next-header-50',esp_offset=at,esp_end=end)
 elif next_header==17:
  if at+8>end:return dict(result,reason='short-UDP')
  src,dst,length=struct.unpack_from('!HHH',w,at)
  result.update(udp_source_port=src,udp_destination_port=dst,udp_length=length)
  if length<8 or at+length>end:return dict(result,reason='invalid-UDP-envelope')
  body=w[at+8:at+length]
  if (src==4500 or dst==4500)and len(body)>=8 and body[:4]!=b'\0'*4:
   result.update(selection='UDP4500-ESP-candidate-not-IP50',esp_offset=at+8,esp_end=at+length)
  else:result['reason']='UDP-without-nonzero-ESP-header-candidate'
 elif next_header in(1,58):result['reason']='ICMP-quotation-boundary'
 else:result['reason']='non-ESP-network-protocol'
 if 'esp_offset'in result:
  body=w[result['esp_offset']:result['esp_end']]
  result.update(esp_length=len(body),esp_header_complete=len(body)>=8,esp_sha256=hashlib.sha256(body).hexdigest())
  if len(body)>=4:result['spi']=int.from_bytes(body[:4],'big')
  if len(body)>=8:result['sequence']=int.from_bytes(body[4:8],'big')
 return result
