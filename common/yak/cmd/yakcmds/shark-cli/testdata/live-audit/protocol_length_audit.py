"""Compare complete native/CLI exports around a default PDU length correction.

Inputs must be ordered replays of the same original capture with identical
options. Only a zero pdu.length with nonempty native Raw may change, to that
exact observed byte count. Every other JSON value, row and association must
remain identical. Explicit nonzero lengths (including omitted/preview bodies)
are preserved. This is a bounded-metadata differential, not a new oracle for
protocol semantics, reported flow counts or capture completeness.
"""
import base64
import collections
import hashlib
import itertools
import json
from pathlib import Path
import sys


def sha(path):
    digest = hashlib.sha256()
    with path.open('rb') as source:
        for chunk in iter(lambda: source.read(1 << 20), b''):
            digest.update(chunk)
    return digest.hexdigest()


def rows(path):
    with path.open(encoding='utf-8-sig') as source:
        for line in source:
            if line.startswith('{'):
                yield json.loads(line)


def length_value(pdu):
    values = [v for v in pdu['field_values']['values'] if v['name'] == 'pdu.length']
    assert len(values) == 1, ('length occurrence count', pdu['id'])
    v = values[0]
    assert v['origin'] == 'event' and 'pdu_range' not in v
    assert isinstance(v['value'], int) and not isinstance(v['value'], bool)
    assert str(v['value']) == v['exact_value']
    return v['value']


def normalized(value):
    if isinstance(value, list):
        return [normalized(v) for v in value]
    if not isinstance(value, dict):
        return value
    result = {k: normalized(v) for k, v in value.items()}
    if result.get('name') == 'pdu.length':
        result['value'], result['exact_value'] = 0, '0'
    if 'pdu.length' in result:
        result['pdu.length'] = [0]
    return result


def compare(native_before, native_after, cli_before, cli_after):
    changed = {}
    expected_cli = {}
    native_counts, cli_counts = collections.Counter(), collections.Counter()
    native_total, cli_total = 0, 0
    for before, after in itertools.zip_longest(rows(native_before), rows(native_after)):
        assert before is not None and after is not None, 'native row count changed'
        native_total += 1
        old, new = length_value(before), length_value(after)
        observed = len(base64.b64decode(before.get('raw') or '', validate=True))
        assert len(base64.b64decode(after.get('raw') or '', validate=True)) == observed
        if old == 0 and observed:
            assert new == observed, ('default length not repaired', before['id'], observed, new)
            changed[before['id']] = {'protocol': before['protocol'], 'length': observed}
            native_counts[before['protocol']] += 1
        else:
            assert new == old, ('explicit original length changed', before['id'], old, new)
        expected_cli[before['id']] = new
        assert normalized(before) == normalized(after), ('unrelated native value changed', before['id'])
    for before, after in itertools.zip_longest(rows(cli_before), rows(cli_after)):
        assert before is not None and after is not None, 'CLI frame count changed'
        cli_total += 1
        assert normalized(before) == normalized(after), ('unrelated CLI value changed', before['number'])
        for old_pdu, new_pdu in itertools.zip_longest(before.get('pdus') or [], after.get('pdus') or []):
            assert old_pdu is not None and new_pdu is not None
            ident = old_pdu['id']
            if ident == 0:  # An explicitly bounded observation placeholder.
                continue
            if ident not in expected_cli:
                # A CLI-only transport observation is still required to be
                # byte-for-byte equivalent, including every field length.
                assert old_pdu == new_pdu, ('non-native observation changed', before['number'], ident)
                continue
            old, new = length_value(old_pdu), length_value(new_pdu)
            assert new == expected_cli[ident], ('CLI/native disagreement', before['number'], ident, new)
            assert old_pdu['fields']['pdu.length'] == [old]
            assert new_pdu['fields']['pdu.length'] == [new]
            if old != new:
                assert ident in changed
                cli_counts[old_pdu['protocol']] += 1
    return {'native_rows': native_total, 'physical_CLI_frames': cli_total,
            'corrected_native_lengths': dict(native_counts),
            'corrected_CLI_PDU_occurrences': dict(cli_counts),
            'corrected_native_total': len(changed),
            'unrelated_differences': 0,
            'scope': __doc__}


if __name__ == '__main__':
    if len(sys.argv) != 6:
        raise SystemExit('usage: protocol_length_audit.py BEFORE_SDK AFTER_SDK BEFORE_CLI AFTER_CLI OUT')
    inputs = list(map(Path, sys.argv[1:5]))
    result = compare(*inputs)
    result['input_sha256'] = {str(p): sha(p) for p in inputs}
    result['script_sha256'] = sha(Path(__file__))
    Path(sys.argv[5]).write_bytes((json.dumps(result, indent=2, sort_keys=True)+'\n').encode())
    print(json.dumps({k:v for k,v in result.items() if k not in {'scope','input_sha256'}}, sort_keys=True))
