"""Bind VXLAN headers, nested VNI namespaces and native ownership to wire bytes.

This is a bounded Ethernet/VLAN/IPv4/IPv6/UDP/VXLAN source audit, not a VTEP,
group-policy evaluator, GPE payload dissector or independent TCP receive engine.
RFC7348 receive-ignored bits are observations, not rejection conditions. GBP
A/D are meaningful only with G; GPE uses a separate destination service and
only a coherent v0 header, without admitting its inner payload profile.
"""
import argparse,base64,collections,copy,hashlib,ipaddress,json
from pathlib import Path
from capture_clock_wire_audit import physical
from pptp_parent_source_audit import instant


def canonical(v):return json.dumps(v,sort_keys=True,separators=(',',':'))
def digest(v):return hashlib.sha256(canonical(v).encode()).hexdigest()
def sha(p):
    h=hashlib.sha256()
    with p.open('rb')as f:
        for b in iter(lambda:f.read(1<<20),b''):h.update(b)
    return h.hexdigest()
def rows(p):
    with p.open(encoding='utf-8-sig')as f:
        for line in f:yield json.loads(line)
def domain(record,path):return dict(Section=record['section'],Interface=record['interface'],Encapsulation=path)
def integer(v):
    assert type(v)is int
    return v


def scan(record):
    w=record['wire'];observed=[];quotes=[];leaf={};addresses=None
    def end_at(at,count,end):assert 0<=at and at+count<=end<=len(w),'required original header bytes unavailable'
    def ethernet(at,end,path,depth,quoted=False):
        end_at(at,14,end);kind=int.from_bytes(w[at+12:at+14],'big');start=at;at+=14
        while kind in[0x8100,0x88a8,0x9100]:
            end_at(at,4,end);path+='/vlan:'+str(int.from_bytes(w[at:at+2],'big')&4095);kind=int.from_bytes(w[at+2:at+4],'big');at+=4
        if kind in[0x800,0x86dd]:return network(at,end,path,depth,kind,quoted)
        if kind==0x806:
            end_at(at,8,end);size=8+2*w[at+4]+2*w[at+5];end_at(at,size,end)
            leaf.update(kind='arp',payload=w[at:at+size],domain=path,offset=at,ethernet_offset=start)
        else:leaf.update(kind='unselected-EtherType',domain=path,EtherType=kind)
    def network(at,end,path,depth,kind,quoted=False):
        nonlocal addresses
        assert depth<=16,'bounded independent source walk'
        if kind==0x800:
            end_at(at,20,end);assert w[at]>>4==4;size=(w[at]&15)*4;decl=int.from_bytes(w[at+2:at+4],'big');assert size>=20 and decl>=size;end_at(at,size,end)
            assert quoted or at+decl<=end,'source capture lacks declared IP bytes';end=min(end,at+decl);protocol=w[at+9];source=str(ipaddress.ip_address(w[at+12:at+16]));destination=str(ipaddress.ip_address(w[at+16:at+20]));fragment=int.from_bytes(w[at+6:at+8],'big')&0x3fff;payload=at+size
        else:
            end_at(at,40,end);assert w[at]>>4==6;decl=int.from_bytes(w[at+4:at+6],'big');assert decl>0 and(quoted or at+40+decl<=end);end=min(end,at+40+decl);source=str(ipaddress.ip_address(w[at+8:at+24]));destination=str(ipaddress.ip_address(w[at+24:at+40]));protocol=w[at+6];payload=at+40;fragment=False
            for _ in range(16):
                if protocol==44:fragment=True;break
                if protocol not in[0,43,60]:break
                end_at(payload,2,end);size=(w[payload+1]+1)*8;end_at(payload,size,end);protocol=w[payload];payload+=size
            else:raise AssertionError('extension budget')
        addresses=(source,destination)
        if fragment:leaf.update(kind='fragment',domain=path);return
        if protocol in[4,41]:
            path+=('/ipip:'if protocol==4 else'/6in4:')+'~'.join(sorted([source,destination]));return network(payload,end,path,depth+1,0x800 if protocol==4 else 0x86dd,quoted)
        if protocol==17:
            end_at(payload,8,end);src,dst,decl=(int.from_bytes(w[payload+i:payload+i+2],'big')for i in[0,2,4]);assert decl>=8 and(quoted or payload+decl<=end),'complete UDP envelope required before interpreting any inner header';body=payload+8;udp_end=min(end,payload+decl);data=w[body:udp_end]
            if dst==4789:
                h=data[:8];vni=int.from_bytes(h[4:7],'big')if len(h)>=7 else None;entry=dict(offset=body,header=h.hex(),available=len(data),source=source,destination=destination,source_port=src,destination_port=dst,parent_path=path,VNI=vni,I=bool(h and h[0]&8),selected=not quoted and len(h)==8 and bool(h[0]&8),source_scope='ICMP quote; no tunnel state'if quoted else'actual packet carrier',reserved_receive_rule='ignored',GBP_G=bool(h and h[0]&128));(quotes if quoted else observed).append(entry)
                if len(h)==8 and h[0]&128:entry.update(GBP_A=bool(h[1]&8),GBP_D=bool(h[1]&64),GBP_ID=int.from_bytes(h[2:4],'big'),group_policy_evaluated=False)
                if vni is not None:path+='/vxlan:'+'~'.join(sorted([source,destination]))+':'+str(vni)
                entry['observed_path']=path
                if len(h)!=8 or not entry['I']or len(data)<22:leaf.update(kind='refused-VXLAN',domain=path,header=entry);return
                return ethernet(body+8,udp_end,path,depth+1,quoted)
            if dst==4790 and len(data)>=8 and data[0]&8 and not data[0]&0x30 and(bool(data[0]&4)==bool(data[3])):
                vni=int.from_bytes(data[4:7],'big');path+='/vxlan-gpe:'+'~'.join(sorted([source,destination]))+':'+str(vni);leaf.update(kind='unselected-GPE-payload',domain=path,VNI=vni,next_protocol=data[3],offset=body,header=data[:8].hex());return
            leaf.update(kind='udp',domain=path,source=source,destination=destination,source_port=src,destination_port=dst,payload=data,offset=body)
        elif protocol==6:
            end_at(payload,20,end);size=(w[payload+12]>>4)*4;assert size>=20;end_at(payload,size,end);src=int.from_bytes(w[payload:payload+2],'big');dst=int.from_bytes(w[payload+2:payload+4],'big');leaf.update(kind='tcp',domain=path,source=source,destination=destination,source_port=src,destination_port=dst,sequence=int.from_bytes(w[payload+4:payload+8],'big'),flags=w[payload+13],payload=w[payload+size:end],offset=payload+size)
        elif protocol in[1,58]:
            leaf.update(kind='icmp'if protocol==1 else'icmpv6',domain=path,payload=w[payload:end],offset=payload,network_offset=at,network_end=end)
            if payload+8<end and(protocol==1 and w[payload]in[3,4,5,11,12]or protocol==58 and w[payload]in[1,2,3,4]):
                actual=copy.deepcopy(leaf)
                try:
                    version=w[payload+8]>>4
                    if version in[4,6]:network(payload+8,end,path,depth+1,0x800 if version==4 else 0x86dd,True)
                except AssertionError:pass # quote bytes need not include the original packet's end
                leaf.clear();leaf.update(actual)
        else:leaf.update(kind='unselected-IP-protocol',domain=path,IP_protocol=protocol)
    link=record['link']
    if link==1:ethernet(0,len(w),'',0)
    elif link in[101,228,229]:
        assert w;network(0,len(w),'',0,0x800 if w[0]>>4==4 else 0x86dd)
    elif link in[0,108]:
        assert len(w)>=4;family=int.from_bytes(w[:4],'big'if w[:2]==b'\0\0'else'little');assert family in[2,10,24,28,30];network(4,len(w),'',0,0x800 if family==2 else 0x86dd)
    else:raise AssertionError(('unselected source link',link))
    return dict(headers=observed,quoted_headers=quotes,leaf=leaf)


