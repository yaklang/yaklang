"""Verify a closed 1MiB-history Windows identity query without restored details.

This selected profile binds physical frame294/PDU299 and every visible captured
octet. It does not claim recovery of evicted protocol fields or plaintext.
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
    with Path(path).open('rb') as f:
        for chunk in iter(lambda: f.read(1 << 20), b''):
            h.update(chunk)
    return h.hexdigest()


def read(path):
    return json.loads(Path(path).read_text('utf-8-sig'))


def verify(snapshot, expected, cells, scope):
    text = '\n'.join(snapshot['Rows'])
    assert ' EOF ' in snapshot['Rows'][0]
    assert 'fields pdu.id == 299' in snapshot['Rows'][1] and '▏' not in snapshot['Rows'][1]
    assert re.search(r'buffer\s+1865\s*/', text)
    assert re.search(r'· 1 visible(?:\s|╮)', text)
    assert len([r for r in snapshot['Rows'] if re.search(r'(?:^|\s)›294\s+\S+\s+HTTP/3(?:\s|$)', r)]) == 1
    assert '2 HTTP/3 FIELDS · detail unavailable' in text
    assert 'PDU #299 · detail unavailable' in text
    assert 'Contributor coverage · detail unavailable' in text
    assert 'PDU #0' not in text and not re.search(r'\bMESSAGE\s*·\s*PDU', text)
    assert text.count('Protocol detail absent or evicted from bounded message history') == 1
    assert 'Field filter partial: incomplete field evidence' in text
    assert 'protocol detail unavailable in retained history' in text
    assert 'Captured frame' in scope(snapshot) and 'authenticated plaintext' not in scope(snapshot)
    actual, _ = cells(snapshot)
    assert actual and len(actual) <= len(expected) and actual == expected[:len(actual)]
    return len(actual)


def main():
    ap = argparse.ArgumentParser(description=__doc__)
    for name in ('runtime', 'audit-modules', 'out'):
        ap.add_argument('--'+name, type=Path, required=True)
    ap.add_argument('--source', required=True)
    a = ap.parse_args()
    a.out.mkdir(exist_ok=False)
    sys.path.insert(0, str(a.audit_modules))
    from pim_windows_cells_audit import state, cells, scope
    from vxlan_endpoint_projection_audit import capture_records
    p = read(a.runtime/'proof.json')
    assert p['source'] == a.source and p['passed'] and p['exit_code'] == 0
    assert p['input_bytes_unchanged'] and p['input_changes'] == []
    assert p['source_snapshot_SHA256'] == sha(a.runtime/'inputs-before.json')
    assert p['source_after_SHA256'] == sha(a.runtime/'inputs-after.json')
    before, after = read(a.runtime/'inputs-before.json'), read(a.runtime/'inputs-after.json')
    assert before['inputs'] == after['inputs'] and after['changes'] == []
    for name, entry in before['inputs'].items():
        assert sha(entry['path']) == entry['SHA256']
        assert Path(entry['path']).stat().st_size == entry['bytes']
    for name, entry in before['source_snapshots'].items():
        assert Path(entry['path']).resolve().is_relative_to((a.runtime/'frozen-inputs').resolve())
        assert sha(entry['path']) == entry['SHA256'] == before['inputs'][name]['SHA256']
    plan = read(before['source_snapshots']['plan']['path'])
    assert sha(before['source_snapshots']['plan']['path']) == p['plan_SHA256']
    assert plan['capture_SHA256'] == p['capture_SHA256'] == sha(p['arguments'][3])
    assert plan['keylog_SHA256'] == p['keylog_SHA256'] == sha(p['arguments'][4])
    assert p['arguments'][7:] == ['--ignore-embedded-secrets', '--protocol-history-mib', '1']
    assert p['restored'] == dict(error='<nil>', input_cp=936, output_cp=936)
    build = read(before['source_snapshots']['app_build_receipt']['path'])
    helper = read(before['source_snapshots']['helper_build_receipt']['path'])
    assert build['exit_code'] == helper['exit_code'] == 0 and build['source'] == a.source
    assert build['binary_SHA256'] == p['binary_SHA256'] == sha(p['arguments'][2])
    assert helper['source'] == p['helper_source'] and helper['binary_SHA256'] == p['helper_SHA256'] == sha(p['arguments'][0])
    assert helper['source_file_SHA256'] == before['inputs']['native_console_helper_source']['SHA256']
    for stem, item in p['snapshots'].items():
        assert sha(a.runtime/'console'/(stem+'.json')) == item['cells_SHA256']
        assert sha(a.runtime/'console'/(stem+'.png')) == item['PNG_SHA256']
        s = state(a.runtime/'console'/(stem+'.json'))
        assert s['Width'] == item['width'] and s['Height'] == item['height']
    wire = next(r['wire'] for n, r in enumerate(capture_records(Path(plan['capture'])), 1) if n == 294)
    assert len(wire) == 1232 and wire[:4] == b'\x02\x00\x00\x00'
    assert wire[4] == 0x45 and wire[13] == 17
    assert int.from_bytes(wire[6:8], 'big') == 1228
    assert int.from_bytes(wire[28:30], 'big') == 1208
    first = state(a.runtime/'console/01-protocol-fields.json')
    visible = verify(first, wire, cells, scope)
    sources = []
    for name in ('02-first-byte-source', '02-byte-sources'):
        s = state(a.runtime/'console'/(name+'.json'))
        got, _ = cells(s)
        assert 'Captured frame' in scope(s) and 'authenticated plaintext' not in scope(s)
        assert got and got == wire[:len(got)]
        sources.append(dict(snapshot=name, visible_physical_prefix_bytes=len(got)))
    negatives = []
    for kind in ('caption', 'PDU0', 'duplicate-reason', 'physical-octet', 'plaintext-caption'):
        bad = copy.deepcopy(first)
        if kind == 'caption':
            bad['Rows'] = [r.replace('2 HTTP/3 FIELDS', '2 UDP FIELDS') for r in bad['Rows']]
        elif kind == 'PDU0':
            bad['Rows'] = [r.replace('Contributor coverage', 'PDU #0') for r in bad['Rows']]
        elif kind == 'duplicate-reason':
            # Preserve the contributor heading, title, notice and hex source;
            # this mutation must fail specifically because the reason repeats.
            y = next(i for i, r in enumerate(bad['Rows']) if 'capture all' in r and 'display all' in r)
            bad['Rows'][y] = 'Protocol detail absent or evicted from bounded message history'.ljust(bad['Width'])
        elif kind == 'physical-octet':
            y = next(i for i, r in enumerate(bad['Rows']) if '000000  02 ' in r)
            bad['Rows'][y] = bad['Rows'][y].replace('000000  02 ', '000000  03 ', 1)
        else:
            bad['Rows'] = [r.replace('Captured frame', 'authenticated plaintext') for r in bad['Rows']]
        try:
            verify(bad, wire, cells, scope)
        except AssertionError:
            negatives.append(kind)
        else:
            raise AssertionError('accepted wrong history-availability evidence: '+kind)
    result = dict(scope=__doc__, source=a.source, proof_SHA256=sha(a.runtime/'proof.json'),
        input_snapshot_SHA256=p['source_snapshot_SHA256'], actual_console_buffers=len(p['snapshots']),
        physical_frame=294, ID_only_query=299, detail_budget_bytes=1<<20,
        caption='HTTP/3', detail_unavailable=True, fake_PDU0=False, duplicate_reason=False,
        whole_physical_frame_bytes=len(wire), whole_physical_frame_SHA256=hashlib.sha256(wire).hexdigest(),
        visible_physical_prefix_bytes=visible, full_source_visible=False, source_checks=sources,
        no_evicted_plaintext_source=True, rejected_negative_controls=negatives,
        auditor_SHA256=sha(__file__), passed=True)
    target = a.out/'summary.json'
    target.write_text(json.dumps(result, indent=2, sort_keys=True)+'\n', encoding='utf8')
    print(json.dumps(dict(summary=str(target), SHA256=sha(target), passed=True)))


if __name__ == '__main__':
    main()
