"""Independent literal expectations for the CLI endpoint metadata selector.

These are in-memory metadata adversarial controls, not captured traffic or new
engine executions. Expected results below are explicit; they are not generated
by the selector under test. SDK JSON integer fields correspond to the worker's
UseNumber decoder. An invalid specialized GPE observation must fall back to the
generic refusal contract, while invalid header-name lists fail atomically.
The actual-input source proof and whole-row audit remain separate dependencies.
"""
import argparse
import copy
import hashlib
import importlib.util
import json
import sys
from pathlib import Path


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def exact(actual, expected, name):
    assert type(actual) is type(expected), (name, type(actual).__name__, type(expected).__name__)
    if isinstance(expected, dict):
        assert actual.keys() == expected.keys(), (name, actual, expected)
        for key, value in expected.items():
            exact(actual[key], value, name + '/' + key)
    elif isinstance(expected, list):
        assert len(actual) == len(expected), (name, actual, expected)
        for a, e in zip(actual, expected):
            exact(a, e, name)
    else:
        assert actual == expected, (name, actual, expected)


def audit(auditor, expected_sha, events):
    assert sha(auditor) == expected_sha
    sys.path.insert(0, str(auditor.parent))
    spec = importlib.util.spec_from_file_location('endpoint_selector_under_review', auditor)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    native = [json.loads(line) for line in events.open(encoding='utf-8-sig')]
    base = next(e for e in native if e['protocol'] == 'ip' and e['profile'] == 'network-encapsulation-refusal'
                and e['expert'] == 'VXLANProfileUnselected'
                and e['session'].get('Unselected Carrier') == 'VXLAN-GPE')
    assert base['session']['Observed Header Layers'] == ['Ethernet', 'IPv4']
    assert base['session']['VXLAN-GPE Flags'] == 12 and base['session']['VXLAN-GPE Next Protocol'] == 3
    checks = []
    selected = {'headers': ['Ethernet', 'IPv4'], 'native_headers': False}
    generic = {'headers': ['Ethernet', 'IPv4'], 'native_headers': True}

    def check(name, mutation, expected):
        event = copy.deepcopy(base)
        mutation(event)
        actual = module.contract(event)
        exact(actual, expected, name)
        checks.append({'name': name, 'passed': True, 'expected': expected,
                       'metadata_SHA256': hashlib.sha256(json.dumps(event, sort_keys=True,
                                                                   separators=(',', ':')).encode()).hexdigest()})

    def session(key, value):
        return lambda e: e['session'].__setitem__(key, value)

    check('actual complete GPE header selects its specialized header contract', lambda e: None, selected)
    # Neither Error nor Header Observation Scope is an additional condition in
    # validVXLANGPEHeaderObservation; adding either would invent a new policy.
    check('diagnostic Error alone does not invalidate complete GPE header evidence', lambda e: e.update(error='body profile diagnostic'), selected)
    check('scope text is not an invented additional GPE selector condition', session('Header Observation Scope', 'diagnostic text'), selected)
    check('missing scope text does not change the declared GPE selector', lambda e: e['session'].pop('Header Observation Scope'), selected)
    check('receive-ignored high reserved bit is still an eight-bit header', session('VXLAN-GPE Flags', 140), selected)
    check('maximum literal VNI is valid', session('VXLAN-GPE VNI', 0xffffff), selected)
    def no_next(e):
        e['session'].update({'VXLAN-GPE Flags': 8, 'VXLAN-GPE Next Protocol': 0,
                             'VXLAN-GPE Next Protocol Present': False})
    check('legacy NP-absent header is concrete GPE without selecting a child', no_next, selected)

    for label, key, value in [
        ('flags above wire byte', 'VXLAN-GPE Flags', 268),
        ('negative flags', 'VXLAN-GPE Flags', -1),
        ('floating-point JSON flags', 'VXLAN-GPE Flags', 12.0),
        ('string flags', 'VXLAN-GPE Flags', '12'),
        ('version boolean false', 'VXLAN-GPE Version', False),
        ('floating-point JSON version', 'VXLAN-GPE Version', 0.0),
        ('nonzero version', 'VXLAN-GPE Version', 1),
        ('wrong carrier', 'Unselected Carrier', 'VXLAN'),
        ('negative VNI', 'VXLAN-GPE VNI', -1),
        ('VNI above wire 24 bits', 'VXLAN-GPE VNI', 0x1000000),
        ('boolean VNI', 'VXLAN-GPE VNI', True),
        ('floating-point JSON VNI', 'VXLAN-GPE VNI', 42.0),
        ('next protocol above wire byte', 'VXLAN-GPE Next Protocol', 256),
        ('zero next protocol with P bit present', 'VXLAN-GPE Next Protocol', 0),
        ('P flag does not match observed next protocol', 'VXLAN-GPE Next Protocol Present', False),
        ('present flag must be boolean', 'VXLAN-GPE Next Protocol Present', 1),
    ]:
        check('invalid specialized GPE falls back: ' + label, session(key, value), generic)
    check('missing carrier cannot select GPE', lambda e: e['session'].pop('Unselected Carrier'), generic)
    check('missing flags cannot select GPE', lambda e: e['session'].pop('VXLAN-GPE Flags'), generic)
    check('missing version cannot select GPE', lambda e: e['session'].pop('VXLAN-GPE Version'), generic)
    check('generic refusal with non-IP-ending names uses IP fallback',
          lambda e: e['session'].update({'Unselected Carrier': 'unknown', 'Observed Header Layers': ['Ethernet', 'IPv4', 'UDP']}),
          {'headers': ['IP'], 'native_headers': True})
    check('wrong profile has no endpoint-refusal contract', lambda e: e.update(profile='unrelated'), None)
    check('decoded status is not a limited GPE refusal', lambda e: e.update(status='decoded'), None)

    invalid_names = [
        ('empty list', []),
        ('string instead of list', 'Ethernet'),
        ('non-string entry', ['Ethernet', 'IPv4', 4, 'IPv4']),
        ('empty intermediate entry', ['Ethernet', 'IPv4', '', 'IPv4']),
        ('too many entries', ['Ethernet', 'IPv4'] + ['UDP'] * 126 + ['IPv4']),
        ('129-byte name', ['Ethernet', 'IPv4', 'x' * 129, 'IPv4']),
        ('40 characters but 160 UTF8 bytes', ['Ethernet', 'IPv4', '\U0001f600' * 40, 'IPv4']),
        ('Cc escape control', ['Ethernet', 'IPv4', '\x1b', 'IPv4']),
        ('Cc tab control', ['Ethernet', 'IPv4', '\t', 'IPv4']),
        ('Cf format control', ['Ethernet', 'IPv4', '\u200d', 'IPv4']),
    ]
    for label, value in invalid_names:
        exact(module.names(value), [], 'names atomically invalid: ' + label)
        check('valid GPE with invalid whole names list: ' + label,
              session('Observed Header Layers', value), {'headers': [], 'native_headers': False})
        check('generic refusal with invalid whole names list: ' + label,
              lambda e, value=value: e['session'].update({'Unselected Carrier': 'unknown', 'Observed Header Layers': value}),
              {'headers': ['IP'], 'native_headers': True})
    valid_boundary = ['Ethernet', 'IPv4', '\U0001f600' * 32]
    exact(module.names(valid_boundary), valid_boundary, '128 UTF8-byte safe name boundary')
    valid_count = ['Ethernet'] + ['IPv4'] * 127
    exact(module.names(valid_count), valid_count, '128 header count boundary')
    checks.append({'name': '128-byte UTF8 and 128-entry bounds remain inclusive', 'passed': True})
    assert sha(auditor) == expected_sha
    return {'scope': __doc__, 'auditor_SHA256': expected_sha, 'review_driver_SHA256': sha(Path(__file__)),
            'SDK_fixture_SHA256': sha(events), 'explicit_selector_controls': len(checks),
            'all_literal_selector_expectations_passed': True, 'controls': checks,
            'actual_engine_executions_in_this_run': 0,
            'source_or_Go_or_shared_auditor_modified': False}


if __name__ == '__main__':
    ap = argparse.ArgumentParser(description=__doc__)
    for name in ['auditor', 'events', 'out']:
        ap.add_argument('--' + name, required=True, type=Path)
    ap.add_argument('--expected-auditor-sha', required=True)
    a = ap.parse_args()
    proof = audit(a.auditor, a.expected_auditor_sha, a.events)
    with a.out.open('x', encoding='utf8') as f:
        json.dump(proof, f, indent=2)
        f.write('\n')
    print(json.dumps({'controls': proof['explicit_selector_controls'], 'all_passed': True,
                      'proof_SHA256': sha(a.out)}), flush=True)
