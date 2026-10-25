"""Audit observed L2TPv3 setup against original capture bytes and TShark fields.

Usage: l2tp3_session_audit.py ORACLE_JSON EVENTS_JSONL OUTPUT_JSON
The all-frame oracle must retain -T json -x --no-duplicate-keys. Keep original
and explicitly declared Decode-As oracles as separate runs. Never normalize
their missing dissections into an automatic-recognition success. Raw captures,
oracles and exports stay outside Git; only aggregate results may be committed.
Cookie values are checked against RFC3931 receiver proposals even when the
reference dissector merely uses the advertised cookie length.
"""
import base64
import collections
import hashlib
import ipaddress
import json
from pathlib import Path
import sys

oracle, export, output = map(Path, sys.argv[1:4])
for path in (oracle, export, output):
    if any((p / '.git').exists() for p in (path.resolve(), *path.resolve().parents)):
        raise ValueError('raw evidence must remain outside Git')


def sha(w): return hashlib.sha256(w).hexdigest()
def num(v): return int(v, 0) if isinstance(v, str) and v.startswith('0x') else int(v)
def descs(v): return ([v] if isinstance(v[0], str) else v) if v else []


def fields(node, key):
    if isinstance(node, dict):
        if key in node:
            v = node[key]
            yield from v if isinstance(v, list) else [v]
        for k, v in node.items():
            if not k.endswith('_raw'):
                yield from fields(v, key)
    elif isinstance(node, list):
        for v in node:
            yield from fields(v, key)


def wire_carriers(ls):
    frame = bytes.fromhex(ls['frame_raw'][0])
    for d in descs(ls.get('ip_raw')):
        w, at = bytes.fromhex(d[0]), int(d[1])
        if len(w) < 20 or w[0] >> 4 != 4:
            continue
        h = (w[0] & 15) * 4
        end = at + int.from_bytes(w[2:4], 'big')
        if h < 20 or h > len(w) or end > len(frame) or end < at+h:
            continue
        off = at+h
        src, dst = str(ipaddress.ip_address(w[12:16])), str(ipaddress.ip_address(w[16:20]))
        udp = w[9] == 17
        if udp:
            if off+8 > end:
                continue
            u = frame[off:off+8]
            uend = off+int.from_bytes(u[4:6], 'big')
            if uend < off+8 or uend > end:
                continue
            src += ':'+str(int.from_bytes(u[:2], 'big'))
            dst += ':'+str(int.from_bytes(u[2:4], 'big'))
            off, end = off+8, uend
        elif w[9] != 115:
            continue
        yield off, frame[off:end], udp, src, dst


def control(w, udp):
    start = 0 if udp else 4
    assert udp or w[:4] == b'\0'*4
    assert len(w) >= start+12
    assert int.from_bytes(w[start+2:start+4], 'big') == len(w)-start
    avps = {}
    at = start+12
    while at < len(w):
        n = int.from_bytes(w[at:at+2], 'big') & 1023
        assert n >= 6 and at+n <= len(w)
        assert int.from_bytes(w[at+2:at+4], 'big') == 0
        typ = int.from_bytes(w[at+4:at+6], 'big')
        assert typ not in avps or typ == 59
        avps[typ] = w[at+6:at+n]
        at += n
    return avps


packets = json.loads(oracle.read_text(encoding='utf-8-sig'))
layers = {int(p['_source']['layers']['frame']['frame.number']): p['_source']['layers'] for p in packets}
physical = collections.defaultdict(list)
views = {}
for frame, ls in layers.items():
    trees = ls.get('l2tp') or []
    trees = trees if isinstance(trees, list) else [trees]
    raws = descs(ls.get('l2tp_raw'))
    for off, w, udp, src, dst in wire_carriers(ls):
        physical[(frame, sha(w))].append((w, udp, src, dst))
        for raw, tree in zip(raws, trees):
            if int(raw[1]) == off and (not udp or len(w) >= 2 and w[1] & 15 == 3):
                views[(frame, sha(w))] = tree

