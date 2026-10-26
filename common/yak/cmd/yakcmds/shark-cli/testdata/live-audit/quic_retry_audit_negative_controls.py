"""Exercise independent Retry/Initial crypto checks with explicitly rebound inputs.
These mutations bypass only the prior header-certificate hash binding so the
separate crypto/transcript checks are exercised. Rebound certificates are test
inputs, never replacement certificates for the original physical capture.
"""
import argparse,copy,hashlib,json,subprocess,sys
from pathlib import Path
from cryptography.hazmat.primitives.ciphers.aead import AESGCM
ap=argparse.ArgumentParser(description=__doc__)
for n in ['oracle','events','header-audit','out']:ap.add_argument('--'+n,type=Path,required=True)
ap.add_argument('--expect-retries',type=int,required=True)
a=ap.parse_args()
for p in [a.oracle,a.events,a.header_audit,a.out]:assert not any((q/'.git').exists()for q in[p.resolve(),*p.resolve().parents])
a.out.mkdir(parents=True,exist_ok=False);oracle=json.loads(a.oracle.read_text(encoding='utf-8-sig'));events=[json.loads(l)for l in a.events.open(encoding='utf-8-sig')];certificate=json.loads(a.header_audit.read_text(encoding='utf-8-sig'));audit=Path(__file__).with_name('quic_retry_initial_audit.py')
def sha(w):return hashlib.sha256(w).hexdigest()
def packet(rows,typ):
 for row in rows:
  l=row['_source']['layers'];raw=l.get('quic_raw')or[];raw=[raw]if raw and isinstance(raw[0],str)else raw;views=l.get('quic')or[];views=views if isinstance(views,list)else[views]
  for v,r in zip(views,raw):
   w=bytes.fromhex(r[0])
   if len(w)>=7 and w[0]&0xc0==0xc0 and w[1:5]==b'\0\0\0\1'and(w[0]>>4&3)==typ:return l,v,r,w
 raise AssertionError('missing selected packet')
def wire_flip(rows,typ,index):
 _,v,r,w=packet(rows,typ);b=bytearray(w);b[index]^=1;r[0]=b.hex()
def ref_pn(rows):_,v,_,_=packet(rows,0);v['quic.packet_number']=str(int(v['quic.packet_number'])+1)
def ref_crypto(rows):
 _,v,_,_=packet(rows,0);fs=v['quic.frame'];fs=fs if isinstance(fs,list)else[fs];f=next(f for f in fs if int(f['quic.frame_type'],0)==6);w=bytearray.fromhex(f['quic.crypto.crypto_data_raw'][0]);w[6]^=1;f['quic.crypto.crypto_data_raw'][0]=w.hex()
def wrong_retry_source(rows):
 _,_,_,first=packet(rows,0);original=first[6:6+first[5]];_,_,r,w=packet(rows,3);dl=w[5];off=6+dl;sl=w[off];body=w[:off]+bytes([len(original)])+original+w[off+1+sl:-16];pseudo=bytes([len(original)])+original+body;tag=AESGCM(bytes.fromhex('be0c690b9f66575a1d766b54e368c84e')).encrypt(bytes.fromhex('461599d35d632bf2239825bb'),b'',pseudo);r[0]=(body+tag).hex();r[2]=len(body)+16
def strip_refused(es):
 for e in es:
  if'conflicting retransmission'in e.get('error',''):e.pop('error')
def remove_post_initial(rows):
 retry_seen=False
 for row in rows:
  l=row['_source']['layers'];vs=l.get('quic')or[];vs=vs if isinstance(vs,list)else[vs];raw=l.get('quic_raw')or[];raw=[raw]if raw and isinstance(raw[0],str)else raw
  for v,r in zip(vs,raw):
   w=bytes.fromhex(r[0])
   if len(w)>=7 and w[1:5]==b'\0\0\0\1'and(w[0]>>4&3)==3:retry_seen=True
   elif retry_seen and len(w)>=7 and w[0]&0xc0==0xc0 and(w[0]>>4&3)==0 and w[1:5]==b'\0\0\0\1':r[0]='00'*len(w)

controls=[('Initial-ciphertext',lambda o,e,c:wire_flip(o,0,-1)),('Initial-HP-sample',lambda o,e,c:wire_flip(o,0,-9)),('Initial-DCID-AAD',lambda o,e,c:wire_flip(o,0,6)),('Initial-SCID-AAD',lambda o,e,c:wire_flip(o,0,15)),('Retry-integrity',lambda o,e,c:wire_flip(o,3,-1)),('Retry-original-CID-reuse-valid-tag',lambda o,e,c:wrong_retry_source(o)),('reference-decoded-PN',lambda o,e,c:ref_pn(o)),('reference-CRYPTO-bytes',lambda o,e,c:ref_crypto(o)),('native-conflict-diagnostic',lambda o,e,c:strip_refused(e)),('missing-post-Retry-transcript',lambda o,e,c:remove_post_initial(o)),('header-certificate-digest',lambda o,e,c:c.__setitem__('oracle_sha256','00'*32))]
results=[]
def run(name,o,e,c,must_pass=False):
 d=a.out/name;d.mkdir();ep=d/'events.jsonl';op=d/'oracle.json';cp=d/'header-certificate-explicit-test-rebinding.json';ep.write_text(''.join(json.dumps(v)+'\n'for v in e),encoding='utf8');op.write_text(json.dumps(o),encoding='utf8')
 if name!='header-certificate-digest':c['oracle_sha256']=sha(op.read_bytes())
 c['events_sha256']=sha(ep.read_bytes());cp.write_text(json.dumps(c),encoding='utf8')
 args=[sys.executable,'-X','utf8',str(audit),'--oracle',str(op),'--events',str(ep),'--header-audit',str(cp),'--expect-retries',str(a.expect_retries),'--out',str(d/'audit.json')];r=subprocess.run(args,capture_output=True,timeout=60);(d/'audit.log').write_bytes(r.stdout+r.stderr);assert(r.returncode==0)==must_pass,(name,(r.stdout+r.stderr).decode('utf8','replace')[-1000:]);results.append({'control':name,'accepted':r.returncode==0})
run('baseline',copy.deepcopy(oracle),copy.deepcopy(events),copy.deepcopy(certificate),True)
for name,change in controls:
 o,e,c=copy.deepcopy(oracle),copy.deepcopy(events),copy.deepcopy(certificate);change(o,e,c);run(name,o,e,c)
out={'scope':__doc__,'baseline_passed':True,'rejected_corruptions':len(controls),'controls':results};(a.out/'summary.json').write_text(json.dumps(out,indent=2)+'\n',encoding='utf8');print(json.dumps(out))
