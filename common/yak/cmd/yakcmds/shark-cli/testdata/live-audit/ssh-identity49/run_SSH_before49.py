"""Record source-frozen SSH authored controls using the closed pre-fix SDK."""
from pathlib import Path
import hashlib,json,os,subprocess,time
root=Path(__file__).parent
repo=Path(r'C:\Users\V\.codex\worktrees\shark-protocol-infrastructure\yaklang')
plan=root/'SSH-source49-audit01/plan01'
out=root/'SSH-before-c04a49-01';assert not out.exists();out.mkdir()
def sha(p):return hashlib.sha256(Path(p).read_bytes()).hexdigest()
def read(p):return json.loads(Path(p).read_text('utf8'))
assert sha(plan/'manifest.json')=='37e004f4266ed3ce1bf3fd65c0df789ef2d16a11f0691713471ea8c69a2ab4b0'
manifest=read(plan/'manifest.json');cap=Path(manifest['capture']['path']);assert sha(cap)==manifest['capture']['SHA256']
for f in manifest['files']:assert sha(f['path'])==f['SHA256'],f['path']
binary=root/'bins-48/events.exe';receipt=binary.with_name('events-receipt.json');b=read(receipt)
assert b['source']=='c04a60630ce2a3d0207aeca507e99dc535f8a4dd'and b['exit_code']==0 and sha(binary)==b['binary_SHA256']
head=subprocess.check_output(['git','rev-parse','HEAD'],cwd=repo,encoding='utf8').strip()
frozen={str(p):sha(p)for p in(plan/'manifest.json',cap,binary,receipt,Path(__file__))}
args=[str(binary),'replay',str(cap),'-','--ignore-embedded-secrets'];started=time.monotonic()
with(out/'events.jsonl').open('xb')as output,(out/'stderr').open('xb')as err:
 p=subprocess.run(args,stdout=output,stderr=err,env=dict(os.environ,GOMAXPROCS='1',GOMEMLIMIT='1GiB'),creationflags=0x08000000)
assert frozen=={path:sha(path)for path in frozen}
assert subprocess.check_output(['git','rev-parse','HEAD'],cwd=repo,encoding='utf8').strip()==head
proof=dict(scope=__doc__,binary_source=b['source'],observed_working_HEAD=head,arguments=args,inputs=frozen,exit_code=p.returncode,seconds=time.monotonic()-started,export_SHA256=sha(out/'events.jsonl'),stderr_SHA256=sha(out/'stderr'),authored_not_live=True,source_cases=12,physical_frames=148,source_plan_SHA256=sha(plan/'manifest.json'))
(out/'producer.json').write_text(json.dumps(proof,indent=2)+'\n',encoding='utf8')
print(json.dumps(dict(output=str(out),exit_code=p.returncode,source=b['source'],export_SHA256=proof['export_SHA256'])))
raise SystemExit(p.returncode)
