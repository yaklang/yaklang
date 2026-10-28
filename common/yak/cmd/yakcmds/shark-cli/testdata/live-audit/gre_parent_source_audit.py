"""Source-bind unselected GRE/Geneve parents without confirming their children.

The original capture is authoritative for physical bytes and capture domains.
Only direct unsupported GRE versions, the declared option-free Geneve profile,
observed terminal GRE topology and bounded IPIP depth are adjudicated here.
Retained opaque extension/EtherIP/MPLS parents certify the literal outer protocol
and unchanged refusal policy, not any inferred inner grammar. Wireshark's own
raw fields/columns remain separately reported rather than rewritten to match us.
"""
import argparse,base64,collections,hashlib,ipaddress,json
from pathlib import Path
from mail_rtsp_public_wire_audit import records
from owned_quic_clock_audit import vals,number


def sha(p):return hashlib.sha256(p.read_bytes()).hexdigest()
def digest(w):return hashlib.sha256(w).hexdigest()
def canonical(v):return json.dumps(v,sort_keys=True,separators=(',',':'),ensure_ascii=True)
def rows(p):
    for line in p.open(encoding='utf8'):yield json.loads(line)
def integer(v):assert type(v)is int;return v
def parents(p):
    out={}
    for e in rows(p):
        if e.get('expert')!='EncapsulationDepthOrProfile':continue
        refs=e['source_bytes'].get('PacketRefs')or[];assert len(refs)==1
        n=integer(refs[0]['Number']);assert n>0 and n not in out;out[n]=e
    return out


def ipv4(w,at,end):
    assert at+20<=end and w[at]>>4==4
    hlen=(w[at]&15)*4;declared=int.from_bytes(w[at+2:at+4],'big')or(end-at)
    assert 20<=hlen<=declared and at+hlen<=end
    options=w[at+20:at+hlen]
    while options:
        if options[0]==0:break
        if options[0]==1:options=options[1:];continue
        assert len(options)>=2 and 2<options[1]<=len(options);options=options[options[1]:]
    end=min(end,at+declared);peers=sorted(str(ipaddress.ip_address(w[at+i:at+i+4]))for i in[12,16])
    return dict(offset=at,payload=at+hlen,end=end,protocol=w[at+9],fragmented=bool(int.from_bytes(w[at+6:at+8],'big')&0x3fff),peers='~'.join(peers),source=str(ipaddress.ip_address(w[at+12:at+16])),destination=str(ipaddress.ip_address(w[at+16:at+20])))


def gre(w,at,end):
    assert at+4<=end;start=at;flags=w[at:at+2];version=flags[1]&7;kind=int.from_bytes(w[at+2:at+4],'big');at+=4
    if flags[0]&0xc0:at+=4
    key=0
    if flags[0]&0x20:
        assert at+4<=end;key=int.from_bytes(w[at:at+4],'big');at+=4
    if flags[0]&0x10:at+=4
    assert at<=end
    if flags[0]&0x40:
        for _ in range(128):
            assert at+4<=end and at-start<=4096
            terminal=w[at:at+2]==b'\0\0'and w[at+3]==0;size=w[at+3];at+=4+size;assert at<=end
            if terminal:break
        else:raise AssertionError('bounded observed GRE route')
    if flags[1]&0x80:at+=4
    assert at<=end and at-start<=4096
    return dict(offset=start,payload=at,end=end,version=version,protocol=kind,key=key,flags=int.from_bytes(flags,'big'))


def observed_gre_domain(w,outer):
    """Trace literal observed identity tags; this never selects child semantics."""
    ip=outer;domain='';trace=[]
    for _ in range(8):
        assert ip['protocol']==47 and not ip['fragmented']
        try:g=gre(w,ip['payload'],ip['end'])
        except AssertionError:
            assert trace,'the directly reported GRE header must be complete';break
        trace.append(g);domain+='/gre:'+ip['peers']+':'+str(g['key'])
        if g['protocol']!=0x0800 or g['payload']+20>g['end']or w[g['payload']]>>4!=4:break
        try:ip=ipv4(w,g['payload'],g['end'])
        except AssertionError:break
        if ip['protocol']!=47:break
    else:raise AssertionError('bounded observed GRE domain')
    return domain,trace


