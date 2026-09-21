#!/usr/bin/env python3
"""Reject empty/missing/skipped required Go tests; consume go test -json logs."""
from __future__ import annotations
import argparse
import json
import sys
from pathlib import Path

def main() -> int:
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument('log', type=Path)
    p.add_argument('--require', action='append', required=True,
                   help='Exact top-level test name; repeat for multiple required tests')
    args = p.parse_args()
    try:
        text = args.log.read_text(encoding='utf-8')
    except OSError as exc:
        p.error(str(exc))
    seen, passed, failures, skipped = set(), set(), [], []
    malformed = []
    for lineno, line in enumerate(text.splitlines(), 1):
        if not line.strip():
            continue
        try:
            e = json.loads(line)
        except json.JSONDecodeError:
            malformed.append(lineno)
            continue
        name, action = e.get('Test', ''), e.get('Action', '')
        relevant = any(name == r or name.startswith(r + '/') for r in args.require)
        if action == 'run' and name:
            seen.add(name)
        if action == 'pass' and name:
            passed.add(name)
        if action == 'fail':
            failures.append({'package': e.get('Package', ''), 'test': name or '<package>'})
        if action == 'skip' and relevant:
            skipped.append(name)
    missing = sorted(set(args.require) - seen)
    unpassed = sorted(set(args.require) - passed)
    ok = not (missing or unpassed or failures or skipped or malformed)
    print(json.dumps({'ok': ok, 'required': args.require, 'seen_tests': len(seen),
                      'missing': missing, 'unpassed': unpassed,
                      'required_skipped': skipped, 'failures': failures,
                      'non_json_lines': malformed}, ensure_ascii=False, indent=2))
    return 0 if ok else 1

if __name__ == '__main__':
    sys.exit(main())
