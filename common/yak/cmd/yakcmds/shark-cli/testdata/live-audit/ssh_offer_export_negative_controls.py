"""Reject unrelated SSH changes while allowing literal sender-offer projections."""
import argparse,base64,copy,json,subprocess,sys
from pathlib import Path
ap=argparse.ArgumentParser(description=__doc__)
for n in['before','after','out']:ap.add_argument('--'+n,type=Path,required=True)
a=ap.parse_args()
for p in vars(a).values():assert not any((q/'.git').exists()for q in[p.resolve(),*p.resolve().parents])
a.out.mkdir(parents=True,exist_ok=False);original=[json.loads(l)for l in a.after.open(encoding='utf-8-sig')];audit=Path(__file__).with_name('ssh_offer_export_invariants.py')
def kex(es):return next(e for e in es if(e.get('session')or{}).get('Packet Name')=='KEXINIT')
def chosen(es):return next(e for e in es if(e.get('session')or{}).get('Kex Algorithm'))
def raw(es):
 e=kex(es);b=bytearray(base64.b64decode(e['raw']));b[-1]^=1;e['raw']=base64.b64encode(b).decode()
controls=[('changed-offer',lambda es:kex(es)['session'].__setitem__('Offered Encryption C2S','aes256-ctr')),('changed-selection',lambda es:chosen(es)['session'].__setitem__('Encryption C2S','aes256-ctr')),('unrelated-session-field',lambda es:kex(es)['session'].__setitem__('invented field',True)),('literal-padding-octets',raw),('changed-flow',lambda es:kex(es).__setitem__('flow',999)),('changed-PDU-ID',lambda es:kex(es).__setitem__('id',999)),('changed-field-tree',lambda es:kex(es)['fields'].__setitem__('Packet Length',1)),('changed-typed-field',lambda es:kex(es)['field_values']['values'].append({'name':'invented.field','value':1})),('extra-record',lambda es:es.append(copy.deepcopy(kex(es))))]
results=[]
for name,mutate in [('baseline',None),*controls]:
 d=a.out/name;d.mkdir();es=copy.deepcopy(original)
 if mutate:mutate(es)
 ep=d/'events.jsonl';ep.write_text(''.join(json.dumps(e)+'\n'for e in es));r=subprocess.run([sys.executable,'-X','utf8',str(audit),'--kind','SDK','--before',str(a.before),'--after',str(ep),'--out',str(d/'audit.json')],capture_output=True,timeout=60);(d/'audit.log').write_bytes(r.stdout+r.stderr);assert(r.returncode==0)==(name=='baseline'),(name,(r.stdout+r.stderr).decode('utf8','replace')[-700:]);results.append({'control':name,'accepted':r.returncode==0})
out={'scope':__doc__,'baseline_passed':True,'rejected_corruptions':len(controls),'controls':results};(a.out/'summary.json').write_text(json.dumps(out,indent=2)+'\n');print(json.dumps(out))
