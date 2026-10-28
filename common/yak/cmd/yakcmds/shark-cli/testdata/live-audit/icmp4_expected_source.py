"""Whole ICMPv4 observations from literal original bytes and declared bounded carriers.
Legacy IPv4 totalLength0 is recorded as an observed compatibility profile, not wire validity.
No source location is found by matching Native.Raw, decoded values, or quoted child labels.
"""
import base64,copy,hashlib,struct
from literal_own_header import parent,ip

def same(a,b):
 if type(a)is not type(b):return False
 if isinstance(a,dict):return a.keys()==b.keys()and all(same(a[k],b[k])for k in a)
 if isinstance(a,list):return len(a)==len(b)and all(same(x,y)for x,y in zip(a,b))
 return a==b

def source(record):
 wire=record['wire'];loc=parent(record)
 if loc['covered']:
  assert loc['protocol']=='icmp'
  loc.update(raw_start=0,raw_end=len(wire),encapsulation=('/mpacket:express'if record['link']==274 else'')+''.join('/vlan:%d'%x for x in loc.get('vlans',[])),scope='canonical direct/simple carrier'if loc.get('family_consistent')else'cross-family observed own-header prefix',carrier_headers=[])
  return loc
 assert record['link']==1 and len(wire)>=14,'unselected fallback link'
 at=14;end=len(wire);kind=int.from_bytes(wire[12:14],'big');path=['Ethernet'];encap='';headers=[];scopes=[]
 for _ in range(8):
  if kind not in(0x8100,0x88a8):break
  assert at+4<=end;tag=int.from_bytes(wire[at:at+2],'big')&4095;encap+='/vlan:%d'%tag;headers.append(dict(kind='VLAN',offset=at,length=4,identifier=tag));kind=int.from_bytes(wire[at+2:at+4],'big');at+=4;path.append('802.1Q')
 assert kind not in(0x8100,0x88a8)
 if kind==0x8864:
  assert at+8<=end and wire[at]==0x11 and wire[at+1]==0
  sid=int.from_bytes(wire[at+2:at+4],'big');size=int.from_bytes(wire[at+4:at+6],'big');assert sid>0 and size>=2 and at+6+size<=end and wire[at+6:at+8]==b'\x00\x21'
  encap+='/pppoe:%d'%sid;headers.append(dict(kind='PPPoE-session-uncompressed-PPP-IPv4',offset=at,length=8,session=sid,declared_payload=size));end=at+6+size;at+=8;kind=0x800;path+=['PPPoE','PPP'];scopes.append('validated PPPoE session + uncompressed PPP IPv4')
 assert kind==0x800,'unselected fallback network'
 raw_start=0;raw_end=len(wire);normalized=False
 for depth in range(5):
  start=at;assert start+20<=end and wire[start]>>4==4
  ihl=(wire[start]&15)*4;total=int.from_bytes(wire[start+2:start+4],'big');assert ihl>=20 and start+ihl<=end and int.from_bytes(wire[start+6:start+8],'big')&0x3fff==0
  pos=start+20
  while pos<start+ihl:
   typ=wire[pos]
   if typ==0:break
   if typ==1:pos+=1;continue
   assert pos+2<=start+ihl and wire[pos+1]>=2 and pos+wire[pos+1]<=start+ihl;pos+=wire[pos+1]
  if total==0:scopes.append('observed IPv4 totalLength0 compatibility; not a validated wire extent')
  else:assert total>=ihl;end=min(end,start+total)
  assert start+ihl<=end
  src,dst=ip(wire[start+12:start+16]),ip(wire[start+16:start+20]);proto=wire[start+9];headers.append(dict(kind='IPv4',offset=start,length=ihl,declared_total=total,available=end-start,protocol=proto,source=src,destination=dst));path.append('IPv4');at=start+ihl
  if normalized:raw_end=end
  if proto==1:
   return dict(covered=True,protocol='icmp',offset=at,end=end,available=end-at,source=src,destination=dst,path=path+['ICMPv4'],raw_start=raw_start,raw_end=raw_end,encapsulation=encap,scope='; '.join(scopes)or'canonical bounded carrier',carrier_headers=headers)
  assert proto==47 and depth<4,'unselected fallback child'
  g=at;assert g+4<=end;flags=int.from_bytes(wire[g:g+2],'big');ptype=int.from_bytes(wire[g+2:g+4],'big');assert flags&7==0 and ptype==0x800,'unselected GRE version or payload'
  pos=g+4
  if flags&0xc000:pos+=4
  key=0
  if flags&0x2000:assert pos+4<=end;key=int.from_bytes(wire[pos:pos+4],'big');pos+=4
  if flags&0x1000:pos+=4
  assert pos<=end
  if flags&0x4000:
   for _ in range(128):
    assert pos+4<=end;family=int.from_bytes(wire[pos:pos+2],'big');length=wire[pos+3];pos+=4;assert pos+length<=end;pos+=length
    if family==0 and length==0:break
   else:raise AssertionError('GRE route budget')
  assert pos<=end
  # RFC2784 bits6..12 are receive-ignored; RFC1701 legacy s/Recur metadata
  # is observed without claiming route execution or a valid delivery header.
  encap+='/gre:'+ '~'.join(sorted((src,dst)))+':%d'%key
  headers.append(dict(kind='GREv0',offset=g,length=pos-g,flags=flags,protocol=ptype,key=key));path.append('GRE');scopes.append('GREv0 literal payload layout'+(' with legacy flag observation'if flags&0x0ff8 else''));at=pos;raw_start=pos;raw_end=end;normalized=True
 raise AssertionError('carrier depth')

