#!/usr/bin/env python3
"""Freeze separately, then time only binaries. Python standard library only."""
import argparse
import datetime as dt
import hashlib
import json
import os
from pathlib import Path
import platform
import re
import statistics
import subprocess
import time

HERE = Path(__file__).resolve().parent
REPO = HERE.parents[3]
PARSER = REPO / 'common/bin-parser'
BASE = '5b6cbe1f8'
PHASES = [
    ('original', BASE), ('p0', 'b6cfeef97'), ('compact', '8b15fd133'),
    ('windows', '93573afb1'), ('rules', 'a70f37551'), ('node_batch', '7a008559e'),
    ('native_bridge', '6a26dad1a'), ('prefix', '27fca80fb'),
    ('rule_batch', 'cc4acef59'), ('projection', '93ddd19be'),
    ('descriptors', '18152b701'), ('clone_plan', '97494dd99'),
    ('import_config', '69d3d823d'),
]
REPRESENTATIVES = [
    '^BenchmarkCurrentCorpusApplicationFields$',
    '^BenchmarkCurrentCorpusEnvelopes$',
    '^BenchmarkNHRPClientList$/^(32|4096)$/^legacy-false$',
    '^BenchmarkDICOMPDVList$/^items-(128|4096)$',
    '^BenchmarkDICOMUserInformation$',
    '^BenchmarkProtocolCorpusIPX$',
    '^BenchmarkProtocolCorpusSVExecOut$',
    '^BenchmarkParserConsumedLength$/^(ordinary-184B|counts-1454B)$/^AllJoynNS$/^DirectLookup$',
]

def command(args, cwd=REPO):
    return subprocess.check_output(args, cwd=cwd, text=True).strip()

def digest(path):
    h = hashlib.sha256()
    with open(path, 'rb') as f:
        for chunk in iter(lambda: f.read(1 << 20), b''):
            h.update(chunk)
    return h.hexdigest()

def freeze(artifacts, candidate):
    artifacts.mkdir(parents=True, exist_ok=True)
    metadata = artifacts / 'freeze.json'
    if metadata.exists():
        raise RuntimeError('refusing to overwrite a frozen experiment')
    phases = PHASES + [('candidate', candidate)]
    result = {'created_utc': dt.datetime.now(dt.timezone.utc).isoformat(),
              'platform': platform.platform(), 'machine': platform.machine(),
              'go_version': command(['go', 'version']),
              'go_env': json.loads(command(['go', 'env', '-json', 'GOVERSION', 'GOOS', 'GOARCH', 'CGO_ENABLED', 'GOFLAGS', 'GOTOOLCHAIN', 'GOEXPERIMENT'])),
              'stages': [], 'script_sha256': digest(__file__)}
    source = artifacts / 'source'
    subprocess.run(['git', 'worktree', 'add', '--detach', str(source), BASE], cwd=REPO, check=True)
    # A complete original source archive plus binary patches reconstruct every
    # stage. Binaries and archives stay outside the repository; hashes are kept.
    archive = artifacts / 'original-source.tar.gz'
    subprocess.run(['git', 'archive', '--format=tar.gz', '-o', str(archive), BASE], cwd=REPO, check=True)
    result['original_archive_sha256'] = digest(archive)
    for label, ref in phases:
        sha = command(['git', 'rev-parse', ref])
        subprocess.run(['git', 'switch', '--detach', sha], cwd=source, check=True)
        if command(['git', 'status', '--porcelain'], source):
            raise RuntimeError('freeze source is dirty')
        patch = artifacts / (label + '.patch')
        with patch.open('wb') as f:
            subprocess.run(['git', 'diff', '--binary', BASE, sha], cwd=REPO, stdout=f, check=True)
        binary = artifacts / (label + '.test')
        with (artifacts / (label + '-build.txt')).open('x') as f:
            subprocess.run(['go', 'test', '-c', '-o', str(binary), './common/bin-parser'], cwd=source, stdout=f, stderr=subprocess.STDOUT, check=True)
        item = {'label': label, 'commit': sha, 'git_tree': command(['git', 'rev-parse', sha+'^{tree}']),
                'binary': str(binary), 'binary_sha256': digest(binary),
                'patch_sha256': digest(patch), 'manifest_sha256': digest(source/'common/bin-parser/testdata/protocol-corpus/manifest.json')}
        result['stages'].append(item)
        metadata.write_text(json.dumps(result, indent=2)+'\n')
        print('frozen', label, sha, flush=True)
    result['complete'] = True
    metadata.write_text(json.dumps(result, indent=2)+'\n')


