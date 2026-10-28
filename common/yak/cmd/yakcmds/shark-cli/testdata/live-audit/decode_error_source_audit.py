"""Read-only source/accounting proof; all generated captures and exports stay external.

The three accounting changes are discovered from actual per-source old/new replay.
They are not passed as an expected counter difference.  The optional full-capture
counterfactual changes only those sources' first attempted inner EtherType.
"""
import argparse
import base64
import copy
import hashlib
import json
import os
from pathlib import Path
import struct
import subprocess
import sys
import time


def sha(path):
    h = hashlib.sha256()
    with Path(path).open('rb') as f:
        for b in iter(lambda: f.read(1024 * 1024), b''):
            h.update(b)
    return h.hexdigest()


def save(path, value):
    Path(path).write_text(json.dumps(value, ensure_ascii=False, indent=2, sort_keys=True) + '\n', encoding='utf-8')


def load(path):
    return json.loads(Path(path).read_text(encoding='utf-8-sig'))


def same(a, b):
    if type(a) is not type(b):
        return False
    if isinstance(a, dict):
        return a.keys() == b.keys() and all(same(a[k], b[k]) for k in a)
    if isinstance(a, list):
        return len(a) == len(b) and all(same(x, y) for x, y in zip(a, b))
    return a == b


def whole_delta(before, after, delta):
    expected = copy.deepcopy(before)
    expected['stats']['DecodeErrors'] += delta
    assert same(expected, after), 'unexplained complete flow object value/type difference'


def receipt(path):
    path = Path(path)
    return {'path': str(path.resolve()), 'SHA256': sha(path), 'bytes': path.stat().st_size}


def run(argv, output, binary_source=None):
    env = dict(os.environ, GOMAXPROCS='1', GOMEMLIMIT='1GiB')
    output = Path(output)
    before = time.monotonic()
    with output.open('wb') as f, output.with_suffix(output.suffix + '.stderr').open('wb') as e:
        p = subprocess.run(list(map(str, argv)), stdout=f, stderr=e, env=env, timeout=240)
    r = {'arguments': list(map(str, argv)), 'exit_code': p.returncode,
         'seconds': round(time.monotonic() - before, 3), 'binary': receipt(argv[0]),
         'output': receipt(output), 'stderr': receipt(output.with_suffix(output.suffix + '.stderr'))}
    if binary_source:
        b = load(binary_source)
        assert b['exit_code'] == 0 and b['binary_SHA256'] == r['binary']['SHA256']
        r.update(source=b['source'], binary_build_receipt=receipt(binary_source))
    save(output.with_suffix(output.suffix + '.producer.json'), r)
    assert p.returncode == 0, r
    return r


def source_layout(record):
    """Limited, explicit direct Ethernet/IPv4/EtherIP/inner-Ethernet grammar.

    Describes attempted legacy decoders, not admitted inner network identity.
    Zero IP length is identified separately as legacy compatibility.
    """
    w = record['wire']
    assert record['link'] == 1 and len(w) >= 34 and w[12:14] == b'\x08\x00'
    assert w[14] >> 4 == 4 and (w[14] & 15) == 5 and w[23] == 97
    assert int.from_bytes(w[20:22], 'big') & 0x3fff == 0
    total = int.from_bytes(w[16:18], 'big')
    end = len(w) if total == 0 else min(len(w), 14 + total)
    assert end >= 36
    layers = [{'kind': 'Ethernet', 'offset': 0, 'length': 14},
              {'kind': 'IPv4 fixed header', 'offset': 14, 'length': 20,
               'Protocol': 97, 'declared_total_length': total,
               'extent': 'legacy-zero-length' if total == 0 else ('incomplete-declared-extent' if 14 + total > len(w) else 'complete')}]
    p = 34
    networks = 1
    first_ethertype_offset = None
    for _ in range(8):
        assert p + 2 <= end
        word = int.from_bytes(w[p:p + 2], 'big')
        layers.append({'kind': 'EtherIP observed header; profile unselected', 'offset': p, 'length': 2,
                       'observed_version': word >> 12, 'observed_reserved': word & 4095})
        p += 2
        available = end - p
        if available < 14:
            return {'layers': layers, 'failure': {'decoder': 'Ethernet', 'offset': p, 'available': available, 'required': 14,
                    'error': 'Ethernet packet too small'}, 'partial_network_layers': networks,
                    'first_inner_ethertype_offset': first_ethertype_offset}
        et = int.from_bytes(w[p + 12:p + 14], 'big')
        layers.append({'kind': 'attempted inner Ethernet', 'offset': p, 'length': 14, 'declared_EtherType': et})
        if first_ethertype_offset is None:
            first_ethertype_offset = p + 12
        assert et == 0x0800
        p += 14
        networks += 1  # legacy decoder adds its partial IPv4 layer before returning error
        available = end - p
        if available < 20:
            return {'layers': layers, 'failure': {'decoder': 'IPv4', 'offset': p, 'available': available, 'required': 20,
                    'observed_first_nibble': w[p] >> 4 if available else None,
                    'error': f'Invalid ip4 header. Length {available} less than 20'},
                    'partial_network_layers': networks, 'first_inner_ethertype_offset': first_ethertype_offset}
        version = w[p] >> 4
        if version != 4:
            return {'layers': layers, 'failure': {'decoder': 'IPv4 version', 'offset': p, 'available': available,
                    'observed_version': version, 'expected_version': 4},
                    'partial_network_layers': networks, 'first_inner_ethertype_offset': first_ethertype_offset}
        ihl = (w[p] & 15) * 4
        assert ihl >= 20 and p + ihl <= end and w[p + 9] == 97
        total = int.from_bytes(w[p + 2:p + 4], 'big')
        assert total == 0 or total >= ihl
        end = end if total == 0 else min(end, p + total)
        layers.append({'kind': 'attempted inner IPv4 fixed header', 'offset': p, 'length': ihl,
                       'Protocol': 97, 'declared_total_length': total,
                       'extent': 'legacy-zero-length' if total == 0 else 'bounded'})
        p += ihl
    raise AssertionError('bounded layout did not reach a witnessed failure')


