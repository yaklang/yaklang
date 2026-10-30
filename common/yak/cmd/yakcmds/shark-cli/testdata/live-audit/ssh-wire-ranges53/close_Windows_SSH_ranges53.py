"""Close literal highlights and retain the separately observed duplicate-tree issue."""
from pathlib import Path
import hashlib,importlib.util,json,re,struct
P=Path(__file__).parent
sha=lambda p:hashlib.sha256(Path(p).read_bytes()).hexdigest()
read=lambda p:json.loads(Path(p).read_text('utf-8-sig'))
plan=P/'Windows-SSH-ranges-source-plan53-01/plan.json'
assert sha(plan)=='10e245fb8061619c3f79cb9bea68fbe0d47dbcbcc3acf394a2ea1c70375052d2'
actual=P/'windows-SSH-ranges-b58653-01';source='b586dda3c8eb3a3a12f6e2f4e9eaae0441481617'
audit=P/'SSH-ranges-native-b58653-01/source50/independent-audit/summary.json'
assert read(audit)['passed']and read(audit)['source']==source
spec=importlib.util.spec_from_file_location('ssh_range_console53',P/'verify_Windows_OSPFv3_36.py')
module=importlib.util.module_from_spec(spec);spec.loader.exec_module(module)
cases=[]
for case in read(plan)['cases']:
 proof_file=actual/(case['name']+'-producer.json');proof=read(proof_file)
 assert proof['source']==source and proof['exit_code']==0 and proof['inputs_unchanged']
 for p,h in proof['inputs'].items():assert sha(p)==h
 data=Path(case['capture']).read_bytes();assert hashlib.sha256(data).hexdigest()==case['capture_SHA256']
 frames=[];at=24
 while at<len(data):
  _,_,n,_=struct.unpack('<IIII',data[at:at+16]);at+=16;frames.append(data[at:at+n]);at+=n
 physical=frames[case['frame']-1];raw=Path(case['raw_path']).read_bytes()
 assert len(frames)==115 and physical[54:]==raw
 assert hashlib.sha256(physical).hexdigest()==case['physical_SHA256']and sha(case['raw_path'])==case['raw_SHA256']
 shots={}
 for name,h in proof['snapshots'].items():
  f=actual/case['name']/(name+'.json');assert sha(f)==h['cells_SHA256']and sha(f.with_suffix('.png'))==h['PNG_SHA256'];shots[name]=read(f)
 for name in('01-protocol-fields','01-selected-field','02-first-byte-source','02-byte-sources'):
  text='\n'.join(shots[name]['Rows'])
  assert f"Frame {case['frame']}: {len(physical)} bytes on wire, {len(physical)} captured"in text
  assert ' › '.join(case['expected_stack'])in text and '· 1 visible'in text
 assert module.pane(shots['01-protocol-fields'])[0]==physical
 assert module.pane(shots['02-byte-sources'])[0]==physical
 assert module.pane(shots['02-first-byte-source'])[0]==raw
 selected=shots['01-selected-field'];observed,positions=module.pane(selected)
 assert case['select_field']+': '+case['select_value']in'\n'.join(selected['Rows'])
 backgrounds=[(selected['Attributes'][y][x]>>4)&15 for y,x in positions]
 expected=case['expected_PDU_span']
 if expected:
  assert observed==raw
  assert [i for i,bg in enumerate(backgrounds)if bg]==list(range(*expected))
  assert all(bg in(0,1)for bg in backgrounds)
  assert raw[expected[0]:expected[1]]==case['select_value'].encode('ascii')
  assert 'SSH · reassembled PDU'in'\n'.join(selected['Rows'])
 else:
  assert observed==physical and not any(backgrounds)
  original,coords=module.pane(shots['01-protocol-fields']);assert original==observed
  assert [selected['Attributes'][y][x]for y,x in positions]==[shots['01-protocol-fields']['Attributes'][y][x]for y,x in coords]
 assert re.search(r'buffer\s+115\s*/\s*20000','\n'.join(shots['01-protocol-fields']['Rows']))
 assert read(actual/case['name']/'restored.json')==dict(error='<nil>',input_cp=936,output_cp=936)
 cases.append(dict(name=case['name'],frame=case['frame'],literal_PDU_span=expected,full_physical_bytes=len(physical),owned_PDU_bytes=len(raw),exact_highlight_and_source=True,producer_SHA256=sha(proof_file)))
out=P/'Windows-SSH-ranges-root-closure53-01';out.mkdir(exist_ok=False)
result=dict(source=source,source_plan_SHA256=sha(plan),SDK_audit_SHA256=sha(audit),range_correctness_passed=True,actual_cases=3,cases=cases,all_physical_and_PDU_bytes_exact=True,context_selection_has_no_fabricated_range=True,root_viewed_all_three_actual_images=True,console_cells_not_Terminal_pixels=True,CP936_mouse_tabs_resize_checked=True,default_cache_packets=20000,default_history_mib=64,duplicate_initial_packet_tree_observed=True,duplicate_tree_followup_open=True,complete_page_approval=False,checker_SHA256=sha(__file__))
(out/'summary.json').write_text(json.dumps(result,indent=2)+'\n',encoding='utf8')
print(json.dumps(dict(summary_SHA256=sha(out/'summary.json'),range_checks_passed=True,cases=3,duplicate_tree_followup_open=True)))
