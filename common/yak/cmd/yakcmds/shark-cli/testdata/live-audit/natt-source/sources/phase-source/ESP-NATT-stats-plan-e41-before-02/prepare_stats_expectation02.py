"""Freeze full after API/stats objects using only the closed OLD probe output."""
import argparse,copy,hashlib,json,traceback,sys
from pathlib import Path
FROZEN01=Path(r'C:\tmp\yak-shark-ESP-SSH-UI-20261028\ESP-NATT-whole-plan-before-01')
sys.path.insert(0,str(FROZEN01))
from natt_expected import same,protocol_stats,canonical,sha_bytes

def sha(p):
    h=hashlib.sha256()
    with Path(p).open('rb') as f:
        for b in iter(lambda:f.read(1<<20),b''):h.update(b)
    return h.hexdigest()
def read(p):return json.loads(Path(p).read_text(encoding='utf-8-sig'))
def write(p,v):p.write_text(json.dumps(v,ensure_ascii=False,indent=2)+'\n',encoding='utf8')
def old_api(row):
    e=row['baseline_native_event'];ref=row['expected_API_contract']
    return {'ID':e['id'],'FlowID':e['flow'],'Protocol':e['protocol'],'Transport':e['transport'],
        'Profile':e['profile'],'Admission':e['admission'],'Rule':'','Entry':e['entry'],'Status':e['status'],
        'Completeness':e['completeness'],'Error':'','ExpertCode':e['expert'],'Length':96,
        'TimestampNS':ref['TimestampNS'],'Source':e['source'],'Destination':e['destination'],'Direction':e['direction'],
        'Domain':copy.deepcopy(e['domain']),'CarrierLayers':copy.deepcopy(e['carrier_layers']),
        'TransactionID':e['transaction'],'ResponseTo':e['response_to'],'TCPFlowID':0,
        'SourceBytes':copy.deepcopy(e['source_bytes']),'Raw':e['raw'],'Summary':e['summary'],
        'Session':None,'GetFields':None,'GetFieldsError':e['fields_error'],'Decode':None,'DecodeError':e['fields_error'],
        'DisplayFieldValues':copy.deepcopy(e['field_values'])}
def main():
    p=argparse.ArgumentParser(description=__doc__)
    for name in ['plan','before-probe','source-equivalence','out']:p.add_argument('--'+name,type=Path,required=True)
    a=p.parse_args();a.out.mkdir(exist_ok=False)
    try:
        plan=read(a.plan);frozen_path=a.plan.parent/'seven-complete-native-expectations.json'
        assert sha(frozen_path)==plan['outputs'][frozen_path.name]
        frozen=read(frozen_path);by_frame={r['frame']:r for r in frozen}
        producer_path=a.before_probe/'producer.json';producer=read(producer_path);data_path=a.before_probe/'probe.jsonl'
        equivalent=read(a.source_equivalence)
        assert equivalent['baseline_source'] in [plan['native_baseline_source'],plan['CLI_baseline_source']]
        assert equivalent['source']==producer['source']
        assert equivalent['all_changes_are_documentation_or_standalone_audit_sources']is True
        assert equivalent['production_code_equivalent_for_native_stats_API_probe']is True
        assert equivalent['baseline_source_metadata_not_rewritten']is True
        assert sha(equivalent['git_evidence_file'])==equivalent['git_evidence_SHA256']
        assert all(item['equal']is True and item['before_tree']==item['source_tree'] for item in equivalent['production_trees'].values())
        assert set(equivalent['production_trees'])=={'common/pcapx/pcaputil','common/bin-parser'}
        assert equivalent['changed_files'] and all(path.startswith('docs/') or path.startswith('common/yak/cmd/yakcmds/shark-cli/testdata/live-audit/') for path in equivalent['changed_files'])
        assert producer['exit_code']==0 and producer['sources_byte_equal']is True and producer['source_before']==producer['source_after']
        assert producer['export_SHA256']==sha(data_path)
        helper=FROZEN01/'stats_api_probe.go';runner=FROZEN01/'run_probe.py'
        assert producer['probe_source_SHA256']==sha(helper) and producer['auditor_SHA256']==sha(runner)
        actual=[json.loads(r) for r in data_path.open(encoding='utf-8-sig')]
        assert len(actual)==8
        expected=[];seen=set()
        for old in actual[:-1]:
            assert set(old)=={'kind','frame','event'} and old['kind']=='selected' and old['frame'] not in seen
            row=by_frame[old['frame']];seen.add(old['frame'])
            assert same(old['event'],old_api(row)),('old API differs from source-bound baseline',old['frame'])
            expected.append(dict(kind='selected',frame=old['frame'],event=row['expected_API_contract']))
        assert seen==by_frame.keys()
        last=actual[-1]
        weber=next(r for r in plan['native'] if r['relative_file']=='preservation/weber/events.jsonl')
        assert set(last)=={'kind','capture_SHA256','observed_event_count','selected_observations','protocol_stats_callbacks','protocol_stats',
            'replay_error','ignore_embedded_secrets','explicit_keylog_used','AllowIncomplete'}
        assert last['kind']=='complete' and last['capture_SHA256']==weber['capture_SHA256']
        assert last['observed_event_count']==weber['baseline_rows'] and last['selected_observations']==7 and last['protocol_stats_callbacks']==1
        assert last['replay_error']=='' and last['ignore_embedded_secrets']is True and last['explicit_keylog_used']is True and last['AllowIncomplete']is True
        new_last=copy.deepcopy(last);new_last['protocol_stats']=protocol_stats(last['protocol_stats']);expected.append(new_last)
        write(a.out/'expected-probe-whole-objects.json',expected)
        summary=dict(scope=__doc__,after_output_read=False,expected_whole_objects_available=True,
            original_native_plan=str(a.plan),original_native_plan_SHA256=sha(a.plan),seven_native_plan_SHA256=sha(frozen_path),
            before_probe_producer=str(producer_path),before_probe_producer_SHA256=sha(producer_path),
            before_probe_export=str(data_path),before_probe_export_SHA256=sha(data_path),
            expected_objects='expected-probe-whole-objects.json',expected_objects_SHA256=sha(a.out/'expected-probe-whole-objects.json'),
            before_complete_object=last,expected_after_complete_object=new_last,
            probe_source_SHA256=sha(helper),probe_runner_SHA256=sha(runner),auditor_SHA256=sha(__file__),
            source_equivalence=str(a.source_equivalence),source_equivalence_SHA256=sha(a.source_equivalence),
            source_equivalence_proves_only_docs_audit_checkpoint=True,
            no_ID_mapping=True,no_changed_path_allowance=True,all_other_stats_values_types_literal=True)
        write(a.out/'plan.json',summary)
        print(json.dumps(dict(plan=str(a.out/'plan.json'),SHA256=sha(a.out/'plan.json'),objects=8,after_output_read=False)))
    except Exception:
        write(a.out/'failure.json',dict(failure=traceback.format_exc(),no_success_certificate=True));raise
if __name__=='__main__':main()
