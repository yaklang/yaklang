"""Preserve every non-IMAP/RTSP SDK record across recovered-message insertion.

PDU IDs necessarily shift when formerly discarded messages are recovered.
Bind the explicit ID bijection to ordered immutable wire/provenance identities;
resolve only declared reference fields through it. All protocol labels, wire
values, types, ordering, domains, endpoints, flows and other fields remain exact.
Selected IMAP/RTSP semantics require their separate independent wire audit.
"""
import argparse
import collections
import copy
import hashlib
import itertools
import json
from pathlib import Path

SELECTED={'imap','rtsp'}
# These contracts use observed request PDU IDs (protocol_dns/dhcp_phase,
# radius_event, bin_parser_http and the BFD observer). Other protocols can use
# literal on-wire transaction numbers, e.g. BACnet invoke IDs; keep those exact.
REQUEST_PDU_TRANSACTIONS={'http','ipp','dns','llmnr','dhcp','dhcpv6','radius','bfd','ftp-data'}


def canonical(value):return json.dumps(value,sort_keys=True,separators=(',',':'),ensure_ascii=True)
def digest(value):return hashlib.sha256(canonical(value).encode()).hexdigest()
def sha(path):
    with path.open('rb')as f:return hashlib.file_digest(f,'sha256').hexdigest()
def rows(path):
    def unique(pairs):
        out={}
        for k,v in pairs:
            assert k not in out,'duplicate JSON key';out[k]=v
        return out
    for line in path.open(encoding='utf-8-sig'):
        yield json.loads(line,object_pairs_hook=unique,parse_constant=lambda v:(_ for _ in()).throw(ValueError(v)))
def identity(e):
    source=e.get('source_bytes')or{}
    return canonical([e.get(k)for k in ['protocol','entry','transport','direction','flow','source','destination','domain','raw']]+[source.get('SHA256'),source.get('PacketRefs')])


class References:
    def __init__(self,before,after):
        self.ids={0:0};self.reverse={0:0};self.targets={};self.selected_changes=[];self.unmatched_selected=[]
        self.selected_old=[];self.selected_new=[]
        def filtered(path,selected):
            for e in rows(path):
                if e['protocol']in SELECTED:
                    selected.append(e)
                else:yield e
        self.nonselected=0
        for old,new in itertools.zip_longest(filtered(before,self.selected_old),filtered(after,self.selected_new)):
            assert old is not None and new is not None,'all nonselected ordered records present'
            assert identity(old)==identity(new),'nonselected wire/provenance/flow identity cannot shift'
            self.bind(old,new)
            self.nonselected+=1
        candidates=collections.defaultdict(collections.deque)
        for new in self.selected_new:candidates[identity(new)].append(new)
        for old in self.selected_old:
            pool=candidates[identity(old)]
            if pool:
                new=pool.popleft();self.bind(old,new)
                self.selected_changes.append(dict(before=old['id'],after=new['id'],protocol=old['protocol'],raw_identity_sha256=hashlib.sha256(identity(old).encode()).hexdigest()))
            else:
                self.unmatched_selected.append(dict(before=old['id'],protocol=old['protocol'],raw_identity_sha256=hashlib.sha256(identity(old).encode()).hexdigest()))
        self.inserted=[dict(after=e['id'],protocol=e['protocol'],status=e['status'],raw_identity_sha256=hashlib.sha256(identity(e).encode()).hexdigest())for pool in candidates.values()for e in pool]
        assert self.nonselected,'empty input cannot certify preservation'
    def bind(self,old,new):
        a,b=old['id'],new['id']
        assert type(a)is int and type(b)is int and a>0 and b>0,'literal positive PDU identities'
        assert a not in self.ids and b not in self.reverse,'one-to-one PDU identity binding'
        self.ids[a]=b;self.reverse[b]=a
        self.targets[a]=dict(protocol=old['protocol'],flow=old['flow'],source=old['source'],destination=old['destination'],domain=old['domain'],identity_sha256=hashlib.sha256(identity(old).encode()).hexdigest())
    def ref(self,value):
        assert type(value)is int and value in self.ids,'every changed reference resolves to a wire-bound PDU'
        return self.ids[value]
    def source(self,source):
        if not isinstance(source,dict):return
        if source.get('ParentPDU'):
            source['ParentPDU']=self.ref(source['ParentPDU'])
        if source.get('ParentPDUs')is not None:
            source['ParentPDUs']=[self.ref(v)for v in source['ParentPDUs']]
    def semantic_refs(self,node):
        if isinstance(node,dict):
            for k,v in node.items():
                if k=='SDP Event IDs':
                    assert isinstance(v,list)
                    for identifier in v:
                        assert self.targets[identifier]['protocol']in ['sip','rtsp','http','sdp','sap'],'SDP references resolve to control/application evidence'
                    node[k]=[self.ref(x)for x in v]
                elif k=='Data Transfer ID' and type(v)is int:
                    if v:
                        assert self.targets[v]['protocol']=='ftp','FTP transfer identity resolves to the observed control command'
                    node[k]=self.ref(v)
                elif k=='Original Carrier PDU' and type(v)is int:
                    node[k]=self.ref(v)
                else:self.semantic_refs(v)
        elif isinstance(node,list):
            for child in node:self.semantic_refs(child)
    def expected(self,old,new):
        expected=copy.deepcopy(old)
        expected['id']=self.ref(old['id'])
        expected['response_to']=self.ref(old['response_to'])
        # Classify from the declared source contract, never from the desired
        # new value. Keeping a stale old PDU number is also an audit failure.
        transaction=old.get('transaction',0)
        if transaction and old['protocol']in REQUEST_PDU_TRANSACTIONS:
            assert transaction in self.targets,'transaction must resolve to a known request PDU'
            target=self.targets[transaction]
            if old['protocol']=='ftp-data':
                session=old['session']
                assert target['protocol']=='ftp' and session['Control Flow ID']==target['flow'] and session['Data Transfer ID']==transaction,'FTP data resolves to its exact observed control request'
                assert target['domain']==old['domain'],'FTP control/data capture domains agree'
            else:
                assert target['protocol']==old['protocol'] and target['flow']==old['flow'],'transaction target retains protocol and connection identity'
            expected['transaction']=self.ref(transaction)
        self.source(expected.get('source_bytes'))
        snapshot=expected.get('field_values')
        if isinstance(snapshot,dict):
            if 'pdu'in snapshot:snapshot['pdu']=self.ref(snapshot['pdu'])
            self.source(snapshot.get('source'))
            for field in snapshot.get('values')or[]:
                if field['name']in ['pdu.id','pdu.response_to']:
                    field['value']=self.ref(field['value'])
                    if 'exact_value'in field:field['exact_value']=str(field['value'])
        self.semantic_refs(expected.get('fields'))
        self.semantic_refs(expected.get('session'))
        return expected


