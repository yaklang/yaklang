"""Check declared call-order controls after original-wire byte certification.

These are constructed TCP/RPC sequences carrying real opaque service octets.
Literal Wireshark associations are retained and differences are reported, never
normalized into Yak IDs or assumed to establish an invalid reused call's owner.
"""
import argparse
import base64
import collections
import hashlib
import json
from pathlib import Path

from dcerpc_call_links import leaves
from dcerpc_fragment_sdk_invariants import verify_aliases
from mail_rtsp_export_invariants import canonical, rows, sha


def original_control_refusals(capture, manifest, with_owners=False):
    """Derive exact refusal causes from original complete control wire bytes.

    Expected labels/statuses in the manifest are not used by this derivation.
    The declared constructed profile has one complete RPC PDU per TCP payload,
    a captured SYN in both directions, no holes/overlaps and unchanged tuples.
    Original request-start frame numbers identify calls independently of Yak's
    private counters and public generated PDU namespace.
    """
    from dcerpc_wire_audit import records
    from mail_rtsp_public_wire_audit import tcp_packet
    plan = json.loads(manifest.read_text(encoding='utf8'))
    assert capture.stat().st_size <= 4 << 20 and len(plan['PDUs']) <= 2048 and len(plan['packets']) <= 8192
    assert sha(capture) == plan['capture_SHA256']
    wanted = {p['frame']: p for p in plan['PDUs']}
    pending = {}; fragments = {}; cursors = {}; refusals = {}; native_frames = set(); owners = {}
    for number, section, interface, link, frame in records(capture):
        assert link == 0 and frame[:4] == b'\x02\0\0\0' and frame[4] == 0x45
        assert len(frame) == 4+int.from_bytes(frame[6:8], 'big')
        packet = tcp_packet(bytes(12)+b'\x08\0'+frame[4:], 1, section, interface)
        assert packet is not None
        direction = 0 if packet['source'] == '127.0.0.1:41000' else 1
        assert packet['destination'] == ('127.0.0.1:135' if direction == 0 else '127.0.0.1:41000')
        assert sha_bytes(frame) == plan['packets'][number-1]['SHA256']
        if packet['syn']:
            assert direction not in cursors
            cursors[direction] = packet['sequence']
        assert direction in cursors and packet['sequence'] == cursors[direction], 'no missing/overlapping constructed TCP bytes'
        cursors[direction] += len(packet['payload'])+int(packet['syn'])+int(bool(frame[37] & 1))
        raw = packet['payload']
        if not raw:
            continue
        assert number in wanted
        assert raw == base64.b64decode(wanted[number]['wire_base64'])
        native_frames.add(number)
        owners[number] = None
        assert len(raw) >= 16 and raw[:2] == b'\x05\0' and raw[4:8] == b'\x10\0\0\0'
        assert int.from_bytes(raw[8:10], 'little') == len(raw) and raw[10:12] == b'\0\0'
        ptype, flags, call = raw[2], raw[3], int.from_bytes(raw[12:16], 'little')
        key = (direction, call); message = raw; start_frame = number; frozen = None
        cause = None; prior = None
        if flags & 1 and key in fragments:
            prior = fragments.pop(key)['start']; cause = 'duplicate-first-fragment'
        elif flags & 1 and ptype == 0 and call in pending:
            prior = pending[call]['start']; cause = 'request-start-reuses-outstanding-call-id'
        elif flags & 3 != 3:
            assert ptype in [0, 2, 3]
            header = 40 if ptype == 0 and flags & 128 else 32 if ptype == 3 else 24
            assert len(raw) >= header
            if flags & 1:
                owner = pending.get(call)
                request = fragments.get((1-direction, call))
                frozen = owner['start'] if owner else request['start'] if request and request['raw'][2] == 0 else None
                fragments[key] = dict(start=number, raw=raw, header=header, frozen=frozen)
                continue
            previous = fragments.get(key)
            if not previous:
                cause = 'continuation-without-observed-first'
            else:
                first = previous['raw']; prior = previous['start']
                if first[2] != ptype or first[20:22] != raw[20:22] or ptype == 0 and first[22:24] != raw[22:24]:
                    fragments.pop(key); cause = 'call-fragment-type-context-or-operation-drift'
                else:
                    previous['raw'] += raw[header:]
                    if not flags & 2:
                        continue
                    fragments.pop(key)
                    message, start_frame, frozen = previous['raw'], previous['start'], previous['frozen']
        if cause:
            refusals[number] = dict(reason=cause, previous_request_or_fragment_frame=prior,
                                    expected_status='context-required' if cause == 'continuation-without-observed-first' else 'malformed',
                                    raw_sha256=sha_bytes(raw))
            continue
        if ptype in [0, 11, 14]:
            assert call not in pending
            pending[call] = dict(start=start_frame, direction=direction, ptype=ptype,
                                 context=int.from_bytes(message[20:22], 'little') if ptype == 0 else None)
        elif ptype in [2, 3, 12, 15]:
            owner = pending.get(call)
            correct_type = {2:0, 3:0, 12:11, 15:14}[ptype]
            if owner and owner['direction'] != direction and owner['ptype'] == correct_type:
                if ptype in [2, 3] and (owner['context'] != int.from_bytes(message[20:22], 'little') or flags & 3 != 3 and owner['start'] != frozen):
                    continue
                owners[number] = owner['start']
                pending.pop(call)
    assert native_frames == wanted.keys() and number == plan['physical_frames']
    return (refusals, owners) if with_owners else refusals


