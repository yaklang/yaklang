"""Exercise generated RPC references in a deliberately different CLI namespace.

Literal wire Call ID/context/opnum/lengths never undergo ID conversion. The
original whole capture and logical audits must pass separately; these controls
target the exact full typed SDK-to-CLI projection, not TCP or NDR semantics.
"""
import argparse
import collections
import copy
import json
from pathlib import Path

from dcerpc_cli_invariants import GENERATED_RPC_REFERENCES, identity, native_identity, selected_projection
from mail_rtsp_export_invariants import rows, sha


def controls(events, cli):
    native = {native_identity(e): e for e in rows(events) if e['protocol'] == 'dcerpc'}
    mapping = {0: 0, **{e['id']: e['id']+100000 for e in native.values()}}
    observed = {p['id']: p for row in rows(cli) for p in (row.get('pdus') or [])+(row.get('published_pdus') or []) if p['protocol'] == 'dcerpc'}
    positives = []; refused = []
    for pdu in observed.values():
        event = native[identity(pdu)]
        if not (event.get('session') or {}).get('Reassembled Message'):
            continue
        remapped = copy.deepcopy(pdu)
        remapped['id'] = mapping[event['id']]; remapped['response_to'] = mapping[event['response_to']]
        remapped['field_values']['pdu'] = remapped['id']
        references = GENERATED_RPC_REFERENCES | {'pdu.id', 'pdu.response_to'}
        for value in remapped['field_values']['values']:
            if value['name'] in references:
                value['value'] = mapping[value['value']]; value['exact_value'] = str(value['value'])

        def flat(value):
            fields = collections.defaultdict(list)
            for field in value['field_values']['values']:
                fields[field['name']].append(field['value'])
            value['fields'] = dict(fields)

        flat(remapped)
        selected_projection(remapped, event, mapping)
        positives.append(dict(SDK_PDU=event['id'], CLI_PDU=remapped['id']))
        names = GENERATED_RPC_REFERENCES | {'pdu.response_to', 'dcerpc.cn_call_id'}
        for name in sorted(names):
            values = [v for v in remapped['field_values']['values'] if v['name'] == name]
            if not values:
                continue
            altered = copy.deepcopy(remapped)
            field = next(v for v in altered['field_values']['values'] if v['name'] == name)
            # An internally consistent flat/typed cache is still wrong when a
            # generated parent is unconverted or a wire Call ID is converted.
            field['value'] += -100000 if name != 'dcerpc.cn_call_id' else 100000
            if 'exact_value' in field:
                field['exact_value'] = str(field['value'])
            flat(altered)
            try:
                selected_projection(altered, event, mapping)
            except AssertionError:
                refused.append(dict(SDK_PDU=event['id'], wrong_field=name))
            else:
                raise AssertionError(('namespace corruption escaped full projection', name))
    assert len(positives) >= 3 and refused
    return dict(scope=__doc__, SDK_sha256=sha(events), CLI_sha256=sha(cli), positive_changed_namespace_snapshots=positives,
                refused_namespace_corruptions=refused, all_generated_and_literal_ID_corruptions_refused=True)


if __name__ == '__main__':
    ap = argparse.ArgumentParser(description=__doc__)
    for key in ['events', 'cli', 'out']:
        ap.add_argument('--'+key, type=Path, required=True)
    args = ap.parse_args()
    for path in vars(args).values():
        assert not any((p/'.git').exists() for p in [path.resolve(), *path.resolve().parents])
    result = controls(args.events, args.cli)
    args.out.write_text(json.dumps(result, indent=2)+'\n', encoding='utf8')
    print(json.dumps(dict(positive_snapshots=len(result['positive_changed_namespace_snapshots']), rejected_corruptions=len(result['refused_namespace_corruptions']))))
