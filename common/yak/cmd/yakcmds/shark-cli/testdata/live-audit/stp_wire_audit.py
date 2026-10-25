"""Compare original Wireshark BPDU trees against owned native wire/fields.

Usage: stp_wire_audit.py ORACLE_JSON EVENTS_JSONL OUTPUT_JSON
ORACLE_JSON is unmodified `tshark -2 -n -Y stp -T json -x
--no-duplicate-keys`. No Decode-As, removed frames or label normalization.
Raw exports/captures stay external; this emits aggregate and refusal evidence.
"""
import base64
import collections
from decimal import Decimal
import hashlib
import json
from pathlib import Path
import sys

oracle, events_path, output = map(Path, sys.argv[1:4])


def descriptors(v):
    if isinstance(v, list) and len(v) >= 3 and isinstance(v[0], str) and isinstance(v[1], int):
        return [v]
    if isinstance(v, list):
        return [d for x in v for d in descriptors(x)]
    return []


def named(v, key):
    if isinstance(v, dict):
        for k, x in v.items():
            if k == key:
                yield x
            else:
                yield from named(x, key)
    elif isinstance(v, list):
        for x in v:
            yield from named(x, key)


def fields(v, omit_mst=False):
    result = collections.defaultdict(list)
    def walk(x):
        if isinstance(x, dict):
            for k, y in x.items():
                if omit_mst and k == 'MST Extension':
                    continue
                if (k.startswith('stp.') or k.startswith('mstp.')) and not k.endswith('_raw') and not isinstance(y, dict):
                    if isinstance(y, list) and all(isinstance(z, str) for z in y):
                        result[k].extend(y)
                    elif isinstance(y, str):
                        result[k].append(y)
                elif isinstance(y, (dict, list)):
                    walk(y)
        elif isinstance(x, list):
            for y in x:
                walk(y)
    walk(v)
    return dict(result)


def carrier(frame, start, eth_offsets):
    # Independently read the literal length, VLAN tags and LLC/SNAP bytes.
    # No destination-MAC or Wireshark label is used as a selector.
    candidates = []
    for at in eth_offsets:
        if at + 14 > len(frame):
            continue
        p = at + 14
        typ = int.from_bytes(frame[p-2:p], 'big')
        depth = 0
        while typ in (0x8100, 0x88a8) and p+4 <= len(frame) and depth < 4:
            typ = int.from_bytes(frame[p+2:p+4], 'big')
            p += 4
            depth += 1
        if typ > 1500:
            continue
        if frame[p:p+3] == b'\x42\x42\x03':
            header, mode = 3, 'IEEE LLC'
        elif frame[p:p+8] == b'\xaa\xaa\x03\x00\x00\x0c\x01\x0b':
            header, mode = 8, 'Cisco SNAP'
        else:
            continue
        if p+header == start and typ >= header:
            candidates.append((at, frame[p+header:min(len(frame), p+typ)], mode, p+typ <= len(frame)))
    return max(candidates, key=lambda x: x[0]) if candidates else None


native = []
by_frame = collections.defaultdict(list)
for line in events_path.open(encoding='utf-8-sig'):
    if not line.startswith('{'):
        continue
    e = json.loads(line)
    if e['protocol'] != 'stp':
        continue
    wire = base64.b64decode(e.get('raw') or '')
    e['_wire'], e['_hash'] = wire, hashlib.sha256(wire).hexdigest()
    native.append(e)
    for ref in e['source_bytes'].get('PacketRefs') or []:
        by_frame[ref['Number']].append(e)

numeric = {
    'stp.protocol', 'stp.version', 'stp.type', 'stp.flags', 'stp.root.prio',
    'stp.root.ext', 'stp.root.cost', 'stp.bridge.prio', 'stp.bridge.ext', 'stp.port',
    'stp.flags.tc', 'stp.flags.tcack', 'stp.flags.agreement', 'stp.flags.forwarding',
    'stp.flags.learning', 'stp.flags.port_role', 'stp.flags.proposal',
    'stp.version_1_length', 'stp.pvst.tlvtype', 'stp.pvst.tlvlen', 'stp.pvst.origvlan',
    'mstp.version_3_length', 'mstp.config_format_selector', 'mstp.config_revision_level',
    'mstp.cist_internal_root_path_cost', 'mstp.cist_bridge.prio', 'mstp.cist_bridge.ext',
    'mstp.cist_remaining_hops', 'mstp.msti.flags', 'mstp.msti.priority',
    'mstp.msti.msti_id', 'mstp.msti.root_cost', 'mstp.msti.bridge_priority',
    'mstp.msti.port_priority', 'mstp.msti.remaining_hops',
}
texts = {'stp.root.hw', 'stp.bridge.hw', 'mstp.cist_bridge.hw', 'mstp.msti.root.hw', 'mstp.config_name', 'mstp.config_digest'}
seconds = {'stp.msg_age', 'stp.max_age', 'stp.hello', 'stp.forward'}


