from pathlib import Path
import hashlib,json,os,subprocess,sys,time
root=Path(__file__).parent;out=root/sys.argv[1];out.mkdir(exist_ok=False)
plan=root/'OSPFv2-direct-source-plan41-01';input_path=plan/'probe-input.jsonl'
binary=root/'OSPFv2-direct-build41-01/ospfv2-direct-probe.exe';receipt=binary.with_name('receipt.json')
def sha(p):return hashlib.sha256(Path(p).read_bytes()).hexdigest()
assert sha(input_path)=='c3bdda8524c5c05d9dff7f441a1005c1ab7e302e613e6d106c28ac7f08e3c676'
assert sha(plan/'manifest.json')=='507507c8edb1a1e234e2fe5a5379edc7d2573a9113f4dbffbb3675e0201a1680'
build=json.loads(receipt.read_text('utf8'));assert build['exit_code']==0 and build['source_unchanged']and sha(binary)==build['binary_SHA256']
manifest=json.loads((plan/'manifest.json').read_text('utf8'))
inputs={str(input_path):sha(input_path),str(plan/'manifest.json'):sha(plan/'manifest.json'),str(binary):sha(binary),str(receipt):sha(receipt)}
for row in map(json.loads,input_path.read_text('utf8').splitlines()):
 p=row['input_path'];assert sha(p)==row['input_file_SHA256'];inputs[p]=row['input_file_SHA256']
start=time.monotonic()
with input_path.open('rb')as src,(out/'direct.jsonl').open('xb')as stream,(out/'stderr').open('xb')as err:
 p=subprocess.run([str(binary)],stdin=src,stdout=stream,stderr=err,env=dict(os.environ,GOMAXPROCS='1',GOMEMLIMIT='1GiB'),creationflags=0x08000000)
after={p:sha(p)for p in inputs};assert inputs==after
proof=dict(scope='Actual bounded direct-codec invocation; no native/CLI ingress claim',parent_HEAD=build['parent_HEAD'],source_snapshot_SHA256=sha(binary.parent/'source-before.json'),binary_SHA256=sha(binary),build_SHA256=sha(receipt),source_plan_SHA256=sha(plan/'manifest.json'),input_SHA256=sha(input_path),exit_code=p.returncode,seconds=time.monotonic()-start,inputs_unchanged=inputs==after,export_SHA256=sha(out/'direct.jsonl'),stderr_SHA256=sha(out/'stderr'),scenario_lines=len((out/'direct.jsonl').read_text('utf8').splitlines()),controller_SHA256=sha(__file__))
(out/'producer.json').write_text(json.dumps(proof,indent=2)+'\n',encoding='utf8');print(json.dumps(proof));assert p.returncode==0
