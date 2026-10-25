"""Original IS-IS wire/field comparison, including retained reference defects.

Usage: isis_wire_audit.py TSHARK_JSON NATIVE_EVENTS_JSONL OUTPUT_JSON
Reference: tshark -2 -n -Y isis -T json -x --no-duplicate-keys
Only canonical physical root LLC PDUs (including CRC-verified mPacket express)
are selected. Inner tunnel/reconstructed views remain in unselected_reference.
No PCAP, keylog, raw event exports or original reference JSON belong in Git.
"""
import base64
import collections
import hashlib
import ipaddress
import json
from pathlib import Path
import sys
import zlib
from isis_wire_associations import verify as verify_associations

oracle, events, output = map(Path, sys.argv[1:4])


def fields(v, result=None):
    if result is None:
        result = collections.defaultdict(list)
    if isinstance(v, dict):
        for k, x in v.items():
            if k.startswith('isis.') and not k.endswith('_raw') and not isinstance(x, dict) and not (isinstance(x, list) and any(isinstance(v, (dict, list)) for v in x)):
                result[k].extend(x if isinstance(x, list) else [x])
            elif not k.endswith('_raw'):
                fields(x, result)
    elif isinstance(v, list):
        for x in v:
            fields(x, result)
    return result


def raw_fields(v, result=None):
    if result is None:
        result = collections.defaultdict(list)
    if isinstance(v, dict):
        for k, x in v.items():
            if k.startswith('isis.') and k.endswith('_raw'):
                result[k[:-4]].extend(x if x and isinstance(x[0], list) else [x])
            elif not k.endswith('_raw'):
                raw_fields(x, result)
    elif isinstance(v, list):
        for x in v:
            raw_fields(x, result)
    return result


def carrier(ls):
    b = bytes.fromhex(ls['frame_raw'][0])
    at, end, kind = 0, len(b), 'Ethernet'
    # This is an encapsulation-selected CRC proof, never an arbitrary scan for
    # 0x83 or an unconditional eight-byte offset adjustment.
    if str(ls['frame']['frame.encap_type']) == '198':
        at = int(b[:1] == b'\x50')
        while at < min(66, len(b)) and b[at] == 0x55:
            at += 1
        if at >= len(b) or b[at] != 0xd5 or len(b)-at < 19:
            return None, 'unselected mPacket delimiter/fragment'
        at += 1
        end -= 4
        if zlib.crc32(b[at:end]) != int.from_bytes(b[end:], 'little'):
            return None, 'invalid mPacket express FCS'
        kind = 'CRC-verified mPacket express'
    elif str(ls['frame']['frame.encap_type']) != '1':
        return None, 'no selected physical root Ethernet encapsulation'
    if end-at < 17:
        return None, 'incomplete physical Ethernet/LLC'
    n = int.from_bytes(b[at+12:at+14], 'big')
    p = at+14
    for depth in range(5):
        if n not in (0x8100, 0x88a8):
            break
        if depth == 4 or p+4 > end:
            return None, 'unselected VLAN depth/truncated tag'
        n = int.from_bytes(b[p+2:p+4], 'big')
        p += 4
    if n > 1500 or n < 4 or p+n > end or b[p:p+2] != b'\xfe\xfe' or b[p+2] not in (3, 0x13) or b[p+3] != 0x83:
        return None, 'no complete canonical root FE/FE UI/NLPID83 carrier'
    w = b[p+3:p+n]
    return (w, p+3, kind), None


def canon(k, x, typ):
    if typ == 'boolean':
        return x if isinstance(x, bool) else bool(int(str(x), 0))
    if typ == 'unsigned':
        if isinstance(x, int):
            return x
        return int(str(x), 16) if str(x).startswith('0x') or k.endswith('.source_circuit') else int(x)
    if typ == 'address':
        return str(ipaddress.ip_address(x))
    return str(x).lower()


