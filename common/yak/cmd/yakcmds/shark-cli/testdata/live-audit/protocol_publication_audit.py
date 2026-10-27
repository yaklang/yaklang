"""Verify additive late-publication exports against the complete original SDK.

Existing physical rows, ordinary PDUs and every original value remain exact.
Each new positive publication must equal the complete SDK source/typed snapshot
under its literal ID, without ID remapping or a new byte-contributor claim.
This certifies preservation/export completeness, not new protocol conformance.
"""
import argparse
import collections
import copy
import itertools
import json
from pathlib import Path

from mail_rtsp_export_invariants import canonical,rows,sha

PUBLICATION_SCOPE='available_during_processing; byte_contribution_is_defined_by_byte_source'


def native_projection(event):
    snapshot=event['field_values'];fields=collections.defaultdict(list)
    for field in snapshot['values']:fields[field['name']].append(field['value'])
    expected=dict(id=event['id'],protocol=event['protocol'],status=event['status'],completeness=event['completeness'],response_to=event['response_to'],byte_source=event['source_bytes'],fields=dict(fields),field_values=snapshot)
    if event.get('error')or event.get('expert'):
        expected.update(summary=event['summary'],error=event.get('error',''),expert=event.get('expert',''))
    return expected


def audit(before,after,sdk):
    publications={};old_ids=set();all_ids=set();count=0;changed=[];budget_markers=[];response_links=[]
    for old,new in itertools.zip_longest(rows(before),rows(after)):
        assert old is not None and new is not None,'every original physical row remains ordered'
        count+=1;expected=copy.deepcopy(new)
        extra=expected.pop('published_pdus',[]);scope=expected.pop('published_pdu_scope',None)
        assert canonical(old)==canonical(expected),('every original physical fact, label, ordinary PDU and metadata remains exact',new['number'])
        for pdu in new.get('pdus')or[]:
            if pdu['id']>0:old_ids.add(pdu['id']);all_ids.add(pdu['id'])
            if pdu.get('response_to'):response_links.append((pdu['id'],pdu['response_to'],pdu['protocol']))
        if extra:
            assert scope==PUBLICATION_SCOPE,'explicit availability-only publication scope'
            changed.append(dict(frame=new['number'],published_PDUs=[p['id']for p in extra]))
        else:assert scope is None,'no fabricated empty publication scope'
        for pdu in extra:
            if pdu['id']==0:
                assert pdu['protocol']=='message'and pdu['status']=='limited'and pdu['completeness']=='limited'
                budget_markers.append(new['number']);continue
            identifier=pdu['id'];assert type(identifier)is int and identifier>0 and identifier not in publications,'one new publication per original PDU identity'
            assert all(ref['Number']!=new['number']for ref in pdu['byte_source'].get('PacketRefs')or[]),'a current contributor belongs to ordinary PDUs, not the late-publication channel'
            publications[identifier]=pdu;all_ids.add(identifier)
            if pdu.get('response_to'):response_links.append((identifier,pdu['response_to'],pdu['protocol']))
    assert not(old_ids&publications.keys()),'new publications cannot duplicate previously exported ordinary PDUs'
    matched=set();protocols=collections.Counter();sdk_records=0;missing_decoded=[];decoded_messages=0
    for event in rows(sdk):
        sdk_records+=1
        identifier=event['id']
        if event['status']in['decoded','deferred']and event.get('raw'):
            decoded_messages+=1
            if identifier not in all_ids:missing_decoded.append(dict(PDU=identifier,protocol=event['protocol'],status=event['status'],source=event['source_bytes']))
        if identifier not in publications:continue
        assert identifier not in matched,'the complete original SDK has a unique positive owner'
        assert canonical(publications[identifier])==canonical(native_projection(event)),('complete literal native SDK publication projection',identifier)
        matched.add(identifier);protocols[event['protocol']]+=1
    assert matched==publications.keys(),'every new publication resolves to its complete original SDK event'
    unresolved=[dict(PDU=p,request_PDU=r,protocol=protocol)for p,r,protocol in response_links if r not in all_ids]
    return dict(scope=__doc__,before_sha256=sha(before),after_sha256=sha(after),SDK_sha256=sha(sdk),physical_rows_compared=count,SDK_records_scanned=sdk_records,positive_publications_checked=len(matched),publication_protocol_counts=dict(sorted(protocols.items())),changed_rows=changed,publication_budget_marker_frames=budget_markers,decoded_nonempty_SDK_messages=decoded_messages,decoded_SDK_messages_still_unexported=missing_decoded,all_decoded_nonempty_SDK_messages_present=not missing_decoded,unresolved_exported_numeric_response_targets=unresolved,all_original_rows_values_types_IDs_order_and_ordinary_PDUs_exact=True,all_new_positive_publications_equal_full_original_SDK=True,SDK_CLI_ID_remapping=False)


if __name__=='__main__':
    ap=argparse.ArgumentParser(description=__doc__)
    for name in['before','after','sdk','out']:ap.add_argument('--'+name,type=Path,required=True)
    a=ap.parse_args()
    for path in vars(a).values():assert not any((q/'.git').exists()for q in[path.resolve(),*path.resolve().parents])
    proof=audit(a.before,a.after,a.sdk)
    a.out.write_text(json.dumps(proof,indent=2)+'\n',encoding='utf8')
    print(json.dumps({key:proof[key]for key in['physical_rows_compared','positive_publications_checked','publication_protocol_counts','publication_budget_marker_frames','all_original_rows_values_types_IDs_order_and_ordinary_PDUs_exact']}))