def objects(value):
    if isinstance(value,dict):
        yield value
        for child in value.values():yield from objects(child)
    elif isinstance(value,list):
        for child in value:yield from objects(child)
def values(value,name):
    out=[]
    for obj in objects(value):
        if name in obj:
            found=obj[name];out.extend(found if isinstance(found,list)and(found and isinstance(found[0],list))else[found])
    return out
def scalar(value):
    if isinstance(value,list):assert len(value)==1;value=value[0]
    return int(str(value),16 if str(value).startswith('0x')else 10)
def raw_field(value,name,offset,length,wire,expected):
    matches=[r for r in values(value,name+'_raw')if isinstance(r,list)and len(r)==6 and r[1]==offset]
    assert len(matches)==1;raw=matches[0];assert all(type(v)is int for v in raw[1:])and raw[1:4]==[offset,length,0]and raw[5]==0
    assert bytes.fromhex(raw[0])==wire[offset:offset+length]and int.from_bytes(wire[offset:offset+length],'big')==expected
    matching=[obj for obj in objects(value)if name+'_raw'in obj and obj[name+'_raw']==raw];assert len(matching)==1 and scalar(matching[0][name])==expected
    return matching[0]


def reference_header(layers,header,wire):
    at=header['offset'];value=int.from_bytes(wire[at:at+2],'big');owner=raw_field(layers,'vxlan.flags',at,2,wire,value);raw_field(layers,'vxlan.vni',at+4,3,wire,header['VNI'])
    for name,mask in [('vxlan.flag_g',0x8000),('vxlan.flag_i',0x0800),*([('vxlan.flag_d',0x40),('vxlan.flag_a',8)]if value&0x8000 else[])]:
        field=[v for v in objects(owner)if name+'_raw'in v];assert len(field)==1;field=field[0];descriptor=field[name+'_raw'];expected=int(bool(value&mask))
        assert len(descriptor)==6 and descriptor[1:]==[at,2,mask,2,0]and int(descriptor[0],16)==expected and scalar(field[name])==expected,'masked flags remain bound to their own original header and source zero'
    if value&0x8000:raw_field(layers,'vxlan.gbp',at+2,2,wire,int.from_bytes(wire[at+2:at+4],'big'))


