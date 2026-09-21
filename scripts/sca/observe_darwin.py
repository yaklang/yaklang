#!/usr/bin/env python3
"""Observe the libc attempts used by CGO-disabled Go on Darwin, without root.

This is a development observer, not a kernel-wide tracer. It calibrates known
read/stat/write/socket/connect/fork/execve attempts in the same sandbox first.
Direct raw syscalls and APIs outside the listed boundaries are not observed;
combine this evidence with source_audit.go and dependency_audit.py, which must
exclude acquisition/unsafe/syscall code from the production SCA closure.
"""
import argparse
from collections import Counter
import hashlib
import json
import os
from pathlib import Path
import platform
import subprocess

ROOT = Path(__file__).resolve().parents[2]
SOURCE = ROOT / 'scripts/sca/audit-darwin'
NETWORK = {'socket', 'connect', 'bind', 'listen', 'sendto'}
PROCESS = {'fork', 'execve'}


def events(text):
    out = []
    for line in text.splitlines():
        if not line.startswith('SCA_AUDIT\t'):
            continue
        _, kind, path = line.split('\t')
        out.append((kind, bytes.fromhex(path).decode('utf-8', errors='strict')))
    return out


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--out', type=Path, required=True, help='module-external evidence directory')
    parser.add_argument('--go', default='go')
    args = parser.parse_args()
    if platform.system() != 'Darwin':
        parser.error('Darwin observer; use a native syscall tracer on other systems')
    out = args.out.resolve()
    if out == ROOT or ROOT in out.parents:
        parser.error('evidence must be outside the checkout')
    out.mkdir(parents=True, exist_ok=True)
    env = dict(os.environ, CGO_ENABLED='0', GOWORK='off', GOPROXY='off', GOSUMDB='off')
    env.pop('DYLD_INSERT_LIBRARIES', None)
    def run(command, cwd=ROOT):
        subprocess.run(list(map(str, command)), cwd=cwd, env=env, check=True)
    library, launch, control, target = [out / n for n in ('observer.dylib', 'launch', 'control', 'sca.test')]
    run(['clang', '-dynamiclib', '-o', library, SOURCE / 'interpose.c'])
    run(['clang', '-o', launch, SOURCE / 'launch.c'])
    run([args.go, 'build', '-o', control, SOURCE / 'control.go'])
    run([args.go, 'test', '-c', '-o', target, './common/sca'])
    profile = '(version 1)(allow default)(deny network*)(deny file-write*)(deny process-fork)'
    def observed(binary, name, flags=()):
        result = subprocess.run(['/usr/bin/sandbox-exec', '-p', profile, str(launch), str(library), str(binary), *flags], cwd=ROOT / 'common/sca', env=env, capture_output=True, text=True)
        (out / (name + '.log')).write_text(result.stdout + result.stderr)
        if result.returncode:
            raise RuntimeError(f'{name} exited {result.returncode}; see {out / (name + ".log")}')
        return result, events(result.stderr)
    calibrated, controls = observed(control, 'control')
    counts = Counter(k for k, _ in controls)
    required = {'loaded', 'read-open', 'stat', 'lstat', 'readlink', 'write-open', 'write-mkdir', 'socket', 'connect', 'fork', 'execve'}
    if not required <= counts.keys() or 'CONTROLS_PASSED' not in calibrated.stdout:
        raise RuntimeError('observer missed a positive control: ' + repr(required - counts.keys()))
    tested, seen = observed(target, 'frozen-formats', ['-test.run=^(TestFourFormatFullFieldContract|TestRPMAllFrozenFields|TestMemoryFilesystemNoTemp)$', '-test.v', '-test.count=1'])
    counts = Counter(k for k, _ in seen)
    if counts['loaded'] != 1 or '--- PASS: TestFourFormatFullFieldContract' not in tested.stdout or '--- PASS: TestRPMAllFrozenFields' not in tested.stdout:
        raise RuntimeError('observer not loaded or expected tests did not execute')
    outside = []
    for kind, path in seen:
        if kind in {'read-openat', 'fstatat'} and path and not path.startswith('/'):
            outside.append((kind, path))
            continue
        if not path or kind.startswith('write-') or kind in PROCESS:
            continue
        full = Path(path) if path.startswith('/') else ROOT / 'common/sca' / path
        # Avoid resolving symlinks here: the observed input path is the evidence.
        normalized = Path(os.path.abspath(full))
        if normalized != ROOT and ROOT not in normalized.parents and str(normalized) != '/dev/urandom':
            outside.append((kind, path))
    result = dict(head=subprocess.check_output(['git', 'rev-parse', 'HEAD'], cwd=ROOT, text=True).strip(), platform=platform.platform(), controls=dict(Counter(k for k, _ in controls)), attempts=dict(counts), network_attempts=sum(counts[k] for k in NETWORK), process_attempts=sum(counts[k] for k in PROCESS), write_attempts=sum(n for k, n in counts.items() if k.startswith('write-')), outside_input_paths=outside, coverage='Darwin libc boundaries used by pinned Go; not raw-syscall/kernel-wide tracing', tests=['TestFourFormatFullFieldContract', 'TestRPMAllFrozenFields', 'TestMemoryFilesystemNoTemp'])
    result['working_tree_dirty'] = bool(subprocess.check_output(['git', 'status', '--porcelain'], cwd=ROOT, text=True).strip())
    result['test_binary_sha256'] = hashlib.sha256(target.read_bytes()).hexdigest()
    result['toolchain'] = subprocess.check_output([args.go, 'version'], text=True).strip()
    result['passed'] = not (result['network_attempts'] or result['process_attempts'] or result['write_attempts'] or outside)
    (out / 'result.json').write_text(json.dumps(result, indent=2) + '\n')
    print(json.dumps(result, indent=2))
    if not result['passed']:
        raise SystemExit(1)


if __name__ == '__main__':
    main()
