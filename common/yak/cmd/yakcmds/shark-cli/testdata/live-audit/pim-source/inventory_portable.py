"""Read-only literal PIM source inventory; never search Native.Raw for an offset."""
import argparse,base64,collections,hashlib,ipaddress,json,shutil,sys
from pathlib import Path

def sha(p):
 h=hashlib.sha256()
 with p.open('rb')as f:
  for b in iter(lambda:f.read(1<<20),b''):h.update(b)
 return h.hexdigest()
def dump(p,v):p.write_text(json.dumps(v,ensure_ascii=False,indent=2)+'\n',encoding='utf8')
def rows(p):
 for l in p.open(encoding='utf8'):yield json.loads(l)
def checksum(w,source,destination,pseudo_length):
 b=bytes(w);src=ipaddress.ip_address(source);dst=ipaddress.ip_address(destination)
 if src.version==6:
  assert dst.version==6
  b=src.packed+dst.packed+pseudo_length.to_bytes(4,'big')+b'\0\0\0'+bytes([103])+b
 if len(b)&1:b+=b'\0'
 total=sum(int.from_bytes(b[i:i+2],'big')for i in range(0,len(b),2))
 while total>>16:total=(total&65535)+(total>>16)
 return(~total)&65535

def own_fields(body):
 if len(body)<4:return []
 out=[dict(name='pim.version',offset=0,length=1,value=body[0]>>4),dict(name='pim.type',offset=0,length=1,value=body[0]&15),dict(name='pim.cksum',offset=2,length=2,value=int.from_bytes(body[2:4],'big'))]
 if body[0]>>4!=2:return out
 if body[0]&15==4:
  for name,start,size in [('pim.fragment_tag',4,2),('pim.hash_mask_len',6,1),('pim.bsr_priority',7,1)]:
   if start+size<=len(body):out.append(dict(name=name,offset=start,length=size,value=int.from_bytes(body[start:start+size],'big')))
 return out