def endpoint(address,port):return('['+address+']'if':'in address else address)+':'+str(port)
def native_ownership(capture_records,scans,events,flows):
    assert not flows['error'];known={integer(f['flow']):f for f in flows.get('flows')or[]};bindings={};packets=0
    for packet in flows.get('packets')or[]:
        ident=integer(packet['flow']);direction=integer(packet['direction']);assert ident in known and direction in[0,1];flow=known[ident];refs=packet['refs'];assert len(refs)==1,'this audit does not claim independent IP fragment reconstruction'
        ref=refs[0];n=integer(ref['Number']);record=capture_records[n];leaf=scans[n]['leaf'];assert leaf['kind']=='tcp'and canonical(ref['Domain'])==canonical(domain(record,leaf['domain']))==canonical(flow['domain']);assert instant(packet['captured'])==record['time_ns']
        peers=[endpoint(leaf['source'],leaf['source_port']),endpoint(leaf['destination'],leaf['destination_port'])];assert peers==([flow['source'],flow['destination']]if direction==0 else[flow['destination'],flow['source']]);binding=(ident,direction);assert bindings.setdefault(n,binding)==binding;packets+=1
    counts=collections.Counter();source_checks=[]
    for event in events:
        refs=event['source_bytes'].get('PacketRefs')or[];raw=base64.b64decode(event['raw']or'',validate=True);counts[event['protocol']]+=1
        assert refs or not raw and event['status']!='decoded','an exact message requires original byte contributors'
        leaves=[]
        for ref in refs:
            n=integer(ref['Number']);record=capture_records[n];leaf=scans[n]['leaf'];assert canonical(ref['Domain'])==canonical(event['domain'])==canonical(domain(record,leaf['domain']));leaves.append(leaf)
            if event.get('tcp_flow_id'):assert bindings[n][0]==event['tcp_flow_id']and bindings[n][1]==event['direction']
        if leaves:
            source=event['source_bytes'];assert source['Kind']==('reassembled'if leaves[0]['kind']=='tcp'else'captured')and source['ParentPDU']==0 and source['ParentPDUs']is None
            assert canonical(event['field_values']['source'])==canonical(source)and event['field_values']['pdu']==event['id']
            if source.get('CompletedIn')is not None:assert canonical(source['CompletedIn'])==canonical(max(refs,key=lambda r:r['Number']))
        if event['protocol']in['arp','icmp','icmpv6']:
            assert len(leaves)==1 and leaves[0]['kind']==event['protocol'],'a quoted child cannot become a separate native application/link observation'
        if event['protocol']=='dns':assert leaves and all(l['kind']=='udp'for l in leaves)
        if not raw:continue
        assert event['source_bytes']['SHA256']==hashlib.sha256(raw).hexdigest()
        if leaves and leaves[0]['kind']=='tcp':
            assert all(l['kind']=='tcp'for l in leaves);data={}
            for leaf in leaves:
                for offset,byte in enumerate(leaf['payload']):
                    position=leaf['sequence']+bool(leaf['flags']&2)+offset;assert data.setdefault(position,byte)==byte
            ordered=sorted(data);spans=[];current=bytearray();previous=None
            for offset in ordered:
                if previous is not None and offset!=previous+1:spans.append(bytes(current));current.clear()
                current.append(data[offset]);previous=offset
            if current:spans.append(bytes(current))
            assert any(raw in span for span in spans),'message bytes must belong to an actual same-direction TCP source span'
        elif leaves:
            assert len(leaves)==1;record=capture_records[refs[0]['Number']];leaf=leaves[0]
            scopes=[leaf.get('payload',b'')]
            if event['protocol']in['arp','icmp','icmpv6']:
                scopes.append(record['wire'])
                if'network_offset'in leaf:scopes.append(record['wire'][leaf['network_offset']:leaf['network_end']])
            assert any(raw in scope for scope in scopes),'datagram/link-message bytes must belong to their exact physical source'
        source_checks.append(dict(PDU=event['id'],protocol=event['protocol'],source_frames=[r['Number']for r in refs],raw_SHA256=hashlib.sha256(raw).hexdigest(),native_TCP_flow=event.get('tcp_flow_id',0)))
    return dict(native_flows=len(known),source_bound_TCP_packet_callbacks=packets,source_bound_PDUs=len(source_checks),protocol_counts=dict(counts),messages=source_checks,independent_TCP_state_algorithm_claim=False)


def header_source(record,event,n,path):
    d=domain(record,path);expected=dict(Kind='captured',SHA256='',ParentPDU=0,ParentPDUs=None,PacketRefs=[dict(Number=n,Domain=d)])
    assert event['protocol']=='ip'and event['transport']=='network'and event['profile']=='network-encapsulation-refusal'and event['status']=='limited'
    assert all(type(event.get(k,0))is int and event.get(k,0)==0 for k in['flow','tcp_flow_id','transaction','response_to'])and not event['raw']and not event['source']and not event['destination']and not event['fields']
    assert canonical(event['domain'])==canonical(d)and canonical(event['source_bytes'])==canonical(expected)and canonical(event['field_values']['source'])==canonical(expected)
    assert event['field_values']['pdu']==event['id']and event['fields_error']=='protocol parser: event is limited, not an exact message'
    return d


