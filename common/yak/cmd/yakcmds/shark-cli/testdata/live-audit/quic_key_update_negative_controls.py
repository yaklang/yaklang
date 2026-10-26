"""Challenge independent 1-RTT verification with explicitly rebound test inputs.
Rebinding bypasses the input hashes only to exercise crypto, CID context and
native/reference PN comparisons. These are declared test controls, never new
capture evidence or replacements for the original certificates.
"""
import argparse,copy,hashlib,json,subprocess,sys
from pathlib import Path
ap=argparse.ArgumentParser(description=__doc__)
for n in['oracle','events','header-audit','initial-audit','keylog','out']:ap.add_argument('--'+n,type=Path,required=True)
a=ap.parse_args()
for p in vars(a).values():assert not any((q/'.git').exists()for q in[p.resolve(),*p.resolve().parents])
a.out.mkdir(parents=True,exist_ok=False);oracle=json.loads(a.oracle.read_text(encoding='utf-8-sig'));events=[json.loads(l)for l in a.events.open(encoding='utf-8-sig')];header=json.loads(a.header_audit.read_text(encoding='utf-8-sig'));initial=json.loads(a.initial_audit.read_text(encoding='utf-8-sig'));keys=a.keylog.read_text();audit=Path(__file__).with_name('quic_key_update_wire_audit.py')
def sha(w):return hashlib.sha256(w).hexdigest()
def short(o):
 for row in o:
  l=row['_source']['layers'];vs=l.get('quic')or[];vs=vs if isinstance(vs,list)else[vs];rs=l.get('quic_raw')or[];rs=[rs]if rs and isinstance(rs[0],str)else rs
  for v,r in zip(vs,rs):
   w=bytes.fromhex(r[0])
   if w and w[0]&0xc0==0x40 and'quic.packet_number'in(v.get('quic.short')or{}):return v,r,w
 raise AssertionError('selected short packet missing')
def flip(o,index):
 v,r,w=short(o);b=bytearray(w);b[index]^=1;r[0]=b.hex()
def pn(o):
 v,_,_=short(o);v['quic.short']['quic.packet_number']=str(int(v['quic.short']['quic.packet_number'])+1)
def native_pn(es):
 for e in es:
  if(e.get('session')or{}).get('Header Form')=='short'and e['session'].get('Authentication Verified'):e['session']['Packet Number']+=1;return
 raise AssertionError('native authenticated short missing')
def native_auth(es):
 for e in es:
  if(e.get('session')or{}).get('Header Form')=='short'and e['session'].get('Authentication Verified'):e['session']['Authentication Verified']=False;return
 raise AssertionError('native authenticated short missing')
def badkey(k):
 lines=k.splitlines()
 for i,line in enumerate(lines):
  if line.startswith('CLIENT_TRAFFIC_SECRET_0 '):
   label,random,secret=line.split();b=bytearray.fromhex(secret);b[-1]^=1;lines[i]=' '.join([label,random,b.hex()]);return'\n'.join(lines)+'\n'
 raise AssertionError('client application secret missing')
controls=[('AEAD-tag',lambda o,e,i,k:flip(o,-1)),('HP-sample',lambda o,e,i,k:flip(o,-9)),('DCID-context',lambda o,e,i,k:flip(o,1)),('key-phase-AAD',lambda o,e,i,k:flip(o,0)),('reference-PN',lambda o,e,i,k:pn(o)),('native-PN',lambda o,e,i,k:native_pn(e)),('native-authentication',lambda o,e,i,k:native_auth(e)),('wire-version-context',lambda o,e,i,k:i['observed_connections'][0].__setitem__('wire_version',1)),('receiver-CID-context',lambda o,e,i,k:i['observed_connections'][0]['receiver_CID_lengths'].__setitem__(1,7)),('traffic-secret',lambda o,e,i,k:badkey(k))]
results=[]
for name,mutate in [('baseline',None),*controls]:
 d=a.out/name;d.mkdir();o,e,h,i,k=copy.deepcopy(oracle),copy.deepcopy(events),copy.deepcopy(header),copy.deepcopy(initial),keys
 if mutate:k=mutate(o,e,i,k)or k
 op=d/'oracle.json';ep=d/'events.jsonl';hp=d/'header-explicit-test-rebinding.json';ip=d/'initial-explicit-test-rebinding.json';kp=d/'session.keys';op.write_text(json.dumps(o),encoding='utf8');ep.write_text(''.join(json.dumps(v)+'\n'for v in e),encoding='utf8');kp.write_text(k,encoding='utf8')
 for c in[h,i]:c['oracle_sha256']=sha(op.read_bytes());c['events_sha256']=sha(ep.read_bytes())
 hp.write_text(json.dumps(h),encoding='utf8');i['header_certificate_sha256']=sha(hp.read_bytes());ip.write_text(json.dumps(i),encoding='utf8')
 args=[sys.executable,'-X','utf8',str(audit),'--oracle',str(op),'--events',str(ep),'--header-audit',str(hp),'--initial-audit',str(ip),'--keylog',str(kp),'--out',str(d/'audit.json')];r=subprocess.run(args,capture_output=True,timeout=60);(d/'audit.log').write_bytes(r.stdout+r.stderr);assert(r.returncode==0)==(name=='baseline'),(name,(r.stdout+r.stderr).decode('utf8','replace')[-600:]);results.append({'control':name,'accepted':r.returncode==0})
out={'scope':__doc__,'baseline_passed':True,'rejected_corruptions':len(controls),'controls':results};(a.out/'summary.json').write_text(json.dumps(out,indent=2)+'\n',encoding='utf8');print(json.dumps(out))
