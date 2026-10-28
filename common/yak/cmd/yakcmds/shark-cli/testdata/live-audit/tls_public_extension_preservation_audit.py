"""Preserve every preexisting SDK/CLI value while extending TLS metadata.

This bridge does not prove new metadata is correct. The separate wire/reference
metadata auditor must do that. IDs, sources, domains, arrays, old aliases and
their relative order are literal. The only nonadditive correction is the
source-port-only pseudo-VXLAN packet in the original Sharon capture; recompute
that expected correction from its physical Ethernet/IPv4/UDP bytes.
"""
import argparse
import base64
import collections
import copy
import hashlib
import itertools
import json
from pathlib import Path

from capture_clock_wire_audit import physical
from tls_resumption_metadata_audit import (
    CONTEXT_PATHS, DERIVED_FIELDS, FORBIDDEN_ALIASES, WIRE_NAMESPACE,
    exact, message_fields,
)

MESSAGE_TYPES = {'Session ID': {1, 2}, 'PSK Offer': {1},
                 'PSK Key Exchange Modes': {1}, 'PSK Key Exchange Modes Length': {1},
                 'Session Ticket Offer': {1}, 'PSK Selection': {2}, 'New Session Ticket': {4}}
ALIASES = WIRE_NAMESPACE | {'tls.session.' + k for k in CONTEXT_PATHS}
SHARON_SHA = 'aa4fcc81af216e2b788171c64271b43681974d9156aa9ddbdc1a7a25e730714c'


def tls_carrier(event):
    return event['protocol'] == 'tls' or (event['protocol'] in ['quic','http3','doq'] and
        event['entry'] == 'QUIC' and event['transport'] == 'udp' and
        (event.get('session') or {}).get('Native Carrier') is True)


def sha(path):
    h = hashlib.sha256()
    with path.open('rb') as f:
        for block in iter(lambda: f.read(1 << 20), b''):
            h.update(block)
    return h.hexdigest()


def digest(value):
    return hashlib.sha256(json.dumps(value, sort_keys=True, separators=(',', ':'), ensure_ascii=True).encode()).hexdigest()


def rows(path):
    with path.open(encoding='utf8') as f:
        for line in f:
            yield json.loads(line)


def field_map(snapshot):
    out = collections.defaultdict(list)
    for item in snapshot.get('values') or []:
        out[item['name']].append(item['value'])
    return dict(out)


def compare(before, after, kind, path=(), tls=False, message_type=None, counts=None):
    counts = counts if counts is not None else collections.Counter()
    assert type(before) is type(after), (kind, path, 'old type changed')
    if isinstance(before, dict):
        assert before.keys() <= after.keys(), (kind, path, 'old key removed')
        for key in after.keys() - before.keys():
            allowed = False
            if kind == 'SDK' and tls:
                allowed = len(path) == 1 and path[0] in ['fields', 'session'] and key == 'Session Establishment'
                if len(path) == 3 and path[0] in ['fields', 'session'] and path[1] in ['Handshake Messages', 'TLS Handshakes']:
                    allowed = key in MESSAGE_TYPES and message_type in MESSAGE_TYPES[key]
            if kind == 'CLI' and tls and len(path) == 3 and path[0] in ['pdus', 'published_pdus'] and path[2] == 'fields':
                allowed = key in ALIASES
            assert allowed, (kind, path, 'noncontract addition', key)
            counts['new_map_keys'] += 1
        for key in before:
            compare(before[key], after[key], kind, path + (key,), tls, message_type, counts)
    elif isinstance(before, list):
        if path[-2:] == ('field_values', 'values'):
            old_names = {v['name'] for v in before}
            cursor = 0
            for value in after:
                if value['name'] in old_names:
                    assert cursor < len(before)
                    exact(value, before[cursor], str((kind, path, cursor)))
                    cursor += 1
                    counts['old_alias_occurrences'] += 1
                else:
                    name = value['name']
                    assert tls and name in ALIASES, (kind, path, name)
                    assert set(value) == {'name', 'value', 'origin'}, (kind, path, value)
                    origin = 'context' if name.startswith('tls.session.') else 'derived' if name in DERIVED_FIELDS else 'pdu'
                    assert value['origin'] == origin
                    counts['new_alias_occurrences'] += 1
            assert cursor == len(before), (kind, path, 'old alias removed')
        else:
            assert len(before) == len(after), (kind, path, 'old array length changed')
            for i, (a, b) in enumerate(zip(before, after)):
                child_tls, child_type = tls, message_type
                if kind == 'SDK' and len(path) == 2 and path[0] in ['fields', 'session'] and path[1] in ['Handshake Messages', 'TLS Handshakes']:
                    child_type = a['Type']
                if kind == 'CLI' and len(path) == 1 and path[0] in ['pdus', 'published_pdus']:
                    child_tls = a['protocol'] in ['tls', 'quic', 'http3', 'doq']
                compare(a, b, kind, path + (i,), child_tls, child_type, counts)
    else:
        assert before == after, (kind, path, 'old scalar changed', before, after)
        counts['old_scalar_values'] += 1
    return counts


