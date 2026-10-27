"""Capture actual local TCP five-tuple reuse with bounded read-only RPC traffic.

Usage: dcerpc_windows_tuple_reuse_live.py OUTPUT_DIRECTORY TSHARK
Abortive socket close sends RST and avoids TIME_WAIT before reusing one client
port. Include an incomplete request, an unbound-context rejection, a different
accepted context and another complete lookup. Never modify endpoints, supply
credentials or invoke a discovered service. Keep captures/receipts outside Git.
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


def pdu(ptype, flags, call, body):
    return struct.pack('<BBBB4sHHI', 5, 0, ptype, flags, b'\x10\0\0\0', 16+len(body), 0, call)+body


def run(out, tshark):
    out.mkdir(parents=True, exist_ok=False)
    generator_digest = hashlib.sha256(Path(__file__).read_bytes()).hexdigest()
    receipts = []; cases = []; failures = []; client_port = None
    abstract = uuid.UUID('e1af8308-5d1f-11c9-91a4-08002b14a0fa').bytes_le+struct.pack('<HH', 3, 0)
    transfer = uuid.UUID('8a885d04-1ceb-11c9-9fe8-08002b104860').bytes_le+struct.pack('<I', 2)
    lookup = struct.pack('<IIII', 0, 0, 0, 1)+bytes(20)+struct.pack('<I', 500)

    def record(generation, direction, wire):
        receipts.append(dict(generation=generation, direction=direction,
                             source='127.0.0.1:'+str(client_port if direction == 0 else 135),
                             destination='127.0.0.1:'+str(135 if direction == 0 else client_port),
                             time_ns=time.time_ns(), bytes=len(wire), SHA256=hashlib.sha256(wire).hexdigest(),
                             wire_base64=base64.b64encode(wire).decode()))

    def exact(s, count):
        data = b''
        while len(data) < count:
            piece = s.recv(count-len(data))
            if not piece:
                raise EOFError('endpoint closed before a whole declared RPC PDU')
            data += piece
        return data

    def receive(s, generation, call, accepted):
        parts = []; total = 0
        for index in range(2048):
            header = exact(s, 16); size, auth, actual = struct.unpack_from('<HHI', header, 8)
            assert header[:2] == b'\x05\0' and header[4:8] == b'\x10\0\0\0' and 16 <= size <= 65535 and auth == 0 and actual == call
            wire = header+exact(s, size-16); record(generation, 1, wire)
            assert wire[2] in accepted
            assert bool(wire[3] & 1) == (index == 0)
            total += len(wire); assert total <= 1 << 20
            parts.append(wire)
            if wire[3] & 2:
                return parts
        raise AssertionError('bounded actual RPC reply fragment count exceeded')

    cap, marker = start_capture(tshark, out/'live.pcap', 'tcp port 135', 30)
    try:
        for generation in range(4):
            case = dict(generation=generation, offered_context=1 if generation == 2 else 0,
                        sent_complete_lookup=False, expected_abortive_close=True)
            with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as s:
                s.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
                s.setsockopt(socket.SOL_SOCKET, socket.SO_LINGER, struct.pack('HH', 1, 0))
                s.setsockopt(socket.IPPROTO_TCP, socket.TCP_NODELAY, 1); s.settimeout(5)
                s.bind(('127.0.0.1', client_port or 0)); s.connect(('127.0.0.1', 135))
                observed = s.getsockname()[1]
                if client_port is None:
                    client_port = observed
                assert observed == client_port
                context = case['offered_context']
                if generation != 1:
                    bind = pdu(11, 3, 1, struct.pack('<HHIB3x', 2048, 2048, 0, 1)+struct.pack('<HBB', context, 1, 0)+abstract+transfer)
                    record(generation, 0, bind); s.sendall(bind)
                    ack = receive(s, generation, 1, [12]); assert len(ack) == 1
                    body = ack[0][16:]; at = (10+int.from_bytes(body[8:10], 'little')+3)&~3
                    assert body[at] == 1 and body[at+4:at+8] == bytes(4)
                    assert body[at+8:at+24] == transfer[:16] and int.from_bytes(body[at+24:at+28], 'little') == 2
                    case['accepted_context'] = context
                first = pdu(0, 1, 2, struct.pack('<IHH', len(lookup), context, 2)+lookup[:20])
                if generation == 0:
                    record(generation, 0, first); s.sendall(first); time.sleep(.03)
                    case['outcome'] = 'RST-after-incomplete-request'
                elif generation == 1:
                    request = pdu(0, 3, 2, struct.pack('<IHH', len(lookup), 0, 2)+lookup)
                    record(generation, 0, request); s.sendall(request)
                    try:
                        reply = receive(s, generation, 2, [3, 13])
                        if reply[-1][2] == 13:
                            case['outcome'] = 'actual-unbound-context-bind-nak'
                            case['reject_reason'] = int.from_bytes(reply[-1][16:18], 'little')
                        else:
                            case['outcome'] = 'actual-unbound-context-fault'
                            case['fault_status'] = int.from_bytes(reply[-1][24:28], 'little')
                    except (EOFError, ConnectionResetError):
                        case['outcome'] = 'actual-unbound-context-connection-close'
                else:
                    request = pdu(0, 3, 2, struct.pack('<IHH', len(lookup), context, 2)+lookup)
                    record(generation, 0, request); s.sendall(request)
                    reply = receive(s, generation, 2, [2]); stub = b''.join(w[24:] for w in reply)
                    assert len(stub) >= 28
                    case.update(outcome='actual-lookup-response', sent_complete_lookup=True,
                                reply_fragments=len(reply), reply_stub_bytes=len(stub),
                                reply_stub_SHA256=hashlib.sha256(stub).hexdigest(), returned_entries=int.from_bytes(stub[20:24], 'little'))
                    if stub[:20] != bytes(20):
                        free = pdu(0, 3, 3, struct.pack('<IHH', 20, context, 4)+stub[:20])
                        record(generation, 0, free); s.sendall(free)
                        freed = receive(s, generation, 3, [2]); assert freed[-1][24:44] == bytes(20)
                        case['explicit_context_cleanup'] = True
                cases.append(case)
            time.sleep(.05)
    except Exception as exc:
        failures.append(type(exc).__name__+': '+str(exc)); raise
    finally:
        _, stderr = cap.communicate(timeout=40); (out/'capture.stderr').write_bytes(stderr)
        (out/'wire-receipts.json').write_text(json.dumps(receipts, indent=2)+'\n', encoding='utf8')
        manifest = dict(scope=__doc__, generator_SHA256=generator_digest,
                        capture_SHA256=hashlib.sha256((out/'live.pcap').read_bytes()).hexdigest(), marker_port=marker,
                        reused_client_port=client_port, target='actual local Windows EPM over loopback TCP/135',
                        cases=cases, wire_PDU_receipts=len(receipts), failures=failures,
                        authentication_or_endpoint_modification_or_discovered_service_invocation=False)
        (out/'manifest.json').write_text(json.dumps(manifest, indent=2)+'\n', encoding='utf8')
    assert cap.returncode == 0 and not failures
    print(json.dumps(manifest), flush=True)


if __name__ == '__main__':
    output, tshark = map(Path, sys.argv[1:])
    assert not any((p/'.git').exists() for p in [output.resolve(), *output.resolve().parents])
    run(output, tshark)
