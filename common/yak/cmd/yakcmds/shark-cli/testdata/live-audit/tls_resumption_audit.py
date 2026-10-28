"""Certify owned full/resumed/fresh TLS records, peers, fields and TCP reuse.

Independently split literal captured TCP bytes by observed SYN generations.
Compare every TLS record byte and source packet reference, negotiated context,
all HTTP plaintext, native/WS flow ownership and true endpoint reuse receipts.
Missing native resumption fields are reported as an explicit feature gap,
never replaced by a label-only parity claim. This is not binder/certificate
trust verification, 0-RTT coverage or proof of every TLS recovery boundary.
"""
import argparse,base64,collections,csv,hashlib,json
from pathlib import Path
from capture_clock_wire_audit import physical,ip
from owned_tls_clock_audit import raw_records
from owned_quic_clock_audit import vals,number,aliases


def sha(data):return hashlib.sha256(data).hexdigest()
def rows(path):return[json.loads(line)for line in path.open(encoding='utf8')]


def hello(body,client):
    assert len(body)>=35;session_size=body[34];at=35+session_size;assert session_size<=32 and at<=len(body)
    result=dict(random=body[2:34],session_id=body[35:at],legacy_version=int.from_bytes(body[:2],'big'))
    if client:
        size=int.from_bytes(body[at:at+2],'big');at+=2;assert size>=2 and size%2==0 and at+size<len(body);at+=size
        size=body[at];at+=1;assert size and at+size<=len(body);at+=size
    else:
        assert at+3<=len(body);result['cipher']=int.from_bytes(body[at:at+2],'big');at+=3
    extensions={}
    if at<len(body):
        size=int.from_bytes(body[at:at+2],'big');at+=2;assert at+size==len(body)
        while at<len(body):
            assert at+4<=len(body);kind=int.from_bytes(body[at:at+2],'big');size=int.from_bytes(body[at+2:at+4],'big');at+=4
            assert kind not in extensions and at+size<=len(body);extensions[kind]=body[at:at+size];at+=size
    result['extensions']=extensions;return result


def wire_connections(capture,peer):
    records=physical(capture);profiles={p['server_port']:p for p in peer['profiles']};current={};generations=collections.Counter();connections={};frame_owner={};cells=collections.defaultdict(dict)
    for n,r in enumerate(records,1):
        _,header,payload=ip(r)
        if header[9]!=6:assert header[9]==17;continue
        assert len(payload)>=20 and header[12:20]==bytes.fromhex('7f0000017f000001')
        src,dst=int.from_bytes(payload[:2],'big'),int.from_bytes(payload[2:4],'big');server=dst if dst in profiles else src;assert server in profiles
        profile=profiles[server];client=src if dst==server else dst;assert client==profile['reused_client_port'];direction=int(src==server)
        flags=payload[13];seq=int.from_bytes(payload[4:8],'big');key=(client,server)
        if flags&2 and not flags&16:
            assert direction==0;generation=generations[server];generations[server]+=1;assert generation<4
            cid=len(connections)+1;current[key]=cid;connections[cid]=dict(profile=profile['profile'],generation=generation,server_port=server,client_port=client,SYN_frame=n,SYN_sequence=seq,frames=[],reset_frames=[],SYN_ACK_frames=[])
        assert key in current;cid=current[key];connection=connections[cid];connection['frames'].append(n);frame_owner[n]=(cid,direction)
        if flags&4:connection['reset_frames'].append(n)
        if flags&2 and flags&16:connection['SYN_ACK_frames'].append(n)
        start=(payload[12]>>4)*4;assert 20<=start<=len(payload);seq+=bool(flags&2)
        for offset,byte in enumerate(payload[start:],seq):
            cell=cells[(cid,direction)].setdefault(offset,[byte,set()]);assert cell[0]==byte;cell[1].add(n)
    assert len(connections)==8 and set(generations.values())=={4}
    assert len({v['SYN_sequence']for v in connections.values()})==8
    for c in connections.values():assert c['reset_frames']and len(c['SYN_ACK_frames'])==1
    tls=collections.defaultdict(collections.deque);hellos={};total=0
    for key,stream in cells.items():
        lo,hi=min(stream),max(stream)+1;assert len(stream)==hi-lo,'complete captured TCP extent in this owned baseline'
        wire=bytes(stream[i][0]for i in range(lo,hi));at=0;index=0;after_change_cipher_spec=False
        while at<len(wire):
            assert at+5<=len(wire)and 20<=wire[at]<=24 and wire[at+1]==3
            end=at+5+int.from_bytes(wire[at+3:at+5],'big');assert end<=len(wire);record=wire[at:end]
            refs=sorted(set().union(*(stream[i][1]for i in range(lo+at,lo+end))))
            tls[(key,sha(record))].append(dict(refs=refs,index=index,wire=record,offset=at));total+=len(record)
            if record[0]==20:after_change_cipher_spec=True
            if record[0]==22 and not after_change_cipher_spec:
                hand=5
                while hand<len(record):
                    assert hand+4<=len(record);size=int.from_bytes(record[hand+1:hand+4],'big');finish=hand+4+size;assert finish<=len(record)
                    if record[hand]in[1,2]:
                        kind=record[hand];assert(key[0],kind)not in hellos;hellos[key[0],kind]=hello(record[hand+4:finish],kind==1)
                    hand=finish
            at=end;index+=1
    return records,connections,frame_owner,tls,hellos,total


