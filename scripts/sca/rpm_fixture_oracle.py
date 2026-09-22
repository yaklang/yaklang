#!/usr/bin/env python3
"""Reproduce the frozen RPM field oracle with Python's read-only SQLite engine.

This producer is not invoked by Go tests or production SCA. It never imports
our RPM reader. Tag numbers/layout follow RPM header tags (go-rpmdb v0.1.0
pkg/rpmtags.go; require/provide flags and versions are RPM tags 1048/1050 and
1112/1113). The input snapshot, rather than ScanReport output, is the oracle.
"""
import argparse
import hashlib
import json
import sqlite3
import struct
from pathlib import Path


def header(blob):
    count, length = struct.unpack_from('>II', blob)
    start = 8 + 16 * count
    assert start + length == len(blob)
    store = blob[start:]
    result = {}
    for i in range(count):
        tag, kind, offset, size = struct.unpack_from('>IIII', blob, 8 + 16 * i)
        assert tag not in result and offset <= length
        if kind in (6, 8, 9):
            parts = []
            for _ in range(size):
                end = store.index(b'\0', offset)
                parts.append(store[offset:end].decode('utf-8'))
                offset = end + 1
            result[tag] = parts
        elif kind == 4:
            assert offset + 4 * size <= length
            result[tag] = list(struct.unpack_from('>' + 'I' * size, store, offset))
        elif kind == 7:
            assert offset + size <= length
            result[tag] = store[offset:offset + size].hex()
    return result


def package(h):
    def single(tag, default=''):
        v = h.get(tag, [default])
        assert len(v) == 1
        return v[0]

    def deps(names, flags, versions):
        n, f, v = h.get(names, []), h.get(flags, []), h.get(versions, [])
        assert len(n) == len(f) == len(v)
        return [dict(name=a, flags=b, version=c) for a, b, c in zip(n, f, v)]

    return dict(name=single(1000), version=single(1001), release=single(1002),
                epoch=single(1003, 0), architecture=single(1022),
                license=single(1014), md5=h.get(261, ''),
                provides=deps(1047, 1112, 1113), requires=deps(1049, 1048, 1050))


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('snapshot', type=Path)
    parser.add_argument('output', type=Path)
    args = parser.parse_args()
    with sqlite3.connect(args.snapshot.resolve().as_uri() + '?mode=ro&immutable=1', uri=True) as db:
        packages = [package(header(row[0])) for row in db.execute('SELECT blob FROM Packages ORDER BY hnum')]
    packages.sort(key=lambda p: (p['name'], p['version'], p['architecture'], p['release']))
    output = dict(source_sha256=hashlib.sha256(args.snapshot.read_bytes()).hexdigest(), packages=packages)
    args.output.write_text(json.dumps(output, ensure_ascii=False, indent=2) + '\n')


if __name__ == '__main__':
    main()
