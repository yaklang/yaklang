"""Declared offline IS-IS corruption/reordering controls, never live replay.

Usage: isis_faults.py WIKI_ORACLE_JSON OUTDIR TSHARK SDK
Captures/raw references remain outside Git. Frames are derived from real P2P
PDUs; these are advertisement isolation tests, not proof of network convergence.
"""
import collections
import hashlib
import json
from pathlib import Path
import struct
import subprocess
import sys

oracle, out = map(Path, sys.argv[1:3])
tshark, sdk = sys.argv[3:5]
out.mkdir(parents=True, exist_ok=True)
rows = json.loads(oracle.read_text(encoding='utf-8-sig'))
wires = {}
source = None
for row in rows:
    ls = row['_source']['layers']
    frame = bytes.fromhex(ls['frame_raw'][0])
    typ = int(ls['isis']['isis.type'])
    n = int.from_bytes(frame[12:14], 'big')
    wires.setdefault(typ, frame[17:14+n])
    if source is None:
        source = frame


def fletcher(w, start, at):
    w = bytearray(w)
    w[at:at+2] = b'\0\0'
    a = b = 0
    for v in w[start:]:
        a = (a+v) % 255
        b = (b+a) % 255
    x = ((len(w)-at-1)*a-b) % 255 or 255
    y = (510-a-x) % 255 or 255
    w[at:at+2] = bytes((x, y))
    return bytes(w)


def sized(w, tlvs=b''):
    w = bytearray(w)+tlvs
    at = 17 if w[4] & 31 in (15, 16, 17) else 8
    w[at:at+2] = len(w).to_bytes(2, 'big')
    return fletcher(w, 12, 24) if w[4] & 31 in (18, 20) else bytes(w)


def body(w):
    return source[:12]+(len(w)+3).to_bytes(2, 'big')+b'\xfe\xfe\3'+w


hello, lsp = wires[17], wires[20]
short_hello, short_lsp = sized(hello[:20]), sized(lsp[:27])
cases = []
def add(name, frames, statuses, **kwargs):
    cases.append({'name': name, 'frames': frames, 'statuses': statuses, **kwargs})


add('original-p2p-hello', [body(hello)], ['decoded'])
add('duplicate-advertisements', [body(lsp), body(lsp)], ['decoded']*2)
changed = bytearray(lsp); changed[20:24] = (0x12345678).to_bytes(4, 'big'); changed = sized(changed)
add('reordered-sequence-numbers-remain-literal', [body(changed), body(lsp), body(changed)], ['decoded']*3, sequences=[0x12345678, int.from_bytes(lsp[20:24], 'big'), 0x12345678])
add('missing-intermediate-LSP-does-not-invent-LSDB', [body(lsp), body(changed)], ['decoded']*2)
changed = bytearray(short_hello); changed[9:15] = b'\1\2\3\4\5\6'
original_id = short_hello[9:15].hex()
original_id = '.'.join(original_id[i:i+4] for i in range(0, 12, 4))
add('same-MAC-different-SystemID-and-repair', [body(changed), body(short_hello)], ['decoded']*2, identities=['0102.0304.0506', original_id])
bad = bytearray(lsp); bad[23] ^= 1
add('invalid-active-checksum-and-full-repair', [body(bad), body(lsp)], ['malformed', 'decoded'], checksum_repair=True)
purge = bytearray(bad); purge[10:12] = b'\0\0'
add('purge-retains-literal-unverified-checksum', [body(purge)], ['decoded'], purge=True)
es_lsp = bytearray(short_lsp); es_lsp[4] = 18; es_lsp = sized(es_lsp)
broken = sized(es_lsp, b'\x89\x02r1\x03\x03\0\x80\x80')
add('complete-header-and-broken-ES-TLV', [body(broken), body(lsp)], ['malformed', 'decoded'], broken_tlv=True)
add('ES-shared-metrics-empty-ID-list', [body(sized(es_lsp, b'\x03\x04\0\x80\x80\x80'))], ['decoded'], es_counts=[0])
add('ES-two-IDs-share-one-metric-block', [body(sized(es_lsp, bytes([3,16,1,0x82,0x83,0x84,1,2,3,4,5,6,6,5,4,3,2,1])))], ['decoded'], es_counts=[2])
add('ES-different-costs-use-separate-TLVs', [body(sized(es_lsp, bytes([3,10,1,0x82,0x83,0x84,1,2,3,4,5,6,3,10,5,0x86,7,8,6,5,4,3,2,1])))], ['decoded'], es_counts=[1,1], es_costs=[1,5])
raw = body(lsp)
add('capture-cut-and-complete-repair', [(raw[:-1], len(raw)), raw], ['incomplete', 'decoded'])
bad = bytearray(short_hello); bad[18] -= 1
add('inconsistent-PDU-length-and-repair', [body(bad), body(hello)], ['malformed', 'decoded'])
for offset in (14, 15, 16, 17):
    bad = bytearray(body(hello)); bad[offset] ^= 1
    add('invalid-carrier-octet-%d' % offset, [bytes(bad), body(hello)], ['decoded'])