def audit(capture,events,cli,flows,manifest,oracle,labels):
    peer=json.loads(manifest.read_text('utf8'));assert not peer['failures']and len(peer['peer_receipts'])==16 and len(peer['operations'])==24
    assert peer['capture_SHA256']==sha(capture.read_bytes())and peer['early_data_claim']is False and peer['network_fault_injection']is False
    physical_records,connections,owners,wire,hellos,total=wire_connections(capture,peer)
    native=rows(events);by_id={e['id']:e for e in native};assert len(by_id)==len(native)
    physical_to_flow={};flowdoc=json.loads(flows.read_text('utf8'));assert not flowdoc['error']and len(flowdoc['flows'])==len(flowdoc['closed'])==8
    assert flowdoc['capture_SHA256']==sha(capture.read_bytes())and all(not f['half_open']for f in flowdoc['flows'])
    assert all(c['reason']=='rst'and c['pending_gaps']is False for c in flowdoc['closed'])
    for p in flowdoc['packets']:
        assert type(p['flow'])is int and type(p['direction'])is int and p['direction']in[0,1]
        assert len(p['refs'])==1;ref=p['refs'][0];assert ref['Domain']==dict(Section=0,Interface=0,Encapsulation='')
        assert type(ref['Number'])is int and all(type(ref['Domain'][k])is int for k in['Section','Interface'])
        n=ref['Number'];assert n not in physical_to_flow;physical_to_flow[n]=(p['flow'],p['direction'])
    assert physical_to_flow==owners,'all captured TCP packets and directions bind literal SYN generations'
    for cid,c in connections.items():
        receipts=[r for r in peer['peer_receipts']if r['profile']==c['profile']and r['generation']==c['generation']];assert len(receipts)==2 and{r['peer']for r in receipts}=={'client','server'}
        expected_resumed=c['generation']in[1,2]
        for receipt in receipts:
            assert receipt['session_reused']is expected_resumed and receipt['ALPN']=='http/1.1'
            assert receipt['version']==('TLSv1.2'if c['profile']=='tls12'else'TLSv1.3')
            assert receipt['cipher'][0]==('ECDHE-ECDSA-AES128-GCM-SHA256'if c['profile']=='tls12'else'TLS_AES_256_GCM_SHA384')
        client,server=hellos[cid,1],hellos[cid,2];c['ClientRandom_SHA256']=sha(client['random']);c['ServerRandom_SHA256']=sha(server['random'])
        assert server['cipher']==(0xc02b if c['profile']=='tls12'else 0x1302)
        if c['profile']=='tls13':
            assert server['extensions'][43]==b'\x03\x04'
            assert(41 in server['extensions'])is expected_resumed
            if expected_resumed:assert server['extensions'][41]==b'\0\0'and 41 in client['extensions']
        else:
            assert server['legacy_version']==0x303
            if expected_resumed:assert client['session_id']and client['session_id']==server['session_id']and len(client['extensions'][35])>0
        c['peer_session_resumed']=expected_resumed
    assert len({v['ClientRandom_SHA256']for v in connections.values()})==8 and len({v['ServerRandom_SHA256']for v in connections.values()})==8
    expected_records=collections.Counter({key:len(value)for key,value in wire.items()});seen=collections.Counter();http=collections.Counter();request_events={};response_pairs=[];native_handshakes=collections.Counter();negotiated_fields=0;native_feature_keys=set()
    ops={(o['profile'],o['generation'],o['index']):o for o in peer['operations']}
    for e in native:
        if e['protocol']not in['tls','http']:assert e['transport']=='udp'and e['status']=='unrecognized';continue
        assert type(e['id'])is int and e['id']>0 and type(e['tcp_flow_id'])is int and type(e['direction'])is int and e['direction']in[0,1]
        assert e['status']=='decoded'and not e.get('error')
        cid=e['tcp_flow_id'];c=connections[cid];direction=e['direction'];refs=e['source_bytes']['PacketRefs'];assert refs and all(owners[r['Number']]==(cid,direction)and r['Domain']==e['domain']for r in refs)
        assert all(type(r['Number'])is int and all(type(r['Domain'][k])is int for k in['Section','Interface'])for r in refs)
        assert all(type(e['domain'][k])is int for k in['Section','Interface'])and type(e['domain']['Encapsulation'])is str
        assert e['domain']==dict(Section=0,Interface=0,Encapsulation='');raw=base64.b64decode(e['raw'],validate=True);assert e['source_bytes']['SHA256']==sha(raw)
        fields=aliases(e);assert fields['tcp.flow.id']==[cid]and fields['pdu.length']==[len(raw)]
        if e['protocol']=='tls':
            key=((cid,direction),sha(raw));assert wire[key];record=wire[key].popleft();assert sorted(r['Number']for r in refs)==record['refs']and raw==record['wire'];seen[key]+=1
            for field,value in[('tls.record.wire_type',raw[0]),('tls.record.version',int.from_bytes(raw[1:3],'big')),('tls.record.length',len(raw)-5)]:assert fields[field]==[value]
            s=e['session'];native_feature_keys.update(s);assert s['Certificate Trust']=='not-evaluated'
            for m in(s.get('Handshake Messages')or[]):native_handshakes[cid,direction,m['Type']]+=1
            if s.get('Negotiated Version'):
                assert s['Negotiated Version']==(0x303 if c['profile']=='tls12'else 0x304)and s['Cipher Suite']==(0xc02b if c['profile']=='tls12'else 0x1302);negotiated_fields+=2
            if s.get('ALPN'):assert s['ALPN']=='http/1.1';negotiated_fields+=1
            if s.get('Authentication Verified')is True:assert s['Secret Lookup Status']=='found'and s['Content Visibility']=='authenticated-plaintext'
        else:
            assert e['source_bytes']['Kind']=='decrypted'and e['source_bytes']['ParentPDUs']
            for pdu in e['source_bytes']['ParentPDUs']:
                parent=by_id[pdu];assert parent['protocol']=='tls'and parent['tcp_flow_id']==cid and parent['direction']==direction and parent['session']['Authentication Verified']is True
            head,body=raw.split(b'\r\n\r\n',1)
            if direction==0:
                first=head.split(b'\r\n',1)[0].split(b' ');assert first[0]==b'GET';uri=first[1].decode();parts=uri.split('/');assert parts[1]==c['profile']and int(parts[2])==c['generation'];index=int(parts[3]);op=ops[c['profile'],c['generation'],index]
                assert sha(raw)==op['request_SHA256']and not body and fields['http.request.method']==['GET']
                detail=e['fields']['Message']['HTTP Request'];assert detail['FirstLine']==dict(Method='GET',Path=uri,Version='HTTP/1.1')
                assert detail['Headers']==head.decode('ascii').split('\r\n')[1:]+['']
                request_events[cid,index]=e['id'];http[cid,0,index]+=1
            else:
                assert head.startswith(b'HTTP/1.1 200 OK\r\n')and fields['http.response.code']==[200]
                matches=[(key,op)for key,op in ops.items()if key[:2]==(c['profile'],c['generation'])and op['response_SHA256']==sha(raw)];assert len(matches)==1
                key,op=matches[0];index=key[2];assert len(body)==op['body_bytes']and sha(body)==op['body_SHA256']
                detail=e['fields']['Message']['HTTP Response'];assert detail['FirstLine']==dict(Message='OK',Status='200',Version='HTTP/1.1')
                assert detail['Headers']==head.decode('ascii').split('\r\n')[1:]+['']and detail['Body']['Octets'].encode('ascii')==body
                http[cid,1,index]+=1;response_pairs.append((cid,index,e['response_to']))
    assert seen==expected_records and all(not v for v in wire.values())
    assert http==collections.Counter({(cid,direction,index):1 for cid in connections for direction in[0,1]for index in range(3)})
    assert all(request_events[cid,index]==request for cid,index,request in response_pairs),'responses cannot attach to a previous tuple generation'
    reference=json.loads(oracle.read_text('utf8'));assert len(reference)==len(physical_records)
    reference_records=collections.Counter();reference_handshakes=collections.Counter();reference_http=collections.Counter();reference_resumed={};reference_streams={};reference_fields=0
    for n,entry in enumerate(reference,1):
        layers=entry['_source']['layers'];assert type(layers['frame']['frame.number'])is str and number(layers['frame']['frame.number'])==n
        assert bytes.fromhex(layers['frame_raw'][0])==physical_records[n-1]['wire']and layers['frame_raw'][1:]==[0,len(physical_records[n-1]['wire']),0,1,0]
        if n not in owners:continue
        cid,direction=owners[n];c=connections[cid];tcp=layers['tcp'];assert type(tcp['tcp.stream'])is str and number(tcp['tcp.stream'])==cid-1;reference_streams[cid]=number(tcp['tcp.stream'])
        for raw in raw_records(layers):reference_records[((cid,direction),sha(raw))]+=1
        types=list(map(number,vals(layers,'tls.handshake.type')))
        for kind in types:reference_handshakes[cid,direction,kind]+=1
        if 2 in types:
            assert list(map(number,vals(layers,'tls.handshake.ciphersuite')))==[0xc02b if c['profile']=='tls12'else 0x1302];reference_fields+=1
            if c['profile']=='tls12':reference_resumed[cid]=bool(vals(layers,'tls.resumed'))
            else:reference_resumed[cid]=vals(layers,'tls.handshake.extensions.psk.identity.selected')==['0']
        for alpn in vals(layers,'tls.handshake.extensions_alpn_str'):assert alpn=='http/1.1';reference_fields+=1
        methods=vals(layers,'http.request.method');uris=vals(layers,'http.request.uri')
        if methods:
            assert len(methods)==len(uris)
            for method,uri in zip(methods,uris):
                assert direction==0 and method=='GET';parts=uri.split('/');assert parts[1]==c['profile']and int(parts[2])==c['generation'];reference_http[cid,0,int(parts[3])]+=1
        bodies=list(raw_records(layers,'http.file_data_raw'));codes=vals(layers,'http.response.code');assert len(codes)==len(bodies)
        for code,body in zip(codes,bodies):
            assert direction==1 and number(code)==200;matches=[k for k,op in ops.items()if k[:2]==(c['profile'],c['generation'])and op['body_SHA256']==sha(body)];assert len(matches)==1;reference_http[cid,1,matches[0][2]]+=1
    assert reference_records==expected_records and reference_handshakes==native_handshakes and reference_http==http
    assert reference_resumed=={cid:c['peer_session_resumed']for cid,c in connections.items()}
    cli_rows=rows(cli);assert len(cli_rows)==len(physical_records)
    with labels.open(encoding='utf8')as f:literal=list(csv.DictReader(f,delimiter='\t'))
    differences=[]
    for n,(row,other)in enumerate(zip(cli_rows,literal),1):
        assert type(row['number'])is int and row['number']==int(other['frame.number'])==n
        assert row['captured_length']==row['length']==len(physical_records[n-1]['wire'])
        if n in owners:
            cid,direction=owners[n];c=connections[cid]
            if row['protocol']=='HTTP/1.1':assert row['protocol_stack']==['Loopback','IPv4','TCP','TLSv1.2'if c['profile']=='tls12'else'TLSv1.3','HTTP/1.1']
            if row['protocol']=='TLS':
                assert other['tls.handshake.type']=='1'and row['protocol_stack']==['Loopback','IPv4','TCP','TLS'];category='ClientHello offered version versus native not-yet-negotiated identity'
            elif row['protocol']=='HTTP/1.1'and other['_ws.col.protocol']=='HTTP':category='HTTP version-specific native display versus generic reference column'
            else:assert row['protocol']==other['_ws.col.protocol'];continue
            differences.append(dict(frame=n,native=row['protocol'],reference=other['_ws.col.protocol'],category=category))
        else:assert row['protocol']==other['_ws.col.protocol']=='UDP'
    missing=not any('resum'in k.casefold()or'psk'in k.casefold()or'ticket'in k.casefold()for k in native_feature_keys)
    return dict(scope=__doc__,capture_SHA256=sha(capture.read_bytes()),SDK_SHA256=sha(events.read_bytes()),CLI_SHA256=sha(cli.read_bytes()),flows_SHA256=sha(flows.read_bytes()),manifest_SHA256=sha(manifest.read_bytes()),reference_SHA256=sha(oracle.read_bytes()),labels_SHA256=sha(labels.read_bytes()),auditor_SHA256=sha(Path(__file__).read_bytes()),physical_packets=len(physical_records),TCP_packets=len(owners),native_TCP_generations=len(connections),real_resumed_connections=sum(c['peer_session_resumed']for c in connections.values()),complete_literal_TLS_records=sum(seen.values()),complete_literal_TLS_bytes=total,source_packet_references_exact=True,native_reference_handshake_messages=sum(native_handshakes.values()),native_negotiated_context_field_checks=negotiated_fields,reference_selected_cipher_ALPN_checks=reference_fields,HTTP_requests=24,HTTP_responses=24,HTTP_raw_messages_and_source_TLS_authentication_equal_peer_receipts=True,HTTP_reference_fields_and_bodies_equal_peer_receipts=True,response_to_current_generation_requests_only=True,all_reference_frame_bytes_and_TCP_generation_ids_exact=True,resumption_metadata_missing_from_native=missing,missing_resumption_field_scope='Native TLS decrypts real resumed connections but exposes no ticket/PSK/resumed handshake fields'if missing else'Native now contains resumption metadata; rerun a dedicated field-level verifier',literal_label_differences=differences,connections=connections,full_TLS_feature_parity_claim=False,early_data_or_0RTT_claim=False)


if __name__=='__main__':
    ap=argparse.ArgumentParser(description=__doc__)
    for name in['capture','events','cli','flows','manifest','oracle','labels','out']:ap.add_argument('--'+name,type=Path,required=True)
    a=ap.parse_args()
    for path in vars(a).values():assert not any((p/'.git').exists()for p in[path.resolve(),*path.resolve().parents])
    proof=audit(a.capture,a.events,a.cli,a.flows,a.manifest,a.oracle,a.labels)
    with a.out.open('x',encoding='utf8')as out:json.dump(proof,out,indent=2);out.write('\n')
    print(json.dumps({k:v for k,v in proof.items()if k not in['literal_label_differences','connections']}),flush=True)
