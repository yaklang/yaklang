"""Independent RFC2210/2215 source model for complete Intserv object bodies.

Produces neutral source observations, not a candidate API schema. No Go or
runtime output is imported. Finite floats use exact decimal strings plus owned
IEEE32 bits; nonfinite encodings use explicit classifications and never JSON
NaN/Infinity. Unknown service/parameter data is opaque. Full malformed/partial
control matrices remain explicitly untested in this initial source inventory.
"""
import base64
from decimal import Decimal,localcontext
from fractions import Fraction
import hashlib

WR='Wire Field Ranges'
OBJECT_NAMES={9:'FLOWSPEC',12:'SENDER_TSPEC',13:'ADSPEC'}
PARAM_NAMES={4:'Composed IS Hop Count',6:'Composed Available Path Bandwidth',8:'Composed Minimum Path Latency',10:'Composed Path MTU',127:'Token Bucket TSpec'}

def digest(w):return hashlib.sha256(w).hexdigest()

def ieee32(w):
 assert len(w)==4
 bits=int.from_bytes(w,'big');sign=bits>>31;exponent=(bits>>23)&255;fraction=bits&0x7fffff
 if exponent==255:
  category='nan'if fraction else'negative-infinity'if sign else'positive-infinity'
  return {'wire_uint32':bits,'classification':category,'exact_decimal':None,'display':{'nan':'NaN','positive-infinity':'+Infinity','negative-infinity':'-Infinity'}[category]}
 if exponent==0 and fraction==0:
  return {'wire_uint32':bits,'classification':'negative-zero'if sign else'positive-zero','exact_decimal':'-0'if sign else'0','display':'-0'if sign else'0'}
 value=Fraction(fraction if exponent==0 else(1<<23)+fraction,1)*Fraction(2)**(exponent-127-23 if exponent else-126-23)
 if sign:value=-value
 with localcontext()as context:
  context.prec=200
  decimal=format(Decimal(value.numerator)/Decimal(value.denominator),'f')
  if'.'in decimal:decimal=decimal.rstrip('0').rstrip('.')
 return {'wire_uint32':bits,'classification':'finite','exact_decimal':decimal,'display':decimal}


def service_name(cls,number):
 if cls==12 and number==1:return'Global Sender Traffic Specification'
 if cls==9 and number==5:return'Controlled-Load Receiver Traffic Specification'
 if cls==9 and number==2:return'Guaranteed Receiver Traffic Specification'
 if cls==13 and number==1:return'Default General Characterization Parameters'
 if cls==13 and number==5:return'Controlled-Load Characterization'
 if cls==13 and number==2:return'Guaranteed Characterization'
 return'Unselected service '+str(number)+' in '+OBJECT_NAMES.get(cls,'unknown object')


