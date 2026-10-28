"""Read-only literal ARP/ICMP key-field versus native SDK and TShark inventory."""
import argparse,base64,collections,csv,hashlib,ipaddress,json,os,subprocess,sys,time
from decimal import Decimal
from pathlib import Path
ap=argparse.ArgumentParser()
for name in ('native','census','capture-weber','capture-sharon','audit-modules','constructors','source-ledger','bridge','bins','tshark','out'):ap.add_argument('--'+name,type=Path,required=True)
ap.add_argument('--source',required=True)
a=ap.parse_args();a.out.mkdir(exist_ok=False);sys.path.insert(0,str(a.constructors));sys.path.insert(1,str(a.audit_modules))
from literal_own_header import parent,ip
from icmpv6_expected import source_profile
from vxlan_endpoint_projection_audit import capture_records

def sha(p):
 h=hashlib.sha256()
 with Path(p).open('rb')as f:
  for b in iter(lambda:f.read(1<<20),b''):h.update(b)
 return h.hexdigest()

def typed_equal(x,y):return type(x)is type(y)and x==y

def scalar(v):return int(v,16)if v.lower().startswith('0x')else int(v)
fields=['frame.number','frame.time_epoch','frame.cap_len','frame.len','frame.protocols','arp.opcode','arp.src.hw_mac','arp.src.proto_ipv4','arp.dst.proto_ipv4','icmp.type','icmp.code','icmp.ident','icmp.seq','icmpv6.type','icmpv6.code','icmpv6.echo.identifier','icmpv6.echo.sequence_number','icmpv6.mtu','icmpv6.nd.ns.target_address','icmpv6.nd.na.target_address','icmpv6.nd.rd.target_address','icmpv6.rd.na.destination_address','_ws.expert.message']
receipt=a.bins/'events-receipt.json';build=json.loads(receipt.read_text());assert build['source']==a.source and build['exit_code']==0 and sha(a.bins/'events.exe')==build['binary_SHA256']
census=json.loads((a.census/'summary.json').read_text());bridge=json.loads(a.bridge.read_text());assert bridge['source_before']==census['source'] and bridge['source_after']==a.source and bridge['whole_source_expected_changes']==63 and bridge['expected_source_ledger_SHA256']==sha(a.source_ledger) and bridge['expected_constructor_SHA256']==sha(a.constructors/'icmpv6_expected.py') and bridge['literal_parser_SHA256']==sha(a.constructors/'literal_own_header.py');extra=[json.loads(x)for x in a.source_ledger.open()];assert len(extra)==63;allresults=[]
for name,capture in [('weber',a.capture_weber),('sharon',a.capture_sharon)]:
 folder=a.out/name;folder.mkdir();sdk=a.native/'preservation'/name/'events.jsonl';producer=sdk.with_name('events-producer.json');meta=json.loads(producer.read_text());assert meta['source']==a.source and meta['exit_code']==0 and meta['capture_SHA256']==sha(capture)and meta['export_SHA256']==sha(sdk)and meta['binary_SHA256']==build['binary_SHA256'];bridge_file=next(x for x in bridge['files']if x['input'].replace('\\','/')=='preservation/'+name+'/events.jsonl');assert bridge_file['after_SHA256']==meta['export_SHA256'];census_file=next(x for x in census['corpora']if x['corpus']==name);assert census_file['eligible_SHA256']==sha(a.census/name/'eligible.jsonl')
 eligible={x['frame']:x for x in map(json.loads,(a.census/name/'eligible.jsonl').open())}
 assert len(eligible)==sum(census_file['direct_source_bound_checked'].values())-census_file['own_short_or_option_findings']
 if name=='sharon':
  for x in extra:eligible[x['frame']]=dict(native_id=x['expected_new_event']['id'],frame=x['frame'],wire_SHA256=x['wire_SHA256'])
 ids={x['native_id']for x in eligible.values()};events={}
 for line in sdk.open(encoding='utf8'):
  e=json.loads(line)
  if e['id']in ids:assert e['id']not in events;events[e['id']]=e
 assert events.keys()==ids
 argv=[str(a.tshark),'-n','-r',str(capture),'-Y','arp or icmp or icmpv6','-T','fields','-E','header=y','-E','separator=\t','-E','quote=d','-E','occurrence=a','-E','aggregator=|']
 for field in fields:argv+=['-e',field]
 target=folder/'wireshark.tsv';started=time.monotonic()
 with target.open('xb')as out,(folder/'wireshark.stderr').open('xb')as err:run=subprocess.run(argv,stdout=out,stderr=err,timeout=180)
 assert run.returncode==0
 ws={}
 with target.open(encoding='utf8',newline='')as src:
  for row in csv.DictReader(src,delimiter='\t'):
   frame=int(row['frame.number']);assert frame not in ws;ws[frame]=row
 checks=collections.Counter();native_mismatches=[];ws_differences=[];metadata_bound=0;scopes=collections.Counter();lines=[]
 for frame,rec in enumerate(capture_records(capture),1):
  if frame not in eligible:continue
  old=eligible[frame];e=events[old['native_id']];w=rec['wire'];rec['frame']=frame
  assert hashlib.sha256(w).hexdigest()==old['wire_SHA256']==e['source_bytes']['SHA256']and base64.b64decode(e['raw'])==w
  refs=e['source_bytes']['PacketRefs'];assert len(refs)==1 and refs[0]['Number']==frame
  loc=parent(rec)
  if not loc['covered']and e['protocol']=='icmpv6':loc=source_profile(rec)
  assert loc['covered']and loc['protocol']==e['protocol'];assert refs[0]['Domain']==e['domain']==dict(Section=rec['section'],Interface=rec['interface'],Encapsulation=('/mpacket:express'if rec['link']==274 else'')+''.join('/vlan:%d'%v for v in loc.get('vlans',[])));body=w[loc['offset']:loc['end']];at=loc['offset'];protocol=e['protocol'];session=e['session'];want=[]
  def item(key,value,field,offset,size):want.append(dict(key=key,literal=value,ws_field=field,absolute_offset=offset,length=size))
  if protocol=='arp':
   assert len(body)>=8;hl,pl=body[4:6];assert len(body)>=8+2*(hl+pl);item('Operation',int.from_bytes(body[6:8],'big'),'arp.opcode',at+6,2)
   if body[2:4]==b'\x08\x00'and hl==6 and pl==4:
    item('Sender MAC',':'.join('%02x'%x for x in body[8:14]),'arp.src.hw_mac',at+8,6);item('Sender IP',ip(body[14:18]),'arp.src.proto_ipv4',at+14,4);item('Target IP',ip(body[24:28]),'arp.dst.proto_ipv4',at+24,4)
  else:
   assert len(body)>=4;typ,code=body[:2];prefix='icmp'if protocol=='icmp'else'icmpv6';item('Type',typ,prefix+'.type',at,1);item('Code',code,prefix+'.code',at+1,1)
   if (protocol=='icmp'and typ in(0,8)or protocol=='icmpv6'and typ in(128,129))and len(body)>=8:
    item('Identifier',int.from_bytes(body[4:6],'big'),'icmp.ident'if protocol=='icmp'else'icmpv6.echo.identifier',at+4,2);item('Sequence',int.from_bytes(body[6:8],'big'),'icmp.seq'if protocol=='icmp'else'icmpv6.echo.sequence_number',at+6,2)
   if protocol=='icmpv6'and typ==2 and len(body)>=8:item('MTU',int.from_bytes(body[4:8],'big'),'icmpv6.mtu',at+4,4)
   if protocol=='icmpv6'and typ in(135,136,137)and len(body)>=24:item('Target',ip(body[8:24]),{135:'icmpv6.nd.ns.target_address',136:'icmpv6.nd.na.target_address',137:'icmpv6.nd.rd.target_address'}[typ],at+8,16)
   if protocol=='icmpv6'and typ==137 and len(body)>=40:item('Destination',ip(body[24:40]),'icmpv6.rd.na.destination_address',at+24,16)
  scope='validated canonical direct/simple carrier'if loc.get('family_consistent')else loc.get('interpretation_scope','cross-family own-header observation; invalid family association')
  scopes[scope]+=1;ts=ws.get(frame);record=dict(frame=frame,id=e['id'],protocol=protocol,status=e['status'],scope=scope,section=rec['section'],interface=rec['interface'],wire_SHA256=sha(capture)if False else old['wire_SHA256'],own_offset=at,own_extent=len(body),fields=[])
  if ts:
   assert Decimal(ts['frame.time_epoch'])*Decimal(1000000000)==rec['time_ns']and int(ts['frame.cap_len'])==rec['captured_length']and int(ts['frame.len'])==rec['original_length'];metadata_bound+=1
  for test in want:
   key=test['key'];expected=test['literal'];actual=session.get(key);test['native']=actual;test['native_type']=type(actual).__name__;test['native_matches_literal']=typed_equal(actual,expected)
   if not test['native_matches_literal']:native_mismatches.append(dict(frame=frame,id=e['id'],**test))
   observed=ts.get(test['ws_field'],'').split('|')[0]if ts else'';test['ws_literal_first']=observed
   if not observed:relation='absent'
   else:
    try:parsed=scalar(observed)if type(expected)is int else str(ipaddress.ip_address(observed))if key in('Target','Destination')else observed.lower()if key=='Sender MAC'else observed
    except ValueError:parsed=observed
    relation='equal'if typed_equal(parsed,expected)else'different'
   test['ws_relation']=relation;checks[(protocol,key,relation)]+=1
   if relation!='equal':ws_differences.append(dict(frame=frame,id=e['id'],scope=scope,protocol=protocol,key=key,literal=expected,ws_first=observed,ws_protocols=ts.get('frame.protocols')if ts else None,ws_diagnostic=ts.get('_ws.expert.message')if ts else None))
   record['fields'].append(test)
  lines.append(record)
 proof=folder/'source-fields.jsonl';proof.write_text(''.join(json.dumps(x,sort_keys=True)+'\n'for x in lines),encoding='utf8')
 different=folder/'ws-differences.jsonl';different.write_text(''.join(json.dumps(x,sort_keys=True)+'\n'for x in ws_differences),encoding='utf8')
 item=dict(corpus=name,checked_source_observations=len(lines),scope_counts=dict(scopes),native_scalar_mismatches=native_mismatches,source_metadata_matches_WS_frames=metadata_bound,WS_fields=[dict(protocol=k[0],field=k[1],relation=k[2],count=v)for k,v in sorted(checks.items())],WS_difference_count=len(ws_differences),capture_SHA256=sha(capture),native_SHA256=sha(sdk),producer_SHA256=sha(producer),source_ledger_SHA256=sha(proof),WS_difference_SHA256=sha(different),WS_export_SHA256=sha(target),WS_arguments=argv,WS_exit=run.returncode,seconds=round(time.monotonic()-started,3));allresults.append(item)
 print(json.dumps({k:item[k]for k in('corpus','checked_source_observations','scope_counts','native_scalar_mismatches','source_metadata_matches_WS_frames','WS_difference_count')}),flush=True)
summary=dict(scope='Literal original-source native key-field correctness and literal TShark differential inventory. Cross-family/zero-length legacy observations are explicitly separate, not valid carrier claims. Of the original 391 census exclusions, exactly source285780 is explicitly covered by the63 source extension; the remaining390 remain uncovered.',source=a.source,auditor_SHA256=sha(Path(__file__)),tshark_SHA256=sha(a.tshark),bins_receipt_SHA256=sha(receipt),census_summary_SHA256=sha(a.census/'summary.json'),source_63_ledger_SHA256=sha(a.source_ledger),whole_SDK_bridge_SHA256=sha(a.bridge),corpora=allresults)
p=a.out/'summary.json';p.write_text(json.dumps(summary,indent=2,sort_keys=True)+'\n',encoding='utf8');print(json.dumps(dict(summary=str(p),SHA256=sha(p))),flush=True)
assert not any(x['native_scalar_mismatches']for x in allresults)
