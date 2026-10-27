"""Corrupt logical metadata on a real, independently proven RPC wire sequence.

Only derived metadata changes. Re-sealing the export hash isolates the logical
checker from its input-hash guard; raw bytes, fields, contributors and IDs must
remain literal. These temporary negative fixtures are never wire certificates.
"""
import argparse
import copy
import hashlib
import json
import tempfile
from pathlib import Path

from dcerpc_fragment_messages import audit
from mail_rtsp_export_invariants import canonical, rows


def controls(events, wire_certificate, oracle):
    audit(events, wire_certificate, oracle)
    original = list(rows(events))
    completed = next(e for e in original if (e.get('session') or {}).get('Reassembled Message'))
    target = completed['id']
    first = completed['session']['Message ID']
    reply = next(e['id'] for e in original if (e.get('session') or {}).get('Reassembled Message') and e['session']['PType'] == 2)
    cases = []

    def message(name, change):
        cases.append((name, target, lambda e: change(e['session']['Reassembled Message'])))

    for key, value in [('Message ID', target), ('Complete In PDU', first), ('Header In PDU', target),
                       ('Fragment Count', 1), ('Stub Bytes', 48), ('Stub', 'AAAA'), ('Stub Decoded', True)]:
        message('wrong-'+key, lambda m, k=key, v=value: m.__setitem__(k, v))
    message('missing-parent', lambda m: m['Fragment Sources'].pop())
    message('duplicate-parent', lambda m: m['Fragment Sources'].__setitem__(1, copy.deepcopy(m['Fragment Sources'][0])))
    message('header-included-in-stub', lambda m: m['Fragment Sources'][1].__setitem__('Stub Range', [16, 44]))
    message('wrong-stub-offset', lambda m: m['Fragment Sources'][1].__setitem__('Stub Offset', 16))
    message('wrong-stub-length', lambda m: m['Fragment Sources'][1].__setitem__('Stub Length', 28))
    message('wrong-stub-sha256', lambda m: m['Byte Source'].__setitem__('SHA256', '0'*64))
    message('wrong-source-kind', lambda m: m['Byte Source'].__setitem__('Kind', 'captured'))
    message('wrong-source-parents', lambda m: m['Byte Source']['ParentPDUs'].reverse())
    message('false-packet-projection', lambda m: m['Byte Source'].__setitem__('PacketRefs', [{'Number': 12}]))
    cases.extend([
        ('wrong-current-message-id', target, lambda e: e['session'].__setitem__('Message ID', target)),
        ('wrong-fragment-index', target, lambda e: e['session'].__setitem__('Fragment Index', 1)),
        ('missing-complete-message', target, lambda e: e['session'].pop('Reassembled Message')),
        ('premature-complete-message', first, lambda e: e['session'].__setitem__('Reassembled Message', copy.deepcopy(completed['session']['Reassembled Message']))),
        ('wrong-logical-request-length', target, lambda e: e['session'].__setitem__('Stub Bytes', 48)),
        ('wrong-observed-operation', target, lambda e: e['session'].__setitem__('OpNum', 3)),
        ('wrong-complete-response-owner', reply, lambda e: e.__setitem__('response_to', target)),
        ('premature-response-owner', reply-1, lambda e: e.__setitem__('response_to', first)),
    ])
    for key, value in [('Interface', 'SRVSVC'), ('Abstract Syntax', '0'*32), ('Interface Version Major', 4),
                       ('Interface Version Minor', 1), ('Packet Name', 'EPM.ept_delete')]:
        cases.append(('wrong-accepted-'+key, first, lambda e, k=key, v=value: e['session'].__setitem__(k, v)))
    cases.append(('wrong-matched-operation', reply, lambda e: e['session'].__setitem__('Matched Request', 'EPM.ept_delete')))
    refused = []
    with tempfile.TemporaryDirectory(prefix='rpc-logical-negative-') as directory:
        root = Path(directory)
        for name, identifier, change in cases:
            altered = copy.deepcopy(original)
            change(next(e for e in altered if e['id'] == identifier))
            for before, after in zip(original, altered):
                for key in ['id', 'raw', 'fields', 'source_bytes', 'domain', 'flow', 'source', 'destination']:
                    assert canonical(before.get(key)) == canonical(after.get(key)), 'negative controls retain original physical inputs'
            export = root/'events.jsonl'
            export.write_text(''.join(json.dumps(e, separators=(',', ':'))+'\n' for e in altered), encoding='utf8')
            wire = json.loads(wire_certificate.read_text(encoding='utf8'))
            wire['events_sha256'] = hashlib.sha256(export.read_bytes()).hexdigest()
            wire['negative_control_metadata_only'] = True
            certificate = root/'isolated-negative-input.json'
            certificate.write_text(json.dumps(wire), encoding='utf8')
            try:
                audit(export, certificate, oracle)
            except (AssertionError, KeyError):
                refused.append(name)
            else:
                raise AssertionError(('logical corruption escaped verification', name))
    return dict(scope=__doc__, checks=len(refused), refused=refused, all_logical_corruptions_refused=True)


if __name__ == '__main__':
    ap = argparse.ArgumentParser(description=__doc__)
    for key in ['events', 'wire-certificate', 'oracle', 'out']:
        ap.add_argument('--'+key, type=Path, required=True)
    args = ap.parse_args()
    for path in vars(args).values():
        assert not any((p/'.git').exists() for p in [path.resolve(), *path.resolve().parents])
    result = controls(args.events, args.wire_certificate, args.oracle)
    args.out.write_text(json.dumps(result, indent=2)+'\n', encoding='utf8')
    print(json.dumps(result))
