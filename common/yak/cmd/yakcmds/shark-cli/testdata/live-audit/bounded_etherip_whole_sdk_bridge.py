"""Source-prove EtherIP parent refusals and every old/new generated PDU reference."""
import argparse,collections,copy,hashlib,itertools,json,sys
from pathlib import Path
from bounded_etherip_refusal_expected import expected
ap=argparse.ArgumentParser(description=__doc__)
for name in('before','after','source-inventory','bins','audit-modules','source-modules','capture','out'):ap.add_argument('--'+name,type=Path,required=True)
ap.add_argument('--source-after',required=True)
a=ap.parse_args();a.out.mkdir(exist_ok=False);sys.path.insert(0,str(a.audit_modules));sys.path.insert(0,str(a.source_modules))
from known_pdu_reference_paths_v2 import remap_known_pdu_references
from icmp4_expected_source import same
from vxlan_endpoint_projection_audit import capture_records
def sha(p):
 h=hashlib.sha256()
 with p.open('rb')as f:
  for b in iter(lambda:f.read(1<<20),b''):h.update(b)
 return h.hexdigest()
def canonical(v):return json.dumps(v,sort_keys=True,separators=(',',':'))
def frame(e):
 refs=e['source_bytes'].get('PacketRefs')or[]
 return refs[0]['Number']if e['source_bytes']['Kind']=='captured'and len(refs)==1 else None
def anchor(e):
 s=e['source_bytes']
 return(e['protocol'],e['transport'],e['profile'],e['status'],canonical(e['raw']),s['Kind'],s['SHA256'],canonical(s.get('PacketRefs')),canonical(s.get('CompletedIn')))
meta=json.loads((a.source_inventory/'summary.json').read_text());ledger=a.source_inventory/'source-parents.jsonl';assert meta['source_ledger_SHA256']==sha(ledger)
rows={v['frame']:v for v in map(json.loads,ledger.open())};assert len(rows)==17532 and {v['corpus']for v in rows.values()}=={'sharon'}
records={};capture_hash=sha(a.capture)
for number,r in enumerate(capture_records(a.capture),1):
 if number not in rows:continue
 r['frame']=number;v=rows[number];assert v['capture_SHA256']==capture_hash and v['wire_SHA256']==hashlib.sha256(r['wire']).hexdigest();records[number]=r
assert records.keys()==rows.keys()
built=json.loads((a.bins/'events-receipt.json').read_text());assert built['source']==a.source_after and built['exit_code']==0 and built['binary_SHA256']==sha(a.bins/'events.exe')
files=[];all_expected=[];all_mappings={};bad=[];cases=sorted(a.before.glob('*/*/events-producer.json'));assert len(cases)==37
for op in cases:
 np=a.after/op.relative_to(a.before);om=json.loads(op.read_text());nm=json.loads(np.read_text());old=op.with_name('events.jsonl');new=np.with_name('events.jsonl');name=op.parent.name
 assert om['source']=='5403ffe55b219702af97a3422dd6d59996aadfbe'and nm['source']==built['source']and om['exit_code']==nm['exit_code']==0
 assert om['capture_SHA256']==nm['capture_SHA256']and om['keylog_used']==nm['keylog_used']and om['arguments'][1:]==nm['arguments'][1:]
 assert om['export_SHA256']==sha(old)and nm['export_SHA256']==sha(new)and nm['binary_SHA256']==built['binary_SHA256']
 if sha(old)==sha(new):
  count=sum(1 for _ in old.open('rb'));files.append(dict(case=name,old_rows=count,new_rows=count,literal_entire_export=True,SHA256=sha(old)));continue
 assert name=='sharon','only source-inventoried EtherIP parents may change a public export'
 before=[json.loads(v)for v in old.open()];after=[json.loads(v)for v in new.open()];mapping={};old_targets={};new_targets={};left=[];right=[]
 for e in before:
  f=frame(e)
  if f in records:
   assert e['protocol']in('arp','ip')and f not in old_targets;old_targets[f]=e
  else:left.append(e)
 for e in after:
  f=frame(e)
  if f in records:
   assert f not in new_targets;new_targets[f]=e
  else:right.append(e)
 assert new_targets.keys()==records.keys()and len(left)==len(right)
 for oe,ne in zip(left,right):
  assert anchor(oe)==anchor(ne),'ordered unchanged physical-source/PDU anchor mismatch'
  assert oe['id']not in mapping;mapping[oe['id']]=ne['id']
 for f,oe in old_targets.items():assert oe['id']not in mapping;mapping[oe['id']]=new_targets[f]['id']
 assert len(mapping)==len(before)and len(set(mapping.values()))==len(mapping)
 targets=[]
 for f,ne in new_targets.items():
  want=expected(ne['id'],records[f]);delta=[k for k in ne.keys()|want.keys()if not same(ne.get(k),want.get(k))]
  if delta:bad.append(dict(frame=f,id=ne['id'],keys=delta,expected_domain=want['domain'],actual_domain=ne['domain']))
  targets.append(dict(frame=f,expected_new_event=want,old_id=old_targets[f]['id']if f in old_targets else None))
 remapped=0
 for oe,ne in zip(left,right):
  want=remap_known_pdu_references(oe,mapping)
  if not same(want,ne):bad.append(dict(old_id=oe['id'],new_id=ne['id'],protocol=oe['protocol'],keys=[k for k in ne.keys()|want.keys()if not same(ne.get(k),want.get(k))]))
  remapped+=1
 assert len(targets)==17532 and len(old_targets)==6351
 all_expected+=targets;all_mappings[name]=mapping
 (a.out/(name+'-pdu-map.json')).write_text(json.dumps(mapping,sort_keys=True)+'\n',encoding='utf8')
 files.append(dict(case=name,old_rows=len(before),new_rows=len(after),retained_source_anchors=remapped,old_parent_observations_replaced=len(old_targets),source_built_parents=len(targets),new_observations=len(targets)-len(old_targets),before_SHA256=sha(old),after_SHA256=sha(new),whole_source_expected_errors=len(bad)))
