"""Verify original PIM words, repeated occurrences and Windows console cells.

Consumes closed executions of the CLI's own native console harness. Source0
is the entire captured frame; Source1 is the independently located PIM body.
Opaque suffixes are extracted values with explicitly unavailable frame offsets.
Console-cell PNGs are not Windows Terminal pixel screenshots.
"""
import argparse
import copy
import hashlib
import json
from pathlib import Path
import re
import sys


def sha(path):
    h = hashlib.sha256()
    with Path(path).open('rb') as stream:
        for data in iter(lambda: stream.read(1 << 20), b''):
            h.update(data)
    return h.hexdigest()


def read(path):
    return json.loads(Path(path).read_text('utf-8-sig'))


def scope(snapshot):
    rows = [row.split('3 ', 1)[1] for row in snapshot['Rows'] if ' FIELDS ' in row and '3 ' in row]
    assert len(rows) == 1
    return rows[0]


def state(path):
    value = read(path)
    assert len(value['Rows']) == len(value['Attributes']) == value['Height']
    assert all(len(row) == len(attr) == value['Width'] for row, attr in zip(value['Rows'], value['Attributes']))
    return value


def cells(snapshot):
    out = bytearray()
    positions = []
    for row, attrs in zip(snapshot['Rows'], snapshot['Attributes']):
        marker = row.find('││')
        if marker < 0:
            continue
        first = marker + 2
        match = re.match(r'\s*([0-9a-f]{6})  ', row[first:])
        if not match:
            continue
        offset = int(match[1], 16)
        assert offset == len(out)
        first += match.end()
        for column in range(16):
            x = first + 3*column + (1 if column >= 8 else 0)
            word = row[x:x+2]
            if word == '  ':
                break
            assert re.fullmatch('[0-9a-f]{2}', word)
            assert attrs[x] >> 4 & 15 == attrs[x+1] >> 4 & 15
            out.append(int(word, 16))
            positions.append(attrs[x] >> 4 & 15)
    return bytes(out), positions