def metadata_consistency(event):
    """Check projection agreement only; wire accuracy is a separate proof."""
    session = event.get('session') or {}
    messages = session.get('Handshake Messages') or session.get('TLS Handshakes') or []
    expected = collections.defaultdict(list)
    for message in messages:
        metadata = {k: v for k, v in message.items() if k in MESSAGE_TYPES}
        for key in metadata:
            assert type(message['Type']) is int and message['Type'] in MESSAGE_TYPES[key]
        for key, values in message_fields(message['Type'], metadata).items():
            expected[key].extend(values)
    context = session.get('Session Establishment') or {}
    for key, path in CONTEXT_PATHS.items():
        value = context
        for part in path:
            if not isinstance(value, dict) or part not in value:
                break
            value = value[part]
        else:
            expected['tls.session.' + key].append(value)
    actual = field_map(event['field_values'])
    exact({k: v for k, v in actual.items() if k in ALIASES}, dict(expected), 'metadata/alias projection ' + str(event['id']))
    assert not FORBIDDEN_ALIASES.intersection(actual)
    fields = event.get('fields')
    if fields is not None and event['protocol'] == 'tls':
        for key in ['Session Establishment', 'Handshake Messages', 'TLS Handshakes']:
            if key == 'Session Establishment' or key in fields:
                exact(fields.get(key), session.get(key), 'GetFields/session ' + str(event['id']) + '/' + key)
    elif fields is None and (context or any(k in m for m in messages for k in MESSAGE_TYPES)):
        assert event.get('fields_error') and event['status'] != 'decoded'
    # Native QUIC GetFields is its preexisting captured wire envelope. Its
    # decrypted TLS observations live in Session and field_values, rather than
    # being inserted into that byte-backed header tree. compare() preserves
    # the entire original envelope independently.


def corrected_source_event(before, packet):
    """The known original source-port-only case, not a general difference waiver."""
    assert packet['section'] == 0 and packet['interface'] == 225 and packet['link'] == 1
    wire = packet['wire']
    assert len(wire) == 83 and wire[12:14] == b'\x08\x00'
    # This original frame has fourteen captured octets after the IPv4 extent.
    # Preserve them in the physical CLI row; they are not UDP/VXLAN payload.
    extent = int.from_bytes(wire[16:18], 'big')
    assert extent == 55 and 14 + extent <= len(wire)
    ip = wire[14:14+extent]
    assert ip[0] == 0x45 and ip[9] == 17 and int.from_bytes(ip[2:4], 'big') == len(ip)
    assert int.from_bytes(ip[6:8], 'big') & 0x3fff == 0
    assert ip[12:16] == bytes([1,2,3,4]) and ip[16:20] == bytes([4,3,2,1])
    udp = ip[20:]
    assert int.from_bytes(udp[:2], 'big') == 4789 and int.from_bytes(udp[2:4], 'big') == 55810
    assert int.from_bytes(udp[4:6], 'big') == len(udp) == 35
    payload = udp[8:]
    assert len(payload) == 27 and payload[0] & 8 == 0
    assert base64.b64decode(before['raw'], validate=True) == payload
    assert before['protocol'] == '' and before['fields'] is None and before['session'] is None
    assert before['flow'] == 0 and before.get('tcp_flow_id', 0) == 0
    assert before['source_bytes']['SHA256'] == hashlib.sha256(payload).hexdigest()
    exact(before['domain'], {'Section':0,'Interface':225,'Encapsulation':'/vxlan:1.2.3.4~4.3.2.1:1381653'}, 'old erroneous VXLAN domain')
    assert before['source_bytes']['PacketRefs'] == [{'Number':48644,'Domain':before['domain']}]
    result = copy.deepcopy(before)
    result['domain']['Encapsulation'] = ''
    for source in [result['source_bytes'], result['field_values']['source']]:
        source['PacketRefs'][0]['Domain']['Encapsulation'] = ''
    result.update(status='unrecognized', completeness='unrecognized', expert='ProtocolNotRecognized',
                  summary='UDP application protocol not recognized (1.2.3.4:4789 -> 4.3.2.1:55810, 27 bytes)',
                  fields_error='protocol parser: event is unrecognized, not an exact message')
    del result['error']
    assert result['field_values'].pop('incomplete') is True
    result['field_values']['completeness'] = 'unrecognized'
    statuses = [v for v in result['field_values']['values'] if v['name']=='status']
    assert len(statuses)==1 and statuses[0]['value']=='incomplete'
    statuses[0]['value']='unrecognized'
    return result