def header_refusal(record,event,n,parsed=None):
    parsed=parsed or scan(record);leaf=parsed['leaf'];assert leaf['kind']=='refused-VXLAN';h=leaf['header'];domain_value=header_source(record,event,n,leaf['domain']);raw=bytes.fromhex(h['header']);names=['Ethernet'if record['link']==1 else'Loopback','IPv4']
    fields={'Observed Header Layers':names,'Header Observation Scope':'outer network prefix only','Child Header Identity':'unconfirmed','Encapsulation Admission':'unselected profile','Unselected Carrier':'VXLAN','Available VXLAN Bytes':h['available'],'Required VXLAN Header Bytes':8,'Reserved Receive Rule':'ignored per RFC 7348'}
    if len(raw)<8:
        reason=f"incomplete fixed header: {len(raw)} of 8 bytes available";fields['VXLAN Header Truncated']=True
    elif not h['I']:reason='I bit is clear; VNI is not valid'
    else:
        available=h['available']-8;assert 0<=available<14;reason=f'Ethernet child prefix incomplete: {available} of 14 bytes available';fields.update({'Header Observation Scope':'validated VXLAN fixed header; incomplete Ethernet child prefix','Available Ethernet Header Bytes':available,'Required Ethernet Header Bytes':14})
    fields['Parent Admission Reason']=reason
    if raw:fields.update({'Observed VXLAN Flags':raw[0],'VXLAN I Bit':bool(raw[0]&8)})
    if len(raw)>=7:fields['Observed VXLAN VNI']=h['VNI']
    assert event['expert']=='VXLANHeaderAdmission'and event['completeness']=='limited'and event['error']=='VXLAN header: '+reason and event['summary']==event['error']+'; inner identity unconfirmed'
    assert canonical(event['session'])==canonical(fields),'every refusal field must come from the actual bounded parent bytes'
    return dict(frame=n,source_wire_SHA256=hashlib.sha256(record['wire']).hexdigest(),source_domain=domain_value,reason=reason,one_precise_refusal_no_child=True)


def gpe_header(record,event,n,parsed=None):
    parsed=parsed or scan(record);leaf=parsed['leaf'];assert leaf['kind']=='unselected-GPE-payload';h=bytes.fromhex(leaf['header']);header_source(record,event,n,leaf['domain'])
    expected={'Observed Header Layers':['Ethernet'if record['link']==1 else'Loopback','IPv4'],'Header Observation Scope':'validated VXLAN-GPE v0 header only','Child Header Identity':'unconfirmed','Encapsulation Admission':'unselected payload profile','Unselected Carrier':'VXLAN-GPE','VXLAN-GPE Version':0,'VXLAN-GPE Flags':h[0],'VXLAN-GPE VNI':leaf['VNI'],'VXLAN-GPE Next Protocol Present':bool(h[0]&4),'VXLAN-GPE Next Protocol':h[3],'VXLAN-GPE OAM':bool(h[0]&1),'VXLAN-GPE Replicated BUM':bool(h[0]&2),'Reserved Receive Rule':'ignored','Payload Profile Implemented':False}
    assert event['expert']=='VXLANProfileUnselected'and event['completeness']=='headers'and not event.get('error')and canonical(event['session'])==canonical(expected)
    assert event['summary']==f"VXLAN-GPE v0 header VNI={leaf['VNI']} next={h[3]}; payload profile not selected; inner identity unconfirmed"
    return dict(frame=n,source_wire_SHA256=hashlib.sha256(record['wire']).hexdigest(),source_domain=event['domain'],literal_VNI=leaf['VNI'],literal_next_protocol=h[3],validated_header_only=True,no_payload_or_session_claim=True)


