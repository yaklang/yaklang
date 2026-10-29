"""Compare actual short-LSA console sources/highlight with frozen wire facts."""
from pathlib import Path
import hashlib,json,re
from verify_Windows_OSPFv3_36 import pane
def read(p):return json.loads(Path(p).read_text('utf-8-sig'))
def verify(folder,case):
    shots={p.stem:read(p)for p in folder.glob('*.json')if 'Rows'in read(p)}
    required={'01-protocol-fields','01-selected-field','02-first-byte-source','02-byte-sources','05-narrow','06-restored'}
    assert required<=shots.keys()
    for s in shots.values():
        assert len(s['Rows'])==len(s['Attributes'])==s['Height']
        assert all(len(r)==len(a)==s['Width']for r,a in zip(s['Rows'],s['Attributes']))
    first=shots['01-protocol-fields'];text='\n'.join(first['Rows'])
    assert ' EOF 'in first['Rows'][0] and case['query']in first['Rows'][1]
    assert re.search(r'buffer\s+1\s*/\s*20000',text)
    assert re.search(r'· 1 visible(?:\s|╮)',text)
    assert any(re.search(r'›1\s+\S+\s+OSPFv3(?:\s|$)',r)for r in first['Rows'])
    assert ' › '.join(case['progressive_stack_prefix'])in text
    raw=Path(case['physical_file']).read_bytes();assert hashlib.sha256(raw).hexdigest()==case['physical_SHA256']
    wire=raw[slice(*case['PDU_range'])];assert hashlib.sha256(wire).hexdigest()==case['PDU_SHA256']
    assert pane(first)[0]==raw
    selected=shots['01-selected-field'];joined='\n'.join(selected['Rows'])
    assert case['field']in joined and '0x2e–0x2f · 2 bytes'in joined
    assert 'enclosing IPv6 pseudo-header not supplied'not in '\n'.join(r for s in shots.values()for r in s['Rows'])
    data,pos=pane(selected);assert data==wire
    a,b=case['field_PDU_range'];attrs=[selected['Attributes'][y][x]for y,x in pos];style=attrs[a]
    assert all(v==style for v in attrs[a:b])and all(v!=style for i,v in enumerate(attrs)if not a<=i<b)
    assert data[a:b].hex()==case['field_wire_hex']
    assert pane(shots['02-first-byte-source'])[0]==raw and pane(shots['02-byte-sources'])[0]==wire
    assert read(folder/'restored.json')==dict(error='<nil>',input_cp=936,output_cp=936)
    return dict(physical_frame=1,PDU=1,physical_captured_bytes=116,physical_original_bytes=122,PDU_captured_bytes=62,field_PDU_range=[46,48],field_physical_range=[100,102],exact_sources_and_highlight=True,authored_control_not_live_traffic=True,default_history_mib=64,default_packet_cache=20000,no_key_or_authentication_claim=True,snapshots=len(shots))
