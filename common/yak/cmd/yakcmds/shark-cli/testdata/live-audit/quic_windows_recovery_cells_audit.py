"""Bind actual Windows recovered QUIC views to wire and independent AEAD data.

Only the visible byte prefix is certified for a viewport shorter than a source.
Physical frame, encrypted QUIC packet, authenticated STREAM and extracted wire
remainder remain separate. Derived key-update facts have no ciphertext span.
"""
import argparse
import base64
import copy
import hashlib
import json
from pathlib import Path
import sys


def sha(path):
    return hashlib.sha256(Path(path).read_bytes()).hexdigest()


def read(path):
    return json.loads(Path(path).read_text('utf-8-sig'))


def check_prefix(snapshot, expected, name, cells, scope):
    assert name in scope(snapshot), (name, scope(snapshot))
    actual, _ = cells(snapshot)
    assert actual and len(actual) <= len(expected)
    assert actual == expected[:len(actual)]
    return dict(source_caption=scope(snapshot).strip(' ─╮'),
                visible_prefix_bytes=len(actual), complete_source_bytes=len(expected),
                whole_source_SHA256=hashlib.sha256(expected).hexdigest(),
                all_visible_octets_source_exact=True, viewport_contains_entire_source=len(actual) == len(expected))


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    for name in ('manifest','runtime','native','requirements','audit-modules','out'):
        parser.add_argument('--'+name, type=Path, required=True)
    args = parser.parse_args()
    args.out.mkdir(exist_ok=False)
    sys.path.insert(0, str(args.audit_modules))
    sys.path.insert(0, str(args.audit_modules/'tls-quic-source/recovery-02'))
    from vxlan_endpoint_projection_audit import capture_records
    from pim_windows_cells_audit import state, cells, scope
    from sparse_epoch_packet_audit import verify_receipt
    manifest = read(args.manifest)
    case, = manifest.values()
    runtime = read(args.runtime/'runtime-summary.json')
    assert runtime['actual_cases'] == 1
    entry, = runtime['case_proofs']
    proof = read(entry['path'])
    assert sha(entry['path']) == entry['SHA256']
    assert proof['expected'] == case and proof['exit_code'] == 0
    assert proof['manifest_SHA256'] == sha(args.manifest)
    assert proof['source'] == runtime['source'] == '24ca25d193994e8911c6fed7076bd4be1b8a69ac'
    assert sha(case['capture']) == proof['capture_SHA256'] == case['capture_SHA256']
    assert sha(case['keys']) == case['keylog_SHA256']
    assert proof['restored'] == dict(error='<nil>', input_cp=936, output_cp=936)
    binary = Path(proof['arguments'][2])
    build = read(binary.parent/'yak-receipt.json')
    assert build['source'] == proof['source'] and build['exit_code'] == 0
    assert sha(binary) == proof['binary_SHA256'] == build['binary_SHA256']
    assert sha(binary.parent/'yak-receipt.json') == proof['build_receipt_SHA256']
    assert sha(proof['arguments'][0]) == proof['helper_SHA256']
    assert '--protocol-history-mib' not in proof['arguments'], 'this proof requires the unchanged default budget'
    name = entry['case']
    for stem, binding in proof['snapshots'].items():
        assert sha(args.runtime/name/(stem+'.json')) == binding['cells_SHA256']
        assert sha(args.runtime/name/(stem+'.png')) == binding['PNG_SHA256']
    requirement, = [r for r in read(args.requirements) if r['PDU'] == 299]
    assert requirement['frame'] == 294 and requirement['source_frame'] == 464
    native_proof = verify_receipt(args.native, case['capture'], case['keys'])
    assert native_proof['source'] == runtime['source']
    event = next(json.loads(line) for line in (args.native/'events.jsonl').open(encoding='utf8') if json.loads(line)['id'] == 299)
    raw = base64.b64decode(event['raw'])
    assert raw == base64.b64decode(requirement['raw_packet_base64'])
    assert sha(args.requirements) == 'd0f405a6c0fea2ac8500e68499e081978021afd1557e7e32db252255615e5ea3'
    assert hashlib.sha256(raw).hexdigest() == requirement['wire_SHA256'] == event['source_bytes']['SHA256']
    assert event['protocol'] == 'http3' and event['session']['Authentication Verified'] is True
    assert event['session']['Packet Number'] == requirement['independent_authenticated_PN'] == 229
    assert event['session']['Key Update Steps'] == 2 and event['session']['Unobserved Key Epochs'] == 1
    assert event['session']['QPACK Blocked'] is True and not event['session'].get('Headers')
    assert event['source_bytes']['CompletedIn'] == dict(Number=294, Domain=event['domain'])
    assert event['source_bytes']['PacketRefs'] == [event['source_bytes']['CompletedIn']]
    record = next(r for n, r in enumerate(capture_records(Path(case['capture'])), 1) if n == 294)
    wire = record['wire']
    assert record['link'] == 0 and int.from_bytes(wire[:4], 'little') == 2
    ip = 4
    assert wire[ip] >> 4 == 4 and wire[ip+9] == 17
    ihl = (wire[ip] & 15)*4
    assert ihl >= 20 and int.from_bytes(wire[ip+6:ip+8], 'big') & 0x3fff == 0
    udp = ip+ihl
    size = int.from_bytes(wire[udp+4:udp+6], 'big')
    assert ip+int.from_bytes(wire[ip+2:ip+4], 'big') == udp+size == len(wire)
    assert wire[udp+8:udp+size] == raw
    stream, = [f for f in requirement['expected_wire_frame_core'] if f['Frame Type'] == 'STREAM']
    stream_bytes = base64.b64decode(stream['Stream Data'])
    assert stream['Stream ID'] == 16 and stream['Offset'] == 0 and stream['Length'] == len(stream_bytes)
    observed_stream, = [f for f in event['session']['Frames'] if f['Frame Type'] == 'STREAM']
    assert base64.b64decode(observed_stream['Stream Data']) == stream_bytes
    snapshots = {stem: state(args.runtime/name/(stem+'.json')) for stem in
                 ('01-protocol-fields','01-deep-protocol-fields','02-first-byte-source','02-byte-sources')}
    first = snapshots['01-protocol-fields']
    assert case['query'] in first['Rows'][1]
    assert any('›294 ' in row and 'HTTP/3' in row for row in first['Rows'])
    assert any('HTTP/3 · PDU #299' in row for row in first['Rows'])
    checks = []
    for stem, source, caption in (
        ('01-protocol-fields', wire, 'Captured frame'),
        ('01-deep-protocol-fields', raw, 'QUIC wire packet (HTTP/3) · captured PDU'),
        ('02-first-byte-source', stream_bytes, 'QUIC STREAM · authenticated plaintext'),
        ('02-byte-sources', raw[1:], 'QUIC wire remainder · extracted octets'),
    ):
        check = check_prefix(snapshots[stem], source, caption, cells, scope)
        check['snapshot'] = stem
        checks.append(check)
    negatives = []
    for mode in ('physical-to-PDU', 'ciphertext-to-plaintext', 'plaintext-to-ciphertext', 'source-caption'):
        snapshot = copy.deepcopy(snapshots['02-first-byte-source'])
        expected = stream_bytes
        caption = 'QUIC STREAM · authenticated plaintext'
        if mode == 'physical-to-PDU':
            snapshot = snapshots['01-deep-protocol-fields']
            expected, caption = wire, 'QUIC wire packet (HTTP/3) · captured PDU'
        elif mode == 'ciphertext-to-plaintext':
            expected = raw
        elif mode == 'plaintext-to-ciphertext':
            snapshot = snapshots['01-deep-protocol-fields']
            caption = 'QUIC wire packet (HTTP/3) · captured PDU'
        else:
            snapshot['Rows'] = [row.replace('authenticated plaintext', 'ciphertext') for row in snapshot['Rows']]
        try:
            check_prefix(snapshot, expected, caption, cells, scope)
        except AssertionError:
            negatives.append(mode)
        else:
            raise AssertionError('accepted wrong QUIC byte source: '+mode)
    target = args.out/'summary.json'
    summary = dict(scope=__doc__, source=runtime['source'], actual_console_buffers=runtime['actual_console_buffers'],
        unchanged_default_history_budget=True, selected_frame=294, selected_PDU=299, source_frame=464,
        independent_AEAD_PN=229, key_update_steps=2, unobserved_key_epochs=1, QPACK_blocked=True,
        decoded_dynamic_headers_claim=False, runtime_SHA256=sha(args.runtime/'runtime-summary.json'),
        requirements_SHA256=sha(args.requirements), native_producer_SHA256=sha(args.native/'producer.json'),
        manifest_SHA256=sha(args.manifest), auditor_SHA256=sha(__file__), checks=checks,
        rejected_negative_controls=negatives, differences=0, passed=True)
    target.write_text(json.dumps(summary, indent=2, sort_keys=True)+'\n', encoding='utf8')
    print(json.dumps(dict(summary=str(target), SHA256=sha(target), actual_console_buffers=runtime['actual_console_buffers'], differences=0)), flush=True)


if __name__ == '__main__':
    main()
