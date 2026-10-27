"""Compare whole CLI exports when the only change is verified RPC aliases.

Both ordinary and availability-only published PDUs are covered. All original
rows, IDs, values, types, order, sources, status and presentation stay exact.
"""
import argparse
import collections
import copy
import itertools
import json
from pathlib import Path

from dcerpc_cli_invariants import identity, native_identity, selected_projection
from dcerpc_fragment_sdk_invariants import PATHS
from mail_rtsp_export_invariants import canonical, rows, sha


def audit(before, after, sdk, sdk_certificate):
    proof = json.loads(sdk_certificate.read_text(encoding='utf8'))
    assert proof['after_sha256'] == sha(sdk)
    assert proof['all_original_wire_fields_sources_IDs_flows_order_exact'] is True
    native = collections.defaultdict(list)
    for e in rows(sdk):
        if e['protocol'] == 'dcerpc':
            native[native_identity(e)].append(e)
    pairs = {}; mapping = {0: 0}; seen = {}
    for row in rows(after):
        for pdu in (row.get('pdus') or []) + (row.get('published_pdus') or []):
            if pdu['protocol'] != 'dcerpc':
                continue
            if pdu['id'] in seen:
                assert canonical(seen[pdu['id']]) == canonical(pdu), 'all repeat publications preserve the same complete snapshot'
                continue
            seen[pdu['id']] = pdu
            candidates = native[identity(pdu)]
            assert len(candidates) == 1, 'unique SDK wire/source/endpoint identity'
            event = candidates[0]
            assert event['id'] not in mapping, 'one native PDU cannot become several generated CLI identities'
            mapping[event['id']] = pdu['id']; pairs[pdu['id']] = (pdu, event)
    for pdu, event in pairs.values():
        selected_projection(pdu, event, mapping)
    physical = 0; selected_instances = 0; publications = 0; counts = collections.Counter()
    for old, new in itertools.zip_longest(rows(before), rows(after)):
        assert old is not None and new is not None, 'all physical rows remain present and ordered'
        expected = copy.deepcopy(new)
        for key in ['pdus', 'published_pdus']:
            for pdu in expected.get(key) or []:
                if pdu['protocol'] != 'dcerpc':
                    continue
                assert pdu['id'] in pairs
                selected_instances += 1
                publications += int(key == 'published_pdus')
                pdu['field_values']['values'] = [v for v in pdu['field_values']['values'] if v['name'] not in PATHS]
                for name in PATHS:
                    pdu['fields'].pop(name, None)
        assert canonical(expected) == canonical(old), ('all original physical/ordinary/published metadata except certified added RPC aliases', new['number'])
        physical += 1; counts[new['protocol']] += 1
    return dict(scope=__doc__, before_sha256=sha(before), after_sha256=sha(after), SDK_sha256=sha(sdk),
                SDK_certificate_sha256=sha(sdk_certificate), physical_rows_compared=physical,
                unique_RPC_PDUs=len(pairs), RPC_PDU_instances=selected_instances, RPC_publications=publications,
                protocol_label_counts=dict(sorted(counts.items())), SDK_CLI_namespace_equality_assumed=False,
                all_original_rows_values_types_IDs_order_ordinary_and_published_metadata_exact=True)


if __name__ == '__main__':
    ap = argparse.ArgumentParser(description=__doc__)
    for key in ['before', 'after', 'sdk', 'sdk-certificate', 'out']:
        ap.add_argument('--'+key, type=Path, required=True)
    args = ap.parse_args()
    for path in vars(args).values():
        assert not any((p/'.git').exists() for p in [path.resolve(), *path.resolve().parents])
    result = audit(args.before, args.after, args.sdk, args.sdk_certificate)
    args.out.write_text(json.dumps(result, indent=2)+'\n', encoding='utf8')
    print(json.dumps({k: result[k] for k in ['physical_rows_compared', 'unique_RPC_PDUs', 'RPC_publications', 'all_original_rows_values_types_IDs_order_ordinary_and_published_metadata_exact']}))
