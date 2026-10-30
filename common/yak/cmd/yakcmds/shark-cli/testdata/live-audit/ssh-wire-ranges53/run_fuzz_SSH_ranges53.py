"""Bounded committed-source SSH role, refusal and nested-boundary fuzz checks."""
from pathlib import Path
import hashlib,json,os,shutil,subprocess,sys,time
P=Path(__file__).parent;dest=P/sys.argv[1];dest.mkdir(exist_ok=False)
repo=Path(r'C:\Users\V\.codex\worktrees\shark-protocol-infrastructure\yaklang')
go=Path(r'C:\Users\V\sdk\go1.22.12\bin\go.exe')
source=subprocess.check_output(['git','rev-parse','HEAD'],cwd=repo,text=True).strip()
task_temp=P/'fuzz-temp-01';assert task_temp.is_dir()
def guard():
 assert subprocess.check_output(['git','rev-parse','HEAD'],cwd=repo,text=True).strip()==source
 assert not[x for x in subprocess.check_output(['git','status','--porcelain','--untracked-files=all'],cwd=repo,text=True).splitlines()if x[3:].endswith(('.go','.mod','.sum','.yaml','.yml','.zst'))]
def put(p,v):
 with p.open('x',encoding='utf8')as f:json.dump(v,f,indent=2);f.write('\n')
proofs=[]
for pkg,name in[('./common/pcapx/pcaputil','FuzzSSHClientRoleNegotiationIsolation'),('./common/pcapx/pcaputil','FuzzSSHNativeRefusalFieldIsolation'),('./common/bin-parser/parser/stream_parser','FuzzSSHPlaintextEdwardsBoundaries')]:
 guard();assert shutil.disk_usage(P).free>20*(1<<30)
 log=dest/(name+'.log');args=[str(go),'test','-p','1','-ldflags=-w',pkg,'-run','^$','-fuzz','^'+name+'$','-fuzztime','60s','-parallel','1','-timeout','12m'];start=time.monotonic()
 with log.open('xb')as out:r=subprocess.run(args,cwd=repo,stdout=out,stderr=subprocess.STDOUT,env=dict(os.environ,GOMAXPROCS='1',GOMEMLIMIT='1GiB',GOTMPDIR=r'C:\tmp\yak-shark-IP-admission-20261028\go-work-tests',TMP=str(task_temp),TEMP=str(task_temp)),creationflags=0x08000000)
 guard();proof=dict(source=source,target=name,arguments=args,exit_code=r.returncode,seconds=round(time.monotonic()-start,3),log_SHA256=hashlib.sha256(log.read_bytes()).hexdigest(),free_bytes_after=shutil.disk_usage(P).free)
 put(dest/(name+'-receipt.json'),proof);proofs.append(proof)
 print(json.dumps(proof),flush=True)
 if r.returncode:sys.exit(r.returncode)
put(dest/'summary.json',dict(source=source,targets=proofs,all_closed=True))
