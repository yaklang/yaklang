"""Explicit offline RIPng controls derived from a pinned real reference frame.

Usage: ripng_faults.py WEBER_ORACLE_JSON OUTDIR TSHARK SDK
No network replay. All derived captures and raw exports stay external. Each
message has its own advisory scope; no synthetic connection/reassembly proof.
"""
import collections
import hashlib
import ipaddress
import json
from pathlib import Path
import struct
import subprocess
import sys

oracle, out = map(Path, sys.argv[1:3])
tshark, sdk = sys.argv[3:5]
out.mkdir(parents=True, exist_ok=True)
source = json.loads(oracle.read_text(encoding='utf-8-sig'))[0]['_source']['layers']
real = bytes.fromhex(source['frame_raw'][0])
wire = bytes.fromhex(source['ripng_raw'][0])
ip_at, udp_at = source['ipv6_raw'][1], source['udp_raw'][1]
assert source['ripng_raw'][1] == udp_at+8


def checksum(w):
    w += b'\0'*(len(w) % 2)
    n = sum(struct.unpack('!%dH' % (len(w)//2), w))
    while n >> 16:
        n = (n & 65535)+(n >> 16)
    return (~n) & 65535


def body(w):
    frame = bytearray(real[:udp_at+8])+w
    frame[ip_at+4:ip_at+6] = (8+len(w)).to_bytes(2, 'big')
    frame[udp_at+4:udp_at+6] = (8+len(w)).to_bytes(2, 'big')
    frame[udp_at+6:udp_at+8] = b'\0\0'
    pseudo = bytes(frame[ip_at+8:ip_at+40])+(8+len(w)).to_bytes(4, 'big')+b'\0\0\0\x11'
    frame[udp_at+6:udp_at+8] = (checksum(pseudo+frame[udp_at:]) or 65535).to_bytes(2, 'big')
    return bytes(frame)


def rte(address, tag, prefix, metric):
    return ipaddress.IPv6Address(address).packed+struct.pack('!HBB', tag, prefix, metric)


cases = []
def add(name, frames, count, **checks):
    cases.append({'name': name, 'frames': frames, 'expected_messages': count, **checks})


add('real-byte-control', [real], 1)
add('duplicate-independent-advertisements', [real, real], 2)
changed = bytearray(wire); changed[20:22] = b'\x00\x71'
add('same-tuple-reordered-tag-changes', [body(changed), real, body(changed)], 3, first_tags=[113, 0, 113])
add('lost-intermediate-advertisement', [real, body(changed)], 2)
add('capture-cut-and-full-repair', [(real[:-1], len(real)), real], 1)
bad = bytearray(wire); bad[-1] = 17
add('invalid-metric-and-full-repair', [body(bad), real], 1)
add('complete-carrier-short-entry-and-repair', [body(wire[:-1]), real], 1)
bad = bytearray(wire); bad[1] = 8
add('unknown-version-and-full-repair', [body(bad), real], 1)
add('header-alone-cannot-become-identity', [body(wire[:4]), real], 1)
advisory = wire[:4]+rte('fe80::9', 0, 0, 255)+wire[4:]
add('message-local-next-hop-does-not-stick', [body(advisory), real], 2, next_hops=['fe80::9', 'originator'])
advisory = wire[:4]+rte('fe80::9', 65535, 255, 255)+wire[4:]
add('ignored-next-hop-parameters-stay-literal', [body(advisory)], 1, ignored_parameters=True)
advisory = wire[:4]+rte('2001:db8::9', 0, 0, 255)+wire[4:]
add('non-link-local-advisory-falls-back', [body(advisory)], 1, next_hops=['originator'], advisory_warning=True)
add('whole-table-request-is-not-unreachable-route', [body(b'\x01\x01\0\0'+rte('::', 0, 0, 16))], 1, whole_request=True)
add('literal-mapped-ipv6-prefix', [body(wire[:4]+rte('::ffff:192.0.2.1', 3, 128, 1))], 1, mapped=True, authored=True)
add('adjacent-datagrams-are-not-one-message', [body(wire[:4]), body(wire[4:])], 0)

results = []
for case in cases:
    d = out/case['name']; d.mkdir(exist_ok=True)
    capture = d/'offline-control.pcap'
    w = bytearray(struct.pack('<IHHIIII', 0xa1b2c3d4, 2, 4, 0, 0, 65535, 1))
    for i, item in enumerate(case['frames']):
        frame, original = item if isinstance(item, tuple) else (item, len(item))
        w += struct.pack('<IIII', 1700000000, i*1000, len(frame), original)+frame
    capture.write_bytes(w)
    for args, name in [([tshark, '-2', '-n', '-r', str(capture), '-Y', 'ripng', '-T', 'json', '-x', '--no-duplicate-keys'], 'oracle.json'),
                       ([sdk, 'replay', str(capture), '-', '--ignore-embedded-secrets'], 'events.jsonl')]:
        with (d/name).open('wb') as stdout, (d/(name+'.stderr')).open('wb') as stderr:
            subprocess.run(args, stdout=stdout, stderr=stderr, check=True, timeout=120)
    es = [json.loads(x) for x in (d/'events.jsonl').read_text().splitlines() if x.startswith('{')]
    es = [e for e in es if e['protocol'] == 'ripng']
    assert len(es) == case['expected_messages'], (case['name'], [e['status'] for e in es])
    checks = 1
    assert all(e['flow'] == 0 and not e.get('response_to') and e['status'] == 'decoded' for e in es)
    checks += len(es)
    if 'first_tags' in case:
        assert [e['session']['Entries'][0]['Route Tag'] for e in es] == case['first_tags']; checks += len(es)
    if 'next_hops' in case:
        assert [[v for v in e['session']['Entries'] if v['Entry Type'] == 'Route'][0]['Effective Next Hop Advisory'] for e in es] == case['next_hops']; checks += len(es)
    if case.get('ignored_parameters'):
        row = es[0]['session']['Entries'][0]
        assert row['Route Tag'] == 65535 and row['Prefix Length'] == 255 and row['Ignored Next Hop Parameters']; checks += 3
    if case.get('advisory_warning'):
        assert es[0]['session']['Entries'][0]['Conformance Warning']; checks += 1
    if case.get('whole_request'):
        assert es[0]['session']['Whole Table Request'] and 'Advertised Unreachable' not in es[0]['session']['Entries'][0]; checks += 2
    if case.get('mapped'):
        assert es[0]['session']['Entries'][0]['IPv6 Prefix'] == '::ffff:192.0.2.1'; checks += 1
    with (d/'wire-audit.log').open('wb') as log:
        subprocess.run([sys.executable, '-X', 'utf8', str(Path(__file__).with_name('ripng_wire_audit.py')), str(d/'oracle.json'), str(d/'events.jsonl'), str(d/'wire-audit.json')], stdout=log, stderr=subprocess.STDOUT, check=True)
    audit = json.loads((d/'wire-audit.json').read_text())
    results.append({'name': case['name'], 'authored': case.get('authored', False), 'capture_sha256': hashlib.sha256(w).hexdigest(), 'physical_frames': len(case['frames']),
                    'native_messages': len(es), 'direct_checks': checks, 'wire_field_checks': audit['checks_total'], 'selected_differences': audit['selected_differences'],
                    'missing_selected': audit['missing_selected'], 'unselected_reference': audit['unselected_reference']})
    print(case['name'], 'messages', len(es), 'checks', checks+audit['checks_total'], flush=True)
result = {'source_frame': source['frame']['frame.number'], 'source_frame_sha256': hashlib.sha256(real).hexdigest(), 'sdk_sha256': hashlib.sha256(Path(sdk).read_bytes()).hexdigest(),
          'scope': 'Declared offline controls derived from real bytes; independent message-local advisories, not transport recovery, installed routes, delivery or peer authentication. Original reference refusals/views retained.', 'cases': results}
(out/'summary.json').write_bytes((json.dumps(result, indent=2)+'\n').encode())
