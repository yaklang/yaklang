"""Compare complete replay files with their original source/build/input receipts.

This certifies literal preservation only. It cannot certify the correctness of
unchanged protocol fields, and never renumbers PDUs or normalizes JSON values.
"""
import argparse
import hashlib
import json
from pathlib import Path


def sha(path):
    digest = hashlib.sha256()
    with Path(path).open('rb') as stream:
        for block in iter(lambda: stream.read(1 << 20), b''):
            digest.update(block)
    return digest.hexdigest()


def read(path):
    return json.loads(Path(path).read_text('utf-8-sig'))


def binding(folder, case, plan):
    proof = read(folder/'producer.json')
    assert proof['exit_code'] == 0
    assert proof['plan_SHA256'] == sha(plan)
    assert proof['capture_SHA256'] == case['capture_SHA256'] == sha(case['capture'])
    assert proof['keylog_SHA256'] == case['keylog_SHA256'] == sha(case['keylog'])
    assert proof['export_SHA256'] == sha(folder/'events.jsonl')
    assert proof['stderr_SHA256'] == sha(folder/'stderr')
    binary = Path(proof['arguments'][0])
    build_path = binary.parent/'events-receipt.json'
    build = read(build_path)
    assert build['exit_code'] == 0 and proof['source'] == build['source']
    assert len(proof['source']) == 40
    assert proof['build_receipt_SHA256'] == sha(build_path)
    assert proof['binary_SHA256'] == build['binary_SHA256'] == sha(binary)
    assert proof['arguments'][1:] == ['replay', case['capture'], case['keylog'], '--ignore-embedded-secrets']
    return proof


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    for name in ('before', 'after', 'plan', 'out'):
        parser.add_argument('--'+name, type=Path, required=True)
    args = parser.parse_args()
    args.out.mkdir(exist_ok=False)
    plan = read(args.plan)
    checks = []
    before_sources, after_sources = set(), set()
    for case in plan['cases']:
        name = case['case']
        before, after = args.before/name, args.after/name
        old, new = binding(before, case, args.plan), binding(after, case, args.plan)
        before_sources.add(old['source'])
        after_sources.add(new['source'])
        old_file, new_file = before/'events.jsonl', after/'events.jsonl'
        assert old_file.stat().st_size == new_file.stat().st_size
        assert old['export_SHA256'] == new['export_SHA256'], name
        count = 0
        # Compare actual bytes as well as the two independently bound digests.
        with old_file.open('rb') as a, new_file.open('rb') as b:
            while True:
                x, y = a.readline(), b.readline()
                assert x == y, (name, count+1)
                if not x:
                    break
                assert isinstance(json.loads(x), dict)
                count += 1
        checks.append(dict(case=name, whole_events=count,
            whole_export_SHA256=new['export_SHA256'],
            before_producer_SHA256=sha(before/'producer.json'),
            after_producer_SHA256=sha(after/'producer.json')))
    assert len(before_sources) == len(after_sources) == 1
    summary = dict(scope=__doc__, before_source=before_sources.pop(),
        after_source=after_sources.pop(), cases=len(checks),
        whole_events=sum(c['whole_events'] for c in checks),
        differences=0, ID_mapping=False, normalization=False,
        plan_SHA256=sha(args.plan), auditor_SHA256=sha(__file__), checks=checks)
    target = args.out/'summary.json'
    target.write_text(json.dumps(summary, indent=2, sort_keys=True)+'\n', encoding='utf8')
    print(json.dumps(dict(summary=str(target), SHA256=sha(target),
        whole_events=summary['whole_events'], differences=0)), flush=True)


if __name__ == '__main__':
    main()
