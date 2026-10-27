"""Independently certify clear RPC logical stubs after the physical wire audit.

This profile requires lossless, ordered, complete clear call fragments. It does
not infer NDR semantics, authenticate bodies, bridge TCP holes, or convert a
parent PDU's frame contributors into a claimed exact logical-stub projection.
"""
import argparse
import base64
import collections
import hashlib
import json
from pathlib import Path

from dcerpc_call_links import leaves
from mail_rtsp_export_invariants import canonical, rows, sha


def audit(events, wire_certificate, oracle):
    proof = json.loads(wire_certificate.read_text(encoding='utf8'))
    assert proof['events_sha256'] == sha(events)
    assert proof['oracle_sha256'] == sha(oracle)
    proven = {v['PDU']: v for v in proof['compared_PDUs']}
    native = [e for e in rows(events) if e['protocol'] == 'dcerpc']
    reference = {}
    for row in json.loads(oracle.read_text(encoding='utf8')):
        layers = row['_source']['layers']
        reference[int(layers['frame']['frame.number'])] = layers
    fragments = {}; pending = {}; contexts = {}; expected = {}; messages = []; links = []
    checks = 0

    def equal(a, b, why):
        nonlocal checks
        checks += 1
        assert canonical(a) == canonical(b), (why, a, b)

    def frame(e):
        # Reference attribution follows independently reconstructed contiguous
        # TCP delivery. A physically earlier continuation may be available only
        # after a later packet fills the hole; byte contributors stay unchanged.
        return proven[e['id']]['stream_prefix_delivery_frame']

    def reference_frame(number, name, expected):
        # The reference tree can repeat one generated frame field in both
        # RPC and service subtrees. Check every occurrence, requiring at least
        # one; never choose a single convenient value from conflicting views.
        observed = [int(v) for v in leaves(reference[number], name)]
        assert observed, ('required reference field', number, name)
        equal(observed, [expected]*len(observed), 'every literal '+name)

    for e in native:
        identifier = e['id']; want = 0
        assert identifier in proven, 'every selected PDU requires full original-wire proof'
        raw = base64.b64decode(e['raw']); session = e.get('session') or {}
        assert e['status'] in ['decoded', 'deferred'], 'clear complete capture profile only'
        assert raw[:2] == b'\x05\0' and raw[4:8] == b'\x10\0\0\0'
        assert int.from_bytes(raw[8:10], 'little') == len(raw)
        assert int.from_bytes(raw[10:12], 'little') == 0, 'no opaque security body'
        ptype, flags = raw[2:4]; call = int.from_bytes(raw[12:16], 'little')
        equal(session['Call ID'], call, 'literal wire Call ID')
        equal(session['PType'], ptype, 'literal wire PType')
        equal(session['PFC Flags'], flags, 'literal per-PDU flags')
        connection = canonical([e['domain'], e['flow'], sorted([e['source'], e['destination']])])
        key = (connection, call); fragment_key = (connection, e['source'], call)
        complete = flags & 3 == 3; parts = None
        if ptype in [0, 2, 3]:
            header = (40 if flags & 128 else 24) if ptype == 0 else (32 if ptype == 3 else 24)
            assert len(raw) >= header
            equal(session['Context ID'], int.from_bytes(raw[20:22], 'little'), 'wire context')
            if ptype == 0:
                equal(session['OpNum'], int.from_bytes(raw[22:24], 'little'), 'wire operation')
                context = contexts.get((connection, session['Context ID']))
                if context:
                    for context_key, value in context.items():
                        equal(session[context_key], value, 'accepted original interface context '+context_key)
                    operation = {0:'ept_insert', 1:'ept_delete', 2:'ept_lookup', 3:'ept_map', 4:'ept_lookup_handle_free', 5:'ept_inq_object', 6:'ept_mgmt_delete'}
                    equal(session['Packet Name'], 'EPM.'+operation[session['OpNum']], 'accepted UUID/version and literal operation name')
                else:
                    assert 'Interface' not in session
                if complete:
                    equal(session['Stub Bytes'], len(raw)-header, 'whole request stub')
            else:
                equal(session['Packet Name'], {2:'Response', 3:'Fault'}[ptype], 'literal response PType name')
            if not complete:
                if flags & 1:
                    assert fragment_key not in fragments, 'unique first fragment'
                    fragments[fragment_key] = []
                assert fragment_key in fragments, 'captured first fragment required'
                parts = fragments[fragment_key]
                if parts:
                    previous = parts[0][1]
                    equal(raw[2], previous[2], 'stable PType')
                    equal(raw[20:22].hex(), previous[20:22].hex(), 'stable context')
                    if ptype == 0:
                        equal(raw[22:24].hex(), previous[22:24].hex(), 'stable operation')
                        if flags & 128:
                            assert previous[3] & 128
                            equal(raw[24:40].hex(), previous[24:40].hex(), 'stable object identity')
                    if ptype == 3:
                        equal(raw[24:28].hex(), previous[24:28].hex(), 'stable fault status')
                parts.append((e, raw, header))
                equal(session['Fragment Stub Bytes'], len(raw)-header, 'per-PDU stub excludes its own call header')
                equal(session['Message ID'], parts[0][0]['id'], 'logical message identity')
                equal(session['Fragment Index'], len(parts), 'logical fragment index')
                complete = bool(flags & 2)
                if complete:
                    fragments.pop(fragment_key)
                    stub = b''.join(w[h:] for _, w, h in parts)
                    digest = hashlib.sha256(stub).hexdigest()
                    parents = [p['id'] for p, _, _ in parts]
                    sources = [{'PDU ID': p['id'], 'Stub Range': [h, len(w)], 'Stub Offset': h, 'Stub Length': len(w)-h} for p, w, h in parts]
                    message = {
                        'Message ID': parents[0], 'Complete In PDU': identifier, 'Header In PDU': parents[0],
                        'Fragment Count': len(parts), 'Stub Bytes': len(stub), 'Stub': base64.b64encode(stub).decode(),
                        'Stub Decoded': False, 'Fragment Sources': sources,
                        'Byte Source': {'Kind': 'reassembled', 'SHA256': digest, 'ParentPDUs': parents,
                                        'Packet Projection': 'not asserted; use the exact parent-PDU stub ranges'},
                    }
                    equal(session['Reassembled Message'], message, 'complete owned logical source and every parent interval')
                    equal(session['Reassembled'], True, 'completion is explicit')
                    first_frame, final_frame = frame(parts[0][0]), frame(e)
                    for parent, _, _ in parts[:-1]:
                        reference_frame(frame(parent), 'dcerpc.reassembled_in', final_frame)
                    if ptype == 0:
                        equal(session['Stub Bytes'], len(stub), 'request logical stub byte count')
                    else:
                        # This actual read-only NDR32 EPM lookup capture has
                        # these fields independently exposed by Wireshark.
                        values = list(leaves(reference[final_frame], 'epm.num_ents'))
                        if values:
                            equal(int.from_bytes(stub[20:24], 'little'), int(values[0]), 'independent NDR32 num_ents vs Wireshark')
                            equal(int.from_bytes(stub[-4:], 'little'), int(list(leaves(reference[final_frame], 'epm.rc'))[0], 0), 'independent terminal EPM status vs Wireshark')
                    messages.append(dict(first_PDU=parents[0], complete_PDU=identifier, PType=ptype, fragments=len(parts),
                                         stub_bytes=len(stub), stub_sha256=digest, first_frame=first_frame, completion_frame=final_frame,
                                         first_PDU_byte_contributors=parts[0][0]['source_bytes']['PacketRefs'],
                                         completion_PDU_byte_contributors=e['source_bytes']['PacketRefs']))
                else:
                    assert 'Reassembled Message' not in session, 'incomplete fragment cannot publish a logical message'
                    if ptype == 0:
                        equal(session['Stub Bytes'], len(raw)-header, 'request fragment stub')
        elif not complete:
            raise AssertionError('fragmented control PDU outside this profile')
        if complete:
            first = parts[0][0] if parts else e
            if ptype in [0, 11, 14]:
                assert key not in pending, 'no outstanding owner overwritten'
                pending[key] = dict(id=first['id'], source=e['source'], ptype=ptype,
                                    context=int.from_bytes(raw[20:22], 'little') if ptype == 0 else None,
                                    event=first, complete_event=e)
                if ptype in [11, 14]:
                    equal(session['Packet Name'], {11:'Bind', 14:'AlterContext'}[ptype], 'literal control PType name')
                    assert raw[24] == 1, 'this controlled EPM observation offers one context'
                    assert raw[32:48].hex() == '0883afe11f5dc91191a408002b14a0fa'
                    assert raw[48:52] == b'\x03\0\0\0'
                    pending[key]['proposal'] = (int.from_bytes(raw[28:30], 'little'), {
                        'Interface': 'EPM', 'Abstract Syntax': raw[32:48].hex(),
                        'Interface Version Major': 3, 'Interface Version Minor': 0})
            elif ptype in [2, 3, 12, 13, 15]:
                owner = pending.get(key)
                expected_type = {2: 0, 3: 0, 12: 11, 15: 14}.get(ptype)
                assert owner and owner['source'] != e['source']
                assert owner['ptype'] == expected_type or ptype == 13 and owner['ptype'] in [11, 14]
                if ptype in [2, 3]:
                    equal(owner['context'], int.from_bytes(raw[20:22], 'little'), 'response context matches original request')
                want = owner['id']; pending.pop(key)
                equal(session['Association Status'], 'matched', 'completed reply association')
                equal(session['Matched Request'], owner['event']['session']['Packet Name'], 'same original validated request operation')
                if ptype in [12, 15]:
                    at = (26+int.from_bytes(raw[24:26], 'little')+3)&~3
                    assert raw[at] == 1 and raw[at+4:at+6] == b'\0\0'
                    context, accepted = owner['proposal']
                    contexts[(connection, context)] = accepted
                if ptype in [2, 3]:
                    first_reply = parts[0][0] if parts else e
                    request_frame = frame(owner['event']); response_frame = frame(first_reply)
                    for parent, _, _ in parts or [(e, raw, 0)]:
                        reference_frame(frame(parent), 'dcerpc.request_in', request_frame)
                    reference_frame(frame(owner['event']), 'dcerpc.response_in', response_frame)
                links.append(dict(response_PDU=identifier, request_PDU=want, wire_Call_ID=call, response_PType=ptype))
        equal(e['response_to'], want, 'only complete validated reply acquires the first request PDU owner')
        values = [v for v in e['field_values']['values'] if v['name'] == 'pdu.response_to']
        equal([v['value'] for v in values], [want] if want else [], 'typed request ownership')
        if values:
            equal(values[0]['exact_value'], str(want), 'exact generated ownership value')
        expected[str(identifier)] = want
    assert not fragments and not pending, 'this complete controlled observation must finish every call'
    return dict(scope=__doc__, checks=checks, events_sha256=sha(events), wire_certificate_sha256=sha(wire_certificate),
                oracle_sha256=sha(oracle), SDK_RPC_records=len(native), messages=messages, links=links,
                positive_links=len(links), verified_response_owner_PDU_IDs=expected,
                all_numeric_links_match_original_request_ownership=True,
                logical_stubs_bound_to_exact_parent_PDU_intervals=True, NDR_service_semantics_certified=False)


if __name__ == '__main__':
    ap = argparse.ArgumentParser(description=__doc__)
    for key in ['events', 'wire-certificate', 'oracle', 'out']:
        ap.add_argument('--'+key, type=Path, required=True)
    args = ap.parse_args()
    for path in vars(args).values():
        assert not any((p/'.git').exists() for p in [path.resolve(), *path.resolve().parents])
    result = audit(args.events, args.wire_certificate, args.oracle)
    args.out.write_text(json.dumps(result, indent=2)+'\n', encoding='utf8')
    print(json.dumps({k: result[k] for k in ['checks', 'SDK_RPC_records', 'messages', 'positive_links']}))
