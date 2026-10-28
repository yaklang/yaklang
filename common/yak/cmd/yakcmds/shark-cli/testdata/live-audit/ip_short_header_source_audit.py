"""Audit literal short-header observations and their SDK/CLI refusal boundary.

Scope: captured Ethernet/loopback IPv4 or IPv6, selected IPv4 IPIP/6in4 and
flag-free GRE-v0 parents. This proves available bytes and refusal ownership, not a valid
complete packet or agreement with heuristic inner Wireshark column labels.
"""
import argparse,collections,csv,hashlib,ipaddress,json
from pathlib import Path
from mail_rtsp_public_wire_audit import records
from owned_quic_clock_audit import vals,number


def sha(p):return hashlib.sha256(p.read_bytes()).hexdigest()
def rows(p):
    for line in p.open(encoding='utf8'):yield json.loads(line)


def short_header(raw,link):
    if link==0:
        assert raw[:4]in[b'\x02\0\0\0',b'\x18\0\0\0',b'\x1c\0\0\0',b'\x1e\0\0\0'];at=4;expected=4 if raw[:4]==b'\x02\0\0\0'else 6;names=['Loopback']
    else:
        assert link==1 and len(raw)>=14 and raw[12:14]in[b'\x08\0',b'\x86\xdd'];at=14;expected=4 if raw[12:14]==b'\x08\0'else 6;names=['Ethernet']
    end=len(raw);encapsulation=''
    for _ in range(5):
        required=20 if expected==4 else 40;available=end-at;assert available>=0;names.append('IPv'+str(expected))
        if available<required:return dict(expected=expected,available=available,required=required,observed=raw[at]>>4 if available else None,offset=at,names=names,encapsulation=encapsulation)
        assert expected==4 and raw[at]>>4==4,'unselected complete outer carrier'
        size=(raw[at]&15)*4;declared=int.from_bytes(raw[at+2:at+4],'big');assert size>=20 and size<=available
        assert not int.from_bytes(raw[at+6:at+8],'big')&0x3fff
        if declared==0:declared=available # Existing capture-decoder envelope; no on-wire validity claim.
        assert size<=declared
        protocol=raw[at+9];peers=sorted(str(ipaddress.ip_address(raw[at+i:at+i+4]))for i in[12,16]);end=min(end,at+declared);at+=size
        if protocol in[4,41]:
            expected=4 if protocol==4 else 6;encapsulation+=('/ipip:'if expected==4 else'/6in4:')+'~'.join(peers)
        else:
            assert protocol==47 and at+4<=end and raw[at:at+2]==b'\0\0'
            kind=raw[at+2:at+4];assert kind in[b'\x08\0',b'\x86\xdd'];expected=4 if kind==b'\x08\0'else 6
            names.append('GRE');encapsulation+='/gre:'+'~'.join(peers)+':0';at+=4
    raise AssertionError('bounded selected parent walk')


