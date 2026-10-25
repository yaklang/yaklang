"""Original RIPng reference/wire/typed-field audit; no Decode-As or packet removal.

Usage: ripng_wire_audit.py ORIGINAL_TSHARK_JSON NATIVE_EVENTS_JSONL OUTPUT_JSON
Use `tshark -2 -n -Y ripng -T json -x --no-duplicate-keys`. Captures and raw
exports stay external. IPv4 port-driven views and invalid complete grammars
remain explicit reference differences, never automatic RIPng successes.
"""
import base64
import collections
import hashlib
import ipaddress
import json
from pathlib import Path
import sys

oracle, events, output = map(Path, sys.argv[1:4])


def named(v, key):
    if isinstance(v, dict):
        for k, x in v.items():
            if k == key:
                if isinstance(x, list):
                    yield from x
                else:
                    yield x
            else:
                yield from named(x, key)
    elif isinstance(v, list):
        for x in v:
            yield from named(x, key)


def numeric(v):
    return int(str(v), 16) if str(v).startswith('0x') else int(v)


def canon(k, v):
    if k == 'ripng.rte.ipv6_prefix':
        return str(ipaddress.IPv6Address(v))
    if k == 'ripng.reserved':
        return str(v).replace(':', '').lower()
    return numeric(v)


native = []
by_frame = collections.defaultdict(list)
for line in events.open(encoding='utf-8-sig'):
    if not line.startswith('{'):
        continue
    e = json.loads(line)
    if e['protocol'] == 'ripng':
        native.append(e)
        for ref in e['source_bytes'].get('PacketRefs') or []:
            by_frame[ref['Number']].append(e)

checks = collections.Counter()
missing, differences, unselected, used = [], [], [], set()
rows = json.loads(oracle.read_text(encoding='utf-8-sig'))
fields = ['ripng.cmd', 'ripng.version', 'ripng.reserved', 'ripng.rte.ipv6_prefix',
          'ripng.rte.route_tag', 'ripng.rte.prefix_length', 'ripng.rte.metric']
matched = 0
for r in rows:
    ls = r['_source']['layers']
    n = int(ls['frame']['frame.number'])
    raw = ls.get('ripng_raw')
    if not raw or isinstance(raw[0], list) or 'udp_raw' not in ls:
        unselected.append({'frame': n, 'reason': 'reference span/physical UDP boundary unavailable'})
        continue
    w = bytes.fromhex(raw[0])
    reason = None
    if 'ipv6' not in ls:
        reason = 'original reference RIPng view is carried over IPv4'
    elif len(w) < 24:
        reason = 'header alone is insufficient for automatic identity'
    elif (len(w)-4) % 20 or w[0] not in (1, 2) or w[1] != 1:
        reason = 'no complete selected v1 request/response entry grammar'
    elif any(w[i+19] != 255 and (w[i+18] > 128 or not 1 <= w[i+19] <= 16) for i in range(4, len(w), 20)):
        reason = 'route entry metric/prefix length outside selected grammar'
    if reason:
        unselected.append({'frame': n, 'reason': reason, 'wire_bytes': len(w), 'header_hex': w[:4].hex(), 'wire_sha256': hashlib.sha256(w).hexdigest(), 'native_ripng_observations': len(by_frame[n])})
        continue
    candidates = [e for e in by_frame[n] if base64.b64decode(e.get('raw') or '') == w]
    if not candidates:
        missing.append({'frame': n, 'wire_sha256': hashlib.sha256(w).hexdigest()})
        continue
    e = candidates[0]
    used.add(e['id'])
    matched += 1
    checks['exact_wire_and_physical_contributor'] += 1
    if e['status'] not in ('decoded', 'deferred') or e['flow'] != 0 or e.get('response_to', 0) != 0:
        differences.append({'frame': n, 'reason': 'independent message was refused or given an inferred flow/response identity'})
    else:
        checks['no_inferred_connection_or_query_association'] += 1
    frame = bytes.fromhex(ls['frame_raw'][0])
    udp_at = ls['udp_raw'][1]
    end = udp_at+int(ls['udp']['udp.length'])
    if frame[udp_at+8:end] != w or raw[1] != udp_at+8 or end > len(frame):
        differences.append({'frame': n, 'reason': 'original physical UDP length and reference body differ'})
    else:
        checks['physical_udp_boundary'] += 1
    observed = collections.defaultdict(list)
    for v in e['field_values']['values']:
        observed[v['name']].append(v.get('exact_value', v['value']))
    for k in fields:
        want = list(named(ls.get('ripng', {}), k))
        got = observed[k]
        checks[k] += 1
        if collections.Counter(canon(k, x) for x in want) != collections.Counter(canon(k, x) for x in got):
            differences.append({'frame': n, 'field': k, 'wireshark': want, 'native': got})

extras = [{'id': e['id'], 'status': e['status'], 'refs': e['source_bytes'].get('PacketRefs')} for e in native if e['id'] not in used]
result = {'oracle_sha256': hashlib.sha256(oracle.read_bytes()).hexdigest(), 'events_sha256': hashlib.sha256(events.read_bytes()).hexdigest(),
          'reference_frames': len(rows), 'native_events': len(native), 'matched': matched, 'checks': dict(checks), 'checks_total': sum(checks.values()),
          'selected_differences': differences, 'missing_selected': missing, 'unselected_reference': unselected, 'additional_native': extras,
          'scope': 'Original full reference, physical UDP boundaries and repeated literal fields. Observed advertisements only, no installed routes, peer/hop-limit validation or response association; invalid/unsupported reference views remain separate.'}
output.write_bytes((json.dumps(result, indent=2)+'\n').encode())
print(json.dumps({k: v for k, v in result.items() if k not in {'unselected_reference', 'scope', 'checks'}}))
raise SystemExit(bool(differences or missing or extras))
