"""Execute authored source03 only; no keylog, direct codec cases excluded.

Prepared before candidate native output. This runner executes existing binaries,
never Go. Source/capture/controller/build/binary bytes are bound before and after.
"""
import argparse,hashlib,json,os,subprocess,time,traceback
from pathlib import Path

PLAN_SHA='e9f5b8e4f2820bf12ca79bf7049562442f0bec7540d1a910c7dd2d11da570cd0'
def sha(p):return hashlib.sha256(Path(p).read_bytes()).hexdigest()
def read(p):return json.loads(Path(p).read_text('utf-8-sig'))
def write(p,v):
    with Path(p).open('x',encoding='utf8')as f:json.dump(v,f,indent=2);f.write('\n')

def main():
    ap=argparse.ArgumentParser(description=__doc__)
    for name in('bins','out'):ap.add_argument('--'+name,type=Path,required=True)
    ap.add_argument('--source',required=True);a=ap.parse_args();assert not a.out.exists()
    planroot=Path(__file__).parent/'frozen-source03';planpath=planroot/'plan.json';plan=read(planpath)
    assert sha(planpath)==PLAN_SHA and sha(planroot/'cases.json')==plan['cases_SHA256']
    cases=read(planroot/'cases.json');assert len(cases)==20;a.out.mkdir()
    bound={str(p):sha(p)for p in[Path(__file__),planpath,planroot/'cases.json']}
    for c in cases:
        for k in('capture','original_frame','source_facts'):
            assert sha(c[k])==c[k+'_SHA256'];bound[c[k]]=c[k+'_SHA256']
    builds={}
    for kind in('events','yak'):
        binary=a.bins/(kind+'.exe');bp=a.bins/(kind+'-receipt.json');build=read(bp)
        assert build['source']==a.source and build['exit_code']==0 and sha(binary)==build['binary_SHA256']
        builds[kind]=(binary,bp,build);bound[str(binary)]=sha(binary);bound[str(bp)]=sha(bp)
    write(a.out/'inputs-before.json',bound);receipts=[];skipped=[]
    try:
        for c in cases:
            if c.get('requires_direct_codec_invocation'):
                skipped.append(c['name']);continue
            folder=a.out/c['name'];folder.mkdir()
            for kind,(binary,bp,build)in builds.items():
                command=([str(binary),'replay',c['capture'],'-','--ignore-embedded-secrets']if kind=='events'else[str(binary),'shark','-r',c['capture'],'--json','--ignore-embedded-secrets'])
                target=folder/(kind+'.jsonl');started=time.monotonic()
                with target.open('xb')as out,(folder/(kind+'.stderr')).open('xb')as err:
                    result=subprocess.run(command,stdout=out,stderr=err,env=dict(os.environ,GOMAXPROCS='1',GOMEMLIMIT='1GiB'),creationflags=0x08000000,timeout=60)
                receipt=dict(source=a.source,source_plan_SHA256=PLAN_SHA,controller_SHA256=bound[str(Path(__file__))],build_receipt_SHA256=bound[str(bp)],binary_SHA256=bound[str(binary)],case=c['name'],capture_SHA256=c['capture_SHA256'],source_facts_SHA256=c['source_facts_SHA256'],arguments=command,exit_code=result.returncode,seconds=round(time.monotonic()-started,3),export_SHA256=sha(target),stderr_SHA256=sha(folder/(kind+'.stderr')),authored_not_real_traffic=True,direct_codec_overrides_claimed=False)
                write(folder/(kind+'-receipt.json'),receipt);receipts.append(receipt)
                assert result.returncode==0
            print(json.dumps(dict(case=c['name'],source=a.source,executed=['events','CLI'])),flush=True)
        assert len(receipts)==34 and len(skipped)==3
        after={p:sha(p)for p in bound};write(a.out/'inputs-after.json',after);assert bound==after
        write(a.out/'summary.json',dict(scope=__doc__,source=a.source,source_plan_SHA256=PLAN_SHA,executions=receipts,physical_cases=17,skipped_direct_cases=skipped,authored_not_real_traffic=True,no_keylog=True,inputs_before_SHA256=sha(a.out/'inputs-before.json'),inputs_after_SHA256=sha(a.out/'inputs-after.json')))
        print(json.dumps(dict(summary=str(a.out/'summary.json'),SHA256=sha(a.out/'summary.json'),physical_cases=17,direct_cases_not_run=3)),flush=True)
    except Exception:
        write(a.out/'controller-failure.json',dict(error=traceback.format_exc()));raise

if __name__=='__main__':main()
