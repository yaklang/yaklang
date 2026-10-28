"""Independent literal source oracle for direct IPv4 refusal observations.

This selects a refusal only after validating the physical fixed parent and its
options. IPv4 HOPOPT remains an unselected carrier, not a proved IPv6 child.
The corpus's literal zero IPv4 total length is recorded separately; it proves
neither a valid wire envelope nor a capture-loss cause. No packet/session state
or child field is admitted by these diagnostics.
"""
import copy,hashlib,ipaddress,json

def same(a,b):
 if type(a)is not type(b):return False
 if isinstance(a,dict):return a.keys()==b.keys()and all(same(a[k],b[k])for k in a)
 if isinstance(a,list):return len(a)==len(b)and all(same(x,y)for x,y in zip(a,b))
 return a==b

def literal(rec):
 w=rec['wire']
 if rec['link']!=1 or len(w)<34 or w[12:14]!=b'\x08\x00':return None
 ip=w[14:];ihl=(ip[0]&15)*4;total=int.from_bytes(ip[2:4],'big');flags=int.from_bytes(ip[6:8],'big')
 if ip[0]>>4!=4 or ihl<20 or ihl>len(ip) or flags&0x3fff:return None
 # The existing capture adapter accommodates zero length in host captures.
 # Preserve that literal fact; it cannot validate child wire lengths.
 effective=total or len(ip)
 if effective<ihl:return None
 at=20
 while at<ihl:
  if ip[at]==0:break
  if ip[at]==1:at+=1;continue
  if at+2>ihl or not 2<ip[at+1]<=ihl-at:return None
  at+=ip[at+1]
 payload=ip[ihl:min(effective,len(ip))];proto=ip[9]
 facts=dict(protocol=proto,literal_total_length=total,header_length=ihl,captured_network_length=len(ip),ipv4_options_valid=True,fragment_word=flags)
 source=destination=''
 if total>=ihl:
  source=str(ipaddress.ip_address(ip[12:16]));destination=str(ipaddress.ip_address(ip[16:20]))
 if proto==0:return dict(facts,profile='network-encapsulation-refusal',status='limited',expert='EncapsulationDepthOrProfile',error='EncapsulationDepthOrProfile',source='',destination='',summary='Encapsulation profile unavailable; inner identity unconfirmed',session={'Observed Header Layers':['Ethernet','IPv4'],'Header Observation Scope':'outer network prefix only','Child Header Identity':'unconfirmed','Encapsulation Admission':'unselected profile'})
 code=reason=None;names=['Ethernet','IPv4']
 for _ in range(128):
  if proto==0:
   code='IPv6OptionsHeaderOrder';reason='IPv6 Hop-by-Hop Options must immediately follow the IPv6 fixed header';break
  if proto==51:
   code='AHHeaderBounds'
   if len(payload)<12:reason=f'AH fixed header requires 12 bytes, got {len(payload)}';break
   if payload[1]==0:reason='AH payload length cannot be zero';break
   n=(payload[1]+2)*4
   if n>len(payload):reason=f'AH declared header requires {n} bytes, got {len(payload)}';break
   names.append('IPSecAH');proto,payload=payload[0],payload[n:];continue
  if proto==43:
   if len(payload)<8:return None
   n=(payload[1]+1)*8
   if n>len(payload)or payload[2]!=0 or(n-8)%16!=0:return None
   names.append('IPv6Routing');proto,payload=payload[0],payload[n:];continue
  if proto==60:
   code='IPv6OptionsHeaderBounds'
   if len(payload)<2:reason='IPv6 options extension requires a two-byte fixed prefix';break
   n=(payload[1]+1)*8
   if n>len(payload):reason=f'IPv6 options extension requires {n} bytes, got {len(payload)}';break
   at=2
   while at<n:
    if payload[at]==0:at+=1;continue
    if at+2>n:reason='IPv6 option length octet exceeds its extension header';break
    if at+2+payload[at+1]>n:reason='IPv6 option value exceeds its extension header';break
    at+=2+payload[at+1]
   if reason:break
   names.append('IPv6Destination');proto,payload=payload[0],payload[n:];continue
  if proto==50:
   if total!=0:return None
   code='ESPParentLengthInvalid';reason='ESP parent IP length is invalid; clear ESP header not admitted';break
  break
 if not(code and reason):return None
 return dict(facts,failure_next_header=proto,profile='network-header-refusal',status='malformed',expert=code,error=reason,source=source,destination=destination,summary=reason+'; child identity not admitted',session={'Observed Header Layers':names,'Header Observation Scope':'validated network prefix before refused child header','Child Header Identity':'unconfirmed','Parent Header Refusal':code})

def expected_event(event_id,rec):
 facts=literal(rec);assert facts is not None,'Source is outside the declared direct-parent refusal profile'
 domain=dict(Section=rec['section'],Interface=rec['interface'],Encapsulation='')
 source=dict(Kind='captured',SHA256='',ParentPDU=0,ParentPDUs=None,PacketRefs=[dict(Number=rec['frame'],Domain=domain)])
 values=[]
 def field(name,value,exact=False):
  item=dict(name=name,value=value,origin='event')
  if exact:item['exact_value']=str(value)
  values.append(item)
 if facts['destination']:field('destination',facts['destination'])
 field('flow.id',0,True)
 for address in [facts['source'],facts['destination']]:
  if address:field('ip.addr',address)
 field('pdu.id',event_id,True);field('pdu.length',0,True);field('protocol','ip')
 if facts['source']:field('source',facts['source'])
 field('status',facts['status'])
 expected=dict(admission='',carrier_layers=None,completeness=facts['status'],destination=facts['destination'],direction=0,domain=domain,entry='',error=facts['error'],expert=facts['expert'],field_values=dict(schema_version=1,pdu=event_id,source=source,values=values,completeness=facts['status']),fields=None,fields_error='protocol parser: no exact rule for this message',flow=0,id=event_id,profile=facts['profile'],protocol='ip',raw=None,response_to=0,session=facts['session'],source=facts['source'],source_bytes=source,status=facts['status'],summary=facts['summary'],transaction=0,transport='network')
 if facts['status']=='limited':
  expected['fields_error']='protocol parser: event is limited, not an exact message'
  expected['field_values']['incomplete']=True
 return expected,facts

def check(event,rec):
 assert type(event['id'])is int and event['id']>0
 expected,facts=expected_event(event['id'],rec)
 assert same(expected,event),(rec['frame'],{k:[expected.get(k),event.get(k)]for k in expected.keys()|event.keys()if not same(expected.get(k),event.get(k))})
 return facts
