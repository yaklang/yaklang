from pathlib import Path
import hashlib,json,os,shutil,subprocess,sys,time

root=Path(__file__).parent
repo=Path(r'C:\Users\V\.codex\worktrees\shark-protocol-infrastructure\yaklang')
go=Path(r'C:\Users\V\sdk\go1.22.12\bin\go.exe')
out=root/sys.argv[1];out.mkdir(exist_ok=False)
mode=sys.argv[2]
helper=repo/'common/bin-parser/rules/internal/generate/main.go' if mode=='generator' else root/'ospfv2_direct_probe41.go'
binary=out/('rules-generator.exe' if mode=='generator' else 'ospfv2-direct-probe.exe')
def sha(p):return hashlib.file_digest(Path(p).open('rb'),'sha256').hexdigest()
def git(*args):return subprocess.check_output(['git',*args],cwd=repo,encoding='utf8').strip()
def snapshot():
 paths=git('ls-files','--cached','--others','--exclude-standard','--','*.go','*.yaml','*.yml','go.mod','go.sum','common/bin-parser/rules/rules.tar.zst').splitlines()
 return dict(HEAD=git('rev-parse','HEAD'),files={p:sha(repo/p) for p in sorted(set(paths))})
assert shutil.disk_usage(root).free>20*(1<<30)
before=snapshot();helper_sha=sha(helper)
(out/'source-before.json').write_text(json.dumps(before,indent=2)+'\n',encoding='utf8')
runtime_temp=root/'build-temp-03';runtime_temp.mkdir(exist_ok=True)
args=[str(go),'build','-p','1','-ldflags=-s -w','-o',str(binary),str(helper)]
start=time.monotonic()
with (out/'build.log').open('xb') as log:
 result=subprocess.run(args,cwd=repo,stdout=log,stderr=subprocess.STDOUT,env=dict(os.environ,GOMAXPROCS='2',GOMEMLIMIT='1GiB',GOTMPDIR=r'C:\tmp\yak-shark-IP-admission-20261028\go-work-tests',TMP=str(runtime_temp),TEMP=str(runtime_temp)),creationflags=0x08000000)
after=snapshot();(out/'source-after.json').write_text(json.dumps(after,indent=2)+'\n',encoding='utf8')
proof=dict(mode=mode,parent_HEAD=before['HEAD'],source_files=len(before['files']),source_unchanged=before==after,helper_SHA256=helper_sha,helper_unchanged=helper_sha==sha(helper),arguments=args,exit_code=result.returncode,seconds=round(time.monotonic()-start,3),log_SHA256=sha(out/'build.log'),free_bytes_after=shutil.disk_usage(root).free)
if result.returncode==0:proof['binary_SHA256']=sha(binary)
(out/'receipt.json').write_text(json.dumps(proof,indent=2)+'\n',encoding='utf8')
print(json.dumps(proof),flush=True)
assert result.returncode==0 and before==after and proof['helper_unchanged']
