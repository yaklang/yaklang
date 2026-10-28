"""Certify original malformed IPv4 witnesses and precise native refusals.

Scope: the declared 269 original Ethernet/IPv4 fuzz witnesses. Full capture
checks prevent these source frames from contributing to any application or
fragment session. Reference expert/version fields stay literal, while heuristic
inner protocol labels remain an explicit presentation difference.
"""
import argparse,collections,csv,hashlib,json
from pathlib import Path
from capture_clock_wire_audit import physical
from owned_quic_clock_audit import vals,number,aliases


def sha(p):return hashlib.sha256(p.read_bytes()).hexdigest()
def rows(p):
    for l in p.open(encoding='utf8'):yield json.loads(l)
def refusal(e,expected):
    assert e['protocol']=='ip'and e['status']==e['completeness']=='malformed'and e['expert']=='IPVersionMismatch'
    assert e['flow']==e.get('tcp_flow_id',0)==0 and not e['source']and not e['destination']and not e['raw']
    assert e['session']=={'Expected IP Version':4,'Observed IP Version':expected,'Observed Header Layers':['Ethernet','IPv4']}
    assert type(e['session']['Expected IP Version'])is int and type(e['session']['Observed IP Version'])is int
    assert e['error']==e['summary']==f'invalid IPv4 version {expected}'


