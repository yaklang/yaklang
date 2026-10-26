"""Characterize unchanged original reference v10 views before native admission.
This establishes necessary header/Set bounds only, not complete IPFIX semantics.
"""
import argparse,collections,json
from pathlib import Path
ap=argparse.ArgumentParser(description=__doc__)
ap.add_argument('--oracle',required=True,type=Path);ap.add_argument('--out',required=True,type=Path);a=ap.parse_args()
def val(x,key):
 if isinstance(x,dict):
  for k,v in x.items():
   if k==key:return v
   z=val(v,key)
   if z is not None:return z
 if isinstance(x,list):
  for v in x:
   z=val(v,key)
   if z is not None:return z
counts=collections.Counter();rows=[]
for packet in json.loads(a.oracle.read_text(encoding='utf-8-sig')):
 l=packet['_source']['layers']
 if val(l,'cflow.version')!='10':continue
 raw=val(l,'cflow_raw');assert isinstance(raw,list) and isinstance(raw[0],str),'unselected multi-view carrier needs explicit handling'
 w=bytes.fromhex(raw[0]);reason='complete header/sets';transport='udp' if 'udp' in l else 'tcp' if 'tcp' in l else 'other'
 if len(w)<16:reason='short header'
 elif int.from_bytes(w[2:4],'big')!=len(w):reason='message length differs'
 else:
  at=16
  while at<len(w):
   n=int.from_bytes(w[at+2:at+4],'big')
   if len(w)-at<4 or n<4 or at+n>len(w):reason='invalid Set bound';break
   at+=n
 counts[transport+': '+reason]+=1
 rows.append({'frame':val(l,'frame.number'),'transport':transport,'reason':reason,'wire_bytes':len(w),'declared_length':int.from_bytes(w[2:4],'big') if len(w)>=4 else None})
out={'scope':__doc__,'original_v10_reference_views':len(rows),'complete_header_and_sets':sum(x['reason']=='complete header/sets' for x in rows),'categories':dict(counts),'views':rows}
a.out.parent.mkdir(parents=True,exist_ok=True);a.out.write_text(json.dumps(out,indent=2)+'\n',encoding='utf8');print({k:v for k,v in out.items() if k!='views'})
