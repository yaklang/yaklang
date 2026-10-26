"""Declared IPv4 fragmentation controls from the pinned v5 sample."""
from pathlib import Path
import struct,json,subprocess,hashlib,collections,sys
import argparse
ap=argparse.ArgumentParser(description=__doc__)
ap.add_argument('--capture',required=True,type=Path,help='Single Ethernet/IPv4 packet PCAP produced by baseline-standard v5 control')
ap.add_argument('--out',required=True,type=Path,help='External scratch directory for derived/raw artifacts')
ap.add_argument('--events',required=True,type=Path)
ap.add_argument('--tshark',required=True,type=Path)
ap.add_argument('--audit',type=Path,default=Path(__file__).with_name('netflow5_wire_audit.py'))
a=ap.parse_args();out=a.out;out.mkdir(parents=True,exist_ok=True)
full=a.capture.read_bytes()[40:];assert len(full)==1458 and full[23]==17
payload=full[34:]
def checksum(b):
 if len(b)%2:b+=b'\0'
 n=sum(struct.unpack('!%dH'%(len(b)//2),b))
 while n>>16:n=(n&65535)+(n>>16)
 return (~n)&65535

def frag(start,end):
 w=bytearray(full[:34]+payload[start:end]);w[16:18]=struct.pack('!H',len(w)-14);w[18:20]=b'\x45\xa1';w[20:22]=struct.pack('!H',(start//8)|(0x2000 if end<len(payload) else 0));w[24:26]=b'\0\0';w[24:26]=struct.pack('!H',checksum(w[14:34]));return bytes(w)
f0,f1=frag(0,720),frag(720,len(payload));bad=bytearray(f0);bad[50]^=1
cases={'ordered':[f0,f1],'reversed':[f1,f0],'duplicate-first':[f0,f0,f1],'missing-tail':[f0],'conflicting-overlap':[f0,bytes(bad),f1]}
summary=[];ts=str(a.tshark)
for name,frames in cases.items():
 p=out/(name+'.pcap');w=bytearray(struct.pack('<IHHIIII',0xa1b2c3d4,2,4,0,0,65535,1))
 for i,b in enumerate(frames):w+=struct.pack('<IIII',1700000000+i,0,len(b),len(b))+b
 p.write_bytes(w);events=out/(name+'-events.jsonl')
 with events.open('wb') as f:r=subprocess.run([str(a.events),'replay',str(p),'-','--ignore-embedded-secrets'],stdout=f,stderr=subprocess.PIPE)
 assert r.returncode==0,(name,r.stderr)
 native=[json.loads(l) for l in events.read_text(encoding='utf-8-sig').splitlines()];nf=[e for e in native if e['protocol']=='netflow-v5']
 r=subprocess.run([ts,'-2','-n','-r',str(p),'-Y','cflow','-T','json','-x','--no-duplicate-keys'],capture_output=True);assert r.returncode==0;(out/(name+'-oracle.json')).write_bytes(r.stdout)
 entry={'case':name,'kind':'declared offline IPv4 fragmentation/reorder/overlap of project sample, not live capture','frames':len(frames),'sha256':hashlib.sha256(w).hexdigest(),'native_statuses':[e['status'] for e in nf],'native_refs':[e['source_bytes'] for e in nf],'native_records':[e['session']['Data Record Count'] for e in nf],'all_native_counts':dict(collections.Counter(e['protocol']+'/'+e['status'] for e in native)),'reference_cflow_frames':len(json.loads(r.stdout))}
 expected=['decoded'] if name in ['ordered','reversed','duplicate-first'] else []
 entry['expected_native_statuses']=expected;entry['matches_expected']=entry['native_statuses']==expected
 if name=='missing-tail':entry['matches_expected'] &= entry['all_native_counts'].get('ip/incomplete')==1
 if name=='conflicting-overlap':entry['matches_expected'] &= entry['all_native_counts'].get('ip/malformed')==1
 assert all(e['flow']==0 for e in nf)
 audit=out/(name+'-audit.json')
 run=subprocess.run([sys.executable,'-X','utf8',str(a.audit),str(out/(name+'-oracle.json')),str(events),str(audit),str(p)],capture_output=True)
 entry['audit_exit']=run.returncode
 if audit.exists():
  verified=json.loads(audit.read_bytes())
  for key in ['checks_total','selected_differences','original_reference_v5_record_views','unmatched_valid_reference','unselected_carrier_assessments','source_range_views']:
   entry[key]=verified[key]
 else:entry['audit_error']=run.stderr.decode(errors='replace')
 summary.append(entry);print(name,entry['native_statuses'],entry['native_records'],entry['reference_cflow_frames'],flush=True)
(out/'summary.json').write_bytes((json.dumps(summary,indent=2)+'\n').encode())
raise SystemExit(any(not x['matches_expected'] or x['audit_exit'] for x in summary))
