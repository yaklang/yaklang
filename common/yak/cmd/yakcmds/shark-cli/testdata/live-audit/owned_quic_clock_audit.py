"""Check owned QUIC bytes, complete peer transcripts and available WS fields.

This covers the declared three owned QUIC profiles, not every QUIC feature.
Socket receipts independently certify complete HTTP3 streams. Missing per-packet
connection metadata may inherit a datagram's unique literal reference connection
only when both packets have the same source-bound wire destination CID.
"""
import argparse,base64,collections,hashlib,json
from pathlib import Path
from capture_clock_wire_audit import physical,ip


def sha(value):return hashlib.sha256(value).hexdigest()
def many(v):return v if isinstance(v,list)else[v]
def vals(v,key):
    if isinstance(v,dict):return[x for k,w in v.items()for x in(many(w)if k==key else vals(w,key))]
    if isinstance(v,list):return[x for w in v for x in vals(w,key)]
    return[]
def number(v):return int(v,0)if isinstance(v,str)else int(v)
def aliases(e):
    out=collections.defaultdict(list)
    for v in e['field_values'].get('values')or[]:out[v['name']].append(v['value'])
    return out


def datagrams(capture):
    pending={};by_ref={};result=[]
    for n,record in enumerate(physical(capture),1):
        _,header,payload=ip(record)
        if header[9]!=17:continue
        domain=(record['section'],record['interface']);flags=int.from_bytes(header[6:8],'big');refs=[n]
        if flags&0x3fff:
            key=(domain,header[12:20],header[4:6]);parts=pending.setdefault(key,[])
            parts.append(((flags&0x1fff)*8,payload,n,bool(flags&0x2000)));parts.sort()
            if parts[-1][3]or any(a[0]+len(a[1])!=b[0]for a,b in zip(parts,parts[1:]))or parts[0][0]!=0:continue
            payload=b''.join(p[1]for p in parts);refs=[p[2]for p in parts];del pending[key]
        assert len(payload)>=8 and int.from_bytes(payload[4:6],'big')==len(payload)
        item=dict(frame=n,refs=refs,domain=domain,source_port=int.from_bytes(payload[:2],'big'),destination_port=int.from_bytes(payload[2:4],'big'),payload=payload[8:])
        result.append(item)
        for ref in refs:by_ref[ref]=item
    assert not pending,'declared owned controls contain complete datagrams'
    return result,by_ref


def varint(w,at):
    assert at<len(w);size=1<<(w[at]>>6);assert at+size<=len(w)
    return int.from_bytes(w[at:at+size],'big')&((1<<(size*8-2))-1),at+size
def prefix(w,at,bits):
    value=w[at]&((1<<bits)-1);at+=1
    if value<(1<<bits)-1:return value,at
    shift=0
    while True:
        assert at<len(w)and shift<=28
        byte=w[at];at+=1;value+=(byte&127)<<shift
        if byte<128:return value,at
        shift+=7
def frames(w):
    at=0;out={}
    while at<len(w):
        start=at;kind,at=varint(w,at);length,at=varint(w,at);end=at+length
        assert end<=len(w);payload=w[at:end];headers=None
        if kind==1:
            assert payload[:2]==b'\0\0','owned literal QPACK block without dynamic dependencies'
            pos=2;headers=[]
            while pos<len(payload):
                assert payload[pos]&0xf8==0x20,'owned uncompressed literal QPACK field name'
                count,pos=prefix(payload,pos,3);name=payload[pos:pos+count].decode('ascii');pos+=count
                assert pos<len(payload)and payload[pos]&128==0
                count,pos=prefix(payload,pos,7);value=payload[pos:pos+count].decode('ascii');pos+=count
                headers.append((name,value))
            assert pos==len(payload)
        out[start]=dict(kind=kind,payload=payload,wire=w[start:end],header=w[start:at],headers=headers);at=end
    return out


