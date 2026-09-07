#!/usr/bin/env python3
"""Verify raw evidence and compare paired windows without dropping size suffixes."""
import argparse
import hashlib
import json
from pathlib import Path
import re
import statistics


def load_rows(directory):
    rows = [json.loads(line) for line in (directory/'runs.jsonl').read_text().splitlines()]
    for row in rows:
        raw = (directory/row['raw']).read_bytes()
        if hashlib.sha256(raw).hexdigest() != row['raw_sha256'] or row['exit_code'] != 0:
            raise ValueError('changed or failed raw evidence: '+row['raw'])
        # Reparse original benchmark output. Early timing harness versions
        # incorrectly stripped numeric workload sizes when the CPU count was 1.
        row['measurements'] = []
        for line in raw.decode().splitlines():
            match = re.match(r'^(Benchmark\S+)\s+(\d+)\s+(.*)$', line)
            if not match:
                continue
            name = match.group(1)
            if row['workers'] > 1:
                name = re.sub(r'-'+str(row['workers'])+r'$', '', name)
            values = match.group(3).split()
            row['measurements'].append({'name': name, 'iterations': int(match.group(2)),
                'metrics': {values[i+1]: float(values[i]) for i in range(0, len(values)-1, 2)}})
    return rows


def benchmarks(directory):
    rows = load_rows(directory)
    pairs = {}
    for row in rows:
        for m in row['measurements']:
            key = (row['pair'], m['name'])
            group = pairs.setdefault(key, {})
            if row['label'] in group:
                raise ValueError('duplicate label in a pair')
            group[row['label']] = (row, m['metrics'])
    groups = {}
    for (_, name), pair in pairs.items():
        if len(pair) != 2 or 'original' not in pair:
            raise ValueError('unpaired workload: '+name)
        label = next(x for x in pair if x != 'original')
        row, metrics = pair[label]
        key = (label, row['workers'], name)
        groups.setdefault(key, []).append((pair['original'][1], metrics))
    result = []
    for (label, workers, name), samples in groups.items():
        if len(samples) < 5:
            raise ValueError('fewer than five windows: '+name)
        median = lambda side: {k: statistics.median(s[side][k] for s in samples) for k in samples[0][side]}
        original, candidate = median(0), median(1)
        result.append({'label': label, 'workers': workers, 'name': name, 'samples': len(samples),
            'original': original, 'candidate': candidate,
            'speedup': original['ns/op']/candidate['ns/op'],
            'change_percent': (candidate['ns/op']/original['ns/op']-1)*100,
            'original_samples_ns': [s[0]['ns/op'] for s in samples],
            'candidate_samples_ns': [s[1]['ns/op'] for s in samples]})
    return result


def digests(directory):
    rows = load_rows(directory)
    reference, result = None, []
    for row in rows:
        text = (directory/row['raw']).read_text()
        partitions = {}
        for workload, worker, count, sha in re.findall(
                r'CURRENT_EXPORT_DIGEST workload=(\S+) worker=(\d+) records=(\d+) sha256=([a-f0-9]+)', text):
            key = workload+'/'+worker
            if key in partitions:
                raise ValueError('duplicate partition')
            partitions[key] = {'records': int(count), 'sha256': sha}
        if set(partitions) != {w+'/'+str(i) for w in ['application', 'envelopes'] for i in range(4)}:
            raise ValueError('missing digest partition')
        for workload, expected in [('application', 11), ('envelopes', 58533)]:
            if sum(v['records'] for k, v in partitions.items() if k.startswith(workload+'/')) != expected:
                raise ValueError('workload count changed')
        if reference is None:
            reference = partitions
        if partitions != reference:
            raise ValueError('digest mismatch: '+row['label'])
        result.append({'label': row['label'], 'commit': row['commit'], 'partitions': partitions})
    return result


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('mode', choices=['benchmarks', 'digests'])
    parser.add_argument('directory', type=Path)
    args = parser.parse_args()
    result = benchmarks(args.directory) if args.mode == 'benchmarks' else digests(args.directory)
    print(json.dumps(result, indent=2))
