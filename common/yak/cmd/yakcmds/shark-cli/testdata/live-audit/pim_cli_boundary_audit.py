"""Source-frozen whole CLI proof for minimal PIM values and parent extent.

Prepare consumes literal-source native expectations, never a new CLI export.
Verify checks all37 complete executions without any ID/path normalization.
"""
import argparse
import copy
import datetime
import hashlib
import json
from pathlib import Path
import sys


def sha(path):
    h = hashlib.sha256()
    with Path(path).open('rb') as f:
        for b in iter(lambda: f.read(1 << 20), b''):
            h.update(b)
    return h.hexdigest()


def read(path):
    return json.loads(Path(path).read_text('utf-8-sig'))


def canonical(value):
    return json.dumps(value, sort_keys=True, separators=(',', ':'), ensure_ascii=False)


def same(left, right):
    return canonical(left) == canonical(right)


def fields(snapshot):
    out = {}
    for item in snapshot['values']:
        out.setdefault(item['name'], []).append(item['value'])
    return out


def diagnostic(protocol, error):
    expert, message = error.split(': ', 1)
    return protocol.upper()+' · '+expert+' · '+message+' · '


def export_paths(root):
    summary = read(root/'summary.json')
    assert summary['cases'] == 37 and summary['actual_new_binary_execution'] is True
    paths = sorted(root.glob('*/*/yak-producer.json'))
    assert len(paths) == len(summary['outputs']) == 37
    out = {}
    for path in paths:
        item = read(path)
        assert item in summary['outputs'] and item['source'] == summary['source']
        binary = Path(item['arguments'][0])
        build = read(binary.parent/'yak-receipt.json')
        assert build['exit_code'] == item['exit_code'] == 0
        assert build['source'] == item['source']
        assert sha(binary) == build['binary_SHA256'] == item['binary_SHA256']
        assert sha(item['capture']) == item['capture_SHA256']
        output = path.parent/'yak.jsonl'
        assert sha(output) == item['export_SHA256']
        out[str(path.parent.relative_to(root))] = dict(receipt=item, receipt_SHA256=sha(path), output=output)
    return summary, out


def prepare(args):
    args.out.mkdir(exist_ok=False)
    original, exports = export_paths(args.before)
    minimal = [json.loads(line) for line in args.minimal.open(encoding='utf8')]
    assert len(minimal) == 6
    selected = {v['frame']: dict(old=v['whole_old_native'], expected=v['minimal_no_fabricated_fields_whole_expected'], source=v['source']) for v in minimal}
    extent = [json.loads(line) for line in args.extent.open(encoding='utf8')]
    item, = [v for v in extent if v['frame'] == 308490]
    assert item['source']['extent'] == 'incomplete'
    selected[308490] = dict(old=item['baseline_minimal_native22_expected'], expected=item['whole_next_extent_scope_expected'], source=item['source'])
    plan = []
    for case, entry in exports.items():
        if case != r'preservation\sharon':
            continue
        with entry['output'].open(encoding='utf-8-sig') as f:
            for line in f:
                row = json.loads(line)
                frame = row.get('number')
                if frame not in selected:
                    continue
                spec = selected[frame]
                old, new = spec['old'], spec['expected']
                assert len(row['pdus']) == 1
                current = row['pdus'][0]
                assert current['id'] == old['id'] == new['id']
                assert same(current['field_values'], old['field_values'])
                assert same(current['fields'], fields(old['field_values']))
                assert same(row['capture_domain'], old['domain'])
                assert current['status'] == old['status']
                assert row['info'] == diagnostic(old['protocol'], old['error'])
                expected = copy.deepcopy(row)
                pdu = expected['pdus'][0]
                pdu['field_values'] = copy.deepcopy(new['field_values'])
                pdu['fields'] = fields(new['field_values'])
                for key in ('completeness', 'error', 'expert', 'status', 'summary'):
                    pdu[key] = new[key]
                expected['protocol_status'] = new['status']
                expected['info'] = diagnostic(new['protocol'], new['error'])
                assert not same(row, expected)
                plan.append(dict(case=case, frame=frame, id=old['id'], source=spec['source'], whole_before=row, whole_expected=expected))
    assert len(plan) == 7 and {v['frame'] for v in plan} == set(selected)
    target = args.out/'seven-source-whole-CLI-plan.jsonl'
    with target.open('x', encoding='utf8') as f:
        for item in plan:
            f.write(json.dumps(item, sort_keys=True)+'\n')
    controls = []
    for item in plan:
        for mode in ('status-type', 'source-ref', 'domain', 'value-type', 'extra-field', 'info', 'PDU-id'):
            bad = copy.deepcopy(item['whole_expected'])
            if mode == 'status-type':
                bad['protocol_status'] = 0
            elif mode == 'source-ref':
                bad['pdus'][0]['field_values']['source']['PacketRefs'][0]['Number'] += 1
            elif mode == 'domain':
                bad['capture_domain']['Interface'] += 1
            elif mode == 'value-type':
                number = next(v for v in bad['pdus'][0]['field_values']['values'] if type(v['value']) is int)
                number['value'] = bool(number['value'])
            elif mode == 'extra-field':
                bad['pdus'][0]['fields']['pim.nonexistent'] = [0]
            elif mode == 'info':
                bad['info'] += 'unproved'
            else:
                bad['pdus'][0]['id'] += 1
            assert not same(bad, item['whole_expected'])
            controls.append(dict(frame=item['frame'], rejected=mode))
    summary = dict(scope=__doc__, UTC=datetime.datetime.now(datetime.timezone.utc).isoformat(),
                   baseline_source=original['source'], before_runtime_SHA256=sha(args.before/'summary.json'),
                   native_minimal_plan_SHA256=sha(args.minimal), native_extent_plan_SHA256=sha(args.extent),
                   frozen_before_any_new_CLI_execution=True, whole_changed_rows=7,
                   positives=7, negatives=len(controls), negative_controls=controls,
                   plan_SHA256=sha(target), auditor_SHA256=sha(__file__))
    (args.out/'summary.json').write_text(json.dumps(summary, indent=2)+'\n', encoding='utf8')
    print(json.dumps(dict(summary=str(args.out/'summary.json'), SHA256=sha(args.out/'summary.json'), planned_rows=7, negative_controls=len(controls))), flush=True)


