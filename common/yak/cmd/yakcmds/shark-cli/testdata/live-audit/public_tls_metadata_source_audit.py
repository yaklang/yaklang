"""Verify new public-corpus TLS metadata from literal messages and Wireshark.

Use exact physical frame numbers, record bytes and the complete native domain /
flow / TCP-generation identity. Never match by endpoints or timestamps. The old
source/flow/record proof is a separate required composition input; this auditor
does not replace it with a frame-only TLS-reassembly guess. QUIC GetFields keeps
its established wire-envelope API, while its Session contains TLS semantics.
All raw tickets, PSKs and decrypted bytes stay in the external inputs.
"""
import argparse
import base64
import collections
import hashlib
import json
import struct
from pathlib import Path

from owned_quic_clock_audit import aliases, many, number, vals
from tls_resumption_audit import hello
from tls_resumption_metadata_audit import (
    CONTEXT_PATHS, DERIVED_FIELDS, FORBIDDEN_ALIASES, WIRE_NAMESPACE,
    WS_WIRE_FIELDS, exact, message_fields, new_ticket, opaque, psk_offer,
)

MESSAGE_KEYS = {'Session ID', 'PSK Offer', 'PSK Key Exchange Modes',
                'PSK Key Exchange Modes Length', 'Session Ticket Offer',
                'PSK Selection', 'New Session Ticket'}
HRR_RANDOM = hashlib.sha256(b'HelloRetryRequest').digest()


def sha(data):
    return hashlib.sha256(data).hexdigest()


def file_sha(path):
    h = hashlib.sha256()
    with path.open('rb') as f:
        for block in iter(lambda: f.read(1 << 20), b''):
            h.update(block)
    return h.hexdigest()


def ws_objects(path, duplicates):
    """Preserve repeated JSON keys (notably coalesced quic/quic_raw pairs)."""
    def pairs(items):
        out, repeated = {}, set()
        for k, v in items:
            if k in out:
                duplicates[k] += 1
                if k not in repeated:
                    out[k] = [out[k]]
                    repeated.add(k)
                out[k].append(v)
            else:
                out[k] = v
        return out
    decoder = json.JSONDecoder(object_pairs_hook=pairs)
    with path.open(encoding='utf-8-sig') as f:
        buf, began, eof = '', False, False
        while True:
            buf = buf.lstrip()
            if not began:
                if not buf:
                    part = f.read(1 << 20)
                    assert part
                    buf += part
                    continue
                assert buf[0] == '['
                buf, began = buf[1:], True
                continue
            if buf.startswith(','):
                buf = buf[1:]
                continue
            if buf.startswith(']'):
                assert not buf[1:].strip() and not f.read().strip()
                return
            try:
                item, end = decoder.raw_decode(buf)
            except json.JSONDecodeError:
                assert not eof, 'truncated Wireshark JSON'
                part = f.read(1 << 20)
                eof = not part
                buf += part
                assert len(buf) <= 32 << 20, '32 MiB per-WS-frame parsing budget'
                continue
            yield item
            buf = buf[end:]


def walk(node):
    if isinstance(node, dict):
        yield node
        for k, v in node.items():
            if not k.endswith('_raw'):
                yield from walk(v)
    elif isinstance(node, list):
        for v in node:
            yield from walk(v)


def raw_entries(node):
    if isinstance(node, list) and node and isinstance(node[0], str):
        yield node
    elif isinstance(node, list):
        for v in node:
            yield from raw_entries(v)


def raw_bytes(entry):
    assert len(entry) >= 5 and type(entry[1]) is int and type(entry[2]) is int
    out = bytes.fromhex(entry[0])
    assert len(out) == entry[2]
    return out


def handshake_parts(node):
    """Return only complete messages; retain explicit partial-reference counts."""
    result, partial = [], 0
    for parent in walk(node):
        if 'tls.handshake_raw' not in parent:
            continue
        raw = list(raw_entries(parent['tls.handshake_raw']))
        children = many(parent.get('tls.handshake', []))
        assert len(raw) == len(children), 'WS handshake/raw occurrence mismatch'
        for encoded, child in zip(raw, children):
            b = raw_bytes(encoded)
            if len(b) < 4 or 4 + int.from_bytes(b[1:4], 'big') != len(b):
                partial += 1
                continue
            assert list(map(number, vals(child, 'tls.handshake.type'))) == [b[0]]
            result.append((b, child))
    return result, partial


