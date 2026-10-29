"""Independent neutral Intserv source48 model. No candidate Go/SDK imported.

The frozen source46 IEEE32 arithmetic is reused, while byte ownership, partial
observations and service/parameter budgets are independently modelled here.
Value-domain issues do not erase complete wire primitives or verify a path.
"""
from pathlib import Path
import base64,copy,hashlib,importlib.util,math,struct
from decimal import Decimal
OLD=Path(r'C:\tmp\yak-shark-ESP-SSH-UI-20261028\rsvp_intserv_source46.py')
OLD_SHA='f345e62afbedc80a4d53ef4dc0077403ea08d9a57b147ffb944c3300a11a99ef'
assert hashlib.sha256(OLD.read_bytes()).hexdigest()==OLD_SHA
_spec=importlib.util.spec_from_file_location('frozen_intserv_ieee32_source46',OLD)
_old=importlib.util.module_from_spec(_spec);_spec.loader.exec_module(_old)
WR='Wire Field Ranges'
NAMES={9:'FLOWSPEC',12:'SENDER_TSPEC',13:'ADSPEC'}
PARAMS={4:'Composed IS Hop Count',6:'Composed Available Path Bandwidth',8:'Composed Minimum Path Latency',10:'Composed Path MTU',127:'Token Bucket TSpec'}

def sha(w):return hashlib.sha256(w).hexdigest()
def equal(a,b):
 if type(a)is not type(b):return False
 if isinstance(a,dict):return a.keys()==b.keys()and all(equal(a[k],b[k])for k in a)
 if isinstance(a,list):return len(a)==len(b)and all(equal(x,y)for x,y in zip(a,b))
 return a==b