def audit(before,after):
    references=References(before,after)
    old=(e for e in rows(before)if e['protocol']not in SELECTED)
    new=(e for e in rows(after)if e['protocol']not in SELECTED)
    counts=collections.Counter();changed_ids=0
    for a,b in zip(old,new):
        expected=references.expected(a,b)
        assert canonical(expected)==canonical(b),('non-certified complete SDK record change',a['id'],b['id'],a['protocol'])
        counts[a['protocol']]+=1;changed_ids+=a['id']!=b['id']
    return dict(scope=__doc__,before_sha256=sha(before),after_sha256=sha(after),nonselected_records_compared=references.nonselected,
                protocol_counts=dict(sorted(counts.items())),renumbered_records=changed_ids,PDU_reference_bijection_sha256=digest(sorted(references.ids.items())),
                retained_selected_identities=references.selected_changes,retired_selected_identities=references.unmatched_selected,
                inserted_selected_messages=references.inserted,selected_counts_before=dict(collections.Counter(e['protocol']for e in references.selected_old)),
                selected_counts_after=dict(collections.Counter(e['protocol']for e in references.selected_new)),
                all_other_values_types_order_flows_and_wire_fields_exact=True,wire_field_or_protocol_label_normalization=False)


if __name__=='__main__':
    ap=argparse.ArgumentParser(description=__doc__)
    for name in ['before','after','out']:ap.add_argument('--'+name,type=Path,required=True)
    a=ap.parse_args()
    for p in vars(a).values():assert not any((q/'.git').exists()for q in[p.resolve(),*p.resolve().parents])
    result=audit(a.before,a.after)
    a.out.write_text(json.dumps(result,indent=2)+'\n',encoding='utf8')
    print(json.dumps({k:v for k,v in result.items()if k in ['nonselected_records_compared','renumbered_records','selected_counts_before','selected_counts_after','all_other_values_types_order_flows_and_wire_fields_exact']}))
