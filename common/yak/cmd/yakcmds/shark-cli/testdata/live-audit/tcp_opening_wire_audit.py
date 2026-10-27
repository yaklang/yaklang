"""Certify declared opening/old-close observations against the original acquisition.

Only zero-payload opening SYN records can be copied, only RST records can be
removed/delayed, and every retained record (including its timestamp) must remain
byte-identical to its original record. Non-RST first occurrences retain original
order. Original two-way handshakes independently establish source generations;
the observer never receives this sidecar. This is a scoped offline observation
certificate, not a claim about injected retransmissions or endpoint acceptance.
"""
import collections
import hashlib
import json
import struct
from pathlib import Path

from mail_rtsp_public_wire_audit import tcp_packet
from tcp_epoch_audit import partition


def sha(value):return hashlib.sha256(value).hexdigest()
def canonical(value):return json.dumps(value,sort_keys=True,separators=(',',':'))


def capture_records(path):
    data=path.read_bytes();assert len(data)<=32<<20,'bounded classic-PCAP observation profile'
    assert data[:4]in[b'\xd4\xc3\xb2\xa1',b'\xa1\xb2\xc3\xd4',b'\x4d\x3c\xb2\xa1',b'\xa1\xb2\x3c\x4d']
    endian='<'if data[:4]in[b'\xd4\xc3\xb2\xa1',b'\x4d\x3c\xb2\xa1']else'>'
    nanos=data[:4]in[b'\x4d\x3c\xb2\xa1',b'\xa1\xb2\x3c\x4d']
    assert struct.unpack_from(endian+'HH',data,4)==(2,4)
    link=struct.unpack_from(endian+'I',data,20)[0];at=24;records={};physical={}
    while at<len(data):
        assert at+16<=len(data)
        seconds,fraction,captured,original=struct.unpack_from(endian+'IIII',data,at)
        assert captured<=original and at+16+captured<=len(data)
        record=data[at:at+16+captured];frame=record[16:];number=len(records)+1
        records[number]=dict(record=record,frame=frame,time_ns=seconds*1000000000+fraction*(1 if nanos else 1000))
        decoded,decode_link=frame,link
        if link==0:
            assert len(frame)>=4
            if frame[:4]not in[b'\x02\0\0\0',b'\0\0\0\x02']:
                at+=16+captured;continue
            decoded,decode_link=bytes(12)+b'\x08\0'+frame[4:],1
        packet=tcp_packet(decoded,decode_link,0,0,include_control=True)
        if packet is not None:physical[number]=dict(packet,frame_sha256=sha(frame),time_ns=records[number]['time_ns'])
        at+=16+captured
    assert at==len(data)
    return data[:24],records,physical


