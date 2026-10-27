"""Compare native initial SSH fields with literal public-reference PDU views.
Captured domains and byte-contributing frames are preserved. Complete native
messages, reference views, opaque encrypted suffixes and refusal diagnostics
remain separate counts; this does not certify all encrypted SSH contents.
"""
import argparse,base64,collections,hashlib,json
from pathlib import Path
ap=argparse.ArgumentParser(description=__doc__)
for n in['events','oracle','out']:ap.add_argument('--'+n,type=Path,required=True)
a=ap.parse_args();checks=0
for p in vars(a).values():assert not any((q/'.git').exists()for q in[p.resolve(),*p.resolve().parents])
def check(ok,why):
 global checks
 checks+=1;assert ok,why
def eq(x,y,why):check(x==y,(why,x,y))
def sha(w):return hashlib.sha256(w).hexdigest()
def leaves(o,key):
 if isinstance(o,dict):
  for k,v in o.items():
   if k==key:yield from(v if isinstance(v,list)else[v])
   elif isinstance(v,(dict,list)):yield from leaves(v,key)
 elif isinstance(o,list):
  for v in o:yield from leaves(v,key)
reference=collections.defaultdict(list);reference_records=[];used_reference=set();raw_views=0;rows=json.loads(a.oracle.read_text(encoding='utf-8-sig'))
for row in rows:
 l=row['_source']['layers'];views=l.get('ssh')or[];views=views if isinstance(views,list)else[views];raws=l.get('ssh_raw')or[];raws=[raws]if raws and isinstance(raws[0],str)else raws
 for view,raw in zip(views,raws):
  w=bytes.fromhex(raw[0]);at=0
  while at<len(w):
   if w[at:].startswith(b'SSH-'):
    end=w.find(b'\n',at);n=end-at+1 if end>=at else 0
   elif len(w)-at>=16:n=4+int.from_bytes(w[at:at+4],'big')
   else:break
   if n<=0 or at+n>len(w)or not w[at:].startswith(b'SSH-')and(n<16 or n>35000 or n%8):break
   part=w[at:at+n];frame=int(l['frame']['frame.number']);digest=sha(part);reference[digest].append((frame,view));reference_records.append({'frame':frame,'raw_sha256':digest,'type':'identification'if part.startswith(b'SSH-')else str(part[5])});raw_views+=1;at+=n
names={'Kex Algorithms':'ssh.kex_algorithms','Host Key Algorithms':'ssh.server_host_key_algorithms','Offered Encryption C2S':'ssh.encryption_algorithms_client_to_server','Offered Encryption S2C':'ssh.encryption_algorithms_server_to_client','Offered MAC C2S':'ssh.mac_algorithms_client_to_server','Offered MAC S2C':'ssh.mac_algorithms_server_to_client','Offered Compression C2S':'ssh.compression_algorithms_client_to_server','Offered Compression S2C':'ssh.compression_algorithms_server_to_client'};native_count=0;matched=0;kex=0;fields=0;unavailable=[];diagnostics=collections.Counter();records=[]
for line in a.events.open(encoding='utf-8-sig'):
 e=json.loads(line)
 if e['protocol']!='ssh':continue
 s=e.get('session')or{}
 if'Packet Name'not in s:
  diagnostics[(e['status'],e.get('expert')or'')]+=1;continue
 native_count+=1;raw=base64.b64decode(e['raw']or'');refs={r['Number']for r in e['source_bytes']['PacketRefs']};views=[(frame,v)for frame,v in reference[sha(raw)]if frame in refs]
 if not views:
  unavailable.append({'PDU':e['id'],'type':s['Packet Name'],'physical_frames':sorted(refs)});continue
 matched+=1;used_reference.update((f,sha(raw))for f,v in views);records.append({'PDU':e['id'],'type':s['Packet Name'],'reference_frames':sorted({f for f,v in views}),'physical_frames':sorted(refs),'raw_sha256':sha(raw)})
 if raw.startswith(b'SSH-'):
  values=[v.rstrip('\r\n')for f,view in views for v in leaves(view,'ssh.protocol')];check(s['Identification']in values,('reference identification',e['id']));fields+=1;continue
 eq(s['Message Number'],raw[5],'literal native message code');values=[int(v)for f,view in views for v in leaves(view,'ssh.message_code')];check(raw[5]in values,('literal reference message code',e['id']));fields+=1
 if raw[5]==20:
  kex+=1
  for key,field in names.items():
   values=[v for f,view in views for v in leaves(view,field)];check(s[key]in values,('reference exact sender offer',e['id'],key));fields+=1
out={'scope':__doc__,'checks':checks,'reference_physical_rows':len(rows),'reference_complete_clear_views':raw_views,'native_complete_initial_PDUs':native_count,'native_PDUs_compared':matched,'KEXINITs_compared':kex,'field_comparisons':fields,'reference_unavailable_native_PDUs':unavailable,'reference_only_clear_views':[r for r in reference_records if(r['frame'],r['raw_sha256'])not in used_reference],'native_refusal_profiles':{str(k):v for k,v in diagnostics.items()},'compared_PDUs':records,'events_sha256':sha(a.events.read_bytes()),'oracle_sha256':sha(a.oracle.read_bytes())};a.out.parent.mkdir(parents=True,exist_ok=True);a.out.write_text(json.dumps(out,indent=2)+'\n');print(json.dumps({k:v for k,v in out.items()if k not in['scope','compared_PDUs','reference_only_clear_views']}));print('unmatched original clear reference views',len(out['reference_only_clear_views']))
