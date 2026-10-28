"""Verify TLS resumption observations against literal owned wire and WS messages.

The input is the declared eight-generation OpenSSL capture, including fresh,
resumed and fresh-again connections on reused TCP tuples. Verify each new
message field separately from inherited connection observations. A PSK selection
is not proof of ticket resumption, binder verification or certificate trust.
Raw ticket/identity/binder material stays in external oracle files.
This capture's messages each occupy one complete TLS record; mixed-record
handshake authentication needs the separate implementation regression tests.
"""
import argparse
import base64
import collections
import hashlib
import json
from pathlib import Path

from owned_quic_clock_audit import aliases, many, number, vals
from tls_resumption_audit import hello, rows, wire_connections

WS_WIRE_FIELDS = {'tls.handshake.' + k for k in [
    'session_id_length', 'extensions.psk.identities.length', 'extensions.psk.binders_len',
    'extensions.psk.identity.identity_length', 'extensions.psk.identity.obfuscated_ticket_age',
    'extensions.psk.binder.binder_length', 'extensions.psk.identity.selected',
    'session_ticket_lifetime_hint', 'session_ticket_length', 'session_ticket_age_add',
    'session_ticket_nonce_length']}
WS_WIRE_FIELDS.update(['tls.extension.psk_ke_modes_length', 'tls.extension.psk_ke_mode'])

DERIVED_FIELDS = {'tls.handshake.' + k for k in [
    'session_id.sha256', 'session_ticket_offer.sha256', 'extensions.psk.identity.sha256',
    'extensions.psk.offer_match', 'extensions.psk.binder_verified', 'session_ticket.sha256',
    'session_ticket_nonce.sha256', 'session_ticket.authenticated',
    'session_ticket.lifetime_within_server_limit', 'session_ticket.max_client_cache_seconds']}

WIRE_NAMESPACE = WS_WIRE_FIELDS | DERIVED_FIELDS | {
    'tls.handshake.session_ticket_offer.length', 'tls.handshake.session_ticket.max_early_data_size'}

FORBIDDEN_ALIASES = {'tls.resumed', 'tls.handshake.session_ticket',
    'tls.handshake.extensions.session_ticket', 'tls.handshake.extensions.psk.identity.identity',
    'tls.handshake.extensions.psk.binder.binder'}

CONTEXT_PATHS = {
    'evidence_scope': ['Evidence Scope'], 'id_echo': ['Session ID Echo'],
    'compatibility_id_echo': ['Compatibility Session ID Echo'],
    'resumption_indicated': ['Session ID Resumption Indicated'],
    'psk.selected': ['PSK Selected'], 'psk.selected_identity': ['PSK Selection', 'Selected Identity'],
    'psk.offer_match': ['PSK Selection', 'Offer Match'],
    'psk.identity_length': ['PSK Selection', 'Length'], 'psk.identity_sha256': ['PSK Selection', 'SHA256'],
    'psk.key_exchange_mode': ['PSK Selection', 'Key Exchange Mode'],
    'psk.binder_verified': ['PSK Selection', 'Binder Verified'],
    'psk.ticket_association': ['PSK Selection', 'Ticket Association'],
}


def sha(b):
    return hashlib.sha256(b).hexdigest()


def exact(actual, expected, path):
    assert type(actual) is type(expected), (path, type(actual).__name__, type(expected).__name__)
    if isinstance(expected, dict):
        assert actual.keys() == expected.keys(), (path, sorted(actual), sorted(expected))
        for k, v in expected.items():
            exact(actual[k], v, path + '/' + k)
    elif isinstance(expected, list):
        assert len(actual) == len(expected), (path, len(actual), len(expected))
        for i, (a, b) in enumerate(zip(actual, expected)):
            exact(a, b, path + '/' + str(i))
    else:
        assert actual == expected, (path, actual, expected)


def opaque(b):
    return {'Length': len(b), 'SHA256': sha(b)}


def raw_entries(v):
    return [v] if v and isinstance(v[0], str) else v


def records(v):
    if isinstance(v, dict):
        if 'tls.record' in v:
            children = many(v['tls.record'])
            raw = raw_entries(v['tls.record_raw'])
            assert len(children) == len(raw)
            for child, source in zip(children, raw):
                b = bytes.fromhex(source[0])
                assert source[2] == len(b) and len(b) == 5 + int.from_bytes(b[3:5], 'big')
                yield b, child
        else:
            for k, child in v.items():
                if not k.endswith('_raw'):
                    yield from records(child)
    elif isinstance(v, list):
        for child in v:
            yield from records(child)


