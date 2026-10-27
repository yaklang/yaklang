"""Capture real local Windows RPC endpoint-mapper bind replies, without login.

Usage: dcerpc_windows_live.py OUTPUT_DIR TSHARK [--unfragmented]
Only open TCP/135, send declared bind proposals and read their first reply.
The NTLM case sends an independently built Type-1 negotiate token; it never
sends credentials, an authenticate token, a procedure request or Auth3.
Captures, server tokens, receipts and manifests remain outside Git.
"""
import base64
import hashlib
import json
import socket
import struct
import sys
import time
import uuid
from pathlib import Path

from capture_ready import start_capture

out=Path(sys.argv[1]).resolve()
assert not any((p/'.git').exists()for p in[out,*out.parents])
out.mkdir(exist_ok=False,parents=True)

def syntax(value,version):return uuid.UUID(value).bytes_le+struct.pack('<I',version)
abstract=uuid.UUID('e1af8308-5d1f-11c9-91a4-08002b14a0fa').bytes_le+struct.pack('<HH',3,0)
ndr32=syntax('8a885d04-1ceb-11c9-9fe8-08002b104860',2)
ndr64=syntax('71710533-beba-4937-8319-b5dbef9ccc36',1)
features=syntax('6cb71c2c-9812-4540-0300-000000000000',1)
receipts=[];operations=[];failures=[]

def bind(transfers,token=b''):
    body=struct.pack('<HHIB3x',5840,5840,0,len(transfers))
    for context,transfer in enumerate(transfers):
        body+=struct.pack('<HBB',context,1,0)+abstract+transfer
    if token:
        padding=(-len(body))%16
        body+=bytes(padding)+struct.pack('<BBBBI',10,2,padding,0,0)+token
    return struct.pack('<BBBB4sHHI',5,0,11,3,b'\x10\0\0\0',16+len(body),len(token),2)+body

def read_exact(s,n):
    data=b''
    while len(data)<n:
        part=s.recv(n-len(data));assert part,'endpoint closed before complete declared reply'
        data+=part
    return data

def record(case,direction,wire,source,destination):
    receipts.append(dict(case=case,direction=direction,source=source,destination=destination,
                         bytes=len(wire),sha256=hashlib.sha256(wire).hexdigest(),
                         wire_base64=base64.b64encode(wire).decode(),time_ns=time.time_ns()))

# No optional VERSION flag/structure, no username/domain/workstation buffers.
ntlm_negotiate=b'NTLMSSP\0'+struct.pack('<II',1,0xA0088205)+bytes(16)
cases=[('NDR32',[ndr32],b''),('NDR32-NDR64-feature-negotiation',[ndr32,ndr64,features],b''),
       ('NTLM-negotiate-only',[ndr32],ntlm_negotiate)]
cap,marker_port=start_capture(sys.argv[2],out/'live.pcap','tcp port 135',22)
fragmented='--unfragmented'not in sys.argv[3:]
try:
    for case,transfers,token in cases:
        with socket.create_connection(('127.0.0.1',135),timeout=5)as s:
            s.settimeout(5);s.setsockopt(socket.IPPROTO_TCP,socket.TCP_NODELAY,1)
            source='127.0.0.1:'+str(s.getsockname()[1]);destination='127.0.0.1:135'
            wire=bind(transfers,token);record(case,'c2s',wire,source,destination)
            # Actual TCP fragmentation, not a modified replay capture.
            if fragmented:
                for at in range(0,len(wire),7):s.sendall(wire[at:at+7]);time.sleep(.001)
            else:s.sendall(wire)
            header=read_exact(s,16)
            assert header[:2]==b'\x05\x00'and header[4:8]==b'\x10\0\0\0'
            size=struct.unpack_from('<H',header,8)[0];assert 16<=size<=65535
            reply=header+read_exact(s,size-16);record(case,'s2c',reply,destination,source)
            operations.append(dict(case=case,proposals=len(transfers),reply_PType=reply[2],reply_bytes=len(reply),
                                   reply_auth_bytes=struct.unpack_from('<H',reply,10)[0],authentication_completed=False))
except Exception as exc:
    failures.append(type(exc).__name__+': '+str(exc));raise
finally:
    _,stderr=cap.communicate(timeout=30);(out/'capture.stderr').write_bytes(stderr)
    (out/'wire-receipts.json').write_text(json.dumps(receipts,indent=2)+'\n',encoding='utf8')
    manifest=dict(generator_sha256=hashlib.sha256(Path(__file__).read_bytes()).hexdigest(),
                  capture_sha256=hashlib.sha256((out/'live.pcap').read_bytes()).hexdigest(),marker_port=marker_port,
                  endpoint='actual local Windows RPC endpoint mapper on TCP/135',client='independent Python wire builder',
                  operations=operations,failures=failures,client_TCP_write_bytes=7 if fragmented else 'complete PDU',credentials_or_procedure_calls_sent=False)
    (out/'manifest.json').write_text(json.dumps(manifest,indent=2)+'\n',encoding='utf8')
assert cap.returncode==0 and not failures
print(json.dumps(dict(operations=len(operations),receipts=len(receipts),capture_sha256=manifest['capture_sha256'])))
