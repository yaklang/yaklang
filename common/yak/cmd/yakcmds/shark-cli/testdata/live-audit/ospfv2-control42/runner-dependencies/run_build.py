from pathlib import Path
import hashlib, json, os, shutil, subprocess, sys, time

root=Path(r'C:\tmp\yak-shark-TLS-metadata-20261028')
out=root/sys.argv[1];out.mkdir(exist_ok=False)
go=Path(r'C:\Users\V\sdk\go1.22.12\bin\go.exe')
temp=Path(r'C:\tmp\yak-shark-IP-admission-20261028\go-work-tests')
assert temp.is_dir() and shutil.disk_usage(root).free>20*(1<<30)
source=subprocess.check_output(['git','rev-parse','HEAD'],encoding='utf8').strip()
def assert_source():
 assert subprocess.check_output(['git','rev-parse','HEAD'],encoding='utf8').strip()==source
 status=subprocess.check_output(['git','status','--porcelain','--untracked-files=all'],encoding='utf8').splitlines()
 assert not [s for s in status if s[3:].endswith(('.go','.mod','.sum'))], 'Go source must be committed and frozen'
def sha(p):
 digest=hashlib.sha256()
 with p.open('rb') as f:
  for b in iter(lambda:f.read(1<<20),b''):digest.update(b)
 return digest.hexdigest()
assert_source();proofs=[]
for mode,pkg in [('events','./common/yak/cmd/yakcmds/shark-cli/testdata/live-audit/events'),('flows','./common/yak/cmd/yakcmds/shark-cli/testdata/live-audit/flow-observer'),('yak','./common/yak/cmd/yak.go')]:
 assert_source();binary=out/(mode+'.exe');log=out/(mode+'.log')
 args=[str(go),'build','-p','1','-ldflags=-s -w',*(['-gcflags=github.com/yaklang/yaklang/common/yak/cmd/yakcmds=-dwarf=false'] if mode=='yak' else []),'-o',str(binary),pkg]
 started=time.monotonic()
 with log.open('xb') as stream:r=subprocess.run(args,stdout=stream,stderr=subprocess.STDOUT,env=dict(os.environ,GOMAXPROCS='3',GOMEMLIMIT='2GiB',GOTMPDIR=str(temp)),creationflags=0x08000000)
 assert_source()
 proof=dict(source=source,arguments=args,exit_code=r.returncode,seconds=round(time.monotonic()-started,3),log_SHA256=sha(log))
 if r.returncode==0:proof['binary_SHA256']=sha(binary)
 (out/(mode+'-receipt.json')).write_text(json.dumps(proof,indent=2)+'\n',encoding='utf8');proofs.append(proof)
 print(json.dumps(proof),flush=True)
 if r.returncode:sys.exit(r.returncode)
(out/'summary.json').write_text(json.dumps(dict(source=source,builds=proofs,free_bytes_after=shutil.disk_usage(root).free),indent=2)+'\n',encoding='utf8')
