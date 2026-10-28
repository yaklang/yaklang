"""Whole CLI generations: exact generated references plus source-built parent rows."""
import argparse,copy,hashlib,itertools,json,sys
from pathlib import Path
ap=argparse.ArgumentParser(description=__doc__)
for name in('before','after','SDK-proof','TCP-expected','TCP-verified','bins','capture','audit-modules','source-modules','out'):ap.add_argument('--'+name,type=Path,required=True)
a=ap.parse_args();a.out.mkdir(exist_ok=False);sys.path.insert(0,str(a.audit_modules));sys.path.insert(0,str(a.source_modules))
from known_cli_pdu_reference_paths import remap_cli_row_pdu_references
from icmp4_expected_source import same
from partial_cli_bridge import projection
from vxlan_endpoint_projection_audit import capture_records
def sha(p):
 h=hashlib.sha256()
 with p.open('rb')as f:
  for b in iter(lambda:f.read(1<<20),b''):h.update(b)
 return h.hexdigest()
proof=json.loads((a.SDK_proof/'summary.json').read_text());ledger=a.SDK_proof/'parent-expected.jsonl';assert proof['whole_expected_errors']==0 and proof['parent_expected_SHA256']==sha(ledger)
parents={v['frame']:v['expected_new_event']for v in map(json.loads,ledger.open())};assert len(parents)==17532
mapping={int(k):v for k,v in json.loads((a.SDK_proof/'sharon-pdu-map.json').read_text()).items()}
tcp_meta=json.loads((a.TCP_expected/'summary.json').read_text());tcp_check=json.loads((a.TCP_verified/'summary.json').read_text())
tcp_ledger=a.TCP_expected/'whole-expected.jsonl';assert sha(tcp_ledger)==tcp_meta['whole_expected_SHA256']
assert tcp_check['expected_summary_SHA256']==sha(a.TCP_expected/'summary.json') and tcp_check['native_contributions']==0
tcp={v['frame']:v for v in map(json.loads,tcp_ledger.open())};assert len(tcp)==705 and not tcp.keys()&parents.keys()
cli_build=json.loads((a.bins/'yak-receipt.json').read_text());assert cli_build['source']==proof['source_after'] and cli_build['exit_code']==0 and cli_build['binary_SHA256']==sha(a.bins/'yak.exe')
records={}
for f,r in enumerate(capture_records(a.capture),1):
 if f in parents or f in tcp:
  r['frame']=f;records[f]=r
  if f in tcp:assert hashlib.sha256(r['wire']).hexdigest()==tcp[f]['wire_SHA256']
assert records.keys()==parents.keys()|tcp.keys()
tcp_seen=set()
def expected_row(old,event,record):
 w=record['wire'];h=(w[14]&15)*4;total=int.from_bytes(w[16:18],'big')
 src='.'.join(map(str,w[26:30]))if total>=h else'';dst='.'.join(map(str,w[30:34]))if total>=h else''
 out=copy.deepcopy(old)
 for key in('physical_source','physical_destination','source_port','destination_port','message_source','message_destination','connection_protocol','connection_evidence','connection_stack','stream_id'):
  out.pop(key,None)
 out.update(source=src,destination=dst,protocol='IPv4',transport='network',protocol_stack=['Ethernet','IPv4'],protocol_status='limited',protocol_scope='outer network prefix; inner identity unconfirmed',protocol_evidence='encapsulation profile unselected; child headers not admitted',visibility='unselected-network-profile',message_transport='network',message_domain=event['domain'],pdus=[projection(event)],pdu_scope='observed_at_packet',info=event['summary'])
 if src:out.update(physical_source=src,physical_destination=dst,message_source=src,message_destination=dst)
 return out