def main():
 p=argparse.ArgumentParser(description=__doc__)
 for n in('native','audit-modules','weber','sharon','out'):p.add_argument('--'+n,type=Path,required=True)
 a=p.parse_args();assert shutil.disk_usage(a.out.parent).free>=20*(1<<30),'bounded disk guard'
 a.out.mkdir(exist_ok=False);sys.path[:0]=[str(a.audit_modules),str(Path(__file__).parent)]
 from vxlan_endpoint_projection_audit import capture_records
 from pim_carrier_source import locate
 summary={'scope':__doc__,'auditor_SHA256':sha(Path(__file__)),'carrier_SHA256':sha(Path(__file__).with_name('pim_carrier_source.py')),'cases':{}}
 for case in('weber','sharon'):
  old=[]
  for line in(a.native/'preservation'/case/'events.jsonl').open(encoding='utf8'):
   e=json.loads(line)
   if e['protocol']=='pim':old.append(e)
  byframe=collections.defaultdict(list)
  for e in old:
   refs=e['source_bytes']['PacketRefs'];assert len(refs)==1
   byframe[refs[0]['Number']].append(e)
  found={};candidates=[];allcounts=collections.Counter();fail=[];bound=[]
  cap=getattr(a,case)
  for number,record in enumerate(capture_records(cap),1):
   record['frame']=number
   if number not in byframe:
    # The literal walker enumerates selected and explicitly marked compatibility
    # paths; a non-PIM/quoted child is never promoted by scanning bytes.
    try:loc=locate(record)
    except (AssertionError,KeyError,IndexError,ValueError):continue
    if loc['protocol']!='pim':continue
    candidates.append(dict(frame=number,location=loc,record={k:v for k,v in record.items()if k!='wire'},wire_SHA256=hashlib.sha256(record['wire']).hexdigest(),native_event_count=0))
    continue
   found[number]=True
   try:
    loc=locate(record);assert loc['protocol']=='pim','literal carrier did not select PIM'
    w=record['wire'];body=w[loc['offset']:loc['end']]
    ip=next(h for h in reversed(loc['headers'])if h['kind']in('IPv4','IPv6'))
    declared=-1 if ip['kind']=='IPv4'and ip['declared_length']==0 else ip['declared_length']-ip['length'] if ip['kind']=='IPv4'else ip['declared_payload']-(loc['offset']-ip['offset']-40)
    extent='unknown'if declared<0 else'incomplete'if len(body)<declared else'complete'
    fact=dict(frame=number,record={k:v for k,v in record.items()if k!='wire'},wire_SHA256=hashlib.sha256(w).hexdigest(),location=loc,declared_PIM_bytes=declared,available_PIM_bytes=len(body),extent=extent,capture_complete=record['captured_length']==record['original_length'],common_header_values=own_fields(body),body_hex=body.hex())
    for e in byframe[number]:
     raw=base64.b64decode(e['raw']);assert raw==body,'Raw differs from exact parent-derived body'
     assert e['source_bytes']['SHA256']==hashlib.sha256(body).hexdigest()
     domain='/mpacket:express'if record['link']==274 else''
     if len(body)>=8 and body[0]==0x21 and not(body[4]&0x40)and(checksum(body[:8],ip['source'],ip['destination'],8)==0 or checksum(body,ip['source'],ip['destination'],8)==0):domain+='/pim-register:'+'~'.join(sorted((ip['source'],ip['destination'])))
     assert e['domain']==dict(Section=record['section'],Interface=record['interface'],Encapsulation=domain),'unexpected source domain; must be independently extended'
     assert e['source_bytes']['PacketRefs']==[dict(Number=number,Domain=e['domain'])]
     assert e['source']==ip['source']and e['destination']==ip['destination']
     assert e['flow']==0 and e['transport']=='network'
     ss=e.get('session')or{}
     for v in (fact['common_header_values'] if ss else []):
      key={'pim.version':'Version','pim.type':'Type','pim.cksum':'Checksum','pim.fragment_tag':'Fragment Tag','pim.hash_mask_len':'Hash Mask Length','pim.bsr_priority':'BSR Priority'}[v['name']]
      assert ss.get(key)==v['value'],('native common scalar mismatch',key)
     item=dict(source=fact,whole_old_native=e)
     bound.append(item);allcounts[(extent,e['status'],ss.get('Type'),ss.get('Observation Scope',''))]+=1
   except (AssertionError,KeyError,IndexError,ValueError)as ex:
    fail.append(dict(frame=number,error=str(ex),event_ids=[e['id']for e in byframe[number]]))
  assert len(found)==len(byframe),'native referenced frame absent from capture'
  (a.out/(case+'-whole-source-bindings.jsonl')).write_text(''.join(json.dumps(x,ensure_ascii=False,separators=(',',':'))+'\n'for x in bound),encoding='utf8')
  dump(a.out/(case+'-unbound.json'),fail);dump(a.out/(case+'-literal-candidates-without-native.json'),candidates)
  summary['cases'][case]=dict(capture=str(cap),capture_SHA256=sha(cap),native_events=len(old),native_frames=len(byframe),bound_events=len(bound),unbound_events=sum(len(byframe[x['frame']])for x in fail),unbound_frames=len(fail),literal_candidates_without_native=len(candidates),counts=[dict(extent=k[0],status=k[1],type=k[2],scope=k[3],count=v)for k,v in sorted(allcounts.items(),key=lambda x:str(x[0]))],bindings_SHA256=sha(a.out/(case+'-whole-source-bindings.jsonl')))
 dump(a.out/'summary.json',summary);print(json.dumps({'summary':str(a.out/'summary.json'),'SHA256':sha(a.out/'summary.json'),'cases':{k:{q:v[q]for q in('native_events','bound_events','unbound_frames','literal_candidates_without_native')}for k,v in summary['cases'].items()}},ensure_ascii=False))
if __name__=='__main__':main()
