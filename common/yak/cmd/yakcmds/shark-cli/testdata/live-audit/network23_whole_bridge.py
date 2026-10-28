"""Strict37-generation bridges for source-built ARP/ICMP aliases, without path waivers."""
import argparse,collections,copy,hashlib,itertools,json,sys
from pathlib import Path
ap=argparse.ArgumentParser(description=__doc__)
for name in('native-before','native-after','CLI-before','CLI-after','proposal','carriers','bins','source-modules','out'):ap.add_argument('--'+name,type=Path,required=True)
a=ap.parse_args();a.out.mkdir(exist_ok=False);sys.path.insert(0,str(a.source_modules))
from icmp4_expected_source import same
from partial_cli_bridge import projection
def sha(p):
 h=hashlib.sha256()
 with p.open('rb')as f:
  for b in iter(lambda:f.read(1<<20),b''):h.update(b)
 return h.hexdigest()
meta=json.loads((a.proposal/'summary.json').read_text());carrier=json.loads((a.carriers/'summary.json').read_text());built=json.loads((a.bins/'events-receipt.json').read_text())
assert built['source']=='5403ffe55b219702af97a3422dd6d59996aadfbe'and built['exit_code']==0 and built['binary_SHA256']==sha(a.bins/'events.exe')
assert carrier['whole_expected_equal']==carrier['observations']==284 and carrier['errors']==0 and carrier['expected_ledger_SHA256']==sha(a.carriers/'expected.jsonl')
proposed={};expected={};old_hashes={};source_pins={}
for item in meta['corpora']:
 name=item['corpus'];p=a.proposal/name/'snapshots.jsonl';assert item['snapshots_SHA256']==sha(p)
 rows=[json.loads(v)for v in p.open()];assert len(rows)==item['source_observations'];old_hashes[name]=item['SDK_SHA256'];source_pins[name]=sha(p)
 for row in rows:
  key=name,row['id'];assert key not in proposed;proposed[key]=row
for row in map(json.loads,(a.carriers/'expected.jsonl').open()):
 key=row['corpus'],row['id'];assert key not in proposed and key not in expected;expected[key]=row
assert len(proposed)==11191 and len(expected)==284
files=[];seen=set();oldfiles=sorted(a.native_before.glob('*/*/events-producer.json'));assert len(oldfiles)==37
for op in oldfiles:
 np=a.native_after/op.relative_to(a.native_before);om=json.loads(op.read_text());nm=json.loads(np.read_text());old=op.with_name('events.jsonl');new=np.with_name('events.jsonl');name=op.parent.name
 assert om['source']=='1ea484dcf78cc443bf553a10ee8b9488e9f3b244'and nm['source']==built['source']and om['exit_code']==nm['exit_code']==0
 assert om['capture_SHA256']==nm['capture_SHA256']and om['keylog_used']==nm['keylog_used']and om['arguments'][1:]==nm['arguments'][1:]
 assert om['export_SHA256']==sha(old)and nm['export_SHA256']==sha(new)and nm['binary_SHA256']==built['binary_SHA256']
 if name in old_hashes:assert sha(old)==old_hashes[name]
 count=literal=changed=0
 with old.open('rb')as left,new.open('rb')as right:
  for x,y in itertools.zip_longest(left,right):
   assert x is not None and y is not None;count+=1
   if x==y:literal+=1;continue
   oe,ne=json.loads(x),json.loads(y);key=name,oe['id'];assert key not in seen
   if key in proposed:
    row=proposed[key];assert same(oe['field_values'],row['whole_old_snapshot'])
    want=copy.deepcopy(oe);want['field_values']=copy.deepcopy(row['whole_proposed_snapshot']);expected[key]=dict(corpus=name,id=oe['id'],frame=row['frame'],old_event=oe,expected_new_event=want,added_values=row['expected_added_values'])
   else:
    assert key in expected;row=expected[key];assert same(oe,row['old_event']);want=row['expected_new_event']
   assert same(ne,want),(str(old),oe['id'],[k for k in want.keys()|ne.keys()if not same(want.get(k),ne.get(k))])
   seen.add(key);changed+=1
 files.append(dict(case=name,rows=count,literal_rows=literal,whole_source_expected_changes=changed,before_SHA256=sha(old),after_SHA256=sha(new),before_producer_SHA256=sha(op),after_producer_SHA256=sha(np)))
assert len(expected)==len(seen)==11475 and seen==expected.keys()
flows=[]
for old in a.native_before.glob('*/*/flows.json'):
 new=a.native_after/old.relative_to(a.native_before);assert sha(old)==sha(new);flows.append(dict(case=old.parent.name,SHA256=sha(old)))
