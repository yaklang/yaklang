"""Run pinned authored SSH source/API plan with closed exact-source SDK/CLI."""
from pathlib import Path
import argparse,hashlib,json,os,subprocess,time
root=Path(__file__).parent
repo=Path(r'C:\Users\V\.codex\worktrees\shark-protocol-infrastructure\yaklang')
ap=argparse.ArgumentParser(description=__doc__)
for key in('bins','out'):ap.add_argument('--'+key,type=Path,required=True)
ap.add_argument('--source',required=True);a=ap.parse_args()
assert not a.out.exists();a.out.mkdir()
def sha(p):return hashlib.sha256(Path(p).read_bytes()).hexdigest()
def read(p):return json.loads(Path(p).read_text('utf8'))
def put(p,v):p.write_text(json.dumps(v,indent=2)+'\n',encoding='utf8')
def guard():
 assert subprocess.check_output(['git','rev-parse','HEAD'],cwd=repo,encoding='utf8').strip()==a.source
 changes=subprocess.check_output(['git','status','--porcelain','--untracked-files=all'],cwd=repo,encoding='utf8').splitlines()
 assert not [x for x in changes if x[3:].endswith(('.go','.mod','.sum','.yaml','.yml','.zst'))]
plan=root/'SSH-SDK-CLI-source49-01/plan01';checker=plan.parent/'ssh_native_cli49.py'
assert sha(plan/'manifest.json')=='6152f55c293920a466756dcc090e1b4d0627f8ca7379d33e21cb175c37c2d85d'
assert sha(checker)=='70b86676bddbd93420693421be2e8d454fc177b51af3ff5dec3c9fd8ab7eb63a'
commands=read(plan/'runtime-command-plan.json');capture=Path(commands['capture']);assert sha(capture)==commands['capture_SHA256']
builds={}
for kind in('events','yak'):
 binary=a.bins/(kind+'.exe');receipt=a.bins/(kind+'-receipt.json');b=read(receipt)
 assert b['source']==a.source and b['exit_code']==0 and sha(binary)==b['binary_SHA256']
 builds[kind]=(binary,receipt)
proofs=[]
for name,kind,key in(('events','events','native_args'),('yak','yak','CLI_args'),('field-descriptors','yak','field_descriptor_args')):
 guard();binary,receipt=builds[kind]
 frozen={str(p):sha(p)for p in(binary,receipt,Path(__file__),checker,plan/'manifest.json',plan/'runtime-command-plan.json',capture)}
 output=a.out/(name+('.json'if name=='field-descriptors'else'.jsonl'))
 args=[str(binary),*commands[key]];start=time.monotonic()
 with output.open('xb')as stream,output.with_suffix('.stderr').open('xb')as err:
  p=subprocess.run(args,stdout=stream,stderr=err,env=dict(os.environ,GOMAXPROCS='1',GOMEMLIMIT='1GiB'),creationflags=0x08000000)
 guard();assert frozen=={p:sha(p)for p in frozen}
 proof=dict(source=a.source,kind=kind,arguments=args,inputs=frozen,exit_code=p.returncode,seconds=time.monotonic()-start,export_SHA256=sha(output),stderr_SHA256=sha(output.with_suffix('.stderr')),source_unchanged=True)
 put(a.out/(name+'-producer.json'),proof);proofs.append(proof);assert p.returncode==0
args=[os.sys.executable,'-B','-X','utf8',str(checker),'verify','--plan',str(plan),'--events',str(a.out/'events.jsonl'),'--CLI',str(a.out/'yak.jsonl'),'--descriptors',str(a.out/'field-descriptors.json'),'--out',str(a.out/'independent-audit')]
q=subprocess.run(args,capture_output=True,encoding='utf8',creationflags=0x08000000)
(a.out/'checker.stdout').write_text(q.stdout,encoding='utf8');(a.out/'checker.stderr').write_text(q.stderr,encoding='utf8')
audit=a.out/'independent-audit/summary.json'
put(a.out/'summary.json',dict(source=a.source,source_plan_SHA256=sha(plan/'manifest.json'),runs=proofs,checker_exit=q.returncode,audit_SHA256=sha(audit)if audit.exists()else None,passed=q.returncode==0,authored_control_connections=12,physical_frames=148,real_capture_count=0))
print(q.stdout,flush=True)
if q.returncode:print(q.stderr,flush=True)
raise SystemExit(q.returncode)
