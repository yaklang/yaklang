"""Full ICMPv6 failure objects derived from original captured bytes, not new output."""
import base64,copy,hashlib
from literal_own_header import parent,ip

def same(a,b):
 if type(a)is not type(b):return False
 if isinstance(a,dict):return a.keys()==b.keys()and all(same(a[k],b[k])for k in a)
 if isinstance(a,list):return len(a)==len(b)and all(same(x,y)for x,y in zip(a,b))
 return a==b

def source_profile(record):
 loc=parent(record)
 if loc['covered']:
  assert loc['protocol']=='icmpv6';return loc
 # The 21 other observed sources explicitly have totalLength zero. The
 # dependency's documented Windows-TSO compatibility uses the captured tail;
 # this records existing parser interpretation and never asserts wire validity.
 w=record['wire'];assert loc['reason']=='invalid-IPv4-length'and record['link']==1 and w[12:14]==b'\x08\x00'
 at=14;assert w[at]>>4==4 and int.from_bytes(w[at+2:at+4],'big')==0 and w[at+9]==58
 size=(w[at]&15)*4;assert size>=20 and at+size<=len(w)and int.from_bytes(w[at+6:at+8],'big')&0x3fff==0
 pos=at+20
 while pos<at+size:
  if w[pos]==0:break
  if w[pos]==1:pos+=1;continue
  assert pos+2<=at+size and w[pos+1]>2 and pos+w[pos+1]<=at+size;pos+=w[pos+1]
 return dict(covered=True,protocol='icmpv6',offset=at+size,end=len(w),available=len(w)-at-size,path=['Ethernet','IPv4','ICMPv6'],source=ip(w[at+12:at+16]),destination=ip(w[at+16:at+20]),family_consistent=False,parent_extent_complete=None,declared_IPv4_total_length=0,interpretation_scope='observed existing zero-length compatibility; not a validated wire IP extent')

def fixed_prefix(record):
 p=source_profile(record);b=record['wire'][p['offset']:p['end']];assert len(b)>=4;typ,code=b[:2];required={1:8,2:8,3:8,4:8,128:8,129:8,133:8,134:16,135:24,136:24,137:40}[typ]
 fields={'Type':typ,'Code':code};payload=b[4:];options=[];ranges=[]
 if typ in(135,136,137)and len(payload)>=20:fields['Target']=ip(payload[4:20])
 if typ==137 and len(payload)>=36:fields['Destination']=ip(payload[20:36])
 if typ in(1,2,3,4)and len(payload)>=4:
  fields['Quoted Packet']=base64.b64encode(payload[4:]).decode()
  if typ==2:fields['MTU']=int.from_bytes(payload[:4],'big')
 if typ in(128,129)and len(payload)>=4:fields['Identifier']=int.from_bytes(payload[:2],'big');fields['Sequence']=int.from_bytes(payload[2:4],'big')
 if len(b)<required:
  status='incomplete';expert='ICMPv6HeaderTruncated';fields['Available ICMPv6 Bytes']=len(b);fields['Required ICMPv6 Header Bytes']=required;refusal=dict(offset=p['offset']+len(b),available=len(b),required=required)
 else:
  assert 133<=typ<=137;at=required;status='decoded';expert='';refusal=None
  while at<len(b):
   if at+2>len(b)or b[at+1]==0 or at+8*b[at+1]>len(b):
    status='malformed';expert='NDOptionLength';refusal=dict(offset=p['offset']+at,remaining=len(b)-at,type=b[at],length_units=b[at+1]if at+1<len(b)else None);break
   size=8*b[at+1];options.append(dict(Type=b[at],Value=base64.b64encode(b[at+2:at+size]).decode()));ranges.append(dict(offset=p['offset']+at,length=size));at+=size
  fields['Options']=options or None
  if not expert:fields['Observation']='unverified-neighbor'
 return dict(parent=p,available=len(b),required=required,fields=fields,status=status,expert=expert,valid_options=ranges,refusal=refusal)

def type_code(typ,code):
 names={1:'DestinationUnreachable',2:'PacketTooBig',3:'TimeExceeded',4:'ParameterProblem',128:'EchoRequest',129:'EchoReply',133:'RouterSolicitation',134:'RouterAdvertisement',135:'NeighborSolicitation',136:'NeighborAdvertisement',137:'Redirect'}
 codes={1:{0:'NoRouteToDst',1:'AdminProhibited',2:'BeyondScopeOfSrc',3:'AddressUnreachable',4:'PortUnreachable',5:'SrcAddressFailedPolicy',6:'RejectRouteToDst'},3:{0:'HopLimitExceeded',1:'FragmentReassemblyTimeExceeded'},4:{0:'ErroneousHeaderField',1:'UnrecognizedNextHeader',2:'UnrecognizedIPv6Option'}}
 name=names[typ]
 if typ in codes:return name+'('+codes[typ][code]+')'if code in codes[typ]else name+'(Code: %d)'%code
 return name if code==0 else name+'(Code: %d)'%code

def expected_failure(identifier,record):
 assert type(identifier)is int and identifier>0;fact=fixed_prefix(record);assert fact['status']in('incomplete','malformed');p=fact['parent'];fields=fact['fields'];status=fact['status'];expert=fact['expert'];name=type_code(fields['Type'],fields['Code'])
 summary='ICMPv6 '+name+'; invalid ND option length'if expert=='NDOptionLength'else'ICMPv6 '+name+'; incomplete fixed header (%d/%d bytes)'%(fact['available'],fact['required'])
 domain={'Section':record['section'],'Interface':record['interface'],'Encapsulation':''};wire=record['wire'];source={'Kind':'captured','SHA256':hashlib.sha256(wire).hexdigest(),'ParentPDU':0,'ParentPDUs':None,'PacketRefs':[{'Number':record['frame'],'Domain':domain}]}
 values=[{'name':'destination','value':p['destination'],'origin':'event'},{'name':'flow.id','value':0,'exact_value':'0','origin':'event'},{'name':'ip.addr','value':p['source'],'origin':'event'},{'name':'ip.addr','value':p['destination'],'origin':'event'},{'name':'pdu.id','value':identifier,'exact_value':str(identifier),'origin':'event'},{'name':'pdu.length','value':len(wire),'exact_value':str(len(wire)),'origin':'event'},{'name':'protocol','value':'icmpv6','origin':'event'},{'name':'source','value':p['source'],'origin':'event'},{'name':'status','value':status,'origin':'event'}]
 return {'id':identifier,'flow':0,'protocol':'icmpv6','status':status,'profile':'icmpv6-header-observation','completeness':status,'summary':summary,'expert':expert,'source_bytes':source,'fields':copy.deepcopy(fields),'session':fields,'field_values':{'schema_version':1,'pdu':identifier,'source':copy.deepcopy(source),'values':values,'completeness':status},'raw':base64.b64encode(wire).decode(),'entry':'','admission':'','source':p['source'],'destination':p['destination'],'direction':0,'transport':'network','domain':domain,'carrier_layers':None,'transaction':0,'response_to':0,'error':expert}
