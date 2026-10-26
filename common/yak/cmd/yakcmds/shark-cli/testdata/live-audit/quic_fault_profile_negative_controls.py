"""Ensure the exact controlled reserved-bit profile cannot weaken default AEAD QA.
Only explicitly declared, receiver-drained wire controls are allowed. Original
capture/oracle/native exports and certified Initial/header inputs stay untouched.
"""
import argparse,copy,json,subprocess,sys
from pathlib import Path
ap=argparse.ArgumentParser(description=__doc__)
for n in['oracle','events','header-audit','initial-audit','keylog','manifest','out']:ap.add_argument('--'+n,type=Path,required=True)
a=ap.parse_args()
for p in vars(a).values():assert not any((q/'.git').exists()for q in[p.resolve(),*p.resolve().parents])
a.out.mkdir(parents=True,exist_ok=False);manifest=json.loads(a.manifest.read_text(encoding='utf-8-sig'));audit=Path(__file__).with_name('quic_key_update_wire_audit.py')
def reserved(m):return next(r for r in m['transport_faults']if r['case']=='reserved-bits')
controls=[('default-remains-strict',None),('wrong-control-role',lambda m:reserved(m).__setitem__('case','allowed-server-token')),('wrong-reserved-bits',lambda m:reserved(m).__setitem__('reserved_bits',0)),('wrong-control-PN',lambda m:reserved(m).__setitem__('packet_number',999)),('wrong-control-plaintext',lambda m:reserved(m).__setitem__('plaintext_hex','02')),('undrained-control',lambda m:reserved(m).__setitem__('receiver_observed_and_drained',False)),('wrong-control-wire',lambda m:reserved(m).__setitem__('wire_sha256','0'*64)),('duplicate-control-wire',lambda m:m['transport_faults'][0].__setitem__('wire_sha256',m['transport_faults'][1]['wire_sha256']))]
results=[]
for name,mutate in [('baseline',None),*controls]:
 d=a.out/name;d.mkdir();m=copy.deepcopy(manifest)
 if mutate:mutate(m)
 mp=d/'declared-test-manifest.json';mp.write_text(json.dumps(m),encoding='utf8');args=[sys.executable,'-X','utf8',str(audit)]
 for n in['oracle','events','header-audit','initial-audit','keylog']:args+=['--'+n,str(getattr(a,n.replace('-','_'))) ]
 if name!='default-remains-strict':args+=['--declared-fault-manifest',str(mp)]
 args+=['--out',str(d/'audit.json')];r=subprocess.run(args,capture_output=True,timeout=60);(d/'audit.log').write_bytes(r.stdout+r.stderr);assert(r.returncode==0)==(name=='baseline'),(name,(r.stdout+r.stderr).decode('utf8','replace')[-700:]);results.append({'control':name,'accepted':r.returncode==0})
out={'scope':__doc__,'baseline_passed':True,'rejected_corruptions':len(controls),'controls':results};(a.out/'summary.json').write_text(json.dumps(out,indent=2)+'\n',encoding='utf8');print(json.dumps(out))
