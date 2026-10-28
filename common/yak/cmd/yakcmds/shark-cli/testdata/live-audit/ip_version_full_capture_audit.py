"""Audit every reported version refusal in the original public fuzz capture.

Scope: source-bound Ethernet/IPv4, including observed IPv4-in-IPv4 headers.
Unselected parents are separate limited outer-prefix observations. Their child
bytes must never be used to certify another IP version or application identity.
Compare all native refusal claims to literal original header bytes and available
Wireshark version/expert fields. Reference-only observations remain explicit;
this does not certify every malformed carrier or every protocol field.
"""
import argparse,collections,hashlib,ipaddress,json
from pathlib import Path
from mail_rtsp_public_wire_audit import records
from owned_quic_clock_audit import vals,number


def sha(p):return hashlib.sha256(p.read_bytes()).hexdigest()
def rows(p):
    for l in p.open(encoding='utf8'):yield json.loads(l)
def header(raw):
    assert len(raw)>=34 and raw[12:14]==b'\x08\0'
    at=14;names=['Ethernet'];versions=[];domain=''
    for _ in range(5):
        assert at+20<=len(raw);got=raw[at]>>4;versions.append((at,got));names.append('IPv4')
        if got!=4:return got,domain,names,versions,at
        size=(raw[at]&15)*4;assert 20<=size and at+size+20<=len(raw)
        assert int.from_bytes(raw[at+6:at+8],'big')&0x3fff==0
        assert raw[at+9]==4
        peers=sorted(str(ipaddress.ip_address(raw[at+i:at+i+4]))for i in[12,16]);domain+='/ipip:'+'~'.join(peers);at+=size
    raise AssertionError('bounded declared IPIP profile')


def objects(value):
    if isinstance(value,dict):
        yield value
        for child in value.values():yield from objects(child)
    elif isinstance(value,list):
        for child in value:yield from objects(child)


def reference_versions(layers,raw,headers):
    """Bind each version to its own captured byte, never global field order.

    Wireshark exports an unmasked raw byte for the bogus IPv4 field, and may
    also export the same offset as a masked ip.version inside an IPv6 tree.
    Both forms have explicit literal checks. A bogus-field expert must belong
    to the same owning field node as the observed version.
    """
    frame=layers['frame_raw']
    assert len(frame)==6 and type(frame[0])is str and all(type(v)is int for v in frame[1:])
    assert frame[1:]==[0,len(raw),0,1,0]
    assert bytes.fromhex(frame[0])==raw,'reference uses the original captured frame'
    result=[]
    for offset,version in headers:
        found=[]
        for node in objects(layers):
            if 'ip.version'not in node or 'ip.version_raw'not in node:continue
            values=node['ip.version'];descriptors=node['ip.version_raw']
            if not isinstance(values,list):values=[values]
            if descriptors and isinstance(descriptors[0],str):descriptors=[descriptors]
            assert len(values)==len(descriptors),'version values keep their own raw descriptors'
            for value,descriptor in zip(values,descriptors):
                assert len(descriptor)==6
                literal,at,length,mask,kind,source=descriptor
                assert type(literal)is str and type(value)is str and all(type(v)is int for v in descriptor[1:])
                if at!=offset or length!=1 or source!=0:continue
                assert kind==4 and mask in[0,0xf0]
                wire=int(literal,16)
                assert wire==(raw[offset]if mask==0 else raw[offset]>>4)
                assert number(value)==version==raw[offset]>>4
                bogus=any(str(v)in['Bogus IPv4 version','Bogus IP version']for v in vals(node.get('ip.version_tree',{}),'_ws.expert.message'))
                found.append(dict(offset=offset,version=version,mask=mask,data_source=source,bogus_version_expert=bogus))
        assert found,('source-bound reference version unavailable',offset,version)
        if version!=4:assert any(v['bogus_version_expert']for v in found),'the refused header owns its bogus-version expert'
        result.append(found)
    return result


