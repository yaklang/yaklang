"""Prove that the owned TLS metadata verifier rejects semantic corruption.

Mutations run in memory against the same physical capture and independent
Wireshark messages. File-digest rejection alone is not a successful control.
"""
import argparse
import copy
import json
from pathlib import Path

from tls_resumption_audit import rows
from tls_resumption_metadata_audit import inspect, sha


def run(capture, events, manifest, oracle):
    original = rows(events)
    peer = json.loads(manifest.read_text('utf-8-sig'))
    reference = json.loads(oracle.read_text('utf-8-sig'))
    inspect(capture, original, peer, reference)

    def message(data, kind, key=None, flow=None):
        for e in data:
            if e['protocol'] != 'tls' or flow is not None and e['tcp_flow_id'] != flow:
                continue
            for m in e['session'].get('Handshake Messages') or []:
                if m['Type'] == kind and (key is None or key in m):
                    return e, m
        raise AssertionError(('control target missing', kind, key, flow))

    def selected(data):
        return message(data, 2, 'PSK Selection')

    def ticket(data):
        return message(data, 4, 'New Session Ticket')

    def alias(e, key):
        return next(v for v in e['field_values']['values'] if v['name'] == key)

    def context(data, fresh=False):
        return next(e for e in data if e['protocol'] == 'tls' and e['tcp_flow_id'] == (5 if fresh else 6) and e['session'].get('Session Establishment'))

    def set_message(kind, key, inner, value, flow=None):
        def mutate(data):
            _, m = message(data, kind, key, flow)
            m[key][inner] = value
        return mutate

    def set_selected(inner, value):
        return set_message(2, 'PSK Selection', inner, value)

    def set_offer(inner, value):
        def mutate(data):
            _, m = message(data, 1, 'PSK Offer')
            m['PSK Offer']['Identities'][0][inner] = value
        return mutate

    controls = []
    def add(name, mutate, sync_fields=True):
        data = copy.deepcopy(original)
        mutate(data)
        if sync_fields:
            # Challenge the wire/semantic oracle even when both public views
            # consistently carry the same corrupt metadata.
            for e in data:
                if e['protocol'] == 'tls':
                    e['fields'] = copy.deepcopy(e['session'])
        try:
            inspect(capture, data, peer, reference)
        except (AssertionError, KeyError, StopIteration) as err:
            controls.append({'name': name, 'rejected': True, 'failure_type': type(err).__name__})
        else:
            raise AssertionError(('semantic corruption accepted', name))

    add('PSK selection cannot verify binder', set_selected('Binder Verified', True))
    add('PSK selected index belongs to the observed offer', set_selected('Selected Identity', 1))
    add('PSK identity hash is literal', set_selected('SHA256', '0' * 64))
    add('PSK current-offer match cannot become ticket-cache confirmation', set_selected('Offer Match', 'cached-ticket-verified'))
    add('PSK origin cannot be asserted as resumed session', set_selected('Ticket Association', 'resumption confirmed'))
    add('PSK binder length matches each offered wire binder', set_offer('Binder Length', 1))
    add('PSK age is literal u32', set_offer('Obfuscated Ticket Age', -1))
    add('PSK offer identity hash is literal', set_offer('SHA256', 'f' * 64))
    add('ticket hash is literal', set_message(4, 'New Session Ticket', 'Ticket', {'Length': 1, 'SHA256': 'f' * 64}))
    add('ticket lifetime is literal', set_message(4, 'New Session Ticket', 'Lifetime Hint', 0xffffffff))
    add('ticket acceptance is not tracked across connections', set_message(4, 'New Session Ticket', 'Ticket Association', 'verified previous connection'))
    add('zero-length session ID is retained', lambda data: message(data, 1, 'Session ID', 1)[1].pop('Session ID'))
    add('empty session ID hash is literal', set_message(1, 'Session ID', 'SHA256', '0' * 64, 1))
    add('zero session ID length cannot become boolean false', set_message(1, 'Session ID', 'Length', False, 1))
    add('raw PSK identity is not exported in metadata', set_selected('Identity Bytes', 'unrequested-raw-material'))
    add('raw ticket cannot be smuggled as a sibling handshake field', lambda data: selected(data)[1].__setitem__('Ticket Bytes', 'unrequested-raw-material'))
    add('fresh TLS13 compatibility echo is not legacy resumption', lambda data: context(data, True)['session']['Session Establishment'].__setitem__('Session ID Resumption Indicated', True))
    add('connection evidence cannot become certificate trust', lambda data: context(data)['session']['Session Establishment'].__setitem__('Certificate Trust', 'verified'))
    add('connection PSK origin remains unconfirmed', lambda data: context(data)['session']['Session Establishment']['PSK Selection'].__setitem__('Ticket Association', 'resumed'))
    add('fresh connection cannot inherit preceding PSK selection', lambda data: context(data, True)['session']['Session Establishment'].__setitem__('PSK Selection', copy.deepcopy(context(data)['session']['Session Establishment']['PSK Selection'])))
    add('new ClientHello has no established session context', lambda data: message(data, 1, 'Session ID', 8)[0]['session'].__setitem__('Session Establishment', copy.deepcopy(context(data)['session']['Session Establishment'])))
    add('TLS13 cannot expose a guessed tls.resumed alias', lambda data: context(data, True)['field_values']['values'].append({'name': 'tls.resumed', 'value': True, 'origin': 'context'}))
    add('PSK wire field is PDU scoped', lambda data: alias(selected(data)[0], 'tls.handshake.extensions.psk.identity.selected').__setitem__('origin', 'context'))
    add('context field cannot claim a new wire value', lambda data: alias(context(data), 'tls.session.psk.selected').__setitem__('origin', 'pdu'))
    add('PSK selected index alias has numeric type', lambda data: alias(selected(data)[0], 'tls.handshake.extensions.psk.identity.selected').__setitem__('value', '0'))
    add('TLS record-relative byte span is not invented for handshake metadata', lambda data: alias(selected(data)[0], 'tls.handshake.extensions.psk.identity.selected').__setitem__('pdu_range', {'offset': 0, 'length': 2}))
    add('PDU session-ID alias cannot silently disappear', lambda data: message(data, 1, 'Session ID')[0]['field_values'].__setitem__('values', [v for v in message(data, 1, 'Session ID')[0]['field_values']['values'] if v['name'] != 'tls.handshake.session_id_length']))
    add('handshake source refs are physical, not another generation', lambda data: selected(data)[0]['source_bytes']['PacketRefs'][0].__setitem__('Number', 6))
    add('handshake messages cannot be dropped', lambda data: ticket(data)[0]['session'].__setitem__('Handshake Messages', []))
    add('clear ClientHello is not authenticated record plaintext', lambda data: message(data, 1)[1].__setitem__('Authenticated', True))
    add('public GetFields cannot omit accepted session metadata', lambda data: selected(data)[0]['fields'].pop('Session Establishment'), sync_fields=False)
    add('public GetFields error cannot coexist with claimed decoded fields', lambda data: selected(data)[0].__setitem__('fields_error', 'wrong TLS source association'))
    add('current record authentication cannot contradict decrypted NST', lambda data: message(data, 4, 'New Session Ticket', 5)[0]['session'].__setitem__('Authentication Verified', False))
    for forbidden in ['tls.handshake.session_ticket', 'tls.handshake.extensions.session_ticket', 'tls.handshake.extensions.psk.identity.identity', 'tls.handshake.extensions.psk.binder.binder']:
        add('raw secret alias forbidden: ' + forbidden, lambda data, key=forbidden: selected(data)[0]['field_values']['values'].append({'name': key, 'value': 'raw-material', 'origin': 'pdu'}))
    add('semantic flow ownership cannot become zero', lambda data: selected(data)[0].__setitem__('flow', 0))
    add('TCP generation has strict integer schema', lambda data: selected(data)[0].__setitem__('tcp_flow_id', 6.0))
    add('direction has strict integer schema', lambda data: selected(data)[0].__setitem__('direction', 1.0))
    add('PDU ID cannot become zero', lambda data: selected(data)[0].__setitem__('id', 0))
    add('public source cannot change from reassembled record to decrypted child', lambda data: selected(data)[0]['source_bytes'].__setitem__('Kind', 'decrypted'))
    add('physical frame numbers have strict integer schema', lambda data: selected(data)[0]['source_bytes']['PacketRefs'][0].__setitem__('Number', float(selected(data)[0]['source_bytes']['PacketRefs'][0]['Number'])))
    add('source cannot inherit an older parent PDU', lambda data: selected(data)[0]['source_bytes'].__setitem__('ParentPDU', 3))
    add('snapshot schema version is checked', lambda data: selected(data)[0]['field_values'].__setitem__('schema_version', 0))
    add('zero-length ServerHelloDone is numeric, not boolean', lambda data: message(data, 14)[1].__setitem__('Length', False))
    return {'scope': __doc__, 'capture_SHA256': sha(capture.read_bytes()), 'SDK_SHA256': sha(events.read_bytes()),
            'manifest_SHA256': sha(manifest.read_bytes()), 'reference_SHA256': sha(oracle.read_bytes()),
            'auditor_SHA256': sha(Path(__file__).with_name('tls_resumption_metadata_audit.py').read_bytes()),
            'negative_driver_SHA256': sha(Path(__file__).read_bytes()),
            'semantic_controls': len(controls), 'public_views_synchronized_for_semantic_mutations': True,
            'all_rejected': True, 'controls': controls}


if __name__ == '__main__':
    ap = argparse.ArgumentParser(description=__doc__)
    for name in ['capture', 'events', 'manifest', 'oracle', 'out']:
        ap.add_argument('--' + name, type=Path, required=True)
    a = ap.parse_args()
    for p in vars(a).values():
        assert not any((d / '.git').exists() for d in [p.resolve(), *p.resolve().parents])
    proof = run(a.capture, a.events, a.manifest, a.oracle)
    with a.out.open('x', encoding='utf8') as f:
        json.dump(proof, f, indent=2)
        f.write('\n')
    print(json.dumps({k: v for k, v in proof.items() if k != 'controls'}), flush=True)
