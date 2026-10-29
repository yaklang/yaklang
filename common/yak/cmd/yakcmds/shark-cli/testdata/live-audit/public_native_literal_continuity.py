"""Bind every complete native event/flow export to its actual closed producer.

This proves preservation against the specified source version; it is not a
claim that all unchanged protocol interpretations are correct.
"""
import argparse
import functools
import hashlib
import json
from pathlib import Path


@functools.cache
def sha(path):
    path = Path(path)
    before = path.stat()
    h = hashlib.sha256()
    with path.open('rb') as stream:
        for block in iter(lambda: stream.read(1 << 20), b''):
            h.update(block)
    after = path.stat()
    assert (before.st_size, before.st_mtime_ns) == (after.st_size, after.st_mtime_ns)
    return h.hexdigest()


def read(path):
    return json.loads(Path(path).read_text('utf-8-sig'))


def executions(root):
    summary = read(root/'summary.json')
    assert summary['cases'] == 37 and summary['actual_new_binary_execution'] is True
    result = {}
    receipts = sorted(root.glob('*/*/*-producer.json'))
    assert len(receipts) == len(summary['outputs']) == 39
    for path in receipts:
        p = read(path)
        assert p in summary['outputs'] and p['source'] == summary['source'] and p['exit_code'] == 0
        kind = path.stem.removesuffix('-producer')
        assert kind in ('events', 'flows')
        binary = Path(p['arguments'][0])
        build = read(binary.parent/(kind+'-receipt.json'))
        assert build['source'] == p['source'] and build['exit_code'] == 0
        assert sha(binary) == build['binary_SHA256'] == p['binary_SHA256']
        assert sha(p['capture']) == p['capture_SHA256']
        output = path.with_name(kind+('.jsonl' if kind == 'events' else '.json'))
        assert sha(output) == p['export_SHA256']
        if kind == 'events':
            rows = 0
            with output.open(encoding='utf-8-sig') as stream:
                for line in stream:
                    json.loads(line)
                    rows += 1
        else:
            value = read(output)
            assert set(value) == {'capture_SHA256', 'closed', 'error', 'flows', 'packets', 'stats'}
            assert value['capture_SHA256'] == p['capture_SHA256']
            rows = 1
        key = str(output.relative_to(root))
        assert key not in result
        result[key] = dict(kind=kind, producer=p, receipt_SHA256=sha(path),
                           whole_export_SHA256=sha(output), rows=rows)
    return summary, result


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    for name in ('before', 'after', 'out'):
        parser.add_argument('--'+name, type=Path, required=True)
    args = parser.parse_args()
    args.out.mkdir(exist_ok=False)
    before, old = executions(args.before)
    after, new = executions(args.after)
    assert old.keys() == new.keys()
    checks = []
    for key in old:
        a, b = old[key], new[key]
        for field in ('capture', 'capture_SHA256', 'keylog_used'):
            assert a['producer'][field] == b['producer'][field]
        assert a['producer']['arguments'][1:] == b['producer']['arguments'][1:]
        assert a['rows'] == b['rows'] and a['whole_export_SHA256'] == b['whole_export_SHA256'], key
        checks.append(dict(export=key, kind=b['kind'], rows=b['rows'],
                           whole_export_SHA256=b['whole_export_SHA256'],
                           before_receipt_SHA256=a['receipt_SHA256'], after_receipt_SHA256=b['receipt_SHA256']))
    summary = dict(scope=__doc__, before_source=before['source'], after_source=after['source'],
                   inputs=37, closed_executions=39, native_event_rows=sum(c['rows'] for c in checks if c['kind'] == 'events'),
                   whole_flow_files=sum(c['kind'] == 'flows' for c in checks),
                   all_exports_byte_identical=True, differences=0, ID_mapping=False,
                   before_runtime_SHA256=sha(args.before/'summary.json'),
                   after_runtime_SHA256=sha(args.after/'summary.json'), auditor_SHA256=sha(__file__), checks=checks)
    target = args.out/'summary.json'
    target.write_text(json.dumps(summary, indent=2)+'\n', encoding='utf8')
    print(json.dumps(dict(summary=str(target), SHA256=sha(target),
                          native_event_rows=summary['native_event_rows'], whole_flow_files=summary['whole_flow_files'], differences=0)), flush=True)


if __name__ == '__main__':
    main()
