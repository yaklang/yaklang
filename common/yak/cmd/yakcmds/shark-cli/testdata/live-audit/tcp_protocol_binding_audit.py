"""Verify protocol-session/TCP-generation/page links against native packet hooks.

This is a projection/ownership audit, not an independent TCP receive algorithm.
Wire/oracle correctness needs its separate source-certified capture audit. IDs
in each namespace stay literal; no endpoint/time-window ID guess is accepted.
"""
import argparse
import collections
import hashlib
import json
from pathlib import Path

from dcerpc_cli_invariants import identity
from dcerpc_wire_audit import records
from mail_rtsp_export_invariants import canonical,rows,sha
from mail_rtsp_public_wire_audit import tcp_packet


def domain(value):
    assert type(value['Section'])is int and type(value['Interface'])is int and isinstance(value.get('Encapsulation',''),str),'capture-domain metadata keeps literal integer/string types'
    return value['Section'],value['Interface'],value.get('Encapsulation','')


def key(value):return hashlib.sha256(value.encode()).hexdigest()


def sdk_projection_identity(e):
    # The SDK owns string endpoints, while the display schema deliberately
    # omits unavailable empty strings. Verify that exact absence contract;
    # never invent an endpoint for an undecoded network diagnostic.
    endpoints=[];missing=0
    for name in['source','destination']:
        projected=[v['value']for v in(e['field_values'].get('values')or[])if v['name']==name]
        if e[name]=='':
            assert not projected,'unavailable SDK endpoint stays absent in display fields'
            endpoints.append(None);missing+=1
        else:
            assert projected==[e[name]],'available endpoint has an exact display projection'
            endpoints.append(e[name])
    source=e['source_bytes']
    return canonical([e['protocol'],*endpoints,e['flow'],source.get('Kind'),source.get('SHA256'),source.get('PacketRefs')]),missing


def network_fragment(wire,link):
    """Read an actual outer fragment header, without guessing an inner TCP tuple."""
    if link!=1 or len(wire)<14:return None
    at,kind=14,int.from_bytes(wire[12:14],'big')
    for _ in range(16):
        if kind not in[0x8100,0x88a8,0x9100]:break
        if at+4>len(wire):return None
        kind=int.from_bytes(wire[at+2:at+4],'big');at+=4
    if kind==0x0800:
        if at+20>len(wire)or wire[at]>>4!=4:return None
        size=(wire[at]&15)*4;total=int.from_bytes(wire[at+2:at+4],'big')
        if size<20 or total<size or at+total>len(wire):return None
        value=int.from_bytes(wire[at+6:at+8],'big')
        if not value&0x3fff:return None
        return dict(version=4,identifier=int.from_bytes(wire[at+4:at+6],'big'),offset=(value&0x1fff)*8,more=bool(value&0x2000),next_header=wire[at+9])
    if kind!=0x86dd or at+40>len(wire)or wire[at]>>4!=6:return None
    end=at+40+int.from_bytes(wire[at+4:at+6],'big')
    if end>len(wire):return None
    nxt=wire[at+6];at+=40
    for _ in range(16):
        if nxt==44:
            if at+8>end:return None
            value=int.from_bytes(wire[at+2:at+4],'big')
            if value&6 or not(value&0xfff9):return None # reserved/atomic is not a deferred fragment
            return dict(version=6,identifier=int.from_bytes(wire[at+4:at+8],'big'),offset=value&0xfff8,more=bool(value&1),next_header=wire[at])
        if nxt not in[0,43,60]or at+2>end:return None
        size=(wire[at+1]+1)*8
        if at+size>end:return None
        nxt=wire[at];at+=size
    return None


