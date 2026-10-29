"""Independent RSVP authored-boundary source model, not a production parser.

No candidate source/output is imported. Half-open literal ranges refer to the
entire logical RSVP PDU. This follows only the frozen baseline profile and the
explicit source contract: v1/common messages 1..7, object envelopes, IPv4/IPv6
baseline layouts and SCOPE addresses. Opaque values prove ownership, not service
semantics or state. Unknown-class reception/forwarding rules remain unexecuted.
"""
import base64
import hashlib
import ipaddress

WR='Wire Field Ranges'
MESSAGE_NAMES={1:'Path',2:'Resv',3:'PathErr',4:'ResvErr',5:'PathTear',6:'ResvTear',7:'ResvConf'}
CLASS_NAMES={1:'SESSION',3:'RSVP_HOP',5:'TIME_VALUES',6:'ERROR_SPEC',7:'SCOPE',8:'STYLE',9:'FLOWSPEC',10:'FILTER_SPEC',11:'SENDER_TEMPLATE',12:'SENDER_TSPEC',13:'ADSPEC',15:'RESV_CONFIRM'}

def digest(raw):
 return hashlib.sha256(raw).hexdigest()

def checksum(raw):
 data=raw+b'\0'*(len(raw)%2)
 total=sum(int.from_bytes(data[i:i+2],'big') for i in range(0,len(data),2))
 while total>>16:total=(total&65535)+(total>>16)
 return (~total)&65535

def layout(cls,ctype):
 if ctype not in (1,2):return None
 width=4 if ctype==1 else 16
 if cls==1:return [('Destination Address',0,width,'address'),('Protocol ID',width,1,'uint'),('Flags',width+1,1,'uint'),('Destination Port',width+2,2,'uint')]
 if cls==3:return [('Hop Address',0,width,'address'),('Logical Interface Handle',width,4,'uint')]
 if cls==6:return [('Error Node Address',0,width,'address'),('Flags',width,1,'uint'),('Error Code',width+1,1,'uint'),('Error Value',width+2,2,'uint')]
 if cls in (10,11):return [('Source Address',0,width,'address'),('Unused',width,2,'uint'),('Source Port',width+2,2,'uint')]
 if cls==15:return [('Receiver Address',0,width,'address')]
 if cls==5 and ctype==1:return [('Refresh Period Milliseconds',0,4,'uint')]
 if cls==8 and ctype==1:return [('Flags',0,1,'uint'),('Option Vector',1,3,'uint')]
 return None

