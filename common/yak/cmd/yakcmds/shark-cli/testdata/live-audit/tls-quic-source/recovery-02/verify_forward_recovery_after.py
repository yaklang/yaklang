"""Check a future native replay against frozen KU recovery requirements.

The source audit separately verifies every packet's ownership and independent
AEAD/frame bytes. This bridge requires literal full-object preservation for
1658 frozen events, immutable ownership for105 recoverable packets, and records
every remaining full-object delta without normalizing keys, types or IDs.
"""
import argparse,collections,copy,json
from pathlib import Path
from recovery_probe import file_sha,write
from sparse_epoch_packet_audit import exact_core,verify_receipt

def typed_differences(a,b,path=''):
    if type(a)is not type(b):return [{'path':path,'before':a,'after':b,'kind':'type'}]
    if isinstance(a,dict):
        out=[]
        for k in sorted(a.keys()|b.keys()):
            at=path+'/'+k
            if k not in a:out.append({'path':at,'after':b[k],'kind':'added'})
            elif k not in b:out.append({'path':at,'before':a[k],'kind':'removed'})
            else:out.extend(typed_differences(a[k],b[k],at))
        return out
    if isinstance(a,list):
        if len(a)!=len(b):return [{'path':path,'before':a,'after':b,'kind':'list-extent-or-order'}]
        return[d for i,(x,y)in enumerate(zip(a,b))for d in typed_differences(x,y,path+'/'+str(i))]
    return []if a==b else[{'path':path,'before':a,'after':b,'kind':'value'}]

def run(a):
    frozen=json.loads((a.plan/'summary.json').read_text('utf8'))
    reqpath=a.plan/'recovery-core-requirements.json';literalpath=a.plan/'whole-literal-events.json'
    assert file_sha(reqpath)==frozen['recovery_requirements_SHA256']and file_sha(literalpath)==frozen['whole_literal_events_SHA256']
    requirements=json.loads(reqpath.read_text('utf8'));literal=json.loads(literalpath.read_text('utf8'))
    assert len(requirements)==105 and len(literal)==1658
    source_plan=json.loads((a.source_plan/'plan-before.json').read_text('utf8'));case=source_plan['cases'][1]
    before=a.before/case['case'];after=a.after/case['case']
    old_proof=verify_receipt(before,case['capture'],case['keylog']);new_proof=verify_receipt(after,case['capture'],case['keylog'])
    assert file_sha(before/'events.jsonl')==frozen['baseline_export_SHA256']
    assert file_sha(before/'producer.json')==frozen['baseline_producer_SHA256']
    old=[json.loads(x)for x in(before/'events.jsonl').read_text('utf8').splitlines()]
    new=[json.loads(x)for x in(after/'events.jsonl').read_text('utf8').splitlines()]
    assert len(old)==len(new)==1880
    oldids=[e['id']for e in old];assert oldids==[e['id']for e in new]and len(set(oldids))==1880
    byid={e['id']:e for e in new};oldbyid={e['id']:e for e in old}
    for e in literal:assert type(byid[e['id']])is type(e)and not typed_differences(e,byid[e['id']]),('whole literal event changed',e['id'])
    recovered=[];fields=0
    for r in requirements:
        e=byid[r['PDU']]
        for key,value in r['immutable_event_keys'].items():assert not typed_differences(value,e[key]),(e['id'],key)
        assert e['raw']==r['raw_packet_base64']
        s=e['session'];assert s['Authentication Verified']is True and s.get('Authentication Failure')is None
        assert s['Peer Identity Verified']is False and s['Packet Number']==r['independent_authenticated_PN']
        assert s['Key Phase']==r['independent_KeyPhase']and s['Packet Number Length']==r['independent_PN_width']
        assert e['admission']=='authenticated-wire-and-observed-CID'
        fields+=exact_core(s.get('Frames',[]),r['expected_wire_frame_core'])
        recovered.append(dict(PDU=e['id'],source_frame=r['source_frame'],source_PN=r['independent_authenticated_PN'],protocol=e['protocol'],status=e['status'],expert=e.get('expert'),full_object_delta=typed_differences(oldbyid[e['id']],e)))
    remaining=[]
    for n in frozen['target_remaining_events_requiring_explicit_state_review']:
        e=byid[n];b=oldbyid[n]
        # These 117 reverse packets already passed source AEAD in the baseline.
        for key in ('id','flow','direction','domain','source','destination','source_bytes','raw','transport'):
            assert not typed_differences(b[key],e[key]),(n,key)
        assert e['session']['Authentication Verified']is True
        for key in ('Packet Number','Key Phase','Packet Number Length','Cipher'):
            assert not typed_differences(b['session'][key],e['session'][key]),(n,key)
        remaining.append(dict(PDU=n,full_object_unchanged=not typed_differences(b,e),full_object_delta=typed_differences(b,e)))
    a.out.mkdir(exist_ok=False);write(a.out/'recovered-packet-whole-deltas.json',recovered);write(a.out/'remaining-target-whole-deltas.json',remaining)
    summary=dict(scope=__doc__,auditor_SHA256=file_sha(__file__),frozen_plan_SHA256=file_sha(a.plan/'summary.json'),before_source=old_proof['source'],after_source=new_proof['source'],before_producer_SHA256=file_sha(before/'producer.json'),after_producer_SHA256=file_sha(after/'producer.json'),same_PDU_count_and_ID_order=True,whole_literal_objects=1658,recovered_independently_AEAD_source_packets=105,recovered_exact_core_wire_fields=fields,remaining_target_events=117,remaining_target_whole_objects_unchanged=sum(r['full_object_unchanged']for r in remaining),remaining_target_whole_deltas_SHA256=file_sha(a.out/'remaining-target-whole-deltas.json'),recovered_whole_deltas_SHA256=file_sha(a.out/'recovered-packet-whole-deltas.json'),whole_new_HTTP3_state_precomputed_claim=False,full_application_recovery_claim=False)
    write(a.out/'summary.json',summary);print(json.dumps({'summary':str(a.out/'summary.json'),'SHA256':file_sha(a.out/'summary.json'),'recovered':105,'fields':fields,'remaining_unchanged':summary['remaining_target_whole_objects_unchanged']}))
if __name__=='__main__':
    p=argparse.ArgumentParser(description=__doc__)
    for n in ('plan','source-plan','before','after','out'):p.add_argument('--'+n,type=Path,required=True)
    run(p.parse_args())
