"""Whole CLI projection from source-frozen PIM prefix/native expectations.

The native prefix source plan has an explicit post-discovery API-order
correction, proved against every old full snapshot. The CLI expectation is
frozen before any CLI24 execution. No arrays or identities are normalized.
"""
import argparse
import copy
import datetime
import json
from pathlib import Path

base = Path(__file__).with_name('pim_cli_boundary_audit.py')
helpers = base.read_text('utf8').split('\nparser = argparse.ArgumentParser', 1)[0]
helpers = helpers.replace('seven-source-whole-CLI-plan.jsonl', 'source-whole-CLI-plan.jsonl')
assert helpers.count('len(used) == source_rows == 7') == 1
helpers = helpers.replace('len(used) == source_rows == 7', 'len(used) == source_rows == len(selected)')
helpers = helpers.replace('auditor_SHA256=sha(__file__), cases=checks', 'auditor_SHA256=sha(__file__), helper_SHA256=sha(base), cases=checks')
scope = __doc__
exec(compile(helpers, str(base), 'exec'), globals())
__doc__ = scope
verify_complete = verify


def prepare_prefix(args):
    args.out.mkdir(exist_ok=False)
    summary, exports = export_paths(args.before)
    sources = {}
    for corpus in ('weber', 'sharon'):
        path = args.native_plan/(corpus+'-all-prefix-whole-plan.jsonl')
        sources[corpus] = [json.loads(line) for line in path.open(encoding='utf8')]
    assert sum(map(len, sources.values())) == 464
    selected = {(str(Path('preservation')/corpus), item['frame']): item
                for corpus, rows in sources.items() for item in rows
                if not same(item['baseline_extent_scope_expected'], item['whole_prefix_expected'])}
    assert len(selected) == 142
    plan = []
    for case, entry in exports.items():
        if not any(key[0] == case for key in selected):
            continue
        with entry['output'].open(encoding='utf-8-sig') as f:
            for line in f:
                row = json.loads(line)
                key = case, row.get('number')
                if key not in selected:
                    continue
                spec = selected[key]
                old, new = spec['baseline_extent_scope_expected'], spec['whole_prefix_expected']
                prior = [pdu for pdu in row['pdus'] if pdu['id'] == old['id']]
                assert len(prior) == 1 and old['id'] == new['id']
                current, = prior
                assert same(current['field_values'], old['field_values'])
                assert same(current['fields'], fields(old['field_values']))
                expected = copy.deepcopy(row)
                pdu, = [pdu for pdu in expected['pdus'] if pdu['id'] == old['id']]
                pdu['field_values'] = copy.deepcopy(new['field_values'])
                pdu['fields'] = fields(new['field_values'])
                for name in ('completeness', 'error', 'expert', 'status', 'summary'):
                    pdu[name] = new[name]
                assert old['status'] == new['status'] and old['error'] == new['error']
                assert not same(row, expected)
                plan.append(dict(case=case, frame=key[1], id=old['id'], source=spec['source'], whole_before=row, whole_expected=expected))
    assert len(plan) == len(selected) == 142
    target = args.out/'source-whole-CLI-plan.jsonl'
    with target.open('x', encoding='utf8') as f:
        for item in plan:
            f.write(json.dumps(item, sort_keys=True)+'\n')
    negatives = []
    for item in plan:
        for mode in ('range', 'source', 'value', 'order', 'PDU-id'):
            bad = copy.deepcopy(item['whole_expected'])
            pdu, = [p for p in bad['pdus'] if p['id'] == item['id']]
            if mode == 'range':
                field = next(v for v in pdu['field_values']['values'] if v.get('pdu_range'))
                field['pdu_range']['offset'] += 1
            elif mode == 'source':
                pdu['field_values']['source']['SHA256'] = '0'*64
            elif mode == 'value':
                pdu['field_values']['values'][0]['value'] = []
            elif mode == 'order':
                pdu['field_values']['values'].reverse()
            else:
                pdu['id'] += 1
            assert not same(bad, item['whole_expected'])
            negatives.append(dict(frame=item['frame'], rejected=mode))
    proof = dict(scope=__doc__, UTC=datetime.datetime.now(datetime.timezone.utc).isoformat(),
                 before_runtime_SHA256=sha(args.before/'summary.json'), baseline_source=summary['source'],
                 native_plan_summary_SHA256=sha(args.native_plan/'summary.json'),
                 native_plan_order_correction_explicit=True, frozen_before_new_CLI_execution=True,
                 plan_SHA256=sha(target), whole_changed_rows=len(plan), positives=len(plan), negatives=len(negatives),
                 negative_controls=negatives, auditor_SHA256=sha(__file__), helper_SHA256=sha(base))
    path = args.out/'summary.json'
    path.write_text(json.dumps(proof, indent=2)+'\n', encoding='utf8')
    print(json.dumps(dict(summary=str(path), SHA256=sha(path), whole_rows=len(plan), negatives=len(negatives))), flush=True)


parser = argparse.ArgumentParser(description=__doc__)
commands = parser.add_subparsers(dest='mode', required=True)
pre = commands.add_parser('prepare')
for name in ('before', 'native-plan', 'out'):
    pre.add_argument('--'+name, type=Path, required=True)
ver = commands.add_parser('verify')
for name in ('before', 'after', 'plan', 'out'):
    ver.add_argument('--'+name, type=Path, required=True)
args = parser.parse_args()
if args.mode == 'prepare':
    prepare_prefix(args)
else:
    assert read(args.plan/'summary.json')['helper_SHA256'] == sha(base)
    verify_complete(args)
