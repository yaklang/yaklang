"""Declared offline RFC7011 UDP controls. These packets are not live captures.
Reference best-known-schema decoding and native quarantine remain distinct.
"""
import argparse,base64,hashlib,json,struct,subprocess
from pathlib import Path
ap=argparse.ArgumentParser(description=__doc__)
for n in ['events','tshark','out']:ap.add_argument('--'+n,required=True,type=Path)
a=ap.parse_args();a.out.mkdir(parents=True,exist_ok=True)
assert not any((p/'.git').exists() for p in [a.out.resolve(),*a.out.resolve().parents])
def msg(seq,*sets):
 w=struct.pack('!HHIII',10,16+sum(map(len,sets)),1700000000,seq,19)+b''.join(sets);return w
def st(i,b):return struct.pack('!HH',i,len(b)+4)+b
template=st(2,bytes.fromhex('010000020008000400020004'));changed=st(2,bytes.fromhex('01000002000c000400020004'));data=st(256,bytes.fromhex('c000020900000011'));withdraw=st(2,bytes.fromhex('01000000'))
def cs(w):
 if len(w)%2:w+=b'\0'
 n=sum(struct.unpack('!%dH'%(len(w)//2),w))
 while n>>16:n=(n&65535)+(n>>16)
 return (~n)&65535
def packet(payload,udp_port=4739):
 src,dst=bytes.fromhex('c0000201'),bytes.fromhex('c6336402');udp=struct.pack('!HHHH',52000,udp_port,len(payload)+8,0)+payload
 checksum=cs(src+dst+struct.pack('!BBH',0,17,len(udp))+udp) or 65535;udp=udp[:6]+struct.pack('!H',checksum)+udp[8:]
 ip=bytes.fromhex('4500')+struct.pack('!H',20+len(udp))+bytes.fromhex('005d000040110000')+src+dst
 return ip[:10]+struct.pack('!H',cs(ip))+ip[12:]+udp
def capture(rows):
 w=struct.pack('<IHHIIII',0xa1b2c3d4,2,4,0,0,65535,101)
 for seconds,payload in rows:
  b=packet(payload);w+=struct.pack('<IIII',1700000000+seconds,0,len(b),len(b))+b
 return w
def vals(x,key):
 if isinstance(x,dict):return [z for k,v in x.items() for z in ((v if isinstance(v,list) else [v]) if k==key else vals(v,key))]
 if isinstance(x,list):return [z for v in x for z in vals(v,key)]
 return []
cases={
 'empty-template-Set':([(1,msg(0,st(2,b'')))],['malformed'],[0]),
 'empty-options-Set':([(1,msg(0,st(3,b'')))],['malformed'],[0]),
 'empty-data-Set':([(1,msg(0,st(256,b'')))],['malformed'],[0]),
 'padding-without-record':([(1,msg(0,template,st(256,b'\0\0\0')))],['malformed'],[0]),
 'template-same-sequence':([(1,msg(603,template)),(2,msg(603,data))],['decoded','decoded'],[0,1]),
 'duplicate-data':([(1,msg(603,template)),(2,msg(603,data)),(3,msg(603,data)),(4,msg(604,data))],['decoded']*4,[0,1,1,1]),
 'loss-and-template-recovery':([(1,msg(603,template)),(2,msg(605,data)),(3,msg(605,template,data)),(4,msg(606,data))],['decoded','context-required','decoded','decoded'],[0,0,1,1]),
 'reorder-and-recovery':([(1,msg(603,template)),(2,msg(603,data)),(3,msg(602,data)),(4,msg(603,template,data))],['decoded','decoded','context-required','decoded'],[0,1,0,1]),
 'lease-expiry':([(1,msg(603,template)),(302,msg(603,data)),(303,msg(603,template,data))],['decoded','context-required','decoded'],[0,0,1]),
 'UDP-withdrawal-ignored':([(1,msg(603,template)),(2,msg(603,withdraw,data))],['decoded']*2,[0,1]),
 'UDP-definition-replacement':([(1,msg(603,template)),(2,msg(603,changed,data))],['decoded']*2,[0,1]),
 'future-template-not-retroactive':([(1,msg(603,data,template)),(2,msg(603,template,data))],['context-required','decoded'],[0,1]),
 'short-template':([(1,msg(603,st(2,bytes.fromhex('0100000180080004'))))],['malformed'],[0]),
 'binary-enterprise-collision':([(1,msg(603,st(2,bytes.fromhex('010000018008000400000000')),st(256,bytes.fromhex('c0000209'))))],['decoded'],[1]),
 'tuple-reuse':([(1,msg(603,template)),(2,b'unrelated message'),(3,msg(603,data))],['decoded','malformed','context-required'],[0,0,0]),
 'sequence-wrap':([(1,msg(0xffffffff,template)),(2,msg(0xffffffff,data)),(3,msg(0,data))],['decoded']*3,[0,1,1]),
}
summary=[];checks=0
for name,(rows,statuses,counts) in cases.items():
 cap=a.out/(name+'.pcap');cap.write_bytes(capture(rows));ev=a.out/(name+'.jsonl')
 with ev.open('wb') as f:r=subprocess.run([str(a.events),'replay',str(cap),'-','--ignore-embedded-secrets','udp:4739:ipfix'],stdout=f,stderr=subprocess.PIPE)
 assert r.returncode==0,r.stderr;events=[json.loads(s) for s in ev.read_text(encoding='utf-8-sig').splitlines()]
 assert len(events)==len(rows),(name,len(events),len(rows))
 for i,(e,(seconds,payload),status,count) in enumerate(zip(events,rows,statuses,counts)):
  assert e['protocol']=='ipfix' and e['status']==status,(name,i,e['protocol'],e['status'],status)
  assert e['flow']==0 and base64.b64decode(e['raw'])==payload
  assert (e['fields'] or {}).get('Data Record Count',0)==count,(name,i,e['fields'])
  assert e['source_bytes']['PacketRefs'][0]['Number']==i+1
  if status=='decoded':assert e['source_bytes']['CompletedIn']['Number']==i+1
  else:assert 'CompletedIn' not in e['source_bytes'] # API publishes completion only on admitted successful messages.
  checks+=6
  if status!='decoded':assert not any(v['name'] in ['cflow.srcaddr','cflow.dstaddr','cflow.packets'] for v in (e['field_values'].get('values') or []));checks+=1
 r=subprocess.run([str(a.tshark),'-2','-n','-r',str(cap),'-d','udp.port==4739,cflow','-T','json','-x','-J','frame ip udp cflow','--no-duplicate-keys'],capture_output=True);assert r.returncode==0,r.stderr
 ref=json.loads(r.stdout);(a.out/(name+'-oracle.json')).write_bytes(r.stdout)
 reference_counts=[sum(1 for x in vals(p,'cflow.srcaddr'))+sum(1 for x in vals(p,'cflow.dstaddr')) for p in ref]
 # Count one address per our selected record; pure enterprise IE8 does not
 # acquire a standard srcaddr alias. Preserve all actual reference differences.
 native_alias_counts=[sum(v['name'] in ['cflow.srcaddr','cflow.dstaddr'] for v in (e['field_values'].get('values') or [])) for e in events]
 differences=[{'frame':i+1,'reference_address_values':r,'native_address_values':n,'native_status':statuses[i]} for i,(r,n) in enumerate(zip(reference_counts,native_alias_counts)) if r!=n]
 summary.append({'case':name,'kind':'declared offline control','frames':len(rows),'sha256':hashlib.sha256(cap.read_bytes()).hexdigest(),'statuses':statuses,'native_data_records':counts,'reference_scalar_differences_retained':differences});print(name,'PASS',flush=True)
out={'scope':__doc__,'controls':len(summary),'frames':sum(x['frames'] for x in summary),'checks':checks,'cases':summary}
(a.out/'summary.json').write_text(json.dumps(out,indent=2)+'\n',encoding='utf8')