assert len(flows)==2
(a.out/'source-expected.jsonl').write_text(''.join(json.dumps(v,sort_keys=True)+'\n'for v in expected.values()),encoding='utf8')
CLI=[];observed=set();cases=sorted(a.CLI_before.glob('*/*/yak-producer.json'));assert len(cases)==37
for op in cases:
 np=a.CLI_after/op.relative_to(a.CLI_before);om=json.loads(op.read_text());nm=json.loads(np.read_text());old=op.with_name('yak.jsonl');new=np.with_name('yak.jsonl');name=op.parent.name
 assert om['source']=='1ea484dcf78cc443bf553a10ee8b9488e9f3b244'and nm['source']==built['source']and om['exit_code']==nm['exit_code']==0
 assert om['capture_SHA256']==nm['capture_SHA256']and om['keylog_used']==nm['keylog_used']and om['arguments'][1:]==nm['arguments'][1:]
 assert om['export_SHA256']==sha(old)and nm['export_SHA256']==sha(new)
 count=literal=changed=0
 with old.open('rb')as left,new.open('rb')as right:
  for x,y in itertools.zip_longest(left,right):
   assert x is not None and y is not None;count+=1
   if x==y:literal+=1;continue
   oe,ne=json.loads(x),json.loads(y);want=copy.deepcopy(oe);replacements=0
   for group in('pdus','published_pdus'):
    for index,pdu in enumerate(oe.get(group,[])):
     key=name,pdu['id']
     if key not in expected:continue
     row=expected[key];assert same(pdu,projection(row['old_event'])),(name,oe.get('number'),key,'old CLI native projection differs')
     want[group][index]=projection(row['expected_new_event']);replacements+=1
     assert key not in observed,'unexpected duplicate publication of one own-header observation';observed.add(key)
   assert replacements>0 and same(want,ne),(name,oe.get('number'),[k for k in want.keys()|ne.keys()if not same(want.get(k),ne.get(k))])
   changed+=1
 CLI.append(dict(case=name,rows=count,literal_rows=literal,whole_source_expected_changes=changed,before_SHA256=sha(old),after_SHA256=sha(new),before_producer_SHA256=sha(op),after_producer_SHA256=sha(np)))
assert observed==expected.keys()
negative=[];sample=next(iter(expected.values()))
for name,mutate in[
 ('wrong-value',lambda e:e['field_values']['values'][-1].__setitem__('value',False)),
 ('missing-alias',lambda e:e['field_values']['values'].pop()),
 ('missing-range',lambda e:e['field_values']['values'][-1].pop('pdu_range')),
 ('wrong-range',lambda e:e['field_values']['values'][-1]['pdu_range'].__setitem__('offset',0)),
 ('foreign-PDU-ID',lambda e:e.__setitem__('id',0)),
 ('foreign-source-reference',lambda e:e['source_bytes']['PacketRefs'][0].__setitem__('Number',0)),
 ('invented-checksum-trust',lambda e:e['session'].__setitem__('Checksum Verified',True))]:
 forged=copy.deepcopy(sample['expected_new_event']);mutate(forged);assert not same(forged,sample['expected_new_event']);negative.append(name)
summary=dict(scope=__doc__,source_before='1ea484dcf78cc443bf553a10ee8b9488e9f3b244',source_after=built['source'],native_generations=len(files),native_rows=sum(v['rows']for v in files),native_literal_rows=sum(v['literal_rows']for v in files),native_whole_source_expected_changes=sum(v['whole_source_expected_changes']for v in files),CLI_generations=len(CLI),CLI_rows=sum(v['rows']for v in CLI),CLI_literal_rows=sum(v['literal_rows']for v in CLI),CLI_whole_source_expected_changes=sum(v['whole_source_expected_changes']for v in CLI),added_source_values=sum(len(v['added_values'])for v in expected.values()),flows_whole_files_literal=flows,negative_controls=negative,source_expected_ledger_SHA256=sha(a.out/'source-expected.jsonl'),proposal_proof_SHA256=sha(a.proposal/'summary.json'),carrier_proof_SHA256=sha(a.carriers/'summary.json'),source_snapshot_pins=source_pins,auditor_SHA256=sha(Path(__file__)),native_files=files,CLI_files=CLI,admission_limit='46 old EtherIP/ARP admissions are known wrong and require the subsequent explicit parent-refusal phase; this alias certificate does not validate inherited protocol admission or neighbor/peer trust.')
(a.out/'summary.json').write_text(json.dumps(summary,indent=2,sort_keys=True)+'\n',encoding='utf8')
print(json.dumps(dict(summary=str(a.out/'summary.json'),SHA256=sha(a.out/'summary.json'),counts={k:summary[k]for k in('native_generations','native_rows','native_literal_rows','native_whole_source_expected_changes','CLI_generations','CLI_rows','CLI_literal_rows','CLI_whole_source_expected_changes','added_source_values')})),flush=True)
