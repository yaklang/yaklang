"""Challenge initial SSH role/field/byte auditing without rewriting source PCAP."""
import argparse,base64,copy,json,subprocess,sys
from pathlib import Path
ap=argparse.ArgumentParser(description=__doc__)
for n in['capture','manifest','events','oracle','out']:ap.add_argument('--'+n,type=Path,required=True)
a=ap.parse_args()
for p in vars(a).values():assert not any((q/'.git').exists()for q in[p.resolve(),*p.resolve().parents])
a.out.mkdir(parents=True,exist_ok=False);events=[json.loads(l)for l in a.events.open(encoding='utf-8-sig')];oracle=json.loads(a.oracle.read_text());audit=Path(__file__).with_name('ssh_wire_audit.py')
def kex(es):return next(e for e in es if(e.get('session')or{}).get('Packet Name')=='KEXINIT')
def raw(es):
 e=kex(es);w=bytearray(base64.b64decode(e['raw']));w[-1]^=1;e['raw']=base64.b64encode(w).decode()
def reference(o,key,value):
 def walk(v):
  if isinstance(v,dict):
   if key in v:v[key]=value;return True
   return any(walk(child)for child in v.values()if isinstance(child,(dict,list)))
  if isinstance(v,list):return any(walk(child)for child in v)
  return False
 assert walk(o)
controls=[('invented-selected-kex',lambda es,o:kex(es)['session'].__setitem__('Kex Algorithm','curve25519-sha256')),('invented-selected-cipher',lambda es,o:kex(es)['session'].__setitem__('Encryption C2S','aes256-ctr')),('wrong-offer-order',lambda es,o:kex(es)['session'].__setitem__('Offered Encryption C2S','aes128-ctr,aes256-ctr')),('missing-role-limitation',lambda es,o:kex(es)['session'].pop('Negotiation Context')),('invented-selected-alias',lambda es,o:kex(es)['field_values']['values'].append({'name':'ssh.kex.selected','value':'curve25519-sha256','origin':'context'})),('false-peer-identity',lambda es,o:kex(es)['session'].__setitem__('Peer Identity Verified',True)),('changed-message-status',lambda es,o:kex(es).__setitem__('status','malformed')),('literal-PDU-octets',lambda es,o:raw(es)),('owned-PDU-digest',lambda es,o:kex(es)['source_bytes'].__setitem__('SHA256','0'*64)),('wrong-source-packet',lambda es,o:kex(es)['source_bytes']['PacketRefs'][0].__setitem__('Number',1)),('wrong-native-flow',lambda es,o:kex(es).__setitem__('flow',999)),('extra-logical-PDU',lambda es,o:es.append(copy.deepcopy(kex(es)))),('missing-logical-PDU',lambda es,o:es.remove(kex(es))),('reference-offer-field',lambda es,o:reference(o,'ssh.kex_algorithms','invented-kex')),('reference-physical-number',lambda es,o:reference(o,'frame.number','999999'))]
results=[]
for name,mutate in [('baseline',None),*controls]:
 d=a.out/name;d.mkdir();es=copy.deepcopy(events);o=copy.deepcopy(oracle)
 if mutate:mutate(es,o)
 ep=d/'events.jsonl';op=d/'oracle.json';ep.write_text(''.join(json.dumps(e)+'\n'for e in es));op.write_text(json.dumps(o));r=subprocess.run([sys.executable,'-X','utf8',str(audit),'--capture',str(a.capture),'--manifest',str(a.manifest),'--events',str(ep),'--oracle',str(op),'--profile','no-syn-negative-control','--out',str(d/'audit.json')],capture_output=True,timeout=60);(d/'audit.log').write_bytes(r.stdout+r.stderr);assert(r.returncode==0)==(name=='baseline'),(name,(r.stdout+r.stderr).decode('utf8','replace')[-700:]);results.append({'control':name,'accepted':r.returncode==0})
out={'scope':__doc__,'baseline_passed':True,'rejected_corruptions':len(controls),'controls':results};(a.out/'summary.json').write_text(json.dumps(out,indent=2)+'\n');print(json.dumps(out))
