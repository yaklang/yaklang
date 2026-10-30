"""Produce both independent authored data sets on one frozen Windows source."""
from pathlib import Path
import argparse,hashlib,json,os,subprocess,time
P=Path(__file__).parent
repo=Path(r'C:\Users\V\.codex\worktrees\shark-protocol-infrastructure\yaklang')
ap=argparse.ArgumentParser()
for k in('bins','out'):ap.add_argument('--'+k,type=Path,required=True)
ap.add_argument('--source',required=True);a=ap.parse_args()
sha=lambda p:hashlib.sha256(Path(p).read_bytes()).hexdigest()
read=lambda p:json.loads(Path(p).read_text('utf-8-sig'))
def put(p,v):
 with p.open('x',encoding='utf8')as f:json.dump(v,f,indent=2);f.write('\n')
def guard():
 assert subprocess.check_output(['git','rev-parse','HEAD'],cwd=repo,text=True).strip()==a.source
 assert not[x for x in subprocess.check_output(['git','status','--porcelain','--untracked-files=all'],cwd=repo,text=True).splitlines()if x[3:].endswith(('.go','.mod','.sum','.yaml','.yml','.zst'))]
plan=P/'SSH-wire-ranges-API-CLI-source53-01/plan02'
assert sha(plan/'manifest.json')=='30a5ee3060b3bb4678e6bfb355324a0ba3a3f404fffbaff1b402e127d9b6d436'
for p,h in read(plan/'manifest.json')['inputs'].items():assert sha(p)==h
commands=read(plan/'runtime-command-plan.json');a.out.mkdir(exist_ok=False);proofs=[]
for dataset in('source49','source50'):
 c=commands[dataset];capture=Path(c['capture']);assert sha(capture)==c['capture_SHA256']
 dest=a.out/dataset;dest.mkdir()
 for name,kind,key in[('events','events','events_args'),('yak','yak','CLI_args'),('field-descriptors','yak','descriptor_args')]:
  guard();binary=a.bins/(kind+'.exe');build=a.bins/(kind+'-receipt.json');b=read(build)
  assert b['source']==a.source and b['exit_code']==0 and b['binary_SHA256']==sha(binary)
  frozen={str(p):sha(p)for p in(binary,build,capture,plan/'manifest.json',plan/'runtime-command-plan.json',Path(__file__))}
  output=dest/(name+('.json'if name=='field-descriptors'else'.jsonl'));stderr=dest/(name+'.stderr')
  args=[str(binary),*c[key]];start=time.monotonic()
  with output.open('xb')as out,stderr.open('xb')as err:
   r=subprocess.run(args,stdout=out,stderr=err,env=dict(os.environ,GOMAXPROCS='1',GOMEMLIMIT='1GiB'),creationflags=0x08000000)
  guard();assert frozen=={p:sha(p)for p in frozen}
  proof=dict(source=a.source,dataset=dataset,arguments=args,inputs=frozen,capture_SHA256=sha(capture),exit_code=r.returncode,seconds=round(time.monotonic()-start,3),export_SHA256=sha(output),stderr_SHA256=sha(stderr),source_unchanged=True)
  put(dest/(name+'-producer.json'),proof);proofs.append(proof);assert r.returncode==0
 put(dest/'producer-summary.json',dict(source=a.source,dataset=dataset,closed_executions=3,runs=proofs[-3:],all_source_inputs_unchanged=True))
 print(json.dumps(dict(source=a.source,dataset=dataset,closed_executions=3)),flush=True)
put(a.out/'producer-summary.json',dict(source=a.source,closed_executions=6,source_plan_SHA256=sha(plan/'manifest.json'),physical_frames=263,authored_connections=21,real_capture_count=0,runs=proofs))
