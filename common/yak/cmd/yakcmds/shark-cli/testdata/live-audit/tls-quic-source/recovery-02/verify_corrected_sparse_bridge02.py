"""Verify explicit source-corrected KU bridge without normalizing actual output.

The original failed bridge remains a separate immutable artifact. The correction
script reads only source ciphertext and the old baseline; it independently
derives completion frames, client bidi HEADERS wire anchors and observed FINs.
All117 remaining target objects are compared whole to that exact expectation.
"""
import argparse,collections,json
from pathlib import Path
from recovery_probe import file_sha,write
from sparse_epoch_packet_audit import exact_core,verify_receipt
from verify_forward_recovery_after import typed_differences

def run(a):
    frozen=json.loads((a.original_plan/'summary.json').read_text('utf8'))
    correction=json.loads((a.correction/'summary.json').read_text('utf8'))
    assert correction['original_frozen_plan_SHA256']==file_sha(a.original_plan/'summary.json')
    assert correction['candidate_output_read']is False
    assert correction['corrected_core_requirements_SHA256']==file_sha(a.correction/'recovery-core-requirements.json')
    assert correction['remaining_whole_expected_SHA256']==file_sha(a.correction/'remaining-target-whole-expected.json')
    requirements=json.loads((a.correction/'recovery-core-requirements.json').read_text('utf8'))
    remaining_expected=json.loads((a.correction/'remaining-target-whole-expected.json').read_text('utf8'))
    literal=json.loads((a.original_plan/'whole-literal-events.json').read_text('utf8'))
    assert len(requirements)==105 and len(remaining_expected)==117 and len(literal)==1658
    plan=json.loads((a.source_plan/'plan-before.json').read_text('utf8'));case=plan['cases'][1]
    before=a.before/case['case'];after=a.after/case['case']
    oldproof=verify_receipt(before,case['capture'],case['keylog']);newproof=verify_receipt(after,case['capture'],case['keylog'])
    assert file_sha(before/'events.jsonl')==frozen['baseline_export_SHA256']
    old=[json.loads(x)for x in(before/'events.jsonl').read_text('utf8').splitlines()]
    new=[json.loads(x)for x in(after/'events.jsonl').read_text('utf8').splitlines()]
    assert len(old)==len(new)==1880 and[e['id']for e in old]==[e['id']for e in new]
    byid={e['id']:e for e in new};oldbyid={e['id']:e for e in old}
    for e in literal:assert not typed_differences(e,byid[e['id']]),('literal object changed',e['id'])
    recovered=[];fields=0;query=0;blocked=0;transactions=collections.Counter()
    for r in requirements:
        e=byid[r['PDU']]
        for key,value in r['immutable_event_keys'].items():assert not typed_differences(value,e[key]),(e['id'],key)
        s=e['session'];assert s['Authentication Verified']is True and s.get('Authentication Failure')is None
        assert s['Peer Identity Verified']is False and s['Packet Number']==r['independent_authenticated_PN']
        assert s['Key Phase']==r['independent_KeyPhase']and s['Packet Number Length']==r['independent_PN_width']
        assert e['admission']=='authenticated-wire-and-observed-CID'and e['status']=='decoded'
        assert e['field_values']['source']==e['source_bytes']and e['field_values']['pdu']==e['id']
        for name,value,origin in [('pdu.completed_in',r['frame'],'event'),('quic.packet_number',r['independent_authenticated_PN'],'derived'),('quic.packet.authenticated',True,'derived')]:
            got=[f for f in e['field_values']['values']if f['name']==name]
            expected={'name':name,'value':value,'origin':origin}
            if type(value)is int:expected['exact_value']=str(value)
            assert got==[expected]and type(got[0]['value'])is type(value);query+=1
        fields+=exact_core(s['Frames'],r['expected_wire_frame_core'])
        # The captured HEADERS wire is real; missing dynamic encoder state must
        # remain blocked. An AEAD success is not a decoded method/header block.
        assert not s.get('Headers')and not s.get('QPACK Resumed Streams')
        assert all(not fr.get('HTTP3 Frames')for fr in s['Frames'])
        if s.get('QPACK Blocked'):blocked+=1
        transactions[e['transaction']]+=1
        recovered.append(dict(PDU=e['id'],source_frame=r['source_frame'],source_PN=r['independent_authenticated_PN'],whole_object_delta=typed_differences(oldbyid[e['id']],e)))
    changes=[]
    for expected in remaining_expected:
        e=byid[expected['id']];differences=typed_differences(expected,e)
        assert not differences,('remaining whole object differs',e['id'],differences[:3])
        before_delta=typed_differences(oldbyid[e['id']],e)
        if before_delta:changes.append(dict(PDU=e['id'],whole_object_delta=before_delta))
    assert len(changes)==53 and blocked==43
    # The complete real network sample is required to remain byte-identical.
    base=plan['cases'][0]
    verify_receipt(a.before/base['case'],base['capture'],base['keylog']);verify_receipt(a.after/base['case'],base['capture'],base['keylog'])
    assert file_sha(a.before/base['case']/'events.jsonl')==file_sha(a.after/base['case']/'events.jsonl')
    a.out.mkdir(exist_ok=False);write(a.out/'recovered105-whole-deltas.json',recovered);write(a.out/'remaining53-whole-deltas.json',changes)
    summary=dict(scope=__doc__,auditor_SHA256=file_sha(__file__),source_correction_SHA256=file_sha(a.correction/'summary.json'),original_failure_SHA256=file_sha(a.failure),before_source=oldproof['source'],after_source=newproof['source'],before_producer_SHA256=file_sha(before/'producer.json'),after_producer_SHA256=file_sha(after/'producer.json'),original_real_network_2051_entire_export_byte_identical=True,sparse_packets=1880,unchanged_original_literal_objects=1658,source_recovered_packets=105,source_recovered_core_frame_fields=fields,recovered_exact_query_fields=query,recovered_QPACK_blocked_observations=blocked,recovered_source_STREAM_anchor_transactions=dict(transactions),remaining_whole_expected_events=117,remaining_whole_changed_objects=53,remaining_whole_literal_objects=64,whole_changed_remaining_differences=0,sparse_whole_literal_objects_total=1722,ID_mapping=False,key_or_type_or_array_order_normalization=False,correction_reads_candidate_output=False,source_completion_and_STREAM_anchors_explicitly_corrected=True,decoded_dynamic_request_header_semantics_or_full_HTTP3_recovery_claim=False,recovered105_whole_deltas_SHA256=file_sha(a.out/'recovered105-whole-deltas.json'),remaining53_whole_deltas_SHA256=file_sha(a.out/'remaining53-whole-deltas.json'))
    write(a.out/'summary.json',summary);print(json.dumps({'summary':str(a.out/'summary.json'),'SHA256':file_sha(a.out/'summary.json'),'whole_literal':1722,'source_recovered':105,'remaining_whole_source_changes':53,'blocked':43}))
if __name__=='__main__':
    p=argparse.ArgumentParser(description=__doc__)
    for n in ('original-plan','correction','source-plan','before','after','failure','out'):p.add_argument('--'+n,type=Path,required=True)
    run(p.parse_args())
