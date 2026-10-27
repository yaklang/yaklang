"""Audit selected IMAP/RTSP bytes in original merged PCAPNG captures.

Scope: directly carried Ethernet/VLAN IPv4/IPv6 TCP. Parse the original PCAPNG
record/interface and TCP sequence boundaries independently; certify exact native
PDU bytes, contributors, namespace layouts and observed control association.
Missing peer sequence ranges are explicit, not invented protocol malformation.
Wireshark field coverage is reported independently from byte/source agreement.
"""
import argparse
import base64
import collections
import hashlib
import ipaddress
import json
import struct
from pathlib import Path

from imap_rtsp_wire_audit import namespace_groups


def sha(w):return hashlib.sha256(w).hexdigest()
def canonical(v):return json.dumps(v,sort_keys=True,separators=(',',':'),ensure_ascii=True)
def records(path):
    data=path.read_bytes();at=0;section=-1;interfaces=[];number=0;endian=None
    while at<len(data):
        assert at+12<=len(data),'complete PCAPNG block'
        if data[at:at+4]==b'\x0a\x0d\x0d\x0a':
            endian='<'if data[at+8:at+12]==b'\x4d\x3c\x2b\x1a'else'>'
            assert data[at+8:at+12]in[b'\x4d\x3c\x2b\x1a',b'\x1a\x2b\x3c\x4d']
            section+=1;interfaces=[]
        assert endian is not None,'PCAPNG section header precedes packets'
        kind,size=struct.unpack_from(endian+'II',data,at)
        assert size>=12 and size%4==0 and at+size<=len(data)
        assert struct.unpack_from(endian+'I',data,at+size-4)[0]==size,'PCAPNG trailing block length'
        if kind==1:
            assert size>=20
            interfaces.append((struct.unpack_from(endian+'H',data,at+8)[0],struct.unpack_from(endian+'I',data,at+12)[0]))
        elif kind==6:
            assert size>=32
            interface,_,_,captured,original=struct.unpack_from(endian+'IIIII',data,at+8)
            assert interface<len(interfaces) and captured<=original and 28+captured<=size-4
            number+=1
            yield number,section,interface,interfaces[interface][0],data[at+28:at+28+captured]
        elif kind==3:
            assert interfaces and size>=16
            original=struct.unpack_from(endian+'I',data,at+8)[0]
            captured=min(original,interfaces[0][1]or original)
            assert captured<=size-16
            number+=1
            yield number,section,0,interfaces[0][0],data[at+12:at+12+captured]
        at+=size
    assert at==len(data)


def tcp_packet(wire,link,section,interface,include_control=False):
    if link!=1 or len(wire)<14:return None
    at,etype=14,int.from_bytes(wire[12:14],'big');encapsulation=''
    while etype in [0x8100,0x88a8,0x9100]:
        if at+4>len(wire):return None
        encapsulation+='/vlan:'+str(int.from_bytes(wire[at:at+2],'big')&4095)
        etype=int.from_bytes(wire[at+2:at+4],'big');at+=4
    if etype==0x0800:
        if at+20>len(wire)or wire[at]>>4!=4:return None
        hlen=(wire[at]&15)*4;total=int.from_bytes(wire[at+2:at+4],'big')
        if hlen<20 or at+hlen>len(wire)or wire[at+9]!=6 or int.from_bytes(wire[at+6:at+8],'big')&0x3fff:return None
        source,destination=str(ipaddress.ip_address(wire[at+12:at+16])),str(ipaddress.ip_address(wire[at+16:at+20]))
        end=min(at+total,len(wire));at+=hlen
    elif etype==0x86dd:
        if at+40>len(wire)or wire[at]>>4!=6:return None
        source,destination=str(ipaddress.ip_address(wire[at+8:at+24])),str(ipaddress.ip_address(wire[at+24:at+40]))
        end=min(at+40+int.from_bytes(wire[at+4:at+6],'big'),len(wire));nxt=wire[at+6];at+=40
        for _ in range(16):
            if nxt==6:break
            if nxt not in [0,43,60]or at+2>end:return None
            size=(wire[at+1]+1)*8
            if at+size>end:return None
            nxt=wire[at];at+=size
        if nxt!=6:return None
    else:return None
    if at+20>end:return None
    sport,dport,sequence=struct.unpack_from('>HHI',wire,at)
    hlen=(wire[at+12]>>4)*4
    if hlen<20 or at+hlen>end:return None
    endpoint=lambda host,port:('['+host+']'if ':'in host else host)+':'+str(port)
    packet=dict(domain={'Section':section,'Interface':interface,'Encapsulation':encapsulation},source=endpoint(source,sport),destination=endpoint(destination,dport),sequence=sequence,syn=bool(wire[at+13]&2),payload=wire[at+hlen:end])
    if include_control:
        packet.update(ack=bool(wire[at+13]&16),ack_sequence=struct.unpack_from('>I',wire,at+8)[0],fin=bool(wire[at+13]&1),reset=bool(wire[at+13]&4))
    return packet


