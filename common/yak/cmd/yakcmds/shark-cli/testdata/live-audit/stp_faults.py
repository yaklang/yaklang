"""Declared offline BPDU boundary/order/reuse controls, not live traffic.

Usage: stp_faults.py WEBER_STP_ORACLE OUTDIR TSHARK SDK
The real-byte control retains its source frame/hash. Independent MST controls
are explicitly authored. Every generated capture and raw export stays external.
"""
import base64
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
source = next(x['_source']['layers'] for x in rows if x['_source']['layers'].get('llc', {}).get('llc.cisco_pid') == '0x010b' and 'vlan' in x['_source']['layers'])
real = bytes.fromhex(source['frame_raw'][0])
start = source['stp_raw'][1]
wire = bytes.fromhex(source['stp_raw'][0])
assert start == 26 and real[12:14] == b'\x81\x00'


def replace_body(body):
    frame = bytearray(real[:start])
    frame[16:18] = (8+len(body)).to_bytes(2, 'big')
    return bytes(frame)+body


def ieee(body):
    return real[:12]+(3+len(body)).to_bytes(2, 'big')+b'\x42\x42\x03'+body


def mst_control(two=False):
    body = bytearray(wire[:36])
    body[2] = 3
    body += (80+(16 if two else 0)).to_bytes(2, 'big')
    body += b'\x00'+b'region-a'.ljust(32, b'\0')+b'\x00\x07'+bytes([0x12])*16
    body += bytes.fromhex('0000000b800002000000000314')
    body += bytes.fromhex('7c80010200000000040000000c908013')
    if two:
        body += bytes.fromhex('3c90020200000000050000000d807012')
    return bytes(body)


cases = []
def add(name, frames, statuses, names=None, **extra):
    cases.append({'name': name, 'frames': frames, 'expected_status': statuses, 'expected_names': names, **extra})


add('real-base', [real], ['decoded'], ['RPVST+'])
add('duplicate-advertisements', [real, real], ['decoded']*2, ['RPVST+']*2)
changed = bytearray(wire)
changed[12] ^= 1  # an independently advertised root; same physical sender
add('same-sender-root-change-and-reorder', [replace_body(changed), real, replace_body(changed)], ['decoded']*3, ['RPVST+']*3, root_order=[bytes(changed[7:13]).hex(), wire[7:13].hex(), bytes(changed[7:13]).hex()])
add('lost-intermediate-advertisement', [real, replace_body(changed)], ['decoded']*2, ['RPVST+']*2)
add('snaplen-and-full-repair', [(real[:-3], len(real)), real], ['incomplete', 'decoded'], [None, 'RPVST+'])
add('complete-short-body-and-next-advertisement', [replace_body(wire[:8]), real], ['malformed', 'decoded'], [None, 'RPVST+'])
add('tcn-after-configuration-no-inheritance', [real, ieee(bytes.fromhex('00000080'))], ['decoded']*2, ['RPVST+', 'STP'], tcn_without_prior_fields=True)
bad = bytearray(real); bad[19] = 0x43
add('wrong-ssap', [bad, real], ['decoded'], ['RPVST+'])
bad = bytearray(real); bad[23] ^= 1
add('wrong-cisco-oui', [bad, real], ['decoded'], ['RPVST+'])
bad = bytearray(wire); bad[2] = 8
add('unknown-version-does-not-stick', [real, replace_body(bad), real], ['decoded', 'context-required', 'decoded'], ['RPVST+', None, 'RPVST+'])
bad = bytearray(wire[:35]); bad[3] = 0
add('version2-type0-configuration-compatibility', [ieee(bad), real], ['decoded', 'decoded'], ['STP', 'RPVST+'])
add('pvst-origin-tlv-required', [replace_body(wire[:36]), real], ['malformed', 'decoded'], [None, 'RPVST+'])
vlan = int.from_bytes(wire[-2:], 'big')
add('pvst-conflicting-origin-claims', [replace_body(wire+b'\x00\x00\x00\x02'+(vlan+1).to_bytes(2, 'big')), real], ['malformed', 'decoded'], [None, 'RPVST+'])
add('pvst-unknown-bounded-tlv', [replace_body(wire+b'\x00\x07\x00\x02\xff\xee')], ['decoded'], ['RPVST+'], incomplete_projection=True)
add('authored-standard-mst-one-instance', [ieee(mst_control())], ['decoded'], ['MSTP'], authored=True)
add('authored-standard-mst-two-instances', [ieee(mst_control(True))], ['decoded'], ['MSTP'], authored=True)
add('authored-mst-truncated-record-and-repair', [ieee(mst_control()[:-1]), ieee(mst_control())], ['malformed', 'decoded'], [None, 'MSTP'], authored=True)

