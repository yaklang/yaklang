"""Challenge public TLS metadata with coherent in-memory semantic corruption.

Prepare and verify actual producer/capture/reference/old-value-bridge hashes
once, then mutate semantic objects without invoking file-digest rejection.
When a message/context changes, synchronize TLS GetFields and new aliases so
the literal-message oracle, rather than public-view disagreement, must fail.
This is a targeted corpus audit; it does not establish every TLS feature.
"""
import argparse
import base64
import collections
import copy
import json
from pathlib import Path

from public_tls_metadata_source_audit import (
    CONTEXT_PATHS, DERIVED_FIELDS, MESSAGE_KEYS, WIRE_NAMESPACE,
    bind_preservation, context_aliases, file_sha, inspect, message_fields,
    messages, prepare, relevant, sha, walk,
)


def synchronize(event):
    session = event['session']
    if event['protocol'] == 'tls':
        event['fields'] = copy.deepcopy(session)
    wanted = collections.defaultdict(list)
    for message in messages(event):
        meta = {k: v for k, v in message.items() if k in MESSAGE_KEYS}
        # A deletion control must reach the source oracle. The baseline helper
        # requires SID for hello types, so ask it to project only remaining
        # independent metadata when that field was deliberately removed.
        kind = message['Type'] if 'Session ID' in meta or message['Type'] not in [1, 2] else -1
        for name, values in message_fields(kind, meta).items():
            wanted[name].extend(values)
    wanted.update(context_aliases(session.get('Session Establishment') or {}))
    old = [v for v in event['field_values']['values']
           if v['name'] not in WIRE_NAMESPACE and not v['name'].startswith('tls.session.')]
    for name, values in wanted.items():
        origin = 'context' if name.startswith('tls.session.') else 'derived' if name in DERIVED_FIELDS else 'pdu'
        old.extend(dict(name=name, value=value, origin=origin) for value in values)
    event['field_values']['values'] = old


def first(events, predicate):
    for i, event in enumerate(events):
        if predicate(event):
            return i
    raise AssertionError('required corpus negative-control target absent')


def target_message(event, key):
    return next(m for m in messages(event) if key in m)