def controls(capture,old_events,new_events,old_flows,new_flows,new_cli):
    records=physical(capture);prior=list(rows(old_events));current=list(rows(new_events));cli=list(rows(new_cli));by_frame=collections.defaultdict(list);old_by_frame={}
    for e in prior:
        refs=e['source_bytes'].get('PacketRefs')or[];assert len(refs)==1 and refs[0]['Number']not in old_by_frame;old_by_frame[refs[0]['Number']]=e
    for e in current:
        refs=e['source_bytes'].get('PacketRefs')or[];assert len(refs)==1;by_frame[refs[0]['Number']].append(e)
    assert set(by_frame)==set(range(1,len(records)+1))and all(len(v)==1 for v in by_frame.values()),'every controlled source has exactly one native observation and no child contribution'
    counts=collections.Counter();proof=[]
    for n,record in enumerate(records,1):
        parsed=scan(record);leaf=parsed['leaf'];event=by_frame[n][0];row=[r for r in cli if r.get('record_type')!='protocol_publication'and r.get('number')==n];assert len(row)==1;row=row[0]
        assert instant(row['time'])==record['time_ns']and row['captured_length']==row['length']==len(record['wire'])and row['capture_domain']['Section']==record['section']and row['capture_domain']['Interface']==record['interface']
        ps=row.get('pdus')or[];assert len(ps)==1 and ps[0]['id']==event['id']and canonical(ps[0]['byte_source'])==canonical(event['source_bytes'])
        assert not row.get('tcp_flow_id')and not row.get('stream_id')and not row.get('connection_protocol')
        if leaf['kind']=='refused-VXLAN':
            p=header_refusal(record,event,n,parsed);counts['precise_refusal']+=1;assert row['protocol']=='IPv4'and row['protocol_status']=='limited'and row['info']==event['summary']and row['protocol_stack'][-1]=='IPv4';proof.append(p)
        elif leaf['kind']=='unselected-GPE-payload':
            p=gpe_header(record,event,n,parsed);counts['validated_GPE_header_only']+=1;assert row['protocol']=='VXLAN-GPE'and row['protocol_status']=='limited'and row['visibility']=='tunnel-header'and row['protocol_scope']=='VXLAN-GPE v0 header; inner payload profile unselected'and row['info']==event['summary'];assert row['protocol_stack'][-1]=='VXLAN-GPE'and row['pdus'][0]['status']=='limited'
            assert canonical(row.get('message_domain'))==canonical(event['domain']),'the observed header namespace must remain visible as the literal native source domain'
            assert canonical(row['capture_domain'])==canonical(domain(record,'')),'these controls preserve the outer physical capture domain separately from the header observation'
            p['CLI_message_domain_matches_native_header_source']=True;p['physical_capture_domain_preserved_separately']=True;proof.append(p)
        elif parsed['headers']:
            assert event['protocol']=='dns'and event['status']=='decoded'and row['protocol']=='DNS';counts['standard_DNS']+=1
            assert canonical(event)==canonical(old_by_frame[n]),'legal standard/receive-ignored/GBP data keeps its complete native message fields, types, bytes and connection identity'
            assert row['protocol_stack']==['Ethernet'if record['link']==1 else'Loopback','IPv4','UDP','VXLAN','Ethernet','IPv4','UDP','DNS']
            proof.append(dict(frame=n,source_wire_SHA256=hashlib.sha256(record['wire']).hexdigest(),selected_standard_profile=True,header=parsed['headers'][0],reserved_receive_rule='ignored',GBP_policy_not_evaluated=True))
        else:
            assert leaf['kind']=='udp'and event['protocol']==''and event['transport']=='udp'and row['protocol']=='UDP';counts['opaque_UDP_service_hint_only']+=1;assert not event['domain']['Encapsulation']and'VXLAN'not in row.get('protocol_stack',[])
            proof.append(dict(frame=n,source_wire_SHA256=hashlib.sha256(record['wire']).hexdigest(),service_hint_does_not_confirm_tunnel=True))
            if leaf['source_port']!=4789:assert canonical(event)==canonical(old_by_frame[n]),'an incoherent/unselected GPE service hint cannot change an existing opaque datagram observation'
    old=json.loads(old_flows.read_text('utf8'));new=json.loads(new_flows.read_text('utf8'));expected=copy.deepcopy(old);expected['stats']['DecodeErrors']=counts['precise_refusal'];assert old['stats']['DecodeErrors']==0
    assert canonical(new)==canonical(expected),'only source-proved typed header refusals change capture DecodeErrors; all other flow/statistics values remain literal'
    assert not new['error']and not(new.get('flows')or new.get('packets')or new.get('closed'))
    return dict(scope='Source-bound authored/live UDP headers; only standard Ethernet data selected, header-only GPE, no unselected child contribution',auditor_SHA256=sha(Path(__file__)),capture_SHA256=sha(capture),old_SDK_SHA256=sha(old_events),SDK_SHA256=sha(new_events),CLI_SHA256=sha(new_cli),old_flows_SHA256=sha(old_flows),flows_SHA256=sha(new_flows),physical_packets=len(records),old_SDK_records=len(prior),new_SDK_records=len(current),counts=dict(counts),DecodeErrors=new['stats']['DecodeErrors'],all_other_capture_flow_values_literal=True,observations=proof)


def candidate_wire(record):
    wire=record['wire'];assert record['link']==1 and wire[12:14]==b'\x08\0'and wire[14]>>4==4 and wire[23]==17
    ihl=(wire[14]&15)*4;at=14+ihl;end=14+int.from_bytes(wire[16:18],'big');assert ihl>=20 and at+8<=end<=len(wire)
    src,dst,count=(int.from_bytes(wire[at+i:at+i+2],'big')for i in[0,2,4]);body=wire[at+8:end];source=str(ipaddress.ip_address(wire[26:30]));destination=str(ipaddress.ip_address(wire[30:34]));vni=int.from_bytes(body[4:7],'big')if len(body)>=7 else None
    return dict(source=source,destination=destination,source_port=src,destination_port=dst,UDP_length=count,IP_UDP_extent=end-at,IP_end=end,padding_bytes=len(wire)-end,payload=body,payload_offset=at+8,VNI=vni,I=bool(body and body[0]&8),legacy_domain='/vxlan:'+'~'.join(sorted([source,destination]))+':'+str(vni))