def normalize(v, key):
    if key in seconds:
        return str(Decimal(str(v)).normalize())
    if key in numeric:
        if isinstance(v, bool):
            return int(v)
        s = str(v)
        return int(s, 16) if s.startswith('0x') else int(s)
    return str(v).replace(':', '').lower() if key.endswith(('.hw', '_digest')) else str(v)


checks = collections.Counter()
views, matched, missing, refused, unselected, differences = [], [], [], [], [], []
used = set()
rows = json.loads(oracle.read_text(encoding='utf-8-sig'))
for row in rows:
    ls = row['_source']['layers']
    n = int(ls['frame']['frame.number'])
    frame = bytes.fromhex(ls['frame_raw'][0])
    eth_offsets = [d[1] for v in named(ls, 'eth_raw') for d in descriptors(v)]
    raw_views = [d for v in named(ls, 'stp_raw') for d in descriptors(v)]
    trees = [b for v in named(ls, 'stp') for b in (v if isinstance(v, list) else [v])]
    for index, d in enumerate(raw_views):
        record = {'frame': n, 'offset': d[1], 'oracle_bytes': d[2]}
        views.append(record)
        selected = carrier(frame, d[1], eth_offsets)
        if not selected:
            unselected.append({**record, 'reason': 'no exact supported Ethernet length/LLC/SNAP carrier'})
            continue
        _, wire, mode, complete = selected
        record.update(carrier=mode, carrier_complete=complete, observed_bytes=len(wire), sha256=hashlib.sha256(wire).hexdigest())
        candidates = [e for e in by_frame[n] if e['_wire'] == wire]
        if not candidates:
            missing.append(record)
            continue
        e = candidates[0]
        used.add(e['id'])
        checks['wire_bytes_and_physical_contributor'] += 1
        if e['flow'] != 0:
            differences.append({**record, 'reason': 'independent BPDU created a flow identity'})
        else:
            checks['no_inferred_connection'] += 1
        if e['status'] not in ('decoded', 'deferred'):
            refused.append({**record, 'status': e['status'], 'error': e.get('error'), 'oracle_experts': list(named(ls, '_ws.expert.message'))})
            continue
        matched.append(record)
        if wire != bytes.fromhex(d[0]):
            differences.append({**record, 'reason': 'selected native wire differs from original oracle BPDU span'})
        else:
            checks['original_oracle_wire_span'] += 1
        observed = collections.defaultdict(list)
        for v in e.get('field_values', {}).get('values', []):
            observed[v['name']].append(v.get('exact_value', v['value']))
        tree = trees[index] if index < len(trees) else {}
        want = fields(tree)
        # Wireshark reuses stp.flags.* inside MSTI records; the native STP
        # flag fields describe the common BPDU. Keep that source scope exact.
        root = fields(tree, omit_mst=True)
        for key in numeric | texts | seconds:
            expected = root.get(key, []) if key.startswith('stp.flags.') else want.get(key, [])
            if not expected:
                continue
            actual = observed.get(key, [])
            a = collections.Counter(normalize(v, key) for v in actual)
            b = collections.Counter(normalize(v, key) for v in expected)
            checks[key] += 1
            if a != b:
                differences.append({**record, 'field': key, 'wireshark': expected, 'yak': actual})

extras = [{'pdu': e['id'], 'status': e['status'], 'error': e.get('error'), 'sha256': e['_hash'], 'bytes': len(e['_wire']), 'refs': e['source_bytes']['PacketRefs']} for e in native if e['id'] not in used]
result = {
    'oracle_sha256': hashlib.sha256(oracle.read_bytes()).hexdigest(),
    'events_sha256': hashlib.sha256(events_path.read_bytes()).hexdigest(),
    'reference_frames': len(rows), 'reference_views': len(views),
    'native_events': len(native), 'native_status': dict(collections.Counter(e['status'] for e in native)),
    'native_variant_names': dict(collections.Counter(e.get('session', {}).get('Protocol Name', 'unconfirmed') for e in native)),
    'matched_decoded_views': len(matched), 'checks': dict(checks), 'checks_total': sum(checks.values()),
    'selected_field_or_wire_differences': differences, 'missing_selected_carriers': missing,
    'refused_reference_views': refused, 'unselected_reference_views': unselected, 'additional_native_events': extras,
    'scope': 'Original reference spans/typed selected fields and exact literal carrier boundaries; advertised BPDU state only, no elected topology/peer authentication/digest verification. Refusals and unsupported carriers remain separate; no label normalization.',
}
output.write_bytes((json.dumps(result, indent=2)+'\n').encode('utf-8'))
print(json.dumps({k: v for k, v in result.items() if k not in {'refused_reference_views', 'unselected_reference_views', 'additional_native_events', 'scope', 'checks'}}))
raise SystemExit(bool(differences or missing))
