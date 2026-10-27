"""Certify every declared clock-control record against original socket captures.

Read classic PCAP/PCAPNG independently of the producer. Check exact source
bytes/times, unchanged transport slices, IP extent/checksum/offset/header fields,
complete original-record order and separately declared foreign clock shifts.
This proves the offline transformation, not endpoint acceptance or decryption.
"""
import argparse,collections,hashlib,json,struct
from pathlib import Path


def sha(value):return hashlib.sha256(value).hexdigest()


def physical(path):
    wire=path.read_bytes();result=[]
    if wire[:4]in[b'\xd4\xc3\xb2\xa1',b'\x4d\x3c\xb2\xa1']:
        assert len(wire)>=24 and struct.unpack_from('<HH',wire,4)==(2,4)
        link=struct.unpack_from('<I',wire,20)[0];factor=1000 if wire[0]==0xd4 else 1;at=24
        while at<len(wire):
            assert at+16<=len(wire)
            seconds,fraction,count,original=struct.unpack_from('<IIII',wire,at);at+=16
            assert count==original and at+count<=len(wire)
            result.append(dict(wire=wire[at:at+count],section=0,interface=0,link=link,time_ns=seconds*1000000000+fraction*factor));at+=count
        assert at==len(wire)
        return result
    at=0;section=-1;interfaces=[]
    while at<len(wire):
        assert at+12<=len(wire)
        kind,size=struct.unpack_from('<II',wire,at)
        assert size>=12 and size%4==0 and at+size<=len(wire)
        assert struct.unpack_from('<I',wire,at+size-4)[0]==size
        if kind==0x0a0d0d0a:
            assert wire[at+8:at+12]==b'\x4d\x3c\x2b\x1a'
            section+=1;interfaces=[]
        elif kind==1:
            assert section>=0 and size>=20
            link=struct.unpack_from('<H',wire,at+8)[0];precision=6;option=at+16
            while option+4<=at+size-4:
                code,count=struct.unpack_from('<HH',wire,option);option+=4
                if code==0:break
                assert option+count<=at+size-4
                if code==9:
                    assert count==1 and wire[option]<=9
                    precision=wire[option]
                option+=(count+3)&~3
            interfaces.append((link,precision))
        elif kind==6:
            assert section>=0 and size>=32
            interface,high,low,count,original=struct.unpack_from('<IIIII',wire,at+8)
            assert interface<len(interfaces)and count==original and at+28+count<=at+size-4
            link,precision=interfaces[interface]
            result.append(dict(wire=wire[at+28:at+28+count],section=section,interface=interface,link=link,time_ns=((high<<32)|low)*10**(9-precision)))
        at+=size
    assert at==len(wire)and section==0
    return result


def ip(record):
    raw=record['wire'];link=record['link']
    if link==0:assert raw[:4]==b'\x02\0\0\0';start=4
    else:assert link==1 and raw[12:14]==b'\x08\0';start=14
    assert start+20<=len(raw)and raw[start]>>4==4
    size=(raw[start]&15)*4;end=start+int.from_bytes(raw[start+2:start+4],'big')
    assert size>=20 and start+size<=end==len(raw)
    return raw[:start],raw[start:start+size],raw[start+size:end]