def corrected_opaque_candidate(first,last,record,n):
    facts=candidate_wire(record);src,dst,count=facts['source_port'],facts['destination_port'],facts['UDP_length'];body=facts['payload']
    assert src==4789 and dst==55810 and count==facts['IP_UDP_extent']==35 and len(body)==27 and not facts['I']and facts['padding_bytes']==14
    old_summary=f'UDP carrier incomplete: declared {count} bytes, available {count}; capture {len(record["wire"])}/{len(record["wire"])} bytes'
    assert first['summary']==old_summary and first['error']=='NeedMore: '+old_summary
    expected=copy.deepcopy(first);expected.update(status='unrecognized',completeness='unrecognized',expert='ProtocolNotRecognized',domain=domain(record,''),summary=f'UDP application protocol not recognized ({first["source"]} -> {first["destination"]}, {len(body)} bytes)',fields_error='protocol parser: event is unrecognized, not an exact message');expected.pop('error')
    assert expected['source_bytes']['PacketRefs']==[dict(Number=n,Domain=domain(record,facts['legacy_domain']))]
    expected['source_bytes']['PacketRefs'][0]['Domain']=domain(record,'');expected['field_values']['source']=copy.deepcopy(expected['source_bytes']);expected['field_values'].pop('incomplete');expected['field_values']['completeness']='unrecognized'
    statuses=[v for v in expected['field_values']['values']if v['name']=='status'];assert len(statuses)==1 and statuses[0]['value']=='incomplete';statuses[0]['value']='unrecognized'
    assert canonical(expected)==canonical(last),'every corrected native value/type must equal the literal-source-derived complete expected event'
    assert first['domain']==domain(record,facts['legacy_domain'])and last['domain']==domain(record,'')
    assert base64.b64decode(first['raw'])==base64.b64decode(last['raw'])==body and first['source_bytes']['SHA256']==last['source_bytes']['SHA256']==hashlib.sha256(body).hexdigest()
    return facts


def public_candidates(capture,inventory,before,events,before_flows,flows):
    declared=json.loads(inventory.read_text('utf8'));wanted={v['frame']for v in declared['observations']};assert len(wanted)==35
    original={n:r for n,r in enumerate(physical(capture),1)if n in wanted};assert set(original)==wanted and declared['capture_SHA256']==sha(capture)
    for entry in declared['observations']:
        record=original[entry['frame']];assert record['section']==entry['section']and record['interface']==entry['interface']and record['time_ns']==entry['time_ns']and hashlib.sha256(record['wire']).hexdigest()==entry['wire_SHA256']
    paired=[];sides=[]
    for path in[before,events]:
        selected={};identifiers=set()
        for event in rows(path):
            refs=event['source_bytes'].get('PacketRefs')or[];found=[r['Number']for r in refs if r['Number']in wanted]
            if found:
                assert len(found)==len(refs)==1 and found[0]not in selected
                ident=integer(event['id']);assert ident>0 and ident not in identifiers;identifiers.add(ident);selected[found[0]]=event
        assert set(selected)==wanted;sides.append(selected)
    def exact_own_identity(event):
        ident=integer(event['id']);assert integer(event['field_values']['pdu'])==ident
        found=[]
        for field in event['field_values'].get('values')or[]:
            if field['name']=='pdu.id':
                assert integer(field['value'])==ident and type(field.get('exact_value'))is str and field['exact_value']==str(ident);found.append(field)
        assert len(found)==1
    for n in sorted(wanted):
        first,last=sides[0][n],sides[1][n];record=original[n]
        for event in[first,last]:
            exact_own_identity(event);assert event['flow']==event.get('tcp_flow_id',0)==0 and event['protocol']==''and not event['fields']
        assert first['id']==last['id'],'this candidate-only correction publishes no new PDU and requires literal generated IDs too'
        changed=sorted(k for k in set(first)|set(last)if(k in first)!=(k in last)or canonical(first.get(k))!=canonical(last.get(k)))
        if n!=48644:assert not changed,('unrelated original-source candidate changed',n,changed)
        else:
            corrected_opaque_candidate(first,last,record,n)
            assert last['source']=='1.2.3.4:4789'and last['destination']=='4.3.2.1:55810'and last['transport']=='udp'
        paired.append(dict(frame=n,wire_SHA256=hashlib.sha256(record['wire']).hexdigest(),source_section=record['section'],source_interface=record['interface'],source_time_ns=record['time_ns'],before_PDU=first['id'],after_PDU=last['id'],before_event_SHA256=digest(first),after_event_SHA256=digest(last),changed_non_generated_root_fields=changed,before_domain=first['domain'],after_domain=last['domain']))
    assert sha(before_flows)==sha(flows),'all native TCP identity, generation, capture counters, bytes and ownership remain literal'
    state=json.loads(flows.read_text('utf8'));assert not state['error']and not any(ref['Number']in wanted for packet in state.get('packets')or[]for ref in packet['refs'])
    return dict(scope='Original full Sharon source ordinals and interface/section identity; candidate35 only, no ID normalization or all-protocol field parity claim',auditor_SHA256=sha(Path(__file__)),capture_SHA256=sha(capture),inventory_SHA256=sha(inventory),before_SDK_SHA256=sha(before),SDK_SHA256=sha(events),original_selected_frames=35,literal_complete_observations=34,actual_own_PDU_alias_changes=0,own_PDU_snapshot_and_typed_ID_values_and_types_literal=True,source_proved_opaque_service_correction_frame=48644,recognized_application_fields_removed=0,before_flows_SHA256=sha(before_flows),flows_SHA256=sha(flows),complete_full_flow_export_byte_identical=True,full_native_TCP_flows=len(state.get('flows')or[]),full_owned_TCP_packets=len(state.get('packets')or[]),candidate_frames_with_TCP_ownership=0,observations=paired)


