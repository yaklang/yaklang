"""Execute the independently frozen49 Intserv inputs with exact-source binding."""
from pathlib import Path
import argparse,hashlib,json,os,subprocess,time
root=Path(__file__).parent
repo=Path(r'C:\Users\V\.codex\worktrees\shark-protocol-infrastructure\yaklang')
def sha(p):return hashlib.sha256(Path(p).read_bytes()).hexdigest()
def read(p):return json.loads(Path(p).read_text('utf8'))
def put(p,v):p.write_text(json.dumps(v,indent=2)+'\n',encoding='utf8')
def git(*args):return subprocess.check_output(['git',*args],cwd=repo,encoding='utf8').strip()
p=argparse.ArgumentParser(description=__doc__);p.add_argument('--build',type=Path,required=True);p.add_argument('--out',type=Path,required=True);a=p.parse_args()
assert not a.out.exists();a.out.mkdir()
plan=root/'RSVP-Intserv-API-plan48-03';checker=root/'rsvp_intserv_API48.py'
inputs=root/'Intserv-source48-01/frozen-plan01/probe-input.jsonl'
assert sha(plan/'manifest.json')=='1ee14f677769da754b5dcd0b37bfd9e30dc2e0cdfce50791696b5021d2e73227'
assert read(plan/'manifest.json')['checker_SHA256']==sha(checker)
assert sha(inputs)==read(plan/'manifest.json')['probe_input_SHA256']
binary=a.build/'rsvp-direct-probe.exe';receipt=a.build/'receipt.json';build=read(receipt);source=read(a.build/'source-before.json')
assert build['exit_code']==0 and build['source_unchanged']and build['helper_unchanged']and sha(binary)==build['binary_SHA256']
def source_guard():
 assert git('rev-parse','HEAD')==source['HEAD']
 for path,digest in source['files'].items():assert sha(repo/path)==digest,path
source_guard()
frozen={str(q):sha(q)for q in(checker,inputs,plan/'manifest.json',plan/'API-policy.json',binary,receipt,Path(__file__))}
for row in map(json.loads,inputs.read_text('utf8').splitlines()):frozen[row['input_path']]=sha(row['input_path'])
started=time.monotonic()
with inputs.open('rb')as data,(a.out/'direct.jsonl').open('xb')as output,(a.out/'stderr').open('xb')as err:
 result=subprocess.run([str(binary)],stdin=data,stdout=output,stderr=err,env=dict(os.environ,GOMAXPROCS='1',GOMEMLIMIT='1GiB'),creationflags=0x08000000)
source_guard();assert frozen=={path:sha(path)for path in frozen}
proof=dict(scope=__doc__,parent_HEAD=source['HEAD'],source_snapshot_SHA256=sha(a.build/'source-before.json'),build_receipt_SHA256=sha(receipt),binary_SHA256=sha(binary),API_plan_SHA256=sha(plan/'manifest.json'),checker_SHA256=sha(checker),inputs=frozen,exit_code=result.returncode,seconds=time.monotonic()-started,export_SHA256=sha(a.out/'direct.jsonl'),stderr_SHA256=sha(a.out/'stderr'),source_unchanged=True,unique_physical_frames_not_inferred_from_evaluation_count=True)
put(a.out/'producer.json',proof);assert result.returncode==0
verify=subprocess.run([os.sys.executable,'-B','-X','utf8',str(checker),'verify','--plan',str(plan),'--actual',str(a.out/'direct.jsonl'),'--out',str(a.out/'independent-audit')],stdout=subprocess.PIPE,stderr=subprocess.PIPE,encoding='utf8',creationflags=0x08000000)
(a.out/'checker.stdout').write_text(verify.stdout,encoding='utf8');(a.out/'checker.stderr').write_text(verify.stderr,encoding='utf8')
print(verify.stdout,flush=True)
if verify.returncode:print(verify.stderr,flush=True)
raise SystemExit(verify.returncode)
