"""Derive declared clock/fragment controls from actual socket captures.

Only this offline producer changes IP envelopes or adds capture-clock offsets.
Original transport bytes, source records and every output transformation are
hashed in an external ledger. These are not claims of live network impairment.
Usage: capture_clock_controls.py TLS_DIR QUIC_DIR COAP_DIR OUTPUT_DIR
"""
import hashlib,json,struct,sys
from pathlib import Path


def sha(value):return hashlib.sha256(value).hexdigest()


def records(path):
    wire=path.read_bytes();magic=wire[:4]
    assert magic in[bytes.fromhex(v)for v in['d4c3b2a1','4d3cb2a1']],'little-endian classic input PCAP'
    assert len(wire)>=24 and struct.unpack_from('<HH',wire,4)==(2,4)
    link=struct.unpack_from('<I',wire,20)[0];scale=1000 if magic==bytes.fromhex('d4c3b2a1')else 1
    at=24;answer=[]
    while at<len(wire):
        assert at+16<=len(wire)
        sec,sub,count,original=struct.unpack_from('<IIII',wire,at);at+=16
        assert count==original and at+count<=len(wire),'full original capture record'
        answer.append(dict(number=len(answer)+1,time_ns=sec*1000000000+sub*scale,wire=wire[at:at+count],link=link));at+=count
    assert answer and at==len(wire)
    return answer


def ipv4(record):
    wire=record['wire'];link=record['link']
    if link==0:
        if len(wire)<4 or int.from_bytes(wire[:4],'little')!=2:return None
        at=4
    elif link==1:
        if len(wire)<14 or wire[12:14]!=b'\x08\0':return None
        at=14
    else:raise AssertionError(('unselected input link',link))
    if at+20>len(wire)or wire[at]>>4!=4:return None
    size=(wire[at]&15)*4;end=at+int.from_bytes(wire[at+2:at+4],'big')
    assert size>=20 and at+size<=end==len(wire)
    if int.from_bytes(wire[at+6:at+8],'big')&0x3fff:return None
    protocol=wire[at+9];payload=wire[at+size:end]
    if protocol not in[6,17]or len(payload)<(20 if protocol==6 else 8):return None
    source,destination=struct.unpack_from('>HH',payload)
    header=(payload[12]>>4)*4 if protocol==6 else 8
    assert header>= (20 if protocol==6 else 8)and header<=len(payload)
    return dict(at=at,header=size,protocol=protocol,source=source,destination=destination,payload=payload,data=payload[header:])


