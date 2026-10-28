"""Source-derived whole unselected-EtherIP parent observations.
Every child namespace requires its whole declared input within the parent; backing capacity is unavailable. An observed namespace still does not admit its child.
"""
import copy,hashlib

def observed_domain(record,at,end):
 w=record['wire'];suffix='';at+=2
 if at+14>end:return dict(Section=record['section'],Interface=record['interface'],Encapsulation=suffix)
 kind=int.from_bytes(w[at+12:at+14],'big');at+=14
 if kind<=1500:
  end=min(end,at+kind)
  if at+8>end or w[at:at+6]!=b'\xaa\xaa\x03\x00\x00\x00':kind=None
  else:kind=int.from_bytes(w[at+6:at+8],'big');at+=8
 for _ in range(32):
  if kind not in(0x8100,0x88a8):break
  if at+4>end:break
  suffix+='/vlan:%d'%(int.from_bytes(w[at:at+2],'big')&4095);kind=int.from_bytes(w[at+2:at+4],'big');at+=4
 if kind in(0x8863,0x8864)and at+6<=end and int.from_bytes(w[at+4:at+6],'big')<=end-at-6:
  suffix+='/pppoe:%d'%int.from_bytes(w[at+2:at+4],'big')
 return dict(Section=record['section'],Interface=record['interface'],Encapsulation=suffix)

def expected(identifier,record):
 assert type(identifier)is int and identifier>0
 w=record['wire'];assert record['link']==1 and len(w)>=34 and w[12:14]==b'\x08\x00'and w[14]>>4==4 and w[23]==97
 h=(w[14]&15)*4;total=int.from_bytes(w[16:18],'big');assert h>=20 and 14+h<=len(w)and(total==0 or total>=h)and int.from_bytes(w[20:22],'big')&0x3fff==0
 end=min(len(w),14+total)if total else len(w);at=14+h;p=34
 while p<at:
  if w[p]==0:break
  if w[p]==1:p+=1;continue
  assert p+2<=at and w[p+1]>=2 and p+w[p+1]<=at;p+=w[p+1]
 domain=observed_domain(record,at,end)
 ref=dict(Number=record['frame'],Domain=copy.deepcopy(domain));source=dict(Kind='captured',SHA256='',ParentPDU=0,ParentPDUs=None,PacketRefs=[ref]);code='EncapsulationDepthOrProfile'
 values=[dict(name='flow.id',value=0,exact_value='0',origin='event'),dict(name='pdu.id',value=identifier,exact_value=str(identifier),origin='event'),dict(name='pdu.length',value=0,exact_value='0',origin='event'),dict(name='protocol',value='ip',origin='event'),dict(name='status',value='limited',origin='event')]
 return dict(id=identifier,flow=0,protocol='ip',status='limited',profile='network-encapsulation-refusal',completeness='limited',summary='Encapsulation profile unavailable; inner identity unconfirmed',expert=code,source_bytes=source,fields=None,session={'Observed Header Layers':['Ethernet','IPv4'],'Header Observation Scope':'outer network prefix only','Child Header Identity':'unconfirmed','Encapsulation Admission':'unselected profile'},field_values=dict(schema_version=1,pdu=identifier,source=copy.deepcopy(source),values=values,incomplete=True,completeness='limited'),raw=None,entry='',admission='',source='',destination='',direction=0,transport='network',domain=domain,carrier_layers=None,transaction=0,response_to=0,error=code,fields_error='protocol parser: event is limited, not an exact message')
