"""Complete native export bridge with full original-byte expected ICMPv6 failures."""
import argparse,base64,collections,copy,hashlib,itertools,json,sys
from pathlib import Path
ap=argparse.ArgumentParser()
for name in('before','after','capture-sharon','audit-modules','bins','out'):ap.add_argument('--'+name,type=Path,required=True)
ap.add_argument('--source-before',required=True);ap.add_argument('--source-after',required=True)
a=ap.parse_args();a.out.mkdir(exist_ok=False);sys.path.insert(1,str(a.audit_modules))
from vxlan_endpoint_projection_audit import capture_records
from icmpv6_expected import expected_failure,fixed_prefix,same

def sha(path):
 h=hashlib.sha256()
 with Path(path).open('rb')as f:
  for chunk in iter(lambda:f.read(1<<20),b''):h.update(chunk)
 return h.hexdigest()
receipt=a.bins/'events-receipt.json';built=json.loads(receipt.read_text());assert built['source']==a.source_after and built['exit_code']==0 and built['binary_SHA256']==sha(a.bins/'events.exe')
oldsh=a.before/'preservation/sharon/events.jsonl';old={}
for line in oldsh.open(encoding='utf8'):
 e=json.loads(line);refs=e['source_bytes'].get('PacketRefs')or[]
 if e['expert']in('ICMPv6HeaderTruncated','NDOptionLength','NDOptionBudget')or any(r['Number']in(223884,268572,285780,309597)for r in refs):
  assert len(refs)==1;frame=refs[0]['Number'];assert frame not in old;old[frame]=e
assert len(old)==63;expected={};ledger=[]
for frame,record in enumerate(capture_records(a.capture_sharon),1):
 if frame not in old:continue
 record['frame']=frame;e=old[frame];ref=e['source_bytes']['PacketRefs'][0];assert ref['Number']==frame and ref['Domain']==e['domain']=={'Section':record['section'],'Interface':record['interface'],'Encapsulation':''}
 value=expected_failure(e['id'],record);facts=fixed_prefix(record);assert e['flow']==0 and e['transaction']==0 and e['response_to']==0
 if e['raw']:assert base64.b64decode(e['raw'])==record['wire']and e['source_bytes']['SHA256']==hashlib.sha256(record['wire']).hexdigest()
 expected[e['id']]=value;ledger.append(dict(frame=frame,link=record['link'],section=record['section'],interface=record['interface'],time_ns=record['time_ns'],captured_length=record['captured_length'],original_length=record['original_length'],wire_SHA256=hashlib.sha256(record['wire']).hexdigest(),wire_hex=record['wire'].hex(),old_event=e,literal_facts=facts,expected_new_event=value))
assert len(expected)==63
ledger_path=a.out/'source63-expected.jsonl';ledger_path.write_text(''.join(json.dumps(row,sort_keys=True)+'\n'for row in ledger),encoding='utf8')
oldfiles={p.relative_to(a.before):p for p in a.before.rglob('events.jsonl')};newfiles={p.relative_to(a.after):p for p in a.after.rglob('events.jsonl')};assert oldfiles.keys()==newfiles.keys();files=[];observed=set();unexpected=[]
for rel in sorted(oldfiles):
 before=oldfiles[rel];after=newfiles[rel];bp=before.with_name('events-producer.json');np=after.with_name('events-producer.json');bm=json.loads(bp.read_text());nm=json.loads(np.read_text());assert bm['source']==a.source_before and nm['source']==a.source_after and bm['exit_code']==nm['exit_code']==0 and bm['capture_SHA256']==nm['capture_SHA256'];assert bm['export_SHA256']==sha(before)and nm['export_SHA256']==sha(after)and nm['binary_SHA256']==built['binary_SHA256']
 lines=0;literal=0;typed=0;changes=[]
 with before.open('rb')as oldlines,after.open('rb')as newlines:
  for number,(x,y)in enumerate(itertools.zip_longest(oldlines,newlines),1):
   assert x is not None and y is not None,(str(rel),'export length differs');lines+=1
   if x==y:literal+=1;continue
   oe=json.loads(x);ne=json.loads(y)
   if str(rel).replace('\\','/')=='preservation/sharon/events.jsonl'and oe['id']in expected:
    assert oe['id']==ne['id']and same(ne,expected[oe['id']]),(str(rel),number,oe['id'],{key:[ne.get(key),expected[oe['id']].get(key)]for key in ne.keys()|expected[oe['id']].keys()if not same(ne.get(key),expected[oe['id']].get(key))});assert oe['id']not in observed;observed.add(oe['id']);typed+=1;changes.append(dict(id=oe['id'],frame=oe['source_bytes']['PacketRefs'][0]['Number'],old_protocol=oe['protocol'],new_protocol=ne['protocol'],old_status=oe['status'],new_status=ne['status']))
   else:unexpected.append(dict(input=str(rel),row=number,old_id=oe.get('id'),new_id=ne.get('id'),old_protocol=oe.get('protocol'),new_protocol=ne.get('protocol'),changed_keys=[key for key in oe.keys()|ne.keys()if not same(oe.get(key),ne.get(key))]))
 files.append(dict(input=str(rel),rows=lines,unchanged_literal_rows=literal,whole_source_expected_changes=typed,changes=changes,before_SHA256=bm['export_SHA256'],after_SHA256=nm['export_SHA256'],before_producer_SHA256=sha(bp),after_producer_SHA256=sha(np)))
 assert typed==0 or str(rel).replace('\\','/')=='preservation/sharon/events.jsonl'
