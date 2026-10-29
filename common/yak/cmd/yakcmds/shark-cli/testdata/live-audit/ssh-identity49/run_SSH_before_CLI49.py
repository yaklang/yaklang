"""Complete the old binary reference with CLI and descriptors; no new SDK run claimed."""
from pathlib import Path
import hashlib,json,os,subprocess,time
root=Path(__file__).parent
repo=Path(r'C:\Users\V\.codex\worktrees\shark-protocol-infrastructure\yaklang')
out=root/'SSH-before-c04a49-01'
def sha(p):return hashlib.sha256(Path(p).read_bytes()).hexdigest()
def read(p):return json.loads(Path(p).read_text('utf8'))
plan=root/'SSH-SDK-CLI-source49-01/plan01';manifest=plan/'manifest.json'
assert sha(manifest)=='6152f55c293920a466756dcc090e1b4d0627f8ca7379d33e21cb175c37c2d85d'
command=read(plan/'runtime-command-plan.json')
binary=root/'bins-48/yak.exe';build=binary.with_name('yak-receipt.json');b=read(build)
assert b['source']=='c04a60630ce2a3d0207aeca507e99dc535f8a4dd'and b['exit_code']==0 and sha(binary)==b['binary_SHA256']
head=subprocess.check_output(['git','rev-parse','HEAD'],cwd=repo,encoding='utf8').strip()
for name,key in(('yak','CLI_args'),('field-descriptors','field_descriptor_args')):
 output=out/(name+('.jsonl'if name=='yak'else'.json'));assert not output.exists()
 cap=Path(command['capture']);assert sha(cap)==command['capture_SHA256']
 frozen={str(p):sha(p)for p in(manifest,plan/'runtime-command-plan.json',cap,binary,build,Path(__file__))}
 args=[str(binary),*command[key]];start=time.monotonic()
 with output.open('xb')as stream,output.with_suffix('.stderr').open('xb')as err:
  p=subprocess.run(args,stdout=stream,stderr=err,env=dict(os.environ,GOMAXPROCS='1',GOMEMLIMIT='1GiB'),creationflags=0x08000000)
 assert frozen=={p:sha(p)for p in frozen}
 assert subprocess.check_output(['git','rev-parse','HEAD'],cwd=repo,encoding='utf8').strip()==head
 proof=dict(binary_source=b['source'],observed_working_HEAD=head,arguments=args,inputs=frozen,exit_code=p.returncode,seconds=time.monotonic()-start,export_SHA256=sha(output),candidate_current_Go_not_executed=True)
 output.with_name(name+'-producer.json').write_text(json.dumps(proof,indent=2)+'\n',encoding='utf8')
 assert p.returncode==0;print(json.dumps(dict(kind=name,exit_code=0,output=str(output),SHA256=sha(output))))