def verify_clicked(snapshot, case, body):
    start, end = case['clicked_field_span']
    assert 0 <= start < end <= len(body)
    caption = scope(snapshot)
    assert case['protocol']+' · captured PDU' in caption
    assert f'0x{start:x}–0x{end-1:x} · {end-start} bytes' in caption
    matches = []
    for y, (row, attrs) in enumerate(zip(snapshot['Rows'], snapshot['Attributes'])):
        left = row.split('││', 1)[0]
        if case['clicked_field_text'] in left:
            x = row.index(case['clicked_field_text'])
            matches.append((y, x, attrs[x] >> 4 & 15))
    assert matches
    selected = [v for v in matches if v[2] != 0]
    assert len(selected) == 1, 'exactly one matching field occurrence must be selected'
    ordinal = matches.index(selected[0])
    assert ordinal == case.get('select_field_occurrence', 0)
    actual, backgrounds = cells(snapshot)
    assert actual == body, 'visible native source must equal the whole source-derived PIM body'
    highlighted = [i for i, bg in enumerate(backgrounds) if bg == selected[0][2]]
    assert highlighted == list(range(start, end))
    return dict(start=start, end=end, bytes_hex=body[start:end].hex(),
                visible_equal_field_occurrences=len(matches), selected_occurrence=ordinal,
                exactly_selected_hex_octets=len(highlighted))


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    for name in ('manifest', 'runtime', 'audit-modules', 'out'):
        parser.add_argument('--'+name, type=Path, required=True)
    args = parser.parse_args()
    args.out.mkdir(exist_ok=False)
    sys.path.insert(0, str(args.audit_modules))
    sys.path.insert(0, str(args.audit_modules/'pim-source'))
    from vxlan_endpoint_projection_audit import capture_records
    from pim_carrier_source import locate
    manifest = read(args.manifest)
    runtime_path = args.runtime/'runtime-summary.json'
    runtime = read(runtime_path)
    checks = []
    for entry in runtime['case_proofs']:
        name = entry['case']
        proof = read(entry['path'])
        case = manifest[name]
        assert sha(entry['path']) == entry['SHA256']
        assert proof['expected'] == case and proof['exit_code'] == 0
        assert proof['source'] == runtime['source']
        assert proof['manifest_SHA256'] == sha(args.manifest)
        assert proof['capture_SHA256'] == case['capture_SHA256'] == sha(case['capture'])
        assert proof['restored'] == dict(error='<nil>', input_cp=936, output_cp=936)
        binary = Path(proof['arguments'][2])
        build = read(binary.parent/'yak-receipt.json')
        assert build['source'] == proof['source'] and build['exit_code'] == 0
        assert proof['binary_SHA256'] == build['binary_SHA256'] == sha(binary)
        assert proof['build_receipt_SHA256'] == sha(binary.parent/'yak-receipt.json')
        assert proof['helper_SHA256'] == sha(proof['arguments'][0])
        for stem, binding in proof['snapshots'].items():
            assert sha(args.runtime/name/(stem+'.json')) == binding['cells_SHA256']
            assert sha(args.runtime/name/(stem+'.png')) == binding['PNG_SHA256']
        record, = capture_records(Path(case['capture']))
        wire = record['wire']
        assert hashlib.sha256(wire).hexdigest() == case['source_wire_SHA256']
        original_bound = False
        if case.get('original_capture'):
            assert sha(case['original_capture']) == case['original_capture_SHA256']
            original = next(r for number, r in enumerate(capture_records(Path(case['original_capture'])), 1) if number == case['original_frame'])
            for key in ('wire', 'link', 'time_ns', 'original_length', 'captured_length'):
                assert original[key] == record[key]
            assert original['section'] == case['original_section']
            assert original['interface'] == case['original_interface']
            assert record['section'] == record['interface'] == 0
            original_bound = True
        location = locate(record)
        assert location['protocol'] == 'pim'
        body = wire[location['offset']:location['end']]
        snapshots = {stem: state(args.runtime/name/(stem+'.json')) for stem in
                     ('01-protocol-fields', '01-deep-protocol-fields', '01-selected-field', '02-first-byte-source', '02-byte-sources')}
        assert 'Captured frame' in scope(snapshots['01-protocol-fields'])
        assert cells(snapshots['01-protocol-fields'])[0] == wire
        assert case['protocol']+' · captured PDU' in scope(snapshots['01-deep-protocol-fields'])
        assert cells(snapshots['01-deep-protocol-fields'])[0] == body
        click = verify_clicked(snapshots['01-selected-field'], case, body)
        cycle = [scope(snapshots[stem]) for stem in ('02-first-byte-source', '02-byte-sources')]
        if any('observed IPv4 totalLength0' in v for v in location['profiles']):
            assert 'Extracted field value · frame offsets unavailable · PIM Unparsed Body' in cycle[0]
            assert cells(snapshots['02-first-byte-source'])[0] == body[10:]
            assert 'Captured frame' in cycle[1]
            assert cells(snapshots['02-byte-sources'])[0] == wire
            texts = '\n'.join(snapshots['01-protocol-fields']['Rows'])
            assert 'Message extent: unavailable (original Total Length: 0)' in texts
            assert 'BSR=unavailable' in texts
            assert 'YAML dissector incomplete' not in texts and 'Ethernet {Contents=' not in texts
        else:
            assert 'Captured frame' in cycle[0]
            assert cells(snapshots['02-first-byte-source'])[0] == wire
            assert case['protocol']+' · captured PDU' in cycle[1]
            assert cells(snapshots['02-byte-sources'])[0] == body
        negatives = []
        for mode in ('start', 'end', 'field-text', 'occurrence', 'wire', 'source-caption'):
            bad = copy.deepcopy(case)
            snapshot = copy.deepcopy(snapshots['01-selected-field'])
            bad_body = body
            if mode == 'start':
                bad['clicked_field_span'][0] += 1
            elif mode == 'end':
                bad['clicked_field_span'][1] -= 1
            elif mode == 'field-text':
                bad['clicked_field_text'] += ' absent'
            elif mode == 'occurrence':
                bad['select_field_occurrence'] = 0 if case.get('select_field_occurrence') else 1
            elif mode == 'wire':
                changed = bytearray(body)
                changed[case['clicked_field_span'][0]] ^= 1
                bad_body = bytes(changed)
            else:
                snapshot['Rows'] = [row.replace('captured PDU', 'unavailable source') for row in snapshot['Rows']]
            try:
                verify_clicked(snapshot, bad, bad_body)
            except AssertionError:
                negatives.append(mode)
            else:
                raise AssertionError('accepted broken PIM console evidence: '+mode)
        checks.append(dict(case=name, original_public_frame_bound=original_bound,
                           authored='original_capture' not in case, captured_frame_bytes=len(wire),
                           native_body_bytes=len(body), physical_PIM_offset=location['offset'],
                           clicked_range=click, source_cycle_captions=cycle,
                           rejected_negative_controls=negatives))
    assert len(checks) == runtime['actual_cases'] == 3
    target = args.out/'summary.json'
    summary = dict(scope=__doc__, source=runtime['source'], checks=checks,
                   actual_console_buffers=runtime['actual_console_buffers'],
                   runtime_SHA256=sha(runtime_path), manifest_SHA256=sha(args.manifest),
                   auditor_SHA256=sha(__file__), literal_carrier_SHA256=sha(Path(locate.__code__.co_filename)),
                   differences=0, passed=True)
    target.write_text(json.dumps(summary, indent=2, sort_keys=True)+'\n', encoding='utf8')
    print(json.dumps(dict(summary=str(target), SHA256=sha(target), cases=len(checks), differences=0)), flush=True)


if __name__ == '__main__':
    main()