def group(e):
    d = e['domain']
    assert set(d) == {'Section', 'Interface', 'Encapsulation'}
    assert type(d['Section']) is type(d['Interface']) is int
    assert isinstance(d['Encapsulation'], str)
    assert type(e['flow']) is int and e['flow'] > 0
    assert e.get('tcp_flow_id') is None or (type(e['tcp_flow_id']) is int and e['tcp_flow_id'] > 0)
    assert type(e['direction']) is int and e['direction'] in [0, 1]
    family = 'quic' if e['protocol'] in ['quic', 'http3', 'doq'] else 'tls'
    return (family, d['Section'], d['Interface'], d['Encapsulation'],
            e['flow'], e.get('tcp_flow_id'))


def messages(e):
    s = e.get('session') or {}
    assert not (s.get('Handshake Messages') and s.get('TLS Handshakes'))
    return s.get('Handshake Messages') or s.get('TLS Handshakes') or []


def relevant(e):
    s = e.get('session') or {}
    if e['protocol'] not in ['tls', 'quic', 'http3', 'doq']:
        return False
    # Select by the already-existing message Type as well as new fields, so
    # deleting every new metadata key cannot remove a hello from this audit.
    if s.get('Session Establishment') or any(m.get('Type') in [1, 2, 4]
                                            or set(m) & MESSAGE_KEYS for m in messages(e)):
        return True
    # Sticky-context presence cannot be its own admission criterion. These
    # legacy status/raw/authentication values are independently preserved by
    # the old-value bridge, and survive removal of every new context field.
    if e['status'] != 'decoded' or not e.get('raw'):
        return False
    if e['protocol'] == 'tls':
        raw = base64.b64decode(e['raw'], validate=True)
        return (len(raw) >= 5 and 20 <= raw[0] <= 24
                and len(raw) == 5 + int.from_bytes(raw[3:5], 'big'))
    return s.get('Authentication Verified') is True


def read_events(path):
    events, parents, ids, excluded = [], {}, set(), []
    with path.open(encoding='utf8') as f:
        for line in f:
            e = json.loads(line)
            assert type(e['id']) is int and e['id'] > 0
            if e['protocol'] == 'radius':
                parents[e['id']] = e
            if relevant(e):
                assert e['id'] not in ids, 'duplicate relevant metadata PDU'
                ids.add(e['id'])
                events.append(e)
            elif e['protocol'] in ['tls', 'quic', 'http3', 'doq']:
                # Preserve refusals/no-source diagnostics, rather than force a
                # successful GetFields result or invent a TLS observation.
                assert not (e.get('session') or {}).get('Session Establishment')
                assert not (e.get('fields') or {}).get('Session Establishment')
                assert not any(v['name'].startswith('tls.session.') for v in
                               (e.get('field_values') or {}).get('values') or [])
                excluded.append(dict(PDU=e['id'], protocol=e['protocol'], status=e['status'],
                                     raw_present=bool(e.get('raw')), fields_refused=bool(e.get('fields_error')),
                                     reason='explicit refusal/no-source diagnostic' if e['status'] != 'decoded'
                                     else 'unprotected QUIC packet; no TLS-context contract'))
    return events, parents, excluded


def selected_physical(path, wanted):
    """Stream PCAPNG; save only exact selected EPBs, including domain IDs."""
    result, section, interfaces, frame = {}, -1, [], 0
    with path.open('rb') as f:
        endian = '<'
        while True:
            prefix = f.read(12)
            if not prefix:
                break
            assert len(prefix) == 12
            if prefix[:4] == b'\x0a\x0d\x0d\x0a':
                assert prefix[8:12] in [b'\x4d\x3c\x2b\x1a', b'\x1a\x2b\x3c\x4d']
                endian = '<' if prefix[8:12] == b'\x4d\x3c\x2b\x1a' else '>'
            kind, size = struct.unpack(endian + 'II', prefix[:8])
            assert size >= 12 and size % 4 == 0 and size <= 32 << 20
            rest = f.read(size - 12)
            assert len(rest) == size - 12
            block = prefix + rest
            assert struct.unpack_from(endian + 'I', block, size - 4)[0] == size
            if kind == 0x0a0d0d0a:
                section += 1
                interfaces = []
            elif kind == 1:
                assert section >= 0 and size >= 20
                interfaces.append(struct.unpack_from(endian + 'H', block, 8)[0])
            elif kind == 6:
                assert size >= 32
                interface, _, _, captured, original = struct.unpack_from(endian + 'IIIII', block, 8)
                assert interface < len(interfaces) and 28 + captured <= size - 4
                frame += 1
                if frame in wanted:
                    result[frame] = dict(raw=block[28:28 + captured], section=section,
                                         interface=interface, captured=captured, original=original)
            else:
                # The public fixtures use Enhanced Packet Blocks. A different
                # packet-bearing block must not silently shift frame numbers.
                assert kind not in [2, 3], 'unsupported packet-bearing PCAPNG block'
    assert wanted <= result.keys(), ('missing physical frames', sorted(wanted - result.keys()))
    return result


