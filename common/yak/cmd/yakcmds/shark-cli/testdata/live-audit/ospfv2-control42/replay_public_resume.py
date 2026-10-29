"""Source-frozen public replay; resume only hash-verified closed executions."""
from pathlib import Path
import hashlib,json,os,shutil,subprocess,sys,time
root=Path(__file__).parent
mode,run,bin_run=sys.argv[1:];assert mode in ['native','CLI'] and len(run)==len(bin_run)==2 and run.isdigit()and bin_run.isdigit()
baseline=Path(r'C:\tmp\yak-shark-TLS-metadata-20261028')/('runtime-public-native-03'if mode=='native'else'runtime-public-CLI-04')
out=root/('runtime-public-'+mode+'-'+run);out.mkdir(exist_ok=True)
subprocess.run(['compact.exe','/C','/I','/Q',str(out)],stdout=subprocess.DEVNULL,check=True)
bins=root/('bins-'+bin_run)
cases=sorted(p for p in baseline.glob('*/*')if p.is_dir());assert len(cases)==37
assert not(out/'summary.json').exists(),'A finalized generation is immutable'
def sha(p):
 h=hashlib.sha256()
 with p.open('rb')as f:
  for b in iter(lambda:f.read(1<<20),b''):h.update(b)
 return h.hexdigest()
proofs=[];resumed=0;built_source=None
for case in cases:
 assert shutil.disk_usage(root).free>20*(1<<30),'Disk guard: resume this generation after freeing margin'
 dest=out/case.parent.name/case.name;dest.mkdir(parents=True,exist_ok=True)
 kinds=['events']if mode=='native'else['yak']
 if mode=='native'and case.name in ['weber','sharon']:kinds.append('flows')
 current=[]
 for kind in kinds:
  prior_path=case/(kind+'-producer.json');before=json.loads(prior_path.read_text('utf-8-sig'))
  build=json.loads((bins/(kind+'-receipt.json')).read_text('utf8'));assert build['exit_code']==0
  if built_source is None:built_source=build['source']
  assert built_source==build['source']
  binary=bins/(kind+'.exe');assert sha(binary)==build['binary_SHA256']
  args=[str(binary),*before['arguments'][1:]];cap=Path(before['capture']);assert sha(cap)==before['capture_SHA256']
  target=dest/(kind+('.json'if kind=='flows'else'.jsonl'));receipt_path=dest/(kind+'-producer.json')
  binding=dict(source=built_source,arguments=args,binary_SHA256=build['binary_SHA256'],capture=str(cap),capture_SHA256=before['capture_SHA256'],keylog_used=before['keylog_used'],previous_export=str(case/target.name),previous_export_SHA256=sha(case/target.name),baseline_producer_SHA256=sha(prior_path))
  if receipt_path.exists():
   proof=json.loads(receipt_path.read_text('utf8'))
   assert proof['exit_code']==0 and proof['export_SHA256']==sha(target)
   assert all(proof[k]==v for k,v in binding.items()),'Closed execution provenance mismatch'
   resumed+=1
  else:
   assert not target.exists(),'Unreceipted output requires independent recovery, never overwrite'
   started=time.monotonic()
   with target.open('xb')as stream,(dest/(kind+'.stderr')).open('xb')as err:
    p=subprocess.run(args,stdout=stream,stderr=err,env=dict(os.environ,GOMAXPROCS='1',GOMEMLIMIT='1GiB'),creationflags=0x08000000)
   assert sha(binary)==build['binary_SHA256']and sha(cap)==binding['capture_SHA256']
   proof=dict(binding,export_SHA256=sha(target),exit_code=p.returncode,seconds=round(time.monotonic()-started,3))
   proof['complete_byte_identical_to_previous']=proof['export_SHA256']==proof['previous_export_SHA256']
   with receipt_path.open('x',encoding='utf8')as f:json.dump(proof,f,indent=2);f.write('\n')
   assert p.returncode==0,(case,kind)
  proofs.append(proof);current.append(proof)
 print(json.dumps(dict(case=case.name,mode=mode,outputs=kinds,all_byte_identical=all(v['complete_byte_identical_to_previous']for v in current))),flush=True)
with(out/'summary.json').open('x',encoding='utf8')as f:json.dump(dict(source=built_source,cases=37,actual_new_binary_execution=True,closed_executions_verified_on_resume=resumed,outputs=proofs,free_bytes_after=shutil.disk_usage(root).free),f,indent=2);f.write('\n')