def append_freeze(artifacts, candidate, label):
    metadata = artifacts/'freeze.json'
    result = json.loads(metadata.read_text())
    if not result.get('complete') or any(s['label'] == label for s in result['stages']):
        raise RuntimeError('incomplete freeze or duplicate stage label')
    with (artifacts/('freeze-before-'+label+'.json')).open('x') as f:
        f.write(metadata.read_text())
    sha = command(['git', 'rev-parse', candidate])
    source = artifacts/'source'
    if command(['git', 'status', '--porcelain'], source):
        raise RuntimeError('freeze source is dirty')
    subprocess.run(['git', 'switch', '--detach', sha], cwd=source, check=True)
    patch, binary = artifacts/(label+'.patch'), artifacts/(label+'.test')
    if binary.exists():
        raise RuntimeError('binary already exists')
    with patch.open('xb') as f:
        subprocess.run(['git', 'diff', '--binary', BASE, sha], cwd=REPO, stdout=f, check=True)
    with (artifacts/(label+'-build.txt')).open('x') as f:
        subprocess.run(['go', 'test', '-c', '-o', str(binary), './common/bin-parser'], cwd=source, stdout=f, stderr=subprocess.STDOUT, check=True)
    result['stages'].append({'label': label, 'commit': sha,
                            'git_tree': command(['git', 'rev-parse', sha+'^{tree}']),
                            'binary': str(binary), 'binary_sha256': digest(binary),
                            'patch_sha256': digest(patch),
                            'manifest_sha256': digest(source/'common/bin-parser/testdata/protocol-corpus/manifest.json')})
    result['script_sha256'] = digest(__file__)
    metadata.write_text(json.dumps(result, indent=2)+'\n')
    print('frozen', label, sha, flush=True)


