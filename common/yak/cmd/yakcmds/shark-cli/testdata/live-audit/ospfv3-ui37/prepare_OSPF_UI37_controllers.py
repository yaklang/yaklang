"""Create a new, separately frozen display audit; preserve the closed 636 audit."""
from pathlib import Path
import hashlib, json

root = Path(__file__).parent
current = '68527c4d9355d2df37c5c0c9b8bc19f7766ca9c6'
old = '636816f1da7c97228a6adf11cbc363a41c53c33f'
for stem in ('prepare', 'run', 'verify'):
    before = root / (stem + '_Windows_OSPFv3_36.py')
    after = root / (stem + '_Windows_OSPFv3_37.py')
    assert not after.exists()
    source = before.read_text('utf8')
    source = source.replace('_OSPFv3_36.py', '_OSPFv3_37.py')
    source = source.replace('Windows-OSPFv3-source-plan-before36-01', 'Windows-OSPFv3-source-plan-before37-01')
    source = source.replace("bins-36/yak-receipt.json", "bins-37/yak-receipt.json")
    if stem == 'prepare':
        assert source.count(old) == 1
        source = source.replace(old, current)
        anchor = "(out/'case.json').write_text"
        assert source.count(anchor) == 1
        source = source.replace(anchor, "case['required_human_type_names']=['LS Type: 0x2009 (Intra-Area-Prefix-LSA)', 'Referenced LS Type: 0x2001 (Router-LSA)']\n" + anchor)
    elif stem == 'verify':
        anchor = " first=shots['01-protocol-fields'];text='\\n'.join(first['Rows'])"
        assert source.count(anchor) == 1
        source = source.replace(anchor, " all_text='\\n'.join(row for s in shots.values() for row in s['Rows'])\n for label in case['required_human_type_names']:assert label in all_text,label\n" + anchor)
    after.write_text(source, encoding='utf8', newline='\n')
print(json.dumps({'controllers': [str(root/(s+'_Windows_OSPFv3_37.py')) for s in ('prepare','run','verify')], 'previous_proofs_unchanged': True}))
