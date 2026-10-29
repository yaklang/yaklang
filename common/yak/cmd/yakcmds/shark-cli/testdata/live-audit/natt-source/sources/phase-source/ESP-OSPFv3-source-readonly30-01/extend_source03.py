"""All 417 OSPFv3 sources and original Intra-Area-Prefix byte/bit differential.

Models only the pinned dependency's explicit IntraAreaPrefix offset arithmetic;
this is a source-level Python differential, not execution of its Go decoder.
"""
from pathlib import Path
import base64,collections,csv,hashlib,importlib.util,ipaddress,json,sys
root=Path(r'C:\tmp\yak-shark-ESP-SSH-UI-20261028\ESP-OSPFv3-source-readonly30-01')
out=root/'source03';out.mkdir(exist_ok=False)
spec=importlib.util.spec_from_file_location('oracle',root/'audit_source02.py');o=importlib.util.module_from_spec(spec);spec.loader.exec_module(o)
facts_path=root/'source02/source-facts.json';base=json.loads(facts_path.read_text())
reader=Path(base['reader']);sys.path.insert(0,str(reader.parent))
spec=importlib.util.spec_from_file_location('reader',reader);reader_module=importlib.util.module_from_spec(spec);spec.loader.exec_module(reader_module)
ws={int(r[0]):r for r in csv.reader(Path(base['Wireshark_CSV']).open(encoding='utf-8-sig',newline=''))}
by_frame={r['frame']:r for r in base['selected']};all_v3=[];all_natt=[];counts=collections.Counter();predicted_failures=[]
dependency=Path(r'C:\Users\V\go\pkg\mod\github.com\gopacket\gopacket@v1.3.1\layers\ospf.go')
source=dependency.read_text();assert 'prefixOffset = prefixOffset + 4 + uint32(prefixLen)' in source
for number,record in enumerate(reader_module.capture_records(Path(base['capture'])),1):
 try:ip=o.outer(record)
 except (AssertionError,ValueError,IndexError):continue
 if not ip:continue
 if 'NATT' in ip:
  all_natt.append(dict(frame=number,section=record['section'],interface=record['interface'],link=record['link'],
   captured_length=record['captured_length'],original_length=record['original_length'],time_ns=record['time_ns'],
   frame_wire_SHA256=o.digest(record['wire']),Wireshark_label=ws[number][5],Wireshark_stack=ws[number][6],source=ip))
 if ip.get('OSPF',{}).get('version')!=3:continue
 p=ip['OSPF'];counts['type'+str(p['type'])]+=1;counts['checksums_valid']+=p['checksum_residual']==0
 assert ws[number][5]=='OSPF'
 row=dict(frame=number,section=record['section'],interface=record['interface'],link=record['link'],
  captured_length=record['captured_length'],original_length=record['original_length'],time_ns=record['time_ns'],
  frame_wire_SHA256=o.digest(record['wire']),Wireshark_label=ws[number][5],Wireshark_stack=ws[number][6],
  missing_current_presentation=number in base['gap_frames'],source=ip,IntraAreaPrefixes=[],legacy_IntraAreaPrefix_model=[])
 raw=record['wire'];ospf_end=p['declared_range'][1]
 for lsa_index,lsa in enumerate(p.get('LSAs',[])):
  counts['full_LSA_headers']+=1;counts['full_LSA_checksums_valid']+=lsa['fletcher']['valid']
  if lsa['LS_type']!=0x2009:continue
  start=lsa['header_range'][0];end=start+lsa['length'];view=raw[start:ospf_end]
  assert start+32<=end
  count=o.uint(raw,start+20);n=start+32;prefixes=[]
  for k in range(count):
   assert n+4<=end;bits=raw[n];assert bits<=128
   stored=((bits+31)//32)*4;assert n+4+stored<=end
   prefix=raw[n+4:n+4+stored]+bytes(16-stored)
   prefixes.append(dict(index=k,prefix_length=bits,prefix_options=raw[n+1],metric=o.uint(raw,n+2),
    encoded_octets=stored,header_range=[n,n+4],prefix_range=[n+4,n+4+stored],
    prefix=str(ipaddress.IPv6Address(prefix))+'/'+str(bits),next_header_offset=n+4+stored))
   n+=4+stored
  assert n==end
  row['IntraAreaPrefixes'].append(dict(LSA_index=lsa_index,LSA_range=[start,end],LSA_length=lsa['length'],
   count=count,count_range=[start+20,start+22],referenced_LS_type=o.uint(raw,start+22),
   referenced_link_state_ID=o.addr(raw[start+24:start+28]),referenced_advertising_router=o.addr(raw[start+28:start+32]),prefixes=prefixes))
  legacy_offset=32;trace=[];failure=None
  for k in range(count):
   if legacy_offset>=len(view):
    failure=dict(index=k,operation='read prefix length',offset=legacy_offset,remaining_decoder_view=len(view),
     LSA_length=lsa['length'],absolute_offset=start+legacy_offset);break
   bits=view[legacy_offset]
   if legacy_offset+4+bits//8>len(view):
    failure=dict(index=k,operation='slice address prefix',offset=legacy_offset,observed_length_bits=bits,
     claimed_slice_end=legacy_offset+4+bits//8,remaining_decoder_view=len(view),LSA_length=lsa['length'],absolute_offset=start+legacy_offset);break
   trace.append(dict(index=k,legacy_offset=legacy_offset,absolute_offset=start+legacy_offset,
    observed_length_bits=bits,legacy_next_offset=legacy_offset+4+bits,
    correct_offset=prefixes[k]['header_range'][0]-start,correct_next_offset=prefixes[k]['next_header_offset']-start,
    matches_actual_prefix_start=start+legacy_offset==prefixes[k]['header_range'][0]))
   legacy_offset+=4+bits
  row['legacy_IntraAreaPrefix_model'].append(dict(LSA_index=lsa_index,LSA_range=[start,end],
   decoder_view_range=[start,ospf_end],count=count,trace=trace,failure=failure,
   source_level_model_not_Go_execution=True))
  if failure and number not in predicted_failures:predicted_failures.append(number)
 all_v3.append(row)
assert len(all_v3)==417 and len(all_natt)==34
actual_failed=[r['frame'] for r in all_v3 if r['missing_current_presentation']]
assert sorted(predicted_failures)==sorted(actual_failed)
sdk=Path(r'C:\tmp\yak-shark-ESP-SSH-UI-20261028\runtime-public-native-30\preservation\weber\events.jsonl')
natt_esp={r['frame'] for r in all_natt if r['source']['NATT']['kind']=='ESP-clear-header'}
sdk_events=[]
for raw in sdk.open('rb'):
 e=json.loads(raw);refs=e['source_bytes'].get('PacketRefs') or []
 match={r['Number'] for r in refs}&natt_esp
 if match:
  assert len(match)==1 and len(refs)==1
  number=next(iter(match));r=next(x for x in all_natt if x['frame']==number);u=r['source']['UDP'];n=r['source']['NATT']
  prefix=base64.b64decode(e['raw']);wire=(root/'source02'/by_frame[number]['raw_frame_file']).read_bytes()[slice(*u['payload_range'])]
  assert wire[:len(prefix)]==prefix and len(prefix)==64 and len(wire)==96
  assert e['length']==96 if 'length' in e else True
  assert e['source_bytes']['SHA256']==o.digest(prefix)
  sdk_events.append(dict(frame=number,before_event=e,before_literal_object_SHA256=o.digest(raw.rstrip(b'\r\n')),
   source_ESP_wire_range=u['payload_range'],source_full_ESP_length=len(wire),source_full_ESP_SHA256=o.digest(wire),
   source_full_ESP_base64=base64.b64encode(wire).decode(),source_ESP_clear_fields=n['ESP'],
   original_ID_preservable=True,unchanged_identity_fields=['id','domain','source','destination','direction','flow','response_to','transaction','transport'],
   explicit_required_source_change='Raw and SourceBytes SHA256 must expand from first64 preview to full96 observed UDP payload; SourceBytes packet/domain association unchanged'))
assert len(sdk_events)==7
for name,value in [('all-417-OSPFv3-source-facts.json',all_v3),('all-34-NATT-source-facts.json',all_natt),('all-7-ESP-before-whole-source-plan.json',sdk_events)]:
 (out/name).write_text(json.dumps(value,ensure_ascii=False,indent=2)+'\n',encoding='utf8')
summary=dict(scope=__doc__,auditor_SHA256=o.sha(__file__),source02_SHA256=o.sha(facts_path),
 dependency=str(dependency),dependency_SHA256=o.sha(dependency),
 capture_SHA256=o.sha(Path(base['capture'])),SDK_SHA256=o.sha(sdk),all_v3_count=len(all_v3),counts=dict(counts),
 failed_v3_frames=actual_failed,source_level_legacy_prefix_failure_predictions=predicted_failures,
 legacy_arithmetic_explains_all_and_only_22_missing_v3_frames=True,all_natt_count=len(all_natt),
 outputs={p.name:o.sha(p) for p in out.glob('*.json')})
(out/'summary.json').write_text(json.dumps(summary,indent=2)+'\n',encoding='utf8')
print(json.dumps(dict(result=str(out/'summary.json'),SHA256=o.sha(out/'summary.json'),counts=dict(counts),explains_exact_22=True)))
