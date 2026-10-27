"""Compare every CLI row across independently wire-audited direct RPC recovery.

Bind generated CLI PDU references by ordered immutable source/endpoint identities,
without assuming SDK/CLI numeric namespaces agree. Other protocol PDU metadata
and all physical facts stay exact. RPC-specific presentation changes are allowed
only inside the exact original capture-domain/TCP tuple independently audited.
"""
import argparse
import collections
import copy
import itertools
import json
from pathlib import Path

from mail_rtsp_export_invariants import canonical, rows, sha

SELECTED='dcerpc'
PRESENTATION={'protocol','protocol_stack','connection_protocol','connection_stack','connection_evidence',
              'protocol_status','protocol_scope','protocol_evidence','visibility','info',
              'message_source','message_destination','message_transport','message_domain','pdu_scope'}


def one(pdu,name):
    values=(pdu.get('fields')or{}).get(name)
    if values is None:
        values=[v['value']for v in(pdu.get('field_values')or{}).get('values')or[]if v['name']==name]
    assert len(values)<=1,('unique literal PDU alias',pdu['id'],name)
    # Unrecognized network-layer diagnostics have no fabricated endpoints.
    # Their missing values are still part of the immutable identity and their
    # complete metadata is compared below; selected RPC must have endpoints.
    if pdu['protocol']==SELECTED:assert len(values)==1,('selected endpoint alias',pdu['id'],name)
    return values[0]if values else None


def identity(pdu):
    source=pdu.get('byte_source')or{}
    return canonical([pdu['protocol'],one(pdu,'source'),one(pdu,'destination'),one(pdu,'flow.id'),
                      source.get('Kind'),source.get('SHA256'),source.get('PacketRefs')])


def native_identity(e):
    source=e['source_bytes']
    return canonical([e['protocol'],e['source'],e['destination'],e['flow'],source.get('Kind'),source.get('SHA256'),source.get('PacketRefs')])


def distinct(path,selected):
    seen={}
    for row in rows(path):
        for pdu in row.get('pdus')or[]:
            identifier=pdu['id'];assert type(identifier)is int and identifier>0
            key=identity(pdu)
            if identifier in seen:
                assert seen[identifier]==key,'one CLI identity cannot represent different wire/endpoint evidence'
                continue
            seen[identifier]=key
            if pdu['protocol']==SELECTED:selected.append(pdu)
            else:yield pdu


class References:
    def __init__(self,before,after):
        self.ids={0:0};self.reverse={0:0};self.old_selected=[];self.new_selected=[];self.nonselected=0
        for old,new in itertools.zip_longest(distinct(before,self.old_selected),distinct(after,self.new_selected)):
            assert old is not None and new is not None,'all non-RPC CLI PDUs remain present in order'
            assert identity(old)==identity(new),'non-RPC source/endpoint/flow identity cannot change'
            self.bind(old,new);self.nonselected+=1
        pools=collections.defaultdict(collections.deque)
        for pdu in self.new_selected:pools[identity(pdu)].append(pdu)
        for pdu in self.old_selected:
            if pools[identity(pdu)]:self.bind(pdu,pools[identity(pdu)].popleft())
    def bind(self,old,new):
        a,b=old['id'],new['id']
        assert a not in self.ids and b not in self.reverse,'one-to-one generated identity binding'
        self.ids[a]=b;self.reverse[b]=a
    def ref(self,value):
        assert type(value)is int and value in self.ids,('reference resolves to an original source-bound CLI PDU',value)
        return self.ids[value]
    def source(self,source):
        if not isinstance(source,dict):return
        if source.get('ParentPDU'):source['ParentPDU']=self.ref(source['ParentPDU'])
        if source.get('ParentPDUs')is not None:source['ParentPDUs']=[self.ref(v)for v in source['ParentPDUs']]
    def expected(self,pdu):
        out=copy.deepcopy(pdu);out['id']=self.ref(out['id']);out['response_to']=self.ref(out['response_to'])
        self.source(out.get('byte_source'))
        snapshot=out.get('field_values')
        if isinstance(snapshot,dict):
            snapshot['pdu']=self.ref(snapshot['pdu']);self.source(snapshot.get('source'))
            for field in snapshot.get('values')or[]:
                if field['name']in ['pdu.id','pdu.response_to']:
                    field['value']=self.ref(field['value'])
                    if 'exact_value'in field:field['exact_value']=str(field['value'])
        for name in ['pdu.id','pdu.response_to']:
            if name in(out.get('fields')or{}):out['fields'][name]=[self.ref(v)for v in out['fields'][name]]
        return out


def selected_projection(pdu,e):
    assert canonical(pdu['byte_source'])==canonical(e['source_bytes']),'complete selected source projection'
    assert pdu['status']==e['status'],'selected PDU status is the native status'
    assert pdu['response_to']==0 and e['response_to']==0,'this observed profile does not claim missing numeric response links'
    expected=copy.deepcopy(e['field_values']);expected['pdu']=pdu['id']
    for field in expected.get('values')or[]:
        if field['name']=='pdu.id':field['value']=pdu['id'];field['exact_value']=str(pdu['id'])
    assert canonical(expected)==canonical(pdu['field_values']),'complete selected typed alias snapshot'
    aliases=collections.defaultdict(list)
    for field in expected.get('values')or[]:aliases[field['name']].append(field['value'])
    assert canonical(dict(aliases))==canonical(pdu.get('fields')or{}),'flat selected aliases equal the owned typed values'


