"""Bridge old/new same-capture TLS SDK and CLI exports without rewriting IDs.

All existing keys, scalar types, array members and their order are exact. New
values are limited to the named TLS metadata contract. Source metadata is
independently checked with the frozen literal-wire/Wireshark baseline verifier;
control observations are restricted by their exact retained-frame ledger and
the ClientHello/ServerHello actually admitted in each original peer generation.
This is a composition bridge, not a new proof of untested TLS features.
"""
import argparse
import base64
import collections
import copy
import hashlib
import json
from pathlib import Path

from capture_clock_wire_audit import physical
from tls_resumption_audit import wire_connections, rows
from tls_resumption_metadata_audit import (
    CONTEXT_PATHS, DERIVED_FIELDS, FORBIDDEN_ALIASES, WIRE_NAMESPACE,
    exact, inspect as inspect_metadata, message_fields,
)

MESSAGE_KEYS = {'Session ID', 'PSK Offer', 'PSK Key Exchange Modes',
                'PSK Key Exchange Modes Length', 'Session Ticket Offer',
                'PSK Selection', 'New Session Ticket'}
MESSAGE_TYPES = {'Session ID': {1, 2}, 'PSK Offer': {1},
                 'PSK Key Exchange Modes': {1}, 'PSK Key Exchange Modes Length': {1},
                 'Session Ticket Offer': {1}, 'PSK Selection': {2}, 'New Session Ticket': {4}}
ALIASES = WIRE_NAMESPACE | {'tls.session.' + name for name in CONTEXT_PATHS}


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def expected_origin(name):
    return 'context' if name.startswith('tls.session.') else 'derived' if name in DERIVED_FIELDS else 'pdu'


def field_map(snapshot):
    out = collections.defaultdict(list)
    for item in snapshot.get('values') or []:
        out[item['name']].append(item['value'])
    return dict(out)


def metadata_aliases(messages, context):
    out = collections.defaultdict(list)
    for message in messages:
        meta = {k: v for k, v in message.items() if k in MESSAGE_KEYS}
        for key, values in message_fields(message['Type'], meta).items():
            out[key].extend(values)
    for key, path in CONTEXT_PATHS.items():
        value = context
        for part in path:
            if not isinstance(value, dict) or part not in value:
                break
            value = value[part]
        else:
            out['tls.session.' + key].append(value)
    return dict(out)


def compare_existing(before, after, kind, path=(), tls=False, message_type=None, counts=None):
    """Only explicitly located new TLS keys/aliases may extend old objects."""
    counts = counts if counts is not None else collections.Counter()
    assert type(before) is type(after), (kind, path, 'value type changed')
    if isinstance(before, dict):
        for key in before:
            assert key in after, (kind, path, 'old key deleted', key)
        for key in after.keys() - before.keys():
            allowed = False
            if kind == 'SDK' and tls:
                allowed = len(path) == 2 and path[1] in ['fields', 'session'] and key == 'Session Establishment'
                if len(path) == 4 and path[1] in ['fields', 'session'] and path[2] == 'Handshake Messages':
                    allowed = key in MESSAGE_KEYS and message_type in MESSAGE_TYPES[key]
            if kind == 'CLI' and tls and len(path) == 4 and path[1] in ['pdus', 'published_pdus'] and path[3] == 'fields':
                allowed = key in ALIASES
            assert allowed, (kind, path, 'noncontract key added', key)
            counts['new_map_keys'] += 1
        for key in before:
            compare_existing(before[key], after[key], kind, path + (key,), tls, message_type, counts)
    elif isinstance(before, list):
        if len(path) >= 2 and path[-2:] == ('field_values', 'values'):
            old_names = {v['name'] for v in before}
            cursor = 0
            for item in after:
                if item['name'] not in old_names:
                    assert tls and item['name'] in ALIASES, (kind, path, 'noncontract alias added', item['name'])
                    assert set(item) == {'name', 'value', 'origin'}, (kind, path, 'new alias schema')
                    assert item['origin'] == expected_origin(item['name'])
                    counts['new_alias_occurrences'] += 1
                else:
                    assert cursor < len(before)
                    exact(item, before[cursor], kind + '/' + '/'.join(map(str, path)) + '/old occurrence ' + str(cursor))
                    cursor += 1
                    counts['old_alias_occurrences'] += 1
            assert cursor == len(before), (kind, path, 'old alias deleted')
        else:
            assert len(before) == len(after), (kind, path, 'old array length changed')
            for i, (a, b) in enumerate(zip(before, after)):
                child_tls, child_type = tls, message_type
                if kind == 'SDK' and not path:
                    child_tls = a['protocol'] == 'tls'
                if kind == 'CLI' and len(path) == 2 and path[1] in ['pdus', 'published_pdus']:
                    child_tls = a['protocol'] == 'tls'
                if kind == 'SDK' and len(path) == 3 and path[1] in ['fields', 'session'] and path[2] == 'Handshake Messages':
                    child_type = a['Type']
                compare_existing(a, b, kind, path + (i,), child_tls, child_type, counts)
    else:
        assert before == after, (kind, path, 'old scalar changed')
        counts['old_scalar_values'] += 1
    return counts


