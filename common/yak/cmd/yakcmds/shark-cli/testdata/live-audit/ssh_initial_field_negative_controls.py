"""Challenge the independent initial-field auditor with corrupted projections."""
import argparse,base64,copy,json,subprocess,sys
from pathlib import Path
ap=argparse.ArgumentParser(description=__doc__)
for n in['events','oracle','out']:ap.add_argument('--'+n,type=Path,required=True)
a=ap.parse_args()
for p in vars(a).values():assert not any((q/'.git').exists()for q in[p.resolve(),*p.resolve().parents])
a.out.mkdir(exist_ok=False,parents=True);events=[json.loads(l)for l in a.events.open()];audit=Path(__file__).with_name('ssh_initial_field_audit.py')
def event(es,name):return next(e for e in es if(e.get('session')or{}).get('Packet Name')==name)
def tree(es,name):return event(es,name)['session']['Initial Packet Fields']
def mutate_raw(es):
 e=event(es,'KEX_ECDH_INIT');w=bytearray(base64.b64decode(e['raw']));w[12]^=128;e['raw']=base64.b64encode(w).decode()
def host(es):return tree(es,'KEX_ECDH_REPLY')['Payload']['Host Key']
controls=[('packet-length',lambda es:tree(es,'KEX_ECDH_INIT').__setitem__('Packet Length',1)),('padding-length',lambda es:tree(es,'KEX_ECDH_INIT').__setitem__('Padding Length',0)),('padding-bytes',lambda es:tree(es,'KEX_ECDH_INIT').__setitem__('Padding','AQ==')),('message-code',lambda es:tree(es,'KEX_ECDH_INIT')['Payload'].__setitem__('Message Number',31)),('literal-public-key',lambda es:tree(es,'KEX_ECDH_INIT')['Payload'].__setitem__('Client Curve25519 Public Key','AA==')),('lossy-public-key',lambda es:tree(es,'KEX_ECDH_INIT')['Payload'].__setitem__('Client Curve25519 Public Key','replacement-\ufffd-text')),('literal-source-bytes',mutate_raw),('lazy-bin-parser-divergence',lambda es:event(es,'KEX_ECDH_INIT')['fields'].__setitem__('Payload',{'Message Number':30,'SSHKexDHInit':{}})),('host-key-format',lambda es:host(es).__setitem__('Host Key Format','invented-key')),('host-key-exponent',lambda es:host(es).__setitem__('RSA Exponent','AQ==')),('host-key-fingerprint',lambda es:event(es,'KEX_ECDH_REPLY')['session'].__setitem__('Host Key SHA256','0'*64)),('signature-verified-claim',lambda es:event(es,'KEX_ECDH_REPLY')['session'].__setitem__('Signature Verified',True)),('peer-verified-claim',lambda es:event(es,'KEX_ECDH_REPLY')['session'].__setitem__('Peer Identity Verified',True)),('GEX-group-size',lambda es:tree(es,'KEX_DH_GEX_REQUEST')['Payload'].__setitem__('Preferred Group Bits',1)),('GEX-prime',lambda es:tree(es,'KEX_DH_GEX_GROUP')['Payload'].__setitem__('Group Prime','AQ=='))]
results=[]
for name,mutate in [('baseline',None),*controls]:
 d=a.out/name;d.mkdir();es=copy.deepcopy(events)
 if mutate:mutate(es)
 ep=d/'events.jsonl';ep.write_text(''.join(json.dumps(e)+'\n'for e in es));r=subprocess.run([sys.executable,'-X','utf8',str(audit),'--events',str(ep),'--oracle',str(a.oracle),'--out',str(d/'audit.json')],capture_output=True,timeout=60);(d/'audit.log').write_bytes(r.stdout+r.stderr);assert(r.returncode==0)==(mutate is None),(name,(r.stdout+r.stderr).decode('utf8','replace')[-700:]);results.append({'control':name,'accepted':r.returncode==0})
summary={'scope':__doc__,'baseline_passed':True,'rejected_corruptions':len(controls),'controls':results};(a.out/'summary.json').write_text(json.dumps(summary,indent=2)+'\n');print(json.dumps(summary))
