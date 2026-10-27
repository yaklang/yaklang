"""Certify declared six-suite TLS captures against wire, peer receipts and WS.

The owned profiles use distinct TCP endpoint pairs and complete IPv4 captures.
All TLS record bytes and contributing frame references must match independently
reconstructed TCP bytes. HTTP plaintext must equal the independent OpenSSL peer
transcript; available reference wire records and HTTP bodies are compared too.
This does not certify every TLS version, extension, resumption or TCP boundary.
"""
import argparse,base64,collections,hashlib,json
from pathlib import Path
from capture_clock_wire_audit import physical,ip
from owned_quic_clock_audit import vals,many,aliases,number


def sha(w):return hashlib.sha256(w).hexdigest()
def tcp_bytes(capture):
    pending={};streams=collections.defaultdict(dict)
    for n,record in enumerate(physical(capture),1):
        _,header,payload=ip(record)
        if header[9]!=6:continue
        domain=(record['section'],record['interface']);flags=int.from_bytes(header[6:8],'big');refs={n}
        if flags&0x3fff:
            key=(domain,header[12:20],header[4:6]);parts=pending.setdefault(key,[])
            parts.append(((flags&0x1fff)*8,payload,n,bool(flags&0x2000)));parts.sort()
            if parts[-1][3]or parts[0][0]!=0 or any(a[0]+len(a[1])!=b[0]for a,b in zip(parts,parts[1:])):continue
            payload=b''.join(p[1]for p in parts);refs={p[2]for p in parts};del pending[key]
        assert len(payload)>=20
        start=(payload[12]>>4)*4;assert 20<=start<=len(payload)
        key=(domain,int.from_bytes(payload[:2],'big'),int.from_bytes(payload[2:4],'big'))
        sequence=int.from_bytes(payload[4:8],'big')+bool(payload[13]&2)
        for index,byte in enumerate(payload[start:],sequence):
            if index in streams[key]:assert streams[key][index][0]==byte,'owned source retransmission must agree'
            else:streams[key][index]=[byte,set()]
            streams[key][index][1].update(refs)
    assert not pending
    return streams


def raw_records(tree,key='tls.record_raw'):
    if isinstance(tree,dict):
        for name,value in tree.items():
            if name==key:
                for v in([value]if value and isinstance(value[0],str)else value):yield bytes.fromhex(v[0])
            elif not name.endswith('_raw'):yield from raw_records(value,key)
    elif isinstance(tree,list):
        for v in tree:yield from raw_records(v,key)