def source_partition(capture,source_capture,manifest,source_certificate):
    plan=json.loads(manifest.read_text(encoding='utf8'))
    certificate=json.loads(source_certificate.read_text(encoding='utf8'))
    assert plan['source_capture_SHA256']==certificate['capture_sha256']==sha(source_capture.read_bytes())
    assert plan['source_certificate_SHA256']==sha(source_certificate.read_bytes())
    assert plan['capture_SHA256']==sha(capture.read_bytes())
    assert plan['wire_or_timestamp_rewrite']is False and plan['network_retransmission_or_loss_injection']is False,'explicit record-only observation scope'
    header,source,source_physical=capture_records(source_capture)
    current_header,current,physical=capture_records(capture)
    assert current_header==header,'record-profile controls retain the exact original PCAP header'
    source_by_frame,source_epochs=partition(source_physical)
    assert canonical(source_epochs)==canonical(certificate['original_TCP_epochs']),'recompute source handshakes instead of trusting a declared epoch label'
    mapping=plan['original_frame_by_observation_frame']
    assert set(mapping)=={str(n)for n in current}and plan['physical_frames']==len(current)
    mapping={int(k):v for k,v in mapping.items()}
    for no,parent in mapping.items():
        assert type(parent)is int and parent in source,'literal existing original record identity'
        assert current[no]['record']==source[parent]['record'],'all packet bytes, lengths and timestamps remain exact'
        assert(no in physical)==(parent in source_physical),'original TCP/non-TCP carrier classification'
    counts=collections.Counter(mapping.values());removed=sorted(set(source)-set(counts));duplicated=sorted(n for n,count in counts.items()if count>1)
    assert removed==plan['removed_source_frames']and duplicated==plan['duplicated_source_frames']
    for parent in removed:assert parent in source_physical and source_physical[parent]['reset'],'only original reset records may be absent'
    for parent in duplicated:
        packet=source_physical[parent]
        assert packet['syn']and not packet['ack']and not packet['payload']and not packet['fin']and not packet['reset'],'only complete payload-free opening SYN observations may be copied'
    first={}
    for no,parent in mapping.items():first.setdefault(parent,no)
    ordered=[parent for no,parent in mapping.items()if first[parent]==no and not source_physical.get(parent,{}).get('reset')]
    assert ordered==sorted(ordered),'first non-RST observations keep original order'
    delayed=[]
    for parent,position in first.items():
        if not source_physical.get(parent,{}).get('reset'):continue
        assert counts[parent]==1,'reset observations are not duplicated'
        assert all(first[n]<position for n in first if n<parent and not source_physical.get(n,{}).get('reset')),'old close cannot precede its original observed bytes'
        if any(first[n]<position for n in first if n>parent and not source_physical.get(n,{}).get('reset')):delayed.append(parent)
    assert len(delayed)<=1 and plan['delayed_old_reset_source_frame']==(delayed[0]if delayed else None)
    # The delayed reset must still be the exact old connection's observed end.
    # Check both original sequence cursors independently, including wraparound.
    reset_checks=[]
    for parent in delayed:
        packet=source_physical[parent];epoch=next(v for v in source_epochs if v['epoch']==source_by_frame[parent]);bases={};cells=collections.defaultdict(dict)
        for n in epoch['frames']:
            p=source_physical[n]
            if p['syn']:bases[p['source']]=(p['sequence']+1)&0xffffffff
            if p['payload']:
                assert p['source']in bases
                start=(p['sequence']+int(p['syn'])-bases[p['source']])&0xffffffff
                assert start+len(p['payload'])<=4<<20
                for at,byte in enumerate(p['payload'],start):
                    assert at not in cells[p['source']]or cells[p['source']][at]==byte
                    cells[p['source']][at]=byte
        assert len(bases)==2
        ends={}
        for endpoint,base in bases.items():
            data=cells[endpoint];assert set(data)==set(range(len(data))),'original close has no unproved prior TCP gap'
            ends[endpoint]=(base+len(data))&0xffffffff
        assert not packet['payload']and packet['ack']and packet['sequence']==ends[packet['source']]and packet['ack_sequence']==ends[packet['destination']],'delayed RST belongs to exact original sender/peer end cursors'
        next_epoch=next(v for v in source_epochs if v['connection']==epoch['connection']and v['epoch']>epoch['epoch'])
        assert first[next_epoch['SYN_frame']]<first[parent]<first[next_epoch['SYN_ACK_frame']],'only delay an old close into the next opening-confirmation interval'
        reset_checks.append(dict(original_reset_frame=parent,observed_reset_frame=first[parent],original_epoch=epoch['epoch'],sequence=packet['sequence'],acknowledgment=packet['ack_sequence']))
    by_frame={no:source_by_frame[parent]for no,parent in mapping.items()if no in physical};epochs=[]
    for state in source_epochs:
        value=dict(state);value['frames']=[no for no in physical if by_frame[no]==state['epoch']]
        value['SYN_frame']=first[state['SYN_frame']];value['SYN_ACK_frame']=first[state['SYN_ACK_frame']];epochs.append(value)
    evidence=dict(scope=__doc__,source_capture_SHA256=sha(source_capture.read_bytes()),source_certificate_SHA256=sha(source_certificate.read_bytes()),manifest_SHA256=sha(manifest.read_bytes()),capture_SHA256=sha(capture.read_bytes()),removed_original_reset_frames=removed,duplicated_original_opening_frames=duplicated,delayed_old_reset_cursor_checks=reset_checks,original_frame_by_observation_frame=mapping,all_complete_original_records_byte_identical=True,auditor_SHA256=sha(Path(__file__).read_bytes()))
    return physical,by_frame,epochs,evidence
