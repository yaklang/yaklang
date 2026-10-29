"""Compare every API and ProtocolStats object to the pre-frozen full expectation."""
import argparse,hashlib,json,traceback,sys
from pathlib import Path
FROZEN01=Path(r'C:\tmp\yak-shark-ESP-SSH-UI-20261028\ESP-NATT-whole-plan-before-01')
sys.path.insert(0,str(FROZEN01))
from natt_expected import same

def sha(p):
    h=hashlib.sha256()
    with Path(p).open('rb') as f:
        for b in iter(lambda:f.read(1<<20),b''):h.update(b)
    return h.hexdigest()
def read(p):return json.loads(Path(p).read_text(encoding='utf-8-sig'))
def write(p,v):p.write_text(json.dumps(v,ensure_ascii=False,indent=2)+'\n',encoding='utf8')
def main():
    p=argparse.ArgumentParser(description=__doc__)
    for name in ['plan','after-probe','out']:p.add_argument('--'+name,type=Path,required=True)
    p.add_argument('--after-source',required=True);a=p.parse_args();a.out.mkdir(exist_ok=False)
    try:
        plan=read(a.plan)
        assert plan['after_output_read']is False and plan['expected_whole_objects_available']is True
        assert plan['source_equivalence_proves_only_docs_audit_checkpoint']is True
        assert sha(plan['source_equivalence'])==plan['source_equivalence_SHA256']
        equivalent=read(plan['source_equivalence'])
        assert equivalent['production_code_equivalent_for_native_stats_API_probe']is True
        assert sha(equivalent['git_evidence_file'])==equivalent['git_evidence_SHA256']
        assert sha(plan['original_native_plan'])==plan['original_native_plan_SHA256']
        assert sha(plan['before_probe_producer'])==plan['before_probe_producer_SHA256']
        assert sha(plan['before_probe_export'])==plan['before_probe_export_SHA256']
        expectation=a.plan.parent/plan['expected_objects'];assert sha(expectation)==plan['expected_objects_SHA256']
        producer_path=a.after_probe/'producer.json';producer=read(producer_path);data=a.after_probe/'probe.jsonl'
        assert producer['source']==a.after_source and producer['exit_code']==0
        assert producer['sources_byte_equal']is True and producer['source_before']==producer['source_after']
        assert producer['probe_source_SHA256']==plan['probe_source_SHA256']
        assert producer['auditor_SHA256']==plan['probe_runner_SHA256']
        assert producer['export_SHA256']==sha(data)
        previous=read(plan['before_probe_producer'])
        assert producer['arguments'][1:]==previous['arguments'][1:]
        for key in ['capture_SHA256','keylog_SHA256','explicit_keylog_used','ignore_embedded_secrets','AllowIncomplete']:
            assert producer[key]==previous[key]
        actual=[json.loads(line) for line in data.open(encoding='utf-8-sig')];expected=read(expectation)
        assert same(actual,expected),'Complete API/stats object or source/order differs; no changed-path allowances'
        summary=dict(scope=__doc__,after_source=a.after_source,plan_SHA256=sha(a.plan),
            producer_SHA256=sha(producer_path),export_SHA256=sha(data),expected_objects_SHA256=sha(expectation),
            complete_API_objects=7,complete_ProtocolStats_objects=1,whole_probe_objects=8,unplanned_differences=0,
            ID_mapping=False,auditor_SHA256=sha(__file__))
        write(a.out/'summary.json',summary)
        print(json.dumps(dict(summary=str(a.out/'summary.json'),SHA256=sha(a.out/'summary.json'),objects=8,unplanned_differences=0)))
    except Exception:
        write(a.out/'failure.json',dict(failure=traceback.format_exc(),no_success_certificate=True));raise
if __name__=='__main__':main()
