"""Read-only source facts for bounded native ESP NAT-T / OSPFv3 ingress leads.

No Go execution or repository mutation. No ciphertext interpretation, SA
identity, AH authentication, peer identity or OSPF adjacency is inferred.
Physical frame numbers are retained from the original, unmodified capture.
"""
import argparse, collections, csv, hashlib, importlib.util, ipaddress, json, struct, sys, zlib
from pathlib import Path

def sha(path):
    h=hashlib.sha256()
    with Path(path).open('rb') as f:
        for b in iter(lambda:f.read(1<<20), b''): h.update(b)
    return h.hexdigest()
def checksum(data):
    if len(data)%2:data+=b'\0'
    total=sum(struct.unpack('!%dH'%(len(data)//2),data))
    while total>>16:total=(total&65535)+(total>>16)
    return (~total)&65535
def uint(w,a,n=2):return int.from_bytes(w[a:a+n],'big')
def addr(w):return str(ipaddress.ip_address(w))
def digest(w):return hashlib.sha256(w).hexdigest()
def fletcher(w):
    a=b=0
    for x in w[2:]:a=(a+x)%255;b=(b+a)%255
    return dict(c0=a,c1=b,valid=a==b==0)

def outer(record):
    w=record['wire'];eth_start=0;frame_end=len(w);link_detail={}
    if record['link']==274:
        if w and w[0]==0x50:eth_start+=1
        while eth_start<len(w) and w[eth_start]==0x55:eth_start+=1
        assert eth_start<=64 and eth_start<len(w) and w[eth_start]==0xd5
        eth_start+=1;frame_end-=4
        assert frame_end-eth_start>=14 and zlib.crc32(w[eth_start:frame_end])==int.from_bytes(w[frame_end:],'little')
        link_detail=dict(MPacket_preamble_range=[0,eth_start],Ethernet_range=[eth_start,frame_end],FCS_range=[frame_end,len(w)],FCS_verified=True)
    else:assert record['link']==1
    assert frame_end>=eth_start+14
    start=eth_start+14;typ=uint(w,eth_start+12);tags=[]
    while typ in [0x8100,0x88a8,0x9100]:
        assert start+4<=frame_end;tags.append(dict(offset=start,tci=uint(w,start)))
        typ=uint(w,start+2);start+=4
    result=dict(network_start=start,vlan=tags,link_detail=link_detail)
    if typ==0x800:
        assert start+20<=frame_end and w[start]>>4==4
        n=(w[start]&15)*4;total=uint(w,start+2);end=start+total
        assert n>=20 and start+n<=frame_end and total>=n
        flags=uint(w,start+6);next_header=w[start+9];at=start+n
        src=w[start+12:start+16];dst=w[start+16:start+20]
        result.update(version=4,source=addr(src),destination=addr(dst),parent_length=total,
            parent_declared_end=end,parent_capture_end=min(end,frame_end),header_length=n,
            network_header_checksum_residual=checksum(w[start:start+n]),
            fragmented=bool(flags&0x3fff),fragment_offset=(flags&8191)*8,more_fragments=bool(flags&8192),
            initial_next_header=next_header,initial_next_header_range=[start+9,start+10])
    elif typ==0x86dd:
        assert start+40<=frame_end and w[start]>>4==6
        length=uint(w,start+4);end=start+40+length;at=start+40;next_header=w[start+6]
        src=w[start+8:start+24];dst=w[start+24:start+40]
        result.update(version=6,source=addr(src),destination=addr(dst),parent_length=40+length,
            ipv6_payload_length=length,parent_declared_end=end,parent_capture_end=min(end,frame_end),
            header_length=40,fragmented=False,initial_next_header=next_header,
            initial_next_header_range=[start+6,start+7],hop_limit=w[start+7])
    else:return None
    limit=min(end,frame_end);extensions=[];parent_header_range=result['initial_next_header_range']
    for _ in range(16):
        if next_header not in [0,43,44,51,60]:break
        assert at+2<=limit
        old=next_header;next_header=w[at]
        if old==44:
            n=8;assert at+n<=limit
            flags=uint(w,at+2);result['fragmented']=bool(flags&0xfff9)
            extension=dict(kind='fragment',next_header=next_header,range=[at,at+n],
                offset=(flags>>3)*8,more=bool(flags&1),identification=uint(w,at+4,4))
        else:
            n=(w[at+1]+(2 if old==51 else 1))*(4 if old==51 else 8);assert at+n<=limit
            extension=dict(kind={0:'hop-by-hop',43:'routing',51:'AH',60:'destination'}[old],
                next_header=next_header,range=[at,at+n],payload_length_field=w[at+1])
            if old==51:
                assert n>=12
                extension.update(SPI=uint(w,at+4,4),sequence=uint(w,at+8,4),
                    ICV_range=[at+12,at+n],ICV_SHA256=digest(w[at+12:at+n]),authentication_verified=False)
        extensions.append(extension);parent_header_range=[at,at+1];at+=n
        if result['fragmented']:break
    result.update(extensions=extensions,terminal_next_header=next_header,terminal_next_header_range=parent_header_range,
        payload_start=at,payload_range=[at,limit],capture_short=frame_end<end,
        frame_padding_range=[end,frame_end] if end<frame_end else None)
    if next_header==17 and not result['fragmented']:
        assert at+8<=limit
        source_port,destination_port,length,cs=struct.unpack_from('!HHHH',w,at)
        udp_end=at+length
        result['UDP']=dict(source_port=source_port,destination_port=destination_port,length=length,
            checksum=cs,header_range=[at,at+8],payload_range=[at+8,min(udp_end,limit)],
            declared_end=udp_end,parent_declared_end=end,
            parent_extent_valid=8<=length and udp_end<=end,
            complete=8<=length and udp_end<=limit,
            extra_parent_bytes=max(0,end-udp_end))
        if length>=8 and udp_end<=limit:
            udp=w[at:udp_end]
            pseudo=(src+dst+b'\0\x11'+length.to_bytes(2,'big')) if result['version']==4 else (src+dst+length.to_bytes(4,'big')+b'\0\0\0\x11')
            result['UDP']['checksum_residual']=checksum(pseudo+udp) if cs else None
            if 4500 in [source_port,destination_port]:
                payload=udp[8:];kind='truncated'
                if payload==b'\xff':kind='keepalive'
                elif len(payload)>=4 and payload[:4]==b'\0'*4:kind='non-ESP-marker'
                elif len(payload)>=8:kind='ESP-clear-header'
                result['NATT']=dict(kind=kind,wire_range=[at+8,udp_end],wire_length=len(payload),wire_SHA256=digest(payload))
                if kind=='ESP-clear-header':result['NATT']['ESP']=dict(SPI=uint(payload,0,4),SPI_hex='0x%08x'%uint(payload,0,4),
                    sequence=uint(payload,4,4),SPI_range=[at+8,at+12],sequence_range=[at+12,at+16],
                    opaque_range=[at+16,udp_end],opaque_length=len(payload)-8,authentication_verified=False,
                    SA_context_available=False,extended_sequence_reconstructed=False)
                if kind=='non-ESP-marker' and len(payload)>=32:
                    result['NATT']['IKE']=dict(version=payload[21],exchange_type=payload[22],flags=payload[23],
                        message_ID=uint(payload,24,4),declared_length=uint(payload,28,4),available_length=len(payload)-4,
                        header_range=[at+12,at+40])
    if next_header==50 and not result['fragmented'] and at+8<=limit:
        result['ESP']=dict(SPI=uint(w,at,4),SPI_hex='0x%08x'%uint(w,at,4),sequence=uint(w,at+4,4),
            SPI_range=[at,at+4],sequence_range=[at+4,at+8],opaque_range=[at+8,limit],
            wire_length=limit-at,wire_SHA256=digest(w[at:limit]),authentication_verified=False)
    if next_header==89 and not result['fragmented']:
        result['OSPF']=ospf(w,at,limit,result,src,dst)
    return result

def ospf(w,at,limit,ip,src,dst):
    assert at+4<=limit
    version=w[at];typ=w[at+1];length=uint(w,at+2);end=at+length
    out=dict(version=version,type=typ,declared_length=length,available_length=limit-at,
        declared_range=[at,end],parent_payload_range=[at,limit],extent_valid=length<=(limit-at),
        version_range=[at,at+1],type_range=[at+1,at+2],length_range=[at+2,at+4])
    if version!=3:return out
    assert length>=16 and end<=limit
    p=w[at:end];header=bytearray(p);header[12:14]=b'\0\0'
    pseudo=src+dst+length.to_bytes(4,'big')+b'\0\0\0\x59'
    out.update(router_ID=addr(p[4:8]),area_ID=addr(p[8:12]),checksum=uint(p,12),
        computed_checksum=checksum(pseudo+bytes(header)),checksum_residual=checksum(pseudo+p),
        instance_ID=p[14],reserved=p[15],header_range=[at,at+16],wire_SHA256=digest(p),
        trailing_parent_bytes=limit-end,checksum_pseudo_next_header=89,
        checksum_pseudo_upper_layer_length=length,checksum_pseudo_excludes_AH=True)
    body=p[16:];out['body_range']=[at+16,end]
    if typ==1:
        assert len(body)>=20 and (len(body)-20)%4==0
        out['Hello']=dict(interface_ID=uint(body,0,4),priority=body[4],options=uint(body,5,3),
            hello_interval=uint(body,8),dead_interval=uint(body,10),designated_router=addr(body[12:16]),
            backup_designated_router=addr(body[16:20]),neighbors=[addr(body[n:n+4]) for n in range(20,len(body),4)])
    elif typ==2:
        assert len(body)>=12 and (len(body)-12)%20==0
        out['DBD']=dict(reserved_1=body[0],options=uint(body,1,3),interface_MTU=uint(body,4),
            reserved_2=body[6],flags=body[7],DD_sequence=uint(body,8,4))
        out['LSA_headers']=lsa_headers(body[12:],at+28)
    elif typ==3:
        assert len(body)%12==0
        out['requests']=[dict(reserved=uint(body,n),LS_type=uint(body,n+2),
            link_state_ID=addr(body[n+4:n+8]),advertising_router=addr(body[n+8:n+12]),
            source_range=[at+16+n,at+28+n]) for n in range(0,len(body),12)]
    elif typ==4:
        assert len(body)>=4
        count=uint(body,0,4);n=4;lsas=[]
        for _ in range(count):
            assert n+20<=len(body);length=uint(body,n+18);assert length>=20 and n+length<=len(body)
            lsa=body[n:n+length];row=lsa_headers(lsa[:20],at+16+n)[0]
            row.update(full_wire_SHA256=digest(lsa),fletcher=fletcher(lsa),body_range=[at+36+n,at+16+n+length])
            lsas.append(row);n+=length
        assert n==len(body);out['LSA_count']=count;out['LSAs']=lsas
    elif typ==5:
        assert len(body)%20==0;out['LSA_headers']=lsa_headers(body,at+16)
    else:out['unsupported_packet_type']=True
    return out
def lsa_headers(w,base):
    assert len(w)%20==0
    return [dict(age=uint(w,n),LS_type=uint(w,n+2),link_state_ID=addr(w[n+4:n+8]),
        advertising_router=addr(w[n+8:n+12]),sequence=uint(w,n+12,4),checksum=uint(w,n+16),
        length=uint(w,n+18),header_range=[base+n,base+n+20]) for n in range(0,len(w),20)]

def main():
    p=argparse.ArgumentParser(description=__doc__)
    for a in ['capture','cli','wireshark-csv','reader','out']:p.add_argument('--'+a,type=Path,required=True)
    a=p.parse_args();a.out.mkdir(exist_ok=False)
    sys.path.insert(0,str(a.reader.parent))
    spec=importlib.util.spec_from_file_location('bounded_capture_reader',a.reader);m=importlib.util.module_from_spec(spec);spec.loader.exec_module(m)
    ws={int(r[0]):dict(label=r[5],stack=r[6],captured_length=int(r[1]),original_length=int(r[2]),interface=int(r[3])) for r in csv.reader(a.wireshark_csv.open(encoding='utf-8-sig',newline=''))}
    labels={}
    with a.cli.open(encoding='utf-8-sig') as f:
        for line in f:
            r=json.loads(line)
            if 'number' in r:labels[r['number']]={k:r.get(k) for k in ['protocol','layers','info','source','destination','length','captured_length','capture_domain']}
    gaps={n for n,r in ws.items() if (r['label']=='OSPF' and labels[n]['protocol']!='OSPF') or (r['label']=='ESP' and labels[n]['protocol']!='ESP')}
    results=[];counts=collections.Counter();all_source_errors=[]
    for n,r in enumerate(m.capture_records(a.capture),1):
        try:o=outer(r)
        except (AssertionError,ValueError,IndexError) as e:
            if n in gaps:raise
            all_source_errors.append(n);continue
        if o is None:continue
        kind='NATT' if 'NATT' in o else 'OSPF' if 'OSPF' in o else 'ESP' if 'ESP' in o else None
        if kind is None:continue
        counts[kind]+=1
        subtype=(o[kind].get('kind') if kind=='NATT' else o[kind].get('version') if kind=='OSPF' else None)
        counts[kind+'/'+str(subtype)]+=1
        if n not in gaps and counts[kind+'/'+str(subtype)]>3:continue
        row=dict(frame=n,section=r['section'],interface=r['interface'],link=r['link'],
            captured_length=r['captured_length'],original_length=r['original_length'],time_ns=r['time_ns'],
            frame_wire_SHA256=digest(r['wire']),Wireshark=ws[n],CLI=labels[n],gap=n in gaps,source=o)
        assert r['captured_length']==ws[n]['captured_length'] and r['original_length']==ws[n]['original_length'] and r['interface']==ws[n]['interface']
        name='frame-%d.bin'%n;(a.out/name).write_bytes(r['wire']);row['raw_frame_file']=name
        results.append(row)
    assert gaps<={r['frame'] for r in results}
    summary=dict(scope=__doc__,auditor_SHA256=sha(__file__),capture=str(a.capture),capture_SHA256=sha(a.capture),
        CLI=str(a.cli),CLI_SHA256=sha(a.cli),Wireshark_CSV=str(a.wireshark_csv),Wireshark_CSV_SHA256=sha(a.wireshark_csv),
        reader=str(a.reader),reader_SHA256=sha(a.reader),physical_frames=len(ws),source_protocol_counts=dict(counts),
        gap_frames=sorted(gaps),selected_count=len(results),source_parse_refusals_count=len(all_source_errors),
        source_parse_refusals_include_unselected_profiles=True,selected=results)
    (a.out/'source-facts.json').write_text(json.dumps(summary,ensure_ascii=False,indent=2)+'\n',encoding='utf8')
    print(json.dumps(dict(result=str(a.out/'source-facts.json'),SHA256=sha(a.out/'source-facts.json'),counts=dict(counts),gap_frames=sorted(gaps))))
if __name__=='__main__':main()
