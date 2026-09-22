#!/usr/bin/env python3
"""Read/version/repack SCA ZIP corpora without extracting files into the checkout."""
import argparse
from collections import defaultdict
import hashlib
import io
import json
from pathlib import Path, PurePosixPath
import zipfile

SCA = Path(__file__).resolve().parents[2] / 'common/sca'


def valid_path(value):
    return bool(value) and '\\' not in value and not value.startswith('/') and all(p not in ('', '.', '..') for p in value.split('/'))


def sha(data):
    return hashlib.sha256(data).hexdigest()


class FixtureStore:
    def __init__(self, root=SCA):
        self.root = Path(root)
        self.manifest = json.loads((self.root / 'testdata/manifest.json').read_text())
        self.rows = {r['path']: r for r in self.manifest['fixtures']}
        if len(self.rows) != len(self.manifest['fixtures']):
            raise ValueError('duplicate logical fixture path')
        self.archives = {}

    def archive(self, name):
        if not valid_path(name):
            raise ValueError('invalid archive path: ' + name)
        if name not in self.archives:
            z = zipfile.ZipFile(io.BytesIO((self.root / name).read_bytes()))
            names = z.namelist()
            if len(names) != len(set(names)) or not all(valid_path(n) for n in names):
                raise ValueError('invalid/duplicate ZIP members: ' + name)
            self.archives[name] = z
        return self.archives[name]

    def read(self, logical):
        row = self.rows[logical]
        archive, member = row['archive'], row['member']
        if not valid_path(member) or str(PurePosixPath(archive).parent / member) != logical:
            raise ValueError('ZIP member changes logical path: ' + logical)
        data = self.archive(archive).read(member)  # Verifies CRC in memory.
        if len(data) != row['bytes'] or sha(data) != row['sha256']:
            raise ValueError('missing or stale fixture hash: ' + logical)
        return data

    def verify(self):
        expected = defaultdict(set)
        for logical, row in self.rows.items():
            self.read(logical)
            expected[row['archive']].add(row['member'])
        tracked = {r['path']: r for r in self.manifest['archives']}
        if set(tracked) != set(expected):
            raise ValueError('archive manifest differs from fixture mapping')
        for name, members in expected.items():
            if set(self.archive(name).namelist()) != members:
                raise ValueError('unmapped ZIP entries: ' + name)
            data = (self.root / name).read_bytes()
            if sha(data) != tracked[name]['sha256'] or len(data) != tracked[name]['bytes']:
                raise ValueError('stale archive hash: ' + name)
        actual = {p.relative_to(self.root).as_posix() for p in self.root.rglob('*.zip') if 'testdata' in p.parts}
        if actual != set(expected):
            raise ValueError('unmapped fixture archives: ' + repr(actual ^ set(expected)))


def pack(rows, source):
    buf = io.BytesIO()
    name = rows[0]['archive']
    with zipfile.ZipFile(buf, 'w', compression=zipfile.ZIP_DEFLATED, compresslevel=9) as z:
        z.comment = ('yaklang-sca-test-corpus/v1:' + name).encode()
        for row in sorted(rows, key=lambda r: r['member']):
            data = source(row)
            if sha(data) != row['sha256'] or len(data) != row['bytes']:
                raise ValueError('input differs from pinned manifest: ' + row['path'])
            info = zipfile.ZipInfo(row['member'], (1980, 1, 1, 0, 0, 0))
            info.create_system = 3
            info.external_attr = (0o100000 | row['mode']) << 16
            z.writestr(info, data, compress_type=zipfile.ZIP_DEFLATED, compresslevel=9)
    return buf.getvalue()


def main():
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument('--source-root', type=Path, help='expanded common/sca tree for deterministic repack; hashes must match manifest')
    ap.add_argument('--check-reproducible', action='store_true', help='repack in memory and require identical ZIP bytes')
    args = ap.parse_args()
    store = FixtureStore()
    groups = defaultdict(list)
    for row in store.rows.values():
        groups[row['archive']].append(row)
    if args.source_root or args.check_reproducible:
        for name, rows in groups.items():
            read = (lambda row: (args.source_root / row['path']).read_bytes()) if args.source_root else (lambda row: store.read(row['path']))
            data = pack(rows, read)
            if args.check_reproducible and data != (SCA / name).read_bytes():
                raise ValueError('ZIP is not reproducible: ' + name)
            if args.source_root:
                (SCA / name).write_bytes(data)
    store.verify()
    print(json.dumps({'passed': True, 'fixtures': len(store.rows), 'archives': len(groups)}))


if __name__ == '__main__':
    main()
