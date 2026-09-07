#!/usr/bin/env python3
"""Build diagnostic fixtures first; then run costs, CPU and allocation separately."""
import argparse
import datetime as dt
import hashlib
import json
import os
from pathlib import Path
import subprocess

REPO = Path(__file__).resolve().parents[4]


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def git(*args, cwd=REPO):
    return subprocess.check_output(['git', *args], cwd=cwd, text=True).strip()


def diagnose(artifacts):
    frozen = json.loads((artifacts/'freeze.json').read_text())
    stages = {s['label']: s for s in frozen['stages']}
    final = frozen['stages'][-1]
    source = artifacts/'source'
    output = artifacts/'diagnostics'
    output.mkdir()
    (output/'diagnose.py').write_bytes(Path(__file__).read_bytes())
    if git('status', '--porcelain', cwd=source):
        raise ValueError('source checkout is dirty')
    fixtures = ['common/bin-parser/protocol_hotpath_benchmark_test.go',
                'common/bin-parser/parser/stream_parser/hotpath_benchmark_test.go']
    git('switch', '--detach', final['commit'], cwd=source)
    data = {name: (source/name).read_bytes() for name in fixtures}
    metadata = {'created_utc': dt.datetime.now(dt.timezone.utc).isoformat(),
                'script_sha256': sha(Path(__file__)), 'fixtures': {}, 'binaries': {}, 'runs': []}
    for name, content in data.items():
        saved = output/(Path(name).name+'.txt')
        saved.write_bytes(content)
        metadata['fixtures'][name] = {'file': saved.name, 'sha256': sha(saved)}
    # Only benchmark files are added to the original source. No implementation,
    # captures, rule data, compiler flags or dependencies are changed.
    git('switch', '--detach', stages['original']['commit'], cwd=source)
    # The original tree construction lived inside the decoding transaction.
    # Extract that body verbatim into a test-only helper for the same pure
    # construction fixture. Do not edit the production function under test.
    original_tree = (source/'common/bin-parser/parser/stream_parser/tls_certificate_tree.go').read_text()
    begin = original_tree.index('\tstaged := &base.Node')
    end = original_tree.index('\tcommitted = true\n\treturn nil\n}', begin)
    helper = ('package stream_parser\nimport ("fmt"; '
        '"github.com/yaklang/yaklang/common/bin-parser/parser/base"; '
        'yaml "github.com/yaklang/yaklang/common/utils/orderedyaml")\n'
        'func buildExactByteFieldTree(node *base.Node, fields []tlsCertificateField, info map[string]any, '
        'start, bits uint64, profile, endian string) error {\nvar err error\n'
        + original_tree[begin:end] + '\treturn nil\n}\n')
    helper_name = 'common/bin-parser/parser/stream_parser/hotpath_original_tree_test.go'
    data[helper_name] = helper.encode()
    saved = output/(Path(helper_name).name+'.txt')
    saved.write_text(helper)
    metadata['fixtures'][helper_name] = {'file': saved.name, 'sha256': sha(saved),
        'extracted_from': 'common/bin-parser/parser/stream_parser/tls_certificate_tree.go',
        'source_commit': stages['original']['commit']}
    for name, content in data.items():
        if (source/name).exists():
            raise ValueError('diagnostic fixture already exists in original source')
        (source/name).write_bytes(content)
    def build(label, package, commit):
        binary = output/(label+'.test')
        args = ['go', 'test', '-c', '-o', str(binary), package]
        with (output/(label+'-build.txt')).open('x') as f:
            subprocess.run(args, cwd=source, stdout=f, stderr=subprocess.STDOUT, check=True)
        metadata['binaries'][label] = {'commit': commit, 'sha256': sha(binary), 'command': args}
    try:
        build('original-main', './common/bin-parser', stages['original']['commit'])
        build('original-stream', './common/bin-parser/parser/stream_parser', stages['original']['commit'])
    finally:
        for name in data:
            (source/name).unlink()
        git('switch', '--detach', final['commit'], cwd=source)
    build('final-stream', './common/bin-parser/parser/stream_parser', final['commit'])
    if git('status', '--porcelain', cwd=source):
        raise ValueError('source checkout was changed')
    # Every compile finishes before any measurement begins.
    def run(label, binary, args, cwd, kind):
        raw = output/(label+'.txt')
        env = dict(os.environ, GOMAXPROCS='1', PWD=str(cwd))
        counts = [] if any(a.startswith('-test.count=') for a in args) else ['-test.count=1']
        command = [str(binary), '-test.run=^$', *counts, '-test.timeout=5m', *args]
        started = dt.datetime.now(dt.timezone.utc).isoformat()
        with raw.open('x') as f:
            subprocess.run(command, cwd=cwd, env=env, stdout=f, stderr=subprocess.STDOUT, check=True)
        metadata['runs'].append({'label': label, 'kind': kind, 'command': command,
            'cwd': str(cwd), 'pwd': str(cwd), 'started_utc': started, 'raw_sha256': sha(raw),
            'binary_sha256': sha(binary)})
        (output/'diagnostics.json').write_text(json.dumps(metadata, indent=2)+'\n')
        print('PASS', label, flush=True)
    parser = REPO/'common/bin-parser'
    for label, binary in [('original', output/'original-main.test'), ('final', Path(final['binary']))]:
        run(label+'-stages', binary, ['-test.bench=^BenchmarkCurrentCorpusStages$', '-test.benchtime=3s', '-test.benchmem', '-test.count=3'], parser, 'cost')
    for label in ['original', 'final']:
        run(label+'-native', output/(label+'-stream.test'),
            ['-test.bench=^BenchmarkNativeField(Decoding|TreeLong)$', '-test.benchtime=3s', '-test.benchmem', '-test.count=3'],
            parser/'parser/stream_parser', 'cost')
    for label in ['original', final['label']]:
        binary = Path(stages[label]['binary'])
        for kind, flag in [('cpu', '-test.cpuprofile='), ('alloc', '-test.memprofile=')]:
            profile = output/(label+'.'+kind)
            run(label+'-'+kind, binary, ['-test.bench=^BenchmarkCurrentCorpusApplicationJSON$', '-test.benchtime=3s', '-test.benchmem', flag+str(profile)], parser, kind)
            options = ['-sample_index=alloc_space'] if kind == 'alloc' else []
            with (output/(label+'-'+kind+'-top.txt')).open('x') as f:
                subprocess.run(['go', 'tool', 'pprof', '-top', '-nodecount=40', *options, str(binary), str(profile)], stdout=f, stderr=subprocess.STDOUT, check=True)
            metadata.setdefault('profiles', {})[profile.name] = {'sha256': sha(profile), 'binary_sha256': sha(binary)}
    (output/'diagnostics.json').write_text(json.dumps(metadata, indent=2)+'\n')


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--artifacts', required=True, type=Path)
    args = parser.parse_args()
    diagnose(args.artifacts.resolve())