def handshakes(record):
    children = many(record['tls.handshake']) if 'tls.handshake' in record else []
    raw = raw_entries(record['tls.handshake_raw']) if 'tls.handshake_raw' in record else []
    assert len(children) == len(raw)
    for child, source in zip(children, raw):
        b = bytes.fromhex(source[0])
        assert source[2] == len(b) and len(b) == 4 + int.from_bytes(b[1:4], 'big')
        assert list(map(number, vals(child, 'tls.handshake.type'))) == [b[0]]
        yield b, child


def psk_offer(v):
    assert len(v) >= 2
    size = int.from_bytes(v[:2], 'big')
    end = 2 + size
    assert size >= 7 and end + 2 <= len(v)
    entries = []
    at = 2
    while at < end:
        assert at + 2 <= end
        n = int.from_bytes(v[at:at+2], 'big')
        at += 2
        assert n and at + n + 4 <= end
        item = opaque(v[at:at+n])
        item['Index'] = len(entries)
        item['Obfuscated Ticket Age'] = int.from_bytes(v[at+n:at+n+4], 'big')
        entries.append(item)
        at += n + 4
    assert at == end
    bn = int.from_bytes(v[end:end+2], 'big')
    at = end + 2
    assert bn >= 33 and at + bn == len(v)
    for item in entries:
        assert at < len(v)
        n = v[at]
        at += 1
        assert n >= 32 and at + n <= len(v)
        item['Binder Length'] = n
        at += n
    assert at == len(v)
    return {'Identities Length': size, 'Binders Length': bn, 'Identities': entries,
            'Binder Verified': False, 'Identity Origin': 'external PSK or session ticket; unconfirmed'}


def new_ticket(body, version, authenticated):
    assert len(body) >= 6
    lifetime = int.from_bytes(body[:4], 'big')
    out = {'Lifetime Hint': lifetime, 'Record Authentication Verified': authenticated,
           'Ticket Association': 'not tracked across connections',
           'Evidence Scope': 'this NewSessionTicket message'}
    if version == 0x303:
        n = int.from_bytes(body[4:6], 'big')
        assert n == len(body)-6
        out['Ticket'] = opaque(body[6:])
    else:
        assert version == 0x304 and len(body) >= 13 and authenticated
        out['Age Add'] = int.from_bytes(body[4:8], 'big')
        n = body[8]
        at = 9
        assert at + n + 4 <= len(body)
        out['Nonce'] = opaque(body[at:at+n])
        at += n
        n = int.from_bytes(body[at:at+2], 'big')
        at += 2
        assert n and at + n + 2 <= len(body)
        out['Ticket'] = opaque(body[at:at+n])
        at += n
        n = int.from_bytes(body[at:at+2], 'big')
        at += 2
        assert at + n == len(body)
        ext, seen = [], set()
        while at < len(body):
            assert at + 4 <= len(body)
            kind, n = int.from_bytes(body[at:at+2], 'big'), int.from_bytes(body[at+2:at+4], 'big')
            at += 4
            assert kind not in seen and at + n <= len(body)
            seen.add(kind)
            item = {'Type': kind, 'Length': n}
            if kind == 42:
                assert n == 4
                item['Maximum Early Data Size'] = int.from_bytes(body[at:at+n], 'big')
            ext.append(item)
            at += n
        out['Extensions'] = ext
        out['Lifetime Within Server Limit'] = lifetime <= 604800
        out['Maximum Client Cache Seconds'] = min(lifetime, 604800)
    return out