def audit(capture,events,manifest,oracle):
    peer=json.loads(manifest.read_text('utf8'));assert peer['capture_ready_observed']is True and len(peer['profiles'])==3 and len(peer['operations'])==12
    packets,by_ref=datagrams(capture);client,server=peer['client_port'],peer['server_port']
    initial=[]
    for p in packets:
        w=p['payload']
        if p['source_port']==client and len(w)>6 and w[0]&0xf0==0xc0:
            # The client can replace its Initial DCID after Server Initial.
            # Its source CID stays fixed in these declared owned profiles.
            at=6+w[5];assert at<len(w)
            cid=w[at+1:at+1+w[at]]
            if cid not in initial:initial.append(cid)
    assert len(initial)==3
    reference_packets={};reference_streams=collections.Counter();reference_cells=collections.defaultdict(dict)
    epoch_connection={};coalesced_associations=[]
    for row in json.loads(oracle.read_text('utf8')):
        layers=row['_source']['layers'];no=number(layers['frame']['frame.number']);offset=0
        qs=[q for q in many(layers.get('quic',[]))if isinstance(q,dict)and'quic.packet_length'in q]
        literal_connections={str(c)for q in qs for c in vals(q,'quic.connection.number')}
        owners={}
        for q in qs:
            connections=vals(q,'quic.connection.number');cids=vals(q,'quic.dcid')
            if connections and len(cids)==1:
                assert len(connections)==1
                owners[cids[0]]=str(connections[0])
        for q in qs:
            if not isinstance(q,dict)or 'quic.packet_length'not in q:continue
            length=number(q['quic.packet_length']);signature=(no,offset,length)
            connections=vals(q,'quic.connection.number');cids=vals(q,'quic.dcid')
            connection=str(connections[0])if len(connections)==1 else None
            assert len(connections)<=1
            if connection is None and len(literal_connections)==1 and len(cids)==1 and cids[0]in owners:
                packet=by_ref[no];wire=packet['payload'][offset:offset+length]
                cid=bytes.fromhex(cids[0].replace(':',''));start=6 if wire[0]&128 else 1
                assert cid and wire[start:start+len(cid)]==cid,'coalesced association requires the literal wire CID'
                connection=owners[cids[0]]
                coalesced_associations.append(dict(frame=no,datagram_offset=offset,length=length,connection=connection,destination_CID=cid.hex(),raw_SHA256=sha(wire),rule='unique literal connection in the same UDP datagram; identical destination CID'))
            reference_packets[signature]=(q,connection);offset+=length
            scids=vals(q,'quic.scid');types=vals(q,'quic.long.packet_type')
            if scids and types and number(types[0])==0 and number(layers['udp']['udp.srcport'])==client:
                cid=bytes.fromhex(scids[0].replace(':',''));epoch=initial.index(cid)
                assert connection is not None and(epoch not in epoch_connection or epoch_connection[epoch]==connection)
                epoch_connection[epoch]=connection
            for f in many(q.get('quic.frame',[])):
                if not isinstance(f,dict)or not 8<=number(f.get('quic.frame_type','0'))<=15:continue
                sid=number(f['quic.stream.stream_id'])
                if sid&2:continue
                ports=vals(layers['udp'],'udp.srcport'),vals(layers['udp'],'udp.dstport');assert len(ports[0])==len(ports[1])==1
                start=number(f.get('quic.stream.offset','0'));raw=f.get('quic.stream_data_raw');body=bytes.fromhex(raw[0])if raw else b''
                declared=number(f['quic.stream.length'])if 'quic.stream.length'in f else len(body)
                assert declared==len(body),'reference must expose the literal STREAM bytes for this profile'
                assert connection is not None,'every reference STREAM needs a literal or wire-verified coalesced owner'
                signature=(no,connection,number(ports[0][0]),number(ports[1][0]),sid,start,len(body),sha(body),bool(number(f['quic.frame_type'])&1))
                reference_streams[signature]+=1
                key=(connection,signature[2],signature[3],sid)
                for index,byte in enumerate(body,start):
                    assert index not in reference_cells[key]or reference_cells[key][index]==byte
                    reference_cells[key][index]=byte
    assert set(epoch_connection)=={0,1,2}and len(set(epoch_connection.values()))==3
    native=[json.loads(l)for l in events.open(encoding='utf8')];flow_epoch={}
    for e in native:
        w=base64.b64decode(e.get('raw')or'')
        if e['protocol']in['quic','http3']and e['source'].endswith(':'+str(client))and len(w)>6 and w[0]&0xf0==0xc0 and(e.get('session')or{}).get('Authentication Verified')is True:
            at=6+w[5];epoch=initial.index(w[at+1:at+1+w[at]])
            if e['flow']in flow_epoch:assert flow_epoch[e['flow']]==epoch
            flow_epoch[e['flow']]=epoch
    assert set(flow_epoch.values())=={0,1,2}and len(flow_epoch)==3
    streams=collections.defaultdict(dict);ends={};native_streams=collections.Counter();headers_checked=collections.Counter();reference_checked=set();padding=[];h3=[];accepted=0
    for e in native:
        if e['protocol']not in['quic','http3']or e['status']not in['decoded','deferred']:continue
        assert e['flow']in flow_epoch
        s=e['session'];refs=e['source_bytes'].get('PacketRefs')or[];assert refs
        p=by_ref[refs[0]['Number']];assert set(r['Number']for r in refs)==set(p['refs'])and all(by_ref[r['Number']]is p for r in refs)
        assert(e['domain']['Section'],e['domain']['Interface'])==p['domain']and e['domain'].get('Encapsulation','')==''
        assert p['source_port']==(client if e['direction']==0 else server)and p['destination_port']==(server if e['direction']==0 else client)
        fields=aliases(e)
        raw=base64.b64decode(e['raw'],validate=True);offset=s['Datagram Offset'];assert type(offset)is int and p['payload'][offset:offset+len(raw)]==raw and fields['pdu.length']==[len(raw)]
        assert all(r['Domain']==e['domain']for r in refs)
        assert e['source_bytes']['SHA256']==sha(raw)
        if e['completeness']=='datagram-padding':
            assert e['admission']=='observed-datagram-padding'and raw and not any(raw)and s['Authentication Verified']is False and 'Frames'not in s
            assert fields['quic.packet.authenticated']==[False]and fields['quic.datagram.padding_length']==[len(raw)]and fields['quic.datagram.padding_offset']==[offset]and not fields['quic.packet_number']
            padding.append(dict(PDU=e['id'],frame=p['frame'],offset=offset,length=len(raw)));continue
        assert s['Authentication Verified']is True and fields['quic.packet.authenticated']==[True]
        accepted+=1;signature=(p['frame'],offset,len(raw));reference=reference_packets.get(signature)
        epoch=flow_epoch[e['flow']]
        if reference is not None:
            q,connection=reference
            assert connection==epoch_connection[epoch],'source Initial CID must bind native flow and literal reference connection'
            reference_checked.add(signature)
            for name in['quic.packet_number','quic.dcid','quic.scid','quic.version','quic.long.packet_type']:
                values=vals(q,name)
                if not values:continue
                assert len(values)==1
                value=values[0]
                expected=value.replace(':','').lower()if name in['quic.dcid','quic.scid']else number(value)
                assert fields[name]==[expected],('literal available reference header field',p['frame'],name,expected,fields[name])
                headers_checked[name]+=1
        for f in s.get('Frames',[]):
            if f.get('Frame Type')!='STREAM'or f['Stream ID']&2:continue
            body=base64.b64decode(f.get('Stream Data')or'');assert len(body)==f['Length']
            sid,start=f['Stream ID'],f['Offset'];key=(epoch,e['direction'],sid)
            native_streams[(p['frame'],epoch_connection[epoch],p['source_port'],p['destination_port'],sid,start,len(body),sha(body),bool(f['FIN']))]+=1
            for index,byte in enumerate(body,start):
                assert index not in streams[key]or streams[key][index]==byte
                streams[key][index]=byte
            if f['FIN']:
                end=start+len(body);assert key not in ends or ends[key]==end;ends[key]=end
            for frame in f.get('HTTP3 Frames',[]):h3.append((key,frame,e['id']))
    expected={};expected_frames={}
    for operation in peer['operations']:
        for direction,name in[(0,'request'),(1,'reply')]:
            w=bytes.fromhex(operation[name+'_hex']);assert sha(w)==operation[name+'_sha256']
            key=(operation['epoch'],direction,operation['stream_id']);expected[key]=w;expected_frames[key]=frames(w)
    assert set(streams)==set(expected)==set(ends)
    for key,w in expected.items():
        assert set(streams[key])==set(range(len(w)))and ends[key]==len(w),'complete native stream extent and FIN match the actual peer receipt'
        assert bytes(streams[key][i]for i in range(len(w)))==w,'complete native bytes match independently received socket bytes'
    seen=collections.Counter();header_fields=0
    for key,frame,pdu in h3:
        start=frame['Stream Offset'];wanted=expected_frames[key][start];seen[(key,start)]+=1
        assert frame['Frame Type Value']==wanted['kind']and frame['Payload Length']==len(wanted['payload'])and frame['Frame Wire Length']==len(wanted['wire'])and frame['Frame SHA256']==sha(wanted['wire'])
        if wanted['kind']==1:
            assert[(v['Name'],v['Value'])for v in frame['Headers']]==wanted['headers'];header_fields+=len(wanted['headers'])
        if wanted['kind']==0:assert base64.b64decode(frame.get('Payload')or'')==wanted['payload']
    assert seen==collections.Counter({(key,start):1 for key,f in expected_frames.items()for start in f}),'every complete peer HTTP3 frame has exactly one native logical delivery'
    missing=reference_streams-native_streams;extra=native_streams-reference_streams
    holes=[]
    for key,cells in reference_cells.items():
        gap=[];start=None
        for i in range(max(cells,default=-1)+1):
            if i not in cells and start is None:start=i
            if i in cells and start is not None:gap.append([start,i]);start=None
        if start is not None:gap.append([start,max(cells)+1])
        if gap:holes.append(dict(reference_stream=list(key),missing_intervals=gap))
    assert not(missing or extra or holes),'declared owned profile must have every literal reference STREAM occurrence and byte interval'
    return dict(scope=__doc__,capture_SHA256=sha(capture.read_bytes()),SDK_SHA256=sha(events.read_bytes()),peer_receipts_SHA256=sha(manifest.read_bytes()),reference_SHA256=sha(oracle.read_bytes()),auditor_SHA256=sha(Path(__file__).read_bytes()),native_connection_to_source_epoch=flow_epoch,source_epoch_to_literal_reference_connection=epoch_connection,coalesced_connection_associations=coalesced_associations,complete_peer_streams=len(expected),complete_peer_bytes=sum(map(len,expected.values())),native_complete_HTTP3_frames=sum(seen.values()),native_literal_header_occurrences=header_fields,source_bound_authenticated_native_packets=accepted,unprotected_padding=padding,available_reference_header_checks=dict(headers_checked),available_reference_STREAM_occurrences=sum(reference_streams.values()),exact_reference_STREAM_occurrences=sum((reference_streams&native_streams).values()),reference_STREAM_views_without_native=[],native_STREAM_views_without_reference=[],reference_stream_holes=[],all_complete_native_streams_and_frames_equal_actual_peer_receipts=True,all_available_matched_reference_header_values_exact=True,all_profile_reference_STREAM_occurrences_and_bytes_exact=True,full_Wireshark_parity_claim=False)


if __name__=='__main__':
    ap=argparse.ArgumentParser(description=__doc__)
    for name in['capture','events','manifest','oracle','out']:ap.add_argument('--'+name,type=Path,required=True)
    a=ap.parse_args()
    for path in vars(a).values():assert not any((p/'.git').exists()for p in[path.resolve(),*path.resolve().parents])
    proof=audit(a.capture,a.events,a.manifest,a.oracle)
    with a.out.open('x',encoding='utf8',newline='\n')as out:json.dump(proof,out,indent=2);out.write('\n')
    print(json.dumps(dict(peer_streams=proof['complete_peer_streams'],HTTP3_frames=proof['native_complete_HTTP3_frames'],all_peer_bytes_exact=True,reference_holes=len(proof['reference_stream_holes']),full_Wireshark_parity_claim=proof['full_Wireshark_parity_claim'])),flush=True)