def metric_proofs(w, physical):
    """Independent four-octet metric layout, tcpdump and ISO10589 semantics.

    Wireshark v4.6.9 dissect_lsp_eis_neighbors_clv_inner reads the three
    non-default values/unsupported bits from default's offset. Do not emulate.
    """
    result = collections.defaultdict(list)
    if w[4] & 31 in (18, 20):
        result['isis.lsp.default_metric'].append((bool(w[26] & 8), physical+26, 1))
    at = w[1]
    while at+2 <= len(w):
        t, n = w[at:at+2]
        body, stop = at+2, at+2+n
        if stop > len(w):
            break
        if t in (2, 3):
            step = 11 if t == 2 else 10
            start = body+int(t == 2)
            if (stop-start) % step:
                at = stop
                continue
            for pos in range(start, stop, step):
                for j, metric in enumerate(('delay', 'expense', 'error'), 1):
                    root = 'isis.lsp.eis_neighbors.'
                    supported = root+metric+'_metric_supported'
                    if metric == 'delay':
                        supported = 'isis.lsp.eis_neighbors_delay_metric.supported'
                    result[root+metric+'_metric'].append((w[pos+j] & 63, physical+pos+j, 1))
                    result[supported].append((bool(w[pos+j] & 128), physical+pos+j, 1))
        at = stop
    return result


native, by_frame, types = [], collections.defaultdict(list), {}
for line in events.open(encoding='utf-8-sig'):
    if not line.startswith('{'):
        continue
    e = json.loads(line)
    if e['protocol'] != 'isis':
        continue
    native.append(e)
    for r in e['source_bytes'].get('PacketRefs') or []:
        by_frame[r['Number']].append(e)
    for v in e['field_values']['values']:
        if v['name'].startswith('isis.'):
            x = v['value']
            types[v['name']] = 'boolean' if isinstance(x, bool) else 'unsigned' if isinstance(x, int) else 'address' if 'ipv' in v['name'] and ('prefix' in v['name'] or 'addr' in v['name']) else 'string'

checks = collections.Counter()
missing, differences, known_reference, broad_reference, unselected, unprojected, additional_fields = [], [], [], [], [], collections.Counter(), collections.Counter()
used = set()
rows = json.loads(oracle.read_text(encoding='utf-8-sig'))
for row in rows:
    ls = row['_source']['layers']
    frame = int(ls['frame']['frame.number'])
    selected, reason = carrier(ls)
    if reason:
        unselected.append({'frame': frame, 'reason': reason, 'native_observations': len(by_frame[frame])})
        continue
    w, physical, kind = selected
    fixed = {15: 27, 16: 27, 17: 20, 18: 27, 20: 27, 24: 33, 25: 33, 26: 17, 27: 17}
    typ = w[4] & 31 if len(w) >= 8 else -1
    length_at = 17 if typ in (15, 16, 17) else 8
    if (len(w) < fixed.get(typ, 65536) or w[1] != fixed.get(typ) or w[2] != 1 or w[3] not in (0, 6) or w[5] != 1
            or int.from_bytes(w[length_at:length_at+2], 'big') != len(w)
            or int(ls['frame']['frame.cap_len']) < int(ls['frame']['frame.len'])):
        unselected.append({'frame': frame, 'reason': 'complete selected fixed-header/PDU/capture grammar unavailable', 'native_statuses': [e['status'] for e in by_frame[frame]]})
        continue
    candidates = [e for e in by_frame[frame] if base64.b64decode(e.get('raw') or '') == w]
    if not candidates:
        missing.append({'frame': frame, 'wire_sha256': hashlib.sha256(w).hexdigest()})
        continue
    e = candidates[0]
    used.add(e['id'])
    checks['exact_wire_and_physical_contributor'] += 1
    if e['flow'] != 0 or e.get('response_to', 0) != 0:
        differences.append({'frame': frame, 'reason': 'advertisement was assigned an inferred connection/response'})
    else:
        checks['no_inferred_connection_or_response'] += 1
    if e['session'] is None or e['session'].get('Authentication Verified') is not False:
        differences.append({'frame': frame, 'reason': 'independent complete fixed header/authentication scope unavailable'})
    else:
        checks['fixed_header_and_unverified_authentication'] += 1
    want, spans = fields(ls), raw_fields(ls)
    got = collections.defaultdict(list)
    for v in e['field_values']['values']:
        if v['name'].startswith('isis.'):
            got[v['name']].append(v)
    proofs = metric_proofs(w, physical)
    try:
        association_checks, association_differences = verify_associations(w, e['session'])
        checks.update(association_checks)
        differences.extend({'frame': frame, **d} for d in association_differences)
    except (IndexError, KeyError, TypeError, ValueError) as error:
        differences.append({'frame': frame, 'association_error': repr(error)})
    for k in sorted(set(want) | set(got)):
        if k not in types:
            unprojected[k] += len(want[k])
            continue
        expected, observed = want[k], [v['value'] for v in got[k]]
        # Native literal checksums, optional circuit fields, and CSNP entry IDs
        # absent as filterable fields in the original reference remain explicit.
        if not expected:
            additional_fields[k] += len(observed)
            continue
        checks[k] += max(len(expected), len(observed), 1)
        equal = collections.Counter(canon(k, x, types[k]) for x in expected) == collections.Counter(canon(k, x, types[k]) for x in observed)
        ranges = [(physical+v['pdu_range']['offset'], v['pdu_range']['length']) for v in got[k] if 'pdu_range' in v]
        reference_ranges = [(x[1], x[2]) for x in spans[k] if len(x) >= 3]
        range_equal = not ranges or not reference_ranges or collections.Counter(ranges) == collections.Counter(reference_ranges)
        if ranges and len(ranges) == len(observed) == len(expected) == len(reference_ranges):
            native_pairs = collections.Counter((canon(k, v, types[k]), *span) for v, span in zip(observed, ranges))
            reference_pairs = collections.Counter((canon(k, v, types[k]), *span) for v, span in zip(expected, reference_ranges))
            range_equal = range_equal and native_pairs == reference_pairs
        if equal and not range_equal and k == 'isis.lsp.ip_reachability.ipv4_prefix' and len(ranges) == len(observed) == len(expected) == len(reference_ranges) and native_pairs == collections.Counter((canon(k, v, types[k]), lo+4, 4) for v, (lo, size) in zip(expected, reference_ranges) if size == 12):
            broad_reference.append({'frame': frame, 'field': k, 'wireshark_record_ranges': reference_ranges, 'native_literal_address_ranges': ranges, 'reason': 'Wireshark highlights the entire metric/address/mask record; native highlights its actual four-octet address'})
            range_equal = True
        if not equal or not range_equal:
            detail = {'frame': frame, 'field': k, 'wireshark': expected, 'native': observed, 'wireshark_ranges': reference_ranges, 'native_ranges': ranges}
            if k in proofs and collections.Counter((v['value'], physical+v['pdu_range']['offset'], v['pdu_range']['length']) for v in got[k]) == collections.Counter(proofs[k]):
                detail['independent_wire_layout'] = proofs[k]
                detail['reason'] = 'Wireshark4.6.9 legacy neighbor metric/unsupported-bit offset or attached-default mask defect; native follows independent wire layout'
                known_reference.append(detail)
            else:
                differences.append(detail)

