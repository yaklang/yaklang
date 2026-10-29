"""Prove literal checksum owner/range and retain normative reference differences.

Raw field equality does not prove checksum applicability semantics. This
checks the actual PDML ancestor and original-byte span, and compares the
official v4.6.9 reported-length checksum path to bounded RFC packet math.
"""
import argparse,hashlib,importlib.util,json,xml.etree.ElementTree as ET
from pathlib import Path


def sha(p):return hashlib.sha256(Path(p).read_bytes()).hexdigest()
def read(p):return json.loads(Path(p).read_text('utf8'))
def main():
    ap=argparse.ArgumentParser(description=__doc__)
    for n in('source','reference','official-source','codec','out'):ap.add_argument('--'+n,type=Path,required=True)
    a=ap.parse_args();assert not a.out.exists()
    assert sha(a.official_source)=='2db8f577f2dd857cbc2b4d73c79112c6256b9bb91eda6a72c6d78f62f04d0a53'
    spec=importlib.util.spec_from_file_location('source_crc_math',a.codec);codec=importlib.util.module_from_spec(spec);spec.loader.exec_module(codec)
    assert sha(a.codec)=='fb45475fec822b21989d398541c51ec5d62841db4ce75d2e76737fbf1bffb662'
    cases={r['name']:r for r in read(a.source/'cases.json')};records=[]
    for name in('LLS-only-good','LLS-only-bad-block-sum','AT-plus-LLS-dummy-digest'):
        case=cases[name];raw=Path(case['original_frame']).read_bytes();assert sha(case['original_frame'])==case['original_frame_SHA256']
        message=raw[54:];n=int.from_bytes(message[2:4],'big');assert n==36
        pdml=a.reference/name/'wireshark.pdml';tree=ET.parse(pdml).getroot();parents={child:parent for parent in tree.iter()for child in parent}
        fields=[]
        for node in tree.iter('field'):
            if node.get('name')not in('ospf.checksum','ospf.lls.checksum'):continue
            ancestry=[];p=parents.get(node)
            while p is not None:
                if p.tag in('field','proto'):ancestry.append(dict(tag=p.tag,name=p.get('name'),pos=p.get('pos'),size=p.get('size')))
                p=parents.get(p)
            fields.append(dict(attributes=node.attrib,ancestors=ancestry))
        header=next(r for r in fields if r['attributes']['name']=='ospf.checksum')
        ll=next(r for r in fields if r['attributes']['name']=='ospf.lls.checksum')
        assert header['attributes']['pos']=='66'and header['attributes']['size']=='2'and header['ancestors'][0]['name']=='ospf.header'
        assert ll['attributes']['pos']=='90'and ll['attributes']['size']=='2'
        assert bytes.fromhex(header['attributes']['value'])==raw[66:68]and bytes.fromhex(ll['attributes']['value'])==raw[90:92]
        source=read(case['source_facts']);oracle=source['independent_source_codec'];pseudo=raw[22:54]+n.to_bytes(4,'big')+bytes([0,0,0,89])
        zeroed=bytearray(message);zeroed[12:14]=bytes(2)
        prefix_shouldbe=codec.checksum(pseudo+bytes(zeroed[:n]))
        reported_shouldbe=codec.checksum(pseudo+bytes(zeroed))
        if name=='LLS-only-good':assert oracle['source_checksum']['mathematical_match']and '[correct]'in header['attributes']['showname']
        else:
            assert '[incorrect,'in header['attributes']['showname']
            assert '0x%04x'%reported_shouldbe in header['attributes']['showname']
        records.append(dict(case=name,packet_header_checksum_physical_range=[66,68],packet_header_checksum_PDU_range=[12,14],
            LLS_checksum_physical_range=[90,92],LLS_checksum_PDU_range=[36,38],actual_fields=fields,
            conditional_RFC_OSPFn_mathematical_match=oracle['source_checksum']['mathematical_match'],
            bounded_OSPFn_computed_checksum=prefix_shouldbe,whole_reported_payload_computed_checksum=reported_shouldbe,
            AT_observed=oracle['options_AT_bit'],LLS_observed=oracle['options_LLS_bit'],PDML_SHA256=sha(pdml),original_frame_SHA256=case['original_frame_SHA256']))
    good=Path(cases['LLS-only-good']['original_frame']).read_bytes();bad=Path(cases['LLS-only-bad-block-sum']['original_frame']).read_bytes()
    assert good[54:90]==bad[54:90]and good[:90]==bad[:90]
    report=dict(scope=__doc__,source_plan_SHA256=sha(a.source/'plan.json'),reference_summary_SHA256=sha(a.reference/'summary.json'),
        official_source_URL='https://raw.githubusercontent.com/wireshark/wireshark/v4.6.9/epan/dissectors/packet-ospf.c',
        official_source_version_commit='v4.6.9-0-g2d548b197c75',official_source_SHA256=sha(a.official_source),
        official_source_lines=dict(header_field_owner=1566,reported_length=1579,nonzero_sum_gate=1580,OSPFn_pseudo_length=1610,checksum_payload_uses_reported_length=1613,LLS_checksum_not_verified=1933),
        records=records,normative_reference_disagreements=2,
        conclusions=['Bad LLS block changes Wireshark OSPF header-checksum annotation although its OSPFn prefix is identical and source sum matches;RFC5613 requires OSPF packet processing independent of discarding bad LLS.',
                     'Observed AT with nonzero checksum is annotated bad by reference despite RFC7166 requiring omission of header checksum verification;dummy HMAC does not authenticate anything.'],
        normative_references=['https://www.rfc-editor.org/rfc/rfc5340.html#appendix-A.3.1','https://www.rfc-editor.org/rfc/rfc5613.html#section-2.2','https://www.rfc-editor.org/rfc/rfc7166.html#section-4.2'],
        raw_header_field_comparison_differences=0,checksum_semantics_not_claimed_equal=True,driver_SHA256=sha(__file__),candidate_output_read=False)
    a.out.write_text(json.dumps(report,indent=2)+'\n',encoding='utf8')
    print(json.dumps(dict(proof=str(a.out),SHA256=sha(a.out),normative_reference_disagreements=2,owner_and_ranges_verified=True)),flush=True)


if __name__=='__main__':main()
