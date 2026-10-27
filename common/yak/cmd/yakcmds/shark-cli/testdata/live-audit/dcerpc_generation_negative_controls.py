"""Corrupt two declared constructed controls while keeping original wire bytes.

Re-sealing metadata input hashes isolates semantic guards from simple hash
checks. Temporary fixtures are not independent wire certificates. No whole
public-corpus corruption coverage is claimed by these bounded controls.
"""
import argparse
import base64
import copy
import json
import shutil
import tempfile
from pathlib import Path

from dcerpc_generation_control_audit import audit
from dcerpc_wire_audit import audit as wire_audit
from mail_rtsp_export_invariants import canonical, rows, sha


def controls(root):
    case = root/'reply-before-any-request'; plan = json.loads((case/'manifest.json').read_text(encoding='utf8'))
    original = list(rows(case/'after/events.jsonl'))
    by_frame = {e['source_bytes']['PacketRefs'][0]['Number']: e for e in original}
    stale = next(p for p in plan['PDUs'] if p['label'].startswith('orphan-reply') and base64.b64decode(p['wire_base64'])[3] & 2)
    fresh = next(p for p in plan['PDUs'] if p['label'] == 'fresh-request')
    reply = next(p for p in plan['PDUs'] if p['label'] == 'fresh-reply')
    target = by_frame[stale['frame']]['id']; fresh_id = by_frame[fresh['frame']]['id']; reply_id = by_frame[reply['frame']]['id']
    alias = next(v for v in by_frame[reply['frame']]['field_values']['values'] if v['name'] == 'pdu.response_to')

    def owner(e, value):
        e['response_to'] = value
        values = e['field_values']['values']; values[:] = [v for v in values if v['name'] != 'pdu.response_to']
        if value:
            item = copy.deepcopy(alias); item['value'] = value; item['exact_value'] = str(value); values.append(item); values.sort(key=lambda v: v['name'])

    changes = [
        ('future-request-stolen-by-old-reply', target, lambda e: owner(e, fresh_id)),
        ('real-fresh-reply-loses-owner', reply_id, lambda e: owner(e, 0)),
        ('false-matched-status', target, lambda e: e['session'].__setitem__('Association Status', 'matched')),
        ('missing-unmatched-diagnostic', target, lambda e: e['session'].pop('Unmatched')),
        ('false-complete-context', target, lambda e: e['session'].__setitem__('Context Level', 'observed')),
        ('false-matched-request-name', reply_id, lambda e: e['session'].__setitem__('Matched Request', 'EPM.ept_delete')),
        ('wrong-typed-source', target, lambda e: e['field_values']['source'].__setitem__('SHA256', '0'*64)),
        ('wrong-typed-status', target, lambda e: next(v for v in e['field_values']['values'] if v['name'] == 'status').__setitem__('value', 'malformed')),
    ]
    for name, change in [
        ('false-stub-decoded', lambda m: m.__setitem__('Stub Decoded', True)),
        ('wrong-stub-octets', lambda m: m.__setitem__('Stub', 'AAAA')),
        ('missing-parent', lambda m: m['Fragment Sources'].pop()),
        ('wrong-parent-interval', lambda m: m['Fragment Sources'][1].__setitem__('Stub Range', [16, 2048])),
        ('false-stub-frame-projection', lambda m: m['Byte Source'].__setitem__('PacketRefs', [{'Number': stale['frame']}])),
    ]:
        changes.append((name, target, lambda e, mutate=change: mutate(e['session']['Reassembled Message'])))
    refused = []
    with tempfile.TemporaryDirectory(prefix='rpc-generation-negative-') as directory:
        temp = Path(directory); shutil.copyfile(case/'control.pcap', temp/'control.pcap')
        for name, identifier, change in changes:
            altered = copy.deepcopy(original); change(next(e for e in altered if e['id'] == identifier))
            for old, new in zip(original, altered):
                for key in ['id', 'raw', 'fields', 'source_bytes', 'source', 'destination', 'domain', 'flow']:
                    assert canonical(old.get(key)) == canonical(new.get(key)), 'logical mutations retain original physical data'
            events = temp/'events.jsonl'; events.write_text(''.join(json.dumps(e)+'\n' for e in altered), encoding='utf8')
            manifest = temp/'manifest.json'; manifest.write_text(json.dumps(plan), encoding='utf8')
            proof = json.loads((case/'after/wire-physical-audit.json').read_text(encoding='utf8'))
            proof['events_sha256'] = sha(events); proof['control_manifest_sha256'] = sha(manifest)
            proof['negative_control_metadata_only'] = True
            certificate = temp/'negative-input.json'; certificate.write_text(json.dumps(proof), encoding='utf8')
            try:
                audit(manifest, root.parent/'capture-01/wire-receipts.json', case/'before/events.jsonl', events, certificate, case/'oracle.json')
            except (AssertionError, KeyError):
                refused.append(name)
            else:
                raise AssertionError(('generation corruption escaped the complete logical audit', name))
        # A correctly formed header cannot excuse a semantically invalid
        # call-start control; acceptance is checked from original TCP bytes.
        overlap = root/'overlap-before-old-reply'
        try:
            wire_audit(overlap/'control.pcap', overlap/'after/events.jsonl', overlap/'oracle.json', None, True, False)
        except AssertionError:
            refused.append('semantic-refusal-needs-original-wire-cause')
        else:
            raise AssertionError('generic wire checker silently accepted an unproven semantic refusal')
    return dict(scope=__doc__, control_manifest_SHA256=sha(case/'manifest.json'), checks=len(refused), refused=refused,
                all_derived_metadata_corruptions_refused=True)


if __name__ == '__main__':
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument('--controls', type=Path, required=True); ap.add_argument('--out', type=Path, required=True)
    args = ap.parse_args()
    for path in vars(args).values():
        assert not any((p/'.git').exists() for p in [path.resolve(), *path.resolve().parents])
    result = controls(args.controls)
    args.out.write_text(json.dumps(result, indent=2)+'\n', encoding='utf8')
    print(json.dumps(result))
