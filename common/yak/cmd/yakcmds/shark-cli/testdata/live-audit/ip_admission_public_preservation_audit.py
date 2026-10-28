"""Compare every public application event and physical row after IP admission.

All non-IP events must retain literal wire bytes, contributors, capture domain,
endpoints, native TCP generation, protocol session, fields, types and reasons.
Only generated PDU-ID aliases may change. IP diagnostics are counted separately;
their wire correctness is the responsibility of the independent source audit.
Physical label changes must belong to explicit native IP refusal observations.
This preservation check does not certify every application protocol against
Wireshark, or treat a conservative refusal as a matching reference label.
"""
import argparse,collections,json
from pathlib import Path
from clock_public_preservation_audit import canonical,key,rows,sha,without_generated_id


def audit(capture,before,after,before_cli,after_cli):
    old=collections.defaultdict(collections.deque);old_ip=collections.Counter();new_ip=collections.Counter()
    for event in rows(before):
        if event['protocol']=='ip':old_ip[(event['status'],event.get('expert',''),event.get('error',''))]+=1
        else:old[key(event)].append(event)
    paired=exact=renumbered=0;mapping=[];refused={}
    for event in rows(after):
        if event['protocol']=='ip':
            new_ip[(event['status'],event.get('expert',''),event.get('error',''))]+=1
            if event.get('expert')in['IPVersionMismatch','EncapsulationDepthOrProfile']:
                refs=event['source_bytes'].get('PacketRefs')or[]
                assert len(refs)==1 and refs[0]['Number']not in refused
                refused[refs[0]['Number']]=event['expert']
            continue
        available=old.get(key(event))
        assert available,('non-IP source/bytes/connection identity changed',event['protocol'],event['id'])
        previous=available.popleft();paired+=1
        if canonical(previous)==canonical(event):exact+=1
        else:
            assert canonical(without_generated_id(previous))==canonical(without_generated_id(event)),('a non-generated application value changed',event['protocol'],event['id'])
            assert previous['id']!=event['id'];renumbered+=1;mapping.append((previous['id'],event['id']))
    assert not any(old.values()),'no earlier non-IP PDU disappears'
    previous={r['number']:r for r in rows(before_cli)if r.get('record_type')!='protocol_publication'}
    label_changes=[];seen=set();labels=collections.Counter();old_labels=collections.Counter()
    for row in rows(after_cli):
        if row.get('record_type')=='protocol_publication':continue
        n=row['number'];assert n in previous and n not in seen;seen.add(n);prior=previous[n]
        for field in['number','time','length','captured_length','capture_domain']:
            assert canonical(row[field])==canonical(prior[field]),('original physical fact changed',n,field)
        labels[row['protocol']]+=1;old_labels[prior['protocol']]+=1
        if row['protocol']!=prior['protocol']:
            assert n in refused and row['protocol']in['IPv4','IPv6','IP']
            label_changes.append(dict(frame=n,before=prior['protocol'],after=row['protocol'],reason=refused[n]))
    assert seen==set(previous)
    def diagnostics(counter):
        return[dict(status=status,expert=expert,reason=reason,records=count)for(status,expert,reason),count in sorted(counter.items())]
    return dict(scope=__doc__,capture_SHA256=sha(capture),before_SDK_SHA256=sha(before),after_SDK_SHA256=sha(after),before_CLI_SHA256=sha(before_cli),after_CLI_SHA256=sha(after_cli),auditor_SHA256=sha(Path(__file__)),source_bound_non_IP_events=paired,complete_non_IP_events_identical=exact,non_IP_events_with_only_generated_ID_changes=renumbered,generated_ID_mapping=mapping,all_other_application_values_and_connections_literal=True,physical_rows=len(seen),physical_facts_unchanged=True,before_literal_CLI_labels=dict(old_labels),after_literal_CLI_labels=dict(labels),source_bound_label_changes=label_changes,before_IP_diagnostics=diagnostics(old_ip),after_IP_diagnostics=diagnostics(new_ip),all_protocol_Wireshark_parity_claim=False)


if __name__=='__main__':
    ap=argparse.ArgumentParser(description=__doc__)
    for name in['capture','before','after','before-cli','after-cli','out']:ap.add_argument('--'+name,type=Path,required=True)
    a=ap.parse_args()
    for p in vars(a).values():assert not any((v/'.git').exists()for v in[p.resolve(),*p.resolve().parents])
    proof=audit(a.capture,a.before,a.after,a.before_cli,a.after_cli)
    with a.out.open('x',encoding='utf8',newline='\n')as f:json.dump(proof,f,indent=2);f.write('\n')
    print(json.dumps({k:proof[k]for k in['source_bound_non_IP_events','complete_non_IP_events_identical','non_IP_events_with_only_generated_ID_changes','physical_rows','physical_facts_unchanged']}),flush=True)