def inspect_projection(before_events, after_events, before_cli, after_cli, before_flows, after_flows):
    counts = collections.Counter()
    compare_existing(before_events, after_events, 'SDK', counts=counts)
    compare_existing(before_cli, after_cli, 'CLI', counts=counts)
    exact(after_flows, before_flows, 'default flows')
    by_id = {e['id']: e for e in after_events}
    assert len(by_id) == len(after_events)
    joined = 0
    for row in after_cli:
        for pdu in (row.get('pdus') or []) + (row.get('published_pdus') or []):
            event = by_id[pdu['id']]
            exact(pdu['field_values'], event['field_values'], 'CLI/SDK snapshot ' + str(pdu['id']))
            exact(pdu['fields'], field_map(event['field_values']), 'CLI/SDK field map ' + str(pdu['id']))
            exact(pdu['byte_source'], event['source_bytes'], 'CLI/SDK source ' + str(pdu['id']))
            assert pdu['protocol'] == event['protocol'] and pdu['status'] == event['status']
            assert not FORBIDDEN_ALIASES.intersection(pdu['fields'])
            joined += 1
    return dict(counts, SDK_events=len(after_events), CLI_packets=len(after_cli), CLI_PDU_occurrences_joined_to_SDK=joined)


def source_expectations(capture, events, source_capture, source_manifest, source_events, source_oracle, control_manifest=None):
    """Revalidate the complete source oracle, then restrict it to retained wire."""
    peer = json.loads(source_manifest.read_text('utf-8-sig'))
    source_native = rows(source_events)
    reference = json.loads(source_oracle.read_text('utf-8-sig'))
    baseline = inspect_metadata(source_capture, source_native, peer, reference)
    original, connections, owners, _, _, _ = wire_connections(source_capture, peer)
    current = physical(capture)
    if control_manifest:
        declaration = json.loads(control_manifest.read_text('utf-8-sig'))
        assert declaration['capture_SHA256'] == sha(capture)
        assert declaration['source_capture_SHA256'] == sha(source_capture)
        assert declaration['source_manifest_SHA256'] == sha(source_manifest)
        assert len(current) == len(declaration['ledger'])
        numbers = {}
        for n, (packet, entry) in enumerate(zip(current, declaration['ledger']), 1):
            assert type(entry['number']) is int and entry['number'] == n
            source = entry['source_frame']
            assert type(source) is int and packet == original[source - 1]
            assert entry['source_wire_SHA256'] == hashlib.sha256(packet['wire']).hexdigest()
            assert entry['source_time_ns'] == packet['time_ns']
            numbers[n] = source
    else:
        assert current == original and sha(capture) == sha(source_capture)
        numbers = {n: n for n in range(1, len(current) + 1)}
    source_by_wire = {}
    for event in source_native:
        if event['protocol'] != 'tls':
            continue
        refs = tuple(r['Number'] for r in event['source_bytes']['PacketRefs'])
        key = (event['tcp_flow_id'], event['direction'], event['source_bytes']['SHA256'], refs)
        assert key not in source_by_wire
        source_by_wire[key] = event
    clients, servers, contexts, witnesses = set(), set(), {}, []
    for event in events:
        if event['protocol'] != 'tls':
            continue
        session = event['session'] or {}
        refs = event['source_bytes'].get('PacketRefs') or []
        if not refs:
            assert not session.get('Session Establishment') and not (ALIASES & field_map(event['field_values']).keys())
            fields = event['fields'] or {}
            assert not fields.get('Session Establishment')
            assert not any(MESSAGE_KEYS.intersection(message) for message in fields.get('Handshake Messages') or [])
            continue
        assert not event.get('fields_error'), ('current admitted TLS GetFields refused', event['id'])
        exact(event['fields'], event['session'], 'current admitted TLS GetFields/session ' + str(event['id']))
        original_refs = tuple(sorted(numbers[r['Number']] for r in refs))
        ownership = {owners[n] for n in original_refs}
        assert len(ownership) == 1
        cid, direction = ownership.pop()
        key = (cid, direction, event['source_bytes']['SHA256'], original_refs)
        source = source_by_wire[key]
        assert base64.b64decode(event['raw'], validate=True) == base64.b64decode(source['raw'], validate=True)
        current_messages = session.get('Handshake Messages') or []
        source_messages = source['session'].get('Handshake Messages') or []
        expected_messages = []
        if current_messages:
            assert len(current_messages) == len(source_messages)
            for message, reference_message in zip(current_messages, source_messages):
                assert type(message['Type']) is int and message['Type'] == reference_message['Type']
                expected = copy.deepcopy(reference_message)
                if message['Type'] == 1:
                    clients.add(cid)
                elif message['Type'] == 2:
                    servers.add(cid)
                    if cid not in clients and 'PSK Selection' in expected:
                        index = expected['PSK Selection']['Selected Identity']
                        expected['PSK Selection'] = {'Selected Identity': index,
                            'Offer Match': 'unavailable-client-hello', 'Binder Verified': False,
                            'Ticket Association': 'unconfirmed; selected PSK may be external',
                            'Evidence Scope': 'current ServerHello'}
                    context = copy.deepcopy(source['session'].get('Session Establishment') or {})
                    if cid not in clients:
                        if 'PSK Selection' in expected:
                            context = {'Evidence Scope': 'current connection hello observations',
                                'Certificate Trust': 'not-evaluated', 'PSK Selected': True,
                                'PSK Selection': copy.deepcopy(expected['PSK Selection'])}
                        else:
                            context = {}
                    contexts[cid] = context
                exact({k: v for k, v in message.items() if k in MESSAGE_KEYS},
                      {k: v for k, v in expected.items() if k in MESSAGE_KEYS}, 'source-bound message metadata ' + str(event['id']))
                expected_messages.append(expected)
        context = contexts.get(cid, {}) if cid in servers else {}
        exact(session.get('Session Establishment') or {}, context, 'retained-generation context ' + str(event['id']))
        wanted = metadata_aliases(expected_messages, context)
        fields = field_map(event['field_values'])
        exact({k: v for k, v in fields.items() if k in ALIASES}, wanted, 'source-bound metadata aliases ' + str(event['id']))
        witnesses.append({'PDU': event['id'], 'original_TCP_generation': cid,
            'native_TCP_generation': event.get('tcp_flow_id'), 'source_frames': list(original_refs),
            'observed_current_ClientHello': cid in clients, 'observed_current_ServerHello': cid in servers,
            'metadata_aliases': len(wanted)})
    return {'source_baseline_metadata_revalidated': True, 'source_baseline_handshakes': sum(baseline['handshake_counts'].values()),
            'source_capture_SHA256': sha(source_capture), 'source_SDK_SHA256': sha(source_events),
            'source_manifest_SHA256': sha(source_manifest), 'source_Wireshark_SHA256': sha(source_oracle),
            'retained_ledger_references': len(numbers), 'witnesses': witnesses}