def corrected_source_cli(before, event):
    result = copy.deepcopy(before)
    assert result['number'] == 48644 and len(result['pdus']) == 1 and not result.get('published_pdus')
    result['capture_domain']['Encapsulation'] = ''
    result['protocol_status'] = 'unrecognized'
    result['info'] = 'UDP carrier · unrecognized · '+event['summary']+' · 4789 → 55810 Payload=27 (port hint: VXLAN)'
    pdu = result['pdus'][0]
    assert pdu['id'] == event['id']
    for key in ['summary','expert','status','completeness']:
        pdu[key] = event[key]
    pdu.update(error='', field_values=event['field_values'], fields=field_map(event['field_values']), byte_source=event['source_bytes'])
    return result


def corrected_malformed_source_cli(before, packet):
    """Keep the native malformed UDP refusal; remove only a false VXLAN domain."""
    assert before['number'] in [48568,98505,133116]
    assert packet['section']==0 and packet['interface']==225 and packet['link']==1
    wire=packet['wire']
    assert wire[12:14]==b'\x08\x00'
    extent=int.from_bytes(wire[16:18],'big')
    ip=wire[14:14+extent]
    assert len(ip)==extent and ip[0]==0x45 and ip[9]==17 and int.from_bytes(ip[6:8],'big')&0x3fff==0
    assert ip[12:16]==bytes([1,2,3,4]) and ip[16:20]==bytes([4,3,2,1])
    udp=ip[20:]
    source,destination,declared=(int.from_bytes(udp[n:n+2],'big')for n in [0,2,4])
    assert source==4789 and destination!=4789 and declared>len(udp)>=16
    vni=int.from_bytes(udp[12:15],'big')
    exact(before['capture_domain'],{'Section':0,'Interface':225,'Encapsulation':f'/vxlan:1.2.3.4~4.3.2.1:{vni}'},'old malformed source-port-only domain')
    assert before['protocol']=='UDP' and len(before['pdus'])==1
    pdu=before['pdus'][0]
    summary=f'UDP length {declared} exceeds declared IP payload available to UDP ({len(udp)} bytes)'
    assert pdu['status']=='malformed' and pdu['summary']==summary
    info=f'UDP carrier · malformed · {summary} · {source} → {destination} Payload={len(udp)-8} (port hint: VXLAN)'
    assert before['info']==info+' [incomplete/undecoded]'
    result=copy.deepcopy(before)
    result['capture_domain']['Encapsulation']=''
    result['info']=info
    return result


def binding(event):
    return digest([event['field_values'],field_map(event['field_values']),event['source_bytes'],event['protocol'],event['status']])


def inspect(before_events, after_events, before_cli, after_cli, correction_packets=None):
    counts = collections.Counter()
    bindings, changed, corrective_event = {}, [], None
    for old, new in itertools.zip_longest(rows(before_events), rows(after_events)):
        assert old is not None and new is not None, 'SDK event count changed'
        identifier = new['id']
        assert type(identifier) is int and identifier > 0 and identifier not in bindings
        assert type(new['field_values']['pdu']) is int and new['field_values']['pdu'] == identifier
        exact(new['field_values']['source'], new['source_bytes'], 'SDK snapshot/source '+str(identifier))
        if correction_packets is not None and old['source_bytes']['PacketRefs'] and [p['Number'] for p in old['source_bytes']['PacketRefs']] == [48644] and old['transport']=='udp':
            assert corrective_event is None
            expected = corrected_source_event(old, correction_packets[48644])
            exact(new, expected, 'physical source-port-only VXLAN correction')
            corrective_event = new
            counts['source_port_only_VXLAN_corrections'] += 1
        else:
            compare(old, new, 'SDK', tls=tls_carrier(old), counts=counts)
        if old != new:
            changed.append(identifier)
        if tls_carrier(new):
            metadata_consistency(new)
        bindings[identifier] = binding(new)
        counts['SDK_events'] += 1
    if correction_packets is not None:
        assert corrective_event is not None
    occurrences = collections.Counter()
    for old, new in itertools.zip_longest(rows(before_cli), rows(after_cli)):
        assert old is not None and new is not None, 'CLI packet count changed'
        if corrective_event is not None and old.get('number') == 48644:
            exact(new, corrected_source_cli(old, corrective_event), 'source-port-only CLI correction')
        elif correction_packets is not None and old.get('number') in [48568,98505,133116]:
            exact(new, corrected_malformed_source_cli(old, correction_packets[old['number']]), 'source-port-only malformed UDP CLI domain correction')
            counts['malformed_source_port_only_VXLAN_CLI_domain_corrections'] += 1
        else:
            compare(old, new, 'CLI', counts=counts)
        for group in ['pdus','published_pdus']:
            for pdu in new.get(group) or []:
                identifier = pdu['id']
                assert type(identifier) is int and identifier in bindings
                actual = digest([pdu['field_values'],pdu['fields'],pdu['byte_source'],pdu['protocol'],pdu['status']])
                assert actual == bindings[identifier], ('CLI/SDK snapshot, fields or source mismatch',new['number'],identifier)
                occurrences[identifier] += 1
        counts['CLI_rows'] += 1
        if 'number' in new:
            counts['CLI_physical_packets'] += 1
    assert set(occurrences) == set(bindings), ('SDK publication missing from CLI',len(set(bindings)-set(occurrences)))
    return dict(counts, changed_SDK_events=len(changed), changed_PDU_ids=changed,
                CLI_PDU_occurrences_joined_to_exact_SDK=sum(occurrences.values()),
                every_SDK_event_published_in_CLI=True, all_preexisting_IDs_and_sources_literal=True,
                new_metadata_projection_consistency=True, new_metadata_wire_accuracy_claim=False)


