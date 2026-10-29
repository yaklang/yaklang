"""Freeze raw packet and independent core-field requirements before a KU fix.

This deliberately does not claim to precompute new HTTP/3 stream state. Other
connections and the target's prefix before its first epoch-2 packet must retain
entire literal event objects. All remaining events require source-exact packet
ownership, independent AEAD/frame fields and explicit application-state audit.
"""
import argparse,base64,copy,json,sys
from pathlib import Path
from recovery_probe import file_sha,sha,write

def run(a):
    sys.path.insert(0,str(a.audit));from vxlan_endpoint_projection_audit import capture_records
    source_audit=json.loads((a.certificate/'summary.json').read_text('utf8'))
    assert source_audit['unopened_source_valid_packets']==105
    plan=json.loads((a.plan/'plan-before.json').read_text('utf8'))
    baseline=a.actual/'all-client-middle-epoch-omitted'
    events=[json.loads(x)for x in(baseline/'events.jsonl').read_text('utf8').splitlines()]
    byid={e['id']:e for e in events};assert len(byid)==len(events)==1880
    source=list(capture_records(Path(plan['source_capture'])))
    witnesses=json.loads((a.certificate/'unopened-source-packets.json').read_text('utf8'))
    first=min(w['PDU']for w in witnesses);target=plan['target_oracle_connection']+1
    prior=[e for e in events if e['id']<first and e['flow']==target and e['direction']==0 and (e.get('session')or{}).get('Authentication Verified')is True and (e.get('session')or{}).get('Packet Name')=='1-RTT']
    largest=max(e['session']['Packet Number']for e in prior)
    requirement=[]
    for witness in witnesses:
        r=witness['source_packet'];e=byid[witness['PDU']]
        assert e['flow']==target and e['direction']==0
        raw=source[r['source_frame']-1]['wire'][r['packet_offset']:r['packet_end']]
        assert sha(raw)==r['packet_SHA256']and base64.b64decode(e['raw'])==raw
        assert r['packet_number']>largest
        frozen={k:copy.deepcopy(e[k])for k in ('id','flow','direction','domain','source','destination','source_bytes','raw','response_to','transaction','transport')}
        requirement.append(dict(PDU=e['id'],source_frame=r['source_frame'],frame=witness['frame'],immutable_event_keys=frozen,raw_packet_base64=base64.b64encode(raw).decode(),wire_SHA256=sha(raw),independent_authenticated_PN=r['packet_number'],independent_epoch=r['epoch'],independent_KeyPhase=r['key_phase'],independent_PN_width=r['packet_number_bytes'],plaintext_length=r['plaintext_bytes'],plaintext_SHA256=r['plaintext_SHA256'],expected_wire_frame_core=witness['expected_core_frames'],requires_authentication_true=True,requires_no_authentication_failure=True,requires_no_peer_identity_claim=True))
    literal=[e for e in events if e['flow']!=target or e['id']<first]
    remainder=[e['id']for e in events if e not in literal and e['id']not in {r['PDU']for r in requirement}]
    a.out.mkdir(exist_ok=False)
    write(a.out/'recovery-core-requirements.json',requirement)
    write(a.out/'whole-literal-events.json',literal)
    summary=dict(scope=__doc__,script_SHA256=file_sha(__file__),source_plan_SHA256=file_sha(a.plan/'plan-before.json'),baseline_actual_source=json.loads((baseline/'producer.json').read_text('utf8'))['source'],baseline_producer_SHA256=file_sha(baseline/'producer.json'),baseline_export_SHA256=file_sha(baseline/'events.jsonl'),source_certificate_SHA256=file_sha(a.certificate/'summary.json'),source_witnesses_SHA256=file_sha(a.certificate/'unopened-source-packets.json'),required_recoverable_source_packets=105,first_recoverable_PDU=first,last_preceding_authenticated_PN=largest,whole_literal_required_events=len(literal),whole_literal_events_SHA256=file_sha(a.out/'whole-literal-events.json'),recovery_requirements_SHA256=file_sha(a.out/'recovery-core-requirements.json'),target_remaining_events_requiring_explicit_state_review=remainder,precomputed_whole_changed_object_claim=False,full_HTTP3_recovery_claim=False)
    write(a.out/'summary.json',summary);print(json.dumps(summary))
if __name__=='__main__':
    p=argparse.ArgumentParser(description=__doc__)
    for n in ('plan','actual','certificate','audit','out'):p.add_argument('--'+n,type=Path,required=True)
    run(p.parse_args())