def classify(raw,link,old=False):
    assert link==1 and len(raw)>=34 and raw[12:14]==b'\x08\0';outer=ipv4(raw,14,len(raw));assert not outer['fragmented']
    proto=outer['protocol'];at=outer['payload'];end=outer['end']
    if proto==47:
        domain,trace=observed_gre_domain(raw,outer);direct=trace[0]
        if direct['version']>1:return dict(kind='direct-unsupported-GRE',outer=outer,encapsulation=domain,trace=trace,version=direct['version'])
        assert len(trace)==2 and all(v['version']==0 for v in trace)and trace[-1]['protocol']==0 and trace[-1]['payload']==trace[-1]['end']
        return dict(kind='terminal-GRE-without-selected-network-child',outer=outer,encapsulation=domain,trace=trace)
    if proto==17:
        assert at+8<=end;sport=int.from_bytes(raw[at:at+2],'big');dport=int.from_bytes(raw[at+2:at+4],'big');length=int.from_bytes(raw[at+4:at+6],'big')
        assert 6081 in[sport,dport]and 8<=length<=end-at
        body=raw[at+8:at+length];assert len(body)>=8
        option_bytes=(body[0]&63)*4;assert 8+option_bytes<=len(body)
        h=body[:8];selected=option_bytes==0 and h[0]==h[1]==h[7]==0 and h[2:4]==b'\x65\x58';assert not selected
        vni=int.from_bytes(h[4:7],'big');domain='/geneve:'+outer['peers']+':'+str(vni)
        return dict(kind='unselected-Geneve-profile',outer=outer,encapsulation=domain,Geneve=dict(offset=at+8,header=h.hex(),version=h[0]>>6,option_bytes=option_bytes,flags=h[1],protocol=int.from_bytes(h[2:4],'big'),VNI=vni),UDP=dict(offset=at,source=sport,destination=dport,length=length,payload_SHA256=digest(body),preview=body[:64].hex(),payload_length=len(body)))
    if proto==4:
        ip=outer;domain='';trace=[]
        for _ in range(8):
            if ip['protocol']!=4 or ip['fragmented']:break
            domain+='/ipip:'+ip['peers'];trace.append(ip['offset']);ip=ipv4(raw,ip['payload'],ip['end'])
        else:raise AssertionError('bounded observed IPIP domain')
        assert len(trace)>4 and ip['fragmented']
        return dict(kind='IPIP-depth-exceeds-four',outer=outer,encapsulation=domain,IPIP_offsets=trace)
    assert old and proto in[0,97,137],'new opaque carrier needs separate adjudication'
    return dict(kind='retained-opaque-parent-policy',outer=outer,encapsulation='',outer_IP_protocol=proto,inner_grammar_confirmed=False)


def refusal(e,n,section,interface,claim):
    domain=dict(Section=section,Interface=interface,Encapsulation=claim['encapsulation'])
    assert all(type(e['domain'][k])is int for k in['Section','Interface'])and type(e['domain']['Encapsulation'])is str
    assert e['domain']==domain and canonical(e['source_bytes'])==canonical(dict(Kind='captured',SHA256='',ParentPDU=0,ParentPDUs=None,PacketRefs=[dict(Number=n,Domain=domain)]))
    assert type(e['id'])is int and e['id']>0 and e['protocol']=='ip'and e['transport']=='network'and e['profile']=='network-encapsulation-refusal'and e['expert']=='EncapsulationDepthOrProfile'
    assert e['status']==e['completeness']=='limited'and e['error']=='EncapsulationDepthOrProfile'and not e['fields']and e['fields_error']=='protocol parser: event is limited, not an exact message'
    assert all(type(e[k])is int and e[k]==0 for k in['flow','response_to','transaction'])and type(e.get('tcp_flow_id',0))is int and e.get('tcp_flow_id',0)==0 and not e['source']and not e['destination']and not e['raw']
    expected={'Observed Header Layers':['Ethernet','IPv4'],'Header Observation Scope':'outer network prefix only','Child Header Identity':'unconfirmed','Encapsulation Admission':'unselected profile'}
    summary='Encapsulation profile unavailable; inner identity unconfirmed'
    if claim['kind']=='direct-unsupported-GRE':
        expected.update({'Unselected Carrier':'GRE','Observed GRE Version':claim['version']});summary=f"GRE version {claim['version']} not selected; inner identity unconfirmed"
        assert type(e['session'].get('Observed GRE Version'))is int
    assert e['session']==expected and e['summary']==summary
    assert type(e['field_values']['pdu'])is int and e['field_values']['pdu']==e['id']and canonical(e['field_values']['source'])==canonical(e['source_bytes'])
    return domain