def audit(capture,events,oracle):
    checks=0
    def check(ok,why):
        nonlocal checks
        checks+=1;assert ok,why
    def eq(a,b,why):check(canonical(a)==canonical(b),why)
    def conn(e):return canonical([e['domain'],sorted([e['source'],e['destination']])])
    def key(e):return canonical([e['domain'],e['source'],e['destination']])
    def leaves(v,name):
        if isinstance(v,dict):
            for k,x in v.items():
                if k==name:yield from x if isinstance(x,list)else[x]
                elif isinstance(x,(dict,list)):yield from leaves(x,name)
        elif isinstance(v,list):
            for x in v:yield from leaves(x,name)
    native=[];closures=[];wanted=set()
    for line in events.open(encoding='utf-8-sig'):
        e=json.loads(line)
        if e['protocol']not in ['imap','rtsp']:continue
        if not e.get('raw'):
            check(e['status']in ['incomplete','context-required'],'empty selected records are explicit closure/context diagnostics')
            closures.append(dict(PDU=e['id'],protocol=e['protocol'],status=e['status'],summary=e['summary']));continue
        native.append(e);wanted.add(conn(e))
    streams={};physical={};frame_count=0
    for no,section,interface,link,wire in records(capture):
        frame_count=no
        packet=tcp_packet(wire,link,section,interface)
        if packet is None or conn(packet)not in wanted:continue
        physical[no]=dict(packet,raw_sha256=sha(wire))
        state=streams.setdefault(key(packet),dict(meta={k:packet[k]for k in ['domain','source','destination']},packets=[],syns=[],cursor=0))
        if packet['syn']:state['syns'].append((packet['sequence']+1)&0xffffffff)
        if packet['payload']:state['packets'].append((packet['sequence']+int(packet['syn']),packet['payload'],no))
    for state in streams.values():
        check(bool(state['packets']),'each selected direction has actual captured bytes')
        anchor=state['packets'][0][0]
        signed=lambda value:((value-anchor+(1<<31))&0xffffffff)-(1<<31)
        base=state['syns'][0]if state['syns']else(anchor+min(signed(seq)for seq,_,_ in state['packets']))&0xffffffff
        check(not state['syns']or all(v==base for v in state['syns']),'no tuple-generation mixing inside selected connection')
        cells,owners={},{}
        for seq,wire,no in state['packets']:
            offset=(seq-base)&0xffffffff
            check(offset+len(wire)<=4<<20,'bounded selected stream')
            for i,byte in enumerate(wire,offset):
                check(i not in cells or cells[i]==byte,'no conflicting selected TCP overlap')
                cells[i]=byte;owners.setdefault(i,no)
        holes=[];end=max(cells)+1;start=None
        for i in range(end):
            if i not in cells and start is None:start=i
            elif i in cells and start is not None:holes.append([start,i]);start=None
        if start is not None:holes.append([start,end])
        state.update(cells=cells,owners=owners,holes=holes,end=end,SYN_observed=bool(state['syns']),contiguous=holes[0][0]if holes else end)
    reference={};reference_rtsp=collections.defaultdict(list)
    for row in json.loads(oracle.read_text(encoding='utf-8-sig')):
        layers=row['_source']['layers'];no=int(layers['frame']['frame.number'])
        check(no in physical,'reference packet belongs to the exact selected tuple/domain')
        eq(sha(bytes.fromhex(layers['frame_raw'][0])),physical[no]['raw_sha256'],'reference physical frame equals original PCAPNG bytes')
        reference[no]=layers
        spans=layers.get('rtsp_raw')or[]
        if spans and isinstance(spans[0],str):spans=[spans]
        for span in spans:reference_rtsp[no,sha(bytes.fromhex(span[0]))].append(layers.get('rtsp',{}))
    compared=[];reference_lines=0;wire_only=[];sasl={};namespace_pending={};rtsp_pending={};peer_context=[];rtsp_fields=collections.Counter();rtsp_limits=[]
    for e in native:
        raw=base64.b64decode(e['raw'],validate=True);state=streams[key(e)];start=state['cursor'];end=start+len(raw)
        check(all(i in state['cells']for i in range(start,end)),'native never invents missing captured bytes')
        eq(sha(bytes(state['cells'][i]for i in range(start,end))),sha(raw),'complete selected PDU bytes equal independent TCP sequence reconstruction')
        state['cursor']=end
        source=e['source_bytes'];eq(source['SHA256'],sha(raw),'native raw SHA')
        owners=sorted(set(state['owners'][i]for i in range(start,end)))
        eq(sorted(r['Number']for r in source['PacketRefs']),owners,'complete contributor identity')
        for ref in source['PacketRefs']:
            eq(ref['Domain'],e['domain'],'native contributor domain')
            eq(physical[ref['Number']]['domain'],e['domain'],'independent Ethernet/VLAN/interface/section domain')
        if source.get('CompletedIn'):
            eq(source['CompletedIn']['Number'],max(owners),'native completion equals final byte arrival')
            eq(source['CompletedIn']['Domain'],e['domain'],'native completion domain')
        else:check(e['status']not in ['decoded','deferred'],'accepted message requires completion source')
        s=e.get('session')or{};c=conn(e)
        if e['status']=='context-required':
            peer=streams[canonical([e['domain'],e['destination'],e['source']])]
            visible_holes=[span for span in peer['holes']if any(i>=span[1]and peer['owners'][i]<=max(owners)for i in peer['owners'])]
            check(bool(visible_holes),'missing peer context is supported by a TCP hole visible at this exact observation')
            eq(e['fields'],None,'context diagnostic publishes no accepted generic fields')
            peer_context.append(dict(PDU=e['id'],physical_frames=owners,peer_missing_ranges=visible_holes,status=e['status']))
        elif e['protocol']=='imap':
            check(e['status']=='decoded','selected non-context IMAP messages decode')
            check(isinstance(e['fields'],dict),'accepted IMAP result has an actual field projection')
            if raw.endswith(b'\r\n'):
                displays=list(leaves(reference.get(max(owners),{}).get('imap'),'imap.line'))
                if raw.decode('ascii',errors='backslashreplace')in displays:reference_lines+=1
                else:wire_only.append(dict(PDU=e['id'],protocol='imap',reason='reference literal/line attribution differs; exact source bytes independently compared'))
            stage=s.get('SASL Stage');words=raw[:-2].split(b' ')if raw.endswith(b'\r\n')else[]
            if stage=='command':
                check(c not in sasl,'one observed authentication owner')
                sasl[c]=dict(tag=words[0],mechanism=words[2],client=e['source'],request=e['id'],challenge=0)
                owner=sasl[c]
                eq(s['Mechanism'],words[2].decode(),'exact observed SASL mechanism')
            elif stage=='challenge':
                owner=sasl[c];check(owner['client']!=e['source']and raw.startswith(b'+ '),'observed opposite-direction SASL challenge')
                eq(e['response_to'],owner['request'],'challenge association');owner['challenge']=e['id']
            elif stage in ['response','cancel']:
                owner=sasl[c];check(owner['client']==e['source']and owner['challenge'],'observed client response has exact captured challenge')
                eq(e['response_to'],owner['challenge'],'response association')
                expected=b''if raw==b'*\r\n'else base64.b64decode(raw[:-2],validate=True)
                eq(s['Transport Value']['Decoded Payload'],base64.b64encode(expected).decode(),'complete independently decoded SASL value')
                value=s['Transport Value'];encoded=b''if raw==b'*\r\n'else raw[:-2]
                eq(value['Encoded Value'],base64.b64encode(encoded).decode(),'complete encoded SASL value')
                eq(value['Encoded Length'],len(encoded),'SASL encoded length')
                eq(value['Decoded Length'],len(expected),'SASL decoded length')
                eq(value['Canceled'],stage=='cancel','SASL cancellation is not a value')
                eq(value['Decoded Values Are Wire Spans'],False,'decoded SASL value is not a frame span')
                if stage=='response':eq(value['Wire Field Ranges']['Encoded Value'],[0,len(encoded)],'exact encoded response range')
            elif stage=='result':
                owner=sasl.pop(c);check(owner['client']!=e['source']and words[0]==owner['tag'],'matching observed server result')
                eq(e['response_to'],owner['request'],'authentication result association')
            if stage:
                eq(s['Tag'],owner['tag'].decode(),'exact SASL owning command tag')
                eq(s['Mechanism'],owner['mechanism'].decode(),'exact SASL owning mechanism')
                for flag in ['Authentication Verified','Peer Identity Verified','Security Layer Verified']:eq(s[flag],False,'no unobserved verification claim')
            ns=s.get('Namespace Stage')
            if ns=='command':namespace_pending[c,s['Tag']]=(e['id'],e['source'])
            elif ns=='response':
                expected=namespace_groups(raw)
                for name,value in expected.items():eq(s['IMAP Fields'][name],value,'complete namespace values/extensions/ranges')
                candidates=[value for (connection,_),value in namespace_pending.items()if connection==c and value[1]!=e['source']]
                eq(e['response_to'],candidates[0][0]if len(candidates)==1 else 0,'namespace ambiguity-safe association')
            elif ns=='result':
                request,client=namespace_pending.pop((c,s['Tag']));check(client!=e['source'],'namespace result direction');eq(e['response_to'],request,'namespace tagged result association')
        else:
            check(e['status']=='decoded','selected RTSP messages decode')
            header,separator,body=raw.partition(b'\r\n\r\n');check(bool(separator),'complete RTSP header')
            lines=header.split(b'\r\n');h=collections.defaultdict(list)
            for line in lines[1:]:
                name,value=line.split(b':',1);h[name.lower()].append(value.strip())
            eq(s['Content Length'],len(body),'native RTSP body length')
            eq(len(body),int(h.get(b'content-length',[b'0'])[0]),'independent RTSP body boundary')
            parts=lines[0].split(b' ',2)
            views=[view for no in owners for digest in {sha(raw),sha(header+separator)} for view in reference_rtsp[no,digest]]
            required={'rtsp.status':int(parts[1])}if parts[0]==b'RTSP/1.0'else{'rtsp.method':parts[0].decode(),'rtsp.url':parts[1].decode()}
            if h[b'cseq']:required['rtsp.cseq']=int(h[b'cseq'][0])
            if h[b'content-length']:required['rtsp.content-length']=len(body)
            missing=[]
            for name,value in required.items():
                available=[v for view in views for v in leaves(view,name)]
                if available:
                    if isinstance(value,int):available=[int(v)for v in available]
                    check(value in available,('exact frame/byte-bound reference RTSP field',e['id'],name))
                    rtsp_fields[name]+=1
                else:missing.append(name)
            if missing:rtsp_limits.append(dict(PDU=e['id'],physical_frames=owners,method=parts[0].decode(),unavailable_reference_fields=missing))
            if not h[b'cseq']:
                eq(s['CSeq Observed'],False,'Apple control missing CSeq remains explicit')
                check(s['Transaction Association']=='unavailable-no-cseq','no invented no-CSeq transaction')
            else:
                eq(len(h[b'cseq']),1,'unique CSeq');seq=int(h[b'cseq'][0]);eq(s['CSeq'],seq,'exact CSeq')
                if parts[0]==b'RTSP/1.0':
                    method=rtsp_pending.pop((c,e['destination'],seq),None)
                    eq(s['Status'],int(parts[1]),'exact RTSP response status')
                    eq(s['Matched'],method is not None,'RTSP request/reply matching')
                    if method is not None:eq(s['In Reply To'],method,'correct method correlation')
                else:
                    method=parts[0].decode();eq(s['Packet Name'],method,'complete RTSP method');eq(s['URI'],parts[1].decode(),'RTSP target')
                    if (c,e['source'],seq)in rtsp_pending:eq(s.get('Retransmission'),True,'identical outstanding CSeq remains explicit retransmission')
                    rtsp_pending[c,e['source'],seq]=method
                    if method=='FLUSH':
                        eq(s['Media Effect Verified'],False,'no inferred media reset')
                        if h[b'rtp-info']:
                            params=dict(part.strip().split(b'=',1)for part in h[b'rtp-info'][0].split(b';'))
                            for name in ['seq','rtptime']:
                                if name.encode()in params:eq(s['RTP Info'][0][name],int(params[name.encode()]),'exact FLUSH RTP-Info')
        compared.append(dict(PDU=e['id'],protocol=e['protocol'],status=e['status'],physical_frames=owners,raw_sha256=sha(raw)))
    check(not sasl,'all selected observed authentication exchanges complete')
    check(not namespace_pending,'all selected namespace requests complete')
    details=[]
    for state in streams.values():
        check(state['cursor']<=state['contiguous'],'native protocol bytes never jump over a TCP hole')
        details.append(dict(**state['meta'],SYN_observed=state['SYN_observed'],native_bytes=state['cursor'],captured_contiguous_bytes=state['contiguous'],captured_bytes=len(state['cells']),missing_ranges=state['holes']))
    return dict(scope=__doc__,checks=checks,capture_sha256=sha(capture.read_bytes()),events_sha256=sha(events.read_bytes()),oracle_sha256=sha(oracle.read_bytes()),physical_frames=frame_count,
                compared_PDUs=compared,wire_bound_record_counts=dict(collections.Counter(e['protocol']for e in native)),reference_IMAP_line_fields=reference_lines,reference_line_attribution_limits=wire_only,
                reference_RTSP_field_counts=dict(rtsp_fields),reference_RTSP_layout_limits=rtsp_limits,
                peer_gap_context_observations=peer_context,streams=details,closure_diagnostics=closures,outstanding_RTSP_requests=len(rtsp_pending),wire_or_domain_normalization=False)


if __name__=='__main__':
    ap=argparse.ArgumentParser(description=__doc__)
    for name in ['capture','events','oracle','out']:ap.add_argument('--'+name,type=Path,required=True)
    a=ap.parse_args()
    for p in vars(a).values():assert not any((q/'.git').exists()for q in[p.resolve(),*p.resolve().parents])
    result=audit(a.capture,a.events,a.oracle)
    a.out.write_text(json.dumps(result,indent=2)+'\n',encoding='utf8')
    print(json.dumps({k:v for k,v in result.items()if k in ['checks','physical_frames','wire_bound_record_counts','reference_IMAP_line_fields','outstanding_RTSP_requests']}))