def opening_controls(capture,wanted):
    """Certify deferred original SYNs or outer fragments, with separate scopes."""
    found={};fragments=[];openings=[]
    for no,section,interface,link,frame in records(capture):
        if(no,section,interface)not in wanted:continue
        decoded,decode_link=frame,link
        if link in[0,108]and len(frame)>=4:
            family=int.from_bytes(frame[:4],'little'if link==0 else'big')
            if family in[2,10,23,24,28,30]and len(frame)>4:
                version=frame[4]>>4
                assert(version==4 and family==2)or(version==6 and family!=2),'original loopback family matches its captured IP version'
                decoded,decode_link=bytes(12)+(b'\x08\0'if version==4 else b'\x86\xdd')+frame[4:],1
        elif link in[12,101,228,229]and frame:
            version=frame[0]>>4
            if version in[4,6]:decoded,decode_link=bytes(12)+(b'\x08\0'if version==4 else b'\x86\xdd')+frame,1
        elif link==113 and len(frame)>=16:
            decoded,decode_link=bytes(12)+frame[14:16]+frame[16:],1
        elif link==276 and len(frame)>=20:
            decoded,decode_link=bytes(12)+frame[:2]+frame[20:],1
        fragment=network_fragment(decoded,decode_link)
        if fragment is not None:
            fragments.append(dict(frame=no,section=section,interface=interface,original_frame_SHA256=hashlib.sha256(frame).hexdigest(),**fragment))
            found[(no,section,interface)]=True
            continue
        packet=tcp_packet(decoded,decode_link,section,interface,True)
        assert packet is not None,('original held-opening control must have independently readable TCP bytes',no)
        assert packet['syn']and not packet['ack']and not packet['fin']and not packet['reset']and not packet['payload'],('only a held payload-free opening SYN may await later ownership',no)
        found[(no,section,interface)]=True
        openings.append(no)
    assert set(found)==wanted,'every deferred row has an original physical control record'
    return dict(openings=sorted(openings),network_fragments=fragments)


