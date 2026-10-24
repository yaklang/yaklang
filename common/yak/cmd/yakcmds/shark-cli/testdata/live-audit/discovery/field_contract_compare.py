"""Replay retained real captures through registered fields and typed filters.
Usage: field_contract_compare.py OUTPUT TSHARK YAK INGRESS_ROOT RECOVERY_ROOT
No new traffic is claimed. Every difference retains its original frame number.
"""
import csv, hashlib, io, json, subprocess, sys
from pathlib import Path

out, ts, yak, ingress, recovery = map(Path, sys.argv[1:6])
out.mkdir(exist_ok=True)

def run(args, path):
    p = subprocess.run([str(a) for a in args], capture_output=True, timeout=120)
    path.write_bytes(p.stdout)
    path.with_suffix(path.suffix+'.stderr').write_bytes(p.stderr)
    assert p.returncode == 0, (path, p.stderr.decode(errors='replace')[:1200])
    return p.stdout.decode('utf-8-sig')

schema = json.loads(run([yak, 'shark', '--list-fields', '--json'], out/'field-schema.json'))
registered = {f['name']: f for f in schema['fields']}
assert len(registered) == len(schema['fields']) and schema['schema_version'] == 1

cases = [
    ('wg-init', 'wireguard', 'wg.type == 1'),
    ('wg-response', 'wireguard', 'wg.type == 2'),
    ('wg-counter', 'wireguard', 'wg.type == 4 and wg.counter >= 2'),
    ('wg-missing-sender', 'wireguard', 'wg.type == 4 and not wg.sender'),
    ('ssh-kex-init', 'ssh', 'ssh.message_code == 20'),
    ('ssh-newkeys', 'ssh', 'ssh.message_code == 21'),
    ('rip-route', 'control', 'rip.command == 2 and rip.metric == 2'),
    ('rip-prefix-negative', 'control', 'rip.version == 2'),
    ('ike-init', 'control', 'isakmp.exchangetype == 34 and isakmp.length >= 28'),
    ('rpc-call', 'control', 'rpc.msgtyp == 0 and rpc.program == 100000'),
    ('l2tp-request', 'control', 'l2tp.avp.message_type == 1'),
    ('openvpn-data', 'control', 'openvpn.opcode == 9'),
    ('ipmi-rmcp', 'control', 'rmcp.version == 6 and rmcp.class == 7'),
    ('tls-record', 'tls', 'tls.record.content_type == 23 and tls.record.version == 0x0303'),
    ('tls13-opaque', 'tls', 'tls.record.opaque_type == 23'),
    ('tls13-decrypted-app', 'tls-keys', 'tls.record.content_type == 23 and tls.record.opaque_type == 23'),
]

inputs = {
    'wireguard': ingress/'wireguard-02',
    'ssh': ingress/'ssh-04',
    'control': ingress/'control-05',
    'tls': recovery/'tls-suites-02',
    'tls-keys': recovery/'tls-suites-02',
}