def projection(row,e,legacy_generic_marker=False):
    assert type(row['number'])is int and row['protocol']=='IPv4'and row['protocol_stack']==['Ethernet','IPv4']
    assert row['visibility']=='unselected-network-profile'and row['protocol_status']=='limited'and row['message_domain']==row['capture_domain']==e['domain']
    assert not row['source']and not row['destination']and not row.get('connection_protocol')and all(type(row.get(k,0))is int and row.get(k,0)==0 for k in['tcp_flow_id','stream_id'])
    info=e['summary']
    if row['length']>row['captured_length']:info+=f" [truncated capture: {row['captured_length']}/{row['length']} bytes]"
    accepted=[info]
    if legacy_generic_marker:accepted.append(info+' [incomplete/undecoded]')
    assert row['info']in accepted
    ps=row.get('pdus')or[];assert len(ps)==1 and type(ps[0]['id'])is int and ps[0]['id']==e['id']and ps[0]['protocol']=='ip'and ps[0]['status']=='limited'and canonical(ps[0]['byte_source'])==canonical(e['source_bytes'])


def exclusive(events,refused):
    count=collections.Counter()
    for e in events:
        selected=[r for r in(e['source_bytes'].get('PacketRefs')or[])if r['Number']in refused]
        if selected:
            assert len(selected)==1 and e['id']==refused[selected[0]['Number']]['id'],'parent-refused bytes cannot contribute to a second native/transport/application observation'
            count[selected[0]['Number']]+=1
    assert count==collections.Counter({n:1 for n in refused})


def removed_udp(e,n,claim,domain):
    refs=e['source_bytes'].get('PacketRefs')or[]
    assert claim['kind']=='unselected-Geneve-profile'and e['protocol']==''and e['status']==e['completeness']=='unrecognized'and e['transport']=='udp'and e['flow']==0
    udp=claim['UDP'];outer=claim['outer'];source=outer['source']+':'+str(udp['source']);destination=outer['destination']+':'+str(udp['destination'])
    assert e['domain']==domain and refs==[dict(Number=n,Domain=domain)]and e['source']==source and e['destination']==destination
    preview=bytes.fromhex(udp['preview'])
    # Unrecognized UDP observations retain a 64-byte preview; their native
    # byte-source digest covers that preview, not the omitted full payload.
    assert canonical(e['source_bytes'])==canonical(dict(Kind='captured',SHA256=digest(preview),ParentPDU=0,ParentPDUs=None,PacketRefs=refs))
    assert base64.b64decode(e['raw'],validate=True)==preview and not e['session']and not e['fields']
    assert e['summary']==f"UDP application protocol not recognized ({source} -> {destination}, {udp['payload_length']} bytes)"
    return dict(frame=n,domain=domain,before_PDU=e['id'],before_event_canonical_SHA256=digest(canonical(e).encode()),before_source_bytes=e['source_bytes'],before_protocol=e['protocol'],before_status=e['status'],before_transport=e['transport'],wire_UDP_payload_SHA256=udp['payload_SHA256'],old_preview_SHA256=digest(preview))


def objects(v):
    if isinstance(v,dict):
        yield v
        for child in v.values():yield from objects(child)
    elif isinstance(v,list):
        for child in v:yield from objects(child)


def raw_scalar(layers,name,offset,length,wire,value):
    matches=[]
    for node in objects(layers):
        if name not in node or name+'_raw'not in node:continue
        values=node[name];values=values if isinstance(values,list)else[values]
        descriptors=node[name+'_raw'];descriptors=descriptors if descriptors and isinstance(descriptors[0],list)else[descriptors]
        assert len(values)==len(descriptors)
        for scalar,descriptor in zip(values,descriptors):
            if len(descriptor)!=6:continue
            literal,at,size,mask,kind,source=descriptor
            assert type(literal)is str and type(scalar)is str and all(type(x)is int for x in descriptor[1:])
            if at!=offset or size!=length or source!=0:continue
            assert mask==0 and kind in[4,5,7]and bytes.fromhex(literal)==wire[offset:offset+length]and number(scalar)==value
            matches.append(descriptor)
    assert matches,('source-bound reference field unavailable',name,offset,length)
    return matches


