"""Independent bounded PIMv2 prefix facts, not a routing/neighbor state machine.
RFC7761 common/Hello/Register/Join/Prune/Assert, RFC3973 Graft/StateRefresh,
and RFC5059 Bootstrap/CandidateRP layouts. Encoding zero only is selected.
Unknown encodings retain their real AF/encoding prefix without inventing width.
"""
import base64,ipaddress

NAMES=['Hello','Register','Register-Stop','Join/Prune','Bootstrap','Assert','Graft','Graft-Ack','Candidate-RP-Advertisement','State Refresh']
class Stop(Exception):
 def __init__(self,kind,reason,offset,required=None,available=None):self.fact=dict(kind=kind,reason=reason,offset=offset,required_end=required,available_end=available)
class Reader:
 def __init__(self,w,fields,at=0,end=None):self.w=w;self.fields=fields;self.at=at;self.end=len(w)if end is None else end
 def take(self,path,n,alias=None,value=None,mask=None):
  a=self.at
  if n<0 or a+n>self.end:raise Stop('short-field','field crosses selected envelope: '+'.'.join(map(str,path)),a,a+n,self.end)
  b=self.w[a:a+n];self.at+=n
  kind='bytes'if value is None else'address'
  if value is None:value=base64.b64encode(b).decode()
  self.fields.append(dict(path=path,alias=alias,offset=a,length=n,value=value,raw_hex=b.hex(),mask=mask,kind=kind))
  return b
 def num(self,path,n,alias=None):
  a=self.at;b=self.take(path,n,alias);v=int.from_bytes(b,'big');self.fields[-1]['value']=v;self.fields[-1]['kind']='unsigned';return v
 def bits(self,path,alias,a,n,value,mask):self.fields.append(dict(path=path,alias=alias,offset=a,length=n,value=value,raw_hex=self.w[a:a+n].hex(),mask=mask,kind='boolean'if type(value)is bool else'unsigned'))

def checksum(w,src,dst,n):
 b=bytes(w);s=ipaddress.ip_address(src);d=ipaddress.ip_address(dst)
 if s.version!=d.version:raise ValueError('address family mismatch')
 if s.version==6:b=s.packed+d.packed+n.to_bytes(4,'big')+b'\0\0\0'+bytes([103])+b
 if len(b)&1:b+=b'\0'
 v=sum(int.from_bytes(b[i:i+2],'big')for i in range(0,len(b),2))
 while v>>16:v=(v&65535)+(v>>16)
 return(~v)&65535

