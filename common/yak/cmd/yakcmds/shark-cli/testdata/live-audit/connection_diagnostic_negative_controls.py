"""Challenge diagnostic-restoration witnesses using declared real IPv4/IPv6/domain subsets."""
import argparse,copy,hashlib,json,subprocess,sys
from pathlib import Path
ap=argparse.ArgumentParser(description=__doc__)
for name in ['before','after','SDK','proof','out']:ap.add_argument('--'+name,type=Path,required=True)
a=ap.parse_args()
for path in vars(a).values():assert not any((p/'.git').exists()for p in[path.resolve(),*path.resolve().parents])
def sha(path):
 with path.open('rb')as f:return hashlib.file_digest(f,'sha256').hexdigest()
proof=json.loads(a.proof.read_text());assert proof['before_sha256']==sha(a.before)and proof['after_sha256']==sha(a.after)and proof['SDK_sha256']==sha(a.SDK);targets=proof['changed_rows'][:3];assert len(targets)==3
frames={v['physical_frame']for v in targets};ids={x for v in targets for x in v['SDK_PDU_witnesses']}
def select(path,key,values):
 out=[]
 for line in path.open(encoding='utf-8-sig'):
  e=json.loads(line)
  if e[key]in values:out.append(e)
 return out
before=select(a.before,'number',frames);after=select(a.after,'number',frames);sdk=select(a.SDK,'id',ids);assert len(before)==len(after)==3 and sdk;a.out.mkdir(exist_ok=False,parents=True)
def run(name,mutate,positive=False):
 b,n,s=copy.deepcopy(before),copy.deepcopy(after),copy.deepcopy(sdk);mutate(b,n,s);d=a.out/name;d.mkdir()
 for file,rows in [('before',b),('after',n),('SDK',s)]: (d/(file+'.jsonl')).write_text(''.join(json.dumps(v,sort_keys=True)+'\n'for v in rows))
 r=subprocess.run([sys.executable,'-X','utf8',str(Path(__file__).with_name('connection_diagnostic_projection_audit.py')),'--before',str(d/'before.jsonl'),'--after',str(d/'after.jsonl'),'--SDK',str(d/'SDK.jsonl'),'--out',str(d/'proof.json')],capture_output=True,timeout=30);(d/'audit.log').write_bytes(r.stdout+r.stderr);assert (r.returncode==0)==positive,(name,r.returncode,(r.stdout+r.stderr).decode('utf8','replace')[-600:])
controls={
 'wrong-status':lambda b,n,s:n[0].__setitem__('protocol_status','decoded'),
 'wrong-scope':lambda b,n,s:n[0].__setitem__('protocol_scope','connection-associated bytes'),
 'false-encryption':lambda b,n,s:n[0].__setitem__('visibility','encrypted'),
 'changed-reason':lambda b,n,s:n[0].__setitem__('info','accepted'),
 'cross-domain':lambda b,n,s:n[1]['capture_domain'].__setitem__('Interface',9999),
 'changed-CLI-flow':lambda b,n,s:n[0].__setitem__('tcp_flow_id',9999),
 'changed-PDU-ref':lambda b,n,s:n[0]['pdus'][0].__setitem__('id',99999),
 'wrong-SDK-endpoint':lambda b,n,s:s[0].__setitem__('source','192.0.2.9:9'),
 'wrong-SDK-status':lambda b,n,s:s[0].__setitem__('status','decoded'),
 'wrong-SDK-error':lambda b,n,s:s[0].__setitem__('error','different parser result'),
 'wrong-SDK-wire':lambda b,n,s:s[0]['source_bytes'].__setitem__('SHA256','0'*64),
 'drop-native-witnesses':lambda b,n,s:s.clear(),
 'drop-row':lambda b,n,s:n.pop(),
 'duplicate-row':lambda b,n,s:n.append(copy.deepcopy(n[-1])),
 'row-order':lambda b,n,s:n.reverse(),
}
run('positive-format-and-key-order',lambda b,n,s:None,True)
for name,mutate in controls.items():run(name,mutate)
result={'scope':__doc__,'subset_physical_frames':sorted(frames),'subset_rows':3,'subset_native_records':len(sdk),'positive_controls':1,'negative_controls_rejected':list(controls),'full_projection_proof_sha256':sha(a.proof),'numeric_namespace_equality_not_assumed':True};(a.out/'summary.json').write_text(json.dumps(result,indent=2)+'\n');print(json.dumps(result))
