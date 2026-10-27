"""Derive declared capture faults from an actual owned loopback SSH capture.
These are replay controls, not new network-loss or endpoint-acceptance evidence.
Every retained packet is byte-identical except one explicitly shortened snaplen
record per flow. Original wire lengths and timestamps are preserved.
"""
import argparse,collections,hashlib,json,struct
from pathlib import Path
ap=argparse.ArgumentParser(description=__doc__)
for n in['capture','manifest','out']:ap.add_argument('--'+n,type=Path,required=True)
a=ap.parse_args()
for p in vars(a).values():assert not any((q/'.git').exists()for q in[p.resolve(),*p.resolve().parents])
def sha(w):return hashlib.sha256(w).hexdigest()
w=a.capture.read_bytes();assert w[:4]in[b'\xd4\xc3\xb2\xa1',b'\x4d\x3c\xb2\xa1'];head=w[:24];assert len(head)==24;link=struct.unpack_from('<I',head,20)[0];assert link in[0,1];packets=[];at=24
while at<len(w):
 assert at+16<=len(w);sec,fraction,n,wire=struct.unpack_from('<IIII',w,at);at+=16;assert 0<n<=wire and at+n<=len(w);packets.append((sec,fraction,n,wire,w[at:at+n]));at+=n
m=json.loads(a.manifest.read_text(encoding='utf-8-sig'));ops={o['server_port']:o for o in m['operations']};assert len(ops)==5 and not m['errors']
def tcp(record):
 b=record[4];ip=4 if link==0 else 14
 if link==1 and b[12:14]!=b'\x08\0':return None
 if len(b)<ip+20 or b[ip]>>4!=4 or b[ip+9]!=6:return None
 ihl=(b[ip]&15)*4;assert ihl>=20 and len(b)>=ip+ihl+20;assert int.from_bytes(b[ip+6:ip+8],'big')&0x3fff==0;t=ip+ihl;sport,dport=struct.unpack_from('!HH',b,t);port=sport if sport in ops else dport
 if port not in ops:return None
 o=ops[port];assert {sport,dport}=={port,o['captured_client_port']};n=(b[t+12]>>4)*4;assert n>=20 and t+n<=len(b);end=ip+int.from_bytes(b[ip+2:ip+4],'big');assert end<=len(b);return{'port':port,'direction':'server'if sport==port else'client','flags':b[t+13],'payload':b[t+n:end],'payload_at':t+n}
facts=[tcp(r)for r in packets];by=collections.defaultdict(list);first={}
for i,f in enumerate(facts):
 if f and f['payload']:
  first.setdefault(f['port'],(i,f));by[(f['port'],f['direction'])].append(i)
assert len(first)==5
for port,(i,f)in first.items():
 assert f['direction']=='server'and f['payload'].startswith(b'SSH-'),'actual server-first wire observation required';record=next(r for r in m['proxy_records']if r['server_port']==port);assert sha(f['payload'])==record['server_banner_sha256']
a.out.mkdir(parents=True,exist_ok=False);summary={'scope':__doc__,'source_sha256':sha(Path(__file__).read_bytes()),'capture_sha256':sha(w),'manifest_sha256':sha(a.manifest.read_bytes()),'original_physical_frames':len(packets),'actual_first_payload_server_frames':{str(k):v[0]+1 for k,v in first.items()},'variants':{}}
for name in['no-syn','out-of-order','duplicate','loss','snaplen']:
 rows=list(enumerate(packets));changes=[]
 if name=='no-syn':
  removed={i for i,f in enumerate(facts)if f and f['flags']&2};assert len(removed)==10;rows=[r for r in rows if r[0]not in removed];changes=[{'removed_original_frame':i+1,'reason':'TCP SYN role evidence removed'}for i in sorted(removed)]
 else:
  for port in ops:
   direction='server'if name=='out-of-order'else'client';candidates=[i for i in by[(port,direction)]if not facts[i]['payload'].startswith(b'SSH-')];assert len(candidates)>=2
   i=candidates[0];position=next(j for j,r in enumerate(rows)if r[0]==i)
   if name=='out-of-order':
    other=candidates[1];j=next(j for j,r in enumerate(rows)if r[0]==other);rows[position],rows[j]=rows[j],rows[position];changes.append({'server_port':port,'swapped_original_frames':[i+1,other+1]})
   elif name=='duplicate':rows.insert(position+1,rows[position]);changes.append({'server_port':port,'duplicated_original_frame':i+1})
   elif name=='loss':rows.pop(position);changes.append({'server_port':port,'removed_original_frame':i+1})
   else:
    sec,fraction,n,wire,b=rows[position][1];cut=facts[i]['payload_at']+max(1,len(facts[i]['payload'])//2);assert cut<n;rows[position]=(i,(sec,fraction,cut,wire,b[:cut]));changes.append({'server_port':port,'shortened_original_frame':i+1,'captured_before':n,'captured_after':cut,'wire_length':wire})
 raw=bytearray(head)
 for original,r in rows:raw+=struct.pack('<IIII',*r[:4])+r[4]
 path=a.out/(name+'.pcap');path.write_bytes(raw);proof={'physical_frames':len(rows),'capture_sha256':sha(raw),'changes':changes,'original_frame_by_output_frame':[i+1 for i,r in rows]};(a.out/(name+'.json')).write_text(json.dumps(proof,indent=2)+'\n',encoding='utf8');summary['variants'][name]={k:v for k,v in proof.items()if k!='original_frame_by_output_frame'}
(a.out/'summary.json').write_text(json.dumps(summary,indent=2)+'\n',encoding='utf8');print(json.dumps({k:v for k,v in summary.items()if k!='variants'}));print({k:v['physical_frames']for k,v in summary['variants'].items()})