unknown = sized(short_hello, b'\xfa\x02\1\2')
add('unknown-extension-does-not-stick', [body(unknown), body(short_hello)], ['decoded']*2, unknown=True)
add('TLV-context-is-not-inherited', [body(sized(short_lsp, b'\xf0\x01\0')), body(short_hello)], ['decoded']*2, unknown=True)
optional = sized(short_hello, b'\x0c\x02\0\0')
add('optional-zero-is-accepted-not-verified', [body(optional)], ['decoded'], optional_zero=True)
optional = fletcher(optional, 0, len(optional)-2)
add('optional-nonzero-full-PDU-checksum', [body(optional)], ['decoded'], optional_good=True)
bad = bytearray(optional); bad[-1] ^= 1
add('optional-bad-checksum-and-repair', [body(bad), body(optional)], ['malformed', 'decoded'])
add('optional-duplicate-must-be-rejected', [body(sized(short_hello, b'\x0c\x02\0\0'*2))], ['malformed'])
add('optional-in-LSP-must-be-rejected', [body(sized(short_lsp, b'\x0c\x02\0\0'))], ['malformed'])
add('HMAC-shape-is-not-authentication-proof', [body(sized(short_hello, b'\x0a\x11\x36'+b'X'*16))], ['decoded'])
add('HMAC-short-value-is-not-complete-authentication', [body(sized(short_hello, b'\x0a\x02\x36X')), body(hello)], ['malformed', 'decoded'])
add('separate-incomplete-datagrams-are-not-reassembled', [body(hello[:10]), body(hello[10:])], ['malformed'])
for flags in (0x0b, 0x83):
    w = bytearray(short_lsp); w[26] = flags
    add('attached-default-and-partition-flag-%02x' % flags, [body(sized(w))], ['decoded'], attached=bool(flags & 8), partition=bool(flags & 128))
add('IPv4-unused-prefix-bits-remain-literal', [body(sized(short_lsp, bytes([135, 7, 0, 0, 0, 9, 9, 192, 255])))], ['decoded'], prefix='192.128.0.0/9', literal='192.255.0.0')
add('IPv6-unused-prefix-bits-remain-literal', [body(sized(short_lsp, bytes([236, 11, 0, 0, 0, 9, 0, 33, 0x20, 1, 0x0d, 0xb8, 255])))], ['decoded'], prefix='2001:db8:8000::/33', literal='2001:db8:ff00::')
add('legacy-IPv4-default-high-bit-is-distribution', [body(sized(short_lsp, bytes([128, 12, 0x81, 0x80, 0x80, 0x80, 192, 0, 2, 0, 255, 255, 255, 0])))], ['decoded'], legacy_down=True)
fragment = bytearray(lsp); fragment[19] ^= 1; fragment = sized(fragment)
add('LSP-fragments-are-not-concatenated', [body(fragment), body(lsp)], ['decoded']*2, fragments=[fragment[19], lsp[19]])
add('P2P-advertised-state-does-not-stick', [body(sized(short_hello, b'\xf0\x01\0')), body(sized(short_hello, b'\xf0\x01\2')), body(short_hello)], ['decoded']*3, adjacency=True)
composition = sized(short_hello, b'\x0a\x11\x36'+b'X'*16+b'\x0c\x02\0\0')
composition = fletcher(composition, 0, len(composition)-2)
add('Fletcher-success-is-not-HMAC-composition-acceptance', [body(composition)], ['decoded'], composition=True)
for typ in (25, 27):
    circuit = bytearray(wires[typ]); circuit[16] = 0xab
    add('SNP-type-%d-circuit-is-not-system-ID' % typ, [body(circuit)], ['decoded'], circuit=0xab, system=circuit[10:16].hex())

