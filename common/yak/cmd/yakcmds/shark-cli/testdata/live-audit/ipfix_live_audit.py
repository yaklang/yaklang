"""Audit the ipfix_live_windows owned socket experiment against unmodified TShark.
Independently rebuild TCP bytes/contributors; retain receiver-policy differences.
"""
import argparse,base64,collections,hashlib,ipaddress,json,struct,re
from pathlib import Path
ap=argparse.ArgumentParser(description=__doc__);ap.add_argument('--dir',required=True,type=Path);ap.add_argument('--events',type=Path);ap.add_argument('--out',type=Path);a=ap.parse_args();d=a.dir
oracle=json.loads((d/'oracle.json').read_text(encoding='utf-8-sig'));events=[json.loads(s) for s in (a.events or d/'events.jsonl').read_text(encoding='utf-8-sig').splitlines()];generated=json.loads((d/'summary.json').read_text());checks=0
def eq(x,y,name):
 global checks
 checks+=1;assert x==y,(name,x,y)
def values(x,key):
 if isinstance(x,dict):return [z for k,v in x.items() for z in ((v if isinstance(v,list) else [v]) if k==key else values(v,key))]
 if isinstance(x,list):return [z for v in x for z in values(v,key)]
 return []
def scalar(x,key):v=values(x,key);assert len(v)==1,(key,v);return v[0]
def digest(w):return hashlib.sha256(w).hexdigest()
reference={};tcp=collections.defaultdict(list);udp=[]
for packet in oracle:
 l=packet['_source']['layers'];frame=int(l['frame']['frame.number'])
 if 'tcp' in l:
  t=l['tcp'];key=(scalar(l,'ip.src'),int(t['tcp.srcport']),scalar(l,'ip.dst'),int(t['tcp.dstport']))
  raw=values(t,'tcp.payload_raw')
  if raw:
   assert isinstance(raw[0],str);payload=bytes.fromhex(raw[0]);tcp[key].append((int(t['tcp.seq_raw']),payload,frame))
 else:
  key=(scalar(l,'ip.src'),int(l['udp']['udp.srcport']),scalar(l,'ip.dst'),int(l['udp']['udp.dstport']))
 if 'cflow' not in l:continue
 cs=l['cflow'] if isinstance(l['cflow'],list) else [l['cflow']];raws=l['cflow_raw'] if isinstance(l['cflow_raw'][0],list) else [l['cflow_raw']]
 eq(len(cs),len(raws),'reference messages/raw spans')
 for c,raw in zip(cs,raws):
  w=bytes.fromhex(raw[0]);h=digest(w);reference.setdefault((key,h),[]).append((frame,c,w))
  if 'tcp' not in l:udp.append(h)
eq(udp,generated['UDP_received_message_hashes'],'all captured UDP equals collector received bytes')
independent={};tcp_hashes=[]
for key,chunks in tcp.items():
 if key[3]!=4739:continue
 start=min(x[0] for x in chunks);cells={}
 for seq,b,frame in chunks:
  for i,c in enumerate(b):
   if seq+i in cells:eq(cells[seq+i],c,'TCP overlap bytes')
   cells[seq+i]=c
 end=max(cells)+1;eq(len(cells),end-start,'no TCP holes');w=bytes(cells[i] for i in range(start,end));at=0;hashes=[]
 while at<len(w):
  version,n=struct.unpack_from('!HH',w,at);eq(version,10,'independent TCP version');assert n>=16 and at+n<=len(w);message=w[at:at+n]
  refs=sorted({frame for seq,b,frame in chunks if seq<start+at+n and seq+len(b)>start+at});h=digest(message);independent.setdefault((key,h),[]).append(refs);hashes.append(h);at+=n
 tcp_hashes.append(hashes)
