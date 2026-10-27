"""Verify added RPC aliases and preserve every unrelated SDK value exactly.

Common-header aliases are checked against original wire-certified bytes. Other
aliases must project the same owned session as before, or a separately certified
fragment message. This is not a blanket certificate of NDR service semantics.
"""
import argparse
import base64
import collections
import copy
import itertools
import json
from pathlib import Path

from mail_rtsp_export_invariants import canonical, rows, sha

PATHS = {
    'dcerpc.cn_call_id': ('pdu', ['Call ID']),
    'dcerpc.pkt_type': ('pdu', ['PType']),
    'dcerpc.pfc_flags': ('pdu', ['PFC Flags']),
    'dcerpc.cn_ctx_id': ('pdu', ['Context ID']),
    'dcerpc.opnum': ('pdu', ['OpNum']),
    'dcerpc.packet_name': ('context', ['Packet Name']),
    'dcerpc.message_id': ('derived', ['Message ID']),
    'dcerpc.message_complete_in': ('derived', ['Reassembled Message', 'Complete In PDU']),
    'dcerpc.message_fragments': ('derived', ['Reassembled Message', 'Fragment Count']),
    'dcerpc.reassembled_length': ('derived', ['Reassembled Message', 'Stub Bytes']),
    'dcerpc.reassembled_sha256': ('derived', ['Reassembled Message', 'Byte Source', 'SHA256']),
    'dcerpc.message_parent': ('derived', ['Reassembled Message', 'Fragment Sources', '*', 'PDU ID']),
    'dcerpc.message_stub_offset': ('derived', ['Reassembled Message', 'Fragment Sources', '*', 'Stub Offset']),
    'dcerpc.message_stub_length': ('derived', ['Reassembled Message', 'Fragment Sources', '*', 'Stub Length']),
    'dcerpc.stub_decoded': ('derived', ['Reassembled Message', 'Stub Decoded']),
}
UNSIGNED_64 = {'dcerpc.message_id', 'dcerpc.message_complete_in', 'dcerpc.reassembled_length', 'dcerpc.message_parent'}


def path_values(value, path):
    if not path:
        return [value]
    key, *tail = path
    if key == '*':
        assert isinstance(value, list)
        return [found for element in value for found in path_values(element, tail)]
    return path_values(value[key], tail) if isinstance(value, dict) and key in value else []


def verify_aliases(e):
    session = e.get('session') or {}; expected = []
    raw = base64.b64decode(e.get('raw') or '')
    if any(key in session for key in ['Call ID', 'PType', 'PFC Flags']):
        assert len(raw) >= 16
        for key, value in [('Call ID', int.from_bytes(raw[12:16], 'little')), ('PType', raw[2]), ('PFC Flags', raw[3])]:
            assert session[key] == value, 'literal original RPC common header'
        if int.from_bytes(raw[10:12], 'little') == 0 and raw[2] in [0, 2, 3] and 'Context ID' in session:
            assert session['Context ID'] == int.from_bytes(raw[20:22], 'little')
            if raw[2] == 0:
                assert session['OpNum'] == int.from_bytes(raw[22:24], 'little')
    for name, (origin, path) in PATHS.items():
        for value in path_values(session, path):
            assert type(value) in [int, str, bool], 'literal supported typed value'
            field = dict(name=name, value=value, origin=origin)
            if name in UNSIGNED_64:
                field['exact_value'] = str(value)
            expected.append(field)
    actual = [v for v in e['field_values']['values'] if v['name'].startswith('dcerpc.')]
    expected.sort(key=lambda v: v['name'])
    assert canonical(actual) == canonical(expected), ('complete typed RPC aliases and origins', e['id'])
    return len(actual)