def sha_bytes(value):
    return hashlib.sha256(value).hexdigest()


def audit(manifest, receipts, before, after, wire_certificate, oracle):
    plan = json.loads(manifest.read_text(encoding='utf8'))
    assert plan['source_receipts_SHA256'] == sha(receipts)
    certificate = json.loads(wire_certificate.read_text(encoding='utf8'))
    assert certificate['events_sha256'] == sha(after)
    assert certificate['oracle_sha256'] == sha(oracle)
    assert certificate['control_manifest_sha256'] == sha(manifest)
    assert certificate['capture_sha256'] == plan['capture_SHA256']
    wire_refusals, wire_owners = original_control_refusals(manifest.parent/'control.pcap', manifest, True)
    proven = {p['PDU'] for p in certificate['compared_PDUs']}
    original = json.loads(receipts.read_text(encoding='utf8'))
    original_wires = {base64.b64decode(r['wire_base64']) for r in original}
    source_case = original[0]['case']
    source_reply = b''.join(base64.b64decode(r['wire_base64'])[24:] for r in original
                            if r['case'] == source_case and base64.b64decode(r['wire_base64'])[2] == 2
                            and int.from_bytes(base64.b64decode(r['wire_base64'])[12:16], 'little') == 2)
    request_last = next(w for w in original_wires if w[2] == 0 and w[3] == 2)
    altered_last = bytearray(request_last); altered_last[20] ^= 1
    by_frame = {}
    aliases = 0
    for event in rows(after):
        assert event['protocol'] == 'dcerpc' and event['id'] in proven
        refs = event['source_bytes']['PacketRefs']; assert len(refs) == 1
        frame = refs[0]['Number']; assert frame not in by_frame
        assert canonical(event['field_values']['source']) == canonical(event['source_bytes'])
        assert event['field_values']['completeness'] == event['completeness']
        for name, value in [('status', event['status']), ('protocol', event['protocol']), ('pdu.length', len(base64.b64decode(event['raw']))),
                            ('source', event['source']), ('destination', event['destination'])]:
            assert [v['value'] for v in event['field_values']['values'] if v['name'] == name] == [value]
        by_frame[frame] = event; aliases += verify_aliases(event)
    before_by_frame = {}
    before_by_id = {}
    for event in rows(before):
        refs = event['source_bytes']['PacketRefs']
        if event['protocol'] == 'dcerpc' and len(refs) == 1:
            before_by_frame[refs[0]['Number']] = event
            before_by_id[event['id']] = refs[0]['Number']
    reference = {int(row['_source']['layers']['frame']['frame.number']): row['_source']['layers']
                 for row in json.loads(oracle.read_text(encoding='utf8'))}
    assert len(by_frame) == len(plan['PDUs']) == len(proven)
    labels = {p['label']: by_frame[p['frame']]['id'] for p in plan['PDUs']}
    label_frames = {p['label']: p['frame'] for p in plan['PDUs']}
    labels_by_id = {identifier: label for label, identifier in labels.items()}
    body_parts = {}; body_ids = {}; first_frames = {}
    compared = []; baseline_differences = []; reference_differences = []; reference_limits = []; messages = []
    for p in plan['PDUs']:
        event = by_frame[p['frame']]; session = event.get('session') or {}
        raw = base64.b64decode(event['raw'])
        assert raw == base64.b64decode(p['wire_base64']) and hashlib.sha256(raw).hexdigest() == p['wire_SHA256']
        assert raw in original_wires or raw == bytes(altered_last) or raw[2:4] == b'\x02\x03' and raw[24:] == source_reply
        if p['expected_refusal']:
            assert p['frame'] in wire_refusals, 'declared refusal is independently derived from original wire ordering'
            assert event['status'] in ['malformed', 'context-required', 'unsupported']
            assert session['Error Scope'] == 'message' and not session.get('Reassembled Message')
        else:
            assert p['frame'] not in wire_refusals
            assert event['status'] in ['decoded', 'deferred'], (p['label'], event['status'])
        want = labels[p['expected_numeric_owner_label']] if p['expected_numeric_owner_label'] else 0
        assert label_frames.get(p['expected_numeric_owner_label']) == wire_owners[p['frame']], 'recipe owner agrees with independent original-wire call-start identity'
        assert event['response_to'] == want, ('declared original request ownership', p['label'], want)
        if raw[2] in [2, 3, 12, 15] and raw[3] & 2:
            if want:
                assert session['Association Status'] == 'matched' and not session.get('Unmatched')
                owner = next(e for e in by_frame.values() if e['id'] == want)
                assert session['Matched Request'] == owner['session']['Packet Name']
            else:
                assert session['Unmatched'] is True and session['Context Level'] == 'partial'
                assert session['Association Status'] in ['request-not-observed-at-message-start', 'original-request-no-longer-outstanding-or-incomplete']
                assert 'Matched Request' not in session
        ownership = [v for v in event['field_values']['values'] if v['name'] == 'pdu.response_to']
        assert [v['value'] for v in ownership] == ([want] if want else [])
        if ownership:
            assert ownership[0]['exact_value'] == str(want)
        assert int.from_bytes(raw[10:12], 'little') == 0
        if raw[2] in [0, 2, 3] and not p['expected_refusal']:
            header = 40 if raw[2] == 0 and raw[3] & 128 else 32 if raw[2] == 3 else 24
            key = (p['direction'], int.from_bytes(raw[12:16], 'little'))
            if raw[3] & 1:
                body_parts[key] = []; body_ids[key] = []; first_frames[key] = p['frame']
            if raw[3] & 3 != 3:
                assert key in body_parts
                body_parts[key].append((raw, header)); body_ids[key].append(event['id'])
                if raw[3] & 2:
                    pieces = body_parts.pop(key); parents = body_ids.pop(key)
                    expected_stub = b''.join(w[h:] for w, h in pieces)
                    message = session['Reassembled Message']
                    assert base64.b64decode(message['Stub']) == expected_stub
                    assert message['Stub Bytes'] == len(expected_stub) and message['Stub Decoded'] is False
                    assert message['Message ID'] == parents[0] and message['Complete In PDU'] == event['id']
                    assert message['Byte Source']['ParentPDUs'] == parents
                    assert message['Byte Source']['SHA256'] == hashlib.sha256(expected_stub).hexdigest()
                    assert 'PacketRefs' not in message['Byte Source']
                    assert message['Fragment Sources'] == [{'PDU ID': identifier, 'Stub Range': [h, len(w)], 'Stub Offset': h, 'Stub Length': len(w)-h}
                                                           for identifier, (w, h) in zip(parents, pieces)]
                    if want:
                        assert label_frames[labels_by_id[want]] < first_frames[key]
                    messages.append(dict(complete_PDU=event['id'], first_PDU=parents[0], fragments=len(parents),
                                         stub_bytes=len(expected_stub), stub_SHA256=hashlib.sha256(expected_stub).hexdigest(), numeric_owner=want))
        baseline = before_by_frame.get(p['frame'])
        baseline_owner_frame = before_by_id.get(baseline['response_to']) if baseline else None
        expected_owner_frame = label_frames.get(p['expected_numeric_owner_label'])
        if not baseline or baseline_owner_frame != expected_owner_frame or (baseline['status'] in ['malformed', 'context-required', 'unsupported']) != p['expected_refusal']:
            baseline_differences.append(dict(label=p['label'], missing=baseline is None,
                                             baseline_status=baseline['status'] if baseline else None,
                                             baseline_owner_frame=baseline_owner_frame, expected_owner_frame=expected_owner_frame))
        observed = [int(v) for v in leaves(reference[p['frame']], 'dcerpc.request_in')]
        if raw[2] in [2, 3]:
            expected_reference = label_frames.get(p['original_request_at_reply_start'])
            if manifest.parent.name == 'known-request-late-completion' or p['label'] == 'fresh-reply':
                assert observed and expected_reference is not None and all(v == expected_reference for v in observed), 'normal/recovered reply requires every literal Wireshark request link'
            if not observed:
                reference_limits.append(dict(label=p['label'], frame=p['frame'], reason='no literal Wireshark request_in field'))
            if observed and (expected_reference is None or any(v != expected_reference for v in observed)):
                reference_differences.append(dict(label=p['label'], frame=p['frame'], literal_Wireshark_request_in=observed,
                                                 frozen_observed_request_frame=expected_reference,
                                                 reason='reference per-PDU Call ID correlation differs from the retained original call-start identity; constructed invalid/reuse control'))
        for field in ['ip.checksum.status', 'tcp.checksum.status']:
            values = list(leaves(reference[p['frame']], field))
            assert values and all(int(v) == 1 for v in values), ('reference verifies constructed carrier checksum', field, values)
        compared.append(dict(label=p['label'], PDU=event['id'], frame=p['frame'], status=event['status'], numeric_owner=want,
                             literal_Wireshark_request_in=observed))
    if manifest.parent.name == 'overlap-before-old-reply':
        assert not messages, 'the refused overlap cannot publish a completed fragment message'
    else:
        assert messages
    if manifest.parent.name == 'known-request-late-completion':
        assert not baseline_differences and not reference_differences
    else:
        assert baseline_differences, 'the corresponding old error must actually reproduce on this complete stream'
    return dict(scope=__doc__, manifest_SHA256=sha(manifest), receipts_SHA256=sha(receipts), before_SHA256=sha(before), after_SHA256=sha(after),
                wire_certificate_SHA256=sha(wire_certificate), oracle_SHA256=sha(oracle), physical_frames=plan['physical_frames'],
                compared_PDUs=compared, typed_RPC_values=aliases, completed_logical_messages=messages,
                baseline_errors=baseline_differences, literal_reference_association_differences=reference_differences,
                reference_field_limits=reference_limits,
                all_declared_call_start_owners_and_logical_bytes_correct=True, reference_associations_normalized=False,
                real_endpoint_behavior_for_controls_claimed=False)


if __name__ == '__main__':
    ap = argparse.ArgumentParser(description=__doc__)
    for name in ['manifest', 'receipts', 'before', 'after', 'wire-certificate', 'oracle', 'out']:
        ap.add_argument('--'+name, type=Path, required=True)
    a = ap.parse_args()
    for path in vars(a).values():
        assert not any((p/'.git').exists() for p in [path.resolve(), *path.resolve().parents])
    result = audit(a.manifest, a.receipts, a.before, a.after, a.wire_certificate, a.oracle)
    a.out.write_text(json.dumps(result, indent=2)+'\n', encoding='utf8')
    print(json.dumps(dict(frames=result['physical_frames'], PDUs=len(result['compared_PDUs']), baseline_errors=len(result['baseline_errors']),
                          reference_differences=len(result['literal_reference_association_differences']))))