def audit(capture,observer,sdk,cli,allow_empty_tcp=False):
    hooks=json.loads(observer.read_text('utf8'))
    assert hooks['capture_SHA256']==sha(capture),'observer binds the exact input file'
    assert not hooks['error'],('a failed native replay cannot certify ownership',hooks['error'])
    for name in['flows','packets','closed']:assert hooks[name]is None or type(hooks[name])is list
    observed_flows=hooks['flows']or[];observed_packets=hooks['packets']or[]
    flows={v['flow']:v for v in observed_flows}
    assert all(type(v['flow'])is int and v['flow']>0 for v in observed_flows)
    assert len(flows)==len(observed_flows),'unique physical TCP generation IDs'
    owners=collections.defaultdict(set)
    physical_owners=collections.defaultdict(set)
    physical_states=collections.defaultdict(set)
    for packet in observed_packets:
        assert type(packet['flow'])is int and packet['flow']in flows
        for ref in packet['refs']:
            assert type(ref['Number'])is int and ref['Number']>0
            owners[(ref['Number'],domain(ref['Domain']))].add(packet['flow'])
            physical_owners[(ref['Number'],ref['Domain']['Section'],ref['Domain']['Interface'])].add(packet['flow'])
            if 'state'in packet:
                assert packet['state']in['observed','closed-direction-observation','closed-observation']
                physical_states[(ref['Number'],ref['Domain']['Section'],ref['Domain']['Interface'])].add((packet['flow'],packet['state']))
    pools=collections.defaultdict(collections.deque)
    protocol_to_tcp=collections.defaultdict(set)
    event_bindings={};parent_links=[];missing=[];captured=0;tcp_without_direct_binding=0
    tcp_different_protocol_numbers=0;context_only=[];unavailable_endpoint_projections=0
    for e in rows(sdk):
        identifier=e['id'];assert type(identifier)is int and identifier>0 and identifier not in event_bindings
        assert type(e['flow'])is int and e['flow']>=0
        native=e.get('tcp_flow_id',0)
        assert type(native)is int and native>=0
        event_bindings[identifier]=native
        source=e['source_bytes'];refs=source.get('PacketRefs')or[]
        if native:
            assert e['transport']=='tcp'and native in flows,'a physical TCP link needs an actual TCP generation'
            flow=flows[native]
            assert domain(flow['domain'])==domain(e['domain']),'event and native TCP capture domains agree'
            assert type(e['direction'])is int and e['direction']in[0,1]
            endpoints=[flow['source'],flow['destination']]
            if e['direction']==1:endpoints.reverse()
            assert[e['source'],e['destination']]==endpoints,'literal native endpoints and original-observer direction'
            for ref in refs:
                assert type(ref['Number'])is int and ref['Number']>0
                assert native in owners[(ref['Number'],domain(ref['Domain']))],('every event contributor belongs to its exact native generation',identifier,ref['Number'],native)
            completed=source.get('CompletedIn')
            if completed:
                assert type(completed['Number'])is int and completed['Number']>0
                assert native in owners[(completed['Number'],domain(completed['Domain']))],'complete-in is a real contributor in the same native generation'
            parents=source.get('ParentPDUs')or[]
            if source.get('ParentPDU'):parents=[source['ParentPDU'],*parents]
            assert all(type(parent)is int and parent>0 for parent in parents)
            parent_links.extend((identifier,native,parent)for parent in parents)
            if not refs and not parents:
                assert not e['raw'],'nonempty TCP message bytes need actual source evidence'
                context_only.append(dict(PDU=identifier,protocol=e['protocol'],status=e['status']))
            alias=[v for v in e['field_values'].get('values')or[]if v['name']=='tcp.flow.id']
            assert canonical(alias)==canonical([dict(name='tcp.flow.id',value=native,exact_value=str(native),origin='event')]),'typed explicit TCP generation alias has no fabricated wire range'
            protocol_to_tcp[e['flow']].add(native)
            captured+=1;tcp_different_protocol_numbers+=native!=e['flow']
        else:
            assert not any(v['name']=='tcp.flow.id'for v in e['field_values'].get('values')or[]),'absence is not an invented zero TCP binding'
            if e['transport']=='tcp':
                tcp_without_direct_binding+=1
                candidates=set(flows)
                for ref in refs:candidates&=owners[(ref['Number'],domain(ref['Domain']))]
                candidates={n for n in candidates if domain(flows[n]['domain'])==domain(e['domain'])and{flows[n]['source'],flows[n]['destination']}=={e['source'],e['destination']}}
                if refs and candidates:missing.append(dict(PDU=identifier,native_candidates=sorted(candidates)))
        projected_identity,absent=sdk_projection_identity(e)
        unavailable_endpoint_projections+=absent
        pools[key(projected_identity)].append((identifier,native))
    assert not missing,('direct captured TCP events must retain their explicit native binding',missing[:20])
    assert all(len(values)==1 for values in protocol_to_tcp.values()),'a protocol session cannot span separate TCP generations'
    for child,native,parent in parent_links:
        assert parent in event_bindings and event_bindings[parent]==native,('TCP child and original parent keep their generation',child,parent)
    empty_tcp=not flows and not observed_packets and not hooks['closed']and captured==tcp_without_direct_binding==0 and bool(event_bindings)
    assert captured>0 or allow_empty_tcp and empty_tcp,'positive captured-TCP coverage is required unless explicitly certifying an observed empty TCP namespace'
    selected={};preview_by_tcp={};preview_to_tcp={};deferred_controls=set()
    physical=0;PDU_count=0;close_publications=0;unconfirmed_closed_without_page=0
    for row in rows(cli):
        publication=row.get('record_type')=='protocol_publication'
        if publication:
            assert row['publication_phase']=='capture_close'
            assert set(row)=={'record_type','publication_phase','published_pdu_scope','published_pdus'},'close publication has no fabricated physical columns'
            assert len(row['published_pdus'])==1,'one independently bounded close publication'
            close_publications+=1
        else:
            assert 'record_type'not in row,'unknown records cannot masquerade as physical packets'
            physical+=1
        native=row.get('tcp_flow_id',0);preview=row.get('stream_id',0)
        assert type(native)is int and native>=0 and type(preview)is int and preview>=0
        if native:
            assert native in flows
            assert domain(row['tcp_flow_domain'])==domain(flows[native]['domain'])
            physical_key=(row['number'],row['capture_domain']['Section'],row['capture_domain']['Interface'])
            assert native in physical_owners[physical_key],'physical row links to its actual native packet observation (including normalized tunnel domains)'
            if preview==0:
                state=row.get('tcp_flow_state');assert state in['closed-observation','closed-direction-observation']and(native,state)in physical_states[physical_key],'a missing page requires its actual closed native packet-hook state'
                assert not row.get('connection_protocol')and not row.get('connection_stack')and row['protocol']=='TCP'
                assert row['protocol_status']=='context-required'and row['visibility']in['header-only','unassembled-stream']and'after observed close; generation unconfirmed'in row['info']
                unconfirmed_closed_without_page+=1
            else:
                if native in preview_by_tcp:assert preview_by_tcp[native]==preview,'one preview per native generation'
                if preview in preview_to_tcp:assert preview_to_tcp[preview]==native,'a preview cannot merge different native generations'
                preview_by_tcp[native]=preview;preview_to_tcp[preview]=native
        elif not publication and(row['number'],row['capture_domain']['Section'],row['capture_domain']['Interface'])in physical_owners:
            deferred_controls.add((row['number'],row['capture_domain']['Section'],row['capture_domain']['Interface']))
        for pdu in(row.get('pdus')or[])+(row.get('published_pdus')or[]):
            if pdu['id']==0:continue # explicit bounded detail marker, not a PDU
            if pdu['id']in selected:
                assert selected[pdu['id']]==pdu,'retained copies of a CLI PDU remain immutable'
                continue
            selected[pdu['id']]=pdu
            available=pools[key(identity(pdu))]
            assert available,('CLI PDU binds to an original SDK source/protocol-session identity',row.get('number','capture_close'),pdu['id'],pdu['protocol'])
            _,expected=available.popleft()
            assert type(pdu.get('tcp_flow_id',0))is int and pdu.get('tcp_flow_id',0)==expected,'CLI PDU retains a literal SDK TCP generation, separate from flow.id'
            values=pdu.get('fields')or{}
            assert canonical(values.get('tcp.flow.id',[]))==canonical([expected]if expected else[]),'CLI typed TCP binding is exact'
            aliases=[v for v in(pdu['field_values'].get('values')or[])if v['name']=='tcp.flow.id']
            assert canonical(aliases)==canonical([dict(name='tcp.flow.id',value=expected,exact_value=str(expected),origin='event')]if expected else[])
            PDU_count+=1
    deferred=opening_controls(capture,deferred_controls)if deferred_controls else dict(openings=[],network_fragments=[])
    assert not any(pools.values()),'no SDK PDU disappeared from the frozen CLI projection'
    return dict(unconfirmed_closed_TCP_records_without_application_page=unconfirmed_closed_without_page,closed_packet_state_hook_count=sum(len(v)for v in physical_states.values()),scope=__doc__,capture_SHA256=sha(capture),observer_SHA256=sha(observer),SDK_SHA256=sha(sdk),CLI_SHA256=sha(cli),auditor_SHA256=sha(Path(__file__)),physical_rows=physical,capture_close_publications=close_publications,native_TCP_generations=len(flows),source_bound_TCP_events=captured,positive_captured_TCP_coverage=captured>0,explicit_empty_TCP_namespace=empty_tcp and allow_empty_tcp,events_with_different_protocol_and_TCP_numbers=tcp_different_protocol_numbers,protocol_sessions_with_captured_TCP=len(protocol_to_tcp),context_only_TCP_records=context_only,TCP_events_without_direct_capture_binding=tcp_without_direct_binding,unavailable_endpoint_projections=unavailable_endpoint_projections,compared_CLI_PDUs=PDU_count,source_bound_parent_links=len(parent_links),deferred_opening_rows=deferred['openings'],deferred_network_fragment_rows=deferred['network_fragments'],preview_by_native_TCP=preview_by_tcp,all_checked_bindings_exact=True,wire_or_Wireshark_parity_claim=False)


if __name__=='__main__':
    ap=argparse.ArgumentParser(description=__doc__)
    for name in['capture','observer','sdk','cli','out']:ap.add_argument('--'+name,type=Path,required=True)
    ap.add_argument('--allow-empty-tcp',action='store_true',help='Explicit absence/projection check for complete observations with no native TCP flows, packets or closes; does not certify positive TCP coverage.')
    a=ap.parse_args()
    for path in[a.capture,a.observer,a.sdk,a.cli,a.out]:assert not any((p/'.git').exists()for p in[path.resolve(),*path.resolve().parents])
    result=audit(a.capture,a.observer,a.sdk,a.cli,a.allow_empty_tcp)
    with a.out.open('x',encoding='utf8',newline='\n')as out:json.dump(result,out,indent=2);out.write('\n')
    print(json.dumps(dict(TCP_events=result['source_bound_TCP_events'],CLI_PDUs=result['compared_CLI_PDUs'],all_checked_bindings_exact=True)),flush=True)
