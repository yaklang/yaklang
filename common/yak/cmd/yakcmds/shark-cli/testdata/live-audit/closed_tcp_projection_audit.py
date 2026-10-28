"""Certify only the closed-TCP application/page projection correction.

This is projection continuity against literal before/after exports and optional
native packet-hook ownership, not an independent TCP close/receive algorithm.
The close state and unadmitted-payload witness are explicitly inherited native
projection metadata. The witness gates later headers in that literal namespace;
a normal final ACK without such a witness retains its valid history. Every
changed physical record is bound to its original capture bytes and interface.
No numeric IDs, domains, PDUs, message fields or unrelated differences can be
normalized away. Other independently justified protocol corrections require
their own audit stage; this auditor deliberately rejects them.
"""
import argparse,hashlib,itertools,json,re
from pathlib import Path
from dcerpc_wire_audit import records


ALLOWED=frozenset(['connection_protocol','connection_stack','connection_evidence','stream_id','protocol_stack','protocol_status','protocol_scope','visibility','protocol_evidence','info'])
def canonical(value):return json.dumps(value,sort_keys=True,separators=(',',':'),ensure_ascii=True)
def sha(path):
    h=hashlib.sha256()
    with path.open('rb')as f:
        for piece in iter(lambda:f.read(1<<20),b''):h.update(piece)
    return h.hexdigest()
def rows(path):
    with path.open(encoding='utf-8-sig')as f:
        for line in f:yield json.loads(line)
def domain(value):
    assert isinstance(value,dict)and type(value['Section'])is int and type(value['Interface'])is int and type(value['Encapsulation'])is str
    return value['Section'],value['Interface'],value['Encapsulation']


