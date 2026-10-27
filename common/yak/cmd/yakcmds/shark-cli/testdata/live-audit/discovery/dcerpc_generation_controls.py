"""Build declared RPC call-order controls from actual local socket receipts.

Usage: dcerpc_generation_controls.py WIRE_RECEIPTS_JSON OUTPUT_DIRECTORY
The TCP/IP carriers and call order are constructed controls, not live endpoint
behavior or capture-loss simulations. Opaque service octets come from the given
real read-only EPM lookup; raw controls and manifests must stay outside Git.
"""
import base64
import hashlib
import json
import struct
import sys
from pathlib import Path


def checksum(wire):
    if len(wire) & 1:
        wire += b'\0'
    total = sum(struct.unpack('!%dH' % (len(wire)//2), wire))
    while total >> 16:
        total = (total & 65535)+(total >> 16)
    return (~total) & 65535


def pdu(ptype, flags, call, body):
    return struct.pack('<BBBB4sHHI', 5, 0, ptype, flags, b'\x10\0\0\0', len(body)+16, 0, call)+body


def build(receipts, destination):
    source = json.loads(receipts.read_text(encoding='utf8'))
    wire = [base64.b64decode(r['wire_base64']) for r in source]
    request_parts = [w for w in wire if w[2] == 0 and w[3] & 3 != 3]
    assert len(request_parts) == 2
    first, last = request_parts
    assert first[3] == 1 and last[3] == 2
    bind = next(w for w in wire if w[2] == 11)
    ack = next(w for w in wire if w[2] == 12)
    whole_request = next(w for w in wire if w[2] == 0 and w[3] & 3 == 3 and int.from_bytes(w[12:16], 'little') == 2)
    first_case = source[0]['case']
    response = [base64.b64decode(r['wire_base64']) for r in source if r['case'] == first_case
                and base64.b64decode(r['wire_base64'])[2] == 2
                and int.from_bytes(base64.b64decode(r['wire_base64'])[12:16], 'little') == 2]
    assert response and response[0][3] & 1 and response[-1][3] & 2
    stub = b''.join(w[24:] for w in response)
    assert len(stub)+24 <= 65535, 'constructed whole-reply control requires a legal single RPC fragment'
    whole_response = pdu(2, 3, 2, struct.pack('<IHBB', len(stub), 0, 0, 0)+stub)
    bad_last = bytearray(last); bad_last[20] ^= 1; bad_last = bytes(bad_last)
    destination.mkdir(parents=True, exist_ok=False)
    cases = {
        'overlap-before-old-reply': [
            ('old-request', 0, whole_request, None, False),
            ('refused-overlap-first', 0, first, None, True),
            ('old-reply', 1, whole_response, 'old-request', False),
            ('refused-overlap-tail', 0, last, None, True),
            ('fresh-request', 0, whole_request, None, False),
            ('fresh-reply', 1, whole_response, 'fresh-request', False),
        ],
        'reply-before-any-request': [
            ('orphan-reply-first', 1, response[0], None, False),
            ('fresh-request', 0, whole_request, None, False),
            *[('orphan-reply-%d' % index, 1, part, None, False) for index, part in enumerate(response[1:], 1)],
            ('fresh-reply', 1, whole_response, 'fresh-request', False),
        ],
        'invalid-first-request-replaced': [
            ('invalid-request-first', 0, first, None, False),
            ('old-reply-first', 1, response[0], 'invalid-request-first', False),
            ('invalid-request-tail', 0, bad_last, None, True),
            ('fresh-request', 0, whole_request, None, False),
            *[('old-reply-%d' % index, 1, part, 'invalid-request-first', False) for index, part in enumerate(response[1:], 1)],
            ('fresh-reply', 1, whole_response, 'fresh-request', False),
        ],
        'known-request-late-completion': [
            ('request-first', 0, first, None, False),
            ('reply-first', 1, response[0], 'request-first', False),
            ('request-tail', 0, last, None, False),
            *[('reply-%d' % index, 1, part, 'request-first', False) for index, part in enumerate(response[1:], 1)],
        ],
    }
    for name, calls in cases.items():
        output = destination/name; output.mkdir()
        data = bytearray(struct.pack('<IHHIIII', 0xa1b23c4d, 2, 4, 0, 0, 65535, 0))
        sequence = [1000, 5000]; packets = []; labels = []

        def packet(direction, flags, payload=b''):
            address = b'\x7f\0\0\x01'; ports = [41000, 135]
            tcp = struct.pack('!HHIIBBHHH', ports[direction], ports[1-direction], sequence[direction], sequence[1-direction] if flags & 16 else 0, 0x50, flags, 65535, 0, 0)+payload
            pseudo = address+address+b'\0\x06'+struct.pack('!H', len(tcp))
            tcp = tcp[:16]+struct.pack('!H', checksum(pseudo+tcp))+tcp[18:]
            ip = struct.pack('!BBHHHBBH4s4s', 0x45, 0, 20+len(tcp), len(packets)+1, 0x4000, 64, 6, 0, address, address)
            ip = ip[:10]+struct.pack('!H', checksum(ip))+ip[12:]
            assert checksum(ip) == 0 and checksum(pseudo+tcp) == 0
            frame = b'\x02\0\0\0'+ip+tcp
            number = len(packets)+1; data.extend(struct.pack('<IIII', 1700000000, number*1000000, len(frame), len(frame))+frame)
            packets.append(dict(frame=number, direction=direction, TCP_sequence=sequence[direction], payload_bytes=len(payload), SHA256=hashlib.sha256(frame).hexdigest()))
            sequence[direction] += len(payload)+int(bool(flags & 3))
            return number

        packet(0, 2); packet(1, 18); packet(0, 16)
        for label, direction, payload, owner, refused in [('bind', 0, bind, None, False), ('bind-ack', 1, ack, 'bind', False), *calls]:
            frame = packet(direction, 24, payload); packet(1-direction, 16)
            expected_owner = owner if payload[2] in [2, 3, 12, 15] and payload[3] & 2 else None
            if name == 'invalid-first-request-replaced' and label.startswith('old-reply'):
                expected_owner = None
            labels.append(dict(label=label, frame=frame, direction=direction, wire_base64=base64.b64encode(payload).decode(),
                               wire_SHA256=hashlib.sha256(payload).hexdigest(), expected_numeric_owner_label=expected_owner,
                               original_request_at_reply_start=owner, expected_refusal=refused))
        packet(0, 17); packet(1, 16); packet(1, 17); packet(0, 16)
        capture = output/'control.pcap'; capture.write_bytes(data)
        manifest = dict(scope=__doc__, source_receipts_SHA256=hashlib.sha256(receipts.read_bytes()).hexdigest(),
                        capture_SHA256=hashlib.sha256(data).hexdigest(), physical_frames=len(packets),
                        source_reply_stub_bytes=len(stub), source_reply_stub_SHA256=hashlib.sha256(stub).hexdigest(),
                        packets=packets, PDUs=labels)
        (output/'manifest.json').write_text(json.dumps(manifest, indent=2)+'\n', encoding='utf8')
        print(json.dumps(dict(case=name, frames=len(packets), PDUs=len(labels), capture_SHA256=manifest['capture_SHA256'])), flush=True)


if __name__ == '__main__':
    receipts, destination = map(Path, sys.argv[1:])
    for path in [receipts, destination]:
        assert not any((p/'.git').exists() for p in [path.resolve(), *path.resolve().parents])
    build(receipts, destination)