print(json.dumps(dict(inputs=len(files),expected_changes=len(observed),unexpected=unexpected)),flush=True)
assert not unexpected and observed==expected.keys()
flows=[]
for before in sorted(a.before.rglob('flows.json')):
 after=a.after/before.relative_to(a.before);assert sha(before)==sha(after),(str(before),'flow file differs');flows.append(dict(input=str(before.relative_to(a.before)),SHA256=sha(before)))
negative=[];positive=ledger[0];record={'link':positive['link'],'wire':bytes.fromhex(positive['wire_hex']),'frame':positive['frame'],'section':positive['section'],'interface':positive['interface']};base=positive['expected_new_event']
for name,change in[
 ('false-decoded-message',lambda e:(e.__setitem__('status','decoded'),e.__setitem__('completeness','message'))),
 ('invented-finished-source',lambda e:e['source_bytes'].__setitem__('CompletedIn',e['source_bytes']['PacketRefs'][0])),
 ('wrong-binary-body',lambda e:e.__setitem__('raw',base64.b64encode(b'\0'+base64.b64decode(e['raw'])[1:]).decode())),
 ('wrong-source-domain',lambda e:e['source_bytes']['PacketRefs'][0]['Domain'].__setitem__('Interface',e['domain']['Interface']+1)),
 ('invented-neighbor-trust',lambda e:e['session'].__setitem__('Authentication Verified',True)),
 ('range-or-wire-metadata-invention',lambda e:e['session'].__setitem__('Wire Field Ranges',{'Type':[0,1]})),
 ('wrong-opaque-prefix-type',lambda e:e['session'].__setitem__('Type',str(e['session']['Type']))),
 ('fake-child-flow',lambda e:e.__setitem__('flow',1)),
 ('wrong-field-snapshot',lambda e:e['field_values'].__setitem__('pdu',e['id']+1))]:
 changed=copy.deepcopy(base);change(changed);assert not same(changed,expected_failure(base['id'],record));negative.append(name)
summary=dict(scope='Complete native08->09 streaming SDK bridge. Only63 entire replacement objects are accepted, each rebuilt from its original source. No own-ID/wire/domain/endpoint normalization.',source_before=a.source_before,source_after=a.source_after,inputs=len(files),total_rows=sum(x['rows']for x in files),literal_rows=sum(x['unchanged_literal_rows']for x in files),whole_source_expected_changes=sum(x['whole_source_expected_changes']for x in files),files=files,flows_literal=flows,expected_source_ledger_SHA256=sha(ledger_path),capture_sharon_SHA256=sha(a.capture_sharon),expected_constructor_SHA256=sha(Path(__file__).with_name('icmpv6_expected.py')),literal_parser_SHA256=sha(Path(__file__).with_name('literal_own_header.py')),auditor_SHA256=sha(Path(__file__)),negative_controls=negative)
target=a.out/'summary.json';target.write_text(json.dumps(summary,indent=2,sort_keys=True)+'\n',encoding='utf8');print(json.dumps(dict(summary=str(target),SHA256=sha(target))),flush=True)
