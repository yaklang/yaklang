"""Reconstruct and bind generated-ID ledgers from source-proved closed exports.

This closure never creates protocol expectations. It verifies the exact files
already certified by the frozen whole-object bridge, independently reconstructs
both public ledgers, and binds their bytes for a later CLI consumer.
"""
from pathlib import Path
import hashlib,itertools,json

root=Path(__file__).parent
proof=root/'OSPFv3-native37-whole-636-01'
summary=proof/'summary.json'
def sha(p):
 h=hashlib.sha256()
 with p.open('rb')as f:
  for b in iter(lambda:f.read(1<<20),b''):h.update(b)
 return h.hexdigest()
def rows(p):
 with p.open(encoding='utf-8-sig')as f:
  for line in f:yield json.loads(line)
assert sha(summary)=='463250f963d5816b1e9b3565fb27ae3e444f74f4cae4455caa183d5727936022'
s=json.loads(summary.read_text('utf8'));assert s['unplanned_differences']==0 and s['event_objects']==307262
plan=root/'OSPFv3-native-whole-plan-before36-02'
assert sha(plan/'plan.json')==s['plan_SHA256']=='cdd52b2aa22d1a1d67b363cbf605932a123b5ccc911706ca47c0a631d4ee1e01'
cases={v['export']:v for v in json.loads((plan/'cases.json').read_text('utf8'))}
oldroot=root/'runtime-public-native-35';newroot=root/'runtime-public-native-36'
outputs=[]
for r in s['results']:
 if not r['export'].endswith('events.jsonl'):continue
 rel=Path(r['export']);old=oldroot/rel;new=newroot/rel
 assert sha(old)==cases[r['export']]['export_SHA256'] and sha(new)==r['export_SHA256']
 mapping={};added={};left=iter(rows(old))
 for e in rows(new):
  if e['protocol']=='ospf':
   refs=e['source_bytes']['PacketRefs'];assert len(refs)==1
   f=refs[0]['Number'];assert f not in added;added[f]=e['id']
  else:
   previous=next(left);assert previous['id']not in mapping;mapping[previous['id']]=e['id']
 assert next(left,None)is None
 assert len(mapping)==r['old_events'] and len(set(mapping.values()))==len(mapping)
 folder=proof/rel.parent
 paths=[]
 for name,expected in [('OSPF-generated-PDU-map.json',mapping),('OSPF-source-published-IDs.json',added)]:
  p=folder/name;actual=json.loads(p.read_text('utf8'))
  assert actual=={str(k):v for k,v in expected.items()}
  paths.append(dict(path=str(p),SHA256=sha(p),entries=len(expected)))
 assert sha(old)==cases[r['export']]['export_SHA256'] and sha(new)==r['export_SHA256']
 outputs.append(dict(export=r['export'],before_SHA256=sha(old),after_SHA256=sha(new),native_producer_SHA256=r['producer_SHA256'],ledgers=paths))
assert len(outputs)==37 and sum(r['ledgers'][1]['entries']for r in outputs)==807
out=root/'OSPFv3-native-ledger-closure36-01.json'
data=dict(scope=__doc__,source=s['source'],whole_native_summary=str(summary),whole_native_summary_SHA256=sha(summary),whole_native_plan=str(plan/'plan.json'),whole_native_plan_SHA256=sha(plan/'plan.json'),field_plan_SHA256='0f7caa92c06cf6971fe13cae804810b6cab4bbda38235d1f7cd78dd7a3326036',closer_SHA256=sha(Path(__file__)),closed_event_exports=37,reconstructed_generated_ID_ledgers=37,reconstructed_source_frame_ID_ledgers=37,source_insertions=807,all_maps_reconstructed_from_hash_bound_closed_exports=True,files=outputs)
with out.open('x',encoding='utf8')as f:json.dump(data,f,indent=2);f.write('\n')
print(json.dumps(dict(closure=str(out),SHA256=sha(out),exports=37,source_insertions=807)))
