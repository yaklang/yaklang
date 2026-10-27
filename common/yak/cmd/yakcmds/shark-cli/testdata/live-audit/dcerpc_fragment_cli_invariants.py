"""Bind both CLI versions to their complete SDK snapshots for RPC fragments.

Ordinary and availability-only PDUs retain the same ordered physical identities.
Only independently certified RPC presentation may change; unrelated records and
every other original physical column remain exact, including publication scope.
"""
import argparse
import collections
import copy
import itertools
import json
from pathlib import Path

from dcerpc_cli_invariants import identity, native_identity, physical_tuple, PRESENTATION, selected_projection
from mail_rtsp_export_invariants import canonical, rows, sha


def bind_cli(export, sdk):
    native = collections.defaultdict(list)
    for e in rows(sdk):
        if e['protocol'] == 'dcerpc':
            native[native_identity(e)].append(e)
    selected = {}; sdk_to_cli = {0: 0}; pairs = []
    for row in rows(export):
        for pdu in (row.get('pdus') or []) + (row.get('published_pdus') or []):
            if pdu['protocol'] != 'dcerpc':
                continue
            if pdu['id'] in selected:
                assert canonical(selected[pdu['id']]) == canonical(pdu), 'repeated publication retains its complete original snapshot'
                continue
            candidates = native[identity(pdu)]
            assert len(candidates) == 1
            event = candidates[0]
            assert event['id'] not in sdk_to_cli
            sdk_to_cli[event['id']] = pdu['id']; pairs.append((pdu, event)); selected[pdu['id']] = pdu
    for pdu, event in pairs:
        selected_projection(pdu, event, sdk_to_cli)
    expected = {e['id'] for values in native.values() for e in values if e['status'] in ['decoded', 'deferred'] and e.get('raw')}
    assert expected <= sdk_to_cli.keys(), 'all complete native RPC wire PDUs must be available in this unfiltered export'
    return selected, pairs


def audit(before, after, before_sdk, after_sdk, sdk_certificate, wire_certificate):
    preservation = json.loads(sdk_certificate.read_text(encoding='utf8'))
    assert preservation['before_sha256'] == sha(before_sdk) and preservation['after_sha256'] == sha(after_sdk)
    assert preservation['all_original_wire_fields_sources_IDs_flows_order_exact'] is True
    wire = json.loads(wire_certificate.read_text(encoding='utf8'))
    assert wire['events_sha256'] == sha(after_sdk)
    links = wire['numeric_call_links']
    assert links['all_numeric_links_match_original_request_ownership'] is True
    old_selected, _ = bind_cli(before, before_sdk)
    selected, pairs = bind_cli(after, after_sdk)
    assert old_selected.keys() == selected.keys(), 'same literal generated wire identities in these paired exports'
    for identifier in selected:
        assert identity(old_selected[identifier]) == identity(selected[identifier]), 'same original bytes/endpoints/domain/flow'
    tuples = set()
    for _, event in pairs:
        assert links['verified_response_owner_PDU_IDs'][str(event['id'])] == event['response_to']
        tuples.add(canonical([event['domain'], sorted([event['source'], event['destination']])]))
    count = 0; changes = []; publications = 0
    for old, new in itertools.zip_longest(rows(before), rows(after)):
        assert old is not None and new is not None
        a, b = copy.deepcopy(old), copy.deepcopy(new)
        for row, known in [(a, old_selected), (b, selected)]:
            for key in ['pdus', 'published_pdus']:
                for index, pdu in enumerate(row.get(key) or []):
                    if pdu['protocol'] != 'dcerpc':
                        continue
                    assert canonical(pdu) == canonical(known[pdu['id']])
                    row[key][index] = dict(id=pdu['id'], immutable_original_identity=identity(pdu))
        difference = {key for key in a.keys() | b.keys()
                      if canonical(a.get(key)) != canonical(b.get(key)) or (key in a) != (key in b)}
        if difference:
            assert difference <= PRESENTATION
            assert physical_tuple(a) == physical_tuple(b) and physical_tuple(b) in tuples
            for key in difference:
                a.pop(key, None); b.pop(key, None)
            changes.append(dict(frame=new['number'], presentation_columns=sorted(difference)))
        assert canonical(a) == canonical(b), 'all other physical/ordinary/published metadata remain exact'
        publications += len(new.get('published_pdus') or []); count += 1
    assert count == wire['physical_frames']
    return dict(scope=__doc__, before_sha256=sha(before), after_sha256=sha(after), before_SDK_sha256=sha(before_sdk),
                after_SDK_sha256=sha(after_sdk), SDK_certificate_sha256=sha(sdk_certificate), wire_certificate_sha256=sha(wire_certificate),
                physical_rows_compared=count, unique_RPC_PDUs=len(selected), availability_publication_instances=publications,
                changed_RPC_presentation_rows=changes, all_original_wire_IDs_sources_order_and_other_metadata_exact=True,
                both_versions_full_RPC_snapshots_bound_to_their_original_SDK=True, SDK_CLI_namespace_equality_assumed=False)


if __name__ == '__main__':
    ap = argparse.ArgumentParser(description=__doc__)
    for key in ['before', 'after', 'before-sdk', 'after-sdk', 'sdk-certificate', 'wire-certificate', 'out']:
        ap.add_argument('--'+key, type=Path, required=True)
    args = ap.parse_args()
    for path in vars(args).values():
        assert not any((p/'.git').exists() for p in [path.resolve(), *path.resolve().parents])
    result = audit(args.before, args.after, args.before_sdk, args.after_sdk, args.sdk_certificate, args.wire_certificate)
    args.out.write_text(json.dumps(result, indent=2)+'\n', encoding='utf8')
    print(json.dumps({k: result[k] for k in ['physical_rows_compared', 'unique_RPC_PDUs', 'availability_publication_instances', 'all_original_wire_IDs_sources_order_and_other_metadata_exact']}))
