"""Account for every unselected original IS-IS reference view without relabeling.

Usage: isis_rejected_reference_audit.py TSHARK_JSON ROOT_AUDIT SDK_JSONL CLI_JSONL OUT
Checks a necessary v1/six-octet common-header signature in each original TVB,
including multiple views per frame. This cannot certify physical carriers, body
acceptance or future/other-ID-width implementations. Captures/raw exports stay
outside Git; no Decode-As, view stripping or normalization is performed.
"""
import collections
import hashlib
import json
from pathlib import Path
import sys

oracle, audit, sdk, cli, output = map(Path, sys.argv[1:6])
def sha(p):
    h = hashlib.sha256()
    with p.open('rb') as f:
        for b in iter(lambda: f.read(1 << 20), b''):
            h.update(b)
    return h.hexdigest()

def load(p):
    return json.loads(p.read_text(encoding='utf-8-sig'))

root = load(audit)
assert root['oracle_sha256'] == sha(oracle)
assert root['events_sha256'] == sha(sdk)
ids = {v['frame'] for v in root['unselected_reference']}
fixed = {15: 27, 16: 27, 17: 20, 18: 27, 20: 27, 24: 33, 25: 33, 26: 17, 27: 17}
groups = collections.defaultdict(collections.Counter)
views, candidates, native_confirmed, cli_confirmed = [], [], [], []
ip_bounds = collections.Counter()
for row in load(oracle):
    ls = row['_source']['layers']
    number = int(ls['frame']['frame.number'])
    if number not in ids:
        continue
    chain = ls['frame']['frame.protocols']
    group = ('IP/OSI' if chain.startswith('eth:ethertype:ip:osi:isis') else 'UDP/MPLS' if ':udp:mpls:osi:isis' in chain
             else 'GRE/PPP' if ':gre:ppp_hdlc:' in chain else 'EtherIP' if ':etherip:' in chain else 'other')
    groups[group]['frames'] += 1
    if group == 'IP/OSI':
        frame = bytes.fromhex(ls['frame_raw'][0])
        at = ls['ip_raw'][1]
        ihl, total = (frame[at] & 15)*4, int.from_bytes(frame[at+2:at+4], 'big')
        ip_bounds['frames'] += 1
        if total < ihl:
            ip_bounds['total_length_smaller_than_header'] += 1
        elif at+total > len(frame):
            ip_bounds['declared_datagram_exceeds_capture'] += 1
        else:
            ip_bounds['declared_datagram_captured'] += 1
    raw = ls.get('isis_raw') or []
    raw = raw if raw and isinstance(raw[0], list) else [raw]
    for index, span in enumerate(raw):
        if not span:
            groups[group]['raw_view_unavailable'] += 1
            continue
        w = bytes.fromhex(span[0])
        reasons = []
        if len(w) < 8:
            reasons.append('common_header_incomplete')
        else:
            if w[0] != 0x83:
                reasons.append('NLPID_mismatch')
            if w[2] != 1 or w[5] != 1:
                reasons.append('unselected_version_or_protocol_extension')
            if w[3] not in (0, 6):
                reasons.append('unselected_System_ID_width')
            typ = w[4] & 31
            if typ not in fixed:
                reasons.append('unselected_PDU_type')
            elif w[1] != fixed[typ]:
                reasons.append('type_fixed_header_length_mismatch')
        groups[group]['views'] += 1
        groups[group].update(reasons)
        detail = {'frame': number, 'view': index, 'carrier': group, 'reference_offset': span[1], 'reference_bytes': len(w),
                  'head_hex': w[:8].hex(), 'view_sha256': hashlib.sha256(w).hexdigest(), 'reasons': reasons}
        views.append(detail)
        if not reasons:
            candidates.append(detail)
for line in sdk.open(encoding='utf-8-sig'):
    if not line.startswith('{'):
        continue
    e = json.loads(line)
    if e['protocol'] == 'isis' and (e.get('session') or {}).get('Fixed Header Complete'):
        for ref in e['source_bytes'].get('PacketRefs') or []:
            if ref['Number'] in ids:
                native_confirmed.append({'frame': ref['Number'], 'id': e['id'], 'status': e['status']})
for line in cli.open(encoding='utf-8-sig'):
    if not line.startswith('{'):
        continue
    row = json.loads(line)
    if row['number'] in ids and row['protocol'] in {'IS-IS', 'ISIS', 'IS-IS Hello', 'IS-IS LSP', 'IS-IS CSNP', 'IS-IS PSNP'}:
        cli_confirmed.append({'frame': row['number'], 'protocol': row['protocol'], 'info': row['info']})
result = {'oracle_sha256': sha(oracle), 'root_audit_sha256': sha(audit), 'sdk_sha256': sha(sdk), 'cli_sha256': sha(cli),
          'script_sha256': sha(Path(__file__)), 'reference_frames': len(ids), 'original_views': len(views),
          'groups': {k: dict(v) for k, v in groups.items()}, 'IP_OSI_physical_bounds': dict(ip_bounds),
          'candidate_common_signatures': candidates, 'unexpected_confirmed_native': native_confirmed, 'unexpected_confirmed_CLI': cli_confirmed,
          'views': views, 'scope': __doc__}
output.write_bytes((json.dumps(result, indent=2)+'\n').encode())
print(json.dumps({k: v for k, v in result.items() if k not in {'views', 'scope'}}, indent=2))
raise SystemExit(bool(candidates or native_confirmed or cli_confirmed or sum(v['frames'] for v in groups.values()) != len(ids)))