refused = [{'id': e['id'], 'status': e['status'], 'refs': e['source_bytes'].get('PacketRefs')} for e in native if e['id'] not in used and not (e.get('session') or {}).get('Fixed Header Complete')]
extras = [{'id': e['id'], 'status': e['status'], 'refs': e['source_bytes'].get('PacketRefs')} for e in native if e['id'] not in used and (e.get('session') or {}).get('Fixed Header Complete')]
result = {'oracle_sha256': hashlib.sha256(oracle.read_bytes()).hexdigest(), 'events_sha256': hashlib.sha256(events.read_bytes()).hexdigest(),
          'reference_frames': len(rows), 'native_events': len(native), 'matched': len(used), 'checks': dict(checks), 'checks_total': sum(checks.values()),
          'selected_differences': differences, 'wire_verified_reference_defects': known_reference, 'broader_reference_container_ranges': broad_reference, 'missing_selected': missing,
          'unselected_reference': unselected, 'additional_native': extras, 'refused_native_without_complete_header': refused, 'reference_fields_not_projected': dict(unprojected),
          'additional_native_fields_without_original_reference_values': dict(additional_fields),
          'native_statuses': dict(collections.Counter(e['status'] for e in native)),
          'scope': 'Original physical root PDUs, exact bytes, independent advertisement scope, repeated selected typed fields and real PDU-relative ranges. No elected DIS, authenticated adjacency, assembled LSDB, installed routes or automatic tunneling inference. Original reference defects, malformed extensions and unavailable fields remain explicit.'}
output.write_bytes((json.dumps(result, indent=2)+'\n').encode())
print(json.dumps({k: v for k, v in result.items() if k not in {'checks', 'scope', 'unselected_reference', 'wire_verified_reference_defects', 'selected_differences'}}))
print('selected_differences:', len(differences), 'wire_verified_reference_defects:', len(known_reference))
raise SystemExit(bool(differences or missing or extras))