def audit(capture,before,after,observer=None):
    changes=[];total=0;unchanged=0;physical_rows=0;ambiguous_namespaces=set();required_unconfirmed=0
    for ordinal,(a,b)in enumerate(itertools.zip_longest(rows(before),rows(after)),1):
        assert a is not None and b is not None,'same exported row/publication sequence required';total+=1
        if a.get('record_type')!='protocol_publication':physical_rows+=1
        namespace=None;state=a.get('tcp_flow_state');payload_refused=False
        if a.get('tcp_flow_id'):
            assert type(a['tcp_flow_id'])is int
            namespace=(a['tcp_flow_id'],*domain(a['tcp_flow_domain']))
            payload_refused=state in['closed-observation','closed-direction-observation']and a.get('protocol_scope')=='unadmitted closed TCP payload'
            if payload_refused:
                assert a['visibility']=='unassembled-stream'and a['info'].startswith('TCP payload after observed close; generation unconfirmed · ')
                if state=='closed-observation':ambiguous_namespaces.add(namespace)
        unconfirmed=payload_refused or state=='closed-observation'and namespace in ambiguous_namespaces
        if unconfirmed:
            required_unconfirmed+=1
            assert not b.get('connection_protocol')and not b.get('connection_stack')and not b.get('stream_id'),'unadmitted payload and subsequent ambiguous closed headers cannot regain an old application/page'
        if canonical(a)==canonical(b):unchanged+=1;continue
        assert a.get('record_type')!='protocol_publication'and b.get('record_type')!='protocol_publication','capture-close publications cannot change'
        n=b['number'];assert type(n)is int and type(a['number'])is int and n==a['number']and n>0
        assert unconfirmed and a.get('tcp_flow_state')==b.get('tcp_flow_state'),'only native unadmitted closed payload or its later ambiguous headers qualify'
        assert type(a['tcp_flow_id'])is int and type(b['tcp_flow_id'])is int and a['tcp_flow_id']==b['tcp_flow_id']and b['tcp_flow_id']>0
        assert domain(a['tcp_flow_domain'])==domain(b['tcp_flow_domain'])
        assert canonical({k:v for k,v in a.items()if k not in ALLOWED})==canonical({k:v for k,v in b.items()if k not in ALLOWED}),'all native IDs, physical facts, PDUs and message fields remain exact'
        assert a['protocol']==b['protocol']=='TCP'and a['transport']==b['transport']=='tcp'
        if'stream_id'in b:assert type(b['stream_id'])is int and b['stream_id']==0
        if'connection_protocol'in b:assert type(b['connection_protocol'])is str and not b['connection_protocol']
        if'connection_stack'in b:assert b['connection_stack']is None or type(b['connection_stack'])is list and not b['connection_stack']
        view=b.get('transport_view');layers=view['layers']if view else b['layers'];assert type(layers)is list and'TCP'in layers and b['protocol_stack']==layers
        assert b['protocol_evidence']=='application generation unconfirmed after observed TCP close'
        assert b['protocol_status']==(a.get('protocol_status')or'context-required')
        if a.get('protocol_scope')=='unadmitted closed TCP payload':
            assert a['visibility']==b['visibility']=='unassembled-stream'and b['protocol_scope']==a['protocol_scope']
            assert b['connection_evidence']=='native closed TCP observation; payload not admitted to reassembly'and b['info']==a['info'];kind='unadmitted payload'
        else:
            assert b['protocol_scope']=='closed TCP observation'and b['visibility']=='header-only'
            assert b['connection_evidence']=='native closed TCP observation; application generation unconfirmed'
            packet_info=re.sub(r' \[connection #[0-9]+: [^\r\n]*\]$','',a['info'])
            assert re.search(r'(?:^|[ ·])Payload=0(?:[ ·\[(]|$)',packet_info),'header-only classification retains its observed zero-payload fact'
            assert b['info']=='TCP header after observed close; generation unconfirmed · '+packet_info;kind='zero-payload header'
        assert type(b['captured_length'])is int and type(b['length'])is int and 0<=b['captured_length']<=b['length']
        changes.append(dict(ordinal=ordinal,frame=n,native_TCP_flow=b['tcp_flow_id'],native_TCP_domain=b['tcp_flow_domain'],capture_domain=b['capture_domain'],captured_length=b['captured_length'],kind=kind,old_preview=a.get('stream_id',0),new_preview=b.get('stream_id',0),changed_fields=sorted(k for k in ALLOWED if canonical(a.get(k))!=canonical(b.get(k)))))
    assert total>0
    wanted={c['frame']for c in changes};assert len(wanted)==len(changes),'one physical row per corrected capture record';found=set();physical_count=0
    by_number={c['frame']:c for c in changes}
    for n,section,interface,link,wire in records(capture):
        physical_count=n
        if n not in wanted:continue
        item=by_number[n];assert(n not in found)and(section,interface)==domain(item['capture_domain'])[:2]and len(wire)==item['captured_length']
        item.update(source_section=section,source_interface=interface,source_link_type=link,original_captured_wire_SHA256=hashlib.sha256(wire).hexdigest());found.add(n)
    assert found==wanted and physical_rows==physical_count,'exported physical packet count equals the source capture'
    hook_bindings=0
    if observer is not None:
        hooks=json.loads(observer.read_text('utf8'));assert not hooks['error']and hooks['capture_SHA256']==sha(capture);bindings={}
        for p in hooks['packets']:
            assert type(p['flow'])is int and type(p['direction'])is int and p['direction']in[0,1]
            for ref in p['refs']:
                assert type(ref['Number'])is int
                if ref['Number']not in wanted:continue
                key=(ref['Number'],*domain(ref['Domain']));value=(p['flow'],p['direction']);assert bindings.setdefault(key,value)==value
        for item in changes:
            key=(item['frame'],*domain(item['native_TCP_domain']));assert key in bindings and bindings[key][0]==item['native_TCP_flow'];hook_bindings+=1
    return dict(scope=__doc__,capture_SHA256=sha(capture),before_CLI_SHA256=sha(before),after_CLI_SHA256=sha(after),observer_SHA256=sha(observer)if observer else None,auditor_SHA256=sha(Path(__file__)),exported_rows=total,physical_packets=physical_count,unchanged_rows=unchanged,source_bound_projection_corrections=len(changes),native_packet_hook_ownership_bindings=hook_bindings,source_projected_unconfirmed_payload_and_later_header_observations=required_unconfirmed,all_other_fields_and_PDUs_exact=True,native_TCP_ids_and_domains_literal=True,independent_TCP_close_algorithm_claim=False,corrections=changes)


if __name__=='__main__':
    ap=argparse.ArgumentParser(description=__doc__)
    for name in['capture','before','after','out']:ap.add_argument('--'+name,type=Path,required=True)
    ap.add_argument('--observer',type=Path);a=ap.parse_args()
    for p in vars(a).values():
        if p is not None:assert not any((v/'.git').exists()for v in[p.resolve(),*p.resolve().parents])
    proof=audit(a.capture,a.before,a.after,a.observer)
    with a.out.open('x',encoding='utf8')as out:json.dump(proof,out,indent=2);out.write('\n')
    print(json.dumps(dict(rows=proof['exported_rows'],source_bound_corrections=proof['source_bound_projection_corrections'],native_packet_hooks=proof['native_packet_hook_ownership_bindings'])),flush=True)
