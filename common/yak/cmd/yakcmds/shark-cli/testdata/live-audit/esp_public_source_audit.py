"""Portable whole-object ESP source audit. Captures, references and exports stay outside Git.

The only generated identifiers accepted from current output are positive unique
PDU IDs tied to one original source. A separate whole-SDK reference certificate
must prove the global ID bijection. No endpoint/time/window or wire scalar ID
normalization is performed here.
"""
import argparse,collections,copy,hashlib,importlib.util,json,sys
from pathlib import Path
ap=argparse.ArgumentParser(description="Verify full ESP event/CLI objects from two public original captures and a receipted Wireshark source inventory.")
for name in('native','CLI','baseline','bins','reference','capture-weber','capture-sharon','out'):ap.add_argument('--'+name,required=True,type=Path)
ap.add_argument('--source',required=True)
ap.add_argument('--audit-modules',type=Path,default=Path(__file__).resolve().parent,help='Directory with the existing adjacent live-audit capture reader modules; defaults to this directory.')
a=ap.parse_args();HERE=Path(__file__).resolve().parent;HIST=a.reference;a.out.mkdir(exist_ok=False)
sys.path.insert(1,str(a.audit_modules))
from esp_exact_expected import expected_esp,expected_esp_cli,expected_pdu,same
from esp_parent_exact_expected import expected_parent
from esp_source_wire import network
CAPTURES={"weber":a.capture_weber,"sharon":a.capture_sharon}
import vxlan_endpoint_projection_audit as capture_reader
from vxlan_endpoint_projection_audit import capture_records
from pptp_parent_source_audit import instant

def sha(p):
 h=hashlib.sha256()
 with Path(p).open('rb')as f:
  for block in iter(lambda:f.read(1<<20),b''):h.update(block)
 return h.hexdigest()
def parent_option_failure(record,literal):
 if literal.get('ip_version')!=4:return None
 wire=record['wire'];start=literal['ip_offset'];at=start+20;end=start+(wire[start]&15)*4
 while at<end:
  option=wire[at]
  if option==0:return None
  if option==1:at+=1;continue
  if at+2>end:return dict(offset=at,reason='IPv4 option length byte outside IHL')
  length=wire[at+1]
  if length>end-at or length<=2:return dict(offset=at,option_type=option,option_length=length,available=end-at)
  at+=length
 return None

def load_candidates(path,selected):
 groups=collections.defaultdict(list);allids=set();esp_count=0;count=0
 with path.open(encoding='utf-8-sig')as f:
  for line in f:
   event=json.loads(line);assert type(event['id'])is int and event['id']>0 and event['id']not in allids;allids.add(event['id']);count+=1
   refs=event['source_bytes'].get('PacketRefs')or[];hits={ref['Number']for ref in refs if ref['Number']in selected}
   if event['protocol']=='esp':assert len(refs)==len(hits)==1;esp_count+=1
   for frame in hits:groups[frame].append(event)
 return groups,esp_count,count

def own_id_bridge(before,after):
 out=copy.deepcopy(before);out['id']=after['id'];out['field_values']['pdu']=after['id']
 for value in out['field_values']['values']:
  if value['name']=='pdu.id':
   assert value['value']==before['id']and value.get('exact_value')==str(before['id']);value['value']=after['id'];value['exact_value']=str(after['id'])
 return out

receipts={}
for name in('events','flows','yak'):
 receipt=a.bins/(name+'-receipt.json');data=json.loads(receipt.read_text());assert data['source']==a.source and data['exit_code']==0 and data['binary_SHA256']==sha(a.bins/(name+'.exe'));receipts[name]=dict(receipt_sha256=sha(receipt),binary_sha256=data['binary_SHA256'])