def audit(source,capture,manifest,events,cli,oracle,reference_labels_path,full_events=None):
    doc=json.loads(manifest.read_text('utf8'));assert doc['source_capture_SHA256']==sha(source)and doc['capture_SHA256']==sha(capture)
    original=physical(source);observed=physical(capture);assert len(observed)==len(doc['ledger'])==269
    targets={};by_number={}
    for record,entry in zip(observed,doc['ledger']):
        old=original[entry['source_frame']-1];assert record['wire']==old['wire']and record['time_ns']==old['time_ns']==entry['time_ns']and sha_bytes(record['wire'])==entry['wire_SHA256']
        assert old['link']==record['link']==1 and entry['source_domain']==dict(Section=old['section'],Interface=old['interface'],Encapsulation='')
        assert record['interface']==record['section']==0 and entry['output_domain']==dict(Section=0,Interface=0,Encapsulation='')
        raw=old['wire'];assert len(raw)>=34 and raw[12:14]==b'\x08\0'and raw[14]>>4==entry['observed_IP_version']!=4 and entry['expected_IP_version']==4
        targets[entry['source_frame']]=entry;by_number[entry['number']]=entry
    native=list(rows(events));assert len(native)==269 and len({e['id']for e in native})==269
    seen=collections.Counter()
    for e in native:
        assert type(e['id'])is int and e['id']>0
        refs=e['source_bytes']['PacketRefs'];assert len(refs)==1 and e['source_bytes']['Kind']=='captured'
        assert type(refs[0]['Number'])is int
        entry=by_number[refs[0]['Number']];assert refs[0]['Domain']==e['domain']==entry['output_domain'];refusal(e,entry['observed_IP_version'])
        assert e['field_values']['pdu']==e['id']and e['field_values']['source']==e['source_bytes']
        fields=aliases(e);assert fields['pdu.id']==[e['id']]and fields['pdu.length']==[0]and fields['protocol']==['ip']and fields['status']==['malformed']
        seen[refs[0]['Number']]+=1
    assert seen==collections.Counter({n:1 for n in by_number}),'every witness has exactly one native refusal'
    with reference_labels_path.open(encoding='utf8')as f:label_rows=list(csv.reader(f,delimiter='\t'))
    assert label_rows[0]==['frame.number','_ws.col.protocol']and len(label_rows)==270
    labels_by_number={int(r[0]):r[1]for r in label_rows[1:]};assert set(labels_by_number)==set(by_number)
    reference_labels=collections.Counter();expert_checks=0;literal_version_checks=0
    reference=json.loads(oracle.read_text('utf8'));assert len(reference)==269
    for r in reference:
        layers=r['_source']['layers'];entry=by_number[number(layers['frame']['frame.number'])]
        assert any('Bogus IPv4 version'in str(message)for message in vals(layers,'_ws.expert.message'))
        versions=vals(layers.get('ip',{}),'ip.version');assert versions and number(versions[0])==entry['observed_IP_version'];literal_version_checks+=1;expert_checks+=1
        reference_labels[labels_by_number[entry['number']]]+=1
    physical_rows=[r for r in rows(cli)if r.get('record_type')!='protocol_publication'];assert len(physical_rows)==269
    labels=collections.Counter();projected=[]
    for row in physical_rows:
        entry=by_number[row['number']];assert row['length']==row['captured_length']==len(observed[row['number']-1]['wire'])
        pdus=row.get('pdus')or[];assert len(pdus)==1
        p=pdus[0];assert p['protocol']=='ip'and p['status']=='malformed'and p['error']==f'invalid IPv4 version {entry["observed_IP_version"]}'
        assert p['byte_source']['PacketRefs']==[dict(Number=row['number'],Domain=entry['output_domain'])]
        assert row['protocol_status']=='malformed'and row['protocol_scope']and not row.get('connection_protocol')
        assert row['protocol']=='IPv4'and row['protocol_stack']==['Ethernet','IPv4']and row['visibility']=='invalid-network-header'
        labels[row['protocol']]+=1;projected.append(dict(frame=row['number'],PDU=p['id'],protocol=row['protocol'],status=row['protocol_status']))
    full=None
    if full_events is not None:
        seen=collections.Counter();total=0
        for e in rows(full_events):
            refs=e['source_bytes'].get('PacketRefs')or[];selected=[r for r in refs if r['Number']in targets]
            if not selected:continue
            assert len(refs)==len(selected)==1
            entry=targets[selected[0]['Number']];assert selected[0]['Domain']==e['domain']==entry['source_domain'];refusal(e,entry['observed_IP_version']);seen[selected[0]['Number']]+=1;total+=1
        assert seen==collections.Counter({n:1 for n in targets}),'each original bad-version frame has one refusal and no fragment/application contribution'
        full=dict(events_SHA256=sha(full_events),source_frames_checked=total,all_source_frames_refused_before_fragment_or_application_state=True)
    return dict(scope=__doc__,source_capture_SHA256=sha(source),capture_SHA256=sha(capture),manifest_SHA256=sha(manifest),SDK_SHA256=sha(events),CLI_SHA256=sha(cli),reference_SHA256=sha(oracle),reference_labels_SHA256=sha(reference_labels_path),auditor_SHA256=sha(Path(__file__)),original_wire_witnesses=269,literal_reference_version_checks=literal_version_checks,reference_bogus_version_expert_checks=expert_checks,literal_reference_protocol_labels=dict(reference_labels),literal_native_protocol_labels=dict(labels),full_source_refusal=full,CLI_refusals=projected,full_protocol_label_parity_claim=False)


def sha_bytes(w):return hashlib.sha256(w).hexdigest()
if __name__=='__main__':
    ap=argparse.ArgumentParser(description=__doc__)
    for n in['source','capture','manifest','events','cli','oracle','reference-labels','out']:ap.add_argument('--'+n,type=Path,required=True)
    ap.add_argument('--full-events',type=Path);a=ap.parse_args()
    for p in vars(a).values():
        if p is not None:assert not any((v/'.git').exists()for v in[p.resolve(),*p.resolve().parents])
    proof=audit(a.source,a.capture,a.manifest,a.events,a.cli,a.oracle,a.reference_labels,a.full_events)
    with a.out.open('x',encoding='utf8',newline='\n')as f:json.dump(proof,f,indent=2);f.write('\n')
    print(json.dumps({k:v for k,v in proof.items()if k not in['CLI_refusals','scope']}),flush=True)