events = [json.loads(s) for s in export.read_text(encoding='utf-8-sig').splitlines() if s.startswith('{')]
native = [e for e in events if e.get('profile') == 'l2tp3-bounded-wire-envelope']
by_id = {e['id']: e for e in native}
checks = collections.Counter()
diffs, missing, extra, proofs, rejected = [], [], [], [], []
used = set()
owned = {}
known_sessions = {}
for e in native:
    w = base64.b64decode(e.get('raw') or '')
    refs = e['source_bytes'].get('PacketRefs') or []
    matches = []
    for ref in refs:
        matches += physical.get((ref['Number'], sha(w)), [])
    if len(matches) != 1:
        diffs.append({'pdu': e['id'], 'reason': 'whole original carrier does not uniquely establish this PDU'})
        continue
    wire, udp, src, dst = matches[0]
    owned[e['id']] = (wire, udp, src, dst)
    checks['whole_carrier_and_physical_refs'] += 1
    if (e['source'], e['destination']) != (src, dst):
        diffs.append({'pdu': e['id'], 'reason': 'carrier endpoint mismatch'})
    m = e.get('session') or {}
    if m.get('Session Setup Complete'):
        known_sessions[m['Observed Session']] = m.get('Context Proof') or []
    key = (refs[0]['Number'], sha(w))
    tree = views.get(key)
    if tree is None:
        extra.append({'frame': key[0], 'pdu': e['id'], 'reason': 'original reference view absent; retained separately'})
        continue
    used.add(key)
    for f in e.get('field_values', {}).get('values', []):
        name, got = f['name'], f['value']
        if not name.startswith('l2tp.'):
            continue
        # Raw aggregate masks are native convenience fields. The individually
        # named boolean fields below use the reference's actual field names.
        if name in {'l2tp.avp.framing_capabilities', 'l2tp.avp.bearer_capabilities', 'l2tp.avp.bearer_type', 'l2tp.avp.framing_type'}:
            continue
        want = list(fields(tree, name))
        actual = [x['value'] for x in e['field_values']['values'] if x['name'] == name]
        if isinstance(got, (int, bool)):
            want = list(map(num, want))
        if name in {'l2tp.cookie', 'l2tp.avp.assigned_cookie'}:
            want = [x.replace(':', '').lower() for x in want]
        # Compare each repeated field once.
        if f is not next(x for x in e['field_values']['values'] if x['name'] == name):
            continue
        checks['reference_field_values'] += max(len(actual), len(want))
        if want != actual:
            diffs.append({'frame': key[0], 'field': name, 'native': actual, 'reference': want})


def receiver_layout(e, proof):
    assert len(proof) == 6, 'both three-message exchanges must be proved'
    cs = []
    for p in proof:
        source = by_id[p['PDU']]
        w, udp, src, dst = owned[p['PDU']]
        assert sha(w) == p['Wire SHA256']
        assert source['source_bytes']['PacketRefs'] == p['Packet Refs']
        cs.append((control(w, udp), src, dst))
    assert [num(int.from_bytes(x[0][0], 'big')) for x in cs[:3]] == [1, 2, 3]
    mts = [int.from_bytes(x[0][0], 'big') for x in cs[3:]]
    assert mts in ([10, 11, 12], [7, 8, 9])
    rq, rp, cn = cs[3:]
    ids = [int.from_bytes(x[0][63], 'big') for x in (rq, rp)]
    assert int.from_bytes(rq[0][64], 'big') == 0
    assert int.from_bytes(rp[0][64], 'big') == ids[0]
    assert int.from_bytes(cn[0][63], 'big') in ids
    sid = int.from_bytes(owned[e['id']][0][4:8] if owned[e['id']][1] else owned[e['id']][0][:4], 'big')
    matches = [x for x in (rq, rp) if int.from_bytes(x[0][63], 'big') == sid and x[1] == e['destination']]
    assert len(matches) == 1
    a, peer, _ = matches[0]
    assert e['source'] == (rp[1] if peer == rq[1] else rq[1])
    cookie = a.get(65, b'')
    sub, seq = int.from_bytes(a.get(69, b'\0\0'), 'big'), int.from_bytes(a.get(70, b'\0\0'), 'big')
    if cn[1] == peer:
        sub = int.from_bytes(cn[0].get(69, sub.to_bytes(2, 'big')), 'big')
        seq = int.from_bytes(cn[0].get(70, seq.to_bytes(2, 'big')), 'big')
    return cookie, sub, seq, int.from_bytes(rq[0][68], 'big')


