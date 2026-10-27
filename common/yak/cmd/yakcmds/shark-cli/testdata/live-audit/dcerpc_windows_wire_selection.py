"""Check actual native Windows RPC byte selection against endpoint wire bytes.

This consumes unmodified console cells and attributes after a real mouse click.
The selected token has an independently known PDU offset, unlike extracted
decoded values that have no captured byte range. All artifacts stay outside Git.
"""
import argparse
import base64
import hashlib
import json
import re
from pathlib import Path

ap=argparse.ArgumentParser(description=__doc__)
for name in ['before','selected','receipts','out']:ap.add_argument('--'+name,type=Path,required=True)
a=ap.parse_args()
for p in vars(a).values():assert not any((q/'.git').exists()for q in[p.resolve(),*p.resolve().parents])
receipts=json.loads(a.receipts.read_text(encoding='utf8'))
raws=[base64.b64decode(r['wire_base64'],validate=True)for r in receipts if r['direction']=='c2s'and int.from_bytes(base64.b64decode(r['wire_base64'])[10:12],'little')]
assert len(raws)==1,'one independently captured client security envelope'
raw=raws[0];auth=int.from_bytes(raw[10:12],'little');start=len(raw)-auth
before=json.loads(a.before.read_text(encoding='utf8'));selected=json.loads(a.selected.read_text(encoding='utf8'))
assert before['Width']==selected['Width']==180 and before['Height']==selected['Height']==48
header=next(i for i,row in enumerate(selected['Rows'])if '3 DCE/RPC' in row and 'reassembled PDU'in row)
assert f'0x{start:x}–0x{len(raw)-1:x} · {auth} bytes'in selected['Rows'][header],'actual selected PDU-relative byte interval'
wire=bytearray();changed=[]
for y in range(header+1,len(selected['Rows'])):
    row=selected['Rows'][y]
    match=re.search(r'000[0-9a-f]{3}  ',row[85:])
    if not match:continue
    at=85+match.start();offset=int(row[at:at+6],16)
    assert offset==len(wire),'all displayed PDU bytes remain contiguous'
    tokens=list(re.finditer(r'\b[0-9a-f]{2}\b',row[at+8:]))[:min(16,len(raw)-len(wire))]
    for token in tokens:
        x=at+8+token.start();position=len(wire);wire.append(int(token.group(),16))
        old=before['Attributes'][y][x:x+2];new=selected['Attributes'][y][x:x+2]
        assert len(old)==len(new)==2 and old[0]==old[1]and new[0]==new[1],'whole native hex byte has one visual state'
        if old!=new:changed.append(position)
assert bytes(wire)==raw,'all actual native PDU hex cells equal endpoint send bytes'
assert changed==list(range(start,len(raw))),'mouse highlight changes exactly the complete authentication token range'
proof=dict(scope=__doc__,before_sha256=hashlib.sha256(a.before.read_bytes()).hexdigest(),selected_sha256=hashlib.sha256(a.selected.read_bytes()).hexdigest(),receipts_sha256=hashlib.sha256(a.receipts.read_bytes()).hexdigest(),PDU_bytes=len(raw),token_bytes=auth,token_range=[start,len(raw)],complete_native_hex_exact=True,highlighted_native_byte_offsets=changed,offset_scope='reassembled PDU')
a.out.write_text(json.dumps(proof,indent=2)+'\n',encoding='utf8')
print(json.dumps({k:proof[k]for k in ['PDU_bytes','token_bytes','token_range','complete_native_hex_exact','offset_scope']}))