CAPTURE_SHA={'weber':'ecbca543fbe011a85c8876420fcb2c244b9290ad653d13cd1a574158167073f4','sharon':'aa4fcc81af216e2b788171c64271b43681974d9156aa9ddbdc1a7a25e730714c'}
corpora=[];positive=None
for name,capture in CAPTURES.items():
 assert sha(capture)==CAPTURE_SHA[name]
 stock_path=HIST/name/'sources.jsonl';stock_summary=HIST/name/'summary.json';stock_metadata=json.loads(stock_summary.read_text());assert stock_metadata['capture']['sha256']==CAPTURE_SHA[name]and stock_metadata['sources']['sha256']==sha(stock_path)
 stock={r['frame']:r for r in map(json.loads,stock_path.open())};records={};packet_count=0
 for frame,record in enumerate(capture_records(capture),1):
  packet_count=frame
  if frame in stock:
   record['frame']=frame;assert hashlib.sha256(record['wire']).hexdigest()==stock[frame]['wire_sha256'];records[frame]=record
 assert records.keys()==stock.keys()
 sdk=a.native/'preservation'/name/'events.jsonl';producer_path=sdk.with_name('events-producer.json');producer=json.loads(producer_path.read_text())
 assert producer['source']==a.source and producer['exit_code']==0 and producer['capture_SHA256']==sha(capture)and producer['export_SHA256']==sha(sdk)and producer['binary_SHA256']==receipts['events']['binary_sha256']
 current,native_esp,total=load_candidates(sdk,stock);baseline=a.baseline/'preservation'/name/'events.jsonl';old,old_esp,_=load_candidates(baseline,stock);assert not old_esp
 exact={};kept=[];new_refusals=[];absent=[];source_ledger=[];counts=collections.Counter()
 for frame,prior in stock.items():
  record=records[frame];literal=network(record['wire'],record['link']);assert same(literal,prior['literal']);events=current[frame];before=old[frame];esp=[e for e in events if e['protocol']=='esp']
  source_row=dict(frame=frame,section=record['section'],interface=record['interface'],time_ns=record['time_ns'],wire_sha256=prior['wire_sha256'],literal=literal)
  if esp:
   assert len(events)==1 and not before and parent_option_failure(record,literal)is None
   assert prior['wireshark']is not None and all(value['match']for value in prior['header_comparison'].values()),(name,frame,'literal/native ESP word differs from Wireshark')
   expected=expected_esp(esp[0]['id'],record,literal);assert same(esp[0],expected),(name,frame,'whole ESP object differs')
   exact[frame]=expected;counts['ESP/'+expected['status']]+=1;source_row['expected_whole_event']=expected
   if positive is None:positive=(record,literal,expected)
  else:
   assert all(e['flow']==0 and e['transaction']==0 and e['response_to']==0 and not e.get('tcp_flow_id')for e in events)
   assert all(not value['name'].startswith('esp.')for e in events for value in e['field_values']['values'])
   if before:
    assert len(before)==len(events)==1 and same(own_id_bridge(before[0],events[0]),events[0]),(name,frame,'retained candidate changed beyond exact own ID')
    kept.append(dict(frame=frame,old_pdu=before[0]['id'],new_pdu=events[0]['id'],expert=events[0]['expert']));counts['retained/'+events[0]['expert']]+=1
   elif events:
    assert len(events)==1 and same(events[0],expected_parent(events[0]['id'],record)),(name,frame,'new parent entire object not proved by literal header')
    new_refusals.append(dict(frame=frame,event=events[0]));counts['new-refusal/'+events[0]['expert']]+=1
   else:
    error=parent_option_failure(record,literal)
    assert error is not None,(name,frame,'no observation without bounded parent-option refusal')
    absent.append(dict(frame=frame,invalid_options=error,literal_parent_reason=literal.get('reason')));counts['no-child-PDU']+=1
   source_row['native_observations']=events;source_row['baseline_observations']=before
  source_ledger.append(source_row)
 assert len(exact)==native_esp
 cli=a.CLI/'preservation'/name/'yak.jsonl';cli_producer_path=cli.with_name('yak-producer.json');cli_producer=json.loads(cli_producer_path.read_text());assert cli_producer['source']==a.source and cli_producer['exit_code']==0 and cli_producer['capture_SHA256']==sha(capture)and cli_producer['export_SHA256']==sha(cli)and cli_producer['binary_SHA256']==receipts['yak']['binary_sha256']
 seen=set();cli_inventory=[];whole_cli_esp=0;unrelated_publications=0
 with cli.open(encoding='utf-8-sig')as f:
  for line in f:
   row=json.loads(line)
   if row.get('record_type')=='protocol_publication':
    assert set(row)=={'record_type','publication_phase','published_pdu_scope','published_pdus'}and row['published_pdu_scope']=='analysis_publication_only; byte_contribution_is_defined_by_byte_source'
    assert type(row['published_pdus'])is list
    for pdu in row['published_pdus']:
     assert pdu['protocol']!='esp'and all(ref['Number']not in stock for ref in(pdu.get('byte_source',{}).get('PacketRefs')or[])),(name,'ESP candidate contributed at close publication')
    unrelated_publications+=1;continue
   frame=row['number']
   if frame not in stock:continue
   # These bounded ESP sources have no TCP/application close publication.
   assert frame not in seen,(name,frame,'repeated candidate row');seen.add(frame);record=records[frame]
   assert instant(row['time'])==record['time_ns']and row['length']==record['original_length']and row['captured_length']==record['captured_length']
   if frame in exact:
    expected=expected_esp_cli(exact[frame],record,stock[frame]['literal'],row['time']);assert same(row,expected),(name,frame,'whole ESP CLI row differs');whole_cli_esp+=1
   else:
    assert row['protocol']!='ESP'and 'ESP'not in row.get('protocol_stack',[])
    assert len(row.get('pdus',[]))==len(current[frame])
    for pdu,event in zip(row.get('pdus',[]),current[frame]):
     assert same(pdu,expected_pdu(event)),(name,frame,'whole retained/refused PDU projection differs')
   cli_inventory.append(dict(frame=frame,protocol=row['protocol'],status=row.get('protocol_status'),stack=row.get('protocol_stack'),source=row['source'],destination=row['destination'],pdus=[p['id']for p in row.get('pdus',[])]))
 assert seen==stock.keys()and whole_cli_esp==native_esp
 flow=a.native/'preservation'/name/'flows.json';flow_producer_path=flow.with_name('flows-producer.json');flow_producer=json.loads(flow_producer_path.read_text());assert flow_producer['source']==a.source and flow_producer['exit_code']==0 and flow_producer['capture_SHA256']==CAPTURE_SHA[name]and flow_producer['export_SHA256']==sha(flow)and flow_producer['binary_SHA256']==receipts['flows']['binary_sha256']
 oldflow=a.baseline/'preservation'/name/'flows.json';now=json.loads(flow.read_text());previous=json.loads(oldflow.read_text())
 for key in('capture_SHA256','flows','packets','closed','error'):assert same(now[key],previous[key]),(name,key,'flow ownership changed')
 ledger=a.out/(name+'-sources.jsonl');ledger.write_text(''.join(json.dumps(row,sort_keys=True)+'\n'for row in source_ledger),encoding='utf8');cl=a.out/(name+'-CLI-inventory.json');cl.write_text(json.dumps(cli_inventory,indent=2)+'\n',encoding='utf8')
 item=dict(corpus=name,physical_frames=packet_count,WS_ESP_candidate_frames=len(stock),ESP_PDU_objects_fully_exact=native_esp,ESP_CLI_rows_fully_exact=whole_cli_esp,unrelated_close_publications_inspected=unrelated_publications,counts=dict(counts),retained_candidate_observations=kept,new_precise_refusals=new_refusals,no_child_observations=absent,all_flow_packet_close_values_types_literal=True,flow_stat_differences={key:dict(before=previous['stats'].get(key),after=now['stats'].get(key))for key in previous['stats'].keys()|now['stats'].keys()if not same(previous['stats'].get(key),now['stats'].get(key))},capture_SHA256=CAPTURE_SHA[name],frozen_source_inventory_sha256=sha(stock_path),flow_producer_sha256=sha(flow_producer_path),source_ledger_sha256=sha(ledger),CLI_inventory_sha256=sha(cl),native_producer_sha256=sha(producer_path),CLI_producer_sha256=sha(cli_producer_path),SDK_sha256=sha(sdk),CLI_sha256=sha(cli))
 corpora.append(item);print(json.dumps({key:item[key]for key in('corpus','physical_frames','WS_ESP_candidate_frames','ESP_PDU_objects_fully_exact','ESP_CLI_rows_fully_exact','counts')}),flush=True)