def decode(w,source,destination,declared_bytes,budget=4096):
 w=bytes(w);fields=[];r=Reader(w,fields);out={'available_bytes':len(w),'declared_bytes':declared_bytes,'fields':fields,'authentication_verified':False,'route_state_installed':False,'neighbor_state_verified':False,'checksum_verified':False,'checksum_scope':'not verified','extent':'unknown'if declared_bytes<0 else'incomplete'if len(w)<declared_bytes else'complete'}
 remain=[budget]
 def charge(at):
  if remain[0]<=0:raise Stop('budget','collection observation budget',at)
  remain[0]-=1
 def addr(c,path,kind,role):
  charge(c.at);af=c.num(path+['Address Family'],1,'pim.addr_address_family');enc=c.num(path+['Encoding'],1,'pim.addr_encoding_type')
  if af not in(1,2)or enc!=0:raise Stop('context','unselected address family/encoding %d/%d'%(af,enc),c.at)
  n=4 if af==1 else 16
  if kind in('group','source'):
   a=c.at;flags=c.num(path+['Flags'],1)
   mask=c.num(path+['Mask Length'],1,'pim.mask_len')
   if kind=='group':
    c.bits(path+['Bidirectional'],'pim.group_addr.flags.b',a,1,bool(flags&128),128);c.bits(path+['Admin Scope Zone'],'pim.group_addr.flags.z',a,1,bool(flags&1),1)
   else:
    for key,alias,bit in[('Sparse','s',4),('Wildcard','w',2),('RPT','r',1)]:c.bits(path+[key],'pim.source_addr.flags.'+alias,a,1,bool(flags&bit),bit)
   if mask>n*8:raise Stop('invalid-field','mask exceeds address width',c.at-1)
  a=c.at;b=c.take(path+['Address'],n);text=str(ipaddress.ip_address(b));c.fields[-1]['value']=text;c.fields[-1]['kind']='address';c.fields[-1]['alias']='pim.'+role+(('6'if role in('join_ip','prune_ip')else'_ip6')if n==16 else'')
  if kind=='group'and not ipaddress.ip_address(b).is_multicast:raise Stop('invalid-field','encoded group is not multicast',a)
  return text
 try:
  x=r.num(['Version and Type'],1);version=x>>4;typ=x&15
  r.bits(['Version'],'pim.version',0,1,version,240);r.bits(['Type'],'pim.type',0,1,typ,15)
  r.num(['Reserved'],1);r.num(['Checksum'],2,'pim.cksum')
  out.update(version=version,type=typ,message_name=NAMES[typ]if typ<10 else'Unknown')
  if version!=2:raise Stop('context','unselected PIM version',0)
  if typ==0:
   option=0
   while r.at<r.end:
    charge(r.at);base=r.at;c=Reader(w,fields,base);path=['Options',option]
    t=c.num(path+['Option Type'],2,'pim.optiontype');n=c.num(path+['Option Length'],2,'pim.optionlength');end=c.at+n
    if end>c.end:raise Stop('short-option','option value exceeds observed message',c.at,end,c.end)
    c.end=end;fixed={1:2,2:4,19:4,20:4,21:4}.get(t)
    if fixed is not None and n<fixed:raise Stop('invalid-option-length','known option length %d requires %d'%(n,fixed),base+2)
    if t==1:c.num(path+['Hold Time'],2,'pim.holdtime')
    elif t==2:
     a=c.at;v=c.num(path+['Propagation Flags'],2);c.bits(path+['Tracking Support'],'pim.t',a,2,bool(v&32768),32768);c.bits(path+['Propagation Delay'],'pim.propagation_delay',a,2,v&32767,32767);c.num(path+['Override Interval'],2,'pim.override_interval')
    elif t==19:c.num(path+['DR Priority'],4,'pim.dr_priority')
    elif t==20:c.num(path+['Generation ID'],4,'pim.generation_id')
    elif t==21:
     c.num(path+['State Refresh Version'],1,'pim.state_refresh_version');c.num(path+['State Refresh Interval'],1,'pim.state_refresh_interval');c.num(path+['State Refresh Reserved'],2,'pim.state_refresh_reserved')
    elif t in(24,65001):
     index=0
     while c.at<c.end:addr(c,path+['Addresses',index],'unicast','address_list');index+=1
    else:c.take(path+['Unknown Option Value'],n)
    if c.at!=end:raise Stop('invalid-option-length','known option trailing bytes',c.at)
    r.at=end;option+=1
  elif typ==1:
   a=r.at;flags=r.num(['Register Flags'],4,'pim.register_flag');null=bool(flags&0x40000000)
   r.bits(['Border'],'pim.register_flag.border',a,4,bool(flags&0x80000000),0x80000000);r.bits(['Null Register'],'pim.register_flag.null_register',a,4,null,0x40000000)
   start=r.at;b=w[start:]
   if not b:raise Stop('short-field','missing Register IP header',start,start+20,r.end)
   ver=b[0]>>4
   if null and ver==0:ver=ipaddress.ip_address(source).version
   if ver not in(4,6):raise Stop('context','unselected Register IP version',start)
   if ver!=ipaddress.ip_address(source).version:raise Stop('invalid-field','Register inner/outer address family mismatch',start)
   minimum=20 if ver==4 else 40
   if len(b)<minimum:raise Stop('short-field','incomplete Register IP header',start,start+minimum,r.end)
   if not null:
    if ver==4:
     ihl=(b[0]&15)*4;total=int.from_bytes(b[2:4],'big')
     if ihl<20 or ihl>len(b)or total<ihl:raise Stop('invalid-field','invalid Register IPv4 length',start)
     if total>len(b):raise Stop('short-field','incomplete Register IPv4 payload',start,start+total,r.end)
    elif int.from_bytes(b[4:6],'big')+40>len(b):raise Stop('short-field','incomplete Register IPv6 payload',start,start+40+int.from_bytes(b[4:6],'big'),r.end)
   n=4 if ver==4 else 16
   for name,off in[('Source',12 if ver==4 else 8),('Group',16 if ver==4 else 24)]:
    at=start+off;text=str(ipaddress.ip_address(w[at:at+n]));c=Reader(w,fields,at);c.take(['Register IP Header',name],n,'pim.observed.register.'+name.lower(),text)
    if name=='Group'and not ipaddress.ip_address(text).is_multicast:raise Stop('invalid-field','Register group is not multicast',at)
   r.take(['Encapsulated Bytes'],len(w)-start)
  elif typ in(2,5,9):
   addr(r,['Group'],'group','group');addr(r,['Source'],'unicast','source')
   if typ==9:addr(r,['Originator'],'unicast','originator')
   if typ in(5,9):
    a=r.at;v=r.num(['Metric Preference Word'],4);r.bits(['RPT'],'pim.rpt',a,4,bool(v&0x80000000),0x80000000);r.bits(['Metric Preference'],'pim.metric_pref',a,4,v&0x7fffffff,0x7fffffff);r.num(['Metric'],4,'pim.metric')
   if typ==9:
    r.num(['Mask Length'],1,'pim.mask_len');r.num(['TTL'],1,'pim.ttl');a=r.at;v=r.num(['Refresh Flags'],1)
    for key,alias,mask in[('Prune Indicator','prune_indicator',128),('Prune Now','prune_now',64),('Assert Override','assert_override',32)]:r.bits([key],'pim.'+alias,a,1,bool(v&mask),mask)
    r.num(['Interval'],1,'pim.interval')
  elif typ in(3,6,7):
   addr(r,['Upstream Neighbor'],'unicast','upstream_neighbor');r.num(['Join Reserved'],1);n=r.num(['Group Count'],1,'pim.numgroups');r.num(['Hold Time'],2,'pim.holdtime')
   for i in range(n):
    charge(r.at);path=['Groups',i];addr(r,path+['Group'],'group','group');joined=r.num(path+['Join Count'],2,'pim.numjoins');pruned=r.num(path+['Prune Count'],2,'pim.numprunes')
    for key,num,role in[('Joined Sources',joined,'join_ip'),('Pruned Sources',pruned,'prune_ip')]:
     for j in range(num):addr(r,path+[key,j],'source',role)
  elif typ==4:
   r.num(['Fragment Tag'],2,'pim.fragment_tag');r.num(['Hash Mask Length'],1,'pim.hash_mask_len');r.num(['BSR Priority'],1,'pim.bsr_priority');addr(r,['BSR'],'unicast','bsr');i=0
   while r.at<r.end:
    charge(r.at);path=['Groups',i];addr(r,path+['Group'],'group','group');n=r.num(path+['RP Count'],1,'pim.rp_count');fragment=r.num(path+['Fragment RP Count'],1,'pim.frp_count');r.num(path+['RP Reserved'],2)
    if fragment>n:raise Stop('invalid-field','fragment RP count exceeds total',r.at-3)
    for j in range(fragment):
     charge(r.at);p=path+['RPs',j];addr(r,p+['RP'],'unicast','rp');r.num(p+['Hold Time'],2,'pim.holdtime');r.num(p+['Priority'],1,'pim.priority');r.num(p+['Reserved'],1)
    i+=1
  elif typ==8:
   n=r.num(['Prefix Count'],1,'pim.prefix_count');r.num(['Priority'],1,'pim.priority');r.num(['Hold Time'],2,'pim.holdtime');addr(r,['RP'],'unicast','rp')
   for i in range(n):addr(r,['Advertised Groups',i],'group','group')
  else:raise Stop('context','unselected PIM message type',r.at)
  if r.at!=r.end:raise Stop('invalid-length','message trailing bytes',r.at)
  out['grammar_complete']=True
  if out['extent']=='complete':
   n=8 if typ==1 else len(w);good=checksum(w[:n],source,destination,n)==0;scope='register-header'if typ==1 else'message'
   if typ==1 and not good and checksum(w,source,destination,8)==0:good=True;scope='register-whole-message-interop'
   out['checksum_verified']=good;out['checksum_scope']=scope
   if not good:raise Stop('checksum','PIM checksum failed',2)
 except Stop as e:out['diagnostic']=e.fact;out['grammar_complete']=False
 out['observed_prefix_end']=max((x['offset']+x['length']for x in fields),default=0)
 # Unknown/incomplete parent extent is never corrected by a successful body parse.
 out['complete_observation']=out.get('grammar_complete',False)and out['extent']=='complete'and out['checksum_verified']
 return out
