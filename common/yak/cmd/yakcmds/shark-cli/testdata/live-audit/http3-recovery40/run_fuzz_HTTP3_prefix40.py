from pathlib import Path
import hashlib,json,os,shutil,subprocess,sys,time
root=Path(__file__).parent;dest=root/sys.argv[1];dest.mkdir()
source=subprocess.check_output(['git','rev-parse','HEAD'],encoding='utf8').strip()
go=Path(r'C:\Users\V\sdk\go1.22.12\bin\go.exe')
task_temp=root/'fuzz-temp-01';task_temp.mkdir(exist_ok=True)
subprocess.run(['compact.exe','/C','/I','/Q',str(task_temp)],check=True,stdout=subprocess.DEVNULL)
def check():
 assert subprocess.check_output(['git','rev-parse','HEAD'],encoding='utf8').strip()==source
 changed=subprocess.check_output(['git','status','--porcelain','--untracked-files=all'],encoding='utf8').splitlines()
 assert not [s for s in changed if s[3:].endswith(('.go','.mod','.sum','.yaml','.yml','.zst'))]
check();proofs=[]
for pkg,name in [('./common/pcapx/pcaputil','FuzzHTTP3RetransmissionDoesNotReplayApplication')]:
 assert shutil.disk_usage(root).free>20*(1<<30)
 log=dest/(name+'.log')
 args=[str(go),'test','-p','1','-ldflags=-w',pkg,'-run','^$','-fuzz','^'+name+'$','-fuzztime','60s','-parallel','2','-timeout','12m']
 start=time.monotonic()
 with log.open('xb')as output:r=subprocess.run(args,stdout=output,stderr=subprocess.STDOUT,env=dict(os.environ,GOMAXPROCS='2',GOMEMLIMIT='2GiB',GOTMPDIR=r'C:\tmp\yak-shark-IP-admission-20261028\go-work-tests',TMP=str(task_temp),TEMP=str(task_temp)),creationflags=0x08000000)
 check();proof=dict(source=source,target=name,arguments=args,exit_code=r.returncode,seconds=round(time.monotonic()-start,3),log_SHA256=hashlib.sha256(log.read_bytes()).hexdigest(),free_bytes_after=shutil.disk_usage(root).free)
 (dest/(name+'-receipt.json')).write_text(json.dumps(proof,indent=2)+'\n',encoding='utf8');proofs.append(proof)
 print(json.dumps(proof),flush=True);print(log.read_text('utf8')[-1800:],flush=True)
 if r.returncode:sys.exit(r.returncode)
(dest/'summary.json').write_text(json.dumps(dict(source=source,targets=proofs),indent=2)+'\n',encoding='utf8')