def audit(events_path, producer, capture, references, preservation):
    native, parents, reference, physical, provenance = prepare(events_path, producer, capture, references, preservation)
    baseline = inspect(native, parents, reference, physical)
    assert baseline['every_new_handshake_source_verified'] and not baseline['gaps']
    controls = []

    def reject(name, mutate, index=None, coherent=True, reselect=False):
        changed = list(native)
        if index is not None:
            changed[index] = copy.deepcopy(native[index])
            mutate(changed[index])
            if coherent:
                synchronize(changed[index])
        else:
            mutate(changed)
        if reselect:
            # Exercise the real read_events admission predicate after changing
            # all public metadata, rather than keeping an already-selected row.
            changed = [e for e in changed if relevant(e)]
        try:
            proof = inspect(changed, parents, reference, physical)
            assert proof['every_new_handshake_source_verified'] and not proof['gaps']
        except (AssertionError, KeyError, ValueError, TypeError) as error:
            controls.append(dict(name=name, rejected=True, failure_type=type(error).__name__))
        else:
            raise AssertionError('semantic corruption was accepted: ' + name)

    sid = first(native, lambda e: e['protocol'] == 'tls' and any('Session ID' in m for m in messages(e)))
    def sid_value(e, key, value):
        target_message(e, 'Session ID')['Session ID'][key] = value
    reject('literal session ID length', lambda e: sid_value(e, 'Length', target_message(e, 'Session ID')['Session ID']['Length'] + 1), sid)
    reject('literal session ID hash', lambda e: sid_value(e, 'SHA256', '0' * 64), sid)
    reject('session ID integer cannot become boolean', lambda e: sid_value(e, 'Length', False), sid)
    reject('zero/nonzero session ID evidence cannot disappear', lambda e: target_message(e, 'Session ID').pop('Session ID'), sid)
    def drop_all_new(e):
        for m in messages(e):
            for k in MESSAGE_KEYS:
                m.pop(k, None)
    reject('deleting all hello metadata still enters by original message Type', drop_all_new, sid)

    modes = first(native, lambda e: any('PSK Key Exchange Modes' in m for m in messages(e)))
    reject('PSK mode vector is literal', lambda e: target_message(e, 'PSK Key Exchange Modes').update({'PSK Key Exchange Modes': 'AA=='}), modes)
    offer = first(native, lambda e: any('PSK Offer' in m for m in messages(e)))
    def offer_item(e, key, value):
        target_message(e, 'PSK Offer')['PSK Offer']['Identities'][0][key] = value
    reject('PSK identity hash is literal', lambda e: offer_item(e, 'SHA256', '0' * 64), offer)
    reject('PSK binder length is literal', lambda e: offer_item(e, 'Binder Length', 17), offer)
    reject('PSK age is literal u32', lambda e: offer_item(e, 'Obfuscated Ticket Age', target_message(e, 'PSK Offer')['PSK Offer']['Identities'][0]['Obfuscated Ticket Age'] ^ 1), offer)
    selection = first(native, lambda e: any('PSK Selection' in m for m in messages(e)))
    def selection_value(e, key, value):
        target_message(e, 'PSK Selection')['PSK Selection'][key] = value
        context = e['session'].get('Session Establishment') or {}
        if 'PSK Selection' in context:
            context['PSK Selection'][key] = value
    reject('PSK selection cannot verify binder', lambda e: selection_value(e, 'Binder Verified', True), selection)
    reject('PSK selection cannot confirm cross-connection ticket', lambda e: selection_value(e, 'Ticket Association', 'confirmed'), selection)
    reject('selected PSK index belongs to source offer', lambda e: selection_value(e, 'Selected Identity', 65535), selection)
    reject('PSK offer match cannot become guessed resumption', lambda e: selection_value(e, 'Offer Match', 'resumed-session'), selection)

    ticket = first(native, lambda e: any('New Session Ticket' in m for m in messages(e)))
    def ticket_value(e, key, value):
        target_message(e, 'New Session Ticket')['New Session Ticket'][key] = value
    reject('NST lifetime is literal', lambda e: ticket_value(e, 'Lifetime Hint', target_message(e, 'New Session Ticket')['New Session Ticket']['Lifetime Hint'] + 1), ticket)
    reject('NST ticket hash is literal', lambda e: target_message(e, 'New Session Ticket')['New Session Ticket']['Ticket'].update(SHA256='0' * 64), ticket)
    reject('NST ticket length is literal', lambda e: target_message(e, 'New Session Ticket')['New Session Ticket']['Ticket'].update(Length=1), ticket)
    clear_ticket = first(native, lambda e: any('New Session Ticket' in m and not m['New Session Ticket']['Record Authentication Verified'] for m in messages(e)))
    reject('clear NST cannot become authenticated', lambda e: ticket_value(e, 'Record Authentication Verified', True), clear_ticket)
    reject('NST does not export raw ticket metadata', lambda e: ticket_value(e, 'Raw Ticket', 'forbidden-raw-placeholder'), ticket)

    context = first(native, lambda e: e['protocol'] == 'tls' and bool(e['session'].get('Session Establishment')))
    reject('context evidence scope is current observed connection', lambda e: e['session']['Session Establishment'].update({'Evidence Scope': 'confirmed prior session'}), context)
    reject('context observations do not verify certificate trust', lambda e: e['session']['Session Establishment'].update({'Certificate Trust': 'verified'}), context)
    legacy = first(native, lambda e: 'Session ID Resumption Indicated' in (e['session'].get('Session Establishment') or {}))
    reject('legacy indicated flag follows literal current SID echo', lambda e: e['session']['Session Establishment'].update({'Session ID Resumption Indicated': not e['session']['Session Establishment']['Session ID Resumption Indicated']}), legacy)
    compat = first(native, lambda e: 'Compatibility Session ID Echo' in (e['session'].get('Session Establishment') or {}))
    reject('TLS13 compatibility echo does not establish legacy resumption', lambda e: e['session']['Session Establishment'].update({'Session ID Resumption Indicated': True}), compat)
    reject('valid ClientHello cannot inherit established context', lambda e: e['session'].update({'Session Establishment': copy.deepcopy(native[context]['session']['Session Establishment'])}), sid)
    reject('native flow change cannot borrow another connection hello', lambda e: e.update(flow=e['flow'] + 1000000), context)
    reject('direction has strict integer schema', lambda e: e.update(direction=False), sid)
    reject('PDU ID must remain a positive literal ID', lambda e: (e.update(id=0), e['field_values'].update(pdu=0)), sid)
    reject('snapshot schema cannot become boolean true', lambda e: e['field_values'].update(schema_version=True), sid)
    def domain_change(e):
        e['domain']['Interface'] += 1
        for ref in e['source_bytes']['PacketRefs']:
            ref['Domain'] = copy.deepcopy(e['domain'])
        if e['source_bytes'].get('CompletedIn'):
            e['source_bytes']['CompletedIn']['Domain'] = copy.deepcopy(e['domain'])
        e['field_values']['source'] = copy.deepcopy(e['source_bytes'])
    reject('capture interface is physical source evidence', domain_change, sid)
    def source_change(e):
        old = e['source_bytes']['PacketRefs'][0]['Number']
        e['source_bytes']['PacketRefs'][0]['Number'] = next(n for n in physical if n != old)
        e['field_values']['source'] = copy.deepcopy(e['source_bytes'])
    reject('same public source copies cannot move literal record to another frame', source_change, sid)
    reject('GetFields alone cannot contradict Session metadata', lambda e: target_message(dict(session=e['fields']), 'Session ID')['Session ID'].update(Length=999), sid, coherent=False)
    reject('decoded metadata cannot hide a GetFields refusal', lambda e: e.update(fields_error='ContextRequired'), sid)
    reject('clear hello has no authenticated handshake claim', lambda e: target_message(e, 'Session ID').update(Authenticated=True), sid)
    def new_alias(e, name, origin, value):
        e['field_values']['values'].append(dict(name=name, origin=origin, value=value))
    reject('guessed tls.resumed alias is forbidden', lambda e: new_alias(e, 'tls.resumed', 'context', True), sid, coherent=False)
    def alias_origin(e, prefix, origin):
        next(v for v in e['field_values']['values'] if v['name'].startswith(prefix))['origin'] = origin
    reject('wire metadata aliases are PDU scoped', lambda e: alias_origin(e, 'tls.handshake.session_id_length', 'context'), sid, coherent=False)
    reject('retained context aliases are not new wire messages', lambda e: alias_origin(e, 'tls.session.', 'pdu'), context, coherent=False)
    reject('metadata must not invent a TLS-record byte range', lambda e: next(v for v in e['field_values']['values'] if v['name'] == 'tls.handshake.session_id_length').update(pdu_range={'offset': 0, 'length': 1}), sid, coherent=False)
    def remove_messages(e):
        e['session'].pop('Handshake Messages', None)
    reject('removing a hello cannot preserve later established context', remove_messages, sid)
    ordinary = first(native, lambda e: e['protocol'] == 'tls' and not messages(e)
                     and bool(e['session'].get('Session Establishment')))
    reject('ordinary post-SH record with all context and aliases removed remains in audit admission',
           lambda e: e['session'].pop('Session Establishment'), ordinary, reselect=True)

    def reject_reference(name, modified):
        try:
            proof = inspect(native, parents, modified, physical)
            assert proof['every_new_handshake_source_verified'] and not proof['gaps']
        except (AssertionError, KeyError, ValueError, TypeError) as error:
            controls.append(dict(name=name, rejected=True, failure_type=type(error).__name__))
        else:
            raise AssertionError('semantic reference corruption accepted: ' + name)

    source_event = native[sid]
    source_key = next((r['Number'], sha(base64.b64decode(source_event['raw'], validate=True)))
                      for r in source_event['source_bytes']['PacketRefs']
                      if reference['records'][r['Number'], sha(base64.b64decode(source_event['raw'], validate=True))])
    modified = dict(reference)
    modified['records'] = reference['records'].copy()
    bucket = copy.deepcopy(reference['records'][source_key])
    ws_node = bucket[0][1]
    ws_sid = next(node for node in walk(ws_node) if 'tls.handshake.session_id_length' in node)
    ws_sid['tls.handshake.session_id_length'] = '999'
    modified['records'][source_key] = bucket
    reject_reference('WS decoded SID field must equal its actual literal handshake bytes', modified)

    quic_indices = [i for i, e in enumerate(native) if e['protocol'] == 'quic' and messages(e)]
    if quic_indices:
        qi = quic_indices[0]
        reject('QUIC Initial protection is not authenticated TLS peer identity', lambda e: e['session'].update({'Peer Identity Verified': True}), qi)
        reject('QUIC Initial TLS message cannot claim peer authentication', lambda e: target_message(e, 'Session ID').update(Authenticated=True), qi)
        reject('QUIC new message metadata cannot disappear', drop_all_new, qi)
        def crypto_change(e):
            frame = next(f for f in e['session']['Frames'] if f.get('Frame Type') == 'CRYPTO')
            raw = bytearray(base64.b64decode(frame['Crypto Data'], validate=True))
            raw[-1] ^= 1
            frame['Crypto Data'] = base64.b64encode(raw).decode('ascii')
        reject('native QUIC CRYPTO must match actually decrypted WS bytes', crypto_change, qi)
        qc = first(native, lambda e: e['protocol'] == 'quic' and bool(e['session'].get('Session Establishment')))
        reject('QUIC context aliases cannot vanish while Session remains', lambda e: e['field_values'].update(values=[v for v in e['field_values']['values'] if not v['name'].startswith('tls.session.')]), qc, coherent=False)
        qo = first(native, lambda e: e['protocol'] == 'quic' and not messages(e)
                   and bool(e['session'].get('Session Establishment')))
        def remove_quic_context(e):
            e['session'].pop('Session Establishment')
            # QUIC GetFields is a wire envelope; remove any accidental copy too.
            if isinstance(e.get('fields'), dict):
                e['fields'].pop('Session Establishment', None)
        reject('authenticated QUIC post-SH context deletion remains in audit admission',
               remove_quic_context, qo, reselect=True)
        qe = native[qi]
        modified = dict(reference)
        modified['quic'] = reference['quic'].copy()
        for ref in qe['source_bytes']['PacketRefs']:
            modified['quic'][ref['Number'], sha(base64.b64decode(qe['raw'], validate=True))] = []
        reject_reference('losing first duplicate-key coalesced QUIC Initial cannot pass source verification', modified)

    eap_indices = [i for i, e in enumerate(native)
                   if e['protocol'] == 'tls' and e['source_bytes'].get('ParentPDUs') and messages(e)
                   and not any(reference['records'][r['Number'], sha(base64.b64decode(e['raw'], validate=True))]
                               for r in e['source_bytes']['PacketRefs'])]
    if eap_indices:
        ei = next(i for i in eap_indices if native[i]['source_bytes']['ParentPDU'] in parents)
        def parent_change(e):
            source = e['source_bytes']
            other = next(i for i in parents if i != source['ParentPDU'])
            source.update(ParentPDU=other, ParentPDUs=[other])
            e['field_values']['source'] = copy.deepcopy(source)
        reject('RADIUS/EAP child cannot point to an unrelated literal parent', parent_change, ei)

    bridge_document = json.loads(preservation.read_text('utf-8-sig'))
    producer_document = json.loads(producer.read_text('utf-8-sig'))
    matching = next(i for i, r in enumerate(bridge_document['results']) if r['SDK_export_path'] == str(events_path))
    bridge_mutations = [
        ('composed old-source bridge must bind current SDK hash', lambda d: d['results'][matching].update(SDK_SHA256='0' * 64)),
        ('composed old-source bridge must bind original capture hash', lambda d: d['results'][matching].update(capture_SHA256='0' * 64)),
        ('composed old-source bridge cannot translate source IDs', lambda d: d.update(generated_ID_translation=True)),
        ('composed old-source bridge must preserve all prior values', lambda d: d.update(all_old_scalar_types_values_arrays_and_alias_order_preserved=False)),
    ]
    for name, mutation in bridge_mutations:
        document = copy.deepcopy(bridge_document)
        mutation(document)
        try:
            bind_preservation(document, producer_document, events_path)
        except (AssertionError, KeyError, ValueError, TypeError) as error:
            controls.append(dict(name=name, rejected=True, failure_type=type(error).__name__))
        else:
            raise AssertionError('composition corruption accepted: ' + name)
    return dict(scope=__doc__, source=provenance['source'], semantic_controls=len(controls),
                all_rejected=all(c['rejected'] for c in controls), controls=controls,
                current_metadata_handshakes=baseline['new_handshakes_source_verified'],
                current_context_events=baseline['new_handshake_counts']['context_events'],
                capture_SHA256=provenance['capture_SHA256'], SDK_SHA256=provenance['SDK_SHA256'],
                reference_SHA256=provenance['reference_SHA256'],
                source_flow_record_continuity_bridge_SHA256=provenance['source_flow_record_continuity_bridge_SHA256'],
                auditor_SHA256=file_sha(Path(__file__).with_name('public_tls_metadata_source_audit.py')),
                negative_driver_SHA256=file_sha(Path(__file__)),
                file_digest_rejection_used_for_semantic_controls=False,
                metadata_public_views_synchronized_where_relevant=True)


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
    print(json.dumps({k: v for k, v in proof.items() if k != 'controls'}), flush=True)