def audit(capture,events,cli,oracle,labels):
    native={}
    for event in rows(events):
        if event.get('expert')!='IPHeaderTruncated':continue
        refs=event['source_bytes'].get('PacketRefs')or[];assert len(refs)==1 and type(refs[0]['Number'])is int and refs[0]['Number']not in native
        native[refs[0]['Number']]=event
    assert native
    physical={n:(section,interface,link,raw)for n,section,interface,link,raw in records(capture)if n in native};assert set(physical)==set(native)
    reference={number(v['_source']['layers']['frame']['frame.number']):v['_source']['layers']for v in json.loads(oracle.read_text('utf8'))};assert set(native)<=set(reference)
    with labels.open(encoding='utf8')as f:literal=list(csv.reader(f,delimiter='\t'))
    assert literal[0][0]=='frame.number'and literal[0][1].casefold()=='_ws.col.protocol';columns={int(n):p for n,p in literal[1:]}
    checked=[];reference_columns=collections.Counter();native_columns=collections.Counter()
    for n,e in native.items():
        section,interface,link,raw=physical[n];header=short_header(raw,link);domain=dict(Section=section,Interface=interface,Encapsulation=header['encapsulation'])
        for d in[e['domain'],e['source_bytes']['PacketRefs'][0]['Domain']]:assert all(type(d[k])is int for k in['Section','Interface'])and type(d['Encapsulation'])is str
        assert e['source_bytes']['Kind']=='captured'and e['source_bytes']['PacketRefs']==[dict(Number=n,Domain=domain)]and e['domain']==domain
        assert type(e['id'])is int and e['id']>0 and e['protocol']=='ip'and e['transport']=='network'and e['status']==e['completeness']=='incomplete'
        assert type(e['flow'])is int and type(e.get('tcp_flow_id',0))is int and e['flow']==e.get('tcp_flow_id',0)==0 and not e['raw']and not e['source']and not e['destination']
        assert not e['fields']and e.get('fields_error')=='protocol parser: event is incomplete, not an exact message'and e.get('error')
        expected={'Expected IP Version':header['expected'],'Available Header Bytes':header['available'],'Required Header Bytes':header['required'],'Header Observation Scope':'incomplete fixed header; declared carrier only','Observed Header Layers':header['names']}
        if header['observed']is not None:expected['Observed IP Version']=header['observed']
        assert e['session']==expected and all(type(e['session'][k])is int for k in expected if k.endswith('Bytes')or k.endswith('Version'))
        assert e['summary']==f"IPv{header['expected']} header incomplete: {header['available']} of {header['required']} fixed bytes available; transport not admitted"
        layers=reference[n];frame=layers['frame_raw'];assert len(frame)==6 and all(type(v)is int for v in frame[1:])and frame[1:]==[0,len(raw),0,1,0]and bytes.fromhex(frame[0])==raw
        reference_columns[columns[n]]+=1
        checked.append(dict(frame=n,PDU=e['id'],domain=domain,header=header,original_frame_SHA256=hashlib.sha256(raw).hexdigest(),literal_reference_protocol=columns[n],literal_reference_experts=list(map(str,vals(layers,'_ws.expert.message')))))
    contributors=collections.Counter()
    for event in rows(events):
        for ref in event['source_bytes'].get('PacketRefs')or[]:
            if ref['Number']in native:
                assert event['id']==native[ref['Number']]['id'],'incomplete headers cannot contribute to another transport or application'
                contributors[ref['Number']]+=1
    assert contributors==collections.Counter({n:1 for n in native})
    seen=collections.Counter()
    for row in rows(cli):
        if row.get('record_type')=='protocol_publication'or row['number']not in native:continue
        n=row['number'];event=native[n];expected=event['session']['Expected IP Version'];assert row['protocol']=='IPv'+str(expected)and row['protocol_stack']==event['session']['Observed Header Layers']
        assert type(n)is int and type(row.get('tcp_flow_id',0))is int and type(row.get('stream_id',0))is int
        info=event['summary']
        if row['length']>row['captured_length']:info+=f" [truncated capture: {row['captured_length']}/{row['length']} bytes]"
        assert row['protocol_status']=='incomplete'and row['visibility']=='truncated-network-header'and row['info']==info
        assert not row.get('connection_protocol')and not row.get('tcp_flow_id',0)and not row.get('stream_id',0)and not row['source']and not row['destination']
        seen[n]+=1;native_columns[row['protocol']]+=1
    assert seen==collections.Counter({n:1 for n in native})
    return dict(scope=__doc__,capture_SHA256=sha(capture),SDK_SHA256=sha(events),CLI_SHA256=sha(cli),reference_SHA256=sha(oracle),reference_labels_SHA256=sha(labels),auditor_SHA256=sha(Path(__file__)),source_bound_short_headers=len(native),each_source_has_one_refusal_and_no_transport_contribution=True,literal_reference_columns=dict(reference_columns),literal_native_columns=dict(native_columns),observations=checked,complete_header_or_full_protocol_parity_claim=False)


if __name__=='__main__':
    ap=argparse.ArgumentParser(description=__doc__)
    for n in['capture','events','cli','oracle','labels','out']:ap.add_argument('--'+n,type=Path,required=True)
    a=ap.parse_args()
    for p in vars(a).values():assert not any((v/'.git').exists()for v in[p.resolve(),*p.resolve().parents])
    proof=audit(a.capture,a.events,a.cli,a.oracle,a.labels)
    with a.out.open('x',encoding='utf8')as f:json.dump(proof,f,indent=2);f.write('\n')
    print(json.dumps(dict(checked=proof['source_bound_short_headers'],source_and_refusal_exact=True)),flush=True)
