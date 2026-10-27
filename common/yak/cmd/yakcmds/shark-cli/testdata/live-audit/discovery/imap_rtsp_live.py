"""Capture owned loopback IMAP/RTSP traffic using stdlib clients and sockets.

Usage: imap_rtsp_live.py OUTPUT_DIR TSHARK
The servers are independent test endpoints, not real mailbox/Apple products.
X-AUDIT is explicitly a transport exercise, not a registered SASL mechanism.
PCAPs, wire receipts and manifests must stay outside every Git checkout.
"""
import base64
import hashlib
import imaplib
import json
import secrets
import socket
import subprocess
import sys
import threading
import time
from pathlib import Path

from capture_ready import start_capture

out = Path(sys.argv[1]).resolve()
assert not any((p / '.git').exists() for p in [out, *out.parents])
out.mkdir(exist_ok=False, parents=True)
nonce = secrets.token_hex(12)
wire_receipts, operations, failures = [], [], []
lock = threading.Lock()
dummy_plain = b'\0dummy\0secret'


def listener():
    s = socket.socket()
    s.bind(('127.0.0.1', 0))
    s.listen(8)
    s.settimeout(20)
    return s


def record(case, role, wire, peer):
    with lock:
        wire_receipts.append(dict(case=case, direction=role, server_port=peer,
                                  bytes=len(wire), sha256=hashlib.sha256(wire).hexdigest(),
                                  wire_base64=base64.b64encode(wire).decode(), time_ns=time.time_ns()))


def send(s, wire, case, role, peer, fragmented=False):
    record(case, role, wire, peer)
    if fragmented:
        for offset in range(0, len(wire), 3):
            s.sendall(wire[offset:offset + 3])
            time.sleep(.002)
    else:
        s.sendall(wire)


def read_line(file, case, role, peer):
    wire = file.readline(65537)
    assert wire and len(wire) <= 65536 and wire.endswith(b'\r\n'), 'incomplete/oversized test line'
    record(case, role, wire, peer)
    return wire


def imap_server(listen, cases):
    try:
        for case in cases:
            s, _ = listen.accept()
            s.settimeout(10)
            s.setsockopt(socket.IPPROTO_TCP, socket.TCP_NODELAY, 1)
            port = s.getsockname()[1]
            file = s.makefile('rb')
            emit = lambda wire: send(s, wire, case, 's2c', port, True)
            emit(b'* OK IMAP4rev1 owned audit endpoint\r\n')
            while True:
                line = read_line(file, case, 'c2s', port)
                words = line[:-2].split(b' ')
                assert len(words) >= 2
                tag, command = words[:2]
                if command == b'CAPABILITY':
                    emit(b'* CAPABILITY IMAP4rev1 AUTH=PLAIN SASL-IR NAMESPACE IDLE\r\n' + tag + b' OK capability\r\n')
                elif command == b'AUTHENTICATE':
                    mechanism = words[2]
                    if mechanism == b'X-AUDIT':
                        for challenge, expected in [(b'audit-one', b'response-one'), (b'audit-two', b'response-two')]:
                            emit(b'+ ' + base64.b64encode(challenge) + b'\r\n')
                            response = read_line(file, case, 'c2s', port)
                            assert base64.b64decode(response[:-2], validate=True) == expected
                        emit(tag + b' NO transport exercise only\r\n')
                        operations.append(dict(case=case, operation='declared-X-AUDIT-transport-only', rounds=2, result='NO'))
                        continue
                    assert mechanism == b'PLAIN'
                    response = words[3] + b'\r\n' if len(words) == 4 and words[3] != b'=' else None
                    if response is None:
                        emit(b'+ \r\n')
                        response = read_line(file, case, 'c2s', port)
                    if response == b'*\r\n':
                        emit(tag + b' NO canceled\r\n')
                        operations.append(dict(case=case, operation='PLAIN-cancel', result='NO'))
                        continue
                    try:
                        decoded = base64.b64decode(response[:-2], validate=True)
                    except ValueError:
                        emit(tag + b' BAD invalid base64\r\n')
                        operations.append(dict(case=case, operation='PLAIN-invalid-value', result='BAD'))
                        continue
                    assert decoded == dummy_plain, 'dummy credentials did not match'
                    emit(tag + b' OK accepted dummy credentials\r\n')
                    operations.append(dict(case=case, operation='PLAIN', result='OK', payload_sha256=hashlib.sha256(decoded).hexdigest()))
                elif command.upper() == b'NAMESPACE':
                    emit(b'* NAMESPACE ((\"\" \"/\")) NIL ((\"#shared/\" \"/\" \"X-AUDIT\" (\"one\" \"two\")))\r\n' + tag + b' OK namespace\r\n')
                    operations.append(dict(case=case, operation='NAMESPACE', result='OK'))
                elif command == b'IDLE':
                    emit(b'* 3 EXISTS\r\n+ idling\r\n* 4 EXISTS\r\n')
                    assert read_line(file, case, 'c2s', port) == b'DONE\r\n'
                    emit(tag + b' OK idle complete\r\n')
                    operations.append(dict(case=case, operation='IDLE-DONE', result='OK', untagged_updates=2))
                elif command == b'NOOP':
                    emit(tag + b' OK noop\r\n')
                    operations.append(dict(case=case, operation='NOOP', result='OK'))
                elif command == b'LOGOUT':
                    # imaplib may close immediately after BYE. Submit the whole
                    # completion together rather than writing after that close.
                    send(s, b'* BYE audit complete\r\n' + tag + b' OK logout\r\n', case, 's2c', port)
                    break
                else:
                    raise AssertionError(('unexpected IMAP command', command))
            file.close()
            s.close()
    except BaseException as exc:
        failures.append(('imap-server', repr(exc)))
    finally:
        listen.close()