for e in native:
    m = e.get('session') or {}
    if m.get('Control') is not False:
        continue
    if m.get('Payload Decoded'):
        try:
            cookie, sub, seq, pw = receiver_layout(e, m['Context Proof'])
            w, udp, _, _ = owned[e['id']]
            at = 8 if udp else 4
            assert w[at:at+len(cookie)] == cookie
            assert base64.b64decode(m['Cookie']) == cookie
            assert m['Sublayer Type'] == sub and m['Data Sequencing'] == seq and pw in (4, 5)
            at += len(cookie)+(4 if sub == 1 else 0)
            assert base64.b64decode(m['Ethernet Bytes']) == w[at:]
            assert m['Payload Offset'] == at
            checks['six_setup_PDUs_and_receiver_layout'] += 12
            proofs.append({'frame': e['source_bytes']['PacketRefs'][0]['Number'], 'pdu': e['id'], 'session': m['Observed Session'], 'inner_sha256': sha(w[at:]), 'setup_PDUs': [p['PDU'] for p in m['Context Proof']]})
        except (AssertionError, KeyError) as err:
            diffs.append({'pdu': e['id'], 'reason': 'independent setup/layout proof failed', 'error': str(err)})
    elif 'receiver cookie or peer' in e.get('error', ''):
        for candidate in m.get('Receiver Candidates') or []:
            try:
                cookie, sub, seq, pw = receiver_layout(e, known_sessions[candidate['Observed Session']])
                w, udp, _, _ = owned[e['id']]
                at = 8 if udp else 4
                assert w[at:at+len(cookie)] != cookie
                checks['independently_proved_cookie_mismatch'] += 1
                rejected.append({'frame': e['source_bytes']['PacketRefs'][0]['Number'], 'expected_cookie': cookie.hex(), 'actual_prefix': w[at:at+len(cookie)].hex()})
            except (AssertionError, KeyError) as err:
                diffs.append({'pdu': e['id'], 'reason': 'receiver mismatch proof failed', 'error': str(err)})

for key in views.keys()-used:
    missing.append({'frame': key[0], 'sha256': key[1]})
result = {'scope': 'original whole carriers, selected TShark fields, observed setup proofs and independently checked receiver cookie/sublayer; no authentication or endpoint delivery claim', 'oracle_sha256': sha(oracle.read_bytes()), 'export_sha256': sha(export.read_bytes()), 'audit_sha256': sha(Path(__file__).read_bytes()), 'checks': dict(checks), 'checks_total': sum(checks.values()), 'native_status': dict(collections.Counter(e['status'] for e in native)), 'native_inner_protocols': dict(collections.Counter(e['protocol'] for e in events if 'L2TPv3' in (e.get('carrier_layers') or []))), 'missing': missing, 'unselected_reference_views': extra, 'differences': diffs, 'admitted_data': proofs, 'rejected_cookie_mismatches': rejected}
output.write_text(json.dumps(result, indent=2)+'\n', encoding='utf-8')
print(json.dumps({k: len(v) if isinstance(v, list) else v for k, v in result.items() if k not in {'scope', 'oracle_sha256', 'export_sha256', 'audit_sha256'}}))
assert not diffs and not missing, 'retain differences for review; never rewrite original reference counts'