(a.out/'parent-expected.jsonl').write_text(''.join(json.dumps(v,sort_keys=True)+'\n'for v in all_expected),encoding='utf8')
(a.out/'mismatches.jsonl').write_text(''.join(json.dumps(v,sort_keys=True)+'\n'for v in bad),encoding='utf8')
flows=[]
for old in a.before.glob('*/*/flows.json'):
 new=a.after/old.relative_to(a.before);x,y=json.loads(old.read_text()),json.loads(new.read_text());keys=[k for k in x.keys()|y.keys()if not same(x.get(k),y.get(k))];flows.append(dict(case=old.parent.name,entire_literal=sha(old)==sha(new),changed_top_keys=keys,before_SHA256=sha(old),after_SHA256=sha(new)))
negative=[]
sample=all_expected[0]['expected_new_event']
for name,mutate in[
 ('invented-child-ARP',lambda e:e.__setitem__('protocol','arp')),
 ('decoded-child-status',lambda e:e.__setitem__('status','decoded')),
 ('new-child-wire-bytes',lambda e:e.__setitem__('raw','AA==')),
 ('false-completion',lambda e:e['source_bytes'].__setitem__('CompletedIn',copy.deepcopy(e['source_bytes']['PacketRefs'][0]))),
 ('forged-source-domain',lambda e:e['domain'].__setitem__('Encapsulation','/vlan:4095')),
 ('wrong-pdu-ID-type',lambda e:e.__setitem__('id',str(e['id']))),
 ('invented-ARP-field',lambda e:e['field_values']['values'].append(dict(name='arp.opcode',value=1,origin='pdu')))]:
 forged=copy.deepcopy(sample);mutate(forged);assert not same(sample,forged);negative.append(name)
summary=dict(scope=__doc__,source_before='5403ffe55b219702af97a3422dd6d59996aadfbe',source_after=built['source'],generations=len(files),old_rows=sum(v['old_rows']for v in files),new_rows=sum(v['new_rows']for v in files),source_built_parents=len(all_expected),whole_expected_errors=len(bad),error_examples=bad[:8],flows=flows,source_inventory_SHA256=sha(a.source_inventory/'summary.json'),parent_expected_SHA256=sha(a.out/'parent-expected.jsonl'),constructor_SHA256=sha(Path(__file__).with_name('bounded_etherip_refusal_expected.py')),auditor_SHA256=sha(Path(__file__)),known_reference_helper_SHA256=sha(a.audit_modules/'known_pdu_reference_paths_v2.py'),negative_controls=negative,namespace_scope='Original declared IPv4/EtherIP/Ethernet envelopes bound the child namespace; PPPoE payload length must fit its current input. Extra backing capacity and uint16 overflow cannot authorize fields. Namespace observation does not certify a valid PPPoE version/type or session.',files=files)
(a.out/'summary.json').write_text(json.dumps(summary,indent=2,sort_keys=True)+'\n',encoding='utf8')
print(json.dumps(dict(summary=str(a.out/'summary.json'),SHA256=sha(a.out/'summary.json'),counts={k:summary[k]for k in('generations','old_rows','new_rows','source_built_parents','whole_expected_errors')},errors=bad[:6],flows=flows)),flush=True)
assert not bad
