"""Independent owned IPv4-loopback IMAP/RTSP wire and association audit.

Compare every selected SDK byte/contributor with classic PCAP TCP sequences,
independent endpoint receipts and Wireshark Follow TCP Stream. Compare complete
IMAP line fields and available RTSP fields, reporting missing reference layouts
explicitly. This does not certify credentials, mailbox state, media effects,
unknown SASL mechanisms, production servers or arbitrary capture link types.
"""
import argparse
import base64
import collections
import hashlib
import ipaddress
import json
import re
import struct
from pathlib import Path


def canonical(value):
    def extra(v):
        if isinstance(v, bytes): return {'wire_octets':base64.b64encode(v).decode()}
        if isinstance(v, set): return {'set_items':sorted(v)}
        raise TypeError(type(v).__name__)
    return json.dumps(value, sort_keys=True, separators=(',', ':'), ensure_ascii=True, default=extra)


def sha(wire):
    return hashlib.sha256(wire).hexdigest()


def namespace_groups(wire):
    """Independent cursor for this live capture's quoted/NIL namespace profile."""
    at = 0
    def take(value):
        nonlocal at
        assert wire[at:at+len(value)].upper() == value.upper(), 'independent namespace punctuation/token'
        at += len(value)
    def quoted():
        nonlocal at
        take(b'"')
        value, spans, fragment = bytearray(), [], at
        while at < len(wire) and wire[at] != 34:
            assert wire[at] not in [0,10,13] and wire[at] < 128
            if wire[at] == 92:
                if fragment < at:
                    spans.append([fragment,at]);value.extend(wire[fragment:at])
                at += 1
                assert wire[at] in [34,92]
                spans.append([at,at+1]);value.append(wire[at]);at += 1;fragment = at
            else:
                at += 1
        if fragment < at:
            spans.append([fragment,at]);value.extend(wire[fragment:at])
        take(b'"')
        return base64.b64encode(value).decode(),spans
    def group():
        nonlocal at
        if wire[at:at+3].upper() == b'NIL':
            take(b'NIL');return {'Kind':'nil','Entries':[]}
        take(b'(');entries=[]
        while wire[at:at+1] == b'(':
            start=at;take(b'(');prefix,prefix_spans=quoted();take(b' ')
            if wire[at:at+3].upper()==b'NIL':
                take(b'NIL');delimiter,kind=None,'nil'
            else:
                delimiter,_=quoted();kind='quoted'
                assert len(base64.b64decode(delimiter))==1
            extensions=[]
            while wire[at:at+1]==b' ':
                take(b' ');name,_=quoted();take(b' (');values=[]
                while wire[at:at+1]!=b')':
                    beginning=at
                    if values:take(b' ')
                    value,spans=quoted()
                    values.append({'Value':value,'Encoding':'quoted','Encoded Byte Ranges':spans,'Relative Byte Range':[beginning,at]})
                take(b')');assert values
                extensions.append({'Name':name,'Values':values,'Semantics Validated':False})
            take(b')')
            entries.append({'Prefix':prefix,'Prefix Encoding':'quoted','Prefix Encoded Byte Ranges':prefix_spans,
                            'Hierarchy Delimiter':delimiter,'Hierarchy Delimiter Kind':kind,'Extensions':extensions,'Relative Byte Range':[start,at]})
        take(b')');assert entries
        return {'Kind':'namespace-list','Entries':entries}
    take(b'* NAMESPACE ')
    groups={}
    for name in ['Personal Namespaces','Other Users Namespaces','Shared Namespaces']:
        if groups:take(b' ')
        groups[name]=group()
    take(b'\r\n');assert at==len(wire)
    return groups