def build_reference(paths, physical):
    records, quic, frames, duplicates, partial = collections.defaultdict(list), collections.defaultdict(list), {}, collections.Counter(), 0
    for path in paths:
        for entry in ws_objects(path, duplicates):
            layers = entry['_source']['layers']
            frame = number(layers['frame']['frame.number'])
            if frame not in physical:
                continue
            p = physical[frame]
            assert raw_bytes(layers['frame_raw']) == p['raw'], ('WS physical bytes', frame)
            assert number(layers['frame']['frame.interface_id']) == p['interface']
            if 'frame.section_number' in layers['frame']:
                # Wireshark's section_number is one-based; native Section and
                # this literal PCAPNG block walk are zero-based. No native ID
                # or event domain is rewritten by this explicit comparison.
                assert number(layers['frame']['frame.section_number']) == p['section'] + 1
            frames[frame] = layers
            for parent in walk(layers):
                for key, target in [('tls.record', records), ('quic', quic)]:
                    if key not in parent or key + '_raw' not in parent:
                        continue
                    children, encoded = many(parent[key]), list(raw_entries(parent[key + '_raw']))
                    assert len(children) == len(encoded), ('WS raw occurrence cardinality', frame, key)
                    for child, source in zip(children, encoded):
                        raw = raw_bytes(source)
                        # The raw TLS record may be a reassembled data source;
                        # exact source/flow continuity is supplied separately.
                        if key == 'quic':
                            assert p['raw'][source[1]:source[1]+len(raw)] == raw
                        item = (raw, child, frame)
                        bucket = target[frame, sha(raw)]
                        if not any(old[0] == raw and old[1] == child for old in bucket):
                            bucket.append(item)
            _, n = handshake_parts(layers)
            partial += n
    return dict(records=records, quic=quic, frames=frames,
                duplicates=dict(duplicates), partial_handshakes=partial)


def eap_record(e, raw, parents, reference, physical):
    """Prove the early TLS record inside actual RADIUS EAP fragments."""
    source = e['source_bytes']
    assert source['ParentPDUs'] == [source['ParentPDU']]
    parent = parents[source['ParentPDU']]
    assert parent['protocol'] == 'radius' and parent['domain'] == e['domain']
    refs = source['PacketRefs']
    assert len(refs) == 1
    frame = refs[0]['Number']
    layers = reference['frames'][frame]
    parent_raw = base64.b64decode(parent['raw'], validate=True)
    assert parent['source_bytes']['SHA256'] == sha(parent_raw)
    radius = raw_bytes(layers['radius_raw'])
    assert parent_raw == radius and len(radius) >= 20
    extent = int.from_bytes(radius[2:4], 'big')
    assert extent == len(radius)
    fragments, at = [], 20
    while at < extent:
        assert at + 2 <= extent
        kind, n = radius[at:at+2]
        assert n >= 2 and at + n <= extent
        if kind == 79:
            fragments.append(radius[at+2:at+n])
        at += n
    eap = b''.join(fragments)
    assert len(eap) >= 6 and int.from_bytes(eap[2:4], 'big') == len(eap)
    assert eap[4] in [13, 21, 25]
    flags, start = eap[5], 6
    if flags & 0x80:
        assert len(eap) >= 10
        total = int.from_bytes(eap[6:10], 'big')
        start = 10
        assert total >= len(eap)-start
    assert eap[start:start+len(raw)] == raw, ('RADIUS/EAP literal TLS prefix', e['id'])
    reassembled = set(number(v) for node in walk(layers) for v in vals(node, 'eap.tls.reassembled_in'))
    assert len(reassembled) == 1
    completed = reassembled.pop()
    candidates = reference['records'][completed, sha(raw)]
    assert len(candidates) == 1 and candidates[0][0] == raw
    return candidates[0], dict(kind='RADIUS attributes / EAP TLS fragment prefix',
                               parent_PDU=parent['id'], physical_frame=frame,
                               WS_reassembled_in=completed, EAP_method=eap[4])