def message_fields(kind, metadata):
    out = collections.defaultdict(list)
    def add(key, value):
        out['tls.handshake.' + key].append(value)
    if kind in [1, 2]:
        sid = metadata['Session ID']
        add('session_id_length', sid['Length'])
        add('session_id.sha256', sid['SHA256'])
    if 'PSK Key Exchange Modes Length' in metadata:
        out['tls.extension.psk_ke_modes_length'].append(metadata['PSK Key Exchange Modes Length'])
        out['tls.extension.psk_ke_mode'].extend(base64.b64decode(metadata['PSK Key Exchange Modes'], validate=True))
    if 'Session Ticket Offer' in metadata:
        add('session_ticket_offer.length', metadata['Session Ticket Offer']['Length'])
        add('session_ticket_offer.sha256', metadata['Session Ticket Offer']['SHA256'])
    if 'PSK Offer' in metadata:
        p = metadata['PSK Offer']
        add('extensions.psk.identities.length', p['Identities Length'])
        add('extensions.psk.binders_len', p['Binders Length'])
        for item in p['Identities']:
            add('extensions.psk.identity.identity_length', item['Length'])
            add('extensions.psk.identity.sha256', item['SHA256'])
            add('extensions.psk.identity.obfuscated_ticket_age', item['Obfuscated Ticket Age'])
            add('extensions.psk.binder.binder_length', item['Binder Length'])
    if 'PSK Selection' in metadata:
        add('extensions.psk.identity.selected', metadata['PSK Selection']['Selected Identity'])
        add('extensions.psk.offer_match', metadata['PSK Selection']['Offer Match'])
        add('extensions.psk.binder_verified', metadata['PSK Selection']['Binder Verified'])
    if 'New Session Ticket' in metadata:
        p = metadata['New Session Ticket']
        add('session_ticket_lifetime_hint', p['Lifetime Hint'])
        add('session_ticket_length', p['Ticket']['Length'])
        add('session_ticket.sha256', p['Ticket']['SHA256'])
        add('session_ticket.authenticated', p['Record Authentication Verified'])
        if 'Age Add' in p:
            add('session_ticket_age_add', p['Age Add'])
            add('session_ticket_nonce_length', p['Nonce']['Length'])
            add('session_ticket_nonce.sha256', p['Nonce']['SHA256'])
            add('session_ticket.lifetime_within_server_limit', p['Lifetime Within Server Limit'])
            add('session_ticket.max_client_cache_seconds', p['Maximum Client Cache Seconds'])
            for ext in p['Extensions']:
                if 'Maximum Early Data Size' in ext:
                    add('session_ticket.max_early_data_size', ext['Maximum Early Data Size'])
    return out