results = []
for c in cases:
    d = out/c['name']; d.mkdir(exist_ok=True)
    capture = d/'offline-control.pcap'
    raw = bytearray(struct.pack('<IHHIIII', 0xa1b2c3d4, 2, 4, 0, 0, 65535, 1))
    for i, item in enumerate(c['frames']):
        frame, original = item if isinstance(item, tuple) else (item, len(item))
        raw += struct.pack('<IIII', 1700000000, i*1000, len(frame), original)+bytes(frame)
    capture.write_bytes(raw)
    with (d/'oracle.json').open('w', encoding='utf-8') as stdout, (d/'oracle.stderr').open('w', encoding='utf-8') as stderr:
        subprocess.run([tshark, '-2', '-n', '-r', str(capture), '-Y', 'stp', '-T', 'json', '-x', '--no-duplicate-keys'], stdout=stdout, stderr=stderr, check=True)
    with (d/'events.jsonl').open('w', encoding='utf-8') as stdout, (d/'events.stderr').open('w', encoding='utf-8') as stderr:
        subprocess.run([sdk, 'replay', str(capture), '-', '--ignore-embedded-secrets'], stdout=stdout, stderr=stderr, check=True)
    events = [json.loads(line) for line in (d/'events.jsonl').read_text(encoding='utf-8-sig').splitlines() if line.startswith('{')]
    events = [e for e in events if e['protocol'] == 'stp']
    status = [e['status'] for e in events]
    names = [(e.get('session') or {}).get('Protocol Name') for e in events]
    assert status == c['expected_status'], (c['name'], status, c['expected_status'])
    assert names == c['expected_names'], (c['name'], names, c['expected_names'])
    assert all(e['flow'] == 0 for e in events), 'BPDU observations cannot become a transport connection'
    checks = len(events)+2
    if 'root_order' in c:
        actual = [e['session']['Root MAC'].replace(':', '') for e in events]
        assert actual == c['root_order']; checks += len(actual)
    if c.get('tcn_without_prior_fields'):
        assert not {'Root MAC', 'Flags', 'MST Instances', 'PVST TLVs'} & events[-1]['session'].keys()
        checks += 4
    if c.get('incomplete_projection'):
        assert events[0]['completeness'] == 'limited'; checks += 1
    audit = d/'wire-audit.json'
    with (d/'wire-audit.log').open('w', encoding='utf-8') as log:
        p = subprocess.run([sys.executable, '-X', 'utf8', str(Path(__file__).with_name('stp_wire_audit.py')), str(d/'oracle.json'), str(d/'events.jsonl'), str(audit)], stdout=log, stderr=subprocess.STDOUT)
    a = json.loads(audit.read_text(encoding='utf-8-sig'))
    assert not a['selected_field_or_wire_differences'] and not a['missing_selected_carriers'], (c['name'], a['selected_field_or_wire_differences'], a['missing_selected_carriers'])
    results.append({'name': c['name'], 'authored': c.get('authored', False), 'capture_sha256': hashlib.sha256(raw).hexdigest(), 'physical_frames': len(c['frames']), 'direct_checks': checks, 'native_status': status, 'native_names': names, 'reference_frames': a['reference_frames'], 'reference_views': a['reference_views'], 'unselected_reference_views': len(a['unselected_reference_views']), 'refused_reference_views': len(a['refused_reference_views']), 'additional_native_events': len(a['additional_native_events']), 'wire_field_checks': a['checks_total'], 'differences': a['selected_field_or_wire_differences'], 'missing': a['missing_selected_carriers']})
    print(c['name'], status, 'reference', a['reference_views'], 'checks', checks+a['checks_total'], flush=True)

summary = {'scope': 'Declared offline controls; BPDU messages have independent state, not TCP-style recovery. No physical network replay, elected topology, delivery, digest or authentication claim.', 'real_source_frame': source['frame']['frame.number'], 'real_source_frame_sha256': hashlib.sha256(real).hexdigest(), 'sdk_sha256': hashlib.sha256(Path(sdk).read_bytes()).hexdigest(), 'files': results, 'direct_checks_total': sum(x['direct_checks'] for x in results), 'wire_field_checks_total': sum(x['wire_field_checks'] for x in results)}
(out/'summary.json').write_bytes((json.dumps(summary, indent=2)+'\n').encode('utf-8'))
