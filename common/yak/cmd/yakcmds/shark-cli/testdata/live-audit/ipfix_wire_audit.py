"""Independent RFC7011 UDP wire/field audit; explicit reference selection stays separate.
Only bounded complete positive messages are asserted here. No live-capture claim.
"""
import argparse,base64,hashlib,ipaddress,json,struct
from datetime import datetime,timedelta,timezone
from pathlib import Path
ap=argparse.ArgumentParser(description=__doc__)
for n in ['oracle','events','out']:ap.add_argument('--'+n,required=True,type=Path)
a=ap.parse_args();checks=0
def eq(got,want,why):
 global checks
 checks+=1
 assert got==want,(why,got,want)
def values(x,key):
 if isinstance(x,dict):
  return [z for k,v in x.items() for z in ((v if isinstance(v,list) else [v]) if k==key else values(v,key))]
 if isinstance(x,list):return [z for v in x for z in values(v,key)]
 return []
def u(w,p,n):return int.from_bytes(w[p:p+n],'big')
def parse(w,schemas):
 eq(u(w,0,2),10,'wire version');eq(u(w,2,2),len(w),'message boundary')
 result=[];at=16
 while at<len(w):
  sid,n=struct.unpack_from('!HH',w,at);end=at+n;assert n>=4 and end<=len(w);p=at+4;s={'id':sid,'length':n,'templates':[],'records':[]}
  if sid in [2,3]:
   while p<end:
    if end-p<4:assert not any(w[p:end]);break
    start=p;tid,nf=struct.unpack_from('!HH',w,p);p+=4;assert nf>0 and tid>=256
    scope=0
    if sid==3:scope=u(w,p,2);p+=2;assert 0<scope<=nf
    fs=[]
    for i in range(nf):
     literal,length=struct.unpack_from('!HH',w,p);p+=4;pen=None
     if literal&32768:pen=u(w,p,4);p+=4
     assert p<=end;fs.append((literal&32767,length,pen,i<scope))
    assert any(f[1] for f in fs)
    schemas[tid]={'fields':fs,'hash':hashlib.sha256(w).hexdigest(),'definition_hash':hashlib.sha256(w[start:p]).hexdigest(),'offset':start,'length':p-start}
    s['templates'].append((tid,fs))
  else:
   assert sid>=256 and sid in schemas,('unknown schema',sid)
   t=schemas[sid];s['template']=t;minimum=sum(1 if f[1]==65535 else f[1] for f in t['fields']);assert minimum>0
   while p<end:
    if end-p<minimum:assert not any(w[p:end]);s['padding']=w[p:end];p=end;break
    fs=[]
    for typ,length,pen,scope in t['fields']:
     if length==65535:
      length=u(w,p,1);p+=1
      if length==255:length=u(w,p,2);p+=2
     assert p+length<=end;fs.append((typ,pen,scope,w[p:p+length],p));p+=length
    s['records'].append(fs)
  result.append(s);at=end
 return result
native={}
with a.events.open(encoding='utf-8-sig') as f:
 for line in f:
  e=json.loads(line)
  if e['protocol']=='ipfix':native.setdefault(base64.b64decode(e['raw']),[]).append(e)