def decode(raw,start,object_end,cls,budget=4096,ctype=2):
 assert isinstance(raw,bytes)and 0<=start<=object_end and start<=len(raw)and type(budget)is int and budget>=0
 captured_end=min(object_end,len(raw));leaves=[];issues=[];initial_budget=budget;selected_all=True;halt=False
 fields={'Object Class':cls,'Object Name':NAMES.get(cls,'Unselected object '+str(cls)),'Header':{},'Services':[]}
 def issue(kind,code,path,span):
  x={'kind':kind,'code':code,'path':path,'range':list(span)}
  if x not in issues:issues.append(x)
 def derived(row,path,name,value,span,mask,shift=0):
  row[name]=value;leaves.append({'path':path+'.'+name,'kind':'bool'if type(value)is bool else'uint','literal':False,'value':value,'source_range':list(span),'mask':mask,'shift':shift})
 def literal(row,path,name,at,width,kind='uint'):
  w=raw[at:at+width];assert start<=at<=at+width<=captured_end
  value=int.from_bytes(w,'big')if kind=='uint'else _old.ieee32(w)if kind=='ieee32'else base64.b64encode(w).decode('ascii')
  row[name]=value;row.setdefault(WR,{})[name]=[at,at+width]
  leaves.append({'path':path+'.'+name,'kind':kind,'literal':True,'value':value,'range':[at,at+width],'raw_SHA256':sha(w)})
  return value
 def owned(row,path,name,a,b):
  a=min(max(a,start),captured_end);b=min(max(b,a),captured_end)
  return literal(row,path,name,a,b-a,'bytes')
 def charge(path,at):
  nonlocal budget,halt
  if budget==0:issue('budget','collection-token-exhausted',path,[at,at]);halt=True;return False
  budget-=1;return True
 class Reader:
  def __init__(self,at,end,row,path):self.at=at;self.end=end;self.row=row;self.path=path;self.failed=False
  def num(self,name,width,kind='uint'):
   if self.failed:return None
   if self.at+width>self.end:
    issue('format','primitive-crosses-declared-parent',self.path+'.'+name,[self.at,min(self.end,captured_end)]);self.failed=True;return None
   if self.at+width>captured_end:
    issue('shortage','primitive-not-fully-captured',self.path+'.'+name,[self.at,captured_end]);self.failed=True;return None
   value=literal(self.row,self.path,name,self.at,width,kind);self.at+=width;return value
 if cls not in NAMES or ctype!=2:
  issue('context','object-class-or-ctype-unselected','Object',[start,captured_end]);owned(fields,'Root','Opaque Message Data',start,captured_end)
  return finish(fields,leaves,issues,initial_budget,budget,False)
 h=Reader(start,object_end,fields['Header'],'Header');combined=h.num('Version and Unused',2)
 if combined is not None:
  derived(fields['Header'],'Header','Version',combined>>12,[start,start+1],240,4)
  derived(fields['Header'],'Header','Unused Bits',combined&4095,[start,start+2],4095)
 words=h.num('Message Length Words',2)
 if h.failed:
  if not fields['Header']:fields.pop('Header')
  if h.at<captured_end:owned(fields,'Root','Unparsed Message Data',h.at,captured_end)
  return finish(fields,leaves,issues,initial_budget,budget,False)
 if combined>>12!=0:
  issue('context','message-version-unselected','Header.Version',[start,start+1]);owned(fields,'Root','Opaque Message Data',h.at,captured_end)
  return finish(fields,leaves,issues,initial_budget,budget,False)
 message_end=start+4+4*words;limit=min(message_end,object_end)
 if message_end>object_end:issue('format','message-extent-crosses-object','Header.Message Length Words',[start+2,start+4])
 if message_end< object_end:
  issue('format','object-bytes-after-intserv-message','Header.Message Length Words',[start+2,start+4]);owned(fields,'Root','Trailing Message Bytes',message_end,captured_end)
 if limit>captured_end:issue('shortage','message-not-fully-captured','Root',[captured_end,limit])
 cursor=start+4
 while cursor<min(limit,captured_end)and not halt:
  index=len(fields['Services']);path='Services['+str(index)+']'
  if not charge(path,cursor):break
  row={'Header':{},'Parameters':[]};sh=Reader(cursor,limit,row['Header'],path+'.Header')
  number=sh.num('Service Number',1);flags=sh.num('Flags',1);words=sh.num('Data Length Words',2)
  if flags is not None:
   derived(row['Header'],path+'.Header','Break Bit Observed',bool(flags&128),[cursor+1,cursor+2],128,7)
   derived(row['Header'],path+'.Header','Reserved Flag Bits',flags&127,[cursor+1,cursor+2],127)
  fields['Services'].append(row)
  if sh.failed:
   if sh.at<min(limit,captured_end):owned(row,path,'Unparsed Service Data',sh.at,min(limit,captured_end))
   cursor=limit;halt=True;break
  service_end=cursor+4+4*words;service_limit=min(service_end,limit);row['declared_range']=[cursor,service_end]
  row['Namespace']=_old.service_name(cls,number)
  selected=(cls==12 and number==1)or(cls==9 and number==5)or(cls==13 and number in(1,5))
  row['Layout Selected']=selected;selected_all=selected_all and selected
  if service_end>limit:issue('format','service-extent-crosses-message',path+'.Header.Data Length Words',[cursor+2,cursor+4])
  if service_limit>captured_end:issue('shortage','service-not-fully-captured',path,[captured_end,service_limit])
  pos=sh.at
  if not selected:
   owned(row,path,'Opaque Service Data',pos,min(service_limit,captured_end));pos=service_limit
  else:
   while pos<min(service_limit,captured_end)and not halt:
    pi=len(row['Parameters']);pp=path+'.Parameters['+str(pi)+']'
    if not charge(pp,pos):break
    parameter={'Header':{},'Body':{}};ph=Reader(pos,service_limit,parameter['Header'],pp+'.Header')
    num=ph.num('Parameter Number',1);pflags=ph.num('Flags',1);pwords=ph.num('Data Length Words',2)
    if pflags is not None:
     derived(parameter['Header'],pp+'.Header','Invalid Bit Observed',bool(pflags&128),[pos+1,pos+2],128,7)
     derived(parameter['Header'],pp+'.Header','Reserved Flag Bits',pflags&127,[pos+1,pos+2],127)
    row['Parameters'].append(parameter)
    if ph.failed:
     if ph.at<min(service_limit,captured_end):owned(parameter,pp,'Unparsed Parameter Data',ph.at,min(service_limit,captured_end))
     pos=service_limit;halt=True;break
    parameter_end=pos+4+4*pwords;parameter_limit=min(parameter_end,service_limit);parameter['declared_range']=[pos,parameter_end]
    parameter['Parameter Name']=PARAMS.get(num,'Unselected parameter '+str(num))
    if parameter_end>service_limit:issue('format','parameter-extent-crosses-service',pp+'.Header.Data Length Words',[pos+2,pos+4])
    if parameter_limit>captured_end:issue('shortage','parameter-not-fully-captured',pp,[captured_end,parameter_limit])
    body=parameter['Body'];br=Reader(ph.at,parameter_limit,body,pp+'.Body');layout=None
    if num==127 and((cls==12 and number==1)or(cls==9 and number==5)):
     layout=[('Token Bucket Rate','ieee32'),('Token Bucket Size','ieee32'),('Peak Data Rate','ieee32'),('Minimum Policed Unit','uint'),('Maximum Packet Size','uint')]
    elif cls==13 and number in(1,5)and num in(4,6,8,10):layout=[(PARAMS[num],'ieee32'if num==6 else'uint')]
    if layout is None:
     parameter['Layout Selected']=False;selected_all=False;owned(body,pp+'.Body','Opaque Parameter Data',br.at,min(parameter_limit,captured_end));br.at=parameter_limit
    else:
     if pwords!=len(layout):issue('format','selected-parameter-word-count',pp+'.Header.Data Length Words',[pos+2,pos+4])
     for name,kind in layout:br.num(name,4,kind)
     value_domain(body,pp+'.Body',issues)
     if br.at<min(parameter_limit,captured_end):owned(body,pp+'.Body','Unparsed Parameter Data',br.at,min(parameter_limit,captured_end))
    hard=br.failed or parameter_end>service_limit or(layout is not None and pwords!=len(layout))or parameter_limit>captured_end
    pos=parameter_limit
    if hard:halt=True
   if halt and pos<min(service_limit,captured_end):owned(row,path,'Unparsed Service Data',pos,min(service_limit,captured_end))
  cursor=service_limit
  if service_end>limit or service_limit>captured_end:halt=True
 if cursor<min(limit,captured_end):owned(fields,'Root','Unparsed Message Data',cursor,min(limit,captured_end))
 return finish(fields,leaves,issues,initial_budget,budget,selected_all)

