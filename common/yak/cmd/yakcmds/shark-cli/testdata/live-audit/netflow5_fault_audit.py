from pathlib import Path
import json,struct,hashlib,subprocess,sys,collections
import argparse
ap=argparse.ArgumentParser(description='Declared offline NetFlow v5 faults from a pinned upstream capture; not fresh live traffic.')
ap.add_argument('--oracle',required=True,type=Path,help='Pinned Akvorado supplemental Decode-As JSON with packet raw bytes')
ap.add_argument('--out',required=True,type=Path,help='External scratch directory for derived/raw artifacts')
ap.add_argument('--events',required=True,type=Path)
ap.add_argument('--tshark',required=True,type=Path)
ap.add_argument('--audit',type=Path,default=Path(__file__).with_name('netflow5_wire_audit.py'))
a=ap.parse_args();out=a.out;out.mkdir(parents=True,exist_ok=True)
l=json.loads(a.oracle.read_text(encoding='utf-8-sig'))[0]['_source']['layers'];original=bytes.fromhex(l['frame_raw'][0]);base=bytes.fromhex(l['cflow_raw'][0]);ipoff=l['ip_raw'][1];udpoff=l['udp_raw'][1];assert ipoff==14 and udpoff==34
sha=lambda b:hashlib.sha256(b).hexdigest()
def checksum(w):
 if len(w)%2:w+=b'\0'
 s=sum(struct.unpack('!%dH'%(len(w)//2),w))
 while s>>16:s=(s&65535)+(s>>16)
 return (~s)&65535

def packet(body,port=2055):
 w=bytearray(original[:42]+body);w[16:18]=struct.pack('!H',len(w)-14);w[24:26]=b'\0\0';w[24:26]=struct.pack('!H',checksum(w[14:34]))
 w[36:38]=struct.pack('!H',port);w[38:40]=struct.pack('!H',8+len(body));w[40:42]=b'\0\0'
 pseudo=w[26:34]+b'\0\x11'+struct.pack('!H',8+len(body));w[40:42]=struct.pack('!H',checksum(pseudo+w[34:]) or 65535)
 return bytes(w)
def changed(off,w):
 b=bytearray(base);b[off:off+len(w)]=w;return bytes(b)
max30=bytearray(base+base[24:72]);max30[2:4]=struct.pack('!H',30)
seqs=[changed(16,struct.pack('!I',v)) for v in [100,800,2,0xffffffff,0]]
cases={
 'baseline-standard':([base],['decoded']),
 'nondefault-explicit':([base],['decoded']),
 'sequence-gap-reorder-wrap':(seqs,['decoded']*5),
 'exact-duplicate':([base,base],['decoded']*2),
 'engine-reuse':([base,changed(20,b'\xff\xff')],['decoded']*2),
 'thirty-records':([bytes(max30)],['decoded']),
 'mask-warning':([changed(24+44,b'\xff')],['decoded']),
 'unused-padding-warning':([changed(24+36,b'\x55')],['decoded']),
 'sampling-mode-warning':([changed(22,b'\xc0\x09')],['decoded']),
 'no-record-header':([base[:24]],['malformed']),
 'two-field-prefix':([base[:4]],['malformed']),
 'count-zero':([changed(2,b'\0\0')],['malformed']),
 'count-thirty-one':([changed(2,b'\0\x1f')],['malformed']),
 'count-body-disagree':([changed(2,b'\0\x1c')],['malformed']),
 'nanoseconds-overflow':([changed(12,struct.pack('!I',1000000000))],['malformed']),
 'last-record-truncated':([base[:-1]],['malformed']),
 'extra-trailing-byte':([base+b'\0'],['malformed']),
 'capture-truncation':([base],[]),
}
summary=[];ts=str(a.tshark)
for name,(bodies,expected) in cases.items():
 port=9990 if name=='nondefault-explicit' else 2055
 p=out/(name+'.pcap');data=bytearray(struct.pack('<IHHIIII',0xa1b2c3d4,2,4,0,0,65535,1))
 for i,body in enumerate(bodies):
  w=packet(body,port);wirelen=len(w)
  if name=='capture-truncation':w=w[:-1]
  data+=struct.pack('<IIII',1700000000+i,0,len(w),wirelen)+w
 p.write_bytes(data)
 def sdk(suffix,forced):
  f=out/(name+suffix+'-events.jsonl');args=[str(a.events),'replay',str(p),'-','--ignore-embedded-secrets']
  if forced:args+=['udp:'+str(port)+':netflow-v5']
  with f.open('wb') as dst:r=subprocess.run(args,stdout=dst,stderr=subprocess.PIPE)
  assert r.returncode==0,(name,r.stderr)
  return f,[json.loads(l) for l in f.read_text(encoding='utf-8-sig').splitlines()]
 events,native=sdk('-explicit',True);nf=[x for x in native if x['protocol']=='netflow-v5'];statuses=[x['status'] for x in nf]
 default,automatic=sdk('-default',False);auto=[x for x in automatic if x['protocol']=='netflow-v5']
 other=[x for x in automatic if x.get('protocol') not in ['', 'netflow-v5'] and x['status']=='decoded']
 assert not other,(name,other)
 expected_auto=['malformed'] if name=='nanoseconds-overflow' else [] if name=='nondefault-explicit' or expected!=['decoded']*len(expected) else expected
 ref=out/(name+'-oracle.json');r=subprocess.run([ts,'-2','-n','-r',str(p),'-d','udp.port=='+str(port)+',cflow','-Y','cflow','-T','json','-x','--no-duplicate-keys'],capture_output=True);assert r.returncode==0;ref.write_bytes(r.stdout)
 entry={'case':name,'kind':'declared offline modification/repetition of pinned Akvorado project packet, not fresh live traffic','frames':len(bodies),'sha256':sha(data),'explicit_statuses':statuses,'expected_explicit_statuses':expected,'default_statuses':[x['status'] for x in auto],'expected_default_statuses':expected_auto,'matches_expected':statuses==expected and [x['status'] for x in auto]==expected_auto,'reference_frames':len(json.loads(r.stdout)),'default_all_statuses':dict(collections.Counter(x.get('protocol','')+'/'+x['status'] for x in automatic)),'explicit_all_statuses':dict(collections.Counter(x.get('protocol','')+'/'+x['status'] for x in native)),'no_other_automatic_decoded_protocols':not other,'decoded_records':sum((x.get('session') or {}).get('Data Record Count',0) for x in nf if x['status']=='decoded'),'malformed_retained_records':sum((x.get('session') or {}).get('Data Record Count',0) for x in nf if x['status']=='malformed'),'records':sum((x.get('session') or {}).get('Data Record Count',0) for x in nf)}
 audit=out/(name+'-audit.json')
 check=subprocess.run([sys.executable,'-X','utf8',str(a.audit),str(ref),str(events),str(audit)],capture_output=True)
 entry['audit_exit']=check.returncode
 if audit.exists():
  verified=json.loads(audit.read_bytes())
  for key in ['checks_total','selected_differences','additional_native','unmatched_valid_reference','reference_fields_not_projected']:
   entry[key]=verified[key]
 else:entry['audit_error']=check.stderr.decode(errors='replace')
 summary.append(entry);print(name,statuses,[x['status'] for x in auto],'PASS' if entry['matches_expected'] and check.returncode==0 else 'FAIL',flush=True)
(out/'summary.json').write_bytes((json.dumps(summary,indent=2)+'\n').encode())
raise SystemExit(any(not x['matches_expected'] or x['audit_exit'] for x in summary))