def audit(capture, manifest, receipts, events, oracle, follow_dir):
    checks = 0
    def check(ok, why):
        nonlocal checks
        checks += 1
        assert ok, why
    def eq(x, y, why):
        check(canonical(x) == canonical(y), why)
    def b64(wire):
        return base64.b64encode(wire).decode()
    def load(path):
        return json.loads(path.read_text(encoding='utf-8-sig'))
    def leaves(value, key):
        if isinstance(value, dict):
            for name, child in value.items():
                if name == key:
                    yield from child if isinstance(child, list) else [child]
                elif isinstance(child, (dict, list)):
                    yield from leaves(child, key)
        elif isinstance(value, list):
            for child in value:
                yield from leaves(child, key)
    m = load(manifest)
    eq(m['failures'], [], 'independent endpoint operations succeeded')
    data = capture.read_bytes()
    eq(sha(data), m['capture_sha256'], 'complete original capture identity')
    check(data[:4] in [b'\xd4\xc3\xb2\xa1', b'\x4d\x3c\xb2\xa1'], 'selected classic little-endian PCAP')
    eq(struct.unpack_from('<I', data, 20)[0], 0, 'selected actual Npcap DLT_NULL capture')
    ports = {m['imap_port'], m['rtsp_port']}
    frames, streams = {}, {}
    at, number = 24, 0
    while at < len(data):
        check(at + 16 <= len(data), 'complete PCAP record header')
        _, _, size, original = struct.unpack_from('<IIII', data, at)
        at += 16
        check(size <= original and at + size <= len(data), 'bounded captured record')
        packet = data[at:at + size]
        at += size
        number += 1
        frames[number] = dict(wire=packet)
        if len(packet) < 24 or packet[4] >> 4 != 4 or packet[13] != 6:
            continue
        ip = 4
        ihl = (packet[ip] & 15) * 4
        total = int.from_bytes(packet[ip + 2:ip + 4], 'big')
        tcp = ip + ihl
        check(ihl >= 20 and ip + total <= len(packet) and tcp + 20 <= ip + total, 'complete IPv4/TCP carrier')
        sport, dport = struct.unpack_from('>HH', packet, tcp)
        if sport not in ports and dport not in ports:
            continue
        src = str(ipaddress.ip_address(packet[ip + 12:ip + 16])) + ':' + str(sport)
        dst = str(ipaddress.ip_address(packet[ip + 16:ip + 20])) + ':' + str(dport)
        key = (src, dst)
        seq = int.from_bytes(packet[tcp + 4:tcp + 8], 'big')
        hlen = (packet[tcp + 12] >> 4) * 4
        check(hlen >= 20 and tcp + hlen <= ip + total, 'complete TCP header')
        flags = packet[tcp + 13]
        state = streams.setdefault(key, dict(base=None, octets={}, frames={}, cursor=0))
        if flags & 2:
            base = (seq + 1) & 0xffffffff
            check(state['base'] in [None, base], 'no hidden tuple generation in this declared live capture')
            state['base'] = base
        payload = packet[tcp + hlen:ip + total]
        frames[number].update(key=key, payload=payload)
        if not payload:
            continue
        check(state['base'] is not None, 'actual live SYN establishes this direction sequence base')
        offset = (seq + bool(flags & 2) - state['base']) & 0xffffffff
        check(offset + len(payload) <= 1 << 20, 'bounded owned stream')
        for i, byte in enumerate(payload, offset):
            check(i not in state['octets'] or state['octets'][i] == byte, 'no conflicting TCP overlap')
            state['octets'][i] = byte
            state['frames'].setdefault(i, number)
    eq(at, len(data), 'all PCAP bytes consumed')
    for state in streams.values():
        eq(sorted(state['octets']), list(range(len(state['octets']))), 'owned complete stream has no sequence hole')
        state['wire'] = bytes(state['octets'][i] for i in range(len(state['octets'])))
    eq(len(streams), 10, 'five real connections, ten directions')
    follows = {}
    for path in sorted(follow_dir.glob('*.txt')):
        nodes, payloads = {}, [bytearray(), bytearray()]
        for line in path.read_text(encoding='utf-8-sig').splitlines():
            match = re.fullmatch(r'Node ([01]): (.+)', line)
            if match:
                nodes[int(match[1])] = match[2]
            elif re.fullmatch(r'[0-9a-fA-F]+', line.strip()):
                payloads[int(line.startswith('\t'))].extend(bytes.fromhex(line.strip()))
        eq(set(nodes), {0, 1}, 'reference Follow declares both endpoints')
        for direction in [0, 1]:
            key = (nodes[direction], nodes[1 - direction])
            check(key not in follows, 'one reference stream per actual direction')
            follows[key] = bytes(payloads[direction])
    eq(set(follows), set(streams), 'reference and independent capture connection directions are bijective')
    for key, state in streams.items():
        eq(sha(follows[key]), sha(state['wire']), 'Wireshark complete TCP direction bytes equal independent sequence reconstruction')
    reference, reference_complete_rtsp = {}, collections.defaultdict(list)
    for row in load(oracle):
        layers = row['_source']['layers']
        no = int(layers['frame']['frame.number'])
        eq(bytes.fromhex(layers['frame_raw'][0]), frames[no]['wire'], 'reference literal physical frame bytes')
        reference[no] = layers
        raw_views = layers.get('rtsp_raw', [])
        if raw_views and isinstance(raw_views[0], str):
            raw_views = [raw_views]
        for raw in raw_views:
            reference_complete_rtsp[no,sha(bytes.fromhex(raw[0]))].append(layers.get('rtsp', {}))
    eq(set(reference), set(frames), 'every physical frame has a reference witness')
    expected_receipts, actual_receipts = collections.Counter(), collections.Counter()
    for receipt in load(receipts):
        wire = base64.b64decode(receipt['wire_base64'], validate=True)
        eq(sha(wire), receipt['sha256'], 'exact independent endpoint receipt bytes')
        eq(len(wire), receipt['bytes'], 'exact endpoint receipt length')
        units = [line + b'\r\n' for line in wire.split(b'\r\n')[:-1]] if receipt['server_port'] == m['imap_port'] else [wire]
        for unit in units:
            expected_receipts[receipt['server_port'], receipt['direction'], sha(unit)] += 1
    sasl, idle, namespaces, rtsp_pending = {}, {}, {}, {}
    selected, reference_fields, stages, limited_reference = [], 0, collections.Counter(), []
    for line in events.open(encoding='utf-8-sig'):
        e = json.loads(line)
        if e['protocol'] not in ['imap', 'rtsp'] or e['transport'] != 'tcp':
            continue
        wire = base64.b64decode(e['raw'], validate=True)
        key = (e['source'], e['destination'])
        check(key in streams, 'native direction belongs to the independently captured connections')
        state = streams[key]
        start, end = state['cursor'], state['cursor'] + len(wire)
        eq(wire, state['wire'][start:end], 'each complete native PDU owns the next exact contiguous stream bytes')
        state['cursor'] = end
        source = e['source_bytes']
        eq(source['SHA256'], sha(wire), 'native complete raw identity')
        contributors = sorted(set(state['frames'][i] for i in range(start, end)))
        eq(sorted(r['Number'] for r in source['PacketRefs']), contributors, 'every contributor equals independent TCP byte ownership')
        for r in source['PacketRefs']:
            eq(r['Domain'], e['domain'], 'exact capture-domain attribution')
        if source.get('CompletedIn'):
            eq(source['CompletedIn']['Number'], max(contributors), 'completion frame equals final required byte arrival')
            eq(source['CompletedIn']['Domain'],e['domain'],'completion domain')
        else:
            # A malformed diagnostic need not advertise an accepted-PDU
            # CompletedIn. Its entire line and contributor set are independently
            # certified above; do not fabricate that optional API property.
            check(e['status']=='malformed' and (e.get('session')or{}).get('Error Scope')=='message','only the selected bounded refusal may omit accepted-PDU completion')
        port = next(int(endpoint.rsplit(':', 1)[1]) for endpoint in key if int(endpoint.rsplit(':', 1)[1]) in ports)
        direction = 's2c' if int(key[0].rsplit(':', 1)[1]) == port else 'c2s'
        actual_receipts[port, direction, sha(wire)] += 1
        conn = tuple(sorted(key))
        s = e.get('session') or {}
        if e['protocol'] == 'imap':
            check(wire.endswith(b'\r\n') and b'\r\n' not in wire[:-2], 'exact single IMAP transport line')
            expected_line = wire.decode('ascii')
            check(expected_line in list(leaves(reference[max(contributors)].get('imap'), 'imap.line')), ('complete byte/frame-matched Wireshark IMAP line field',e['id'],max(contributors)))
            reference_fields += 1
            words = wire[:-2].split(b' ')
            stage = s.get('SASL Stage')
            owner = sasl.get(conn)
            if len(words) >= 3 and words[1].upper() == b'AUTHENTICATE':
                check(owner is None and conn not in idle, 'no cross-exchange authentication owner')
                owner = dict(tag=words[0], mechanism=words[2].decode(), client=key[0], id=e['id'], challenge=0, waiting=False, challenges=0, responses=0)
                sasl[conn] = owner
                eq(stage, 'command', 'observed complete AUTHENTICATE starts SASL context')
                eq(s['Mechanism'], words[2].decode(), 'mechanism is the exact wire name')
                eq(e['transaction'], e['id'], 'authentication transaction is its actual command PDU')
            elif owner is not None and key[0] != owner['client'] and words[0] == owner['tag']:
                eq(stage, 'result', 'opposite-direction matching tag completes authentication')
                eq(s['Server Result Observed'], words[1].decode().upper(), 'server result remains an observation')
                eq(e['response_to'], owner['id'], 'authentication result owns the exact request PDU')
                del sasl[conn]
            elif owner is not None and wire.startswith(b'+ '):
                check(key[0] != owner['client'] and not owner['waiting'], 'one opposite-direction challenge at a time')
                owner['challenge'], owner['waiting'] = e['id'], True
                owner['challenges'] += 1
                eq(stage, 'challenge', 'challenge stage is transport-bound')
                eq(e['response_to'], owner['id'], 'challenge belongs to this AUTHENTICATE')
            elif owner is not None and key[0] == owner['client']:
                check(owner['waiting'], 'response requires a captured challenge')
                owner['waiting'] = False
                value = wire[:-2]
                try:
                    decoded = b'' if value == b'*' else base64.b64decode(value, validate=True)
                except ValueError:
                    eq(e['status'], 'malformed', 'declared invalid base64 is refused')
                    eq(e['fields'], None, 'refusal never publishes successful generic fields')
                    eq(e['response_to'], 0, 'refusal never publishes a valid response association')
                else:
                    owner['responses'] += 1
                    eq(stage, 'cancel' if value == b'*' else 'response', 'response/cancellation stage')
                    eq(s['Transport Value']['Decoded Payload'], b64(decoded), 'independent complete decoded transport payload')
                    eq(e['response_to'], owner['challenge'], 'response owns exactly the latest challenge')
            if stage is not None:
                eq(s['Tag'], owner['tag'].decode(), 'SASL tag cannot drift from the captured owner')
                eq(s['Mechanism'], owner['mechanism'], 'SASL mechanism cannot drift between PDUs')
                eq(s['Challenge Count'], owner['challenges'], 'independent challenge count')
                eq(s['Response Count'], owner['responses'], 'independent response count')
                eq(s['Authentication Verified'], False, 'no authentication proof manufactured')
                eq(s['Peer Identity Verified'], False, 'no peer identity manufactured')
                eq(s['Security Layer Verified'], False, 'no security-layer verification manufactured')
                stages['SASL ' + stage] += 1
            for value_name in ['Transport Value', 'Initial Response']:
                if value_name in s:
                    value = s[value_name]
                    encoded = base64.b64decode(value['Encoded Value'], validate=True)
                    eq(value['Encoded Length'], len(encoded), 'transport encoded length')
                    decoded = b'' if encoded in [b'*', b'='] else base64.b64decode(encoded, validate=True)
                    eq(value['Decoded Payload'], b64(decoded), 'derived decoded transport bytes')
                    eq(value['Decoded Length'], len(decoded), 'derived decoded transport length')
                    eq(value['Base64 Canonical'], encoded in [b'*', b'='] or base64.b64encode(decoded) == encoded, 'independent canonical base64 fact')
                    eq(value['Decoded Values Are Wire Spans'], False, 'decoded bytes cannot claim encoded wire offsets')
                    if encoded:
                        span = value['Wire Field Ranges']['Encoded Value']
                        eq(wire[span[0]:span[1]], encoded, 'encoded spans refer to the whole native PDU coordinate system')
            if s.get('Namespace Stage') == 'command':
                namespaces[conn, words[0]] = (e['id'], key[0])
            elif s.get('Namespace Stage') == 'response':
                candidates = [o for (c, _), o in namespaces.items() if c == conn and o[1] != key[0]]
                eq(e['response_to'], candidates[0][0] if len(candidates) == 1 else 0, 'untagged NAMESPACE association is withheld when ambiguous')
                info = s['IMAP Fields']
                eq(info['Response Name'], 'NAMESPACE', 'namespace layout')
                eq(info['Mailbox Names Decoded'], False, 'encoded mailbox prefixes are not mislabeled decoded names')
                expected_groups=namespace_groups(wire)
                for group in ['Personal Namespaces', 'Other Users Namespaces', 'Shared Namespaces']:
                    check(group in info, 'all three namespace groups are displayed')
                    eq(info[group],expected_groups[group], 'complete independently parsed namespace group, extensions and provenance')
                    for entry in info[group]['Entries']:
                        encoded = b''.join(wire[a:b] for a, b in entry['Prefix Encoded Byte Ranges'])
                        eq(b64(encoded), entry['Prefix'], 'all decoded prefix bytes have exact escaped-string provenance')
            elif s.get('Namespace Stage') == 'result':
                request, client = namespaces.pop((conn, words[0]))
                check(client != key[0], 'NAMESPACE completion direction')
                eq(e['response_to'], request, 'NAMESPACE tagged completion owns exact request')
            idle_stage = s.get('IDLE Stage')
            if idle_stage == 'command':
                check(conn not in idle and conn not in sasl, 'IDLE cannot steal an exchange')
                idle[conn] = dict(id=e['id'], client=key[0], continued=0, done=False)
            elif idle_stage == 'continuation':
                o = idle[conn]
                check(o['client'] != key[0] and not o['continued'], 'opposite-direction IDLE continuation')
                eq(e['response_to'], o['id'], 'IDLE continuation request association')
                o['continued'] = e['id']
            elif idle_stage == 'done':
                o = idle[conn]
                check(o['client'] == key[0] and o['continued'] and not o['done'], 'one client DONE per continuation')
                eq(wire, b'DONE\r\n', 'exact owned live DONE line')
                eq(e['response_to'], o['continued'], 'DONE satisfies this observed continuation')
                o['done'] = True
            elif idle_stage == 'result':
                o = idle.pop(conn)
                check(o['done'] and o['client'] != key[0], 'server result follows DONE')
                eq(e['response_to'], o['id'], 'IDLE result owns the exact command')
            if idle_stage:
                stages['IDLE ' + idle_stage] += 1
            if str(s.get('Profile', '')).startswith(('imap-sasl-', 'imap-namespace-', 'imap-idle-')) and e['status'] == 'decoded':
                eq(e['fields'], s, 'native progressive fields and session projection agree')
        else:
            header, separator, body = wire.partition(b'\r\n\r\n')
            check(bool(separator), 'complete RTSP header boundary')
            lines = header.split(b'\r\n')
            h = collections.defaultdict(list)
            for line in lines[1:]:
                name, value = line.split(b':', 1)
                h[name.lower()].append(value.strip())
            eq(len(h[b'cseq']), 1, 'one actual CSeq')
            seq = int(h[b'cseq'][0])
            eq(s['CSeq'], seq, 'RTSP native sequence equals independent header')
            eq(len(body), int(h.get(b'content-length', [b'0'])[0]), 'RTSP exact declared body boundary')
            eq(s['Content Length'], len(body), 'native RTSP body length')
            parts = lines[0].split(b' ', 2)
            # Wireshark's RTSP layer span may cover headers only, with body in
            # rtsp.data. Bind either exact span to actual contributor frames.
            views=[view for no in contributors for digest in {sha(wire),sha(header+separator)} for view in reference_complete_rtsp[no,digest]]
            if parts[0] == b'RTSP/1.0':
                request = rtsp_pending.pop((conn, key[1], seq))
                eq(s['In Reply To'], request, 'RTSP response uses opposite direction and matching CSeq')
                eq(s['Status'], int(parts[1]), 'RTSP status wire value')
                check(s['Matched'] is True, 'RTSP transaction matched')
                statuses=[int(v) for view in views for v in leaves(view,'rtsp.status')]
                if int(parts[1]) in statuses:
                    reference_fields+=1
                else:
                    limited_reference.append(dict(PDU=e['id'],status=int(parts[1]),physical_frames=contributors,
                                                  limitation='reference has no complete response status view for the declared three-byte prefix split; whole TCP bytes independently agree'))
            else:
                method = parts[0].decode()
                check((conn,key[0],seq) not in rtsp_pending, 'declared live reuse follows completed CSeq')
                rtsp_pending[conn, key[0], seq] = method
                eq(s['Packet Name'], method, 'RTSP complete method token')
                eq(s['URI'], parts[1].decode(), 'RTSP complete request target')
                if method == 'FLUSH':
                    eq(s['Media Effect Verified'], False, 'FLUSH is not proof of a media reset')
                    values = dict(x.split(b'=', 1) for x in h[b'rtp-info'][0].split(b';'))
                    info = s['RTP Info'][0]
                    for name in ['seq', 'rtptime']:
                        eq(info[name], int(values[name.encode()]), 'exact observed FLUSH RTP-Info integer')
                methods = [v for view in views for v in leaves(view, 'rtsp.method')]
                if method in methods:
                    reference_fields += 1
                    check(parts[1].decode() in [v for view in views for v in leaves(view,'rtsp.url')], 'exact frame-bound reference request target')
                    check(seq in [int(v) for view in views for v in leaves(view,'rtsp.cseq')], 'exact frame-bound reference CSeq')
                    check(len(body) in [int(v) for view in views for v in leaves(view,'rtsp.content-length')], 'exact reference body length')
                    reference_fields += 3
                else:
                    check(method=='FLUSH', 'complete standard RTSP request requires its exact reference method field')
                    limited_reference.append(dict(PDU=e['id'], method=method, physical_frames=contributors,
                                                  limitation='reference has no complete request method view; whole TCP bytes independently agree'))
        selected.append(dict(PDU=e['id'], protocol=e['protocol'], status=e['status'], flow=e['flow'], raw_sha256=sha(wire), physical_frames=contributors))
    eq(sasl, {}, 'no authentication owner survives completed real operations')
    eq(idle, {}, 'no IDLE owner survives completed operations')
    eq(namespaces, {}, 'no namespace tag survives completed operations')
    eq(rtsp_pending, {}, 'all actual RTSP CSeq owners complete, including reuse')
    eq(sorted((str(k), v) for k, v in actual_receipts.items()), sorted((str(k), v) for k, v in expected_receipts.items()), 'every native PDU equals one independent endpoint unit, including multiplicity')
    for state in streams.values():
        eq(state['cursor'], len(state['wire']), 'every captured protocol byte is covered exactly once by selected native PDUs')
    return dict(scope=__doc__, checks=checks, capture_sha256=sha(data), events_sha256=sha(events.read_bytes()),
                oracle_sha256=sha(oracle.read_bytes()), physical_frames=number, TCP_directions=len(streams),
                compared_PDUs=selected, native_counts=dict(collections.Counter(e['protocol'] for e in selected)),
                native_status_counts=dict(collections.Counter(e['status'] for e in selected)), reference_fields_compared=reference_fields,
                stages=dict(stages), reference_layout_limits=limited_reference, byte_and_flow_normalization=False)


if __name__ == '__main__':
    ap = argparse.ArgumentParser(description=__doc__)
    for name in ['capture', 'manifest', 'receipts', 'events', 'oracle', 'follow-dir', 'out']:
        ap.add_argument('--' + name, type=Path, required=True)
    args = ap.parse_args()
    for path in vars(args).values():
        assert not any((p / '.git').exists() for p in [path.resolve(), *path.resolve().parents])
    result = audit(args.capture, args.manifest, args.receipts, args.events, args.oracle, args.follow_dir)
    args.out.write_text(json.dumps(result, indent=2) + '\n', encoding='utf8')
    print(json.dumps({k: v for k, v in result.items() if k in ['checks', 'physical_frames', 'TCP_directions', 'native_counts', 'native_status_counts', 'reference_fields_compared', 'stages']}))