class SourceModel:
 def __init__(self,raw,budget):
  assert isinstance(raw,bytes) and isinstance(budget,int) and budget>=0
  self.raw=raw
  self.budget=budget
  self.used=0
  self.leaves=[]
  self.issues=[]
  self.layouts_complete=True
  self.fields={'Header':{},'Authentication Verified':False,'Procedure Semantics Verified':False,'Reservation State Inferred':False}
  self.cursor=0
  self.limit=len(raw)

 def issue(self,kind,reason,offset):
  self.issues.append({'kind':kind,'reason':reason,'offset':offset})

 def literal(self,target,path,name,start,width,kind='uint'):
  end=start+width
  assert 0<=start<=end<=len(self.raw)
  raw=self.raw[start:end]
  if kind=='uint':value=int.from_bytes(raw,'big')
  elif kind=='address':value=str(ipaddress.ip_address(raw))
  elif kind=='bytes':value=base64.b64encode(raw).decode('ascii')
  else:raise AssertionError(kind)
  target[name]=value
  target.setdefault(WR,{})[name]=[start,end]
  self.leaves.append({'path':path+'.'+name,'kind':kind,'literal':True,'value':value,'range':[start,end],'raw_SHA256':digest(raw)})
  return value

 def unparsed(self,start):
  if start<self.limit and 'Unparsed Body' not in self.fields:
   self.literal(self.fields,'Root','Unparsed Body',start,self.limit-start,'bytes')

 def token(self,offset,kind):
  if self.used>=self.budget:
   self.layouts_complete=False
   self.issue('budget','shared object/address collection budget exhausted before '+kind,offset)
   self.unparsed(offset)
   return False
  self.used+=1
  return True

 def known_body(self,row,path,start,objend,capend,cls,ctype):
  body=row['Body']
  if cls==7 and ctype in (1,2):
   width=4 if ctype==1 else 16
   declared_bytes=objend-start
   if declared_bytes%width:
    self.issue('format','SCOPE declared body is not a whole number of addresses',start)
    self.layouts_complete=False
   body['Addresses']=None
   pos=start
   while pos+width<=capend:
    if not self.token(pos,'SCOPE address'):return False
    addresses=body['Addresses']
    if addresses is None:addresses=body['Addresses']=[]
    item={}
    self.literal(item,path+'.Body.Addresses['+str(len(addresses))+']','Address',pos,width,'address')
    addresses.append(item)
    pos+=width
   self.cursor=pos
   if pos<capend:self.unparsed(pos)
   return True
  spec=layout(cls,ctype)
  if spec is None:
   self.layouts_complete=False
   self.literal(body,path+'.Body','Opaque Body',start,max(0,capend-start),'bytes')
   self.cursor=capend
   return True
  expected=max(offset+width for _,offset,width,_ in spec)
  if objend-start!=expected:
   self.issue('format','selected fixed body length contradicts its layout',start)
   self.layouts_complete=False
  pos=start
  for name,rel,width,kind in spec:
   offset=start+rel
   if offset+width>capend:
    self.cursor=offset
    self.layouts_complete=False
    self.unparsed(min(offset,capend))
    return True
   self.literal(body,path+'.Body',name,offset,width,kind)
   pos=offset+width
  self.cursor=pos
  if pos<capend:self.unparsed(pos)
  return True

 def run(self):
  header=self.fields['Header']
  for name,start,width in [('Version and Flags',0,1),('Message Type',1,1),('Checksum',2,2),('Send TTL',4,1),('Reserved',5,1),('Message Length',6,2)]:
   if start+width<=len(self.raw):self.literal(header,'Header',name,start,width)
  if self.raw:
   for name,mask,shift in [('Version',240,4),('Flags',15,0)]:
    value=(self.raw[0]&mask)>>shift
    header[name]=value
    self.leaves.append({'path':'Header.'+name,'kind':'uint','literal':False,'value':value,'source_range':[0,1],'mask':mask,'shift':shift})
  if len(self.raw)<8:
   self.layouts_complete=False
   self.issue('need_more','fixed common header is incomplete',len(self.raw))
   return self.finish(None)
  declared=header['Message Length']
  if declared<8:
   self.layouts_complete=False
   self.issue('format','declared message length is smaller than fixed common header',6)
   return self.finish(None)
  self.limit=min(declared,len(self.raw))
  if declared>len(self.raw):
   self.layouts_complete=False
   self.issue('need_more','declared RSVP message extent exceeds captured bytes',len(self.raw))
  if declared<len(self.raw):self.literal(self.fields,'Root','Trailing Bytes',declared,len(self.raw)-declared,'bytes')
  if header['Version']!=1 or header['Message Type'] not in MESSAGE_NAMES:
   self.layouts_complete=False
   self.issue('context','message type/version is outside selected baseline profile',1)
   if self.limit>8:self.literal(self.fields,'Root','Unparsed Body',8,self.limit-8,'bytes')
   return self.finish(declared)
  self.fields['Objects']=None
  pos=8
  while pos<self.limit:
   available=self.limit-pos
   if available<2:
    self.layouts_complete=False
    self.issue('format' if declared<=len(self.raw) else 'need_more','object has no complete Length primitive',pos)
    self.unparsed(pos)
    break
   if not self.token(pos,'object'):break
   rows=self.fields['Objects']
   if rows is None:rows=self.fields['Objects']=[]
   path='Objects['+str(len(rows))+']'
   row={'Header':{},'Body':{}}
   rows.append(row)
   for name,rel,width in [('Length',0,2),('Class',2,1),('C-Type',3,1)]:
    if rel+width<=available:self.literal(row['Header'],path+'.Header',name,pos+rel,width)
   length=row['Header']['Length']
   if available<4:
    self.layouts_complete=False
    self.issue('format' if declared<=len(self.raw) else 'need_more','object header is incomplete',pos+available)
    break
   if length<4:
    self.layouts_complete=False
    self.issue('format','object length is smaller than its fixed header',pos)
    self.unparsed(pos+4)
    break
   if length%4:
    self.layouts_complete=False
    self.issue('format','object length is not a multiple of four',pos)
    self.unparsed(pos+4)
    break
   objend=pos+length
   cross_parent=objend>declared
   short_capture=objend>len(self.raw) and not cross_parent
   if cross_parent:
    self.layouts_complete=False
    self.issue('format','object declared end exceeds RSVP declared extent',pos)
   if short_capture:
    self.layouts_complete=False
    self.issue('need_more','object declared end exceeds captured extent',len(self.raw))
   capend=min(objend,self.limit)
   self.cursor=pos+4
   if not self.known_body(row,path,pos+4,objend,capend,row['Header']['Class'],row['Header']['C-Type']):break
   if cross_parent or short_capture:break
   # A selected layout contradiction is hard. Unknown/opaque objects may
   # continue to the independently declared sibling boundary.
   if any(i['kind']=='format' and i['offset']>=pos for i in self.issues):
    self.unparsed(self.cursor)
    break
   pos=objend
  return self.finish(declared)

 def finish(self,declared):
  header=self.fields['Header']
  observed_checksum=header.get('Checksum')
  omitted=observed_checksum==0 if observed_checksum is not None else None
  can_check=declared is not None and 8<=declared<=len(self.raw) and observed_checksum is not None and not omitted
  verified=bool(can_check and checksum(self.raw[:declared])==0)
  self.fields['Checksum Omitted']=omitted
  self.fields['Checksum Verified']=verified
  self.fields['Field Layouts Complete']=self.layouts_complete
  if can_check and not verified:self.issue('integrity','complete declared RSVP checksum is nonzero and arithmetically invalid',2)
  return {'fields':self.fields,'leaves':self.leaves,'issues':self.issues,
          'source_observation':{'raw_SHA256':digest(self.raw),'captured_PDU_bytes':len(self.raw),
                                'budget':self.budget,'collection_tokens_used':self.used,
                                'checksum_scope_bytes':declared if can_check else None,
                                'checksum_verified':verified,'checksum_omitted':omitted,
                                'no_authentication_or_procedure_or_reservation_state_proved':True}}


def decode_source(raw,budget=4096):
 return SourceModel(raw,budget).run()