"""Verify live-source recovery controls against precomputed independent AEAD.

This tests exact authenticated packet/frame sources, first-bad-tag recovery,
late epoch1 ownership and a late epoch0 duplicate with no frame redelivery.
It does not certify all application/QPACK state or all possible reorder depth.
"""
import argparse,base64,collections,json,sys
from pathlib import Path
from recovery_probe import file_sha,sha,write
from sparse_epoch_packet_audit import exact_core,verify_receipt

def run(a):
    sys.path.insert(0,str(a.audit));from vxlan_endpoint_projection_audit import capture_records
    plan=json.loads((a.plan/'plan-before.json').read_text('utf8'))
    certified=json.loads((a.certificate/'summary.json').read_text('utf8'))
    assert certified['plan_SHA256']==file_sha(a.plan/'plan-before.json')
    source=json.loads((a.source_plan/'plan-before.json').read_text('utf8'))
    source_rows=[json.loads(x)for x in(a.source_plan/'packet-source-plan.jsonl').read_text('utf8').splitlines()]
    source_byframe=collections.defaultdict(list)
    for r in source_rows:source_byframe[r['source_frame']].append(r)
    a.out.mkdir(exist_ok=False);checks=[]
    for case,cert in zip(plan['cases'],certified['results']):
        assert case['case']==cert['case']
        path=a.certificate/(case['case']+'-source-packets.json');assert file_sha(path)==cert['source_packets_SHA256']
        witness=json.loads(path.read_text('utf8'));good={(r['frame'],r['packet_SHA256']):r for r in witness['good']}
        bad={(r['frame'],r['packet_SHA256']):r for r in witness['declared_bad_tags']}
        folder=a.actual/case['case'];proof=verify_receipt(folder,case['capture'],case['keylog'])
        assert proof['plan_SHA256']==file_sha(a.plan/'plan-before.json')
        manifest=json.loads((Path(case['capture']).parent/'manifest.json').read_text('utf8'))
        records=list(capture_records(Path(case['capture'])));expected=[]
        for r,entry in zip(records,manifest['ledger']):
            assert sha(r['wire'])==entry['source_wire_SHA256']and r['time_ns']==entry['source_time_ns']
            for old in source_byframe[entry['source_frame']]:
                raw=r['wire'][old['packet_offset']:old['packet_end']]
                expected.append((entry['number'],entry['source_frame'],raw,old))
        events=[json.loads(x)for x in(folder/'events.jsonl').read_text('utf8').splitlines()]
        assert len(events)==len(expected)
        seen=set();domains={'Section':0,'Interface':0,'Encapsulation':''};flows={};fields=0;old_events=[]
        counts=collections.Counter()
        for e,(frame,source_frame,raw,original)in zip(events,expected):
            assert e['id']not in seen;seen.add(e['id'])
            assert base64.b64decode(e['raw'])==raw and e['source_bytes']['SHA256']==sha(raw)
            assert e['domain']==domains and e['source_bytes']['PacketRefs']==[{'Number':frame,'Domain':domains}]
            assert e['source_bytes']['Kind']=='captured'and e['field_values']['source']==e['source_bytes']
            assert e['direction']==original['direction']and e['source'].endswith(':'+str(original['source_port']))and e['destination'].endswith(':'+str(original['destination_port']))
            flows.setdefault(e['flow'],original['oracle_connection']);assert flows[e['flow']]==original['oracle_connection']
            key=(frame,sha(raw));s=e['session']or{}
            if key in good:
                w=good[key];assert s['Authentication Verified']is True and s['Peer Identity Verified']is False
                assert s['Packet Number']==w['PN']and s['Key Phase']==w['phase']
                assert e['status']=='decoded',('valid source packet refused',e['id'],source_frame,e['status'],e.get('expert'))
                assert e['admission']=='authenticated-wire-and-observed-CID'
                fields+=exact_core(s['Frames'],w['core_frames']);counts['independent_AEAD_short_packets']+=1
                if original['oracle_connection']==0 and original['direction']==0 and original.get('epoch')==2:counts['target_epoch2_authenticated']+=1
                if case['case']=='late-epoch1-and-old-epoch0-duplicate'and frame in(295,296):
                    assert e['flow']==1 and e['direction']==0
                    old_events.append(dict(PDU=e['id'],frame=frame,source_frame=source_frame,PN=s['Packet Number'],wire_phase=s['Key Phase'],Duplicate=s.get('Duplicate',False)))
                    if frame==295:assert source_frame==110 and s['Packet Number']==58 and s['Key Phase']==1
                    else:
                        assert source_frame==108 and s['Packet Number']==57 and s['Key Phase']==0 and s['Duplicate']is True
                        assert all(not fr.get('Delivered Bytes')and not fr.get('HTTP3 Frames')for fr in s['Frames'])
                        counts['late_epoch0_duplicate_without_redelivery']+=1
            elif key in bad:
                assert s['Authentication Verified']is False and s['Authentication Failure']=='packet-protection-failed'
                assert not s.get('Frames')and not s.get('Decrypted')and e['status']=='malformed'
                counts['declared_bad_tag_refused']+=1
            elif original['long_header']is False:
                assert original['packet_number']is None and s.get('Authentication Failure')=='packet-protection-failed'
                assert s.get('Authentication Verified')is False and not s.get('Frames');counts['original_bad_tags_refused']+=1
        assert len(flows)==3 and set(flows.values())=={0,1,2}
        assert counts['independent_AEAD_short_packets']==cert['independently_authenticated_short_occurrences']
        assert counts['declared_bad_tag_refused']==cert['declared_extra_bad_tags']and counts['original_bad_tags_refused']==3
        assert counts['target_epoch2_authenticated']==(104 if case['case']=='first-epoch2-tag-damaged'else 105)
        if case['case']=='late-epoch1-and-old-epoch0-duplicate':assert len(old_events)==2 and counts['late_epoch0_duplicate_without_redelivery']==1
        checks.append(dict(case=case['case'],source=proof['source'],producer_SHA256=file_sha(folder/'producer.json'),packets=len(events),counts=dict(counts),exact_independent_frame_fields=fields,late_source_observations=old_events,native_flow_to_source_connection=flows))
    summary=dict(scope=__doc__,auditor_SHA256=file_sha(__file__),plan_SHA256=file_sha(a.plan/'plan-before.json'),source_certificate_SHA256=file_sha(a.certificate/'summary.json'),checks=checks,full_HTTP3_feature_parity_claim=False)
    write(a.out/'summary.json',summary);print(json.dumps({'summary':str(a.out/'summary.json'),'SHA256':file_sha(a.out/'summary.json'),'checks':checks}))
if __name__=='__main__':
    p=argparse.ArgumentParser(description=__doc__)
    for n in ('plan','source-plan','certificate','actual','audit','out'):p.add_argument('--'+n,type=Path,required=True)
    run(p.parse_args())
