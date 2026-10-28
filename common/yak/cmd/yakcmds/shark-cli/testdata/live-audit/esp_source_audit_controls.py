"""Portable semantic controls against an actual source-bound public ESP witness.

These prove rejection by a full independent expected object. They do not claim
that every negative is a separate codec execution or that generated IDs can be
inferred from the wire.
"""
import argparse,base64,copy,hashlib,json,sys
from pathlib import Path
ap=argparse.ArgumentParser()
for name in('capture','native-jsonl','CLI-jsonl','public-proof','out'):ap.add_argument('--'+name,type=Path,required=True)
ap.add_argument('--corpus',choices=['weber','sharon'],required=True)
ap.add_argument('--audit-modules',type=Path,default=Path(__file__).resolve().parent)
a=ap.parse_args();sys.path.insert(1,str(a.audit_modules));HERE=Path(__file__).resolve().parent
from esp_exact_expected import expected_esp,expected_esp_cli,same
from esp_source_wire import network
from vxlan_endpoint_projection_audit import capture_records
def sha(p):
 h=hashlib.sha256()
 with Path(p).open('rb')as f:
  for chunk in iter(lambda:f.read(1<<20),b''):h.update(chunk)
 return h.hexdigest()
proof=json.loads(a.public_proof.read_text());item=next(c for c in proof['corpora']if c['corpus']==a.corpus)
assert item['capture_SHA256']==sha(a.capture)and item['SDK_sha256']==sha(a.native_jsonl)and item['CLI_sha256']==sha(a.CLI_jsonl)
assert proof['expected_constructor_sha256']==sha(HERE/'esp_exact_expected.py')
event=None
with a.native_jsonl.open(encoding='utf-8-sig')as f:
 for line in f:
  candidate=json.loads(line)
  if candidate['protocol']=='esp'and candidate['status']=='limited':event=candidate;break
assert event is not None;frame=event['source_bytes']['PacketRefs'][0]['Number']
for number,record in enumerate(capture_records(a.capture),1):
 if number==frame:record['frame']=frame;break
else:raise AssertionError('PDU source frame missing')
literal=network(record['wire'],record['link']);assert same(event,expected_esp(event['id'],record,literal))
with a.CLI_jsonl.open(encoding='utf-8-sig')as f:
 for line in f:
  row=json.loads(line)
  if row.get('number')==frame:break
 else:raise AssertionError('PDU CLI physical row missing')
assert same(row,expected_esp_cli(event,record,literal,row['time']));positive=(record,literal,event,row)
record,literal,event,row=positive;negative=[]
def whole_native(changed):assert same(changed,expected_esp(event['id'],record,literal))
def coherent_header(changed):
 wire=b'\x11\x22\x33\x44'+base64.b64decode(changed['raw'])[4:];digest=hashlib.sha256(wire).hexdigest();changed['raw']=base64.b64encode(wire).decode()
 for source in[changed['source_bytes'],changed['field_values']['source']]:source['SHA256']=digest
 for fields in[changed['fields'],changed['session']]:fields['SPI']=int.from_bytes(wire[:4],'big')
 for field in changed['field_values']['values']:
  if field['name']=='esp.spi':field['value']=int.from_bytes(wire[:4],'big')
 changed['summary']='ESP · SPI=0x11223344 · seq=%d · opaque=%d bytes; SA context unavailable'%(int.from_bytes(wire[4:8],'big'),len(wire)-8)
for name,change in[
 ('coherent-header-vs-original-wire',coherent_header),
 ('authentication-claim',lambda e:(e['fields'].__setitem__('Authentication Verified',True),e['session'].__setitem__('Authentication Verified',True))),
 ('invented-SA-transaction',lambda e:e.__setitem__('transaction',1)),
 ('invented-flow',lambda e:e.__setitem__('flow',1)),
 ('source-domain',lambda e:e['source_bytes']['PacketRefs'][0]['Domain'].__setitem__('Interface',record['interface']+1)),
 ('extra-session-metadata',lambda e:e['session'].__setitem__('Unexpected Metadata',True)),
 ('nil-vs-empty-body',lambda e:e['session'].__setitem__('Opaque Body',None)),
 ('word-type',lambda e:e['session'].__setitem__('SPI',str(e['session']['SPI']))),
 ('logical-range',lambda e:e['session']['Wire Field Ranges'].__setitem__('SPI',[1,5])),
 ('scalar-order',lambda e:e['field_values']['values'].reverse())]:
 changed=copy.deepcopy(event);change(changed)
 try:whole_native(changed)
 except AssertionError:negative.append(name)
 else:raise AssertionError('negative accepted '+name)
cli_negative=[]
for name,change in[
 ('wrong-profile-label',lambda r:r.__setitem__('protocol','TCP')),
 ('fake-child-flow',lambda r:r.__setitem__('tcp_flow_id',1)),
 ('hidden-row-field',lambda r:r.__setitem__('unknown',True)),
 ('wrong-prefix',lambda r:r['protocol_stack'].insert(1,'UDP')),
 ('wrong-physical-endpoint',lambda r:r.__setitem__('source','203.0.113.99')),
 ('missing-PDU-metadata',lambda r:r['pdus'][0].pop('field_values')),
 ('range-alias-mutation',lambda r:r['pdus'][0]['field_values']['values'][-1]['pdu_range'].__setitem__('length',3))]:
 changed=copy.deepcopy(row);change(changed);assert not same(changed,expected_esp_cli(event,record,literal,row['time']));cli_negative.append(name)
summary=dict(scope='One actual public-source positive plus whole-object semantic negative controls; no Go/native executable rerun is inferred.',source=proof['source'],source_frame=frame,positive_PDU=event['id'],native_negative_controls=negative,CLI_negative_controls=cli_negative,expected_constructor_sha256=sha(HERE/'esp_exact_expected.py'),wire_interpreter_sha256=sha(HERE/'esp_source_wire.py'),controls_sha256=sha(Path(__file__)),public_proof_sha256=sha(a.public_proof),arguments={key:str(value)for key,value in vars(a).items()})
assert not a.out.exists();a.out.write_text(json.dumps(summary,indent=2,sort_keys=True)+'\n',encoding='utf8');print(json.dumps(dict(proof=str(a.out),SHA256=sha(a.out),native_negative_controls=len(negative),CLI_negative_controls=len(cli_negative))))