eq(tcp_hashes,generated['TCP_received_message_hashes'],'all reconstructed TCP PDUs equal collector')
flow_keys=collections.defaultdict(set);differences=[];records=0;reference_records=0;udp_status=[]
for e in events:
 eq(e['protocol'],'ipfix','native protocol');w=base64.b64decode(e['raw']);h=digest(w)
 src,sp=e['source'].rsplit(':',1);dst,dp=e['destination'].rsplit(':',1);key=(src,int(sp),dst,int(dp));queue=reference.get((key,h),[]);assert queue,'missing exact reference PDU';frame,c,rw=queue.pop(0);eq(w,rw,'native/reference exact PDU')
 m=e['fields'];eq(e['source_bytes']['SHA256'],h,'native source hash');eq(m['Version'],10,'native version');eq(m['Message Length'],len(w),'native message length');eq(m['Export Time'],int.from_bytes(w[4:8],'big'),'export time');eq(m['Sequence Number'],int.from_bytes(w[8:12],'big'),'sequence');eq(m['Observation Domain ID'],19,'Observation Domain')
 if e['transport']=='tcp':
  refs=independent[(key,h)].pop(0);eq([r['Number'] for r in e['source_bytes']['PacketRefs']],refs,'exact physical TCP contributors');flow_keys[key].add(e['flow']);assert e['flow']>0
 else:
  eq(e['flow'],0,'UDP reported flows are not TCP IDs');eq([r['Number'] for r in e['source_bytes']['PacketRefs']],[frame],'physical UDP frame');udp_status.append(e['status'])
 ref_count=len(values(c,'cflow.srcaddr'));reference_records+=ref_count;records+=m['Data Record Count']
 if e['status']=='decoded':
  eq(m['Data Record Count'],ref_count,'reference/native records')
  for s in m['FlowSets']:
   for r in s.get('Records',[]):
    fs=r['Fields'];eq(len(fs),3,'authored schema fields')
    for f in fs:
     p,q=f['Wire Field Ranges']['Value'];b=w[p:q];expected=str(ipaddress.ip_address(b)) if f['Field Type']==8 else int.from_bytes(b,'big');eq(f['Value'],expected,'literal field bytes')
  eq([v['value'] for v in e['field_values']['values'] if v['name']=='cflow.srcaddr'],values(c,'cflow.srcaddr'),'reported address')
  eq([v['value'] for v in e['field_values']['values'] if v['name']=='cflow.packets'],[int(x) for x in values(c,'cflow.packets')],'reported packets')
 else:
  eq(e['status'],'context-required','refusal kind');eq(m['Data Record Count'],0,'unknown schema no data records');assert not any(v['name'] in ['cflow.srcaddr','cflow.packets'] for v in (e['field_values'].get('values') or []))
 if ref_count!=m['Data Record Count']:differences.append({'frame':frame,'transport':e['transport'],'reference_records':ref_count,'native_records':m['Data Record Count'],'native_status':e['status'],'cause':m.get('Context Reset','template absent in this transport generation')})
eq(udp_status,['decoded','decoded','context-required','decoded','decoded','context-required','context-required','decoded','decoded'],'owned proxy loss/reorder/recovery timeline')
eq(len(flow_keys),3,'TCP connections');eq(len({next(iter(ids)) for ids in flow_keys.values()}),3,'distinct captured generation IDs');eq(sum(map(len,reference.values())),0,'all reference messages matched');eq(sum(map(len,independent.values())),0,'all independently rebuilt PDUs matched')
stats=re.search(r'Packets received/dropped on interface .*?:\s*(\d+)/(\d+)',(d/'dumpcap.log').read_text(errors='replace'));assert stats,'capture statistics absent'
eq(int(stats[1]),len(oracle),'captured physical packets');eq(int(stats[2]),0,'Npcap capture drops')
out={'scope':__doc__,'physical_frames':len(oracle),'messages':len(events),'TCP_connections':3,'native_data_records':records,'reference_data_records':reference_records,'checks':checks,'proxy':generated['proxy'],'capture_drops':0,'receiver_policy_differences_retained':differences}
(a.out or d/'audit-summary.json').write_text(json.dumps(out,indent=2)+'\n',encoding='utf8');print(json.dumps(out))