def inspect(before_events, after_events, before_cli, after_cli, before_flows, after_flows, source_validator=None):
    proof = inspect_projection(before_events, after_events, before_cli, after_cli, before_flows, after_flows)
    if source_validator is not None:
        proof['metadata_source_scope'] = source_validator(after_events)
    proof.update({'all_old_values_and_types_exact': True, 'old_alias_relative_order_exact': True,
                  'flow_PDU_stream_source_identifiers_rewritten': False,
                  'full_TLS_feature_parity_claim': False})
    return proof


def audit(args):
    before_events, after_events = rows(args.before_events), rows(args.after_events)
    validator = lambda events: source_expectations(args.capture, events, args.source_capture, args.source_manifest,
        args.source_events, args.source_oracle, args.control_manifest)
    proof = inspect(before_events, after_events, rows(args.before_cli), rows(args.after_cli),
        json.loads(args.before_flows.read_text('utf-8-sig')), json.loads(args.after_flows.read_text('utf-8-sig')), validator)
    assert args.before_flows.read_bytes() == args.after_flows.read_bytes()
    proof['default_flow_bytes_exact'] = True
    if args.before_proof:
        historical = json.loads(args.before_proof.read_text('utf-8-sig'))
        for key, path in [('capture_SHA256', args.capture), ('SDK_SHA256', args.before_events),
                          ('CLI_SHA256', args.before_cli), ('flows_SHA256', args.before_flows)]:
            assert historical[key] == sha(path), ('historical proof must bind this exact same capture/export', key)
        if args.control_manifest:
            assert historical['control_manifest_SHA256'] == sha(args.control_manifest)
            assert historical['source_capture_SHA256'] == sha(args.source_capture)
            assert historical['display_projection_correct'] is True
        proof['historical_same_capture_proof_SHA256'] = sha(args.before_proof)
        proof['historical_proof_reexecuted_with_new_exports'] = False
        proof['historical_result_composed_through_exact_old_value_bridge'] = True
    for label, receipt_path, export in [('before', args.before_producer, args.before_events),
                                        ('after', args.after_producer, args.after_events)]:
        if receipt_path:
            receipt = json.loads(receipt_path.read_text('utf-8-sig'))
            # The historical producer used output_SHA256; current producers
            # use export_SHA256. Both, when present, must bind the same bytes.
            keys = [k for k in ['export_SHA256', 'output_SHA256'] if k in receipt]
            assert keys and all(receipt[k] == sha(export) for k in keys)
            assert receipt['capture_SHA256'] == sha(args.capture)
            assert receipt['exit_code'] == 0 and isinstance(receipt['source'], str)
            proof[label + '_export_source'] = receipt['source']
            proof[label + '_binary_SHA256'] = receipt['binary_SHA256']
    proof['scope'] = __doc__
    proof['inputs_SHA256'] = {k: sha(p) for k, p in vars(args).items() if isinstance(p, Path) and k != 'out' and p is not None}
    proof['auditor_SHA256'] = sha(Path(__file__))
    proof['source_metadata_auditor_SHA256'] = sha(Path(__file__).with_name('tls_resumption_metadata_audit.py'))
    return proof


def arguments():
    parser = argparse.ArgumentParser(description=__doc__)
    for name in ['capture', 'before-events', 'after-events', 'before-cli', 'after-cli', 'before-flows', 'after-flows',
                 'source-capture', 'source-manifest', 'source-events', 'source-oracle', 'out']:
        parser.add_argument('--' + name, type=Path, required=True)
    parser.add_argument('--control-manifest', type=Path)
    parser.add_argument('--before-proof', type=Path)
    parser.add_argument('--before-producer', type=Path)
    parser.add_argument('--after-producer', type=Path)
    return parser


if __name__ == '__main__':
    args = arguments().parse_args()
    for path in vars(args).values():
        if isinstance(path, Path):
            assert not any((p/'.git').exists() for p in [path.resolve(), *path.resolve().parents])
    proof = audit(args)
    with args.out.open('x', encoding='utf8') as output:
        json.dump(proof, output, indent=2); output.write('\n')
    print(json.dumps({k:v for k,v in proof.items() if k != 'metadata_source_scope'}), flush=True)