def verify(args):
    args.out.mkdir(exist_ok=False)
    frozen = read(args.plan/'summary.json')
    plan_path = args.plan/'seven-source-whole-CLI-plan.jsonl'
    assert sha(plan_path) == frozen['plan_SHA256']
    plan = [json.loads(line) for line in plan_path.open(encoding='utf8')]
    selected = {(r['case'], r['frame']): r for r in plan}
    before, old = export_paths(args.before)
    after, new = export_paths(args.after)
    assert sha(args.before/'summary.json') == frozen['before_runtime_SHA256']
    assert old.keys() == new.keys()
    checks = []
    total, source_rows, literal_rows = 0, 0, 0
    used = set()
    for case in old:
        a, b = old[case], new[case]
        assert a['receipt']['arguments'][1:] == b['receipt']['arguments'][1:]
        for key in ('capture_SHA256', 'capture', 'keylog_used'):
            assert a['receipt'][key] == b['receipt'][key]
        count, expected_count, literal_count = 0, 0, 0
        with a['output'].open(encoding='utf-8-sig') as left, b['output'].open(encoding='utf-8-sig') as right:
            for line in left:
                other = right.readline()
                assert other, (case, count, 'new export ended early')
                old_row, actual = json.loads(line), json.loads(other)
                key = case, old_row.get('number')
                if key in selected:
                    assert key not in used
                    used.add(key)
                    spec = selected[key]
                    assert same(old_row, spec['whole_before'])
                    assert same(actual, spec['whole_expected']), (case, old_row['number'], 'entire expected row differs')
                    expected_count += 1
                else:
                    assert line == other, (case, old_row.get('number'), 'unplanned full-row difference')
                    literal_count += 1
                count += 1
            assert right.readline() == '', (case, 'new export has extra rows')
        checks.append(dict(case=case, rows=count, source_expected_rows=expected_count, literal_rows=literal_count,
                           before_producer_SHA256=a['receipt_SHA256'], after_producer_SHA256=b['receipt_SHA256'],
                           actual_SHA256=sha(b['output'])))
        total += count
        source_rows += expected_count
        literal_rows += literal_count
    assert len(used) == source_rows == 7 and total == 384518
    summary = dict(scope=__doc__, before_source=before['source'], after_source=after['source'],
                   inputs=37, whole_rows=total, source_expected_rows=source_rows,
                   byte_literal_rows=literal_rows, unexplained_differences=0, ID_mapping=False,
                   frozen_plan_summary_SHA256=sha(args.plan/'summary.json'),
                   runtime_SHA256=sha(args.after/'summary.json'), auditor_SHA256=sha(__file__), cases=checks)
    target = args.out/'summary.json'
    target.write_text(json.dumps(summary, indent=2)+'\n', encoding='utf8')
    print(json.dumps(dict(summary=str(target), SHA256=sha(target), whole_rows=total, exact_source_expected_rows=source_rows, unexplained_differences=0)), flush=True)


parser = argparse.ArgumentParser(description=__doc__)
sub = parser.add_subparsers(dest='mode', required=True)
pre = sub.add_parser('prepare')
for name in ('before', 'minimal', 'extent', 'out'):
    pre.add_argument('--'+name, type=Path, required=True)
ver = sub.add_parser('verify')
for name in ('before', 'after', 'plan', 'out'):
    ver.add_argument('--'+name, type=Path, required=True)
args = parser.parse_args()
(prepare if args.mode == 'prepare' else verify)(args)
