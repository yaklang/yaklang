"""Carry an original-wire version proof across a parent-admission correction.

Every prior version-refusal record must remain literal except generated PDU-ID
aliases. Recheck exclusivity in the complete new SDK export and its physical CLI
projection. Raw-reference hashes stay bound to the preceding independent proof;
new parent/short-header cases need their own source audits.
"""
import collections,hashlib,json
from pathlib import Path
from clock_public_preservation_audit import canonical,key,rows,sha,without_generated_id


def audit(capture,before,after,cli,proof,reference,reference_labels):
    previous=json.loads(proof.read_text('utf8'))
    assert previous['capture_SHA256']==sha(capture)and previous['SDK_SHA256']==sha(before)
    assert previous['reference_SHA256']==sha(reference)and previous['reference_labels_SHA256']==sha(reference_labels)
    expected={};actual={}
    for path,target in[(before,expected),(after,actual)]:
        for e in rows(path):
            if e.get('expert')!='IPVersionMismatch':continue
            k=key(e);assert k not in target;target[k]=e
    assert expected and set(expected)==set(actual)and len(expected)==previous['source_bound_version_refusals']
    refs={};exact=renumbered=0
    for k,e in actual.items():
        old=expected[k]
        if canonical(e)==canonical(old):exact+=1
        else:assert canonical(without_generated_id(e))==canonical(without_generated_id(old));renumbered+=1
        source=e['source_bytes']['PacketRefs'];assert len(source)==1 and type(source[0]['Number'])is int
        n=source[0]['Number'];assert n not in refs;refs[n]=e
    contributions=collections.Counter()
    for e in rows(after):
        for ref in e['source_bytes'].get('PacketRefs')or[]:
            if ref['Number']in refs:
                assert type(ref['Number'])is int and e['id']==refs[ref['Number']]['id']and e.get('expert')=='IPVersionMismatch'
                contributions[ref['Number']]+=1
    assert contributions==collections.Counter({n:1 for n in refs})
    seen=collections.Counter()
    for row in rows(cli):
        if row.get('record_type')=='protocol_publication'or row['number']not in refs:continue
        e=refs[row['number']];assert row['protocol']=='IPv4'and row['protocol_stack']==e['session']['Observed Header Layers']and row['protocol_status']=='malformed'and row['visibility']=='invalid-network-header'
        assert not row['source']and not row['destination']and not row.get('tcp_flow_id',0)and not row.get('stream_id',0)and not row.get('connection_protocol')
        assert row['info']==e['summary'];seen[row['number']]+=1
    assert seen==contributions
    return dict(scope=__doc__,capture_SHA256=sha(capture),baseline_SDK_SHA256=sha(before),SDK_SHA256=sha(after),CLI_SHA256=sha(cli),original_wire_proof_SHA256=sha(proof),reference_SHA256=sha(reference),reference_labels_SHA256=sha(reference_labels),auditor_SHA256=sha(Path(__file__)),version_refusals=len(refs),complete_records_exact=exact,only_generated_PDU_ID_aliases_changed=renumbered,every_other_value_and_type_literal=True,complete_new_source_contributors_exclusive=True,physical_CLI_projection_exact=True,new_parent_or_short_header_parity_claim=False)
