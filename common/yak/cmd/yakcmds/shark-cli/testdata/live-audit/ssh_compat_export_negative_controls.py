"""Challenge exact-message ID mapping and SSH recovery with derived subsets.
Use the original public SSH connection plus one unchanged neighbouring STP PDU.
For metadata controls deliberately rebind only the enclosing file digest so the
per-message byte/field/endpoint proofs and ID bijection, not just the digest gate,
must reject the corruption. Original message proofs are never regenerated.
"""
import argparse,base64,copy,hashlib,json,subprocess,sys
from pathlib import Path
ap=argparse.ArgumentParser(description=__doc__)
for n in['before','after','oracle','out']:ap.add_argument('--'+n,type=Path,required=True)
a=ap.parse_args()
for p in vars(a).values():assert not any((q/'.git').exists()for q in[p.resolve(),*p.resolve().parents])
a.out.mkdir(parents=True,exist_ok=False);audit=Path(__file__).with_name('ssh_compat_export_invariants.py');fields=Path(__file__).with_name('ssh_initial_field_audit.py')
def select(p,after):
 out=[]
 for line in p.open():
  e=json.loads(line)
  if e['id']>4304:break
  if e['protocol']=='ssh'and e['flow']==23 or e['id']==(4282 if after else 4277):out.append(e)
 return out
old=select(a.before,False);new=select(a.after,True);assert(len(old),len(new))==(3,13)
def write(p,es):p.write_text(''.join(json.dumps(e)+'\n'for e in es))
before=a.out/'before.jsonl';positive=a.out/'after.jsonl';write(before,old);write(positive,new);certpath=a.out/'field-certificate.json';r=subprocess.run([sys.executable,'-X','utf8',str(fields),'--events',str(positive),'--oracle',str(a.oracle),'--out',str(certpath)],capture_output=True,timeout=60);(a.out/'initial-field-baseline.log').write_bytes(r.stdout+r.stderr);assert r.returncode==0
certificate=json.loads(certpath.read_text())
def neighbour(es):return next(e for e in es if e['protocol']=='stp')
def first_kex(es):return next(e for e in es if(e.get('session')or{}).get('Message Number')==20)
def typed(es,name):return next(v for v in neighbour(es)['field_values']['values']if v['name']==name)
def raw(es):
 e=neighbour(es);w=bytearray(base64.b64decode(e['raw']));w[-1]^=1;e['raw']=base64.b64encode(w).decode()
def source_frame(es):neighbour(es)['source_bytes']['PacketRefs'][0]['Number']=4277
def interface(es):neighbour(es)['source_bytes']['PacketRefs'][0]['Domain']['Interface']+=1
def truncated_kex(es):
 e=first_kex(es);e['raw']=base64.b64encode(base64.b64decode(e['raw'])[:-8]).decode()
def late_warning(es):next(e for e in es if e.get('expert')=='Encrypted')['session'].pop('Identification Warning')
def false_identity(es):next(e for e in es if(e.get('session')or{}).get('Message Number')==33)['session']['Peer Identity Verified']=True
controls=[('literal-contributor-number',source_frame),('literal-capture-interface',interface),('literal-other-protocol-bytes',raw),('other-connection-flow',lambda es:neighbour(es).__setitem__('flow',999)),('duplicate-generated-PDU-id',lambda es:neighbour(es).__setitem__('id',4276)),('wrong-typed-id',lambda es:typed(es,'pdu.id').__setitem__('value',4283)),('wire-length-is-not-an-ID',lambda es:typed(es,'pdu.length').__setitem__('value',4277)),('wire-field-ID-collision',lambda es:neighbour(es)['fields'].__setitem__('Wire value must stay literal',4277)),('wrong-byte-source-parent',lambda es:neighbour(es)['source_bytes'].__setitem__('ParentPDU',4276)),('wrong-response-association',lambda es:neighbour(es).__setitem__('response_to',4276)),('invented-completion-frame',lambda es:neighbour(es)['source_bytes'].__setitem__('CompletedIn',{'Number':99999,'Domain':neighbour(es)['domain']})),('truncated-added-KEXINIT',truncated_kex),('changed-added-field-tree',lambda es:first_kex(es)['fields'].__setitem__('Packet Length',1)),('changed-added-endpoint',lambda es:first_kex(es).__setitem__('source','[198.18.0.1]:22')),('lost-opaque-warning',late_warning),('false-peer-identity',false_identity),('duplicate-added-PDU',lambda es:es.append(copy.deepcopy(first_kex(es)))),('missing-added-PDU',lambda es:es.remove(first_kex(es)))]
results=[]
for name,mutate in [('baseline',None),*controls]:
 d=a.out/name;d.mkdir();es=copy.deepcopy(new)
 if mutate:mutate(es)
 ep=d/'after.jsonl';write(ep,es);cert=copy.deepcopy(certificate);cert['events_sha256']=hashlib.sha256(ep.read_bytes()).hexdigest();cp=d/'field-certificate.json';cp.write_text(json.dumps(cert));r=subprocess.run([sys.executable,'-X','utf8',str(audit),'--before',str(before),'--after',str(ep),'--field-certificate',str(cp),'--target-flow','23','--out',str(d/'audit.json')],capture_output=True,timeout=60);(d/'audit.log').write_bytes(r.stdout+r.stderr);assert(r.returncode==0)==(mutate is None),(name,(r.stdout+r.stderr).decode('utf8','replace')[-700:]);results.append({'control':name,'accepted':r.returncode==0})
summary={'scope':__doc__,'baseline_original_PDUs':len(old),'baseline_recovered_PDUs':len(new),'baseline_passed':True,'rejected_corruptions':len(controls),'controls':results};(a.out/'summary.json').write_text(json.dumps(summary,indent=2)+'\n');print(json.dumps(summary))
