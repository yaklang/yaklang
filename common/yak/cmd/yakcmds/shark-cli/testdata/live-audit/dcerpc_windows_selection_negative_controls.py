"""Reject pixel-data and ordinal highlight forgeries of actual native RPC cells.

Use the feature result whose zero UUID equals another ordinal's UUID. Matching
the displayed bytes alone cannot certify the selected wire occurrence.
"""
import argparse
import copy
import json
import re
from pathlib import Path

from dcerpc_windows_scoped_selection import audit,sha

ap=argparse.ArgumentParser(description=__doc__)
for name in['snapshot','receipts','out-dir']:ap.add_argument('--'+name,type=Path,required=True)
a=ap.parse_args()
for p in vars(a).values():assert not any((q/'.git').exists()for q in[p.resolve(),*p.resolve().parents])
baseline=audit(a.snapshot,a.receipts,'feature');state=json.loads(a.snapshot.read_text(encoding='utf8'))
a.out_dir.mkdir(exist_ok=False)
header=next(i for i,row in enumerate(state['Rows'])if '3 DCE/RPC'in row and 'reassembled PDU'in row)
positions=[]
for y in range(header+1,len(state['Rows'])):
    row=state['Rows'][y];match=re.search(r'000[0-9a-f]{3}  ',row[85:])
    if not match:continue
    at=85+match.start()
    for token in list(re.finditer(r'\b[0-9a-f]{2}\b',row[at+8:]))[:min(16,baseline['PDU_bytes']-len(positions))]:positions.append((y,at+8+token.start()))
assert len(positions)==baseline['PDU_bytes']
base_y,base_x=positions[0];plain_style=state['Attributes'][base_y][base_x]
select_y,select_x=positions[88];selected_style=state['Attributes'][select_y][select_x]
assert plain_style!=selected_style

def replace_cell(s,y,x,text):s['Rows'][y]=s['Rows'][y][:x]+text+s['Rows'][y][x+len(text):]
def paint(s,offset,style):
    y,x=positions[offset];s['Attributes'][y][x:x+2]=[style,style]
def wrong_zero_ordinal(s):
    for offset in range(88,104):paint(s,offset,plain_style)
    for offset in range(40,56):paint(s,offset,selected_style)
def wrong_result_name(s):
    for y,row in enumerate(s['Rows']):
        old='Result: 3 (Feature negotiation ACK)'
        if old in row:
            replace_cell(s,y,row.index(old),'Result: 0 (Acceptance)'.ljust(len(old)));return
    raise AssertionError('actual feature result is visible')

cases=[
    ('wire-byte',lambda s:replace_cell(s,*positions[0],'04')),
    ('wrong-PDU-interval',lambda s:s['Rows'].__setitem__(header,s['Rows'][header].replace('0x58–0x67','0x28–0x37'))),
    ('missing-highlight-byte',lambda s:paint(s,88,plain_style)),
    ('same-zero-UUID-wrong-ordinal',wrong_zero_ordinal),
    ('wrong-result-role',wrong_result_name),
]
rejected=[]
for name,mutate in cases:
    changed=copy.deepcopy(state);mutate(changed);path=a.out_dir/(name+'.json')
    path.write_text(json.dumps(changed,indent=2)+'\n',encoding='utf8')
    try:audit(path,a.receipts,'feature')
    except (AssertionError,KeyError,ValueError,IndexError,TypeError,StopIteration)as exc:rejected.append(dict(name=name,rejection=str(exc),snapshot_sha256=sha(path)))
    else:raise AssertionError('native console mutation accepted: '+name)
proof=dict(scope=__doc__,source_snapshot_sha256=sha(a.snapshot),receipts_sha256=sha(a.receipts),negative_controls_rejected=len(rejected),rejected=rejected)
(a.out_dir/'summary.json').write_text(json.dumps(proof,indent=2)+'\n',encoding='utf8')
print(json.dumps(dict(negative_controls_rejected=len(rejected))))