def audit(native_root, cli_root):
    native = json.loads((native_root/'summary.json').read_text('utf8'))
    cli = json.loads((cli_root/'summary.json').read_text('utf8'))
    assert native['cases'] == cli['cases'] == 37
    assert native['source'] == cli['source']
    cli_by_capture = {r['capture']:r for r in cli['outputs']}
    assert len(cli_by_capture)==37
    results = []
    for producer in native['outputs']:
        if Path(producer['arguments'][0]).name != 'events.exe':
            continue
        other = cli_by_capture[producer['capture']]
        before_events, before_cli = Path(producer['previous_export']), Path(other['previous_export'])
        after_events = native_root/before_events.relative_to(before_events.parents[2])
        after_cli = cli_root/before_cli.relative_to(before_cli.parents[2])
        # Producer files name the old root but both exports preserve its group/case layout.
        capture = Path(producer['capture'])
        for p, expected in [(capture,producer['capture_SHA256']),(before_events,producer['previous_export_SHA256']),
                            (after_events,producer['export_SHA256']),(before_cli,other['previous_export_SHA256']),
                            (after_cli,other['export_SHA256']),(Path(producer['arguments'][0]),producer['binary_SHA256']),
                            (Path(other['arguments'][0]),other['binary_SHA256'])]:
            assert sha(p)==expected,(str(p),'producer hash mismatch')
        assert producer['exit_code']==other['exit_code']==0 and producer['source']==other['source']==native['source']
        packets = {n:p for n,p in enumerate(physical(capture),1) if n in [48568,48644,98505,133116]} if producer['capture_SHA256']==SHARON_SHA else None
        result = inspect(before_events,after_events,before_cli,after_cli,packets)
        result.update(capture_SHA256=producer['capture_SHA256'],SDK_SHA256=producer['export_SHA256'],CLI_SHA256=other['export_SHA256'],
                      old_SDK_SHA256=producer['previous_export_SHA256'],old_CLI_SHA256=other['previous_export_SHA256'],
                      SDK_export_path=str(after_events),CLI_export_path=str(after_cli),
                      SDK_producer=producer,CLI_producer=other)
        results.append(result)
        print(json.dumps({'capture':capture.name,**{k:v for k,v in result.items() if k in ['SDK_events','CLI_rows','CLI_physical_packets','changed_SDK_events','old_scalar_values','old_alias_occurrences']}}),flush=True)
    assert len(results)==37
    for producer in native['outputs']:
        if Path(producer['arguments'][0]).name != 'flows.exe':
            continue
        assert producer['complete_byte_identical_to_0c2d'] is True
        before = Path(producer['previous_export'])
        after = native_root/before.relative_to(before.parents[2])
        assert sha(after)==sha(before)==producer['export_SHA256']==producer['previous_export_SHA256']
    return dict(scope=__doc__,source=native['source'],auditor_SHA256=sha(Path(__file__)),
                cases=37,results=results,all_old_scalar_types_values_arrays_and_alias_order_preserved=True,
                generated_ID_translation=False,new_metadata_wire_accuracy_claim=False)


if __name__=='__main__':
    ap=argparse.ArgumentParser(description=__doc__)
    for key in ['native-root','cli-root','out']:
        ap.add_argument('--'+key,type=Path,required=True)
    args=ap.parse_args()
    proof=audit(args.native_root,args.cli_root)
    with args.out.open('x',encoding='utf8') as f:
        json.dump(proof,f,indent=2);f.write('\n')