def native_cli_pdu(event):
    out={k:copy.deepcopy(event[k])for k in['id','protocol','status','summary','completeness','expert','response_to']};out['byte_source']=copy.deepcopy(event['source_bytes']);out['field_values']=copy.deepcopy(event['field_values']);fields={}
    for field in event['field_values']['values']:fields.setdefault(field['name'],[]).append(copy.deepcopy(field['value']))
    out['fields']=fields
    if event.get('error')or event.get('expert'):out['error']=event.get('error','')
    return out


def exact_patch(first,last,path=''):
    if type(first)is dict and type(last)is dict:
        out=[]
        for key in sorted(set(first)|set(last)):
            child=path+'/'+key.replace('~','~0').replace('/','~1')
            if key not in first:out.append(dict(op='add',path=child,value=copy.deepcopy(last[key])))
            elif key not in last:out.append(dict(op='remove',path=child,old_value=copy.deepcopy(first[key])))
            else:out.extend(exact_patch(first[key],last[key],child))
        return out
    if canonical(first)==canonical(last):return[]
    return[dict(op='replace',path=path,old_value=copy.deepcopy(first),value=copy.deepcopy(last))]


def public_candidate_cli(capture,inventory,before_events,events,before_cli,cli):
    declared=json.loads(inventory.read_text('utf8'));wanted={v['frame']for v in declared['observations']};assert len(wanted)==35 and sha(capture)==declared['capture_SHA256'];records={n:r for n,r in enumerate(physical(capture),1)if n in wanted};event_sides=[];row_sides=[]
    for path in[before_events,events]:
        selected={}
        for event in rows(path):
            refs=event['source_bytes'].get('PacketRefs')or[];found=[r['Number']for r in refs if r['Number']in wanted]
            if found:assert len(refs)==len(found)==1 and found[0]not in selected;selected[found[0]]=event
        assert set(selected)==wanted;event_sides.append(selected)
    for path in[before_cli,cli]:
        selected={}
        for row in rows(path):
            if 'number'not in row:
                assert row.get('record_type')=='protocol_publication'and row.get('publication_phase')=='capture_close'and isinstance(row.get('published_pdus'),list)
                assert not any(ref['Number']in wanted for pdu in row['published_pdus']for ref in pdu['byte_source'].get('PacketRefs')or[]),'a nonselected candidate must not acquire a capture-close contribution'
                continue
            if row['number']in wanted:assert row['number']not in selected;selected[row['number']]=row
        assert set(selected)==wanted;row_sides.append(selected)
    changes=[];unchanged=0
    for n in sorted(wanted):
        record=records[n];facts=candidate_wire(record);old,new=row_sides[0][n],row_sides[1][n];a,b=event_sides[0][n],event_sides[1][n]
        source=next(v for v in declared['observations']if v['frame']==n);assert source['wire_SHA256']==hashlib.sha256(record['wire']).hexdigest()and source['time_ns']==record['time_ns']and source['section']==record['section']and source['interface']==record['interface']
        for row,event in[(old,a),(new,b)]:
            assert integer(row['number'])==n and instant(row['time'])==record['time_ns']and integer(row['length'])==integer(row['captured_length'])==len(record['wire'])
            assert row['source']==facts['source']and row['destination']==facts['destination']and row['protocol']=='UDP'and row['transport']=='udp'
            for key in['source_port','destination_port']:
                assert(key in row)==bool(facts[key]),'omitempty wire zero is absent, not normalized to another scalar type'
                if facts[key]:assert integer(row[key])==facts[key]
            assert row['pdus']==[native_cli_pdu(event)]and canonical(row['pdus'][0])==canonical(native_cli_pdu(event))
            assert not row.get('stream_id')and not row.get('tcp_flow_id')and not row.get('connection_protocol')and not row.get('protocol_stack')
        expected=copy.deepcopy(old)
        if canonical(old)!=canonical(new):
            assert facts['source_port']==4789 and facts['destination_port']!=4789 and len(facts['payload'])>=8
            assert old['capture_domain']==domain(record,facts['legacy_domain'])and new['capture_domain']==domain(record,'')
            expected['capture_domain']=domain(record,'')
            if n==48644:
                corrected_opaque_candidate(a,b,record,n);expected['pdus']=[native_cli_pdu(b)];expected['protocol_status']='unrecognized';expected['info']=f'UDP carrier · unrecognized · {b["summary"]} · {facts["source_port"]} → {facts["destination_port"]} Payload={len(facts["payload"])} (port hint: VXLAN)'
            else:
                assert canonical(a)==canonical(b)and a['status']==a['completeness']=='malformed'and a['expert']=='MalformedMessage'and facts['UDP_length']>facts['IP_UDP_extent']
                message=f'UDP length {facts["UDP_length"]} exceeds declared IP payload available to UDP ({facts["IP_UDP_extent"]} bytes)';assert a['summary']==message and a['error']=='MalformedMessage: '+message
                assert old['info'].endswith(' [incomplete/undecoded]');expected['info']=old['info'][:-len(' [incomplete/undecoded]')]
            assert canonical(expected)==canonical(new),'the entire new row must equal a literal-source-derived expected row; no path waiver or ID normalization'
            evidence={k:v for k,v in facts.items()if k!='payload'};evidence['header_hex']=record['wire'][:facts['payload_offset']+8].hex();evidence['wire_SHA256']=hashlib.sha256(record['wire']).hexdigest();evidence['source_time_ns']=record['time_ns'];evidence['source_section']=record['section'];evidence['source_interface']=record['interface'];changes.append(dict(frame=n,literal_source=evidence,before_row_SHA256=digest(old),after_row_SHA256=digest(new),before_SDK_SHA256=digest(a),after_SDK_SHA256=digest(b),patch=exact_patch(old,new)))
        else:unchanged+=1;assert canonical(a)==canonical(b)
    assert {v['frame']for v in changes}=={48568,48644,98505,133116}and unchanged==31
    return dict(scope='Full original Sharon candidate35; all CLI PDUs match their exact SDK values and physical source, with four literal-derived complete expected row corrections, no path waiver',auditor_SHA256=sha(Path(__file__)),capture_SHA256=sha(capture),inventory_SHA256=sha(inventory),before_SDK_SHA256=sha(before_events),SDK_SHA256=sha(events),before_CLI_SHA256=sha(before_cli),CLI_SHA256=sha(cli),original_sources=35,complete_literal_CLI_rows_unchanged=31,source_derived_complete_CLI_row_corrections=4,actual_generated_ID_changes=0,all_non_correction_row_values_and_types_literal=True,changes=changes)


