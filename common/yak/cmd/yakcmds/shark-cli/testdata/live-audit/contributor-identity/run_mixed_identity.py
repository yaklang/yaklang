"""Prepared only; root invokes build/run after the Go/source freeze ends."""
import argparse,hashlib,json,os,subprocess,time
from pathlib import Path
here=Path(__file__).resolve().parent
ap=argparse.ArgumentParser()
ap.add_argument("mode",choices=("build","run"))
for k in ("repo","source","out"):ap.add_argument("--"+k,required=True,type=Path if k!="source" else str)
ap.add_argument("--go",type=Path,default=Path(r"C:\Users\V\sdk\go1.22.12\bin\go.exe"))
ap.add_argument("--producer",type=Path)
ap.add_argument("--capture",type=Path,default=Path(r"C:\tmp\yak-shark-ESP-SSH-UI-20261028\QUIC-sparse-epoch-plan-before-01\observation.pcap"))
ap.add_argument("--keylog",type=Path,default=Path(r"C:\tmp\yak-shark-recovery-20261024\quic-epochs-02\session.keys"))
a=ap.parse_args()
def sha(p):return hashlib.sha256(Path(p).read_bytes()).hexdigest()
def git(*args):return subprocess.check_output(["git",*args],cwd=a.repo,encoding="utf8").strip()
def dirty_go():
 lines=git("status","--porcelain","--untracked-files=all").splitlines()
 return [s for s in lines if s[3:].endswith((".go","go.mod","go.sum"))]
plan=json.loads((here/"plan.json").read_text())
a.out.mkdir(exist_ok=False)
receipt={"requested_source":a.source,"mode":a.mode,"driver_SHA256":sha(__file__),"helper_source_SHA256":sha(here/"mixed_identity_replay.go")}
if a.mode=="build":
 before=git("rev-parse","HEAD"); dirty=dirty_go()
 receipt.update(before_source=before,before_dirty_go=dirty)
 assert before==a.source and not dirty, "build requires requested clean Go source"
 binary=a.out/"mixed-identity-replay.exe"
 args=[str(a.go),"build","-p","1","-ldflags=-s -w","-o",str(binary),str(here/"mixed_identity_replay.go")]
 env=dict(os.environ,GOMAXPROCS="2",GOMEMLIMIT="1GiB")
 started=time.monotonic()
 p=subprocess.run(args,cwd=a.repo,env=env,stdout=subprocess.PIPE,stderr=subprocess.PIPE,timeout=300)
 (a.out/"build.stdout").write_bytes(p.stdout);(a.out/"build.stderr").write_bytes(p.stderr)
 after=git("rev-parse","HEAD");after_dirty=dirty_go()
 receipt.update(source=before,arguments=args,exit_code=p.returncode,seconds=time.monotonic()-started,
  after_source=after,after_dirty_go=after_dirty,closed=p.returncode==0 and after==before and not after_dirty,
  binary=str(binary),binary_SHA256=sha(binary)if binary.exists()else None,
  stdout_SHA256=sha(a.out/"build.stdout"),stderr_SHA256=sha(a.out/"build.stderr"))
 (a.out/"producer.json").write_text(json.dumps(receipt,indent=2)+"\n")
 assert receipt["closed"],receipt
else:
 assert a.producer,"--producer required for run"
 producer=json.loads(a.producer.read_text());binary=Path(producer["binary"])
 assert producer["source"]==a.source and producer["closed"] and sha(binary)==producer["binary_SHA256"]
 assert sha(a.capture)==plan["capture_SHA256"] and sha(a.keylog)==plan["keylog_SHA256"]
 args=[str(binary),"--capture",str(a.capture),"--keylog",str(a.keylog),"--out",str(a.out/"actual.json")]
 started=time.monotonic()
 p=subprocess.run(args,stdout=subprocess.PIPE,stderr=subprocess.PIPE,timeout=120)
 (a.out/"stdout.log").write_bytes(p.stdout);(a.out/"stderr.log").write_bytes(p.stderr)
 receipt.update(source=a.source,producer=str(a.producer),producer_SHA256=sha(a.producer),
  binary_SHA256=sha(binary),capture_SHA256=sha(a.capture),keylog_SHA256=sha(a.keylog),
  arguments=args,exit_code=p.returncode,seconds=time.monotonic()-started,
  stdout_SHA256=sha(a.out/"stdout.log"),stderr_SHA256=sha(a.out/"stderr.log"),
  actual_SHA256=sha(a.out/"actual.json")if(a.out/"actual.json").exists()else None)
 (a.out/"receipt.json").write_text(json.dumps(receipt,indent=2)+"\n")
 assert p.returncode==0,receipt
 actual=json.loads((a.out/"actual.json").read_text())
 assert actual["passed"] and actual["analyzed_PDUs"]==1880
print(json.dumps(receipt,indent=2))