def decode_complete(pdu,start,end,cls):
 """Input extent is independently validated by the original object header."""
 assert cls in OBJECT_NAMES and 0<=start<=end<=len(pdu)
 leaves=[];issues=[]
 def literal(target,path,name,offset,width,kind='uint'):
  assert start<=offset<=offset+width<=end
  wire=pdu[offset:offset+width]
  value=int.from_bytes(wire,'big')if kind=='uint'else ieee32(wire)if kind=='ieee32'else base64.b64encode(wire).decode('ascii')
  target[name]=value;target.setdefault(WR,{})[name]=[offset,offset+width]
  leaves.append({'path':path+'.'+name,'kind':kind,'literal':True,'value':value,'range':[offset,offset+width],'raw_SHA256':digest(wire)})
  return value
 def derived(target,path,name,value,span,mask,shift=0):
  target[name]=value
  leaves.append({'path':path+'.'+name,'kind':'bool'if type(value)is bool else'uint','literal':False,'value':value,'source_range':span,'mask':mask,'shift':shift})
 def issue(code,path,span,primary):issues.append({'code':code,'path':path,'range':span,'primary':primary,'scope':'advertised value-domain issue; wire boundaries/field observations preserved; no reservation result inferred'})
 assert end-start>=4
 root={'Object Class':cls,'Object Name':OBJECT_NAMES[cls],'Header':{},'Services':[]}
 h=root['Header'];combined=literal(h,'Header','Version and Unused',start,2);words=literal(h,'Header','Message Length Words',start+2,2)
 derived(h,'Header','Version',pdu[start]>>4,[start,start+1],240,4)
 derived(h,'Header','Unused Bits',combined&4095,[start,start+2],4095)
 declared=start+4+4*words;assert declared==end,'source inventory requires complete exact object-body extent'
 assert h['Version']==0,'future version must not select version0 body layouts'
 cursor=start+4
 while cursor<declared:
  assert cursor+4<=declared
  index=len(root['Services']);path='Services['+str(index)+']';service={'Header':{},'Parameters':[]}
  sh=service['Header'];number=literal(sh,path+'.Header','Service Number',cursor,1);flags=literal(sh,path+'.Header','Flags',cursor+1,1);length=literal(sh,path+'.Header','Data Length Words',cursor+2,2)
  derived(sh,path+'.Header','Break Bit Observed',bool(flags&128),[cursor+1,cursor+2],128,7)
  derived(sh,path+'.Header','Reserved Flag Bits',flags&127,[cursor+1,cursor+2],127)
  stop=cursor+4+4*length;assert stop<=declared
  service['Namespace']=service_name(cls,number);service['declared_range']=[cursor,stop]
  pos=cursor+4
  selected=(cls==12 and number==1)or(cls==9 and number==5)or(cls==13 and number in(1,5))
  if not selected:
   literal(service,path,'Opaque Service Data',pos,stop-pos,'bytes');service['Layout Selected']=False
  else:
   service['Layout Selected']=True
   while pos<stop:
    assert pos+4<=stop
    pindex=len(service['Parameters']);ppath=path+'.Parameters['+str(pindex)+']';parameter={'Header':{},'Body':{}}
    ph=parameter['Header'];num=literal(ph,ppath+'.Header','Parameter Number',pos,1);pflags=literal(ph,ppath+'.Header','Flags',pos+1,1);pwords=literal(ph,ppath+'.Header','Data Length Words',pos+2,2)
    derived(ph,ppath+'.Header','Invalid Bit Observed',bool(pflags&128),[pos+1,pos+2],128,7)
    derived(ph,ppath+'.Header','Reserved Flag Bits',pflags&127,[pos+1,pos+2],127)
    pend=pos+4+4*pwords;assert pend<=stop
    parameter['declared_range']=[pos,pend];parameter['Parameter Name']=PARAM_NAMES.get(num,'Unselected parameter '+str(num));body=parameter['Body'];at=pos+4
    if num==127 and((cls==12 and number==1)or(cls==9 and number==5)):
     assert pwords==5
     for field,offset,kind in [('Token Bucket Rate',0,'ieee32'),('Token Bucket Size',4,'ieee32'),('Peak Data Rate',8,'ieee32'),('Minimum Policed Unit',12,'uint'),('Maximum Packet Size',16,'uint')]:literal(body,ppath+'.Body',field,at+offset,4,kind)
     for field in('Token Bucket Rate','Token Bucket Size'):
      f=body[field];valid=f['classification']in('finite','positive-zero')and not f['wire_uint32']>>31
      if not valid:issue('invalid-finite-nonnegative-float-domain',ppath+'.Body.'+field,body[WR][field],{'RFC':2215,'section':'3.6','LF_lines':[791,797]})
     peak=body['Peak Data Rate']
     if peak['classification']not in('finite','positive-zero','positive-infinity')or peak['wire_uint32']>>31:issue('invalid-peak-rate-float-domain',ppath+'.Body.Peak Data Rate',body[WR]['Peak Data Rate'],{'RFC':2215,'section':'3.6','LF_lines':[795,797]})
     m=body['Minimum Policed Unit'];maximum=body['Maximum Packet Size']
     if m==0:issue('minimum-policed-unit-zero-explicitly-illegal',ppath+'.Body.Minimum Policed Unit',body[WR]['Minimum Policed Unit'],{'RFC':2210,'section':'3.1','LF_lines':[637,639]})
     if maximum==0:issue('maximum-packet-size-not-positive',ppath+'.Body.Maximum Packet Size',body[WR]['Maximum Packet Size'],{'RFC':2215,'section':'3.6','LF_lines':[768,769]})
     if m>maximum:issue('minimum-policed-unit-exceeds-maximum',ppath+'.Body.Minimum Policed Unit',body[WR]['Minimum Policed Unit'],{'RFC':2215,'section':'3.6','LF_lines':[768,769]})
    elif cls==13 and number in(1,5)and num in(4,6,8,10):
     assert pwords==1
     field=PARAM_NAMES[num];literal(body,ppath+'.Body',field,at,4,'ieee32'if num==6 else'uint')
     if num==6:
      f=body[field]
      if f['classification']not in('finite','positive-zero')or f['wire_uint32']>>31:issue('path-bandwidth-requires-finite-nonnegative',ppath+'.Body.'+field,[at,at+4],{'RFC':2215,'section':'3.3','LF_lines':[499,501]})
    else:
     literal(body,ppath+'.Body','Opaque Parameter Data',at,pend-at,'bytes');parameter['Layout Selected']=False
    service['Parameters'].append(parameter);pos=pend
  root['Services'].append(service);cursor=stop
 assert cursor==declared
 return {'fields':root,'leaves':leaves,'semantic_value_issues':issues,'body_range':[start,end],'body_SHA256':digest(pdu[start:end]),'all_complete_observed_envelopes':True,'no_authentication_or_reservation_or_path_state_verified':True}