baseline, arguments, negative = {}, {}, set()
for mode, directory in inputs.items():
    m = json.loads((directory/'manifest.json').read_text(encoding='utf8'))
    ws = [ts, '-2', '-n', '-r', directory/'live.pcap']
    ys = [yak, 'shark', '-r', directory/'live.pcap', '--json']
    if mode == 'tls-keys':
        ws += ['-o', 'tls.keylog_file:'+str(directory/'session.keys')]
        ys += ['--tls-keylog', directory/'session.keys']
    if mode == 'ssh':
        for port in m['ports']: ws += ['-d', f'tcp.port=={port},ssh']
    if mode == 'wireguard': ws += ['-d', 'udp.port==51820,wg']
    if mode == 'control':
        decoders = {'ike':'isakmp','ike-natt':'udpencap','rip':'rip','nbt-dg':'nbdgm','onc-rpc':'rpc','l2tp':'l2tp','openvpn':'openvpn','ipmi':'rmcp'}
        for name, port in m['ports'].items(): ws += ['-d', f'udp.port=={port},{decoders[name]}']
        for f in m['fallbacks']: ys += ['--decode-as', f"udp:{f['actual']}:{f['protocol']}"]
        ws += ['-d', f"tcp.port=={m['telnet_port']},telnet", '-d', f"tcp.port=={m['nbss_port']},nbss"]
        # Compare strict native admission with explicit Wireshark selection.
        # Only independently recorded negative input may explain an extra WS
        # prefix field; arbitrary discrepancies are failures.
        bad = {o['sha256'] for o in m['operations'] if o.get('positive') is False}
        data = run(ws+['-T','fields','-e','frame.number','-e','udp.payload'], out/'negative-wire.tsv')
        for line in data.splitlines():
            parts = line.split('\t')
            if len(parts) == 2 and parts[1]:
                if hashlib.sha256(bytes.fromhex(parts[1].replace(':',''))).hexdigest() in bad:
                    negative.add(int(parts[0]))
    arguments[mode] = (ws, ys)
    rows = [json.loads(l) for l in run(ys, out/(mode+'-all.jsonl')).splitlines()]
    baseline[mode] = rows
    for row in rows:
        for pdu in row.get('pdus', []):
            snapshot = pdu['field_values']
            assert snapshot['schema_version'] == 1
            assert not snapshot.get('incomplete') and not snapshot.get('truncated'), (mode, row['number'])
            fields = {}
            for value in snapshot['values']:
                name = value['name']; assert name in registered
                d = registered[name]
                if d['type'] == 'unsigned':
                    assert type(value['value']) is int and 0 <= value['value'] < 2**d['bits']
                    if d['bits'] > 53: assert value['exact_value'] == str(value['value'])
                if 'pdu_range' in value:
                    assert value['origin'] == 'pdu'
                    assert value['pdu_range']['offset'] >= 0 and value['pdu_range']['length'] >= 0
                fields.setdefault(name, []).append(value['value'])
            assert fields == pdu['fields'], (mode, row['number'], 'map/snapshot disagreement')
            if pdu['protocol'] in ['wireguard','ssh','ipmi','openvpn']:
                assert not any(name.startswith('tls.') for name in fields), (mode, row['number'], 'TLS namespace contamination')

results = []
for name, mode, expression in cases:
    wsargs, yargs = arguments[mode]
    w = {int(n) for n in run(wsargs+['-Y',expression,'-T','fields','-e','frame.number'],out/(name+'-ws.txt')).splitlines() if n}
    rows = [json.loads(l) for l in run(yargs+['--display-filter',expression],out/(name+'-yak.jsonl')).splitlines()]
    y = {r['number'] for r in rows}
    extra_ws, extra_y = sorted(w-y), sorted(y-w)
    assert not extra_y, (name, 'native extra matches', extra_y)
    assert not extra_ws or mode == 'control' and set(extra_ws) <= negative, (name, 'unexplained WS-only matches',extra_ws)
    result = dict(name=name, expression=expression, wireshark_frames=sorted(w), yak_frames=sorted(y),
                  ws_only=extra_ws, yak_only=extra_y,
                  difference_reason='explicit WS prefix fields on independently known truncated/garbage controls' if extra_ws else '')
    results.append(result);print('PASS',name,len(y),'matches;',len(extra_ws),'explained WS-only',flush=True)

(out/'summary.json').write_text(json.dumps(dict(schema_fields=len(registered),input_frames=sum(len(baseline[m]) for m in ['wireguard','ssh','control','tls']),replay_frames=sum(len(v) for v in baseline.values()),
    queries=results,inputs={mode:dict(path=str(p/'live.pcap'),sha256=hashlib.sha256((p/'live.pcap').read_bytes()).hexdigest()) for mode,p in inputs.items()}),ensure_ascii=False,indent=2),encoding='utf8')
