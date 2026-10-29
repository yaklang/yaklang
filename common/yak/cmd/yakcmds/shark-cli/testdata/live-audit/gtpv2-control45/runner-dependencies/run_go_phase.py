from pathlib import Path
import collections, hashlib, json, os, shutil, subprocess, sys, time

root=Path(r'C:\tmp\yak-shark-TLS-metadata-20261028')
out=root/sys.argv[1];out.mkdir(exist_ok=False)
go=Path(r'C:\Users\V\sdk\go1.22.12\bin\go.exe')
temp=Path(r'C:\tmp\yak-shark-IP-admission-20261028\go-work-tests')
assert temp.is_dir() and shutil.disk_usage(root).free>20*(1<<30)
def sha(p):return hashlib.sha256(p.read_bytes()).hexdigest()
def snapshot():
 paths=subprocess.check_output(['git','status','--porcelain','--untracked-files=all'],encoding='utf8').splitlines()
 selected=[Path(line[3:]) for line in paths if line[3:].endswith(('.go','.mod','.sum','.yaml','.yml','.zst'))]
 return {'parent_HEAD':subprocess.check_output(['git','rev-parse','HEAD'],encoding='utf8').strip(),'Go_working_files':{str(p):sha(p) for p in selected}}
before=snapshot();(out/'source-before.json').write_text(json.dumps(before,indent=2)+'\n',encoding='utf8')
args=[str(go),'test','-p','1','-ldflags=-w','-json','./common/pcapx/pcaputil','./common/yak/cmd/yakcmds/shark-cli','./common/bin-parser','./common/bin-parser/rules','-count=1','-timeout','30m']
if len(sys.argv)>2:args.extend(['-run',sys.argv[2]])
started=time.monotonic()
with (out/'tests.jsonl').open('xb') as log,(out/'tests.stderr').open('xb') as err:
 r=subprocess.run(args,stdout=log,stderr=err,env=dict(os.environ,GOMAXPROCS='3',GOMEMLIMIT='2GiB',GOTMPDIR=str(temp)),creationflags=0x08000000)
after=snapshot();(out/'source-after.json').write_text(json.dumps(after,indent=2)+'\n',encoding='utf8')
assert before==after,'Go sources changed while tests were running'
actions=collections.Counter();packages=[];fails=[];runs=collections.Counter();terminals=collections.Counter()
for line in (out/'tests.jsonl').open(encoding='utf8'):
 e=json.loads(line)
 if e.get('Test'):
  key=(e['Package'],e['Test'])
  if e['Action']=='run':runs[key]+=1
  if e['Action'] in ['pass','skip','fail']:terminals[key]+=1;actions[e['Action']]+=1
 if e['Action']=='fail':fails.append({k:v for k,v in e.items() if k!='Output'})
 if not e.get('Test') and e['Action'] in ['pass','fail','skip']:packages.append(e)
receipt=dict(source=before,arguments=args,exit_code=r.returncode,seconds=round(time.monotonic()-started,3),test_actions=dict(actions),run_count=sum(runs.values()),all_runs_have_unique_terminal=all(n==1 and terminals[k]==1 for k,n in runs.items()),packages=packages,failures=fails,log_SHA256=sha(out/'tests.jsonl'),stderr_SHA256=sha(out/'tests.stderr'),free_bytes_after=shutil.disk_usage(root).free)
(out/'receipt.json').write_text(json.dumps(receipt,indent=2)+'\n',encoding='utf8')
print(json.dumps({**{k:v for k,v in receipt.items() if k not in ['source','failures']},'failure_count':len(fails),'first_failures':fails[:5]}),flush=True)
sys.exit(r.returncode)
