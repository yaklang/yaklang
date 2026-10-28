"""Verify original records and declared incomplete-header capture observations.

This checks capture provenance, not an injected network fault or all-protocol
correctness. Epoch zero is a literal file timestamp, not an absent Go timestamp.
"""
import argparse,hashlib,json,struct
from pathlib import Path
from capture_clock_wire_audit import physical


def sha(p):return hashlib.sha256(p.read_bytes()).hexdigest()
def digest(w):return hashlib.sha256(w).hexdigest()


def observations(path):
    """Read the generated single-interface nanosecond PCAPNG independently."""
    data=path.read_bytes();at=0;link=None;section=False;result=[]
    while at<len(data):
        assert at+12<=len(data)
        kind,size=struct.unpack_from('<II',data,at);assert size>=12 and size%4==0 and at+size<=len(data)
        assert struct.unpack_from('<I',data,at+size-4)[0]==size
        body=data[at+8:at+size-4]
        if kind==0x0a0d0d0a:
            assert not section and body==struct.pack('<IHHq',0x1a2b3c4d,1,0,-1);section=True
        elif kind==1:
            assert section and link is None
            link,reserved,snap=struct.unpack_from('<HHI',body);assert reserved==0 and snap==65535
            assert body[8:]==struct.pack('<HH',9,1)+b'\x09\0\0\0'+b'\0'*4
        elif kind==6:
            assert link is not None and len(body)>=20
            interface,high,low,captured,original=struct.unpack_from('<IIIII',body)
            assert interface==0 and 0<=captured<=original<=65535 and 20+captured<=len(body)
            assert body[20+captured:]==b'\0'*((-captured)%4)
            result.append(dict(link=link,wire=body[20:20+captured],original_length=original,time_ns=(high<<32)|low))
        else:raise AssertionError('unselected generated block kind')
        at+=size
    assert at==len(data)and section and link is not None
    return result


def audit(capture,manifest):
    doc=json.loads(manifest.read_text('utf8'));source=Path(doc['source_capture']);assert sha(source)==doc['source_capture_SHA256']and sha(capture)==doc['capture_SHA256']
    original=physical(source);actual=observations(capture);assert len(actual)==len(doc['ledger'])==doc['physical_records'];order=[];prefixes=[]
    for n,(record,entry)in enumerate(zip(actual,doc['ledger']),1):
        for field in['number','source_frame','source_section','source_interface','source_time_ns','time_ns']:assert type(entry[field])is int
        assert entry['number']==n and 0<entry['source_frame']<=len(original);old=original[entry['source_frame']-1];wire=old['wire']
        assert entry['source_section']==old['section']and entry['source_interface']==old['interface']
        assert entry['source_time_ns']==old['time_ns']and entry['time_ns']==record['time_ns']and entry['source_wire_SHA256']==digest(wire)
        assert record['link']==old['link']and record['original_length']==len(wire)
        change=entry['change'];kind=change['kind']
        if kind in['exact','exact-filtered-record']:
            assert record['wire']==wire and record['time_ns']==old['time_ns'];order.append(entry['source_frame'])
            if kind=='exact-filtered-record':assert record['link']==1 and wire[12:14]==b'\x08\0'and 14<len(wire)<34
        else:
            assert kind=='incomplete-fixed-header-observation'
            for field in['IP_header_offset','available_fixed_header_bytes','required_fixed_header_bytes','original_wire_length','observation_clock_offset_ns']:assert type(change[field])is int
            at=4 if old['link']==0 else 14
            assert (wire[:4]==b'\x02\0\0\0'if old['link']==0 else old['link']==1 and wire[12:14]==b'\x08\0')
            assert change['IP_header_offset']==at and wire[at]>>4==4 and len(wire)>=at+20
            available=change['available_fixed_header_bytes'];assert available in[0,4]and change['required_fixed_header_bytes']==20
            assert record['wire']==wire[:at+available]and change['original_wire_length']==len(wire)
            shift=record['time_ns']-old['time_ns'];assert shift==change['observation_clock_offset_ns']
            interpretation=change['timestamp_interpretation']
            assert interpretation in['literal PCAP epoch zero','literal future timestamp']
            assert (record['time_ns']==0 if interpretation=='literal PCAP epoch zero'else shift==3600_000_000_000)
            prefixes.append(dict(number=n,source_frame=entry['source_frame'],IP_offset=at,available=available,required=20,source_wire_SHA256=digest(wire),captured_prefix_SHA256=digest(record['wire']),timestamp_interpretation=interpretation))
    if doc['case_scope']=='original-record-filter':
        assert len(actual)==5 and not prefixes and order==sorted(set(order))and not doc['all_original_primary_records_preserved']
    else:
        assert doc['case_scope']=='all-original-records-plus-one-declared-prefix'and len(prefixes)==1 and doc['all_original_primary_records_preserved']is True
        assert order==list(range(1,len(original)+1)),'all original socket observations remain exact and in order'
    return dict(scope=__doc__,capture_SHA256=sha(capture),manifest_SHA256=sha(manifest),source_capture_SHA256=sha(source),auditor_SHA256=sha(Path(__file__)),physical_records=len(actual),original_records=len(order),all_declared_source_bytes_and_timestamps_exact=True,inserted_prefixes=prefixes,network_fault_injection_claim=False)


if __name__=='__main__':
    ap=argparse.ArgumentParser(description=__doc__)
    for n in['capture','manifest','out']:ap.add_argument('--'+n,type=Path,required=True)
    a=ap.parse_args()
    for p in vars(a).values():assert not any((v/'.git').exists()for v in[p.resolve(),*p.resolve().parents])
    proof=audit(a.capture,a.manifest)
    with a.out.open('x',encoding='utf8')as f:json.dump(proof,f,indent=2);f.write('\n')
    print(json.dumps(dict(records=proof['physical_records'],prefixes=len(proof['inserted_prefixes']),source_exact=True)),flush=True)
