"""Declared offline faults derived from real capture frames, never live traffic."""
from pathlib import Path
import json,struct,hashlib,subprocess,sys,base64,collections
import argparse
ap=argparse.ArgumentParser(description=__doc__)
ap.add_argument('--oracle',required=True,type=Path,help='Original full Weber CFLOW JSON with raw packet bytes')
ap.add_argument('--out',required=True,type=Path,help='External scratch directory for derived captures and raw exports')
ap.add_argument('--events',required=True,type=Path)
ap.add_argument('--tshark',required=True,type=Path)
ap.add_argument('--audit',type=Path,default=Path(__file__).with_name('netflow9_wire_audit.py'))
a=ap.parse_args();out=a.out;out.mkdir(parents=True,exist_ok=True)
oracle=json.loads(a.oracle.read_text(encoding='utf-8-sig'))
original={int(x['_source']['layers']['frame']['frame.number']):x['_source']['layers'] for x in oracle}
def csum(w):
 if len(w)%2:w+=b'\x00'
 s=sum(struct.unpack('!%dH'%(len(w)//2),w))
 while s>>16:s=(s&65535)+(s>>16)
 return (~s)&65535 or 65535

def packet(n,seq=None,uptime=None,source=None,payload=None,port=None,flip=None,cut=None):
 l=original[n];w=bytearray.fromhex(l['frame_raw'][0]);udp=l['udp_raw'][1];at=l['cflow_raw'][1]
 assert udp==54 and at==62 and w[14]>>4==6
 body=bytearray.fromhex(l['cflow_raw'][0]) if payload is None else bytearray(payload)
 if seq is not None:body[12:16]=struct.pack('!I',seq)
 if uptime is not None:body[4:8]=struct.pack('!I',uptime)
 if source is not None:body[16:20]=struct.pack('!I',source)
 if flip is not None:body[flip]^=1
 if cut is not None:body=body[:cut]
 w=w[:at]+body
 w[18:20]=struct.pack('!H',len(w)-54)
 w[udp+4:udp+6]=struct.pack('!H',len(w)-udp);w[udp+6:udp+8]=b'\0\0'
 if port is not None:w[udp:udp+2]=struct.pack('!H',port)
 pseudo=w[22:54]+struct.pack('!I',len(w)-udp)+b'\0\0\0\x11'
 w[udp+6:udp+8]=struct.pack('!H',csum(pseudo+w[udp:]));return bytes(w)
T=lambda seq=198,**kw:packet(20548,seq=seq,**kw)
D=lambda seq=199,**kw:packet(20549,seq=seq,**kw)
# The inserted non-NetFlow message is just a declared opaque codec switch;
# Go's separate regression also exercises a fully valid MAVLink decoder claim.
other=packet(20549,payload=b'not-a-flow-exporter-datagram')
cases={
 'baseline':([T(),D(),D(200)],['decoded']*3),
 'missing-template':([D(),D(200)],['context-required']*2),
 'gap-then-fresh-template':([T(),D(200),T(201),D(202)],['decoded','context-required','decoded','decoded']),
 'reorder-then-fresh-template':([T(),D(200),D(199),T(201),D(202)],['decoded','context-required','context-required','decoded','decoded']),
 'exact-duplicate':([T(),D(),D(),D(200)],['decoded']*4),
 'conflicting-sequence-duplicate':([T(),D(),D(flip=24),D(200),T(201),D(202)],['decoded','decoded','context-required','context-required','decoded','decoded']),
 'tuple-codec-switch':([T(),other,D(),T(200),D(201)],['decoded','context-required','decoded','decoded']),
 'malformed-template-then-recover':([T(),D(),T(200,cut=30),D(201),T(202),D(203)],['decoded','decoded','malformed','context-required','decoded','decoded']),
 'source-id-isolation':([T(),D(source=2),D(),D(200)],['decoded','context-required','decoded','decoded']),
 'source-port-isolation':([T(),D(port=53068),D(),D(200)],['decoded','context-required','decoded','decoded']),
 'sequence-wrap':([T(0xffffffff),D(0),D(1)],['decoded']*3),
 'uptime-wrap':([T(uptime=0xfffffffe),D(uptime=2),D(200,uptime=3)],['decoded']*3),
 'uptime-regression-recover':([T(uptime=1000),D(uptime=999),T(200,uptime=1001),D(201,uptime=1002)],['decoded','context-required','decoded','decoded']),
 'capture-truncation-recover':([T(),(T(199)[:100],len(T(199))),D(),T(200),D(201)],['decoded','context-required','decoded','decoded']),
 'lease-expiry-recover':([T(),D(),T(200),D(201)],['decoded','context-required','decoded','decoded']),
}
manifest=[]
ts=str(a.tshark)
for name,(frames,expected) in cases.items():
 p=out/(name+'.pcap');data=bytearray(struct.pack('<IHHIIII',0xa1b2c3d4,2,4,0,0,65535,1))
 for i,item in enumerate(frames):
  w,wirelen=item if isinstance(item,tuple) else (item,len(item))
  second=1700000000+i+(301 if name=='lease-expiry-recover' and i else 0)
  data+=struct.pack('<IIII',second,0,len(w),wirelen)+w
 p.write_bytes(data)
 events=out/(name+'-events.jsonl')
 with events.open('wb') as f:
  r=subprocess.run([str(a.events),'replay',str(p),'-','--ignore-embedded-secrets'],stdout=f,stderr=subprocess.PIPE)
 assert r.returncode==0,(name,r.stderr)
 native=[json.loads(l) for l in events.read_text(encoding='utf-8-sig').splitlines()]
 nf=[e for e in native if e['protocol']=='netflow-v9'];statuses=[e['status'] for e in nf]
 # TShark keeps its original heuristics and best-known-template policy.
 ref=out/(name+'-oracle.json');r=subprocess.run([ts,'-2','-n','-r',str(p),'-Y','cflow','-T','json','-x','--no-duplicate-keys'],capture_output=True)
 assert r.returncode==0,(name,r.stderr);ref.write_bytes(r.stdout)
 records=json.loads(r.stdout);details=[]
 def count(node):
  import re
  if isinstance(node,dict):return sum((1 if re.match(r'^(?:Options? )?Flow\s+\d+\b',k) else 0)+count(v) for k,v in node.items())
  if isinstance(node,list):return sum(map(count,node))
  return 0
 for row in records:
  l=row['_source']['layers'];details.append({'frame':int(l['frame']['frame.number']),'reference_data_records':count(l.get('cflow')),'reference_version':l.get('cflow',{}).get('cflow.version')})
 entry={'case':name,'kind':'declared offline mutation/reordering of original real packet bytes','original_source_frames':[20548,20549], 'sha256':hashlib.sha256(data).hexdigest(),'frames':len(frames),'native_statuses':statuses,'expected_native_statuses':expected,'matches_expected':statuses==expected,'native_frames':[(e['source_bytes'].get('CompletedIn') or (e['source_bytes'].get('PacketRefs') or [{}])[-1]).get('Number') for e in nf],'native_records':[e['session'].get('Data Record Count') for e in nf],'reference_frames':details,'native_all_protocols':dict(collections.Counter(e['protocol'] for e in native))}
 audit=out/(name+'-audit.json')
 check=subprocess.run([sys.executable,'-X','utf8',str(a.audit),str(ref),str(events),str(audit)],capture_output=True)
 entry['audit_exit']=check.returncode
 if audit.exists():
  detail=json.loads(audit.read_bytes())
  for key in ['checks_total','selected_differences','contextual_refusals','reference_fields_not_projected']:
   entry[key]=detail[key]
 else: entry['audit_error']=check.stderr.decode(errors='replace')
 manifest.append(entry);print(name,statuses,'PASS' if statuses==expected and check.returncode==0 else 'FAIL',flush=True)
(out/'summary.json').write_bytes((json.dumps(manifest,indent=2)+'\n').encode())
raise SystemExit(any(not x['matches_expected'] or x['audit_exit'] for x in manifest))