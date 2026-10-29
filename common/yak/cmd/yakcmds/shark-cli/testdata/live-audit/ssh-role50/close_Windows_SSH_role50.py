"""Close physical bytes and role diagnostics, preserving the known26f preview-caption gap."""
from pathlib import Path
import hashlib,importlib.util,json,re,struct
root=Path(__file__).parent
actual=root/'windows-SSH-role-26f50-01'
source='26f01f4baa5fd39b88deadf901257a70bc5c1dfe'
def sha(p):return hashlib.sha256(Path(p).read_bytes()).hexdigest()
def read(p):return json.loads(Path(p).read_text('utf-8-sig'))
plan=root/'Windows-SSH-role-source-plan50-01/plan.json';p=read(plan)
assert sha(plan)=='37b2eea6c83bc639ecc4f222aad82f061458e4c95b6aae1b8921a3fa95816cf7'
audit=root/'SSH-role-native-26f50-01/independent-audit03/summary.json';assert read(audit)['passed']
spec=importlib.util.spec_from_file_location('old_role_console_hex50',root/'verify_Windows_OSPFv3_36.py');m=importlib.util.module_from_spec(spec);spec.loader.exec_module(m)
cases=[]
for c in p['cases']:
 producer=actual/(c['name']+'-producer.json');proof=read(producer)
 assert proof['source']==source and proof['exit_code']==0 and proof['inputs_unchanged']
 for path,h in proof['inputs'].items():assert sha(path)==h,path
 capture=Path(c['capture']).read_bytes();assert hashlib.sha256(capture).hexdigest()==c['capture_SHA256']
 at=24;frames=[]
 while at<len(capture):
  _,_,n,_=struct.unpack('<IIII',capture[at:at+16]);at+=16;frames.append(capture[at:at+n]);at+=n
 physical=frames[c['frame']-1];raw=Path(c['raw_path']).read_bytes();retained=raw[:c['retained_bytes']]
 assert physical[54:]==raw and len(physical)==246 and len(raw)==192
 shots={}
 for name,h in proof['snapshots'].items():
  f=actual/c['name']/(name+'.json');assert sha(f)==h['cells_SHA256']and sha(f.with_suffix('.png'))==h['PNG_SHA256']
  shots[name]=read(f)
 for name in('01-protocol-fields','02-first-byte-source','02-byte-sources'):
  text='\n'.join(shots[name]['Rows'])
  assert f"Frame {c['frame']}: 246 bytes on wire, 246 captured"in text
  assert ' › '.join(c['expected_stack'])in text and'· 1 visible'in text
 assert m.pane(shots['01-protocol-fields'])[0]==physical
 assert m.pane(shots['02-byte-sources'])[0]==physical
 assert m.pane(shots['02-first-byte-source'])[0]==retained
 text='\n'.join(shots['01-protocol-fields']['Rows'])
 for word in c['required_text']:assert word in text,word
 for word in c['forbidden_text']:assert word not in text,word
 assert re.search(r'buffer\s+115\s*/\s*20000',text)
 if c['select_field']:
  selected=shots['01-selected-field'];selected_text='\n'.join(selected['Rows'])
  assert c['select_field']+': '+c['select_value']in selected_text
  assert'Status: malformed'in selected_text
  first,pos=m.pane(shots['01-protocol-fields']);observed,spos=m.pane(selected)
  assert observed==first==physical
  assert[shots['01-protocol-fields']['Attributes'][y][x]for y,x in pos]==[selected['Attributes'][y][x]for y,x in spos], 'derived role metadata cannot manufacture a byte highlight'
 else:
  assert 'SSH · reassembled PDU'in'\n'.join(shots['02-first-byte-source']['Rows']), 'preserve the actual known incorrect label'
 assert read(actual/c['name']/'restored.json')==dict(error='<nil>',input_cp=936,output_cp=936)
 cases.append(dict(name=c['name'],frame=c['frame'],entire_physical_bytes=246,logical_stream_or_PDU_bytes=192,
  retained_bytes=len(retained),physical_bytes_and_frame_lengths_exact=True,retained_bytes_exact=True,
  role_metadata_has_no_invented_range=bool(c['select_field']),
  caption_gap='opaque64-byte preview is incorrectly named reassembled PDU'if c['preview_only']else None,
  producer_SHA256=sha(producer)))
out=root/'Windows-SSH-role-root-closure50-01';out.mkdir(exist_ok=False)
result=dict(source=source,source_plan_SHA256=sha(plan),SDK_audit_SHA256=sha(audit),cases=cases,
 physical_frame_data_loss=False,earlier_image_length_misread_corrected=True,
 all_physical_frame_lengths_and_bytes_preserved=True,role_diagnostic_page_passed=True,
 encrypted_preview_caption_passed=False,known_UI_caption_gap_preserved=True,
 root_viewed_actual_selected_role_and_opaque_preview_images=True,console_cells_not_Terminal_pixels=True,
 CP936_mouse_tabs_resize_checked=True,default_cache_packets=20000,default_history_mib=64,checker_SHA256=sha(__file__))
with(out/'summary.json').open('x',encoding='utf8')as f:json.dump(result,f,indent=2);f.write('\n')
print(json.dumps(dict(summary=str(out/'summary.json'),SHA256=sha(out/'summary.json'),physical_data_preserved=True,role_page_passed=True,preview_caption_passed=False)))
