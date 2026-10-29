"""Freeze full expected native objects and all 37 old export/producer bindings.

Only the seven source-proven native ESP objects have a complete new expectation.
CLI originals and independent source facts are frozen; complete new CLI rows
must be supplied in a separate plan before a whole-CLI proof can be issued.
"""
import argparse,base64,collections,copy,hashlib,importlib.util,itertools,json,sys
from pathlib import Path
from natt_expected import event,api_contract,canonical,same,sha_bytes,PROTOCOL_STATS_DELTA,PROTOCOL_STATS_KEYS

def sha(p):
    h=hashlib.sha256()
    with Path(p).open('rb') as f:
        for b in iter(lambda:f.read(1<<20),b''):h.update(b)
    return h.hexdigest()
def read(p):return json.loads(Path(p).read_text(encoding='utf-8-sig'))
def module(name,path):
    spec=importlib.util.spec_from_file_location(name,path);m=importlib.util.module_from_spec(spec);spec.loader.exec_module(m);return m
def write(path,value):path.write_text(json.dumps(value,indent=2,ensure_ascii=False,allow_nan=False)+'\n',encoding='utf8')

def main():
    p=argparse.ArgumentParser(description=__doc__)
    for name in ['before-native','before-cli','source-proof','audit-modules','out']:p.add_argument('--'+name,type=Path,required=True)
    a=p.parse_args();a.out.mkdir(exist_ok=False)
    nmod=module('native_bindings',a.audit_modules/'public_native_literal_continuity.py')
    cmod=module('cli_bindings',a.audit_modules/'public_cli_literal_continuity.py')
    ns,ne=nmod.executions(a.before_native);cs,ce,csummarysha=cmod.executions(a.before_cli)
    proof_manifest=read(a.source_proof/'manifest01.json')
    assert proof_manifest['frozen_after_delivery'] is True
    for key,item in proof_manifest['files'].items():assert sha(a.source_proof/key)==item['SHA256'],key
    facts=read(a.source_proof/'source02/source-facts.json')
    all_natt=read(a.source_proof/'source03/all-34-NATT-source-facts.json')
    before_seven=read(a.source_proof/'source03/all-7-ESP-before-whole-source-plan.json')
    assert len(before_seven)==7 and len(all_natt)==34
    originals={r['frame']:r for r in facts['selected']}
    frames={r['frame'] for r in before_seven};ids={r['before_event']['id'] for r in before_seven}
    frozen=[]
    for r in before_seven:
        frame=r['frame'];source=originals[frame];wire=(a.source_proof/'source02'/source['raw_frame_file']).read_bytes()
        assert sha_bytes(wire)==source['frame_wire_SHA256']
        lo,hi=source['source']['UDP']['payload_range'];esp=wire[lo:hi]
        assert source['source']['UDP']['complete'] and source['source']['UDP']['parent_extent_valid']
        assert esp==base64.b64decode(r['source_full_ESP_base64'],validate=True)
        assert sha_bytes(esp)==r['source_full_ESP_SHA256']
        expected=event(r['before_event'],esp)
        frozen.append(dict(frame=frame,baseline_native_event=copy.deepcopy(r['before_event']),expected_native_event=expected,
            expected_native_event_canonical_SHA256=sha_bytes(canonical(expected)),
            expected_API_contract=api_contract(r['before_event'],esp,source['time_ns']),
            independent_source={k:source[k] for k in ['section','interface','link','captured_length','original_length','time_ns','frame_wire_SHA256']},
            ESP_wire_range=[lo,hi],ESP_wire_SHA256=sha_bytes(esp),ESP_wire_base64=base64.b64encode(esp).decode(),
            frame_wire_file=str(a.source_proof/'source02'/source['raw_frame_file']),
            frozen_CLI_row=None,frozen_CLI_line_SHA256=None,
            CLI_complete_expected_status='pending independently frozen presentation contract; never infer from future actual output'))
    frozen_by_id={r['baseline_native_event']['id']:r for r in frozen}
    native=[]
    for key,value in ne.items():
        relative=Path(key).as_posix();path=a.before_native/Path(key);pr=value['producer'];kind=value['kind']
        item=dict(relative_file=relative,kind=kind,baseline_file=str(path),baseline_SHA256=value['whole_export_SHA256'],
            baseline_rows=value['rows'],baseline_producer=str(path.with_name(kind+'-producer.json')),
            baseline_producer_SHA256=value['receipt_SHA256'],baseline_source=ns['source'],
            capture=pr['capture'],capture_SHA256=pr['capture_SHA256'],arguments_tail=pr['arguments'][1:],
            keylog_used_recorded=pr['keylog_used'],expected_changed_objects=7 if relative=='preservation/weber/events.jsonl' else 0)
        if relative=='preservation/weber/events.jsonl':
            digest=hashlib.sha256();matched=set();row_count=0
            for ordinal,raw in enumerate(path.open('rb'),1):
                old=json.loads(raw);expected=old
                if old['id'] in frozen_by_id:
                    row=frozen_by_id[old['id']]
                    assert same(old,row['baseline_native_event'])
                    assert row['frame'] in {x['Number'] for x in old['source_bytes']['PacketRefs']}
                    row['baseline_native_ordinal']=ordinal;row['baseline_native_line_SHA256']=sha_bytes(raw)
                    expected=row['expected_native_event'];matched.add(old['id'])
                digest.update(canonical(expected)+b'\n');row_count+=1
            assert matched==ids and row_count==value['rows']
            item['expected_whole_export_canonical_SHA256']=digest.hexdigest()
        native.append(item)
    cli=[];matched_frames=set()
    for key,value in ce.items():
        relative=(Path(key)/'yak.jsonl').as_posix();path=a.before_cli/Path(key)/'yak.jsonl';pr=value['receipt']
        cli.append(dict(relative_file=relative,kind='CLI',baseline_file=str(path),baseline_SHA256=value['whole_export_SHA256'],
            baseline_rows=value['rows'],baseline_producer=str(path.with_name('yak-producer.json')),
            baseline_producer_SHA256=value['receipt_SHA256'],baseline_source=cs['source'],
            capture=pr['capture'],capture_SHA256=pr['capture_SHA256'],arguments_tail=pr['arguments'][1:],
            keylog_used_recorded=pr['keylog_used'],expected_changed_objects=7 if relative=='preservation/weber/yak.jsonl' else 0))
        if relative=='preservation/weber/yak.jsonl':
            for ordinal,raw in enumerate(path.open('rb'),1):
                value=json.loads(raw);number=value.get('number')
                if number not in frames:continue
                row=next(r for r in frozen if r['frame']==number)
                assert number not in matched_frames;matched_frames.add(number)
                assert value['length']==row['independent_source']['original_length']
                assert value['captured_length']==row['independent_source']['captured_length']
                assert value['capture_domain']==row['baseline_native_event']['domain']
                assert len(value['pdus'])==1 and value['pdus'][0]['id']==row['baseline_native_event']['id']
                row['frozen_CLI_row']=value;row['frozen_CLI_line_SHA256']=sha_bytes(raw);row['baseline_CLI_ordinal']=ordinal
    assert matched_frames==frames and len(native)==39 and len(cli)==37
    write(a.out/'seven-complete-native-expectations.json',frozen)
    # Close the whole-object constructor against intentional coherent failures.
    controls=[]
    seed=frozen[0]['expected_native_event']
    for name,change in [
        ('extra-SA-context',lambda v:v['session'].__setitem__('SA',1)),
        ('authenticate-every-view',lambda v:(v['session'].__setitem__('Authentication Verified',True),v['fields'].__setitem__('Authentication Verified',True))),
        ('opaque-tail-first64-preview',lambda v:v.__setitem__('raw',base64.b64encode(base64.b64decode(v['raw'])[:64]).decode())),
        ('wrong-capture-domain',lambda v:v['source_bytes']['PacketRefs'][0]['Domain'].__setitem__('Interface',274)),
        ('string-sequence',lambda v:v['session'].__setitem__('Sequence',str(v['session']['Sequence']))),
        ('SPI-seq-swapped-range',lambda v:v['field_values']['values'][-1].__setitem__('pdu_range',{'offset':4,'length':4})),
        ('extra-error-key',lambda v:v.__setitem__('error','')),
        ('new-flow',lambda v:v.__setitem__('flow',1)),
        ('reordered-occurrences',lambda v:v['field_values']['values'].reverse())]:
        changed=copy.deepcopy(seed);change(changed);assert not same(changed,seed);controls.append(name)
    stats_plan=dict(ProtocolStats_source='common/pcapx/pcaputil/bin_parser.go:ProtocolStats and stats()',
        keys=sorted(PROTOCOL_STATS_KEYS),expected_delta=PROTOCOL_STATS_DELTA,
        baseline_absolute_stats_available=False,whole_after_stats_available=False,
        reason='37 event helpers do not subscribe/export ProtocolStats; two plain flow observers export TCPReassemblyStats and do not enable the protocol parser',
        plain_flow_observer_all_objects_including_stats_literal_unchanged=True,
        enabled_parser_other_helpers=['flow-admission','replay-clock'],
        enabled_parser_other_helpers_not_part_of_39_native_export_runs=True,
        required_next_step='Run frozen same-source stats/API probe against old binary and new binary; freeze full old and expected new stats from old output plus five source deltas, before reading new output')
    write(a.out/'protocol-stats-plan.json',stats_plan)
    template=dict(scope='Complete new CLI rows remain OPEN until presentation contract is separately source-frozen.',
        complete_expected_available=False,baseline_CLI_source=cs['source'],baseline_CLI_runtime_SHA256=csummarysha,
        seven_native_plan_SHA256=sha(a.out/'seven-complete-native-expectations.json'),
        required_complete_rows=[dict(frame=r['frame'],baseline_ordinal=r['baseline_CLI_ordinal'],baseline_line_SHA256=r['frozen_CLI_line_SHA256'],
            baseline_whole_row=r['frozen_CLI_row'],expected_whole_row=None) for r in frozen],
        independent_known_facts='Original frame/domain/clock/endpoints/lengths and source-derived native PDU fields are frozen in seven-complete-native-expectations; no future CLI label/stack/summary guessed',
        verifier_must_refuse_whole_CLI_certificate_until_complete=True)
    write(a.out/'CLI-whole-plan-template.json',template)
    binding_files=[Path(__file__),Path(__file__).with_name('natt_expected.py'),a.audit_modules/'public_native_literal_continuity.py',
        a.audit_modules/'public_cli_literal_continuity.py',a.source_proof/'manifest01.json',a.before_native/'summary.json',a.before_cli/'summary.json',
        a.audit_modules/'events/main.go',a.audit_modules/'flow-observer/main.go',a.audit_modules/'flow-admission/main.go',a.audit_modules/'replay-clock/main.go']
    report=dict(scope=__doc__,native_baseline_source=ns['source'],CLI_baseline_source=cs['source'],
        baseline_native_root=str(a.before_native),baseline_CLI_root=str(a.before_cli),
        native_runtime_SHA256=sha(a.before_native/'summary.json'),CLI_runtime_SHA256=csummarysha,
        independent_source_manifest_SHA256=sha(a.source_proof/'manifest01.json'),
        native_inputs=37,native_closed_exports=39,CLI_inputs=37,native_event_rows=sum(x['baseline_rows'] for x in native if x['kind']=='events'),
        CLI_whole_rows=sum(x['baseline_rows'] for x in cli),native=native,CLI=cli,
        unchanged_native_objects_except_exact_seven=True,ID_mapping=False,
        negative_constructor_controls=controls,native_new_whole_objects_frozen=True,CLI_new_whole_objects_frozen=False,
        extra_API_contract_not_in_event_export=['Rule','TimestampNS','Length','Decode'],
        source_file_bindings={str(p):sha(p) for p in binding_files},
        outputs={p.name:sha(p) for p in a.out.glob('*.json')})
    write(a.out/'plan.json',report)
    print(json.dumps(dict(plan=str(a.out/'plan.json'),SHA256=sha(a.out/'plan.json'),native_rows=report['native_event_rows'],CLI_rows=report['CLI_whole_rows'],seven_native_complete=True,CLI_complete=False)))
if __name__=='__main__':main()