def context_aliases(ctx):
    out = {}
    for key, path in CONTEXT_PATHS.items():
        value = ctx
        for part in path:
            if not isinstance(value, dict) or part not in value:
                break
            value = value[part]
        else:
            out['tls.session.' + key] = [value]
    return out


def hello_metadata(body, kind, current, direction):
    h = hello(body, kind == 1)
    meta = {'Session ID': opaque(h['session_id'])}
    ext = h['extensions']
    if kind == 1:
        if 45 in ext:
            modes = ext[45]
            assert len(modes) >= 2 and modes[0] == len(modes)-1
            meta['PSK Key Exchange Modes Length'] = len(modes)-1
            meta['PSK Key Exchange Modes'] = base64.b64encode(modes[1:]).decode('ascii')
        if 41 in ext:
            assert list(ext)[-1] == 41 and 45 in ext and 43 in ext
            versions = ext[43]
            assert versions[0] == len(versions)-1 and (len(versions)-1) % 2 == 0
            assert b'\x03\x04' in [versions[i:i+2] for i in range(1, len(versions), 2)]
            meta['PSK Offer'] = psk_offer(ext[41])
        if ext.get(35):
            meta['Session Ticket Offer'] = opaque(ext[35])
        return meta, h, None, None
    version = int.from_bytes(ext[43], 'big') if 43 in ext else h['legacy_version']
    if 43 in ext:
        assert len(ext[43]) == 2
    ch = current.get('hello')
    ctx = {'Evidence Scope': 'current connection hello observations', 'Certificate Trust': 'not-evaluated'}
    if ch is not None:
        assert current['direction'] != direction
        echo = ch['session_id'] == h['session_id']
        ctx['Session ID Echo'] = echo
        if 0x300 <= version <= 0x303:
            ctx['Session ID Resumption Indicated'] = bool(ch['session_id']) and bool(h['session_id']) and echo
        elif version == 0x304:
            ctx['Compatibility Session ID Echo'] = echo
    if 41 in ext:
        assert version == 0x304 and h['random'] != HRR_RANDOM and len(ext[41]) == 2
        index = int.from_bytes(ext[41], 'big')
        selection = {'Selected Identity': index, 'Offer Match': 'unavailable-client-hello',
                     'Binder Verified': False, 'Ticket Association': 'unconfirmed; selected PSK may be external',
                     'Evidence Scope': 'current ServerHello'}
        if ch is not None:
            assert 41 in ch['extensions']
            offer = psk_offer(ch['extensions'][41])
            assert index < len(offer['Identities'])
            identity = offer['Identities'][index]
            mode = int(51 in ext)
            assert mode in ch['extensions'][45][1:]
            expected_size = {0x1301: 32, 0x1302: 48, 0x1303: 32}.get(h['cipher'])
            if expected_size is not None:
                assert identity['Binder Length'] == expected_size
            selection.update({k: identity[k] for k in ['Length', 'SHA256', 'Binder Length', 'Obfuscated Ticket Age']})
            selection.update({'Offer Match': 'matched-current-client-hello', 'Key Exchange Mode': mode})
        meta['PSK Selection'] = selection
        ctx.update({'PSK Selection': selection, 'PSK Selected': True})
    elif version == 0x304 and ch is not None and h['random'] != HRR_RANDOM:
        ctx['PSK Selected'] = False
    return meta, h, ctx if len(ctx) > 2 else {}, version