def audit(before, after, wire_certificate, fragment_certificate=None):
    wire = json.loads(wire_certificate.read_text(encoding='utf8'))
    assert wire['events_sha256'] == sha(after)
    fragmented = None
    if fragment_certificate:
        fragmented = json.loads(fragment_certificate.read_text(encoding='utf8'))
        assert fragmented['events_sha256'] == sha(after)
        assert fragmented['wire_certificate_sha256'] == sha(wire_certificate)
        assert fragmented['logical_stubs_bound_to_exact_parent_PDU_intervals'] is True
    count = 0; aliases = 0; changed = []; exact = 0; exact_rpc = 0
    for old, new in itertools.zip_longest(rows(before), rows(after)):
        assert old is not None and new is not None, 'same ordered full SDK coverage'
        count += 1
        if old['protocol'] != 'dcerpc' or new['protocol'] != 'dcerpc':
            assert canonical(old) == canonical(new), 'all unrelated original records remain exact'
            exact += 1
            continue
        assert old['id'] == new['id'], 'same original physical PDU identity'
        aliases += verify_aliases(new)
        if canonical(old) == canonical(new):
            exact_rpc += 1
            continue
        expected = copy.deepcopy(new)
        expected['field_values']['values'] = [v for v in expected['field_values']['values'] if v['name'] not in PATHS]
        if canonical(expected) == canonical(old):
            changed.append(dict(PDU=new['id'], additions='wire/session-bound typed aliases only'))
            continue
        assert fragmented, 'actual semantic changes need a separate logical-message proof'
        assert fragmented['verified_response_owner_PDU_IDs'][str(new['id'])] == new['response_to']
        # Only these exact independently certified fragment observations may
        # change; replacing an entire Session would hide unrelated regressions.
        old_session = old.get('session') or {}; new_session = new.get('session') or {}
        changed_keys = {k for k in old_session.keys() | new_session.keys()
                        if canonical(old_session.get(k)) != canonical(new_session.get(k)) or (k in old_session) != (k in new_session)}
        permitted = {'Context ID', 'Fragment Stub Bytes', 'Message ID', 'Fragment Index', 'Reassembled Message',
                     'Stub Bytes', 'Packet Name', 'Interface', 'Abstract Syntax', 'Interface Version Major', 'Interface Version Minor'}
        assert changed_keys <= permitted and new_session.get('Fragment') is True
        for key in changed_keys:
            if key in old_session:
                expected['session'][key] = copy.deepcopy(old_session[key])
            else:
                expected['session'].pop(key)
        assert new['summary'] == f"DCE/RPC {new_session['Packet Name']} call {new_session['Call ID']}"
        expected['summary'] = old['summary']; expected['response_to'] = old['response_to']
        values = expected['field_values']['values']
        values[:] = [v for v in values if v['name'] != 'pdu.response_to']
        values.extend(copy.deepcopy(v) for v in old['field_values']['values'] if v['name'] == 'pdu.response_to')
        values.sort(key=lambda v: v['name'])
        assert canonical(expected) == canonical(old), ('all original wire fields and source metadata stay exact', new['id'])
        changed.append(dict(PDU=new['id'], additions='independently certified fragment semantics, ownership and typed aliases'))
    assert count == wire['SDK_records_with_unique_positive_IDs']
    return dict(scope=__doc__, before_sha256=sha(before), after_sha256=sha(after), wire_certificate_sha256=sha(wire_certificate),
                fragment_certificate_sha256=sha(fragment_certificate) if fragment_certificate else None,
                complete_records_compared=count, byte_equivalent_unrelated_records=exact, byte_equivalent_RPC_records=exact_rpc, added_RPC_typed_values=aliases,
                changed_RPC_records=len(changed), changes=changed, all_original_wire_fields_sources_IDs_flows_order_exact=True)


if __name__ == '__main__':
    ap = argparse.ArgumentParser(description=__doc__)
    for key in ['before', 'after', 'wire-certificate', 'out']:
        ap.add_argument('--'+key, type=Path, required=True)
    ap.add_argument('--fragment-certificate', type=Path)
    args = ap.parse_args()
    for path in vars(args).values():
        if isinstance(path, Path):
            assert not any((p/'.git').exists() for p in [path.resolve(), *path.resolve().parents])
    result = audit(args.before, args.after, args.wire_certificate, args.fragment_certificate)
    args.out.write_text(json.dumps(result, indent=2)+'\n', encoding='utf8')
    print(json.dumps({k: result[k] for k in ['complete_records_compared', 'added_RPC_typed_values', 'changed_RPC_records', 'all_original_wire_fields_sources_IDs_flows_order_exact']}))