results = []
for case in cases:
    d = out/case['name']; d.mkdir(exist_ok=True)
    capture = d/'offline-control.pcap'
    pcap = bytearray(struct.pack('<IHHIIII', 0xa1b2c3d4, 2, 4, 0, 0, 65535, 1))
    for i, item in enumerate(case['frames']):
        frame, original = item if isinstance(item, tuple) else (item, len(item))
        pcap += struct.pack('<IIII', 1700000000, i*1000, len(frame), original)+frame
    capture.write_bytes(pcap)
    for args, name in [([tshark, '-2', '-n', '-r', str(capture), '-Y', 'isis', '-T', 'json', '-x', '--no-duplicate-keys'], 'oracle.json'),
                       ([sdk, 'replay', str(capture), '-', '--ignore-embedded-secrets'], 'events.jsonl')]:
        with (d/name).open('wb') as stdout, (d/(name+'.stderr')).open('wb') as stderr:
            subprocess.run(args, stdout=stdout, stderr=stderr, check=True, timeout=120)
    es = [json.loads(x) for x in (d/'events.jsonl').read_text().splitlines() if x.startswith('{')]
    es = [e for e in es if e['protocol'] == 'isis']
    assert [e['status'] for e in es] == case['statuses'], (case['name'], [e['status'] for e in es])
    checks = 1
    assert all(e['flow'] == 0 and not e.get('response_to') for e in es); checks += len(es)
    assert all((e.get('session') or {}).get('Authentication Verified') is not True for e in es); checks += len(es)
    if 'sequences' in case:
        assert [e['session']['Sequence Number'] for e in es] == case['sequences']; checks += len(es)
    if 'identities' in case:
        assert [e['session']['Source System ID'] for e in es] == case['identities']; checks += len(es)
    if case.get('purge'):
        assert es[0]['session']['Checksum Verified'] is False and es[0]['session']['Wire Checksum'] == int.from_bytes(purge[24:26], 'big'); checks += 2
    if case.get('checksum_repair'):
        assert es[0]['session']['Checksum Failure'] is True and es[1]['session']['Checksum Verified'] is True; checks += 2
    if case.get('broken_tlv'):
        tlvs = es[0]['session']['TLVs']
        assert tlvs[0]['Hostname'] == 'r1' and tlvs[1]['TLV Name'] == 'Malformed TLV value' and 'Neighbors' not in tlvs[1]; checks += 3
    if case.get('unknown'):
        assert es[0]['session']['Unselected Extensions'] is True and 'Unselected Extensions' not in es[1]['session']; checks += 2
    if case.get('optional_zero') or case.get('optional_good'):
        assert es[0]['session']['TLVs'][0]['Checksum Verified'] == bool(case.get('optional_good')); checks += 1
    if 'attached' in case:
        assert es[0]['session']['Attached Default Metric'] == case['attached'] and es[0]['session']['Partition Repair'] == case['partition']; checks += 2
    if 'prefix' in case:
        entry = es[0]['session']['TLVs'][0]['Reachability Entries'][0]
        assert entry['Canonical Prefix'] == case['prefix'] and entry['Prefix Address'] == case['literal']; checks += 2
    if case.get('legacy_down'):
        entry = es[0]['session']['TLVs'][0]['Reachability Entries'][0]
        assert entry['Up Down'] is True and 'Default Metric Supported' not in entry; checks += 2
    if case.get('fragments'):
        assert [int(e['session']['LSP ID'].split('-')[-1], 16) for e in es] == case['fragments']; checks += len(es)
    if case.get('adjacency'):
        assert [e['session']['TLVs'][0]['Adjacency State'] for e in es[:2]] == [0, 2] and es[2]['session'].get('TLVs') in (None, []); checks += 3
    if case.get('composition'):
        assert es[0]['session']['Checksum Verified'] is True and es[0]['session']['Authentication Verified'] is False and 'RFC3358' in es[0]['session']['Conformance Warning']; checks += 3
    if 'circuit' in case:
        m = es[0]['session']; text = m['Source System ID'].replace('.', '')
        assert m['Source Circuit ID'] == case['circuit'] and text == case['system']; checks += 2
    if 'es_counts' in case:
        rows = es[0]['session']['TLVs']; assert [v['ES Neighbor Count'] for v in rows] == case['es_counts']; checks += len(rows)
        assert all('Default Metric' not in n for v in rows for n in v.get('Neighbors') or []); checks += 1
        if 'es_costs' in case:assert [v['Shared Metrics']['Default Metric'] for v in rows] == case['es_costs']; checks += len(rows)
    with (d/'wire-audit.log').open('wb') as log:
        subprocess.run([sys.executable, '-X', 'utf8', str(Path(__file__).with_name('isis_wire_audit.py')), str(d/'oracle.json'), str(d/'events.jsonl'), str(d/'wire-audit.json')], stdout=log, stderr=subprocess.STDOUT, check=True)
    audit = json.loads((d/'wire-audit.json').read_text())
    results.append({'name': case['name'], 'capture_sha256': hashlib.sha256(pcap).hexdigest(), 'physical_frames': len(case['frames']),
                    'native_statuses': dict(collections.Counter(e['status'] for e in es)), 'direct_checks': checks,
                    'original_reference_frames': len(json.loads((d/'oracle.json').read_text())),
                    'original_reference_sha256': hashlib.sha256((d/'oracle.json').read_bytes()).hexdigest(), 'wire_field_checks': audit['checks_total'],
                    'selected_differences': audit['selected_differences'], 'wire_verified_reference_defects': audit['wire_verified_reference_defects'], 'unselected_reference': audit['unselected_reference']})
    print(case['name'], 'checks', checks, flush=True)
result = {'source_capture_oracle_sha256': hashlib.sha256(oracle.read_bytes()).hexdigest(), 'sdk_sha256': hashlib.sha256(Path(sdk).read_bytes()).hexdigest(),
          'scope': 'Declared offline controls derived from original real P2P advertisements. Independent isolation, malformed/checksum evidence and later complete observations; no live network, transport/session recovery or routing convergence claims.', 'cases': results}
(out/'summary.json').write_bytes((json.dumps(result, indent=2)+'\n').encode())