def fragments(record,cut,identifier=None):
    ip=ipv4(record);assert ip and ip['header']==20 and 0<cut<len(ip['payload'])and cut%8==0
    result=[]
    for start,end,more in[(0,cut,True),(cut,len(ip['payload']),False)]:
        head=bytearray(record['wire'][ip['at']:ip['at']+20]);payload=ip['payload'][start:end]
        head[2:4]=(20+len(payload)).to_bytes(2,'big');head[6:8]=((0x2000 if more else 0)|(start//8)).to_bytes(2,'big')
        if identifier is not None:head[4:6]=identifier.to_bytes(2,'big')
        head[10:12]=b'\0\0';total=sum(struct.unpack('>10H',head))
        while total>>16:total=(total&65535)+(total>>16)
        head[10:12]=((~total)&65535).to_bytes(2,'big')
        result.append((record['wire'][:ip['at']]+head+payload,start,end))
    return result


def block(kind,payload):
    payload+=b'\0'*((-len(payload))%4);size=len(payload)+12
    return struct.pack('<II',kind,size)+payload+struct.pack('<I',size)


def write_case(dest,name,primary,foreign,items,description):
    folder=dest/name;folder.mkdir(exist_ok=False)
    output=block(0x0a0d0d0a,struct.pack('<IHHq',0x1a2b3c4d,1,0,-1))
    for link in[primary[0]['link'],foreign[0]['link']]:
        output+=block(1,struct.pack('<HHI',link,0,65535)+struct.pack('<HH',9,1)+b'\x09\0\0\0'+bytes(4))
    ledger=[]
    for domain,record,wire,stamp,change in items:
        output+=block(6,struct.pack('<IIIII',domain,stamp>>32,stamp&0xffffffff,len(wire),len(wire))+wire)
        ledger.append(dict(number=len(ledger)+1,interface=domain,time_ns=stamp,wire_SHA256=sha(wire),source='primary'if domain==0 else'CoAP',source_frame=record['number'],source_wire_SHA256=sha(record['wire']),source_time_ns=record['time_ns'],change=change))
    with(folder/'observation.pcapng').open('xb')as out:out.write(output)
    proof=dict(scope=__doc__,description=description,generator_SHA256=sha(Path(__file__).read_bytes()),capture_SHA256=sha(output),physical_records=len(ledger),ledger=ledger)
    with(folder/'manifest.json').open('x',encoding='utf8',newline='\n')as out:json.dump(proof,out,indent=2);out.write('\n')


def main():
    tls,quic,coap,dest=map(Path,sys.argv[1:])
    assert not any((p/'.git').exists()for path in[tls,quic,coap,dest]for p in[path.resolve(),*path.resolve().parents])
    dest.mkdir(exist_ok=False)
    sources={name:records(folder/'live.pcap')for name,folder in[('TLS',tls),('QUIC',quic),('CoAP',coap)]}
    peers=json.loads((coap/'manifest.json').read_text('utf8'));qpeers=json.loads((quic/'manifest.json').read_text('utf8'))
    foreign=next(r for r in sources['CoAP']if(ipv4(r)or{}).get('source')==peers['client'][1])
    assert ipv4(foreign)['protocol']==17
    for protocol in['TLS','QUIC']:
        primary=sources[protocol]
        selected=next(r for r in primary if (p:=ipv4(r))and len(p['payload'])>128 and(p['protocol']==6 and p['data'][:1]==b'\x16'if protocol=='TLS'else p['protocol']==17 and p['source']==qpeers['client_port']and p['data'][0]&0xf0==0xc0))
        for interfere in[False,True]:
            items=[]
            for record in primary:
                if record is not selected:items.append((0,record,record['wire'],record['time_ns'],dict(kind='exact')));continue
                pieces=fragments(record,64)
                future=record['time_ns']+3600*1000000000
                other=fragments(foreign,16,0x6a09)
                for i,(wire,start,end)in enumerate(pieces):
                    items.append((0,record,wire,record['time_ns']+i,dict(kind='IPv4-fragment',IP_payload_interval=[start,end],offset=i)))
                    if interfere:
                        wire,start,end=other[i]
                        items.append((1,foreign,wire,future+i,dict(kind='IPv4-fragment',IP_payload_interval=[start,end],identifier=0x6a09,foreign_clock_offset_ns=future+i-foreign['time_ns'])))
            name=protocol+'-fragment-'+('foreign-clock'if interfere else'local')
            write_case(dest,name,primary,sources['CoAP'],items,'Fragment original protected transport bytes; '+('interleave an independent CoAP fragment clock one hour ahead.'if interfere else'no foreign clock.'))
    primary=sources['QUIC']
    server_cids=[]
    for r in primary:
        p=ipv4(r)
        if not p or p['protocol']!=17 or p['source']!=qpeers['server_port']:continue
        data=p['data']
        if len(data)<7 or data[0]&0xf0!=0xc0:continue
        at=6+data[5]
        if at>=len(data):continue
        cid=data[at+1:at+1+data[at]]
        if cid and cid not in server_cids:server_cids.append(cid)
    assert len(server_cids)==3
    # The readiness marker can itself have the short-header fixed bit. Select
    # actual protected peer packets by an independently visible server CID.
    first_short=next(r for r in primary if(p:=ipv4(r))and p['protocol']==17 and p['source']==qpeers['client_port']and p['data'][:1]and p['data'][0]&0xc0==0x40 and p['data'][1:1+len(server_cids[0])]==server_cids[0])
    copied=next(r for r in primary if(p:=ipv4(r))and p['protocol']==17 and p['source']==qpeers['client_port']and p['data'][:1]and p['data'][0]&0xc0==0x40 and p['data'][1:1+len(server_cids[1])]==server_cids[1])
    for mode in['CoAP','unknown-CID']:
        items=[]
        for record in primary:
            items.append((0,record,record['wire'],record['time_ns'],dict(kind='exact')))
            if record is first_short:
                selected=foreign if mode=='CoAP'else copied;future=record['time_ns']+3600*1000000000
                items.append((1 if mode=='CoAP'else 0,selected,selected['wire'],future,dict(kind='record-copy-clock-shift',offset_ns=future-selected['time_ns'])))
        write_case(dest,'QUIC-'+mode+'-foreign-clock',primary,sources['CoAP'],items,'Insert one whole captured '+mode+' datagram with an explicitly shifted capture timestamp; original QUIC records remain exact.')
    source_proof={name:dict(capture_SHA256=sha((folder/'live.pcap').read_bytes()),manifest_SHA256=sha((folder/'manifest.json').read_bytes()),physical_records=len(sources[name]))for name,folder in[('TLS',tls),('QUIC',quic),('CoAP',coap)]}
    with(dest/'sources.json').open('x',encoding='utf8',newline='\n')as out:json.dump(source_proof,out,indent=2);out.write('\n')
    print(json.dumps(dict(derived_cases=6,original_sources=source_proof)),flush=True)


if __name__=='__main__':main()
