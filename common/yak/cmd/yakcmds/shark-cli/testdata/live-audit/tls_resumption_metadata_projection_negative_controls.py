"""Challenge exact old-value preservation and retained-generation metadata.

Mutations are in-memory semantic changes. No capture/hash/file is modified.
Where relevant both public SDK projections and every CLI PDU occurrence are
updated coherently, so a new-metadata negative must fail its source oracle.
"""
import copy
import json
from pathlib import Path

from tls_resumption_audit import rows
from tls_resumption_metadata_projection_audit import (
    ALIASES, arguments, audit, expected_origin, field_map, inspect,
    metadata_aliases, sha, source_expectations,
)


def run(args):
    audit(args)  # Positive must pass before any rejection can be counted.
    original = {'SDK': rows(args.after_events), 'CLI': rows(args.after_cli),
                'flows': json.loads(args.after_flows.read_text('utf-8-sig'))}
    before = {'SDK': rows(args.before_events), 'CLI': rows(args.before_cli),
              'flows': json.loads(args.before_flows.read_text('utf-8-sig'))}
    validator = lambda events: source_expectations(args.capture, events, args.source_capture,
        args.source_manifest, args.source_events, args.source_oracle, args.control_manifest)

    def event(data):
        return next(e for e in data['SDK'] if e['protocol'] == 'tls' and e.get('session'))

    def context(data):
        return next(e for e in data['SDK'] if e['protocol'] == 'tls' and (e['session'] or {}).get('Session Establishment'))

    def metadata_event(data):
        return next(e for e in data['SDK'] if e['protocol'] == 'tls' and
                    any(v['name'] in ALIASES for v in e['field_values']['values']))

    def packet(data):
        return next(row for row in data['CLI'] if row.get('tcp_flow_id') and row.get('stream_id'))

    def first_message(data):
        for e in data['SDK']:
            for message in (e.get('session') or {}).get('Handshake Messages') or []:
                if message['Type'] == 1:
                    return e, message
        raise AssertionError('source-bound ClientHello control is missing')

    def old_alias(e, name):
        return next(v for v in e['field_values']['values'] if v['name'] == name)

    def sync(data, e, metadata=False):
        if metadata:
            e['fields'] = copy.deepcopy(e['session'])
            wanted = metadata_aliases(e['session'].get('Handshake Messages') or [], e['session'].get('Session Establishment') or {})
            old = [v for v in e['field_values']['values'] if v['name'] not in ALIASES]
            additions = [{'name': name, 'value': value, 'origin': expected_origin(name)}
                         for name in sorted(wanted) for value in wanted[name]]
            e['field_values']['values'] = old + additions
        for row in data['CLI']:
            for pdu in (row.get('pdus') or []) + (row.get('published_pdus') or []):
                if pdu['id'] == e['id']:
                    pdu['field_values'] = copy.deepcopy(e['field_values'])
                    pdu['fields'] = field_map(e['field_values'])
                    pdu['byte_source'] = copy.deepcopy(e['source_bytes'])

    controls = []
    def add(name, mutate):
        current = copy.deepcopy(original)
        mutate(current)
        try:
            inspect(before['SDK'], current['SDK'], before['CLI'], current['CLI'],
                    before['flows'], current['flows'], validator)
        except (AssertionError, KeyError, StopIteration) as err:
            controls.append({'name': name, 'rejected': True, 'failure_type': type(err).__name__})
        else:
            raise AssertionError(('semantic corruption accepted', name))

    add('SDK original protocol cannot change', lambda d: event(d).__setitem__('protocol', 'tcp'))
    add('SDK original scalar type cannot become floating point', lambda d: event(d).__setitem__('id', float(event(d)['id'])))
    add('SDK original field cannot disappear', lambda d: event(d)['session'].pop('Record Version'))
    add('SDK event order is preserved', lambda d: d['SDK'].reverse())
    add('CLI packet label is preserved', lambda d: packet(d).__setitem__('protocol', 'forged'))
    add('CLI stream ID cannot move to another generation', lambda d: packet(d).__setitem__('stream_id', packet(d)['stream_id'] + 1))
    add('CLI TCP generation cannot move to another connection', lambda d: packet(d).__setitem__('tcp_flow_id', packet(d)['tcp_flow_id'] + 1))
    add('default flow state is exact', lambda d: d['flows'].__setitem__('error', 'invented'))

    def alias_value(d):
        e = event(d)
        old_alias(e, 'tls.record.version')['value'] += 1
        sync(d, e)
    add('existing alias scalar cannot change even with matching CLI', alias_value)

    def alias_type(d):
        e = event(d)
        old_alias(e, 'tls.record.length')['value'] = str(old_alias(e, 'tls.record.length')['value'])
        sync(d, e)
    add('existing alias type cannot be coerced', alias_type)

    def alias_order(d):
        e = event(d)
        values = e['field_values']['values']
        a = next(i for i,v in enumerate(values) if v['name'] == 'source')
        b = next(i for i,v in enumerate(values) if v['name'] == 'destination')
        values[a], values[b] = values[b], values[a]
        sync(d, e)
    add('existing alias relative order cannot change', alias_order)

    def source_ref(d):
        e = event(d)
        e['source_bytes']['PacketRefs'][0]['Number'] += 1
        e['field_values']['source'] = copy.deepcopy(e['source_bytes'])
        sync(d, e)
    add('source packet reference cannot move even when public copies agree', source_ref)

    def native_flow(d):
        e = event(d)
        e['tcp_flow_id'] += 1
        item = old_alias(e, 'tcp.flow.id')
        item['value'] = e['tcp_flow_id']; item['exact_value'] = str(e['tcp_flow_id'])
        sync(d, e)
    add('native TCP generation identifiers are never normalized', native_flow)

    def extra_alias(d):
        e = event(d)
        e['field_values']['values'].append({'name':'tls.resumed','value':True,'origin':'context'})
        sync(d, e)
    add('noncontract guessed resumed alias cannot be added', extra_alias)
    add('noncontract metadata sibling cannot be added', lambda d: first_message(d)[1].__setitem__('Raw Ticket', 'unrequested-raw-material'))

    def guessed_context(d):
        e = context(d)
        e['session']['Session Establishment']['Session ID Resumption Indicated'] = 'guessed'
        sync(d, e, metadata=True)
    add('allowed metadata namespace cannot hide an incorrect wire fact', guessed_context)

    def missing_sid(d):
        e, message = first_message(d)
        message.pop('Session ID')
        e['fields'] = copy.deepcopy(e['session'])
        e['field_values']['values'] = [v for v in e['field_values']['values']
            if v['name'] not in ['tls.handshake.session_id_length', 'tls.handshake.session_id.sha256']]
        sync(d, e)
    add('new zero/nonzero session ID facts cannot be silently removed', missing_sid)

    def new_origin(d):
        e = metadata_event(d)
        item = next(v for v in e['field_values']['values'] if v['name'] in ALIASES)
        item['origin'] = 'event'
        sync(d, e)
    add('new alias cannot claim an unrelated origin', new_origin)

    def cross_generation(d):
        contexts = [e for e in d['SDK'] if e['protocol'] == 'tls' and (e['session'] or {}).get('Session Establishment')]
        source = next(e for e in contexts if e['session']['Session Establishment'].get('PSK Selected') is True)
        target = next(e for e in contexts if e['session']['Session Establishment'].get('PSK Selected') is False)
        target['session']['Session Establishment'] = copy.deepcopy(source['session']['Session Establishment'])
        sync(d, target, metadata=True)
    add('coherent metadata and aliases cannot inherit a preceding TLS generation', cross_generation)
    add('GetFields-only metadata cannot contradict the current Session and CLI',
        lambda d: context(d)['fields']['Session Establishment'].__setitem__('Certificate Trust', 'verified'))

    return {'scope': __doc__, 'semantic_controls': len(controls), 'all_rejected': True,
            'SDK_and_CLI_public_views_synchronized_where_relevant': True,
            'capture_SHA256': sha(args.capture), 'after_SDK_SHA256': sha(args.after_events),
            'after_CLI_SHA256': sha(args.after_cli), 'controls': controls,
            'auditor_SHA256': sha(Path(__file__).with_name('tls_resumption_metadata_projection_audit.py')),
            'negative_driver_SHA256': sha(Path(__file__))}


if __name__ == '__main__':
    args = arguments().parse_args()
    for path in vars(args).values():
        if isinstance(path, Path):
            assert not any((p/'.git').exists() for p in [path.resolve(), *path.resolve().parents])
    proof = run(args)
    with args.out.open('x', encoding='utf8') as output:
        json.dump(proof, output, indent=2); output.write('\n')
    print(json.dumps({k:v for k,v in proof.items() if k != 'controls'}), flush=True)
