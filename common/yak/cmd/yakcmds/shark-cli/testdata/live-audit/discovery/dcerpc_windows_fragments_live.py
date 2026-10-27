"""Capture bounded, read-only EPM lookups with actual RPC fragmentation.

Usage: dcerpc_windows_fragments_live.py OUTPUT_DIR TSHARK
Only query local TCP/135 using EPM v3/NDR32. Never register/delete endpoints,
negotiate credentials or invoke a discovered service. Keep replies outside Git.
The independently built fragmented request repeats its per-PDU request header.
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

out=Path(sys.argv[1]).resolve();assert not any((p/'.git').exists()for p in[out,*out.parents])
out.mkdir(exist_ok=False,parents=True)
receipts=[];cases=[];failures=[]
abstract=uuid.UUID('e1af8308-5d1f-11c9-91a4-08002b14a0fa').bytes_le+struct.pack('<HH',3,0)
transfer=uuid.UUID('8a885d04-1ceb-11c9-9fe8-08002b104860').bytes_le+struct.pack('<I',2)

def pdu(ptype,flags,call,body):
    return struct.pack('<BBBB4sHHI',5,0,ptype,flags,b'\x10\0\0\0',16+len(body),0,call)+body

def record(case,direction,wire,source,destination):
    receipts.append(dict(case=case,direction=direction,source=source,destination=destination,bytes=len(wire),sha256=hashlib.sha256(wire).hexdigest(),wire_base64=base64.b64encode(wire).decode(),time_ns=time.time_ns()))

def exact(s,n):
    data=b''
    while len(data)<n:
        piece=s.recv(n-len(data));assert piece,'endpoint closed inside declared RPC PDU'
        data+=piece
    return data

def receive(s,case,source,destination,call,expected):
    fragments=[];total=0
    for index in range(2048):
        header=exact(s,16);length,auth,actual=struct.unpack_from('<HHI',header,8)
        assert header[:2]==b'\x05\0'and header[4:8]==b'\x10\0\0\0'and 16<=length<=65535 and auth==0 and actual==call
        wire=header+exact(s,length-16);record(case,'s2c',wire,destination,source)
        assert wire[2]in[expected,3],('unexpected reply PType',wire[2])
        if wire[2]==3:raise AssertionError('actual endpoint returned RPC fault 0x'+wire[24:28][::-1].hex())
        assert bool(wire[3]&1)==(index==0),'unexpected RPC first-fragment sequence'
        total+=len(wire);assert total<=1<<20,'bounded actual endpoint response'
        fragments.append(wire)
        if wire[3]&2:return fragments
    raise AssertionError('bounded RPC fragment count exceeded')

cap,marker=start_capture(sys.argv[2],out/'live.pcap','tcp port 135',24)
try:
    for maximum,fragment_request in[(1024,True),(5840,False)]:
        case=f'maxfrag-{maximum}-request-'+('fragmented'if fragment_request else'whole')
        with socket.create_connection(('127.0.0.1',135),timeout=5)as s:
            s.settimeout(5);s.setsockopt(socket.IPPROTO_TCP,socket.TCP_NODELAY,1)
            source='127.0.0.1:'+str(s.getsockname()[1]);destination='127.0.0.1:135'
            bind=pdu(11,3,1,struct.pack('<HHIB3x',maximum,maximum,0,1)+struct.pack('<HBB',0,1,0)+abstract+transfer)
            record(case,'c2s',bind,source,destination);s.sendall(bind)
            ack=receive(s,case,source,destination,1,12);assert len(ack)==1
            body=ack[0][16:];at=(10+int.from_bytes(body[8:10],'little')+3)&~3
            assert body[at]==1 and int.from_bytes(body[at+4:at+6],'little')==0 and body[at+8:at+24]==transfer[:16]and int.from_bytes(body[at+24:at+28],'little')==2
            # MS-RPCE 2.2.1.2.4: ALL_ELTS, null object/Ifid, VERS_ALL,
            # null context handle and bounded max_ents=500. No service calls.
            stub=struct.pack('<IIII',0,0,0,1)+bytes(20)+struct.pack('<I',500)
            requests=[pdu(0,1,2,struct.pack('<IHH',len(stub),0,2)+stub[:20]),pdu(0,2,2,struct.pack('<IHH',len(stub)-20,0,2)+stub[20:])]if fragment_request else[pdu(0,3,2,struct.pack('<IHH',len(stub),0,2)+stub)]
            for wire in requests:record(case,'c2s',wire,source,destination);s.sendall(wire);time.sleep(.002)
            replies=receive(s,case,source,destination,2,2)
            assert all(len(wire)>=24 and wire[20:22]==b'\0\0'for wire in replies)
            # Independent receipt logical stub excludes the 8-byte response
            # header repeated in EACH physical RPC fragment.
            response_stub=b''.join(wire[24:]for wire in replies)
            assert len(response_stub)>=28
            handle=response_stub[:20];cleanup=0
            if handle!=bytes(20):
                free=pdu(0,3,3,struct.pack('<IHH',20,0,4)+handle)
                record(case,'c2s',free,source,destination);s.sendall(free)
                freed=receive(s,case,source,destination,3,2);assert len(freed)==1 and freed[0][24:44]==bytes(20)and int.from_bytes(freed[0][-4:],'little')==0
                cleanup=1
            cases.append(dict(case=case,offered_max_fragment=maximum,ACK_max_xmit=int.from_bytes(body[:2],'little'),ACK_max_recv=int.from_bytes(body[2:4],'little'),request_RPC_fragments=len(requests),request_stub_bytes=len(stub),request_stub_sha256=hashlib.sha256(stub).hexdigest(),response_RPC_fragments=len(replies),response_wire_bytes=sum(map(len,replies)),response_stub_bytes=len(response_stub),response_stub_sha256=hashlib.sha256(response_stub).hexdigest(),returned_entries=int.from_bytes(response_stub[20:24],'little'),explicit_context_cleanup_calls=cleanup,reply_auth_lengths=[int.from_bytes(wire[10:12],'little')for wire in replies]))
except Exception as exc:failures.append(type(exc).__name__+': '+str(exc));raise
finally:
    _,stderr=cap.communicate(timeout=35);(out/'capture.stderr').write_bytes(stderr)
    (out/'wire-receipts.json').write_text(json.dumps(receipts,indent=2)+'\n',encoding='utf8')
    manifest=dict(generator_sha256=hashlib.sha256(Path(__file__).read_bytes()).hexdigest(),capture_sha256=hashlib.sha256((out/'live.pcap').read_bytes()).hexdigest(),marker_port=marker,client='independent Python wire builder with actual per-fragment request/response headers',target='actual local Windows EPM v3/NDR32 over TCP/135',cases=cases,wire_PDU_receipts=len(receipts),failures=failures,endpoint_registration_or_discovered_service_invocation=False)
    (out/'manifest.json').write_text(json.dumps(manifest,indent=2)+'\n',encoding='utf8')
assert cap.returncode==0 and not failures
print(json.dumps(dict(cases=cases,wire_PDU_receipts=len(receipts),capture_sha256=manifest['capture_sha256'])))
