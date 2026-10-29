"""Diagnose first failed immutable source/PDU binding; emit hashes only."""
from pathlib import Path
import base64,collections,hashlib,json,struct
root=Path(__file__).parent
plan=root/'SSH-role-API-CLI-source50-01/plan01'
rows=json.loads((plan/'source-rows.json').read_text('utf8'));byframe={r['frame']:r for r in rows}
cap=Path(json.loads((root/'SSH-role-source50-01/plan02/manifest.json').read_text('utf8'))['capture']['path']).read_bytes()
at=24;physical={}
while at<len(cap):
 _,_,n,_=struct.unpack('<IIII',cap[at:at+16]);at+=16
 physical[len(physical)+1]=cap[at:at+n][54:];at+=n
def dg(b):return hashlib.sha256(b).hexdigest()
facts=[]
for label in('SSH-role-before-cc9d50-01','SSH-role-native-26f50-01'):
 for line in(root/label/'events.jsonl').read_text('utf8').splitlines():
  e=json.loads(line)
  if not e.get('raw'):continue
  refs=e['source_bytes'].get('PacketRefs')or[]
  if len(refs)!=1:continue
  f=refs[0]['Number'];r=byframe[f];wire=Path(r['raw_path']).read_bytes();actual=base64.b64decode(e['raw'])
  if wire!=actual:
   facts.append(dict(run=label,frame=f,case=r['sourcecase'],kind=r['kind'],expected_bytes=len(wire),actual_bytes=len(actual),
    expected_SHA256=dg(wire),actual_SHA256=dg(actual),physical_SHA256=dg(physical[f]),
    expected_equals_source_physical=wire==physical[f],actual_equals_source_physical=actual==physical[f],
    captured_source_SHA256=e['source_bytes']['SHA256'],source_equal_frames=[n for n,w in physical.items()if w==actual]))
print(json.dumps(facts[:8]))
with(root/'SSH-role-binding-discovery50-01.json').open('x',encoding='utf8')as out:json.dump(dict(mismatches=facts),out,indent=2);out.write('\n')
