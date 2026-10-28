"""Check every native protocol observation after one declared IP prefix.

Translate only attested physical frame references and generated PDU references,
including explicit native QUIC/HTTP3 stream byte-source annotations and HTTP3
transactions whose implementation identity is the source request PDU. Native
HTTP request/response links use that same explicitly generated PDU namespace.
All wire fields, plaintext, source bytes, JSON types, protocol-session IDs and
native TCP IDs remain literal. This is source/graph preservation, not independent
implementation of TLS/QUIC/CoAP or a claim covering their untested features.
"""
import argparse,collections,copy,hashlib,json
from pathlib import Path
from clock_public_preservation_audit import canonical,key


def sha(p):return hashlib.sha256(p.read_bytes()).hexdigest()
def load(p):return json.loads(p.read_text('utf8'))
def rows(p):return[json.loads(line)for line in p.open(encoding='utf8')]


def audit(before,before_receipt,after,after_receipt,manifest,wire_proof):
    doc=load(manifest);wire=load(wire_proof);old_receipt=load(before_receipt);new_receipt=load(after_receipt)
    assert wire['manifest_SHA256']==sha(manifest)and wire['source_capture_SHA256']==old_receipt['capture_SHA256']
    assert sha(before)==old_receipt['export_SHA256']and sha(after)==new_receipt['export_SHA256']and wire['capture_SHA256']==new_receipt['capture_SHA256']
    assert doc['case_scope']=='all-original-records-plus-one-declared-prefix'and len(wire['inserted_prefixes'])==1
    ledger={v['number']:v for v in doc['ledger']};prefix=wire['inserted_prefixes'][0]['number']
    def reference(ref):
        n=ref['Number'];assert type(n)is int and n>=0
        if n==0:return
        assert n in ledger and n!=prefix,'a protocol message cannot own the inserted incomplete header'
        entry=ledger[n];assert entry['change']['kind']=='exact'
        assert all(type(ref['Domain'][k])is int for k in['Section','Interface'])and ref['Domain']['Section']==ref['Domain']['Interface']==0
        ref['Number']=entry['source_frame'];ref['Domain']['Section']=entry['source_section'];ref['Domain']['Interface']=entry['source_interface']
    def sources(event):
        for source in[event['source_bytes'],event['field_values']['source']]:
            for ref in source.get('PacketRefs')or[]:reference(ref)
            if source.get('CompletedIn')is not None:reference(source['CompletedIn'])
        for field in event['field_values'].get('values')or[]:
            if field['name']=='pdu.completed_in':
                n=field['value'];assert type(n)is int and n>=0
                if n:
                    assert n!=prefix and ledger[n]['change']['kind']=='exact';field['value']=ledger[n]['source_frame']
                    if'exact_value'in field:
                        assert field['exact_value']==str(n);field['exact_value']=str(field['value'])
    original=rows(before);assert original and all(e['protocol']!='ip'for e in original)
    available=collections.defaultdict(collections.deque)
    for event in original:
        assert type(event['id'])is int and event['id']>0;available[key(event)].append(event)
    paired=[];ids={};diagnostics=[]
    for actual in rows(after):
        assert type(actual['id'])is int and actual['id']>0
        if actual['protocol']=='ip':
            assert actual.get('expert')=='IPHeaderTruncated'and actual['source_bytes']['PacketRefs'][0]['Number']==prefix
            diagnostics.append(actual);continue
        event=copy.deepcopy(actual);sources(event);matches=available.get(key(event));assert matches,('literal source/protocol/connection identity changed',actual['protocol'],actual['id'])
        expected=matches.popleft();assert actual['id']not in ids;ids[actual['id']]=expected['id'];paired.append((event,expected))
    assert not any(available.values())and len(paired)==len(original)and len(diagnostics)==1
    def pdu(n):
        assert type(n)is int and n>=0
        if n==0:return n
        assert n in ids,'generated parents must belong to an actual paired source message';return ids[n]
    def quic_references(refs):
        for ref in refs:
            assert set(ref)=={'Packet','Section','Interface','Encapsulation'}
            current={'Number':ref['Packet'],'Domain':{k:ref[k]for k in['Section','Interface','Encapsulation']}}
            reference(current);ref['Packet']=current['Number']
            for k in['Section','Interface']:ref[k]=current['Domain'][k]
    def quic_source(source):
        assert source['Kind']=='quic-authenticated-stream-reassembly'
        source['Parent PDUs']=[pdu(n)for n in source['Parent PDUs']]
        quic_references(source['Packet References'])
        for span in source['Ranges']or[]:
            span['Parent PDU']=pdu(span['Parent PDU']);quic_references(span['Packet References'])
    def http3_sources(frames):
        for application in frames:
            if'Message Conformance'in application:conformance(application['Message Conformance'])
            for name in['Byte Source','Payload Byte Source']:
                if name in application:quic_source(application[name])
    def conformance(value):
        if'Header PDU ID'in value:value['Header PDU ID']=pdu(value['Header PDU ID'])
    for event,expected in paired:
        event['id']=pdu(event['id']);event['field_values']['pdu']=pdu(event['field_values']['pdu'])
        for source in[event['source_bytes'],event['field_values']['source']]:
            source['ParentPDU']=pdu(source.get('ParentPDU',0))
            if source.get('ParentPDUs')is not None:source['ParentPDUs']=[pdu(n)for n in source['ParentPDUs']]
        for field in event['field_values'].get('values')or[]:
            if field['name']=='pdu.id'or field['name']=='pdu.response_to'and event['protocol']in['quic','http3','http']:
                n=field['value'];field['value']=pdu(n)
                if'exact_value'in field:
                    assert field['exact_value']==str(n);field['exact_value']=str(field['value'])
        if event['protocol']=='http'and event['transaction']:
            assert event['response_to']in[0,event['transaction']]
            event['transaction']=pdu(event['transaction']);event['response_to']=pdu(event['response_to'])
        if event['protocol']in['quic','http3']:
            request_ids={frame['Request PDU ID']for frame in event['session'].get('Frames')or[]if frame.get('Request PDU ID',0)}
            if event['transaction']:
                assert request_ids=={event['transaction']}and event['response_to']in[0,event['transaction']]
                event['transaction']=pdu(event['transaction']);event['response_to']=pdu(event['response_to'])
            if'HTTP3 Message Conformance'in event['session']:conformance(event['session']['HTTP3 Message Conformance'])
            http3_sources(event['session'].get('HTTP3 Frames')or[])
            for frame in event['session'].get('Frames')or[]:
                for name in['Request PDU ID','Transaction ID']:
                    if name in frame:frame[name]=pdu(frame[name])
                if'HTTP3 Message Conformance'in frame:conformance(frame['HTTP3 Message Conformance'])
                source=frame.get('Byte Source')
                if source is not None:
                    assert source['Kind']=='quic-authenticated-plaintext'
                    source['Parent PDU']=pdu(source['Parent PDU'])
                if'Delivered Byte Source'in frame:quic_source(frame['Delivered Byte Source'])
                http3_sources(frame.get('HTTP3 Frames')or[])
        assert canonical(event)==canonical(expected),('a non-reference message value changed',event['protocol'],event['id'])
    return dict(scope=__doc__,baseline_SDK_SHA256=sha(before),SDK_SHA256=sha(after),baseline_receipt_SHA256=sha(before_receipt),receipt_SHA256=sha(after_receipt),manifest_SHA256=sha(manifest),wire_proof_SHA256=sha(wire_proof),auditor_SHA256=sha(Path(__file__)),compared_native_records=len(paired),protocol_counts=dict(collections.Counter(e['protocol']for e in original)),generated_ID_mapping_SHA256=hashlib.sha256(canonical(sorted(ids.items())).encode()).hexdigest(),inserted_header_diagnostics=1,wire_plaintext_types_and_connection_namespaces_literal=True,only_attested_frame_and_generated_PDU_references_translated=True,full_protocol_parity_claim=False)


if __name__=='__main__':
    ap=argparse.ArgumentParser(description=__doc__)
    for n in['before','before-receipt','after','after-receipt','manifest','wire-proof','out']:ap.add_argument('--'+n,type=Path,required=True)
    a=ap.parse_args()
    for p in vars(a).values():assert not any((v/'.git').exists()for v in[p.resolve(),*p.resolve().parents])
    proof=audit(a.before,a.before_receipt,a.after,a.after_receipt,a.manifest,a.wire_proof)
    with a.out.open('x',encoding='utf8')as f:json.dump(proof,f,indent=2);f.write('\n')
    print(json.dumps(dict(native_records=proof['compared_native_records'],literal_protocol_values_preserved=True)),flush=True)
