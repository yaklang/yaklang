#!/usr/bin/env python3
"""Validate required package-qualified tests against actual go test -json events."""
from __future__ import annotations
import argparse
import json
import sys
from pathlib import Path


def check(text, required):
    seen, passed, package_passed, complete = set(), set(), set(), set()
    active, current_seen, current_passed = set(), {}, {}
    failed, skipped, malformed = [], [], []
    for number, line in enumerate(text.splitlines(), 1):
        if not line.strip():
            continue
        try:
            event = json.loads(line)
            if not isinstance(event, dict):
                raise ValueError("not an event")
        except (json.JSONDecodeError, ValueError):
            malformed.append(number)
            continue
        package, name, action = event.get('Package', ''), event.get('Test', ''), event.get('Action', '')
        key = (package, name)
        # Concatenated fuzz logs contain multiple invocations of the same
        # package. An earlier terminal pass cannot complete a later short log.
        if action == 'start' or action == 'run' and package not in active:
            active.add(package)
            current_seen[package], current_passed[package] = set(), set()
        if action == 'run' and name:
            seen.add(key)
            current_seen[package].add(name)
        if action == 'pass':
            if name:
                passed.add(key)
                current_passed.setdefault(package, set()).add(name)
            else:
                package_passed.add(package)
                complete.update((package, t) for t in current_seen.get(package, set()) & current_passed.get(package, set()))
                active.discard(package)
        if action == 'fail':
            failed.append({'package': package, 'test': name or '<package>'})
        if action == 'skip' and any((p is None or p == package) and
                                  (name == t or name.startswith(t + '/')) for p, t in required):
            skipped.append({'package': package, 'test': name or '<package>'})
        if action in ('skip', 'fail') and not name:
            active.discard(package)
    def matches(items, package, test):
        return any(t == test and (package is None or p == package) for p, t in items)
    missing = [{'package': p, 'test': t} for p, t in required if not matches(seen, p, t)]
    unpassed = [{'package': p, 'test': t} for p, t in required if not matches(complete, p, t)]
    required_packages = {p for p, _ in required if p is not None}
    required_packages.update(p for p, t in seen if any(rp is None and rt == t for rp, rt in required))
    packages_unpassed = sorted((required_packages - package_passed) | (required_packages & active))
    ok = bool(required) and not (missing or unpassed or packages_unpassed or failed or skipped or malformed)
    return {'ok': ok, 'required_count': len(required), 'seen_tests': len(seen),
            'passed_tests': len(passed), 'missing': missing, 'unpassed': unpassed,
            'packages_unpassed': packages_unpassed, 'required_skipped': skipped,
            'failures': failed, 'non_json_lines': malformed}


def inventory(path, tier):
    entries = json.loads(path.read_text(encoding='utf-8'))['tests']
    selected = [(e['package'], e['test']) for e in entries if tier in e['tiers']]
    if not selected or len(set(selected)) != len(selected):
        raise ValueError('inventory tier must contain unique nonempty package/test pairs')
    if any(not p or not t.startswith(('Test', 'Fuzz')) for p, t in selected):
        raise ValueError('invalid required package/test pair')
    return selected


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('log', type=Path)
    parser.add_argument('--require', action='append', default=[], help='Legacy unqualified test; --inventory is preferred')
    parser.add_argument('--inventory', type=Path)
    parser.add_argument('--tier', choices=['quick', 'full', 'race', 'sustained'], default='full')
    args = parser.parse_args()
    try:
        required = [(None, test) for test in args.require]
        if args.inventory:
            required += inventory(args.inventory, args.tier)
        if not required:
            raise ValueError('provide --inventory or --require; an empty run cannot pass')
        report = check(args.log.read_text(encoding='utf-8'), required)
    except (OSError, ValueError, KeyError, TypeError) as exc:
        parser.error(str(exc))
    print(json.dumps(report, ensure_ascii=False, indent=2))
    return 0 if report['ok'] else 1


if __name__ == '__main__':
    sys.exit(main())
