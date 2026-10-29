"""Preserve actual Windows byte-source/physical-frame scope disagreements."""
from pathlib import Path
import hashlib,importlib.util,json,re,struct
root=Path(__file__).parent
actual=root/'windows-SSH-role-26f50-01'
plan=json.loads((root/'Windows-SSH-role-source-plan50-01/plan.json').read_text('utf8'))
spec=importlib.util.spec_from_file_location('physical_hex_scope50',root/'verify_Windows_OSPFv3_36.py');m=importlib.util.module_from_spec(spec);spec.loader.exec_module(m)
def dg(b):return hashlib.sha256(b).hexdigest()
def read(p):return json.loads(Path(p).read_text('utf8'))
observations=[]
for case in plan['cases']:
 producer=read(actual/(case['name']+'-producer.json'))
 assert producer['exit_code']==0 and producer['inputs_unchanged']
 for path,h in producer['inputs'].items():assert dg(Path(path).read_bytes())==h
 capture=Path(case['capture']).read_bytes();at=24;frames=[]
 while at<len(capture):
  _,_,n,_=struct.unpack('<IIII',capture[at:at+16]);at+=16;frames.append(capture[at:at+n]);at+=n
 physical=frames[case['frame']-1];raw=Path(case['raw_path']).read_bytes();preview=raw[:case['retained_bytes']]
 assert len(physical)==case['full_physical_bytes']and physical[54:]==raw
 for name in('01-protocol-fields','01-selected-field','02-first-byte-source','02-byte-sources'):
  if name not in producer['snapshots']:continue
  p=actual/case['name']/(name+'.json');s=read(p);assert dg(p.read_bytes())==producer['snapshots'][name]['cells_SHA256']
  data,_=m.pane(s);text='\n'.join(s['Rows'])
  frame_lengths=re.search(r'Frame\s+'+str(case['frame'])+r':\s+(\d+) bytes on wire,\s+(\d+) captured',text)
  titles=[line.strip('─╭╮ ')for line in s['Rows']if'3 'in line and ('Captured frame'in line or'PDU'in line or'bytes'in line)]
  observations.append(dict(case=case['name'],snapshot=name,source_frame=case['frame'],
    expected_physical_bytes=len(physical),actual_hex_bytes=len(data),hex_SHA256=dg(data),
    equals_entire_physical=data==physical,equals_physical_prefix=data==physical[:len(data)],
    equals_entire_source_PDU=data==raw,equals_retained_preview=data==preview,
    displayed_frame_lengths=[int(x)for x in frame_lengths.groups()]if frame_lengths else None,
    right_titles=titles,snapshot_SHA256=dg(p.read_bytes())))
out=root/'Windows-SSH-role-scope-discovery50-01';out.mkdir(exist_ok=False)
with(out/'observations.json').open('x',encoding='utf8')as f:json.dump(observations,f,indent=2,ensure_ascii=False);f.write('\n')
print(json.dumps(observations,ensure_ascii=False))
