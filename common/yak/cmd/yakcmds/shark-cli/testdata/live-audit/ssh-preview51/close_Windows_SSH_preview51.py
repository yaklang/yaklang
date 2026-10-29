"""Close three actual UI51 console cases and unchanged native/CLI byte exports."""
from pathlib import Path
import hashlib,importlib.util,json,re,struct,subprocess
root=Path(__file__).parent
repo=Path(r'C:\Users\V\.codex\worktrees\shark-protocol-infrastructure\yaklang')
source='7fcbf0ac931c6881fd5a3baf4eba8d5c9c9889f8';oldsource='26f01f4baa5fd39b88deadf901257a70bc5c1dfe'
actual=root/'windows-SSH-preview-7fcb51-01'
def sha(p):return hashlib.sha256(Path(p).read_bytes()).hexdigest()
def read(p):return json.loads(Path(p).read_text('utf-8-sig'))
plan=root/'Windows-SSH-preview-source-plan51-01/plan.json';p=read(plan)
assert sha(plan)=='d595f04fc3afebe6381ed7f3bb847062e41c677adbf2c9bf580edc25e7f05a54'
audit=root/'SSH-role-native-7fcb51-01/independent-audit03/summary.json';assert read(audit)['passed']
spec=importlib.util.spec_from_file_location('preview51_console_hex',root/'verify_Windows_OSPFv3_36.py');m=importlib.util.module_from_spec(spec);spec.loader.exec_module(m)
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
 assert len(frames)==115 and physical[54:]==raw
 assert len(physical)==c['full_physical_bytes']and hashlib.sha256(physical).hexdigest()==c['full_physical_SHA256']
 shots={}
 for name,h in proof['snapshots'].items():
  f=actual/c['name']/(name+'.json');assert sha(f)==h['cells_SHA256']and sha(f.with_suffix('.png'))==h['PNG_SHA256']
  shots[name]=read(f)
 for name in('01-protocol-fields','02-first-byte-source','02-byte-sources'):
  text='\n'.join(shots[name]['Rows'])
  assert f"Frame {c['frame']}: {len(physical)} bytes on wire, {len(physical)} captured"in text
  assert ' › '.join(c['expected_stack'])in text and'· 1 visible'in text
  assert f"{c['required_group_kind']} #{proof['source_PDU_id']}"in text
 assert m.pane(shots['01-protocol-fields'])[0]==physical
 assert m.pane(shots['02-byte-sources'])[0]==physical
 assert m.pane(shots['02-first-byte-source'])[0]==retained
 text='\n'.join(shots['01-protocol-fields']['Rows'])
 for word in c['required_text']:assert word in text,word
 for word in c['forbidden_text']:assert word not in text,word
 assert c['source_caption']in'\n'.join(shots['02-first-byte-source']['Rows'])
 assert re.search(r'buffer\s+115\s*/\s*20000',text)
 if c['opaque']:
  assert 'reassembled PDU'not in'\n'.join(shots['02-first-byte-source']['Rows'])
  assert f"PDU #{proof['source_PDU_id']}"not in text
 else:
  selected=shots['01-selected-field'];selected_text='\n'.join(selected['Rows'])
  assert c['select_field']+': '+c['select_value']in selected_text and'Status: malformed'in selected_text
  first,pos=m.pane(shots['01-protocol-fields']);observed,spos=m.pane(selected);assert observed==first==physical
  assert[shots['01-protocol-fields']['Attributes'][y][x]for y,x in pos]==[selected['Attributes'][y][x]for y,x in spos]
 assert read(actual/c['name']/'restored.json')==dict(error='<nil>',input_cp=936,output_cp=936)
 cases.append(dict(name=c['name'],physical_frame=c['frame'],generated_event_id=proof['source_PDU_id'],
  entire_physical_bytes=len(physical),logical_bytes=len(raw),retained_bytes=len(retained),
  whole_physical_and_retained_sources_exact=True,source_caption=c['source_caption'],
  caption_and_group_scope_correct=True,derived_role_metadata_has_no_false_highlight=not c['opaque'],
  producer_SHA256=sha(producer)))
binary_bridges=[]
for kind in('events','flows'):
 a=root/'bins-50'/(kind+'.exe');b=root/'bins-51'/(kind+'.exe')
 assert sha(a)==sha(b)
 for generation in('50','51'):
  r=read(root/('bins-'+generation)/(kind+'-receipt.json'))
  assert r['exit_code']==0 and r['binary_SHA256']==sha(root/('bins-'+generation)/(kind+'.exe'))
 binary_bridges.append(dict(kind=kind,SHA256=sha(a),byte_identical=True))
exports=[]
for name in('events.jsonl','yak.jsonl','field-descriptors.json'):
 a=root/'SSH-role-native-26f50-01'/name;b=root/'SSH-role-native-7fcb51-01'/name
 assert sha(a)==sha(b)
 exports.append(dict(name=name,SHA256=sha(a),byte_identical=True))
assert not subprocess.check_output(['git','diff',oldsource,source,'--','common/pcapx/pcaputil','common/bin-parser','go.mod','go.sum'],cwd=repo)
out=root/'Windows-SSH-preview-root-closure51-01';out.mkdir(exist_ok=False)
result=dict(source=source,source_plan_SHA256=sha(plan),SDK_audit_SHA256=sha(audit),cases=cases,
 physical_frames=115,actual_console_cases=3,passed=True,all_original_frame_metadata_preserved=True,
 opaque_arrivals_never_named_complete_PDU=True,root_viewed_all_three_actual_case_images=True,
 console_cells_not_Terminal_pixels=True,CP936_mouse_tabs_resize_checked=True,default_cache_packets=20000,
 default_history_mib=64,native_binary_bridges=binary_bridges,closed_SDK_CLI_export_bridges=exports,
 backend_Go_and_rules_unchanged_from_26f=True,fresh37_case_execution_for_UI51_not_claimed=True,
 physical_tail_not_in_retained_preview_hash=True,checker_SHA256=sha(__file__))
with(out/'summary.json').open('x',encoding='utf8')as f:json.dump(result,f,indent=2);f.write('\n')
print(json.dumps(dict(summary=str(out/'summary.json'),SHA256=sha(out/'summary.json'),passed=True,cases=3)))
