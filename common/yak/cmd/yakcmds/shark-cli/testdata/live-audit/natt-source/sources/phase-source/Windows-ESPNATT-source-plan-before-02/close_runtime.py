"""Close the old actual Windows run with a scoped02 auditor; never reruns native."""
from pathlib import Path
import argparse,hashlib,json,traceback
from verify_cells import verify
ROOT=Path(__file__).resolve().parent
PARENT=ROOT.parent/'Windows-ESPNATT-source-plan-before-01'
def sha(p):
    h=hashlib.sha256()
    with Path(p).open('rb') as f:
        for b in iter(lambda:f.read(1<<20),b''):h.update(b)
    return h.hexdigest()
def read(p):return json.loads(Path(p).read_text(encoding='utf-8-sig'))
def write(p,v):p.write_text(json.dumps(v,indent=2,ensure_ascii=False)+'\n',encoding='utf8')
def main():
    p=argparse.ArgumentParser(description=__doc__)
    for name in ['runtime','out']:p.add_argument('--'+name,type=Path,required=True)
    a=p.parse_args();a.out.mkdir(exist_ok=False)
    try:
        plan=read(ROOT/'plan02.json');bundle=read(ROOT/'manifest02.json')
        for name,entry in bundle['files'].items():assert sha(ROOT/name)==entry['SHA256']
        assert sha(PARENT/'manifest01.json')==plan['parent_manifest_SHA256']
        assert sha(PARENT/'plan.json')==plan['parent_plan_SHA256']
        assert sha(ROOT/'cases.json')==sha(PARENT/'cases.json')==plan['case_file_SHA256']
        old_path=a.runtime/'proof.json';old=read(old_path)
        assert old['source']=='69346fc94722c5b530e0d59b14a9783b6de3f530' and old['exit_code']==0 and old['sources_byte_equal']is True
        assert old['manifest_SHA256']==plan['parent_manifest_SHA256']
        assert old['driver_SHA256']==plan['parent_driver_SHA256']
        assert sha(a.runtime/'inputs-before.json')==old['source_before_SHA256']
        assert sha(a.runtime/'inputs-after.json')==old['source_after_SHA256']
        before=read(a.runtime/'inputs-before.json');after=read(a.runtime/'inputs-after.json');assert before==after
        for value in before.values():assert sha(value['path'])==value['SHA256']
        app=read(before['app_build_receipt']['path'])
        assert app['source']==old['source'] and app['exit_code']==0 and app['binary_SHA256']==old['binary_SHA256']==before['application']['SHA256']
        assert old['arguments'][-1]=='--ignore-embedded-secrets'
        assert '--protocol-history-mib' not in old['arguments'] and '--max-packets' not in old['arguments']
        assert old['environment_overrides']['SHARK_QA_EXPECT_PACKETS']=='20000'
        assert old['environment_overrides']['SHARK_QA_SELECT_FIELD']=='SPI'
        assert (a.runtime/'failure.json').is_file(),'Original01 overbroad-check failure must remain preserved'
        case=read(ROOT/'cases.json')['ESP-NATT-Weber46978']
        result=verify(a.runtime/'ESP-NATT-Weber46978',case)
        value=dict(scope=__doc__,actual_source=old['source'],actual_runtime=str(a.runtime),
            actual_native_execution_rerun=False,original01_failure_preserved=True,
            original01_failure_SHA256=sha(a.runtime/'failure.json'),original_runtime_proof_SHA256=sha(old_path),
            application_binary_SHA256=old['binary_SHA256'],app_build_receipt_SHA256=sha(before['app_build_receipt']['path']),
            original_driver_SHA256=old['driver_SHA256'],original_manifest_SHA256=old['manifest_SHA256'],
            source_snapshots_before_SHA256=sha(a.runtime/'inputs-before.json'),source_snapshots_after_SHA256=sha(a.runtime/'inputs-after.json'),
            inputs_current_and_before_after_byte_equal=True,correction_plan_SHA256=sha(ROOT/'plan02.json'),
            correction_manifest_SHA256=sha(ROOT/'manifest02.json'),checker_SHA256=sha(ROOT/'verify_cells.py'),
            cell_source_proof=result,visual_review_root_reported=True,independent_child_visual_review=False,
            auditor_SHA256=sha(__file__))
        write(a.out/'summary.json',value)
        print(json.dumps(dict(summary=str(a.out/'summary.json'),SHA256=sha(a.out/'summary.json'),passed=True,native_rerun=False)))
    except Exception:
        write(a.out/'failure.json',dict(failure=traceback.format_exc(),no_success_certificate=True));raise
if __name__=='__main__':main()
