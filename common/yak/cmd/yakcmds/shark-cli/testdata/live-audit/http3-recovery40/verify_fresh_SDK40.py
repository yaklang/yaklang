"""Reuse immutable source39 assertions; adapt only producer provenance."""
from pathlib import Path
import argparse, hashlib, importlib.util, json, sys

phase = Path(__file__).parent
sha = lambda p: hashlib.sha256(Path(p).read_bytes()).hexdigest()
read = lambda p: json.loads(Path(p).read_text('utf-8-sig'))
ap = argparse.ArgumentParser(description=__doc__)
for key in ('actual', 'audit', 'recovery', 'out'):
    ap.add_argument('--'+key, type=Path, required=True)
ap.add_argument('--expected-source', required=True)
a = ap.parse_args(); a.phase = phase; a.sources = phase/'fresh-TLS-QUIC-source39-01'
a.out.mkdir(); sys.path.insert(0, str(a.audit))
old = a.sources/'verify_fresh_SDK39.py'
assert sha(old) == 'f9f6517067f129b34f9d376fbd4f88de48dd553b031605cf9002f0852b833c8f'
spec = importlib.util.spec_from_file_location('immutable_SDK_source39', old)
m = importlib.util.module_from_spec(spec); spec.loader.exec_module(m)
bound = {str(old):sha(old), str(Path(__file__)):sha(__file__)}
for item in read(a.sources/'source-manifest01.json')['artifacts']:
    assert sha(item['path']) == item['SHA256']; bound[item['path']] = item['SHA256']
for name in ('TLS-source03-audit02/summary.json', 'QUIC-source01-audit01/summary.json'):
    proof = read(a.sources/name)
    for path, digest in proof.get('input_hashes', proof.get('inputs', {})).items():
        assert sha(path) == digest; bound[path] = digest
assert sha(a.sources/'source-manifest01.json') == '8be15e99bfbd572364a4c0acf5c6b19eb4cd2e2f7c02a0a5c5b9a3fe5482bf72'

def producer(actual, kind, bound):
    top = read(actual/'summary.json'); proof = read(actual/kind/'events-producer.json')
    assert proof in top['producers'] and proof['source'] == top['source'] == a.expected_source and proof['exit_code'] == 0
    output = actual/kind/'events.jsonl'; binary = Path(proof['arguments'][0]); receipt = binary.parent/'events-receipt.json'; build = read(receipt)
    assert sha(output) == proof['export_SHA256'] and sha(receipt) == proof['build_receipt_SHA256']
    assert sha(binary) == proof['binary_SHA256'] == build['binary_SHA256'] and build['source'] == a.expected_source and build['exit_code'] == 0
    for item in proof['inputs'].values():
        assert sha(item['path']) == item['SHA256']; bound[item['path']] = item['SHA256']
    for p in (actual/'summary.json', actual/kind/'events-producer.json', output, binary, receipt):bound[str(p)] = sha(p)
    return m.rows(output)

m.producer = producer
result = {key:fn(a,bound) for key, fn in [('TLS',m.TLS),('QUIC',m.QUIC)]}
assert all(sha(p) == digest for p,digest in bound.items())
failed = sum(r.get('failed_events',r.get('failed_events_or_app_frames',0)) for r in result.values())
target = a.out/'summary.json'
with target.open('x',encoding='utf8') as f:
    json.dump(dict(scope=__doc__, source=a.expected_source, producer_adapter_only=True,
        unchanged_source_assertion_module_SHA256=sha(old), results=result, failed_checks=failed,
        source_manifest_SHA256=sha(a.sources/'source-manifest01.json'), actual_summary_SHA256=sha(a.actual/'summary.json'),
        inputs=bound, inputs_unchanged=True, whole_event_JSON_equality_claim=False), f, indent=2);f.write('\n')
print(json.dumps(dict(summary=str(target),SHA256=sha(target),failed_checks=failed)))
assert failed == 0
