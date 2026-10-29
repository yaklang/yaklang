"""Run only the three source-frozen explicit direct-codec controls."""
from pathlib import Path
import argparse,hashlib,json,os,subprocess,time
root=Path(__file__).parent
def sha(p):return hashlib.sha256(Path(p).read_bytes()).hexdigest()
def read(p):return json.loads(Path(p).read_text('utf-8-sig'))
ap=argparse.ArgumentParser(description=__doc__);ap.add_argument('--out',type=Path,required=True);a=ap.parse_args();a.out.mkdir()
planroot=root/'OSPFv3-boundary-authored-source-before-next-01/frozen-source03';planpath=planroot/'plan.json';plan=read(planpath)
assert sha(planpath)=='e9f5b8e4f2820bf12ca79bf7049562442f0bec7540d1a910c7dd2d11da570cd0'
casespath=planroot/'cases.json';assert sha(casespath)==plan['cases_SHA256']
cases=[r for r in read(casespath)if r.get('requires_direct_codec_invocation')];assert len(cases)==3
buildpath=root/'OSPF-direct-build38-01/receipt.json';build=read(buildpath);binary=buildpath.with_name('ospf-direct-probe.exe');helper=root/'ospfv3_direct_boundary_probe38.go'
assert build['source']=='d6772965c669e288705f7e0d0e25c62972226832' and build['exit_code']==0 and sha(binary)==build['binary_SHA256'] and sha(helper)==build['source_file_SHA256']
inputs=dict(plan=planpath,cases=casespath,build=buildpath,binary=binary,helper=helper)
for case in cases:
    for key in ('original_frame','source_facts'):
        p=Path(case[key]);assert sha(p)==case[key+'_SHA256'];inputs[case['name']+'/'+key]=p
before={k:dict(path=str(p),SHA256=sha(p),bytes=p.stat().st_size)for k,p in inputs.items()}
command=[str(binary),str(casespath)];start=time.monotonic()
with(a.out/'direct.jsonl').open('xb')as out,(a.out/'stderr').open('xb')as err:
    p=subprocess.run(command,stdout=out,stderr=err,env=dict(os.environ,GOMAXPROCS='1',GOMEMLIMIT='1GiB'),creationflags=0x08000000,timeout=30)
after={k:dict(path=str(p),SHA256=sha(p),bytes=p.stat().st_size)for k,p in inputs.items()}
rows=[json.loads(line)for line in(a.out/'direct.jsonl').read_text('utf8').splitlines()];assert sorted(r['case']for r in rows)==sorted(c['name']for c in cases)
proof=dict(scope=__doc__,source=build['source'],controller_SHA256=sha(__file__),arguments=command,exit_code=p.returncode,seconds=round(time.monotonic()-start,3),inputs_before=before,inputs_after=after,inputs_unchanged=before==after,export_SHA256=sha(a.out/'direct.jsonl'),stderr_SHA256=sha(a.out/'stderr'),cases=3,native_or_CLI_ingress_claimed=False,private_typed_aliases_exported=False)
(a.out/'producer.json').write_text(json.dumps(proof,indent=2)+'\n',encoding='utf8')
assert p.returncode==0 and before==after
print(json.dumps(dict(producer=str(a.out/'producer.json'),SHA256=sha(a.out/'producer.json'),cases=3)))
