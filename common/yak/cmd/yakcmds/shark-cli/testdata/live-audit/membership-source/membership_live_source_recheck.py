"""Post-execution independent Wireshark source recheck of fresh Windows membership traffic."""
import argparse, collections, hashlib, importlib.util, json, subprocess, sys
from pathlib import Path
import xml.etree.ElementTree as ET


def sha(p):
    return hashlib.sha256(Path(p).read_bytes()).hexdigest()


def load(name, path):
    spec = importlib.util.spec_from_file_location(name, path)
    result = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(result)
    return result


def main():
    ap = argparse.ArgumentParser(description=__doc__)
    for name in ('capture', 'tshark', 'audit-modules', 'igmp-modules', 'mld-modules', 'out'):
        ap.add_argument('--' + name, type=Path, required=True)
    a = ap.parse_args()
    a.out.mkdir(exist_ok=False)
    sys.path[:0] = [str(a.audit_modules), str(a.igmp_modules), str(a.mld_modules)]
    from vxlan_endpoint_projection_audit import capture_records
    from igmp_carrier_source import locate
    from igmp_expected import decode as igmp_decode
    from mld_literal_source import inspect as mld_inspect
    mld_decode = load('live_mld_expected', a.mld_modules/'MLD-whole-native-prep-03'/'mld_expected.py').decode
    records = list(capture_records(a.capture))
    args = [str(a.tshark), '-n', '-r', str(a.capture), '-T', 'pdml']
    pdml = a.out/'wireshark.pdml'
    with pdml.open('xb') as stream, (a.out/'wireshark.stderr').open('xb') as err:
        result = subprocess.run(args, stdout=stream, stderr=err, creationflags=0x08000000)
    assert result.returncode == 0
    packets = ET.parse(pdml).getroot().findall('packet')
    assert len(packets) == len(records) == 6
    facts, comparisons, unavailable = [], [], []
    standard_igmp = {'igmp.type', 'igmp.checksum', 'igmp.max_resp', 'igmp.maddr', 'igmp.s', 'igmp.qrv', 'igmp.qqic', 'igmp.num_src', 'igmp.saddr', 'igmp.num_grp_recs', 'igmp.record_type', 'igmp.aux_data_len', 'igmp.aux_data'}
    for frame, (record, packet) in enumerate(zip(records, packets), 1):
        assert int(packet.find(".//field[@name='frame.number']").get('show')) == frame
        assert int(packet.find(".//field[@name='frame.cap_len']").get('show')) == len(record['wire'])
        at = locate(record)
        if at['protocol'] == 'stopped' and at['next_protocol'] == 58:
            # The IGMP walker deliberately stops at every other next-header.
            # Select the observed ICMPv6 type below from its literal input.
            at['protocol'] = 'icmpv6'
        assert at['protocol'] in ('igmp', 'icmpv6')
        wire = record['wire'][at['offset']:at['end']]
        network = next(h for h in reversed(at['headers']) if h['kind'] in ('IPv4', 'IPv6'))
        if at['protocol'] == 'igmp':
            declared = network['declared_length'] - network['length']
            expected = igmp_decode(wire, declared)
            fields = [v for v in expected['values'] if v['name'] in standard_igmp]
        else:
            assert wire[0] in (130, 131, 132, 143)
            # The public locator retains IPv6 payload length as declared_length.
            declared = 40+network['declared_payload']-(at['offset']-network['offset'])
            expected = mld_decode(wire, declared)
            fields = mld_inspect(wire)['fields']
            fields = [dict(name=name, offset=offset, length=size, value=int.from_bytes(wire[offset:offset+size], 'big')) for name, offset, size in [('icmpv6.type', 0, 1), ('icmpv6.code', 1, 1), ('icmpv6.checksum', 2, 2)]] + fields
        assert expected['status'] == 'decoded'
        for item in fields:
            name, size = item['name'], item['length']
            position = at['offset']+item['offset']
            nodes = [f for f in packet.iter('field') if f.get('name') == name and f.get('pos') == str(position) and f.get('size') == str(size)]
            observation = dict(frame=frame, name=name, position=position, size=size, expected=item['value'])
            if not nodes:
                unavailable.append(dict(observation, relation='WS field absent; not equality'))
                continue
            word = record['wire'][position:position+size]
            assert any(bytes.fromhex(f.get('unmaskedvalue') or f.get('value') or '') == word for f in nodes), observation
            # These owned reports contain whole words and addresses, no encoded
            # query timers or masked flag aliases. Prove the logical scalar too.
            import ipaddress
            if 'address' in name or name in ('igmp.maddr', 'igmp.saddr'):
                actual = str(ipaddress.ip_address(word))
            elif name == 'igmp.aux_data':
                actual = word.hex()
            else:
                actual = int.from_bytes(word, 'big')
            assert type(actual) is type(item['value']) and actual == item['value'], observation
            comparisons.append(dict(observation, wire_hex=word.hex(), relation='exact position, length, raw word and scalar'))
        facts.append(dict(frame=frame, record={k:v for k,v in record.items() if k != 'wire'}, wire_SHA256=hashlib.sha256(record['wire']).hexdigest(),
                          location=at, declared_bytes=declared, codec_expected=expected))
    (a.out/'source-expectations.jsonl').write_text(''.join(json.dumps(v, sort_keys=True)+'\n' for v in facts), encoding='utf8')
    (a.out/'comparisons.jsonl').write_text(''.join(json.dumps(v, sort_keys=True)+'\n' for v in comparisons), encoding='utf8')
    (a.out/'unavailable.json').write_text(json.dumps(unavailable, indent=2)+'\n', encoding='utf8')
    summary = dict(scope=__doc__, capture_SHA256=sha(a.capture), capture_records=len(records),
                   selected=dict(collections.Counter(v['location']['protocol'] for v in facts)),
                   equal_fields=len(comparisons), unavailable_fields=len(unavailable), differences=0,
                   source_ledger_SHA256=sha(a.out/'source-expectations.jsonl'), comparisons_SHA256=sha(a.out/'comparisons.jsonl'),
                   TShark_arguments=args, TShark_SHA256=sha(a.tshark), TShark_exit=result.returncode, PDML_SHA256=sha(pdml),
                   auditor_SHA256=sha(__file__), limits='Owned OS-generated reports only; no query timer, checksum validation, membership state or peer validation claim; checksum values alone are compared. Candidate Go/SDK/CLI execution remains separate.')
    (a.out/'summary.json').write_text(json.dumps(summary, indent=2, sort_keys=True)+'\n', encoding='utf8')
    print(json.dumps(summary), flush=True)


if __name__ == '__main__':
    main()
