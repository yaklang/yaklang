"""New executions of six closed real-source TLS/QUIC recovery cases."""
from pathlib import Path
import argparse,hashlib,json,os,shutil,subprocess,time
root=Path(__file__).parent
def sha(p):
    h=hashlib.sha256()
    with Path(p).open('rb')as f:
        for b in iter(lambda:f.read(1<<20),b''):h.update(b)
    return h.hexdigest()
def read(p):return json.loads(Path(p).read_text('utf-8-sig'))
def write(p,v):p.write_text(json.dumps(v,indent=2)+'\n',encoding='utf8')
ap=argparse.ArgumentParser(description=__doc__);ap.add_argument('--bins',type=Path,required=True);ap.add_argument('--out',type=Path,required=True);a=ap.parse_args();a.out.mkdir()
buildpath=a.bins/'events-receipt.json';build=read(buildpath);binary=a.bins/'events.exe'
assert build['exit_code']==0 and sha(binary)==build['binary_SHA256']
groups=[('QUIC-sparse-epoch-plan-before-01','QUIC-sparse-epoch-actual30-01'),('QUIC-sparse-recovery-controls-before-01','QUIC-sparse-recovery-controls-actual30-01'),('TLS-whole-record-plan-before-01','TLS-whole-record-actual30-01')]
all_proofs=[]
for planname,before in groups:
    planpath=root/planname/'plan-before.json';plan=read(planpath);folder=a.out/planname;folder.mkdir()
    assert len(plan['cases'])==2
    proofs=[]
    for case in plan['cases']:
        assert shutil.disk_usage(root).free>20*(1<<30)
        old=root/before/case['case'];prior=read(old/'producer.json')
        assert prior['exit_code']==0 and prior['plan_SHA256']==sha(planpath) and prior['export_SHA256']==sha(old/'events.jsonl')
        capture=Path(case['capture']);keys=Path(case['keylog']);assert sha(capture)==case['capture_SHA256']==prior['capture_SHA256'];assert sha(keys)==case['keylog_SHA256']==prior['keylog_SHA256']
        out=folder/case['case'];out.mkdir();command=[str(binary),'replay',str(capture),str(keys),'--ignore-embedded-secrets'];start=time.monotonic()
        with(out/'events.jsonl').open('xb')as stream,(out/'stderr').open('xb')as err:
            p=subprocess.run(command,stdout=stream,stderr=err,env=dict(os.environ,GOMAXPROCS='1',GOMEMLIMIT='1GiB'),creationflags=0x08000000,timeout=180)
        assert sha(binary)==build['binary_SHA256'] and sha(capture)==case['capture_SHA256'] and sha(keys)==case['keylog_SHA256']
        proof=dict(arguments=command,environment=dict(GOMAXPROCS='1',GOMEMLIMIT='1GiB'),source=build['source'],binary_SHA256=sha(binary),build_receipt_SHA256=sha(buildpath),plan_SHA256=sha(planpath),capture_SHA256=sha(capture),keylog_SHA256=sha(keys),exit_code=p.returncode,seconds=round(time.monotonic()-start,3),export_SHA256=sha(out/'events.jsonl'),stderr_SHA256=sha(out/'stderr'),controller_SHA256=sha(__file__))
        write(out/'producer.json',proof);proofs.append(proof);assert p.returncode==0
        print(json.dumps(dict(group=planname,case=case['case'],source=build['source'],closed=True)),flush=True)
    write(folder/'summary.json',dict(scope='Actual new compiled EXE executions;source interpretation and whole-output comparison are separate.',producers=proofs));all_proofs+=proofs
write(a.out/'summary.json',dict(scope=__doc__,source=build['source'],cases=6,producers=all_proofs,original_capture_or_source_exact_offline_control=True,no_new_live_faults_claimed=True))
print(json.dumps(dict(summary=str(a.out/'summary.json'),SHA256=sha(a.out/'summary.json'),cases=6)))
