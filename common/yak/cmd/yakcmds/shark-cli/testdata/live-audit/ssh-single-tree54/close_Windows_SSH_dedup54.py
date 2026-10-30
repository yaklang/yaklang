"""Close the single-tree Windows page and its independent literal source spans."""
from pathlib import Path
import hashlib,importlib.util,json,re,struct
P=Path(__file__).parent
sha=lambda p:hashlib.sha256(Path(p).read_bytes()).hexdigest()
read=lambda p:json.loads(Path(p).read_text('utf-8-sig'))
source='92620d72b0c8a1d62ce923e4e69cc1fad5d4c66d'
actual=P/'windows-SSH-dedup-92654-01';plan=P/'Windows-SSH-ranges-source-plan53-01/plan.json'
assert sha(plan)=='10e245fb8061619c3f79cb9bea68fbe0d47dbcbcc3acf394a2ea1c70375052d2'
audit=P/'SSH-dedup-native-92654-01/source50/independent-audit/summary.json';assert read(audit)['passed']and read(audit)['source']==source
spec=importlib.util.spec_from_file_location('ssh_dedup_console54',P/'verify_Windows_OSPFv3_36.py');m=importlib.util.module_from_spec(spec);spec.loader.exec_module(m)
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
 initial='\n'.join(shots['01-protocol-fields']['Rows'])
 assert f"Frame {case['frame']}: {len(physical)} bytes on wire, {len(physical)} captured"in initial
 pattern=r'(\d+)\s+\d{2}:\d{2}:\d{2}\.\d{3}\s+SSH\s+\S+\s+\S+\s+(\d+)\s+'
 packet_rows=[row for row in shots['01-protocol-fields']['Rows']if re.search(pattern,row)]
 assert len(packet_rows)==1;match=re.search(pattern,packet_rows[0]);assert int(match.group(1))==case['frame']and int(match.group(2))==len(physical)
 for name in('01-protocol-fields','01-selected-field','02-first-byte-source','02-byte-sources'):
  text='\n'.join(shots[name]['Rows']);assert packet_rows[0]in shots[name]['Rows']and' › '.join(case['expected_stack'])in text and'· 1 visible'in text
  assert 'Initial Packet Fields'not in text,'redundant copied tree remains visible'
 assert m.pane(shots['01-protocol-fields'])[0]==physical
 if case['expected_PDU_span']:
  assert m.pane(shots['02-first-byte-source'])[0]==physical and m.pane(shots['02-byte-sources'])[0]==raw
 else:
  assert m.pane(shots['02-first-byte-source'])[0]==raw and m.pane(shots['02-byte-sources'])[0]==physical
 selected=shots['01-selected-field'];observed,coords=m.pane(selected);backgrounds=[(selected['Attributes'][y][x]>>4)&15 for y,x in coords]
 assert case['select_field']+': '+case['select_value']in'\n'.join(selected['Rows'])
 span=case['expected_PDU_span']
 if span:
  assert observed==raw and[i for i,bg in enumerate(backgrounds)if bg]==list(range(*span))and all(bg in(0,1)for bg in backgrounds)
  assert raw[span[0]:span[1]]==case['select_value'].encode('ascii')
 else:
  assert observed==physical and not any(backgrounds)
  _,original_coords=m.pane(shots['01-protocol-fields'])
  assert[selected['Attributes'][y][x]for y,x in coords]==[shots['01-protocol-fields']['Attributes'][y][x]for y,x in original_coords]
 assert re.search(r'buffer\s+115\s*/\s*20000',initial)
 assert read(actual/case['name']/'restored.json')==dict(error='<nil>',input_cp=936,output_cp=936)
 cases.append(dict(name=case['name'],frame=case['frame'],PDU_span=span,physical_bytes=len(physical),PDU_bytes=len(raw),exact_entire_sources_and_highlights=True,duplicate_tree_absent_in_actual_views=True,producer_SHA256=sha(proof_file)))
out=P/'Windows-SSH-dedup-root-closure54-01';out.mkdir(exist_ok=False)
result=dict(source=source,passed=True,case_count=3,cases=cases,source_plan_SHA256=sha(plan),SDK_audit_SHA256=sha(audit),root_viewed_all_three_actual_images=True,console_cells_not_Terminal_pixels=True,CP936_mouse_tabs_resize_checked=True,default_cache_packets=20000,default_history_mib=64,single_tree_worker_and_cross_PDU_proof='ssh_detail_dedup_test.go plus targeted/full receipts; viewport absence alone is not an entire-tree enumeration',previous_range_closure_SHA256=sha(P/'Windows-SSH-ranges-root-closure53-04/summary.json'),checker_SHA256=sha(__file__))
(out/'summary.json').write_text(json.dumps(result,indent=2)+'\n',encoding='utf8')
print(json.dumps(dict(summary_SHA256=sha(out/'summary.json'),passed=True,cases=3)))