schemas={};messages=records=unknown_values=0;cases=[]
aliases={1:'cflow.octets',2:'cflow.packets',4:'cflow.protocol',6:'cflow.tcpflags',7:'cflow.srcport',8:'cflow.srcaddr',10:'cflow.inputint',11:'cflow.dstport',12:'cflow.dstaddr',14:'cflow.outputint',15:'cflow.nexthop',18:'cflow.bgpnexthop'}
with a.oracle.open(encoding='utf-8-sig') as f:oracle=json.load(f)
for packet in oracle:
 layers=packet['_source']['layers']
 if 'cflow' not in layers or layers['cflow'].get('cflow.version')!='10':continue
 w=bytes.fromhex(layers['cflow_raw'][0]);queue=native.get(w,[]);assert queue,'missing exact native message';e=queue.pop(0);m=e['fields'];c=layers['cflow'];messages+=1
 eq(e['status'],'decoded','message status');eq(e['flow'],0,'reported records are not captured connections');eq(e['transport'],'udp','transport')
 eq(e['source_bytes']['SHA256'],hashlib.sha256(w).hexdigest(),'owned source hash')
 eq(e['source_bytes']['CompletedIn']['Number'],int(values(layers,'frame.number')[0]),'physical completion frame')
 for name,offset,n,ref in [('Version',0,2,'cflow.version'),('Message Length',2,2,'cflow.len'),('Export Time',4,4,'cflow.exporttime'),('Sequence Number',8,4,'cflow.sequence'),('Observation Domain ID',12,4,'cflow.od_id')]:
  eq(m[name],u(w,offset,n),name);eq(values(c,ref),[str(m[name])],ref);eq(m['Wire Field Ranges'][name],[offset,offset+n],name+' byte range')
 key=(values(layers,'ip.src')[0],values(layers,'ip.dst')[0],values(layers,'udp.srcport')[0],values(layers,'udp.dstport')[0],u(w,12,4));context=schemas.setdefault(key,{})
 expected=parse(w,context);eq(len(m['FlowSets']),len(expected),'Set count');total=0
 for s,ns in zip(expected,m['FlowSets']):
  eq(ns['FlowSet ID'],s['id'],'Set ID');eq(ns['FlowSet Length'],s['length'],'Set length')
  if s['id'] in [2,3]:
   eq(len(ns['Templates']),len(s['templates']),'template count')
   for nt,(tid,fs) in zip(ns['Templates'],s['templates']):
    eq(nt['Template ID'],tid,'template ID');eq(nt['Field Count'],len(fs),'template fields');eq(len(nt['Fields']),len(fs),'complete native template specifiers')
    for nf,(typ,length,pen,scope) in zip(nt['Fields'],fs):
     eq(nf['Field Type'],typ,'IE ID');eq(nf['Field Length'],length,'IE length');eq(nf['Enterprise'],pen is not None,'enterprise bit');eq(nf.get('Enterprise Number'),pen,'PEN');eq(nf['Scope'],scope,'scope')
  else:
   eq(len(ns['Records']),len(s['records']),'data records');total+=len(s['records']);t=s['template']
   for nk,ek in [('Template PDU SHA256','hash'),('Template Definition SHA256','definition_hash'),('Template Definition Offset','offset'),('Template Definition Length','length')]:eq(ns[nk],t[ek],nk)
   for nr,fr in zip(ns['Records'],s['records']):
    eq(len(nr['Fields']),len(fr),'record fields')
    for nf,(typ,pen,scope,b,start) in zip(nr['Fields'],fr):
     eq(nf['Field Type'],typ,'data IE ID');eq(nf['Enterprise'],pen is not None,'data namespace');eq(nf['Scope'],scope,'data scope');eq(nf['Wire Field Ranges']['Value'],[start,start+len(b)],'data exact range')
     if nf['Value Kind']=='opaque':eq(base64.b64decode(nf['Value']),b,'opaque exact bytes');unknown_values+=1
     elif nf['Value Kind'] in ['IPv4','IPv6']:eq(nf['Value'],str(ipaddress.ip_address(b)),'typed address')
     else:eq(nf['Value'],int.from_bytes(b,'big'),'typed number')
 for typ,alias in aliases.items():
  selected=[(b,start) for ss in expected for fr in ss['records'] for t,p,sc,b,start in fr if t==typ and p is None and not sc]
  expected_values=[str(ipaddress.ip_address(b)) if typ in [8,12,15,18] else int.from_bytes(b,'big') for b,start in selected]
  ref=values(c,alias);eq([int(x,0) if typ==6 else int(x) for x in ref] if typ not in [8,12,15,18] else ref,expected_values,alias+' reference scalars')
  projected=[v for v in e['field_values']['values'] if v['name']==alias]
  eq([v['value'] for v in projected],expected_values,alias+' projected native values')
  eq([v.get('pdu_range') for v in projected],[{'offset':start,'length':len(b)} for b,start in selected],alias+' projected exact ranges')
 for typ,key in [(152,'cflow.abstimestart'),(153,'cflow.abstimeend')]:
  times=[int.from_bytes(b,'big') for ss in expected for fr in ss['records'] for t,p,sc,b,st in fr if t==typ and p is None and not sc]
  text=[(datetime(1970,1,1,tzinfo=timezone.utc)+timedelta(milliseconds=x)).strftime('%Y-%m-%dT%H:%M:%S.')+('%09d'%((x%1000)*1000000))+'Z' for x in times]
  eq(values(c,key),text,key+' absolute millisecond time')
 eq(m['Data Record Count'],total,'message Data Record count');eq(m['Record Count Verified'],True,'count completeness');records+=total
 cases.append({'frame':values(layers,'frame.number')[0],'message_sha256':hashlib.sha256(w).hexdigest(),'bytes':len(w),'data_records':total})
assert messages>0
eq(sum(map(len,native.values())),0,'no unmatched native IPFIX messages')
out={'scope':'complete positive UDP IPFIX messages; independent wire/layout/value/range assertions and selected reference scalar fields; opaque unknown IEs retained, not semantic coverage','messages':messages,'data_records':records,'opaque_values':unknown_values,'checks':checks,'cases':cases,'selected_differences':[]}
a.out.parent.mkdir(parents=True,exist_ok=True);a.out.write_text(json.dumps(out,indent=2)+'\n',encoding='utf8');print(json.dumps(out))