def worker_errors(value):
    out = []
    def walk(v):
        if isinstance(v, dict):
            if isinstance(v.get('Text'), str) and 'Packet decoding error:' in v['Text']:
                out.append(v['Text'])
            for x in v.values(): walk(x)
        elif isinstance(v, list):
            for x in v: walk(x)
    walk(value)
    return out


def experts(value):
    out = []
    def walk(v, p=''):
        if isinstance(v, dict):
            for k, x in v.items():
                if k == '_ws.expert.message': out.append({'path': p + '/' + k, 'message': x})
                walk(x, p + '/' + k)
        elif isinstance(v, list):
            for i, x in enumerate(v): walk(x, p + '/' + str(i))
    walk(value)
    return out


def patch_capture(original, output, mutations):
    """Preserve every PCAPNG block/record coordinate; mutate only explicit wire positions."""
    blob = bytearray(Path(original).read_bytes())
    at = 0
    frame = 0
    endian = None
    changes = []
    while at < len(blob):
        if blob[at:at + 4] == b'\x0a\x0d\x0d\x0a':
            bom = blob[at + 8:at + 12]
            endian = '<' if bom == b'\x4d\x3c\x2b\x1a' else '>'
            assert bom in (b'\x4d\x3c\x2b\x1a', b'\x1a\x2b\x3c\x4d')
        assert endian is not None
        typ, length = struct.unpack_from(endian + 'II', blob, at)
        assert length >= 12 and length % 4 == 0 and at + length <= len(blob)
        assert struct.unpack_from(endian + 'I', blob, at + length - 4)[0] == length
        if typ in (2, 6):
            frame += 1
            if frame in mutations:
                rel = mutations[frame]
                q = at + 28 + rel
                caplen = struct.unpack_from(endian + 'I', blob, at + 20)[0]
                assert rel + 2 <= caplen and blob[q:q + 2] == b'\x08\x00'
                changes.append({'original_frame': frame, 'wire_offset': rel, 'file_offset': q,
                                'before_word': 0x0800, 'after_word': 0xffff})
                blob[q:q + 2] = b'\xff\xff'
        elif typ == 3:
            frame += 1
            assert frame not in mutations, 'simple packet container not supported for mutation'
        at += length
    assert set(mutations) == {x['original_frame'] for x in changes}
    Path(output).write_bytes(blob)
    return changes


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument('--capture', type=Path, required=True)
    p.add_argument('--audit-modules', type=Path, required=True)
    p.add_argument('--six-controls', type=Path, required=True)
    p.add_argument('--before-native', type=Path, required=True)
    p.add_argument('--after-native', type=Path, required=True)
    p.add_argument('--before-bins', type=Path, required=True)
    p.add_argument('--after-bins', type=Path, required=True)
    p.add_argument('--tshark', type=Path, required=True)
    p.add_argument('--out', type=Path, required=True)
    p.add_argument('--counterfactual', action='store_true')
    a = p.parse_args()
    a.out.mkdir(parents=True, exist_ok=False)
    sys.path.insert(0, str(a.audit_modules))
    from vxlan_endpoint_projection_audit import capture_records
    prior = load(a.six_controls / 'summary.json')
    frames = {x['original_frame'] for x in prior['results']}
    source = {i: r for i, r in enumerate(capture_records(a.capture), 1) if i in frames}
    assert set(source) == frames
    facts, changed = [], []
    for row in prior['results']:
        n = row['original_frame']
        root = a.six_controls / str(n)
        r = source[n]
        manifest = load(root / 'manifest.json')
        assert manifest['original_frame'] == n
        assert manifest['original_wire_SHA256'] == hashlib.sha256(r['wire']).hexdigest()
        for k in ('section', 'interface', 'link', 'time_ns', 'original_length', 'captured_length'):
            assert manifest[k] == r[k]
        single = list(capture_records(root / 'observation.pcapng'))
        assert len(single) == 1 and single[0]['wire'] == r['wire']
        for k in ('link', 'time_ns', 'original_length', 'captured_length'):
            assert single[0][k] == r[k]
        layout = source_layout(r)
        small = a.out / str(n)
        small.mkdir()
        values = {}
        for name, bins in (('before', a.before_bins), ('after', a.after_bins)):
            output = small / f'{name}-flows.json'
            run([bins / 'flows.exe', root / 'observation.pcapng'], output, bins / 'flows-receipt.json')
            v = load(output)
            assert all(v[k] is None for k in ('flows', 'packets', 'closed')) and v['error'] == ''
            assert v['stats']['CapturedPackets'] == 1 and v['stats']['CapturedBytes'] == len(r['wire'])
            assert v['stats']['TruncatedCaptures'] == 0
            values[name] = v
        exp = copy.deepcopy(values['before'])
        delta = values['after']['stats']['DecodeErrors'] - values['before']['stats']['DecodeErrors']
        assert delta in (0, 1)
        exp['stats']['DecodeErrors'] += delta
        assert same(exp, values['after'])
        worker = {}
        for suffix, bins in (('09', a.before_bins), ('13', a.after_bins)):
            req = {'Number': n, 'Data': base64.b64encode(r['wire']).decode(), 'Length': r['original_length'], 'Link': r['link'], 'Events': []}
            save(small / f'worker{suffix}.request.json', req)
            argv = [str(bins / 'yak.exe'), 'shark-worker']
            env = dict(os.environ, GOMAXPROCS='1', GOMEMLIMIT='1GiB', YAK_SHARK_WORKER='1')
            proc = subprocess.run(argv, input=json.dumps(req).encode(), stdout=subprocess.PIPE, stderr=subprocess.PIPE, env=env, timeout=30, creationflags=0x08000000)
            stdout = small / f'worker{suffix}.stdout'
            stdout.write_bytes(proc.stdout)
            stdout.with_suffix('.stderr').write_bytes(proc.stderr)
            assert proc.returncode == 0 and b'\x1eYAKSHARK:' in proc.stdout
            resp = json.loads(proc.stdout.split(b'\x1eYAKSHARK:')[-1])
            save(small / f'worker{suffix}.json', resp)
            build = load(bins / 'yak-receipt.json')
            assert build['exit_code'] == 0 and build['binary_SHA256'] == sha(bins / 'yak.exe')
            save(small / f'worker{suffix}.producer.json', {'arguments': argv, 'exit_code': 0, 'source': build['source'],
                  'request': receipt(small / f'worker{suffix}.request.json'), 'stdout': receipt(stdout),
                  'binary': receipt(bins / 'yak.exe'), 'response': receipt(small / f'worker{suffix}.json')})
            worker[suffix] = worker_errors(resp)
        if delta:
            assert layout['failure']['decoder'] in ('IPv4', 'Ethernet')
            assert layout['partial_network_layers'] > 1
            assert worker['09'] == worker['13'] and len(worker['09']) == 1
            assert layout['failure']['error'] in worker['09'][0]
            changed.append(n)
        else:
            assert layout['failure']['decoder'] == 'IPv4 version' and values['before']['stats']['DecodeErrors'] == 1
        ts_out = small / 'wireshark.json'
        run([a.tshark, '-n', '-r', root / 'observation.pcapng', '-T', 'json', '-x'], ts_out)
        ts = load(ts_out)
        assert ts[0]['_source']['layers']['frame_raw'][0] == r['wire'].hex()
        facts.append({'original_frame': n, 'wire_SHA256': manifest['original_wire_SHA256'],
                      'original_record': {k: v for k, v in r.items() if k != 'wire'},
                      'single_capture': receipt(root / 'observation.pcapng'), 'manifest': receipt(root / 'manifest.json'),
                      'layout': layout, 'DecodeErrors_before': values['before']['stats']['DecodeErrors'],
                      'DecodeErrors_after': values['after']['stats']['DecodeErrors'], 'delta': delta,
                      'worker_errors': worker, 'literal_wireshark_diagnostics': experts(ts),
                      'wireshark_scope': 'Literal diagnostics retained; Wireshark may attempt children of an unselected EtherIP profile. Its label/version guesses are not accepted as child identity.'})
    delta = sum(x['delta'] for x in facts)
    before_full = load(a.before_native / 'preservation' / 'sharon' / 'flows.json')
    after_full = load(a.after_native / 'preservation' / 'sharon' / 'flows.json')
    for root, value in ((a.before_native, before_full), (a.after_native, after_full)):
        pr = load(root / 'preservation' / 'sharon' / 'flows-producer.json')
        assert pr['exit_code'] == 0 and pr['capture_SHA256'] == sha(a.capture)
        assert pr['export_SHA256'] == sha(root / 'preservation' / 'sharon' / 'flows.json')
    whole_delta(before_full, after_full, delta)
    result = {'scope': 'Accounting restoration of actual existing errors; no protocol/flow admission is inferred from malformed children.',
              'auditor': receipt(__file__), 'capture': receipt(a.capture), 'facts': facts,
              'changed_sources': changed, 'per_source_delta_sum': delta,
              'full_original_before': receipt(a.before_native / 'preservation' / 'sharon' / 'flows.json'),
              'full_original_after': receipt(a.after_native / 'preservation' / 'sharon' / 'flows.json'),
              'full_original_whole_object_expected': True, 'all_other_full_values_literal': True,
              'DecodeErrors_before': before_full['stats']['DecodeErrors'], 'DecodeErrors_after': after_full['stats']['DecodeErrors']}
    if a.counterfactual:
        output = a.out / 'counterfactual.pcapng'
        mutations = {n: source_layout(source[n])['first_inner_ethertype_offset'] for n in changed}
        edits = patch_capture(a.capture, output, mutations)
        checked = 0
        for n, (old, new) in enumerate(zip(capture_records(a.capture), capture_records(output)), 1):
            exp_record = copy.deepcopy(old)
            if n in mutations:
                w = bytearray(old['wire'])
                q = mutations[n]
                w[q:q + 2] = b'\xff\xff'
                exp_record['wire'] = bytes(w)
            assert same(exp_record, new), (n, 'counterfactual record changed beyond explicit bytes')
            checked += 1
        assert checked == after_full['stats']['CapturedPackets']
        full = {}
        for name, bins in (('before', a.before_bins), ('after', a.after_bins)):
            dst = a.out / f'counterfactual-{name}-flows.json'
            run([bins / 'flows.exe', '-max-capture-bytes', '134217728', output], dst, bins / 'flows-receipt.json')
            full[name] = load(dst)
        assert same(full['before'], full['after']), 'counterfactual full outputs differ'
        cf_expected = copy.deepcopy(after_full)
        cf_expected['capture_SHA256'] = sha(output)
        assert same(cf_expected, full['after']), 'counterfactual affected values beyond input SHA'
        result['counterfactual'] = {'purpose': 'Change only first attempted inner Ethernet EtherType at each changed source; invalid EtherIP remains unselected. This is an authored discriminator control, not valid traffic.',
                                    'capture': receipt(output), 'mutations': edits, 'records_whole_checked': checked,
                                    'both_full_outputs_literal_equal': True,
                                    'entire_original_after_object_equal_except_derived_capture_SHA256': True,
                                    'DecodeErrors_both': full['before']['stats']['DecodeErrors']}
    controls = []
    first = source[changed[0]]
    for name, offset, value in [('outer-version', 14, 0x65), ('outer-IHL', 14, 0x46), ('outer-next-protocol', 23, 6),
                                 ('outer-EtherType', 12, 0x86), ('outer-fragment-offset', 21, 1)]:
        mutant = copy.deepcopy(first)
        w = bytearray(first['wire'])
        w[offset] = value
        mutant['wire'] = bytes(w)
        try:
            source_layout(mutant)
        except AssertionError:
            controls.append({'name': name, 'rejected': True})
        else:
            raise AssertionError('source negative control accepted: ' + name)
    for name, mutate in [('flow-created', lambda x: x.update(flows=[{}])),
                         ('closed-created', lambda x: x.update(closed=[{}])),
                         ('wire-bytes-counter', lambda x: x['stats'].update(CapturedBytes=x['stats']['CapturedBytes'] + 1)),
                         ('counter-scalar-type', lambda x: x['stats'].update(DecodeErrors=float(x['stats']['DecodeErrors']))),
                         ('error-hidden', lambda x: x.update(error='unexpected'))]:
        mutant = copy.deepcopy(after_full)
        mutate(mutant)
        try:
            whole_delta(before_full, mutant, delta)
        except AssertionError:
            controls.append({'name': name, 'rejected': True})
        else:
            raise AssertionError('whole-object negative control accepted: ' + name)
    result['negative_controls'] = controls
    save(a.out / 'summary.json', result)
    print(json.dumps({'summary': str(a.out / 'summary.json'), 'SHA256': sha(a.out / 'summary.json'),
                      'changed_sources': changed, 'delta': delta, 'counterfactual': bool(a.counterfactual)}))


if __name__ == '__main__':
    main()