NAMES={0:'EchoReply',3:'DestinationUnreachable',4:'SourceQuench',5:'Redirect',8:'EchoRequest',9:'RouterAdvertisement',10:'RouterSolicitation',11:'TimeExceeded',12:'ParameterProblem',13:'TimestampRequest',14:'TimestampReply',15:'InfoRequest',16:'InfoReply',17:'AddressMaskRequest',18:'AddressMaskReply'}
CODES={3:{0:'Net',1:'Host',2:'Protocol',3:'Port',4:'FragmentationNeeded',5:'SourceRoutingFailed',6:'NetUnknown',7:'HostUnknown',8:'SourceIsolated',9:'NetAdminProhibited',10:'HostAdminProhibited',11:'NetTOS',12:'HostTOS',13:'CommAdminProhibited',14:'HostPrecedence',15:'PrecedenceCutoff'},5:{0:'Net',1:'Host',2:'TOS+Net',3:'TOS+Host'},11:{0:'TTLExceeded',1:'FragmentReassemblyTimeExceeded'},12:{0:'PointerIndicatesError',1:'MissingOption',2:'BadLength'}}
def typecode(t,c):
 if t not in NAMES:return '%d(%d)'%(t,c)
 name=NAMES[t]
 if t in CODES:return name+'('+CODES[t][c]+')'if c in CODES[t]else name+'(Code: %d)'%c
 return name if c==0 else name+'(Code: %d)'%c

def fields(record,loc,old=False):
 body=record['wire'][loc['offset']:loc['end']];assert len(body)>=8;t,c=body[:2];out={'Type':t,'Code':c};required=8
 if old:
  out['Identifier']=int.from_bytes(body[4:6],'big');out['Sequence']=int.from_bytes(body[6:8],'big')
  if t in(3,11):out['Quoted Packet']=base64.b64encode(body[8:]).decode()
  return out,8,'decoded'
 if t in(0,8,13,14,15,16,17,18):out['Identifier']=int.from_bytes(body[4:6],'big');out['Sequence']=int.from_bytes(body[6:8],'big')
 else:out['Rest of Header']=base64.b64encode(body[4:8]).decode()
 if t==3 and c==4:out['MTU']=int.from_bytes(body[6:8],'big')
 if t==5:out['Gateway']=ip(body[4:8])
 if t==12:out['Pointer']=body[4]
 if t in(3,4,5,11,12):out['Quoted Packet']=base64.b64encode(body[8:]).decode()
 if t in(13,14):
  required=20
  for key,at in[('Originate Timestamp',8),('Receive Timestamp',12),('Transmit Timestamp',16)]:
   if len(body)>=at+4:out[key]=int.from_bytes(body[at:at+4],'big')
 if t in(17,18):
  required=12
  if len(body)>=12:out['Address Mask']=ip(body[8:12])
 status='decoded'if len(body)>=required else'incomplete'
 if status=='incomplete':out['Available ICMP Bytes']=len(body);out['Required ICMP Header Bytes']=required
 return out,required,status

def expected(identifier,record,old=False):
 assert type(identifier)is int and identifier>0;loc=source(record);wire=record['wire'];raw=wire[loc['raw_start']:loc['raw_end']];f,required,status=fields(record,loc,old);domain=dict(Section=record['section'],Interface=record['interface'],Encapsulation=loc['encapsulation']);ref=dict(Number=record['frame'],Domain=copy.deepcopy(domain));src=dict(Kind='captured',SHA256=hashlib.sha256(raw).hexdigest(),ParentPDU=0,ParentPDUs=None,PacketRefs=[copy.deepcopy(ref)])
 if status=='decoded':src['CompletedIn']=copy.deepcopy(ref)
 complete='message'if status=='decoded'else'incomplete';code=''if status=='decoded'else'ICMPHeaderTruncated';summary='ICMP '+typecode(f['Type'],f['Code'])
 if code:summary+='; incomplete fixed header (%d/%d bytes)'%(loc['end']-loc['offset'],required)
 values=[dict(name='destination',value=loc['destination'],origin='event'),dict(name='flow.id',value=0,exact_value='0',origin='event'),dict(name='ip.addr',value=loc['source'],origin='event'),dict(name='ip.addr',value=loc['destination'],origin='event')]
 if status=='decoded':values.append(dict(name='pdu.completed_in',value=record['frame'],exact_value=str(record['frame']),origin='event'))
 values+=[dict(name='pdu.id',value=identifier,exact_value=str(identifier),origin='event'),dict(name='pdu.length',value=len(raw),exact_value=str(len(raw)),origin='event'),dict(name='protocol',value='icmp',origin='event'),dict(name='source',value=loc['source'],origin='event'),dict(name='status',value=status,origin='event')]
 e=dict(id=identifier,flow=0,protocol='icmp',status=status,profile='icmp'if status=='decoded'else'icmp-header-observation',completeness=complete,summary=summary,expert=code,source_bytes=src,fields=copy.deepcopy(f),session=copy.deepcopy(f),field_values=dict(schema_version=1,pdu=identifier,source=copy.deepcopy(src),values=values,completeness=complete),raw=base64.b64encode(raw).decode(),entry='',admission='',source=loc['source'],destination=loc['destination'],direction=0,transport='network',domain=domain,carrier_layers=None,transaction=0,response_to=0)
 if code:e['error']=code
 return e
