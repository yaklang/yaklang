"""Close actual SDK/CLI executions against an independent SSH sender-role plan."""
from pathlib import Path
import argparse,hashlib,json,os,subprocess,time

root=Path(__file__).parent
repo=Path(r'C:\Users\V\.codex\worktrees\shark-protocol-infrastructure\yaklang')
ap=argparse.ArgumentParser()
for k in('bins','out'):ap.add_argument('--'+k,type=Path,required=True)
ap.add_argument('--source',required=True);a=ap.parse_args()
assert not a.out.exists();a.out.mkdir()
def sha(p):return hashlib.sha256(Path(p).read_bytes()).hexdigest()
def read(p):return json.loads(Path(p).read_text('utf-8-sig'))
def put(p,v):
 with p.open('x',encoding='utf8')as f:json.dump(v,f,indent=2);f.write('\n')
def guard():
 assert subprocess.check_output(['git','rev-parse','HEAD'],cwd=repo,encoding='utf8').strip()==a.source
 changed=subprocess.check_output(['git','status','--porcelain','--untracked-files=all'],cwd=repo,encoding='utf8').splitlines()
 assert not[r for r in changed if r[3:].endswith(('.go','.mod','.sum','.yaml','.yml','.zst'))]
plan=root/'SSH-role-source50-01/plan02'
assert sha(plan/'manifest.json')=='ae73e1329488cc159cb0958f483c529e7dd50e270b53249dae8521324a2553af'
manifest=read(plan/'manifest.json')
for p,h in manifest['inputs'].items():assert sha(p)==h,p
capture=Path(manifest['capture']['path']);assert sha(capture)==manifest['capture']['SHA256']
proofs=[]
for name,kind,tail in[
 ('events','events',['replay',str(capture),'-','--ignore-embedded-secrets']),
 ('yak','yak',['shark','-r',str(capture),'--json','--ignore-embedded-secrets']),
 ('field-descriptors','yak',['shark','--list-fields','--json'])]:
 guard();binary=a.bins/(kind+'.exe');build=a.bins/(kind+'-receipt.json');b=read(build)
 assert b['source']==a.source and b['exit_code']==0 and b['binary_SHA256']==sha(binary)
 frozen={str(p):sha(p)for p in(binary,build,capture,plan/'manifest.json',Path(__file__))}
 target=a.out/(name+('.json'if name=='field-descriptors'else'.jsonl'));stderr=a.out/(name+'.stderr')
 args=[str(binary),*tail];start=time.monotonic()
 with target.open('xb')as out,stderr.open('xb')as err:
  r=subprocess.run(args,stdout=out,stderr=err,env=dict(os.environ,GOMAXPROCS='1',GOMEMLIMIT='1GiB'),creationflags=0x08000000)
 guard();assert frozen=={p:sha(p)for p in frozen}
 proof=dict(source=a.source,arguments=args,inputs=frozen,exit_code=r.returncode,seconds=round(time.monotonic()-start,3),
            export_SHA256=sha(target),stderr_SHA256=sha(stderr),capture_SHA256=sha(capture))
 put(a.out/(name+'-producer.json'),proof);proofs.append(proof);assert r.returncode==0
put(a.out/'producer-summary.json',dict(source=a.source,closed_executions=3,source_plan_SHA256=sha(plan/'manifest.json'),
 physical_source_frames=115,authored_source_cases=9,live_captures=0,runs=proofs))
print(json.dumps(dict(source=a.source,closed_executions=3,out=str(a.out))))
