"""Make declared observation controls from the actual Windows DLL RPC capture.

Move/duplicate/drop original complete packets or shorten one captured prefix.
These files model capture observation changes; they are not claims of actual
socket loss or packet injection. Preserve an exact original-frame mapping.
"""
import argparse
import hashlib
import json
import struct
from pathlib import Path

ap=argparse.ArgumentParser(description=__doc__)
ap.add_argument('capture',type=Path);ap.add_argument('out_dir',type=Path);a=ap.parse_args()
for path in[a.capture,a.out_dir]:assert not any((q/'.git').exists()for q in[path.resolve(),*path.resolve().parents])
wire=a.capture.read_bytes();assert wire[:4]==bytes.fromhex('4d3cb2a1'),'selected nanosecond little-endian PCAP'
assert len(wire)>=24 and struct.unpack_from('<I',wire,20)[0]==0,'actual Windows IPv4 loopback link type'
records=[];at=24
while at<len(wire):
    assert at+16<=len(wire)
    seconds,nanos,captured,original=struct.unpack_from('<IIII',wire,at);at+=16
    assert captured<=original and at+captured<=len(wire)
    records.append(dict(frame=len(records)+1,wire=wire[at:at+captured],original=original,time=seconds*10**9+nanos));at+=captured
requests=[]
for index,record in enumerate(records):
    frame=record['wire']
    if len(frame)<44 or frame[:4]!=b'\x02\0\0\0' or frame[4]>>4!=4 or frame[13]!=6:continue
    ip_header=(frame[4]&15)*4;tcp=4+ip_header
    if tcp+20>len(frame)or int.from_bytes(frame[tcp+2:tcp+4],'big')!=135:continue
    payload=tcp+(frame[tcp+12]>>4)*4;data=frame[payload:]
    if len(data)>=24 and data[:4]==b'\x05\0\0\x03'and int.from_bytes(data[8:10],'little')==len(data):requests.append((index,payload))
assert len(requests)==27,'actual owned inquiry trace has 27 clear single-PDU calls'
target,payload=requests[0]
assert requests[2][0]>target+1
variants={
    'duplicate-request-observation':[(i,None)for i in range(len(records))for _ in range(2 if i==target else 1)],
    'request-arrives-after-two-later-requests':[(i,None)for i in range(len(records))if i!=target],
    'permanent-request-capture-hole':[(i,None)for i in range(len(records))if i!=target],
    'snaplen-request-capture-hole':[(i,payload+12 if i==target else None)for i in range(len(records))],
}
delayed=variants['request-arrives-after-two-later-requests']
position=next(i for i,(source,_)in enumerate(delayed)if source==requests[2][0])+1
delayed.insert(position,(target,None))
a.out_dir.mkdir(exist_ok=False,parents=True)
summary=[]
for name,selection in variants.items():
    destination=a.out_dir/name;destination.mkdir()
    output=bytearray(wire[:24]);mapping=[]
    for new,(old,shorten)in enumerate(selection,1):
        record=records[old];frame=record['wire']if shorten is None else record['wire'][:shorten]
        # Synthetic monotone observation timestamps are explicitly declared;
        # every non-truncated captured packet byte remains exact.
        timestamp=records[0]['time']+new*10**6;seconds,nanos=divmod(timestamp,10**9)
        output+=struct.pack('<IIII',seconds,nanos,len(frame),record['original'])+frame
        mapping.append(dict(frame=new,original_frame=old+1,captured_bytes=len(frame),original_wire_bytes=record['original'],captured_sha256=hashlib.sha256(frame).hexdigest()))
    capture=destination/'derived.pcap';capture.write_bytes(output)
    receipt=dict(scope=__doc__,source_capture_sha256=hashlib.sha256(wire).hexdigest(),derived_capture_sha256=hashlib.sha256(output).hexdigest(),generator_sha256=hashlib.sha256(Path(__file__).read_bytes()).hexdigest(),selected_original_request_frame=target+1,physical_frames=len(selection),timestamp_policy='synthetic monotone observations spaced 1 ms; original packet bytes preserved except declared snaplen prefix',mapping=mapping)
    (destination/'derivation.json').write_text(json.dumps(receipt,indent=2)+'\n',encoding='utf8')
    summary.append(dict(case=name,physical_frames=len(selection),derived_capture_sha256=receipt['derived_capture_sha256']))
(a.out_dir/'summary.json').write_text(json.dumps(summary,indent=2)+'\n',encoding='utf8')
print(json.dumps(summary))