def audit(capture,events,manifest,oracle):
    peer=json.loads(manifest.read_text('utf8'));assert not peer['errors']and len(peer['ports'])==6 and len(peer['operations'])==18
    streams=tcp_bytes(capture);wire=collections.Counter();record_refs={};total=0
    for key,cells in streams.items():
        if not cells:continue
        lo,hi=min(cells),max(cells)+1;assert len(cells)==hi-lo,'complete owned TCP byte extent'
        body=bytes(cells[i][0]for i in range(lo,hi));at=0
        while at<len(body):
            assert len(body)-at>=5 and 20<=body[at]<=24 and body[at+1]==3
            end=at+5+int.from_bytes(body[at+3:at+5],'big');assert end<=len(body)
            raw=body[at:end];identity=(key,sha(raw));wire[identity]+=1
            refs=sorted(set().union(*(cells[i][1]for i in range(lo+at,lo+end))))
            record_refs.setdefault(identity,collections.deque()).append(refs);total+=len(raw);at=end
    expected={};expected_requests={};expected_responses={}
    profiles=['tls13-aes128','tls13-aes256','tls13-chacha','tls12-aes128','tls12-aes256','tls12-chacha']
    port_profile=dict(zip(peer['ports'],profiles))
    for op in peer['operations']:
        profile,index=op['profile'],op['index'];body=(f'{peer["nonce"]}/{profile}/{index}|'.encode()*1000)[:19000+index]
        assert len(body)==op['bytes']and sha(body)==op['body_sha256']
        request=f'GET /{profile}/{index} HTTP/1.1\r\nHost: audit.invalid\r\n\r\n'.encode()
        response=f'HTTP/1.1 200 OK\r\nContent-Length: {len(body)}\r\nContent-Type: application/octet-stream\r\n\r\n'.encode()+body
        for direction,w in[(0,request),(1,response)]:expected[(profile,direction,sha(w))]=1
        expected_requests[('/'+profile+'/'+str(index),'GET')]=1;expected_responses[sha(body)]=1
    native=[json.loads(line)for line in events.open(encoding='utf8')];by_id={e['id']:e for e in native};seen=collections.Counter();http=collections.Counter();headers=collections.Counter();connections=collections.defaultdict(set)
    for e in native:
        if e['protocol']not in['tls','http']:continue
        assert e['status']=='decoded','all records in these complete owned TLS profiles must decode'
        raw=base64.b64decode(e['raw'],validate=True);assert e['source_bytes']['SHA256']==sha(raw)
        domain=(e['domain']['Section'],e['domain']['Interface']);assert not e['domain'].get('Encapsulation')
        src,dst=int(e['source'].rsplit(':',1)[1]),int(e['destination'].rsplit(':',1)[1]);fields=aliases(e)
        assert fields['pdu.length']==[len(raw)]
        if e['protocol']=='tls':
            key=((domain,src,dst),sha(raw));assert record_refs.get(key),'literal source TCP record occurrence'
            refs=record_refs[key].popleft();assert sorted(r['Number']for r in e['source_bytes']['PacketRefs'])==refs
            assert all(r['Domain']==e['domain']for r in e['source_bytes']['PacketRefs'])
            assert len(raw)==5+int.from_bytes(raw[3:5],'big')
            for name,value in[('tls.record.wire_type',raw[0]),('tls.record.version',int.from_bytes(raw[1:3],'big')),('tls.record.length',len(raw)-5)]:
                assert fields[name]==[value];headers[name]+=1
            seen[key]+=1;connections[e['tcp_flow_id']].add((src,dst)if e['direction']==0 else(dst,src))
        else:
            server=dst if e['direction']==0 else src;assert server in port_profile
            assert e['source_bytes']['Kind']=='decrypted'and e['source_bytes']['ParentPDUs']
            for parent in e['source_bytes']['ParentPDUs']:
                p=by_id[parent];assert p['protocol']=='tls'and p['tcp_flow_id']==e['tcp_flow_id']and p['session']['Authentication Verified']is True
            head,body=raw.split(b'\r\n\r\n',1);lines=head.split(b'\r\n');first=lines[0].split(b' ')
            if e['direction']==0:assert first[0]==b'GET'and not body and fields['http.request.method']==['GET']
            else:
                assert first[:2]==[b'HTTP/1.1',b'200']and fields['http.response.code']==[200]
                assert int(next(l.split(b':',1)[1]for l in lines if l.lower().startswith(b'content-length:')))==len(body)
            http[(port_profile[server],e['direction'],sha(raw))]+=1
    assert seen==wire and http==collections.Counter(expected)
    assert len(connections)==6 and all(len(v)==1 for v in connections.values())
    reference=collections.Counter();requests=collections.Counter();responses=collections.Counter();reference_headers=collections.Counter()
    for row in json.loads(oracle.read_text('utf8')):
        layers=row['_source']['layers']
        if'tcp'not in layers:continue
        src,dst=number(layers['tcp']['tcp.srcport']),number(layers['tcp']['tcp.dstport'])
        for raw in raw_records(layers):
            assert len(raw)>=5 and len(raw)==5+int.from_bytes(raw[3:5],'big')
            reference[((0,0),src,dst),sha(raw)]+=1
        methods=vals(layers,'http.request.method');uris=vals(layers,'http.request.uri')
        if methods:
            assert len(methods)==len(uris)
            requests.update(zip(uris,methods))
        codes=vals(layers,'http.response.code');data=list(raw_records(layers,'http.file_data_raw'))
        if codes:
            assert all(number(c)==200 for c in codes)and len(codes)==len(data)
            for v in data:responses[sha(v)]+=1
    # Interface identity differs in the declared PCAPNG fragment controls;
    # the literal reference ports and record bytes must still be identical.
    native_reference=collections.Counter()
    for(key,digest),count in wire.items():native_reference[(((0,0),key[1],key[2]),digest)]+=count
    assert reference==native_reference,'every complete literal reference TLS record occurrence'
    assert requests==collections.Counter(expected_requests)and responses==collections.Counter(expected_responses),'reference HTTP fields and response bodies equal actual endpoint receipts'
    return dict(scope=__doc__,capture_SHA256=sha(capture.read_bytes()),SDK_SHA256=sha(events.read_bytes()),manifest_SHA256=sha(manifest.read_bytes()),reference_SHA256=sha(oracle.read_bytes()),auditor_SHA256=sha(Path(__file__).read_bytes()),native_TCP_connections=len(connections),wire_TLS_records=sum(wire.values()),wire_TLS_bytes=total,source_frame_references_exact=True,literal_reference_TLS_records=sum(reference.values()),native_header_checks=dict(headers),HTTP_requests=sum(requests.values()),HTTP_responses=sum(responses.values()),all_HTTP_plaintext_bytes_equal_peer_transcript=True,all_reference_response_bodies_and_request_fields_equal_peer_receipts=True,full_TLS_feature_parity_claim=False)


if __name__=='__main__':
    ap=argparse.ArgumentParser(description=__doc__)
    for n in['capture','events','manifest','oracle','out']:ap.add_argument('--'+n,type=Path,required=True)
    a=ap.parse_args()
    for path in vars(a).values():assert not any((p/'.git').exists()for p in[path.resolve(),*path.resolve().parents])
    proof=audit(a.capture,a.events,a.manifest,a.oracle)
    with a.out.open('x',encoding='utf8',newline='\n')as out:json.dump(proof,out,indent=2);out.write('\n')
    print(json.dumps(proof),flush=True)