def physical_tuple(row):
    if row.get('transport')!='tcp':return None
    def endpoint(host,port):return ('['+host+']'if ':'in host else host)+':'+str(port)
    return canonical([row['capture_domain'],sorted([endpoint(row['source'],row.get('source_port',0)),endpoint(row['destination'],row.get('destination_port',0))])])


def compare_row(old,new,refs,tuples):
    old_p=[refs.expected(pdu)for pdu in old.get('pdus')or[]if pdu['protocol']!=SELECTED]
    new_p=[pdu for pdu in new.get('pdus')or[]if pdu['protocol']!=SELECTED]
    assert canonical(old_p)==canonical(new_p),('complete other-protocol CLI PDU metadata',new['number'])
    a=copy.deepcopy(old);b=copy.deepcopy(new);a.pop('pdus',None);b.pop('pdus',None)
    difference={key for key in a.keys()|b.keys()if canonical(a.get(key))!=canonical(b.get(key))or(key in a)!=(key in b)}
    if difference:
        assert physical_tuple(old)==physical_tuple(new),'physical capture-domain/TCP identity cannot change'
        assert physical_tuple(new)in tuples and difference<=PRESENTATION,('non-certified row/physical-fact change',new['number'],sorted(difference))
        for key in difference:
            if key in a:a.pop(key)
            if key in b:b.pop(key)
        assert canonical(a)==canonical(b),'all other physical columns and types remain exact'
    return difference,len(new_p)


def audit(before,after,sdk,wire_certificate):
    certificate=json.loads(wire_certificate.read_text(encoding='utf8'))
    assert certificate['events_sha256']==sha(sdk),'wire proof binds the exact SDK export'
    proven={e['PDU']for e in certificate['compared_PDUs']}|set(certificate['closure_diagnostics'])
    native=collections.defaultdict(list);tuples=set()
    for e in rows(sdk):
        if e['protocol']!=SELECTED:continue
        if e['id']not in proven:continue
        assert e['transport']=='tcp','selected direct carrier only'
        native[native_identity(e)].append(e)
        tuples.add(canonical([e['domain'],sorted([e['source'],e['destination']])]))
    assert tuples,'nonempty independently audited RPC scope'
    refs=References(before,after)
    # A selected PDU needs full SDK byte/source and alias evidence too; a
    # connection-level label alone never authorizes changes to arbitrary rows.
    sdk_bindings={}
    for pdu in refs.new_selected:
        candidates=native[identity(pdu)]
        assert len(candidates)==1,('unique source-bound selected SDK PDU',pdu['id'])
        e=candidates[0]
        selected_projection(pdu,e)
        sdk_bindings[pdu['id']]=dict(SDK_PDU=e['id'],protocol=e['protocol'],raw_sha256=e['source_bytes']['SHA256'],physical_frames=[r['Number']for r in e['source_bytes']['PacketRefs']or[]])
    count=0;changed=[];nonselected_instances=0
    label_counts=collections.Counter()
    for old,new in itertools.zip_longest(rows(before),rows(after)):
        assert old is not None and new is not None,'all original physical rows remain ordered'
        count+=1;label_counts[new['protocol']]+=1
        difference,instances=compare_row(old,new,refs,tuples)
        nonselected_instances+=instances
        if not difference:continue
        changed.append(dict(frame=new['number'],columns=sorted(difference),protocol_before=old['protocol'],protocol_after=new['protocol'],CLI_PDUs=[p['id']for p in new.get('pdus')or[]if p['protocol']==SELECTED]))
    assert count==certificate['physical_frames'],'CLI rows cover the whole original input'
    return dict(scope=__doc__,before_sha256=sha(before),after_sha256=sha(after),SDK_sha256=sha(sdk),wire_certificate_sha256=sha(wire_certificate),physical_rows_compared=count,
                nonselected_unique_PDUs=refs.nonselected,nonselected_PDU_instances=nonselected_instances,selected_SDK_bindings=sdk_bindings,changed_RPC_scope_rows=changed,
                changed_RPC_scope_rows_count=len(changed),protocol_label_counts=dict(sorted(label_counts.items())),all_other_values_types_physical_facts_and_order_exact=True,SDK_CLI_numeric_namespace_equality_assumed=False)


if __name__=='__main__':
    ap=argparse.ArgumentParser(description=__doc__)
    for name in ['before','after','sdk','wire-certificate','out']:ap.add_argument('--'+name,type=Path,required=True)
    a=ap.parse_args()
    for p in vars(a).values():assert not any((q/'.git').exists()for q in[p.resolve(),*p.resolve().parents])
    proof=audit(a.before,a.after,a.sdk,a.wire_certificate)
    a.out.write_text(json.dumps(proof,indent=2)+'\n',encoding='utf8')
    print(json.dumps({k:proof[k]for k in ['physical_rows_compared','nonselected_unique_PDUs','changed_RPC_scope_rows_count','all_other_values_types_physical_facts_and_order_exact']}))
