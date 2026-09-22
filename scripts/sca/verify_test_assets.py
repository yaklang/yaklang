#!/usr/bin/env python3
"""Prove that ZIP test corpora and their helper are absent from yak.go's build."""
import argparse
import json
import os
from pathlib import Path
import subprocess
from fixture_store import FixtureStore
from dependency_audit import decode_stream

ROOT = Path(__file__).resolve().parents[2]

def main():
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument('--binary', type=Path, help='optional actual yak.go executable to inspect')
    args = ap.parse_args()
    store = FixtureStore()
    store.verify()
    archives = {str((store.root / row['path']).resolve()) for row in store.manifest['archives']}
    env = dict(os.environ, GOWORK='off')
    raw = subprocess.check_output(['go', 'list', '-mod=readonly', '-deps', '-json', './common/yak/cmd/yak.go'], cwd=ROOT, env=env, text=True, encoding='utf-8')
    packages = decode_stream(raw)
    leaks = []
    for pkg in packages:
        if pkg.get('Error') or pkg.get('DepsErrors'):
            raise ValueError('invalid production dependency graph')
        if '/common/sca/internal/testcheck' in pkg['ImportPath']:
            leaks.append(pkg['ImportPath'])
        for name in pkg.get('EmbedFiles', []):
            full = str((Path(pkg['Dir']) / name).resolve())
            if full in archives:
                leaks.append(full)
    # Prevent future accidental non-test embeds even outside yak.go's closure.
    for file in store.root.rglob('*.go'):
        if not file.name.endswith('_test.go') and '//go:embed' in file.read_text():
            leaks.append(str(file))
    if args.binary:
        data = args.binary.read_bytes()
        for marker in [b'yaklang-sca-test-corpus/v1:'] + [Path(x).name.encode() for x in archives]:
            if marker in data:
                leaks.append('binary contains ' + marker.decode())
    print(json.dumps({'passed': not leaks, 'archives': len(archives), 'production_asset_leaks': leaks, 'binary_checked': bool(args.binary)}, indent=2))
    return bool(leaks)

if __name__ == '__main__':
    raise SystemExit(main())
