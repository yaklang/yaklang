#!/usr/bin/env python3
"""Diagnostic stage localization, never a replacement for five-window acceptance."""
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


def run(args):
    frozen = json.loads((args.artifacts/'freeze.json').read_text())
    stages = {s['label']: s for s in frozen['stages']}
    output = args.artifacts/args.tag
    output.mkdir()
    (output/'probe.py').write_bytes(Path(__file__).read_bytes())
    rows = []
    cwd = REPO/'common/bin-parser'
    env = dict(os.environ, GOMAXPROCS=str(args.workers), PWD=str(cwd))
    def execute(label, kind):
        stage = stages[label]
        binary = Path(stage['binary'])
        if sha(binary) != stage['binary_sha256']:
            raise ValueError('frozen binary changed')
        name = f'{len(rows)+1:03d}-{label}-{kind}'
        command = [str(binary), '-test.run=^$', '-test.count=1', '-test.timeout=5m',
                   '-test.bench='+args.bench, '-test.benchtime=3s', '-test.benchmem']
        profile = output/(name+'.pprof')
        if kind != 'time':
            command.append(('-test.cpuprofile=' if kind == 'cpu' else '-test.memprofile=')+str(profile))
        raw = output/(name+'.txt')
        started = dt.datetime.now(dt.timezone.utc).isoformat()
        with raw.open('x') as f:
            rc = subprocess.run(command, cwd=cwd, env=env, stdout=f, stderr=subprocess.STDOUT).returncode
        row = {'label': label, 'kind': kind, 'commit': stage['commit'], 'command': command,
               'binary_sha256': stage['binary_sha256'], 'cwd': str(cwd), 'pwd': str(cwd),
               'started_utc': started, 'exit_code': rc, 'raw': raw.name, 'raw_sha256': sha(raw)}
        rows.append(row)
        (output/'runs.json').write_text(json.dumps(rows, indent=2)+'\n')
        print(name, rc, flush=True)
        if rc:
            raise ValueError('failed diagnostic retained')
        if kind != 'time':
            row['profile_sha256'] = sha(profile)
            options = ['-sample_index=alloc_space'] if kind == 'alloc' else []
            for view in ['flat', 'cum']:
                with (output/(name+'-'+view+'.txt')).open('x') as f:
                    subprocess.run(['go', 'tool', 'pprof', '-top', '-nodecount=40',
                        *(['-cum'] if view == 'cum' else []), *options, str(binary), str(profile)],
                        stdout=f, stderr=subprocess.STDOUT, check=True)
            (output/'runs.json').write_text(json.dumps(rows, indent=2)+'\n')
    if not args.profiles_only:
        for label in stages:
            execute(label, 'time')
        execute('original', 'time')
    for label in ['original', frozen['stages'][-1]['label']]:
        # CPU and allocation profiles use separate processes after all timings.
        for kind in ['cpu', 'alloc']:
            execute(label, kind)


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--artifacts', required=True, type=Path)
    parser.add_argument('--tag', required=True)
    parser.add_argument('--bench', required=True)
    parser.add_argument('--workers', type=int, choices=[1, 10], default=1)
    parser.add_argument('--profiles-only', action='store_true')
    args = parser.parse_args()
    args.artifacts = args.artifacts.resolve()
    run(args)
