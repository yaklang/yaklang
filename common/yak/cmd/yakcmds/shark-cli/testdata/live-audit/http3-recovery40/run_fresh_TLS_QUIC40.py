"""Replay frozen independently audited real captures with a pinned new build."""
from pathlib import Path
import argparse, collections, hashlib, json, os, shutil, subprocess, time

phase = Path(__file__).parent
sha = lambda p: hashlib.sha256(Path(p).read_bytes()).hexdigest()
read = lambda p: json.loads(Path(p).read_text('utf-8-sig'))
def write(p, value):
    with Path(p).open('x', encoding='utf8') as f:
        json.dump(value, f, indent=2); f.write('\n')

ap = argparse.ArgumentParser(description=__doc__)
ap.add_argument('--out', type=Path, required=True)
ap.add_argument('--bins', type=Path, required=True)
ap.add_argument('--expected-source', required=True)
a = ap.parse_args()
a.out.mkdir()
proofs = []
for name, folder in [('TLS', 'fresh-TLS-resumption39-03'), ('QUIC', 'fresh-QUIC-epochs39-01')]:
    target = a.out / name; target.mkdir()
    source = phase / folder
    frozen = {k: dict(path=str(p), SHA256=sha(p), bytes=p.stat().st_size) for k, p in dict(
        capture=source/'live.pcap', keylog=source/'session.keys', peer_manifest=source/'manifest.json').items()}
    for kind in ('events', 'yak'):
        assert shutil.disk_usage(phase).free > 20 * (1 << 30)
        receipt = a.bins / (kind+'-receipt.json'); build = read(receipt)
        binary = a.bins / (kind+'.exe')
        assert build['exit_code'] == 0 and build['source'] == a.expected_source
        assert sha(binary) == build['binary_SHA256']
        capture, keys = source/'live.pcap', source/'session.keys'
        args = ([str(binary), 'replay', str(capture), str(keys), '--ignore-embedded-secrets'] if kind == 'events' else
                [str(binary), 'shark', '-r', str(capture), '--json', '--ignore-embedded-secrets', '--tls-keylog', str(keys)])
        output = target / (kind+'.jsonl'); started = time.monotonic()
        with output.open('xb') as log, (target/(kind+'.stderr')).open('xb') as error:
            r = subprocess.run(args, stdout=log, stderr=error, timeout=180, creationflags=0x08000000,
                               env=dict(os.environ, GOMAXPROCS='1', GOMEMLIMIT='1GiB'))
        assert sha(binary) == build['binary_SHA256'] and all(sha(v['path']) == v['SHA256'] for v in frozen.values())
        counts = collections.Counter(); rows = 0
        for line in output.open(encoding='utf-8-sig'):
            v = json.loads(line); rows += 1; counts[v['protocol']] += 1
        proof = dict(source=a.expected_source, arguments=args, controller_SHA256=sha(__file__),
            build_receipt_SHA256=sha(receipt), binary_SHA256=sha(binary), inputs=frozen,
            exit_code=r.returncode, seconds=round(time.monotonic()-started, 3), export_SHA256=sha(output),
            rows=rows, raw_protocol_counts=dict(counts), correctness_requires_separate_source_audit=True)
        write(target/(kind+'-producer.json'), proof); proofs.append(proof)
        assert r.returncode == 0
        print(json.dumps(dict(capture=name, kind=kind, rows=rows, counts=dict(counts), closed=True)), flush=True)
write(a.out/'summary.json', dict(scope=__doc__, source=a.expected_source, producers=proofs, artifacts_external=True))
print(json.dumps(dict(summary=str(a.out/'summary.json'), SHA256=sha(a.out/'summary.json'))))
