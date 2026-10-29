"""Bind all visible Windows physical, preview and observed-header bytes to wire.

This consumes closed native console executions. It does not drive the desktop.
The two legacy excerpts additionally prove actual address text and option nesting.
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


def read(path):
    return json.loads(path.read_text('utf8'))


def pane(snapshot):
    out = bytearray()
    for row in snapshot['Rows']:
        if '││' not in row:
            continue
        right = row.split('││', 1)[1]
        match = re.match(r'\s*([0-9a-f]{6})  ', right)
        if not match:
            continue
        assert int(match[1], 16) == len(out)
        words = re.findall(r'\b[0-9a-f]{2}\b', right[match.end():match.end()+48])
        assert 0 < len(words) <= 16
        out.extend(bytes.fromhex(' '.join(words)))
    return bytes(out)


def scope(snapshot):
    rows = [row.split('3 ', 1)[1] for row in snapshot['Rows'] if ' FIELDS ' in row and '3 ' in row]
    assert len(rows) == 1
    return rows[0]


def verify_sources(case, wire, initial, first, second):
    assert 'Captured frame' in scope(initial)
    assert pane(initial) == wire
    if 'expected_preview_range' not in case:
        return dict(Source0_bytes=len(wire), additional_logical_sources='not certified in this legacy-only check')
    start, end = case['logical_UDP_header_range']
    begin, finish = case['expected_preview_range']
    assert 0 <= start < end == begin < finish <= len(wire)
    assert end-start == 8
    assert sha_bytes(wire[start:end]) == case['logical_UDP_header_SHA256']
    assert wire[start:end].hex() == case['logical_UDP_header_hex']
    names = case['expected_logical_sources_in_order']
    assert names[0] in scope(first) and names[1] in scope(second)
    assert ' PDU' not in scope(first), 'a bounded unadmitted preview is not a complete application PDU'
    assert pane(first) == wire[begin:finish]
    assert pane(second) == wire[start:end]
    return dict(Source0_bytes=len(wire), Source1_preview_bytes=finish-begin,
                Source1_captured_payload_extent=case['expected_captured_payload_bytes'],
                Source2_observed_header_bytes=end-start,
                all_three_source_extents_and_captions_exact=True)


def sha_bytes(value):
    return hashlib.sha256(value).hexdigest()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    for name in ('manifest', 'runtime', 'audit-modules', 'out'):
        parser.add_argument('--'+name, type=Path, required=True)
    args = parser.parse_args()
    args.out.mkdir(exist_ok=False)
    sys.path.insert(0, str(args.audit_modules))
    from vxlan_endpoint_projection_audit import capture_records
    manifest = read(args.manifest)
    runtime_path = args.runtime/'runtime-summary.json'
    runtime = read(runtime_path)
    checks = []
    for entry in runtime['case_proofs']:
        name = entry['case']
        proof = read(Path(entry['path']))
        assert sha(entry['path']) == entry['SHA256']
        case = manifest[name]
        assert proof['expected'] == case and proof['exit_code'] == 0
        assert proof['source'] == runtime['source']
        assert proof['manifest_SHA256'] == sha(args.manifest)
        assert sha(case['capture']) == case['capture_SHA256'] == proof['capture_SHA256']
        for stem, binding in proof['snapshots'].items():
            assert sha(args.runtime/name/(stem+'.json')) == binding['cells_SHA256']
        record, = capture_records(Path(case['capture']))
        wire = record['wire']
        assert sha_bytes(wire) == case['source_wire_SHA256']
        snapshots = [read(args.runtime/name/(stem+'.json')) for stem in ('01-protocol-fields', '02-first-byte-source', '02-byte-sources')]
        check = dict(case=name, **verify_sources(case, wire, *snapshots))
        texts = '\n'.join(row for stem in ('01-protocol-fields', '01-deep-protocol-fields')
                          if (args.runtime/name/(stem+'.json')).exists()
                          for row in read(args.runtime/name/(stem+'.json'))['Rows'])
        if case.get('expected_physical_metadata_omitted'):
            assert 'IP endpoints: 1.2.3.4 → 4.3.2.1' in texts
            assert 'Physical frame endpoints:' not in texts
            negative = []
            for mode in ('header-range', 'header-SHA', 'preview-range', 'source-order'):
                bad = copy.deepcopy(case)
                if mode == 'header-range':
                    bad['logical_UDP_header_range'][0] += 1
                elif mode == 'header-SHA':
                    bad['logical_UDP_header_SHA256'] = '0'*64
                elif mode == 'preview-range':
                    bad['expected_preview_range'][1] -= 1
                else:
                    bad['expected_logical_sources_in_order'].reverse()
                try:
                    verify_sources(bad, wire, *snapshots)
                except AssertionError:
                    negative.append(mode)
                else:
                    raise AssertionError('accepted broken source evidence: '+mode)
            check['rejected_negative_controls'] = negative
        else:
            assert 'Source: 127.0.0.1' in texts and 'Destination: 127.0.0.1' in texts
            check['literal_IP_addresses_are_readable_in_actual_console'] = True
            if case['protocol'] == 'TCP':
                deep = read(args.runtime/name/'01-deep-protocol-fields.json')
                positions = {}
                for row in deep['Rows']:
                    left = row.split('││', 1)[0]
                    for label in ('── TCP', '── Options', '── Options.Option[0]', 'Options.Option.Kind:'):
                        if label in left and label not in positions:
                            positions[label] = left.index(label)
                assert positions['── TCP'] < positions['── Options'] < positions['── Options.Option[0]'] < positions['Options.Option.Kind:']
                check['actual_TCP_options_are_progressively_indented'] = True
        checks.append(check)
    summary = dict(scope=__doc__, source=runtime['source'], checks=checks,
                   runtime_SHA256=sha(runtime_path), manifest_SHA256=sha(args.manifest),
                   auditor_SHA256=sha(__file__), differences=0)
    target = args.out/'summary.json'
    target.write_text(json.dumps(summary, indent=2, sort_keys=True)+'\n', encoding='utf8')
    print(json.dumps(dict(summary=str(target), SHA256=sha(target), checks=checks)), flush=True)


if __name__ == '__main__':
    main()
