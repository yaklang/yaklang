"""Verify actual native RPC hex cells and highlighted wire ordinals.

Derive token/negotiation intervals from independent endpoint receipt bytes.
Check every displayed PDU octet and require exactly the requested occurrence
to use the selected native style, including same-valued zero UUIDs.
"""
import argparse
import base64
import hashlib
import json
import re
from pathlib import Path


def sha(path):return hashlib.sha256(path.read_bytes()).hexdigest()


def audit(snapshot,receipts,case):
    records=json.loads(receipts.read_text(encoding='utf8'))
    for record in records:
        assert hashlib.sha256(base64.b64decode(record['wire_base64'],validate=True)).hexdigest()==record['sha256']
    operation='NTLM-negotiate-only'if case=='auth'else'NDR32-NDR64-feature-negotiation'
    direction='c2s'if case=='auth'else's2c'
    selected=[r for r in records if r['case']==operation and r['direction']==direction]
    assert len(selected)==1
    wire=base64.b64decode(selected[0]['wire_base64'],validate=True)
    assert wire[:2]==b'\x05\0'and wire[4:8]==bytes.fromhex('10000000')and int.from_bytes(wire[8:10],'little')==len(wire)
    if case=='auth':
        auth=int.from_bytes(wire[10:12],'little');assert auth>0
        start,end=len(wire)-auth,len(wire);assert start>=24
    else:
        assert wire[2]==12 and wire[10:12]==b'\0\0'
        result_list=(26+int.from_bytes(wire[24:26],'little')+3)&~3
        assert wire[result_list]==3 and result_list+4+3*24==len(wire)
        ordinal=1 if case=='NDR64' else 2
        result=result_list+4+24*ordinal;start,end=result+4,result+20
        if case=='NDR64':
            assert wire[result:result+4]==b'\0\0\0\0'and wire[start:end]==bytes.fromhex('33057171babe37498319b5dbef9ccc36')
            assert int.from_bytes(wire[end:end+4],'little')==1
        else:
            assert int.from_bytes(wire[result:result+2],'little')==3 and wire[start:end]==bytes(16)and wire[end:end+4]==bytes(4)
    state=json.loads(snapshot.read_text(encoding='utf8'))
    assert state['Width']==180 and state['Height']==48
    header=next(i for i,row in enumerate(state['Rows'])if '3 DCE/RPC'in row and 'reassembled PDU'in row)
    assert f'0x{start:x}–0x{end-1:x} · {end-start} bytes'in state['Rows'][header]
    tabs=next(i for i,row in enumerate(state['Rows'])if '[2 Fields]'in row and '[3 Bytes]'in row)
    tab_x=state['Rows'][tabs].index('[2 Fields]')+1
    selected_style=state['Attributes'][tabs][tab_x]
    actual=bytearray();highlighted=[]
    for y in range(header+1,len(state['Rows'])):
        row=state['Rows'][y];match=re.search(r'000[0-9a-f]{3}  ',row[85:])
        if not match:continue
        at=85+match.start();assert int(row[at:at+6],16)==len(actual),'all actual native PDU rows remain ordered'
        tokens=list(re.finditer(r'\b[0-9a-f]{2}\b',row[at+8:]))[:min(16,len(wire)-len(actual))]
        for token in tokens:
            x=at+8+token.start();position=len(actual);actual.append(int(token.group(),16))
            style=state['Attributes'][y][x:x+2]
            assert len(style)==2 and style[0]==style[1],'both hex characters have one native visual state'
            if style[0]==selected_style:highlighted.append(position)
    assert bytes(actual)==wire,'complete displayed native PDU equals actual endpoint bytes'
    assert highlighted==list(range(start,end)),'exact native highlight belongs to the intended wire ordinal'
    visible='\n'.join(state['Rows'])
    if case=='NDR64':assert '71710533-beba-4937-8319-b5dbef9ccc36'in visible and 'Result: 0 (Acceptance)'in visible
    if case=='feature':assert 'Result: 3 (Feature negotiation ACK)'in visible and '0x0003 (observed feature bits)'in visible
    return dict(scope=__doc__,case=case,snapshot_sha256=sha(snapshot),receipts_sha256=sha(receipts),PDU_sha256=hashlib.sha256(wire).hexdigest(),PDU_bytes=len(wire),field_bytes=end-start,PDU_range=[start,end],highlighted_native_byte_offsets=highlighted,complete_native_hex_exact=True,exact_ordinal_style=True,offset_scope='reassembled PDU')


if __name__=='__main__':
    ap=argparse.ArgumentParser(description=__doc__)
    for name in['snapshot','receipts','out']:ap.add_argument('--'+name,type=Path,required=True)
    ap.add_argument('--case',choices=['auth','NDR64','feature'],required=True);a=ap.parse_args()
    for p in[a.snapshot,a.receipts,a.out]:assert not any((q/'.git').exists()for q in[p.resolve(),*p.resolve().parents])
    result=audit(a.snapshot,a.receipts,a.case)
    a.out.write_text(json.dumps(result,indent=2)+'\n',encoding='utf8')
    print(json.dumps({k:result[k]for k in['case','PDU_bytes','field_bytes','PDU_range','complete_native_hex_exact','exact_ordinal_style']}))
