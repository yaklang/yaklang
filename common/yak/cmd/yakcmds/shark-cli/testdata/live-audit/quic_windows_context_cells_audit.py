"""Verify actual selected QUIC recovery metadata without a fake byte range."""
import argparse
import copy
import hashlib
import json
from pathlib import Path
import sys


def sha(path):
    return hashlib.sha256(Path(path).read_bytes()).hexdigest()


def read(path):
    return json.loads(Path(path).read_text('utf-8-sig'))


def verify(snapshot, cells, scope):
    rows = snapshot['Rows']
    selected = []
    required = ('Key Update Recovery: authenticated forward traffic-secret derivation',
                'Key Update Steps: 2', 'QPACK Blocked: true', 'Unobserved Key Epochs: 1')
    for label in required:
        matching = [i for i, row in enumerate(rows) if label in row.split('││', 1)[0]]
        assert len(matching) == 1, label
        if label == 'Key Update Steps: 2':
            y = matching[0]
            x = rows[y].index(label)
            selected.append(snapshot['Attributes'][y][x] >> 4 & 15)
    assert len(selected) == 1 and selected[0] != 0
    assert '0x' not in scope(snapshot), 'derived epoch count cannot name a ciphertext interval'
    _, backgrounds = cells(snapshot)
    assert backgrounds and all(bg != selected[0] for bg in backgrounds), 'derived metadata cannot highlight ciphertext octets'
    return list(required)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    for name in ('manifest','runtime','source-proof','audit-modules','out'):
        parser.add_argument('--'+name, type=Path, required=True)
    args = parser.parse_args()
    args.out.mkdir(exist_ok=False)
    sys.path.insert(0, str(args.audit_modules))
    from pim_windows_cells_audit import state, cells, scope
    source = read(args.source_proof)
    runtime = read(args.runtime/'runtime-summary.json')
    assert source['passed'] and runtime['source'] == source['source']
    entry, = runtime['case_proofs']
    proof = read(entry['path'])
    manifest = read(args.manifest)
    case = manifest[entry['case']]
    assert proof['expected'] == case and proof['exit_code'] == 0
    assert sha(entry['path']) == entry['SHA256']
    assert proof['manifest_SHA256'] == sha(args.manifest)
    assert proof['source'] == source['source'] == '24ca25d193994e8911c6fed7076bd4be1b8a69ac'
    assert '--protocol-history-mib' not in proof['arguments']
    assert proof['restored'] == dict(error='<nil>', input_cp=936, output_cp=936)
    assert sha(proof['arguments'][2]) == proof['binary_SHA256']
    for stem, binding in proof['snapshots'].items():
        assert sha(args.runtime/entry['case']/(stem+'.json')) == binding['cells_SHA256']
        assert sha(args.runtime/entry['case']/(stem+'.png')) == binding['PNG_SHA256']
    snapshot = state(args.runtime/entry['case']/'01-selected-field.json')
    assert case['query'] in snapshot['Rows'][1]
    assert any('›294 ' in row and 'HTTP/3' in row for row in snapshot['Rows'])
    evidence = verify(snapshot, cells, scope)
    negative = []
    for mode in ('step-value', 'QPACK-hidden', 'fake-range', 'ciphertext-highlight'):
        bad = copy.deepcopy(snapshot)
        if mode == 'step-value':
            bad['Rows'] = [row.replace('Key Update Steps: 2', 'Key Update Steps: 0') for row in bad['Rows']]
        elif mode == 'QPACK-hidden':
            bad['Rows'] = [row.replace('QPACK Blocked: true', 'QPACK Blocked: false') for row in bad['Rows']]
        elif mode == 'fake-range':
            bad['Rows'] = [row.replace('b source', '0x0–0x0') if ' FIELDS ' in row else row for row in bad['Rows']]
        else:
            color = next(attrs[row.index('Key Update Steps: 2')] >> 4 & 15 for row, attrs in zip(bad['Rows'],bad['Attributes']) if 'Key Update Steps: 2' in row)
            for y, row in enumerate(bad['Rows']):
                if '││ 000000  ' in row:
                    x = row.index('││ 000000  ')+len('││ 000000  ')
                    for p in (x,x+1):
                        bad['Attributes'][y][p] = (bad['Attributes'][y][p] & ~240) | color<<4
                    break
            else:
                raise AssertionError('no visible ciphertext cell for the negative control')
        try:
            verify(bad, cells, scope)
        except AssertionError:
            negative.append(mode)
        else:
            raise AssertionError('accepted broken QUIC metadata evidence: '+mode)
    target = args.out/'summary.json'
    summary = dict(scope=__doc__, source=runtime['source'], actual_console_buffers=runtime['actual_console_buffers'],
        selected_PDU=299, selected_frame=294, visible_context_evidence=evidence,
        selected_context_has_no_packet_span_or_byte_highlight=True,
        source_proof_SHA256=sha(args.source_proof), runtime_SHA256=sha(args.runtime/'runtime-summary.json'),
        manifest_SHA256=sha(args.manifest), auditor_SHA256=sha(__file__),
        rejected_negative_controls=negative, passed=True)
    target.write_text(json.dumps(summary, indent=2, sort_keys=True)+'\n', encoding='utf8')
    print(json.dumps(dict(summary=str(target), SHA256=sha(target), actual_console_buffers=runtime['actual_console_buffers'])), flush=True)


if __name__ == '__main__':
    main()