def reference_proof(capture,proof,oracle,columns=None):
    targets={v['frame']:v for v in proof['observations']};raws={n:w for n,s,i,l,w in records(capture)if n in targets};checks=[];observed=set();column_counts=collections.Counter()
    # The filtered export may include transformed children with source indexes
    # >0. Only a descriptor of the original physical frame (source 0) can bind
    # our literal outer GRE/IP header; no first/global field-order matching.
    reference=json.loads(oracle.read_text('utf8'))
    literal={}
    if columns:
        for line in columns.open(encoding='utf8'):
            n,label=line.rstrip('\r\n').split('\t',1)
            if n=='frame.number':continue
            literal[int(n)]=label
    for record in reference:
        layers=record['_source']['layers'];n=number(layers['frame']['frame.number'])
        if n not in targets:continue
        assert n not in observed;observed.add(n);wire=raws[n];frame=layers['frame_raw']
        assert len(frame)==6 and type(frame[0])is str and all(type(x)is int for x in frame[1:])and frame[1:]==[0,len(wire),0,1,0]and bytes.fromhex(frame[0])==wire
        claim=targets[n]['claim'];outer=claim['outer'];ip=raw_scalar(layers,'ip.proto',outer['offset']+9,1,wire,outer['protocol']);gre_fields=[]
        if claim['kind']in['direct-unsupported-GRE','terminal-GRE-without-selected-network-child']:
            g=claim['trace'][0];gre_fields=raw_scalar(layers,'gre.flags_and_version',g['offset'],2,wire,g['flags'])
            assert wire[g['offset']+1]&7==g['version']
        if columns:assert n in literal;column_counts[literal[n]]+=1
        checks.append(dict(frame=n,outer_IP_protocol_raw=ip,direct_GRE_flags_and_version_raw=gre_fields,literal_reference_protocol=literal.get(n),child_grammar_confirmed=False))
    assert observed==set(targets),'every parent claim needs its original reference frame'
    return dict(capture_SHA256=sha(capture),SDK_SHA256=proof['SDK_SHA256'],reference_SHA256=sha(oracle),reference_columns_SHA256=sha(columns)if columns else None,source_bound_parent_frames=len(checks),source_zero_raw_GRE_flags_and_version_fields=sum(bool(v['direct_GRE_flags_and_version_raw'])for v in checks),literal_reference_columns=dict(column_counts),reference_observations=checks,matching_child_protocol_parity_claim=False)