def inspect(capture, native, peer, reference):
    physical, connections, owners, wire, hellos, total = wire_connections(capture, peer)
    assert len(reference) == len(physical)
    record_count = sum(len(q) for q in wire.values())
    auth_expected = {}
    by_direction = collections.defaultdict(list)
    for ((cid, direction), _), queue in wire.items():
        by_direction[cid, direction].extend(queue)
    for (cid, direction), queue in by_direction.items():
        after_ccs = False
        for record in sorted(queue, key=lambda r: r['index']):
            kind = record['wire'][0]
            auth_expected[cid, direction, record['index']] = kind == 23 or (connections[cid]['profile'] == 'tls12' and after_ccs)
            if kind == 20:
                after_ccs = True
    expected = collections.defaultdict(collections.deque)
    oracle_count = 0
    for n, entry in enumerate(reference, 1):
        layers = entry['_source']['layers']
        assert number(layers['frame']['frame.number']) == n
        assert bytes.fromhex(layers['frame_raw'][0]) == physical[n-1]['wire']
        if n not in owners:
            continue
        cid, direction = owners[n]
        assert number(layers['tcp']['tcp.stream']) == cid-1
        for raw, record in records(layers):
            key = (cid, direction, sha(raw))
            assert wire[((cid, direction), sha(raw))]
            items = list(handshakes(record))
            oracle_count += len(items)
            expected[key].append((n, items))
    assert oracle_count == 56
    witnesses, counts = [], collections.Counter()
    selections = {}
    ids = [e['id'] for e in native]
    assert all(type(i) is int and i > 0 for i in ids) and len(ids) == len(set(ids))
    established = set()
    for e in native:
        if e['protocol'] != 'tls':
            continue
        cid, direction = e['tcp_flow_id'], e['direction']
        assert type(cid) is int and type(direction) is int and direction in [0, 1]
        assert type(e['flow']) is int and e['flow'] == cid and e['transport'] == 'tcp'
        connection = connections[cid]
        client = '127.0.0.1:' + str(connection['client_port'])
        server = '127.0.0.1:' + str(connection['server_port'])
        assert (e['source'], e['destination']) == ((client, server) if direction == 0 else (server, client))
        raw = base64.b64decode(e['raw'], validate=True)
        key = (cid, direction, sha(raw))
        assert expected[key], ('missing literal oracle record', key)
        frame, hs = expected[key].popleft()
        source = wire[((cid, direction), sha(raw))].popleft()
        assert raw == source['wire'] and e['source_bytes']['SHA256'] == sha(raw)
        assert e['status'] == 'decoded' and not e.get('error') and not e.get('fields_error')
        exact(e['fields'], e['session'], 'public GetFields and session at PDU ' + str(e['id']))
        record_auth = auth_expected[cid, direction, source['index']]
        assert e['session']['Authentication Verified'] is record_auth
        assert e['session']['Certificate Trust'] == 'not-evaluated'
        if record_auth:
            assert e['session']['Secret Lookup Status'] == 'found' and e['session']['Content Visibility'] == 'authenticated-plaintext'
        domain = {'Section': 0, 'Interface': 0, 'Encapsulation': ''}
        exact(e['domain'], domain, 'owned capture domain')
        exact(e['source_bytes'], {'Kind': 'reassembled', 'SHA256': sha(raw), 'ParentPDU': 0,
              'ParentPDUs': None, 'PacketRefs': [{'Number': n, 'Domain': domain} for n in source['refs']],
              'CompletedIn': {'Number': max(source['refs']), 'Domain': domain}}, 'literal source at PDU ' + str(e['id']))
        exact(e['field_values']['source'], e['source_bytes'], 'field snapshot source')
        assert type(e['field_values']['pdu']) is int and e['field_values']['pdu'] == e['id']
        assert type(e['field_values']['schema_version']) is int and e['field_values']['schema_version'] == 1
        assert not e['field_values'].get('truncated') and not e['field_values'].get('incomplete')
        refs = [r['Number'] for r in e['source_bytes']['PacketRefs']]
        assert sorted(refs) == source['refs'] and frame in refs
        messages = e['session'].get('Handshake Messages') or []
        assert len(messages) == len(hs), (cid, frame, len(messages), len(hs))
        version = 0x303 if connections[cid]['profile'] == 'tls12' else 0x304
        ch, sh = hellos[cid, 1], hellos[cid, 2]
        want_aliases = collections.defaultdict(list)
        for m, (b, ws) in zip(messages, hs):
            kind, body = b[0], b[4:]
            assert m['Type'] == kind and type(m['Type']) is int and type(m['Length']) is int and m['Length'] == len(body)
            authenticated = record_auth
            assert m['Authenticated'] is authenticated
            meta = {}
            if kind in [1, 2]:
                h = hello(body, kind == 1)
                meta['Session ID'] = opaque(h['session_id'])
                if kind == 1:
                    if 45 in h['extensions']:
                        modes = h['extensions'][45]
                        assert modes and modes[0] == len(modes)-1
                        meta['PSK Key Exchange Modes Length'] = len(modes)-1
                        meta['PSK Key Exchange Modes'] = base64.b64encode(modes[1:]).decode('ascii')
                    if 41 in h['extensions']:
                        meta['PSK Offer'] = psk_offer(h['extensions'][41])
                    if h['extensions'].get(35):
                        meta['Session Ticket Offer'] = opaque(h['extensions'][35])
                else:
                    established.add(cid)
                    if 41 in h['extensions']:
                        index = int.from_bytes(h['extensions'][41], 'big')
                        offer = psk_offer(ch['extensions'][41])
                        item = offer['Identities'][index]
                        meta['PSK Selection'] = {'Selected Identity': index,
                            'Offer Match': 'matched-current-client-hello', 'Binder Verified': False,
                            'Ticket Association': 'unconfirmed; selected PSK may be external',
                            'Evidence Scope': 'current ServerHello', 'Length': item['Length'], 'SHA256': item['SHA256'],
                            'Key Exchange Mode': int(51 in h['extensions']), 'Binder Length': item['Binder Length'],
                            'Obfuscated Ticket Age': item['Obfuscated Ticket Age']}
                        selections[cid] = meta['PSK Selection']
            elif kind == 4:
                meta['New Session Ticket'] = new_ticket(body, version, authenticated)
            own = {'Session ID', 'PSK Offer', 'PSK Key Exchange Modes', 'PSK Key Exchange Modes Length', 'Session Ticket Offer', 'PSK Selection', 'New Session Ticket'}
            exact({k: v for k, v in m.items() if k in own}, meta, 'PDU ' + str(e['id']) + '/message ' + str(kind))
            legacy_keys = {'Type', 'Length', 'Authenticated'}
            if kind == 11:
                legacy_keys.update(['Certificate Message SHA256', 'Certificates'])
                assert m['Certificate Message SHA256'] == sha(body)
            assert set(m) == legacy_keys | set(meta), ('unexpected or missing handshake metadata key', kind, set(m))
            message_aliases = message_fields(kind, meta)
            for k, values in message_aliases.items():
                want_aliases[k].extend(values)
                # WS does not export our privacy-preserving SHA fields.
                if k in WS_WIRE_FIELDS:
                    exact(list(map(number, vals(ws, k))), values, 'WS ' + str(frame) + '/' + k)
            counts[kind] += 1
            witnesses.append({'PDU': e['id'], 'flow': cid, 'frame': frame, 'type': kind,
                              'handshake_SHA256': sha(b), 'metadata': meta,
                              'message_alias_values': dict(message_aliases)})
        fields = aliases(e)
        # The registered new wire namespace is intentionally finite. Do not
        # remove old negotiated context fields which share the handshake prefix.
        actual_own = {k: v for k, v in fields.items() if k in WIRE_NAMESPACE}
        exact(actual_own, dict(want_aliases), 'wire aliases at PDU ' + str(e['id']))
        for v in e['field_values'].get('values') or []:
            if v['name'] in WIRE_NAMESPACE:
                assert v['origin'] == ('derived' if v['name'] in DERIVED_FIELDS else 'pdu') and 'pdu_range' not in v
            if v['name'].startswith('tls.session.'):
                assert v['origin'] == 'context' and 'pdu_range' not in v
        ctx = e['session'].get('Session Establishment') or {}
        expected_context = {}
        if cid in established:
            if version == 0x304:
                selected = connections[cid]['generation'] in [1, 2]
                expected_context = {'Session ID Echo': True, 'Compatibility Session ID Echo': True, 'PSK Selected': selected}
                if selected:
                    expected_context['PSK Selection'] = selections[cid]
            else:
                expected_context = {'Session ID Echo': ch['session_id'] == sh['session_id'],
                                    'Session ID Resumption Indicated': bool(ch['session_id']) and ch['session_id'] == sh['session_id']}
            if expected_context:
                expected_context.update({'Evidence Scope': 'current connection hello observations', 'Certificate Trust': 'not-evaluated'})
        exact(ctx, expected_context, 'context at PDU ' + str(e['id']))
        context_aliases = {}
        for key, path in CONTEXT_PATHS.items():
            value = expected_context
            for name in path:
                if not isinstance(value, dict) or name not in value:
                    break
                value = value[name]
            else:
                context_aliases['tls.session.' + key] = [value]
        exact({k: v for k, v in fields.items() if k.startswith('tls.session.')}, context_aliases, 'context aliases at PDU ' + str(e['id']))
        assert not FORBIDDEN_ALIASES.intersection(fields) and 'Resumed' not in e['session']
    assert all(not q for q in expected.values()) and all(not q for q in wire.values())
    assert sum(counts.values()) == 56 and counts[1] == counts[2] == 8 and counts[4] > 0
    return {'scope': __doc__, 'physical_packets': len(physical), 'TCP_generations': 8,
            'literal_TLS_records': record_count, 'literal_TLS_bytes': total,
            'handshake_counts': dict(counts), 'per_message_metadata_and_WS_field_values_exact': True,
            'all_records_authentication_matches_observed_CCS_epoch_and_known_complete_capture': True,
            'public_GetFields_including_new_metadata_equals_owned_session_projection': True,
            'all_fresh_TLS13_SID_echoes_are_compatibility_only': True,
            'selected_PSK_binder_and_ticket_origin_not_claimed_verified': True,
            'old_connection_context_not_carried_into_next_SYN_generation': True,
            'witnesses': witnesses, 'full_TLS_feature_parity_claim': False}


def audit(capture, events, manifest, oracle):
    proof = inspect(capture, rows(events), json.loads(manifest.read_text('utf-8-sig')), json.loads(oracle.read_text('utf-8-sig')))
    proof.update({k + '_SHA256': sha(p.read_bytes()) for k, p in [('capture', capture), ('SDK', events), ('manifest', manifest), ('reference', oracle), ('auditor', Path(__file__))]})
    return proof


if __name__ == '__main__':
    ap = argparse.ArgumentParser(description=__doc__)
    for name in ['capture', 'events', 'manifest', 'oracle', 'out']:
        ap.add_argument('--' + name, required=True, type=Path)
    a = ap.parse_args()
    for p in vars(a).values():
        assert not any((d / '.git').exists() for d in [p.resolve(), *p.resolve().parents])
    proof = audit(a.capture, a.events, a.manifest, a.oracle)
    with a.out.open('x', encoding='utf8') as f:
        json.dump(proof, f, indent=2)
        f.write('\n')
    print(json.dumps({k: v for k, v in proof.items() if k != 'witnesses'}), flush=True)
