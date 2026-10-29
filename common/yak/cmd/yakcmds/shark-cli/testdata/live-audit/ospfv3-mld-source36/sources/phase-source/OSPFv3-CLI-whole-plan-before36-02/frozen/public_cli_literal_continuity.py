"""Verify full CLI exports and original execution bindings without ID rewrites."""
import argparse
import hashlib
import json
from pathlib import Path


def sha(path):
    h = hashlib.sha256()
    with Path(path).open('rb') as stream:
        for block in iter(lambda: stream.read(1 << 20), b''):
            h.update(block)
    return h.hexdigest()


def read(path):
    return json.loads(Path(path).read_text('utf-8-sig'))


def executions(root):
    summary_path = root/'summary.json'
    summary = read(summary_path)
    assert summary['cases'] == 37 and summary['actual_new_binary_execution'] is True
    receipts = sorted(root.glob('*/*/yak-producer.json'))
    assert len(receipts) == len(summary['outputs']) == 37
    proofs = {}
    for path in receipts:
        item = read(path)
        key = str(path.parent.relative_to(root))
        assert item in summary['outputs'] and item['source'] == summary['source']
        assert item['exit_code'] == 0
        binary = Path(item['arguments'][0])
        build = read(binary.parent/'yak-receipt.json')
        assert build['source'] == item['source'] and build['exit_code'] == 0
        assert sha(binary) == item['binary_SHA256'] == build['binary_SHA256']
        assert sha(item['capture']) == item['capture_SHA256']
        output = path.parent/'yak.jsonl'
        assert sha(output) == item['export_SHA256']
        rows = 0
        with output.open('r', encoding='utf-8-sig') as stream:
            for line in stream:
                json.loads(line)
                rows += 1
        proofs[key] = dict(receipt=item, receipt_SHA256=sha(path), rows=rows,
                           whole_export_SHA256=sha(output))
    return summary, proofs, sha(summary_path)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    for name in ('before', 'after', 'out'):
        parser.add_argument('--'+name, type=Path, required=True)
    args = parser.parse_args()
    args.out.mkdir(exist_ok=False)
    before, old, before_sha = executions(args.before)
    after, new, after_sha = executions(args.after)
    assert old.keys() == new.keys()
    checks = []
    for key in old:
        a, b = old[key], new[key]
        for field in ('capture', 'capture_SHA256', 'keylog_used'):
            assert a['receipt'][field] == b['receipt'][field]
        assert a['receipt']['arguments'][1:] == b['receipt']['arguments'][1:]
        assert a['rows'] == b['rows'] and a['whole_export_SHA256'] == b['whole_export_SHA256'], key
        checks.append(dict(case=key, rows=b['rows'], export_SHA256=b['whole_export_SHA256'],
                           before_receipt_SHA256=a['receipt_SHA256'], after_receipt_SHA256=b['receipt_SHA256']))
    summary = dict(scope=__doc__, before_source=before['source'], after_source=after['source'],
                   cases=37, whole_rows=sum(x['rows'] for x in checks), differences=0,
                   ID_mapping=False, before_runtime_SHA256=before_sha, after_runtime_SHA256=after_sha,
                   auditor_SHA256=sha(__file__), checks=checks)
    target = args.out/'summary.json'
    target.write_text(json.dumps(summary, indent=2)+'\n', encoding='utf8')
    print(json.dumps(dict(summary=str(target), SHA256=sha(target), cases=37,
                          whole_rows=summary['whole_rows'], differences=0)), flush=True)


if __name__ == '__main__':
    main()