def audit(capture,events,cli,baseline_events,legacy_generic_marker=False):
    native=parents(events);prior=parents(baseline_events);assert native and prior
    physical={n:(s,i,l,w)for n,s,i,l,w in records(capture)if n in native or n in prior};assert set(physical)==set(native)|set(prior)
    claims={};checks=[];kinds=collections.Counter();versions=collections.Counter();generic=collections.Counter()
    for n,e in native.items():
        s,i,link,raw=physical[n]
        try:claim=classify(raw,link,n in prior);domain=refusal(e,n,s,i,claim)
        except AssertionError as error:raise AssertionError(('source-bound parent claim',n,e['domain'],e['session']))from error
        claims[n]=claim;kinds[claim['kind']]+=1
        if 'version'in claim:versions[claim['version']]+=1
        else:generic[claim['outer']['protocol']]+=1
        checks.append(dict(frame=n,PDU=e['id'],domain=domain,source_frame_SHA256=digest(raw),claim=claim))
    exclusive(rows(events),native)
    seen=collections.Counter();markers=0
    for row in rows(cli):
        if row.get('record_type')=='protocol_publication'or row['number']not in native:continue
        n=row['number'];projection(row,native[n],legacy_generic_marker);seen[n]+=1;markers+=row['info'].endswith(' [incomplete/undecoded]')
    assert seen==collections.Counter({n:1 for n in native})
    retained=set(prior)&set(native);replaced=set(prior)-set(native);replacements=[]
    selected={}
    for e in rows(events):
        for ref in e['source_bytes'].get('PacketRefs')or[]:
            if ref['Number']in replaced:
                assert ref['Number']not in selected;selected[ref['Number']]=e
    assert set(selected)==replaced
    for n,e in selected.items():
        s,i,link,raw=physical[n];at=14;ip=ipv4(raw,at,len(raw));domain='';names=['Ethernet','IPv4']
        for _ in range(4):
            assert ip['protocol']==4 and not ip['fragmented'];domain+='/ipip:'+ip['peers'];names.append('IPv4');at=ip['payload']
            if at==ip['end']:break
            ip=ipv4(raw,at,ip['end'])
        else:raise AssertionError('replacement requires selected bounded parent path')
        assert e['expert']=='IPHeaderTruncated'and e['status']==e['completeness']=='incomplete'
        assert e['session']=={'Available Header Bytes':0,'Expected IP Version':4,'Header Observation Scope':'incomplete fixed header; declared carrier only','Observed Header Layers':names,'Required Header Bytes':20}
        assert e['domain']==prior[n]['domain']==dict(Section=s,Interface=i,Encapsulation=domain)and e['source_bytes']==prior[n]['source_bytes']
        replacements.append(dict(frame=n,source_frame_SHA256=digest(raw),before_PDU=prior[n]['id'],after_PDU=e['id'],new_expert=e['expert'],domain=e['domain'],zero_header_offset=at,selected_parent_depth=len(names)-2))
    for n in retained:
        assert native[n]['domain']==prior[n]['domain']and native[n]['source_bytes']==prior[n]['source_bytes']and prior[n]['session']['Child Header Identity']=='unconfirmed'
    removed=[];candidate_frames=set(native)-set(prior)
    for e in rows(baseline_events):
        if e['protocol']=='ip':continue
        refs=e['source_bytes'].get('PacketRefs')or[];hits=[r for r in refs if r['Number']in candidate_frames]
        if not hits:continue
        assert len(refs)==len(hits)==1;n=hits[0]['Number'];claim=claims[n]
        certificate=removed_udp(e,n,claim,native[n]['domain']);certificate.update(source_frame_SHA256=digest(physical[n][3]),replacement_parent_PDU=native[n]['id'],wire_profile=claim['kind']);removed.append(certificate)
    assert {v['frame']for v in removed}=={n for n in candidate_frames if claims[n]['kind']=='unselected-Geneve-profile'}and len(removed)==len({v['frame']for v in removed})
    return dict(scope=__doc__,auditor_SHA256=sha(Path(__file__)),capture_SHA256=sha(capture),SDK_SHA256=sha(events),CLI_SHA256=sha(cli),baseline_SDK_SHA256=sha(baseline_events),source_bound_parent_refusals=len(native),categories=dict(kinds),direct_GRE_versions=dict(versions),generic_outer_protocols=dict(generic),exclusive_refused_sources=True,full_observed_domains_preserved=True,CLI_refusals=len(seen),legacy_generic_markers_allowed=legacy_generic_marker,observed_legacy_generic_markers=markers,old_parent_count=len(prior),retained_old_parent_count=len(retained),replaced_old_parents=replacements,authorized_removed_non_IP_observations=removed,observations=checks,complete_inner_protocol_or_full_field_parity_claim=False)


if __name__=='__main__':
    ap=argparse.ArgumentParser(description=__doc__)
    for n in['capture','events','cli','baseline-events','out']:ap.add_argument('--'+n,type=Path,required=True)
    ap.add_argument('--allow-legacy-generic-marker',action='store_true');ap.add_argument('--oracle',type=Path);ap.add_argument('--reference-columns',type=Path);a=ap.parse_args()
    for p in[a.capture,a.events,a.cli,a.baseline_events,a.out,a.oracle,a.reference_columns]:
        if p is not None:assert not any((v/'.git').exists()for v in[p.resolve(),*p.resolve().parents])
    proof=audit(a.capture,a.events,a.cli,a.baseline_events,a.allow_legacy_generic_marker)
    if a.oracle:proof['reference']=reference_proof(a.capture,proof,a.oracle,a.reference_columns)
    with a.out.open('x',encoding='utf8')as f:json.dump(proof,f,indent=2);f.write('\n')
    print(json.dumps({k:v for k,v in proof.items()if k not in['scope','observations','reference','authorized_removed_non_IP_observations']}),flush=True)