# The expected constructor is independent of the current event except for the
# source-bound positive unique PDU identifier. A caller proves global ID mapping.
record,literal,event=positive;negative=[]
for name,mutate in[
 ('coherent-false-metadata',lambda e:(e['session'].__setitem__('Authentication Verified',True),e['fields'].__setitem__('Authentication Verified',True))),
 ('wrong-raw-summary',lambda e:e.__setitem__('summary','ESP decoded/authenticated')),
 ('extra-hidden-session-field',lambda e:e['session'].__setitem__('Inferred SA',1)),
 ('wrong-array-order',lambda e:e['field_values']['values'].reverse()),
 ('wrong-type',lambda e:e['session'].__setitem__('Sequence',str(e['session']['Sequence']))),
 ('wrong-range',lambda e:e['session']['Wire Field Ranges'].__setitem__('SPI',[4,8])),
 ('physical-domain',lambda e:e['source_bytes']['PacketRefs'][0]['Domain'].__setitem__('Interface',record['interface']+1))]:
 changed=copy.deepcopy(event);mutate(changed);assert not same(changed,expected_esp(event['id'],record,literal));negative.append(name)
summary=dict(scope='Whole ESP event and packet-row objects rebuilt from literal original sources; retained native candidates exact after source-bound own-ID translation. Parent399/4 global bridge remains a separate root certificate.',source=a.source,bin_receipts=receipts,auditor_sha256=sha(Path(__file__)),expected_constructor_sha256=sha(HERE/'esp_exact_expected.py'),new_parent_constructor_sha256=sha(HERE/'esp_parent_exact_expected.py'),wire_interpreter_sha256=sha(HERE/'esp_source_wire.py'),capture_reader_sha256=sha(Path(capture_reader.__file__)),arguments={key:str(value)for key,value in vars(a).items()},corpora=corpora,typed_exact_negative_controls=negative)
(a.out/'summary.json').write_text(json.dumps(summary,indent=2,sort_keys=True)+'\n',encoding='utf8');print(json.dumps(dict(summary=str(a.out/'summary.json'),sha256=sha(a.out/'summary.json'))))