def audit(capture,events,oracle,reference_labels_path,cli=None,baseline_events=None):
    native={};parents={}
    for e in rows(events):
        if e.get('profile')=='network-encapsulation-refusal':
            refs=e['source_bytes'].get('PacketRefs')or[];assert len(refs)==1 and refs[0]['Number']not in parents
            parents[refs[0]['Number']]=e;continue
        if e.get('expert')!='IPVersionMismatch':continue
        refs=e['source_bytes'].get('PacketRefs')or[];assert len(refs)==1 and refs[0]['Number']not in native
        native[refs[0]['Number']]=e
    assert native
    source={}
    for n,section,interface,link,raw in records(capture):
        if n in native or n in parents:source[n]=(section,interface,link,raw)
    assert set(source)==set(native)|set(parents)and not(set(native)&set(parents))
    reference={}
    for r in json.loads(oracle.read_text('utf8')):
        layers=r['_source']['layers'];numbers=vals(layers,'frame.number');assert len(numbers)==1
        n=number(numbers[0]);assert n not in reference;reference[n]=layers
    assert set(native)<=set(reference),'every reported refusal needs its literal original reference view'
    literal_labels={}
    for record in json.loads(reference_labels_path.read_text('utf8')):
        layers=record['_source']['layers'];n=number(vals(layers,'frame.number')[0])
        label=next(v for k,v in layers.items()if k.casefold()=='_ws.col.protocol')
        assert n not in literal_labels and len(label)==1;literal_labels[n]=label[0]
    assert set(reference)==set(literal_labels),'raw trees and literal columns select the same original frames'
    checks=[];reference_labels=collections.Counter();parent_checks=[]
    for n,e in native.items():
        section,interface,link,raw=source[n];assert link==1
        try:got,encapsulation,names,versions,at=header(raw)
        except AssertionError as error:raise AssertionError(('independent carrier profile',n,e['domain'],e['session'],raw[:100].hex()))from error
        domain=dict(Section=section,Interface=interface,Encapsulation=encapsulation)
        assert e['source_bytes']['PacketRefs']==[dict(Number=n,Domain=domain)]and e['domain']==domain
        assert e['source_bytes']['Kind']=='captured'and not e['source_bytes']['SHA256']
        assert e['protocol']=='ip'and e['transport']=='network'and e['status']==e['completeness']=='malformed'and e['flow']==e.get('tcp_flow_id',0)==0
        assert not e['raw']and not e['source']and not e['destination']and e['error']==e['summary']==f'invalid IPv4 version {got}'
        assert e['session']=={'Expected IP Version':4,'Observed IP Version':got,'Observed Header Layers':names}
        assert type(e['session']['Observed IP Version'])is int
        bound=reference_versions(reference[n],raw,versions)
        label=literal_labels[n];reference_labels[label]+=1
        checks.append(dict(frame=n,PDU=e['id'],domain=domain,observed_version=got,expected_version=4,original_header_offset=at,raw_frame_SHA256=hashlib.sha256(raw).hexdigest(),reference_protocol=label,reference_header_fields=bound))
    for n,e in parents.items():
        section,interface,link,raw=source[n];assert link==1 and len(raw)>=34 and raw[12:14]==b'\x08\0'and raw[14]>>4==4
        assert e['protocol']=='ip'and e['status']==e['completeness']=='limited'and e['expert']=='EncapsulationDepthOrProfile'
        assert e['flow']==e.get('tcp_flow_id',0)==0 and not e['source']and not e['destination']and not e['raw']
        assert e['session']=={'Observed Header Layers':['Ethernet','IPv4'],'Header Observation Scope':'outer network prefix only','Child Header Identity':'unconfirmed','Encapsulation Admission':'unselected profile'}
        assert len(e['source_bytes']['PacketRefs'])==1 and e['source_bytes']['PacketRefs'][0]['Number']==n and e['domain']['Section']==section and e['domain']['Interface']==interface
        parent_checks.append(dict(frame=n,PDU=e['id'],raw_frame_SHA256=hashlib.sha256(raw).hexdigest(),outer_IP_protocol=raw[23],child_identity_confirmed=False))
    policy=None
    if baseline_events is not None:
        before=collections.Counter(tuple(r['Number']for r in e['source_bytes'].get('PacketRefs')or[])for e in rows(baseline_events)if e.get('expert')=='EncapsulationDepthOrProfile')
        after=collections.Counter((n,)for n in parents)
        assert before==after,'richer parent diagnostics preserve the prior unselected-profile refusal policy'
        policy=dict(baseline_events_SHA256=sha(baseline_events),same_original_refusal_frame_sets=True)
    refused={**native,**parents}
    contributors=0
    for e in rows(events):
        selected=[r for r in(e['source_bytes'].get('PacketRefs')or[])if r['Number']in refused]
        if selected:
            assert len(selected)==1 and e['id']==refused[selected[0]['Number']]['id'],'refused source bytes cannot contribute to another fragment or application PDU'
            contributors+=1
    assert contributors==len(refused)
    labels=None
    if cli is not None:
        seen=collections.Counter();labels=collections.Counter()
        for row in rows(cli):
            if row.get('record_type')=='protocol_publication'or row['number']not in refused:continue
            n=row['number'];e=refused[n]
            assert row['protocol']=='IPv4'and row['protocol_stack']==e['session']['Observed Header Layers']and row['protocol_status']==e['status']
            assert row['visibility']==('invalid-network-header'if n in native else'unselected-network-profile')
            assert not row.get('connection_protocol')and not row['source']and not row['destination']
            seen[n]+=1;labels[row['protocol']]+=1
        assert seen==collections.Counter({n:1 for n in refused})
    return dict(scope=__doc__,capture_SHA256=sha(capture),SDK_SHA256=sha(events),reference_SHA256=sha(oracle),reference_labels_SHA256=sha(reference_labels_path),CLI_SHA256=sha(cli)if cli else None,auditor_SHA256=sha(Path(__file__)),source_bound_version_refusals=len(native),source_contributors_exclusive=True,literal_reference_version_and_expert_checks=len(checks),unselected_parent_observations=parent_checks,parent_refusal_policy=policy,literal_reference_protocol_labels=dict(reference_labels),literal_native_protocol_labels=dict(labels)if labels else None,reference_only_original_frames=sorted(set(reference)-set(native)),reference_only_without_explicit_parent_refusal=sorted(set(reference)-set(refused)),refusals=checks,full_malformed_carrier_or_protocol_parity_claim=False)


if __name__=='__main__':
    ap=argparse.ArgumentParser(description=__doc__)
    for n in['capture','events','oracle','reference-labels-path','out']:ap.add_argument('--'+n,type=Path,required=True)
    ap.add_argument('--cli',type=Path);ap.add_argument('--baseline-events',type=Path);a=ap.parse_args()
    for p in vars(a).values():
        if p is not None:assert not any((v/'.git').exists()for v in[p.resolve(),*p.resolve().parents])
    proof=audit(a.capture,a.events,a.oracle,a.reference_labels_path,a.cli,a.baseline_events)
    with a.out.open('x',encoding='utf8',newline='\n')as f:json.dump(proof,f,indent=2);f.write('\n')
    print(json.dumps({k:v for k,v in proof.items()if k not in['scope','refusals']}),flush=True)
