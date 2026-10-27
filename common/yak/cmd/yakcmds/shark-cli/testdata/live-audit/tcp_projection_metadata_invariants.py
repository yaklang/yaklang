"""Check one explicit additive TCP-generation metadata change, without hiding it.

Callers retain full original exports, both hashes, and all literal differences.
These guards prove preservation of the pre-existing values, not byte-identical
exports and not Wireshark conformance. Separate packet-hook/wire audits establish
the added generation's ownership.
"""

from mail_rtsp_export_invariants import canonical


def without_tcp_metadata(value,cli=False):
    native=value.get('tcp_flow_id',0)
    assert type(native)is int and native>=0,'literal integer TCP generation metadata'
    out=dict(value)
    if 'tcp_flow_id'in out:
        assert native>0,'unavailable TCP binding is absent, never an invented zero'
        out.pop('tcp_flow_id')
    snapshot=dict(value['field_values'])
    values=snapshot.get('values')or[]
    added=[v for v in values if v['name']=='tcp.flow.id']
    expected=[dict(name='tcp.flow.id',value=native,exact_value=str(native),origin='event')]if native else[]
    assert canonical(added)==canonical(expected),'the only added field is a typed event-level TCP ID with no fabricated wire range'
    snapshot['values']=[v for v in values if v['name']!='tcp.flow.id']
    # Preserve absent/null list representations when no metadata was added.
    if not added:snapshot=value['field_values']
    out['field_values']=snapshot
    if cli:
        fields=dict(value.get('fields')or{})
        assert canonical(fields.get('tcp.flow.id',[]))==canonical([native]if native else[])
        fields.pop('tcp.flow.id',None)
        out['fields']=fields if native else value['fields']
    return out,native


def native_values_preserved(before,after):
    projected,native=without_tcp_metadata(after)
    assert canonical(before)==canonical(projected),'every original SDK byte string, field tree, session, source, status, value type and numeric reference stays exact'
    return native


def CLI_values_preserved(before,after):
    assert len(before)==len(after),'no PDU/detail scope disappears when adding TCP metadata'
    added=0
    for old,new in zip(before,after):
        if new.get('id')==0:
            assert canonical(old)==canonical(new),'explicit packet detail-limit markers cannot be altered or hidden'
            continue
        projected,native=without_tcp_metadata(new,True)
        assert canonical(old)==canonical(projected),'every original frozen CLI PDU value and value type stays exact'
        added+=native!=0
    return added
