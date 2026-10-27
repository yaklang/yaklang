"""Certify exact whole-SDK preservation across wire-proven RPC owner additions.

Allow only a verified numeric reply alias and the precise EPM operation names
derived from accepted UUID/version/context/opnum. Every original record, type,
ID, flow, byte, field, refusal and contributor remains exact without ID mapping.
"""
import argparse
import base64
import copy
import itertools
import json
from pathlib import Path

from mail_rtsp_export_invariants import canonical,rows,sha


def audit(before,after,wire_certificate):
    certificate=json.loads(wire_certificate.read_text(encoding='utf8'))
    assert certificate['events_sha256']==sha(after),'complete current export is bound to the original-wire certificate'
    links=certificate['numeric_call_links']
    assert links['all_numeric_links_match_original_request_ownership']is True
    if sha(before)==sha(after):
        count=certificate['SDK_records_with_unique_positive_IDs']
        return dict(scope=__doc__,before_sha256=sha(before),after_sha256=sha(after),wire_certificate_sha256=sha(wire_certificate),complete_records_compared=count,complete_exports_byte_identical=True,byte_equivalent_records=count,changed_RPC_records=0,changed=[],all_other_values_types_IDs_order_flows_fields_and_sources_exact=True)
    old_requests={};new_requests={};changes=[];count=0;unchanged=0
    operations={0:'ept_insert',1:'ept_delete',2:'ept_lookup',3:'ept_map',4:'ept_lookup_handle_free',5:'ept_inq_object',6:'ept_mgmt_delete'}
    for old,new in itertools.zip_longest(rows(before),rows(after)):
        assert old is not None and new is not None,'every original SDK record stays in order'
        count+=1
        if old['protocol']==new['protocol']=='dcerpc':
            assert old['id']==new['id'],'original RPC identities remain exact'
            assert links['verified_response_owner_PDU_IDs'][str(new['id'])]==new['response_to']
            raw=base64.b64decode(new.get('raw')or'')
            if len(raw)>=16 and raw[2]in[0,11,14]and new['status']in['decoded','deferred']:
                old_requests[old['id']]=old;new_requests[new['id']]=new
        if canonical(old)==canonical(new):unchanged+=1;continue
        assert old['protocol']==new['protocol']=='dcerpc','unrelated complete SDK metadata cannot change'
        expected=copy.deepcopy(new);changed=[]
        if old['response_to']!=new['response_to']:
            assert old['response_to']==0 and new['response_to']in new_requests,'only a certified original request may gain a reply reference'
            expected['response_to']=0
            values=expected['field_values']['values'];aliases=[v for v in values if v['name']=='pdu.response_to']
            assert len(aliases)==1 and aliases[0]['value']==new['response_to']and aliases[0]['exact_value']==str(new['response_to'])
            values[:]=[v for v in values if v['name']!='pdu.response_to'];changed.append('wire-proven generated reply owner and typed alias')
        a=old.get('session')or{};b=new.get('session')or{}
        if a.get('Packet Name')!=b.get('Packet Name'):
            raw=base64.b64decode(new['raw']);op=int.from_bytes(raw[22:24],'little');call=int.from_bytes(raw[12:16],'little')
            assert raw[2]==0 and raw[3]&3==3 and int.from_bytes(raw[10:12],'little')==0
            assert b['Interface']==a['Interface']=='EPM'and b['Interface Version Major']==3 and b['Interface Version Minor']==0
            assert a['Packet Name']==f'EPM.op_{op}'and b['Packet Name']=='EPM.'+operations[op]
            assert old['summary']==f"DCE/RPC {a['Packet Name']} call {call}"and new['summary']==f"DCE/RPC {b['Packet Name']} call {call}"
            expected['session']['Packet Name']=a['Packet Name'];expected['summary']=old['summary'];changed.append('verified EPM request operation name')
        if a.get('Matched Request')!=b.get('Matched Request'):
            owner=new['response_to'];assert owner in old_requests and owner in new_requests
            assert a['Matched Request']==old_requests[owner]['session']['Packet Name']and b['Matched Request']==new_requests[owner]['session']['Packet Name']
            expected['session']['Matched Request']=a['Matched Request'];changed.append('same original request operation on the reply')
        assert changed and canonical(expected)==canonical(old),'only the exact original-wire-certified owner/operation projection can change'
        changes.append(dict(PDU=new['id'],changes=changed))
    assert count==certificate['SDK_records_with_unique_positive_IDs'],'complete original SDK coverage'
    return dict(scope=__doc__,before_sha256=sha(before),after_sha256=sha(after),wire_certificate_sha256=sha(wire_certificate),complete_records_compared=count,byte_equivalent_records=unchanged,changed_RPC_records=len(changes),changed=changes,all_other_values_types_IDs_order_flows_fields_and_sources_exact=True)


if __name__=='__main__':
    ap=argparse.ArgumentParser(description=__doc__)
    for name in['before','after','wire-certificate','out']:ap.add_argument('--'+name,type=Path,required=True)
    a=ap.parse_args()
    for path in vars(a).values():assert not any((q/'.git').exists()for q in[path.resolve(),*path.resolve().parents])
    proof=audit(a.before,a.after,a.wire_certificate)
    a.out.write_text(json.dumps(proof,indent=2)+'\n',encoding='utf8')
    print(json.dumps({k:proof[k]for k in['complete_records_compared','byte_equivalent_records','changed_RPC_records','all_other_values_types_IDs_order_flows_fields_and_sources_exact']}))