def value_domain(body,path,issues):
 def add(code,name):issues.append({'kind':'semantic','code':code,'path':path+'.'+name,'range':body[WR][name]})
 for name in('Token Bucket Rate','Token Bucket Size','Peak Data Rate','Composed Available Path Bandwidth'):
  if name not in body:continue
  f=body[name];cls=f['classification'];sign=f['wire_uint32']>>31
  finite=cls in('finite','positive-zero')and sign==0
  if name=='Peak Data Rate':
   if not(finite or cls=='positive-infinity'):add('invalid-peak-rate-representation',name)
  elif not finite:add('invalid-finite-nonnegative-representation',name)
  if name in('Token Bucket Rate','Token Bucket Size')and cls=='positive-zero':add('token-bucket-must-be-positive',name)
 if body.get('Minimum Policed Unit')==0:add('minimum-policed-unit-zero-explicitly-illegal','Minimum Policed Unit')
 if body.get('Maximum Packet Size')==0:add('maximum-packet-size-not-positive','Maximum Packet Size')
 if'Minimum Policed Unit'in body and'Maximum Packet Size'in body and body['Minimum Policed Unit']>body['Maximum Packet Size']:add('minimum-policed-unit-exceeds-maximum','Minimum Policed Unit')

def finish(fields,leaves,issues,initial,remaining,selected):
 return {'fields':fields,'leaves':leaves,'issues':issues,'collection_tokens_consumed':initial-remaining,'budget_remaining':remaining,'wire_envelopes_complete':not any(x['kind']in('format','shortage','budget','context')for x in issues),'field_layouts_complete':selected and not any(x['kind']in('format','shortage','budget','context')for x in issues),'value_domain_issues_observed':sum(x['kind']=='semantic'for x in issues),'authentication_or_reservation_verified':False,'complete_procedure_or_service_grammar_verified':False,'INVALID_bit_is_not_current_validity_proof':True}

def validate_source_leaves(raw,result):
 seen=set()
 for leaf in result['leaves']:
  assert leaf['path']not in seen;seen.add(leaf['path'])
  if leaf['literal']:
   a,b=leaf['range'];w=raw[a:b];assert len(w)==b-a and sha(w)==leaf['raw_SHA256']
   if leaf['kind']=='uint':assert int.from_bytes(w,'big')==leaf['value']
   elif leaf['kind']=='bytes':assert base64.b64decode(leaf['value'],validate=True)==w
   else:
    assert len(w)==4;bits=int.from_bytes(w,'big');f=struct.unpack('!f',w)[0];v=leaf['value'];assert v['wire_uint32']==bits
    if math.isnan(f):assert v['classification']=='nan'and v['exact_decimal']is None
    elif math.isinf(f):assert v['classification']==('negative-infinity'if f<0 else'positive-infinity')and v['exact_decimal']is None
    else:assert Decimal.from_float(f)==Decimal(v['exact_decimal'])
  else:
   a,b=leaf['source_range'];value=(int.from_bytes(raw[a:b],'big')&leaf['mask'])>>leaf['shift']
   assert value==(int(leaf['value'])if type(leaf['value'])is bool else leaf['value'])
 return {'literal_leaves':sum(x['literal']for x in result['leaves']),'derived_leaves':sum(not x['literal']for x in result['leaves'])}

def check_observation(raw,expected,observed):
 validate_source_leaves(raw,expected)
 assert equal(expected,observed),'whole neutral source observation differs'
 return True