def inspect(events, parents, reference, physical):
    states, witnesses, context_witnesses, counts, gaps = {}, [], [], collections.Counter(), []
    for e in events:
        assert type(e['id']) is int and e['id'] > 0
        key = group(e)
        current = states.setdefault(key, {'context': {}, 'hello': None, 'hello_PDU': None,
                                          'server_PDU': None, 'version': None})
        raw = base64.b64decode(e['raw'], validate=True)
        source = e['source_bytes']
        assert source['SHA256'] == sha(raw)
        assert e['field_values']['pdu'] == e['id'] and type(e['field_values']['pdu']) is int
        assert type(e['field_values']['schema_version']) is int and e['field_values']['schema_version'] == 1
        exact(e['field_values']['source'], source, 'field snapshot source')
        refs = source.get('PacketRefs') or []
        assert refs
        for ref in refs:
            assert type(ref['Number']) is int and ref['Number'] > 0 and ref['Domain'] == e['domain']
            p = physical[ref['Number']]
            assert p['section'] == e['domain']['Section'] and p['interface'] == e['domain']['Interface']
        s = e['session']
        ms = messages(e)
        is_quic = key[0] == 'quic'
        if not is_quic:
            if e.get('fields_error'):
                assert e['status'] != 'decoded', ('admitted TLS metadata fields error', e['id'])
            else:
                exact(e['fields'], s, 'TLS GetFields / Session')
        fields = aliases(e)
        assert not FORBIDDEN_ALIASES.intersection(fields) and 'Resumed' not in s
        own_aliases = collections.defaultdict(list)
        message_proofs = []
        own_messages = [m for m in ms if m.get('Type') in [1, 2, 4] or set(m) & MESSAGE_KEYS]
        if own_messages:
            assert e['status'] == 'decoded' and not e.get('error')
            candidates = []
            index = reference['quic' if is_quic else 'records']
            for ref in refs:
                candidates.extend(index[ref['Number'], sha(raw)])
            source_proof = {'kind': 'exact WS record / native raw bytes and physical refs'}
            if not candidates and not is_quic and source.get('ParentPDUs'):
                try:
                    candidate, source_proof = eap_record(e, raw, parents, reference, physical)
                    candidates = [candidate]
                except (AssertionError, KeyError) as error:
                    gaps.append({'PDU': e['id'], 'kind': 'unproved RADIUS/EAP source',
                                 'refs': refs, 'reason': str(error)[:300]})
            if len(candidates) != 1:
                gaps.append({'PDU': e['id'], 'kind': 'missing or ambiguous literal WS source',
                             'candidate_count': len(candidates), 'refs': refs})
                # Never build a current-connection expected context from an
                # unproved hello. A gap is explicit, not an allowed path.
                continue
            ws_raw, ws_node, frame = candidates[0]
            assert ws_raw == raw
            hs, _ = handshake_parts(ws_node)
            if is_quic:
                crypto = []
                for node in walk(ws_node):
                    crypto.extend(raw_bytes(v) for v in raw_entries(node.get('quic.crypto.crypto_data_raw', [])))
                native_crypto = [base64.b64decode(f['Crypto Data'], validate=True)
                                 for f in s.get('Frames', []) if f.get('Frame Type') == 'CRYPTO']
                exact(native_crypto, crypto, 'literal decrypted QUIC CRYPTO frames')
                assert s['Authentication Verified'] is True
                assert s['Peer Identity Verified'] is False
            elif raw[0] == 22:
                assert len(raw) == 5 + int.from_bytes(raw[3:5], 'big')
                assert b''.join(b for b, _ in hs) == raw[5:], ('complete clear TLS message bytes', e['id'])
                assert s['Authentication Verified'] is False and s['Content Visibility'] == 'cleartext'
            else:
                assert raw[0] in [22, 23] and s['Authentication Verified'] is True
                assert s['Content Visibility'] == 'authenticated-plaintext'
                assert s.get('Secret Lookup Status') == 'found'
            assert len(ms) == len(hs), ('complete message count', e['id'], len(ms), len(hs))
            for m, (b, ws) in zip(ms, hs):
                kind, body = b[0], b[4:]
                assert type(m['Type']) is int and m['Type'] == kind
                assert type(m['Length']) is int and m['Length'] == len(body)
                auth = False if is_quic or raw[0] == 22 else True
                assert m['Authenticated'] is auth
                meta = {}
                if kind in [1, 2]:
                    meta, h, ctx, version = hello_metadata(body, kind, current, e['direction'])
                    if kind == 1:
                        current.update(hello=h, hello_PDU=e['id'], direction=e['direction'],
                                       context={}, server_PDU=None)
                    else:
                        current.update(context=ctx, server_PDU=e['id'], version=version)
                elif kind == 4:
                    assert current['version'] is not None, ('NST missing source-proved ServerHello', e['id'])
                    version = current['version']
                    if version == 0x304:
                        assert auth
                        ticket = new_ticket(body, version, auth)
                        if ticket['Lifetime Hint'] > 604800:
                            ticket['Lifetime Diagnostic'] = 'server lifetime exceeds RFC 8446 seven-day limit; ticket acceptance is unconfirmed'
                    else:
                        assert 0x301 <= version <= 0x303 and len(body) >= 6
                        assert int.from_bytes(body[4:6], 'big') == len(body)-6
                        ticket = {'Lifetime Hint': int.from_bytes(body[:4], 'big'),
                                  'Record Authentication Verified': auth,
                                  'Ticket Association': 'not tracked across connections',
                                  'Evidence Scope': 'this NewSessionTicket message', 'Ticket': opaque(body[6:])}
                    meta['New Session Ticket'] = ticket
                if kind in [1, 2, 4]:
                    exact(m, dict(Type=kind, Length=len(body), Authenticated=auth, **meta),
                          'complete metadata message PDU ' + str(e['id']))
                else:
                    assert not set(m) & MESSAGE_KEYS
                wants = message_fields(kind, meta)
                for name, values in wants.items():
                    own_aliases[name].extend(values)
                    if name in WS_WIRE_FIELDS:
                        exact(list(map(number, vals(ws, name))), values,
                              'Wireshark wire metadata ' + str(frame) + '/' + name)
                if meta:
                    counts[str(kind)] += 1
                    message_proofs.append(dict(type=kind, handshake_SHA256=sha(b), metadata=meta))
            witnesses.append(dict(PDU=e['id'], domain=e['domain'], flow=e['flow'], tcp_flow_id=e.get('tcp_flow_id'),
                                  direction=e['direction'], refs=refs, WS_frame=frame,
                                  record_SHA256=sha(raw), source=source_proof, messages=message_proofs))
        expected_context = current['context']
        exact(s.get('Session Establishment') or {}, expected_context,
              'current observed connection context at PDU ' + str(e['id']))
        exact({k: v for k, v in fields.items() if k in WIRE_NAMESPACE}, dict(own_aliases),
              'completed-message metadata aliases at PDU ' + str(e['id']))
        exact({k: v for k, v in fields.items() if k.startswith('tls.session.')}, context_aliases(expected_context),
              'current-context aliases at PDU ' + str(e['id']))
        for item in e['field_values']['values']:
            name = item['name']
            if name in WIRE_NAMESPACE or name.startswith('tls.session.'):
                origin = 'context' if name.startswith('tls.session.') else 'derived' if name in DERIVED_FIELDS else 'pdu'
                assert item['origin'] == origin and 'pdu_range' not in item
        counts['context_events'] += bool(expected_context)
        context_witnesses.append(dict(PDU=e['id'], domain=e['domain'], flow=e['flow'],
                                      tcp_flow_id=e.get('tcp_flow_id'), direction=e['direction'],
                                      ClientHello_PDU=current['hello_PDU'], ServerHello_PDU=current['server_PDU'],
                                      context_SHA256=sha(json.dumps(expected_context, sort_keys=True,
                                                                   separators=(',', ':')).encode('utf8'))))
    metadata_events = sum(bool((e.get('session') or {}).get('Session Establishment')
                               or any(set(m) & MESSAGE_KEYS for m in messages(e))) for e in events)
    return dict(metadata_events=metadata_events, admitted_TLS_QUIC_records=len(events),
                admitted_record_protocol_counts=dict(collections.Counter(e['protocol'] for e in events)),
                records_without_new_metadata_checked_for_absence=len(events)-metadata_events,
                new_handshake_counts=dict(counts),
                new_handshakes_source_verified=sum(v for k, v in counts.items() if k != 'context_events'),
                connection_groups=len(states), witnesses=witnesses, context_witnesses=context_witnesses, gaps=gaps,
                every_new_handshake_source_verified=not gaps,
                old_source_flow_and_record_continuity_requires_separate_exact_bridge=True,
                QUIC_GetFields_wire_envelope_compared_to_Session=False,
                full_TLS_feature_parity_claim=False)


