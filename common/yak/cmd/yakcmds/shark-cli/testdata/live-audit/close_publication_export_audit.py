"""Certify an append-only capture-close export against original SDK projections.

Physical rows, their types/order and all earlier PDU copies must stay exact.
Every new nonphysical publication must retain a full native typed projection,
reason and byte provenance. This is an export audit, not a wire-parser oracle.
"""
import argparse
import collections
import copy
import json
from pathlib import Path

from dcerpc_cli_invariants import identity
from mail_rtsp_export_invariants import canonical,rows,sha
from tcp_protocol_binding_audit import sdk_projection_identity


def audit(before,after,sdk):
    pools=collections.defaultdict(collections.deque)
    native_count=0
    for e in rows(sdk):
        signature,_=sdk_projection_identity(e)
        pools[sha_text(signature)].append(e['id']);native_count+=1
    bindings={0:0};reverse={0:0};seen={};closing={}
    def bind(pdu):
        identifier=pdu['id']
        assert type(identifier)is int and identifier>0,'detail refusal is explicit and cannot certify full projection parity'
        digest=sha_text(canonical(pdu))
        if identifier in seen:
            assert seen[identifier]==digest,'a retained PDU copy stays immutable'
            return
        seen[identifier]=digest
        candidates=pools[sha_text(identity(pdu))]
        assert candidates,'every CLI PDU has an original SDK source identity'
        native=candidates.popleft()
        assert native not in bindings and identifier not in reverse,'one-to-one SDK/CLI namespaces'
        bindings[native]=identifier;reverse[identifier]=native
    old=iter(rows(before));physical=0;new_rows=0
    for row in rows(after):
        publication=row.get('record_type')=='protocol_publication'
        if publication:
            assert set(row)=={'record_type','publication_phase','published_pdu_scope','published_pdus'}
            assert row['publication_phase']=='capture_close'and row['published_pdu_scope']=='analysis_publication_only; byte_contribution_is_defined_by_byte_source'
            assert len(row['published_pdus'])==1,'each publication has a separate bounded projection'
            pdu=row['published_pdus'][0]
            assert pdu['id']not in seen,'a final publication cannot relabel an earlier PDU'
            bind(pdu);closing[reverse[pdu['id']]]=pdu;new_rows+=1
        else:
            assert not new_rows,'capture-close publications follow all physical snapshots'
            original=next(old,None)
            assert original is not None and canonical(original)==canonical(row),('all original physical rows, values and types remain exact',row.get('number'))
            physical+=1
            for pdu in(row.get('pdus')or[])+(row.get('published_pdus')or[]):bind(pdu)
    assert next(old,None)is None,'no original physical row disappeared'
    assert not any(pools.values()),'every SDK record has a CLI publication'
    assert len(bindings)==native_count+1
    def reference(value):
        assert type(value)is int and value in bindings,'original generated reference resolves in its own namespace'
        return bindings[value]
    def source(value):
        value=copy.deepcopy(value)
        if value.get('ParentPDU'):value['ParentPDU']=reference(value['ParentPDU'])
        if value.get('ParentPDUs')is not None:value['ParentPDUs']=[reference(v)for v in value['ParentPDUs']]
        return value
    checked=0;statuses=collections.Counter();protocols=collections.Counter()
    for e in rows(sdk):
        if e['id']not in closing:continue
        pdu=closing[e['id']];fields=copy.deepcopy(e['field_values'])
        fields['pdu']=reference(fields['pdu']);fields['source']=source(fields['source'])
        aliases=collections.defaultdict(list)
        for field in fields.get('values')or[]:
            if field['name']in['pdu.id','pdu.response_to','dcerpc.message_id','dcerpc.message_parent','dcerpc.message_complete_in']:
                field['value']=reference(field['value']);field['exact_value']=str(field['value'])
            aliases[field['name']].append(field['value'])
        expected=dict(id=reference(e['id']),protocol=e['protocol'],status=e['status'],completeness=e['completeness'],response_to=reference(e['response_to']),byte_source=source(e['source_bytes']),fields=dict(aliases),field_values=fields,summary=e['summary'],error=e.get('error',''),expert=e.get('expert',''))
        if e.get('tcp_flow_id'):expected['tcp_flow_id']=e['tcp_flow_id']
        assert canonical(expected)==canonical(pdu),('complete final typed projection, reason and source are native',e['id'],e['protocol'])
        checked+=1;statuses[e['status']]+=1;protocols[e['protocol']]+=1
    assert checked==new_rows
    return dict(scope=__doc__,before_SHA256=sha(before),after_SHA256=sha(after),SDK_SHA256=sha(sdk),auditor_SHA256=sha(Path(__file__)),physical_rows_exact=physical,new_capture_close_records=checked,new_record_statuses=dict(statuses),new_record_protocols=dict(protocols),SDK_records=native_count,all_original_rows_values_types_order_and_PDUs_exact=True,all_SDK_records_exported=True,all_new_complete_typed_projections_reasons_and_sources_exact=True,SDK_CLI_number_equality_assumed=False)


def sha_text(value):
    import hashlib
    return hashlib.sha256(value.encode()).hexdigest()


if __name__=='__main__':
    ap=argparse.ArgumentParser(description=__doc__)
    for name in['before','after','sdk','out']:ap.add_argument('--'+name,type=Path,required=True)
    a=ap.parse_args()
    for path in vars(a).values():assert not any((p/'.git').exists()for p in[path.resolve(),*path.resolve().parents])
    result=audit(a.before,a.after,a.sdk)
    with a.out.open('x',encoding='utf8',newline='\n')as out:json.dump(result,out,indent=2);out.write('\n')
    print(json.dumps(dict(physical_rows_exact=result['physical_rows_exact'],capture_close_records=result['new_capture_close_records'],all_SDK_records_exported=True)),flush=True)
