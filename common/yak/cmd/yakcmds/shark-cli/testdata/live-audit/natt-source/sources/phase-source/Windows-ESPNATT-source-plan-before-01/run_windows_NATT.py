"""Fresh native-console ESP NAT-T tour based on the frozen parameter-only driver.

Adds explicit SPI selection, before/after byte snapshots and dynamic wait-shot
accounting. Never silently raises the default packet/history budgets.
"""
from pathlib import Path
import argparse,hashlib,json,os,re,subprocess,sys,time,traceback
from verify_cells import verify
ROOT=Path(__file__).resolve().parent
def sha(p):
    h=hashlib.sha256()
    with Path(p).open('rb') as f:
        for b in iter(lambda:f.read(1<<20),b''):h.update(b)
    return h.hexdigest()
def read(p):return json.loads(Path(p).read_text(encoding='utf-8-sig'))
def write(p,v):p.write_text(json.dumps(v,ensure_ascii=False,indent=2)+'\n',encoding='utf8')
def main():
    p=argparse.ArgumentParser(description=__doc__)
    for name in ['bins','out']:p.add_argument('--'+name,type=Path,required=True)
    p.add_argument('--source',required=True);a=p.parse_args();a.out.mkdir(exist_ok=False)
    try:
        assert os.name=='nt'
        plan=read(ROOT/'plan.json');bundle=read(ROOT/'manifest01.json');case_name=plan['case'];case=read(ROOT/'cases.json')[case_name]
        for name,entry in bundle['files'].items():assert sha(ROOT/name)==entry['SHA256'],('frozen source changed',name)
        helper=Path(plan['helper']);renderer=Path(plan['renderer']);binary=a.bins/'yak.exe';receipt=a.bins/'yak-receipt.json';producer=read(receipt)
        assert producer['source']==a.source and producer['exit_code']==0 and producer['binary_SHA256']==sha(binary)
        assert sha(helper)==plan['helper_SHA256'] and sha(plan['helper_receipt'])==plan['helper_receipt_SHA256']
        assert sha(renderer)==plan['renderer_SHA256'] and sha(case['capture'])==case['capture_SHA256'] and sha(case['keys'])==case['keylog_SHA256']
        small={'driver':Path(__file__),'plan':ROOT/'plan.json','cases':ROOT/'cases.json','source_manifest':ROOT/'manifest01.json',
            'cell_verifier':ROOT/'verify_cells.py','parameter_driver_baseline':Path(plan['base_parameter_driver']),
            'renderer':renderer,'app_build_receipt':receipt,'helper_build_receipt':Path(plan['helper_receipt'])}
        large={'application':binary,'native_console':helper,'original_capture':Path(case['capture']),'keylog':Path(case['keys'])}
        snap=a.out/'source-snapshots';snap.mkdir();before={}
        for name,path in small.items():
            data=path.read_bytes();assert len(data)<=1<<20
            before[name]=dict(path=str(path),SHA256=hashlib.sha256(data).hexdigest(),bytes=len(data))
            (snap/(name+('.json' if path.suffix=='.json' else '.txt'))).write_bytes(data)
        for name,path in large.items():before[name]=dict(path=str(path),SHA256=sha(path),bytes=path.stat().st_size)
        write(a.out/'inputs-before.json',before)
        folder=a.out/case_name
        args=[str(helper),str(folder),str(binary),case['capture'],case['keys'],case['query'],case['field'],'--ignore-embedded-secrets']
        env={k:v for k,v in os.environ.items() if not k.startswith('SHARK_QA_')}
        env.update(GOMAXPROCS='1',GOMEMLIMIT='1GiB',SHARK_QA_EXPECT_PROTOCOL='ESP',SHARK_QA_EXPECT_QUERY=case['query'],
            SHARK_QA_EXPECT_PACKETS='20000',SHARK_QA_EXPECT_ADAPTIVE='1',SHARK_QA_SELECT_FIELD='SPI',SHARK_QA_SELECT_FIELD_VALUE='4102537167')
        started=time.monotonic()
        try:result=subprocess.run(args,stdout=subprocess.PIPE,stderr=subprocess.PIPE,env=env,creationflags=0x08000000,timeout=180)
        except subprocess.TimeoutExpired as e:
            class Timeout:pass
            result=Timeout();result.returncode=-124;result.stdout=e.stdout or b'';result.stderr=(e.stderr or b'')+b'\nNative helper timeout preserved.'
        (a.out/'stdout.log').write_bytes(result.stdout);(a.out/'stderr.log').write_bytes(result.stderr)
        after={name:dict(path=entry['path'],SHA256=sha(entry['path']),bytes=Path(entry['path']).stat().st_size) for name,entry in before.items()}
        write(a.out/'inputs-after.json',after)
        proof=dict(source=a.source,arguments=args,environment_overrides={k:v for k,v in env.items() if k.startswith('SHARK_QA_')},
            binary_SHA256=sha(binary),build_receipt_SHA256=sha(receipt),driver_SHA256=sha(__file__),
            manifest_SHA256=sha(ROOT/'manifest01.json'),source_before_SHA256=sha(a.out/'inputs-before.json'),
            source_after_SHA256=sha(a.out/'inputs-after.json'),sources_byte_equal=before==after,
            exit_code=result.returncode,seconds=time.monotonic()-started,visual_review_pending=True,snapshots={})
        for cells in sorted(folder.glob('*.json')):
            state=read(cells)
            if 'Rows' not in state:continue
            # Render canonical snapshots and only the latest failing wait for
            # each stage; raw JSON from every wait is always retained.
            if '-wait-' in cells.stem and any(p.stem>cells.stem for p in folder.glob(cells.stem.rsplit('-',1)[0]+'-*.json')):continue
            rendered=cells.with_suffix('.png')
            r=subprocess.run([sys.executable,'-B',str(renderer),str(cells),str(rendered)],stdout=subprocess.PIPE,stderr=subprocess.PIPE,creationflags=0x08000000)
            assert r.returncode==0,(cells.name,'renderer failed')
            proof['snapshots'][cells.stem]=dict(cells_SHA256=sha(cells),PNG_SHA256=sha(rendered))
        write(a.out/'proof.json',proof)
        assert before==after,'Execution input bytes changed; preserve this run and use a new generation'
        assert result.returncode==0,'Native helper failed; logs and all cells remain, no success certificate'
        proof['source_cell_verification']=verify(folder,case)
        proof['passed']=True;write(a.out/'proof.json',proof)
        print(json.dumps(dict(proof=str(a.out/'proof.json'),SHA256=sha(a.out/'proof.json'),passed=True,visual_review_pending=True)))
    except Exception:
        write(a.out/'failure.json',dict(failure=traceback.format_exc(),no_success_certificate=True));raise
if __name__=='__main__':main()
