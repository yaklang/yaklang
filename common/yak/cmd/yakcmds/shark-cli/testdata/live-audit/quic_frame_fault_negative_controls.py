"""Challenge refusal, ownership and payload evidence with explicit test rebinding.
Mutated exports are test controls only; the physical capture and oracle stay
unchanged. Rebinding bypasses the outer export hash to exercise semantic checks.
"""
import argparse,base64,copy,hashlib,json,subprocess,sys
from pathlib import Path
ap=argparse.ArgumentParser(description=__doc__)
for n in['manifest','events','before','oracle','crypto-audit','out']:ap.add_argument('--'+n,type=Path,required=True)
a=ap.parse_args()
for p in vars(a).values():assert not any((q/'.git').exists()for q in[p.resolve(),*p.resolve().parents])
a.out.mkdir(parents=True,exist_ok=False);original=[json.loads(l)for l in a.events.open(encoding='utf-8-sig')];proof=json.loads(a.crypto_audit.read_text(encoding='utf-8-sig'));audit=Path(__file__).with_name('quic_frame_fault_audit.py')
def sha(w):return hashlib.sha256(w).hexdigest()
def refused(es):return next(e for e in es if'Rejected Packet Payload'in(e.get('session')or{})and e['session']['Rejected Packet Payload'])
def payload(es):
 e=refused(es);w=bytearray(base64.b64decode(e['session']['Rejected Packet Payload']));w[0]^=1;e['session']['Rejected Packet Payload']=base64.b64encode(w).decode()
def recovery(es):
 for e in es:
  for fr in(e.get('session')or{}).get('Frames',[]):
   for h in fr.get('HTTP3 Frames',[]):
    for v in h.get('Headers')or[]:
     if v['Name']==':path'and v['Value']=='/after-transport-faults':v['Value']='/wrong-recovery';return
 raise AssertionError('recovery request missing')
controls=[('native-AEAD-fact',lambda es:refused(es)['session'].__setitem__('Authentication Verified',False)),('peer-identity-overclaim',lambda es:refused(es)['session'].__setitem__('Peer Identity Verified',True)),('native-PN',lambda es:refused(es)['session'].__setitem__('Packet Number',999)),('refused-payload-octets',payload),('refused-payload-digest',lambda es:refused(es)['session'].__setitem__('Rejected Packet Payload SHA256','0'*64)),('payload-delivery-overclaim',lambda es:refused(es)['session'].__setitem__('Payload Delivery','delivered to protocol state')),('refused-frame-delivery',lambda es:refused(es)['session'].__setitem__('Frames',[{'Frame Type':'HANDSHAKE_DONE'}])),('native-error-class',lambda es:refused(es).__setitem__('expert','Encrypted')),('native-status',lambda es:refused(es).__setitem__('status','decoded')),('missing-refusal-diagnostic',lambda es:refused(es).__setitem__('error','')),('application-label-overclaim',lambda es:refused(es).__setitem__('protocol','http3')),('wrong-physical-frame',lambda es:refused(es)['source_bytes']['PacketRefs'][0].__setitem__('Number',999999)),('missing-connection',lambda es:refused(es).__setitem__('flow',0)),('wrong-connection-generation',lambda es:refused(es).__setitem__('flow',999)),('real-recovery-request',recovery)]
results=[]
for name,mutate in [('baseline',None),*controls]:
 d=a.out/name;d.mkdir();es=copy.deepcopy(original);cert=copy.deepcopy(proof)
 if mutate:mutate(es)
 ep=d/'events.jsonl';ep.write_text(''.join(json.dumps(v)+'\n'for v in es),encoding='utf8');cert['events_sha256']=sha(ep.read_bytes());cp=d/'crypto-explicit-test-rebinding.json';cp.write_text(json.dumps(cert),encoding='utf8')
 args=[sys.executable,'-X','utf8',str(audit),'--manifest',str(a.manifest),'--events',str(ep),'--before',str(a.before),'--oracle',str(a.oracle),'--crypto-audit',str(cp),'--out',str(d/'audit.json')];r=subprocess.run(args,capture_output=True,timeout=60);(d/'audit.log').write_bytes(r.stdout+r.stderr);assert(r.returncode==0)==(name=='baseline'),(name,(r.stdout+r.stderr).decode('utf8','replace')[-700:]);results.append({'control':name,'accepted':r.returncode==0})
out={'scope':__doc__,'baseline_passed':True,'rejected_corruptions':len(controls),'controls':results};(a.out/'summary.json').write_text(json.dumps(out,indent=2)+'\n',encoding='utf8');print(json.dumps(out))