def audit(capture,manifest,primary,foreign):
    doc=json.loads(manifest.read_text('utf8'));source={'primary':physical(primary),'CoAP':physical(foreign)};actual=physical(capture)
    assert doc['capture_SHA256']==sha(capture.read_bytes())and doc['physical_records']==len(actual)==len(doc['ledger'])
    original_order=[];primary_counts=collections.Counter();fragments=collections.defaultdict(list);copies=[];changes=[]
    for number,(record,entry)in enumerate(zip(actual,doc['ledger']),1):
        for key in['number','interface','time_ns','source_frame','source_time_ns']:assert type(entry[key])is int
        assert entry['number']==number and record['section']==0 and record['interface']==entry['interface']in[0,1]
        assert entry['source']==('primary'if record['interface']==0 else'CoAP')
        assert record['time_ns']==entry['time_ns']and sha(record['wire'])==entry['wire_SHA256']
        assert 0<entry['source_frame']<=len(source[entry['source']])
        original=source[entry['source']][entry['source_frame']-1]
        assert record['link']==original['link']and entry['source_time_ns']==original['time_ns']and entry['source_wire_SHA256']==sha(original['wire'])
        change=entry['change'];kind=change['kind']
        assert kind in['exact','record-copy-clock-shift','IPv4-fragment']
        if kind=='exact':
            assert entry['source']=='primary'and record['wire']==original['wire']and record['time_ns']==original['time_ns']
        elif kind=='record-copy-clock-shift':
            assert record['wire']==original['wire']and record['time_ns']==original['time_ns']+change['offset_ns']
            copies.append(dict(frame=number,source=entry['source'],source_frame=entry['source_frame'],clock_offset_ns=change['offset_ns']))
        else:
            prefix,header,payload=ip(record);old_prefix,old_header,old_payload=ip(original)
            assert prefix==old_prefix and len(header)==len(old_header)==20
            assert int.from_bytes(old_header[6:8],'big')&0x3fff==0
            flags=int.from_bytes(header[6:8],'big');start=(flags&0x1fff)*8;end=start+len(payload)
            assert not flags&0xc000 and [start,end]==change['IP_payload_interval']and payload==old_payload[start:end]
            assert 0<=start<end<=len(old_payload)and bool(flags&0x2000)==(end<len(old_payload))
            if flags&0x2000:assert len(payload)%8==0
            assert sum(struct.unpack('>10H',header))%65535==0,'derived IPv4 checksum'
            allowed={2,3,6,7,10,11}
            if entry['source']=='CoAP':
                assert change['identifier']==int.from_bytes(header[4:6],'big')==0x6a09;allowed|={4,5}
            else:assert header[4:6]==old_header[4:6]
            assert all(a==b for index,(a,b)in enumerate(zip(header,old_header))if index not in allowed),'only declared envelope fields change'
            if entry['source']=='primary':assert record['time_ns']==original['time_ns']+int(start>0)
            fragments[(entry['source'],entry['source_frame'])].append((start,end,number,record['time_ns']))
        if entry['source']=='primary'and kind!='record-copy-clock-shift':
            primary_counts[entry['source_frame']]+=1
            if not original_order or original_order[-1]!=entry['source_frame']:original_order.append(entry['source_frame'])
        if kind!='exact':changes.append(dict(frame=number,source=entry['source'],source_frame=entry['source_frame'],change=change))
    assert original_order==list(range(1,len(source['primary'])+1)),'every original primary record remains in order'
    assert all(value in[1,2]for value in primary_counts.values())
    for key,ranges in fragments.items():
        original=source[key[0]][key[1]-1];ranges.sort();assert len(ranges)==2 and ranges[0][0]==0 and ranges[0][1]==ranges[1][0]and ranges[1][1]==len(ip(original)[2])
    if fragments:
        selected=[v for k,v in fragments.items()if k[0]=='primary'];assert len(selected)==1 and not copies
        initial_time=selected[0][0][3]
        for key,ranges in fragments.items():
            if key[0]=='CoAP':assert [r[3]for r in ranges]==[initial_time+3600*1000000000,initial_time+3600*1000000000+1]
        assert len(fragments)in[1,2]
    else:
        assert len(copies)==1 and copies[0]['clock_offset_ns']>0
    return dict(scope=__doc__,capture_SHA256=sha(capture.read_bytes()),manifest_SHA256=sha(manifest.read_bytes()),primary_capture_SHA256=sha(primary.read_bytes()),foreign_capture_SHA256=sha(foreign.read_bytes()),auditor_SHA256=sha(Path(__file__).read_bytes()),physical_records=len(actual),original_primary_records=len(source['primary']),all_original_primary_records_preserved_in_order=True,all_transport_bytes_equal_declared_original_slices=True,fragmented_original_records=len(fragments),copied_records=copies,transformed_records=changes,wire_or_endpoint_acceptance_claim=False)


if __name__=='__main__':
    ap=argparse.ArgumentParser(description=__doc__)
    for name in['capture','manifest','primary','foreign','out']:ap.add_argument('--'+name,type=Path,required=True)
    a=ap.parse_args()
    for path in vars(a).values():assert not any((p/'.git').exists()for p in[path.resolve(),*path.resolve().parents])
    proof=audit(a.capture,a.manifest,a.primary,a.foreign)
    with a.out.open('x',encoding='utf8',newline='\n')as out:json.dump(proof,out,indent=2);out.write('\n')
    print(json.dumps(dict(physical_records=proof['physical_records'],all_original_records_and_transport_bytes_checked=True)),flush=True)