def run(artifacts, mode, tag, rounds, selected_loads=None, worker_counts=None):
    frozen = json.loads((artifacts/'freeze.json').read_text())
    if not frozen.get('complete'):
        raise RuntimeError('freeze did not complete')
    stages = {s['label']: s for s in frozen['stages']}
    for s in stages.values():
        if digest(s['binary']) != s['binary_sha256']:
            raise RuntimeError('binary changed: '+s['label'])
    output = artifacts / tag
    output.mkdir()  # Do not silently replace previous or failed measurements.
    (output/'measure.py').write_bytes(Path(__file__).read_bytes())
    (output/'run.json').write_text(json.dumps({'mode': mode, 'rounds': rounds,
        'freeze_sha256': digest(artifacts/'freeze.json'), 'script_sha256': digest(__file__),
        'environment': {k: os.environ.get(k) for k in ['GOGC', 'GOMEMLIMIT', 'GODEBUG']},
        'selected_loads': selected_loads, 'worker_counts': worker_counts or [1, 10],
        'started_utc': dt.datetime.now(dt.timezone.utc).isoformat()}, indent=2)+'\n')
    if mode in ('application', 'qualification'):
        loads = ['^BenchmarkCurrentCorpusApplicationJSON$']
        candidates = list(stages)[1:] if mode == 'application' else [frozen['stages'][-1]['label']]
    elif mode == 'representatives':
        loads = REPRESENTATIVES
        candidates = [frozen['stages'][-1]['label']]
    else:
        loads, candidates = [], list(stages)
    if selected_loads:
        if mode != 'representatives':
            raise ValueError('load selection is only for representative retests')
        loads = selected_loads
    rows = output/'runs.jsonl'
    counter = 0
    def execute(label, workers, expression, pair):
        nonlocal counter
        counter += 1
        raw = output/f'{counter:04d}-{label}-w{workers}.txt'
        s = stages[label]
        args = [s['binary'], '-test.count=1', '-test.timeout=20m']
        # Match direct shell/go-test execution from the benchmark directory.
        # subprocess(cwd=...) alone leaves the parent's PWD stale.
        env = dict(os.environ, GOMAXPROCS=str(workers), PWD=str(PARSER))
        if mode == 'digests':
            args += ['-test.run=^TestProtocolCorpusExportDigest$', '-test.v']
            env['BINPARSER_EVALUATE'] = '1'
        else:
            args += ['-test.run=^$', '-test.bench='+expression, '-test.benchtime=3s', '-test.benchmem']
        start = dt.datetime.now(dt.timezone.utc).isoformat()
        begin = time.monotonic()
        with raw.open('x') as f:
            rc = subprocess.run(args, cwd=PARSER, env=env, stdout=f, stderr=subprocess.STDOUT).returncode
        content = raw.read_text()
        measurements = []
        for line in content.splitlines():
            match = re.match(r'^(Benchmark\S+)\s+(\d+)\s+(.*)$', line)
            if match:
                values = match.group(3).split()
                # Go omits the CPU suffix at GOMAXPROCS=1. A sub-benchmark
                # such as items-4096 must retain its input-size suffix.
                name = match.group(1)
                if workers > 1:
                    name = re.sub(r'-'+str(workers)+r'$', '', name)
                measurements.append({'name': name, 'iterations': int(match.group(2)),
                                     'metrics': {values[i+1]: float(values[i]) for i in range(0, len(values)-1, 2)}})
        row = {'sequence': counter, 'label': label, 'commit': s['commit'], 'pair': pair,
               'workers': workers, 'pwd': env.get('PWD'), 'expression': expression, 'command': args, 'cwd': str(PARSER),
               'start_utc': start, 'elapsed_s': time.monotonic()-begin, 'exit_code': rc,
               'raw': raw.name, 'raw_sha256': digest(raw), 'measurements': measurements}
        with rows.open('a') as f:
            f.write(json.dumps(row)+'\n')
        print(counter, label, workers, 'PASS' if rc == 0 else 'FAIL', round(row['elapsed_s'], 2), flush=True)
        if rc != 0 or (mode != 'digests' and not measurements):
            raise RuntimeError('failed run retained at '+str(raw))
    if mode == 'digests':
        for label in candidates:
            execute(label, 4, '', label)
        return
    for workers in worker_counts or [1, 10]:
        for window in range(rounds):
            for load_index, expression in enumerate(loads):
                for candidate in candidates:
                    pair = f'w{workers}-r{window}-{load_index}-{candidate}'
                    labels = ['original', candidate] if window % 2 == 0 else [candidate, 'original']
                    for label in labels:
                        execute(label, workers, expression, pair)
    report = {}
    for line in rows.read_text().splitlines():
        row = json.loads(line)
        for m in row['measurements']:
            key = (row['label'], row['workers'], m['name'])
            report.setdefault(key, []).append(m['metrics'])
    summary = []
    for (label, workers, name), samples in report.items():
        summary.append({'label': label, 'workers': workers, 'name': name, 'samples': len(samples),
                        'median': {k: statistics.median(s[k] for s in samples) for k in samples[0]},
                        'range_ns': [min(s['ns/op'] for s in samples), max(s['ns/op'] for s in samples)]})
    (output/'summary.json').write_text(json.dumps(summary, indent=2)+'\n')

if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('mode', choices=['freeze', 'application', 'qualification', 'representatives', 'digests'])
    parser.add_argument('--artifacts', type=Path, required=True)
    parser.add_argument('--candidate', default='HEAD')
    parser.add_argument('--label', help='append a new immutable stage to an existing freeze')
    parser.add_argument('--tag', default='formal')
    parser.add_argument('--rounds', type=int, default=5)
    parser.add_argument('--load', action='append', help='exact benchmark regex for a representative retest; repeatable')
    parser.add_argument('--workers', type=int, nargs='+', choices=[1, 10], help='worker modes for a representative retest')
    args = parser.parse_args()
    if args.rounds < 5 and args.mode != 'freeze':
        parser.error('formal comparisons require at least five independent windows')
    if (args.load or args.workers) and args.mode != 'representatives':
        parser.error('load/worker selection is only for representative retests')
    if args.mode == 'freeze':
        if args.label:
            append_freeze(args.artifacts.resolve(), args.candidate, args.label)
        else:
            freeze(args.artifacts.resolve(), args.candidate)
    else:
        run(args.artifacts.resolve(), args.mode, args.tag, args.rounds, args.load, args.workers)