def raw_imap(port, case, initial=False):
    s = socket.create_connection(('127.0.0.1', port), timeout=10)
    s.setsockopt(socket.IPPROTO_TCP, socket.TCP_NODELAY, 1)
    file = s.makefile('rb')
    def get():
        value = file.readline(65537)
        assert value and value.endswith(b'\r\n')
        return value
    def put(value):
        s.sendall(value)
    assert get().startswith(b'* OK')
    if initial:
        put(b'a AUTHENTICATE PLAIN ' + base64.b64encode(dummy_plain) + b'\r\n')
        assert get().startswith(b'a OK')
    else:
        put(b'a AUTHENTICATE PLAIN\r\n')
        assert get() == b'+ \r\n'
        put(b'%%%invalid\r\n')
        assert get().startswith(b'a BAD')
        put(b'a AUTHENTICATE X-AUDIT\r\n')
        for challenge, response in [(b'audit-one', b'response-one'), (b'audit-two', b'response-two')]:
            assert base64.b64decode(get()[2:-2], validate=True) == challenge
            put(base64.b64encode(response) + b'\r\n')
        assert get().startswith(b'a NO')
        put(b'a AUTHENTICATE PLAIN =\r\n')
        assert get() == b'+ \r\n'
        put(base64.b64encode(dummy_plain) + b'\r\n')
        assert get().startswith(b'a OK')
    put(b'n NAMESPACE\r\nm NAMESPACE\r\n')
    assert get().startswith(b'* NAMESPACE') and get().startswith(b'n OK')
    assert get().startswith(b'* NAMESPACE') and get().startswith(b'm OK')
    put(b'a IDLE\r\n')
    assert get() == b'* 3 EXISTS\r\n' and get() == b'+ idling\r\n' and get() == b'* 4 EXISTS\r\n'
    put(b'DONE\r\n')
    assert get().startswith(b'a OK')
    put(b'a NOOP\r\n')
    assert get().startswith(b'a OK')
    put(b'a LOGOUT\r\n')
    assert get().startswith(b'* BYE') and get().startswith(b'a OK')
    file.close()
    s.close()


def rtsp_server(listen):
    try:
        s, _ = listen.accept()
        s.settimeout(10)
        s.setsockopt(socket.IPPROTO_TCP, socket.TCP_NODELAY, 1)
        port = s.getsockname()[1]
        file = s.makefile('rb')
        for _ in range(6):
            lines = []
            while not lines or lines[-1] != b'\r\n':
                line = file.readline(65537)
                assert line and line.endswith(b'\r\n')
                lines.append(line)
            first = lines[0][:-2].split(b' ')
            headers = dict(line[:-2].split(b': ', 1) for line in lines[1:-1])
            body = file.read(int(headers.get(b'Content-Length', b'0')))
            wire = b''.join(lines) + body
            record('rtsp-flush-reuse', 'c2s', wire, port)
            assert first[2] == b'RTSP/1.0'
            method, seq = first[0].decode(), int(headers[b'CSeq'])
            if method == 'FLUSH':
                assert headers[b'RTP-Info'].startswith(b'seq=')
            elif method == 'SET_PARAMETER':
                assert body == (nonce + '\r\nbody').encode()
            else:
                assert method == 'OPTIONS'
            reply = b'RTSP/1.0 200 OK\r\nCSeq: ' + headers[b'CSeq'] + b'\r\nContent-Length: 0\r\n\r\n'
            send(s, reply, 'rtsp-flush-reuse', 's2c', port, True)
            operations.append(dict(case='rtsp-flush-reuse', operation=method, cseq=seq, body_bytes=len(body), result=200))
        file.close()
        s.close()
    except BaseException as exc:
        failures.append(('rtsp-server', repr(exc)))
    finally:
        listen.close()