files=[];seen=set();bad=[];representative=None;cases=sorted(a.before.glob('*/*/yak-producer.json'));assert len(cases)==37
with(a.out/'source-expected-rows.jsonl').open('x',encoding='utf8')as log:
 for op in cases:
  np=a.after/op.relative_to(a.before);om=json.loads(op.read_text());nm=json.loads(np.read_text());old=op.with_name('yak.jsonl');new=np.with_name('yak.jsonl');name=op.parent.name
  assert om['source']==proof['source_before']and nm['source']==proof['source_after']and om['exit_code']==nm['exit_code']==0
  assert om['capture_SHA256']==nm['capture_SHA256']and om['keylog_used']==nm['keylog_used']and om['arguments'][1:]==nm['arguments'][1:]
  assert om['export_SHA256']==sha(old)and nm['export_SHA256']==sha(new)and nm['binary_SHA256']==cli_build['binary_SHA256']
  count=literal=remapped=changed=tcp_changed=0
  with old.open('rb')as left,new.open('rb')as right:
   for x,y in itertools.zip_longest(left,right):
    assert x is not None and y is not None,'different physical/close row count';count+=1
    if x==y:literal+=1;continue
    assert name=='sharon';oe,ne=json.loads(x),json.loads(y);want=remap_cli_row_pdu_references(oe,mapping);f=oe.get('number')
    if f in parents:
     assert f not in seen;seen.add(f);want=expected_row(want,parents[f],records[f]);changed+=1
     log.write(json.dumps(dict(frame=f,wire_SHA256=hashlib.sha256(records[f]['wire']).hexdigest(),expected_new_row=want),sort_keys=True)+'\n')
     if representative is None:representative=want
    elif f in tcp:
     assert f not in tcp_seen;tcp_seen.add(f);tcp_changed+=1
     want=copy.deepcopy(tcp[f]['whole_expected_row'])
     assert want['number']==f
     log.write(json.dumps(dict(frame=f,wire_SHA256=tcp[f]['wire_SHA256'],expected_new_row=want,TCP_source_expected=True),sort_keys=True)+'\n')
    else:remapped+=1
    if not same(want,ne):bad.append(dict(frame=f,keys=[k for k in want.keys()|ne.keys()if not same(want.get(k),ne.get(k))],expected={k:want.get(k)for k in want.keys()|ne.keys()if not same(want.get(k),ne.get(k))},actual={k:ne.get(k)for k in want.keys()|ne.keys()if not same(want.get(k),ne.get(k))}))
  files.append(dict(case=name,rows=count,literal_rows=literal,whole_equal_after_only_known_generated_refs=remapped,whole_source_expected_parent_rows=changed,whole_source_expected_TCP_rows=tcp_changed,before_SHA256=sha(old),after_SHA256=sha(new),before_producer_SHA256=sha(op),after_producer_SHA256=sha(np)))
assert seen==parents.keys()and tcp_seen==tcp.keys()
(a.out/'mismatches.jsonl').write_text(''.join(json.dumps(v,sort_keys=True)+'\n'for v in bad),encoding='utf8')
negative=[]
if representative:
 for name,mutate in[
  ('false-ARP-label',lambda r:r.__setitem__('protocol','ARP')),
  ('false-child-stack',lambda r:r['protocol_stack'].append('ARP')),
  ('false-decoded-status',lambda r:r.__setitem__('protocol_status','decoded')),
  ('wrong-physical-source',lambda r:r.__setitem__('physical_source','203.0.113.1')),
  ('invented-connection',lambda r:r.__setitem__('connection_protocol','TLS')),
  ('missing-native-diagnostic',lambda r:r.pop('pdus')),
  ('wrong-field-range',lambda r:r['pdus'][0]['field_values']['values'][0].__setitem__('pdu_range',dict(offset=0,length=1)))]:
  forged=copy.deepcopy(representative);mutate(forged);assert not same(forged,representative);negative.append(name)
summary=dict(scope=__doc__,source_before=proof['source_before'],source_after=proof['source_after'],generations=len(files),rows=sum(v['rows']for v in files),literal_rows=sum(v['literal_rows']for v in files),known_ref_only_rows=sum(v['whole_equal_after_only_known_generated_refs']for v in files),source_parent_rows=len(seen),source_parent_total=len(parents),source_TCP_rows=len(tcp_seen),TCP_frozen_source_summary_SHA256=sha(a.TCP_expected/'summary.json'),TCP_actual_verification_SHA256=sha(a.TCP_verified/'summary.json'),whole_expected_errors=len(bad),error_examples=bad[:2],SDK_certificate_SHA256=sha(a.SDK_proof/'summary.json'),source_expected_rows_SHA256=sha(a.out/'source-expected-rows.jsonl'),negative_controls=negative,auditor_SHA256=sha(Path(__file__)),files=files)
(a.out/'summary.json').write_text(json.dumps(summary,indent=2,sort_keys=True)+'\n',encoding='utf8')
print(json.dumps(dict(summary=str(a.out/'summary.json'),SHA256=sha(a.out/'summary.json'),counts={k:summary[k]for k in('generations','rows','literal_rows','known_ref_only_rows','source_parent_rows','source_TCP_rows','whole_expected_errors')},errors=bad[:1])),flush=True)
assert not bad
