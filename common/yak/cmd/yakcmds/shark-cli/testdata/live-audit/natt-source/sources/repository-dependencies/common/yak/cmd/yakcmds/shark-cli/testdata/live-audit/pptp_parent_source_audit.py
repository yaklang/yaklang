"""Source-bind PPTP parent refusals, single-direction CallID and child bytes.

Scope: the authored Ethernet/IPv4 GRE-v1 controls and original direct
Ethernet/IPv4 GRE-v1 public witnesses. Wireshark's permissive child labels do
not certify RFC 2637 parent validity. A stable observed directional CallID is
not a negotiated PPTP call, peer CallID pairing, PFC agreement or PPP endpoint.
"""
import argparse,base64,collections,copy,datetime,hashlib,ipaddress,json,re,struct
from pathlib import Path
from capture_clock_wire_audit import physical
from gre_parent_source_audit import ipv4,gre,raw_scalar,canonical
from owned_quic_clock_audit import vals,number


def sha(p):return hashlib.sha256(p.read_bytes()).hexdigest()
def digest(w):return hashlib.sha256(w).hexdigest()
def rows(p):
    for line in p.open(encoding='utf8'):yield json.loads(line)
def same(a,b):assert canonical(a)==canonical(b)
def checksum(w):
    if len(w)%2:w+=b'\0'
    value=sum(struct.unpack('>'+str(len(w)//2)+'H',w))
    while value>>16:value=(value&65535)+(value>>16)
    return(~value)&65535
def record_domain(r,encapsulation=''):return dict(Section=r['section'],Interface=r['interface'],Encapsulation=encapsulation)
def stamp(n):
    seconds,nanos=divmod(n,1000000000);text=datetime.datetime.fromtimestamp(seconds,datetime.timezone.utc).strftime('%Y-%m-%dT%H:%M:%S')
    return text+('.'+f'{nanos:09}'.rstrip('0')if nanos else'')+'Z'
def instant(text):
    match=re.fullmatch(r'(\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d)(?:\.(\d{1,9}))?(Z|[+-]\d\d:\d\d)',text);assert match
    base=datetime.datetime.fromisoformat(match[1]+match[3].replace('Z','+00:00')).astimezone(datetime.timezone.utc);delta=base-datetime.datetime(1970,1,1,tzinfo=datetime.timezone.utc)
    return(delta.days*86400+delta.seconds)*1000000000+int((match[2]or'').ljust(9,'0'))


def profile(record):
    """Interpret only literal direct-parent framing, with independent bit tests."""
    raw=record['wire'];assert record['link']==1 and raw[12:14]==b'\x08\0';outer=ipv4(raw,14,len(raw));assert outer['protocol']==47 and not outer['fragmented']
    start=outer['payload'];wire=raw[start:outer['end']];assert len(wire)>=2
    flags=int.from_bytes(wire[:2],'big');version=wire[1]&7;assert version in[0,1]
    result=dict(offset=start,available=len(wire),flags=flags,version=version,outer=outer)
    if version==0:
        g=gre(raw,start,outer['end']);assert g['protocol']==0x0800
        return dict(result,kind='generic-v0',payload_start=g['payload'],payload_end=g['end'],domain='/gre:'+outer['peers']+':'+str(g['key']),protocol=g['protocol'])
    def refused(reason,required,truncated=False):return dict(result,kind='refused',reason=reason,required=required,truncated=truncated,domain='')
    if len(wire)<4:return refused('incomplete protocol type',4,True)
    result['protocol']=int.from_bytes(wire[2:4],'big')
    C,R,K,S,s,recur,A,reserved=bool(wire[0]&128),bool(wire[0]&64),bool(wire[0]&32),bool(wire[0]&16),bool(wire[0]&8),wire[0]&7,bool(wire[1]&128),(wire[1]>>3)&15
    if C or R or not K or s or recur or reserved:return refused('unselected flags; K is required and reserved bits must be zero',8)
    if result['protocol']!=0x880b:return refused('protocol type must be PPP (0x880b)',8)
    if len(wire)<8:return refused('incomplete payload length and Call ID',8,True)
    length=int.from_bytes(wire[4:6],'big');call=int.from_bytes(wire[6:8],'big');header=8+4*int(S)+4*int(A)
    if len(wire)<header:return refused('incomplete sequence or acknowledgment field',header,True)
    if S!=(length!=0)or(not S and not A):return refused('data requires S and a payload; acknowledgment-only requires A and zero payload',header)
    if len(wire)-header!=length:return refused(f'declared payload length {length} differs from {len(wire)-header} available bytes',header+length,len(wire)-header<length)
    body=wire[header:];protocol_start=0
    if body:
        if body[0]==255:
            if len(body)<2:return refused('incomplete PPP address/control prefix',header+2,True)
            if body[1]==3:protocol_start=2
        tail=body[protocol_start:]
        if not tail or(not tail[0]&1 and len(tail)<2):return refused('incomplete PPP protocol field',header+length+1,True)
        if not tail[0]&1 and not tail[1]&1:return refused('invalid PPP protocol field',header)
        protocol_size=1 if tail[0]&1 else 2;PPP=int.from_bytes(tail[:protocol_size],'big');inner=start+header+protocol_start+protocol_size
    else:PPP=None;inner=None
    domain='/pptp:'+outer['source']+'>'+outer['destination']+':'+str(call)
    return dict(result,kind='data'if body else'ack',header=header,payload_length=length,call_ID=call,sequence=int.from_bytes(wire[8:12],'big')if S else None,ack=int.from_bytes(wire[header-4:header],'big')if A else None,PPP_protocol=PPP,payload_start=inner,payload_end=outer['end'],domain=domain)


def expected_refusal_session(claim):
    assert claim['kind']=='refused'
    session={'Observed Header Layers':['Ethernet','IPv4'],'Header Observation Scope':'outer network prefix only','Child Header Identity':'unconfirmed','Encapsulation Admission':'unselected profile','Unselected Carrier':'PPTP','Observed GRE Version':1,'Parent Admission Reason':claim['reason'],'GRE Header Flags':claim['flags'],'Available GRE Bytes':claim['available'],'Required GRE Bytes':claim['required']}
    if 'protocol'in claim:session['GRE Protocol Type']=claim['protocol']
    if claim['truncated']:session['Declared GRE Bytes Unavailable']=True
    return session


def refusal(event,n,record,claim):
    assert type(n)is int and n>0;domain=record_domain(record)
    same(event['domain'],domain);same(event['source_bytes'],dict(Kind='captured',SHA256='',ParentPDU=0,ParentPDUs=None,PacketRefs=[dict(Number=n,Domain=domain)]))
    assert type(event['id'])is int and event['id']>0 and event['protocol']=='ip'and event['transport']=='network'and event['profile']=='network-encapsulation-refusal'
    assert event['status']==event['completeness']=='limited'and event['expert']=='PPTPHeaderAdmission'
    assert all(type(event.get(k,0))is int and event.get(k,0)==0 for k in['flow','tcp_flow_id','transaction','response_to'])
    assert not event['raw']and not event['source']and not event['destination']and not event['fields']and event['fields_error']=='protocol parser: event is limited, not an exact message'
    same(event['session'],expected_refusal_session(claim));assert event['error']=='PPTP GRE header: '+claim['reason']and event['summary']==event['error']+'; inner identity unconfirmed'
    assert type(event['field_values']['pdu'])is int and event['field_values']['pdu']==event['id'];same(event['field_values']['source'],event['source_bytes'])
    return domain


def refusal_row(row,event,n,record):
    assert type(row['number'])is int and row['number']==n and row['length']==row['captured_length']==len(record['wire'])and instant(row['time'])==record['time_ns']
    assert row['protocol']=='IPv4'and row['protocol_stack']==['Ethernet','IPv4']and row['protocol_status']=='limited'and row['visibility']=='unselected-network-profile'
    assert row['info']==event['summary']and not row['source']and not row['destination']and not row.get('connection_protocol')
    assert all(type(row.get(k,0))is int and row.get(k,0)==0 for k in['stream_id','tcp_flow_id']);same(row['message_domain'],event['domain']);same(row['capture_domain'],event['domain'])
    pdus=row.get('pdus')or[];assert len(pdus)==1 and type(pdus[0]['id'])is int and pdus[0]['id']==event['id']and pdus[0]['protocol']=='ip'and pdus[0]['status']=='limited';same(pdus[0]['byte_source'],event['source_bytes'])


def exclusive(events,refused):
    seen=collections.Counter()
    for event in events:
        refs=event['source_bytes'].get('PacketRefs')or[];selected=[r for r in refs if r['Number']in refused]
        if selected:
            assert len(refs)==len(selected)==1 and event['id']==refused[selected[0]['Number']]['id'],'refused source cannot also contribute to another observation'
            seen[selected[0]['Number']]+=1
    assert seen==collections.Counter({n:1 for n in refused})


def reference_frame(layers,record,claim):
    wire=record['wire'];frame=layers['frame_raw'];assert len(frame)==6 and all(type(v)is int for v in frame[1:])and frame[1:]==[0,len(wire),0,1,0]and bytes.fromhex(frame[0])==wire
    raw_scalar(layers,'gre.flags_and_version',claim['offset'],2,wire,claim['flags'])
    if 'protocol'in claim:raw_scalar(layers,'gre.proto',claim['offset']+2,2,wire,claim['protocol'])
    if claim['kind']in['data','ack']:
        raw_scalar(layers,'gre.key.payload_length',claim['offset']+4,2,wire,claim['payload_length']);raw_scalar(layers,'gre.key.call_id',claim['offset']+6,2,wire,claim['call_ID'])
    return dict(literal_reference_layers=layers['frame']['frame.protocols'],raw_frame_SHA256=digest(wire),raw_parent_fields_use_original_source_zero=True,reference_child_labels_do_not_certify_parent_validity=True)


def child(record,claim):
    assert claim['kind']in['data','generic-v0']and(claim['kind']=='generic-v0'or claim['PPP_protocol']==0x21)
    raw=record['wire'];ip=ipv4(raw,claim['payload_start'],claim['payload_end']);assert not ip['fragmented']and checksum(raw[ip['offset']:ip['payload']])==0
    payload=raw[ip['payload']:ip['end']];protocol=ip['protocol'];pseudo=raw[ip['offset']+12:ip['offset']+20]+b'\0'+bytes([protocol])+struct.pack('>H',len(payload));assert checksum(pseudo+payload)==0
    if protocol==17:
        assert len(payload)>=8 and int.from_bytes(payload[4:6],'big')==len(payload)
        return dict(protocol='udp',source_port=int.from_bytes(payload[:2],'big'),destination_port=int.from_bytes(payload[2:4],'big'),payload=payload[8:],IP=ip)
    assert protocol==6 and len(payload)>=20;header=(payload[12]>>4)*4;assert 20<=header<=len(payload)
    return dict(protocol='tcp',source_port=int.from_bytes(payload[:2],'big'),destination_port=int.from_bytes(payload[2:4],'big'),sequence=int.from_bytes(payload[4:8],'big'),flags=payload[13],payload=payload[header:],IP=ip)


def message_source(event,domain,refs,kind,wire):
    expected=dict(Kind=kind,SHA256=digest(wire),ParentPDU=0,ParentPDUs=None,PacketRefs=[dict(Number=n,Domain=domain)for n in refs],CompletedIn=dict(Number=max(refs),Domain=domain))
    same(event['domain'],domain);same(event['source_bytes'],expected);same(event['field_values']['source'],expected)
    assert base64.b64decode(event['raw'],validate=True)==wire and event['status']=='decoded'and event['completeness']=='message'and event['fields']and not event.get('fields_error')


def without_evidence(event):
    result=copy.deepcopy(event)
    for name in['domain','source_bytes']:result.pop(name)
    result['field_values'].pop('source');return result


def TCP_call(records,events,flow,cli,reference,control_event=None):
    assert len(records)==len(reference)==3;claims=[profile(r)for r in records];assert all(c['kind']=='data'and c['call_ID']==42 for c in claims)
    assert len({c['payload_length']for c in claims})==3 and len({c['domain']for c in claims})==1
    parts=[child(r,c)for r,c in zip(records,claims)];assert parts[0]['flags']&2 and not parts[0]['payload']and parts[1]['sequence']==parts[0]['sequence']+1 and parts[2]['sequence']==parts[1]['sequence']+len(parts[1]['payload'])
    body=parts[1]['payload']+parts[2]['payload'];assert body==b'GET /gre-v1 HTTP/1.1\r\nHost: example.test\r\n\r\n'
    domain=record_domain(records[0],claims[0]['domain']);assert len(events)==1;event=events[0];assert event['protocol']=='http'and type(event['flow'])is int and event['flow']==1 and type(event['tcp_flow_id'])is int and event['tcp_flow_id']==1
    message_source(event,domain,[2,3],'reassembled',body)
    if control_event:same(without_evidence(event),without_evidence(control_event))
    assert not flow['error']and type(flow['stats']['CapturedPackets'])is int and flow['stats']['CapturedPackets']==3 and flow['stats']['DecodeErrors']==0
    flows=flow.get('flows')or[];assert len(flows)==1 and type(flows[0]['flow'])is int and flows[0]['flow']==1 and flows[0]['half_open']is False;assert flows[0]['source']==parts[0]['IP']['source']+':'+str(parts[0]['source_port'])and flows[0]['destination']==parts[0]['IP']['destination']+':'+str(parts[0]['destination_port']);same(flows[0]['domain'],domain)
    packets=flow.get('packets')or[];assert len(packets)==3
    for n,packet in enumerate(packets,1):
        assert type(packet['flow'])is int and packet['flow']==1 and type(packet['direction'])is int and packet['direction']==0 and packet['captured']==stamp(records[n-1]['time_ns']);same(packet['refs'],[dict(Number=n,Domain=domain)])
    assert len(flow['closed'])==1 and type(flow['closed'][0]['flow'])is int and flow['closed'][0]['flow']==1 and flow['closed'][0]['pending_gaps']is False
    physical_rows=[r for r in cli if r.get('record_type')!='protocol_publication'];assert [r['number']for r in physical_rows]==[1,2,3]
    for row in physical_rows:
        assert type(row.get('tcp_flow_id'))is int and row['tcp_flow_id']==1 and type(row.get('stream_id'))is int and row['stream_id']==1
    selected=physical_rows[-1];assert selected['protocol']=='HTTP/1.1'and selected['protocol_stack']==['Ethernet','IPv4','GRE','PPP','IPv4','TCP','HTTP/1.1'];same(selected['message_domain'],domain)
    streams=[];bound=[]
    for n,(ref,record,claim)in enumerate(zip(reference,records,claims),1):
        layers=ref['_source']['layers'];bound.append(reference_frame(layers,record,claim));values=vals(layers,'tcp.stream');assert len(values)==1;streams.append(number(values[0]));assert vals(layers,'tcp.checksum.status')==['1']
    assert len(set(streams))==1 and vals(reference[-1]['_source']['layers'],'http.request.method')==['GET']and vals(reference[-1]['_source']['layers'],'http.request.uri')==['/gre-v1']
    return dict(direction_domain=domain,literal_CallID=42,literal_payload_lengths=[c['payload_length']for c in claims],literal_reference_TCP_streams=streams,native_TCP_flows=1,native_HTTP_messages=1,HTTP_wire_SHA256=digest(body),HTTP_source_frames=[2,3],HTTP_field_tree_and_typed_values_equal_generic_control=control_event is not None,reference=bound,negotiated_PPTP_call_or_bidirectional_association_claim=False)


def ACK_header(record,event,flow,cli):
    claim=profile(record);assert claim['kind']=='ack'and claim['call_ID']==42 and claim['ack']==7;domain=record_domain(record,claim['domain'])
    assert event['protocol']=='ip'and event['transport']=='network'and event['profile']=='pptp-acknowledgment'and event['status']=='limited'and event['completeness']=='headers'and event['expert']=='PPTPAcknowledgment'
    same(event['session'],{'Observed GRE Version':1,'PPTP Call ID':42,'PPTP Payload Length':0,'PPTP Acknowledgment Number':7,'Header Observation Scope':'PPTP acknowledgment header only','Child Header Identity':'not present','PPTP Data Path Scope':'one direction; call-control association unavailable'})
    same(event['domain'],domain);same(event['source_bytes'],dict(Kind='captured',SHA256='',ParentPDU=0,ParentPDUs=None,PacketRefs=[dict(Number=1,Domain=domain)]));assert not event['fields']and not event['raw']and not event['source']and not event['destination']and not event.get('error')
    assert event['fields_error']=='protocol parser: event is limited, not an exact message'and all(type(event.get(k,0))is int and event.get(k,0)==0 for k in['flow','tcp_flow_id','transaction','response_to'])
    assert not flow['error']and not(flow.get('flows')or flow.get('packets')or flow.get('closed'))and flow['stats']['CapturedPackets']==1 and flow['stats']['DecodeErrors']==0
    assert len(cli)==1;row=cli[0];assert type(row['number'])is int and row['number']==1 and row['protocol']in['GRE','PPTP']and not row.get('connection_protocol')and all(type(row.get(k,0))is int and row.get(k,0)==0 for k in['tcp_flow_id','stream_id'])
    ps=row.get('pdus')or[];assert len(ps)==1 and ps[0]['id']==event['id']and ps[0]['status']=='limited';same(ps[0]['byte_source'],event['source_bytes'])
    return dict(domain=domain,native_header_only=True,PPP_or_transport_state_created=False,literal_CLI_protocol=row['protocol'],literal_CLI_info=row['info'],native_summary_visible_in_CLI=event['summary']in row['info'],full_PPTP_session_claim=False)


def audit_controls(manifest,exports,baseline,ack_capture=None,ack_exports=None):
    document=json.loads(manifest.read_text('utf8'));cases=document['cases'];assert len(cases)==15
    controls={};checked=[];refused_cases=0;physical_frames=0;reference_permissive=0
    for case in cases:
        capture=Path(case['capture']);assert sha(capture)==case['capture_SHA256'];records=physical(capture);assert len(records)==len(case['frames'])
        for n,(record,entry)in enumerate(zip(records,case['frames']),1):assert entry['frame']==n and entry['wire_SHA256']==digest(record['wire'])and entry['captured_length']==len(record['wire'])
        current=exports/case['name'];old=baseline/case['name'];events=list(rows(current/'events.jsonl'));cli=list(rows(current/'yak.jsonl'));reference=json.loads((old/'oracle.json').read_text('utf8'));assert len(reference)==len(records)
        claims=[profile(r)for r in records];physical_frames+=len(records);bound=[reference_frame(v['_source']['layers'],r,c)for v,r,c in zip(reference,records,claims)]
        proof=dict(case=case['name'],capture_SHA256=sha(capture),SDK_SHA256=sha(current/'events.jsonl'),CLI_SHA256=sha(current/'yak.jsonl'),reference_SHA256=sha(old/'oracle.json'),reference=bound)
        if claims[0]['kind']=='refused':
            assert len(records)==len(events)==1;event=events[0];refusal(event,1,records[0],claims[0]);exclusive(events,{1:event})
            selected=[v for v in cli if v.get('record_type')!='protocol_publication'];assert len(selected)==1;refusal_row(selected[0],event,1,records[0]);refused_cases+=1
            flow=json.loads((current/'flows.json').read_text());assert not flow['error']and not(flow.get('flows')or flow.get('packets')or flow.get('closed'))and type(flow['stats']['CapturedPackets'])is int and flow['stats']['CapturedPackets']==1
            reference_permissive+=any('dns'in v['literal_reference_layers'].split(':')for v in bound)
            proof.update(outcome='precise-parent-refusal',reason=claims[0]['reason'],one_refusal_and_no_child_contribution=True,no_TCP_transport_namespace_created=True,flows_SHA256=sha(current/'flows.json'))
        elif case['name'].startswith('GRE-v0-'):
            for file in['events.jsonl','yak.jsonl',*(['flows.json']if'TCP'in case['name']else[])]:assert(current/file).read_bytes()==(old/file).read_bytes(),('generic v0 changed',case['name'],file)
            proof.update(outcome='generic-v0-control-preserved',complete_export_byte_identical=True)
        elif case['name']=='PPTP-v1-DNS-header-control':
            assert len(records)==len(events)==1 and claims[0]['kind']=='data';event=events[0];body=child(records[0],claims[0]);assert body['protocol']=='udp'and event['protocol']=='dns'
            domain=record_domain(records[0],claims[0]['domain']);message_source(event,domain,[1],'captured',body['payload']);prior=list(rows(old/'events.jsonl'));assert len(prior)==1;same(without_evidence(event),without_evidence(prior[0]))
            proof.update(outcome='validated-PPTP-header-and-observed-PPP-data',field_trees_typed_values_plaintext_and_session_unchanged=True,old_domain=prior[0]['domain'],new_domain=event['domain'],wire_DNS_id=int.from_bytes(body['payload'][:2],'big'))
        else:
            assert case['name']=='PPTP-v1-TCP-variable-length';assert(current/'flows.json').exists();control=list(rows(exports/'GRE-v0-TCP-constant-key/events.jsonl'));assert len(control)==1
            proof.update(outcome='same-direction-CallID-TCP-reassembly',TCP=TCP_call(records,events,json.loads((current/'flows.json').read_text()),cli,reference,control[0]),flows_SHA256=sha(current/'flows.json'))
        checked.append(proof)
    assert physical_frames==19 and refused_cases==11
    ack=None
    if ack_capture or ack_exports:
        assert ack_capture and ack_exports;records=physical(ack_capture);events=list(rows(ack_exports/'events.jsonl'));assert len(records)==len(events)==1
        ack=ACK_header(records[0],events[0],json.loads((ack_exports/'flows.json').read_text()),list(rows(ack_exports/'yak.jsonl')))
        reference=json.loads((ack_exports/'oracle.json').read_text('utf8'));assert len(reference)==1;layers=reference[0]['_source']['layers'];claim=profile(records[0]);ack['reference']=reference_frame(layers,records[0],claim);raw_scalar(layers,'gre.ack_number',claim['offset']+8,4,records[0]['wire'],7);assert'ppp'not in layers['frame']['frame.protocols'].split(':')
        ack['reference_SHA256']=sha(ack_exports/'oracle.json')
        ack.update(capture_SHA256=sha(ack_capture),SDK_SHA256=sha(ack_exports/'events.jsonl'),CLI_SHA256=sha(ack_exports/'yak.jsonl'),flow_export_SHA256=sha(ack_exports/'flows.json'))
    return dict(scope=__doc__,auditor_SHA256=sha(Path(__file__)),manifest_SHA256=sha(manifest),authored_primary_cases=15,authored_primary_frames=19,precise_parent_refusal_cases=refused_cases,literal_reference_still_decodes_DNS_for_refused_parents=reference_permissive,controls=checked,additional_ACK_only_control=ack,source_observation_not_full_PPTP_call_compliance=True)


def direct_v1_records(capture):
    selected={}
    for n,record in enumerate(physical(capture),1):
        w=record['wire']
        if record['link']!=1 or len(w)<34 or w[12:14]!=b'\x08\0':continue
        try:outer=ipv4(w,14,len(w))
        except AssertionError:continue
        at=outer['payload']
        if outer['protocol']==47 and not outer['fragmented']and at+2<=outer['end']and w[at+1]&7==1:selected[n]=record
    return selected


def audit_public(capture,events,cli,inventory,baseline,oracle,columns):
    targets=direct_v1_records(capture);assert len(targets)==271
    declared=json.loads(inventory.read_text('utf8'));assert len(declared)==len(targets)and{v['frame']for v in declared}==set(targets)
    for entry in declared:
        record=targets[entry['frame']];claim=profile(record);assert claim['kind']=='refused'
        assert record['section']==entry['section']and record['interface']==entry['interface']and record['link']==entry['link']and digest(record['wire'])==entry['wire_SHA256']and len(record['wire'])==entry['captured_bytes']
        assert claim['offset']==entry['GRE_offset']and f"{claim['flags']:04x}"==entry['flags']and hex(claim['protocol'])==entry['protocol_type']
    native={};total_PPTP=0
    for e in rows(events):
        if e.get('expert')!='PPTPHeaderAdmission':continue
        total_PPTP+=1;refs=e['source_bytes'].get('PacketRefs')or[]
        if len(refs)==1 and refs[0]['Number']in targets:
            n=refs[0]['Number'];assert type(n)is int and n not in native;native[n]=e
    assert set(native)==set(targets),'every literal public v1 refusal needs a precise native observation'
    reference={number(v['_source']['layers']['frame']['frame.number']):v['_source']['layers']for v in json.loads(oracle.read_text('utf8'))};assert set(reference)==set(targets)
    literal={}
    with columns.open(encoding='utf8')as f:
        assert f.readline().rstrip('\r\n')=='frame.number\t_ws.col.protocol'
        for line in f:
            n,label=line.rstrip('\r\n').split('\t',1);n=int(n);assert n not in literal;literal[n]=label
    assert set(literal)==set(targets)
    checks=[];labels=collections.Counter();reasons=collections.Counter()
    for n,record in targets.items():
        claim=profile(record);event=native[n];refusal(event,n,record,claim);bound=reference_frame(reference[n],record,claim);labels[literal[n]]+=1;reasons[claim['reason']]+=1
        checks.append(dict(frame=n,PDU=event['id'],physical_domain=record_domain(record),source_wire_SHA256=digest(record['wire']),source_time_ns=record['time_ns'],literal_GRE_flags=claim['flags'],literal_GRE_protocol=claim['protocol'],reason=claim['reason'],literal_reference_protocol=literal[n],reference=bound))
    exclusive(rows(events),native);seen=collections.Counter()
    for row in rows(cli):
        if row.get('record_type')=='protocol_publication'or row['number']not in targets:continue
        refusal_row(row,native[row['number']],row['number'],targets[row['number']]);seen[row['number']]+=1
    assert seen==collections.Counter({n:1 for n in targets})
    migrations=[];old_seen=set()
    for e in rows(baseline):
        refs=e['source_bytes'].get('PacketRefs')or[];selected=[r for r in refs if r['Number']in targets]
        if not selected:continue
        assert len(refs)==len(selected)==1;n=selected[0]['Number'];assert n not in old_seen;old_seen.add(n)
        assert e['protocol']=='ip'and e['status']=='unrecognized'and e['expert']=='UnsupportedOrMalformedNetworkLayer'and not e['raw']and not e['fields']and e['flow']==e.get('tcp_flow_id',0)==0,'a recognized child cannot disappear as an unreviewed domain normalization'
        record=targets[n];claim=profile(record)
        try:legacy=gre(record['wire'],claim['offset'],claim['outer']['end']);old_domain='/gre:'+claim['outer']['peers']+':'+str(legacy['key'])
        except AssertionError:old_domain=''
        same(e['domain'],record_domain(record,old_domain));same(refs,[dict(Number=n,Domain=e['domain'])]);assert e['source_bytes']['Kind']=='captured'
        migrations.append(dict(frame=n,source_wire_SHA256=digest(record['wire']),old_PDU=e['id'],new_PDU=native[n]['id'],old_domain=e['domain'],new_domain=native[n]['domain'],same_physical_record_section_interface=True,old_event_canonical_SHA256=digest(canonical(e).encode()),old_class='unrecognized IP observer',new_class='precise unselected PPTP parent'))
    assert len(migrations)==171 and len(targets)-len(old_seen)==100
    return dict(scope=__doc__,auditor_SHA256=sha(Path(__file__)),capture_SHA256=sha(capture),SDK_SHA256=sha(events),CLI_SHA256=sha(cli),inventory_SHA256=sha(inventory),baseline_SDK_SHA256=sha(baseline),reference_SHA256=sha(oracle),reference_columns_SHA256=sha(columns),direct_original_v1_frames=271,direct_source_bound_precise_refusals=271,total_native_PPTP_header_diagnostics=total_PPTP,source_contributors_exclusive=True,old_generic_observations_migrated=171,previously_without_native_observation=100,all_physical_source_records_retained=True,literal_reference_labels=dict(labels),refusal_reasons=dict(reasons),source_bound_refusals=checks,explicit_domain_and_diagnostic_migrations=migrations,full_PPTP_protocol_or_reference_child_label_parity_claim=False)


if __name__=='__main__':
    ap=argparse.ArgumentParser(description=__doc__);sub=ap.add_subparsers(dest='mode',required=True)
    controls=sub.add_parser('controls')
    for n in['manifest','exports','baseline','out']:controls.add_argument('--'+n,type=Path,required=True)
    controls.add_argument('--ack-capture',type=Path);controls.add_argument('--ack-exports',type=Path)
    public=sub.add_parser('public')
    for n in['capture','events','cli','inventory','baseline','oracle','columns','out']:public.add_argument('--'+n,type=Path,required=True)
    a=ap.parse_args()
    for value in vars(a).values():
        if isinstance(value,Path):assert not any((v/'.git').exists()for v in[value.resolve(),*value.resolve().parents])
    proof=audit_controls(a.manifest,a.exports,a.baseline,a.ack_capture,a.ack_exports)if a.mode=='controls'else audit_public(a.capture,a.events,a.cli,a.inventory,a.baseline,a.oracle,a.columns)
    with a.out.open('x',encoding='utf8')as f:json.dump(proof,f,indent=2);f.write('\n')
    print(json.dumps({k:v for k,v in proof.items()if k not in['scope','controls','source_bound_refusals','explicit_domain_and_diagnostic_migrations']}),flush=True)
