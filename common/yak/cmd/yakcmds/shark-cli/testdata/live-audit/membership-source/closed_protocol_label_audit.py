"""Count original-frame labels using closed SDK/CLI producers and fresh TShark.

Labels are compared literally. A difference is an inspection lead, never a
protocol correctness verdict. Logical PDUs and EOF publications are separate.
Reference TShark uses an isolated empty personal configuration, no explicit
keylog, no name resolution and explicit out-of-order TCP reassembly.
"""
import argparse
import collections
import csv
from decimal import Decimal
import hashlib
import itertools
import json
import os
from pathlib import Path
import subprocess
import sys
import time


def sha(path):
    digest = hashlib.sha256()
    with Path(path).open('rb') as stream:
        for chunk in iter(lambda: stream.read(1 << 20), b''):
            digest.update(chunk)
    return digest.hexdigest()


def load(path):
    return json.loads(Path(path).read_text('utf-8-sig'))


def json_rows(path):
    with path.open(encoding='utf-8-sig') as stream:
        for line in stream:
            yield json.loads(line)


def closed(root, corpus, kind):
    folder = root/'preservation'/corpus
    proof_path = folder/(kind+'-producer.json')
    proof = load(proof_path)
    export = folder/(kind+'.jsonl')
    binary = Path(proof['arguments'][0])
    build_path = binary.parent/(kind+'-receipt.json')
    build = load(build_path)
    assert proof['exit_code'] == build['exit_code'] == 0
    assert proof['source'] == build['source']
    assert sha(binary) == proof['binary_SHA256'] == build['binary_SHA256']
    assert sha(export) == proof['export_SHA256']
    capture = Path(proof['capture'])
    assert sha(capture) == proof['capture_SHA256']
    return export, capture, dict(source=proof['source'], export_SHA256=sha(export),
                                 producer_SHA256=sha(proof_path), build_SHA256=sha(build_path),
                                 capture_SHA256=proof['capture_SHA256'])


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    for name in ('native', 'CLI', 'tshark', 'audit-modules', 'out'):
        parser.add_argument('--'+name, type=Path, required=True)
    args = parser.parse_args()
    args.out.mkdir(exist_ok=False)
    sys.path.insert(0, str(args.audit_modules))
    from vxlan_endpoint_projection_audit import capture_records
    config = args.out/'empty-wireshark-config'
    config.mkdir()
    env = dict(os.environ, WIRESHARK_CONFIG_DIR=str(config))
    folders = subprocess.check_output([str(args.tshark), '-G', 'folders'], env=env)
    (args.out/'wireshark-folders.txt').write_bytes(folders)
    assert str(config).lower().encode() in folders.lower(), 'TShark did not use isolated configuration'
    result = dict(scope=__doc__, auditor_SHA256=sha(__file__), TShark_SHA256=sha(args.tshark),
                  folders_SHA256=sha(args.out/'wireshark-folders.txt'), inputs={})
    for corpus in ('weber', 'sharon'):
        native, capture, native_binding = closed(args.native, corpus, 'events')
        cli, cli_capture, cli_binding = closed(args.CLI, corpus, 'yak')
        assert capture.resolve() == cli_capture.resolve()
        out = args.out/corpus
        out.mkdir()
        reference = out/'wireshark.csv'
        fields = ('frame.number', 'frame.cap_len', 'frame.len', 'frame.interface_id',
                  'frame.time_epoch', '_ws.col.Protocol', 'frame.protocols')
        command = [str(args.tshark), '-n', '-o', 'tcp.reassemble_out_of_order:TRUE',
                   '-r', str(capture), '-T', 'fields', '-E', 'separator=,', '-E', 'quote=d', '-E', 'occurrence=f']
        for field in fields:
            command += ['-e', field]
        started = time.monotonic()
        with reference.open('xb') as stream, (out/'wireshark.stderr').open('xb') as stderr:
            run = subprocess.run(command, stdout=stream, stderr=stderr, env=env, creationflags=0x08000000)
        assert run.returncode == 0
        labels, native_protocols, pairs = collections.Counter(), collections.Counter(), collections.Counter()
        ws_labels = collections.Counter()
        publications = 0
        physical_rows = []
        # Retain only the compact original physical-label index in memory.
        for row in json_rows(cli):
            if 'number' in row:
                assert type(row['number']) is int and row['number'] == len(physical_rows)+1
                physical_rows.append((row['number'], row['protocol']))
                labels[row['protocol']] += 1
            else:
                assert row['record_type'] == 'protocol_publication'
                publications += 1
        for event in json_rows(native):
            native_protocols[event['protocol']] += 1
        with reference.open(encoding='utf8', newline='') as stream:
            reference_rows = csv.reader(stream)
            original_rows = capture_records(capture)
            count = 0
            for number, (record, ws, packet) in enumerate(itertools.zip_longest(original_rows, reference_rows, physical_rows), 1):
                assert record is not None and ws is not None and packet is not None
                assert len(ws) == len(fields)
                assert int(ws[0]) == packet[0] == number
                assert int(ws[1]) == len(record['wire'])
                assert int(ws[2]) == record['original_length']
                assert int(ws[3] or '0') == record['interface']
                assert Decimal(ws[4])*Decimal(1000000000) == record['time_ns']
                ws_labels[ws[5]] += 1
                pairs[(ws[5], packet[1])] += 1
                count = number
        assert count == len(physical_rows) == sum(labels.values()) == sum(ws_labels.values())
        with (out/'literal-label-pairs.csv').open('w', encoding='utf8', newline='') as stream:
            writer = csv.writer(stream)
            writer.writerow(('Wireshark literal label', 'Shark literal label', 'physical frames'))
            writer.writerows((*pair, value) for pair, value in sorted(pairs.items()))
        differences = [dict(label=label, reference_frames=ws_labels[label], same_label_native_frames=labels[label],
                            same_label_delta=labels[label]-ws_labels[label])
                       for label in sorted(ws_labels.keys() | labels.keys(), key=lambda key: (-ws_labels[key], key))]
        value = dict(physical_frames=count, native_message_records=sum(native_protocols.values()),
                     CLI_nonphysical_publications=publications, capture_SHA256=sha(capture),
                     SDK=native_binding, CLI=cli_binding, reference_labels=dict(ws_labels), current_labels=dict(labels),
                     native_message_protocols=dict(native_protocols), raw_label_differences=differences,
                     source_frame_number_length_interface_clock_checks=count,
                     TShark_arguments=command, TShark_exit=run.returncode,
                     TShark_seconds=round(time.monotonic()-started, 3),
                     reference_SHA256=sha(reference), label_pairs_SHA256=sha(out/'literal-label-pairs.csv'))
        result['inputs'][corpus] = value
        print(json.dumps(dict(corpus=corpus, physical_frames=count, native_messages=sum(native_protocols.values()),
                              source=cli_binding['source'], literal_label_differences_are_not_protocol_errors=True)), flush=True)
    target = args.out/'summary.json'
    target.write_text(json.dumps(result, indent=2, sort_keys=True)+'\n', encoding='utf8')
    print(json.dumps(dict(summary=str(target), SHA256=sha(target))), flush=True)


if __name__ == '__main__':
    main()