def audit(capture,oracle,events,flows):
    physical_records={n:r for n,r in enumerate(physical(capture),1)};scans={n:scan(r)for n,r in physical_records.items()};reference=json.loads(oracle.read_text('utf8'));assert len(reference)==len(physical_records);occurrences=0;packets=0;quote_packets=0;quote_occurrences=0;depth=collections.Counter();headers=[]
    for n,frame in enumerate(reference,1):
        layers=frame['_source']['layers'];assert scalar(layers['frame']['frame.number'])==n;raw=layers['frame_raw'];record=physical_records[n];assert len(raw)==6 and raw[1:]==[0,len(record['wire']),0,1,0]and bytes.fromhex(raw[0])==record['wire']
        observed=scans[n]['headers'];quoted=scans[n]['quoted_headers'];literal=values(layers,'vxlan.vni_raw');assert len(literal)==len(observed)+len(quoted),('all actual/quoted nested original VXLAN headers require their own source binding',n,len(literal),len(observed),len(quoted))
        if observed:packets+=1;depth[len(observed)]+=1
        if quoted:quote_packets+=1
        for header in observed+quoted:
            assert header['selected']or header['source_scope']=='ICMP quote; no tunnel state','this positive public preservation audit needs standard complete selected VXLAN parents'
            wire=record['wire'];reference_header(layers,header,wire);occurrences+=bool(header['selected']);quote_occurrences+=not header['selected']
            headers.append(dict(frame=n,section=record['section'],interface=record['interface'],time_ns=record['time_ns'],wire_SHA256=hashlib.sha256(wire).hexdigest(),**header))
    native=native_ownership(physical_records,scans,list(rows(events)),json.loads(flows.read_text('utf8')))
    return dict(scope=__doc__,auditor_SHA256=sha(Path(__file__)),capture_SHA256=sha(capture),reference_SHA256=sha(oracle),SDK_SHA256=sha(events),flows_SHA256=sha(flows),physical_packets=len(physical_records),packets_with_selected_VXLAN=packets,source_bound_selected_VXLAN_header_occurrences=occurrences,packets_with_quoted_VXLAN=quote_packets,source_bound_quoted_VXLAN_header_occurrences=quote_occurrences,nested_depth_histogram=dict(depth),source_bound_headers=headers,native=native,group_policy_or_GPE_payload_implementation_claim=False,full_Wireshark_application_field_parity_claim=False)


if __name__=='__main__':
    parser=argparse.ArgumentParser(description=__doc__)
    for name in['capture','oracle','events','flows','out']:parser.add_argument('--'+name,type=Path,required=True)
    args=parser.parse_args()
    for p in vars(args).values():assert not any((v/'.git').exists()for v in[p.resolve(),*p.resolve().parents])
    proof=audit(args.capture,args.oracle,args.events,args.flows)
    with args.out.open('x',encoding='utf8')as f:json.dump(proof,f,indent=2);f.write('\n')
    print(json.dumps({k:v for k,v in proof.items()if k not in['scope','source_bound_headers','native']}),flush=True)