def bind_preservation(document, producer, events_path):
    assert document['source'] == producer['source']
    assert document['all_old_scalar_types_values_arrays_and_alias_order_preserved'] is True
    assert document['generated_ID_translation'] is False
    matches = [v for v in document['results']
               if v['SDK_export_path'] == str(events_path)]
    assert len(matches) == 1
    result = matches[0]
    assert result['SDK_SHA256'] == producer['export_SHA256']
    assert result['capture_SHA256'] == producer['capture_SHA256']
    exact(result['SDK_producer'], producer, 'separate old-source bridge producer binding')
    assert result['all_preexisting_IDs_and_sources_literal'] is True
    assert result['every_SDK_event_published_in_CLI'] is True
    assert result['new_metadata_projection_consistency'] is True
    return dict(old_source_flow_record_continuity_composed=True,
                old_SDK_SHA256=result['old_SDK_SHA256'], current_SDK_SHA256=result['SDK_SHA256'],
                current_CLI_SHA256=result['CLI_SHA256'],
                source_bridge_changed_PDU_ids=result['changed_PDU_ids'],
                old_scalar_values=result['old_scalar_values'], old_alias_occurrences=result['old_alias_occurrences'])


def prepare(events, producer, capture, references, preservation):
    p = json.loads(producer.read_text('utf-8-sig'))
    assert p['exit_code'] == 0 and p['capture'] == str(capture)
    assert p['capture_SHA256'] == file_sha(capture) and p['export_SHA256'] == file_sha(events)
    bridge = bind_preservation(json.loads(preservation.read_text('utf-8-sig')), p, events)
    native, parents, excluded = read_events(events)
    wanted = {r['Number'] for e in native for r in e['source_bytes'].get('PacketRefs') or []}
    # These exact completion frames are read from actual EAP reassembly links,
    # not inferred from timing. Read the reference header first to admit them.
    for path in references:
        duplicate_counts = collections.Counter()
        for entry in ws_objects(path, duplicate_counts):
            l = entry['_source']['layers']
            frame = number(l['frame']['frame.number'])
            if frame in wanted:
                for node in walk(l):
                    wanted.update(number(v) for v in vals(node, 'eap.tls.reassembled_in'))
    physical = selected_physical(capture, wanted)
    reference = build_reference(references, physical)
    provenance = dict(scope=__doc__, source=p['source'], SDK_SHA256=file_sha(events),
                 producer_SHA256=file_sha(producer), capture_SHA256=file_sha(capture),
                 binary_SHA256=p['binary_SHA256'],
                 reference_SHA256={str(path): file_sha(path) for path in references},
                 source_flow_record_continuity_bridge_SHA256=file_sha(preservation),
                 source_flow_record_continuity_bridge=bridge,
                 WS_duplicate_keys_preserved=reference['duplicates'],
                 partial_WS_handshakes_explicitly_not_admitted=reference['partial_handshakes'],
                 context_admission_independent_of_new_metadata_presence=True,
                 explicitly_preserved_TLS_QUIC_diagnostics=excluded,
                 auditor_SHA256=file_sha(Path(__file__)))
    return native, parents, reference, physical, provenance


def audit(events, producer, capture, references, preservation):
    native, parents, reference, physical, provenance = prepare(events, producer, capture, references, preservation)
    proof = inspect(native, parents, reference, physical)
    proof.update(provenance)
    return proof


if __name__ == '__main__':
    ap = argparse.ArgumentParser(description=__doc__)
    for name in ['events', 'producer', 'capture', 'preservation-proof', 'out']:
        ap.add_argument('--' + name, required=True, type=Path)
    ap.add_argument('--reference', action='append', required=True, type=Path)
    a = ap.parse_args()
    proof = audit(a.events, a.producer, a.capture, a.reference, a.preservation_proof)
    with a.out.open('x', encoding='utf8') as f:
        json.dump(proof, f, indent=2)
        f.write('\n')
    print(json.dumps({k: v for k, v in proof.items() if k not in ['witnesses', 'context_witnesses', 'gaps']}), flush=True)
    assert proof['every_new_handshake_source_verified'], proof['gaps']