imap_listen, rtsp_listen = listener(), listener()
imap_port, rtsp_port = imap_listen.getsockname()[1], rtsp_listen.getsockname()[1]
cases = ['imaplib-plain', 'imaplib-cancel-retry', 'raw-tag-reuse-multiround', 'raw-sasl-ir']
workers = [threading.Thread(target=imap_server, args=(imap_listen, cases)), threading.Thread(target=rtsp_server, args=(rtsp_listen,))]
for worker in workers:
    worker.start()
cap, marker_port = start_capture(sys.argv[2], out / 'live.pcap', f'tcp port {imap_port} or tcp port {rtsp_port}', 35)
try:
    for case in cases[:2]:
        client = imaplib.IMAP4('127.0.0.1', imap_port, timeout=10)
        if case == 'imaplib-cancel-retry':
            try:
                client.authenticate('PLAIN', lambda _: None)
                raise AssertionError('cancellation unexpectedly succeeded')
            except imaplib.IMAP4.error:
                pass
        assert client.authenticate('PLAIN', lambda _: dummy_plain)[0] == 'OK'
        assert client.namespace()[0] == 'OK'
        assert client.noop()[0] == 'OK'
        assert client.logout()[0] == 'BYE'
    raw_imap(imap_port, cases[2])
    raw_imap(imap_port, cases[3], True)
    s = socket.create_connection(('127.0.0.1', rtsp_port), timeout=10)
    file = s.makefile('rb')
    requests = []
    for method, seq in [('OPTIONS', 1), ('FLUSH', 7), ('FLUSH', 7), ('FLUSH', 8), ('FLUSH', 9), ('SET_PARAMETER', 10)]:
        body = (nonce + '\r\nbody').encode() if method == 'SET_PARAMETER' else b''
        wire = f'{method} rtsp://example.test/audio RTSP/1.0\r\nCSeq: {seq}\r\n'.encode()
        if method == 'FLUSH':
            wire += f'RTP-Info: seq={seq};rtptime={4294967295-seq}\r\n'.encode()
        requests.append(wire + f'Content-Length: {len(body)}\r\n\r\n'.encode() + body)
    for position, request in enumerate(requests):
        if position == 3:
            s.sendall(requests[3] + requests[4])  # Two complete outstanding CSeq owners.
        elif position != 4:
            s.sendall(request)
        reply = []
        while not reply or reply[-1] != b'\r\n':
            line = file.readline(65537)
            assert line
            reply.append(line)
        assert reply[0] == b'RTSP/1.0 200 OK\r\n'
    file.close()
    s.close()
    for worker in workers:
        worker.join(10)
        assert not worker.is_alive()
    assert not failures, failures
finally:
    stdout, stderr = cap.communicate(timeout=40)
    (out / 'capture.stderr').write_bytes(stderr)
    (out / 'wire-receipts.json').write_text(json.dumps(wire_receipts, indent=2), encoding='utf8')
    manifest = dict(generator_sha256=hashlib.sha256(Path(__file__).read_bytes()).hexdigest(), nonce=nonce,
                    capture_sha256=hashlib.sha256((out / 'live.pcap').read_bytes()).hexdigest(),
                    marker_port=marker_port, imap_port=imap_port, rtsp_port=rtsp_port,
                    client_implementation='Python stdlib imaplib plus explicitly declared socket controls',
                    server_implementation='independent owned test endpoints; no production mailbox or Apple device claim',
                    operations=operations, failures=failures)
    (out / 'manifest.json').write_text(json.dumps(manifest, indent=2), encoding='utf8')
assert cap.returncode == 0 and not failures
print(json.dumps(dict(operations=len(operations), wire_receipts=len(wire_receipts), capture_sha256=manifest['capture_sha256'])))
