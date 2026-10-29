"""Run the frozen30 actual source/boundary inputs with the guarded probe."""
from pathlib import Path
import hashlib,importlib.util,json,os,subprocess,sys,time
root=Path(__file__).parent;out=root/sys.argv[1];out.mkdir(exist_ok=False)
def sha(p):return hashlib.sha256(Path(p).read_bytes()).hexdigest()
plan=root/'GTPv2-API-source-plan43-01';checker=root/'gtpv2_api_source_checker43_v02.py'
assert sha(checker)=='0a526831d4bd9e2e41faba4b1f519c24cae6f69bbf8361fa19620084de82ef10'
spec=importlib.util.spec_from_file_location('frozen_gtpv2_source_checker',checker);m=importlib.util.module_from_spec(spec);spec.loader.exec_module(m)
inputs,facts,policy,manifest=m.load_plan(plan);assert len(inputs)==30
binary=root/'GTPv2-direct-build44-01/gtpv2-direct-probe.exe';receipt=binary.with_name('receipt.json');build=json.loads(receipt.read_text('utf8'));assert build['exit_code']==0 and build['source_unchanged']and sha(binary)==build['binary_SHA256']
paths={str(p):sha(p)for p in[checker,receipt,binary,plan/'manifest.json',plan/'probe-input.jsonl']}
for row in inputs:paths.setdefault(row['input_path'],sha(row['input_path']))
start=time.monotonic()
with(plan/'probe-input.jsonl').open('rb')as source,(out/'direct.jsonl').open('xb')as target,(out/'stderr').open('xb')as err:
 result=subprocess.run([str(binary)],stdin=source,stdout=target,stderr=err,env=dict(os.environ,GOMAXPROCS='1',GOMEMLIMIT='1GiB'),creationflags=0x08000000)
assert paths=={p:sha(p)for p in paths}
proof=dict(scope='Actual direct-codec source/boundary invocation, not native/CLI/UI ingress',parent_HEAD=build['parent_HEAD'],source_snapshot_SHA256=sha(binary.parent/'source-before.json'),source_plan_SHA256=sha(plan/'manifest.json'),input_SHA256=sha(plan/'probe-input.jsonl'),binary_SHA256=sha(binary),build_SHA256=sha(receipt),exit_code=result.returncode,seconds=time.monotonic()-start,inputs_unchanged=True,export_SHA256=sha(out/'direct.jsonl'),stderr_SHA256=sha(out/'stderr'),output_lines=len((out/'direct.jsonl').read_text('utf8').splitlines()),controller_SHA256=sha(__file__))
(out/'producer.json').write_text(json.dumps(proof,indent=2)+'\n',encoding='utf8');print(json.dumps(proof));assert result.returncode==0
