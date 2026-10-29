"""Execute frozen independent34 RSVP inputs with a guarded direct-codec build."""
from pathlib import Path
import hashlib,json,os,subprocess,sys,time
root=Path(__file__).parent;out=root/sys.argv[1];out.mkdir(exist_ok=False)
def sha(p):return hashlib.sha256(Path(p).read_bytes()).hexdigest()
plan=root/'RSVP-direct-API-source-plan46-01';manifest=plan/'manifest.json';inputs=plan/'probe-input.jsonl';checker=root/'rsvp_direct_api_checker46.py'
assert sha(manifest)=='2b5b6c499ddd6f14456ea39f62e6ef3b20ee04f343893ea7ebefcc9b216c32fa'
assert sha(inputs)=='d3fc9977eb61df6462a8c8dac41d6a91ad4ee200c3d0b240f95121365b72be02'
assert sha(checker)=='0a4146d707184cd7426e472cdd9636258b09a2f47a9c28a62898d19f1d32690a'
rows=[json.loads(l)for l in inputs.read_text('utf8').splitlines()];assert len(rows)==34
binary=root/'RSVP-direct-build46-01/rsvp-direct-probe.exe';receipt=binary.with_name('receipt.json');build=json.loads(receipt.read_text('utf8'));assert build['exit_code']==0 and build['source_unchanged']and sha(binary)==build['binary_SHA256']
frozen={str(p):sha(p)for p in(checker,manifest,inputs,binary,receipt)}
for row in rows:frozen.setdefault(row['input_path'],sha(row['input_path']))
start=time.monotonic()
with inputs.open('rb')as data,(out/'direct.jsonl').open('xb')as target,(out/'stderr').open('xb')as err:
 r=subprocess.run([str(binary)],stdin=data,stdout=target,stderr=err,env=dict(os.environ,GOMAXPROCS='1',GOMEMLIMIT='1GiB'),creationflags=0x08000000)
assert frozen=={p:sha(p)for p in frozen}
proof=dict(scope=__doc__,parent_HEAD=build['parent_HEAD'],source_snapshot_SHA256=sha(binary.parent/'source-before.json'),binary_SHA256=sha(binary),build_SHA256=sha(receipt),plan_SHA256=sha(manifest),input_SHA256=sha(inputs),exit_code=r.returncode,seconds=time.monotonic()-start,inputs_unchanged=True,output_lines=len((out/'direct.jsonl').read_text('utf8').splitlines()),export_SHA256=sha(out/'direct.jsonl'),stderr_SHA256=sha(out/'stderr'),controller_SHA256=sha(__file__),native_CLI_UI_claim=False)
(out/'producer.json').write_text(json.dumps(proof,indent=2)+'\n',encoding='utf8');print(json.dumps(proof));assert r.returncode==0
