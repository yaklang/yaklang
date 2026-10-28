"""Verify native console field hierarchy, source cycling and exact clicked ranges.

Inputs are closed executions of the existing Windows CLI console test harness.
No GUI, terminal or parser is driven by this read-only verifier.
"""
import argparse
import copy
import hashlib
import json
from pathlib import Path
import re
import sys


def sha(path):
    return hashlib.sha256(Path(path).read_bytes()).hexdigest()


def state(path):
    result = json.loads(path.read_text('utf8'))
    assert len(result['Rows']) == len(result['Attributes']) == result['Height']
    assert all(len(row) == len(attrs) == result['Width'] for row, attrs in zip(result['Rows'], result['Attributes']))
    return result


def native_scope(snapshot):
    rows = [row for row in snapshot['Rows'] if ' FIELDS ' in row and '3 ' in row]
    assert len(rows) == 1
    return rows[0].split('3 ', 1)[1]


def verify_clicked(snapshot, case, wire):
    start, end = case['clicked_field_span']
    assert 0 <= start < end <= len(wire)
    scope = native_scope(snapshot)
    assert case['protocol'] + ' · captured PDU' in scope
    assert f'0x{start:x}–0x{end-1:x} · {end-start} bytes' in scope
    field_rows = [(row, attrs) for row, attrs in zip(snapshot['Rows'], snapshot['Attributes']) if case['clicked_field_text'] in row]
    assert len(field_rows) == 1
    row, attrs = field_rows[0]
    selected = attrs[row.index(case['clicked_field_text'])] >> 4 & 15
    assert selected != 0, 'clicked field must have a visible selected background'
    observed = bytearray()
    highlighted = []
    for row, attrs in zip(snapshot['Rows'], snapshot['Attributes']):
        marker = row.find('││')
        if marker < 0:
            continue
        right = marker + 2
        match = re.match(r'\s*([0-9a-f]{6})  ', row[right:])
        if not match:
            continue
        offset = int(match[1], 16)
        assert offset == len(observed)
        first = right + match.end()
        for column in range(min(16, len(wire)-offset)):
            x = first + 3*column + (1 if column >= 8 else 0)
            token = row[x:x+2]
            assert re.fullmatch('[0-9a-f]{2}', token)
            observed.append(int(token, 16))
            backgrounds = {attrs[x] >> 4 & 15, attrs[x+1] >> 4 & 15}
            assert len(backgrounds) == 1
            if selected in backgrounds:
                highlighted.append(offset+column)
    assert bytes(observed) == wire
    assert highlighted == list(range(start, end)), (highlighted, start, end)
    return dict(start=start, end=end, bytes_hex=wire[start:end].hex(), exactly_selected_hex_octets=len(highlighted))


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    for name in ('manifest', 'runtime', 'audit-modules', 'out'):
        parser.add_argument('--'+name, type=Path, required=True)
    args = parser.parse_args()
    args.out.mkdir(exist_ok=False)
    sys.path.insert(0, str(args.audit_modules))
    from vxlan_endpoint_projection_audit import capture_records
    manifest = json.loads(args.manifest.read_text('utf8'))
    runtime_path = args.runtime/'runtime-summary.json'
    runtime = json.loads(runtime_path.read_text('utf8'))
    checks = []
    for proof_ref in runtime['case_proofs']:
        name = proof_ref['case']
        proof_path = Path(proof_ref['path'])
        assert sha(proof_path) == proof_ref['SHA256']
        proof = json.loads(proof_path.read_text('utf8'))
        case = manifest[name]
        assert proof['exit_code'] == 0 and proof['expected'] == case
        assert proof['source'] == runtime['source']
        assert proof['manifest_SHA256'] == sha(args.manifest)
        assert sha(case['capture']) == case['capture_SHA256'] == proof['capture_SHA256']
        for stem, binding in proof['snapshots'].items():
            assert sha(args.runtime/name/(stem+'.json')) == binding['cells_SHA256']
        initial = state(args.runtime/name/'01-protocol-fields.json')
        assert 'Captured frame' in native_scope(initial)
        scopes = [native_scope(state(args.runtime/name/(stem+'.json'))) for stem in ('02-first-byte-source', '02-byte-sources')]
        assert sum('Captured frame' in v for v in scopes) == 1
        assert sum(case['protocol']+' · captured PDU' in v for v in scopes) == 1
        check = dict(case=name, source_cycle_has_exact_captured_and_native_names=True)
        if case.get('select_field'):
            records = list(capture_records(Path(case['capture'])))
            assert len(records) == 1
            wire = records[0]['wire']
            selected = state(args.runtime/name/'01-selected-field.json')
            check['clicked_range'] = verify_clicked(selected, case, wire)
            negatives = []
            for label in ('start', 'end', 'field-text', 'wire'):
                bad = copy.deepcopy(case)
                bad_wire = wire
                if label == 'start':
                    bad['clicked_field_span'][0] += 1
                elif label == 'end':
                    bad['clicked_field_span'][1] -= 1
                elif label == 'field-text':
                    bad['clicked_field_text'] += ' nonexistent'
                else:
                    changed = bytearray(wire)
                    changed[case['clicked_field_span'][0]] ^= 1
                    bad_wire = bytes(changed)
                try:
                    verify_clicked(selected, bad, bad_wire)
                except AssertionError:
                    negatives.append(label)
                else:
                    raise AssertionError('accepted broken console evidence: '+label)
            assert len(negatives) == 4
            check['rejected_negative_controls'] = negatives
        checks.append(check)
    summary = dict(scope=__doc__, source=runtime['source'], checks=checks,
                   runtime_SHA256=sha(runtime_path), manifest_SHA256=sha(args.manifest),
                   auditor_SHA256=sha(__file__), differences=0)
    target = args.out/'summary.json'
    target.write_text(json.dumps(summary, indent=2, sort_keys=True)+'\n', encoding='utf8')
    print(json.dumps(dict(summary=str(target), SHA256=sha(target), checks=checks)), flush=True)


if __name__ == '__main__':
    main()
